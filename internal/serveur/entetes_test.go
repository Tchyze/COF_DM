package serveur

import (
	"net/http"
	"strings"
	"testing"

	"github.com/Tchyze/cof_dm/internal/comptes"
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
