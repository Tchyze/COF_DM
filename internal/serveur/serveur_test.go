package serveur

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Tchyze/cof_dm/internal/store"
)

type banc struct {
	t     *testing.T
	s     *Serveur
	jeton string
}

func nouveau(t *testing.T) *banc { return nouveauAvec(t, Options{}) }

func nouveauAvec(t *testing.T, opt Options) *banc {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Ouvrir(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Fermer() })
	opt.Version, opt.DossierDon, opt.Frontend = "test", dir, fstest.MapFS{"index.html": {Data: []byte("<html>")}}
	s := Nouveau(st, opt)
	j, _ := st.Meta("api_jeton")
	return &banc{t, s, j}
}

// appel depuis le navigateur local (sans jeton).
func (b *banc) local(methode, url string, corps any) (int, map[string]any) {
	b.t.Helper()
	return b.faire(methode, url, corps, func(r *httptestReq) {
		r.Host, r.RemoteAddr = "127.0.0.1:8100", "127.0.0.1:5000"
		if methode != "GET" {
			r.Header.Set("X-COF-DM", "1")
		}
	})
}

// appel d'un client API (jeton), depuis une autre machine.
func (b *banc) api(methode, url string, corps any) (int, map[string]any) {
	b.t.Helper()
	return b.faire(methode, url, corps, func(r *httptestReq) {
		r.Host, r.RemoteAddr = "192.168.1.20:8100", "192.168.1.50:5000"
		r.Header.Set("Authorization", "Bearer "+b.jeton)
		r.Header.Set("X-COF-Auteur", "claude")
	})
}

type httptestReq = http.Request

func (b *banc) faire(methode, url string, corps any, prep func(*httptestReq)) (int, map[string]any) {
	var body *bytes.Reader
	switch c := corps.(type) {
	case nil:
		body = bytes.NewReader(nil)
	case string:
		body = bytes.NewReader([]byte(c))
	default:
		j, _ := json.Marshal(c)
		body = bytes.NewReader(j)
	}
	r := httptest.NewRequest(methode, url, body)
	prep(r)
	w := httptest.NewRecorder()
	b.s.ServeHTTP(w, r)
	var v map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &v)
	if v == nil {
		v = map[string]any{"_liste": w.Body.String()}
	}
	return w.Code, v
}

func TestAcces(t *testing.T) {
	b := nouveau(t)
	if c, _ := b.local("GET", "/api/sante", nil); c != 200 {
		t.Fatalf("local : %d", c)
	}
	if c, _ := b.faire("GET", "/api/sante", nil, func(r *httptestReq) { r.Host, r.RemoteAddr = "pirate.example", "127.0.0.1:1" }); c != 401 {
		t.Fatalf("hôte étranger : %d", c)
	}
	if c, _ := b.faire("GET", "/api/sante", nil, func(r *httptestReq) { r.Host, r.RemoteAddr = "192.168.1.20:8100", "192.168.1.50:1" }); c != 401 {
		t.Fatalf("réseau sans jeton : %d", c)
	}
	if c, _ := b.faire("POST", "/api/entites", `{}`, func(r *httptestReq) { r.Host, r.RemoteAddr = "localhost", "127.0.0.1:1" }); c != 403 {
		t.Fatalf("écriture locale sans en-tête : %d", c)
	}
	if c, _ := b.faire("GET", "/api/sante", nil, func(r *httptestReq) { r.Header.Set("Authorization", "Bearer faux") }); c != 401 {
		t.Fatalf("mauvais jeton : %d", c)
	}
	if c, _ := b.api("GET", "/api/schema", nil); c != 200 {
		t.Fatalf("API avec jeton : %d", c)
	}
}

func TestParcoursAPI(t *testing.T) {
	b := nouveau(t)
	c, lieu := b.api("POST", "/api/entites", map[string]any{"type": "lieu", "nom": "Mont-Veille", "data": map[string]any{"categorie": "fort"}})
	if c != 201 {
		t.Fatalf("création lieu : %d %v", c, lieu)
	}
	idLieu := lieu["id"].(float64)
	c, pnj := b.api("POST", "/api/entites", map[string]any{"type": "pnj", "nom": "Gautier Roussel",
		"data": map[string]any{"statut": "vivant", "lieu": idLieu, "role": "Capitaine de [[Mont-Veille]]."}, "tags": []string{"milice"}})
	if c != 201 || pnj["refs"].(map[string]any)[fmt.Sprint(idLieu)] == nil {
		t.Fatalf("création PNJ : %d %v", c, pnj)
	}
	id := pnj["id"].(float64)
	if c, v := b.api("POST", "/api/entites", map[string]any{"type": "pnj", "nom": "X", "data": map[string]any{"statut": "zombie"}}); c != 422 || !strings.Contains(v["detail"].(string), "statut") {
		t.Fatalf("validation : %d %v", c, v)
	}
	if c, _ := b.api("POST", "/api/entites", `{"type":"pnj","nom":"X","champ_inconnu":1}`); c != 422 {
		t.Fatalf("clé inconnue au premier niveau : %d", c)
	}
	// modification partielle
	c, v := b.api("PATCH", fmt.Sprintf("/api/entites/%v", id), map[string]any{"data": map[string]any{"statut": "mort"}})
	if c != 200 || v["data"].(map[string]any)["lieu"] != idLieu || v["data"].(map[string]any)["statut"] != "mort" {
		t.Fatalf("PATCH : %d %v", c, v)
	}
	// relation
	c, f := b.api("POST", "/api/entites", map[string]any{"type": "faction", "nom": "La Compagnie de Mont-Veille"})
	if c, v := b.api("POST", "/api/relations", map[string]any{"a": id, "b": f["id"], "type": "dirige"}); c != 201 {
		t.Fatalf("relation : %d %v", c, v)
	}
	c, v = b.local("GET", fmt.Sprintf("/api/entites/%v", f["id"]), nil)
	rel := v["relations"].([]any)
	if c != 200 || len(rel) != 1 || rel[0].(map[string]any)["libelle"] != "dirigé par" {
		t.Fatalf("relation vue de la faction : %v", v["relations"])
	}
	// références vers le lieu
	_, v = b.local("GET", fmt.Sprintf("/api/entites/%v", idLieu), nil)
	if refs := v["references"].([]any); len(refs) != 2 {
		t.Fatalf("références : %v", refs)
	}
	// image
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.Set(1, 1, color.RGBA{200, 10, 10, 255})
	var buf bytes.Buffer
	png.Encode(&buf, img)
	var corps bytes.Buffer
	mw := multipart.NewWriter(&corps)
	fw, _ := mw.CreateFormFile("fichier", "roussel.png")
	fw.Write(buf.Bytes())
	mw.Close()
	r := httptest.NewRequest("POST", fmt.Sprintf("/api/entites/%v/medias", id), &corps)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	r.Header.Set("Authorization", "Bearer "+b.jeton)
	w := httptest.NewRecorder()
	b.s.ServeHTTP(w, r)
	var m map[string]any
	json.Unmarshal(w.Body.Bytes(), &m)
	if w.Code != 201 || m["principal"] != true {
		t.Fatalf("image : %d %s", w.Code, w.Body.String())
	}
	if c, _ := b.local("GET", "/medias/"+m["fichier"].(string), nil); c != 200 {
		t.Fatalf("image servie : %d", c)
	}
	if c, _ := b.local("GET", "/medias/..%2Ft.db", nil); c == 200 {
		t.Fatal("traversée de chemin")
	}
	if c, _ := b.api("POST", fmt.Sprintf("/api/entites/%v/medias?nom=x.txt", id), "pas une image"); c != 415 {
		t.Fatalf("faux fichier : %d", c)
	}
	// résolution, journal (auteur), suppression
	if c, v := b.local("GET", "/api/resoudre?nom=mont-veille", nil); c != 200 || v["nom"] != "Mont-Veille" {
		t.Fatalf("résolution : %d %v", c, v)
	}
	_, j := b.local("GET", "/api/journal", nil)
	if !strings.Contains(j["_liste"].(string), `"auteur":"claude"`) {
		t.Fatalf("auteur absent du journal : %v", j)
	}
	if c, _ := b.api("DELETE", fmt.Sprintf("/api/entites/%v?definitif=1", idLieu), nil); c != 200 {
		t.Fatal("suppression")
	}
	_, v = b.local("GET", fmt.Sprintf("/api/entites/%v", id), nil)
	if v["data"].(map[string]any)["lieu"] != nil {
		t.Fatalf("référence orpheline : %v", v["data"])
	}
	if c, v := b.local("GET", "/api/export", nil); c != 200 || len(v["entites"].([]any)) != 2 {
		t.Fatalf("export : %d", c)
	}
	if c, _ := b.local("POST", "/api/sauvegarde", nil); c != 200 {
		t.Fatalf("sauvegarde : %d", c)
	}
}

// Une erreur imprévue ne renvoie pas son détail (chemins, requêtes) : il reste dans le journal.
func TestErreurInterneSansDetail(t *testing.T) {
	b := nouveau(t)
	b.s.store.Fermer() // toute lecture échoue désormais
	c, v := b.local("GET", "/api/entites", nil)
	if c != 500 || v["detail"] != msgInterne {
		t.Fatalf("erreur interne : %d %v", c, v)
	}
}
