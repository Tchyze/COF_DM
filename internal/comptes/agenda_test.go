package comptes

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// L'adresse d'abonnement : une par compte, remontrée à son titulaire, reconnue seule (jamais comme
// jeton d'API), changée ou retirée ; elle ne vaut plus pour un compte désactivé ou devenu
// administrateur, et le fichier des comptes n'en garde que l'empreinte et une copie chiffrée.
func TestAgendaDuCompte(t *testing.T) {
	chemin := filepath.Join(t.TempDir(), "comptes.json")
	r, err := Ouvrir(chemin)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Creer("chef", "Grand-Phare-Bleu-2026", true); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Creer("lea", "Petite-Rose-Verte-77", false); err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.NouvelAgenda("chef"); err != ErrAgendaAdmin {
		t.Fatalf("agenda d'un administrateur : %v", err)
	}
	clair, _, err := r.NouvelAgenda("lea")
	if err != nil || !strings.HasPrefix(clair, prefixeAgenda) || len(clair) < 40 {
		t.Fatalf("jeton : %q %v", clair, err)
	}
	if c, ok := r.ParAgenda(clair); !ok || c.Nom != "lea" {
		t.Fatal("jeton non reconnu")
	}
	if _, ok := r.ParJeton(clair); ok {
		t.Fatal("le jeton d'agenda ouvre l'API")
	}
	if l, _ := r.LireAgenda("lea"); l != clair {
		t.Fatalf("relu : %q", l)
	}
	b, _ := os.ReadFile(chemin)
	if strings.Contains(string(b), clair) || strings.Contains(string(b), strings.TrimPrefix(clair, prefixeAgenda)) {
		t.Fatal("jeton en clair dans le fichier des comptes")
	}
	// relu après réouverture (clé des secrets sur le disque)
	r2, _ := Ouvrir(chemin)
	if l, _ := r2.LireAgenda("lea"); l != clair {
		t.Fatalf("relu après réouverture : %q", l)
	}
	nouveau, _, _ := r.NouvelAgenda("lea")
	if _, ok := r.ParAgenda(clair); ok || nouveau == clair {
		t.Fatal("l'ancienne adresse vaut encore")
	}
	r.Modifier("lea", func(c *Compte) error { c.Inactif = true; return nil })
	if _, ok := r.ParAgenda(nouveau); ok {
		t.Fatal("compte désactivé")
	}
	r.Modifier("lea", func(c *Compte) error { c.Inactif = false; return nil })
	if err := r.RetirerAgenda("lea"); err != nil {
		t.Fatal(err)
	}
	if _, ok := r.ParAgenda(nouveau); ok {
		t.Fatal("adresse retirée")
	}
	if l, _ := r.LireAgenda("lea"); l != "" {
		t.Fatal("adresse retirée encore lisible")
	}
	troisieme, _, _ := r.NouvelAgenda("lea")
	if _, err := r.Modifier("lea", func(c *Compte) error { c.Admin = true; return nil }); err != nil {
		t.Fatal(err)
	}
	if _, ok := r.ParAgenda(troisieme); ok {
		t.Fatal("un administrateur garde son agenda")
	}
	for _, faux := range []string{"", "cofag_", "cofag_" + strings.Repeat("A", 200), "cofdm_abc"} {
		if _, ok := r.ParAgenda(faux); ok {
			t.Fatalf("%q reconnu", faux)
		}
	}
}
