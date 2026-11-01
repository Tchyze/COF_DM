package comptes

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRegistre(t *testing.T) {
	chemin := filepath.Join(t.TempDir(), "comptes.json")
	r, err := Ouvrir(chemin)
	if err != nil || !r.Vide() {
		t.Fatalf("registre neuf : %v", err)
	}
	// un compte ordinaire peut exister et changer avant tout administrateur
	if _, err := r.Creer("Aline", "un-bon-mot-de-passe", false); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Modifier("aline", func(c *Compte) error { c.Roles["u1"] = Auditeur; return nil }); err != nil {
		t.Fatalf("rôle sans administrateur : %v", err)
	}
	for nom, attendu := range map[string]error{"ab": ErrNom, "avec espace": ErrNom, "a.b.c": ErrNom, "aline": ErrExiste} {
		if _, err := r.Creer(nom, "un-bon-mot-de-passe", false); !errors.Is(err, attendu) {
			t.Fatalf("création de %q : %v", nom, err)
		}
	}
	if _, err := r.Creer("court", "trop-court", false); !errors.Is(err, ErrMdp) && len("trop-court") < MdpMin {
		t.Fatalf("mot de passe court : %v", err)
	}
	r.Creer("chef", "mot-de-passe-du-chef", true)
	if _, err := r.Modifier("aline", func(c *Compte) error { c.Roles["u1"] = "roi"; return nil }); !errors.Is(err, ErrRole) {
		t.Fatalf("rôle inconnu : %v", err)
	}
	// le dernier administrateur actif reste
	if _, err := r.Modifier("chef", func(c *Compte) error { c.Inactif = true; return nil }); !errors.Is(err, ErrDernierAdmin) {
		t.Fatalf("dernier administrateur désactivé : %v", err)
	}
	if err := r.Supprimer("chef"); !errors.Is(err, ErrDernierAdmin) {
		t.Fatalf("dernier administrateur supprimé : %v", err)
	}
	// mot de passe : vérifié, jamais écrit en clair, lisible du seul propriétaire
	if _, ok := r.Verifier("ALINE", "un-bon-mot-de-passe"); !ok {
		t.Fatal("bon mot de passe refusé")
	}
	if _, ok := r.Verifier("aline", "un-mauvais-mot-de-passe"); ok {
		t.Fatal("mauvais mot de passe accepté")
	}
	if _, ok := r.Verifier("personne", ""); ok {
		t.Fatal("compte inconnu accepté")
	}
	clair, j, err := r.NouveauJeton("aline", "script")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(chemin)
	if strings.Contains(string(b), "un-bon-mot-de-passe") || strings.Contains(string(b), clair) {
		t.Fatal("secret en clair dans le fichier")
	}
	if fi, _ := os.Stat(chemin); fi.Mode().Perm() != 0o600 {
		t.Fatalf("droits du fichier : %v", fi.Mode().Perm())
	}
	// relu, le registre retrouve comptes, rôles et jetons
	r2, err := Ouvrir(chemin)
	if err != nil {
		t.Fatal(err)
	}
	c, ok := r2.ParJeton(clair)
	if !ok || c.Nom != "aline" || c.Role("u1") != Auditeur || c.Role("u2") != "" {
		t.Fatalf("compte du jeton : %+v", c)
	}
	if chef, _ := r2.Lire("chef"); chef.Role("u2") != "" {
		t.Fatal("l'administrateur a un rôle dans un univers")
	}
	v := c.Version
	if err := r2.DefinirMdp("aline", "un-autre-mot-de-passe"); err != nil {
		t.Fatal(err)
	}
	if c, _ := r2.Lire("aline"); c.Version == v {
		t.Fatal("la version n'a pas changé avec le mot de passe")
	}
	r2.Modifier("aline", func(c *Compte) error { c.Inactif = true; return nil })
	if _, ok := r2.ParJeton(clair); ok {
		t.Fatal("jeton d'un compte inactif accepté")
	}
	if _, ok := r2.Verifier("aline", "un-autre-mot-de-passe"); ok {
		t.Fatal("compte inactif accepté")
	}
	if err := r2.RetirerJeton("aline", j.ID); err != nil {
		t.Fatal(err)
	}
}

// L'administrateur n'entre dans aucun univers ; un compte n'y est joueur qu'une fois admis par lui.
func TestAdminEtAdmis(t *testing.T) {
	r, _ := Ouvrir(filepath.Join(t.TempDir(), "comptes.json"))
	r.Creer("chef", "mot-de-passe-du-chef", true)
	r.Creer("lea", "mot-de-passe-de-lea", false)
	if _, err := r.Modifier("chef", func(c *Compte) error { c.Roles["u1"] = MJ; return nil }); !errors.Is(err, ErrAdmin) {
		t.Fatalf("rôle d'un administrateur : %v", err)
	}
	if _, _, err := r.NouveauJeton("chef", "script"); !errors.Is(err, ErrAdmin) {
		t.Fatalf("jeton d'un administrateur : %v", err)
	}
	if _, err := r.Modifier("lea", func(c *Compte) error { c.Roles["u1"] = Joueur; return nil }); !errors.Is(err, ErrNonAdmis) {
		t.Fatalf("joueur sans admission : %v", err)
	}
	r.Modifier("lea", func(c *Compte) error { c.Admis = append(c.Admis, "u1", "u1", "u2"); return nil })
	c, err := r.Modifier("lea", func(c *Compte) error { c.Roles["u1"] = Joueur; c.Persos["u1"] = 7; c.Persos["u2"] = 8; return nil })
	if err != nil || c.Role("u1") != Joueur || !c.PeutJouer("u2") || len(c.Admis) != 2 || c.Persos["u2"] != 0 {
		t.Fatalf("joueur admis : %+v %v", c, err)
	}
	// plus admise : elle quitte l'univers, et son PJ avec
	c, err = r.Modifier("lea", func(c *Compte) error { c.Admis = []string{"u2"}; return nil })
	if err != nil || c.Role("u1") != "" || c.Persos["u1"] != 0 {
		t.Fatalf("admission retirée : %+v %v", c, err)
	}
	if _, err := r.Modifier("lea", func(c *Compte) error { c.Admin = true; return nil }); !errors.Is(err, ErrAdmin) {
		t.Fatalf("administrateur encore admis : %v", err)
	}
	r.Modifier("lea", func(c *Compte) error { c.Roles["u3"] = MJ; return nil })
	if err := r.OublierUnivers("u3"); err != nil {
		t.Fatal(err)
	}
	if err := r.OublierUnivers("u2"); err != nil {
		t.Fatal(err)
	}
	if c, _ := r.Lire("lea"); len(c.Roles) != 0 || len(c.Admis) != 0 {
		t.Fatalf("univers supprimés gardés : %+v", c)
	}
}

// Un fichier d'avant la séparation des rôles : l'administrateur y était MJ partout, avec rôles et
// jetons ; le joueur n'avait pas d'admission.
func TestReprendreAnciensComptes(t *testing.T) {
	chemin := filepath.Join(t.TempDir(), "comptes.json")
	os.WriteFile(chemin, []byte(`[
  {"nom": "chef", "mdp": "x:y", "admin": true, "roles": {"u1": "mj"}, "jetons": [{"id": "a", "empreinte": "b"}], "version": 1},
  {"nom": "lea", "mdp": "x:y", "roles": {"u1": "joueur", "u2": "auditeur"}, "persos": {"u1": 4}, "version": 1}
]`), 0o600)
	r, err := Ouvrir(chemin)
	if err != nil {
		t.Fatal(err)
	}
	if c, _ := r.Lire("chef"); !c.Admin || len(c.Roles)+len(c.Jetons) != 0 {
		t.Fatalf("administrateur repris : %+v", c)
	}
	if c, _ := r.Lire("lea"); !c.PeutJouer("u1") || c.PeutJouer("u2") || c.Role("u1") != Joueur || c.Persos["u1"] != 4 {
		t.Fatalf("joueur repris : %+v", c)
	}
	if b, _ := os.ReadFile(chemin); strings.Contains(string(b), `"jetons"`) || !strings.Contains(string(b), `"admis"`) {
		t.Fatalf("reprise non enregistrée : %s", b)
	}
}

// Un compte d'avant la graine en reçoit une à l'ouverture, écrite aussitôt et gardée ensuite ; un
// compte recréé sous le même nom en tire une autre, et elle ne change pas avec le mot de passe.
func TestGraineDuCompte(t *testing.T) {
	chemin := filepath.Join(t.TempDir(), "comptes.json")
	if err := os.WriteFile(chemin, []byte(`[{"nom":"ancien","mdp":"aa:bb","version":3,"cree":1}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	r, err := Ouvrir(chemin)
	if err != nil {
		t.Fatal(err)
	}
	c, _ := r.Lire("ancien")
	if c.Graine == "" || c.Version != 3 {
		t.Fatalf("reprise : %+v", c)
	}
	r2, err := Ouvrir(chemin)
	if err != nil {
		t.Fatal(err)
	}
	if c2, _ := r2.Lire("ancien"); c2.Graine != c.Graine {
		t.Fatal("la graine doit rester la même d'une ouverture à l'autre")
	}
	n1, err := r.Creer("neuf", "mot-de-passe-neuf", false)
	if err != nil || n1.Graine == "" {
		t.Fatalf("%+v %v", n1, err)
	}
	if err := r.DefinirMdp("neuf", "autre-mot-de-passe"); err != nil {
		t.Fatal(err)
	}
	if x, _ := r.Lire("neuf"); x.Graine != n1.Graine {
		t.Fatal("la graine ne suit pas le mot de passe")
	}
	if err := r.Supprimer("neuf"); err != nil {
		t.Fatal(err)
	}
	n2, err := r.Creer("neuf", "mot-de-passe-neuf", false)
	if err != nil || n2.Graine == n1.Graine {
		t.Fatalf("compte recréé : %+v %v", n2, err)
	}
}
