//go:build windows

package comptes

// Verrouiller : sous Windows, rien à garder. Le mode serveur, seul à s'en servir, n'y existe pas.
func Verrouiller(dossier string) (liberer func(), err error) { return func() {}, nil }
