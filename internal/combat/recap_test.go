package combat

import (
	"strconv"
	"strings"
	"testing"
)

func appliquerDm(t *testing.T, st *Etat, v int) {
	t.Helper()
	if err := st.Appliquer(Action{Type: "appliquer_dm", Valeur: []byte(itoa(v))}); err != nil {
		t.Fatal(err)
	}
}

func itoa(n int) string { return strconv.Itoa(n) }

func TestFinEtRecap(t *testing.T) {
	st, pj, orc := duel(t)
	attaque(t, st, pj, orc, 0, 1) // échec critique
	attaque(t, st, pj, orc, 0, 3) // raté (3 + 4 < 14)
	attaque(t, st, pj, orc, 0, 20)
	appliquerDm(t, st, 7)
	if st.Fin != "" {
		t.Fatal("fin proposée trop tôt")
	}
	attaque(t, st, orc, pj, 0, 15)
	appliquerDm(t, st, 4)
	attaque(t, st, pj, orc, 0, 12)
	appliquerDm(t, st, 9) // l'orc tombe
	if st.Fin != "victoire" || st.Recap == nil {
		t.Fatalf("victoire non détectée : fin=%q", st.Fin)
	}
	// le MJ continue : la proposition disparaît et ne revient pas tant que rien ne change
	if err := st.Appliquer(Action{Type: "continuer"}); err != nil {
		t.Fatal(err)
	}
	if st.Fin != "" {
		t.Fatal("la proposition devait disparaître")
	}
	_ = st.Appliquer(Action{Type: "suivant"})
	if st.Fin != "" {
		t.Fatal("proposition revenue alors que le MJ a choisi de continuer")
	}
	// l'orc se relève (soins), puis retombe : la fin est de nouveau proposée
	_ = st.Appliquer(Action{Type: "pv", ID: orc.ID, Valeur: []byte("3")})
	_ = st.Appliquer(Action{Type: "pv", ID: orc.ID, Valeur: []byte("3"), Sens: "degats"})
	if st.Fin != "victoire" {
		t.Fatal("fin non reproposée après un nouveau KO")
	}
	if err := st.Appliquer(Action{Type: "terminer"}); err != nil {
		t.Fatal(err)
	}
	r := st.Recap
	if !st.Termine || r == nil || r.Issue != "victoire" || r.Vaincus != 1 || r.Ennemis != 1 {
		t.Fatalf("récap : %+v", r)
	}
	b := r.Pjs[0]
	if b.Nom != "Krush" || b.Tentatives != 4 || b.Touches != 2 || b.Critiques != 1 || b.Rates != 2 || b.Echecs != 1 ||
		b.Infliges != 12 || b.Subis != 4 || b.MeilleurCoup != 9 { // 7 + 9 DM sur un orc à 12 PV : 12 PV perdus
		t.Fatalf("bilan : %+v", b)
	}
	if b.Abattus != 1 || b.Titre != "Le Boucher" {
		t.Fatalf("abattus / titre : %+v", b)
	}
	moments := map[string]Moment{}
	for _, m := range r.Moments {
		moments[m.Cle] = m
	}
	if m := moments["gros"]; m.Qui != "Krush" || !strings.Contains(m.Texte, "9 DM à Orc") {
		t.Fatalf("plus gros coup : %+v", m)
	}
	if moments["grace"].Qui != "Krush" || moments["crit"].Qui != "Krush" || moments["maudit"].Qui != "Krush" {
		t.Fatalf("moments : %+v", r.Moments)
	}
	if len(r.Moments) > 6 {
		t.Fatal("trop de moments")
	}
	// rouvrir : plus de récap, pas de proposition immédiate
	_ = st.Appliquer(Action{Type: "rouvrir"})
	if st.Termine || st.Recap != nil || st.Fin != "" {
		t.Fatal("réouverture incorrecte")
	}
}

func TestDefaiteEtChance(t *testing.T) {
	st, pj, orc := duel(t)
	attaque(t, st, pj, orc, 0, 5) // 9 < 14
	if err := st.Appliquer(Action{Type: "chance"}); err != nil {
		t.Fatal(err)
	}
	appliquerDm(t, st, 2)
	attaque(t, st, orc, pj, 0, 18)
	appliquerDm(t, st, 9)
	attaque(t, st, orc, pj, 0, 18)
	appliquerDm(t, st, 9)
	if st.Fin != "defaite" {
		t.Fatalf("défaite non détectée : %q", st.Fin)
	}
	r := st.Recap
	got := map[string]Moment{}
	for _, m := range r.Moments {
		got[m.Cle] = m
	}
	if got["chance"].Qui != "Krush" || got["grace"].Titre != "Coup fatal" || got["grace"].Qui != "Orc" {
		t.Fatalf("moments : %+v", r.Moments)
	}
	if len(r.Tombes) != 1 || !r.Pjs[0].Tombe {
		t.Fatalf("PJ tombés : %+v", r)
	}
}

func TestFroleNemesisHerosCourbe(t *testing.T) {
	st, pj, orc := duel(t) // Krush 16 PV, orc 12 PV
	attaque(t, st, orc, pj, 0, 18)
	appliquerDm(t, st, 15) // Krush à 1 PV
	_ = st.Appliquer(Action{Type: "suivant"})
	_ = st.Appliquer(Action{Type: "suivant"}) // round 2
	attaque(t, st, pj, orc, 0, 20)
	appliquerDm(t, st, 12)
	if err := st.Appliquer(Action{Type: "terminer"}); err != nil {
		t.Fatal(err)
	}
	r := st.Recap
	got := map[string]Moment{}
	for _, m := range r.Moments {
		got[m.Cle] = m
	}
	if m := got["frole"]; m.Qui != "Krush" || !strings.Contains(m.Texte, "1 PV") {
		t.Fatalf("frôlé la mort : %+v", r.Moments)
	}
	if m := got["nemesis"]; m.Qui != "Orc" || !strings.Contains(m.Texte, "15 PV") {
		t.Fatalf("némésis : %+v", m)
	}
	if r.Heros == nil || r.Heros.Nom != "Krush" || !strings.Contains(r.Heros.Texte, "1 adversaire abattu") {
		t.Fatalf("héros : %+v", r.Heros)
	}
	// courbe : 16/16 au round 1, 1/16 au round 2, 1/16 à la fin
	if len(r.Courbe) != 3 || r.Courbe[0].Pv != 16 || r.Courbe[1].Pv != 1 || r.Courbe[1].R != 2 || r.Courbe[2].Max != 16 {
		t.Fatalf("courbe : %+v", r.Courbe)
	}
	if len(r.Moments) > 6 {
		t.Fatal("trop de moments")
	}
}
