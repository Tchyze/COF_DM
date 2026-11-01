package serveur

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/Tchyze/cof_dm/internal/comptes"
)

// bancDirect : un serveur qu'on peut couper et relancer à la même adresse, et un bureau connecté dont
// la copie de son univers est ouverte.
type bancDirect struct {
	t       *testing.T
	a       *Application
	b       *bancAppli
	mj      *navigateur
	adresse string
	ts      *httptest.Server
}

func (d *bancDirect) demarrer() {
	d.t.Helper()
	ln, err := net.Listen("tcp", d.adresse)
	if err != nil {
		d.t.Fatal(err)
	}
	d.ts = httptest.NewUnstartedServer(d.a)
	d.ts.Listener.Close()
	d.ts.Listener = ln
	d.ts.Start()
}

// couper : le serveur disparaît, suivis en direct compris.
func (d *bancDirect) couper() {
	d.t.Helper()
	d.a.srv.salles.fermer()
	d.ts.CloseClientConnections()
	d.ts.Close()
}

func nouveauBancDirect(t *testing.T) *bancDirect {
	t.Helper()
	a, reg, connecter := serveurMulti(t)
	univers := a.st.ID()
	mj := connecter("meneur")
	donner(reg, "meneur", univers, comptes.MJ)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	d := &bancDirect{t: t, a: a, mj: mj, adresse: ln.Addr().String()}
	ln.Close()
	d.demarrer()
	t.Cleanup(func() { d.ts.Close() })
	d.b = nouvelleAppli(t, t.TempDir())
	if c, v := d.b.local("POST", "/api/univers/serveur/connexion", map[string]any{"url": d.ts.URL, "nom": "meneur", "mdp": "mot-de-passe-ruenem"}); c != 200 {
		t.Fatalf("connexion : %d %v", c, v)
	}
	if c, v := d.b.local("POST", "/api/univers/serveur/ouvrir", map[string]any{"univers": univers}); c != 200 {
		t.Fatalf("copie : %d %v", c, v)
	}
	return d
}

// pjPret : la feuille qu'il faut à un PJ pour combattre.
func (d *bancDirect) pjPret(pj int64) {
	d.t.Helper()
	if c, v := d.b.local("PUT", fmt.Sprintf("/api/entites/%d/feuille", pj), map[string]any{"pv": 20, "pv_actuel": 20,
		"valeurs": map[string]any{"FOR": 16, "DEX": 14}, "attaques": []any{map[string]any{"nom": "Épée", "type": "C", "dm": "1d8"}}}); c != 200 {
		d.t.Fatalf("feuille : %d %v", c, v)
	}
}

func idLance(v map[string]any) int64 {
	return int64(v["combat"].(map[string]any)["id"].(float64))
}

func (d *bancDirect) combatLocalExiste(id int64) bool {
	_, err := d.b.a.st.LireDoc(collCombats, id)
	return err == nil
}

// Client connecté : le combat tourne sur le serveur. Le PJ créé juste avant y arrive d'abord, rien du
// combat en cours n'est rangé ici, et la fin revient aussitôt par la synchronisation.
func TestRelaisCombat(t *testing.T) {
	d := nouveauBancDirect(t)
	c, v := d.b.local("POST", "/api/entites", map[string]any{"type": "pj", "nom": "Aldric"})
	if c != 201 {
		t.Fatalf("PJ : %d %v", c, v)
	}
	pj := int64(v["id"].(float64))
	d.pjPret(pj)
	c, v = d.b.local("POST", "/api/combats", map[string]any{"nom": "Au gué", "pj": []int64{pj}})
	if c != 201 {
		t.Fatalf("lancement : %d %v", c, v)
	}
	id := idLance(v)
	if d.combatLocalExiste(id) {
		t.Fatal("le combat en cours est rangé sur le client")
	}
	c, v = d.mj.faire("GET", fmt.Sprintf("/api/combats/%d", id), nil)
	if c != 200 || !strings.Contains(fmt.Sprint(v["etat"]), "Aldric") {
		t.Fatalf("sur le serveur (PJ envoyé d'abord) : %d %v", c, v)
	}
	// une action passe par le client, le serveur l'applique, les joueurs la verraient donc aussitôt
	c, v = d.b.local("POST", fmt.Sprintf("/api/combats/%d/actions", id), map[string]any{"type": "suivant"})
	if c != 200 {
		t.Fatalf("action : %d %v", c, v)
	}
	if _, sv := d.mj.faire("GET", fmt.Sprintf("/api/combats/%d", id), nil); fmt.Sprint(sv["etat"].(map[string]any)["log"]) != fmt.Sprint(v["etat"].(map[string]any)["log"]) {
		t.Fatalf("états différents : %v / %v", sv, v)
	}
	// une action refusée revient telle quelle
	if c, _ = d.b.local("POST", fmt.Sprintf("/api/combats/%d/actions", id), map[string]any{"type": "inconnue"}); c != 422 {
		t.Fatalf("action refusée : %d", c)
	}
	if c, l := listeCombats(t, d.b); c != 200 || len(l) != 1 || l[0]["id"] != float64(id) {
		t.Fatalf("liste : %d %v", c, l)
	}
	// un PJ créé ici après le lancement arrive sur le serveur avant d'y être ajouté
	_, v = d.b.local("POST", "/api/entites", map[string]any{"type": "pj", "nom": "Brune"})
	brune := int64(v["id"].(float64))
	d.pjPret(brune)
	c, v = d.b.local("POST", fmt.Sprintf("/api/combats/%d/actions", id), map[string]any{"type": "ajouter", "kind": "pj", "ref_id": brune})
	if c != 200 || !strings.Contains(fmt.Sprint(v["etat"]), "Brune") {
		t.Fatalf("ajout : %d %v", c, v)
	}
	if c, v = d.b.local("POST", fmt.Sprintf("/api/combats/%d/actions", id), map[string]any{"type": "terminer"}); c != 200 || v["etat"].(map[string]any)["termine"] != true {
		t.Fatalf("fin : %d %v", c, v)
	}
	for fin := time.Now().Add(10 * time.Second); !d.combatLocalExiste(id); time.Sleep(50 * time.Millisecond) {
		if time.Now().After(fin) {
			t.Fatal("le combat terminé n'est pas revenu aussitôt par la synchronisation")
		}
	}
}

func listeCombats(t *testing.T, b *bancAppli) (int, []map[string]any) {
	t.Helper()
	ts := httptest.NewServer(b.a)
	defer ts.Close()
	r, err := http.Get(ts.URL + "/api/combats")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	var l []map[string]any
	_ = json.NewDecoder(r.Body).Decode(&l)
	return r.StatusCode, l
}

// Connexion perdue en plein combat : actions bloquées, rien ne continue ici, et le suivi reprend seul
// au retour du serveur, sur l'état du serveur.
func TestRelaisConnexionPerdue(t *testing.T) {
	d := nouveauBancDirect(t)
	_, v := d.b.local("POST", "/api/entites", map[string]any{"type": "pj", "nom": "Aldric"})
	pj := int64(v["id"].(float64))
	d.pjPret(pj)
	_, v = d.b.local("POST", "/api/combats", map[string]any{"nom": "Au gué", "pj": []int64{pj}})
	id := idLance(v)
	ecran := httptest.NewServer(d.b.a)
	defer ecran.Close()
	conn, _, err := websocket.DefaultDialer.Dial(fmt.Sprintf("ws%s/api/combats/%d/direct", strings.TrimPrefix(ecran.URL, "http"), id), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	lire := func(attendu string) map[string]any {
		t.Helper()
		_ = conn.SetReadDeadline(time.Now().Add(15 * time.Second))
		for {
			_, b, err := conn.ReadMessage()
			if err != nil {
				t.Fatalf("en attendant %q : %v", attendu, err)
			}
			var m map[string]any
			_ = json.Unmarshal(b, &m)
			if m["type"] == attendu || m["etat"] == attendu {
				return m
			}
		}
	}
	lire("etat")
	// le suivi ne fait que recevoir, et répond aux pings de l'écran même sans serveur
	if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"ping"}`)); err != nil {
		t.Fatal(err)
	}
	lire("pong")
	d.couper()
	lire("perdue")
	if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"ping"}`)); err != nil {
		t.Fatal(err)
	}
	lire("pong")
	c, r := d.b.local("POST", fmt.Sprintf("/api/combats/%d/actions", id), map[string]any{"type": "suivant"})
	if c != 503 || !strings.Contains(fmt.Sprint(r["detail"]), "Connexion perdue") {
		t.Fatalf("action sans serveur : %d %v", c, r)
	}
	if c, _ := d.b.local("GET", fmt.Sprintf("/api/combats/%d", id), nil); c != 503 {
		t.Fatalf("lecture sans serveur : %d", c)
	}
	if d.combatLocalExiste(id) {
		t.Fatal("le combat continue ici")
	}
	d.demarrer()
	lire("retablie")
	lire("etat")
	if c, _ := d.b.local("POST", fmt.Sprintf("/api/combats/%d/actions", id), map[string]any{"type": "suivant"}); c != 200 {
		t.Fatalf("action au retour : %d", c)
	}
	lire("etat")
}

// Combat lancé hors connexion : il reste ici, invisible du serveur, et part une fois terminé. Revenu
// le serveur, il continue ici.
func TestRelaisHorsConnexion(t *testing.T) {
	d := nouveauBancDirect(t)
	d.couper()
	c, v := d.b.local("POST", "/api/combats", map[string]any{"nom": "Dans la grotte"})
	if c != 201 {
		t.Fatalf("lancement hors connexion : %d %v", c, v)
	}
	id := idLance(v)
	if !d.combatLocalExiste(id) {
		t.Fatal("le combat hors connexion n'est pas ici")
	}
	url := fmt.Sprintf("/api/combats/%d", id)
	d.demarrer()
	if _, err := d.b.a.echanger(); err != nil {
		t.Fatal(err)
	}
	if c, _ := d.mj.faire("GET", url, nil); c != 404 {
		t.Fatalf("en cours, le combat est visible du serveur : %d", c)
	}
	if c, v := d.b.local("POST", url+"/actions", map[string]any{"type": "terminer"}); c != 200 {
		t.Fatalf("fin ici, serveur revenu : %d %v", c, v)
	}
	// terminé ici et pas encore envoyé : lu ici, le serveur ne le connaît pas
	if c, _ := d.b.local("GET", url, nil); c != 200 {
		t.Fatalf("lecture : %d", c)
	}
	if _, err := d.b.a.echanger(); err != nil {
		t.Fatal(err)
	}
	if c, _ := d.mj.faire("GET", url, nil); c != 200 {
		t.Fatalf("une fois terminé, le combat n'est pas parti : %d", c)
	}
}

// Dés : connecté, les jets vivent sur le serveur ; un jet caché y reste invisible des joueurs. Sans
// serveur, ils restent ici.
func TestRelaisDes(t *testing.T) {
	d := nouveauBancDirect(t)
	if c, v := d.b.local("POST", "/api/des", map[string]any{"formule": "1d6", "libelle": "public"}); c != 201 {
		t.Fatalf("jet : %d %v", c, v)
	}
	if c, v := d.b.local("POST", "/api/des", map[string]any{"formule": "1d6", "libelle": "secret", "cache": true}); c != 201 || v["cache"] != true {
		t.Fatalf("jet caché : %d %v", c, v)
	}
	if l, _ := d.a.srv.jets.depuis(0, false); len(l) != 1 || l[0].Libelle != "public" {
		t.Fatalf("vus des joueurs, sur le serveur : %v", l)
	}
	if l, _ := d.a.srv.jets.depuis(0, true); len(l) != 2 {
		t.Fatalf("vus du MJ, sur le serveur : %v", l)
	}
	if l, _ := d.b.a.srv.jets.depuis(0, true); len(l) != 0 {
		t.Fatalf("jets restés sur le client : %v", l)
	}
	if c, v := d.b.local("GET", "/api/des", nil); c != 200 || len(v["jets"].([]any)) != 2 {
		t.Fatalf("lus du client : %d %v", c, v)
	}
	d.couper()
	if c, v := d.b.local("POST", "/api/des", map[string]any{"formule": "1d6"}); c != 201 {
		t.Fatalf("jet sans serveur : %d %v", c, v)
	}
	if l, _ := d.b.a.srv.jets.depuis(0, true); len(l) != 1 {
		t.Fatalf("jet sans serveur, ici : %v", l)
	}
	d.demarrer()
}

// Un écran qui suit un combat ne bloque rien sur le serveur : ni l'administrateur qui crée un univers,
// ni l'arrêt, sans quoi l'univers ne serait pas sauvegardé et le bureau ne verrait jamais la coupure.
func TestEcranNeBloquePas(t *testing.T) {
	d := nouveauBancDirect(t)
	_, v := d.mj.faire("POST", "/api/combats", map[string]any{"nom": "Au gué"})
	conn, _, err := websocket.DefaultDialer.Dial(fmt.Sprintf("ws%s/api/combats/%d/direct", strings.TrimPrefix(d.ts.URL, "http"), idLance(v)), http.Header{"Authorization": {"Bearer " + d.b.a.cnx().Jeton}})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := d.a.opt.Comptes.Creer("chef", "mot-de-passe-fehc", true); err != nil {
		t.Fatal(err)
	}
	chef := &navigateur{t: t, a: d.a, cookies: map[string]*http.Cookie{}}
	chef.faire("POST", "/api/acces/connexion", map[string]any{"nom": "chef", "mdp": "mot-de-passe-fehc"})
	sous := func(quoi string, f func()) {
		t.Helper()
		fini := make(chan struct{})
		go func() { f(); close(fini) }()
		select {
		case <-fini:
		case <-time.After(5 * time.Second):
			t.Fatalf("%s attend l'écran", quoi)
		}
	}
	sous("la création d'un univers", func() {
		if c, _ := chef.faire("POST", "/api/admin/univers", map[string]any{"nom": "Autre"}); c != 201 {
			t.Errorf("création d'un univers : %d", c)
		}
	})
	sous("l'arrêt", d.a.Fermer)
	// l'écran est coupé par l'arrêt
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				t.Fatal("écran toujours branché après l'arrêt")
			}
			break
		}
	}
}
