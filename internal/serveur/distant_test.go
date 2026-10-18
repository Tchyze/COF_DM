package serveur

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Tchyze/cof_dm/internal/comptes"
	"github.com/Tchyze/cof_dm/internal/reglages"
)

// L'application de bureau se relie à un serveur, tire une copie d'un univers, la tient à jour,
// signale ses conflits ; hors ligne, elle le dit.
func TestBureauRelie(t *testing.T) {
	a, reg, connecter := serveurMulti(t)
	univers := a.st.ID()
	ts := httptest.NewServer(a)
	defer ts.Close()
	meneur := connecter("meneur")
	reg.Modifier("meneur", func(c *comptes.Compte) error { c.Roles[univers] = comptes.MJ; return nil })
	_, v := meneur.faire("POST", "/api/entites", map[string]any{"type": "lieu", "nom": "Le Gué"})
	gue := strconv.Itoa(int(v["id"].(float64)))

	b := nouvelleAppli(t, t.TempDir())
	if c, v := b.local("POST", "/api/univers/serveur/relier", map[string]any{"univers": univers}); c != 409 {
		t.Fatalf("relier sans connexion : %d %v", c, v)
	}
	if c, v := b.local("POST", "/api/univers/serveur/connexion", map[string]any{"url": ts.URL, "nom": "meneur", "mdp": "faux"}); c != 422 {
		t.Fatalf("mauvais mot de passe : %d %v", c, v)
	}
	if c, v := b.local("POST", "/api/univers/serveur/connexion", map[string]any{"url": "http://jdr.example.org", "nom": "meneur", "mdp": "x"}); c != 422 || !strings.Contains(v["detail"].(string), "HTTPS") {
		t.Fatalf("HTTP sur Internet : %d %v", c, v)
	}
	c, v := b.local("POST", "/api/univers/serveur/connexion", map[string]any{"url": ts.URL, "nom": "meneur", "mdp": "mot-de-passe-meneur"})
	if c != 200 || len(v["univers"].([]any)) != 1 {
		t.Fatalf("connexion : %d %v", c, v)
	}
	// seul le jeton est gardé, dans les réglages de l'ordinateur, jamais dans l'univers
	c, v = b.local("POST", "/api/univers/serveur/relier", map[string]any{"univers": univers, "copie": "Portable"})
	if c != 200 || v["courant"].(map[string]any)["id"] != univers {
		t.Fatalf("relier : %d %v", c, v)
	}
	if r, _ := os.ReadFile(filepath.Join(b.dir, "reglages.json")); strings.Contains(string(r), "mot-de-passe") || !strings.Contains(string(r), "cofdm_") {
		t.Fatalf("réglages : %s", r)
	}
	if b.fiches() != 1 {
		t.Fatalf("copie : %d fiches", b.fiches())
	}
	if c, v := b.local("GET", "/api/univers/serveur", nil); c != 200 || v["relie"] != true || v["url"] != ts.URL {
		t.Fatalf("état du lien : %d %v", c, v)
	}

	// des deux côtés, puis un échange
	b.local("POST", "/api/entites", map[string]any{"type": "lieu", "nom": "La Tour"})
	b.local("PATCH", "/api/entites/"+gue, map[string]any{"data": map[string]any{"rumeurs": "Version du portable."}})
	meneur.faire("PATCH", "/api/entites/"+gue, map[string]any{"data": map[string]any{"rumeurs": "Version du serveur."}})
	c, v = b.local("POST", "/api/univers/serveur/synchroniser", nil)
	if c != 200 || v["etat"] != etatAJour || v["bilan"].(map[string]any)["conflits"] != float64(1) {
		t.Fatalf("échange : %d %v", c, v)
	}
	if _, v := meneur.faire("GET", "/api/stats", nil); v["compte"].(map[string]any)["lieu"] != float64(2) {
		t.Fatalf("fiche du portable sur le serveur : %v", v)
	}
	// le conflit attend sur le portable ; garder la version locale la renvoie
	_, v = b.local("GET", "/api/conflits", nil)
	l := v["conflits"].([]any)
	if len(l) != 1 {
		t.Fatalf("conflits : %v", v)
	}
	id := strconv.Itoa(int(l[0].(map[string]any)["id"].(float64)))
	if c, v := b.local("POST", "/api/conflits/"+id, map[string]any{"garder": "local"}); c != 200 || len(v["conflits"].([]any)) != 0 {
		t.Fatalf("résolution : %d %v", c, v)
	}
	b.local("POST", "/api/univers/serveur/synchroniser", nil)
	if _, v := meneur.faire("GET", "/api/entites/"+gue, nil); v["data"].(map[string]any)["rumeurs"] != "Version du portable." {
		t.Fatalf("valeur gardée : %v", v["data"])
	}

	// hors ligne : l'état le dit, rien n'est perdu
	ts.Close()
	b.local("POST", "/api/entites", map[string]any{"type": "lieu", "nom": "Le Moulin"})
	if c, v := b.local("POST", "/api/univers/serveur/synchroniser", nil); c != 200 || v["etat"] != etatHorsLigne {
		t.Fatalf("hors ligne : %d %v", c, v)
	}
	// délier : la copie reste, elle ne se synchronise plus
	if c, _ := b.local("POST", "/api/univers/serveur/delier", nil); c != 200 {
		t.Fatalf("délier : %d", c)
	}
	if c, _ := b.local("POST", "/api/univers/serveur/synchroniser", nil); c != 409 {
		t.Fatalf("synchroniser après délier : %d", c)
	}
}

// « Envoyer sur le serveur » : l'univers local y est publié (administrateur), puis remplacé ici par
// sa copie reliée ; l'original reste dans les sauvegardes.
func TestBureauEnvoie(t *testing.T) {
	a, reg, connecter := serveurMulti(t)
	ts := httptest.NewServer(a)
	defer ts.Close()
	connecter("chef") // administrateur
	connecter("meneur")
	reg.Modifier("meneur", func(c *comptes.Compte) error { c.Roles[a.st.ID()] = comptes.MJ; return nil })

	b := nouvelleAppli(t, t.TempDir())
	b.local("POST", "/api/entites", map[string]any{"type": "lieu", "nom": "Fort Boueux"})
	local := b.courant()
	if c, _ := b.local("POST", "/api/univers/serveur/envoyer", map[string]any{"nom": "Les Marches"}); c != 409 {
		t.Fatalf("envoyer sans connexion : %d", c)
	}
	// un MJ qui n'est pas administrateur ne publie pas d'univers
	b.local("POST", "/api/univers/serveur/connexion", map[string]any{"url": ts.URL, "nom": "meneur", "mdp": "mot-de-passe-meneur"})
	if c, v := b.local("POST", "/api/univers/serveur/envoyer", map[string]any{"nom": "Les Marches"}); c != 422 {
		t.Fatalf("envoi par un MJ : %d %v", c, v)
	}
	b.local("POST", "/api/univers/serveur/connexion", map[string]any{"url": ts.URL, "nom": "chef", "mdp": "mot-de-passe-chef"})
	c, v := b.local("POST", "/api/univers/serveur/envoyer", map[string]any{"nom": "Les Marches"})
	if c != 200 {
		t.Fatalf("envoi : %d %v", c, v)
	}
	courant := v["courant"].(map[string]any)
	if courant["id"] != local["id"] || courant["chemin"] != local["chemin"] || len(a.tous) != 2 {
		t.Fatalf("après envoi : %v (avant : %v), %d univers sur le serveur", courant, local, len(a.tous))
	}
	if c, v := b.local("GET", "/api/univers/serveur", nil); c != 200 || v["relie"] != true || v["copie"] != true {
		t.Fatalf("lien après envoi : %d %v", c, v)
	}
	if n, _, _ := a.tous[local["id"].(string)].st.Compter(); n["lieu"] != 1 {
		t.Fatalf("univers publié : %v", n)
	}
	sauv, _ := filepath.Glob(filepath.Join(b.dir, "sauvegardes", "*", "*_"+motifAvantEnvoi+".cofdm"))
	if len(sauv) != 1 {
		t.Fatalf("sauvegarde d'avant l'envoi : %v", sauv)
	}
	// le même univers ne s'envoie pas deux fois
	b.local("POST", "/api/univers/serveur/connexion", map[string]any{"url": ts.URL, "nom": "chef", "mdp": "mot-de-passe-chef"})
	if c, v := b.local("POST", "/api/univers/serveur/envoyer", map[string]any{"nom": "Encore"}); c != 409 {
		t.Fatalf("second envoi : %d %v", c, v)
	}
	// et l'échange marche aussitôt
	b.local("POST", "/api/entites", map[string]any{"type": "lieu", "nom": "Gué Noir"})
	if c, v := b.local("POST", "/api/univers/serveur/synchroniser", nil); c != 200 || v["etat"] != etatAJour {
		t.Fatalf("échange après envoi : %d %v", c, v)
	}
	if n, _, _ := a.tous[local["id"].(string)].st.Compter(); n["lieu"] != 2 {
		t.Fatalf("échange après envoi : %v", n)
	}
}

// jetonsDe : le nombre de jetons d'API du compte sur le serveur.
func jetonsDe(t *testing.T, reg *comptes.Registre, nom string) int {
	t.Helper()
	c, _ := reg.Lire(nom)
	return len(c.Jetons)
}

// Les deux modes de l'application de bureau : local (elle peut s'ouvrir au réseau) ou reliée à un
// serveur (le réseau local est fermé). Quitter le serveur rend son jeton et met les copies en pause ;
// s'y reconnecter les reprend. Changer de serveur met en pause les copies de l'ancien.
func TestBureauModes(t *testing.T) {
	a, reg, connecter := serveurMulti(t)
	univers := a.st.ID()
	ts := httptest.NewServer(a)
	defer ts.Close()
	connecter("meneur")
	reg.Modifier("meneur", func(c *comptes.Compte) error { c.Roles[univers] = comptes.MJ; return nil })

	b := nouvelleAppli(t, t.TempDir()) // elle écoute sur le réseau local
	if c, v := b.local("GET", "/api/univers/serveur", nil); c != 200 || v["mode"] != modeLocal || v["serveur"] != nil {
		t.Fatalf("mode au départ : %d %v", c, v)
	}
	if c, _ := b.appel(true, "GET", "/api/stats", nil); c != 200 {
		t.Fatalf("réseau en mode local : %d", c)
	}
	if c, _ := b.local("GET", "/api/univers/serveur/univers", nil); c != 409 {
		t.Fatalf("univers du serveur sans connexion : %d", c)
	}

	// connexion : mode serveur, réseau local fermé, ni tablette ni page des joueurs
	c, v := b.local("POST", "/api/univers/serveur/connexion", map[string]any{"url": ts.URL, "nom": "meneur", "mdp": "mot-de-passe-meneur"})
	if c != 200 || v["mode"] != modeServeurDistant || v["serveur"].(map[string]any)["etat"] != etatConnecte || len(v["univers"].([]any)) != 1 {
		t.Fatalf("connexion : %d %v", c, v)
	}
	if n := jetonsDe(t, reg, "meneur"); n != 1 {
		t.Fatalf("jetons après connexion : %d", n)
	}
	if c, v := b.appel(true, "GET", "/api/stats", nil); c != 403 || !strings.Contains(v["detail"].(string), "mode local") {
		t.Fatalf("réseau en mode serveur : %d %v", c, v)
	}
	if c, _ := b.local("POST", "/api/acces/reseau", map[string]any{"reseau": true}); c != 409 {
		t.Fatalf("ouverture au réseau en mode serveur : %d", c)
	}
	if c, _ := b.local("POST", "/api/acces/joueurs", map[string]any{"active": true}); c != 409 {
		t.Fatalf("page des joueurs en mode serveur : %d", c)
	}
	if _, v := b.local("GET", "/api/acces", nil); v["relie"] != true {
		t.Fatalf("état de l'accès : %v", v)
	}
	// un univers local, en mode serveur : rien à synchroniser, mais le serveur répond
	if _, v := b.local("GET", "/api/univers/serveur", nil); v["relie"] != false || v["mode"] != modeServeurDistant {
		t.Fatalf("univers local en mode serveur : %v", v)
	}

	// copie d'un univers du serveur, signalée ensuite comme déjà là
	if c, v := b.local("POST", "/api/univers/serveur/relier", map[string]any{"univers": univers}); c != 200 {
		t.Fatalf("relier : %d %v", c, v)
	}
	_, v = b.local("GET", "/api/univers/serveur/univers", nil)
	if u := v["univers"].([]any)[0].(map[string]any); u["ici"] == nil || u["ici"] == "" {
		t.Fatalf("univers déjà copié : %v", v)
	}
	if _, v := b.local("GET", "/api/univers/serveur", nil); v["actif"] != true || v["pause"] != false {
		t.Fatalf("copie active : %v", v)
	}

	// client seul : le jeton est rendu, la copie en pause, le réseau rouvert
	b.local("POST", "/api/entites", map[string]any{"type": "lieu", "nom": "Écrit hors connexion"})
	if c, v := b.local("POST", "/api/univers/serveur/local", nil); c != 200 || v["mode"] != modeLocal || v["pause"] != true || v["avertissement"] != nil {
		t.Fatalf("passage en local : %d %v", c, v)
	}
	if n := jetonsDe(t, reg, "meneur"); n != 0 {
		t.Fatalf("jeton non rendu : %d", n)
	}
	if c, _ := b.local("POST", "/api/univers/serveur/synchroniser", nil); c != 409 {
		t.Fatalf("échange d'une copie en pause : %d", c)
	}
	if c, _ := b.appel(true, "GET", "/api/stats", nil); c != 200 {
		t.Fatalf("réseau après le retour en local : %d", c)
	}
	if r, _ := os.ReadFile(filepath.Join(b.dir, "reglages.json")); strings.Contains(string(r), "cofdm_") {
		t.Fatalf("jeton resté dans les réglages : %s", r)
	}

	// retour sur le même serveur : la copie reprend, avec ce qui a été écrit entre-temps
	b.local("POST", "/api/univers/serveur/connexion", map[string]any{"url": ts.URL, "nom": "meneur", "mdp": "mot-de-passe-meneur"})
	if c, v := b.local("POST", "/api/univers/serveur/synchroniser", nil); c != 200 || v["etat"] != etatAJour {
		t.Fatalf("reprise : %d %v", c, v)
	}
	if n, _, _ := a.st.Compter(); n["lieu"] != 1 {
		t.Fatalf("écrit hors connexion, arrivé sur le serveur : %v", n)
	}

	// se reconnecter remplace le jeton au lieu d'en ajouter
	b.local("POST", "/api/univers/serveur/connexion", map[string]any{"url": ts.URL, "nom": "meneur", "mdp": "mot-de-passe-meneur"})
	if n := jetonsDe(t, reg, "meneur"); n != 1 {
		t.Fatalf("jetons après reconnexion : %d", n)
	}

	// autre serveur : la copie du premier passe en pause, le jeton du premier est rendu
	a2, reg2, connecter2 := serveurMulti(t)
	ts2 := httptest.NewServer(a2)
	defer ts2.Close()
	connecter2("meneur")
	reg2.Modifier("meneur", func(c *comptes.Compte) error { c.Roles[a2.st.ID()] = comptes.MJ; return nil })
	if c, v := b.local("POST", "/api/univers/serveur/connexion", map[string]any{"url": ts2.URL, "nom": "meneur", "mdp": "mot-de-passe-meneur"}); c != 200 || v["pause"] != true || v["serveur"].(map[string]any)["url"] != ts2.URL {
		t.Fatalf("changement de serveur : %d %v", c, v)
	}
	if n := jetonsDe(t, reg, "meneur"); n != 0 {
		t.Fatalf("jeton du premier serveur non rendu : %d", n)
	}

	// le serveur retire le jeton de ce poste : l'application le dit
	reg2.Modifier("meneur", func(c *comptes.Compte) error { c.Jetons = nil; return nil })
	if c, _ := b.local("GET", "/api/univers/serveur/univers", nil); c != 422 {
		t.Fatalf("jeton retiré : %d", c)
	}
	if _, v := b.local("GET", "/api/univers/serveur", nil); v["serveur"].(map[string]any)["etat"] != etatRefuse {
		t.Fatalf("état après retrait du jeton : %v", v)
	}

	// serveur injoignable en passant en local : le mode change quand même, avec un avertissement
	ts2.Close()
	if c, v := b.local("POST", "/api/univers/serveur/local", nil); c != 200 || v["mode"] != modeLocal || v["avertissement"] == nil {
		t.Fatalf("passage en local hors connexion : %d %v", c, v)
	}
}

// Les liens d'avant les modes portaient chacun leur jeton : l'application les reprend en mode
// serveur, avec une seule connexion.
func TestBureauRepriseLiens(t *testing.T) {
	dir := t.TempDir()
	ancien := `{"serveurs": "{\"u1\":{\"url\":\"https://jdr.example.org\",\"jeton\":\"cofdm_ancien\",\"compte\":\"meneur\",\"copie\":\"Portable\"}}"}`
	if err := os.WriteFile(filepath.Join(dir, "reglages.json"), []byte(ancien), 0o600); err != nil {
		t.Fatal(err)
	}
	if !BureauRelie(reglages.Ouvrir(filepath.Join(dir, "reglages.json"))) {
		t.Fatal("liens d'avant les modes : application non reliée")
	}
	b := nouvelleAppli(t, dir)
	c := b.a.cnx()
	if c == nil || c.URL != "https://jdr.example.org" || c.Jeton != "cofdm_ancien" || c.Compte != "meneur" {
		t.Fatalf("connexion reprise : %+v", c)
	}
	if l := b.a.liens()["u1"]; l.Jeton != "" || l.Copie != "Portable" {
		t.Fatalf("lien repris : %+v", l)
	}
	if !BureauRelie(b.a.reg) {
		t.Fatal("application non reliée après la reprise")
	}
}

// Un jeton d'API se retire lui-même, sans toucher aux autres jetons du compte ; une session de
// navigateur n'a pas de jeton à rendre.
func TestRendreJeton(t *testing.T) {
	_, reg, connecter := serveurMulti(t)
	meneur := connecter("meneur")
	if c, _ := meneur.faire("DELETE", "/api/compte/jeton", nil); c != 409 {
		t.Fatalf("sans jeton : %d", c)
	}
	_, v1 := meneur.faire("POST", "/api/compte/jetons", map[string]any{"nom": "Portable"})
	_, v2 := meneur.faire("POST", "/api/compte/jetons", map[string]any{"nom": "Claude"})
	poste := &navigateur{t: t, a: meneur.a, cookies: map[string]*http.Cookie{}, jeton: v1["jeton"].(string)}
	if c, _ := poste.faire("DELETE", "/api/compte/jeton", nil); c != 200 {
		t.Fatalf("jeton rendu : %d", c)
	}
	if c, _ := poste.faire("GET", "/api/mes-univers", nil); c != 401 {
		t.Fatalf("jeton rendu, encore accepté : %d", c)
	}
	if c, _ := reg.Lire("meneur"); len(c.Jetons) != 1 || c.Jetons[0].ID != v2["id"] {
		t.Fatalf("autres jetons : %+v", c.Jetons)
	}
}

// Relié à un serveur, le bureau passe d'un univers du serveur à l'autre : la copie est tirée la
// première fois, rouverte ensuite, même hors connexion.
func TestBureauUniversDuServeur(t *testing.T) {
	a, _, connecter := serveurMulti(t)
	premier := a.st.ID()
	chef := connecter("chef") // administrateur : MJ partout
	_, v := chef.faire("POST", "/api/univers/nouveau", map[string]any{"nom": "Les Marches"})
	second := v["courant"].(map[string]any)["id"].(string)
	ts := httptest.NewServer(a)
	defer ts.Close()

	b := nouvelleAppli(t, t.TempDir())
	if c, _ := b.local("POST", "/api/univers/serveur/ouvrir", map[string]any{"univers": premier}); c != 409 {
		t.Fatalf("ouvrir en mode local : %d", c)
	}
	b.local("POST", "/api/univers/serveur/connexion", map[string]any{"url": ts.URL, "nom": "chef", "mdp": "mot-de-passe-chef"})
	liste := func() map[string]map[string]any {
		t.Helper()
		c, v := b.local("GET", "/api/univers/serveur/univers", nil)
		if c != 200 {
			t.Fatalf("liste : %d %v", c, v)
		}
		m := map[string]map[string]any{}
		for _, u := range v["univers"].([]any) {
			m[u.(map[string]any)["id"].(string)] = u.(map[string]any)
		}
		return m
	}
	if l := liste(); len(l) != 2 || l[premier]["ici"] != nil || l[premier]["ouvert"] != false {
		t.Fatalf("univers du serveur : %v", l)
	}
	ouvrir := func(id string) {
		t.Helper()
		if c, v := b.local("POST", "/api/univers/serveur/ouvrir", map[string]any{"univers": id}); c != 200 || v["courant"].(map[string]any)["id"] != id {
			t.Fatalf("ouvrir %s : %d %v", id, c, v)
		}
	}
	fichiers := func() int {
		f, _ := filepath.Glob(filepath.Join(b.dir, "univers", "*.cofdm"))
		return len(f)
	}
	ouvrir(premier)
	b.local("POST", "/api/entites", map[string]any{"type": "lieu", "nom": "Le Gué"})
	ouvrir(second)
	avant := fichiers()
	ouvrir(premier) // la copie existe : rouverte, pas tirée de nouveau
	if fichiers() != avant || b.fiches() != 1 {
		t.Fatalf("copie rouverte : %d fichiers (avant %d), %d fiches", fichiers(), avant, b.fiches())
	}
	if l := liste(); l[premier]["ouvert"] != true || l[premier]["ici"] == nil || l[second]["ici"] == nil {
		t.Fatalf("après ouverture : %v", l)
	}
	if l := b.a.liens()[second]; l.Nom != "Les Marches" || l.Chemin == "" {
		t.Fatalf("lien : %+v", l)
	}

	// hors connexion : les copies de cet ordinateur restent listées et s'ouvrent
	ts.Close()
	c, v := b.local("GET", "/api/univers/serveur/univers", nil)
	if c != 200 || v["hors_ligne"] != true || len(v["univers"].([]any)) != 2 {
		t.Fatalf("liste hors connexion : %d %v", c, v)
	}
	ouvrir(second)
}

// Un compte qui a accès à plusieurs univers : le poste rend quand même son jeton (les routes du
// compte ne visent aucun univers), et se reconnecter ne laisse qu'un jeton par poste, même après un
// départ sans réponse du serveur.
func TestBureauJetonUnique(t *testing.T) {
	a, reg, connecter := serveurMulti(t)
	chef := connecter("chef") // administrateur : accès à tous les univers
	chef.faire("POST", "/api/univers/nouveau", map[string]any{"nom": "Les Marches"})
	if len(a.tous) != 2 {
		t.Fatalf("univers : %d", len(a.tous))
	}
	ts := httptest.NewServer(a)
	defer ts.Close()
	b := nouvelleAppli(t, t.TempDir())
	connexion := func() {
		t.Helper()
		if c, v := b.local("POST", "/api/univers/serveur/connexion", map[string]any{"url": ts.URL, "nom": "chef", "mdp": "mot-de-passe-chef"}); c != 200 || v["avertissement"] != nil {
			t.Fatalf("connexion : %d %v", c, v)
		}
	}
	connexion()
	if c, v := b.local("POST", "/api/univers/serveur/local", nil); c != 200 || v["avertissement"] != nil || jetonsDe(t, reg, "chef") != 0 {
		t.Fatalf("client seul : %d %v, %d jetons", c, v, jetonsDe(t, reg, "chef"))
	}
	connexion()
	connexion()
	if n := jetonsDe(t, reg, "chef"); n != 1 {
		t.Fatalf("après deux connexions : %d jetons", n)
	}
	// un jeton laissé par ce poste (serveur injoignable au départ) est retiré à la connexion suivante,
	// pas un jeton créé pour autre chose
	b.a.ecrireConnexion(nil)
	chef.faire("POST", "/api/compte/jetons", map[string]any{"nom": "Claude"})
	connexion()
	c, _ := reg.Lire("chef")
	if len(c.Jetons) != 2 || (c.Jetons[0].Nom != "Claude" && c.Jetons[1].Nom != "Claude") {
		t.Fatalf("jetons après un départ sans rendu : %+v", c.Jetons)
	}
}
