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

// « Montrer à la table » : le MJ montre une aide de jeu (image et texte remis) ou l'image d'une
// fiche ; les joueurs n'en reçoivent que le titre, la légende, le texte remis et l'image elle-même,
// jamais les notes du MJ, le nom du fichier ni la fiche. Rien ne se montre ni ne se lit depuis la page
// libre, et l'image ne se sert plus une fois retirée.
func TestMontrerALaTable(t *testing.T) {
	b, _ := bancReseau(t)
	s := b.scene()
	aide := b.fiche("aide", "Lettre scellée", map[string]any{"contenu": "Rendez-vous au moulin.", "notes": "SECRET-NOTES"})
	b.image(aide, imgAide)
	baron := s.baron
	b.local("POST", "/api/acces/joueurs", map[string]any{"active": true})

	// rien de montré
	if _, corps := b.joueurLibre("GET", "/api/joueur/etat", nil, false); !strings.Contains(corps, `"montre":null`) {
		t.Fatalf("état sans rien de montré : %s", corps)
	}
	if c, _ := b.joueurLibre("GET", "/api/joueur/montre/image", nil, false); c != 404 {
		t.Fatalf("image sans rien de montré : %d", c)
	}
	// les joueurs ne montrent rien, et ne lisent pas ce que lit le MJ
	if c, _ := b.joueurLibre("POST", "/api/afficheur/montrer", map[string]any{"fiche": aide}, true); c != 401 {
		t.Fatalf("montré par un joueur : %d", c)
	}
	for _, u := range []string{"/api/afficheur/montre", "/api/afficheur/attente"} {
		if c, _ := b.joueurLibre("GET", u, nil, false); c != 401 {
			t.Fatalf("GET %s par un joueur : %d", u, c)
		}
	}

	// une fiche sans image ne se montre pas ; une image d'une autre fiche non plus
	if c, _ := b.local("POST", "/api/afficheur/montrer", map[string]any{"fiche": baron}); c != 422 {
		t.Fatalf("fiche sans image montrée : %d", c)
	}
	if c, _ := b.local("POST", "/api/afficheur/montrer", map[string]any{"fiche": aide, "image": imgBaron}); c != 422 {
		t.Fatalf("image d'une autre fiche : %d", c)
	}

	c, v := b.local("POST", "/api/afficheur/montrer", map[string]any{"fiche": aide})
	m, _ := v["montre"].(map[string]any)
	if c != 200 || m["titre"] != "Lettre scellée" || m["texte"] != "Rendez-vous au moulin." || m["image"] != true || m["fichier"] != imgAide || m["fiche"] != aide {
		t.Fatalf("montrer une aide : %d %v", c, v)
	}
	_, corps := b.joueurLibre("GET", "/api/joueur/etat", nil, false)
	for _, interdit := range []string{"SECRET-NOTES", imgAide, `"fiche"`} {
		if strings.Contains(corps, interdit) {
			t.Fatalf("%s vu par les joueurs : %s", interdit, corps)
		}
	}
	if !strings.Contains(corps, "Rendez-vous au moulin.") || !strings.Contains(corps, `"titre":"Lettre scellée"`) {
		t.Fatalf("ce qui est montré, vu des joueurs : %s", corps)
	}
	if c, corps := b.joueurLibre("GET", "/api/joueur/montre/image", nil, false); c != 200 || !strings.Contains(corps, "-test-") {
		t.Fatalf("image montrée : %d %q", c, corps)
	}

	// le titre se vide (une image sans nom), le texte d'une aide peut rester caché
	b.image(baron, imgBaron)
	if _, v := b.local("POST", "/api/afficheur/montrer", map[string]any{"fiche": baron, "titre": ""}); v["montre"].(map[string]any)["titre"] != "" {
		t.Fatalf("titre vidé : %v", v)
	}
	if _, v := b.local("POST", "/api/afficheur/montrer", map[string]any{"fiche": aide, "texte": false}); v["montre"].(map[string]any)["texte"] != nil {
		t.Fatalf("texte retenu : %v", v)
	}

	// retiré : plus rien, plus d'image
	if c, v := b.local("POST", "/api/afficheur/retirer", nil); c != 200 || v["montre"] != nil {
		t.Fatalf("retirer : %d %v", c, v)
	}
	if c, _ := b.joueurLibre("GET", "/api/joueur/montre/image", nil, false); c != 404 {
		t.Fatalf("image après le retrait : %d", c)
	}
	if _, v := b.local("GET", "/api/afficheur/montre", nil); v["montre"] != nil {
		t.Fatalf("montre après le retrait : %v", v)
	}
}

// L'écran d'attente : le nom de la campagne, sa date en jeu en toutes lettres et le lieu du groupe,
// rien d'autre de la campagne.
func TestAfficheurAttente(t *testing.T) {
	b := nouveau(t)
	if c, v := b.local("GET", "/api/afficheur/attente", nil); c != 200 || v["campagne"] != nil {
		t.Fatalf("sans campagne : %d %v", c, v)
	}
	lieu := b.fiche("lieu", "Le pont de pierre", nil)
	b.image(lieu, imgBaron)
	b.fiche("campagne", "Les brumes", map[string]any{"statut": "en cours", "lieu_groupe": lieu, "arc": "SECRET-ARC",
		"date_actuelle": map[string]any{"an": 1216, "mois": 2, "jour": 3}})
	c, v := b.local("GET", "/api/afficheur/attente", nil)
	b2, _ := json.Marshal(v)
	if c != 200 || strings.Contains(string(b2), "SECRET-ARC") || v["campagne"].(map[string]any)["nom"] != "Les brumes" ||
		!strings.HasPrefix(v["date"].(string), "3 ") || v["lieu"].(map[string]any)["nom"] != "Le pont de pierre" || v["lieu"].(map[string]any)["image"] != imgBaron {
		t.Fatalf("écran d'attente : %d %s", c, b2)
	}
}
