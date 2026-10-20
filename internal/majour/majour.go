// Package majour met à jour l'application de bureau depuis les versions publiées sur GitHub.
//
// Deux canaux : « stable » (les versions vX.Y.Z) et « test » (en plus, les préversions
// vX.Y.Z-test.N, compilées depuis une PR pour l'essayer avant de la fusionner). Rien ne s'installe
// sans le MJ : l'application cherche, signale, et installe sur son clic.
//
// Chaque version publie SHA256SUMS (empreintes de ses fichiers) et SHA256SUMS.sig, sa signature
// Ed25519 (en base64) par la clé de publication, gardée dans les secrets du dépôt. L'application ne connaît que
// la clé publique (ClePublique) : un fichier qui ne correspond pas à une liste signée par elle est
// refusé, d'où qu'il vienne.
package majour

import (
	"bufio"
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	Stable = "stable"
	Test   = "test"

	fichierEmpreintes = "SHA256SUMS"
	fichierSignature  = "SHA256SUMS.sig"
	tailleMax         = 300 << 20 // un programme publié
	suffixeAncien     = ".ancien"
)

// ClePublique : la clé publique Ed25519 de publication (base64). Variable pour les tests seulement.
var ClePublique = "3+LE+cwQfJzLJZ1iqtMiO3t62iYvIiDKSONxFgAVKVM="

// API : l'adresse des versions publiées. Variable pour les tests seulement.
var API = "https://api.github.com/repos/Tchyze/COF_DM/releases"

var client = &http.Client{Timeout: 10 * time.Minute}

// ---------------------------------------------------------------- versions

// Version : X.Y.Z, avec une préversion éventuelle (« test.3 »).
type Version struct {
	Maj, Min, Cor int
	Pre           string
}

// Lire lit « v1.2.3 », « 1.2.3 » ou « 1.2.3-test.4 ». Une version de développement (« dev »,
// « 0.0.0-git1a2b3c4 ») ne se lit pas : elle ne se met pas à jour seule. Ce qui suit « + » (un
// serveur construit quelques commits après la version : « 1.2.3+4.1a2b3c4 ») ne compte pas.
func Lire(s string) (Version, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	s, _, _ = strings.Cut(s, "+")
	var v Version
	base, pre, _ := strings.Cut(s, "-")
	parts := strings.Split(base, ".")
	if len(parts) != 3 {
		return v, false
	}
	nums := make([]int, 3)
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return v, false
		}
		nums[i] = n
	}
	v.Maj, v.Min, v.Cor, v.Pre = nums[0], nums[1], nums[2], pre
	if strings.HasPrefix(pre, "git") || (v == Version{}) {
		return v, false
	}
	return v, true
}

func (v Version) String() string {
	s := fmt.Sprintf("%d.%d.%d", v.Maj, v.Min, v.Cor)
	if v.Pre != "" {
		s += "-" + v.Pre
	}
	return s
}

// Comparer : -1, 0 ou 1. Une préversion passe avant sa version (1.2.0-test.9 < 1.2.0) ; deux
// préversions se comparent par leurs parties, nombres comme des nombres (test.10 > test.9).
func Comparer(a, b Version) int {
	for _, d := range [][2]int{{a.Maj, b.Maj}, {a.Min, b.Min}, {a.Cor, b.Cor}} {
		if d[0] != d[1] {
			if d[0] < d[1] {
				return -1
			}
			return 1
		}
	}
	switch {
	case a.Pre == b.Pre:
		return 0
	case a.Pre == "":
		return 1
	case b.Pre == "":
		return -1
	}
	pa, pb := strings.Split(a.Pre, "."), strings.Split(b.Pre, ".")
	for i := 0; i < len(pa) && i < len(pb); i++ {
		na, ea := strconv.Atoi(pa[i])
		nb, eb := strconv.Atoi(pb[i])
		switch {
		case ea == nil && eb == nil && na != nb:
			if na < nb {
				return -1
			}
			return 1
		case (ea != nil || eb != nil) && pa[i] != pb[i]:
			if pa[i] < pb[i] {
				return -1
			}
			return 1
		}
	}
	switch {
	case len(pa) < len(pb):
		return -1
	case len(pa) > len(pb):
		return 1
	}
	return 0
}

// ---------------------------------------------------------------- recherche

type fichier struct {
	Nom    string `json:"name"`
	URL    string `json:"browser_download_url"`
	Taille int64  `json:"size"`
}

type publication struct {
	Tag        string    `json:"tag_name"`
	Brouillon  bool      `json:"draft"`
	Preversion bool      `json:"prerelease"`
	Notes      string    `json:"body"`
	Page       string    `json:"html_url"`
	Date       string    `json:"published_at"`
	Fichiers   []fichier `json:"assets"`
}

// Disponible : une version plus récente que celle qui tourne.
type Disponible struct {
	Version    string `json:"version"`
	Preversion bool   `json:"preversion"`
	Notes      string `json:"notes"`
	Page       string `json:"page"`
	Date       string `json:"date"`
	Fichier    string `json:"fichier"` // le fichier qui remplacera ce programme
	Taille     int64  `json:"taille"`
	url        string
	empreintes string
	signature  string
}

// Programme : le fichier à remplacer pour mettre à jour ce programme, et le nom du fichier publié
// qui lui correspond. Vide : pas de mise à jour automatique ici (programme lancé par go run, binaire
// Linux hors AppImage, autre système).
func Programme() (chemin string, publie func(string) bool) {
	switch runtime.GOOS {
	case "windows":
		exe, err := os.Executable()
		if err != nil {
			return "", nil
		}
		if e, err := filepath.EvalSymlinks(exe); err == nil {
			exe = e
		}
		return exe, func(nom string) bool { return strings.EqualFold(nom, "COF-DM.exe") }
	case "linux":
		if a := os.Getenv("APPIMAGE"); a != "" {
			return a, func(nom string) bool { return strings.HasSuffix(nom, "-x86_64.AppImage") }
		}
	}
	return "", nil
}

// Chercher : la version la plus récente du canal, si elle dépasse « actuelle » (nil sinon). Le
// canal test voit aussi les versions stables : il suit la plus récente des deux.
func Chercher(canal, actuelle string) (*Disponible, error) {
	ici, ok := Lire(actuelle)
	if !ok {
		ici = Version{} // une version de développement : toute version publiée est plus récente
	}
	req, _ := http.NewRequest("GET", API+"?per_page=30", nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "COF-DM/"+actuelle)
	rep, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("GitHub ne répond pas : %w", err)
	}
	defer rep.Body.Close()
	switch {
	case rep.StatusCode == 404:
		return nil, errors.New("versions introuvables sur GitHub (dépôt privé ou déplacé)")
	case rep.StatusCode == 403 || rep.StatusCode == 429:
		return nil, errors.New("GitHub limite les recherches : réessayez dans une heure")
	case rep.StatusCode != 200:
		return nil, fmt.Errorf("GitHub répond %d", rep.StatusCode)
	}
	var pubs []publication
	if err := json.NewDecoder(io.LimitReader(rep.Body, 8<<20)).Decode(&pubs); err != nil {
		return nil, fmt.Errorf("liste des versions illisible : %w", err)
	}
	_, publie := Programme()
	var meilleure *publication
	var vm Version
	for i := range pubs {
		p := &pubs[i]
		v, ok := Lire(p.Tag)
		if !ok || p.Brouillon || (p.Preversion && canal != Test) || (v.Pre != "" && canal != Test) {
			continue
		}
		if meilleure == nil || Comparer(v, vm) > 0 {
			meilleure, vm = p, v
		}
	}
	if meilleure == nil || Comparer(vm, ici) <= 0 {
		return nil, nil
	}
	d := &Disponible{Version: vm.String(), Preversion: vm.Pre != "", Notes: meilleure.Notes, Page: meilleure.Page, Date: meilleure.Date}
	for _, f := range meilleure.Fichiers {
		switch {
		case f.Nom == fichierEmpreintes:
			d.empreintes = f.URL
		case f.Nom == fichierSignature:
			d.signature = f.URL
		case publie != nil && publie(f.Nom):
			d.Fichier, d.Taille, d.url = f.Nom, f.Taille, f.URL
		}
	}
	return d, nil
}

// Installable : la version porte un programme pour ce système, ses empreintes et leur signature.
func (d *Disponible) Installable() bool {
	return d != nil && d.url != "" && d.empreintes != "" && d.signature != ""
}

// ---------------------------------------------------------------- installation

func telecharger(url string, max int64, w io.Writer) error {
	rep, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("téléchargement impossible : %w", err)
	}
	defer rep.Body.Close()
	if rep.StatusCode != 200 {
		return fmt.Errorf("téléchargement refusé (%d)", rep.StatusCode)
	}
	n, err := io.Copy(w, io.LimitReader(rep.Body, max+1))
	if err != nil {
		return fmt.Errorf("téléchargement interrompu : %w", err)
	}
	if n > max {
		return errors.New("fichier trop gros")
	}
	return nil
}

// Verifier : la liste des empreintes est signée par la clé de publication, et elle donne pour
// « nom » l'empreinte de « contenu ».
func Verifier(empreintes, signature []byte, nom string, somme []byte) error {
	cle, err := base64.StdEncoding.DecodeString(ClePublique)
	if err != nil || len(cle) != ed25519.PublicKeySize {
		return errors.New("clé de publication illisible")
	}
	sig, err := base64.StdEncoding.DecodeString(string(bytes.TrimSpace(signature)))
	if err != nil || !ed25519.Verify(ed25519.PublicKey(cle), empreintes, sig) {
		return errors.New("signature invalide : cette version n'a pas été publiée par COF DM")
	}
	attendu := hex.EncodeToString(somme)
	sc := bufio.NewScanner(bytes.NewReader(empreintes))
	for sc.Scan() {
		champs := strings.Fields(sc.Text())
		if len(champs) == 2 && strings.TrimPrefix(champs[1], "*") == nom {
			if strings.EqualFold(champs[0], attendu) {
				return nil
			}
			return errors.New("fichier altéré : son empreinte ne correspond pas à la liste signée")
		}
	}
	return fmt.Errorf("%s absent de la liste signée", nom)
}

// Installer télécharge la version, la vérifie, puis remplace le programme. Renvoie le chemin du
// programme à relancer. Le programme d'avant reste à côté (« .ancien ») jusqu'au lancement suivant.
func Installer(d *Disponible) (string, error) {
	if !d.Installable() {
		return "", errors.New("cette version ne peut pas s'installer seule ici : téléchargez-la depuis sa page")
	}
	cible, _ := Programme()
	if cible == "" {
		return "", errors.New("ce programme ne se met pas à jour seul (version de développement)")
	}
	var empreintes, signature bytes.Buffer
	if err := telecharger(d.empreintes, 1<<20, &empreintes); err != nil {
		return "", err
	}
	if err := telecharger(d.signature, 4<<10, &signature); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(filepath.Dir(cible), ".cof-dm-maj-*")
	if err != nil {
		return "", fmt.Errorf("dossier du programme non modifiable : %w", err)
	}
	defer os.Remove(tmp.Name())
	h := sha256.New()
	err = telecharger(d.url, tailleMax, io.MultiWriter(tmp, h))
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return "", err
	}
	if err := Verifier(empreintes.Bytes(), signature.Bytes(), d.Fichier, h.Sum(nil)); err != nil {
		return "", err
	}
	if err := Remplacer(cible, tmp.Name()); err != nil {
		return "", err
	}
	return cible, nil
}

// Remplacer met « nouveau » à la place de « cible ». Un programme en cours ne s'écrase pas sous
// Windows : il est d'abord renommé en « .ancien ».
func Remplacer(cible, nouveau string) error {
	if err := os.Chmod(nouveau, 0o755); err != nil {
		return err
	}
	ancien := cible + suffixeAncien
	_ = os.Remove(ancien)
	if err := os.Rename(cible, ancien); err != nil {
		return fmt.Errorf("programme non remplaçable : %w", err)
	}
	if err := os.Rename(nouveau, cible); err != nil {
		_ = os.Rename(ancien, cible) // rien n'a changé
		return fmt.Errorf("programme non remplaçable : %w", err)
	}
	return nil
}

// Nettoyer retire le programme d'avant la dernière mise à jour, une fois le nouveau lancé.
func Nettoyer() {
	if cible, _ := Programme(); cible != "" {
		_ = os.Remove(cible + suffixeAncien)
	}
}
