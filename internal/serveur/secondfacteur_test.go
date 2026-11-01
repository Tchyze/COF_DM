package serveur

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Tchyze/cof_dm/internal/comptes"
)

// totpDeTest : la clé TOTP des administrateurs de test.
const totpDeTest = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"

// Une horloge par registre, qui avance d'un pas à chaque code : un code de test n'est jamais un rejeu,
// sauf quand un test le veut.
type horlogeEssai struct {
	mu sync.Mutex
	t  time.Time
}

func (h *horlogeEssai) lire() time.Time {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.t
}

func (h *horlogeEssai) avancer() time.Time {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.t = h.t.Add(30 * time.Second)
	return h.t
}

var (
	horlogesMu sync.Mutex
	horloges   = map[*comptes.Registre]*horlogeEssai{}
)

func horlogeDe(reg *comptes.Registre) *horlogeEssai {
	horlogesMu.Lock()
	defer horlogesMu.Unlock()
	h := horloges[reg]
	if h == nil {
		h = &horlogeEssai{t: time.Now()}
		horloges[reg] = h
		reg.HorlogePourTests(h.lire)
	}
	return h
}

// codeSuivant : un code valable pour le secret de test, jamais encore donné.
func codeSuivant(reg *comptes.Registre) string {
	return comptes.CodeTOTP(totpDeTest, horlogeDe(reg).avancer())
}

// doterDeFacteur : un administrateur qui a déjà son TOTP, pour les tests qui ne portent pas là-dessus.
func doterDeFacteur(t *testing.T, reg *comptes.Registre, nom string) []string {
	t.Helper()
	codes, err := reg.ActiverTOTP(nom, totpDeTest, codeSuivant(reg))
	if err != nil {
		t.Fatal(err)
	}
	return codes
}

// confirmationDe : ce que demande une action sensible, pour un compte au mot de passe de test.
func confirmationDe(reg *comptes.Registre, nom string) map[string]any {
	return map[string]any{"mdp": mdpEssai(nom), "facteur": "totp", "code": codeSuivant(reg)}
}

// finirConnexionAdmin : le mot de passe vient de passer ; le second facteur, et l'enrôlement avant s'il
// le faut.
func (n *navigateur) finirConnexionAdmin(nom string) {
	n.t.Helper()
	x, _ := n.reg.Lire(nom)
	if !x.AFacteur() {
		n.t.Fatalf("connexion de %s : aucun facteur pour ce test (doterDeFacteur, ou enrôlement à la main)", nom)
	}
	if c, v := n.faire("POST", routeSecondFacteur, map[string]any{"facteur": "totp", "code": codeSuivant(n.reg)}); c != 200 {
		n.t.Fatalf("second facteur de %s : %d %v", nom, c, v)
	}
}

// connecterAdmin : la connexion d'un administrateur par le banc d'un seul univers.
func (b *banc) connecterAdmin(nom, mdp string) func(*http.Request) {
	b.t.Helper()
	reg := b.s.acces.comptes
	if x, _ := reg.Lire(nom); !x.AFacteur() {
		doterDeFacteur(b.t, reg, nom)
	}
	var preuve *http.Cookie
	code, _, _ := b.proxy("POST", "/api/acces/connexion", map[string]any{"nom": nom, "mdp": mdp}, nil)
	if code != 200 {
		b.t.Fatalf("mot de passe de %s : %d", nom, code)
	}
	preuve = b.dernierePreuve
	code, v, session := b.proxy("POST", routeSecondFacteur, map[string]any{"facteur": "totp", "code": codeSuivant(reg)}, func(r *http.Request) { r.AddCookie(preuve) })
	if code != 200 || session == nil {
		b.t.Fatalf("second facteur de %s : %d %v", nom, code, v)
	}
	return func(r *http.Request) { r.AddCookie(session) }
}

func preparerAdmin(t *testing.T) (*Application, *comptes.Registre, *navigateur) {
	t.Helper()
	a, reg, connecter := serveurMulti(t)
	if _, err := reg.Creer("chef", mdpEssai("chef"), true); err != nil {
		t.Fatal(err)
	}
	doterDeFacteur(t, reg, "chef")
	return a, reg, connecter("chef")
}

// anonymeDe : un navigateur sans rien, depuis l'adresse commune des tests.
func anonymeDe(a *Application, t *testing.T, reg *comptes.Registre) *navigateur {
	return &navigateur{t: t, a: a, cookies: map[string]*http.Cookie{}, reg: reg}
}

// Le mot de passe seul n'ouvre aucune session : seulement une preuve, qui n'ouvre que l'étape suivante.
func TestMotDePasseSeulSansSession(t *testing.T) {
	a, reg, _ := preparerAdmin(t)
	n := anonymeDe(a, t, reg)
	w := n.brut("POST", "/api/acces/connexion", map[string]any{"nom": "chef", "mdp": mdpEssai("chef")})
	var v map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &v)
	if w.Code != 200 || v["etape"] != "second_facteur" || v["compte"] != nil {
		t.Fatalf("mot de passe : %d %v", w.Code, v)
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control : %q", w.Header().Get("Cache-Control"))
	}
	if n.cookies[cookieMJ] != nil {
		t.Fatal("une session pour le mot de passe seul")
	}
	p := n.cookies[cookiePreuve]
	if p == nil || !p.HttpOnly || p.SameSite != http.SameSiteStrictMode || p.Path != routeSecondFacteur {
		t.Fatalf("preuve : %+v", p)
	}
	if d := time.Until(p.Expires); d > dureePreuve+time.Second || d < dureePreuve-time.Minute {
		t.Fatalf("durée de la preuve : %v", d)
	}
	for _, u := range []string{"/api/admin/comptes", "/api/admin/univers", "/api/compte", "/api/mes-univers"} {
		if c, _ := n.faire("GET", u, nil); c != 401 {
			t.Fatalf("GET %s avec la preuve seule : %d", u, c)
		}
	}
	// la preuve mise à la place d'une session ne vaut rien, ni l'inverse
	vol := anonymeDe(a, t, reg)
	vol.cookies[cookieMJ] = &http.Cookie{Name: cookieMJ, Value: p.Value}
	if c, _ := vol.faire("GET", "/api/admin/comptes", nil); c != 401 {
		t.Fatalf("preuve en guise de session : %d", c)
	}
	session := connecterMJ(t, a, reg)
	inverse := anonymeDe(a, t, reg)
	inverse.cookies[cookiePreuve] = &http.Cookie{Name: cookiePreuve, Value: session.cookies[cookieMJ].Value}
	if c, _ := inverse.faire("POST", routeSecondFacteur, map[string]any{"facteur": "totp", "code": codeSuivant(reg)}); c != 401 {
		t.Fatalf("session en guise de preuve : %d", c)
	}
	// le mauvais mot de passe : la réponse d'un compte inconnu, sans preuve
	x := anonymeDe(a, t, reg)
	mauvais := x.brut("POST", "/api/acces/connexion", map[string]any{"nom": "chef", "mdp": "pas-le-bon-mot-de-passe"})
	inconnu := x.brut("POST", "/api/acces/connexion", map[string]any{"nom": "personne", "mdp": "pas-le-bon-mot-de-passe"})
	if mauvais.Code != 401 || inconnu.Code != 401 || mauvais.Body.String() != inconnu.Body.String() || x.cookies[cookiePreuve] != nil {
		t.Fatalf("mauvais %d %s, inconnu %d %s", mauvais.Code, mauvais.Body, inconnu.Code, inconnu.Body)
	}
}

func connecterMJ(t *testing.T, a *Application, reg *comptes.Registre) *navigateur {
	t.Helper()
	if _, ok := reg.Lire("meneur"); !ok {
		if _, err := reg.Creer("meneur", mdpEssai("meneur"), false); err != nil {
			t.Fatal(err)
		}
	}
	n := anonymeDe(a, t, reg)
	if c, v := n.faire("POST", "/api/acces/connexion", map[string]any{"nom": "meneur", "mdp": mdpEssai("meneur")}); c != 200 || v["etape"] != nil {
		t.Fatalf("connexion du MJ : %d %v", c, v)
	}
	return n
}

// Une preuve expirée, rejouée, d'un autre compte ou d'avant un changement de mot de passe est refusée.
func TestPreuveRefusee(t *testing.T) {
	a, reg, _ := preparerAdmin(t)
	if _, err := reg.Creer("second", mdpEssai("second"), true); err != nil {
		t.Fatal(err)
	}
	doterDeFacteur(t, reg, "second")
	acces := a.opt.acces
	chef, _ := reg.Lire("chef")
	essayer := func(preuve string) (int, map[string]any) {
		n := anonymeDe(a, t, reg)
		n.cookies[cookiePreuve] = &http.Cookie{Name: cookiePreuve, Value: preuve}
		return n.faire("POST", routeSecondFacteur, map[string]any{"facteur": "totp", "code": codeSuivant(reg)})
	}
	if c, v := essayer(acces.preuve(chef, time.Now().Add(-time.Second))); c != 401 || v["reprendre"] != true {
		t.Fatalf("preuve expirée : %d %v", c, v)
	}
	if c, _ := essayer("n.importe.quoi"); c != 401 {
		t.Fatalf("preuve inventée : %d", c)
	}
	bonne := acces.preuve(chef, time.Now().Add(time.Minute))
	// signature d'un autre compte : le nom ne se change pas
	if c, _ := essayer(strings.Replace(bonne, ".chef.", ".second.", 1)); c != 401 {
		t.Fatalf("preuve détournée vers un autre compte : %d", c)
	}
	// d'avant un changement de mot de passe
	ancienne := acces.preuve(chef, time.Now().Add(time.Minute))
	if err := reg.DefinirMdp("chef", mdpEssai("chef")); err != nil {
		t.Fatal(err)
	}
	if c, _ := essayer(ancienne); c != 401 {
		t.Fatalf("preuve d'avant un changement de mot de passe : %d", c)
	}
	// rejouée : un bon code ne la fait pas resservir
	chef, _ = reg.Lire("chef")
	p := acces.preuve(chef, time.Now().Add(time.Minute))
	if c, v := essayer(p); c != 200 {
		t.Fatalf("première fois : %d %v", c, v)
	}
	if c, _ := essayer(p); c != 401 {
		t.Fatalf("rejeu : %d", c)
	}
	// un compte qui n'est plus administrateur, ou désactivé
	s, _ := reg.Lire("second")
	p = acces.preuve(s, time.Now().Add(time.Minute))
	if _, err := reg.Modifier("second", func(c *comptes.Compte) error { c.Inactif = true; return nil }); err != nil {
		t.Fatal(err)
	}
	if c, _ := essayer(p); c != 401 {
		t.Fatalf("compte désactivé : %d", c)
	}
	// cinq erreurs : la preuve meurt, même si le code suivant est bon
	chef, _ = reg.Lire("chef")
	p = acces.preuve(chef, time.Now().Add(time.Minute))
	n := anonymeDe(a, t, reg)
	n.cookies[cookiePreuve] = &http.Cookie{Name: cookiePreuve, Value: p}
	for i := 0; i < essaisMaxPreuve; i++ {
		if c, _ := n.faire("POST", routeSecondFacteur, map[string]any{"facteur": "totp", "code": "12345"}); c != 401 {
			t.Fatalf("erreur %d : %d", i, c)
		}
		n.cookies[cookiePreuve] = &http.Cookie{Name: cookiePreuve, Value: p}
		acces.oublierEssais("2f-compte:chef")
		acces.oublierEssais("2f-couple:chef|127.0.0.1")
	}
	if c, _ := n.faire("POST", routeSecondFacteur, map[string]any{"facteur": "totp", "code": codeSuivant(reg)}); c != 401 {
		t.Fatalf("preuve après cinq erreurs : %d", c)
	}
	n.cookies[cookiePreuve] = &http.Cookie{Name: cookiePreuve, Value: p}
	if c, _ := n.faire("POST", routeSecondFacteur, map[string]any{"facteur": "totp", "code": codeSuivant(reg)}); c != 401 {
		t.Fatalf("preuve morte, reprise : %d", c)
	}
}

// Connexion complète, et session de 12 heures sans jeton d'API. Un code ne sert qu'une fois.
func TestConnexionTOTP(t *testing.T) {
	a, reg, chef := preparerAdmin(t)
	if c, _ := chef.faire("GET", "/api/admin/comptes", nil); c != 200 {
		t.Fatalf("administration : %d", c)
	}
	if d := time.Until(chef.cookies[cookieMJ].Expires); d > dureeSessionAdmin+time.Minute || d < dureeSessionAdmin-time.Minute {
		t.Fatalf("durée de la session : %v", d)
	}
	if c, _ := chef.faire("POST", "/api/compte/jetons", map[string]any{"nom": "script"}); c != 409 {
		t.Fatalf("jeton d'un administrateur : %d", c)
	}
	// le code d'une connexion ne sert pas à une autre
	code := comptes.CodeTOTP(totpDeTest, horlogeDe(reg).avancer())
	n := anonymeDe(a, t, reg)
	n.faire("POST", "/api/acces/connexion", map[string]any{"nom": "chef", "mdp": mdpEssai("chef")})
	if c, v := n.faire("POST", routeSecondFacteur, map[string]any{"facteur": "totp", "code": code}); c != 200 {
		t.Fatalf("code : %d %v", c, v)
	}
	m := anonymeDe(a, t, reg)
	m.faire("POST", "/api/acces/connexion", map[string]any{"nom": "chef", "mdp": mdpEssai("chef")})
	if c, _ := m.faire("POST", routeSecondFacteur, map[string]any{"facteur": "totp", "code": code}); c != 401 {
		t.Fatalf("rejeu du code : %d", c)
	}
	if m.cookies[cookieMJ] != nil {
		t.Fatal("une session pour un code rejoué")
	}
}

// Les comptes MJ, auditeur et joueur ne voient aucun changement, et n'ont rien à enregistrer.
func TestAutresComptesInchanges(t *testing.T) {
	a, reg, _ := preparerAdmin(t)
	mj := connecterMJ(t, a, reg)
	if mj.cookies[cookieMJ] == nil || mj.cookies[cookiePreuve] != nil {
		t.Fatal("connexion d'un MJ : session attendue, sans preuve")
	}
	if len(strings.Split(mj.cookies[cookieMJ].Value, ".")) != 6 {
		t.Fatalf("session d'un MJ changée : %q", mj.cookies[cookieMJ].Value)
	}
	c, v := mj.faire("GET", "/api/compte", nil)
	if c != 200 || v["second_facteur"] != nil {
		t.Fatalf("compte d'un MJ : %d %v", c, v)
	}
	if c, _ := mj.faire("POST", prefixeSecondFacteur+"totp/debut", map[string]any{}); c != 403 {
		t.Fatalf("enregistrer un facteur sur un compte de MJ : %d", c)
	}
}

// routesDeclarees : toutes les routes du paquet, lues dans ses sources : une route ajoutée plus tard y
// est d'office.
func routesDeclarees(t *testing.T) [][2]string {
	t.Helper()
	jeu := token.NewFileSet()
	paquets, err := parser.ParseDir(jeu, ".", nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	motif := regexp.MustCompile(`^(GET|POST|PUT|PATCH|DELETE) (/\S*)$`)
	vues := map[[2]string]bool{}
	var l [][2]string
	ajouter := func(texte string) {
		if m := motif.FindStringSubmatch(texte); m != nil {
			x := [2]string{m[1], regexp.MustCompile(`\{[^}]*\}`).ReplaceAllString(m[2], "1")}
			if !vues[x] {
				vues[x] = true
				l = append(l, x)
			}
		}
	}
	for _, p := range paquets {
		for nom, f := range p.Files {
			if strings.HasSuffix(nom, "_test.go") {
				continue
			}
			ast.Inspect(f, func(x ast.Node) bool {
				b, ok := x.(*ast.BasicLit)
				if ok && b.Kind == token.STRING {
					if s, err := strconv.Unquote(b.Value); err == nil {
						ajouter(s)
					}
				}
				return true
			})
		}
	}
	// ceux qui se composent avec une constante
	for _, x := range [][2]string{{"POST", routeSecondFacteur}, {"POST", prefixeSecondFacteur + "totp/debut"},
		{"POST", prefixeSecondFacteur + "totp/activer"}, {"DELETE", prefixeSecondFacteur + "totp"}, {"POST", prefixeSecondFacteur + "secours"}} {
		ajouter(x[0] + " " + x[1])
	}
	if len(l) < 100 {
		t.Fatalf("seulement %d routes trouvées : la lecture des sources a changé", len(l))
	}
	return l
}

// Une session d'enrôlement n'ouvre que son compte, sur toutes les routes déclarées.
func TestEnrolementFerme(t *testing.T) {
	a, reg, _ := serveurMulti(t)
	id := a.st.ID()
	if _, err := reg.Creer("neuf", mdpEssai("neuf"), true); err != nil {
		t.Fatal(err)
	}
	n := anonymeDe(a, t, reg)
	c, v := n.faire("POST", "/api/acces/connexion", map[string]any{"nom": "neuf", "mdp": mdpEssai("neuf")})
	if c != 200 || v["etape"] != "enrolement" {
		t.Fatalf("connexion sans facteur : %d %v", c, v)
	}
	if d := time.Until(n.cookies[cookieMJ].Expires); d > dureeEnrolement+time.Minute {
		t.Fatalf("durée de la session d'enrôlement : %v", d)
	}
	ouvertes := map[string]bool{
		"GET /api/acces": true, "POST /api/acces/connexion": true, "POST /api/acces/deconnexion": true,
		"POST " + routeSecondFacteur: true, "GET /api/compte": true, "POST /api/compte/mdp": true, "GET /api/sante": true,
	}
	session := n.cookies[cookieMJ]
	for _, x := range routesDeclarees(t) {
		cle := x[0] + " " + x[1]
		if ouvertes[cle] || strings.HasPrefix(x[1], prefixeSecondFacteur) {
			continue
		}
		for _, entete := range [][]string{nil, {enTeteUnivers, id}} {
			n.cookies[cookieMJ] = session
			w := n.brut(x[0], x[1], map[string]any{}, entete...)
			var r map[string]any
			_ = json.Unmarshal(w.Body.Bytes(), &r)
			if w.Code != 403 || r["second_facteur_requis"] != true {
				t.Errorf("%s en enrôlement : %d %s", cle, w.Code, w.Body)
			}
		}
	}
	for _, u := range []string{"/medias/x.png"} {
		if c, _ := n.faire("GET", u, nil); c != 403 {
			t.Errorf("GET %s en enrôlement : %d", u, c)
		}
	}
	if c, v := n.faire("GET", "/api/acces", nil); c != 200 || v["compte"].(map[string]any)["second_facteur"].(map[string]any)["requis"] != true {
		t.Fatalf("état : %d %v", c, v)
	}
}

// Un anonyme n'atteint que ce qu'ouvre la page de connexion, sur toutes les routes déclarées.
func TestRoutesOuvertes(t *testing.T) {
	a, reg, _ := serveurMulti(t)
	permises := map[string]int{"GET /api/acces": 200, "POST /api/acces/connexion": 401, "POST " + routeSecondFacteur: 401, "POST /api/acces/deconnexion": 401}
	for _, x := range routesDeclarees(t) {
		cle := x[0] + " " + x[1]
		n := anonymeDe(a, t, reg)
		c, v := n.faire(x[0], x[1], map[string]any{})
		attendu, ok := permises[cle]
		if !ok {
			attendu = 401
		}
		if c != attendu {
			t.Errorf("%s anonyme : %d %v, attendu %d", cle, c, v, attendu)
		}
	}
}

// L'enrôlement, du mot de passe initial à la session complète.
func TestEnrolementTOTP(t *testing.T) {
	a, reg, _ := serveurMulti(t)
	if _, err := reg.Creer("neuf", mdpEssai("neuf"), true); err != nil {
		t.Fatal(err)
	}
	if err := reg.ExigerChangement("neuf"); err != nil {
		t.Fatal(err)
	}
	n := anonymeDe(a, t, reg)
	n.faire("POST", "/api/acces/connexion", map[string]any{"nom": "neuf", "mdp": mdpEssai("neuf")})
	// le mot de passe initial d'abord
	if c, v := n.faire("POST", prefixeSecondFacteur+"totp/debut", map[string]any{"confirmation": map[string]any{"mdp": mdpEssai("neuf")}}); c != 403 || v["changer_mdp"] != true {
		t.Fatalf("second facteur avant le mot de passe initial : %d %v", c, v)
	}
	if c, v := n.faire("GET", "/api/admin/comptes", nil); c != 403 || v["changer_mdp"] != true {
		t.Fatalf("administration : %d %v", c, v)
	}
	const mdp = "une-phrase-bien-longue"
	if c, v := n.faire("POST", "/api/compte/mdp", map[string]any{"ancien": mdpEssai("neuf"), "nouveau": mdp}); c != 200 {
		t.Fatalf("mot de passe initial : %d %v", c, v)
	}
	// changer le mot de passe ne donne pas le second facteur
	if c, v := n.faire("GET", "/api/admin/comptes", nil); c != 403 || v["second_facteur_requis"] != true {
		t.Fatalf("après le mot de passe, avant le facteur : %d %v", c, v)
	}
	if d := time.Until(n.cookies[cookieMJ].Expires); d > dureeEnrolement+time.Minute {
		t.Fatalf("la session d'enrôlement s'allonge : %v", d)
	}
	// une session volée ne suffit pas : il faut le mot de passe
	if c, _ := n.faire("POST", prefixeSecondFacteur+"totp/debut", map[string]any{}); c != 403 {
		t.Fatalf("sans confirmation : %d", c)
	}
	if c, _ := n.faire("POST", prefixeSecondFacteur+"totp/debut", map[string]any{"confirmation": map[string]any{"mdp": "pas-le-bon-mot-de-passe"}}); c != 401 {
		t.Fatalf("mauvais mot de passe : %d", c)
	}
	c, v := n.faire("POST", prefixeSecondFacteur+"totp/debut", map[string]any{"confirmation": map[string]any{"mdp": mdp}})
	if c != 200 {
		t.Fatalf("début : %d %v", c, v)
	}
	secret := v["secret"].(string)
	if !strings.HasPrefix(v["otpauth"].(string), "otpauth://totp/COF%20DM:neuf?") || !strings.HasPrefix(v["qr"].(string), "data:image/png;base64,") {
		t.Fatalf("montré : %v", v)
	}
	// rien d'actif tant qu'aucun code n'est venu
	if x, _ := reg.Lire("neuf"); x.AFacteur() {
		t.Fatal("actif avant le code")
	}
	if c, _ := n.faire("POST", prefixeSecondFacteur+"totp/activer", map[string]any{"code": "abcdef"}); c != 422 {
		t.Fatalf("code illisible : %d", c)
	}
	// une autre session ne reprend pas ce secret
	autre := anonymeDe(a, t, reg)
	autre.faire("POST", "/api/acces/connexion", map[string]any{"nom": "neuf", "mdp": mdp})
	code := comptes.CodeTOTP(secret, horlogeDe(reg).lire())
	if c, _ := autre.faire("POST", prefixeSecondFacteur+"totp/activer", map[string]any{"code": code}); c != 409 {
		t.Fatalf("secret repris par une autre session : %d", c)
	}
	avant := n.cookies[cookieMJ].Value
	c, v = n.faire("POST", prefixeSecondFacteur+"totp/activer", map[string]any{"code": code})
	if c != 200 {
		t.Fatalf("activation : %d %v", c, v)
	}
	if l, _ := v["secours"].([]any); len(l) != 10 {
		t.Fatalf("codes de secours : %v", v)
	}
	if c, _ := n.faire("GET", "/api/admin/comptes", nil); c != 200 {
		t.Fatalf("après l'activation : %d", c)
	}
	// la session d'enrôlement ne vaut plus rien : ni l'autre, ni l'ancien cookie de celle-ci
	if c, _ := autre.faire("GET", "/api/compte", nil); c != 401 {
		t.Fatalf("autre session d'enrôlement : %d", c)
	}
	vieux := anonymeDe(a, t, reg)
	vieux.cookies[cookieMJ] = &http.Cookie{Name: cookieMJ, Value: avant}
	if c, _ := vieux.faire("GET", "/api/compte", nil); c != 401 {
		t.Fatalf("ancien cookie : %d", c)
	}
	// le secret ne se revoit plus
	if c, v := n.faire("GET", "/api/compte", nil); c != 200 || strings.Contains(toJSON(v), secret) || strings.Contains(toJSON(v), "empreinte") {
		t.Fatalf("compte : %d %v", c, v)
	}
	if c, v := n.faire("GET", "/api/admin/comptes", nil); c != 200 || strings.Contains(toJSON(v), secret) {
		t.Fatalf("liste : %d %v", c, v)
	}
}

// Enregistrer ou remplacer un facteur, régénérer les codes : une session seule ne suffit pas.
func TestSessionVoleeSansFacteur(t *testing.T) {
	_, reg, chef := preparerAdmin(t)
	sans := map[string]any{}
	mdpSeul := map[string]any{"confirmation": map[string]any{"mdp": mdpEssai("chef")}}
	mauvais := map[string]any{"confirmation": map[string]any{"mdp": mdpEssai("chef"), "facteur": "totp", "code": "000000"}}
	for _, u := range []string{"totp/debut", "secours"} {
		if c, v := chef.faire("POST", prefixeSecondFacteur+u, sans); c != 403 || v["confirmation_requise"] != true {
			t.Fatalf("%s sans confirmation : %d %v", u, c, v)
		}
		if c, _ := chef.faire("POST", prefixeSecondFacteur+u, mdpSeul); c != 422 && c != 401 {
			t.Fatalf("%s avec le mot de passe seul : %d", u, c)
		}
		if c, _ := chef.faire("POST", prefixeSecondFacteur+u, mauvais); c != 401 {
			t.Fatalf("%s avec un mauvais code : %d", u, c)
		}
	}
	// le dernier facteur ne se retire pas, et aucun code n'est consommé pour le dire
	if c, _ := chef.faire("DELETE", prefixeSecondFacteur+"totp", map[string]any{"confirmation": confirmationDe(reg, "chef")}); c != 409 {
		t.Fatalf("dernier facteur : %d", c)
	}
	// avec les deux, les codes se régénèrent et un TOTP se remplace
	c, v := chef.faire("POST", prefixeSecondFacteur+"secours", map[string]any{"confirmation": confirmationDe(reg, "chef")})
	if l, _ := v["secours"].([]any); c != 200 || len(l) != 10 {
		t.Fatalf("régénération : %d %v", c, v)
	}
	c, v = chef.faire("POST", prefixeSecondFacteur+"totp/debut", map[string]any{"confirmation": confirmationDe(reg, "chef")})
	if c != 200 {
		t.Fatalf("remplacement : %d %v", c, v)
	}
	code := comptes.CodeTOTP(v["secret"].(string), horlogeDe(reg).lire())
	if c, v := chef.faire("POST", prefixeSecondFacteur+"totp/activer", map[string]any{"code": code}); c != 200 || v["secours"] != nil {
		t.Fatalf("remplacement : %d %v", c, v)
	}
	// l'ancien secret ne vaut plus
	if err := reg.VerifierTOTP("chef", codeSuivant(reg)); err == nil {
		t.Fatal("ancien secret accepté")
	}
}

// Les actions qui donnent la main sur un administrateur se confirment.
func TestActionsSensiblesConfirmees(t *testing.T) {
	_, reg, chef := preparerAdmin(t)
	const mdp = "une-phrase-bien-longue"
	if c, v := chef.faire("POST", "/api/admin/comptes", map[string]any{"nom": "intrus", "mdp": mdp, "admin": true}); c != 403 || v["confirmation_requise"] != true {
		t.Fatalf("créer un administrateur sans confirmer : %d %v", c, v)
	}
	if _, ok := reg.Lire("intrus"); ok {
		t.Fatal("administrateur créé sans confirmation")
	}
	// un compte ordinaire se crée toujours sans confirmer ; le promouvoir, non
	if c, _ := chef.faire("POST", "/api/admin/comptes", map[string]any{"nom": "lea", "mdp": mdp}); c != 201 {
		t.Fatalf("compte ordinaire : %d", c)
	}
	if c, _ := chef.faire("PATCH", "/api/admin/comptes/lea", map[string]any{"admin": true}); c != 403 {
		t.Fatalf("promotion sans confirmer : %d", c)
	}
	if c, _ := chef.faire("PATCH", "/api/admin/comptes/lea", map[string]any{"mdp": "encore-une-phrase-longue"}); c != 200 {
		t.Fatalf("mot de passe d'un compte ordinaire : %d", c)
	}
	if c, v := chef.faire("PATCH", "/api/admin/comptes/lea", map[string]any{"admin": true, "confirmation": confirmationDe(reg, "chef")}); c != 200 || v["second_facteur"] != false {
		t.Fatalf("promotion confirmée : %d %v", c, v)
	}
	// le mot de passe d'un administrateur
	if c, _ := chef.faire("PATCH", "/api/admin/comptes/lea", map[string]any{"mdp": mdp}); c != 403 {
		t.Fatalf("mot de passe d'un administrateur sans confirmer : %d", c)
	}
	if c, _ := chef.faire("PATCH", "/api/admin/comptes/lea", map[string]any{"mdp": mdp, "confirmation": confirmationDe(reg, "chef")}); c != 200 {
		t.Fatalf("mot de passe confirmé : %d", c)
	}
	// le supprimer
	if c, _ := chef.faire("DELETE", "/api/admin/comptes/lea", nil); c != 403 {
		t.Fatalf("suppression sans confirmer : %d", c)
	}
	if c, _ := chef.faire("DELETE", "/api/admin/comptes/lea", map[string]any{"confirmation": confirmationDe(reg, "chef")}); c != 200 {
		t.Fatalf("suppression confirmée : %d", c)
	}
	if c, v := chef.faire("POST", "/api/admin/comptes", map[string]any{"nom": "bis", "mdp": mdp, "admin": true, "confirmation": confirmationDe(reg, "chef")}); c != 201 {
		t.Fatalf("création confirmée : %d %v", c, v)
	}
}

// Un nouvel administrateur, promu avec une session encore courte, ne garde pas son accès.
func TestPromotionSansSecondFacteur(t *testing.T) {
	a, reg, _ := preparerAdmin(t)
	mj := connecterMJ(t, a, reg)
	acces := a.opt.acces
	// une session de MJ de moins de 12 heures : elle passait la vérification de la durée
	m, _ := reg.Lire("meneur")
	exp := time.Now().Add(time.Hour)
	mj.cookies[cookieMJ] = &http.Cookie{Name: cookieMJ, Value: acces.sessionCompte(m, "", exp)}
	if c, _ := mj.faire("GET", "/api/compte", nil); c != 200 {
		t.Fatalf("avant : %d", c)
	}
	if _, err := reg.Modifier("meneur", func(c *comptes.Compte) error { c.Admin = true; return nil }); err != nil {
		t.Fatal(err)
	}
	for _, u := range []string{"/api/admin/comptes", "/api/compte"} {
		if c, _ := mj.faire("GET", u, nil); c != 401 {
			t.Fatalf("GET %s, session d'avant la promotion : %d", u, c)
		}
	}
	// une session d'administrateur sans niveau, ou au niveau inventé (bien signée) : rien
	m, _ = reg.Lire("meneur")
	doterDeFacteur(t, reg, "meneur")
	m, _ = reg.Lire("meneur")
	for _, niveau := range []string{"", "autre"} {
		n := anonymeDe(a, t, reg)
		n.cookies[cookieMJ] = &http.Cookie{Name: cookieMJ, Value: acces.sessionCompte(m, niveau, exp)}
		if c, _ := n.faire("GET", "/api/admin/comptes", nil); c != 401 {
			t.Fatalf("niveau %q : %d", niveau, c)
		}
	}
	// une session d'enrôlement alors qu'il a un facteur : rien non plus
	n := anonymeDe(a, t, reg)
	n.cookies[cookieMJ] = &http.Cookie{Name: cookieMJ, Value: acces.sessionCompte(m, niveauEnrolement, time.Now().Add(time.Minute))}
	if c, _ := n.faire("GET", "/api/compte", nil); c != 401 {
		t.Fatalf("enrôlement avec un facteur : %d", c)
	}
	// un compte ordinaire avec une session à niveau : elle ne vaut rien
	if _, err := reg.Creer("simple", mdpEssai("simple"), false); err != nil {
		t.Fatal(err)
	}
	s, _ := reg.Lire("simple")
	s.Admin = true
	v := acces.sessionCompte(s, niveauComplet, exp)
	n = anonymeDe(a, t, reg)
	n.cookies[cookieMJ] = &http.Cookie{Name: cookieMJ, Value: v}
	if c, _ := n.faire("GET", "/api/compte", nil); c != 401 {
		t.Fatalf("compte ordinaire, session à niveau : %d", c)
	}
}

func TestLimitesDuSecondFacteur(t *testing.T) {
	a, reg, _ := preparerAdmin(t)
	depuis := func(ip string) []string { return []string{"X-Forwarded-For", ip} }
	mdpPuis := func(ip string) *navigateur {
		n := anonymeDe(a, t, reg)
		if w := n.brut("POST", "/api/acces/connexion", map[string]any{"nom": "chef", "mdp": mdpEssai("chef")}, depuis(ip)...); w.Code != 200 {
			t.Fatalf("mot de passe : %d", w.Code)
		}
		return n
	}
	// sans preuve, personne n'atteint un compteur : des milliers d'essais ne gênent pas le titulaire
	tiers := anonymeDe(a, t, reg)
	for i := 0; i < 300; i++ {
		if w := tiers.brut("POST", routeSecondFacteur, map[string]any{"facteur": "totp", "code": "000000"}, depuis("198.51.100.1")...); w.Code != 401 {
			t.Fatalf("essai %d sans preuve : %d", i, w.Code)
		}
	}
	a.opt.acces.mu.Lock()
	for k := range a.opt.acces.echecs {
		if strings.HasPrefix(k, "2f-") {
			t.Errorf("compteur touché sans preuve : %s", k)
		}
	}
	a.opt.acces.mu.Unlock()
	n := mdpPuis("127.0.0.1")
	if c, v := n.faire("POST", routeSecondFacteur, map[string]any{"facteur": "totp", "code": codeSuivant(reg)}); c != 200 {
		t.Fatalf("le titulaire après ces essais : %d %v", c, v)
	}
	acces := a.opt.acces
	acces.oublierEssais("2f-ip:127.0.0.1")
	// les erreurs d'une adresse ne ferment pas l'étape à une autre
	autre := func(ip string, code string) int {
		n := mdpPuis(ip)
		return n.brut("POST", routeSecondFacteur, map[string]any{"facteur": "totp", "code": code}, depuis(ip)...).Code
	}
	for i := 0; i < essaisMax2FCouple; i++ {
		if c := autre("198.51.100.2", "000000"); c != 401 {
			t.Fatalf("erreur %d : %d", i, c)
		}
	}
	if c := autre("198.51.100.2", "000000"); c != 429 {
		t.Fatalf("limite du couple : %d", c)
	}
	if c := autre("198.51.100.3", codeSuivant(reg)); c != 200 {
		t.Fatalf("une autre adresse : %d", c)
	}
	// par compte, le TOTP s'arrête, d'où que viennent les essais
	for i := 0; i < essaisMaxTOTPCompte; i++ {
		ip := "198.51.100." + strconv.Itoa(10+i)
		if c := autre(ip, "000000"); c != 401 {
			t.Fatalf("erreur par compte %d : %d", i, c)
		}
	}
	if c := autre("198.51.100.50", codeSuivant(reg)); c != 429 {
		t.Fatalf("limite du compte : %d", c)
	}
	// mais le titulaire entre encore : un code de secours ne se devine pas et ne compte pas là
	secours, err := reg.RegenererSecours("chef")
	if err != nil {
		t.Fatal(err)
	}
	s := mdpPuis("198.51.100.60")
	w := s.brut("POST", routeSecondFacteur, map[string]any{"facteur": "secours", "code": secours[0]}, depuis("198.51.100.60")...)
	if w.Code != 200 {
		t.Fatalf("code de secours malgré la limite du TOTP : %d %s", w.Code, w.Body)
	}
	// et ce succès rend le TOTP au titulaire
	if c := autre("198.51.100.61", codeSuivant(reg)); c != 200 {
		t.Fatalf("TOTP après ce succès : %d", c)
	}
	// le quota sur une journée se garde au-delà des cinq minutes
	if !acces.reserverEssaiSur("essai-jour", 1, dureeTOTPJour) || acces.reserverEssaiSur("essai-jour", 1, dureeTOTPJour) {
		t.Fatal("quota d'une journée")
	}
	acces.mu.Lock()
	acces.echecs["essai-jour"][0] = time.Now().Add(-time.Hour)
	acces.mu.Unlock()
	acces.reserverEssai("autre-chose", 5)
	if acces.reserverEssaiSur("essai-jour", 1, dureeTOTPJour) {
		t.Fatal("essai d'une journée oublié au bout d'une heure")
	}
	acces.mu.Lock()
	acces.echecs["essai-jour"][0] = time.Now().Add(-25 * time.Hour)
	acces.mu.Unlock()
	if !acces.reserverEssaiSur("essai-jour", 1, dureeTOTPJour) {
		t.Fatal("essai de plus d'une journée encore compté")
	}
}

// Un code de secours : une seule fois, et à la connexion comme à la confirmation.
func TestCodeDeSecoursConnexion(t *testing.T) {
	a, reg, _ := preparerAdmin(t)
	codes, err := reg.RegenererSecours("chef")
	if err != nil {
		t.Fatal(err)
	}
	entrer := func(code string) (int, *navigateur) {
		n := anonymeDe(a, t, reg)
		n.faire("POST", "/api/acces/connexion", map[string]any{"nom": "chef", "mdp": mdpEssai("chef")})
		c, _ := n.faire("POST", routeSecondFacteur, map[string]any{"facteur": "secours", "code": code})
		return c, n
	}
	c, n := entrer(codes[0])
	if c != 200 {
		t.Fatalf("code : %d", c)
	}
	if c, _ := entrer(codes[0]); c != 401 {
		t.Fatalf("code réutilisé : %d", c)
	}
	// perdu le téléphone : le code de secours permet de confirmer un nouveau TOTP
	c, v := n.faire("POST", prefixeSecondFacteur+"totp/debut", map[string]any{"confirmation": map[string]any{"mdp": mdpEssai("chef"), "facteur": "secours", "code": codes[1]}})
	if c != 200 {
		t.Fatalf("nouveau TOTP avec un code de secours : %d %v", c, v)
	}
	if c, v := n.faire("GET", "/api/compte", nil); c != 200 || v["second_facteur"].(map[string]any)["secours_restants"] != float64(8) {
		t.Fatalf("restants : %d %v", c, v)
	}
}

// Réinitialiser le second facteur d'un autre : refus sur soi-même, trace au journal, sessions fermées.
func TestReinitialisationParUnAutre(t *testing.T) {
	buf := journalDe(t)
	a, reg, chef := preparerAdmin(t)
	if _, err := reg.Creer("second", mdpEssai("second"), true); err != nil {
		t.Fatal(err)
	}
	doterDeFacteur(t, reg, "second")
	if _, err := reg.Creer("simple", mdpEssai("simple"), false); err != nil {
		t.Fatal(err)
	}
	s := anonymeDe(a, t, reg)
	s.faire("POST", "/api/acces/connexion", map[string]any{"nom": "second", "mdp": mdpEssai("second")})
	s.finirConnexionAdmin("second")
	route := func(nom string) string { return "/api/admin/comptes/" + nom + "/second-facteur/reinitialiser" }
	if c, _ := chef.faire("POST", route("chef"), map[string]any{"confirmation": confirmationDe(reg, "chef")}); c != 403 {
		t.Fatalf("sur soi-même : %d", c)
	}
	if c, _ := chef.faire("POST", route("CHEF"), map[string]any{"confirmation": confirmationDe(reg, "chef")}); c != 403 {
		t.Fatalf("sur soi-même, en majuscules : %d", c)
	}
	if c, _ := chef.faire("POST", route("second"), map[string]any{}); c != 403 {
		t.Fatalf("sans confirmation : %d", c)
	}
	if c, _ := chef.faire("POST", route("simple"), map[string]any{"confirmation": confirmationDe(reg, "chef")}); c != 409 {
		t.Fatalf("un compte ordinaire : %d", c)
	}
	if c, _ := chef.faire("POST", route("inconnu"), map[string]any{"confirmation": confirmationDe(reg, "chef")}); c != 404 {
		t.Fatalf("inconnu : %d", c)
	}
	mj := connecterMJ(t, a, reg)
	if c, _ := mj.faire("POST", route("second"), map[string]any{}); c != 403 {
		t.Fatalf("par un MJ : %d", c)
	}
	if c, v := chef.faire("POST", route("second"), map[string]any{"confirmation": confirmationDe(reg, "chef")}); c != 200 || v["second_facteur"] != false {
		t.Fatalf("réinitialisation : %d %v", c, v)
	}
	if c, _ := s.faire("GET", "/api/compte", nil); c != 401 {
		t.Fatalf("session d'avant : %d", c)
	}
	l := lignesSecurite(buf)
	if !contient(l, `securite evenement=second_facteur_reinitialise par="chef" nom="second" ip=127.0.0.1`) {
		t.Errorf("pas au journal :\n%s", strings.Join(l, "\n"))
	}
	// il repasse par l'enrôlement, son mot de passe n'ayant pas changé
	n := anonymeDe(a, t, reg)
	if c, v := n.faire("POST", "/api/acces/connexion", map[string]any{"nom": "second", "mdp": mdpEssai("second")}); c != 200 || v["etape"] != "enrolement" {
		t.Fatalf("connexion après : %d %v", c, v)
	}
}

// Le journal note chaque étape, sans jamais un secret, un code ou un mot de passe.
func TestJournalSecondFacteur(t *testing.T) {
	buf := journalDe(t)
	a, reg, _ := serveurMulti(t)
	if _, err := reg.Creer("neuf", mdpEssai("neuf"), true); err != nil {
		t.Fatal(err)
	}
	n := anonymeDe(a, t, reg)
	n.faire("POST", "/api/acces/connexion", map[string]any{"nom": "neuf", "mdp": mdpEssai("neuf")})
	conf := map[string]any{"mdp": mdpEssai("neuf")}
	_, v := n.faire("POST", prefixeSecondFacteur+"totp/debut", map[string]any{"confirmation": conf})
	secret := v["secret"].(string)
	code := comptes.CodeTOTP(secret, horlogeDe(reg).lire())
	_, v = n.faire("POST", prefixeSecondFacteur+"totp/activer", map[string]any{"code": code})
	secours := v["secours"].([]any)
	m := anonymeDe(a, t, reg)
	m.faire("POST", "/api/acces/connexion", map[string]any{"nom": "neuf", "mdp": mdpEssai("neuf")})
	m.faire("POST", routeSecondFacteur, map[string]any{"facteur": "totp", "code": "999999"})
	m.faire("POST", routeSecondFacteur, map[string]any{"facteur": "secours", "code": secours[0]})
	m.faire("POST", prefixeSecondFacteur+"secours", map[string]any{"confirmation": map[string]any{"mdp": "MDP-FAUX-A-NE-PAS-VOIR"}})
	m.faire("POST", prefixeSecondFacteur+"secours", map[string]any{"confirmation": map[string]any{"mdp": mdpEssai("neuf"), "facteur": "secours", "code": secours[1]}})
	l := lignesSecurite(buf)
	for _, attendu := range []string{
		`securite evenement=enrolement_ouvert nom="neuf" ip=127.0.0.1`,
		`securite evenement=second_facteur_active facteur=totp nom="neuf" ip=127.0.0.1`,
		`securite evenement=connexion_ok facteur=totp nom="neuf" ip=127.0.0.1`,
		`securite evenement=second_facteur_demande nom="neuf" ip=127.0.0.1`,
		`securite evenement=second_facteur_echec facteur=totp nom="neuf" ip=127.0.0.1`,
		`securite evenement=secours_utilise restants=9 nom="neuf" ip=127.0.0.1`,
		`securite evenement=connexion_ok facteur=secours nom="neuf" ip=127.0.0.1`,
		`securite evenement=confirmation_echec nom="neuf" ip=127.0.0.1`,
		`securite evenement=secours_regeneres nom="neuf" ip=127.0.0.1`,
	} {
		if !contient(l, attendu) {
			t.Errorf("manque %s dans\n%s", attendu, strings.Join(l, "\n"))
		}
	}
	tout := buf.String()
	for _, interdit := range []string{secret, code, secours[0].(string), secours[1].(string), mdpEssai("neuf"), "MDP-FAUX-A-NE-PAS-VOIR", "999999"} {
		if strings.Contains(tout, interdit) {
			t.Errorf("%q au journal :\n%s", interdit, tout)
		}
	}
	// le fichier des comptes ne garde ni le secret ni les codes en clair
	b := lireFichier(t, filepath.Join(a.opt.DossierDon, "comptes.json"))
	for _, interdit := range []string{secret, secours[2].(string)} {
		if strings.Contains(b, interdit) {
			t.Errorf("%q en clair dans comptes.json", interdit)
		}
	}
}

func lireFichier(t *testing.T, chemin string) string {
	t.Helper()
	b, err := os.ReadFile(chemin)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
