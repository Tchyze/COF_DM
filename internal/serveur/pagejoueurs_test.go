package serveur

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/Tchyze/cof_dm/internal/comptes"
)

// requête d'un appareil du réseau, sans session : code et corps brut.
func (b *banc) joueurLibre(methode, url string, corps any, entete bool) (int, string) {
	b.t.Helper()
	j, _ := json.Marshal(corps)
	r := httptest.NewRequest(methode, url, bytes.NewReader(j))
	r.Host, r.RemoteAddr = "192.168.1.20:8100", "192.168.1.50:5000"
	if entete {
		r.Header.Set("X-COF-DM", "1")
	}
	w := httptest.NewRecorder()
	b.s.ServeHTTP(w, r)
	return w.Code, w.Body.String()
}

// À la maison : fermée par défaut, la page des joueurs s'ouvre sur le réseau local quand le MJ le
// décide, sans mot de passe, et ne donne que la feuille des PJ (sans les notes du MJ), le combat
// réduit et les hauts faits. Tout le reste demande toujours le MJ.
func TestPageJoueursLibre(t *testing.T) {
	b, _ := bancReseau(t)
	s := b.scene()
	feuille := fmt.Sprintf("/api/entites/%v/feuille", s.aldric)
	_, f := b.local("GET", feuille, nil)
	f["notes"] = "SECRET-MJ"
	if c, v := b.local("PUT", feuille, f); c != 200 {
		t.Fatalf("notes du MJ : %d %v", c, v)
	}
	libres := []string{"/api/joueur/etat", "/api/joueur/pj", "/api/joueur/hauts-faits", feuille, "/api/reference"}
	for _, u := range libres {
		if c, _ := b.joueurLibre("GET", u, nil, false); c != 401 {
			t.Fatalf("GET %s, page fermée : %d", u, c)
		}
	}
	// l'ouverture ne se règle que sur l'ordinateur du MJ
	b.local("POST", "/api/acces/mdp", map[string]any{"nouveau": "dragon-vert"})
	_, _, cookie := b.tablette("POST", "/api/acces/connexion", map[string]any{"mdp": "dragon-vert"}, nil)
	if c, _, _ := b.tablette("POST", "/api/acces/joueurs", map[string]any{"active": true}, cookie); c != 403 {
		t.Fatalf("page ouverte depuis la tablette du MJ : %d", c)
	}
	if c, v := b.local("POST", "/api/acces/joueurs", map[string]any{"active": true}); c != 200 || v["page_joueurs"] != true {
		t.Fatalf("ouverture : %d %v", c, v)
	}
	if _, v := b.local("GET", "/api/acces", nil); v["page_joueurs"] != true || v["edition_joueurs"] != false {
		t.Fatalf("état de l'accès : %v", v)
	}
	for _, u := range libres {
		if c, corps := b.joueurLibre("GET", u, nil, false); c != 200 || strings.Contains(corps, "SECRET-MJ") {
			t.Fatalf("GET %s, page ouverte : %d %s", u, c, corps)
		}
	}
	// le reste de l'application reste fermé
	for _, u := range []string{"/api/entites", fmt.Sprintf("/api/entites/%v", s.aldric), "/api/combats", "/api/bestiaire", "/api/export", "/api/regles",
		"/api/table", "/api/journal", "/api/schema", "/api/parametres", "/api/joueurs/edition", "/api/joueur/ailleurs", fmt.Sprintf("/api/entites/%v/difficulte", s.renc)} {
		if c, corps := b.joueurLibre("GET", u, nil, false); c != 401 {
			t.Fatalf("GET %s par un joueur : %d %s", u, c, corps)
		}
	}
	for _, x := range [][2]string{{"POST", "/api/entites"}, {"POST", "/api/combats"}, {"PUT", "/api/joueurs/edition"}, {"POST", "/api/acces/joueurs"}, {"DELETE", fmt.Sprintf("/api/entites/%v", s.aldric)}} {
		if c, _ := b.joueurLibre(x[0], x[1], map[string]any{"active": true}, true); c != 401 && c != 403 {
			t.Fatalf("%s %s par un joueur : %d", x[0], x[1], c)
		}
	}
	// la liste des PJ : ni mort, ni feuille d'une autre fiche
	_, corps := b.joueurLibre("GET", "/api/joueur/pj", nil, false)
	var pjs []map[string]any
	json.Unmarshal([]byte(corps), &pjs)
	if len(pjs) != 3 || pjs[1]["nom"] != "Aldric" || pjs[1]["feuille"] != true || pjs[2]["nom"] != "Mira" || pjs[2]["feuille"] != false {
		t.Fatalf("liste des PJ : %s", corps)
	}
	if c, _ := b.joueurLibre("GET", fmt.Sprintf("/api/entites/%v/feuille", s.baron), nil, false); c != 404 {
		t.Fatalf("feuille d'un PNJ : %d", c)
	}
	// feuilles verrouillées, puis déverrouillées par le MJ ; ses notes ne bougent pas
	var lue map[string]any
	_, corps = b.joueurLibre("GET", feuille, nil, false)
	json.Unmarshal([]byte(corps), &lue)
	lue["equipement"], lue["notes"] = "Une corde", "piraté"
	if c, _ := b.joueurLibre("PUT", feuille, lue, true); c != 403 {
		t.Fatalf("feuille verrouillée modifiée : %d", c)
	}
	if c, _ := b.local("PUT", "/api/joueurs/edition", map[string]any{"active": true}); c != 200 {
		t.Fatalf("déverrouillage : %d", c)
	}
	if c, _ := b.joueurLibre("PUT", feuille, lue, false); c != 403 {
		t.Fatalf("écriture sans l'en-tête de l'application : %d", c)
	}
	if c, corps := b.joueurLibre("PUT", feuille, lue, true); c != 200 || strings.Contains(corps, "piraté") {
		t.Fatalf("feuille déverrouillée : %d %s", c, corps)
	}
	if _, f := b.local("GET", feuille, nil); f["equipement"] != "Une corde" || f["notes"] != "SECRET-MJ" {
		t.Fatalf("feuille après l'écriture du joueur : équipement %v, notes %v", f["equipement"], f["notes"])
	}
	// le combat : l'ordre, qui joue, les états ; aucun chiffre
	_, v := b.local("POST", "/api/combats", map[string]any{"rencontre": s.renc})
	u := fmt.Sprintf("/api/combats/%v", v["combat"].(map[string]any)["id"])
	_, v = b.local("GET", u, nil)
	gluon, autre := cbts(v)["Gluon 1"]["id"], cbts(v)["Gluon 2"]["id"]
	b.local("POST", u+"/actions", map[string]any{"type": "etat", "id": gluon, "etat": "Trempé"})
	b.local("POST", u+"/actions", map[string]any{"type": "pv", "id": autre, "sens": "degats", "valeur": 99})
	_, corps = b.joueurLibre("GET", "/api/joueur/etat", nil, false)
	var etat struct {
		Edition bool
		Combat  struct {
			Nom   string
			Round int
			Ordre []map[string]any
		}
	}
	json.Unmarshal([]byte(corps), &etat)
	for _, interdit := range []string{`"pv`, `"defense"`, `"attaques"`, `"init"`, `"rd"`, `"carac"`, "Gluon 2"} {
		if strings.Contains(corps, interdit) {
			t.Fatalf("le combat des joueurs contient %s : %s", interdit, corps)
		}
	}
	if !etat.Edition || etat.Combat.Nom != "Embuscade" || etat.Combat.Round != 1 || len(etat.Combat.Ordre) != 3 {
		t.Fatalf("combat réduit : %s", corps)
	}
	var courants, suivants int
	for _, p := range etat.Combat.Ordre {
		if p["courant"] == true {
			courants++
		}
		if p["suivant"] == true {
			suivants++
		}
		if p["nom"] == "Gluon 1" && fmt.Sprint(p["etats"]) != "[Trempé]" {
			t.Fatalf("états du Gluon : %v", p)
		}
		if p["nom"] == "Aldric" && p["pj"] != s.aldric {
			t.Fatalf("le PJ ne se reconnaît pas : %v", p)
		}
	}
	if courants != 1 || suivants != 1 {
		t.Fatalf("un combattant joue, un autre suit : %v", etat.Combat.Ordre)
	}
	// hauts faits : le combat terminé et les records du PJ
	if _, corps = b.joueurLibre("GET", "/api/joueur/hauts-faits", nil, false); !strings.Contains(corps, `"combats":[]`) {
		t.Fatalf("hauts faits avant la fin : %s", corps)
	}
	b.local("POST", u+"/actions", map[string]any{"type": "terminer"})
	_, corps = b.joueurLibre("GET", "/api/joueur/hauts-faits", nil, false)
	var hf struct {
		Combats []map[string]any
		Records []map[string]any
	}
	json.Unmarshal([]byte(corps), &hf)
	if len(hf.Combats) != 1 || len(hf.Records) != 1 || hf.Records[0]["nom"] != "Aldric" || hf.Records[0]["combats"] != float64(1) || hf.Records[0]["pj"] != s.aldric {
		t.Fatalf("hauts faits : %s", corps)
	}
	if _, corps = b.joueurLibre("GET", "/api/joueur/etat", nil, false); !strings.Contains(corps, `"combat":null`) {
		t.Fatalf("combat terminé encore montré : %s", corps)
	}
	// refermée, la page ne répond plus
	b.local("POST", "/api/acces/joueurs", map[string]any{"active": false})
	for _, u := range libres {
		if c, _ := b.joueurLibre("GET", u, nil, false); c != 401 {
			t.Fatalf("GET %s, page refermée : %d", u, c)
		}
	}
}

// Sans écoute sur le réseau, la page des joueurs ne répond pas, même ouverte.
func TestPageJoueursReseauFerme(t *testing.T) {
	b := nouveau(t)
	b.local("POST", "/api/acces/joueurs", map[string]any{"active": true})
	if c, _ := b.joueurLibre("GET", "/api/joueur/etat", nil, false); c != 401 {
		t.Fatalf("page des joueurs sans écoute sur le réseau : %d", c)
	}
	// sur l'ordinateur du MJ, la page se regarde quand même
	if c, v := b.local("GET", "/api/joueur/etat", nil); c != 200 || v["serveur"] != false {
		t.Fatalf("aperçu par le MJ : %d %v", c, v)
	}
}

// Sur un serveur : un compte « joueur » ne voit que le PJ qu'il joue, les combats où il se bat et
// leurs hauts faits ; il ne peut ni ouvrir la page libre ni agir sur un combat.
func TestPageJoueursCompte(t *testing.T) {
	b, reg := bancServeur(t)
	mj := b.connecter("meneur")
	creer := func(typ, nom string, data map[string]any) float64 {
		c, v, _ := b.proxy("POST", "/api/entites", map[string]any{"type": typ, "nom": nom, "data": data}, mj)
		if c != 201 {
			t.Fatalf("création de %s : %d %v", nom, c, v)
		}
		return v["id"].(float64)
	}
	feuille := func(id float64) {
		if c, v, _ := b.proxy("PUT", fmt.Sprintf("/api/entites/%v/feuille", id), map[string]any{"pv": 12, "notes": "SECRET-MJ"}, mj); c != 200 {
			t.Fatalf("feuille : %d %v", c, v)
		}
	}
	lea, tom := creer("pj", "Léandre", nil), creer("pj", "Tomas", nil)
	feuille(lea)
	feuille(tom)
	b.proxy("POST", fmt.Sprintf("/api/entites/%v/partage", lea), map[string]any{"partage": true}, mj)
	reg.Creer("joueuse", "mot-de-passe-joueuse", false)
	reg.Modifier("joueuse", func(c *comptes.Compte) error {
		c.Roles[b.s.store.ID()] = comptes.Joueur
		c.Persos[b.s.store.ID()] = int64(lea)
		return nil
	})
	joueuse := b.connecter("joueuse")
	brut := func(url string, qui func(*http.Request)) (int, string) {
		r := httptest.NewRequest("GET", url, nil)
		r.Host, r.RemoteAddr = "jdr.example.org", "127.0.0.1:5000"
		qui(r)
		w := httptest.NewRecorder()
		b.s.ServeHTTP(w, r)
		return w.Code, w.Body.String()
	}
	if c, v, _ := b.proxy("GET", "/api/joueur/etat", nil, joueuse); c != 200 || v["serveur"] != true || v["pj"] != lea || v["combat"] != nil {
		t.Fatalf("état : %d %v", c, v)
	}
	if c, corps := brut("/api/joueur/pj", joueuse); c != 200 || !strings.Contains(corps, "Léandre") || strings.Contains(corps, "Tomas") {
		t.Fatalf("PJ du compte : %d %s", c, corps)
	}
	// un combat où son PJ ne se bat pas ne lui est pas montré ; celui où il se bat, si
	lancer := func(nom string, pj ...float64) string {
		c, v, _ := b.proxy("POST", "/api/combats", map[string]any{"nom": nom, "pj": pj}, mj)
		if c != 201 {
			t.Fatalf("combat %s : %d %v", nom, c, v)
		}
		return fmt.Sprintf("/api/combats/%v", v["combat"].(map[string]any)["id"])
	}
	sansElle := lancer("Ailleurs", tom)
	if _, v, _ := b.proxy("GET", "/api/joueur/etat", nil, joueuse); v["combat"] != nil {
		t.Fatalf("combat d'un autre PJ montré : %v", v["combat"])
	}
	avecElle := lancer("Au col", lea, tom)
	if _, v, _ := b.proxy("GET", "/api/joueur/etat", nil, joueuse); v["combat"] == nil || v["combat"].(map[string]any)["nom"] != "Au col" {
		t.Fatalf("son combat : %v", v["combat"])
	}
	for _, u := range []string{sansElle, avecElle} {
		b.proxy("POST", u+"/actions", map[string]any{"type": "terminer"}, mj)
	}
	if c, corps := brut("/api/joueur/hauts-faits", joueuse); c != 200 || !strings.Contains(corps, "Au col") || strings.Contains(corps, "Ailleurs") {
		t.Fatalf("hauts faits du compte : %d %s", c, corps)
	}
	// le MJ et l'auditeur voient tout ; un compte sans rôle, rien
	if _, corps := brut("/api/joueur/hauts-faits", mj); !strings.Contains(corps, "Ailleurs") {
		t.Fatalf("hauts faits du MJ : %s", corps)
	}
	if c, _, _ := b.proxy("GET", "/api/joueur/etat", nil, b.connecter("audit")); c != 200 {
		t.Fatalf("état lu par l'auditeur : %d", c)
	}
	if c, _, _ := b.proxy("GET", "/api/joueur/etat", nil, b.connecter("passant")); c != 403 {
		t.Fatalf("état lu sans rôle : %d", c)
	}
	// ni page libre sur un serveur, ni action sur le combat
	for _, x := range [][2]string{{"POST", "/api/acces/joueurs"}, {"POST", avecElle + "/actions"}, {"GET", avecElle}, {"GET", fmt.Sprintf("/api/entites/%v/feuille", tom)}} {
		if c, _, _ := b.proxy(x[0], x[1], map[string]any{"active": true, "type": "rouvrir"}, joueuse); c != 403 && c != 404 {
			t.Fatalf("%s %s par la joueuse : %d", x[0], x[1], c)
		}
	}
	if c, _, _ := b.proxy("POST", "/api/acces/joueurs", map[string]any{"active": true}, mj); c != 404 && c != 405 {
		t.Fatalf("page libre ouverte sur un serveur : %d", c)
	}
}

// Suivi en direct : l'état à la connexion, puis le combat à chaque action, sans chiffre.
func TestPageJoueursDirect(t *testing.T) {
	b := nouveau(t)
	s := b.scene()
	srv := httptest.NewServer(b.s)
	defer srv.Close()
	adresse := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/joueur/direct"
	if _, rep, err := websocket.DefaultDialer.Dial(adresse, http.Header{"Origin": {"http://ailleurs.example"}}); err == nil || rep == nil || rep.StatusCode != 403 {
		t.Fatalf("connexion depuis un autre site acceptée : %v", err)
	}
	conn, _, err := websocket.DefaultDialer.Dial(adresse, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	attendre := func(typ string) map[string]any {
		t.Helper()
		for range 20 {
			_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
			var m map[string]any
			if err := conn.ReadJSON(&m); err != nil {
				t.Fatalf("en attendant « %s » : %v", typ, err)
			}
			if m["type"] == typ {
				return m
			}
		}
		t.Fatalf("message « %s » jamais reçu", typ)
		return nil
	}
	if m := attendre("etat"); m["combat"] != nil || m["edition"] != false {
		t.Fatalf("premier message : %v", m)
	}
	_, v := b.local("POST", "/api/combats", map[string]any{"rencontre": s.renc})
	u := fmt.Sprintf("/api/combats/%v", v["combat"].(map[string]any)["id"])
	if m := attendre("combat"); m["combat"] == nil || len(m["combat"].(map[string]any)["ordre"].([]any)) != 4 {
		t.Fatalf("combat lancé : %v", m)
	}
	_, v = b.local("GET", u, nil)
	b.local("POST", u+"/actions", map[string]any{"type": "pv", "id": cbts(v)["Aldric"]["id"], "sens": "degats", "valeur": 4})
	if m := attendre("fiche"); m["id"] != s.aldric {
		t.Fatalf("feuille signalée : %v", m)
	}
	b.local("PUT", "/api/joueurs/edition", map[string]any{"active": true})
	if m := attendre("edition"); m["active"] != true {
		t.Fatalf("verrou : %v", m)
	}
	b.local("POST", u+"/actions", map[string]any{"type": "terminer"})
	attendre("hauts_faits")
	_ = conn.WriteJSON(map[string]any{"type": "ping"})
	attendre("pong")
}
