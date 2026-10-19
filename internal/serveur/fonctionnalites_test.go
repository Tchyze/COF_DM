package serveur

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Tchyze/cof_dm/internal/comptes"
)

// Couper une fonctionnalité : ses routes répondent 404, ses types quittent le schéma et les listes et
// ne se créent plus ; les fiches existantes restent lisibles et reviennent à la réactivation.
func TestFonctionnalites(t *testing.T) {
	b, _ := bancReseau(t)
	s := b.scene()
	secret := b.fiche("secret", "Le pacte", nil)
	aide := b.fiche("aide", "Billet", map[string]any{"indice_de": secret})
	regler := func(choix map[string]any) {
		t.Helper()
		if c, v := b.local("PUT", "/api/fonctionnalites", choix); c != 200 {
			t.Fatalf("réglage %v : %d %v", choix, c, v)
		}
	}
	if c, v := b.local("PUT", "/api/fonctionnalites", map[string]any{"pirate": true}); c != 422 {
		t.Fatalf("fonctionnalité inconnue : %d %v", c, v)
	}
	if _, v := b.local("GET", "/api/fonctionnalites", nil); !strings.Contains(fmt.Sprint(v), "Dés virtuels") {
		t.Fatalf("liste : %v", v)
	}
	regler(map[string]any{"des": false, "secrets": false, "graphe": false, "ecran_mj": false, "combat": false,
		"seances": false, "organigramme": false, "chronologie": false, "obsidian": false, "feuilles": false, "rencontres": false})
	for _, u := range []string{"/api/des", "/api/secrets", "/api/graphe", "/api/table", "/api/combats", "/api/chronologie", "/api/export/obsidian",
		fmt.Sprintf("/api/entites/%v/feuille", s.aldric), fmt.Sprintf("/api/entites/%v/preparation", s.seance), fmt.Sprintf("/api/entites/%v/difficulte", s.renc)} {
		if c, v := b.local("GET", u, nil); c != 404 || v["fonctionnalite"] == nil {
			t.Fatalf("GET %s, coupée : %d %v", u, c, v)
		}
	}
	if c, _ := b.api("POST", "/api/des", map[string]any{"formule": "1d6"}); c != 404 {
		t.Fatalf("dés par l'API : %d", c)
	}
	// schéma : ni Secret ni Rencontre ; le champ « indice de » d'une aide, qui ne vise qu'un secret, disparaît
	_, sch := b.local("GET", "/api/schema", nil)
	texte := fmt.Sprint(sch["types"])
	if strings.Contains(texte, "cle:secret ") || strings.Contains(texte, "cle:rencontre ") || strings.Contains(texte, "indice_de") {
		t.Fatalf("schéma : %s", texte)
	}
	if f := sch["fonctionnalites"].(map[string]any); f["des"] != false || f["aides"] != true {
		t.Fatalf("état dans le schéma : %v", f)
	}
	if c, _ := b.local("POST", "/api/entites", map[string]any{"type": "secret", "nom": "Autre"}); c != 422 {
		t.Fatalf("création d'un secret, coupé : %d", c)
	}
	if _, v := b.local("GET", "/api/entites", nil); strings.Contains(v["_liste"].(string), "Le pacte") || strings.Contains(v["_liste"].(string), "Embuscade") {
		t.Fatalf("liste avec des fiches coupées : %v", v["_liste"])
	}
	if c, v := b.local("GET", fmt.Sprintf("/api/entites/%v", secret), nil); c != 200 || v["nom"] != "Le pacte" {
		t.Fatalf("secret existant, par son lien : %d %v", c, v)
	}
	// l'aide reste active : son envoi marche, sans toucher au secret coupé
	if c, v := b.local("POST", fmt.Sprintf("/api/aides/%v/envoyer", aide), map[string]any{"qui": []any{s.aldric}}); c != 200 {
		t.Fatalf("envoi d'une aide, secrets coupés : %d %v", c, v)
	}
	// réactivés : tout revient
	regler(map[string]any{"secrets": true, "des": true})
	if c, _ := b.local("GET", "/api/secrets", nil); c != 200 {
		t.Fatalf("secrets réactivés : %d", c)
	}
	if _, v := b.local("GET", "/api/entites?type=secret", nil); !strings.Contains(v["_liste"].(string), "Le pacte") {
		t.Fatalf("secret réactivé : %v", v)
	}

	// page des joueurs coupée : même ouverte par le MJ, elle ne répond plus
	b.local("POST", "/api/acces/joueurs", map[string]any{"active": true})
	if c, _ := b.joueurLibre("GET", "/api/joueur/etat", nil, false); c != 200 {
		t.Fatalf("page ouverte : %d", c)
	}
	regler(map[string]any{"page_joueurs": false})
	for _, u := range []string{"/api/joueur/etat", "/api/joueur/pj", fmt.Sprintf("/api/entites/%v/feuille", s.aldric)} {
		if c, _ := b.joueurLibre("GET", u, nil, false); c != 401 {
			t.Fatalf("GET %s, page coupée : %d", u, c)
		}
	}
	if c, _ := b.local("POST", "/api/acces/joueurs", map[string]any{"active": true}); c != 404 {
		t.Fatalf("ouverture de la page coupée : %d", c)
	}
	if c, _ := b.local("GET", "/api/joueur/etat", nil); c != 404 {
		t.Fatalf("aperçu du MJ, page coupée : %d", c)
	}
	// dés actifs mais page coupée : pas de jet des joueurs
	if c, _ := b.local("GET", "/api/joueur/des", nil); c != 404 {
		t.Fatalf("dés des joueurs, page coupée : %d", c)
	}
}

// Sur un serveur : le MJ règle les fonctionnalités de son univers ; l'auditeur et le joueur non.
// Un joueur ne voit pas les routes coupées.
func TestFonctionnalitesServeur(t *testing.T) {
	b, reg := bancServeur(t)
	reg.Creer("joueuse", "mot-de-passe-joueuse", false)
	reg.Modifier("joueuse", func(c *comptes.Compte) error { c.Roles[b.s.store.ID()] = comptes.Joueur; return nil })
	for _, qui := range []string{"audit", "joueuse"} {
		if c, _, _ := b.proxy("PUT", "/api/fonctionnalites", map[string]any{"des": false}, b.connecter(qui)); c != 403 {
			t.Fatalf("réglage par %s : %d", qui, c)
		}
	}
	if c, _, _ := b.proxy("PUT", "/api/fonctionnalites", map[string]any{"des": false, "page_joueurs": false}, b.connecter("meneur")); c != 200 {
		t.Fatalf("réglage par le MJ : %d", c)
	}
	if !b.s.store.Active("combat") || b.s.store.Active("des") {
		t.Fatal("état après le réglage du MJ")
	}
	if c, _, _ := b.proxy("GET", "/api/joueur/etat", nil, b.connecter("joueuse")); c != 404 {
		t.Fatalf("page des joueurs coupée, compte joueur : %d", c)
	}
}
