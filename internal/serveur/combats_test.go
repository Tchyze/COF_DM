package serveur

import (
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

// Règles de combat d'essai, inventées pour les tests : aucun état, aucune valeur ne vient d'un livre.
const reglesCombat = `{"format":"cof-dm-regles","combat":{"echec_naturel":1,"critique_dm":3,"chance":7,"budget_actions":true,
 "etats":[{"nom":"Ébloui","description":"Vise mal.","att":-4,"def":-3,"init":-6},
          {"nom":"Retranché","description":"Posture fermée.","def":3,"fin":"tour","action":"L"}]}}`

// scène de combat : un bestiaire, une campagne avec sa séance, des PJ (un avec feuille, un sans, un
// absent), des PNJ (un avec profil de combat, un sans) et une rencontre.
type scene struct {
	gluon, meneur, camp, seance, aldric, mira, baron, renc float64
}

func (b *banc) scene() scene {
	b.t.Helper()
	monstre := func(m map[string]any) float64 {
		c, v := b.local("POST", "/api/bestiaire", m)
		if c != 201 {
			b.t.Fatalf("monstre : %d %v", c, v)
		}
		return v["id"].(float64)
	}
	var s scene
	s.gluon = monstre(map[string]any{"nom": "Gluon", "nc": 1, "pv": 9, "defense": 12, "init": 8, "rd": 1,
		"attaques": []any{map[string]any{"nom": "Dard", "type": "C", "bonus": 3, "dm": "1d6"}}})
	s.meneur = monstre(map[string]any{"nom": "Meneur gluon", "nc": 2, "pv": 20, "defense": 14, "init": 13})
	s.camp = b.fiche("campagne", "Les Marches", map[string]any{"statut": "en cours"})
	s.seance = b.fiche("seance", "Séance 4", map[string]any{"campagne": s.camp, "numero": 4, "statut": "prévue"})
	s.aldric = b.fiche("pj", "Aldric", map[string]any{"campagne": s.camp, "statut": "actif", "niveau": 3})
	if c, v := b.local("PUT", fmt.Sprintf("/api/entites/%v/feuille", s.aldric), map[string]any{"pv": 21, "pc": 2, "pc_max": 2,
		"valeurs": map[string]any{"FOR": 16, "DEX": 14, "SAG": 10}, "attaques": []any{map[string]any{"nom": "Épée", "type": "C", "dm": "1d8+3"}}}); c != 200 {
		b.t.Fatalf("feuille : %d %v", c, v)
	}
	s.mira = b.fiche("pj", "Mira", map[string]any{"campagne": s.camp, "statut": "actif"}) // pas encore de feuille
	b.fiche("pj", "Abel", map[string]any{"campagne": s.camp, "statut": "absent"})
	s.baron = b.fiche("pnj", "Baron Hugues", map[string]any{"profil_combat": s.meneur})
	valet := b.fiche("pnj", "Le valet", nil)
	s.renc = b.fiche("rencontre", "Embuscade", map[string]any{"campagne": s.camp, "pnj": []any{s.baron, valet},
		"adversaires": []any{map[string]any{"monstre": s.gluon, "nom": "Gluon", "qte": 2}}})
	return s
}

// combattants d'un état, par nom.
func cbts(v map[string]any) map[string]map[string]any {
	out := map[string]map[string]any{}
	etat, _ := v["etat"].(map[string]any)
	l, _ := etat["cbts"].(map[string]any)
	for _, x := range l {
		c := x.(map[string]any)
		out[c["nom"].(string)] = c
	}
	return out
}

func TestCombat(t *testing.T) {
	b := nouveau(t)
	s := b.scene()
	// lancer la rencontre : ses créatures, son PNJ, les PJ en jeu qui ont une feuille
	c, v := b.local("POST", "/api/combats", map[string]any{"rencontre": s.renc})
	if c != 201 {
		t.Fatalf("lancement : %d %v", c, v)
	}
	res := v["combat"].(map[string]any)
	if res["nom"] != "Embuscade" || res["total"] != float64(4) || res["pj"] != float64(1) || res["seance"] != s.seance || res["campagne"] != s.camp || res["rencontre"] != s.renc {
		t.Fatalf("combat lancé : %v", res)
	}
	if a := fmt.Sprint(v["absents"]); !strings.Contains(a, "Mira") || !strings.Contains(a, "Le valet") || strings.Contains(a, "Abel") {
		t.Fatalf("absents : %v", v["absents"])
	}
	u := fmt.Sprintf("/api/combats/%v", res["id"])
	agir := func(a map[string]any) (int, map[string]any) { t.Helper(); return b.local("POST", u+"/actions", a) }
	c, v = b.local("GET", u, nil)
	l := cbts(v)
	if c != 200 || len(l) != 4 || l["Gluon 1"] == nil || l["Gluon 2"] == nil || l["Baron Hugues"]["pvMax"] != float64(20) || l["Aldric"]["pv"] != float64(21) || l["Aldric"]["pc"] != float64(2) {
		t.Fatalf("combattants : %d %v", c, l)
	}
	if r := l["Baron Hugues"]["ref"].(map[string]any); r["kind"] != "pnj" || r["id"] != s.baron {
		t.Fatalf("le PNJ garde le lien vers sa fiche : %v", r)
	}
	aldric, gluon := l["Aldric"]["id"], l["Gluon 1"]["id"]
	// sans règles de combat : combat minimal, états libres, pas de point de chance
	if c, v := agir(map[string]any{"type": "etat", "id": gluon, "etat": "Trempé"}); c != 200 || cbts(v)["Gluon 1"]["etats"].(map[string]any)["Trempé"] == nil {
		t.Fatalf("état libre : %d %v", c, v)
	}
	dm := "5"
	if c, v := agir(map[string]any{"type": "attaque", "att": aldric, "cible": gluon, "attaque": -1, "mode": "manuel", "nat": 20, "dm": dm}); c != 200 || v["etat"].(map[string]any)["resultat"].(map[string]any)["verdict"] != "hit" {
		t.Fatalf("attaque sans règles : %d %v", c, v)
	}
	if c, v := agir(map[string]any{"type": "chance"}); c != 422 || !strings.Contains(fmt.Sprint(v["detail"]), "point de chance") {
		t.Fatalf("point de chance sans règles : %d %v", c, v)
	}
	// avec des règles : les états ont leurs effets dès l'action suivante
	if c, v := b.local("POST", "/api/regles/import", reglesCombat); c != 200 || v["combat"] != true {
		t.Fatalf("import des règles : %d %v", c, v)
	}
	if c, _ := agir(map[string]any{"type": "etat", "id": gluon, "etat": "Ébloui"}); c != 200 {
		t.Fatalf("état des règles : %d", c)
	}
	_, v = agir(map[string]any{"type": "attaque", "att": aldric, "cible": gluon, "attaque": -1, "mode": "manuel", "nat": 10, "dm": dm, "act": "G"})
	if r := v["etat"].(map[string]any)["resultat"].(map[string]any); r["defense"] != float64(12-3) || r["verdict"] != "hit" {
		t.Fatalf("DEF d'une cible éblouie : %v", r)
	}
	// les PV et les points de chance du PJ vont sur sa feuille, annulation comprise
	feuille := func() map[string]any {
		_, f := b.local("GET", fmt.Sprintf("/api/entites/%v/feuille", s.aldric), nil)
		return f
	}
	if c, _ := agir(map[string]any{"type": "pv", "id": aldric, "sens": "degats", "valeur": 6}); c != 200 || feuille()["pv_actuel"] != float64(15) {
		t.Fatalf("PV reportés : %d %v", c, feuille()["pv_actuel"])
	}
	agir(map[string]any{"type": "attaque", "att": aldric, "cible": gluon, "attaque": -1, "mode": "manuel", "nat": 2, "dm": dm, "act": "G", "divers": -9})
	if c, v := agir(map[string]any{"type": "chance"}); c != 200 || feuille()["pc"] != float64(1) {
		t.Fatalf("point de chance reporté : %d %v %v", c, v, feuille()["pc"])
	}
	if c, v := agir(map[string]any{"type": "annuler"}); c != 200 || feuille()["pc"] != float64(2) || v["annulable"] == float64(0) {
		t.Fatalf("annulation : %d %v", c, feuille()["pc"])
	}
	// entrées en cours de combat : la fiche vient du serveur, sauf en saisie libre
	if c, v := agir(map[string]any{"type": "ajouter", "kind": "monstre", "ref_id": s.gluon, "qte": 1}); c != 200 || cbts(v)["Gluon"] == nil {
		t.Fatalf("renfort du bestiaire : %d %v", c, cbts(v))
	}
	if c, v := agir(map[string]any{"type": "ajouter", "kind": "pj", "ref_id": s.mira}); c != 422 || !strings.Contains(fmt.Sprint(v["detail"]), "feuille") {
		t.Fatalf("PJ sans feuille : %d %v", c, v)
	}
	if c, v := agir(map[string]any{"type": "ajouter", "kind": "monstre", "ref_id": 999}); c != 422 {
		t.Fatalf("monstre inconnu : %d %v", c, v)
	}
	if c, v := agir(map[string]any{"type": "ajouter", "camp": "allie", "fiche": map[string]any{"nom": "Chien errant", "pv": 4, "defense": 10, "init": 12}}); c != 200 || cbts(v)["Chien errant"]["camp"] != "allie" {
		t.Fatalf("saisie libre : %d %v", c, v)
	}
	if c, _ := agir(map[string]any{"type": "danse"}); c != 422 {
		t.Fatalf("action inconnue : %d", c)
	}
	// listes, séance, nom
	_, v = b.local("GET", fmt.Sprintf("/api/combats?seance=%v&en_cours=1", s.seance), nil)
	var liste []map[string]any
	json.Unmarshal([]byte(v["_liste"].(string)), &liste)
	if len(liste) != 1 || liste[0]["termine"] != false {
		t.Fatalf("liste : %v", v)
	}
	if c, _ := agir(map[string]any{"type": "terminer"}); c != 200 {
		t.Fatalf("fin : %d", c)
	}
	if _, v = b.local("GET", "/api/combats?en_cours=1", nil); v["_liste"] != "[]\n" {
		t.Fatalf("combats en cours après la fin : %v", v)
	}
	if c, v := b.local("PATCH", u, map[string]any{"nom": "Embuscade au col", "seance": 0}); c != 200 || v["etat"].(map[string]any)["nom"] != "Embuscade au col" || v["etat"].(map[string]any)["seance"] != nil {
		t.Fatalf("renommer et détacher : %d %v", c, v)
	}
	if c, _ := b.local("PATCH", u, map[string]any{"seance": s.camp}); c != 422 {
		t.Fatalf("rattachement à une fiche qui n'est pas une séance : %d", c)
	}
	// le combat est rangé dans l'univers : relu après un redémarrage du serveur
	b.s.Fermer()
	if c, v := b.local("GET", u, nil); c != 200 || v["etat"].(map[string]any)["termine"] != true || len(cbts(v)) != 6 {
		t.Fatalf("relecture : %d", c)
	}
	// l'export l'emporte
	if _, x := b.local("GET", "/api/export", nil); !strings.Contains(fmt.Sprint(x["documents"]), "combats") {
		t.Fatal("combat absent de l'export")
	}
	if c, _ := b.local("DELETE", u, nil); c != 200 {
		t.Fatalf("suppression : %d", c)
	}
	for _, x := range [][2]string{{"GET", u}, {"DELETE", u}, {"POST", u + "/actions"}, {"GET", "/api/combats/abc"}} {
		if c, _ := b.local(x[0], x[1], map[string]any{"type": "suivant"}); c != 404 {
			t.Fatalf("%s %s après suppression : %d", x[0], x[1], c)
		}
	}
	// un combat libre, sans rencontre ni PJ
	if c, v := b.local("POST", "/api/combats", map[string]any{"nom": "Bagarre"}); c != 201 || v["combat"].(map[string]any)["total"] != float64(0) || v["combat"].(map[string]any)["seance"] != nil {
		t.Fatalf("combat libre : %d %v", c, v)
	}
}

// Le combat suit les rôles : le MJ le joue, l'auditeur le regarde, le joueur n'y a pas accès.
func TestCombatRoles(t *testing.T) {
	b, reg := bancServeur(t)
	reg.Creer("joueuse", "mot-de-passe-joueuse", false)
	reg.Modifier("joueuse", func(c *comptes.Compte) error { c.Roles[b.s.store.ID()] = comptes.Joueur; return nil })
	mj, audit := b.connecter("meneur"), b.connecter("audit")
	c, v, _ := b.proxy("POST", "/api/combats", map[string]any{"nom": "Bagarre"}, mj)
	if c != 201 {
		t.Fatalf("lancement par le MJ : %d %v", c, v)
	}
	u := fmt.Sprintf("/api/combats/%v", v["combat"].(map[string]any)["id"])
	libre := map[string]any{"type": "ajouter", "fiche": map[string]any{"nom": "Gluon", "pv": 5}}
	if c, _, _ := b.proxy("POST", u+"/actions", libre, mj); c != 200 {
		t.Fatalf("action du MJ : %d", c)
	}
	for _, x := range [][2]string{{"GET", "/api/combats"}, {"GET", u}} {
		if c, _, _ := b.proxy(x[0], x[1], nil, audit); c != 200 {
			t.Fatalf("%s %s par l'auditeur : %d", x[0], x[1], c)
		}
	}
	for _, x := range [][2]string{{"POST", "/api/combats"}, {"POST", u + "/actions"}, {"PATCH", u}, {"DELETE", u}} {
		if c, _, _ := b.proxy(x[0], x[1], libre, audit); c != 403 {
			t.Fatalf("%s %s par l'auditeur : %d", x[0], x[1], c)
		}
	}
	for _, nom := range []string{"joueuse", "passant"} {
		for _, x := range [][2]string{{"GET", "/api/combats"}, {"GET", u}, {"GET", u + "/direct"}, {"POST", u + "/actions"}, {"POST", "/api/combats"}} {
			if c, _, _ := b.proxy(x[0], x[1], libre, b.connecter(nom)); c != 403 && c != 404 {
				t.Fatalf("%s %s par %s : %d", x[0], x[1], nom, c)
			}
		}
	}
	if _, v, _ := b.proxy("GET", u, nil, mj); len(cbts(v)) != 1 {
		t.Fatalf("le combat a été modifié par un autre que le MJ : %v", cbts(v))
	}
}

// Suivi en direct : l'écran reçoit l'état à la connexion, puis à chaque action ; une page d'un
// autre site ne peut pas s'y brancher.
func TestCombatDirect(t *testing.T) {
	b := nouveau(t)
	srv := httptest.NewServer(b.s)
	defer srv.Close()
	_, v := b.local("POST", "/api/combats", map[string]any{"nom": "Bagarre"})
	id := v["combat"].(map[string]any)["id"]
	adresse := fmt.Sprintf("ws%s/api/combats/%v/direct", strings.TrimPrefix(srv.URL, "http"), id)
	if _, rep, err := websocket.DefaultDialer.Dial(adresse, http.Header{"Origin": {"http://ailleurs.example"}}); err == nil || rep == nil || rep.StatusCode != 403 {
		t.Fatalf("connexion depuis un autre site acceptée : %v", err)
	}
	conn, _, err := websocket.DefaultDialer.Dial(adresse, http.Header{"Origin": {srv.URL}})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	lire := func() map[string]any {
		t.Helper()
		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		var m map[string]any
		if err := conn.ReadJSON(&m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	if m := lire(); m["type"] != "etat" || m["id"] != id || len(cbts(m)) != 0 {
		t.Fatalf("premier message : %v", m)
	}
	// une action envoyée par le WebSocket n'est pas appliquée : il ne fait que recevoir
	_ = conn.WriteJSON(map[string]any{"type": "ajouter", "fiche": map[string]any{"nom": "Intrus", "pv": 5}})
	_ = conn.WriteJSON(map[string]any{"type": "ping"})
	if m := lire(); m["type"] != "pong" {
		t.Fatalf("réponse au ping : %v", m)
	}
	if c, _ := b.local("POST", fmt.Sprintf("/api/combats/%v/actions", id), map[string]any{"type": "ajouter", "fiche": map[string]any{"nom": "Gluon", "pv": 5}}); c != 200 {
		t.Fatalf("action : %d", c)
	}
	if m := lire(); m["type"] != "etat" || len(cbts(m)) != 1 || cbts(m)["Gluon"] == nil || m["annulable"] != float64(1) {
		t.Fatalf("diffusion : %v", m)
	}
	if c, _ := b.local("DELETE", fmt.Sprintf("/api/combats/%v", id), nil); c != 200 {
		t.Fatalf("suppression : %d", c)
	}
	if m := lire(); m["type"] != "supprime" {
		t.Fatalf("après suppression : %v", m)
	}
	// un combat inconnu ne s'ouvre pas
	if _, rep, err := websocket.DefaultDialer.Dial(strings.Replace(adresse, fmt.Sprintf("/%v/", id), "/999/", 1), nil); err == nil || rep.StatusCode != 404 {
		t.Fatalf("combat inconnu : %v", err)
	}
}

// Un PJ entre en combat avec ses points de mana et les capacités de ses voies ; un sort les dépense,
// et la feuille suit (annulation comprise).
func TestCombatMana(t *testing.T) {
	b := nouveau(t)
	ilse := b.fiche("pj", "Ilse", map[string]any{"statut": "actif", "niveau": 2})
	u := fmt.Sprintf("/api/entites/%v/feuille", ilse)
	if c, v := b.local("PUT", u, map[string]any{"pv": 10, "pm": 5, "pm_max": 8,
		"attaques": []any{map[string]any{"nom": "Trait", "type": "M", "dm": "2d6"}},
		"voies":    []any{map[string]any{"nom": "Voie du feu", "rang": 2, "capacites": []any{"Trait", "Mur de flammes", "Brasier"}}}}); c != 200 {
		t.Fatalf("feuille : %d %v", c, v)
	}
	c, v := b.local("POST", "/api/combats", map[string]any{"nom": "Sorts", "pj": []any{ilse}})
	if c != 201 {
		t.Fatalf("combat : %d %v", c, v)
	}
	id := v["combat"].(map[string]any)["id"]
	cu := fmt.Sprintf("/api/combats/%v", id)
	b.local("POST", cu+"/actions", map[string]any{"type": "ajouter", "fiche": map[string]any{"nom": "Gobelin", "pv": 5, "defense": 10}, "camp": "ennemi"})
	_, v = b.local("GET", cu, nil)
	moi, lui := cbts(v)["Ilse"], cbts(v)["Gobelin"]
	if moi["pm"] != float64(5) || moi["pmMax"] != float64(8) || fmt.Sprint(moi["voies"]) != "[Trait Mur de flammes]" {
		t.Fatalf("Ilse en combat : %v", moi)
	}
	pmFeuille := func() any { _, f := b.local("GET", u, nil); return f["pm"] }
	agir := func(a map[string]any) (int, map[string]any) { return b.local("POST", cu+"/actions", a) }
	if c, v := agir(map[string]any{"type": "attaque", "att": moi["id"], "cible": lui["id"], "attaque": 0, "mode": "manuel", "nat": 12, "act": "G", "cout_pm": 9}); c != 422 {
		t.Fatalf("sort trop cher : %d %v", c, v)
	}
	if c, v := agir(map[string]any{"type": "attaque", "att": moi["id"], "cible": lui["id"], "attaque": 0, "mode": "manuel", "nat": 12, "act": "G", "cout_pm": 3}); c != 200 || pmFeuille() != float64(2) {
		t.Fatalf("sort : %d %v, feuille %v", c, v["detail"], pmFeuille())
	}
	agir(map[string]any{"type": "usage", "id": moi["id"], "cle": "Mur de flammes", "valeur": true})
	if c, v := agir(map[string]any{"type": "usage", "id": moi["id"], "cle": "Mur de flammes", "valeur": true}); c != 422 {
		t.Fatalf("double usage : %d %v", c, v)
	}
	agir(map[string]any{"type": "annuler"})
	agir(map[string]any{"type": "annuler"})
	if pmFeuille() != float64(5) {
		t.Fatalf("annulation : PM de la feuille %v", pmFeuille())
	}
}
