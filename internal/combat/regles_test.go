package combat

import (
	"strconv"
	"testing"

	"github.com/Tchyze/cof_dm/internal/fiches"
)

// Le moteur, avec les règles d'essai de essai_test.go (inventées).

func ptr(n int) *int { return &n }

func duel(t *testing.T) (*Etat, *Combattant, *Combattant) {
	t.Helper()
	pj := DepuisFiche(FicheCombat{Nom: "Krush", Init: 11, Defense: 16, Pv: 16, Pc: 2,
		Attaques: []fiches.Attaque{{Nom: "Épée", Type: "C", Bonus: 4, Dm: "1d8+3"}, {Nom: "Arc", Type: "D", Bonus: 1, Dm: "1d6"}}}, "pj", "", "")
	orc := DepuisFiche(FicheCombat{Nom: "Orc", Init: 10, Defense: 14, Pv: 12, Rd: 2,
		Attaques: []fiches.Attaque{{Nom: "Hache", Type: "C", Bonus: 3, Dm: "1d8"}}}, "ennemi", "", "")
	return Nouveau("Duel", []*Combattant{pj, orc}, essai), pj, orc
}

func attaque(t *testing.T, st *Etat, a, c *Combattant, idx, nat int) *Resultat {
	t.Helper()
	// action gratuite : ces tests portent sur la résolution, pas sur le budget d'actions
	if err := st.Appliquer(Action{Type: "attaque", Att: a.ID, Cible: c.ID, Attaque: idx, Mode: "manuel", Nat: ptr(nat), Act: "G"}); err != nil {
		t.Fatal(err)
	}
	return st.Resultat
}

func TestInitiativeEgalites(t *testing.T) {
	// égalité → PJ avant PNJ ; entre deux PJ, la plus haute SAG
	sag := func(v int) *struct {
		SAG int `json:"SAG"`
	} {
		return &struct {
			SAG int `json:"SAG"`
		}{v}
	}
	a := DepuisFiche(FicheCombat{Nom: "Aldo", Init: 12, Pv: 5, Valeurs: sag(10)}, "pj", "", "pj")
	b := DepuisFiche(FicheCombat{Nom: "Zora", Init: 12, Pv: 5, Valeurs: sag(15)}, "pj", "", "pj")
	o := DepuisFiche(FicheCombat{Nom: "Brigand", Init: 12, Pv: 5}, "ennemi", "", "")
	st := Nouveau("Égalités", []*Combattant{o, a, b}, essai)
	if st.Cbts[st.Ordre[0]].Nom != "Zora" || st.Cbts[st.Ordre[1]].Nom != "Aldo" || st.Cbts[st.Ordre[2]].Nom != "Brigand" {
		t.Fatalf("ordre : %s, %s, %s", st.Cbts[st.Ordre[0]].Nom, st.Cbts[st.Ordre[1]].Nom, st.Cbts[st.Ordre[2]].Nom)
	}
	// Ébloui : −6 en initiative
	_ = st.Appliquer(Action{Type: "etat", ID: b.ID, Etat: "Ébloui"})
	_ = st.Appliquer(Action{Type: "trier"})
	if st.Cbts[st.Ordre[2]].Nom != "Zora" {
		t.Fatal("un PJ ébloui (−6 en Init) devrait passer en dernier")
	}
}

func TestAttaqueEtCritiques(t *testing.T) {
	st, pj, orc := duel(t)
	if r := attaque(t, st, pj, orc, 0, 10); r.Verdict != "hit" { // 10 + 4 = 14 ≥ DEF 14
		t.Fatalf("égalité avec la DEF : l'attaque réussit (%s)", r.CalcAffiche)
	}
	if r := attaque(t, st, pj, orc, 0, 9); r.Verdict != "miss" {
		t.Fatal("13 < DEF 14 : raté")
	}
	r := attaque(t, st, pj, orc, 0, 20)
	if r.Verdict != "crit" || *r.Brut%2 != 0 || *r.Final != max(0, *r.Brut-2) {
		t.Fatalf("maximum du dé : critique, DM multipliés bonus inclus, RD soustraite (%v)", *r.DmDetail)
	}
	_ = st.Appliquer(Action{Type: "etat", ID: orc.ID, Etat: "Bousculé"})
	_ = st.Appliquer(Action{Type: "etat", ID: orc.ID, Etat: "Bousculé"})
	orc.Defense = 40
	delete(orc.Etats, "Bousculé")
	if r := attaque(t, st, pj, orc, 0, 1); r.Verdict != "miss" {
		t.Fatal("1 naturel : échec critique")
	}
	orc.Defense = 1
	if r := attaque(t, st, pj, orc, 0, 1); r.Verdict != "miss" {
		t.Fatal("1 naturel : échec même si le total dépasse la DEF")
	}
}

func TestEffetsDesEtats(t *testing.T) {
	st, pj, orc := duel(t)
	// Ébloui : −4 au contact, −9 à distance ; la cible éblouie perd 3 en DEF
	_ = st.Appliquer(Action{Type: "etat", ID: pj.ID, Etat: "Ébloui"})
	if r := attaque(t, st, pj, orc, 0, 10); r.Total != 10+4-4 {
		t.Fatalf("ébloui au contact : %s", r.CalcAffiche)
	}
	if r := attaque(t, st, pj, orc, 1, 10); r.Total != 10+1-9 {
		t.Fatalf("ébloui à distance : %s", r.CalcAffiche)
	}
	if r := attaque(t, st, orc, pj, 0, 10); r.Defense != 16-3 {
		t.Fatalf("DEF d'une cible éblouie : %s", r.CalcAffiche)
	}
	delete(pj.Etats, "Ébloui")
	// Fourbu : dé réduit (d10), dont le maximum n'est pas un critique
	_ = st.Appliquer(Action{Type: "etat", ID: pj.ID, Etat: "Fourbu"})
	if err := st.Appliquer(Action{Type: "attaque", Att: pj.ID, Cible: orc.ID, Mode: "manuel", Nat: ptr(11)}); err == nil {
		t.Fatal("fourbu : un d10 ne peut pas donner 11")
	}
	if r := attaque(t, st, pj, orc, 0, 10); r.Verdict == "crit" || r.De != 10 {
		t.Fatalf("10 au d10 : pas de critique (%+v)", r)
	}
	delete(pj.Etats, "Fourbu")
	// Pétrifié : touché automatiquement et critique
	_ = st.Appliquer(Action{Type: "etat", ID: orc.ID, Etat: "Pétrifié"})
	if r := attaque(t, st, pj, orc, 0, 2); r.Verdict != "crit" {
		t.Fatal("cible pétrifiée : critique automatique")
	}
	// un état que les règles ne décrivent pas se pose quand même, sans effet
	if err := st.Appliquer(Action{Type: "etat", ID: pj.ID, Etat: "Trempé"}); err != nil {
		t.Fatal(err)
	}
	if r := attaque(t, st, pj, orc, 0, 10); r.Total != 10+4 {
		t.Fatalf("état libre : %s", r.CalcAffiche)
	}
}

func TestEtatDuPremierRound(t *testing.T) {
	st, _, orc := duel(t)
	_ = st.Appliquer(Action{Type: "etat", ID: orc.ID, Etat: "Ahuri"})
	_ = st.Appliquer(Action{Type: "suivant"}) // Krush → Orc (round 1)
	if _, ok := orc.Etats["Ahuri"]; !ok {
		t.Fatal("encore ahuri pendant le round 1")
	}
	_ = st.Appliquer(Action{Type: "suivant"}) // round 2
	if _, ok := orc.Etats["Ahuri"]; ok || st.Round != 2 {
		t.Fatal("« Ahuri » doit disparaître à la fin du premier round")
	}
}

func TestPostureDefensive(t *testing.T) {
	st, pj, orc := duel(t)
	_ = st.Appliquer(Action{Type: "etat", ID: pj.ID, Etat: "Retranché"})
	_ = st.Appliquer(Action{Type: "suivant"}) // tour de l'orc
	if r := attaque(t, st, orc, pj, 0, 10); r.Defense != 16+3 {
		t.Fatalf("retranché : DEF %d", r.Defense)
	}
	_ = st.Appliquer(Action{Type: "suivant"}) // retour de Krush : le bonus prend fin
	if _, ok := pj.Etats["Retranché"]; ok {
		t.Fatal("la posture dure jusqu'au prochain tour du combattant")
	}
}

func TestPointDeChanceApresLeJet(t *testing.T) {
	st, pj, orc := duel(t)
	r := attaque(t, st, pj, orc, 0, 5) // 5 + 4 = 9 < 14
	if r.Verdict != "miss" {
		t.Fatal("devrait rater")
	}
	if err := st.Appliquer(Action{Type: "chance"}); err != nil {
		t.Fatal(err)
	}
	if st.Resultat.Verdict != "hit" || st.Resultat.Total != 9+7 || pj.Pc != 1 {
		t.Fatalf("point de chance : %s, PC %d", st.Resultat.CalcAffiche, pj.Pc)
	}
	if err := st.Appliquer(Action{Type: "chance"}); err == nil {
		t.Fatal("un seul point de chance par jet")
	}
	attaque(t, st, pj, orc, 0, 1)
	_ = st.Appliquer(Action{Type: "chance"})
	if st.Resultat.Verdict != "miss" {
		t.Fatal("un point de chance ne rattrape pas un échec critique")
	}
	attaque(t, st, orc, pj, 0, 2)
	if err := st.Appliquer(Action{Type: "chance"}); err == nil {
		t.Fatal("un PNJ n'a pas de points de chance")
	}
}

func TestInconscientSansAction(t *testing.T) {
	st, pj, _ := duel(t)
	_ = st.Appliquer(Action{Type: "suivant"})
	_ = st.Appliquer(Action{Type: "pv", ID: pj.ID, Sens: "degats", Valeur: []byte("99")})
	_ = st.Appliquer(Action{Type: "suivant"})
	if st.Courant().ID == pj.ID {
		t.Fatal("un PJ à 0 PV est inconscient et ne joue pas")
	}
	_ = st.Appliquer(Action{Type: "pv", ID: pj.ID, Sens: "soins", Valeur: []byte("3")})
	_ = st.Appliquer(Action{Type: "suivant"})
	if st.Courant().ID != pj.ID {
		t.Fatal("soigné, il reprend conscience et agit")
	}
}

// Dés réels : le d20 et les dégâts sont lancés à la table. L'application ne tire rien ; elle double
// le total sur un critique et retranche la RD.
func TestDesReels(t *testing.T) {
	st, pj, orc := duel(t) // Orc : DEF 14, RD 2, 12 PV
	ancien := De
	De = func(int) int { t.Fatal("un dé virtuel a été lancé en mode dés réels"); return 1 }
	defer func() { De = ancien }()
	reel := func(nat int) *Resultat {
		t.Helper()
		if err := st.Appliquer(Action{Type: "attaque", Att: pj.ID, Cible: orc.ID, Attaque: 0, Mode: "reel", Nat: ptr(nat), Act: "G"}); err != nil {
			t.Fatal(err)
		}
		return st.Resultat
	}
	des := func(v int) error {
		return st.Appliquer(Action{Type: "appliquer_dm", Cle: "des", Valeur: []byte(strconv.Itoa(v))})
	}
	if err := st.Appliquer(Action{Type: "attaque", Att: pj.ID, Cible: orc.ID, Mode: "reel", Act: "G"}); err == nil {
		t.Fatal("attaque acceptée sans le d20 de la table")
	}
	// touché : dégâts en attente, rien de tiré
	r := reel(12) // 12 + 4 = 16 ≥ 14
	if r.Verdict != "hit" || !r.DmReel || r.Brut != nil || r.SansDm || r.Applique != nil || r.DmExpr != "1d8+3" {
		t.Fatalf("touché en dés réels : %+v", r)
	}
	if err := des(7); err != nil {
		t.Fatal(err)
	}
	if orc.Pv != 12-5 || *st.Resultat.Brut != 7 || *st.Resultat.Final != 5 || *st.Resultat.Applique != 5 {
		t.Fatalf("7 − RD 2 = 5 attendu : PV %d, %+v", orc.Pv, st.Resultat)
	}
	if err := des(3); err == nil {
		t.Fatal("dégâts appliqués deux fois")
	}
	// critique : le total annoncé est doublé, puis la RD
	r = reel(20)
	if r.Verdict != "crit" || r.Brut != nil {
		t.Fatalf("critique en dés réels : %+v", r)
	}
	if err := des(4); err != nil {
		t.Fatal(err)
	}
	if orc.Pv != 7-6 || *st.Resultat.Brut != 8 || *st.Resultat.DmDetail != "(4) × 2" {
		t.Fatalf("(4) × 2 − RD 2 = 6 attendu : PV %d, %+v", orc.Pv, st.Resultat)
	}
	// raté, puis point de chance : toujours rien de tiré, les dégâts restent à saisir
	r = reel(5) // 5 + 4 = 9 < 14
	if r.Verdict != "miss" {
		t.Fatalf("raté attendu : %+v", r)
	}
	if err := des(3); err == nil {
		t.Fatal("dégâts saisis sur une attaque ratée")
	}
	if err := st.Appliquer(Action{Type: "chance"}); err != nil {
		t.Fatal(err)
	}
	if r = st.Resultat; r.Verdict != "hit" || r.Brut != nil || !r.DmReel {
		t.Fatalf("après le point de chance : %+v", r)
	}
	if err := des(-1); err == nil {
		t.Fatal("total négatif accepté")
	}
	if err := des(1); err != nil || orc.Pv != 1 { // 1 − RD 2 : aucun dégât
		t.Fatalf("la RD absorbe tout : PV %d, %v", orc.Pv, err)
	}
	// le mode « manuel » ne change pas : d20 saisi, dégâts tirés par l'application
	De = func(int) int { return 4 }
	if err := st.Appliquer(Action{Type: "attaque", Att: pj.ID, Cible: orc.ID, Mode: "manuel", Nat: ptr(12), Act: "G"}); err != nil {
		t.Fatal(err)
	}
	if r = st.Resultat; r.DmReel || r.Brut == nil || *r.Brut != 7 {
		t.Fatalf("mode manuel : %+v", r)
	}
	if err := des(3); err == nil {
		t.Fatal("total de dés accepté hors du mode dés réels")
	}
}
