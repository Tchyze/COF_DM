package comptes

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestMdpAcceptable(t *testing.T) {
	for _, c := range []struct {
		nom, mdp string
		admin    bool
		attendu  error
	}{
		{"aline", "une-phrase-bien-longue", false, nil},
		{"aline", "une-phrase-bien-longue", true, nil},
		{"aline", "assez-long", false, nil},
		{"aline", "trop-court", true, ErrMdpAdmin},
		{"aline", "court", false, ErrMdp},
		{"aline", "aaaaaaaaaaaa", false, ErrMdpFaible},
		{"aline", "éééééééééééé", false, ErrMdpFaible},
		{"aline", "Aline-2026-Paris", false, ErrMdpFaible},
		{"aline", "1234567890", false, ErrMdpFaible},
		{"aline", "AZERTYUIOP", false, ErrMdpFaible},
		{"aline", "motdepasse123", false, ErrMdpFaible},
		{"admin", "admin12345", false, ErrMdpFaible},
		{"chef", "administrateur", true, ErrMdpFaible},
	} {
		if err := MdpAcceptable(c.nom, c.mdp, c.admin); !errors.Is(err, c.attendu) {
			t.Errorf("%s / %q / admin=%v : %v, attendu %v", c.nom, c.mdp, c.admin, err, c.attendu)
		}
	}
}

// La règle vaut pour créer et pour changer. Un compte existant au mot de passe faible se connecte
// toujours : seul son prochain changement la subit.
func TestMdpFaibleCreationEtChangement(t *testing.T) {
	r, err := Ouvrir(filepath.Join(t.TempDir(), "comptes.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Creer("aline", "1234567890", false); !errors.Is(err, ErrMdpFaible) {
		t.Fatalf("création : %v", err)
	}
	if _, err := r.Creer("chef", "trop-court", true); !errors.Is(err, ErrMdpAdmin) {
		t.Fatalf("création d'un administrateur : %v", err)
	}
	if _, err := r.Creer("aline", "une-phrase-bien-longue", false); err != nil {
		t.Fatal(err)
	}
	if _, err := r.ChangerMdp("aline", "aline-aline-aline", false); !errors.Is(err, ErrMdpFaible) {
		t.Fatalf("changement : %v", err)
	}
	if err := r.ReinitialiserMdp("aline", "aaaaaaaaaaaa"); !errors.Is(err, ErrMdpFaible) {
		t.Fatalf("réinitialisation : %v", err)
	}
	if _, ok := r.Verifier("aline", "une-phrase-bien-longue"); !ok {
		t.Fatal("un refus ne change rien")
	}
	// un compte d'avant la règle
	if _, err := r.Modifier("aline", func(c *Compte) error { c.Mdp = empreinteMdp("1234567890"); return nil }); err != nil {
		t.Fatal(err)
	}
	if _, ok := r.Verifier("aline", "1234567890"); !ok {
		t.Fatal("un compte existant n'est pas bloqué")
	}
}
