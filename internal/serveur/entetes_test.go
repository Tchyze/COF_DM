package serveur

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Tchyze/cof_dm/internal/comptes"
	"github.com/Tchyze/cof_dm/internal/store"
)

// Une image ne reste que dans le cache du navigateur de celui qui y a droit, jamais dans un cache partagé.
func TestMediaCachePrive(t *testing.T) {
	a, reg, connecter := serveurMulti(t)
	mj := connecter("meneur")
	id := a.st.ID()
	donner(reg, "meneur", id, comptes.MJ)
	_, v := mj.faire("POST", "/api/entites", map[string]any{"type": "lieu", "nom": "Fort Boueux"}, enTeteUnivers, id)
	png := []byte("\x89PNG\r\n\x1a\n0000000000000000")
	c, m := mj.faire("POST", "/api/entites/"+jsonNombre(v["id"])+"/medias?nom=a.png", png, enTeteUnivers, id)
	if c != 201 {
		t.Fatalf("image : %d %v", c, m)
	}
	w := mj.brut("GET", "/medias/"+m["fichier"].(string), nil, enTeteUnivers, id)
	cc := w.Header().Get("Cache-Control")
	if w.Code != 200 || !strings.Contains(cc, "private") || strings.Contains(cc, "immutable") {
		t.Fatalf("image : %d, Cache-Control %q", w.Code, cc)
	}
}

// Une réponse JSON ne reste pas sur un appareil partagé ; les pages et les téléchargements gardent leur
// politique. Aucune permission du navigateur n'est ouverte sans raison.
func TestEntetesDeReponse(t *testing.T) {
	a, reg, connecter := serveurMulti(t)
	mj := connecter("meneur")
	id := a.st.ID()
	donner(reg, "meneur", id, comptes.MJ)
	for _, u := range []string{"/api/acces", "/api/mes-univers", "/api/entites?type=lieu"} {
		w := mj.brut("GET", u, nil, enTeteUnivers, id)
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("%s (%d) : Cache-Control %q", u, w.Code, w.Header().Get("Cache-Control"))
		}
		if !strings.Contains(w.Header().Get("Permissions-Policy"), "camera=()") {
			t.Errorf("%s : Permissions-Policy %q", u, w.Header().Get("Permissions-Policy"))
		}
	}
	// un refus aussi
	anonyme := &navigateur{t: t, a: a, cookies: map[string]*http.Cookie{}}
	if w := anonyme.brut("GET", "/api/entites", nil); w.Code != 401 || w.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("refus : %d %q", w.Code, w.Header().Get("Cache-Control"))
	}
	// la page garde no-cache : elle ne porte aucune donnée, et se met à jour avec le serveur
	if w := mj.brut("GET", "/", nil); w.Code != 200 || w.Header().Get("Cache-Control") != "no-cache" || w.Header().Get("Permissions-Policy") == "" {
		t.Errorf("page : %d %q %q", w.Code, w.Header().Get("Cache-Control"), w.Header().Get("Permissions-Policy"))
	}
	// le téléchargement d'un export reste un téléchargement
	w := mj.brut("GET", "/api/export", nil, enTeteUnivers, id)
	if w.Code != 200 || !strings.Contains(w.Header().Get("Content-Disposition"), "attachment") || !strings.Contains(w.Body.String(), "cof-dm") {
		t.Errorf("export : %d %v", w.Code, w.Header())
	}
}

// Les dossiers de l'interface ne se listent pas, et sa page et ses fichiers se servent toujours.
func TestPasDeListageDesDossiers(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Ouvrir(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Fermer()
	s := Nouveau(st, Options{Version: "test", DossierDon: dir, Frontend: fstest.MapFS{"index.html": {Data: []byte("<html>")}, "js/app.js": {Data: []byte("1")}}})
	for u, attendu := range map[string]int{"/": 200, "/js/app.js": 200, "/js/": 404, "/js": 301} {
		r := httptest.NewRequest("GET", u, nil)
		r.Host, r.RemoteAddr = "127.0.0.1:8100", "127.0.0.1:5000"
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != attendu {
			t.Errorf("%s : %d, attendu %d", u, w.Code, attendu)
		}
		if strings.Contains(w.Body.String(), "app.js") {
			t.Errorf("%s : le dossier est listé", u)
		}
	}
}

// Un anonyme n'apprend pas si un identifiant d'univers existe, et son texte ne lui revient pas ; un compte
// connecté garde sa réponse exacte, dont le bureau relié a besoin.
func TestUniversInconnuAnonyme(t *testing.T) {
	a, reg, connecter := serveurMulti(t)
	mj := connecter("meneur")
	id := a.st.ID()
	donner(reg, "meneur", id, comptes.MJ)
	anonyme := &navigateur{t: t, a: a, cookies: map[string]*http.Cookie{}}
	connu := anonyme.brut("GET", "/api/acces", nil, enTeteUnivers, id)
	inconnu := anonyme.brut("GET", "/api/acces", nil, enTeteUnivers, "<b>nimporte</b>")
	if connu.Code != 200 || inconnu.Code != connu.Code || inconnu.Body.String() != connu.Body.String() {
		t.Fatalf("connu %d %q, inconnu %d %q", connu.Code, connu.Body, inconnu.Code, inconnu.Body)
	}
	for _, u := range []string{"/api/entites", "/api/joueur/direct"} {
		k, n := anonyme.brut("GET", u, nil, enTeteUnivers, id), anonyme.brut("GET", u, nil, enTeteUnivers, "nimporte")
		if k.Code != n.Code || k.Code != 401 {
			t.Fatalf("%s : connu %d, inconnu %d", u, k.Code, n.Code)
		}
	}
	if c, _ := mj.faire("GET", "/api/entites", nil, enTeteUnivers, "nimporte"); c != 404 {
		t.Fatalf("compte connecté, univers inconnu : %d", c)
	}
}

// Le manifeste et les icônes se lisent avant de se connecter : le navigateur les demande sans cookie.
func TestManifesteServi(t *testing.T) {
	dir := t.TempDir()
	reg, err := comptes.Ouvrir(filepath.Join(dir, "comptes.json"))
	if err != nil {
		t.Fatal(err)
	}
	a := NouvelleApplication(Options{Version: "test", DossierDon: dir, Port: 8100, Comptes: reg, Frontend: fstest.MapFS{
		"index.html": {Data: []byte("<html>")}, "manifest.webmanifest": {Data: []byte("{}")}, "icones/app-192.png": {Data: []byte("\x89PNG\r\n\x1a\n")}}})
	if err := a.Demarrer(""); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.Fermer)
	anonyme := &navigateur{t: t, a: a, cookies: map[string]*http.Cookie{}}
	w := anonyme.brut("GET", "/manifest.webmanifest", nil)
	if w.Code != 200 || w.Header().Get("Content-Type") != "application/manifest+json" {
		t.Fatalf("manifeste : %d %q", w.Code, w.Header().Get("Content-Type"))
	}
	w = anonyme.brut("GET", "/icones/app-192.png", nil)
	if w.Code != 200 || w.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("icône : %d %q", w.Code, w.Header().Get("Content-Type"))
	}
	if w := anonyme.brut("GET", "/icones/", nil); w.Code != 404 {
		t.Fatalf("dossier des icônes : %d", w.Code)
	}
}
