package installation

import (
	"path/filepath"
	"testing"
)

func TestDecider(t *testing.T) {
	installe := filepath.Join(string(filepath.Separator), "Programs", "COF DM", "COF-DM.exe")
	bureau := filepath.Join(string(filepath.Separator), "Bureau", "COF-DM.exe")
	for _, c := range []struct {
		nom              string
		exe              string
		present          bool
		ici, versionInst string
		attendu          Action
	}{
		{"le programme installé lui-même", installe, true, "1.2.6", "1.2.6", Continuer},
		{"première installation", bureau, false, "1.2.6", "", Installer},
		{"même version : on lance l'installé", bureau, true, "1.2.6", "1.2.6", LancerInstalle},
		{"exe du Bureau plus ancien que l'installé mis à jour", bureau, true, "1.2.6", "1.2.8", LancerInstalle},
		{"exe téléchargé plus récent", bureau, true, "1.2.9", "1.2.8", Installer},
		{"préversion plus récente", bureau, true, "1.2.9-test.3", "1.2.8", Installer},
		{"version installée inconnue", bureau, true, "1.2.6", "", Installer},
		{"version de développement : ne remplace rien", bureau, true, "dev", "1.2.6", LancerInstalle},
	} {
		if a := Decider(c.exe, installe, c.present, c.ici, c.versionInst); a != c.attendu {
			t.Errorf("%s : %v, attendu %v", c.nom, a, c.attendu)
		}
	}
}
