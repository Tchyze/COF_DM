package combat

import (
	"testing"

	"github.com/Tchyze/cof_dm/internal/regles"
)

// Règles d'essai, inventées pour les tests : aucun état, aucune valeur ne vient d'un livre.
const reglesEssai = `{"format":"cof-dm-regles","combat":{
 "de":20,"echec_naturel":1,"critique_dm":2,"chance":7,"budget_actions":true,
 "etats":[
  {"nom":"Fourbu","description":"Lance un dé plus petit.","de":10},
  {"nom":"Ébloui","description":"Vise mal et se défend mal.","att":-4,"att_distance":-9,"def":-3,"init":-6},
  {"nom":"Sonné","description":"Ne fait rien.","def":-3,"bloque":true},
  {"nom":"Englué","description":"Lance un dé plus petit.","de":10},
  {"nom":"Pétrifié","description":"Ne fait rien, offert aux coups.","bloque":true,"critique_automatique":true},
  {"nom":"Pataud","description":"Une seule action.","une_action":true},
  {"nom":"Bousculé","description":"Gêné.","att":-4,"att_distance":-4,"def":-3},
  {"nom":"Ahuri","description":"Pris de court au premier round.","def":-3,"bloque":true,"fin":"round"},
  {"nom":"En garde","description":"Posture prudente.","def":1,"fin":"tour","action":"A"},
  {"nom":"Retranché","description":"Posture fermée.","def":3,"fin":"tour","action":"L"}]}}`

var essai = func() *regles.Combat {
	r, err := regles.Lire([]byte(reglesEssai))
	if err != nil {
		panic(err)
	}
	return r.Combat
}()

// Sans règles, le combat reste jouable : initiative, PV, attaque contre la DEF, dégâts, états
// libres sans effet. Ni critique, ni échec automatique, ni point de chance, ni budget d'actions.
func TestCombatMinimal(t *testing.T) {
	pj := DepuisFiche(FicheCombat{Nom: "Krush", Init: 11, Defense: 16, Pv: 16, Pc: 2}, "pj", "", "")
	orc := DepuisFiche(FicheCombat{Nom: "Orc", Init: 10, Defense: 14, Pv: 12, Rd: 2}, "ennemi", "", "")
	st := Nouveau("Minimal", []*Combattant{pj, orc}, nil)
	ancien := De
	De = func(int) int { return 4 }
	defer func() { De = ancien }()
	att := func(nat int, dm string) *Resultat {
		t.Helper()
		if err := st.Appliquer(Action{Type: "attaque", Att: pj.ID, Cible: orc.ID, Attaque: -1, Mode: "manuel", Nat: ptr(nat), Divers: 4, Dm: &dm}); err != nil {
			t.Fatal(err)
		}
		return st.Resultat
	}
	// plusieurs attaques dans le même tour : pas de budget d'actions
	if r := att(20, "1d8+3"); r.Verdict != "hit" || *r.Brut != 7 || *r.Final != 5 {
		t.Fatalf("le maximum du dé n'est qu'une réussite, sans dégâts multipliés : %+v", r)
	}
	if r := att(1, "1d8"); r.Verdict != "miss" { // 1 + 4 < 14 : raté par le total, pas par le dé
		t.Fatalf("1 naturel : %+v", r)
	}
	orc.Defense = 5
	if r := att(1, "1d8"); r.Verdict != "hit" {
		t.Fatalf("sans règles, un 1 naturel qui atteint la DEF touche : %+v", r)
	}
	if err := st.Appliquer(Action{Type: "chance"}); err == nil {
		t.Fatal("point de chance accepté sans règles")
	}
	// un état libre se pose et se retire, sans effet
	if err := st.Appliquer(Action{Type: "etat", ID: orc.ID, Etat: "  En   feu ", Duree: 2}); err != nil {
		t.Fatal(err)
	}
	if orc.Etats["En feu"] != 2 || st.InitEffective(orc) != 10 {
		t.Fatalf("état libre : %v", orc.Etats)
	}
	if err := st.Appliquer(Action{Type: "etat", ID: orc.ID, Etat: ""}); err == nil {
		t.Fatal("état sans nom accepté")
	}
	if err := st.Appliquer(Action{Type: "defense", Etat: "En garde"}); err == nil {
		t.Fatal("posture défensive acceptée sans règles")
	}
	if err := st.Appliquer(Action{Type: "retirer_etat", ID: orc.ID, Etat: "En feu"}); err != nil || len(orc.Etats) != 0 {
		t.Fatalf("retrait : %v %v", err, orc.Etats)
	}
	// un test de caractéristique : ni réussite ni échec critique
	if err := st.Appliquer(Action{Type: "test", ID: pj.ID, Carac: "FOR", Difficulte: 15, Nat: ptr(20)}); err != nil || !st.Test.Reussi || st.Test.Jet.Critique != "" {
		t.Fatalf("test : %v %+v", err, st.Test)
	}
}
