package reglages

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReglages(t *testing.T) {
	chemin := filepath.Join(t.TempDir(), "sous", "reglages.json")
	r := Ouvrir(chemin)
	if _, ok := r.Meta("univers"); ok {
		t.Fatal("réglage inventé")
	}
	if err := r.SetMeta("univers", "/monde.cofdm"); err != nil {
		t.Fatal(err)
	}
	if err := r.SetMeta("jeton", "abc"); err != nil {
		t.Fatal(err)
	}
	if err := r.SetMeta("jeton", ""); err != nil {
		t.Fatal(err)
	}
	r = Ouvrir(chemin)
	if v, ok := r.Meta("univers"); !ok || v != "/monde.cofdm" {
		t.Fatalf("réglage relu : %q %v", v, ok)
	}
	if _, ok := r.Meta("jeton"); ok {
		t.Fatal("un réglage vidé doit disparaître")
	}
	if info, err := os.Stat(chemin); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("le fichier des réglages doit être réservé à son propriétaire : %v", info.Mode())
	}
}
