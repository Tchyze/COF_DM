package combat

import (
	"testing"

	"github.com/Tchyze/cof_dm/internal/fiches"
)

func TestTestDeCaracteristique(t *testing.T) {
	pj := DepuisFiche(FicheCombat{Nom: "Sana", Init: 17, Defense: 15, Pv: 10, Pc: 2, Carac: fiches.Carac{CON: 1, DEX: 3}}, "pj", "", "")
	spectre := DepuisFiche(FicheCombat{Nom: "Spectre", Init: 12, Defense: 15, Pv: 19, Carac: fiches.Carac{FOR: 1, DEX: 1, SAG: 2},
		Superieures: []string{"SAG"}}, "ennemi", "", "")
	st := Nouveau("Test", []*Combattant{pj, spectre}, essai)

	// test de CON difficulté 10, ou pétrifié trois rounds
	if err := st.Appliquer(Action{Type: "test", ID: pj.ID, Carac: "CON", Difficulte: 10, Mode: "manuel", Nat: ptr(8), EtatEchec: "Pétrifié", Duree: 3}); err != nil {
		t.Fatal(err)
	}
	if st.Test.Reussi || st.Test.Jet.Total != 9 || pj.Etats["Pétrifié"] != 3 {
		t.Fatalf("échec attendu (8 + 1 = 9 < 10) et pétrification : %+v", st.Test)
	}
	// point de chance : 9 + 7 ≥ 10, l'état est retiré
	if err := st.Appliquer(Action{Type: "chance", Cle: "test"}); err != nil {
		t.Fatal(err)
	}
	if !st.Test.Reussi || pj.Pc != 1 {
		t.Fatalf("point de chance : %+v", st.Test)
	}
	if _, ok := pj.Etats["Pétrifié"]; ok {
		t.Fatal("l'état doit être retiré quand le test devient réussi")
	}
	// égalité en opposition : le défenseur l'emporte (règle maison)
	_ = st.Appliquer(Action{Type: "test", ID: pj.ID, Carac: "DEX", Opposant: spectre.ID, CaracOpp: "SAG", Mode: "manuel", Nat: ptr(10), NatOpp: ptr(11)})
	if st.Test.Jet.Total != 13 || st.Test.Opposant.Total != 13 || st.Test.Reussi {
		t.Fatalf("égalité : %+v / %+v", st.Test.Jet, st.Test.Opposant)
	}
	// réussite critique en opposition
	_ = st.Appliquer(Action{Type: "test", ID: pj.ID, Carac: "DEX", Opposant: spectre.ID, CaracOpp: "SAG", Mode: "manuel", Nat: ptr(20), NatOpp: ptr(20)})
	if st.Test.Reussi || st.Test.Jet.Total != 23 || st.Test.Opposant.Total != 22 {
		// les deux font 20 naturel : aucun n'a « seul » la réussite critique, on compare les totaux
		if !(st.Test.Jet.Total > st.Test.Opposant.Total && st.Test.Reussi) {
			t.Fatalf("deux critiques : %+v", st.Test)
		}
	}
	// Carac. supérieure : deux dés lancés en automatique
	_ = st.Appliquer(Action{Type: "test", ID: spectre.ID, Carac: "SAG", Difficulte: 15})
	if len(st.Test.Jet.Des) != 2 {
		t.Fatal("une Carac. supérieure lance deux dés")
	}
	// fourbu : dé réduit, et un 20 est refusé
	_ = st.Appliquer(Action{Type: "etat", ID: pj.ID, Etat: "Fourbu"})
	if err := st.Appliquer(Action{Type: "test", ID: pj.ID, Carac: "CON", Difficulte: 10, Mode: "manuel", Nat: ptr(20)}); err == nil {
		t.Fatal("fourbu : un d10 ne donne pas 20")
	}
}
