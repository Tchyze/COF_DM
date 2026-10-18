package serveur

import (
	"archive/zip"
	"bytes"
	"fmt"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http/httptest"
	"strings"
	"testing"
)

// zipObsidian télécharge l'export Obsidian et renvoie ses fichiers, par chemin.
func zipObsidian(t *testing.T, b *banc, url string) map[string]string {
	t.Helper()
	r := httptest.NewRequest("GET", url, nil)
	r.Host, r.RemoteAddr = "127.0.0.1:8100", "127.0.0.1:5000"
	w := httptest.NewRecorder()
	b.s.ServeHTTP(w, r)
	if w.Code != 200 || w.Header().Get("Content-Type") != "application/zip" {
		t.Fatalf("GET %s : %d %s", url, w.Code, w.Body.String())
	}
	z, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, f := range z.File {
		rc, _ := f.Open()
		c, _ := io.ReadAll(rc)
		rc.Close()
		out[f.Name] = string(c)
	}
	return out
}

// L'export Obsidian du MJ contient tout ; celui des joueurs, rien de ce que la vue des joueurs cache.
func TestExportObsidian(t *testing.T) {
	b := nouveau(t)
	creer := func(typ, nom string, data map[string]any, partage bool) int64 {
		c, v := b.local("POST", "/api/entites", map[string]any{"type": typ, "nom": nom, "data": data})
		if c != 201 {
			t.Fatalf("création de %s : %d %v", nom, c, v)
		}
		id := int64(v["id"].(float64))
		if partage {
			if c, v := b.local("POST", fmt.Sprintf("/api/entites/%d/partage", id), map[string]any{"partage": true}); c != 200 {
				t.Fatalf("partage de %s : %d %v", nom, c, v)
			}
		}
		return id
	}
	cache := creer("pnj", "Le Traître", map[string]any{"resume": "CACHE-MJ"}, false)
	lieu := creer("lieu", "Fort Boueux", map[string]any{"impression": "Une palissade de boue."}, true)
	gautier := creer("pnj", "Gautier", map[string]any{
		"resume": "Aubergiste, ami de [[Fort Boueux]] et de [[Le Traître]].", "secrets": "SECRET-MJ : il empoisonne ses clients.",
		"lieu": float64(lieu), "responsable_secret": float64(cache)}, true)
	creer("secret", "La vérité sur Gautier", map[string]any{"resume": "SECRET-MJ"}, false)
	for _, rel := range []map[string]any{
		{"a": gautier, "b": lieu, "type": "habite", "note": "NOTE-MJ"},
		{"a": gautier, "b": cache, "type": "allie"},
		{"a": gautier, "b": lieu, "type": "possede", "secrete": true},
	} {
		if c, v := b.local("POST", "/api/relations", rel); c != 201 {
			t.Fatalf("relation : %d %v", c, v)
		}
	}
	for _, id := range []int64{gautier, cache} {
		var img, corps bytes.Buffer
		png.Encode(&img, image.NewRGBA(image.Rect(0, 0, 2, 2)))
		mw := multipart.NewWriter(&corps)
		fw, _ := mw.CreateFormFile("fichier", "portrait.png")
		fw.Write(img.Bytes())
		mw.Close()
		r := httptest.NewRequest("POST", fmt.Sprintf("/api/entites/%d/medias", id), &corps)
		r.Host, r.RemoteAddr = "127.0.0.1:8100", "127.0.0.1:5000"
		r.Header.Set("X-COF-DM", "1")
		r.Header.Set("Content-Type", mw.FormDataContentType())
		w := httptest.NewRecorder()
		b.s.ServeHTTP(w, r)
		if w.Code != 201 {
			t.Fatalf("image : %d %s", w.Code, w.Body.String())
		}
	}

	tout := func(f map[string]string) string {
		var s strings.Builder
		for k, v := range f {
			s.WriteString(k + "\n" + v + "\n")
		}
		return s.String()
	}
	mj := zipObsidian(t, b, "/api/export/obsidian")
	gautierMJ := mj["t/02 - PNJ/Gautier.md"]
	for _, attendu := range []string{"SECRET-MJ", `responsable_secret: "[[Le Traître]]"`, "NOTE-MJ", "*secret*", "[[Le Traître]]", "![[Gautier - 1.png]]", "partagé: true"} {
		if !strings.Contains(gautierMJ, attendu) {
			t.Fatalf("export du MJ, %q manque :\n%s", attendu, gautierMJ)
		}
	}
	if mj["t/06 - Secrets/La vérité sur Gautier.md"] == "" || mj["t/Journal.md"] == "" || mj["t/Images/Le Traître - 1.png"] == "" {
		t.Fatalf("export du MJ incomplet : %v", cles(mj))
	}

	joueurs := zipObsidian(t, b, "/api/export/obsidian?secrets=0")
	texte := tout(joueurs)
	for _, interdit := range []string{"SECRET-MJ", "CACHE-MJ", "NOTE-MJ", "responsable_secret", "vérité", "Traître - 1", "Journal", "*secret*", "possède", "allié", "[[Le Traître]]", "partagé"} {
		if strings.Contains(texte, interdit) {
			t.Fatalf("fuite de %q dans l'export des joueurs :\n%s", interdit, texte)
		}
	}
	g := joueurs["t (joueurs)/02 - PNJ/Gautier.md"]
	for _, attendu := range []string{"> Aubergiste, ami de [[Fort Boueux]] et de Le Traître.", `lieu: "[[Fort Boueux]]"`, "| [[Fort Boueux]] | habite à |", "![[Gautier - 1.png]]"} {
		if !strings.Contains(g, attendu) {
			t.Fatalf("export des joueurs, %q manque :\n%s", attendu, g)
		}
	}
	if len(joueurs) != 3 { // Gautier, Fort Boueux, l'image de Gautier
		t.Fatalf("export des joueurs : %v", cles(joueurs))
	}
}

func cles(m map[string]string) []string {
	var l []string
	for k := range m {
		l = append(l, k)
	}
	return l
}
