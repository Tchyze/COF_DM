package fiches

import (
	"strings"
	"testing"

	"github.com/Tchyze/cof_dm/internal/regles"
)

// Les bonus des capacités (inventées) s'ajoutent aux valeurs dérivées ; les valeurs saisies ne
// changent pas, et la feuille rangée n'en garde rien.
func TestEffetsCapacites(t *testing.T) {
	raw := []byte(`{"valeurs":{"FOR":12,"DEX":14,"CON":10,"INT":10,"SAG":12,"CHA":10},"pv":12,"pv_actuel":15,"pm":9,"pm_max":4,"rd":0,
		"attaques":[{"nom":"Bâton","type":"C"},{"nom":"Fronde","type":"D"}],"voies":[{"nom":"Voie du gué","rang":2,"capacites":["Pas sûr","Agile"]}]}`)
	effets := []EffetVoie{
		{Cible: "def", Valeur: 1, ParRang: true, Rang: 2, Capacite: "Pas sûr", Voie: "Voie du gué", RangCapa: 1}, // +2
		{Cible: "DEX", Valeur: 2, Rang: 2, Capacite: "Agile", Voie: "Voie du gué", RangCapa: 2},                  // 14 → 16 : mod. +2 → +3
		{Cible: "init", Valeur: 0, Carac: "SAG", Niveau: true, Rang: 2, Capacite: "Agile", RangCapa: 2},          // + niveau 3 + mod. SAG 1
		{Cible: "pv", Valeur: 4, Rang: 2, Capacite: "Pas sûr", RangCapa: 1},
		{Cible: "pm", Valeur: 5, Rang: 2, Capacite: "Pas sûr", RangCapa: 1},
		{Cible: "att_d", Valeur: 1, Rang: 2, Capacite: "Agile", RangCapa: 2},
	}
	f, err := NormaliserFeuilleAvec(raw, 3, &regles.Feuille{DefenseBase: 10}, effets)
	if err != nil {
		t.Fatal(err)
	}
	if f.Valeurs.DEX != 14 || f.Pv != 12 || f.PmMax != 4 {
		t.Fatalf("valeurs saisies changées : DEX %d, PV %d, PM %d", f.Valeurs.DEX, f.Pv, f.PmMax)
	}
	if f.Carac.DEX != 3 || f.Defense != 10+3+2 || f.Init != 16+3+1 {
		t.Fatalf("dérivées : mod. DEX %d, DEF %d, Init %d", f.Carac.DEX, f.Defense, f.Init)
	}
	if f.Attaques[0].Bonus != 3+1 || f.Attaques[1].Bonus != 3+3+1 {
		t.Fatalf("attaques : %d %d", f.Attaques[0].Bonus, f.Attaques[1].Bonus)
	}
	if f.PvMax() != 16 || f.PmMaxTotal() != 9 || f.ValeurTotale("DEX") != 16 || *f.PvActuel != 15 || f.Pm != 9 {
		t.Fatalf("totaux : PV %d/%d, PM %d/%d, DEX %d", *f.PvActuel, f.PvMax(), f.Pm, f.PmMaxTotal(), f.ValeurTotale("DEX"))
	}
	e := f.Effets
	if e == nil || e.Base["def"] != 12 || e.Bonus["def"] != 3 || e.Base["DEX"] != 14 || e.Bonus["DEX"] != 2 || e.Bonus["att_D"] != 2 ||
		e.Bonus["att_C"] != 0 || e.Bonus["pv"] != 4 || len(e.Detail) != 6 {
		t.Fatalf("effets : %+v", e)
	}
	if strings.Contains(string(f.Stockage()), "effets") || !strings.Contains(string(f.JSON()), "effets") {
		t.Fatal("les effets se lisent, ils ne se rangent pas")
	}

	// sans les capacités, les PV actuels sont ramenés au maximum saisi
	g, _ := NormaliserFeuille(raw, 3, nil)
	if *g.PvActuel != 12 || g.Pm != 4 || g.Effets != nil {
		t.Fatalf("sans effets : PV %d, PM %d, %+v", *g.PvActuel, g.Pm, g.Effets)
	}
	// relue sans la bibliothèque (synchronisation, restauration), elle garde ce qui dépasse
	h, _ := RelireFeuille(raw, 3)
	if *h.PvActuel != 15 || h.Pm != 9 {
		t.Fatalf("relue : PV %d, PM %d", *h.PvActuel, h.Pm)
	}
}

// L'historique des niveaux et les points gardés se rangent avec la feuille et se relisent tels quels
// (synchronisation, restauration).
func TestNiveauxRanges(t *testing.T) {
	raw := []byte(`{"pv":14,"points_capacite":2,"niveaux":[{"niveau":2,"quand":1,"pv":4,"jet":"d6[3] + 1 (CON)","pm":2,
		"rangs":[{"voie":"Voie du gué","de":0,"a":1,"capacites":["Pas sûr"]}],"points":1,"etapes":[{"cle":"don","libelle":"Don","texte":"Vue perçante"}]}]}`)
	f, err := RelireFeuille(raw, 2)
	if err != nil {
		t.Fatal(err)
	}
	g, err := RelireFeuille(f.Stockage(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if g.PointsCapacite != 2 || len(g.Niveaux) != 1 || g.Niveaux[0].Jet != "d6[3] + 1 (CON)" || g.Niveaux[0].Pm != 2 ||
		g.Niveaux[0].Rangs[0].Capacites[0] != "Pas sûr" || g.Niveaux[0].Etapes[0].Texte != "Vue perçante" {
		t.Fatalf("historique relu : %+v", g)
	}
	for _, mauvais := range []string{`{"points_capacite":-1}`, `{"niveaux":[{"niveau":0}]}`, `{"niveaux":[{"niveau":2,"rangs":[{"de":3,"a":1}]}]}`} {
		if _, err := NormaliserFeuille([]byte(mauvais), 1, nil); err == nil {
			t.Fatalf("historique invalide accepté : %s", mauvais)
		}
	}
}
