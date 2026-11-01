//go:build !windows

package main

// installerAuLancement : rien à installer hors de Windows (l'AppImage se lance d'où elle est).
func installerAuLancement(string) (bool, string) { return false, "" }

// rangerCacheFenetre : rien à ranger hors de Windows.
func rangerCacheFenetre(string) {}

func desinstaller() {
	fatal("La désinstallation ne concerne que Windows : sous Linux, supprimez l'AppImage (vos données restent dans %s).", dossierParDefaut())
}
