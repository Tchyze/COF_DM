// Package reference gère les profils, peuples et voies (officiels ou maison) saisis ou importés par
// le MJ. Aucun contenu n'est embarqué : le code n'en garde que la structure.
package reference

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

var sansAccents = strings.NewReplacer(
	"à", "a", "â", "a", "ä", "a", "á", "a", "ç", "c", "é", "e", "è", "e", "ê", "e", "ë", "e",
	"î", "i", "ï", "i", "í", "i", "ô", "o", "ö", "o", "ó", "o", "ù", "u", "û", "u", "ü", "u", "ú", "u",
	"ÿ", "y", "œ", "oe", "æ", "ae",
)

// Format du fichier de profils, peuples et voies. FormatAncien : celui de tracker_COF et des
// versions de COF DM qui rangeaient tout en un seul réglage (version 1), encore lu à l'import.
const (
	Format       = "cof-dm-reference"
	FormatAncien = "tracker-cof-reference"
	Version      = 2
)

// Étiquette d'un profil, d'un peuple ou d'une voie : une simple marque pour trier et filtrer, qui ne
// verrouille rien. Sans étiquette, un élément qui a une référence est officiel, les autres sont
// maison.
const (
	Officielle = "officielle"
	Maison     = "maison"
)

// Collections de l'univers où les éléments sont rangés un par un.
const (
	CollProfils = "profils"
	CollPeuples = "peuples"
	CollVoies   = "voies"
)

// caracs : les caractéristiques d'une feuille (internal/fiches).
var caracs = []string{"FOR", "DEX", "CON", "INT", "SAG", "CHA"}

// Bonus permanent d'une capacité, ajouté à la feuille quand le PJ a atteint son rang. Il vaut
// Valeur, multipliée par le rang atteint dans la voie si ParRang, plus le niveau du PJ si Niveau,
// plus le modificateur de Carac si elle est donnée.
type Bonus struct {
	Cible   string `json:"cible"` // pv, pm, def, init, att (toutes), att_c, att_d, att_m, rd, ou une caractéristique (sa valeur)
	Valeur  int    `json:"valeur"`
	ParRang bool   `json:"par_rang,omitempty"`
	Niveau  bool   `json:"niveau,omitempty"`
	Carac   string `json:"carac,omitempty"`
}

// CiblesBonus : ce qu'un bonus peut augmenter, en plus des caractéristiques.
var CiblesBonus = []string{"pv", "pm", "def", "init", "att", "att_c", "att_d", "att_m", "rd"}

// Action : ce qu'une capacité active préremplit en combat ; le MJ valide toujours.
type Action struct {
	Genre   string `json:"genre"`             // sort, attaque, soin, etat, autre
	CoutPm  int    `json:"cout_pm,omitempty"` // points de mana dépensés
	Attaque string `json:"attaque,omitempty"` // jet d'attaque : C, D ou M (vide : pas de jet)
	Bonus   int    `json:"bonus,omitempty"`   // ajouté au jet d'attaque
	Degats  string `json:"degats,omitempty"`  // formule de dés
	Soin    string `json:"soin,omitempty"`    // formule de dés
	Etat    string `json:"etat,omitempty"`    // clé d'un état des règles de combat
}

// GenresAction : les genres d'une action de capacité.
var GenresAction = []string{"sort", "attaque", "soin", "etat", "autre"}

// Utilisations : « Fois fois par » combat, jour, repos (« repos:<clé> », un repos des règles) ou
// remise à zéro à la main (« main »).
type Utilisations struct {
	Fois int    `json:"fois"`
	Par  string `json:"par"`
}

type Capacite struct {
	Nom          string        `json:"nom"`
	Texte        string        `json:"texte"`
	Limitee      bool          `json:"limitee"` // (L) : action limitée
	Sort         bool          `json:"sort"`    // * : sort
	Bonus        []Bonus       `json:"bonus,omitempty"`
	Action       *Action       `json:"action,omitempty"`
	Utilisations *Utilisations `json:"utilisations,omitempty"`
}

// ID : identifiant de l'élément dans l'univers (son document), donné à la lecture, jamais rangé.
type Voie struct {
	ID          int64      `json:"id,omitempty"`
	Ref         string     `json:"ref"`
	Nom         string     `json:"nom"`
	Etiquette   string     `json:"etiquette"`
	Type        string     `json:"type"` // profil, peuple, prestige, autre
	Profils     []string   `json:"profils"`
	Prerequis   string     `json:"prerequis"`
	Description string     `json:"description"`
	Rangs       []Capacite `json:"rangs"`
	Livre       string     `json:"livre"`
	Page        int        `json:"page"`
}

type Profil struct {
	ID          int64    `json:"id,omitempty"`
	Ref         string   `json:"ref"`
	Nom         string   `json:"nom"`
	Etiquette   string   `json:"etiquette"`
	Famille     string   `json:"famille"`
	Dv          string   `json:"dv"`
	CaracMagie  string   `json:"carac_magie"`
	Armes       string   `json:"armes"`
	Armures     string   `json:"armures"`
	Equipement  string   `json:"equipement"`
	Description string   `json:"description"`
	Voies       []string `json:"voies"`
	Livre       string   `json:"livre"`
	Page        int      `json:"page"`
}

// Peuple : Modificateurs donne ses modificateurs de caractéristiques, pour l'affichage et la
// création d'un PJ ; ils ne s'ajoutent jamais d'office à une feuille, dont les valeurs les
// comprennent déjà. Caracs garde le texte libre (« +2 DEX, −2 CON, ou au choix… »).
type Peuple struct {
	ID            int64          `json:"id,omitempty"`
	Ref           string         `json:"ref"`
	Nom           string         `json:"nom"`
	Etiquette     string         `json:"etiquette"`
	Caracs        string         `json:"caracs"`
	Modificateurs map[string]int `json:"modificateurs,omitempty"`
	Description   string         `json:"description"`
	Voies         []string       `json:"voies"`
	Livre         string         `json:"livre"`
	Page          int            `json:"page"`
}

type Reference struct {
	Format  string   `json:"format"`
	Version int      `json:"version"`
	Source  string   `json:"source,omitempty"`
	Profils []Profil `json:"profils"`
	Peuples []Peuple `json:"peuples"`
	Voies   []Voie   `json:"voies"`
}

func Vide() *Reference {
	return &Reference{Format: Format, Version: Version, Profils: []Profil{}, Peuples: []Peuple{}, Voies: []Voie{}}
}

// Cle : nom sans accents ni majuscules, pour rapprocher « Voie du prêtre » et « voie du pretre ».
func Cle(s string) string {
	var b strings.Builder
	for _, r := range sansAccents.Replace(strings.ToLower(strings.TrimSpace(s))) {
		if unicode.IsSpace(r) || r == '\'' || r == '’' || r == '-' || r == ',' {
			r = ' '
		}
		b.WriteRune(r)
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// Lire décode un fichier de référence (objet complet ou simple tableau de voies), de ce format ou
// de l'ancien, et le valide.
func Lire(data []byte) (*Reference, error) {
	r := Vide()
	r.Format, r.Version = "", 0
	if err := json.Unmarshal(data, r); err != nil {
		var voies []Voie
		if json.Unmarshal(data, &voies) != nil {
			return nil, fmt.Errorf("format non reconnu : attendu un objet {\"profils\", \"peuples\", \"voies\"}")
		}
		r.Voies = voies
	}
	if r.Format != "" && r.Format != Format && r.Format != FormatAncien {
		return nil, fmt.Errorf("ce fichier n'est pas un fichier de profils et voies (format %q)", r.Format)
	}
	if r.Version > Version {
		return nil, fmt.Errorf("ce fichier vient d'une version plus récente de COF DM (version %d)", r.Version)
	}
	r.Format, r.Version = Format, Version
	if err := r.Normaliser(); err != nil {
		return nil, err
	}
	return r, nil
}

// Etiqueter donne la même étiquette à tous les éléments.
func (r *Reference) Etiqueter(e string) error {
	if e != Officielle && e != Maison {
		return fmt.Errorf("étiquette %q : officielle ou maison", e)
	}
	for i := range r.Profils {
		r.Profils[i].Etiquette = e
	}
	for i := range r.Peuples {
		r.Peuples[i].Etiquette = e
	}
	for i := range r.Voies {
		r.Voies[i].Etiquette = e
	}
	return nil
}

func etiquette(e *string, ref string) error {
	switch *e {
	case "":
		*e = Maison
		if ref != "" {
			*e = Officielle
		}
	case Officielle, Maison:
	default:
		return fmt.Errorf("étiquette %q : officielle ou maison", *e)
	}
	return nil
}

func (r *Reference) Normaliser() error {
	if r.Profils == nil {
		r.Profils = []Profil{}
	}
	if r.Peuples == nil {
		r.Peuples = []Peuple{}
	}
	if r.Voies == nil {
		r.Voies = []Voie{}
	}
	for i := range r.Voies {
		if err := r.Voies[i].Normaliser(); err != nil {
			return numero("voie", i, err)
		}
	}
	for i := range r.Profils {
		if err := r.Profils[i].Normaliser(); err != nil {
			return numero("profil", i, err)
		}
	}
	for i := range r.Peuples {
		if err := r.Peuples[i].Normaliser(); err != nil {
			return numero("peuple", i, err)
		}
	}
	return nil
}

// errSansNom : un élément sans nom, désigné par son numéro dans le fichier.
var errSansNom = errors.New("nom manquant")

func numero(genre string, i int, err error) error {
	if errors.Is(err, errSansNom) {
		return fmt.Errorf("%s n° %d : nom manquant", genre, i+1)
	}
	return err
}

// RangsMax : le plus grand nombre de rangs d'une voie ; une voie en a au moins RangsMin.
const (
	RangsMin = 5
	RangsMax = 20
)

func (v *Voie) Normaliser() error {
	v.Nom = strings.TrimSpace(v.Nom)
	if v.Nom == "" {
		return errSansNom
	}
	if err := etiquette(&v.Etiquette, v.Ref); err != nil {
		return fmt.Errorf("voie « %s » : %v", v.Nom, err)
	}
	switch v.Type {
	case "":
		v.Type = "profil"
	case "profil", "peuple", "prestige", "autre":
	default:
		return fmt.Errorf("voie « %s » : type %q inconnu (profil, peuple, prestige ou autre)", v.Nom, v.Type)
	}
	if len(v.Rangs) > RangsMax {
		return fmt.Errorf("voie « %s » : %d rangs au maximum", v.Nom, RangsMax)
	}
	for len(v.Rangs) < RangsMin {
		v.Rangs = append(v.Rangs, Capacite{})
	}
	for i := range v.Rangs {
		if err := v.Rangs[i].Normaliser(); err != nil {
			return fmt.Errorf("voie « %s », rang %d : %v", v.Nom, i+1, err)
		}
	}
	if v.Profils == nil {
		v.Profils = []string{}
	}
	return nil
}

func (c *Capacite) Normaliser() error {
	c.Nom = strings.TrimSpace(c.Nom)
	if len(c.Bonus) > 12 {
		return fmt.Errorf("douze bonus au plus")
	}
	for i := range c.Bonus {
		b := &c.Bonus[i]
		if !slices.Contains(CiblesBonus, b.Cible) && !slices.Contains(caracs, b.Cible) {
			return fmt.Errorf("bonus %q inconnu (%s ou une caractéristique)", b.Cible, strings.Join(CiblesBonus, ", "))
		}
		if b.Carac != "" && !slices.Contains(caracs, b.Carac) {
			return fmt.Errorf("bonus : caractéristique %q inconnue", b.Carac)
		}
		if b.Valeur < -100 || b.Valeur > 100 {
			return fmt.Errorf("bonus : valeur entre −100 et 100")
		}
	}
	if c.Bonus != nil && len(c.Bonus) == 0 {
		c.Bonus = nil
	}
	if a := c.Action; a != nil {
		a.Degats, a.Soin, a.Etat = strings.TrimSpace(a.Degats), strings.TrimSpace(a.Soin), strings.TrimSpace(a.Etat)
		switch {
		case !slices.Contains(GenresAction, a.Genre):
			return fmt.Errorf("action : genre %q inconnu (%s)", a.Genre, strings.Join(GenresAction, ", "))
		case a.Attaque != "" && a.Attaque != "C" && a.Attaque != "D" && a.Attaque != "M":
			return fmt.Errorf("action : attaque C, D ou M")
		case a.CoutPm < 0 || a.CoutPm > 100:
			return fmt.Errorf("action : coût en PM entre 0 et 100")
		case a.Bonus < -100 || a.Bonus > 100:
			return fmt.Errorf("action : bonus entre −100 et 100")
		case len(a.Degats) > 60 || len(a.Soin) > 60 || len(a.Etat) > 60:
			return fmt.Errorf("action : formule ou état trop long")
		}
	}
	if u := c.Utilisations; u != nil {
		par := u.Par
		if r, ok := strings.CutPrefix(par, "repos:"); ok && strings.TrimSpace(r) != "" {
			par = "repos"
		}
		switch {
		case par != "combat" && par != "jour" && par != "repos" && par != "main":
			return fmt.Errorf("utilisations : par combat, jour, repos:<clé> ou main")
		case u.Fois < 1 || u.Fois > 99:
			return fmt.Errorf("utilisations : entre 1 et 99 fois")
		}
	}
	return nil
}

func (p *Profil) Normaliser() error {
	p.Nom = strings.TrimSpace(p.Nom)
	if p.Nom == "" {
		return errSansNom
	}
	if err := etiquette(&p.Etiquette, p.Ref); err != nil {
		return fmt.Errorf("profil « %s » : %v", p.Nom, err)
	}
	switch p.Dv {
	case "", "d4", "d6", "d8", "d10", "d12":
	default:
		return fmt.Errorf("profil « %s » : dé de vie %q invalide", p.Nom, p.Dv)
	}
	switch p.CaracMagie {
	case "", "INT", "SAG", "CHA":
	default:
		return fmt.Errorf("profil « %s » : carac. de magie %q invalide (INT, SAG ou CHA)", p.Nom, p.CaracMagie)
	}
	if p.Voies == nil {
		p.Voies = []string{}
	}
	return nil
}

var reModCarac = regexp.MustCompile(`([+\-−–])\s*(\d{1,2})\s*(FOR|DEX|CON|INT|SAG|CHA)\b`)

func (p *Peuple) Normaliser() error {
	p.Nom = strings.TrimSpace(p.Nom)
	if p.Nom == "" {
		return errSansNom
	}
	if err := etiquette(&p.Etiquette, p.Ref); err != nil {
		return fmt.Errorf("peuple « %s » : %v", p.Nom, err)
	}
	if p.Modificateurs == nil { // repris du texte : « +2 DEX, −2 CON »
		for _, m := range reModCarac.FindAllStringSubmatch(p.Caracs, -1) {
			n, _ := strconv.Atoi(m[2])
			if m[1] != "+" {
				n = -n
			}
			if p.Modificateurs == nil {
				p.Modificateurs = map[string]int{}
			}
			p.Modificateurs[m[3]] += n
		}
	}
	for k, n := range p.Modificateurs {
		if !slices.Contains(caracs, k) {
			return fmt.Errorf("peuple « %s » : caractéristique %q inconnue", p.Nom, k)
		}
		if n < -20 || n > 20 {
			return fmt.Errorf("peuple « %s » : modificateur entre −20 et 20", p.Nom)
		}
		if n == 0 {
			delete(p.Modificateurs, k)
		}
	}
	if len(p.Modificateurs) == 0 {
		p.Modificateurs = nil
	}
	if p.Voies == nil {
		p.Voies = []string{}
	}
	return nil
}

// NormaliserDoc valide un élément rangé dans la collection coll de l'univers, et le renvoie tel
// qu'il se range (sans identifiant).
func NormaliserDoc(coll string, data []byte) ([]byte, error) {
	var x interface{ Normaliser() error }
	switch coll {
	case CollProfils:
		x = &Profil{}
	case CollPeuples:
		x = &Peuple{}
	case CollVoies:
		x = &Voie{}
	default:
		return nil, fmt.Errorf("collection inconnue")
	}
	if err := json.Unmarshal(data, x); err != nil {
		return nil, fmt.Errorf("élément illisible")
	}
	if err := x.Normaliser(); err != nil {
		return nil, err
	}
	switch e := x.(type) {
	case *Profil:
		e.ID = 0
	case *Peuple:
		e.ID = 0
	case *Voie:
		e.ID = 0
	}
	return json.Marshal(x)
}

// Ident : ce qui reconnaît un élément d'un fichier à l'autre : sa référence, ou à défaut son nom.
func Ident(ref, nom string) string { return ident(ref, nom) }

// cle d'identité d'un élément : sa référence, ou à défaut son nom.
func ident(ref, nom string) string {
	if ref != "" {
		return "ref:" + ref
	}
	return "nom:" + Cle(nom)
}

// Bilan d'une fusion.
type Bilan struct {
	Profils  int `json:"profils"`
	Peuples  int `json:"peuples"`
	Voies    int `json:"voies"`
	Nouveaux int `json:"nouveaux"`
	MisAJour int `json:"mis_a_jour"`
}

// Fusionner ajoute ou remplace (même référence, ou même nom sans référence) les éléments de n.
func (r *Reference) Fusionner(n *Reference) Bilan {
	b := Bilan{Profils: len(n.Profils), Peuples: len(n.Peuples), Voies: len(n.Voies)}
	compter := func(remplace bool) {
		if remplace {
			b.MisAJour++
		} else {
			b.Nouveaux++
		}
	}
	idx := map[string]int{}
	for i, p := range r.Profils {
		idx[ident(p.Ref, p.Nom)] = i
	}
	for _, p := range n.Profils {
		i, ok := idx[ident(p.Ref, p.Nom)]
		compter(ok)
		if ok {
			r.Profils[i] = p
		} else {
			idx[ident(p.Ref, p.Nom)] = len(r.Profils)
			r.Profils = append(r.Profils, p)
		}
	}
	idx = map[string]int{}
	for i, p := range r.Peuples {
		idx[ident(p.Ref, p.Nom)] = i
	}
	for _, p := range n.Peuples {
		i, ok := idx[ident(p.Ref, p.Nom)]
		compter(ok)
		if ok {
			r.Peuples[i] = p
		} else {
			idx[ident(p.Ref, p.Nom)] = len(r.Peuples)
			r.Peuples = append(r.Peuples, p)
		}
	}
	idx = map[string]int{}
	for i, v := range r.Voies {
		idx[ident(v.Ref, v.Nom)] = i
	}
	for _, v := range n.Voies {
		i, ok := idx[ident(v.Ref, v.Nom)]
		compter(ok)
		if ok {
			r.Voies[i] = v
		} else {
			idx[ident(v.Ref, v.Nom)] = len(r.Voies)
			r.Voies = append(r.Voies, v)
		}
	}
	r.Trier()
	return b
}

func (r *Reference) Trier() {
	sort.SliceStable(r.Profils, func(i, j int) bool { return Cle(r.Profils[i].Nom) < Cle(r.Profils[j].Nom) })
	sort.SliceStable(r.Peuples, func(i, j int) bool { return Cle(r.Peuples[i].Nom) < Cle(r.Peuples[j].Nom) })
	sort.SliceStable(r.Voies, func(i, j int) bool { return Cle(r.Voies[i].Nom) < Cle(r.Voies[j].Nom) })
}

// Problemes signale les voies citées par un profil ou un peuple mais absentes du fichier.
func (r *Reference) Problemes() []string {
	connues := map[string]bool{}
	for _, v := range r.Voies {
		connues[Cle(v.Nom)] = true
		if v.Ref != "" {
			connues[v.Ref] = true
		}
	}
	out := []string{}
	for _, p := range r.Profils {
		for _, v := range p.Voies {
			if !connues[Cle(v)] && !connues[v] {
				out = append(out, fmt.Sprintf("profil %s : voie « %s » introuvable", p.Nom, v))
			}
		}
	}
	for _, p := range r.Peuples {
		for _, v := range p.Voies {
			if !connues[Cle(v)] && !connues[v] {
				out = append(out, fmt.Sprintf("peuple %s : voie « %s » introuvable", p.Nom, v))
			}
		}
	}
	return out
}
