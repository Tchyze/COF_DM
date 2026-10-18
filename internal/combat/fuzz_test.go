package combat

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"runtime/debug"
	"testing"

	"github.com/Tchyze/cof_dm/internal/fiches"
)

// TestActionsAleatoires enchaîne des milliers d'actions comme pendant une vraie partie :
// aucune ne doit faire planter le moteur, et l'état doit rester cohérent et sérialisable.
func TestActionsAleatoires(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	monstre := FicheCombat{Nom: "Brigand", Init: 12, Defense: 13, Pv: 9, Attaques: []fiches.Attaque{{Nom: "Gourdin", Type: "C", Bonus: 2, Dm: "1d6+1"}}}
	pj := FicheCombat{Nom: "Sana", Init: 17, Defense: 15, Pv: 10, Pc: 3, Crit: 19, Pm: 4, PmMax: 6, Attaques: []fiches.Attaque{{Nom: "Dague", Type: "C", Bonus: 1, Dm: "1d4+1"}}}
	types := []string{"suivant", "precedent", "trier", "deplacer", "ajouter", "retirer", "modifier", "pv", "attaque",
		"appliquer_dm", "ignorer_dm", "etat", "retirer_etat", "action_utilisee", "terminer", "rouvrir", "vider_journal", "pm", "usage"}
	etats := []string{"Inconnu", ""}
	for _, e := range essai.Etats {
		etats = append(etats, e.Nom)
	}
	for partie := range 300 {
		st := Nouveau("Fuzz", append(InstancierGroupe(monstre, 3, "ennemi", "monstre", nil), DepuisFiche(pj, "pj", "", "pj")), essai)
		for n := range 200 {
			ids := append([]string{"", "zzz"}, st.Ordre...)
			id := func() string { return ids[r.IntN(len(ids))] }
			nat := r.IntN(23) - 1
			dm := []string{"", "1d6", "abc", "2d8+3"}[r.IntN(4)]
			val, _ := json.Marshal([]any{r.IntN(15) - 2, true, false}[r.IntN(3)])
			a := Action{Type: types[r.IntN(len(types))], ID: id(), Att: id(), Cible: id(), Sens: []string{"haut", "bas", "degats", "soins"}[r.IntN(4)],
				Attaque: r.IntN(4) - 1, Mode: []string{"auto", "manuel"}[r.IntN(2)], Divers: r.IntN(5) - 2, Pc: r.IntN(2) == 0,
				Valeur: val, Etat: etats[r.IntN(len(etats))], Duree: r.IntN(4), Cle: []string{"A", "M", "L", "X"}[r.IntN(4)],
				Qte: r.IntN(4), Kind: []string{"monstre", "pj", ""}[r.IntN(3)], Camp: []string{"", "pj", "allie", "ennemi"}[r.IntN(4)],
				CoutPm: r.IntN(5) - 1}
			if a.Type == "pm" {
				a.Sens = []string{"depense", "gain"}[r.IntN(2)]
			}
			if r.IntN(2) == 0 {
				a.Nat = &nat
			}
			if r.IntN(2) == 0 {
				a.Dm = &dm
			}
			if a.Type == "ajouter" && r.IntN(4) > 0 {
				a.Fiche = &monstre
			}
			if a.Type == "modifier" {
				a.Champs = map[string]json.RawMessage{"pv": json.RawMessage(fmt.Sprint(r.IntN(20) - 5)), "init": json.RawMessage("15"), "nom": json.RawMessage(`"X"`)}
			}
			func() {
				defer func() {
					if e := recover(); e != nil {
						b, _ := json.Marshal(a)
						t.Fatalf("partie %d, action %d %s : panique %v\n%s\n%s", partie, n, b, e, debug.Stack(), mustJSON(st))
					}
				}()
				_ = st.Appliquer(a)
			}()
			if st.Tour < 0 || (len(st.Ordre) > 0 && st.Tour >= len(st.Ordre)) {
				t.Fatalf("tour hors limites : %d / %d après %s", st.Tour, len(st.Ordre), a.Type)
			}
			for _, c := range st.Cbts {
				if c.Pm < 0 || c.Pm > c.PmMax {
					t.Fatalf("PM hors bornes après %s : %d / %d", a.Type, c.Pm, c.PmMax)
				}
			}
			if len(st.Ordre) != len(st.Cbts) {
				t.Fatalf("ordre et combattants désynchronisés après %s", a.Type)
			}
			var copie Etat
			if err := json.Unmarshal([]byte(mustJSON(st)), &copie); err != nil {
				t.Fatalf("état non relisible après %s : %v", a.Type, err)
			}
		}
	}
}

func mustJSON(v any) string { b, _ := json.Marshal(v); return string(b) }
