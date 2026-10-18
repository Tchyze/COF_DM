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
	if chef, _ := r2.Lire("chef"); chef.Role("u2") != MJ {
		t.Fatal("l'administrateur n'est pas MJ partout")
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
