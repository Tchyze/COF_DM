package combat

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Evenement : trace structurée d'un fait de combat, pour le récapitulatif.
type Evenement struct {
	R     int    `json:"r"`
	T     string `json:"t"` // atk, dm (dégâts sans attaquant), soin, ko
	A     string `json:"a,omitempty"`
	C     string `json:"c,omitempty"`
	AN    string `json:"an,omitempty"`
	CN    string `json:"cn,omitempty"`
	ACamp string `json:"ac,omitempty"`
	CCamp string `json:"cc,omitempty"`
	ARef  int64  `json:"ar,omitempty"` // fiche de personnage de l'auteur (PJ)
	CRef  int64  `json:"cr,omitempty"`
	V     string `json:"v,omitempty"` // hit, crit, miss
	Nat   int    `json:"nat,omitempty"`
	De    int    `json:"de,omitempty"`
	Dm    int    `json:"dm,omitempty"`    // dégâts du coup
	Perte int    `json:"perte,omitempty"` // PV réellement perdus par la cible (sans l'excédent sous 0)
	PvC   int    `json:"pvc,omitempty"`   // PV de la cible après le coup
	Pc    bool   `json:"pc,omitempty"`    // point de chance dépensé sur ce jet
	Atk   string `json:"atk,omitempty"`
}

func refPj(c *Combattant) int64 {
	if c != nil && c.Camp == "pj" && c.Ref != nil && c.Ref.Kind == "pj" {
		return c.Ref.ID
	}
	return 0
}

// evt ajoute un événement et renvoie son numéro (à partir de 1).
func (st *Etat) evt(t string, a, c *Combattant) *Evenement {
	e := Evenement{R: st.Round, T: t}
	if a != nil {
		e.A, e.AN, e.ACamp, e.ARef = a.ID, a.Nom, a.Camp, refPj(a)
	}
	if c != nil {
		e.C, e.CN, e.CCamp, e.CRef = c.ID, c.Nom, c.Camp, refPj(c)
	}
	st.Evts = append(st.Evts, e)
	return &st.Evts[len(st.Evts)-1]
}

// PointPv : total des PV des PJ à un instant du combat.
type PointPv struct {
	R   int `json:"r"`
	Pv  int `json:"pv"`
	Max int `json:"max"`
}

func (st *Etat) pointPv() PointPv {
	p := PointPv{R: st.Round}
	for _, c := range st.Cbts {
		if c.Camp == "pj" {
			p.Pv += max(0, c.Pv)
			p.Max += c.PvMax
		}
	}
	return p
}

// noterCourbe enregistre les PV du groupe au début du round (remplace un point du même round).
func (st *Etat) noterCourbe() {
	p := st.pointPv()
	if p.Max == 0 {
		return
	}
	if n := len(st.Courbe); n > 0 && st.Courbe[n-1].R >= p.R {
		st.Courbe = st.Courbe[:n-1]
		for len(st.Courbe) > 0 && st.Courbe[len(st.Courbe)-1].R >= p.R {
			st.Courbe = st.Courbe[:len(st.Courbe)-1]
		}
	}
	st.Courbe = append(st.Courbe, p)
}

// ---------------------------------------------------------------- fin de combat

// Issue : "victoire" si tous les adversaires sont à terre, "defaite" si tous les PJ le sont, sinon "".
func (st *Etat) Issue() string {
	nPj, pjDebout, nEn, enDebout := 0, 0, 0, 0
	for _, c := range st.Cbts {
		switch c.Camp {
		case "pj":
			nPj++
			if c.Pv > 0 {
				pjDebout++
			}
		case "ennemi":
			nEn++
			if c.Pv > 0 {
				enDebout++
			}
		}
	}
	switch {
	case nPj > 0 && pjDebout == 0:
		return "defaite"
	case nEn > 0 && enDebout == 0:
		return "victoire"
	}
	return ""
}

// verifierFin propose la fin du combat dès qu'un camp est entièrement à terre.
func (st *Etat) verifierFin() {
	if st.Termine {
		st.Fin = ""
		return
	}
	issue := st.Issue()
	switch {
	case issue == "":
		st.Fin, st.FinIgnoree = "", ""
	case issue != st.FinIgnoree && st.Fin != issue:
		st.Fin = issue
		st.Recap = st.CalculerRecap()
		st.log(map[string]string{"victoire": "Tous les adversaires sont à terre : victoire !", "defaite": "Tous les PJ sont à terre…"}[issue], "turn")
	}
}

func (st *Etat) terminer() {
	st.Termine, st.Fin = true, ""
	st.FinA = time.Now().Unix()
	st.Recap = st.CalculerRecap()
	st.log("Fin du combat.", "turn")
}

// ---------------------------------------------------------------- récapitulatif

type Moment struct {
	Cle   string `json:"cle"`
	Titre string `json:"titre"`
	Qui   string `json:"qui"`
	Camp  string `json:"camp"`
	Texte string `json:"texte"`
}

type BilanPJ struct {
	Nom          string `json:"nom"`
	RefID        int64  `json:"ref_id,omitempty"`
	Infliges     int    `json:"infliges"`
	Subis        int    `json:"subis"`
	Touches      int    `json:"touches"`
	Tentatives   int    `json:"tentatives"`
	Critiques    int    `json:"critiques"`
	Rates        int    `json:"rates"`
	Echecs       int    `json:"echecs"` // 1 naturels
	Abattus      int    `json:"abattus"`
	Soins        int    `json:"soins"`
	Tombe        bool   `json:"tombe"`
	MeilleurCoup int    `json:"meilleur_coup"`
	MeilleurTxt  string `json:"meilleur_txt,omitempty"`
	Titre        string `json:"titre"`
	TitreDesc    string `json:"titre_desc"`
}

type Heros struct {
	Nom   string `json:"nom"`
	Texte string `json:"texte"`
}

type Recap struct {
	Heros    *Heros    `json:"heros,omitempty"`
	Courbe   []PointPv `json:"courbe"` // PV du groupe au début de chaque round, puis à la fin
	Nom      string    `json:"nom"`
	Issue    string    `json:"issue"`
	Rounds   int       `json:"rounds"`
	DureeMin int       `json:"duree_min"`
	Date     int64     `json:"date"`
	Vaincus  int       `json:"vaincus"`
	Ennemis  int       `json:"ennemis"`
	Tombes   []string  `json:"tombes"`
	Moments  []Moment  `json:"moments"`
	Pjs      []BilanPJ `json:"pjs"`
}

type statCbt struct {
	nom, camp                               string
	ref                                     int64
	infliges, subis, touches, tent, crits   int
	rates, echecs, abattus, soins, meilleur int
	meilleurTxt                             string
	contrePj                                int // PV retirés aux PJ (pour la némésis)
	pvMin                                   int // plus bas niveau de PV atteint (-1 = jamais touché)
	pvMinR                                  int
}

func (st *Etat) CalculerRecap() *Recap {
	maintenant := time.Now().Unix()
	r := &Recap{Nom: st.Nom, Issue: st.Issue(), Rounds: st.Round, Date: maintenant, Tombes: []string{}, Moments: []Moment{}, Pjs: []BilanPJ{}}
	if st.Debut > 0 {
		r.DureeMin = int((maintenant - st.Debut + 30) / 60)
	}
	stats := map[string]*statCbt{}
	get := func(id, nom, camp string, ref int64) *statCbt {
		s, ok := stats[id]
		if !ok {
			s = &statCbt{nom: nom, camp: camp, ref: ref, pvMin: -1}
			stats[id] = s
		}
		return s
	}
	for _, c := range st.Cbts {
		get(c.ID, c.Nom, c.Camp, refPj(c))
		switch {
		case c.Camp == "ennemi":
			r.Ennemis++
			if c.Pv <= 0 {
				r.Vaincus++
			}
		case c.Camp == "pj" && c.Pv <= 0:
			r.Tombes = append(r.Tombes, c.Nom)
		}
	}
	sort.Strings(r.Tombes)

	var gros *Evenement
	var grace, chance *Evenement
	for i := range st.Evts {
		e := &st.Evts[i]
		switch e.T {
		case "atk":
			a := get(e.A, e.AN, e.ACamp, e.ARef)
			a.tent++
			switch e.V {
			case "miss":
				a.rates++
				if n := st.echecNaturel(); n > 0 && e.Nat == n {
					a.echecs++
				}
			case "crit":
				a.crits++
				a.touches++
			default:
				a.touches++
			}
			if e.Dm > 0 {
				a.infliges += e.Perte
				cb := get(e.C, e.CN, e.CCamp, e.CRef)
				cb.subis += e.Perte
				cb.notePv(e.PvC, e.R)
				if e.CCamp == "pj" && e.ACamp != "pj" {
					a.contrePj += e.Perte
				}
				if e.Dm > a.meilleur {
					a.meilleur, a.meilleurTxt = e.Dm, fmt.Sprintf("%d DM à %s (round %d)", e.Dm, e.CN, e.R)
				}
				if gros == nil || e.Dm > gros.Dm {
					gros = e
				}
			}
			if e.Pc && e.V != "miss" && chance == nil {
				chance = e
			}
		case "dm":
			cb := get(e.C, e.CN, e.CCamp, e.CRef)
			cb.subis += e.Perte
			cb.notePv(e.PvC, e.R)
		case "soin":
			if e.A != "" {
				get(e.A, e.AN, e.ACamp, e.ARef).soins += e.Dm
			}
		case "ko":
			if e.A != "" && e.ACamp != e.CCamp {
				get(e.A, e.AN, e.ACamp, e.ARef).abattus++
				if (r.Issue == "victoire" && e.CCamp == "ennemi") || (r.Issue == "defaite" && e.CCamp == "pj") || r.Issue == "" {
					grace = e
				}
			}
		}
	}

	// moments forts (au plus 6, seulement ceux qui ont eu lieu, par ordre d'intérêt)
	add := func(cle, titre, qui, camp, texte string) {
		if len(r.Moments) < 6 {
			r.Moments = append(r.Moments, Moment{cle, titre, qui, camp, texte})
		}
	}
	ids := make([]string, 0, len(stats))
	for id := range stats {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	meilleur := func(val func(*statCbt) int, min int, garder func(*statCbt) bool) *statCbt {
		var best *statCbt
		for _, id := range ids {
			s := stats[id]
			if garder != nil && !garder(s) {
				continue
			}
			if val(s) >= min && (best == nil || val(s) > val(best)) {
				best = s
			}
		}
		return best
	}
	pjSeul := func(s *statCbt) bool { return s.camp == "pj" }
	if gros != nil {
		crit := ""
		if gros.V == "crit" {
			crit = ", coup critique"
		}
		add("gros", "Plus gros coup", gros.AN, gros.ACamp, fmt.Sprintf("%d DM à %s avec %s%s (round %d)", gros.Dm, gros.CN, gros.Atk, crit, gros.R))
	}
	if grace != nil {
		titre := "Coup de grâce"
		if grace.CCamp == "pj" {
			titre = "Coup fatal"
		}
		add("grace", titre, grace.AN, grace.ACamp, fmt.Sprintf("met %s à terre au round %d", grace.CN, grace.R))
	}
	// frôlé la mort : PJ descendu à 2 PV ou moins (10 % de ses PV si c'est plus) et debout à la fin
	var frole *statCbt
	for _, id := range ids {
		s, c := stats[id], st.Cbts[id]
		if s.camp != "pj" || c == nil || c.Pv <= 0 || s.pvMin <= 0 {
			continue
		}
		if s.pvMin <= max(2, c.PvMax/10) && (frole == nil || s.pvMin < frole.pvMin) {
			frole = s
		}
	}
	if frole != nil {
		add("frole", "Frôlé la mort", frole.nom, frole.camp, fmt.Sprintf("tombé à %s au round %d, et toujours debout", pluriel(frole.pvMin, "PV", "PV"), frole.pvMinR))
	}
	if s := meilleur(func(s *statCbt) int { return s.crits }, 1, nil); s != nil {
		add("crit", "Main du destin", s.nom, s.camp, pluriel(s.crits, "coup critique", "coups critiques"))
	}
	if chance != nil {
		add("chance", "Chance décisive", chance.AN, chance.ACamp, fmt.Sprintf("dépense un point de chance et touche %s (round %d)", chance.CN, chance.R))
	}
	if s := meilleur(func(s *statCbt) int { return s.contrePj }, 1, func(s *statCbt) bool { return s.camp != "pj" }); s != nil {
		add("nemesis", "Némésis", s.nom, s.camp, fmt.Sprintf("a retiré %d PV au groupe", s.contrePj))
	}
	if s := meilleur(func(s *statCbt) int { return s.subis }, 1, pjSeul); s != nil {
		add("mur", "Mur vivant", s.nom, s.camp, fmt.Sprintf("a encaissé %d DM", s.subis))
	}
	if s := meilleur(func(s *statCbt) int { return s.rates*10 + s.echecs }, 20, nil); s != nil {
		t := pluriel(s.rates, "attaque ratée", "attaques ratées")
		if s.echecs > 0 {
			t += fmt.Sprintf(" dont %s", pluriel(s.echecs, "échec critique", "échecs critiques"))
		}
		add("maudit", "Maudit des dés", s.nom, s.camp, t)
	}
	if s := meilleur(func(s *statCbt) int { return s.subis }, 1, func(s *statCbt) bool { return s.camp == "ennemi" }); s != nil {
		add("coriace", "Adversaire le plus coriace", s.nom, s.camp, fmt.Sprintf("a encaissé %d DM", s.subis))
	}

	// courbe des PV du groupe : début de chaque round, puis fin du combat
	r.Courbe = append([]PointPv{}, st.Courbe...)
	if fin := st.pointPv(); fin.Max > 0 {
		fin.R = st.Round + 1 // point « fin »
		r.Courbe = append(r.Courbe, fin)
	}

	// bilan par PJ, dans l'ordre d'initiative
	for _, id := range st.Ordre {
		c := st.Cbts[id]
		if c == nil || c.Camp != "pj" {
			continue
		}
		s := stats[id]
		r.Pjs = append(r.Pjs, BilanPJ{Nom: c.Nom, RefID: s.ref, Infliges: s.infliges, Subis: s.subis, Touches: s.touches,
			Tentatives: s.tent, Critiques: s.crits, Rates: s.rates, Echecs: s.echecs, Abattus: s.abattus, Soins: s.soins,
			Tombe: c.Pv <= 0, MeilleurCoup: s.meilleur, MeilleurTxt: s.meilleurTxt})
	}
	attribuerTitres(r.Pjs)
	r.Heros = heros(r.Pjs)
	return r
}

func (s *statCbt) notePv(pv, round int) {
	if s.pvMin < 0 || pv < s.pvMin {
		s.pvMin, s.pvMinR = pv, round
	}
}

// heros : le PJ le plus décisif (dégâts infligés, adversaires abattus, critiques, dégâts encaissés).
func heros(pjs []BilanPJ) *Heros {
	best, bs := -1, 0
	for i, b := range pjs {
		s := b.Infliges + 10*b.Abattus + 5*b.Critiques + b.Subis/2
		if s > bs {
			best, bs = i, s
		}
	}
	if best < 0 {
		return nil
	}
	b := pjs[best]
	var p []string
	if b.Infliges > 0 {
		p = append(p, fmt.Sprintf("%d DM infligés", b.Infliges))
	}
	if b.Abattus > 0 {
		p = append(p, pluriel(b.Abattus, "adversaire abattu", "adversaires abattus"))
	}
	if b.Critiques > 0 {
		p = append(p, pluriel(b.Critiques, "critique", "critiques"))
	}
	if b.Subis > 0 {
		p = append(p, fmt.Sprintf("%d DM encaissés", b.Subis))
	}
	return &Heros{Nom: b.Nom, Texte: strings.Join(p, " · ")}
}

func pluriel(n int, sing, plur string) string {
	if n > 1 {
		return fmt.Sprintf("%d %s", n, plur)
	}
	return fmt.Sprintf("%d %s", n, sing)
}

// attribuerTitres donne à chaque PJ un titre honorifique, sans doublon tant que possible.
func attribuerTitres(pjs []BilanPJ) {
	type regle struct {
		titre, desc string
		score       func(b BilanPJ) int // 0 = non éligible
	}
	regles := []regle{
		{"Le Boucher", "le plus d'adversaires mis à terre", func(b BilanPJ) int { return b.Abattus }},
		{"Main du destin", "le plus de coups critiques", func(b BilanPJ) int { return b.Critiques }},
		{"Œil de faucon", "la meilleure précision", func(b BilanPJ) int {
			if b.Tentatives < 3 || b.Touches*4 < b.Tentatives*3 {
				return 0
			}
			return b.Touches * 100 / b.Tentatives
		}},
		{"Mur vivant", "le plus de dégâts encaissés", func(b BilanPJ) int { return b.Subis }},
		{"Force de frappe", "le plus de dégâts infligés", func(b BilanPJ) int { return b.Infliges }},
		{"Maudit des dés", "le plus d'attaques ratées", func(b BilanPJ) int {
			if b.Rates < 2 {
				return 0
			}
			return b.Rates*10 + b.Echecs
		}},
	}
	pris := map[int]bool{}
	for _, rg := range regles {
		best, bs := -1, 0
		for i, b := range pjs {
			if s := rg.score(b); !pris[i] && s > bs {
				best, bs = i, s
			}
		}
		if best >= 0 {
			pjs[best].Titre, pjs[best].TitreDesc = rg.titre, rg.desc
			pris[best] = true
		}
	}
	for i := range pjs {
		if pris[i] {
			continue
		}
		b := &pjs[i]
		switch {
		case b.Tombe:
			b.Titre, b.TitreDesc = "Tombé au champ d'honneur", "à terre à la fin du combat"
		case b.Subis == 0 && b.Tentatives > 0:
			b.Titre, b.TitreDesc = "Intouchable", "aucun dégât encaissé"
		case b.Tentatives == 0:
			b.Titre, b.TitreDesc = "Le Stratège", "n'a pas eu besoin de frapper"
		default:
			b.Titre, b.TitreDesc = "Compagnon fidèle", "toujours présent dans la mêlée"
		}
	}
}
