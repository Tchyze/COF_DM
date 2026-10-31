package installation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// unPortable crée à côté d'un programme un dossier portable avec un univers, une sauvegarde et des
// réglages qui pointent dessus (chemins absolus, comme les écrit l'application).
func unPortable(t *testing.T, racine string) (exe, portable string) {
	t.Helper()
	exe = filepath.Join(racine, "Bureau", "COF-DM.exe")
	portable = filepath.Join(filepath.Dir(exe), NomPortable)
	for _, f := range []string{"Mon univers.cofdm", filepath.Join("sauvegardes", "a1", "auto.cofdm")} {
		os.MkdirAll(filepath.Dir(filepath.Join(portable, f)), 0o700)
		os.WriteFile(filepath.Join(portable, f), []byte("univers "+f), 0o600)
	}
	os.WriteFile(exe, []byte("programme"), 0o755)
	ailleurs := filepath.Join(racine, "Ailleurs", "Autre.cofdm")
	recents, _ := json.Marshal([]string{filepath.Join(portable, "Mon univers.cofdm"), ailleurs})
	ecrireReglages(filepath.Join(portable, "reglages.json"), map[string]string{
		"univers": filepath.Join(portable, "Mon univers.cofdm"), "recents": string(recents), "jeton": "abc"})
	return exe, portable
}

func TestReprendrePortableVersDossierVide(t *testing.T) {
	racine := t.TempDir()
	exe, portable := unPortable(t, racine)
	if PortableDe(exe) != portable {
		t.Fatalf("dossier portable non trouvé à côté de %s", exe)
	}
	dest := filepath.Join(racine, "AppData", "COF DM")
	r, err := ReprendrePortable(portable, dest)
	if err != nil || r.Vers != dest || r.Avertissement != "" {
		t.Fatalf("reprise : %+v %v", r, err)
	}
	if PortableDe(exe) != "" {
		t.Fatal("le dossier portable est toujours là")
	}
	if b, _ := os.ReadFile(filepath.Join(dest, "sauvegardes", "a1", "auto.cofdm")); !strings.Contains(string(b), "auto") {
		t.Fatal("sauvegarde perdue")
	}
	m, _ := lireReglages(filepath.Join(dest, "reglages.json"))
	if m["univers"] != filepath.Join(dest, "Mon univers.cofdm") || m["jeton"] != "abc" {
		t.Fatalf("réglages : %v", m)
	}
	var l []string
	json.Unmarshal([]byte(m["recents"]), &l)
	if len(l) != 2 || l[0] != filepath.Join(dest, "Mon univers.cofdm") || l[1] != filepath.Join(racine, "Ailleurs", "Autre.cofdm") {
		t.Fatalf("récents : %v", l)
	}
}

func TestReprendrePortableAvecDonneesExistantes(t *testing.T) {
	racine := t.TempDir()
	exe, portable := unPortable(t, racine)
	dest := filepath.Join(racine, "AppData", "COF DM")
	os.MkdirAll(dest, 0o700)
	os.WriteFile(filepath.Join(dest, "Univers de la maison.cofdm"), []byte("maison"), 0o600)
	recents, _ := json.Marshal([]string{filepath.Join(dest, "Univers de la maison.cofdm")})
	ecrireReglages(filepath.Join(dest, "reglages.json"), map[string]string{"univers": filepath.Join(dest, "Univers de la maison.cofdm"), "recents": string(recents)})

	r, err := ReprendrePortable(portable, dest)
	if err != nil || !strings.HasPrefix(r.Vers, filepath.Join(dest, "Repris du mode portable ")) || !strings.Contains(r.Avertissement, r.Vers) {
		t.Fatalf("reprise : %+v %v", r, err)
	}
	// rien d'écrasé chez l'utilisateur, l'univers ouvert reste le sien
	if b, _ := os.ReadFile(filepath.Join(dest, "Univers de la maison.cofdm")); string(b) != "maison" {
		t.Fatal("données existantes touchées")
	}
	m, _ := lireReglages(filepath.Join(dest, "reglages.json"))
	var l []string
	json.Unmarshal([]byte(m["recents"]), &l)
	if m["univers"] != filepath.Join(dest, "Univers de la maison.cofdm") || len(l) != 2 || l[0] != filepath.Join(r.Vers, "Mon univers.cofdm") {
		t.Fatalf("réglages de l'utilisateur : %v (récents %v)", m, l)
	}
	if b, _ := os.ReadFile(filepath.Join(r.Vers, "Mon univers.cofdm")); !strings.Contains(string(b), "univers") {
		t.Fatal("univers du portable non copié")
	}
	// l'ancien dossier est marqué, pour ne pas être repris deux fois
	if PortableDe(exe) != "" {
		t.Fatal("le dossier portable serait repris une seconde fois")
	}
	if _, err := os.Stat(portable + " (repris)"); err != nil {
		t.Fatalf("ancien dossier : %v", err)
	}
}

func TestSousDossier(t *testing.T) {
	racine := t.TempDir() // un chemin absolu, lettre du disque comprise sous Windows
	base := filepath.Join(racine, "a", "COF DM - données")
	for _, c := range []struct {
		chemin string
		ok     bool
	}{
		{filepath.Join(base, "x.cofdm"), true},
		{base, true},
		{base + " (repris)", false},
		{filepath.Join(racine, "a", "b.cofdm"), false},
		{"relatif.cofdm", false},
	} {
		if _, ok := sousDossier(c.chemin, base, racine); ok != c.ok {
			t.Errorf("%s : %v", c.chemin, ok)
		}
	}
}
