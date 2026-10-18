//go:build nogui

package main

// Version sans fenêtre (Docker, tests) : l'interface s'ouvre dans le navigateur.
func ouvrirFenetre(string) bool { return false }
