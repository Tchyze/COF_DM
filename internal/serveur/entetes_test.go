package serveur

import (
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
