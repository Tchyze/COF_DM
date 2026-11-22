package majour

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppDe(t *testing.T) {
	cas := map[string]string{
		"/Applications/COF DM.app/Contents/MacOS/cof-dm": "/Applications/COF DM.app",
		"/Users/mj/COF DM.app/Contents/MacOS/cof-dm":     "/Users/mj/COF DM.app",
		"/usr/local/bin/cof-dm":                          "",
		"/Applications/COF DM/Contents/MacOS/cof-dm":     "",
		"/Applications/COF DM.app/MacOS/cof-dm":          "",
	}
	for exe, attendu := range cas {
		if got := appDe(filepath.FromSlash(exe)); got != filepath.FromSlash(attendu) {
			t.Errorf("appDe(%q) = %q, attendu %q", exe, got, attendu)
		}
	}
}

func TestLanceur(t *testing.T) {
	if got := Lanceur("/x/COF-DM.exe"); got != "/x/COF-DM.exe" {
		t.Errorf("hors macOS, le programme lui-même : %q", got)
	}
	app := filepath.Join(t.TempDir(), "COF DM.app")
	if got := Lanceur(app); got != filepath.Join(app, "Contents", "MacOS", "cof-dm") {
		t.Errorf("application sans programme : %q", got)
	}
	os.MkdirAll(filepath.Join(app, "Contents", "MacOS"), 0o755)
	os.WriteFile(filepath.Join(app, "Contents", "MacOS", "autre"), []byte("x"), 0o755)
	if got := Lanceur(app); got != filepath.Join(app, "Contents", "MacOS", "autre") {
		t.Errorf("le programme de l'application : %q", got)
	}
}

// zipMac écrit une archive : nom → contenu (un nom finissant par « / » est un dossier, un contenu
// commençant par « lien: » un lien symbolique).
func zipMac(t *testing.T, entrees [][2]string) string {
	t.Helper()
	chemin := filepath.Join(t.TempDir(), "app.zip")
	f, _ := os.Create(chemin)
	z := zip.NewWriter(f)
	for _, e := range entrees {
		h := &zip.FileHeader{Name: e[0], Method: zip.Deflate}
		switch {
		case strings.HasSuffix(e[0], "/"):
			h.SetMode(os.ModeDir | 0o755)
		case strings.HasPrefix(e[1], "lien:"):
			h.SetMode(os.ModeSymlink | 0o777)
			e[1] = strings.TrimPrefix(e[1], "lien:")
		case strings.Contains(e[0], "/MacOS/"):
			h.SetMode(0o755)
		default:
			h.SetMode(0o644)
		}
		w, err := z.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(e[1]))
	}
	z.Close()
	f.Close()
	return chemin
}

func TestDeballerApp(t *testing.T) {
	cible := filepath.Join(t.TempDir(), "COF DM.app")
	bon := [][2]string{
		{"COF DM.app/", ""},
		{"COF DM.app/Contents/", ""},
		{"COF DM.app/Contents/Info.plist", "<plist/>"},
		{"COF DM.app/Contents/MacOS/cof-dm", "programme"},
		{"COF DM.app/Contents/Resources/icone.icns", "icône"},
		{"__MACOSX/._COF DM.app", "métadonnées"},
	}
	app, dossier, err := deballerApp(zipMac(t, bon), cible)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(dossier) != filepath.Dir(cible) || !strings.HasPrefix(filepath.Base(dossier), prefixeTelecharge) {
		t.Errorf("dossier hors de celui de la cible, ou mal nommé : %s", dossier)
	}
	if filepath.Base(app) != "COF DM.app" {
		t.Errorf("application : %s", app)
	}
	i, err := os.Stat(filepath.Join(app, "Contents", "MacOS", "cof-dm"))
	if err != nil || i.Mode().Perm()&0o100 == 0 {
		t.Errorf("programme absent ou non exécutable : %v %v", i, err)
	}
	if _, err := os.Stat(filepath.Join(dossier, "__MACOSX")); err == nil {
		t.Error("métadonnées dépliées")
	}
	// Remplacer prend l'application entière : l'ancienne devient « .ancien », puis se retire
	os.MkdirAll(filepath.Join(cible, "Contents", "MacOS"), 0o755)
	os.WriteFile(filepath.Join(cible, "Contents", "MacOS", "cof-dm"), []byte("ancien"), 0o755)
	if err := Remplacer(cible, app); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(cible, "Contents", "MacOS", "cof-dm")); string(b) != "programme" {
		t.Errorf("application non remplacée : %q", b)
	}
	os.RemoveAll(dossier)
	pausesNettoyage = nil
	if err := NettoyerA(cible); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(cible + suffixeAncien); err == nil {
		t.Error("application d'avant non retirée")
	}

	refus := map[string][][2]string{
		"sortie":        {{"../dehors", "x"}},
		"absolu":        {{"/etc/x", "x"}},
		"hors de l'app": {{"COF DM.app/Contents/Info.plist", "x"}, {"autre.txt", "x"}},
		"deux apps":     {{"A.app/Contents/Info.plist", "x"}, {"B.app/Contents/Info.plist", "x"}},
		"lien":          {{"COF DM.app/Contents/Info.plist", "x"}, {"COF DM.app/Contents/MacOS/cof-dm", "lien:/bin/sh"}},
		"sans plist":    {{"COF DM.app/Contents/MacOS/cof-dm", "x"}},
		"vide":          {},
		"pas une app":   {{"COF DM/Contents/Info.plist", "x"}},
	}
	for nom, entrees := range refus {
		if _, _, err := deballerApp(zipMac(t, entrees), cible); err == nil {
			t.Errorf("%s : archive acceptée", nom)
		}
	}
	l, _ := os.ReadDir(filepath.Dir(cible))
	for _, e := range l {
		if strings.HasPrefix(e.Name(), prefixeTelecharge) {
			t.Errorf("dossier de dépliage laissé après un refus : %s", e.Name())
		}
	}
}
