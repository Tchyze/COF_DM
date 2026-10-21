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

// L'échange selon les numéros : serveur plus récent, rien ne part ni n'arrive ; serveur plus
// ancien, le bureau reçoit seulement ; de nouveau au même modèle, ce qui attendait part.
func TestBureauVersionsEchange(t *testing.T) {
	a, reg, connecter := serveurMulti(t)
	univers := a.st.ID()
	d := &deguise{vrai: a}
	ts := httptest.NewServer(d)
	defer ts.Close()
	meneur := connecter("meneur")
	reg.Modifier("meneur", func(c *comptes.Compte) error { c.Roles[univers] = comptes.MJ; return nil })
	b := nouvelleAppli(t, t.TempDir())
	b.local("POST", "/api/univers/serveur/connexion", map[string]any{"url": ts.URL, "nom": "meneur", "mdp": "mot-de-passe-meneur"})
	if c, v := b.local("POST", "/api/univers/serveur/relier", map[string]any{"univers": univers, "copie": "Portable"}); c != 200 {
		t.Fatalf("relier : %d %v", c, v)
	}
	lieux := func() float64 {
		_, v := meneur.faire("GET", "/api/stats", nil)
		n, _ := v["compte"].(map[string]any)["lieu"].(float64)
		return n
	}

	// serveur plus récent : l'échange attend la mise à jour du bureau
	d.sante = map[string]any{"ok": true, "version": "9.0.0", "protocole": store.VersionProtocole, "modele": store.VersionModele + 1}
	b.local("POST", "/api/entites", map[string]any{"type": "lieu", "nom": "La Tour"})
	meneur.faire("POST", "/api/entites", map[string]any{"type": "lieu", "nom": "Le Gué"})
	c, v := b.local("POST", "/api/univers/serveur/synchroniser", nil)
	if c != 200 || v["etat"] != etatVersion || v["ecart"] != ecartServeurRecent || v["version_serveur"] != "9.0.0" ||
		!strings.Contains(v["erreur"].(string), "mettez-la à jour") || v["bilan"].(map[string]any)["en_attente"] != float64(1) {
		t.Fatalf("serveur plus récent : %d %v", c, v)
	}
	if lieux() != 1 || b.fiches() != 1 {
		t.Fatalf("rien ne doit s'échanger : %v sur le serveur, %d ici", lieux(), b.fiches())
	}
	if _, v := b.local("GET", "/api/univers/serveur", nil); v["synchro"].(map[string]any)["ecart"] != ecartServeurRecent {
		t.Fatalf("état du lien : %v", v)
	}

	// serveur plus ancien : réception seule
	d.sante = map[string]any{"ok": true, "version": "1.2.1"}
	c, v = b.local("POST", "/api/univers/serveur/synchroniser", nil)
	bilan := v["bilan"].(map[string]any)
	if c != 200 || v["etat"] != etatAJour || v["ecart"] != ecartBureauRecent || bilan["en_attente"] != float64(1) ||
		bilan["envoyes"] != float64(0) || bilan["recus"] == float64(0) || !strings.Contains(v["message"].(string), "plus ancien") {
		t.Fatalf("serveur plus ancien : %d %v", c, v)
	}
	if lieux() != 1 || b.fiches() != 2 {
		t.Fatalf("réception seule : %v sur le serveur, %d ici", lieux(), b.fiches())
	}

	// de nouveau au même modèle : ce qui attendait part
	d.sante = nil
	c, v = b.local("POST", "/api/univers/serveur/synchroniser", nil)
	if c != 200 || v["etat"] != etatAJour || v["ecart"] != nil || v["bilan"].(map[string]any)["envoyes"] == float64(0) {
		t.Fatalf("même modèle : %d %v", c, v)
	}
	if lieux() != 2 {
		t.Fatalf("la fiche du bureau doit être arrivée : %v", lieux())
	}
}

// Le serveur juge aussi les numéros annoncés par le bureau : un bureau plus ancien (426) ne reçoit
// rien ; un plus récent ne lui envoie rien qu'il ne connaîtrait pas ; un bureau d'avant les numéros
// est pris pour la 1.2.2.
func TestServeurVersionsCopie(t *testing.T) {
	a, reg, connecter := serveurMulti(t)
	univers := a.st.ID()
	meneur := connecter("meneur")
	reg.Modifier("meneur", func(c *comptes.Compte) error { c.Roles[univers] = comptes.MJ; return nil })
	ancien := []string{"X-COF-Version", "1.2.0", "X-COF-Protocole", "1", "X-COF-Modele", "0"}
	if w := meneur.brut("POST", "/api/sync/copie", map[string]any{"nom": "Vieux"}, ancien...); w.Code != 426 || !strings.Contains(w.Body.String(), "1.2.0) est plus ancienne") {
		t.Fatalf("copie pour un bureau plus ancien : %d %s", w.Code, w.Body.String())
	}
	if w := meneur.brut("POST", "/api/sync/copie", map[string]any{"nom": "Portable"}); w.Code != 200 {
		t.Fatalf("copie d'un bureau d'avant les numéros : %d", w.Code)
	}
	if c, v := meneur.faire("POST", "/api/sync", map[string]any{"replique": 1, "curseur": 0, "changements": []any{}}, ancien...); c != 426 {
		t.Fatalf("échange d'un bureau plus ancien : %d %v", c, v)
	}
	recent := []string{"X-COF-Version", "9.0.0", "X-COF-Protocole", "1", "X-COF-Modele", "99"}
	ch := []map[string]any{{"objet": "entite", "cle": "1099511627781", "champ": "*", "valeur": true}}
	if c, v := meneur.faire("POST", "/api/sync", map[string]any{"replique": 1, "curseur": 0, "changements": ch}, recent...); c != 409 {
		t.Fatalf("changements d'un bureau plus récent : %d %v", c, v)
	}
	if c, v := meneur.faire("POST", "/api/sync", map[string]any{"replique": 1, "curseur": 0, "changements": []any{}}, recent...); c != 200 {
		t.Fatalf("réception d'un bureau plus récent : %d %v", c, v)
	}
	copies := a.st.Copies()
	if len(copies) != 1 || copies[0].Version != "9.0.0" || copies[0].Modele != 99 {
		t.Fatalf("numéros notés : %+v", copies)
	}
}
