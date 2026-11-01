package comptes

import (
	"encoding/hex"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ancienneEmpreinte : la forme d'avant, « sel:empreinte », à 210 000 itérations.
func ancienneEmpreinte(mdp string) string {
	sel := []byte("0123456789abcdef")
	return hex.EncodeToString(sel) + ":" + hex.EncodeToString(deriver(mdp, sel, iterationsAnciennes))
}

func TestEmpreinteVersionnee(t *testing.T) {
	e := empreinteMdp("une-phrase-bien-longue")
	if !strings.HasPrefix(e, "pbkdf2-sha256$600000$") || strings.Count(e, "$") != 3 {
		t.Fatalf("forme : %s", e)
	}
	if iterationsMdp != 600000 {
		t.Fatalf("itérations : %d", iterationsMdp)
	}
	if !lireEmpreinte(e).correspond("une-phrase-bien-longue") || lireEmpreinte(e).correspond("autre") {
		t.Fatal("vérification")
	}
	for _, x := range []string{"", "x:y", ":", "pbkdf2-sha256$0$aa$bb", "pbkdf2-sha256$-1$aa$bb", "pbkdf2-sha256$99999999999$aa$bb", "md5$1000$aa$bb", "pbkdf2-sha256$600000$zz$bb"} {
		if lireEmpreinte(x).correspond("") {
			t.Errorf("empreinte illisible acceptée : %q", x)
		}
	}
}

// Une empreinte ancienne passe à la nouvelle forme à la connexion réussie, et à elle seule, sans fermer
// les sessions du compte.
func TestMigrationEmpreinte(t *testing.T) {
	chemin := filepath.Join(t.TempDir(), "comptes.json")
	r, err := Ouvrir(chemin)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Creer("aline", "une-phrase-bien-longue", false); err != nil {
		t.Fatal(err)
	}
	ancienne := ancienneEmpreinte("une-phrase-bien-longue")
	if _, err := r.Modifier("aline", func(c *Compte) error { c.Mdp = ancienne; return nil }); err != nil {
		t.Fatal(err)
	}
	avant, _ := r.Lire("aline")
	if _, ok := r.Verifier("aline", "mauvais-mot-de-passe"); ok {
		t.Fatal("mauvais mot de passe accepté")
	}
	if c, _ := r.Lire("aline"); c.Mdp != ancienne {
		t.Fatal("un échec ne change rien")
	}
	if _, ok := r.Verifier("aline", "une-phrase-bien-longue"); !ok {
		t.Fatal("une empreinte ancienne reste valable")
	}
	apres, _ := r.Lire("aline")
	if !strings.HasPrefix(apres.Mdp, "pbkdf2-sha256$600000$") || apres.Version != avant.Version || apres.Graine != avant.Graine {
		t.Fatalf("migration : %+v", apres)
	}
	// enregistrée, et toujours bonne au relancement
	r2, err := Ouvrir(chemin)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := r2.Verifier("aline", "une-phrase-bien-longue"); !ok {
		t.Fatal("après relecture")
	}
	if _, ok := r2.Verifier("aline", "mauvais-mot-de-passe"); ok {
		t.Fatal("après relecture : mauvais mot de passe accepté")
	}
}

// Un compte inconnu, ancien ou récent coûte le même calcul à un échec : le temps ne dit pas lequel existe.
func TestEchecMemeCout(t *testing.T) {
	r, err := Ouvrir(filepath.Join(t.TempDir(), "comptes.json"))
	if err != nil {
		t.Fatal(err)
	}
	r.Creer("recent", "une-phrase-bien-longue", false)
	r.Creer("ancien", "une-phrase-bien-longue", false)
	r.Modifier("ancien", func(c *Compte) error { c.Mdp = ancienneEmpreinte("une-phrase-bien-longue"); return nil })
	mesure := func(nom string) time.Duration {
		d := time.Hour
		for i := 0; i < 3; i++ {
			t0 := time.Now()
			r.Verifier(nom, "mauvais-mot-de-passe")
			d = min(d, time.Since(t0))
		}
		return d
	}
	inconnu, recent, ancien := mesure("personne"), mesure("recent"), mesure("ancien")
	t.Logf("inconnu %v, récent %v, ancien %v", inconnu, recent, ancien)
	for _, d := range []time.Duration{recent, ancien} {
		if d < inconnu*7/10 || d > inconnu*14/10 {
			t.Errorf("écart de temps : inconnu %v, récent %v, ancien %v", inconnu, recent, ancien)
		}
	}
}
