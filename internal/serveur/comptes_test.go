package serveur

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Tchyze/cof_dm/internal/comptes"
	"github.com/Tchyze/cof_dm/internal/store"
)

// bancServeur : le mode serveur, avec un administrateur, un MJ et un auditeur de l'univers ouvert,
// et un compte sans rôle.
func bancServeur(t *testing.T) (*banc, *comptes.Registre) {
	reg, err := comptes.Ouvrir(filepath.Join(t.TempDir(), "comptes.json"))
	if err != nil {
		t.Fatal(err)
	}
	b := nouveauAvec(t, Options{Comptes: reg})
	b.s.acces.pause = 0
	for nom, role := range map[string]string{"chef": "", "meneur": comptes.MJ, "audit": comptes.Auditeur, "passant": ""} {
		if _, err := reg.Creer(nom, "mot-de-passe-"+nom, nom == "chef"); err != nil {
			t.Fatal(err)
		}
		if role != "" {
			if _, err := reg.Modifier(nom, func(c *comptes.Compte) error { c.Roles[b.s.store.ID()] = role; return nil }); err != nil {
				t.Fatal(err)
			}
		}
	}
	return b, reg
}

// requête telle que le reverse proxy la transmet : elle vient de cette machine, avec un nom public.
func (b *banc) proxy(methode, url string, corps any, prep func(*http.Request)) (int, map[string]any, *http.Cookie) {
	b.t.Helper()
	j, _ := json.Marshal(corps)
	r := httptest.NewRequest(methode, url, bytes.NewReader(j))
	r.Host, r.RemoteAddr = "jdr.example", "127.0.0.1:5000"
	r.Header.Set("X-Forwarded-For", "203.0.113.9")
	r.Header.Set("X-Forwarded-Proto", "https")
	if methode != "GET" {
		r.Header.Set("X-COF-DM", "1")
	}
	if prep != nil {
		prep(r)
	}
	w := httptest.NewRecorder()
	b.s.ServeHTTP(w, r)
	var v map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &v)
	var recu *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == cookieMJ {
			recu = c
		}
	}
	return w.Code, v, recu
}

func (b *banc) connecter(nom string) func(*http.Request) {
	b.t.Helper()
	c, _, cookie := b.proxy("POST", "/api/acces/connexion", map[string]any{"nom": nom, "mdp": "mot-de-passe-" + nom}, nil)
	if c != 200 || cookie == nil {
		b.t.Fatalf("connexion de %s : %d", nom, c)
	}
	return func(r *http.Request) { r.AddCookie(cookie) }
}

// Derrière un reverse proxy, toute requête vient de la machine elle-même : sans compte, rien.
func TestServeurSansCompte(t *testing.T) {
	b, _ := bancServeur(t)
	for _, u := range []string{"/api/entites", "/api/parametres", "/api/export", "/api/comptes", "/api/compte"} {
		if c, _, _ := b.proxy("GET", u, nil, nil); c != 401 {
			t.Fatalf("GET %s sans compte : %d", u, c)
		}
		// … y compris avec le nom d'hôte de la machine, comme le transmet nginx par défaut
		if c, _, _ := b.proxy("GET", u, nil, func(r *http.Request) { r.Host = "127.0.0.1:8100" }); c != 401 {
			t.Fatalf("GET %s en local sans compte : %d", u, c)
		}
	}
	if c, _, _ := b.proxy("GET", "/", nil, nil); c != 200 {
		t.Fatalf("page de connexion : %d", c)
	}
	if c, v, _ := b.proxy("GET", "/api/acces", nil, nil); c != 200 || v["mj"] != false || v["serveur"] != true {
		t.Fatalf("état sans compte : %d %v", c, v)
	}
	if c, _, cookie := b.proxy("POST", "/api/acces/connexion", map[string]any{"nom": "meneur", "mdp": "faux"}, nil); c != 401 || cookie != nil {
		t.Fatalf("mauvais mot de passe : %d", c)
	}
	if c, _, cookie := b.proxy("POST", "/api/acces/connexion", map[string]any{"nom": "inconnu", "mdp": "mot-de-passe-meneur"}, nil); c != 401 || cookie != nil {
		t.Fatalf("compte inconnu : %d", c)
	}
	// l'ancien jeton unique n'ouvre rien
	b.s.reg.SetMeta("api_jeton", "ancien-jeton-unique")
	if c, _, _ := b.proxy("GET", "/api/entites", nil, func(r *http.Request) { r.Header.Set("Authorization", "Bearer ancien-jeton-unique") }); c != 401 {
		t.Fatalf("ancien jeton : %d", c)
	}
}

func TestServeurRoles(t *testing.T) {
	b, reg := bancServeur(t)
	chef, mj, audit, passant := b.connecter("chef"), b.connecter("meneur"), b.connecter("audit"), b.connecter("passant")
	pnj := map[string]any{"type": "pnj", "nom": "Gautier"}

	// le témoin n'est renvoyé qu'en HTTPS, et dit qui est là
	_, v, cookie := b.proxy("POST", "/api/acces/connexion", map[string]any{"nom": "Meneur ", "mdp": "mot-de-passe-meneur"}, nil)
	if !cookie.Secure || !cookie.HttpOnly || v["mj"] != true {
		t.Fatalf("témoin : %+v %v", cookie, v)
	}
	// MJ : lit et modifie, sans toucher au serveur
	if c, _, _ := b.proxy("POST", "/api/entites", pnj, mj); c != 201 {
		t.Fatalf("création par le MJ : %d", c)
	}
	// administrateur : il n'entre pas dans l'univers
	for _, x := range [][2]string{{"GET", "/api/entites"}, {"POST", "/api/entites"}, {"GET", "/api/parametres"}} {
		if c, _, _ := b.proxy(x[0], x[1], pnj, chef); c != 403 {
			t.Fatalf("%s %s par l'administrateur : %d", x[0], x[1], c)
		}
	}
	if _, v, _ := b.proxy("GET", "/api/acces", nil, chef); v["mj"] != false || v["compte"].(map[string]any)["admin"] != true {
		t.Fatalf("état de l'administrateur : %v", v)
	}
	// auditeur : lit tout, ne modifie rien
	for _, u := range []string{"/api/entites", "/api/secrets", "/api/journal", "/api/export", "/api/parametres"} {
		if c, _, _ := b.proxy("GET", u, nil, audit); c != 200 {
			t.Fatalf("lecture de %s par l'auditeur : %d", u, c)
		}
	}
	for _, x := range [][2]string{{"POST", "/api/entites"}, {"DELETE", "/api/entites/1"}, {"PUT", "/api/calendrier"}, {"POST", "/api/restaurer"}, {"POST", "/api/sauvegarde"}} {
		if c, _, _ := b.proxy(x[0], x[1], pnj, audit); c != 403 {
			t.Fatalf("%s %s par l'auditeur : %d", x[0], x[1], c)
		}
	}
	if _, v, _ := b.proxy("GET", "/api/parametres", nil, audit); v["donnees"] != nil || v["jeton"] != "" {
		t.Fatalf("chemins du serveur montrés à l'auditeur : %v", v)
	}
	if _, v, _ := b.proxy("GET", "/api/acces", nil, audit); v["lecture_seule"] != true {
		t.Fatalf("état de l'auditeur : %v", v)
	}
	// sans rôle dans l'univers : le compte existe, l'univers lui est fermé
	if c, _, _ := b.proxy("GET", "/api/entites", nil, passant); c != 403 {
		t.Fatalf("lecture sans rôle : %d", c)
	}
	if c, v, _ := b.proxy("GET", "/api/acces", nil, passant); c != 200 || v["mj"] != false || v["connecte"] != true {
		t.Fatalf("état sans rôle : %d %v", c, v)
	}
	// une écriture sans l'en-tête de l'interface est refusée (page d'un autre site)
	if c, _, _ := b.proxy("POST", "/api/entites", pnj, func(r *http.Request) { mj(r); r.Header.Del("X-COF-DM") }); c != 403 {
		t.Fatalf("écriture sans en-tête : %d", c)
	}
	reg.Modifier("passant", func(c *comptes.Compte) error { c.Roles[b.s.store.ID()] = comptes.MJ; return nil })
	if c, _, _ := b.proxy("POST", "/api/entites", map[string]any{"type": "pnj", "nom": "Aline"}, passant); c != 201 {
		t.Fatalf("écriture après avoir reçu le rôle : %d", c)
	}
	// désactiver un compte ferme ses sessions
	reg.Modifier("meneur", func(c *comptes.Compte) error { c.Inactif = true; return nil })
	if c, _, _ := b.proxy("GET", "/api/entites", nil, mj); c != 401 {
		t.Fatalf("session d'un compte désactivé : %d", c)
	}
	if c, _, _ := b.proxy("POST", "/api/acces/connexion", map[string]any{"nom": "meneur", "mdp": "mot-de-passe-meneur"}, nil); c != 401 {
		t.Fatalf("connexion d'un compte désactivé : %d", c)
	}
	// le journal porte le nom du compte
	st, _ := b.s.store.Journal(50)
	auteurs := map[string]bool{}
	for _, l := range st {
		auteurs[l.Auteur] = true
	}
	if !auteurs["meneur"] || !auteurs["passant"] || auteurs["chef"] {
		t.Fatalf("auteurs du journal : %v", auteurs)
	}
}

func TestServeurMotDePasseEtJetons(t *testing.T) {
	b, _ := bancServeur(t)
	audit, ailleurs := b.connecter("audit"), b.connecter("audit")
	// changer son mot de passe demande l'ancien, et ferme les autres sessions du compte
	if c, _, _ := b.proxy("POST", "/api/compte/mdp", map[string]any{"ancien": "faux", "nouveau": "un-nouveau-mot-de-passe"}, audit); c != 401 {
		t.Fatalf("ancien mot de passe faux : %d", c)
	}
	c, _, cookie := b.proxy("POST", "/api/compte/mdp", map[string]any{"ancien": "mot-de-passe-audit", "nouveau": "un-nouveau-mot-de-passe"}, audit)
	if c != 200 || cookie == nil {
		t.Fatalf("changement de mot de passe : %d", c)
	}
	if c, _, _ := b.proxy("GET", "/api/compte", nil, ailleurs); c != 401 {
		t.Fatalf("autre session après le changement : %d", c)
	}
	audit = func(r *http.Request) { r.AddCookie(cookie) }
	// jeton d'API : montré une fois, il porte le rôle du compte
	c, v, _ := b.proxy("POST", "/api/compte/jetons", map[string]any{"nom": "script"}, audit)
	clair, _ := v["jeton"].(string)
	if c != 201 || !strings.HasPrefix(clair, "cofdm_") {
		t.Fatalf("création de jeton : %d %v", c, v)
	}
	jeton := func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+clair); r.Header.Del("X-COF-DM") }
	if c, _, _ := b.proxy("GET", "/api/entites", nil, jeton); c != 200 {
		t.Fatalf("lecture avec le jeton : %d", c)
	}
	if c, _, _ := b.proxy("POST", "/api/entites", map[string]any{"type": "pnj", "nom": "X"}, jeton); c != 403 {
		t.Fatalf("écriture avec le jeton d'un auditeur : %d", c)
	}
	if _, v, _ := b.proxy("GET", "/api/compte", nil, jeton); strings.Contains(toJSON(v), clair) || strings.Contains(toJSON(v), "empreinte") {
		t.Fatalf("jeton ou empreinte relus : %v", v)
	}
	// un jeton ne change ni le mot de passe, ni les jetons
	for _, u := range []string{"/api/compte/mdp", "/api/compte/jetons"} {
		if c, _, _ := b.proxy("POST", u, map[string]any{"ancien": "un-nouveau-mot-de-passe", "nouveau": "encore-un-autre-mdp", "nom": "x"}, jeton); c != 403 {
			t.Fatalf("POST %s avec un jeton : %d", u, c)
		}
	}
	id, _ := v["id"].(string)
	if c, _, _ := b.proxy("DELETE", "/api/compte/jetons/"+id, nil, audit); c != 200 {
		t.Fatalf("retrait du jeton : %d", c)
	}
	if c, _, _ := b.proxy("GET", "/api/entites", nil, jeton); c != 401 {
		t.Fatalf("jeton retiré encore accepté : %d", c)
	}
	// déconnexion
	b.proxy("POST", "/api/acces/deconnexion", nil, audit)
	if c, _, _ := b.proxy("GET", "/api/compte", nil, audit); c != 401 {
		t.Fatalf("session après déconnexion : %d", c)
	}
}

// La limite d'essais suit l'adresse que le reverse proxy a vue, pas celle qu'un client prétend.
func TestServeurEssais(t *testing.T) {
	b, _ := bancServeur(t)
	essai := func(xff, mdp string) int {
		c, _, _ := b.proxy("POST", "/api/acces/connexion", map[string]any{"nom": "meneur", "mdp": mdp}, func(r *http.Request) { r.Header.Set("X-Forwarded-For", xff) })
		return c
	}
	for i := 0; i < essaisMax; i++ {
		if c := essai("10.9.9.9, 203.0.113.9", "faux"); c != 401 {
			t.Fatalf("essai %d : %d", i, c)
		}
	}
	if c := essai("172.16.0.1, 203.0.113.9", "mot-de-passe-meneur"); c != 429 {
		t.Fatalf("adresse inventée en tête de X-Forwarded-For : %d", c)
	}
	if c := essai("203.0.113.77", "mot-de-passe-meneur"); c != 200 {
		t.Fatalf("autre adresse : %d", c)
	}
}

func toJSON(v any) string { b, _ := json.Marshal(v); return string(b) }

// navigateur : un compte connecté au serveur, avec ses cookies (session, univers choisi).
type navigateur struct {
	t       *testing.T
	a       *Application
	cookies map[string]*http.Cookie
	jeton   string
}

func (n *navigateur) faire(methode, url string, corps any, enTetes ...string) (int, map[string]any) {
	n.t.Helper()
	w := n.brut(methode, url, corps, enTetes...)
	var v map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &v)
	return w.Code, v
}

// brut : la réponse telle quelle (fichier, en-têtes).
func (n *navigateur) brut(methode, url string, corps any, enTetes ...string) *httptest.ResponseRecorder {
	n.t.Helper()
	j, _ := json.Marshal(corps)
	if brut, ok := corps.([]byte); ok { // un fichier envoyé tel quel
		j = brut
	}
	r := httptest.NewRequest(methode, url, bytes.NewReader(j))
	r.Host, r.RemoteAddr = "127.0.0.1:8100", "127.0.0.1:5000" // comme le transmet nginx par défaut
	if n.jeton != "" {
		r.Header.Set("Authorization", "Bearer "+n.jeton)
	} else {
		r.Header.Set("X-COF-DM", "1")
		for _, c := range n.cookies {
			r.AddCookie(c)
		}
	}
	for i := 0; i+1 < len(enTetes); i += 2 {
		r.Header.Set(enTetes[i], enTetes[i+1])
	}
	w := httptest.NewRecorder()
	n.a.ServeHTTP(w, r)
	for _, c := range w.Result().Cookies() {
		n.cookies[c.Name] = c
	}
	return w
}

func serveurMulti(t *testing.T) (*Application, *comptes.Registre, func(nom string) *navigateur) {
	dir := t.TempDir()
	reg, _ := comptes.Ouvrir(filepath.Join(dir, "comptes.json"))
	a := NouvelleApplication(Options{Version: "test", DossierDon: dir, Port: 8100, Comptes: reg,
		Frontend: fstest.MapFS{"index.html": {Data: []byte("<html>")}}})
	if err := a.Demarrer(""); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.Fermer)
	a.opt.acces.pause = 0
	connecter := func(nom string) *navigateur {
		t.Helper()
		if _, ok := reg.Lire(nom); !ok {
			if _, err := reg.Creer(nom, "mot-de-passe-"+nom, nom == "chef"); err != nil {
				t.Fatal(err)
			}
		}
		n := &navigateur{t: t, a: a, cookies: map[string]*http.Cookie{}}
		if c, _ := n.faire("POST", "/api/acces/connexion", map[string]any{"nom": nom, "mdp": "mot-de-passe-" + nom}); c != 200 {
			t.Fatalf("connexion de %s : %d", nom, c)
		}
		return n
	}
	return a, reg, connecter
}

// Sur un serveur, /api/univers ne crée ni n'ouvre plus rien : l'administration s'en charge. Il reste
// au MJ l'état de son univers et ses sauvegardes, sans chemin du serveur.
func TestServeurUnivers(t *testing.T) {
	a, reg, connecter := serveurMulti(t)
	chef, meneur, lectrice := connecter("chef"), connecter("meneur"), connecter("lectrice")
	donner(reg, "meneur", a.st.ID(), comptes.MJ)
	donner(reg, "lectrice", a.st.ID(), comptes.Auditeur)
	anonyme := &navigateur{t: t, a: a, cookies: map[string]*http.Cookie{}}
	if c, _ := anonyme.faire("GET", "/api/univers", nil); c != 401 {
		t.Fatalf("univers sans compte, depuis la machine : %d", c)
	}
	if c, _ := anonyme.faire("GET", "/api/mes-univers", nil); c != 401 {
		t.Fatalf("liste des univers sans compte : %d", c)
	}
	for _, qui := range []*navigateur{chef, meneur} {
		for _, u := range []string{"/api/univers/nouveau", "/api/univers/ouvrir", "/api/univers/copie", "/api/univers/parcourir"} {
			if c, _ := qui.faire("POST", u, map[string]any{"nom": "Autre monde", "chemin": "/etc/passwd"}); c != 403 {
				t.Fatalf("POST %s sur un serveur : %d", u, c)
			}
		}
	}
	if c, v := meneur.faire("POST", "/api/univers/deposer?nom=Venu", []byte("x")); c != 410 || !strings.Contains(fmt.Sprint(v["detail"]), "importe") {
		t.Fatalf("ancien dépôt d'univers : %d %v", c, v)
	}
	if c, _ := chef.faire("GET", "/api/univers", nil); c != 403 {
		t.Fatalf("état de l'univers pour l'administrateur : %d", c)
	}
	if c, _ := lectrice.faire("GET", "/api/univers", nil); c != 403 {
		t.Fatalf("état de l'univers pour l'auditrice : %d", c)
	}
	c, v := meneur.faire("GET", "/api/univers", nil)
	if c != 200 || v["courant"].(map[string]any)["id"] != a.st.ID() || v["dossier"] != nil || v["dossier_sauvegardes"] != nil ||
		strings.Contains(toJSON(v), a.opt.DossierDon) {
		t.Fatalf("état de l'univers pour son MJ : %d %v", c, v)
	}
	if c, _ := chef.faire("GET", "/api/univers/version", nil); c != 200 {
		t.Fatalf("version du serveur pour l'administrateur : %d", c)
	}
	if c, _ := meneur.faire("GET", "/api/univers/version", nil); c != 403 {
		t.Fatalf("version du serveur pour un MJ : %d", c)
	}
}

// creerUnivers : l'administrateur crée un univers (vide) et renvoie son identifiant.
func creerUnivers(t *testing.T, chef *navigateur, nom string) string {
	t.Helper()
	c, v := chef.faire("POST", "/api/admin/univers", map[string]any{"nom": nom})
	id, _ := v["id"].(string)
	if c != 201 || id == "" {
		t.Fatalf("création de « %s » : %d %v", nom, c, v)
	}
	return id
}

// donner : un rôle dans un univers, comme l'administrateur le donne (joueur : admis et joint).
func donner(reg *comptes.Registre, nom, univers, role string) {
	reg.Modifier(nom, func(c *comptes.Compte) error {
		if role == comptes.Joueur {
			c.Admis = append(c.Admis, univers)
		}
		c.Roles[univers] = role
		return nil
	})
}

// Plusieurs univers ouverts à la fois : chacun entre dans ceux où il a un rôle, avec ce rôle.
func TestServeurMultiUnivers(t *testing.T) {
	a, reg, connecter := serveurMulti(t)
	chef, maitre := connecter("chef"), connecter("maitre")
	premier := a.st.ID()
	second := creerUnivers(t, chef, "Les Marches")
	if second == premier || len(a.tous) != 2 {
		t.Fatalf("second univers : %s, %d univers", second, len(a.tous))
	}
	donner(reg, "maitre", premier, comptes.MJ)
	donner(reg, "maitre", second, comptes.MJ)
	// le MJ des deux passe dans le second, et y écrit
	maitre.faire("POST", "/api/mes-univers/courant", map[string]any{"id": second})
	if c, v := maitre.faire("POST", "/api/entites", map[string]any{"type": "lieu", "nom": "Fort Boueux"}); c != 201 {
		t.Fatalf("écriture dans le second univers : %d %v", c, v)
	}
	if n, _, _ := a.tous[second].st.Compter(); n["lieu"] != 1 {
		t.Fatal("la fiche n'est pas dans le second univers")
	}
	if n, _, _ := a.tous[premier].st.Compter(); n["lieu"] != 0 {
		t.Fatal("la fiche a été écrite dans le premier univers")
	}
	// il revient dans le premier
	if c, _ := maitre.faire("POST", "/api/mes-univers/courant", map[string]any{"id": premier}); c != 200 {
		t.Fatalf("changement d'univers : %d", c)
	}
	if _, v := maitre.faire("GET", "/api/parametres", nil); v["univers"].(map[string]any)["id"] != premier {
		t.Fatalf("univers courant après le changement : %v", v["univers"])
	}
	// des rôles différents selon l'univers
	meneur, lectrice, passant := connecter("meneur"), connecter("lectrice"), connecter("passant")
	reg.Modifier("meneur", func(c *comptes.Compte) error { c.Roles[second] = comptes.MJ; return nil })
	reg.Modifier("lectrice", func(c *comptes.Compte) error {
		c.Roles[premier], c.Roles[second] = comptes.MJ, comptes.Auditeur
		return nil
	})
	// un seul univers accessible : c'est le sien d'office, même s'il n'est pas le premier
	if c, v := meneur.faire("GET", "/api/entites", nil); c != 200 {
		t.Fatalf("MJ du second univers : %d %v", c, v)
	}
	if _, v := meneur.faire("GET", "/api/mes-univers", nil); len(v["univers"].([]any)) != 1 || v["courant"] != second {
		t.Fatalf("univers du MJ : %v", v)
	}
	if c, _ := meneur.faire("POST", "/api/mes-univers/courant", map[string]any{"id": premier}); c != 404 {
		t.Fatalf("entrée dans un univers sans rôle : %d", c)
	}
	if c, _ := meneur.faire("GET", "/api/entites", nil, "X-COF-Univers", premier); c != 403 {
		t.Fatalf("univers sans rôle, par l'en-tête : %d", c)
	}
	// un cookie d'univers falsifié ne donne rien : retour à son univers
	meneur.cookies[cookieUnivers] = &http.Cookie{Name: cookieUnivers, Value: premier}
	if _, v := meneur.faire("GET", "/api/parametres", nil); v["univers"].(map[string]any)["id"] != second {
		t.Fatalf("univers obtenu avec un cookie falsifié : %v", v)
	}
	// deux univers, deux rôles
	if _, v := lectrice.faire("GET", "/api/mes-univers", nil); len(v["univers"].([]any)) != 2 {
		t.Fatalf("univers de la lectrice : %v", v)
	}
	pnj := map[string]any{"type": "pnj", "nom": "Gautier"}
	lectrice.faire("POST", "/api/mes-univers/courant", map[string]any{"id": premier})
	if c, _ := lectrice.faire("POST", "/api/entites", pnj); c != 201 {
		t.Fatalf("écriture là où elle est MJ : %d", c)
	}
	lectrice.faire("POST", "/api/mes-univers/courant", map[string]any{"id": second})
	if c, _ := lectrice.faire("POST", "/api/entites", pnj); c != 403 {
		t.Fatalf("écriture là où elle est auditrice : %d", c)
	}
	if c, _ := lectrice.faire("GET", "/api/entites", nil); c != 200 {
		t.Fatalf("lecture là où elle est auditrice : %d", c)
	}
	// sans rôle nulle part
	if c, _ := passant.faire("GET", "/api/entites", nil); c != 403 {
		t.Fatalf("compte sans rôle : %d", c)
	}
	if _, v := passant.faire("GET", "/api/mes-univers", nil); len(v["univers"].([]any)) != 0 {
		t.Fatalf("univers d'un compte sans rôle : %v", v)
	}
	// jeton d'API : avec plusieurs univers, il doit dire lequel
	_, v := lectrice.faire("POST", "/api/compte/jetons", map[string]any{"nom": "script"})
	script := &navigateur{t: t, a: a, cookies: map[string]*http.Cookie{}, jeton: v["jeton"].(string)}
	if c, _ := script.faire("POST", "/api/entites", pnj); c != 400 {
		t.Fatalf("jeton sans univers précisé : %d", c)
	}
	if c, v := script.faire("GET", "/api/mes-univers", nil); c != 200 || len(v["univers"].([]any)) != 2 {
		t.Fatalf("univers du jeton : %d %v", c, v)
	}
	if c, _ := script.faire("POST", "/api/entites", map[string]any{"type": "pnj", "nom": "Aline"}, "X-COF-Univers", premier); c != 201 {
		t.Fatalf("écriture par jeton dans le premier univers : %d", c)
	}
	if c, _ := script.faire("POST", "/api/entites", pnj, "X-COF-Univers", second); c != 403 {
		t.Fatalf("écriture par jeton là où le compte est auditeur : %d", c)
	}
	if c, _ := script.faire("GET", "/api/entites", nil, "X-COF-Univers", "inconnu"); c != 404 {
		t.Fatalf("univers inconnu : %d", c)
	}
	// la copie d'un univers porte son identifiant : posée dans le dossier, elle n'est pas servie à
	// côté de l'original
	copie := filepath.Join(a.DossierUnivers(), "Copie.cofdm")
	if err := a.tous[second].st.Sauvegarder(copie); err != nil {
		t.Fatal(err)
	}
	a.ouvrirDossier()
	if len(a.tous) != 2 {
		t.Fatalf("copie ouverte à côté de l'original : %d univers", len(a.tous))
	}
	// restaurer une sauvegarde ne touche que l'univers visé
	maitre.faire("POST", "/api/mes-univers/courant", map[string]any{"id": second})
	c, v := maitre.faire("POST", "/api/sauvegarde", nil)
	fichier := filepath.Base(v["fichier"].(string))
	maitre.faire("POST", "/api/entites", map[string]any{"type": "lieu", "nom": "À effacer"})
	if c, _ := lectrice.faire("POST", "/api/univers/restaurer", map[string]any{"fichier": fichier}, enTeteUnivers, second); c != 403 {
		t.Fatalf("restauration par l'auditrice : %d", c)
	}
	if c, v = maitre.faire("POST", "/api/univers/restaurer", map[string]any{"fichier": fichier}); c != 200 {
		t.Fatalf("restauration : %d %v", c, v)
	}
	if n, _, _ := a.tous[second].st.Compter(); n["lieu"] != 1 {
		t.Fatalf("second univers après restauration : %v", n)
	}
	if n, _, _ := a.tous[premier].st.Compter(); n["pnj"] != 2 || len(a.tous) != 2 {
		t.Fatalf("premier univers touché par la restauration du second : %v", n)
	}
	// au redémarrage, les deux univers sont rouverts, et c'est l'original qui est servi, pas sa
	// copie (« Copie » passe avant « Les Marches » dans le dossier)
	chemins := map[string]string{premier: a.tous[premier].st.Chemin(), second: a.tous[second].st.Chemin()}
	a.Fermer()
	b := NouvelleApplication(Options{Version: "test", DossierDon: a.opt.DossierDon, Port: 8100, Comptes: reg,
		Frontend: fstest.MapFS{"index.html": {Data: []byte("<html>")}}})
	if err := b.Demarrer(""); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(b.Fermer)
	if len(b.tous) != 2 || b.st.ID() != premier {
		t.Fatalf("univers rouverts : %d, courant %s", len(b.tous), b.st.ID())
	}
	for id, chemin := range chemins {
		if o := b.tous[id]; o == nil || o.st.Chemin() != chemin {
			t.Fatalf("univers %s servi depuis un autre fichier que %s", id, chemin)
		}
	}
}

// Un compte joueur n'a que sa page de joueur : le reste de l'univers lui est fermé, même les fiches
// partagées. Le filtre des fiches partagées sert à l'export « joueurs » du MJ, et ne laisse rien
// passer de ce qui est réservé au MJ.
func TestServeurJoueur(t *testing.T) {
	b, reg := bancServeur(t)
	reg.Creer("joueuse", "mot-de-passe-joueuse", false)
	reg.Modifier("joueuse", func(c *comptes.Compte) error {
		c.Admis = append(c.Admis, b.s.store.ID())
		c.Roles[b.s.store.ID()] = comptes.Joueur
		return nil
	})
	mj, joueuse := b.connecter("meneur"), b.connecter("joueuse")
	creer := func(typ, nom string, data map[string]any, partage bool) int64 {
		c, v, _ := b.proxy("POST", "/api/entites", map[string]any{"type": typ, "nom": nom, "data": data, "tags": []string{"marches"}}, mj)
		if c != 201 {
			t.Fatalf("création de %s : %d %v", nom, c, v)
		}
		id := int64(v["id"].(float64))
		if partage {
			if c, v, _ := b.proxy("POST", "/api/entites/"+strconv.FormatInt(id, 10)+"/partage", map[string]any{"partage": true}, mj); c != 200 || v["partage"] != true {
				t.Fatalf("partage de %s : %d %v", nom, c, v)
			}
		}
		return id
	}
	cache := creer("pnj", "Le Traître", map[string]any{"resume": "Personne ne le soupçonne."}, false)
	lieu := creer("lieu", "Fort Boueux", nil, true)
	gautier := creer("pnj", "Gautier", map[string]any{
		"resume": "Aubergiste jovial, ami de [[Fort Boueux]] et de [[Le Traître]].", "secrets": "SECRET-MJ : il empoisonne ses clients.",
		"notes": "SECRET-MJ notes", "responsable_secret": float64(cache), "lieu": float64(lieu)}, true)
	secret := creer("secret", "La vérité sur Gautier", map[string]any{"resume": "SECRET-MJ"}, false)
	chemin := func(id int64) string { return "/api/entites/" + strconv.FormatInt(id, 10) }

	// une fiche « Secret » ne se partage pas
	if c, _, _ := b.proxy("POST", chemin(secret)+"/partage", map[string]any{"partage": true}, mj); c != 422 {
		t.Fatalf("partage d'un secret : %d", c)
	}
	// le joueur ne partage rien et n'écrit rien
	if c, _, _ := b.proxy("POST", chemin(cache)+"/partage", map[string]any{"partage": true}, joueuse); c != 403 {
		t.Fatalf("partage par un joueur : %d", c)
	}
	if c, _, _ := b.proxy("PATCH", chemin(gautier), map[string]any{"nom": "X"}, joueuse); c != 403 {
		t.Fatalf("écriture par un joueur : %d", c)
	}
	// ni fiche, même partagée, ni liste, ni schéma, ni rien de l'application du MJ
	png := "\x89PNG\r\n\x1a\n" + strings.Repeat("\x00", 32)
	r := httptest.NewRequest("POST", chemin(gautier)+"/medias?nom=a.png", strings.NewReader(png))
	r.Host, r.RemoteAddr = "jdr.example", "127.0.0.1:5000"
	r.Header.Set("X-COF-DM", "1")
	r.Header.Set("Content-Type", "image/png")
	mj(r)
	w := httptest.NewRecorder()
	b.s.ServeHTTP(w, r)
	var m map[string]any
	json.Unmarshal(w.Body.Bytes(), &m)
	if w.Code != 201 {
		t.Fatalf("image : %d %s", w.Code, w.Body.String())
	}
	for _, u := range []string{chemin(gautier), chemin(lieu), "/api/entites", "/api/entites?q=gaut", "/api/stats", "/api/schema",
		"/api/calendrier", "/api/parametres", "/api/resoudre?nom=gautier", "/medias/" + m["fichier"].(string),
		"/api/journal", "/api/export", "/api/export/obsidian", "/api/export/obsidian?secrets=0", "/api/secrets", "/api/graphe",
		"/api/chronologie", "/api/table", "/api/combats", "/api/liens-morts", "/api/bestiaire", chemin(gautier) + "/organigramme",
		"/api/comptes", "/api/fonctionnalites", "/api/regles"} {
		if c, _, _ := b.proxy("GET", u, nil, joueuse); c != 403 {
			t.Fatalf("GET %s par le joueur : %d", u, c)
		}
	}
	// sa page répond
	for _, u := range []string{"/api/joueur/etat", "/api/joueur/pj", "/api/joueur/hauts-faits", "/api/sante"} {
		if c, v, _ := b.proxy("GET", u, nil, joueuse); c != 200 {
			t.Fatalf("GET %s par le joueur : %d %v", u, c, v)
		}
	}
	if _, v, _ := b.proxy("GET", "/api/acces", nil, joueuse); v["joueur"] != true || v["lecture_seule"] != true || v["mj"] != true {
		t.Fatalf("état du joueur : %v", v)
	}
	// ce que voient les joueurs (export « joueurs ») : les fiches partagées, sans champ du MJ, et
	// leurs références réduites aux fiches partagées
	vues, err := b.s.mondeJoueur()
	if err != nil || len(vues) != 2 {
		t.Fatalf("fiches vues des joueurs : %v %v", err, vues)
	}
	texte := toJSON(vues[gautier])
	if !strings.Contains(texte, "Aubergiste jovial") || vues[gautier].Data["lieu"] != float64(lieu) {
		t.Fatalf("fiche partagée : %s", texte)
	}
	for _, interdit := range []string{"SECRET-MJ", "responsable_secret"} {
		if strings.Contains(texte, interdit) {
			t.Fatalf("fuite de %q vers les joueurs : %s", interdit, texte)
		}
	}
	// retirer le partage, ou archiver, referme la fiche
	b.proxy("POST", chemin(gautier)+"/partage", map[string]any{"partage": false}, mj)
	b.proxy("DELETE", chemin(lieu), nil, mj) // archivage
	if vues, _ := b.s.mondeJoueur(); len(vues) != 0 {
		t.Fatalf("fiches retirées ou archivées : %v", vues)
	}
}

// Édition par les joueurs : fermée par défaut ; ouverte par le MJ de l'univers, chaque joueur modifie,
// depuis sa page, les seuls champs prévus de la fiche de son PJ.
func TestServeurEditionJoueur(t *testing.T) {
	b, reg := bancServeur(t)
	for _, nom := range []string{"joueuse", "autre"} {
		reg.Creer(nom, "mot-de-passe-"+nom, false)
		reg.Modifier(nom, func(c *comptes.Compte) error {
			c.Admis = append(c.Admis, b.s.store.ID())
			c.Roles[b.s.store.ID()] = comptes.Joueur
			return nil
		})
	}
	chef, mj, audit, joueuse, autre := b.connecter("chef"), b.connecter("meneur"), b.connecter("audit"), b.connecter("joueuse"), b.connecter("autre")
	creer := func(typ, nom string) string {
		_, v, _ := b.proxy("POST", "/api/entites", map[string]any{"type": typ, "nom": nom, "data": map[string]any{"secrets": "SECRET-MJ"}}, mj)
		u := "/api/entites/" + strconv.FormatInt(int64(v["id"].(float64)), 10)
		b.proxy("POST", u+"/partage", map[string]any{"partage": true}, mj)
		return u
	}
	pj, pnj := creer("pj", "Aldric"), creer("pnj", "Gautier")
	idPJ, _ := strconv.ParseInt(strings.TrimPrefix(pj, "/api/entites/"), 10, 64)
	idPNJ, _ := strconv.ParseInt(strings.TrimPrefix(pnj, "/api/entites/"), 10, 64)
	page := func(id int64) string { return "/api/joueur/pj/" + strconv.FormatInt(id, 10) }
	texte := map[string]any{"data": map[string]any{"historique": "Né à Fort Boueux."}}

	// rattacher un compte à son PJ : le MJ seulement, et à un PJ
	for qui, n := range map[string]func(*http.Request){"chef": chef, "audit": audit} {
		if c, _, _ := b.proxy("PATCH", "/api/joueurs/comptes/joueuse", map[string]any{"pj": idPJ}, n); c != 403 {
			t.Fatalf("rattachement par %s : %d", qui, c)
		}
	}
	if c, _, _ := b.proxy("PATCH", "/api/joueurs/comptes/joueuse", map[string]any{"pj": idPNJ}, mj); c != 422 {
		t.Fatalf("rattachement à un PNJ : %d", c)
	}
	if c, v, _ := b.proxy("PATCH", "/api/joueurs/comptes/joueuse", map[string]any{"pj": idPJ}, mj); c != 200 || v["pj"] != float64(idPJ) {
		t.Fatalf("rattachement : %d %v", c, v)
	}
	// sa page montre les champs qu'elle tiendra, sans ceux du MJ
	r := httptest.NewRequest("GET", "/api/joueur/pj", nil)
	r.Host, r.RemoteAddr = "jdr.example", "127.0.0.1:5000"
	joueuse(r)
	w := httptest.NewRecorder()
	b.s.ServeHTTP(w, r)
	var liste []pjJoueur
	if err := json.Unmarshal(w.Body.Bytes(), &liste); err != nil || len(liste) != 1 || len(liste[0].Recit) != 5 || strings.Contains(w.Body.String(), "SECRET-MJ") {
		t.Fatalf("PJ de la joueuse : %d %s", w.Code, w.Body.String())
	}
	// édition fermée par défaut
	if c, _, _ := b.proxy("PATCH", page(idPJ), texte, joueuse); c != 403 {
		t.Fatalf("édition fermée : %d", c)
	}
	// le MJ de l'univers l'ouvre (l'administrateur aussi) ; ni l'auditeur ni le joueur
	for nom, qui := range map[string]func(*http.Request){"audit": audit, "joueuse": joueuse} {
		if c, _, _ := b.proxy("PUT", "/api/joueurs/edition", map[string]any{"active": true}, qui); c != 403 {
			t.Fatalf("édition ouverte par %s : %d", nom, c)
		}
	}
	if c, _, _ := b.proxy("PUT", "/api/joueurs/edition", map[string]any{"active": true}, mj); c != 200 {
		t.Fatalf("ouverture de l'édition par le MJ : %d", c)
	}
	if _, v, _ := b.proxy("GET", "/api/acces", nil, joueuse); v["edition_joueurs"] != true {
		t.Fatalf("état : %v", v)
	}
	// la joueuse modifie les champs prévus de son PJ
	c, v, _ := b.proxy("PATCH", page(idPJ), texte, joueuse)
	if c != 200 || !strings.Contains(toJSON(v), "Fort Boueux") || strings.Contains(toJSON(v), "SECRET-MJ") || v["id"] != float64(idPJ) {
		t.Fatalf("modification de son PJ : %d %v", c, v)
	}
	// … et rien d'autre : ni champ du MJ, ni champ non prévu, ni nom, ni tags
	for _, corps := range []map[string]any{
		{"data": map[string]any{"secrets": "j'efface les secrets"}},
		{"data": map[string]any{"statut": "mort"}},
		{"data": map[string]any{"niveau": 20}},
		{"data": map[string]any{"historique": "x", "joueur": "moi"}},
		{"data": map[string]any{"inconnu": "x"}},
		{"data": map[string]any{}},
		{"nom": "Autre nom"},
		{"tags": []string{"x"}},
		{"data": map[string]any{"historique": "x"}, "remplacer": true},
	} {
		if c, _, _ := b.proxy("PATCH", page(idPJ), corps, joueuse); c != 403 && c != 422 {
			t.Fatalf("modification refusée attendue pour %v : %d", corps, c)
		}
	}
	// ni par l'API du MJ, ni la fiche d'un autre, ni une autre fiche : elles n'existent pas pour elle
	for _, m := range []string{"PATCH", "PUT"} {
		if c, _, _ := b.proxy(m, pj, texte, joueuse); c != 403 {
			t.Fatalf("%s %s par la joueuse : %d", m, pj, c)
		}
	}
	if c, _, _ := b.proxy("PATCH", page(idPJ), texte, autre); c != 404 {
		t.Fatalf("PJ d'un autre joueur : %d", c)
	}
	if c, _, _ := b.proxy("PATCH", page(idPNJ), map[string]any{"data": map[string]any{"apparence": "x"}}, joueuse); c != 404 {
		t.Fatalf("fiche d'un PNJ : %d", c)
	}
	// les secrets du MJ sont intacts, et le journal dit qui a écrit
	_, v, _ = b.proxy("GET", pj, nil, mj)
	if d := v["data"].(map[string]any); d["secrets"] != "SECRET-MJ" || d["historique"] != "Né à Fort Boueux." || v["nom"] != "Aldric" {
		t.Fatalf("fiche après les essais : %v", v)
	}
	j, _ := b.s.store.Journal(5)
	if j[0].Auteur != "joueuse" {
		t.Fatalf("auteur de la modification : %+v", j[0])
	}
	// un PJ retiré aux joueurs ne se modifie plus ; l'édition refermée non plus
	b.proxy("POST", pj+"/partage", map[string]any{"partage": false}, mj)
	if c, _, _ := b.proxy("PATCH", page(idPJ), texte, joueuse); c != 404 {
		t.Fatalf("PJ caché : %d", c)
	}
	b.proxy("POST", pj+"/partage", map[string]any{"partage": true}, mj)
	if c, _, _ := b.proxy("PUT", "/api/joueurs/edition", map[string]any{"active": false}, chef); c != 403 {
		t.Fatalf("fermeture par l'administrateur : %d", c)
	}
	if c, _, _ := b.proxy("PUT", "/api/joueurs/edition", map[string]any{"active": false}, mj); c != 200 {
		t.Fatalf("fermeture par le MJ : %d", c)
	}
	if c, _, _ := b.proxy("PATCH", page(idPJ), texte, joueuse); c != 403 {
		t.Fatalf("édition refermée : %d", c)
	}
}

// Le MJ verse dans l'univers créé par l'administrateur les données d'un fichier d'univers, après
// contrôle ; un univers qui a déjà des fiches n'est remplacé qu'à sa demande, après une sauvegarde.
func TestImporterUnivers(t *testing.T) {
	a, reg, connecter := serveurMulti(t)
	chef, meneur, lectrice := connecter("chef"), connecter("meneur"), connecter("lectrice")
	id := creerUnivers(t, chef, "Les Marches")
	donner(reg, "meneur", id, comptes.MJ)
	donner(reg, "lectrice", id, comptes.Auditeur)
	fichier := func(piege string) []byte {
		chemin := filepath.Join(t.TempDir(), "venu.cofdm")
		st, err := store.Ouvrir(chemin)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := st.Creer("lieu", "Fort Boueux", nil, nil, "test"); err != nil {
			t.Fatal(err)
		}
		st.Fermer()
		if piege != "" {
			db, _ := sql.Open("sqlite3", chemin)
			if _, err := db.Exec(piege); err != nil {
				t.Fatal(err)
			}
			db.Close()
		}
		b, err := os.ReadFile(chemin)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	bon := fichier("")
	for _, c := range []struct {
		qui     *navigateur
		url     string
		corps   []byte
		code    int
		attendu string
	}{
		{chef, "/api/import/univers", bon, 403, ""},
		{lectrice, "/api/import/univers", bon, 403, ""},
		{meneur, "/api/import/univers", []byte("pas une base"), 422, "pas un univers"},
		{meneur, "/api/import/univers", nil, 422, "pas un univers"},
		{meneur, "/api/import/univers", fichier(`CREATE TRIGGER piege AFTER INSERT ON journal BEGIN DELETE FROM entites; END`), 422, "pas un univers"},
	} {
		code, v := c.qui.faire("POST", c.url, c.corps, enTeteUnivers, id)
		if code != c.code || !strings.Contains(fmt.Sprint(v["detail"]), c.attendu) {
			t.Fatalf("%s : %d %v (attendu %d, « %s »)", c.url, code, v, c.code, c.attendu)
		}
	}
	// l'import accepté : les fiches arrivent, l'univers garde son identifiant, son nom et ses rôles
	if code, v := meneur.faire("POST", "/api/import/univers", bon, enTeteUnivers, id); code != 200 || v["fiches"] != float64(1) {
		t.Fatalf("import : %d %v", code, v)
	}
	o := a.tous[id]
	if n, _, _ := o.st.Compter(); n["lieu"] != 1 || nomUnivers(o.st.Chemin()) != "Les Marches" || len(a.tous) != 2 {
		t.Fatalf("univers après l'import : %v %s", n, o.st.Chemin())
	}
	if c, _ := reg.Lire("meneur"); c.Role(id) != comptes.MJ {
		t.Fatal("rôle perdu à l'import")
	}
	// le remplacer demande confirmation, et le sauvegarde avant
	if code, _ := meneur.faire("POST", "/api/import/univers", bon, enTeteUnivers, id); code != 409 {
		t.Fatalf("import sur un univers qui a des fiches : %d", code)
	}
	if code, v := meneur.faire("POST", "/api/import/univers?remplacer=1", bon, enTeteUnivers, id); code != 200 || !strings.Contains(fmt.Sprint(v["sauvegarde"]), "avant-import") {
		t.Fatalf("remplacement : %d %v", code, v)
	}
	if es, _ := os.ReadDir(os.TempDir()); len(es) > 0 {
		for _, e := range es {
			if strings.HasPrefix(e.Name(), "cofdm-import-") {
				t.Fatalf("fichier d'import laissé : %s", e.Name())
			}
		}
	}
}

// Chaque onglet désigne son univers : par l'en-tête, ou par ?univers= pour une lecture (images,
// WebSocket, téléchargements). Le paramètre ne vaut pas pour une écriture et ne donne aucun droit.
func TestUniversParOnglet(t *testing.T) {
	a, reg, connecter := serveurMulti(t)
	premier := a.st.ID()
	second := creerUnivers(t, connecter("chef"), "Les Marches")
	chef := connecter("maitre") // MJ des deux, dans le second
	donner(reg, "maitre", premier, comptes.MJ)
	donner(reg, "maitre", second, comptes.MJ)
	chef.faire("POST", "/api/mes-univers/courant", map[string]any{"id": second})
	if c, _ := chef.faire("POST", "/api/entites", map[string]any{"type": "lieu", "nom": "Fort Boueux"}, enTeteUnivers, premier); c != 201 {
		t.Fatalf("écriture par l'en-tête : %d", c)
	}
	lieux := func(url string) any {
		_, v := chef.faire("GET", url, nil)
		return v["compte"].(map[string]any)["lieu"]
	}
	if n := lieux("/api/stats?univers=" + premier); n != float64(1) {
		t.Fatalf("lecture du premier par ?univers= : %v", n)
	}
	if n := lieux("/api/stats"); n != nil && n != float64(0) {
		t.Fatalf("lecture sans paramètre (second univers, celui du navigateur) : %v", n)
	}
	// en écriture, le paramètre est ignoré : c'est l'univers du navigateur qui reçoit la fiche
	chef.faire("POST", "/api/entites?univers="+premier, map[string]any{"type": "lieu", "nom": "Gué Noir"})
	if n, _, _ := a.tous[second].st.Compter(); n["lieu"] != 1 {
		t.Fatalf("écriture avec ?univers= : %v", n)
	}
	// un compte sans rôle dans l'univers désigné n'y lit rien
	meneur := connecter("meneur")
	reg.Modifier("meneur", func(c *comptes.Compte) error { c.Roles[second] = comptes.MJ; return nil })
	if c, _ := meneur.faire("GET", "/api/stats?univers="+premier, nil); c == 200 {
		t.Fatal("lecture d'un univers sans rôle par ?univers=")
	}
}
