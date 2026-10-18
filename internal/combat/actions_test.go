package combat

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Tchyze/cof_dm/internal/fiches"
)

func att(st *Etat, a, c *Combattant, act string) error {
	return st.Appliquer(Action{Type: "attaque", Att: a.ID, Cible: c.ID, Mode: "manuel", Nat: ptr(10), Act: act})
}

func marquer(st *Etat, k string) error {
	return st.Appliquer(Action{Type: "action_utilisee", Cle: k, Valeur: []byte("true")})
}

// Budget d'un tour : 1 action limitée, OU 1 mouvement + 1 attaque, OU 2 mouvements.
func TestBudgetActions(t *testing.T) {
	st, pj, orc := duel(t) // Krush (pj) a la main
	if err := att(st, pj, orc, "A"); err != nil {
		t.Fatal(err)
	}
	if err := att(st, pj, orc, "A"); err == nil {
		t.Fatal("deuxième attaque acceptée dans le même tour")
	}
	if err := att(st, pj, orc, "L"); err == nil {
		t.Fatal("action limitée acceptée après une attaque")
	}
	if err := marquer(st, "M"); err != nil {
		t.Fatalf("attaque + mouvement refusé : %v", err)
	}
	if err := marquer(st, "M"); err == nil {
		t.Fatal("second mouvement accepté après attaque + mouvement")
	}
	if err := st.Appliquer(Action{Type: "action_utilisee", Cle: "A", Valeur: []byte("false")}); err == nil {
		t.Fatal("une action utilisée a pu être rendue")
	}
	// une attaque gratuite (riposte, capacité) reste possible
	if err := att(st, pj, orc, "G"); err != nil {
		t.Fatal(err)
	}

	// tour suivant de Krush : deux mouvements, puis plus d'attaque
	_ = st.Appliquer(Action{Type: "suivant"})
	if err := att(st, pj, orc, "A"); err == nil || !strings.Contains(err.Error(), "pas le tour") {
		t.Fatalf("attaque hors de son tour acceptée : %v", err)
	}
	_ = st.Appliquer(Action{Type: "suivant"})
	if err := marquer(st, "M"); err != nil {
		t.Fatal(err)
	}
	if err := marquer(st, "M"); err != nil {
		t.Fatalf("deux mouvements refusés : %v", err)
	}
	if err := att(st, pj, orc, "A"); err == nil {
		t.Fatal("attaque acceptée après deux mouvements")
	}

	// action limitée : occupe tout le tour, mais une capacité à plusieurs attaques peut continuer
	_ = st.Appliquer(Action{Type: "suivant"})
	_ = st.Appliquer(Action{Type: "suivant"})
	if err := att(st, pj, orc, "LS"); err == nil {
		t.Fatal("suite d'action limitée acceptée sans action limitée")
	}
	if err := att(st, pj, orc, "L"); err != nil {
		t.Fatal(err)
	}
	if err := att(st, pj, orc, "LS"); err != nil {
		t.Fatalf("seconde attaque de la capacité limitée refusée : %v", err)
	}
	for _, k := range []string{"A", "M", "L"} {
		if err := marquer(st, k); err == nil {
			t.Fatalf("action %s acceptée après une action limitée", k)
		}
	}
}

func TestBudgetEtatsEtDefense(t *testing.T) {
	st, pj, orc := duel(t)
	_ = st.Appliquer(Action{Type: "etat", ID: pj.ID, Etat: "Pataud"})
	if err := att(st, pj, orc, "L"); err == nil {
		t.Fatal("pataud : action limitée acceptée")
	}
	if err := marquer(st, "M"); err != nil {
		t.Fatal(err)
	}
	if err := att(st, pj, orc, "A"); err == nil {
		t.Fatal("pataud : attaque acceptée après un mouvement")
	}

	_ = st.Appliquer(Action{Type: "suivant"}) // orc
	_ = st.Appliquer(Action{Type: "etat", ID: orc.ID, Etat: "Sonné", Duree: 1})
	if err := att(st, orc, pj, "A"); err == nil {
		t.Fatal("sonné : attaque acceptée")
	}
	if err := att(st, orc, pj, "G"); err == nil {
		t.Fatal("sonné : attaque gratuite acceptée")
	}

	_ = st.Appliquer(Action{Type: "suivant"}) // Krush (l'état dure toujours)
	delete(pj.Etats, "Pataud")
	if err := st.Appliquer(Action{Type: "defense", Etat: "En garde"}); err != nil {
		t.Fatal(err)
	}
	if err := att(st, pj, orc, "A"); err == nil {
		t.Fatal("en garde : l'action d'attaque n'a pas été consommée")
	}
	if err := st.Appliquer(Action{Type: "defense", Etat: "Retranché"}); err == nil {
		t.Fatal("retranché acceptée après une en garde")
	}
	if err := marquer(st, "M"); err != nil {
		t.Fatalf("en garde + mouvement refusé : %v", err)
	}
	// un PJ à 0 PV ne peut rien faire
	pj.Pv = 0
	if err := att(st, pj, orc, "G"); err == nil {
		t.Fatal("PJ inconscient : attaque acceptée")
	}
}

// Points de mana : un sort les dépense en attaquant (pas plus qu'il n'en reste), une dépense ou une
// récupération reste dans les bornes, et « Annuler » rend tout.
func TestManaEtSorts(t *testing.T) {
	mage := DepuisFiche(FicheCombat{Nom: "Ilse", Init: 15, Defense: 12, Pv: 10, Pm: 5, PmMax: 8,
		Attaques: []fiches.Attaque{{Nom: "Trait de feu", Type: "M", Bonus: 3, Dm: "2d6"}}, Voies: []string{"Trait de feu", "Bouclier"}}, "pj", "", "pj")
	orc := DepuisFiche(FicheCombat{Nom: "Orc", Init: 10, Defense: 14, Pv: 12}, "ennemi", "", "")
	brute := DepuisFiche(FicheCombat{Nom: "Brute", Init: 1, Defense: 10, Pv: 5}, "ennemi", "", "")
	st := Nouveau("Sorts", []*Combattant{mage, orc, brute}, essai)
	if mage.Pm != 5 || mage.PmMax != 8 || len(mage.Voies) != 2 || brute.PmMax != 0 {
		t.Fatalf("PM à l'entrée : %+v", mage)
	}
	sort := func(cout int) error {
		return st.Appliquer(Action{Type: "attaque", Att: mage.ID, Cible: orc.ID, Mode: "manuel", Nat: ptr(10), Act: "G", CoutPm: cout})
	}
	if err := sort(6); err == nil || !strings.Contains(err.Error(), "n'a que 5 PM") {
		t.Fatalf("sort trop cher : %v", err)
	}
	if mage.Pm != 5 || st.Resultat != nil {
		t.Fatalf("un sort refusé ne coûte rien : %d", mage.Pm)
	}
	if err := sort(2); err != nil || mage.Pm != 3 {
		t.Fatalf("sort à 2 PM : %v, reste %d", err, mage.Pm)
	}
	if err := sort(-1); err == nil {
		t.Fatal("coût négatif accepté")
	}
	pm := func(sens string, v int) error {
		return st.Appliquer(Action{Type: "pm", ID: mage.ID, Sens: sens, Valeur: []byte(itoa(v))})
	}
	if err := pm("depense", 4); err == nil {
		t.Fatal("dépense de plus de PM qu'il n'en reste")
	}
	if err := pm("gain", 20); err != nil || mage.Pm != 8 {
		t.Fatalf("récupération bornée au maximum : %v, %d", err, mage.Pm)
	}
	if err := pm("depense", 0); err == nil {
		t.Fatal("dépense nulle acceptée")
	}
	if err := st.Appliquer(Action{Type: "pm", ID: brute.ID, Sens: "gain", Valeur: []byte("1")}); err == nil {
		t.Fatal("PM donnés à un combattant qui n'en a pas")
	}
	if err := st.Appliquer(Action{Type: "modifier", ID: mage.ID, Champs: map[string]json.RawMessage{"pm": json.RawMessage("99"), "pmMax": json.RawMessage("10")}}); err != nil || mage.Pm != 10 || mage.PmMax != 10 {
		t.Fatalf("modification bornée : %v, %d / %d", err, mage.Pm, mage.PmMax)
	}
}

// Capacités à usage limité : une fois marquée, une capacité ne se marque pas deux fois dans le combat.
func TestUsagesLimites(t *testing.T) {
	st, pj, _ := duel(t)
	usage := func(nom string, oui bool) error {
		v := "false"
		if oui {
			v = "true"
		}
		return st.Appliquer(Action{Type: "usage", ID: pj.ID, Cle: nom, Valeur: []byte(v)})
	}
	if err := usage("  Charge   héroïque ", true); err != nil || len(pj.Utilise) != 1 || pj.Utilise[0] != "Charge héroïque" {
		t.Fatalf("usage : %v %v", err, pj.Utilise)
	}
	if err := usage("Charge héroïque", true); err == nil || !strings.Contains(err.Error(), "déjà utilisé") {
		t.Fatalf("double usage : %v", err)
	}
	if err := usage("", true); err == nil {
		t.Fatal("capacité sans nom acceptée")
	}
	if err := usage("Charge héroïque", false); err != nil || len(pj.Utilise) != 0 {
		t.Fatalf("usage rendu : %v %v", err, pj.Utilise)
	}
	if !strings.Contains(mustJSON(st.Log), "peut de nouveau utiliser") {
		t.Fatal("journal sans la capacité rendue")
	}
}
