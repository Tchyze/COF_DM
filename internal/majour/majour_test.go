package majour

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestVersions(t *testing.T) {
	for _, s := range []string{"dev", "0.0.0-git1a2b3c4", "1.2", "v1.x.0", ""} {
		if _, ok := Lire(s); ok {
			t.Fatalf("%q lue comme une version", s)
		}
	}
	ordre := []string{"v1.0.0", "1.1.0", "1.1.1-test.2", "1.1.1-test.10", "1.1.1", "v1.2.0-test.1", "1.2.0", "2.0.0"}
	for i := 0; i+1 < len(ordre); i++ {
		a, _ := Lire(ordre[i])
		b, ok := Lire(ordre[i+1])
		if !ok || Comparer(a, b) != -1 || Comparer(b, a) != 1 || Comparer(a, a) != 0 {
			t.Fatalf("%s < %s attendu", ordre[i], ordre[i+1])
		}
	}
	// un serveur construit quelques commits après la 1.2.2 : la 1.2.2, pour les comparaisons
	a, ok := Lire("1.2.2+3.45e384f")
	b, _ := Lire("1.2.2")
	if !ok || Comparer(a, b) != 0 || a.String() != "1.2.2" {
		t.Fatalf("version de serveur : %v %v", a, ok)
	}
}

// banc : une fausse API GitHub, avec ses versions et leurs fichiers, signés par une clé d'essai.
type banc struct {
	srv      *httptest.Server
	cle      ed25519.PrivateKey
	pubs     []map[string]any
	fichiers map[string][]byte
}

func nouveauBanc(t *testing.T) *banc {
	t.Helper()
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	ancienneCle, ancienneAPI := ClePublique, API
	ClePublique = base64.StdEncoding.EncodeToString(pub)
	b := &banc{cle: priv, fichiers: map[string][]byte{}}
	b.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/releases" {
			_ = json.NewEncoder(w).Encode(b.pubs)
			return
		}
		if c, ok := b.fichiers[r.URL.Path]; ok {
			_, _ = w.Write(c)
			return
		}
		http.NotFound(w, r)
	}))
	API = b.srv.URL + "/releases"
	t.Cleanup(func() { b.srv.Close(); ClePublique, API = ancienneCle, ancienneAPI })
	return b
}

// publier : une version et ses fichiers ; signer=false : une signature d'une autre clé.
func (b *banc) publier(tag string, pre bool, programme []byte, signer bool) {
	nom := "COF-DM.exe"
	if runtime.GOOS == "linux" {
		nom = "COF-DM-" + tag + "-x86_64.AppImage"
	}
	h := sha256.Sum256(programme)
	empreintes := []byte(hex.EncodeToString(h[:]) + "  " + nom + "\n")
	cle := b.cle
	if !signer {
		_, cle, _ = ed25519.GenerateKey(rand.Reader)
	}
	sig := base64.StdEncoding.EncodeToString(ed25519.Sign(cle, empreintes))
	dossier := "/telecharger/" + tag + "/"
	b.fichiers[dossier+nom], b.fichiers[dossier+"SHA256SUMS"], b.fichiers[dossier+"SHA256SUMS.sig"] = programme, empreintes, []byte(sig+"\n")
	var fs []map[string]any
	for _, f := range []string{nom, "SHA256SUMS", "SHA256SUMS.sig"} {
		fs = append(fs, map[string]any{"name": f, "browser_download_url": b.srv.URL + dossier + f, "size": len(b.fichiers[dossier+f])})
	}
	b.pubs = append(b.pubs, map[string]any{"tag_name": tag, "prerelease": pre, "body": "Notes de " + tag, "assets": fs})
}

func TestChercherEtInstaller(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "windows" {
		t.Skip("mise à jour automatique : Windows et Linux seulement")
	}
	b := nouveauBanc(t)
	b.publier("v1.1.0", false, []byte("programme 1.1.0"), true)
	b.publier("v1.2.0", false, []byte("programme 1.2.0"), true)
	b.publier("v1.2.1-test.3", true, []byte("programme de test"), true)
	b.publier("v1.0.0", false, []byte("vieux"), true)

	// le programme à remplacer : une AppImage sous Linux
	dir := t.TempDir()
	cible := filepath.Join(dir, "COF-DM.AppImage")
	if err := os.WriteFile(cible, []byte("programme 1.1.0"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("APPIMAGE", cible)
	if runtime.GOOS == "windows" {
		t.Skip("le remplacement sous Windows vise l'exécutable du test")
	}

	if d, err := Chercher(Stable, "1.2.0"); err != nil || d != nil {
		t.Fatalf("à jour : %+v %v", d, err)
	}
	d, err := Chercher(Stable, "1.1.0")
	if err != nil || d == nil || d.Version != "1.2.0" || d.Preversion || !d.Installable() {
		t.Fatalf("stable : %+v %v", d, err)
	}
	if d, _ := Chercher(Test, "1.2.0"); d == nil || d.Version != "1.2.1-test.3" || !d.Preversion {
		t.Fatalf("test : %+v", d)
	}
	if d, _ := Chercher(Stable, "0.0.0-git1a2b3c4"); d == nil || d.Version != "1.2.0" {
		t.Fatalf("version de développement : %+v", d)
	}

	chemin, err := Installer(d)
	if err != nil || chemin != cible {
		t.Fatalf("installation : %q %v", chemin, err)
	}
	if c, _ := os.ReadFile(cible); string(c) != "programme 1.2.0" {
		t.Fatalf("programme après installation : %q", c)
	}
	if c, _ := os.ReadFile(cible + suffixeAncien); string(c) != "programme 1.1.0" {
		t.Fatalf("programme d'avant : %q", c)
	}
	Nettoyer()
	if _, err := os.Stat(cible + suffixeAncien); !os.IsNotExist(err) {
		t.Fatal("programme d'avant non retiré")
	}

	// signée par une autre clé : refusée, rien ne change
	b.publier("v1.3.0", false, []byte("programme pirate"), false)
	d, _ = Chercher(Stable, "1.2.0")
	if _, err := Installer(d); err == nil {
		t.Fatal("signature d'une autre clé acceptée")
	}
	// fichier altéré après signature : refusé
	b.publier("v1.4.0", false, []byte("programme 1.4.0"), true)
	b.fichiers["/telecharger/v1.4.0/COF-DM-v1.4.0-x86_64.AppImage"] = []byte("programme modifié")
	d, _ = Chercher(Stable, "1.2.0")
	if _, err := Installer(d); err == nil {
		t.Fatal("fichier altéré accepté")
	}
	if c, _ := os.ReadFile(cible); string(c) != "programme 1.2.0" {
		t.Fatalf("programme touché par une installation refusée : %q", c)
	}
	if f, _ := filepath.Glob(filepath.Join(dir, ".cof-dm-maj-*")); len(f) != 0 {
		t.Fatalf("téléchargements laissés : %v", f)
	}
}

// verrous simule des fichiers que Windows (programme en cours) ou un antivirus tient encore : une
// opération sur un chemin verrouillé échoue tant qu'il lui reste des refus.
type verrous map[string]int

func (v verrous) bloque(chemin string) bool {
	if v[chemin] == 0 {
		return false
	}
	if v[chemin] > 0 {
		v[chemin]--
	}
	return true
}

func simulerVerrous(t *testing.T, v verrous) {
	t.Helper()
	r, x, p, pn := renommer, retirer, pauses, pausesNettoyage
	t.Cleanup(func() { renommer, retirer, pauses, pausesNettoyage = r, x, p, pn })
	pauses = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}
	pausesNettoyage = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}
	renommer = func(a, b string) error {
		if v.bloque(a) {
			return &os.LinkError{Op: "rename", Old: a, New: b, Err: errors.New("fichier utilisé par un autre processus")}
		}
		return os.Rename(a, b)
	}
	retirer = func(c string) error {
		if v.bloque(c) {
			return &os.PathError{Op: "remove", Path: c, Err: errors.New("accès refusé")}
		}
		return os.Remove(c)
	}
}

func ecrire(t *testing.T, chemin, contenu string) {
	t.Helper()
	if err := os.WriteFile(chemin, []byte(contenu), 0o755); err != nil {
		t.Fatal(err)
	}
}

func lire(t *testing.T, chemin string) string {
	t.Helper()
	c, err := os.ReadFile(chemin)
	if err != nil {
		return "(absent)"
	}
	return string(c)
}

func TestRemplacerAncienEncoreTenu(t *testing.T) {
	dir := t.TempDir()
	cible, nouveau := filepath.Join(dir, "COF-DM.exe"), filepath.Join(dir, ".cof-dm-maj-1")
	ecrire(t, cible, "v2")
	ecrire(t, cible+suffixeAncien, "v1") // le programme d'avant la mise à jour précédente, toujours tenu
	ecrire(t, nouveau, "v3")
	simulerVerrous(t, verrous{cible + suffixeAncien: -1})
	if err := Remplacer(cible, nouveau); err != nil {
		t.Fatalf("remplacement bloqué par un ancien tenu : %v", err)
	}
	if lire(t, cible) != "v3" || lire(t, cible+suffixeAncien) != "v1" || lire(t, cible+suffixeAncien+"-2") != "v2" {
		t.Fatalf("après : %q, ancien %q, ancien-2 %q", lire(t, cible), lire(t, cible+suffixeAncien), lire(t, cible+suffixeAncien+"-2"))
	}
}

func TestRemplacerVerrouPassager(t *testing.T) {
	dir := t.TempDir()
	cible, nouveau := filepath.Join(dir, "COF-DM.exe"), filepath.Join(dir, ".cof-dm-maj-1")
	ecrire(t, cible, "v1")
	ecrire(t, nouveau, "v2")
	// un antivirus lit le fichier neuf, puis le lâche
	simulerVerrous(t, verrous{nouveau: 2, cible: 1})
	if err := Remplacer(cible, nouveau); err != nil {
		t.Fatalf("verrou passager : %v", err)
	}
	if lire(t, cible) != "v2" || lire(t, cible+suffixeAncien) != "v1" {
		t.Fatalf("après : %q, ancien %q", lire(t, cible), lire(t, cible+suffixeAncien))
	}
}

func TestRemplacerEchecRienNeChange(t *testing.T) {
	dir := t.TempDir()
	cible, nouveau := filepath.Join(dir, "COF-DM.exe"), filepath.Join(dir, ".cof-dm-maj-1")
	ecrire(t, cible, "v1")
	ecrire(t, nouveau, "v2")
	simulerVerrous(t, verrous{nouveau: -1})
	if err := Remplacer(cible, nouveau); err == nil || !strings.Contains(err.Error(), "non remplaçable") {
		t.Fatalf("fichier neuf toujours tenu : %v", err)
	}
	if lire(t, cible) != "v1" {
		t.Fatalf("programme après un échec : %q", lire(t, cible))
	}
	if _, err := os.Stat(cible + suffixeAncien); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("programme d'avant laissé en « .ancien » après un échec")
	}
	// le programme lui-même ne se renomme pas : rien ne bouge non plus
	ecrire(t, nouveau, "v2")
	simulerVerrous(t, verrous{cible: -1})
	if err := Remplacer(cible, nouveau); err == nil || lire(t, cible) != "v1" {
		t.Fatalf("programme tenu : %v, %q", err, lire(t, cible))
	}
}

func TestNettoyer(t *testing.T) {
	dir := t.TempDir()
	cible := filepath.Join(dir, "COF-DM.exe")
	ecrire(t, cible, "v3")
	vieux := time.Now().Add(-2 * time.Hour)
	for _, f := range []string{"COF-DM.exe.ancien", "COF-DM.exe.ancien-2", ".cof-dm-maj-123", ".cof-dm-maj-recent", "autre.txt", "COF-DM.exe.ancien-3"} {
		ecrire(t, filepath.Join(dir, f), "x")
		if f != ".cof-dm-maj-recent" {
			if err := os.Chtimes(filepath.Join(dir, f), vieux, vieux); err != nil {
				t.Fatal(err)
			}
		}
	}
	// le programme d'avant tourne encore deux essais, un autre reste tenu tout du long
	simulerVerrous(t, verrous{filepath.Join(dir, "COF-DM.exe.ancien"): 2, filepath.Join(dir, "COF-DM.exe.ancien-3"): -1})
	err := nettoyer(cible)
	if err == nil || !strings.Contains(err.Error(), "ancien-3") {
		t.Fatalf("un fichier toujours tenu doit être signalé : %v", err)
	}
	reste, _ := os.ReadDir(dir)
	var noms []string
	for _, e := range reste {
		noms = append(noms, e.Name())
	}
	if strings.Join(noms, ",") != ".cof-dm-maj-recent,COF-DM.exe,COF-DM.exe.ancien-3,autre.txt" {
		t.Fatalf("après nettoyage : %v", noms)
	}
	// au lancement suivant, il n'est plus tenu
	simulerVerrous(t, verrous{})
	if err := nettoyer(cible); err != nil {
		t.Fatalf("lancement suivant : %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "COF-DM.exe.ancien-3")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("programme d'avant toujours là au lancement suivant")
	}
}

func TestDossierNonModifiable(t *testing.T) {
	if runtime.GOOS != "linux" || os.Geteuid() == 0 {
		t.Skip("dossier en lecture seule : Linux, hors administrateur")
	}
	dir := t.TempDir()
	cible := filepath.Join(dir, "COF-DM.AppImage")
	ecrire(t, cible, "v1")
	t.Setenv("APPIMAGE", cible)
	if !Modifiable() {
		t.Fatal("dossier modifiable vu comme non modifiable")
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o755) })
	if Modifiable() {
		t.Fatal("dossier en lecture seule vu comme modifiable")
	}
	b := nouveauBanc(t)
	b.publier("v1.2.0", false, []byte("programme 1.2.0"), true)
	d, _ := Chercher(Stable, "1.1.0")
	if _, err := Installer(d); err == nil || !strings.Contains(err.Error(), "n'accepte pas de fichier neuf") {
		t.Fatalf("installation dans un dossier en lecture seule : %v", err)
	}
	if lire(t, cible) != "v1" {
		t.Fatal("programme touché")
	}
}
