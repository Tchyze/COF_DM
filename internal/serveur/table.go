package serveur

import (
	"encoding/json"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/Tchyze/cof_dm/internal/combat"
	"github.com/Tchyze/cof_dm/internal/modele"
	"github.com/Tchyze/cof_dm/internal/store"
)

// Écran du MJ : ce qu'il faut sous les yeux pendant la partie (les PJ, la quête en cours, sa scène,
// ses rencontres), en une réponse. /api/table ne lit que la base, pour s'afficher aussitôt ;
// /api/table/etat donne ce qui bouge pendant la partie (PV des PJ, combat en cours) et se relit en
// boucle.

func (s *Serveur) routesTable() {
	m := s.mux
	m.HandleFunc("GET /api/table", s.table)
	m.HandleFunc("GET /api/table/etat", s.tableEtat)
}

// types de fiches qui composent la scène d'une quête.
var typesScene = map[string]bool{"pnj": true, "lieu": true, "faction": true, "objet": true, "lore": true, "evenement": true, "secret": true}

type pjTable struct {
	Fiche  store.Resume `json:"fiche"`
	Statut string       `json:"statut,omitempty"`
	Peuple string       `json:"peuple,omitempty"`
	Vie    *viePJ       `json:"vie,omitempty"` // lue sur sa feuille de personnage ; absente sans feuille
}

// viePJ : ce que la feuille d'un PJ dit de son état en jeu.
type viePJ struct {
	Pv      int `json:"pv"`
	PvMax   int `json:"pv_max"`
	Defense int `json:"defense"`
	Init    int `json:"init"`
	Pc      int `json:"pc"`
	PcMax   int `json:"pc_max"`
	Pr      int `json:"pr"`
	PrMax   int `json:"pr_max"`
	Pm      int `json:"pm"`
	PmMax   int `json:"pm_max"`
}

// vieDe lit l'état d'un PJ sur sa feuille (nil s'il n'en a pas encore).
func (s *Serveur) vieDe(e store.Entite) *viePJ {
	f, _, existe, err := s.feuilleDe(e)
	if err != nil || !existe {
		return nil
	}
	v := &viePJ{Pv: f.Pv, PvMax: f.Pv, Defense: f.Defense, Init: f.Init, Pc: f.Pc, PcMax: f.PcMax, Pr: f.Pr, PrMax: f.PrMax, Pm: f.Pm, PmMax: f.PmMax}
	if f.PvActuel != nil {
		v.Pv = *f.PvActuel
	}
	return v
}

type ficheScene struct {
	Fiche  store.Resume `json:"fiche"`
	Resume string       `json:"resume,omitempty"`
	Statut string       `json:"statut,omitempty"`
	Raison string       `json:"raison"`           // pourquoi elle est là : « Commanditaire », « cité »…
	Secret bool         `json:"secret,omitempty"` // la fiche a des champs réservés au MJ remplis
}

type rencontreTable struct {
	Fiche     store.Resume `json:"fiche"`
	Resume    string       `json:"resume,omitempty"`
	Lieu      string       `json:"lieu,omitempty"`
	Creatures string       `json:"creatures,omitempty"` // « 4 × Brigand, 1 × Chef »
}

func premierID(v any) int64 {
	if l := idsDe(v); len(l) > 0 {
		return l[0]
	}
	return 0
}

func absent(statut any) bool { return statut == "mort" || statut == "parti" }

// aSecret : un champ réservé au MJ est rempli.
func aSecret(e store.Entite) bool {
	t, ok := modele.TypeDe(e.Type)
	if !ok {
		return false
	}
	for _, ch := range t.Champs {
		if v, _ := e.Data[ch.Cle].(string); ch.Secret && strings.TrimSpace(v) != "" {
			return true
		}
	}
	return false
}

// campagneDeTable : la campagne de la quête, sinon celle qui est en cours, sinon la seule qui existe.
func (s *Serveur) campagneDeTable(quete *store.Entite) int64 {
	if quete != nil {
		if id := premierID(quete.Data["campagne"]); id != 0 {
			return id
		}
	}
	campagnes, _ := s.store.Lister(store.Filtre{Type: "campagne"})
	for _, c := range campagnes {
		if c.Carte["statut"] == "en cours" {
			return c.ID
		}
	}
	if len(campagnes) == 1 {
		return campagnes[0].ID
	}
	return 0
}

// pjDeTable : les PJ en jeu de la campagne (tous les PJ sans campagne), avec l'état lu sur leur
// feuille ; les absents en dernier.
func (s *Serveur) pjDeTable(campagne int64) []pjTable {
	out := []pjTable{}
	pjs, _ := s.store.Lister(store.Filtre{Type: "pj", Campagne: campagne})
	for _, p := range pjs {
		f, err := s.store.Lire(p.ID)
		if err != nil || absent(f.Data["statut"]) {
			continue
		}
		statut, _ := f.Data["statut"].(string)
		peuple, _ := f.Data["peuple"].(string)
		out = append(out, pjTable{Fiche: p, Statut: statut, Peuple: peuple, Vie: s.vieDe(f)})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Statut != "absent" && out[j].Statut == "absent" })
	return out
}

// seanceDeTable : la prochaine séance prévue de la campagne, sinon la dernière.
func (s *Serveur) seanceDeTable(campagne int64) int64 {
	if campagne == 0 {
		return 0
	}
	seances, _ := s.store.Lister(store.Filtre{Type: "seance", Campagne: campagne}) // la plus récente d'abord
	var id int64
	for _, x := range seances {
		if x.Carte["statut"] == "prévue" {
			id = x.ID
		}
	}
	if id == 0 && len(seances) > 0 {
		id = seances[0].ID
	}
	return id
}

// rencontresDeTable : les rencontres rattachées à la quête.
func (s *Serveur) rencontresDeTable(quete int64) ([]rencontreTable, []store.Entite) {
	out, fiches := []rencontreTable{}, []store.Entite{}
	toutes, _ := s.store.Lister(store.Filtre{Type: "rencontre"})
	for _, r := range toutes {
		if premierID(r.Carte["quete"]) != quete {
			continue
		}
		f, err := s.store.Lire(r.ID)
		if err != nil {
			continue
		}
		x := rencontreTable{Fiche: r, Resume: apercu(f), Creatures: creaturesEnClair(f)}
		if l, err := s.store.Lire(premierID(f.Data["lieu"])); err == nil {
			x.Lieu = l.Nom
		}
		out = append(out, x)
		fiches = append(fiches, f)
	}
	return out, fiches
}

// sceneDeTable : les fiches du monde utiles à la quête. Dans l'ordre : ses champs (commanditaire,
// lieu), ses relations, les PNJ et lieux de ses rencontres, les fiches qui pointent vers elle
// (secrets qui la concernent…), puis celles que ses textes citent. Une fiche n'apparaît qu'une fois.
func (s *Serveur) sceneDeTable(q ficheComplete, rencontres []store.Entite) []ficheScene {
	out := []ficheScene{}
	vu := map[int64]bool{q.ID: true}
	ajouter := func(id int64, raison string) {
		if id == 0 || vu[id] {
			return
		}
		e, err := s.store.Lire(id)
		if err != nil || e.Archive || !typesScene[e.Type] {
			return
		}
		vu[id] = true
		statut, _ := e.Data["statut"].(string)
		r := store.Resume{ID: e.ID, Type: e.Type, Nom: e.Nom, Image: e.Image, Modifie: e.Modifie}
		out = append(out, ficheScene{Fiche: r, Resume: apercu(e), Statut: statut, Raison: raison, Secret: aSecret(e)})
	}
	if t, ok := modele.TypeDe(q.Type); ok {
		for _, ch := range t.Champs {
			if ch.Genre == modele.Ref || ch.Genre == modele.Refs {
				for _, id := range idsDe(q.Data[ch.Cle]) {
					ajouter(id, ch.Nom)
				}
			}
		}
	}
	for _, r := range q.Relations {
		ajouter(r.Autre.ID, r.Libelle)
	}
	for _, r := range rencontres {
		for _, k := range []string{"lieu", "pnj"} {
			for _, id := range idsDe(r.Data[k]) {
				ajouter(id, "rencontre « "+r.Nom+" »")
			}
		}
	}
	for _, r := range q.References {
		ajouter(r.Source.ID, r.Champ)
	}
	var cites []int64
	for _, m := range q.Mentions {
		cites = append(cites, m.ID)
	}
	sort.Slice(cites, func(i, j int) bool { return cites[i] < cites[j] })
	for _, id := range cites {
		ajouter(id, "cité")
	}
	rang := map[string]int{}
	for i, t := range modele.Types {
		rang[t.Cle] = i
	}
	sort.SliceStable(out, func(i, j int) bool { return rang[out[i].Fiche.Type] < rang[out[j].Fiche.Type] })
	return out
}

// table : ?quete=<id> choisit la quête ; sans paramètre, la quête en cours modifiée en dernier.
func (s *Serveur) table(w http.ResponseWriter, r *http.Request) {
	toutes, err := s.store.Lister(store.Filtre{Type: "quete"})
	if err != nil {
		erreurStore(w, err)
		return
	}
	enCours := []store.Resume{}
	for _, q := range toutes {
		if q.Carte["statut"] == "en cours" {
			enCours = append(enCours, q)
		}
	}
	sort.SliceStable(enCours, func(i, j int) bool { return enCours[i].Modifie > enCours[j].Modifie })

	out := map[string]any{"quetes": enCours, "quete": nil, "campagne": nil, "seance": nil,
		"scene": []ficheScene{}, "rencontres": []rencontreTable{}}
	id := entierParam(r, "quete")
	if id == 0 && len(enCours) > 0 {
		id = enCours[0].ID
	}
	var quete *store.Entite
	if id != 0 {
		q, err := s.complete(id)
		if err != nil || q.Type != "quete" {
			erreur(w, 404, "Quête introuvable.")
			return
		}
		rencontres, fiches := s.rencontresDeTable(id)
		out["quete"], out["rencontres"], out["scene"] = q, rencontres, s.sceneDeTable(q, fiches)
		quete = &q.Entite
	}
	campagne := s.campagneDeTable(quete)
	if c, err := s.complete(campagne); err == nil && c.Type == "campagne" {
		out["campagne"] = c
	} else {
		campagne = 0
	}
	if se, err := s.complete(s.seanceDeTable(campagne)); err == nil {
		out["seance"] = se
	}
	out["pj"] = s.pjDeTable(campagne)
	repondre(w, 200, out)
}

// tableEtat : ce qui bouge pendant la partie. « pj » : l'état de chaque PJ de la campagne, par
// identifiant de fiche ; « combat » : le combat en cours le plus récent (celui de la campagne
// d'abord), ou null.
func (s *Serveur) tableEtat(w http.ResponseWriter, r *http.Request) {
	campagne := entierParam(r, "campagne")
	pj := map[string]*viePJ{}
	for _, p := range s.pjDeTable(campagne) {
		if p.Vie != nil {
			pj[strconv.FormatInt(p.Fiche.ID, 10)] = p.Vie
		}
	}
	out := map[string]any{"pj": pj, "combat": nil}
	docs, _ := s.store.ListerDocs(collCombats)
	var choisi *resumeCombat
	for _, d := range docs {
		var st combat.Etat
		if json.Unmarshal(d.Data, &st) != nil || st.Termine {
			continue
		}
		c := resumer(d.ID, d.Modifie, &st)
		mieux := choisi == nil || (c.Campagne == campagne) != (choisi.Campagne == campagne) && c.Campagne == campagne ||
			(c.Campagne == campagne) == (choisi.Campagne == campagne) && c.Updated > choisi.Updated
		if mieux {
			choisi = &c
		}
	}
	if choisi != nil {
		out["combat"] = choisi
	}
	repondre(w, 200, out)
}

// socketsDeLaPage complète la politique de sécurité de la page : elle ouvre un WebSocket vers
// l'application elle-même pour suivre un combat, ce que « 'self' » ne couvre pas dans tous les
// navigateurs.
func socketsDeLaPage(r *http.Request) string {
	if !reHote.MatchString(r.Host) {
		return ""
	}
	return "; connect-src 'self' ws://" + r.Host + " wss://" + r.Host
}

var reHote = regexp.MustCompile(`^[A-Za-z0-9.\-\[\]:]+$`)
