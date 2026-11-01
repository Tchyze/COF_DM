package installation

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

const (
	// VariableCacheFenetre : WebView2 y lit où ranger son cache, à la place de son paramètre.
	VariableCacheFenetre = "WEBVIEW2_USER_DATA_FOLDER"

	nomCacheFenetre = "webview"
	ancienCache     = "COF-DM.exe" // %APPDATA%\COF-DM.exe : où WebView2 le rangeait, au nom du programme
	sousDossierVue  = "EBWebView"
)

// CacheFenetre : le cache de la fenêtre (WebView2), rangé avec les données de l'application. Il ne
// tient que ce que garde la vue web : préférences de l'interface, cache des pages.
func CacheFenetre(donnees string) string { return filepath.Join(donnees, nomCacheFenetre) }

// RetirerAncienCache retire %APPDATA%\COF-DM.exe, s'il ne contient que le cache de WebView2. Une
// fenêtre encore ouverte le tient : il sera retiré à un prochain lancement.
func RetirerAncienCache(appdata string) error {
	if appdata == "" {
		return nil
	}
	d := filepath.Join(appdata, ancienCache)
	e, err := os.ReadDir(d)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(e) != 1 || !e[0].IsDir() || e[0].Name() != sousDossierVue {
		return nil // autre chose que ce cache : on n'y touche pas
	}
	return os.RemoveAll(d)
}
