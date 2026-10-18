package combat

import (
	"testing"

	"github.com/Tchyze/cof_dm/internal/fiches"
)

func TestLancer(t *testing.T) {
	for range 200 {
		j, ok := Lancer("2d6+3", 1)
		if !ok || j.Total < 5 || j.Total > 15 {
			t.Fatalf("2d6+3 = %v", j)
		}
	}
	if j, _ := Lancer("1d8+2", 2); j.Total%2 != 0 {
		t.Fatal("critique non doublé")
	}
	if _, ok := Lancer("abc", 1); ok {
		t.Fatal("formule illisible acceptée")
	}
	if j, _ := Lancer("1d4-10", 1); j.Total != 0 {
		t.Fatal("les dégâts ne peuvent pas être négatifs")
	}
	if _, ok := Lancer("", 1); ok {
		t.Fatal("formule vide acceptée")
	}
}

func TestEtatsEtTours(t *testing.T) {
	f := FicheCombat{Nom: "Orc", Init: 10, Defense: 12, Pv: 8, Attaques: []fiches.Attaque{{Nom: "Hache", Type: "C", Bonus: 3, Dm: "1d8"}}}
	p := FicheCombat{Nom: "Héros", Init: 15, Defense: 15, Pv: 12}
	cbts := append(InstancierGroupe(f, 2, "ennemi", "", nil), DepuisFiche(p, "pj", "", ""))
	st := Nouveau("Test", cbts, essai)
	if st.Courant().Nom != "Héros" {
		t.Fatalf("le héros (init 15) doit commencer, pas %s", st.Courant().Nom)
	}
	orc1 := cbts[0].ID
	// Pétrifié sur la cible : touche automatiquement
	_ = st.Appliquer(Action{Type: "etat", ID: orc1, Etat: "Pétrifié", Duree: 2})
	nat := 3
	if err := st.Appliquer(Action{Type: "attaque", Att: cbts[2].ID, Cible: orc1, Mode: "manuel", Nat: &nat}); err != nil {
		t.Fatal(err)
	}
	if st.Resultat.Verdict != "crit" {
		t.Fatalf("pétrifié : %s", st.Resultat.CalcAffiche)
	}
	// les durées baissent à la fin du tour du combattant concerné
	_ = st.Appliquer(Action{Type: "suivant"}) // fin du héros → orc 1
	_ = st.Appliquer(Action{Type: "suivant"}) // fin de l'orc 1 : Pétrifié 2 → 1
	if st.Cbts[orc1].Etats["Pétrifié"] != 1 {
		t.Fatalf("durée : %d", st.Cbts[orc1].Etats["Pétrifié"])
	}
	if err := st.Appliquer(Action{Type: "retirer", ID: orc1}); err != nil {
		t.Fatal(err)
	}
	if len(st.Ordre) != 2 || st.Cbts[orc1] != nil {
		t.Fatal("retrait incorrect")
	}
}
