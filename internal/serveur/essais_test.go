package serveur

import (
	"net/http"
	"strconv"
	"testing"
	"time"
)

// essaiDepuis : une tentative de connexion, vue à travers le reverse proxy.
func essaiDepuis(b *banc, ip, nom, mdp string) int {
	b.t.Helper()
	c, _, _ := b.proxy("POST", "/api/acces/connexion", map[string]any{"nom": nom, "mdp": mdp}, func(r *http.Request) { r.Header.Set("X-Forwarded-For", ip) })
	return c
}

// Un attaquant, même réparti sur plusieurs adresses, ne bloque pas le vrai titulaire : il épuise ses
// propres essais, et le titulaire entre depuis une autre adresse.
func TestEssaisTitulaireNonBloque(t *testing.T) {
	b, _ := bancServeur(t)
	b.s.acces.pasNom = 0
	for _, ip := range []string{"198.51.100.1", "198.51.100.2", "198.51.100.3"} {
		for i := 0; i < essaisMax; i++ {
			if c := essaiDepuis(b, ip, "meneur", "faux"); c != 401 {
				t.Fatalf("%s, essai %d : %d", ip, i, c)
			}
		}
		if c := essaiDepuis(b, ip, "meneur", "mot-de-passe-ruenem"); c != 429 {
			t.Fatalf("%s a épuisé ses essais sur ce nom : %d", ip, c)
		}
	}
	if c := essaiDepuis(b, "203.0.113.50", "meneur", "mot-de-passe-ruenem"); c != 200 {
		t.Fatalf("le titulaire, depuis une autre adresse : %d", c)
	}
}

// Une adresse a un quota tous noms confondus : passer d'un nom à l'autre ne donne pas plus d'essais.
func TestEssaisQuotaAdresse(t *testing.T) {
	b, _ := bancServeur(t)
	b.s.acces.pasNom = 0
	for i := 0; i < essaisMaxAdresse; i++ {
		if c := essaiDepuis(b, "198.51.100.1", "nom"+strconv.Itoa(i), "faux"); c != 401 {
			t.Fatalf("essai %d : %d", i, c)
		}
	}
	if c := essaiDepuis(b, "198.51.100.1", "meneur", "mot-de-passe-ruenem"); c != 429 {
		t.Fatalf("quota de l'adresse dépassé : %d", c)
	}
	if c := essaiDepuis(b, "198.51.100.2", "meneur", "mot-de-passe-ruenem"); c != 200 {
		t.Fatalf("autre adresse : %d", c)
	}
}

// Un essai refusé sur un nom ne consomme pas le quota de l'adresse.
func TestEssaisCoupleNeConsommePasLeQuota(t *testing.T) {
	b, _ := bancServeur(t)
	b.s.acces.pasNom = 0
	for i := 0; i < essaisMax; i++ {
		essaiDepuis(b, "198.51.100.1", "meneur", "faux")
	}
	for i := 0; i < essaisMaxAdresse; i++ {
		if c := essaiDepuis(b, "198.51.100.1", "meneur", "faux"); c != 429 {
			t.Fatalf("essai %d : %d", i, c)
		}
	}
	if c := essaiDepuis(b, "198.51.100.1", "audit", "mot-de-passe-tidua"); c != 200 {
		t.Fatalf("autre compte depuis la même adresse : %d", c)
	}
}

// En IPv6, chaque client tient un /64 entier : le compteur est celui du /64.
func TestEssaisIPv6Par64(t *testing.T) {
	b, _ := bancServeur(t)
	b.s.acces.pasNom = 0
	for i := 0; i < essaisMax; i++ {
		if c := essaiDepuis(b, "2001:db8:1:2::"+strconv.FormatInt(int64(i+1), 16), "meneur", "faux"); c != 401 {
			t.Fatalf("essai %d : %d", i, c)
		}
	}
	if c := essaiDepuis(b, "2001:db8:1:2:ffff::9", "meneur", "mot-de-passe-ruenem"); c != 429 {
		t.Fatalf("même /64 : %d", c)
	}
	if c := essaiDepuis(b, "2001:db8:1:3::1", "meneur", "mot-de-passe-ruenem"); c != 200 {
		t.Fatalf("autre /64 : %d", c)
	}
}

func TestCleAdresse(t *testing.T) {
	for v, attendu := range map[string]string{
		"203.0.113.9":          "203.0.113.9",
		"2001:db8:1:2:3:4:5:6": "2001:db8:1:2::",
		"2001:db8:1:2::9":      "2001:db8:1:2::",
		"::ffff:203.0.113.9":   "203.0.113.9",
		"illisible":            "illisible",
	} {
		if c := cleAdresse(v); c != attendu {
			t.Errorf("%s : %s, attendu %s", v, c, attendu)
		}
	}
}

// Une attaque répartie ralentit les essais sur ce nom, sans jamais les refuser.
func TestEssaisRalentissementParNom(t *testing.T) {
	b, _ := bancServeur(t)
	a := b.s.acces
	a.pasNom = 100 * time.Millisecond
	cle := "ralenti:meneur"
	if d := a.attenteNom(cle); d != 0 {
		t.Fatalf("sans essai : %v", d)
	}
	for i := 0; i < essaisSeuilNom; i++ {
		a.reserverEssai(cle, essaisPlafondNom)
	}
	if d := a.attenteNom(cle); d != 100*time.Millisecond {
		t.Fatalf("au seuil : %v", d)
	}
	for i := 0; i < 500; i++ {
		a.reserverEssai(cle, essaisPlafondNom)
	}
	if d := a.attenteNom(cle); d != attenteNomMax {
		t.Fatalf("attente plafonnée : %v", d)
	}
	// de bout en bout : assez d'adresses différentes pour franchir le seuil
	a.pasNom = 20 * time.Millisecond
	for i := 0; i <= essaisSeuilNom; i++ {
		if c := essaiDepuis(b, "198.51.100."+strconv.Itoa(i+1), "meneur", "faux"); c != 401 {
			t.Fatalf("adresse %d : %d", i, c)
		}
	}
	debut := time.Now()
	if c := essaiDepuis(b, "203.0.113.50", "meneur", "mot-de-passe-ruenem"); c != 200 {
		t.Fatalf("le titulaire est ralenti, pas refusé : %d", c)
	}
	if time.Since(debut) < 20*time.Millisecond {
		t.Fatal("aucun ralentissement")
	}
}

// Des essais de connexion sur son nom n'empêchent pas le titulaire de changer son mot de passe.
func TestEssaisChangementMdpNonBloque(t *testing.T) {
	b, _ := bancServeur(t)
	b.s.acces.pasNom = 0
	mj := b.connecter("meneur")
	for i := 0; i <= essaisMaxMdp; i++ {
		essaiDepuis(b, "198.51.100."+strconv.Itoa(i+1), "meneur", "faux")
	}
	if c, v, _ := b.proxy("POST", "/api/compte/mdp", map[string]any{"ancien": "mot-de-passe-ruenem", "nouveau": "un-nouveau-mot-de-passe"}, mj); c != 200 {
		t.Fatalf("changement de mot de passe : %d %v", c, v)
	}
}
