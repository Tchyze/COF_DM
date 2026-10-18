package serveur

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// appel d'un navigateur du réseau (tablette), avec le cookie de session s'il en a un.
func (b *banc) tablette(methode, url string, corps any, cookie *http.Cookie) (int, map[string]any, *http.Cookie) {
	b.t.Helper()
	j, _ := json.Marshal(corps)
	r := httptest.NewRequest(methode, url, bytes.NewReader(j))
	r.Host, r.RemoteAddr = "192.168.1.20:8100", "192.168.1.50:5000"
	if methode != "GET" {
		r.Header.Set("X-COF-DM", "1")
	}
	if cookie != nil {
		r.AddCookie(cookie)
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

func bancReseau(t *testing.T) (*banc, *bool) {
	enregistre := new(bool)
	b := nouveauAvec(t, Options{Port: 8100, Reseau: true, Config: func(v bool) error { *enregistre = v; return nil }})
	b.s.acces.pause = 0
	return b, enregistre
}

// Sans ouverture au réseau, rien ne change : aucun navigateur du réseau n'obtient quoi que ce soit,
// même si un mot de passe existe.
func TestReseauFerme(t *testing.T) {
	b := nouveau(t)
	if c, _ := b.local("POST", "/api/acces/mdp", map[string]any{"nouveau": "dragon-vert"}); c != 200 {
		t.Fatalf("mot de passe : %d", c)
	}
	for _, u := range []string{"/", "/api/acces", "/api/schema", "/api/sante"} {
		if c, _, _ := b.tablette("GET", u, nil, nil); c != 401 {
			t.Fatalf("GET %s réseau fermé : %d", u, c)
		}
	}
	if c, _, cookie := b.tablette("POST", "/api/acces/connexion", map[string]any{"mdp": "dragon-vert"}, nil); c != 401 || cookie != nil {
		t.Fatalf("connexion réseau fermé : %d", c)
	}
}

func TestAccesTablette(t *testing.T) {
	b, _ := bancReseau(t)
	// pas connecté : l'interface et l'état de l'accès, rien d'autre
	if c, _, _ := b.tablette("GET", "/", nil, nil); c != 200 {
		t.Fatalf("page de connexion : %d", c)
	}
	c, v, _ := b.tablette("GET", "/api/acces", nil, nil)
	if c != 200 || v["mj"] != false || v["mdp_defini"] != false || v["adresses"] != nil || v["reseau"] != nil {
		t.Fatalf("état vu par un inconnu : %d %v", c, v)
	}
	for _, u := range []string{"/api/schema", "/api/entites", "/api/export", "/api/parametres", "/api/sante", "/api/acces/qr?url=http://x", "/medias/0123456789abcdef0123.png"} {
		if c, _, _ := b.tablette("GET", u, nil, nil); c != 401 {
			t.Fatalf("GET %s sans connexion : %d", u, c)
		}
	}
	if c, _, _ := b.tablette("POST", "/api/entites", map[string]any{"type": "pnj", "nom": "X"}, nil); c != 401 {
		t.Fatalf("écriture sans connexion : %d", c)
	}
	// sans mot de passe défini, personne n'entre, et il ne se définit pas depuis le réseau
	if c, _, cookie := b.tablette("POST", "/api/acces/connexion", map[string]any{"mdp": ""}, nil); c != 409 || cookie != nil {
		t.Fatalf("connexion sans mot de passe défini : %d", c)
	}
	if c, _, _ := b.tablette("POST", "/api/acces/mdp", map[string]any{"nouveau": "pirate-pirate"}, nil); c != 401 {
		t.Fatalf("mot de passe défini depuis le réseau : %d", c)
	}
	// le MJ le définit sur son ordinateur
	if c, _ := b.local("POST", "/api/acces/mdp", map[string]any{"nouveau": "court"}); c != 422 {
		t.Fatalf("mot de passe trop court : %d", c)
	}
	if c, _ := b.local("POST", "/api/acces/mdp", map[string]any{"nouveau": "dragon-vert"}); c != 200 {
		t.Fatalf("mot de passe : %d", c)
	}
	if v, _ := b.s.store.Meta(metaMdp); v == "" || strings.Contains(v, "dragon") {
		t.Fatalf("mot de passe gardé en clair ou absent : %q", v)
	}
	if c, _, cookie := b.tablette("POST", "/api/acces/connexion", map[string]any{"mdp": "dragon-rouge"}, nil); c != 401 || cookie != nil {
		t.Fatalf("mauvais mot de passe : %d", c)
	}
	c, _, cookie := b.tablette("POST", "/api/acces/connexion", map[string]any{"mdp": "dragon-vert"}, nil)
	if c != 200 || cookie == nil || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("connexion : %d %v", c, cookie)
	}
	// connecté : l'application entière, les écritures avec l'en-tête, l'auteur dans le journal
	c, v, _ = b.tablette("GET", "/api/acces", nil, cookie)
	if c != 200 || v["mj"] != true || v["local"] != false || v["reseau"] != true {
		t.Fatalf("état vu par la tablette : %d %v", c, v)
	}
	if c, _, _ := b.tablette("GET", "/api/schema", nil, cookie); c != 200 {
		t.Fatalf("lecture connectée : %d", c)
	}
	if c, _, _ := b.tablette("POST", "/api/entites", map[string]any{"type": "pnj", "nom": "Gautier"}, cookie); c != 201 {
		t.Fatalf("écriture connectée : %d", c)
	}
	r := httptest.NewRequest("POST", "/api/entites", strings.NewReader(`{"type":"pnj","nom":"Y"}`))
	r.Host, r.RemoteAddr = "192.168.1.20:8100", "192.168.1.50:5000"
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	b.s.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("écriture connectée sans en-tête X-COF-DM : %d", w.Code)
	}
	if _, j := b.local("GET", "/api/journal", nil); !strings.Contains(j["_liste"].(string), `"auteur":"mj (réseau)"`) {
		t.Fatalf("auteur absent du journal : %v", j)
	}
	// ce qui garde la porte reste sur l'ordinateur du MJ
	if c, v, _ := b.tablette("GET", "/api/parametres", nil, cookie); c != 200 || v["jeton"] != "" {
		t.Fatalf("jeton d'API lu depuis la tablette : %d %v", c, v)
	}
	for _, x := range []struct {
		url   string
		corps any
	}{{"/api/parametres/jeton", nil}, {"/api/acces/mdp", map[string]any{"nouveau": "autre-chose"}}, {"/api/acces/reseau", map[string]any{"reseau": false}}} {
		if c, _, _ := b.tablette("POST", x.url, x.corps, cookie); c != 403 {
			t.Fatalf("POST %s depuis la tablette : %d", x.url, c)
		}
	}
	if _, v := b.local("GET", "/api/parametres", nil); v["jeton"] != b.jeton {
		t.Fatalf("jeton vu de l'ordinateur du MJ : %v", v)
	}
	// un nom de domaine d'internet qui pointe ici n'obtient rien, même avec le cookie
	r = httptest.NewRequest("GET", "/api/schema", nil)
	r.Host, r.RemoteAddr = "pirate.example:8100", "192.168.1.50:5000"
	r.AddCookie(cookie)
	w = httptest.NewRecorder()
	b.s.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatalf("nom de domaine étranger : %d", w.Code)
	}
	// session falsifiée ou périmée
	faux := *cookie
	faux.Value = strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10) + "." + strings.SplitN(cookie.Value, ".", 2)[1]
	if c, _, _ := b.tablette("GET", "/api/schema", nil, &faux); c != 401 {
		t.Fatalf("session falsifiée : %d", c)
	}
	perime := *cookie
	perime.Value = b.s.acces.jeton(time.Now().Add(-time.Minute))
	if c, _, _ := b.tablette("GET", "/api/schema", nil, &perime); c != 401 {
		t.Fatalf("session périmée : %d", c)
	}
	// déconnexion : le navigateur oublie le cookie
	if c, _, efface := b.tablette("POST", "/api/acces/deconnexion", nil, cookie); c != 200 || efface == nil || efface.MaxAge >= 0 {
		t.Fatalf("déconnexion : %d %v", c, efface)
	}
	if c, _, _ := b.tablette("GET", "/api/schema", nil, cookie); c != 401 {
		t.Fatalf("témoin rejoué après la déconnexion : %d", c)
	}
	// … et les autres appareils restent connectés
	_, _, autre := b.tablette("POST", "/api/acces/connexion", map[string]any{"mdp": "dragon-vert"}, nil)
	if c, _, _ := b.tablette("GET", "/api/schema", nil, autre); c != 200 {
		t.Fatalf("autre appareil après une déconnexion : %d", c)
	}
	cookie = autre
	// changer le mot de passe déconnecte les appareils
	if c, _ := b.local("POST", "/api/acces/mdp", map[string]any{"nouveau": "dragon-bleu"}); c != 200 {
		t.Fatalf("changement : %d", c)
	}
	if c, _, _ := b.tablette("GET", "/api/schema", nil, cookie); c != 401 {
		t.Fatalf("ancienne session après changement de mot de passe : %d", c)
	}
	// le retirer ferme la porte
	c, _, cookie = b.tablette("POST", "/api/acces/connexion", map[string]any{"mdp": "dragon-bleu"}, nil)
	if c != 200 {
		t.Fatalf("connexion avec le nouveau mot de passe : %d", c)
	}
	if c, v := b.local("POST", "/api/acces/mdp", map[string]any{"nouveau": ""}); c != 200 || v["mdp_defini"] != false {
		t.Fatalf("retrait : %d %v", c, v)
	}
	if c, _, _ := b.tablette("GET", "/api/schema", nil, cookie); c != 401 {
		t.Fatalf("session après retrait du mot de passe : %d", c)
	}
}

func TestEssaisLimites(t *testing.T) {
	b, _ := bancReseau(t)
	b.local("POST", "/api/acces/mdp", map[string]any{"nouveau": "dragon-vert"})
	for i := 0; i < essaisMax; i++ {
		if c, _, _ := b.tablette("POST", "/api/acces/connexion", map[string]any{"mdp": "essai-" + strconv.Itoa(i)}, nil); c != 401 {
			t.Fatalf("essai %d : %d", i, c)
		}
	}
	// même le bon mot de passe attend, pour cette adresse seulement
	if c, _, cookie := b.tablette("POST", "/api/acces/connexion", map[string]any{"mdp": "dragon-vert"}, nil); c != 429 || cookie != nil {
		t.Fatalf("après trop d'essais : %d", c)
	}
	r := httptest.NewRequest("POST", "/api/acces/connexion", strings.NewReader(`{"mdp":"dragon-vert"}`))
	r.Host, r.RemoteAddr = "192.168.1.20:8100", "192.168.1.77:5000"
	r.Header.Set("X-COF-DM", "1")
	w := httptest.NewRecorder()
	b.s.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("autre appareil pendant le blocage : %d", w.Code)
	}
}

func TestReglageReseau(t *testing.T) {
	b, enregistre := bancReseau(t)
	c, v := b.local("GET", "/api/acces", nil)
	if c != 200 || v["mj"] != true || v["local"] != true || v["reseau"] != true || v["reseau_voulu"] != false || v["reglable"] != true {
		t.Fatalf("état vu du MJ : %d %v", c, v)
	}
	if c, v = b.local("POST", "/api/acces/reseau", map[string]any{"reseau": true}); c != 200 || !*enregistre || v["reseau_voulu"] != true {
		t.Fatalf("réglage : %d %v (enregistré : %v)", c, v, *enregistre)
	}
	if c, _ = b.local("GET", "/api/acces/qr?url=http://192.168.1.20:8100/%23/table", nil); c != 200 {
		t.Fatalf("QR code : %d", c)
	}
	if c, _ = b.local("GET", "/api/acces/qr?url=javascript:alert(1)", nil); c != 400 {
		t.Fatalf("QR code d'une adresse invalide : %d", c)
	}
	// sans fonction d'enregistrement, le réglage est refusé
	if c, _ = nouveau(t).local("POST", "/api/acces/reseau", map[string]any{"reseau": true}); c != 409 {
		t.Fatalf("réglage non enregistrable : %d", c)
	}
}

func TestHoteDuReseau(t *testing.T) {
	for hote, attendu := range map[string]bool{"192.168.1.20:8100": true, "10.0.0.4": true, "[fd00::1]:8100": true, "pc-du-mj:8100": true,
		"pc-du-mj.local:8100": true, "PC-DU-MJ.LOCAL": true, "pirate.example:8100": false, "cof.pirate.com": false, "": false} {
		r := httptest.NewRequest("GET", "/", nil)
		r.Host = hote
		if hoteDuReseau(r) != attendu {
			t.Errorf("hôte %q : attendu %v", hote, attendu)
		}
	}
}

// Ce qui garde la porte ne se règle que sur l'ordinateur du MJ : depuis le réseau, ni la session ni
// le jeton d'API ne changent le mot de passe, l'ouverture au réseau ou le jeton.
func TestReglagesSurPlace(t *testing.T) {
	b, enregistre := bancReseau(t)
	b.local("POST", "/api/acces/mdp", map[string]any{"nouveau": "dragon-vert"})
	b.local("POST", "/api/acces/reseau", map[string]any{"reseau": true})
	_, _, cookie := b.tablette("POST", "/api/acces/connexion", map[string]any{"mdp": "dragon-vert"}, nil)
	for _, x := range []struct {
		methode, url string
		corps        any
	}{
		{"POST", "/api/parametres/jeton", nil},
		{"POST", "/api/acces/mdp", map[string]any{"nouveau": "pirate-pirate"}},
		{"POST", "/api/acces/reseau", map[string]any{"reseau": false}},
	} {
		if c, _ := b.api(x.methode, x.url, x.corps); c != 403 {
			t.Fatalf("%s %s avec le jeton, depuis le réseau : %d", x.methode, x.url, c)
		}
		if c, _, _ := b.tablette(x.methode, x.url, x.corps, cookie); c != 403 {
			t.Fatalf("%s %s depuis la tablette : %d", x.methode, x.url, c)
		}
	}
	if !b.s.acces.verifier("dragon-vert") || !*enregistre {
		t.Fatal("un réglage a changé depuis le réseau")
	}
	if j, _ := b.s.reg.Meta("api_jeton"); j != b.jeton {
		t.Fatal("le jeton a été renouvelé depuis le réseau")
	}
}

// Des essais envoyés en même temps ne passent pas sous la limite, et un bon mot de passe ne compte
// pas comme un essai.
func TestEssaisEnRafale(t *testing.T) {
	b, _ := bancReseau(t)
	b.local("POST", "/api/acces/mdp", map[string]any{"nouveau": "dragon-vert"})
	for i := 0; i < 2*essaisMax; i++ {
		if c, _, _ := b.tablette("POST", "/api/acces/connexion", map[string]any{"mdp": "dragon-vert"}, nil); c != 200 {
			t.Fatalf("connexion %d avec le bon mot de passe : %d", i, c)
		}
	}
	var refus, bloques atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 5*essaisMax; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := httptest.NewRequest("POST", "/api/acces/connexion", strings.NewReader(`{"mdp":"essai-faux"}`))
			r.Host, r.RemoteAddr = "192.168.1.20:8100", "192.168.1.50:5000"
			r.Header.Set("X-COF-DM", "1")
			w := httptest.NewRecorder()
			b.s.ServeHTTP(w, r)
			switch w.Code {
			case 401:
				refus.Add(1)
			case 429:
				bloques.Add(1)
			}
		}()
	}
	wg.Wait()
	if refus.Load() != essaisMax || bloques.Load() != 4*essaisMax {
		t.Fatalf("essais vérifiés : %d (attendu %d), bloqués : %d", refus.Load(), essaisMax, bloques.Load())
	}
}

// Les réponses interdisent l'affichage de l'application dans le cadre d'un autre site.
func TestEnTetes(t *testing.T) {
	b := nouveau(t)
	for _, u := range []string{"/", "/api/sante"} {
		r := httptest.NewRequest("GET", u, nil)
		r.Host, r.RemoteAddr = "127.0.0.1:8100", "127.0.0.1:5000"
		w := httptest.NewRecorder()
		b.s.ServeHTTP(w, r)
		if w.Header().Get("X-Frame-Options") != "SAMEORIGIN" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("%s : en-têtes %v", u, w.Header())
		}
	}
	r := httptest.NewRequest("GET", "/", nil)
	r.Host, r.RemoteAddr = "127.0.0.1:8100", "127.0.0.1:5000"
	w := httptest.NewRecorder()
	b.s.ServeHTTP(w, r)
	if csp := w.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "frame-ancestors 'self'") || !strings.Contains(csp, "form-action 'self'") {
		t.Fatalf("politique de sécurité : %s", csp)
	}
	if c, _ := b.local("POST", "/api/entites", `{"type":"pnj","nom":"`+strings.Repeat("x", tailleMaxJSON)+`"}`); c != 422 {
		t.Fatalf("requête trop lourde : %d", c)
	}
}
