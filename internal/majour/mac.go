package majour

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Sur macOS, le programme vit dans une application (« COF DM.app/Contents/MacOS/cof-dm ») signée
// comme un tout : une mise à jour remplace l'application entière, jamais le seul programme (son
// sceau ne tiendrait plus). La version publie pour cela l'application en zip (« -macOS.zip »), à
// côté de l'image disque (« .dmg ») qui sert à la première installation.

const (
	suffixeMac     = "-macOS.zip"
	tailleMaxApp   = 600 << 20 // une application dépliée
	fichiersMaxMac = 1000
)

// appDe : l'application macOS qui contient ce programme (« …/COF DM.app »), ou "" s'il n'est pas
// dans une application (programme lancé par go run, binaire copié à la main).
func appDe(exe string) string {
	macos := filepath.Dir(exe)
	contents := filepath.Dir(macos)
	app := filepath.Dir(contents)
	if filepath.Base(macos) != "MacOS" || filepath.Base(contents) != "Contents" || !strings.HasSuffix(app, ".app") {
		return ""
	}
	return app
}

// Lanceur : le fichier à exécuter pour relancer « programme », que Programme a désigné : le
// programme lui-même, ou celui d'une application macOS.
func Lanceur(programme string) string {
	if !strings.HasSuffix(programme, ".app") {
		return programme
	}
	if l, err := os.ReadDir(filepath.Join(programme, "Contents", "MacOS")); err == nil {
		for _, e := range l {
			if !e.IsDir() {
				return filepath.Join(programme, "Contents", "MacOS", e.Name())
			}
		}
	}
	return filepath.Join(programme, "Contents", "MacOS", "cof-dm")
}

// deballerApp déplie le zip d'une application macOS dans un dossier neuf, à côté de « cible »
// (l'application à remplacer : même disque, pour la renommer ensuite), et renvoie le chemin de
// l'application dépliée ainsi que ce dossier, à retirer après usage. Le zip ne porte qu'une
// application à sa racine ; un chemin qui en sortirait, un lien ou un contenu démesuré est refusé.
func deballerApp(archive, cible string) (app, dossier string, err error) {
	z, err := zip.OpenReader(archive)
	if err != nil {
		return "", "", fmt.Errorf("archive de l'application illisible : %w", err)
	}
	defer z.Close()
	if len(z.File) > fichiersMaxMac {
		return "", "", errors.New("archive de l'application démesurée")
	}
	tmp, err := os.MkdirTemp(filepath.Dir(cible), prefixeTelecharge+"app-*")
	if err != nil {
		return "", "", errNonModifiable(cible)
	}
	dossier = tmp
	defer func() {
		if err != nil {
			os.RemoveAll(tmp) // « dossier » est déjà vidé par le return
		}
	}()
	racine := ""
	var total int64
	for _, f := range z.File {
		nom := filepath.FromSlash(f.Name)
		if !filepath.IsLocal(nom) {
			return "", "", fmt.Errorf("archive de l'application refusée : chemin %q", f.Name)
		}
		premier := strings.SplitN(filepath.ToSlash(nom), "/", 2)[0]
		if strings.HasPrefix(premier, "__MACOSX") {
			continue // métadonnées ajoutées par certains outils
		}
		if !strings.HasSuffix(premier, ".app") || (racine != "" && premier != racine) {
			return "", "", fmt.Errorf("archive de l'application refusée : %q hors de l'application", f.Name)
		}
		racine = premier
		mode := f.Mode()
		chemin := filepath.Join(dossier, nom)
		switch {
		case mode.IsDir():
			if err := os.MkdirAll(chemin, 0o755); err != nil {
				return "", "", err
			}
		case mode.IsRegular():
			total += int64(f.UncompressedSize64)
			if total > tailleMaxApp {
				return "", "", errors.New("archive de l'application démesurée")
			}
			if err := extraire(f, chemin, mode.Perm()|0o600); err != nil {
				return "", "", err
			}
		default:
			return "", "", fmt.Errorf("archive de l'application refusée : %q n'est pas un fichier", f.Name)
		}
	}
	if racine == "" {
		return "", "", errors.New("archive sans application")
	}
	app = filepath.Join(dossier, racine)
	if _, err := os.Stat(filepath.Join(app, "Contents", "Info.plist")); err != nil {
		return "", "", errors.New("archive de l'application incomplète (Info.plist absent)")
	}
	return app, dossier, nil
}

func extraire(f *zip.File, chemin string, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(chemin), 0o755); err != nil {
		return err
	}
	r, err := f.Open()
	if err != nil {
		return err
	}
	defer r.Close()
	w, err := os.OpenFile(chemin, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return err
	}
	_, err = io.Copy(w, io.LimitReader(r, int64(f.UncompressedSize64)+1))
	if cerr := w.Close(); err == nil {
		err = cerr
	}
	return err
}
