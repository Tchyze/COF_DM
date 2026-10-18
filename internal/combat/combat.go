// Package combat déroule un combat : initiative, tours, attaques contre la DEF, dégâts, états,
// tests de caractéristique, récapitulatif. Il ne contient aucune donnée des livres : les valeurs
// (dé, critique, point de chance) et les états viennent de la partie « combat » du fichier de règles
// de l'univers. Sans elle, le combat se joue sans état prédéfini, critique, point de chance ni
// budget d'actions.
//
// L'état d'un combat est sérialisé en JSON. Le serveur est la seule source de vérité : les clients
// envoient des actions, le moteur les applique.
package combat

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Tchyze/cof_dm/internal/fiches"
	"github.com/Tchyze/cof_dm/internal/regles"
)

var rang = map[string]int{"pj": 0, "allie": 1, "ennemi": 2}
var typesAtk = map[string]string{"C": "au contact", "D": "à distance", "M": "magique"}

// ErrAction : action refusée, le message est affiché tel quel.
type ErrAction string

func (e ErrAction) Error() string { return string(e) }

// Duree d'un état : 0 = sans limite (sérialisé « true »).
type Duree int

func (d Duree) MarshalJSON() ([]byte, error) {
	if d <= 0 {
		return []byte("true"), nil
	}
	return []byte(strconv.Itoa(int(d))), nil
}

func (d *Duree) UnmarshalJSON(b []byte) error {
	s := string(b)
	if s == "true" || s == "false" || s == "null" {
		*d = 0
		return nil
	}
	n, err := strconv.Atoi(s)
	*d = Duree(n)
	return err
}

type Ref struct {
	Kind string `json:"kind"`
	ID   int64  `json:"id"`
}

type Combattant struct {
	ID          string           `json:"id"`
	Nom         string           `json:"nom"`
	Camp        string           `json:"camp"`
	Ref         *Ref             `json:"ref"`
	Init        int              `json:"init"`
	Defense     int              `json:"defense"`
	PvMax       int              `json:"pvMax"`
	Pv          int              `json:"pv"`
	Rd          int              `json:"rd"`
	Crit        int              `json:"crit"`
	Pc          int              `json:"pc"`
	Sag         int              `json:"sag,omitempty"` // valeur de SAG : départage deux PJ à égalité
	Carac       fiches.Carac     `json:"carac"`
	Superieures []string         `json:"superieures,omitempty"`
	Attaques    []fiches.Attaque `json:"attaques"`
	Capacites   string           `json:"capacites"`
	Pm          int              `json:"pm,omitempty"`      // points de mana (PJ)
	PmMax       int              `json:"pmMax,omitempty"`   // 0 : le combattant n'en a pas
	Voies       []string         `json:"voies,omitempty"`   // capacités acquises dans ses voies (PJ)
	Utilise     []string         `json:"utilise,omitempty"` // capacités à usage limité déjà utilisées ce combat
	Etats       map[string]Duree `json:"etats"`
	Act         map[string]bool  `json:"act"`
	Cible       *string          `json:"cible"`
}

type LigneLog struct {
	R int    `json:"r"`
	K string `json:"k"`
	T string `json:"t"`
}

type Resultat struct {
	Verdict     string  `json:"verdict"`
	CalcAffiche string  `json:"calc"`
	Calc        string  `json:"calc_base"`
	Total       int     `json:"total"`
	Defense     int     `json:"defense"`
	De          int     `json:"de"`
	DmExpr      string  `json:"dm_expr"`
	AutoCrit    bool    `json:"auto_crit"`
	TypeAtk     string  `json:"type_atk"`
	Chance      bool    `json:"chance"`
	Nat         int     `json:"nat"`
	Attaque     string  `json:"attaque"`
	DmDetail    *string `json:"dm_detail"`
	Brut        *int    `json:"brut"`
	Final       *int    `json:"final"`
	Rd          int     `json:"rd"`
	Evt         int     `json:"evt"` // numéro de l'événement d'attaque (à partir de 1)
	Cible       string  `json:"cible"`
	Att         string  `json:"att"`
	Applique    *int    `json:"applique"`
	SansDm      bool    `json:"sans_dm"`
	DmReel      bool    `json:"dm_reel"` // dés réels : les dégâts sont lancés à la table, puis saisis
}

type Etat struct {
	Nom         string                 `json:"nom"`
	Round       int                    `json:"round"`
	Tour        int                    `json:"tour"`
	Ordre       []string               `json:"ordre"`
	Cbts        map[string]*Combattant `json:"cbts"`
	Log         []LigneLog             `json:"log"`
	Resultat    *Resultat              `json:"resultat"`
	Test        *ResultatTest          `json:"test"`
	Termine     bool                   `json:"termine"`
	RencontreID *int64                 `json:"rencontre_id,omitempty"` // fiche de la rencontre jouée
	CampagneID  int64                  `json:"campagne,omitempty"`     // campagne dont viennent les PJ
	SeanceID    int64                  `json:"seance,omitempty"`       // séance à laquelle le combat est rattaché
	Evts        []Evenement            `json:"evts"`
	Debut       int64                  `json:"debut,omitempty"`
	FinA        int64                  `json:"fin_a,omitempty"`
	Fin         string                 `json:"fin"`         // fin proposée : victoire ou defaite
	FinIgnoree  string                 `json:"fin_ignoree"` // le MJ a choisi de continuer
	Recap       *Recap                 `json:"recap"`
	Courbe      []PointPv              `json:"courbe"` // PV des PJ au début de chaque round
	source      string                 // auteur des dégâts en cours d'application
	rg          *regles.Combat         // règles de l'univers (nil : combat minimal)
}

// Regler donne au combat les règles de l'univers. À appeler après chaque relecture : elles ne sont
// pas enregistrées avec lui.
func (st *Etat) Regler(rg *regles.Combat) { st.rg = rg }

// FicheCombat : ce qu'il faut d'une fiche (monstre, personnage ou saisie libre) pour combattre.
type FicheCombat struct {
	ID          int64            `json:"id"`
	Nom         string           `json:"nom"`
	Init        int              `json:"init"`
	Defense     int              `json:"defense"`
	Pv          int              `json:"pv"`
	PvActuel    *int             `json:"pv_actuel"`
	Rd          int              `json:"rd"`
	Crit        int              `json:"crit"`
	Pc          int              `json:"pc"`
	Pm          int              `json:"pm"`
	PmMax       int              `json:"pm_max"`
	Attaques    []fiches.Attaque `json:"attaques"`
	Capacites   string           `json:"capacites"`
	Voies       []string         `json:"voies"`
	Carac       fiches.Carac     `json:"carac"`
	Superieures []string         `json:"superieures"`
	Valeurs     *struct {
		SAG int `json:"SAG"`
	} `json:"valeurs"`
}

func uid() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ---------------------------------------------------------------- création

func DepuisFiche(f FicheCombat, camp, nom, kind string) *Combattant {
	pvMax := max(1, f.Pv)
	pv := pvMax
	if f.PvActuel != nil {
		pv = max(0, min(pvMax, *f.PvActuel))
	}
	if _, ok := rang[camp]; !ok {
		camp = "ennemi"
	}
	if nom == "" {
		nom = f.Nom
	}
	if nom == "" {
		nom = "Sans nom"
	}
	crit := f.Crit // 0 : seul le maximum du dé est un critique
	var ref *Ref
	if kind != "" {
		ref = &Ref{Kind: kind, ID: f.ID}
	}
	atks := append([]fiches.Attaque{}, f.Attaques...)
	pmMax := max(0, f.PmMax, f.Pm)
	return &Combattant{
		ID: uid(), Nom: nom, Camp: camp, Ref: ref, Init: f.Init, Defense: f.Defense, PvMax: pvMax, Pv: pv,
		Rd: f.Rd, Crit: crit, Pc: f.Pc, Pm: max(0, min(pmMax, f.Pm)), PmMax: pmMax, Voies: append([]string(nil), f.Voies...),
		Attaques: atks, Capacites: f.Capacites, Sag: sagDe(f),
		Carac: f.Carac, Superieures: append([]string{}, f.Superieures...),
		Etats: map[string]Duree{}, Act: nouvellesActions(),
	}
}

func sagDe(f FicheCombat) int {
	if f.Valeurs != nil {
		return f.Valeurs.SAG
	}
	return 0
}

// InitEffective : initiative modifiée par les états.
func (st *Etat) InitEffective(c *Combattant) int {
	v := c.Init
	for e := range c.Etats {
		v += st.rg.Etat(e).Init
	}
	return v
}

// avant : ordre d'initiative — plus haute Init d'abord, PJ avant PNJ à égalité, deux PJ à égalité
// départagés par la SAG.
func (st *Etat) avant(a, b *Combattant) bool {
	ia, ib := st.InitEffective(a), st.InitEffective(b)
	if ia != ib {
		return ia > ib
	}
	if rang[a.Camp] != rang[b.Camp] {
		return rang[a.Camp] < rang[b.Camp]
	}
	if a.Camp == "pj" && a.Sag != b.Sag {
		return a.Sag > b.Sag
	}
	return strings.ToLower(a.Nom) < strings.ToLower(b.Nom)
}

// InstancierGroupe crée qte combattants depuis une fiche, numérotés si besoin.
func InstancierGroupe(f FicheCombat, qte int, camp, kind string, deja map[string]*Combattant) []*Combattant {
	noms := map[string]bool{}
	for _, c := range deja {
		noms[c.Nom] = true
	}
	base := f.Nom
	if base == "" {
		base = "Sans nom"
	}
	if qte == 1 && !noms[base] {
		return []*Combattant{DepuisFiche(f, camp, base, kind)}
	}
	var out []*Combattant
	for i := 1; len(out) < qte; i++ {
		n := fmt.Sprintf("%s %d", base, i)
		if !noms[n] {
			out = append(out, DepuisFiche(f, camp, n, kind))
			noms[n] = true
		}
	}
	return out
}

func Nouveau(nom string, cbts []*Combattant, rg *regles.Combat) *Etat {
	st := &Etat{rg: rg, Nom: nom, Round: 1, Ordre: []string{}, Cbts: map[string]*Combattant{}, Log: []LigneLog{}, Evts: []Evenement{}, Debut: time.Now().Unix()}
	for _, c := range cbts {
		st.Cbts[c.ID] = c
		st.Ordre = append(st.Ordre, c.ID)
	}
	st.Trier()
	st.Tour = 0
	st.log("Le combat commence : "+nom+".", "turn")
	st.noterCourbe()
	if len(st.Ordre) > 0 {
		st.debutTour()
	}
	return st
}

// Normaliser complète un état relu depuis la base (cartes nulles, etc.).
func (st *Etat) Normaliser() {
	if st.Cbts == nil {
		st.Cbts = map[string]*Combattant{}
	}
	if st.Ordre == nil {
		st.Ordre = []string{}
	}
	if st.Log == nil {
		st.Log = []LigneLog{}
	}
	if st.Evts == nil {
		st.Evts = []Evenement{}
	}
	if st.Courbe == nil {
		st.Courbe = []PointPv{}
	}
	for _, c := range st.Cbts {
		if c.Etats == nil {
			c.Etats = map[string]Duree{}
		}
		if c.Act == nil {
			c.Act = nouvellesActions()
		}
		for k := range nouvellesActions() {
			if _, ok := c.Act[k]; !ok {
				c.Act[k] = false
			}
		}
		if c.Attaques == nil {
			c.Attaques = []fiches.Attaque{}
		}
	}
	if st.Tour >= len(st.Ordre) {
		st.Tour = 0
	}
}

// ---------------------------------------------------------------- utilitaires

func (st *Etat) log(texte, kind string) {
	st.Log = append(st.Log, LigneLog{R: st.Round, K: kind, T: texte})
	if len(st.Log) > 2000 {
		st.Log = st.Log[200:]
	}
}

func (st *Etat) Courant() *Combattant {
	if len(st.Ordre) == 0 {
		return nil
	}
	return st.Cbts[st.Ordre[st.Tour%len(st.Ordre)]]
}

// enLice : à 0 PV, un combattant tombe inconscient et ne peut plus agir, PJ compris.
func enLice(c *Combattant) bool { return c != nil && c.Pv > 0 }

// etatsTries : les états d'un combattant, dans l'ordre des règles, puis les états libres par nom.
func (st *Etat) etatsTries(c *Combattant) []string {
	out := make([]string, 0, len(c.Etats))
	for e := range c.Etats {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		if ri, rj := st.rg.Rang(out[i]), st.rg.Rang(out[j]); ri != rj {
			return ri < rj
		}
		return out[i] < out[j]
	})
	return out
}

func (st *Etat) get(id string) (*Combattant, error) {
	c, ok := st.Cbts[id]
	if !ok {
		return nil, ErrAction("Ce combattant n'existe plus.")
	}
	return c, nil
}

func gras(s string) string { return "**" + s + "**" }

func (st *Etat) Trier() {
	var cur string
	if len(st.Ordre) > 0 {
		cur = st.Ordre[st.Tour]
	}
	sort.SliceStable(st.Ordre, func(i, j int) bool {
		return st.avant(st.Cbts[st.Ordre[i]], st.Cbts[st.Ordre[j]])
	})
	st.Tour = 0
	for i, id := range st.Ordre {
		if id == cur {
			st.Tour = i
		}
	}
}

func (st *Etat) inserer(c *Combattant) {
	pos := len(st.Ordre)
	for i, id := range st.Ordre {
		if st.avant(c, st.Cbts[id]) {
			pos = i
			break
		}
	}
	vide := len(st.Ordre) == 0
	st.Cbts[c.ID] = c
	st.Ordre = append(st.Ordre[:pos], append([]string{c.ID}, st.Ordre[pos:]...)...)
	if !vide && pos <= st.Tour {
		st.Tour++
	}
}

// ---------------------------------------------------------------- tours

func (st *Etat) debutTour() {
	c := st.Courant()
	if c == nil {
		return
	}
	c.Act = nouvellesActions()
	st.Resultat = nil
	st.log("Tour de "+gras(c.Nom), "turn")
	for _, e := range st.etatsTries(c) {
		if st.rg.Etat(e).Fin == "tour" {
			delete(c.Etats, e)
			st.log(gras(c.Nom)+" n'est plus en "+strings.ToLower(e)+".", "etat")
		}
	}
	var bloq []string
	for _, e := range st.etatsTries(c) {
		if st.rg.Etat(e).Bloque {
			bloq = append(bloq, strings.ToLower(e))
		}
	}
	if len(bloq) > 0 {
		st.log(gras(c.Nom)+" est "+strings.Join(bloq, ", ")+" et ne peut pas agir.", "etat")
	}
	if c.Camp == "pj" && c.Pv <= 0 {
		st.log(gras(c.Nom)+" est inconscient.", "etat")
	}
}

func (st *Etat) finTour(c *Combattant) {
	if c == nil {
		return
	}
	for _, e := range st.etatsTries(c) {
		d := c.Etats[e]
		if d <= 0 {
			continue
		}
		if d <= 1 {
			delete(c.Etats, e)
			st.log(gras(c.Nom)+" n'est plus "+strings.ToLower(e)+".", "etat")
		} else {
			c.Etats[e] = d - 1
		}
	}
}

func (st *Etat) suivant() error {
	if len(st.Ordre) == 0 {
		return ErrAction("Aucun combattant.")
	}
	st.finTour(st.Courant())
	for range st.Ordre {
		st.Tour++
		if st.Tour >= len(st.Ordre) {
			st.Tour = 0
			st.Round++
			st.nouveauRound()
		}
		if enLice(st.Courant()) {
			break
		}
	}
	st.debutTour()
	return nil
}

// nouveauRound retire les états limités au premier round.
func (st *Etat) nouveauRound() {
	st.noterCourbe()
	for _, id := range st.Ordre {
		c := st.Cbts[id]
		for _, e := range st.etatsTries(c) {
			if st.rg.Etat(e).Fin == "round" {
				delete(c.Etats, e)
				st.log(gras(c.Nom)+" n'est plus "+strings.ToLower(e)+".", "etat")
			}
		}
	}
}

func (st *Etat) precedent() error {
	if len(st.Ordre) == 0 {
		return ErrAction("Aucun combattant.")
	}
	for range st.Ordre {
		st.Tour--
		if st.Tour < 0 {
			st.Tour = len(st.Ordre) - 1
			st.Round = max(1, st.Round-1)
		}
		if enLice(st.Courant()) {
			break
		}
	}
	st.Resultat = nil
	return nil
}

// ---------------------------------------------------------------- PV

func (st *Etat) modifierPv(id string, delta int, pourquoi string) error {
	c, err := st.get(id)
	if err != nil {
		return err
	}
	avant := c.Pv
	c.Pv = max(0, min(c.PvMax, c.Pv+delta))
	src := st.Cbts[st.source]
	if delta < 0 && src == nil {
		e := st.evt("dm", nil, c)
		e.Dm, e.Perte, e.PvC = -delta, avant-c.Pv, c.Pv
	} else if delta > 0 {
		st.evt("soin", nil, c).Dm = c.Pv - avant
	}
	if avant > 0 && c.Pv == 0 {
		st.evt("ko", src, c)
	}
	if delta < 0 {
		if pourquoi == "" {
			pourquoi = fmt.Sprintf("%s subit %d DM", gras(c.Nom), -delta)
		}
		st.log(fmt.Sprintf("%s (PV %d → %d)", pourquoi, avant, c.Pv), "")
	} else {
		st.log(fmt.Sprintf("%s récupère %d PV (PV %d → %d)", gras(c.Nom), delta, avant, c.Pv), "")
	}
	if avant > 0 && c.Pv == 0 {
		if c.Camp == "pj" {
			st.log(gras(c.Nom)+" tombe inconscient !", "ko")
		} else {
			st.log(gras(c.Nom)+" est hors de combat.", "ko")
		}
	}
	return nil
}

// ---------------------------------------------------------------- attaque

func signe(n int) string {
	if n < 0 {
		return "− " + strconv.Itoa(-n)
	}
	return "+ " + strconv.Itoa(n)
}

func (st *Etat) attaquer(p Action) error {
	a, okA := st.Cbts[p.Att]
	t, okT := st.Cbts[p.Cible]
	if !okA || !okT {
		return ErrAction("Choisissez un attaquant et une cible.")
	}
	if a.ID == t.ID {
		return ErrAction("Un combattant ne peut pas s'attaquer lui-même.")
	}
	if p.CoutPm < 0 {
		return ErrAction("Le coût en PM ne peut pas être négatif.")
	}
	if p.CoutPm > a.Pm {
		return ErrAction(fmt.Sprintf("%s n'a que %d PM : le sort en coûte %d.", a.Nom, a.Pm, p.CoutPm))
	}
	suffixe, err := st.actionAttaque(a, p.Act)
	if err != nil {
		return err
	}
	if p.CoutPm > 0 { // dépensés même si le sort rate ; « Annuler » rend tout
		a.Pm -= p.CoutPm
		st.log(fmt.Sprintf("%s dépense %d PM (reste %d).", gras(a.Nom), p.CoutPm, a.Pm), "")
	}
	atk := fiches.Attaque{Nom: "Attaque", Type: "C"}
	if p.Attaque >= 0 && p.Attaque < len(a.Attaques) {
		atk = a.Attaques[p.Attaque]
	}
	exprDm := atk.Dm
	if p.Dm != nil {
		exprDm = *p.Dm
	}
	modAtt, modDef, de, autoCrit := 0, 0, st.rg.DeNormal(), false
	for e := range a.Etats {
		ef := st.rg.Etat(e)
		if atk.Type == "D" && ef.AttDistance != 0 {
			modAtt += ef.AttDistance
		} else {
			modAtt += ef.Att
		}
		if ef.De != 0 && ef.De < de {
			de = ef.De
		}
	}
	for e := range t.Etats {
		modDef += st.rg.Etat(e).Def
		if st.rg.Etat(e).CritiqueAuto {
			autoCrit = true
		}
	}
	// Qui lance les dés : l'application (mode par défaut), le joueur pour le dé d'attaque seulement
	// (« manuel » : les dégâts sont tirés ici), ou la table pour tout (« reel » : dé et dégâts saisis).
	var nat int
	if p.Mode == "manuel" || p.Mode == "reel" {
		if p.Nat == nil || *p.Nat < 1 || *p.Nat > de {
			return ErrAction(fmt.Sprintf("Saisissez le résultat du d%d lancé à la table (1 à %d).", de, de))
		}
		nat = *p.Nat
	} else {
		nat = De(de)
	}
	morceaux := []string{strconv.Itoa(nat), signe(atk.Bonus)}
	if de != st.rg.DeNormal() {
		morceaux[0] += fmt.Sprintf(" (d%d)", de)
	}
	if modAtt != 0 {
		morceaux = append(morceaux, signe(modAtt))
	}
	if p.Divers != 0 {
		morceaux = append(morceaux, signe(p.Divers))
	}
	nomAtk := atk.Nom
	if nomAtk == "" {
		nomAtk = "Attaque"
	}
	nomAtk += suffixe
	r := &Resultat{Calc: strings.Join(morceaux, " "), Nat: nat, Attaque: nomAtk, Rd: t.Rd, Cible: t.ID, Att: a.ID,
		Total: nat + atk.Bonus + modAtt + p.Divers, Defense: t.Defense + modDef, De: de, DmExpr: exprDm,
		AutoCrit: autoCrit, TypeAtk: atk.Type, DmReel: p.Mode == "reel"}
	if err := st.resoudre(r); err != nil {
		return err
	}
	ev := st.evt("atk", a, t)
	ev.V, ev.Nat, ev.De, ev.Atk = r.Verdict, r.Nat, r.De, atk.Nom
	if ev.Atk == "" {
		ev.Atk = "une attaque"
	}
	r.Evt = len(st.Evts)
	cible := t.ID
	a.Cible = &cible
	st.Resultat = r
	return nil
}

// critiqueDm : par combien un critique multiplie les dégâts (0 : les règles n'ont pas de critique).
func (st *Etat) critiqueDm() int {
	if st.rg == nil {
		return 0
	}
	return st.rg.CritiqueDm
}

// echecNaturel : résultat du dé qui fait toujours échouer (0 : aucun).
func (st *Etat) echecNaturel() int {
	if st.rg == nil {
		return 0
	}
	return st.rg.EchecNaturel
}

// chanceBonus : ce qu'ajoute un point de chance (0 : les règles n'en ont pas).
func (st *Etat) chanceBonus() int {
	if st.rg == nil {
		return 0
	}
	return st.rg.Chance
}

// resoudre fixe le verdict d'une attaque et lance les dégâts.
func (st *Etat) resoudre(r *Resultat) error {
	// un critique n'existe qu'avec le dé normal : avec un dé réduit, le maximum n'est ni une
	// réussite automatique ni un critique
	critMin, normal := 0, st.rg.DeNormal()
	if st.critiqueDm() > 0 && r.De == normal {
		critMin = normal
		if a, ok := st.Cbts[r.Att]; ok && a.Crit >= 2 && a.Crit < normal {
			critMin = a.Crit
		}
	}
	echec := st.echecNaturel()
	switch {
	case r.AutoCrit && st.critiqueDm() > 0: // l'état de la cible : touchée automatiquement, critique
		r.Verdict = "crit"
	case r.AutoCrit:
		r.Verdict = "hit"
	case echec > 0 && r.Nat == echec: // l'action échoue même si le total indique une réussite
		r.Verdict = "miss"
	case critMin > 0 && r.Nat >= critMin:
		r.Verdict = "crit"
	case r.Total >= r.Defense:
		r.Verdict = "hit"
	default:
		r.Verdict = "miss"
	}
	r.DmDetail, r.Brut, r.Final = nil, nil, nil
	if r.Verdict != "miss" && !r.DmReel && strings.TrimSpace(r.DmExpr) != "" {
		mult := 1
		if r.Verdict == "crit" {
			mult = st.critiqueDm()
		}
		j, ok := Lancer(r.DmExpr, mult)
		if !ok {
			return ErrAction(fmt.Sprintf("Formule de dégâts illisible : « %s ». Exemple : 1d8+2.", r.DmExpr))
		}
		brut, final := j.Total, max(0, j.Total-r.Rd)
		d := j.Detail
		r.Brut, r.Final, r.DmDetail = &brut, &final, &d
	}
	r.SansDm = r.Verdict != "miss" && r.Brut == nil && !r.DmReel
	pc := ""
	if r.Chance {
		pc = fmt.Sprintf(" + %d PC", st.chanceBonus())
	}
	r.CalcAffiche = fmt.Sprintf("%s%s = %d vs DEF %d", r.Calc, pc, r.Total, r.Defense)
	libelle := map[string]string{"hit": "touché", "crit": "CRITIQUE", "miss": "raté"}[r.Verdict]
	if r.Verdict == "miss" && echec > 0 && r.Nat == echec {
		libelle = "échec critique"
	}
	if r.AutoCrit {
		libelle = "touché d'office par l'état de la cible"
		if r.Verdict == "crit" {
			libelle = "critique automatique par l'état de la cible"
		}
	}
	nomA, nomT := "?", "?"
	if a, ok := st.Cbts[r.Att]; ok {
		nomA = a.Nom
	}
	if t, ok := st.Cbts[r.Cible]; ok {
		nomT = t.Nom
	}
	typ, ok := typesAtk[r.TypeAtk]
	if !ok {
		typ = "au contact"
	}
	if r.Chance {
		st.log(fmt.Sprintf("%s dépense un point de chance : %s → %s", gras(nomA), r.CalcAffiche, libelle), "atk")
	} else {
		st.log(fmt.Sprintf("%s attaque %s (%s, %s) : %s → %s", gras(nomA), gras(nomT), r.Attaque, typ, r.CalcAffiche, libelle), "atk")
	}
	return nil
}

// chance : un point de chance s'ajoute au dé, après avoir pris connaissance du résultat.
func (st *Etat) chance() error {
	if st.chanceBonus() == 0 {
		return ErrAction("Les règles de cet univers ne prévoient pas de point de chance.")
	}
	r := st.Resultat
	if r == nil || r.Applique != nil {
		return ErrAction("Aucune attaque en cours.")
	}
	if r.Chance {
		return ErrAction("Un point de chance a déjà été dépensé sur ce jet.")
	}
	a, ok := st.Cbts[r.Att]
	if !ok || a.Camp != "pj" || a.Pc <= 0 {
		return ErrAction("Seul un PJ qui a encore des points de chance peut en dépenser.")
	}
	a.Pc--
	r.Chance = true
	r.Total += st.chanceBonus()
	if err := st.resoudre(r); err != nil {
		return err
	}
	if r.Evt > 0 && r.Evt <= len(st.Evts) {
		st.Evts[r.Evt-1].V, st.Evts[r.Evt-1].Pc = r.Verdict, true
	}
	return nil
}

// appliquerDes : dés réels. v est le total annoncé à la table (dés + bonus) ; le critique le
// multiplie (bonus inclus) et la RD de la cible s'en retranche, comme pour des dés lancés ici.
func (st *Etat) appliquerDes(v int) error {
	r := st.Resultat
	if r == nil || r.Applique != nil || r.Verdict == "miss" || !r.DmReel {
		return ErrAction("Aucun dégât à saisir.")
	}
	if v < 0 {
		return ErrAction("Saisissez le total des dés de dégâts.")
	}
	brut, detail := v, strconv.Itoa(v)
	if m := st.critiqueDm(); r.Verdict == "crit" && m > 1 {
		brut, detail = m*v, fmt.Sprintf("(%d) × %d", v, m)
	}
	final := max(0, brut-r.Rd)
	if err := st.appliquerDm(final); err != nil {
		return err
	}
	r.Brut, r.Final, r.DmDetail = &brut, &final, &detail
	return nil
}

func (st *Etat) appliquerDm(v int) error {
	r := st.Resultat
	if r == nil || r.Applique != nil || r.Verdict == "miss" {
		return ErrAction("Aucun dégât en attente.")
	}
	t, err := st.get(r.Cible)
	if err != nil {
		return err
	}
	v = max(0, v)
	nomA := "?"
	if a, ok := st.Cbts[r.Att]; ok {
		nomA = a.Nom
	}
	st.source = r.Att
	avant := t.Pv
	err = st.modifierPv(t.ID, -v, fmt.Sprintf("%s subit %d DM de %s", gras(t.Nom), v, gras(nomA)))
	st.source = ""
	if err != nil {
		return err
	}
	if r.Evt > 0 && r.Evt <= len(st.Evts) {
		st.Evts[r.Evt-1].Dm, st.Evts[r.Evt-1].Perte, st.Evts[r.Evt-1].PvC = v, avant-t.Pv, t.Pv
	}
	r.Applique = &v
	return nil
}

// ---------------------------------------------------------------- actions

type Action struct {
	Carac      string                     `json:"carac"`
	Difficulte int                        `json:"difficulte"`
	Opposant   string                     `json:"opposant"`
	CaracOpp   string                     `json:"carac_opp"`
	NatOpp     *int                       `json:"nat_opp"`
	EtatEchec  string                     `json:"etat_echec"`
	Type       string                     `json:"type"`
	ID         string                     `json:"id"`
	Sens       string                     `json:"sens"`
	Att        string                     `json:"att"`
	Cible      string                     `json:"cible"`
	Attaque    int                        `json:"attaque"`
	Mode       string                     `json:"mode"`
	Nat        *int                       `json:"nat"`
	Divers     int                        `json:"divers"`
	Pc         bool                       `json:"pc"`
	Dm         *string                    `json:"dm"`
	Valeur     json.RawMessage            `json:"valeur"`
	Etat       string                     `json:"etat"`
	Duree      int                        `json:"duree"`
	Cle        string                     `json:"cle"`
	Champs     map[string]json.RawMessage `json:"champs"`
	Kind       string                     `json:"kind"`
	RefID      int64                      `json:"ref_id"`
	Qte        int                        `json:"qte"`
	Camp       string                     `json:"camp"`
	Fiche      *FicheCombat               `json:"fiche"`
	Act        string                     `json:"act"`     // attaque : "A", "L", "LS" (suite de la capacité) ou "G" (gratuite)
	CoutPm     int                        `json:"cout_pm"` // attaque : points de mana dépensés (sort)
}

func (p Action) valeurInt() int {
	var f float64
	_ = json.Unmarshal(p.Valeur, &f)
	return int(f)
}

func (p Action) valeurBool() bool {
	var b bool
	_ = json.Unmarshal(p.Valeur, &b)
	return b
}

// Appliquer exécute une action sur l'état. En cas d'erreur, l'état peut être partiellement
// modifié : l'appelant doit restaurer sa copie.
func (st *Etat) Appliquer(p Action) error {
	if st.Termine && p.Type != "rouvrir" && p.Type != "vider_journal" {
		return ErrAction("Ce combat est terminé. Rouvrez-le pour le modifier.")
	}
	if err := st.appliquer(p); err != nil {
		return err
	}
	st.verifierFin()
	return nil
}

func (st *Etat) appliquer(p Action) error {
	switch p.Type {
	case "suivant":
		return st.suivant()
	case "precedent":
		return st.precedent()
	case "trier":
		st.Trier()
	case "deplacer":
		c, err := st.get(p.ID)
		if err != nil {
			return err
		}
		i := indexOf(st.Ordre, c.ID)
		j := i + 1
		if p.Sens == "haut" {
			j = i - 1
		}
		if j >= 0 && j < len(st.Ordre) {
			cur := st.Ordre[st.Tour]
			st.Ordre[i], st.Ordre[j] = st.Ordre[j], st.Ordre[i]
			st.Tour = indexOf(st.Ordre, cur)
		}
	case "ajouter":
		if p.Fiche == nil || strings.TrimSpace(p.Fiche.Nom) == "" {
			return ErrAction("Le combattant doit avoir un nom.")
		}
		qte := min(30, max(1, p.Qte))
		camp := p.Camp
		if camp == "" {
			camp = "ennemi"
			if p.Kind == "pj" {
				camp = "pj"
			}
		}
		vide := len(st.Ordre) == 0
		for _, c := range InstancierGroupe(*p.Fiche, qte, camp, p.Kind, st.Cbts) {
			st.inserer(c)
		}
		txt := "Entrée en combat : " + gras(p.Fiche.Nom)
		if qte > 1 {
			txt += fmt.Sprintf(" ×%d", qte)
		}
		st.log(fmt.Sprintf("%s (init %d)", txt, p.Fiche.Init), "")
		if vide {
			st.Tour = 0
			st.debutTour()
		}
	case "retirer":
		c, err := st.get(p.ID)
		if err != nil {
			return err
		}
		i := indexOf(st.Ordre, c.ID)
		etaitCourant := i == st.Tour
		st.Ordre = append(st.Ordre[:i], st.Ordre[i+1:]...)
		delete(st.Cbts, c.ID)
		for _, o := range st.Cbts {
			if o.Cible != nil && *o.Cible == c.ID {
				o.Cible = nil
			}
		}
		if i < st.Tour {
			st.Tour--
		}
		if st.Tour >= len(st.Ordre) {
			st.Tour = 0
		}
		st.log(gras(c.Nom)+" quitte le combat.", "")
		if etaitCourant && len(st.Ordre) > 0 {
			st.debutTour()
		}
	case "modifier":
		c, err := st.get(p.ID)
		if err != nil {
			return err
		}
		return modifier(c, p.Champs)
	case "pv":
		v := p.valeurInt()
		if v <= 0 {
			return ErrAction("Indiquez une valeur positive.")
		}
		if p.Sens == "degats" {
			v = -v
		}
		return st.modifierPv(p.ID, v, "")
	case "pm":
		return st.mana(p)
	case "usage":
		return st.usage(p)
	case "attaque":
		return st.attaquer(p)
	case "chance":
		if p.Cle == "test" {
			return st.chanceTest()
		}
		return st.chance()
	case "test":
		return st.test(p)
	case "fermer_test":
		st.Test = nil
	case "appliquer_dm":
		if p.Cle == "des" {
			return st.appliquerDes(p.valeurInt())
		}
		return st.appliquerDm(p.valeurInt())
	case "ignorer_dm":
		st.Resultat = nil
	case "defense":
		return st.defense(p.Etat)
	case "etat":
		c, err := st.get(p.ID)
		if err != nil {
			return err
		}
		nom, err := nomEtat(p.Etat)
		if err != nil {
			return err
		}
		if st.rg.Etat(nom).Action != "" {
			if cur := st.Courant(); cur == nil || cur.ID != c.ID {
				return ErrAction("Cet état se prend à son propre tour.")
			}
			return st.defense(nom)
		}
		p.Etat = nom
		d := max(0, p.Duree)
		c.Etats[p.Etat] = Duree(d)
		suffixe := "."
		if d == 1 {
			suffixe = " (1 round)."
		} else if d > 1 {
			suffixe = fmt.Sprintf(" (%d rounds).", d)
		}
		st.log(gras(c.Nom)+" est "+strings.ToLower(p.Etat)+suffixe, "etat")
	case "retirer_etat":
		c, err := st.get(p.ID)
		if err != nil {
			return err
		}
		if _, ok := c.Etats[p.Etat]; ok {
			delete(c.Etats, p.Etat)
			st.log(gras(c.Nom)+" n'est plus "+strings.ToLower(p.Etat)+".", "etat")
		}
	case "action_utilisee":
		c := st.Courant()
		if c == nil || (p.Cle != "A" && p.Cle != "M" && p.Cle != "L") {
			return ErrAction("Action inconnue.")
		}
		if !p.valeurBool() {
			return ErrAction("Une action utilisée ne se rend pas : utilisez « Annuler » pour corriger une erreur.")
		}
		if err := st.consommer(c, p.Cle); err != nil {
			return err
		}
		st.log(gras(c.Nom)+" utilise "+map[string]string{"A": "son action d'attaque", "M": "une action de mouvement", "L": "une action limitée"}[p.Cle]+".", "")
	case "vider_journal":
		st.Log = []LigneLog{}
	case "terminer":
		st.terminer()
	case "continuer":
		if st.Fin == "" {
			return ErrAction("Aucune fin de combat proposée.")
		}
		st.FinIgnoree, st.Fin = st.Fin, ""
		st.log("Le combat continue.", "turn")
	case "rouvrir":
		st.Termine, st.FinA, st.Recap = false, 0, nil
		st.FinIgnoree = st.Issue()
		st.log("Combat rouvert.", "turn")
	default:
		return ErrAction("Action inconnue : " + p.Type)
	}
	return nil
}

// nomEtat nettoie le nom d'un état. Un état que les règles ne décrivent pas est permis : il
// n'a aucun effet, c'est une note sur le combattant.
func nomEtat(nom string) (string, error) {
	nom = strings.Join(strings.Fields(nom), " ")
	if nom == "" || len([]rune(nom)) > 40 {
		return "", ErrAction("Nommez l'état (40 caractères au plus).")
	}
	return nom, nil
}

// defense : le combattant qui a la main prend un état qui lui coûte une action (posture défensive
// décrite par les règles).
func (st *Etat) defense(etat string) error {
	c := st.Courant()
	if c == nil {
		return ErrAction("Aucun combattant n'a la main.")
	}
	ef := st.rg.Etat(etat)
	if ef.Action == "" {
		return ErrAction("Action défensive inconnue.")
	}
	if err := st.consommer(c, ef.Action); err != nil {
		return err
	}
	c.Etats[etat] = 0
	effet := ""
	if ef.Def != 0 {
		effet = fmt.Sprintf(" (DEF %+d)", ef.Def)
	}
	st.log(gras(c.Nom)+" se met en "+strings.ToLower(etat)+effet+".", "etat")
	return nil
}

func modifier(c *Combattant, champs map[string]json.RawMessage) error {
	entier := func(k string, dst *int) {
		if v, ok := champs[k]; ok {
			var f float64
			if json.Unmarshal(v, &f) == nil {
				*dst = int(f)
			}
		}
	}
	if v, ok := champs["nom"]; ok {
		var s string
		if json.Unmarshal(v, &s) == nil && strings.TrimSpace(s) != "" {
			c.Nom = strings.TrimSpace(s)
		}
	}
	if v, ok := champs["camp"]; ok {
		var s string
		if json.Unmarshal(v, &s) == nil {
			if _, ok := rang[s]; ok {
				c.Camp = s
			}
		}
	}
	if v, ok := champs["capacites"]; ok {
		_ = json.Unmarshal(v, &c.Capacites)
	}
	if v, ok := champs["attaques"]; ok {
		var a []fiches.Attaque
		if json.Unmarshal(v, &a) == nil {
			c.Attaques = a
		}
	}
	entier("init", &c.Init)
	entier("defense", &c.Defense)
	entier("pvMax", &c.PvMax)
	entier("pv", &c.Pv)
	entier("rd", &c.Rd)
	entier("crit", &c.Crit)
	entier("pc", &c.Pc)
	entier("pmMax", &c.PmMax)
	entier("pm", &c.Pm)
	c.PvMax = max(1, c.PvMax)
	c.Pv = max(0, min(c.PvMax, c.Pv))
	c.PmMax = max(0, c.PmMax)
	c.Pm = max(0, min(c.PmMax, c.Pm))
	c.Rd = max(0, c.Rd)
	c.Pc = max(0, c.Pc)
	if c.Crit < 2 {
		c.Crit = 0
	}
	return nil
}

func indexOf(s []string, v string) int {
	for i, x := range s {
		if x == v {
			return i
		}
	}
	return -1
}

// Resume : ligne de la liste des combats.
type Resume struct {
	ID      int64   `json:"id"`
	Nom     string  `json:"nom"`
	Round   int     `json:"round"`
	Termine bool    `json:"termine"`
	Updated float64 `json:"updated"`
	Pj      int     `json:"pj"`
	Ennemis int     `json:"ennemis"`
	Total   int     `json:"total"`
	Issue   string  `json:"issue,omitempty"`
}

func (st *Etat) Resume(id int64, updated float64) Resume {
	r := Resume{ID: id, Nom: st.Nom, Round: st.Round, Termine: st.Termine, Updated: updated, Total: len(st.Cbts)}
	if st.Recap != nil {
		r.Issue = st.Recap.Issue
	}
	for _, c := range st.Cbts {
		if c.Camp == "pj" {
			r.Pj++
		}
		if c.Camp == "ennemi" && c.Pv > 0 {
			r.Ennemis++
		}
	}
	return r
}

// ---------------------------------------------------------------- tests de caractéristique

type JetTest struct {
	ID       string `json:"id"`
	Nom      string `json:"nom"`
	Carac    string `json:"carac"`
	De       int    `json:"de"`
	Des      []int  `json:"des"` // deux dés pour une Carac. supérieure
	Nat      int    `json:"nat"`
	Mod      int    `json:"mod"`
	Divers   int    `json:"divers"`
	Chance   bool   `json:"chance"`
	Total    int    `json:"total"`
	Critique string `json:"critique"` // "reussite", "echec" ou ""
}

type ResultatTest struct {
	Jet          JetTest  `json:"jet"`
	Opposant     *JetTest `json:"opposant"`
	Difficulte   int      `json:"difficulte"`
	Reussi       bool     `json:"reussi"`
	Explication  string   `json:"explication"`
	EtatEchec    string   `json:"etat_echec"`
	Duree        int      `json:"duree"`
	EtatApplique bool     `json:"etat_applique"`
}

func superieure(c *Combattant, carac string) bool {
	for _, s := range c.Superieures {
		if s == carac {
			return true
		}
	}
	return false
}

// jet : le dé normal, ou celui qu'impose un état ; caractéristique supérieure : deux dés, on garde
// le meilleur.
func (st *Etat) jet(c *Combattant, carac string, nat *int, divers int) (JetTest, error) {
	normal := st.rg.DeNormal()
	j := JetTest{ID: c.ID, Nom: c.Nom, Carac: carac, De: normal, Mod: c.Carac.Get(carac), Divers: divers}
	for e := range c.Etats {
		if d := st.rg.Etat(e).De; d != 0 && d < j.De {
			j.De = d
		}
	}
	if nat != nil {
		if *nat < 1 || *nat > j.De {
			return j, ErrAction(fmt.Sprintf("%s : saisissez le d%d conservé (1 à %d).", c.Nom, j.De, j.De))
		}
		j.Nat = *nat
		j.Des = []int{*nat}
	} else {
		j.Des = []int{De(j.De)}
		if superieure(c, carac) {
			j.Des = append(j.Des, De(j.De))
		}
		j.Nat = j.Des[0]
		for _, d := range j.Des {
			j.Nat = max(j.Nat, d)
		}
	}
	j.Total = j.Nat + j.Mod + divers
	if st.critiqueDm() > 0 && j.De == normal && j.Nat == normal {
		j.Critique = "reussite"
	} else if e := st.echecNaturel(); e > 0 && j.Nat == e {
		j.Critique = "echec"
	}
	return j, nil
}

func (j JetTest) calcul() string {
	des := ""
	if len(j.Des) > 1 {
		parts := make([]string, len(j.Des))
		for i, d := range j.Des {
			parts[i] = strconv.Itoa(d)
		}
		des = " [" + strings.Join(parts, ", ") + " : meilleur gardé]"
	}
	out := fmt.Sprintf("d%d %d%s %s %s", j.De, j.Nat, des, signe(j.Mod), j.Carac)
	if j.Divers != 0 {
		out += " " + signe(j.Divers)
	}
	if j.Chance {
		out += " + PC"
	}
	return fmt.Sprintf("%s = %d", out, j.Total)
}

func (st *Etat) test(p Action) error {
	c, err := st.get(p.ID)
	if err != nil {
		return err
	}
	valide := false
	for _, k := range fiches.Caracs {
		if p.Carac == k {
			valide = true
		}
	}
	if !valide {
		return ErrAction("Choisissez une caractéristique.")
	}
	if p.EtatEchec != "" {
		if p.EtatEchec, err = nomEtat(p.EtatEchec); err != nil {
			return err
		}
	}
	j, err := st.jet(c, p.Carac, p.Nat, p.Divers)
	if err != nil {
		return err
	}
	r := &ResultatTest{Jet: j, Difficulte: p.Difficulte, EtatEchec: p.EtatEchec, Duree: max(0, p.Duree)}
	if p.Opposant != "" {
		o, err := st.get(p.Opposant)
		if err != nil {
			return err
		}
		if o.ID == c.ID {
			return ErrAction("Choisissez un autre combattant comme opposant.")
		}
		co := p.CaracOpp
		if co == "" {
			co = p.Carac
		}
		jo, err := st.jet(o, co, p.NatOpp, 0)
		if err != nil {
			return err
		}
		r.Opposant = &jo
	}
	st.Test = r
	st.trancherTest()
	return nil
}

// trancherTest : test simple (total ≥ difficulté) ou en opposition (plus grand total ; une réussite
// critique l'emporte ; égalité : le défenseur l'emporte).
func (st *Etat) trancherTest() {
	r := st.Test
	j := r.Jet
	if r.Opposant == nil {
		switch {
		case j.Critique == "reussite":
			r.Reussi, r.Explication = true, fmt.Sprintf("Réussite critique (%d naturel).", j.Nat)
		case j.Critique == "echec":
			r.Reussi, r.Explication = false, fmt.Sprintf("Échec critique (%d naturel).", j.Nat)
		default:
			r.Reussi = j.Total >= r.Difficulte
			r.Explication = fmt.Sprintf("%d contre une difficulté de %d.", j.Total, r.Difficulte)
		}
	} else {
		o := *r.Opposant
		switch {
		case j.Critique == "reussite" && o.Critique != "reussite":
			r.Reussi, r.Explication = true, "Réussite critique : remporte l'opposition."
		case o.Critique == "reussite" && j.Critique != "reussite":
			r.Reussi, r.Explication = false, o.Nom+" obtient une réussite critique et l'emporte."
		case j.Critique == "echec":
			r.Reussi, r.Explication = false, fmt.Sprintf("Échec critique (%d naturel).", j.Nat)
		case j.Total > o.Total:
			r.Reussi, r.Explication = true, fmt.Sprintf("%d contre %d.", j.Total, o.Total)
		case j.Total == o.Total:
			r.Reussi, r.Explication = false, fmt.Sprintf("Égalité à %d : le défenseur (%s) l'emporte.", j.Total, o.Nom)
		default:
			r.Reussi, r.Explication = false, fmt.Sprintf("%d contre %d.", j.Total, o.Total)
		}
	}
	libelle := "réussi"
	if !r.Reussi {
		libelle = "raté"
	}
	contre := fmt.Sprintf("difficulté %d", r.Difficulte)
	if r.Opposant != nil {
		contre = fmt.Sprintf("%s de %s (%s)", r.Opposant.Carac, gras(r.Opposant.Nom), r.Opposant.calcul())
	}
	st.log(fmt.Sprintf("Test de %s de %s contre %s : %s → %s", j.Carac, gras(j.Nom), contre, j.calcul(), libelle), "etat")
	if !r.Reussi && r.EtatEchec != "" && !r.EtatApplique {
		if c, ok := st.Cbts[j.ID]; ok {
			c.Etats[r.EtatEchec] = Duree(r.Duree)
			r.EtatApplique = true
			suffixe := "."
			if r.Duree == 1 {
				suffixe = " (1 round)."
			} else if r.Duree > 1 {
				suffixe = fmt.Sprintf(" (%d rounds).", r.Duree)
			}
			st.log(gras(c.Nom)+" est "+strings.ToLower(r.EtatEchec)+suffixe, "etat")
		}
	}
}

// chanceTest : un PJ dépense un point de chance après un test raté.
func (st *Etat) chanceTest() error {
	if st.chanceBonus() == 0 {
		return ErrAction("Les règles de cet univers ne prévoient pas de point de chance.")
	}
	r := st.Test
	if r == nil {
		return ErrAction("Aucun test en cours.")
	}
	if r.Reussi || r.Jet.Chance || r.Jet.Critique == "echec" {
		return ErrAction("Un point de chance ne peut servir que sur un test raté, une seule fois, et pas sur un échec critique.")
	}
	c, ok := st.Cbts[r.Jet.ID]
	if !ok || c.Camp != "pj" || c.Pc <= 0 {
		return ErrAction("Seul un PJ qui a encore des points de chance peut en dépenser.")
	}
	c.Pc--
	r.Jet.Chance = true
	r.Jet.Total += st.chanceBonus()
	// l'état infligé par l'échec est retiré si le test devient une réussite
	if r.EtatApplique {
		delete(c.Etats, r.EtatEchec)
		r.EtatApplique = false
	}
	st.log(gras(c.Nom)+" dépense un point de chance sur son test.", "etat")
	st.trancherTest()
	if r.Reussi && r.EtatEchec != "" {
		st.log(gras(c.Nom)+" évite finalement l'état "+strings.ToLower(r.EtatEchec)+".", "etat")
	}
	return nil
}

// mana : un combattant dépense (sens « depense ») ou récupère des points de mana, dans ses bornes.
func (st *Etat) mana(p Action) error {
	c, err := st.get(p.ID)
	if err != nil {
		return err
	}
	v := p.valeurInt()
	if v <= 0 {
		return ErrAction("Indiquez une valeur positive.")
	}
	if c.PmMax == 0 {
		return ErrAction(c.Nom + " n'a pas de points de mana.")
	}
	if p.Sens == "depense" {
		if v > c.Pm {
			return ErrAction(fmt.Sprintf("%s n'a que %d PM.", c.Nom, c.Pm))
		}
		c.Pm -= v
		st.log(fmt.Sprintf("%s dépense %d PM (reste %d).", gras(c.Nom), v, c.Pm), "")
		return nil
	}
	gagne := min(v, c.PmMax-c.Pm)
	c.Pm += gagne
	st.log(fmt.Sprintf("%s récupère %d PM (%d / %d).", gras(c.Nom), gagne, c.Pm, c.PmMax), "")
	return nil
}

// usage : une capacité à usage limité (« une fois par combat ») est marquée utilisée, ou rendue.
func (st *Etat) usage(p Action) error {
	c, err := st.get(p.ID)
	if err != nil {
		return err
	}
	nom := strings.Join(strings.Fields(p.Cle), " ")
	if nom == "" || len([]rune(nom)) > 80 {
		return ErrAction("Nommez la capacité (80 caractères au plus).")
	}
	i := slices.Index(c.Utilise, nom)
	if p.valeurBool() {
		if i >= 0 {
			return ErrAction(fmt.Sprintf("%s a déjà utilisé « %s » dans ce combat.", c.Nom, nom))
		}
		if len(c.Utilise) >= 40 {
			return ErrAction("Trop de capacités notées.")
		}
		c.Utilise = append(c.Utilise, nom)
		st.log(fmt.Sprintf("%s utilise « %s ».", gras(c.Nom), nom), "")
		return nil
	}
	if i >= 0 {
		c.Utilise = slices.Delete(c.Utilise, i, i+1)
		st.log(fmt.Sprintf("%s peut de nouveau utiliser « %s ».", gras(c.Nom), nom), "")
	}
	return nil
}
