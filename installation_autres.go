//go:build !windows

package main

import "runtime"

// installerAuLancement : rien à installer hors de Windows (l'AppImage se lance d'où elle est, et
// l'application Mac se glisse dans le dossier Applications).
func installerAuLancement(string) (bool, string) { return false, "" }

// rangerCacheFenetre : rien à ranger hors de Windows.
func rangerCacheFenetre(string) {}

func desinstaller() {
	if runtime.GOOS == "darwin" {
		fatal("La désinstallation ne concerne que Windows : sur Mac, mettez COF DM à la corbeille depuis le dossier Applications (vos données restent dans %s).", dossierParDefaut())
	}
	fatal("La désinstallation ne concerne que Windows : sous Linux, supprimez l'AppImage (vos données restent dans %s).", dossierParDefaut())
}
