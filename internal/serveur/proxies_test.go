package serveur

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tchyze/cof_dm/internal/comptes"
)

func accesAvec(t *testing.T, proxies string) *acces {
	t.Helper()
	reg, err := comptes.Ouvrir(filepath.Join(t.TempDir(), "comptes.json"))
	if err != nil {
		t.Fatal(err)
	}
	opt := Options{Comptes: reg}
	if proxies != "" {
		if opt.Proxies, err = LireProxies(proxies); err != nil {
			t.Fatal(err)
		}
	}
	return nouveauAvec(t, opt).s.acces
}

func requeteDe(pair, xff, proto string) *http.Request {
	r := httptest.NewRequest("GET", "/api/acces", nil)
	r.RemoteAddr = pair
	if xff != "" {
		r.Header.Set("X-Forwarded-For", xff)
	}
	if proto != "" {
		r.Header.Set("X-Forwarded-Proto", proto)
	}
	return r
}

func TestProxiesAdresse(t *testing.T) {
	const reseauProxy = "10.0.0.0/24,fd00:cafe::/64"
	for _, c := range []struct{ nom, proxies, pair, xff, attendu string }{
		{"proxy de confiance", reseauProxy, "10.0.0.5:1", "203.0.113.9", "203.0.113.9"},
		{"client direct qui forge l'en-tête", reseauProxy, "198.51.100.7:1", "203.0.113.9", "198.51.100.7"},
		{"une adresse locale n'est plus de confiance d'office", reseauProxy, "127.0.0.1:1", "203.0.113.9", "127.0.0.1"},
		{"tête de liste forgée à travers le proxy", reseauProxy, "10.0.0.5:1", "1.2.3.4, 203.0.113.9", "203.0.113.9"},
		{"adresse de proxy forgée à travers le proxy", reseauProxy, "10.0.0.5:1", "10.0.0.8, 203.0.113.9", "203.0.113.9"},
		{"chaîne de proxys", reseauProxy, "10.0.0.5:1", "203.0.113.9, 10.0.0.8", "203.0.113.9"},
		{"chaîne de proxys avec tête forgée", reseauProxy, "10.0.0.5:1", "1.2.3.4, 203.0.113.9, 10.0.0.8, 10.0.0.7", "203.0.113.9"},
		{"chaîne entière de confiance", reseauProxy, "10.0.0.5:1", "10.0.0.9, 10.0.0.8", "10.0.0.9"},
		{"en-tête absent", reseauProxy, "10.0.0.5:1", "", "10.0.0.5"},
		{"en-tête invalide", reseauProxy, "10.0.0.5:1", "n'importe quoi", "10.0.0.5"},
		{"entrée invalide après un proxy", reseauProxy, "10.0.0.5:1", "203.0.113.9, n'importe quoi, 10.0.0.8", "10.0.0.8"},
		{"IPv6 : proxy de confiance", reseauProxy, "[fd00:cafe::2]:1", "2001:db8::1", "2001:db8::1"},
		{"IPv6 : client direct qui forge l'en-tête", reseauProxy, "[2001:db8::2]:1", "203.0.113.9", "2001:db8::2"},
		{"IPv6 : forme réduite ramenée à une seule écriture", reseauProxy, "10.0.0.5:1", "2001:DB8:0:0::1", "2001:db8::1"},
		{"IPv4 écrite en IPv6 par le proxy", reseauProxy, "[::ffff:10.0.0.5]:1", "::ffff:203.0.113.9", "203.0.113.9"},
		{"défaut : nginx sur cette machine", "", "127.0.0.1:1", "203.0.113.9", "203.0.113.9"},
		{"défaut : nginx sur cette machine en IPv6", "", "[::1]:1", "203.0.113.9", "203.0.113.9"},
		{"défaut : client du réseau qui forge l'en-tête", "", "10.0.0.5:1", "203.0.113.9", "10.0.0.5"},
	} {
		t.Run(c.nom, func(t *testing.T) {
			if v := accesAvec(t, c.proxies).adresse(requeteDe(c.pair, c.xff, "")); v != c.attendu {
				t.Fatalf("pair %s, X-Forwarded-For %q : %s, attendu %s", c.pair, c.xff, v, c.attendu)
			}
		})
	}
}

func TestProxiesHTTPS(t *testing.T) {
	const reseauProxy = "10.0.0.0/24"
	for _, c := range []struct {
		nom, proxies, pair, xff, proto string
		attendu                        bool
	}{
		{"proxy de confiance", reseauProxy, "10.0.0.5:1", "203.0.113.9", "https", true},
		{"proxy de confiance, majuscules", reseauProxy, "10.0.0.5:1", "203.0.113.9", "HTTPS", true},
		{"proxy de confiance en HTTP", reseauProxy, "10.0.0.5:1", "203.0.113.9", "http", false},
		{"client direct qui forge l'en-tête", reseauProxy, "198.51.100.7:1", "", "https", false},
		{"en-tête absent", reseauProxy, "10.0.0.5:1", "203.0.113.9", "", false},
		{"en-tête invalide", reseauProxy, "10.0.0.5:1", "203.0.113.9", "gopher", false},
		{"valeur forgée en tête, le proxy a ajouté http", reseauProxy, "10.0.0.5:1", "203.0.113.9", "https, http", false},
		{"chaîne : le client est entré en HTTPS", reseauProxy, "10.0.0.5:1", "203.0.113.9, 10.0.0.8", "https, http", true},
		{"chaîne : le client est entré en HTTP et a forgé https", reseauProxy, "10.0.0.5:1", "203.0.113.9, 10.0.0.8", "https, http, https", false},
		{"chaîne : le dernier proxy n'a transmis qu'une valeur", reseauProxy, "10.0.0.5:1", "203.0.113.9, 10.0.0.8", "https", true},
		{"défaut : nginx sur cette machine", "", "127.0.0.1:1", "203.0.113.9", "https", true},
		{"défaut : client du réseau qui forge l'en-tête", "", "10.0.0.5:1", "", "https", false},
	} {
		t.Run(c.nom, func(t *testing.T) {
			if v := accesAvec(t, c.proxies).https(requeteDe(c.pair, c.xff, c.proto)); v != c.attendu {
				t.Fatalf("pair %s, %q, %q : %v, attendu %v", c.pair, c.xff, c.proto, v, c.attendu)
			}
		})
	}
}

// Le parcours complet : la connexion pose le cookie, le compteur suit la vraie adresse.
func TestProxiesConnexion(t *testing.T) {
	reg, err := comptes.Ouvrir(filepath.Join(t.TempDir(), "comptes.json"))
	if err != nil {
		t.Fatal(err)
	}
	proxies, _ := LireProxies("172.30.0.0/24")
	b := nouveauAvec(t, Options{Comptes: reg, Proxies: proxies})
	b.s.acces.pause = 0
	if _, err := reg.Creer("meneur", "mot-de-passe-ruenem", false); err != nil {
		t.Fatal(err)
	}
	essai := func(pair, xff, proto, mdp string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/api/acces/connexion", strings.NewReader(toJSON(map[string]any{"nom": "meneur", "mdp": mdp})))
		r.Host, r.RemoteAddr = "jdr.example", pair
		r.Header.Set("X-COF-DM", "1")
		r.Header.Set("X-Forwarded-For", xff)
		r.Header.Set("X-Forwarded-Proto", proto)
		w := httptest.NewRecorder()
		b.s.ServeHTTP(w, r)
		return w
	}
	secure := func(w *httptest.ResponseRecorder) bool {
		for _, c := range w.Result().Cookies() {
			if c.Name == cookieMJ {
				return c.Secure
			}
		}
		t.Fatalf("pas de cookie de session : %d", w.Code)
		return false
	}
	if !secure(essai("172.30.0.2:1", "203.0.113.9", "https", "mot-de-passe-ruenem")) {
		t.Fatal("derrière le proxy de confiance, le cookie doit porter Secure")
	}
	if secure(essai("198.51.100.7:1", "203.0.113.9", "https", "mot-de-passe-ruenem")) {
		t.Fatal("un client direct ne dicte pas Secure")
	}
	// un visiteur épuise ses essais : un autre, derrière le même proxy, n'est pas touché
	for i := 0; i < essaisMax; i++ {
		if w := essai("172.30.0.2:1", "203.0.113.66", "https", "faux"); w.Code != 401 {
			t.Fatalf("essai %d : %d", i, w.Code)
		}
	}
	if w := essai("172.30.0.2:1", "203.0.113.66", "https", "faux"); w.Code != 429 {
		t.Fatalf("essai de trop : %d", w.Code)
	}
	if w := essai("172.30.0.2:1", "203.0.113.9", "https", "mot-de-passe-ruenem"); w.Code != 200 {
		t.Fatalf("autre visiteur derrière le proxy : %d", w.Code)
	}
}

func TestLireProxies(t *testing.T) {
	if l, err := LireProxies(" 10.0.0.0/8 , fd00::/8,"); err != nil || len(l) != 2 {
		t.Fatalf("%v %v", l, err)
	}
	if l, err := LireProxies(ProxiesParDefaut); err != nil || len(l) != 2 {
		t.Fatalf("%v %v", l, err)
	}
	for _, v := range []string{"10.0.0.5", "nginx", "10.0.0.0/33", "0.0.0.0/0", "::/0", "10.0.0.0/8,0.0.0.0/0"} {
		if _, err := LireProxies(v); err == nil {
			t.Errorf("%q accepté", v)
		}
	}
}
