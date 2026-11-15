package serveur

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/Tchyze/cof_dm/internal/combat"
)

// Ce que les joueurs voient d'un combat (page des joueurs) : jamais un combattant caché, le nom
// d'affichage plutôt que le vrai nom, et le tour d'un combattant caché ne désigne personne.
func TestReduireCacheAlias(t *testing.T) {
	heros := combat.DepuisFiche(combat.FicheCombat{Nom: "Héros", Init: 10, Pv: 12}, "pj", "", "")
	cache := combat.DepuisFiche(combat.FicheCombat{Nom: "Assassin", Init: 20, Pv: 9}, "ennemi", "", "")
	cache.Cache = true
	masque := combat.DepuisFiche(combat.FicheCombat{Nom: "Nécrophage", Init: 15, Pv: 9}, "ennemi", "", "")
	masque.Alias = "Silhouette"
	sbire := combat.DepuisFiche(combat.FicheCombat{Nom: "Os", Init: 5, Pv: 3, Sbire: true}, "ennemi", "", "")
	sbire.Pv = 0
	st := combat.Nouveau("Essai", []*combat.Combattant{heros, cache, masque, sbire}, nil)
	if st.Courant().ID != cache.ID {
		t.Fatalf("l'assassin (init 20) commence, pas %s", st.Courant().Nom)
	}
	cj := reduire(st)
	b, _ := json.Marshal(cj)
	for _, interdit := range []string{"Assassin", "Nécrophage", "Os"} {
		if strings.Contains(string(b), interdit) {
			t.Fatalf("%s visible des joueurs : %s", interdit, b)
		}
	}
	if len(cj.Ordre) != 2 || cj.Ordre[0].Nom != "Silhouette" || cj.Ordre[1].Nom != "Héros" {
		t.Fatalf("ordre vu : %s", b)
	}
	for _, p := range cj.Ordre {
		if p.Courant {
			t.Fatalf("le tour d'un combattant caché désigne %s", p.Nom)
		}
	}
}

// Bestiaire : « disparaît en tombant » se coche même sur une fiche officielle verrouillée (rien
// d'autre n'y change), survit à la mise à jour par l'import, et passe au combattant.
func TestBestiaireSbire(t *testing.T) {
	b := nouveau(t)
	if c, v := b.local("POST", "/api/bestiaire/import", `[`+gobelin+`]`); c != 200 || v["crees"] != float64(1) {
		t.Fatalf("import : %d %v", c, v)
	}
	_, l := b.local("GET", "/api/bestiaire", nil)
	var liste []map[string]any
	json.Unmarshal([]byte(l["_liste"].(string)), &liste)
	m := liste[0]
	id := int(m["id"].(float64))
	url := "/api/bestiaire/" + strconv.Itoa(id)
	delete(m, "id")
	delete(m, "modifie")
	m["sbire"] = true
	if c, v := b.local("PUT", url, m); c != 200 || v["sbire"] != true || v["source"] != "officiel" {
		t.Fatalf("case sur une fiche verrouillée : %d %v", c, v)
	}
	m["pv"] = 99
	if c, _ := b.local("PUT", url, m); c != 409 {
		t.Fatalf("fiche verrouillée modifiée : %d", c)
	}
	b.local("POST", "/api/bestiaire/import", `[`+strings.Replace(gobelin, `"pv":5`, `"pv":7`, 1)+`]`)
	if _, v := b.local("GET", url, nil); v["sbire"] != true || v["pv"] != float64(7) {
		t.Fatalf("après réimport : %v", v)
	}
	f, err := b.s.ficheCombat("monstre", int64(id))
	if err != nil || !f.Sbire {
		t.Fatalf("fiche de combat : %v %+v", err, f)
	}
}
