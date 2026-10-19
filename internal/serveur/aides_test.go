package serveur

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Tchyze/cof_dm/internal/comptes"
	"github.com/Tchyze/cof_dm/internal/store"
)

// image de test rangée sur une fiche (nom au format des images de l'univers).
func (b *banc) image(id float64, fichier string) {
	b.t.Helper()
	if _, err := b.s.store.AjouterMedia(int64(id), fichier, "test.png", []byte("\x89PNG\r\n\x1a\n-test-"), "mj"); err != nil {
		b.t.Fatal(err)
	}
}

const (
	imgAide  = "0123456789abcdef0123.png"
	imgBaron = "fedcba9876543210fedc.png"
)

// À la maison : le MJ envoie une aide de jeu à un PJ ; elle arrive dans les documents de ce PJ
// seulement, réduite au nom, à la forme, au texte remis et aux images. Le secret dont c'est
// l'indice devient « soupçonné », « qui sait quoi » note l'indice reçu ; rien d'autre ne sort.
func TestAidesDeJeu(t *testing.T) {
	b, _ := bancReseau(t)
	s := b.scene()
	secret := b.fiche("secret", "La vérité du baron", map[string]any{"statut": "caché", "campagne": s.camp, "verite": "SECRET-MJ vérité"})
	aide := b.fiche("aide", "Lettre scellée", map[string]any{"forme": "lettre", "campagne": s.camp, "indice_de": secret,
		"contenu": "Rendez-vous chez [[Baron Hugues|le baron]], à [[Fort Boueux#cave]]. ![[plan.png]]", "notes": "SECRET-MJ notes", "prevue": s.seance})
	b.image(aide, imgAide)
	b.image(s.baron, imgBaron)
	envoyer := func(corps map[string]any) (int, map[string]any) {
		return b.local("POST", fmt.Sprintf("/api/aides/%v/envoyer", aide), corps)
	}

	// la relation « a reçu » ne se pose pas à la main
	if c, v := b.local("POST", "/api/relations", map[string]any{"a": s.aldric, "b": aide, "type": "recu"}); c != 422 {
		t.Fatalf("relation réservée : %d %v", c, v)
	}
	if c, _ := b.local("POST", "/api/relations", map[string]any{"a": s.baron, "b": aide, "type": "autre"}); c != 201 {
		t.Fatalf("autre relation vers une aide : %d", c)
	}
	// on n'envoie qu'à des PJ, et seulement une aide
	if c, _ := envoyer(map[string]any{"qui": []any{s.baron}}); c != 422 {
		t.Fatalf("envoi à un PNJ : %d", c)
	}
	if c, _ := envoyer(map[string]any{}); c != 422 {
		t.Fatalf("envoi sans destinataire : %d", c)
	}
	if c, _ := b.local("POST", fmt.Sprintf("/api/aides/%v/envoyer", s.baron), map[string]any{"qui": []any{s.aldric}}); c != 422 {
		t.Fatalf("envoi d'un PNJ : %d", c)
	}
	// une aide ne se partage pas comme une fiche
	if c, _ := b.local("POST", fmt.Sprintf("/api/entites/%v/partage", aide), map[string]any{"partage": true}); c != 422 {
		t.Fatalf("partage d'une aide : %d", c)
	}
	c, v := envoyer(map[string]any{"qui": []any{s.aldric}, "seance": s.seance})
	if c != 200 || fmt.Sprint(v["envoyes"]) != fmt.Sprintf("[%v]", s.aldric) || v["statut_secret"] != "soupçonné" {
		t.Fatalf("envoi : %d %v", c, v)
	}
	if _, v := envoyer(map[string]any{"qui": []any{s.aldric}}); len(v["envoyes"].([]any)) != 0 || v["statut_secret"] != nil {
		t.Fatalf("second envoi au même PJ : %v", v)
	}
	if _, sec := b.local("GET", fmt.Sprintf("/api/entites/%v", secret), nil); sec["data"].(map[string]any)["statut"] != "soupçonné" {
		t.Fatalf("statut du secret : %v", sec["data"])
	}
	// côté MJ : la liste des aides et « qui sait quoi »
	var aides []map[string]any
	_, corps := b.joueurLibreLocal("/api/aides?campagne=" + fmt.Sprint(s.camp))
	json.Unmarshal([]byte(corps), &aides)
	if len(aides) != 1 || len(aides[0]["envois"].([]any)) != 1 || aides[0]["indice_de"].(map[string]any)["id"] != secret || aides[0]["prevue"] != s.seance {
		t.Fatalf("aides : %s", corps)
	}
	if e := aides[0]["envois"].([]any)[0].(map[string]any); e["pj"].(map[string]any)["id"] != s.aldric || e["seance"].(map[string]any)["id"] != s.seance {
		t.Fatalf("envoi listé : %v", e)
	}
	_, qsq := b.local("GET", "/api/secrets", nil)
	if i, _ := qsq["indices"].([]any); len(i) != 1 || i[0].(map[string]any)["qui"] != s.aldric || i[0].(map[string]any)["secret"] != secret {
		t.Fatalf("qui sait quoi, indices : %v", qsq["indices"])
	}
	if c, _ := len(qsq["connaissances"].([]any)), 0; c != 0 {
		t.Fatalf("l'indice ne révèle pas le secret : %v", qsq["connaissances"])
	}

	// page des joueurs : fermée, rien ; ouverte, les documents du PJ choisi
	docs := fmt.Sprintf("/api/joueur/documents?pj=%v", s.aldric)
	doc := fmt.Sprintf("/api/joueur/documents/%v?pj=%v", aide, s.aldric)
	img := func(f string, pj float64) string {
		return fmt.Sprintf("/api/joueur/documents/%v/images/%s?pj=%v", aide, f, pj)
	}
	for _, u := range []string{docs, doc, img(imgAide, s.aldric)} {
		if c, _ := b.joueurLibre("GET", u, nil, false); c != 401 {
			t.Fatalf("GET %s, page fermée : %d", u, c)
		}
	}
	b.local("POST", "/api/acces/joueurs", map[string]any{"active": true})
	fuites := []string{"SECRET-MJ", "La vérité du baron", "indice_de", "prevue", "Séance 4", "[[", fmt.Sprintf(`"%v"`, secret)}
	propre := func(u, corps string) {
		t.Helper()
		for _, f := range fuites {
			if strings.Contains(corps, f) {
				t.Fatalf("GET %s : %q fuit : %s", u, f, corps)
			}
		}
	}
	c, corps = b.joueurLibre("GET", docs, nil, false)
	var liste []map[string]any
	json.Unmarshal([]byte(corps), &liste)
	if c != 200 || len(liste) != 1 || liste[0]["nom"] != "Lettre scellée" || liste[0]["forme"] != "lettre" || liste[0]["image"] != imgAide {
		t.Fatalf("documents : %d %s", c, corps)
	}
	propre(docs, corps)
	c, corps = b.joueurLibre("GET", doc, nil, false)
	var lu map[string]any
	json.Unmarshal([]byte(corps), &lu)
	if c != 200 || lu["contenu"] != "Rendez-vous chez le baron, à Fort Boueux. " || len(lu["images"].([]any)) != 1 {
		t.Fatalf("document : %d %s", c, corps)
	}
	propre(doc, corps)
	if c, corps := b.joueurLibre("GET", img(imgAide, s.aldric), nil, false); c != 200 || !strings.Contains(corps, "-test-") {
		t.Fatalf("image de l'aide : %d", c)
	}
	// un autre PJ ne l'a pas reçue ; une image d'une autre fiche ne passe pas par l'aide
	if _, corps := b.joueurLibre("GET", fmt.Sprintf("/api/joueur/documents?pj=%v", s.mira), nil, false); corps != "[]\n" {
		t.Fatalf("documents d'un autre PJ : %s", corps)
	}
	for _, u := range []string{fmt.Sprintf("/api/joueur/documents/%v?pj=%v", aide, s.mira), img(imgAide, s.mira), img(imgBaron, s.aldric),
		fmt.Sprintf("/api/joueur/documents/%v?pj=%v", s.baron, s.aldric), fmt.Sprintf("/api/joueur/documents?pj=%v", s.baron), "/api/joueur/documents"} {
		if c, _ := b.joueurLibre("GET", u, nil, false); c != 404 {
			t.Fatalf("GET %s : %d", u, c)
		}
	}
	// l'aide reste hors du reste de la page libre
	if c, _ := b.joueurLibre("GET", fmt.Sprintf("/api/entites/%v", aide), nil, false); c != 401 {
		t.Fatalf("fiche de l'aide par la page libre : %d", c)
	}
	if c, _ := b.joueurLibre("GET", "/medias/"+imgAide, nil, false); c != 401 {
		t.Fatalf("image par /medias : %d", c)
	}

	// à jour : le MJ corrige le texte
	b.local("PATCH", fmt.Sprintf("/api/entites/%v", aide), map[string]any{"data": map[string]any{"contenu": "Corrigé."}})
	if _, corps := b.joueurLibre("GET", doc, nil, false); !strings.Contains(corps, "Corrigé.") {
		t.Fatalf("document corrigé : %s", corps)
	}
	// à tous : les PJ de la campagne, ni morts ni partis
	b.fiche("pj", "Ombre", map[string]any{"campagne": s.camp, "statut": "mort"})
	b.fiche("pj", "Ailleurs", nil)
	if _, v := envoyer(map[string]any{"tous": true}); len(v["envoyes"].([]any)) != 2 {
		t.Fatalf("envoi à tous : %v", v)
	}
	// retirer un envoi, archiver l'aide : elle disparaît des documents
	_, aides0 := b.local("GET", fmt.Sprintf("/api/entites/%v", s.mira), nil)
	var rel float64
	for _, r := range aides0["relations"].([]any) {
		if r := r.(map[string]any); r["type"] == "recu" {
			rel = r["id"].(float64)
		}
	}
	if c, _ := b.local("DELETE", fmt.Sprintf("/api/relations/%v", rel), nil); c != 200 || rel == 0 {
		t.Fatalf("retrait de l'envoi : %d", c)
	}
	if _, corps := b.joueurLibre("GET", fmt.Sprintf("/api/joueur/documents?pj=%v", s.mira), nil, false); corps != "[]\n" {
		t.Fatalf("documents après le retrait : %s", corps)
	}
	b.local("POST", fmt.Sprintf("/api/entites/%v/archive", aide), map[string]any{"archive": true})
	if _, corps := b.joueurLibre("GET", docs, nil, false); corps != "[]\n" {
		t.Fatalf("documents après l'archivage : %s", corps)
	}
	if c, _ := b.joueurLibre("GET", doc, nil, false); c != 404 {
		t.Fatalf("aide archivée lue : %d", c)
	}
	if c, _ := envoyer(map[string]any{"qui": []any{s.aldric}}); c != 422 {
		t.Fatalf("envoi d'une aide archivée : %d", c)
	}
}

// requête GET de l'application (cet ordinateur) : code et corps brut.
func (b *banc) joueurLibreLocal(url string) (int, string) {
	b.t.Helper()
	r := httptest.NewRequest("GET", url, nil)
	r.Host, r.RemoteAddr = "127.0.0.1:8100", "127.0.0.1:5000"
	w := httptest.NewRecorder()
	b.s.ServeHTTP(w, r)
	return w.Code, w.Body.String()
}

// Sur un serveur : un compte « joueur » ne lit que les documents de son PJ, quel que soit le PJ
// qu'il demande ; une aide n'est ni une fiche partagée ni un type de son schéma.
func TestAidesServeur(t *testing.T) {
	b, reg := bancServeur(t)
	mj := b.connecter("meneur")
	creer := func(typ, nom string, data map[string]any) float64 {
		c, v, _ := b.proxy("POST", "/api/entites", map[string]any{"type": typ, "nom": nom, "data": data}, mj)
		if c != 201 {
			t.Fatalf("création de %s : %d %v", nom, c, v)
		}
		return v["id"].(float64)
	}
	lea, tom := creer("pj", "Léandre", nil), creer("pj", "Tomas", nil)
	b.proxy("POST", fmt.Sprintf("/api/entites/%v/partage", lea), map[string]any{"partage": true}, mj)
	pourLea := creer("aide", "Carte du col", map[string]any{"contenu": "Le col est gardé.", "notes": "SECRET-MJ"})
	pourTom := creer("aide", "Billet de Tomas", map[string]any{"contenu": "SECRET-TOM"})
	b.image(pourTom, imgAide)
	for _, x := range [][2]float64{{pourLea, lea}, {pourTom, tom}} {
		if c, v, _ := b.proxy("POST", fmt.Sprintf("/api/aides/%v/envoyer", x[0]), map[string]any{"qui": []any{x[1]}}, mj); c != 200 {
			t.Fatalf("envoi : %d %v", c, v)
		}
	}
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
	for _, u := range []string{"/api/joueur/documents", fmt.Sprintf("/api/joueur/documents?pj=%v", tom)} {
		if c, corps := brut(u, joueuse); c != 200 || !strings.Contains(corps, "Carte du col") || strings.Contains(corps, "Billet") {
			t.Fatalf("GET %s par la joueuse : %d %s", u, c, corps)
		}
	}
	if c, corps := brut(fmt.Sprintf("/api/joueur/documents/%v", pourLea), joueuse); c != 200 || strings.Contains(corps, "SECRET-MJ") {
		t.Fatalf("son document : %d %s", c, corps)
	}
	for _, u := range []string{fmt.Sprintf("/api/joueur/documents/%v?pj=%v", pourTom, tom), fmt.Sprintf("/api/joueur/documents/%v/images/%s?pj=%v", pourTom, imgAide, tom),
		fmt.Sprintf("/api/entites/%v", pourLea), "/medias/" + imgAide} {
		if c, corps := brut(u, joueuse); (c != 404 && c != 403) || strings.Contains(corps, "SECRET") {
			t.Fatalf("GET %s par la joueuse : %d %s", u, c, corps)
		}
	}
	for _, u := range []string{"/api/schema", "/api/entites", "/api/stats"} {
		if _, corps := brut(u, joueuse); strings.Contains(corps, `"cle":"aide"`) || strings.Contains(corps, "Aides de jeu") || strings.Contains(corps, `"recu"`) || strings.Contains(corps, "Carte du col") {
			t.Fatalf("GET %s : l'aide apparaît dans le monde du joueur : %s", u, corps)
		}
	}
	// la joueuse n'envoie rien ; le MJ lit les documents de qui il veut
	if c, _, _ := b.proxy("POST", fmt.Sprintf("/api/aides/%v/envoyer", pourTom), map[string]any{"qui": []any{lea}}, joueuse); c != 403 {
		t.Fatalf("envoi par la joueuse : %d", c)
	}
	if c, _, _ := b.proxy("POST", fmt.Sprintf("/api/aides/%v/envoyer", pourTom), map[string]any{"qui": []any{lea}}, b.connecter("audit")); c != 403 {
		t.Fatalf("envoi par l'auditeur : %d", c)
	}
	if _, corps := brut(fmt.Sprintf("/api/joueur/documents?pj=%v", tom), mj); !strings.Contains(corps, "Billet") {
		t.Fatalf("documents de Tomas lus par le MJ : %s", corps)
	}
}

// En direct : l'arrivée d'une aide n'est annoncée qu'aux appareils de ses destinataires (un compte
// « joueur » n'apprend rien de ce que reçoit un autre PJ) ; un retrait fait relire les listes.
func TestAidesDirect(t *testing.T) {
	b := nouveau(t)
	d := b.s.joueurs
	tous := &ecran{envoi: make(chan []byte, 4)}
	pj1 := &ecran{envoi: make(chan []byte, 4)}
	pj2 := &ecran{envoi: make(chan []byte, 4)}
	d.mu.Lock()
	d.ecrans[tous], d.ecrans[pj1], d.ecrans[pj2] = sansRestriction, 1, 2
	d.mu.Unlock()
	d.document([]int64{1, 3}, store.Entite{ID: 9, Nom: "Lettre"})
	lire := func(e *ecran) map[string]any {
		select {
		case b := <-e.envoi:
			var m map[string]any
			json.Unmarshal(b, &m)
			return m
		default:
			return nil
		}
	}
	if m := lire(tous); m["type"] != "document" || fmt.Sprint(m["pj"]) != "[1 3]" || m["nom"] != "Lettre" {
		t.Fatalf("appareil sans restriction : %v", m)
	}
	if m := lire(pj1); m["type"] != "document" || fmt.Sprint(m["pj"]) != "[1]" {
		t.Fatalf("appareil du PJ 1 : %v", m)
	}
	if m := lire(pj2); m != nil {
		t.Fatalf("appareil du PJ 2 prévenu : %v", m)
	}
	d.documents()
	for _, e := range []*ecran{tous, pj1, pj2} {
		if m := lire(e); m["type"] != "documents" || len(m) != 1 {
			t.Fatalf("retrait : %v", m)
		}
	}
}
