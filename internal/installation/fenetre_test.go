package installation

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRetirerAncienCache(t *testing.T) {
	appdata := t.TempDir()
	if err := RetirerAncienCache(appdata); err != nil {
		t.Fatalf("rien à retirer : %v", err)
	}
	d := filepath.Join(appdata, ancienCache)
	os.MkdirAll(filepath.Join(d, sousDossierVue, "Default"), 0o700)
	os.WriteFile(filepath.Join(d, sousDossierVue, "Default", "Cookies"), []byte("x"), 0o600)
	// autre chose que le cache : gardé
	os.WriteFile(filepath.Join(d, "a-moi.txt"), []byte("x"), 0o600)
	if err := RetirerAncienCache(appdata); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(d, "a-moi.txt")); err != nil {
		t.Fatalf("fichier inconnu retiré : %v", err)
	}
	os.Remove(filepath.Join(d, "a-moi.txt"))
	if err := RetirerAncienCache(appdata); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(d); !os.IsNotExist(err) {
		t.Fatal("ancien cache toujours là")
	}
}

func TestCacheFenetre(t *testing.T) {
	if c := CacheFenetre(filepath.Join("a", "COF DM")); c != filepath.Join("a", "COF DM", "webview") {
		t.Fatal(c)
	}
}
