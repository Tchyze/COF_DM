package combat

import (
	"encoding/json"
	"strings"
	"testing"
)

// Afficheur de table : un combattant ajouté peut porter un nom d'affichage (numéroté dans un groupe)
// et être caché aux joueurs ; le MJ le révèle, le renomme ou le marque sbire en cours de combat, et
// le journal note quand il se révèle.
func TestAliasCacheSbire(t *testing.T) {
	st := Nouveau("Embuscade", []*Combattant{DepuisFiche(FicheCombat{Nom: "Héros", Init: 15, Pv: 12}, "pj", "", "")}, essai)
	f := FicheCombat{Nom: "Ombre", Init: 12, Pv: 6, Sbire: true}
	if err := st.Appliquer(Action{Type: "ajouter", Fiche: &f, Qte: 2, Alias: "Silhouette", Cache: true}); err != nil {
		t.Fatal(err)
	}
	var ombres []*Combattant
	for _, c := range st.Cbts {
		if strings.HasPrefix(c.Nom, "Ombre") {
			ombres = append(ombres, c)
		}
	}
	if len(ombres) != 2 {
		t.Fatalf("groupe : %d", len(ombres))
	}
	alias := map[string]bool{}
	for _, c := range ombres {
		if !c.Cache || !c.Sbire {
			t.Fatalf("caché et sbire attendus : %+v", c)
		}
		alias[c.NomVu()] = true
	}
	if !alias["Silhouette 1"] || !alias["Silhouette 2"] {
		t.Fatalf("noms d'affichage : %v", alias)
	}
	if !strings.Contains(st.Log[len(st.Log)-1].T, "caché aux joueurs") {
		t.Fatalf("journal : %v", st.Log[len(st.Log)-1])
	}

	o := ombres[0]
	champs := func(v map[string]any) map[string]json.RawMessage {
		out := map[string]json.RawMessage{}
		for k, x := range v {
			out[k], _ = json.Marshal(x)
		}
		return out
	}
	if err := st.Appliquer(Action{Type: "modifier", ID: o.ID, Champs: champs(map[string]any{"cache": false, "alias": "  ", "sbire": false})}); err != nil {
		t.Fatal(err)
	}
	if o.Cache || o.Sbire || o.Alias != "" || o.NomVu() != o.Nom {
		t.Fatalf("révélé : %+v", o)
	}
	if !strings.Contains(st.Log[len(st.Log)-1].T, "se révèle") {
		t.Fatalf("journal : %v", st.Log[len(st.Log)-1])
	}
	// un champ absent ne change rien
	if err := st.Appliquer(Action{Type: "modifier", ID: o.ID, Champs: champs(map[string]any{"init": 3})}); err != nil || o.Cache || o.Init != 3 {
		t.Fatalf("modification sans cache : %v %+v", err, o)
	}
	// l'état relu garde tout
	b, _ := json.Marshal(st)
	var relu Etat
	if err := json.Unmarshal(b, &relu); err != nil || relu.Cbts[ombres[1].ID].Alias == "" || !relu.Cbts[ombres[1].ID].Cache {
		t.Fatalf("relecture : %v %+v", err, relu.Cbts[ombres[1].ID])
	}
}
