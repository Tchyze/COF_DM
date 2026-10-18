package serveur

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/Tchyze/cof_dm/internal/comptes"
	"github.com/Tchyze/cof_dm/internal/store"
)

// Une copie se relie au serveur, travaille hors ligne, puis échange : par HTTP, avec les rôles.
func TestSynchronisationHTTP(t *testing.T) {
	a, reg, connecter := serveurMulti(t)
	univers := a.st.ID()
	meneur, lecteur, joueuse := connecter("meneur"), connecter("lecteur"), connecter("joueuse")
	reg.Modifier("meneur", func(c *comptes.Compte) error { c.Roles[univers] = comptes.MJ; return nil })
	reg.Modifier("lecteur", func(c *comptes.Compte) error { c.Roles[univers] = comptes.Auditeur; return nil })
	reg.Modifier("joueuse", func(c *comptes.Compte) error { c.Roles[univers] = comptes.Joueur; return nil })
	c, v := meneur.faire("POST", "/api/entites", map[string]any{"type": "lieu", "nom": "Le Gué"})
	if c != 201 {
		t.Fatalf("fiche : %d %v", c, v)
	}
	gue := int64(v["id"].(float64))

	for _, n := range []*navigateur{lecteur, joueuse} {
		if c, _ := n.faire("POST", "/api/sync/copie", map[string]any{"nom": "Portable"}); c != 403 {
			t.Fatalf("copie demandée sans droit d'écrire : %d", c)
		}
	}
	w := meneur.brut("POST", "/api/sync/copie", map[string]any{"nom": "Portable de Marion"})
	if w.Code != 200 || w.Header().Get("X-COF-Replique") != "1" || w.Header().Get("X-COF-Univers") != univers {
		t.Fatalf("copie : %d %v", w.Code, w.Header())
	}
	fichier := filepath.Join(t.TempDir(), "copie.cofdm")
	if err := os.WriteFile(fichier, w.Body.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	cp, err := store.Ouvrir(fichier)
	if err != nil {
		t.Fatal(err)
	}
	defer cp.Fermer()
	if cp.Replique() != 1 || cp.ID() != univers {
		t.Fatalf("copie : n°%d, univers %s", cp.Replique(), cp.ID())
	}
	if c, v := meneur.faire("GET", "/api/sync/copies", nil); c != 200 || len(v["copies"].([]any)) != 1 {
		t.Fatalf("copies inscrites : %d %v", c, v)
	}

	// hors ligne : une fiche et une image sur la copie ; sur le serveur, un champ
	tour, _ := cp.Creer("lieu", "La Tour", nil, nil, "meneur")
	image := []byte("\x89PNG\r\n\x1a\n" + string(make([]byte, 64)))
	m, err := cp.AjouterMedia(tour.ID, "abcdefabcdefabcdef01.png", "tour.png", image, "meneur")
	if err != nil {
		t.Fatal(err)
	}
	meneur.faire("PATCH", "/api/entites/"+strconv.FormatInt(gue, 10), map[string]any{"data": map[string]any{"rumeurs": "Un loup rôde."}})

	echanger := func(n *navigateur) (int, store.Reponse) {
		curseur, chs, err := cp.Preparer()
		if err != nil {
			t.Fatal(err)
		}
		w := n.brut("POST", "/api/sync", map[string]any{"replique": cp.Replique(), "curseur": curseur, "changements": chs})
		var rep store.Reponse
		_ = json.Unmarshal(w.Body.Bytes(), &rep)
		if w.Code == 200 {
			if err := cp.Integrer(rep, store.EnvoyeJusqua(chs, 0)); err != nil {
				t.Fatal(err)
			}
		}
		return w.Code, rep
	}
	if c, _ := echanger(lecteur); c != 403 {
		t.Fatalf("échange par l'auditeur : %d", c)
	}
	c, rep := echanger(meneur)
	if c != 200 || len(rep.ImagesAttendues) != 1 {
		t.Fatalf("échange : %d %+v", c, rep)
	}
	if e, _ := cp.Lire(gue); e.Data["rumeurs"] != "Un loup rôde." {
		t.Fatalf("champ du serveur non reçu : %#v", e.Data)
	}
	if c, v := meneur.faire("GET", "/api/entites/"+strconv.FormatInt(tour.ID, 10), nil); c != 200 || v["nom"] != "La Tour" {
		t.Fatalf("fiche de la copie sur le serveur : %d %v", c, v)
	}
	// le contenu de l'image : refusé s'il ne correspond pas à son format, accepté sinon
	if c, _ := meneur.faire("PUT", "/api/sync/images/"+m.Fichier, []byte("pas une image")); c != 415 {
		t.Fatalf("contenu d'image invalide : %d", c)
	}
	if c, _ := meneur.faire("PUT", "/api/sync/images/"+m.Fichier, image); c != 204 {
		t.Fatalf("contenu d'image : %d", c)
	}
	if w := meneur.brut("GET", "/medias/"+m.Fichier, nil); w.Code != 200 || w.Body.Len() != len(image) {
		t.Fatalf("image servie : %d", w.Code)
	}
	// une copie inconnue est renvoyée à l'inscription
	if c, _ := meneur.faire("POST", "/api/sync", map[string]any{"replique": 42, "curseur": 0, "changements": []any{}}); c != 409 {
		t.Fatalf("copie inconnue : %d", c)
	}
}

// Sur l'application de bureau, pas de synchronisation à servir.
func TestSynchronisationBureau(t *testing.T) {
	b := nouveau(t)
	if c, _ := b.local("POST", "/api/sync/copie", map[string]any{"nom": "x"}); c != 404 {
		t.Fatalf("copie demandée au bureau : %d", c)
	}
}
