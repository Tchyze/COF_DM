package serveur

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Tchyze/cof_dm/internal/comptes"
	"github.com/Tchyze/cof_dm/internal/store"
)

// deguise : le serveur a, vu du bureau, la version et les numéros donnés (sante : la réponse de
// GET /api/sante, nil pour ceux du vrai serveur ; sans « protocole », un serveur d'avant les
// numéros, qui ne les publie pas non plus dans GET /api/mes-univers). Le reste passe au vrai serveur.
type deguise struct {
	vrai  http.Handler
	sante map[string]any
}

func (d *deguise) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" || d.sante == nil || (r.URL.Path != "/api/sante" && r.URL.Path != "/api/mes-univers") {
		d.vrai.ServeHTTP(w, r)
		return
	}
	out := d.sante
	if r.URL.Path == "/api/mes-univers" {
		rec := httptest.NewRecorder()
		d.vrai.ServeHTTP(rec, r)
		if rec.Code != 200 {
			w.WriteHeader(rec.Code)
			_, _ = w.Write(rec.Body.Bytes())
			return
		}
		out = map[string]any{}
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		delete(out, "version")
		delete(out, "protocole")
		delete(out, "modele")
		if _, ok := d.sante["protocole"]; ok {
			for _, k := range []string{"version", "protocole", "modele"} {
				out[k] = d.sante[k]
			}
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func TestSanteVersions(t *testing.T) {
	b := nouveau(t)
	_, v := b.local("GET", "/api/sante", nil)
	if v["protocole"] != float64(store.VersionProtocole) || v["modele"] != float64(store.VersionModele) {
		t.Fatalf("santé : %v", v)
	}
	_, _, connecter := serveurMulti(t)
	_, v = connecter("meneur").faire("GET", "/api/mes-univers", nil)
	if v["protocole"] != float64(store.VersionProtocole) || v["modele"] != float64(store.VersionModele) || v["version"] == nil {
		t.Fatalf("mes univers : %v", v)
	}
}

// Avant de tirer une copie ou d'envoyer un univers, le bureau compare ses numéros à ceux du serveur ;
// le serveur note ceux de chaque copie.
func TestBureauVersionsServeur(t *testing.T) {
	a, reg, connecter := serveurMulti(t)
	univers := a.st.ID()
	d := &deguise{vrai: a}
	ts := httptest.NewServer(d)
	defer ts.Close()
	connecter("chef") // administrateur
	connecter("meneur")
	reg.Modifier("meneur", func(c *comptes.Compte) error { c.Roles[univers] = comptes.MJ; return nil })
	b := nouvelleAppli(t, t.TempDir())
	b.local("POST", "/api/univers/serveur/connexion", map[string]any{"url": ts.URL, "nom": "meneur", "mdp": "mot-de-passe-meneur"})

	// serveur plus récent : pas de copie, rien d'écrit ici
	d.sante = map[string]any{"ok": true, "version": "9.0.0", "protocole": store.VersionProtocole, "modele": store.VersionModele + 1}
	c, v := b.local("POST", "/api/univers/serveur/relier", map[string]any{"univers": univers, "copie": "Portable"})
	if c != 409 || !strings.Contains(v["detail"].(string), "9.0.0, plus récent") {
		t.Fatalf("copie d'un serveur plus récent : %d %v", c, v)
	}
	if b.courant()["id"] == univers || len(a.st.Copies()) != 0 {
		t.Fatal("rien ne doit être copié")
	}
	// protocole plus récent : de même
	d.sante = map[string]any{"ok": true, "version": "9.0.0", "protocole": store.VersionProtocole + 1, "modele": store.VersionModele}
	if c, _ := b.local("POST", "/api/univers/serveur/relier", map[string]any{"univers": univers}); c != 409 {
		t.Fatalf("protocole plus récent : %d", c)
	}

	// serveur plus ancien (une version d'avant les numéros) : la copie se tire, l'envoi est refusé
	d.sante = map[string]any{"ok": true, "version": "1.2.1"}
	c, v = b.local("POST", "/api/univers/serveur/relier", map[string]any{"univers": univers, "copie": "Portable"})
	if c != 200 || v["courant"].(map[string]any)["id"] != univers {
		t.Fatalf("copie d'un serveur plus ancien : %d %v", c, v)
	}
	copies := a.st.Copies()
	if len(copies) != 1 || copies[0].Modele != store.VersionModele || copies[0].Protocole != store.VersionProtocole || copies[0].Version == "" {
		t.Fatalf("numéros de la copie notés par le serveur : %+v", copies)
	}
	b.local("POST", "/api/univers/nouveau", map[string]any{"nom": "Local"})
	b.local("POST", "/api/univers/serveur/connexion", map[string]any{"url": ts.URL, "nom": "chef", "mdp": "mot-de-passe-chef"})
	c, v = b.local("POST", "/api/univers/serveur/envoyer", map[string]any{"nom": "Local"})
	if c != 409 || !strings.Contains(v["detail"].(string), "1.2.1, plus ancien") || len(a.tous) != 1 {
		t.Fatalf("envoi à un serveur plus ancien : %d %v", c, v)
	}
	// au même modèle : l'envoi passe
	d.sante = nil
	if c, v := b.local("POST", "/api/univers/serveur/envoyer", map[string]any{"nom": "Local"}); c != 200 || len(a.tous) != 2 {
		t.Fatalf("envoi : %d %v", c, v)
	}
}
