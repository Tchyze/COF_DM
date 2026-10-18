package serveur

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Tchyze/cof_dm/internal/comptes"
	"github.com/gorilla/websocket"
)

// À la maison : le MJ lance (jets publics et cachés), les joueurs lancent pour leur PJ depuis la
// page libre et voient les jets publics, jamais les jets cachés.
func TestDesMaison(t *testing.T) {
	b, _ := bancReseau(t)
	s := b.scene()

	if c, v := b.local("POST", "/api/des", map[string]any{"formule": "1d20+3", "libelle": "Perception"}); c != 201 || v["qui"] != "MJ" || v["formule"] != "1d20 + 3" {
		t.Fatalf("jet du MJ : %d %v", c, v)
	}
	if c, v := b.local("POST", "/api/des", map[string]any{"formule": "2d6", "libelle": "EMBUSCADE-CACHEE", "cache": true}); c != 201 || v["cache"] != true {
		t.Fatalf("jet caché : %d %v", c, v)
	}
	if c, v := b.local("POST", "/api/des", map[string]any{"formule": "1d20*2"}); c != 422 || !strings.Contains(fmt.Sprint(v["detail"]), "illisible") {
		t.Fatalf("formule refusée : %d %v", c, v)
	}

	// page fermée : rien ; ouverte : les jets publics seulement
	if c, _ := b.joueurLibre("GET", "/api/joueur/des", nil, false); c != 401 {
		t.Fatalf("page fermée : %d", c)
	}
	b.local("POST", "/api/acces/joueurs", map[string]any{"active": true})
	c, corps := b.joueurLibre("GET", "/api/joueur/des", nil, false)
	if c != 200 || strings.Contains(corps, "EMBUSCADE-CACHEE") || !strings.Contains(corps, "Perception") {
		t.Fatalf("jets vus des joueurs : %d %s", c, corps)
	}

	// un joueur lance pour son PJ, jamais en caché ; le PJ doit exister
	c, corps = b.joueurLibre("POST", "/api/joueur/des", map[string]any{"formule": "2d20kh1+2", "libelle": "Attaque", "pj": s.aldric, "cache": true}, true)
	var j map[string]any
	json.Unmarshal([]byte(corps), &j)
	if c != 201 || j["qui"] != "Aldric" || j["cache"] != nil || j["mj"] != nil {
		t.Fatalf("jet d'un joueur : %d %s", c, corps)
	}
	for _, pj := range []any{0, s.baron, 9999} {
		if c, corps := b.joueurLibre("POST", "/api/joueur/des", map[string]any{"formule": "1d20", "pj": pj}, true); c != 422 && c != 404 {
			t.Fatalf("jet pour %v : %d %s", pj, c, corps)
		}
	}
	if c, _ := b.joueurLibre("POST", "/api/joueur/des", map[string]any{"formule": "1d20", "pj": s.aldric}, false); c != 403 && c != 401 {
		t.Fatalf("écriture sans l'en-tête : %d", c)
	}
	// le jet caché ne s'ouvre pas aux joueurs, ni par la route du MJ
	if c, _ := b.joueurLibre("GET", "/api/des", nil, false); c != 401 {
		t.Fatalf("jets du MJ lus par un joueur : %d", c)
	}
	_, v := b.local("GET", "/api/des", nil)
	if l := v["jets"].([]any); len(l) != 3 || v["dernier"] != float64(3) {
		t.Fatalf("jets du MJ : %v", v)
	}
	_, v = b.local("GET", "/api/des?depuis=2", nil)
	if l := v["jets"].([]any); len(l) != 1 || l[0].(map[string]any)["qui"] != "Aldric" {
		t.Fatalf("jets depuis 2 : %v", v)
	}

	// l'historique ne garde que les derniers jets
	for range jetsGardes + 5 {
		b.local("POST", "/api/des", map[string]any{"formule": "1d6"})
	}
	if _, v := b.local("GET", "/api/des", nil); len(v["jets"].([]any)) != jetsGardes {
		t.Fatalf("historique : %d jets", len(v["jets"].([]any)))
	}
}

// Les jets publics partent en direct vers la page des joueurs ; les jets cachés, jamais.
func TestDesDirect(t *testing.T) {
	b := nouveau(t)
	srv := httptest.NewServer(b.s)
	defer srv.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/api/joueur/direct", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var debut map[string]any
	if err := conn.ReadJSON(&debut); err != nil || debut["type"] != "etat" { // l'écran est inscrit
		t.Fatalf("premier message : %v %v", debut, err)
	}
	b.local("POST", "/api/des", map[string]any{"formule": "1d6", "libelle": "CACHE", "cache": true})
	b.local("POST", "/api/des", map[string]any{"formule": "1d8", "libelle": "PUBLIC"})
	for range 5 {
		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		var m map[string]any
		if err := conn.ReadJSON(&m); err != nil {
			t.Fatal(err)
		}
		if m["type"] != "jet" {
			continue
		}
		if l := m["jet"].(map[string]any)["libelle"]; l != "PUBLIC" {
			t.Fatalf("jet diffusé : %v", l)
		}
		return
	}
	t.Fatal("jet public jamais reçu")
}

// Sur un serveur : un joueur lance pour le PJ de son compte, quel que soit le PJ qu'il demande ;
// l'auditeur lit les jets sans lancer.
func TestDesServeur(t *testing.T) {
	b, reg := bancServeur(t)
	mj, audit := b.connecter("meneur"), b.connecter("audit")
	creer := func(nom string) float64 {
		_, v, _ := b.proxy("POST", "/api/entites", map[string]any{"type": "pj", "nom": nom}, mj)
		return v["id"].(float64)
	}
	lea, tom := creer("Léandre"), creer("Tomas")
	b.proxy("POST", fmt.Sprintf("/api/entites/%v/partage", lea), map[string]any{"partage": true}, mj)
	reg.Creer("joueuse", "mot-de-passe-joueuse", false)
	reg.Modifier("joueuse", func(c *comptes.Compte) error {
		c.Roles[b.s.store.ID()] = comptes.Joueur
		c.Persos[b.s.store.ID()] = int64(lea)
		return nil
	})
	joueuse := b.connecter("joueuse")
	b.proxy("POST", "/api/des", map[string]any{"formule": "1d20", "libelle": "SECRET-MJ", "cache": true}, mj)
	if c, v, _ := b.proxy("POST", "/api/joueur/des", map[string]any{"formule": "1d20+1", "pj": tom}, joueuse); c != 201 || v["qui"] != "Léandre" {
		t.Fatalf("jet de la joueuse : %d %v", c, v)
	}
	if c, v, _ := b.proxy("GET", "/api/joueur/des", nil, joueuse); c != 200 || strings.Contains(toJSON(v), "SECRET-MJ") || len(v["jets"].([]any)) != 1 {
		t.Fatalf("jets vus de la joueuse : %d %v", c, v)
	}
	if c, _, _ := b.proxy("GET", "/api/des", nil, joueuse); c != 403 {
		t.Fatalf("jets du MJ lus par la joueuse : %d", c)
	}
	if c, _, _ := b.proxy("POST", "/api/des", map[string]any{"formule": "1d20"}, audit); c != 403 {
		t.Fatalf("jet de l'auditeur : %d", c)
	}
	if c, v, _ := b.proxy("GET", "/api/des", nil, audit); c != 200 || len(v["jets"].([]any)) != 2 {
		t.Fatalf("jets lus par l'auditeur : %d %v", c, v)
	}
	// une joueuse sans PJ ne lance pas
	reg.Modifier("joueuse", func(c *comptes.Compte) error { c.Persos[b.s.store.ID()] = 0; return nil })
	if c, _, _ := b.proxy("POST", "/api/joueur/des", map[string]any{"formule": "1d20", "pj": lea}, joueuse); c != 422 {
		t.Fatalf("jet sans PJ : %d", c)
	}
}
