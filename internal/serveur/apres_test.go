package serveur

import (
	"fmt"
	"testing"
)

// Après la séance : tout ce qui s'est passé est reporté en une fois, et la séance suivante préparée.
func TestApresSeance(t *testing.T) {
	b := nouveau(t)
	port := b.fiche("lieu", "Port-Brume", nil)
	phare := b.fiche("lieu", "Le phare", nil)
	camp := b.fiche("campagne", "Les Marches", map[string]any{"lieu_groupe": port})
	aelis := b.fiche("pj", "Aelis", map[string]any{"campagne": camp, "statut": "actif"})
	bran := b.fiche("pj", "Bran", map[string]any{"campagne": camp, "statut": "actif"})
	gardien := b.fiche("pnj", "Le gardien", map[string]any{"statut": "vivant"})
	q := b.fiche("quete", "Le phare", map[string]any{"campagne": camp, "statut": "en cours",
		"etapes": []any{map[string]any{"texte": "Trouver la clé", "fait": true}, map[string]any{"texte": "Monter", "fait": false}}})
	sec := b.fiche("secret", "Le maire ment", map[string]any{"campagne": camp, "statut": "caché"})
	renc := b.fiche("rencontre", "Au phare", map[string]any{"quete": q})
	s := b.fiche("seance", "Séance 4", map[string]any{"campagne": camp, "numero": 4, "statut": "prévue"})
	u := fmt.Sprintf("/api/entites/%v/apres-seance", s)
	lire := func(id float64) map[string]any {
		_, v := b.local("GET", fmt.Sprintf("/api/entites/%v", id), nil)
		return v["data"].(map[string]any)
	}

	// une valeur refusée n'en laisse passer aucune
	if c, v := b.local("POST", u, map[string]any{"resume": "Rien ne doit changer.", "quetes": []any{map[string]any{"id": q, "statut": "gagnée"}}}); c != 422 {
		t.Fatalf("statut de quête refusé : %d %v", c, v)
	}
	if d := lire(s); d["statut"] != "prévue" || d["resume"] != nil {
		t.Fatalf("séance modifiée malgré le refus : %v", d)
	}
	if c, _ := b.local("POST", u, map[string]any{"quetes": []any{map[string]any{"id": q, "etapes": []bool{true}}}}); c != 422 {
		t.Fatalf("étapes en nombre différent : %d", c)
	}
	if c, _ := b.local("POST", u, map[string]any{"secrets": []any{map[string]any{"id": sec, "qui": []any{port}}}}); c != 422 {
		t.Fatalf("un lieu qui apprend un secret : %d", c)
	}

	date := map[string]any{"an": 1216, "mois": 3, "jour": 12}
	c, v := b.local("POST", u, map[string]any{
		"resume": "Le phare est rallumé.", "presents": []any{aelis, bran}, "date_jeu": date, "lieu_groupe": phare,
		"quetes":     []any{map[string]any{"id": q, "etapes": []bool{true, true}, "statut": "réussie"}},
		"secrets":    []any{map[string]any{"id": sec, "qui": []any{aelis}, "note": "Le registre"}},
		"morts":      []any{gardien},
		"rencontres": []any{map[string]any{"id": renc}},
		"suivante":   true,
	})
	if c != 200 || v["revelations"] != float64(1) || v["suivante"] == nil {
		t.Fatalf("après-séance : %d %v", c, v)
	}
	if d := lire(s); d["statut"] != "jouée" || d["resume"] != "Le phare est rallumé." || len(d["presents"].([]any)) != 2 {
		t.Fatalf("séance : %v", d)
	}
	if d := lire(camp); d["lieu_groupe"] != phare || d["date_actuelle"].(map[string]any)["jour"] != float64(12) {
		t.Fatalf("campagne : %v", d)
	}
	if d := lire(q); d["statut"] != "réussie" || d["etapes"].([]any)[1].(map[string]any)["fait"] != true {
		t.Fatalf("quête : %v", d)
	}
	if d := lire(gardien); d["statut"] != "mort" {
		t.Fatalf("PNJ : %v", d)
	}
	if d := lire(renc); d["statut"] != "jouée" || d["seance"] != s {
		t.Fatalf("rencontre : %v", d)
	}
	if d := lire(sec); d["statut"] != "en partie révélé" || d["revelation"] != s {
		t.Fatalf("secret : %v", d)
	}
	_, suiv := b.local("GET", fmt.Sprintf("/api/entites/%v", v["suivante"]), nil)
	if sd := suiv["data"].(map[string]any); suiv["nom"] != "Séance 5" || sd["numero"] != float64(5) || sd["statut"] != "prévue" || sd["campagne"] != camp {
		t.Fatalf("séance suivante : %v", suiv)
	}
	if c, _ := b.local("POST", fmt.Sprintf("/api/entites/%v/apres-seance", camp), map[string]any{}); c != 404 {
		t.Fatalf("après-séance d'une campagne : %d", c)
	}
}

func TestNomSuivant(t *testing.T) {
	for _, c := range [][3]any{{"Séance 4", 5, "Séance 5"}, {"Partie n°12", 0, "Partie n°13"}, {"Ouverture", 2, "Séance 2"}} {
		if n := nomSuivant(c[0].(string), c[1].(int)); n != c[2] {
			t.Fatalf("%s → %s", c[0], n)
		}
	}
}
