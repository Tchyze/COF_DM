//go:build !windows

package comptes

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

// Verrouiller réserve le fichier des comptes de ce dossier à ce programme, tant qu'il tourne ou jusqu'à
// liberer. Le système retire le verrou quand le programme s'arrête, même brutalement : rien à nettoyer
// après un conteneur tué. Sur un système de fichiers qui ne connaît pas ce verrou, l'erreur n'est pas
// ErrOccupe : c'est à l'appelant de décider.
func Verrouiller(dossier string) (liberer func(), err error) {
	f, err := os.OpenFile(filepath.Join(dossier, fichierVerrou), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return func() {}, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return func() {}, ErrOccupe
		}
		return func() {}, err
	}
	return func() { f.Close() }, nil
}
