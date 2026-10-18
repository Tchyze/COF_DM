package fiches

import (
	"errors"
	"github.com/Tchyze/cof_dm/internal/regles"
	"testing"
)

func TestModificateur(t *testing.T) {
	cas := map[int]int{3: -4, 7: -2, 8: -1, 9: -1, 10: 0, 11: 0, 12: 1, 13: 1, 17: 3, 18: 4, 20: 5}
	for v, m := range cas {
		if got := Modificateur(v); got != m {
			t.Errorf("Modificateur(%d) = %d, attendu %d", v, got, m)
		}
	}
}

func TestNormaliserMonstre(t *testing.T) {
	m, err := NormaliserMonstre([]byte(`{"nom":"  Bête d'essai ","pv":4,"superieures":["DEX","XYZ"],"attaques":[{"nom":"","bonus":2}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if m.Nom != "Bête d'essai" || m.Source != "maison" || m.Crit != 20 || m.Defense != 10 || len(m.Superieures) != 1 || m.Attaques[0].Type != "C" || m.Attaques[0].Nom != "Attaque" || m.Verrouillee() {
		t.Fatalf("valeurs par défaut : %+v", m)
	}
	for _, mauvais := range []string{`{`, `{"nom":""}`, `{"nom":"x","pv":0}`, `{"nom":"x","rd":-1}`, `{"nom":"x","crit":1}`, `{"nom":"x","nc":-1}`,
		`{"nom":"x","source":"autre"}`, `{"nom":"x","attaques":[{"type":"Z"}]}`, `{"nom":"x","pv":"beaucoup"}`} {
		var ev ErrValidation
		if _, err := NormaliserMonstre([]byte(mauvais)); !errors.As(err, &ev) {
			t.Fatalf("monstre accepté : %s (%v)", mauvais, err)
		}
	}
	if m, _ := NormaliserMonstre([]byte(`{"nom":"x","source":"officiel","ref":" r1 "}`)); !m.Verrouillee() || m.Ref != "r1" {
		t.Fatalf("fiche officielle : %+v", m)
	}
}

// Une feuille vierge part de caractéristiques neutres (modificateur nul), ramenées dans les bornes.
func TestFeuilleNeuveNeutre(t *testing.T) {
	for _, c := range []struct {
		rg      *regles.Feuille
		attendu int
	}{
		{nil, 10},
		{&regles.Feuille{ValeurMin: 1, ValeurMax: 30}, 10},
		{&regles.Feuille{ValeurMin: 12}, 12},
		{&regles.Feuille{ValeurMin: 1, ValeurMax: 8}, 8},
	} {
		f := FeuilleNeuve(c.rg)
		if f.Valeurs.Get("FOR") != c.attendu || f.Valeurs.Get("CHA") != c.attendu {
			t.Fatalf("règles %+v : valeurs %+v, attendu %d", c.rg, f.Valeurs, c.attendu)
		}
	}
	if Modificateur(valeurNeutre) != 0 {
		t.Fatal("la valeur neutre doit avoir un modificateur nul")
	}
}
