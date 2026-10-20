package serveur

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/Tchyze/cof_dm/internal/comptes"
)

// bibliothequeEssai : des profils, peuples et voies inventés.
const bibliothequeEssai = `{"profils":[{"ref":"essai-passeur","nom":"Passeur"},{"nom":"Veilleur"}],
	"peuples":[{"nom":"Gens des marais","caracs":"+1 SAG"},{"nom":"Gens des cols"}],
	"voies":[{"ref":"essai-gue","nom":"Voie du gué","rangs":[{"nom":"Pas sûr"}]},{"nom":"Voie de la lanterne"},{"nom":"Voie cachée","description":"VOIE-MAISON-SECRETE"}]}`

// Import avec une étiquette, export sans les identifiants de l'univers, réimport sans doublon.
func TestReferenceImportExport(t *testing.T) {
	b := nouveau(t)
	if c, v := b.local("POST", "/api/reference/import?etiquette=verrouillee", bibliothequeEssai); c != 422 {
		t.Fatalf("étiquette inconnue : %d %v", c, v)
	}
	c, v := b.local("POST", "/api/reference/import?etiquette=maison", bibliothequeEssai)
	if c != 200 || v["total"].(map[string]any)["voies"] != float64(3) {
		t.Fatalf("import : %d %v", c, v)
	}
	_, v = b.local("GET", "/api/reference", nil)
	for _, x := range v["voies"].([]any) {
		voie := x.(map[string]any)
		if voie["etiquette"] != "maison" || voie["id"] == nil {
			t.Fatalf("voie rangée : %v", voie)
		}
	}
	code, x := b.local("GET", "/api/reference/export", nil)
	corps, _ := json.Marshal(x)
	if code != 200 || x["format"] != "cof-dm-reference" || strings.Contains(string(corps), `"id"`) {
		t.Fatalf("export : %d %s", code, corps)
	}
	if c, v := b.local("POST", "/api/reference/import", string(corps)); c != 200 || v["bilan"].(map[string]any)["nouveaux"] != float64(0) {
		t.Fatalf("réimport de l'export : %d %v", c, v)
	}
}

// Le joueur ne lit de la bibliothèque que les voies de la feuille de son PJ, son profil et son
// peuple : à la maison par la page des joueurs, sur un serveur par son compte.
func TestReferenceJoueurs(t *testing.T) {
	prepare := func(b *banc, ecrire func(methode, url string, corps any) (int, map[string]any), pj float64) {
		t.Helper()
		if c, v := ecrire("POST", "/api/reference/import", json.RawMessage(bibliothequeEssai)); c != 200 {
			t.Fatalf("import : %d %v", c, v)
		}
		if c, v := ecrire("PATCH", fmt.Sprintf("/api/entites/%v", pj), map[string]any{"data": map[string]any{"profil": "Passeur", "peuple": "Gens des marais"}}); c != 200 {
			t.Fatalf("profil du PJ : %d %v", c, v)
		}
		voies := []any{map[string]any{"nom": "Voie du gué", "ref": "essai-gue", "rang": 1, "capacites": []any{"Pas sûr"}},
			map[string]any{"nom": "voie de la LANTERNE", "rang": 0}}
		if c, v := ecrire("PUT", fmt.Sprintf("/api/entites/%v/feuille", pj), map[string]any{"pv": 10, "voies": voies}); c != 200 {
			t.Fatalf("feuille : %d %v", c, v)
		}
	}
	verifier := func(qui string, code int, corps string) {
		t.Helper()
		if code != 200 || !strings.Contains(corps, "Voie du gué") || !strings.Contains(corps, "Voie de la lanterne") ||
			strings.Contains(corps, "VOIE-MAISON-SECRETE") || !strings.Contains(corps, "Passeur") || strings.Contains(corps, "Veilleur") ||
			!strings.Contains(corps, "Gens des marais") || strings.Contains(corps, "Gens des cols") {
			t.Fatalf("%s : %d %s", qui, code, corps)
		}
	}

	t.Run("maison", func(t *testing.T) {
		b, _ := bancReseau(t)
		aldric := b.fiche("pj", "Aldric", map[string]any{"statut": "actif"})
		prepare(b, b.local, aldric)
		if c, v := b.local("POST", "/api/acces/joueurs", map[string]any{"active": true}); c != 200 {
			t.Fatalf("ouverture : %d %v", c, v)
		}
		c, corps := b.joueurLibre("GET", fmt.Sprintf("/api/reference?pj=%v", aldric), nil, false)
		verifier("page des joueurs", c, corps)
		if c, corps := b.joueurLibre("GET", "/api/reference", nil, false); c != 200 || strings.Contains(corps, "Voie") {
			t.Fatalf("sans PJ : %d %s", c, corps)
		}
		lieu := b.fiche("lieu", "Le Gué", nil)
		if c, _ := b.joueurLibre("GET", fmt.Sprintf("/api/reference?pj=%v", lieu), nil, false); c != 404 {
			t.Fatalf("une fiche qui n'est pas un PJ : %d", c)
		}
		// le MJ lit tout
		if _, v := b.local("GET", "/api/reference", nil); len(v["voies"].([]any)) != 3 {
			t.Fatalf("bibliothèque du MJ : %v", v)
		}
	})

	t.Run("serveur", func(t *testing.T) {
		b, reg := bancServeur(t)
		mj := b.connecter("meneur")
		ecrire := func(methode, url string, corps any) (int, map[string]any) {
			c, v, _ := b.proxy(methode, url, corps, mj)
			return c, v
		}
		creer := func(nom string) float64 {
			_, v := ecrire("POST", "/api/entites", map[string]any{"type": "pj", "nom": nom})
			id := v["id"].(float64)
			ecrire("POST", fmt.Sprintf("/api/entites/%v/partage", id), map[string]any{"partage": true})
			return id
		}
		lea, tom := creer("Léandre"), creer("Tomas")
		prepare(b, ecrire, lea)
		reg.Creer("joueuse", "mot-de-passe-joueuse", false)
		reg.Modifier("joueuse", func(c *comptes.Compte) error {
			c.Roles[b.s.store.ID()] = comptes.Joueur
			c.Persos[b.s.store.ID()] = int64(lea)
			return nil
		})
		joueuse := b.connecter("joueuse")
		c, v, _ := b.proxy("GET", fmt.Sprintf("/api/reference?pj=%v", lea), nil, joueuse)
		corps, _ := json.Marshal(v)
		verifier("compte joueur", c, string(corps))
		if c, _, _ := b.proxy("GET", fmt.Sprintf("/api/reference?pj=%v", tom), nil, joueuse); c != 404 {
			t.Fatalf("le PJ d'un autre : %d", c)
		}
		if c, v, _ := b.proxy("GET", "/api/reference", nil, joueuse); c != 200 || len(v["voies"].([]any)) != 0 {
			t.Fatalf("sans PJ : %d %v", c, v)
		}
	})
}
