// Package obsidian écrit un univers en notes Markdown pour un vault Obsidian : une note par fiche,
// rangée comme le vault que lit outils/vault_vers_cofdm.py (01 - Monde, 02 - PNJ,
// 04 - Objets & économie, 05 - Campagnes), avec ses images, la chronologie, les récapitulatifs des
// combats et le journal.
//
// Le paquet ne filtre rien : tout ce qu'il reçoit part dans l'export. C'est au serveur de retirer
// d'abord ce que les joueurs ne doivent pas voir (internal/serveur/obsidian.go).
package obsidian

import (
	"archive/zip"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Tchyze/cof_dm/internal/combat"
	"github.com/Tchyze/cof_dm/internal/modele"
	"github.com/Tchyze/cof_dm/internal/store"
)

// Combat : un combat terminé, avec son récapitulatif.
type Combat struct {
	ID   int64
	Etat *combat.Etat
}

// Univers : ce qui part dans l'export, déjà filtré.
type Univers struct {
	Calendrier modele.Calendrier
	Fiches     []store.Entite
	Relations  []store.RelationBrute
	Medias     []store.Media
	Monstres   map[int64]string // nom des monstres du bestiaire, par identifiant
	Combats    []Combat
	Journal    []store.LigneJournal // le plus récent en tête
	// MJ : export du MJ. Un lien [[Nom]] vers une fiche absente de l'export y reste un lien ; dans
	// l'export des joueurs, il devient du texte (la fiche existe peut-être, mais n'est pas partagée).
	MJ bool
}

// Fichier : une note ou une image, à son chemin dans le vault.
type Fichier struct {
	Chemin  string
	Contenu []byte
}

// Ecrire produit le zip de l'export : tout est rangé sous le dossier racine.
func Ecrire(w io.Writer, racine string, u Univers) error {
	racine = nomFichier(racine)
	z := zip.NewWriter(w)
	maintenant := time.Now()
	for _, f := range Notes(u) {
		h := &zip.FileHeader{Name: racine + "/" + f.Chemin, Method: zip.Deflate, Modified: maintenant}
		if !strings.HasSuffix(f.Chemin, ".md") {
			h.Method = zip.Store // images : déjà compressées
		}
		fw, err := z.CreateHeader(h)
		if err != nil {
			return err
		}
		if _, err := fw.Write(f.Contenu); err != nil {
			return err
		}
	}
	return z.Close()
}

// ---------------------------------------------------------------- noms de fichiers

// nomFichier : un nom de note acceptable partout (Windows, Linux, liens d'Obsidian).
func nomFichier(s string) string {
	s = strings.Map(func(r rune) rune {
		switch {
		case strings.ContainsRune(`\/:*?"<>|#^[]`, r):
			return ' '
		case r < ' ':
			return -1
		}
		return r
	}, s)
	s = strings.Trim(strings.Join(strings.Fields(s), " "), ". ")
	if utf8.RuneCountInString(s) > 100 {
		s = strings.TrimRight(string([]rune(s)[:100]), ". ")
	}
	if s == "" {
		s = "Sans nom"
	}
	return s
}

func cle(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// note : une fiche dans l'export.
type note struct {
	e       store.Entite
	t       *modele.Type
	nom     string // nom du fichier, sans .md, unique dans l'export
	dossier string
}

type exporteur struct {
	u        Univers
	notes    []*note
	parID    map[int64]*note
	parNom   map[string]*note // nom de fiche → note (le premier type dans l'ordre du modèle, comme l'application)
	pris     map[string]bool  // noms de fichiers déjà donnés
	medias   map[int64][]store.Media
	fichiers []Fichier
}

// unique donne un nom de fichier libre, d'après nom ; precision le départage d'un homonyme.
func (x *exporteur) unique(nom, precision string) string {
	n := nomFichier(nom)
	if x.pris[cle(n)] && precision != "" {
		n = nomFichier(nom + " (" + precision + ")")
	}
	for i := 2; x.pris[cle(n)]; i++ {
		n = nomFichier(nom) + " " + strconv.Itoa(i)
	}
	x.pris[cle(n)] = true
	return n
}

// Notes renvoie les fichiers de l'export, triés par chemin.
func Notes(u Univers) []Fichier {
	x := &exporteur{u: u, parID: map[int64]*note{}, parNom: map[string]*note{}, pris: map[string]bool{}, medias: map[int64][]store.Media{}}
	rang := map[string]int{}
	for i, t := range modele.Types {
		rang[t.Cle] = i
	}
	fiches := append([]store.Entite(nil), u.Fiches...)
	sort.SliceStable(fiches, func(i, j int) bool {
		if fiches[i].Archive != fiches[j].Archive {
			return !fiches[i].Archive
		}
		if rang[fiches[i].Type] != rang[fiches[j].Type] {
			return rang[fiches[i].Type] < rang[fiches[j].Type]
		}
		return fiches[i].ID < fiches[j].ID
	})
	for _, e := range fiches {
		t, ok := modele.TypeDe(e.Type)
		if !ok {
			continue
		}
		n := &note{e: e, t: t, nom: x.unique(e.Nom, t.Nom)}
		x.notes = append(x.notes, n)
		x.parID[e.ID] = n
		if _, deja := x.parNom[cle(e.Nom)]; !deja {
			x.parNom[cle(e.Nom)] = n
		}
	}
	for _, m := range u.Medias {
		if _, ok := x.parID[m.Entite]; ok {
			x.medias[m.Entite] = append(x.medias[m.Entite], m)
		}
	}
	for id, l := range x.medias {
		sort.SliceStable(l, func(i, j int) bool {
			if l[i].Principal != l[j].Principal {
				return l[i].Principal
			}
			return l[i].ID < l[j].ID
		})
		x.medias[id] = l
	}
	for _, n := range x.notes {
		n.dossier = x.dossier(n)
	}
	images := x.images()
	combats := x.combats()
	for _, n := range x.notes {
		x.ajouter(n.dossier+"/"+n.nom+".md", x.noteFiche(n, images[n.e.ID], combats[n.e.ID]))
		if fr := texte(n.e.Data["fils_rouges"]); n.e.Type == "campagne" && fr != "" {
			x.ajouter(n.dossier+"/"+x.unique("Fils rouges", n.e.Nom)+".md",
				"---\ntype: fils rouges\ncampagne: "+yaml(x.lienRef(n, false))+"\n---\n# Fils rouges — "+n.e.Nom+"\n\n"+x.reecrire(fr)+"\n")
		}
	}
	x.chronologie()
	if len(u.Journal) > 0 {
		x.journal()
	}
	sort.Slice(x.fichiers, func(i, j int) bool { return x.fichiers[i].Chemin < x.fichiers[j].Chemin })
	return x.fichiers
}

func (x *exporteur) ajouter(chemin, contenu string) {
	x.fichiers = append(x.fichiers, Fichier{Chemin: chemin, Contenu: []byte(contenu)})
}

// campagne : le dossier de la campagne de la fiche, « Sans campagne » sinon.
func (x *exporteur) campagne(n *note) string {
	if c, ok := x.parID[premierID(n.e.Data["campagne"])]; ok && c.e.Type == "campagne" {
		return "05 - Campagnes/" + c.nom
	}
	return "05 - Campagnes/Sans campagne"
}

func (x *exporteur) dossier(n *note) string {
	if n.e.Archive {
		return "Archives/" + nomFichier(n.t.Pluriel)
	}
	switch n.e.Type {
	case "lieu":
		return "01 - Monde/Lieux"
	case "faction":
		return "01 - Monde/Factions"
	case "lore":
		return "01 - Monde/Lore"
	case "evenement":
		return "01 - Monde/Événements"
	case "pnj":
		return "02 - PNJ"
	case "pj":
		return "03 - PJ"
	case "objet":
		return "04 - Objets & économie"
	case "campagne":
		return "05 - Campagnes/" + n.nom
	case "quete":
		// outils/vault_vers_cofdm.py lit « Arc principal » dans le chemin
		sous := map[string]string{"arc principal": "Arc principal", "mission": "Missions"}[texte(n.e.Data["categorie"])]
		if sous == "" {
			sous = "Secondaires"
		}
		return x.campagne(n) + "/Quêtes/" + sous
	case "seance":
		return x.campagne(n) + "/Séances"
	case "rencontre":
		return x.campagne(n) + "/Rencontres"
	case "secret":
		return "06 - Secrets"
	case "regle":
		return "07 - Règles"
	}
	return "Fiches/" + nomFichier(n.t.Pluriel)
}

// images range les images des fiches dans Images/ et renvoie, par fiche, leurs noms de fichiers.
func (x *exporteur) images() map[int64][]string {
	out := map[int64][]string{}
	ids := make([]int64, 0, len(x.medias))
	for id := range x.medias {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		n := x.parID[id]
		for i, m := range x.medias[id] {
			if len(m.Contenu) == 0 {
				continue
			}
			nom := x.unique(fmt.Sprintf("%s - %d", n.nom, i+1), "") + path.Ext(m.Fichier)
			x.fichiers = append(x.fichiers, Fichier{Chemin: "Images/" + nom, Contenu: m.Contenu})
			integre := "![[" + nom + "]]"
			if m.Legende != "" {
				integre += "\n*" + strings.TrimSpace(m.Legende) + "*"
			}
			out[id] = append(out[id], integre)
		}
	}
	return out
}

// ---------------------------------------------------------------- liens

// lienRef : le lien vers la note d'une fiche. Dans un tableau, la barre du texte affiché s'échappe.
func (x *exporteur) lienRef(n *note, tableau bool) string {
	if n.nom == n.e.Nom {
		return "[[" + n.nom + "]]"
	}
	sep := "|"
	if tableau {
		sep = `\|`
	}
	return "[[" + n.nom + sep + n.e.Nom + "]]"
}

func (x *exporteur) lienID(id int64, tableau bool) (string, bool) {
	n, ok := x.parID[id]
	if !ok {
		return "", false
	}
	return x.lienRef(n, tableau), true
}

var reLien = regexp.MustCompile(`(!?)\[\[([^\]\n]+?)\]\]`)

// reecrire met les liens [[Nom]] d'un texte à jour des noms de fichiers de l'export.
func (x *exporteur) reecrire(s string) string {
	return reLien.ReplaceAllStringFunc(strings.TrimSpace(s), func(m string) string {
		sous := reLien.FindStringSubmatch(m)
		bang, interieur := sous[1], sous[2]
		cible, affiche, sep := interieur, "", ""
		if p := strings.Index(cible, `\|`); p >= 0 {
			cible, affiche, sep = cible[:p], cible[p+2:], `\|`
		} else if p := strings.Index(cible, "|"); p >= 0 {
			cible, affiche, sep = cible[:p], cible[p+1:], "|"
		}
		ancre := ""
		if p := strings.Index(cible, "#"); p >= 0 {
			cible, ancre = cible[:p], cible[p:]
		}
		n, ok := x.parNom[cle(cible)]
		if !ok {
			if x.u.MJ || strings.TrimSpace(cible) == "" {
				return m
			}
			if affiche != "" {
				return affiche
			}
			return strings.TrimSpace(cible)
		}
		if affiche == "" && n.nom != strings.TrimSpace(cible) {
			affiche = strings.TrimSpace(cible)
		}
		if sep == "" {
			sep = "|"
		}
		if affiche == "" {
			return bang + "[[" + n.nom + ancre + "]]"
		}
		return bang + "[[" + n.nom + ancre + sep + affiche + "]]"
	})
}

// cellule : un texte d'une ligne dans un tableau Markdown.
func (x *exporteur) cellule(s string) string {
	s = strings.ReplaceAll(x.reecrire(s), `\|`, "|")
	s = strings.ReplaceAll(s, "|", `\|`)
	return strings.Join(strings.Fields(s), " ")
}

// ---------------------------------------------------------------- valeurs

func texte(v any) string { s, _ := v.(string); return strings.TrimSpace(s) }

func premierID(v any) int64 {
	if l := ids(v); len(l) > 0 {
		return l[0]
	}
	return 0
}

func ids(v any) []int64 {
	switch y := v.(type) {
	case float64:
		return []int64{int64(y)}
	case int64:
		return []int64{y}
	case []any:
		var out []int64
		for _, z := range y {
			out = append(out, ids(z)...)
		}
		return out
	}
	return nil
}

func nombre(v any) string {
	switch y := v.(type) {
	case float64:
		return strconv.FormatFloat(y, 'f', -1, 64)
	case int64:
		return strconv.FormatInt(y, 10)
	case int:
		return strconv.Itoa(y)
	}
	return ""
}

// yaml : une chaîne entre guillemets, sur une ligne.
func yaml(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

func (x *exporteur) dateJeu(v any) string {
	m, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	return x.u.Calendrier.Texte(modele.DateJeu{An: int(num(m["an"])), Mois: int(num(m["mois"])), Jour: int(num(m["jour"]))})
}

func num(v any) float64 { f, _ := v.(float64); return f }

// ---------------------------------------------------------------- notes des fiches

// Noms des propriétés que lit outils/vault_vers_cofdm.py, quand ils diffèrent de la clé du champ.
var proprietes = map[string]string{
	"lieu.parent": "région", "faction.siege": "siège", "faction.attitude": "attitude_envers_pj",
	"pnj.peuple": "race", "quete.niveau": "niveau_conseillé", "quete.recompense": "récompense",
}

// Titres de section que reconnaît le script d'import, quand ils diffèrent du nom du champ.
var titres = map[string]string{"quete.consequences": "Enjeux & conséquences"}

// citation : le champ d'une ligne écrit en exergue sous le titre de la note.
func citation(t *modele.Type) string {
	for _, k := range []string{"resume", "impression", "but"} {
		if c, ok := t.Champ(k); ok && c.Genre == modele.Texte {
			return k
		}
	}
	return ""
}

func (x *exporteur) propriete(n *note, c modele.Champ) string {
	v, ok := n.e.Data[c.Cle]
	if !ok {
		return ""
	}
	switch c.Genre {
	case modele.Texte, modele.Choix, modele.DateReelle:
		if s := texte(v); s != "" {
			return yaml(s)
		}
	case modele.Nombre:
		return nombre(v)
	case modele.Booleen:
		if b, ok := v.(bool); ok {
			return strconv.FormatBool(b)
		}
	case modele.Ref:
		if l, ok := x.lienID(premierID(v), false); ok {
			return yaml(l)
		}
	case modele.Refs:
		var l []string
		for _, id := range ids(v) {
			if s, ok := x.lienID(id, false); ok {
				l = append(l, yaml(s))
			}
		}
		if len(l) > 0 {
			return "[" + strings.Join(l, ", ") + "]"
		}
	case modele.DateMonde:
		if s := x.dateJeu(v); s != "" {
			return yaml(s)
		}
	case modele.Monstre:
		if nom := x.u.Monstres[premierID(v)]; nom != "" {
			return yaml(nom)
		}
	}
	return ""
}

func (x *exporteur) noteFiche(n *note, images []string, combats []string) string {
	var fm, corps []string
	fm = append(fm, "type: "+strings.ToLower(n.t.Nom))
	cit := citation(n.t)
	section := func(titre, contenu string) {
		if contenu = strings.TrimSpace(contenu); contenu != "" {
			corps = append(corps, "## "+titre+"\n\n"+contenu)
		}
	}
	for _, c := range n.t.Champs {
		v := n.e.Data[c.Cle]
		titre := c.Nom
		if t, ok := titres[n.e.Type+"."+c.Cle]; ok {
			titre = t
		}
		switch {
		case c.Cle == cit, c.Cle == "fils_rouges" && n.e.Type == "campagne":
			// en exergue ; ou dans une note à part
		case c.Cle == "themes" && n.e.Type == "campagne":
			var l []string
			for _, t := range strings.Split(texte(v), " · ") {
				if t = strings.TrimSpace(t); t != "" {
					l = append(l, "- "+x.reecrire(t))
				}
			}
			section(titre, strings.Join(l, "\n"))
		case c.Genre == modele.Long:
			section(titre, x.reecrire(texte(v)))
		case c.Genre == modele.Etapes:
			var l []string
			lst, _ := v.([]any)
			for _, e := range lst {
				m, _ := e.(map[string]any)
				coche := " "
				if f, _ := m["fait"].(bool); f {
					coche = "x"
				}
				if t := texte(m["texte"]); t != "" {
					l = append(l, "- ["+coche+"] "+x.reecrire(t))
				}
			}
			section(titre, strings.Join(l, "\n"))
		case c.Genre == modele.Adversaires:
			var l []string
			lst, _ := v.([]any)
			for _, e := range lst {
				m, _ := e.(map[string]any)
				nom := texte(m["nom"])
				if nom == "" {
					nom = x.u.Monstres[premierID(m["monstre"])]
				}
				ligne := fmt.Sprintf("- %s × %s", nombre(m["qte"]), nom)
				if texte(m["camp"]) == "allie" {
					ligne += " (allié)"
				}
				l = append(l, ligne)
			}
			section(titre, strings.Join(l, "\n"))
		default:
			cleProp := c.Cle
			if p, ok := proprietes[n.e.Type+"."+c.Cle]; ok {
				cleProp = p
			} else if c.Cle == "categorie" {
				cleProp = "catégorie"
			}
			if p := x.propriete(n, c); p != "" {
				fm = append(fm, cleProp+": "+p)
			}
		}
	}

	// relations, vues depuis cette fiche
	var factions, membres, lignes []string
	for _, r := range x.u.Relations {
		if r.A != n.e.ID && r.B != n.e.ID {
			continue
		}
		autreID, sens := r.B, true
		if r.B == n.e.ID {
			autreID, sens = r.A, false
		}
		autre, ok := x.parID[autreID]
		if !ok {
			continue
		}
		if r.Type == "membre" && !r.Secrete {
			if sens && n.e.Type == "pnj" && autre.e.Type == "faction" {
				factions = append(factions, yaml(x.lienRef(autre, false)))
				continue
			}
		}
		if r.Type == "membre" && !sens && n.e.Type == "faction" {
			m := "- " + x.lienRef(autre, false)
			if r.Rang != "" {
				m += " (" + r.Rang + ")"
			}
			if r.Secrete {
				m += " — *secret*"
			}
			membres = append(membres, m)
			continue
		}
		lignes = append(lignes, "| "+x.lienRef(autre, true)+" | "+x.libelle(r, sens)+" |")
	}
	if len(factions) > 0 {
		fm = append(fm, "factions: ["+strings.Join(factions, ", ")+"]")
	}
	if len(n.e.Tags) > 0 {
		var l []string
		for _, t := range n.e.Tags {
			if t = strings.Join(strings.Fields(t), "-"); t != "" {
				l = append(l, yaml(t))
			}
		}
		fm = append(fm, "tags: ["+strings.Join(l, ", ")+"]")
	}
	if x.u.MJ && n.e.Partage {
		fm = append(fm, "partagé: true")
	}
	if n.e.Archive {
		fm = append(fm, "archivé: true")
	}
	fm = append(fm, "cofdm: "+strconv.FormatInt(n.e.ID, 10))

	if len(membres) > 0 {
		section("Membres", strings.Join(membres, "\n"))
	}
	if len(lignes) > 0 {
		titre := "Relations"
		if n.e.Type == "faction" {
			titre = "Alliés, ennemis et relations" // le script d'import lit « Alliés… »
		}
		section(titre, "| Avec | Relation |\n|---|---|\n"+strings.Join(lignes, "\n"))
	}
	if len(combats) > 0 {
		section("Combats", strings.Join(combats, "\n"))
	}

	var b strings.Builder
	b.WriteString("---\n" + strings.Join(fm, "\n") + "\n---\n# " + n.e.Nom + "\n")
	if s := texte(n.e.Data[cit]); cit != "" && s != "" {
		b.WriteString("\n> " + x.reecrire(s) + "\n")
	}
	if len(images) > 0 {
		b.WriteString("\n" + strings.Join(images, "\n\n") + "\n")
	}
	for _, s := range corps {
		b.WriteString("\n" + s + "\n")
	}
	return b.String()
}

// libelle : la relation lue depuis la fiche (sens : elle est A).
func (x *exporteur) libelle(r store.RelationBrute, sens bool) string {
	tr, _ := modele.RelationDe(r.Type)
	l := tr.Inverse
	if sens {
		l = tr.Nom
	}
	note := r.Note
	if r.Type == "autre" && note != "" {
		l, note = note, ""
	}
	if l == "" {
		l = r.Type
	}
	s := x.cellule(l)
	if r.Rang != "" {
		s += " (" + x.cellule(r.Rang) + ")"
	}
	if note != "" {
		s += " — " + x.cellule(note)
	}
	if c, ok := x.lienID(r.Contexte, true); ok {
		s += " — dans " + c
	}
	if r.Secrete {
		s += " — *secret*"
	}
	return s
}

// ---------------------------------------------------------------- chronologie, combats, journal

// chronologie : les événements datés, dans l'ordre, en un tableau que relit le script d'import.
func (x *exporteur) chronologie() {
	type evt struct {
		n    *note
		jour int
		date string
	}
	var l []evt
	for _, n := range x.notes {
		if n.e.Type != "evenement" || n.e.Archive {
			continue
		}
		v := evt{n: n, jour: int(^uint(0) >> 1)}
		if m, ok := n.e.Data["date_jeu"].(map[string]any); ok {
			d := modele.DateJeu{An: int(num(m["an"])), Mois: int(num(m["mois"])), Jour: int(num(m["jour"]))}
			v.jour, v.date = x.u.Calendrier.Absolu(d), x.u.Calendrier.Texte(d)
		}
		l = append(l, v)
	}
	if len(l) == 0 {
		return
	}
	sort.SliceStable(l, func(i, j int) bool { return l[i].jour < l[j].jour })
	var b strings.Builder
	b.WriteString("---\ntype: chronologie\n---\n# Chronologie\n\n| Date | Événement | Récit | Acteurs |\n|---|---|---|---|\n")
	for _, v := range l {
		lien := x.lienRef(v.n, true)
		recit := lien
		if p := premiereLigne(v.n.e.Data["publique"], v.n.e.Data["verite"]); p != "" {
			recit += " : " + x.cellule(p)
		}
		var acteurs []string
		for _, id := range append(ids(v.n.e.Data["acteurs"]), ids(v.n.e.Data["lieu"])...) {
			if s, ok := x.lienID(id, true); ok {
				acteurs = append(acteurs, s)
			}
		}
		date := v.date
		if date == "" {
			date = "—"
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", x.cellule(date), lien, recit, strings.Join(acteurs, ", "))
	}
	x.ajouter("01 - Monde/"+x.unique("Chronologie", "COF DM")+".md", b.String())
}

// premiereLigne : la première ligne de texte du premier champ rempli, sans les remarques en italique
// (« *Date indiquée : vers 1195.* »). La vérité n'est là que dans l'export du MJ.
func premiereLigne(champs ...any) string {
	for _, c := range champs {
		for _, l := range strings.Split(texte(c), "\n") {
			l = strings.TrimSpace(l)
			if l != "" && !(len(l) > 1 && l[0] == '*' && l[len(l)-1] == '*' && !strings.HasPrefix(l, "**")) {
				return l
			}
		}
	}
	return ""
}

var moisFr = []string{"janvier", "février", "mars", "avril", "mai", "juin", "juillet", "août", "septembre", "octobre", "novembre", "décembre"}

func dateCourte(t time.Time) string {
	return fmt.Sprintf("%d %s %d", t.Day(), moisFr[t.Month()-1], t.Year())
}

func pctPv(p combat.PointPv) int {
	if p.Max == 0 {
		return 0
	}
	return int(float64(p.Pv)/float64(p.Max)*100 + 0.5)
}

// combats écrit une note par combat terminé (son récapitulatif, comme le bouton « Copier pour
// Obsidian » de l'écran de combat) et renvoie, par séance et par rencontre, les liens vers elles.
func (x *exporteur) combats() map[int64][]string {
	liens := map[int64][]string{}
	cbts := append([]Combat(nil), x.u.Combats...)
	sort.Slice(cbts, func(i, j int) bool { return cbts[i].ID < cbts[j].ID })
	for _, c := range cbts {
		st, r := c.Etat, c.Etat.Recap
		if !st.Termine || r == nil {
			continue
		}
		quand := time.Unix(r.Date, 0)
		nom := x.unique(quand.Format("2006-01-02")+" "+r.Nom, "")
		dossier := "05 - Campagnes/Sans campagne/Combats"
		if cp, ok := x.parID[st.CampagneID]; ok && cp.e.Type == "campagne" && !cp.e.Archive {
			dossier = cp.dossier + "/Combats"
		}
		issue := map[string]string{"victoire": "Victoire", "defaite": "Défaite"}[r.Issue]
		if issue == "" {
			issue = "Combat terminé"
		}
		fm := []string{"type: combat", "date: " + quand.Format("2006-01-02"), "issue: " + yaml(map[bool]string{true: r.Issue, false: "terminé"}[r.Issue != ""]), "rounds: " + strconv.Itoa(r.Rounds)}
		if r.Heros != nil {
			fm = append(fm, "heros: "+yaml(r.Heros.Nom))
		}
		if s, ok := x.lienID(st.SeanceID, false); ok {
			fm = append(fm, "séance: "+yaml(s))
		}
		if st.RencontreID != nil {
			if s, ok := x.lienID(*st.RencontreID, false); ok {
				fm = append(fm, "rencontre: "+yaml(s))
			}
		}
		infos := []string{dateCourte(quand), fmt.Sprintf("%d round%s", r.Rounds, map[bool]string{true: "s"}[r.Rounds > 1])}
		if r.DureeMin > 0 {
			infos = append(infos, fmt.Sprintf("%d min", r.DureeMin))
		}
		if r.Ennemis > 0 {
			infos = append(infos, fmt.Sprintf("%d/%d adversaires vaincus", r.Vaincus, r.Ennemis))
		}
		if len(r.Tombes) > 0 {
			infos = append(infos, "à terre : "+strings.Join(r.Tombes, ", "))
		}
		var b strings.Builder
		b.WriteString("---\n" + strings.Join(fm, "\n") + "\n---\n# " + r.Nom + " — " + issue + "\n\n*" + strings.Join(infos, " · ") + "*\n")
		if r.Heros != nil {
			b.WriteString("\n**Héros du combat : " + r.Heros.Nom + "**")
			if r.Heros.Texte != "" {
				b.WriteString(" — " + r.Heros.Texte)
			}
			b.WriteString("\n")
		}
		if len(r.Moments) > 0 {
			b.WriteString("\n## Moments forts\n\n")
			for _, m := range r.Moments {
				fmt.Fprintf(&b, "- **%s** — %s : %s\n", m.Titre, m.Qui, m.Texte)
			}
		}
		if len(r.Pjs) > 0 {
			b.WriteString("\n## Bilan\n\n| PJ | Titre | Infligés | Encaissés | Précision | Crit. | Abattus |\n|---|---|--:|--:|--:|--:|--:|\n")
			for _, p := range r.Pjs {
				pj := x.cellule(p.Nom)
				if p.Tombe {
					pj += " (à terre)"
				}
				fmt.Fprintf(&b, "| %s | %s | %d | %d | %d/%d | %d | %d |\n", pj, x.cellule(p.Titre), p.Infliges, p.Subis, p.Touches, p.Tentatives, p.Critiques, p.Abattus)
			}
		}
		if courbe := r.Courbe; len(courbe) > 1 {
			var pts []string
			varie := false
			for i, p := range courbe {
				e := "R" + strconv.Itoa(p.R)
				if i == len(courbe)-1 {
					e = "Fin"
				}
				pts = append(pts, fmt.Sprintf("%s %d %%", e, pctPv(p)))
				varie = varie || pctPv(p) != pctPv(courbe[0])
			}
			if varie {
				b.WriteString("\nPV du groupe : " + strings.Join(pts, " → ") + "\n")
			}
		}
		x.ajouter(dossier+"/"+nom+".md", b.String())
		lien := "- [[" + nom + "]]"
		if st.SeanceID != 0 {
			liens[st.SeanceID] = append(liens[st.SeanceID], lien)
		}
		if st.RencontreID != nil {
			liens[*st.RencontreID] = append(liens[*st.RencontreID], lien)
		}
	}
	return liens
}

// journal : l'historique des modifications, le plus récent en tête.
func (x *exporteur) journal() {
	var b strings.Builder
	b.WriteString("---\ntype: journal\n---\n# Journal\n\n| Quand | Auteur | Action | Fiche | Détail |\n|---|---|---|---|---|\n")
	for _, l := range x.u.Journal {
		fiche := x.cellule(l.Nom)
		if s, ok := x.lienID(l.Entite, true); ok && l.Entite != 0 {
			fiche = s
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s |\n", time.Unix(int64(l.Quand), 0).Format("2006-01-02 15:04"),
			x.cellule(l.Auteur), x.cellule(l.Action), fiche, x.cellule(l.Detail))
	}
	x.ajouter(x.unique("Journal", "COF DM")+".md", b.String())
}
