package mcp

import (
	"os"
	"testing"

	"github.com/Tchyze/cof_dm/internal/modele"
)

// Les tests de ce paquet partent d'univers où tout est activé ; les fonctionnalités coupées ont
// leurs propres tests (TestFonctionnalites*), qui les coupent explicitement. Les valeurs par
// défaut réelles sont vérifiées dans internal/store.
func TestMain(m *testing.M) {
	for i := range modele.Fonctionnalites {
		modele.Fonctionnalites[i].Defaut = true
	}
	os.Exit(m.Run())
}
