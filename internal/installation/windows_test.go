//go:build windows

package installation

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tchyze/cof_dm/internal/majour"
)

// TestAttendre ne sert qu'à faire tourner le programme installé pendant un autre test (un vrai
// programme en cours, que Windows refuse de retirer).
func TestAttendre(t *testing.T) {
	if os.Getenv("COFDM_ATTENDRE") == "" {
		t.Skip("lancé seulement par TestInstallerWindows")
	}
	time.Sleep(4 * time.Second)
}

// Installation réelle sous Windows, dans des dossiers et une clé de registre d'essai : copie,
// raccourcis, « Applications installées », remplacement pendant que le programme installé tourne,
// nettoyage de l'ancien une fois fermé, désinstallation.
func TestInstallerWindows(t *testing.T) {
	racine := t.TempDir()
	b := make([]byte, 4)
	rand.Read(b)
	w := Windows{Dossier: filepath.Join(racine, "Programs", "COF DM"), Bureau: filepath.Join(racine, "Bureau"),
		Menu: filepath.Join(racine, "Menu"), Cle: "COF DM essai " + hex.EncodeToString(b)}
	t.Cleanup(func() { w.Desinstaller() })
	source, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	if v := w.VersionInstallee(); v != "" {
		t.Fatalf("version avant installation : %q", v)
	}
	bureau, _, err := w.Installer(source, "1.2.6")
	if err != nil {
		t.Fatalf("installation : %v", err)
	}
	if !MemeChemin(bureau, w.Bureau) {
		t.Fatalf("Bureau : %q", bureau)
	}
	for _, f := range []string{w.Programme(), filepath.Join(w.Bureau, "COF DM.lnk"), filepath.Join(w.Menu, "COF DM.lnk")} {
		if _, err := os.Stat(f); err != nil {
			t.Fatalf("absent après installation : %v", err)
		}
	}
	if v := w.VersionInstallee(); v != "1.2.6" {
		t.Fatalf("version inscrite : %q", v)
	}
	out, _ := exec.Command("reg", "query", `HKCU\`+racineDesinstallation+w.Cle, "/v", "UninstallString").Output()
	if !strings.Contains(string(out), `"`+w.Programme()+`" -desinstaller`) {
		t.Fatalf("désinstallation inscrite : %s", out)
	}
	if Decider(source, w.Programme(), true, "1.2.6", w.VersionInstallee()) != LancerInstalle {
		t.Fatal("même version : l'installé doit être lancé, pas remplacé")
	}

	// le programme installé tourne : la version suivante le remplace quand même
	cours := exec.Command(w.Programme(), "-test.run=^TestAttendre$")
	cours.Env = append(os.Environ(), "COFDM_ATTENDRE=1")
	if err := cours.Start(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)
	if _, _, err := w.Installer(source, "1.2.7"); err != nil {
		t.Fatalf("remplacement d'un programme en cours : %v", err)
	}
	if v := w.VersionInstallee(); v != "1.2.7" {
		t.Fatalf("version après remplacement : %q", v)
	}
	ancien := w.Programme() + ".ancien"
	if os.Remove(ancien) == nil {
		t.Fatal("Windows a laissé retirer un programme en cours : l'essai ne prouve rien")
	}
	cours.Wait()
	// une fois fermé, le nettoyage du programme relancé le retire (réessais compris)
	if err := majour.NettoyerA(w.Programme()); err != nil {
		t.Fatalf("nettoyage : %v", err)
	}
	if _, err := os.Stat(ancien); !os.IsNotExist(err) {
		t.Fatal("programme d'avant toujours là")
	}

	if err := w.Desinstaller(); err != nil {
		t.Fatalf("désinstallation : %v", err)
	}
	if v := w.VersionInstallee(); v != "" {
		t.Fatalf("entrée restée dans « Applications installées » : %q", v)
	}
	for _, f := range []string{filepath.Join(w.Bureau, "COF DM.lnk"), filepath.Join(w.Menu, "COF DM.lnk")} {
		if _, err := os.Stat(f); !os.IsNotExist(err) {
			t.Fatalf("raccourci resté : %s", f)
		}
	}
	if err := w.RetirerApresFermeture(); err != nil {
		t.Fatal(err)
	}
	for fin := time.Now().Add(15 * time.Second); time.Now().Before(fin); time.Sleep(500 * time.Millisecond) {
		if _, err := os.Stat(w.Dossier); os.IsNotExist(err) {
			return
		}
	}
	t.Fatal("dossier du programme toujours là")
}
