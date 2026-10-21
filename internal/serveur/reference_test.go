package serveur

import (
	"encoding/json"
	"fmt"
	"net/http"
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

// L'éditeur : créer, refuser un doublon, renommer (suivi par les feuilles et les profils),
// prévenir des « divers » qui compteraient deux fois, retirer (les feuilles gardent leur copie).
func TestReferenceEditeur(t *testing.T) {
	b := nouveau(t)
	if c, v := b.local("POST", "/api/reference/monstres", map[string]any{"nom": "X"}); c != 404 {
		t.Fatalf("collection inconnue : %d %v", c, v)
	}
	c, v := b.local("POST", "/api/reference/voies", map[string]any{"nom": "Voie du gué", "rangs": []any{
		map[string]any{"nom": "Pas sûr", "bonus": []any{map[string]any{"cible": "def", "valeur": 1}}},
		map[string]any{"nom": "Trait d'eau", "action": map[string]any{"genre": "sort", "cout_pm": 1}}}})
	if c != 201 {
		t.Fatalf("création : %d %v", c, v)
	}
	voie := v["element"].(map[string]any)
	id := voie["id"]
	if voie["etiquette"] != "maison" || len(voie["rangs"].([]any)) != 5 {
		t.Fatalf("voie créée : %v", voie)
	}
	if c, v := b.local("POST", "/api/reference/voies", map[string]any{"nom": "voie du GUÉ"}); c != 422 || !strings.Contains(v["detail"].(string), "existe déjà") {
		t.Fatalf("doublon : %d %v", c, v)
	}
	if c, v := b.local("POST", "/api/reference/voies", map[string]any{"nom": " "}); c != 422 || v["detail"] != "Nom manquant." {
		t.Fatalf("sans nom : %d %v", c, v)
	}
	b.local("POST", "/api/reference/profils", map[string]any{"nom": "Passeur", "voies": []any{"Voie du gué"}})
	pj := b.fiche("pj", "Aldric", map[string]any{"profil": "Passeur"})
	if c, v := b.local("PUT", fmt.Sprintf("/api/entites/%v/feuille", pj), map[string]any{"pv": 10, "def_divers": 2,
		"voies": []any{map[string]any{"nom": "Voie du gué", "rang": 1, "capacites": []any{"Pas sûr"}}}}); c != 200 {
		t.Fatalf("feuille : %d %v", c, v)
	}
	url := fmt.Sprintf("/api/reference/voies/%v", id)
	if c, v := b.local("GET", url, nil); c != 200 || len(v["utilisee_par"].([]any)) != 1 {
		t.Fatalf("lecture : %d %v", c, v)
	}
	voie["nom"] = "Voie du grand gué"
	c, v = b.local("PUT", url, voie)
	if c != 200 {
		t.Fatalf("modification : %d %v", c, v)
	}
	if a := v["avertissements"].([]any); len(a) != 1 || a[0].(map[string]any)["champ"] != "DEF, divers" {
		t.Fatalf("avertissements : %v", a)
	}
	_, f := b.local("GET", fmt.Sprintf("/api/entites/%v/feuille", pj), nil)
	if f["voies"].([]any)[0].(map[string]any)["nom"] != "Voie du grand gué" {
		t.Fatalf("feuille après renommage : %v", f["voies"])
	}
	_, ref := b.local("GET", "/api/reference", nil)
	if p := ref["profils"].([]any)[0].(map[string]any); p["voies"].([]any)[0] != "Voie du grand gué" {
		t.Fatalf("profil après renommage : %v", p)
	}
	_, v = b.local("GET", fmt.Sprintf("/api/reference/profils/%v", ref["profils"].([]any)[0].(map[string]any)["id"]), nil)
	if len(v["utilisee_par"].([]any)) != 1 {
		t.Fatalf("profil utilisé par : %v", v)
	}
	if c, _ := b.local("DELETE", url, nil); c != 200 {
		t.Fatalf("retrait : %d", c)
	}
	if c, _ := b.local("GET", url, nil); c != 404 {
		t.Fatalf("voie retirée encore lue : %d", c)
	}
	_, f = b.local("GET", fmt.Sprintf("/api/entites/%v/feuille", pj), nil)
	if v := f["voies"].([]any)[0].(map[string]any); v["nom"] != "Voie du grand gué" || v["capacites"].([]any)[0] != "Pas sûr" {
		t.Fatalf("la feuille garde sa copie : %v", v)
	}
}

// L'éditeur suit les rôles : le MJ écrit, l'auditeur lit, le joueur n'y a pas accès.
func TestReferenceEditeurRoles(t *testing.T) {
	b, reg := bancServeur(t)
	reg.Creer("joueuse", "mot-de-passe-joueuse", false)
	reg.Modifier("joueuse", func(c *comptes.Compte) error { c.Roles[b.s.store.ID()] = comptes.Joueur; return nil })
	mj, audit, joueuse := b.connecter("meneur"), b.connecter("audit"), b.connecter("joueuse")
	c, v, _ := b.proxy("POST", "/api/reference/peuples", map[string]any{"nom": "Gens du gué"}, mj)
	if c != 201 {
		t.Fatalf("création par le MJ : %d %v", c, v)
	}
	url := fmt.Sprintf("/api/reference/peuples/%v", v["element"].(map[string]any)["id"])
	for nom, qui := range map[string]func(*http.Request){"audit": audit, "joueuse": joueuse} {
		for _, x := range [][2]string{{"POST", "/api/reference/peuples"}, {"PUT", url}, {"DELETE", url}} {
			if c, _, _ := b.proxy(x[0], x[1], map[string]any{"nom": "Autre"}, qui); c != 403 {
				t.Fatalf("%s %s par %s : %d", x[0], x[1], nom, c)
			}
		}
	}
	if c, _, _ := b.proxy("GET", url, nil, audit); c != 200 {
		t.Fatalf("lecture par l'auditeur : %d", c)
	}
	if c, _, _ := b.proxy("GET", url, nil, joueuse); c != 403 {
		t.Fatalf("lecture par le joueur : %d", c)
	}
}

// Une voie liée à la bibliothèque ajoute ses bonus à la feuille, jusqu'au rang atteint ; une voie
// libre n'en donne pas. Le combat prend les totaux.
func TestReferenceEffetsFeuille(t *testing.T) {
	b := nouveau(t)
	c, v := b.local("POST", "/api/reference/voies", map[string]any{"nom": "Voie du gué", "rangs": []any{
		map[string]any{"nom": "Pas sûr", "bonus": []any{map[string]any{"cible": "def", "valeur": 1, "par_rang": true}}},
		map[string]any{"nom": "Endurant", "bonus": []any{map[string]any{"cible": "pv", "valeur": 5}}},
		map[string]any{"nom": "Hors d'atteinte", "bonus": []any{map[string]any{"cible": "def", "valeur": 10}}}}})
	if c != 201 {
		t.Fatalf("voie : %d %v", c, v)
	}
	pj := b.fiche("pj", "Aldric", map[string]any{"niveau": 2})
	url := fmt.Sprintf("/api/entites/%v/feuille", pj)
	sans := func(rang int, nom string) map[string]any {
		// une voie liée peut arriver sans ses capacités : les règles en complètent les rangs
		b.local("POST", "/api/regles/import", `{"format":"cof-dm-regles","personnage":{"rang_max":5}}`)
		_, f := b.local("PUT", url, map[string]any{"pv": 10, "pv_actuel": 14, "voies": []any{map[string]any{"nom": nom, "rang": rang, "capacites": []any{}}}})
		return f
	}
	f := sans(2, "Voie libre")
	if f["effets"] != nil || f["pv_actuel"] != float64(10) {
		t.Fatalf("voie libre : %v %v", f["effets"], f["pv_actuel"])
	}
	f = sans(2, "voie du GUÉ") // liée par son nom : rangs 1 et 2, pas le 3
	e := f["effets"].(map[string]any)
	if e["bonus"].(map[string]any)["def"] != float64(2) || e["bonus"].(map[string]any)["pv"] != float64(5) || f["pv_actuel"] != float64(14) || f["pv"] != float64(10) {
		t.Fatalf("voie liée : %v, PV %v/%v", e, f["pv_actuel"], f["pv"])
	}
	if base := e["base"].(map[string]any)["def"].(float64); f["defense"] != base+2 {
		t.Fatalf("DEF : %v, base %v", f["defense"], base)
	}
	// rangée sans les effets, relue avec
	raw, _, _ := b.s.store.LireFeuille(int64(pj))
	if strings.Contains(string(raw), "effets") {
		t.Fatalf("effets rangés : %s", raw)
	}
	if _, f = b.local("GET", url, nil); f["effets"] == nil {
		t.Fatal("effets absents à la lecture")
	}
	fc, err := b.s.ficheCombat("pj", int64(pj))
	if err != nil || fc.Pv != 15 || *fc.PvActuel != 14 {
		t.Fatalf("fiche de combat : %+v %v", fc, err)
	}
}
