package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Tchyze/cof_dm/internal/serveur"
	"github.com/Tchyze/cof_dm/internal/store"
)

// banc : un vrai serveur COF DM (univers vide) et le serveur MCP qui lui parle avec son jeton.
type banc struct {
	t   *testing.T
	mcp *Serveur
	n   int
}

func nouveau(t *testing.T) *banc {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Ouvrir(filepath.Join(dir, "t.cofdm"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Fermer() })
	s := serveur.Nouveau(st, serveur.Options{Version: "test", DossierDon: dir,
		Frontend: fstest.MapFS{"index.html": {Data: []byte("<html>")}}})
	ts := httptest.NewServer(s)
	t.Cleanup(ts.Close)
	j, _ := st.Meta("api_jeton")
	return &banc{t: t, mcp: Nouveau(NouveauClient(ts.URL, j, ""), "test")}
}

// rpc envoie une requête et rend sa réponse décodée.
func (b *banc) rpc(methode string, params any) map[string]any {
	b.t.Helper()
	b.n++
	p, _ := json.Marshal(params)
	brut, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": b.n, "method": methode, "params": json.RawMessage(p)})
	r, ok := b.mcp.traiter(brut)
	if !ok {
		b.t.Fatalf("%s : pas de réponse", methode)
	}
	j, _ := json.Marshal(r)
	var v map[string]any
	_ = json.Unmarshal(j, &v)
	return v
}

// outil appelle un outil ; erreur dit si le résultat doit être en erreur. Rend le texte du résultat.
func (b *banc) outil(nom string, args map[string]any, erreur bool) string {
	b.t.Helper()
	v := b.rpc("tools/call", map[string]any{"name": nom, "arguments": args})
	res, ok := v["result"].(map[string]any)
	if !ok {
		b.t.Fatalf("%s : réponse sans résultat : %v", nom, v)
	}
	contenu := res["content"].([]any)[0].(map[string]any)["text"].(string)
	if enErreur, _ := res["isError"].(bool); enErreur != erreur {
		b.t.Fatalf("%s : isError = %v, attendu %v : %s", nom, enErreur, erreur, contenu)
	}
	return contenu
}

// json décode le texte d'un résultat.
func (b *banc) json(nom string, args map[string]any) map[string]any {
	b.t.Helper()
	var v map[string]any
	if err := json.Unmarshal([]byte(b.outil(nom, args, false)), &v); err != nil {
		b.t.Fatalf("%s : %v", nom, err)
	}
	return v
}

func TestProtocole(t *testing.T) {
	b := nouveau(t)
	init := b.rpc("initialize", map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{},
		"clientInfo": map[string]any{"name": "essai", "version": "1"}})["result"].(map[string]any)
	if init["protocolVersion"] != "2025-06-18" || init["serverInfo"].(map[string]any)["name"] != "cof-dm" {
		t.Fatalf("initialize : %v", init)
	}
	if _, ok := init["capabilities"].(map[string]any)["tools"]; !ok {
		t.Fatal("initialize : l'outil n'est pas annoncé")
	}
	inconnue := b.rpc("initialize", map[string]any{"protocolVersion": "1999-01-01"})["result"].(map[string]any)
	if inconnue["protocolVersion"] != versions[0] {
		t.Fatalf("version inconnue : %v", inconnue["protocolVersion"])
	}
	if v := b.rpc("ping", nil); v["result"] == nil {
		t.Fatalf("ping : %v", v)
	}
	if v := b.rpc("resources/list", nil); v["error"].(map[string]any)["code"] != float64(-32601) {
		t.Fatalf("méthode inconnue : %v", v)
	}
	if v := b.rpc("tools/call", map[string]any{"name": "rien"}); v["error"] == nil {
		t.Fatalf("outil inconnu : %v", v)
	}

	// liste des outils : noms uniques, arguments requis décrits, lecture et écriture annoncées
	l := b.rpc("tools/list", nil)["result"].(map[string]any)["tools"].([]any)
	vus := map[string]bool{}
	for _, x := range l {
		o := x.(map[string]any)
		nom := o["name"].(string)
		if vus[nom] {
			t.Fatalf("outil en double : %s", nom)
		}
		vus[nom] = true
		schema := o["inputSchema"].(map[string]any)
		props := schema["properties"].(map[string]any)
		if req, ok := schema["required"].([]any); ok {
			for _, r := range req {
				if _, ok := props[r.(string)]; !ok {
					t.Fatalf("%s : argument requis %s non décrit", nom, r)
				}
			}
		}
		if o["description"] == "" || o["annotations"] == nil {
			t.Fatalf("%s : sans description ni annotations", nom)
		}
	}
	for _, n := range []string{"schema", "chercher", "lire_fiche", "creer_fiche", "modifier_fiche", "relier", "reveler_secret"} {
		if !vus[n] {
			t.Fatalf("outil manquant : %s", n)
		}
	}
	if vus["supprimer_fiche"] {
		t.Fatal("l'effacement définitif ne doit pas être offert")
	}

	// le transport : une ligne par message, une notification sans réponse, un lot
	entree := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"ping"}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		``,
		`pas du json`,
		`[{"jsonrpc":"2.0","id":"a","method":"ping"},{"jsonrpc":"2.0","method":"notifications/cancelled"}]`,
	}, "\n")
	var sortie bytes.Buffer
	if err := b.mcp.Servir(strings.NewReader(entree), &sortie); err != nil {
		t.Fatal(err)
	}
	lignes := strings.Split(strings.TrimSpace(sortie.String()), "\n")
	if len(lignes) != 3 || !strings.Contains(lignes[0], `"id":1`) || !strings.Contains(lignes[1], "-32700") ||
		!strings.HasPrefix(lignes[2], `[{"jsonrpc":"2.0","id":"a"`) {
		t.Fatalf("transport : %q", lignes)
	}
}

func TestParcours(t *testing.T) {
	b := nouveau(t)
	if s := b.json("schema", nil); s["types"] == nil {
		t.Fatalf("schema : %v", s)
	}

	lieu := b.json("creer_fiche", map[string]any{"type": "lieu", "nom": "Mont-Veille", "data": map[string]any{"categorie": "ville"}})
	idLieu := lieu["id"].(float64)
	pnj := b.json("creer_fiche", map[string]any{"type": "pnj", "nom": "Hilde Ferrat", "tags": []string{"forge"},
		"data": map[string]any{"statut": "vivant", "lieu": idLieu, "resume": "Forgeronne de [[Mont-Veille]]."}})
	idPnj := pnj["id"].(float64)

	// une valeur refusée revient en erreur lisible, et rien n'est créé
	refus := b.outil("creer_fiche", map[string]any{"type": "pnj", "nom": "Zed", "data": map[string]any{"statut": "volant"}}, true)
	if !strings.Contains(refus, "422") || !strings.Contains(refus, "statut") {
		t.Fatalf("refus : %s", refus)
	}

	// lire par le nom ; la mention est résolue
	lu := b.json("lire_fiche", map[string]any{"nom": "hilde ferrat"})
	if lu["id"] != idPnj || !strings.Contains(fmt.Sprint(lu["mentions"]), "Mont-Veille") {
		t.Fatalf("lire_fiche : %v", lu)
	}
	if e := b.outil("lire_fiche", map[string]any{"nom": "Personne"}, true); !strings.Contains(e, "404") {
		t.Fatalf("fiche inconnue : %s", e)
	}

	// renommer le lieu réécrit la mention
	b.json("modifier_fiche", map[string]any{"id": idLieu, "nom": "Haute-Veille"})
	lu = b.json("lire_fiche", map[string]any{"id": idPnj})
	if r := lu["data"].(map[string]any)["resume"]; r != "Forgeronne de [[Haute-Veille]]." {
		t.Fatalf("mention après renommage : %v", r)
	}
	b.json("modifier_fiche", map[string]any{"id": idPnj, "data": map[string]any{"statut": "mort"}})
	if s := b.json("lire_fiche", map[string]any{"id": idPnj})["data"].(map[string]any)["statut"]; s != "mort" {
		t.Fatalf("modification partielle : %v", s)
	}
	b.outil("modifier_fiche", map[string]any{"id": idPnj}, true)

	// recherche (une liste)
	var trouves []map[string]any
	_ = json.Unmarshal([]byte(b.outil("chercher", map[string]any{"q": "forgeronne"}, false)), &trouves)
	if len(trouves) != 1 || trouves[0]["id"] != idPnj {
		t.Fatalf("chercher : %v", trouves)
	}

	// relations et secrets
	pj := b.json("creer_fiche", map[string]any{"type": "pj", "nom": "Aelis", "data": map[string]any{"niveau": 2}})
	idPj := pj["id"].(float64)
	b.json("relier", map[string]any{"a": idPj, "b": idPnj, "type": "allie", "note": "vieille amie"})
	secret := b.json("creer_fiche", map[string]any{"type": "secret", "nom": "La forge maudite", "data": map[string]any{"statut": "caché"}})
	b.outil("reveler_secret", map[string]any{"id": secret["id"], "qui": []any{idPj}}, false)
	qsq := b.json("qui_sait_quoi", nil)
	if c, _ := qsq["connaissances"].([]any); len(c) != 1 {
		t.Fatalf("qui_sait_quoi : %v", qsq)
	}

	// aide de jeu envoyée : l'indice compte dans « qui sait quoi », le secret est soupçonné
	b.json("modifier_fiche", map[string]any{"id": secret["id"], "data": map[string]any{"statut": "caché"}})
	aide := b.json("creer_fiche", map[string]any{"type": "aide", "nom": "Lettre brûlée", "data": map[string]any{"indice_de": secret["id"], "contenu": "Rendez-vous à la forge."}})
	if r := b.json("envoyer_aide", map[string]any{"id": aide["id"], "tous": true}); fmt.Sprint(r["envoyes"]) != fmt.Sprintf("[%v]", idPj) || r["statut_secret"] != "soupçonné" {
		t.Fatalf("envoyer_aide : %v", r)
	}
	var aides []map[string]any
	_ = json.Unmarshal([]byte(b.outil("aides_de_jeu", nil, false)), &aides)
	if len(aides) != 1 || len(aides[0]["envois"].([]any)) != 1 {
		t.Fatalf("aides_de_jeu : %v", aides)
	}
	if i, _ := b.json("qui_sait_quoi", nil)["indices"].([]any); len(i) != 1 {
		t.Fatalf("qui_sait_quoi, indices : %v", i)
	}
	b.outil("envoyer_aide", map[string]any{"id": idPnj, "tous": true}, true) // pas une aide
	if e := b.outil("relier", map[string]any{"a": idPj, "b": aide["id"], "type": "recu"}, true); !strings.Contains(e, "422") {
		t.Fatalf("relation réservée posée à la main : %s", e)
	}

	// feuille : seuls les champs donnés changent
	b.json("modifier_feuille", map[string]any{"id": idPj, "champs": map[string]any{"equipement": "Épée, corde", "bourse": map[string]any{"po": 12}}})
	f := b.json("feuille_personnage", map[string]any{"nom": "Aelis"})
	if f["equipement"] != "Épée, corde" || f["bourse"].(map[string]any)["po"] != float64(12) || f["niveau"] != float64(2) {
		t.Fatalf("feuille : %v", f)
	}
	b.outil("modifier_feuille", map[string]any{"id": idPj, "champs": map[string]any{"niveau": 5}}, true)

	// archiver, puis restaurer
	b.json("archiver_fiche", map[string]any{"id": idLieu})
	if a := b.json("lire_fiche", map[string]any{"id": idLieu})["archive"]; a != true {
		t.Fatalf("archive : %v", a)
	}
	b.json("archiver_fiche", map[string]any{"id": idLieu, "archive": false})

	// préparation d'une séance (par son nom)
	b.json("creer_fiche", map[string]any{"type": "seance", "nom": "Séance 1"})
	if p := b.json("preparer_seance", map[string]any{"nom": "Séance 1"}); p["seance"] == nil {
		t.Fatalf("preparer_seance : %v", p)
	}

	if r := b.json("clore_seance", map[string]any{"id": b.json("lire_fiche", map[string]any{"nom": "Séance 1"})["id"], "resume": "Fin.", "suivante": true}); r["suivante"] == nil {
		t.Fatalf("clore_seance : %v", r)
	}

	// dés : caché par défaut, formule refusée lisible
	if j := b.json("lancer_des", map[string]any{"formule": "2d6+1", "libelle": "Moral"}); j["cache"] != true || j["formule"] != "2d6 + 1" {
		t.Fatalf("lancer_des : %v", j)
	}
	b.outil("lancer_des", map[string]any{"formule": "beaucoup"}, true)
	if j := b.json("jets", nil); len(j["jets"].([]any)) != 1 {
		t.Fatalf("jets : %v", j)
	}

	// le journal porte l'auteur
	var journal []map[string]any
	_ = json.Unmarshal([]byte(b.outil("journal", map[string]any{"limite": 5}, false)), &journal)
	if len(journal) == 0 || !strings.Contains(fmt.Sprint(journal[0]), "claude") {
		t.Fatalf("journal : %v", journal)
	}

	// lectures sans argument : aucune ne doit échouer sur un univers presque vide
	for _, n := range []string{"statistiques", "chronologie", "partie_en_cours", "bestiaire", "combats", "liens_morts"} {
		b.outil(n, nil, false)
	}
	b.outil("graphe", map[string]any{"id": idPnj, "profondeur": 2}, false)
	b.outil("organigramme", map[string]any{"id": "abc"}, true)
}

func TestJetonInvalide(t *testing.T) {
	b := nouveau(t)
	b.mcp.api.Jeton = "faux"
	if e := b.outil("statistiques", nil, true); !strings.Contains(e, "401") {
		t.Fatalf("jeton invalide : %s", e)
	}
	b.mcp.api.Base = "http://127.0.0.1:1"
	if e := b.outil("statistiques", nil, true); !strings.Contains(e, "ne répond pas") {
		t.Fatalf("application éteinte : %s", e)
	}
}

func TestReponseCoupee(t *testing.T) {
	b := nouveau(t)
	o := &outil{Nom: "gros", Faire: func(*Client, arguments) (any, error) { return strings.Repeat("é", tailleMax+10), nil }}
	r := b.mcp.appeler(o, arguments{})
	txt := r["content"].([]map[string]any)[0]["text"].(string)
	if !strings.HasSuffix(txt, "affinez la demande, avec un filtre ou une limite)") || len([]rune(txt)) > tailleMax+100 {
		t.Fatalf("réponse non coupée : %d caractères", len([]rune(txt)))
	}
}
