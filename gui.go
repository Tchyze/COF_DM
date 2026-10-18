//go:build !nogui

package main

import (
	"runtime"

	webview "github.com/webview/webview_go"
)

// La fenêtre native doit vivre sur le thread principal du système.
func init() { runtime.LockOSThread() }

// ouvrirFenetre affiche l'interface dans une fenêtre native et bloque jusqu'à sa fermeture.
// Renvoie false si aucune vue web n'est disponible (WebView2 absent sous Windows, par exemple).
func ouvrirFenetre(url string) bool {
	w := webview.New(false)
	if w == nil {
		return false
	}
	defer w.Destroy()
	w.SetTitle("COF DM")
	w.SetSize(1400, 900, webview.HintNone)
	w.Navigate(url)
	w.Run()
	return true
}
