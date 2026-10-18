// Package reglages garde les réglages de l'application sur cet ordinateur : l'univers ouvert, les
// univers récents, le mot de passe MJ, le jeton d'API. Ils ne voyagent pas
// avec les fichiers d'univers.
package reglages

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Fichier est un petit dictionnaire enregistré en JSON, lisible par son seul propriétaire.
type Fichier struct {
	chemin string
	mu     sync.Mutex
	m      map[string]string
}

// Ouvrir lit le fichier de réglages ; absent ou illisible, les réglages sont vides.
func Ouvrir(chemin string) *Fichier {
	f := &Fichier{chemin: chemin, m: map[string]string{}}
	if b, err := os.ReadFile(chemin); err == nil {
		_ = json.Unmarshal(b, &f.m)
	}
	return f
}

func (f *Fichier) Meta(cle string) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.m[cle]
	return v, ok
}

// SetMeta enregistre une valeur ; vide, elle est retirée.
func (f *Fichier) SetMeta(cle, valeur string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	ancienne, existait := f.m[cle]
	if valeur == "" {
		delete(f.m, cle)
	} else {
		f.m[cle] = valeur
	}
	if err := f.ecrire(); err != nil {
		if existait {
			f.m[cle] = ancienne
		} else {
			delete(f.m, cle)
		}
		return err
	}
	return nil
}

// ecrire remplace le fichier d'un seul geste, pour ne jamais le laisser à moitié écrit.
func (f *Fichier) ecrire() error {
	b, err := json.MarshalIndent(f.m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(f.chemin), 0o700); err != nil {
		return err
	}
	tmp := f.chemin + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, f.chemin)
}
