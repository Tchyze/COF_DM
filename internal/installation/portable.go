// Package installation installe l'application de bureau pour l'utilisateur, sans droits
// d'administrateur (Windows : %LOCALAPPDATA%\Programs\COF DM, raccourcis, « Applications
// installées »), et reprend les données de l'ancien mode portable. Il ne dépend ni de SQLite ni de
// la fenêtre : ses tests se compilent pour Windows et s'y lancent tels quels.
package installation

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// NomPortable : le dossier que l'ancien mode portable posait à côté du programme (ou de l'AppImage).
const NomPortable = "COF DM - données"

// Clés de reglages.json qui portent des chemins d'univers (celles d'internal/serveur/univers.go).
const (
	cleUnivers = "univers"
	cleRecents = "recents"
	recentsMax = 8
)

// Reprise : ce que sont devenues les données d'un ancien dossier portable.
type Reprise struct {
	Depuis        string // l'ancien dossier portable
	Vers          string // où sont ses données maintenant
	Avertissement string // à montrer au MJ, s'il y a lieu
}

// PortableDe : le dossier portable posé à côté de ce programme, ou "".
func PortableDe(exe string) string {
	if exe == "" {
		return ""
	}
	d := filepath.Join(filepath.Dir(exe), NomPortable)
	if fi, err := os.Stat(d); err == nil && fi.IsDir() {
		return d
	}
	return ""
}

// ReprendrePortable passe les données d'un dossier portable dans dest (le dossier de données de
// l'utilisateur), sans rien perdre. Le mode portable n'existe plus : ses données rejoignent les
// autres.
//   - dest absent ou vide : le dossier y est déplacé (copié, s'il est sur un autre disque) ;
//   - dest a déjà des données : le dossier portable est copié dans « dest/Repris du mode portable
//     AAAA-MM-JJ », et son dernier univers rejoint les récents.
//
// Les chemins d'univers rangés dans les réglages suivent. Un dossier d'origine qui reste (copie) est
// renommé « … (repris) », pour ne pas être repris une seconde fois.
func ReprendrePortable(portable, dest string) (*Reprise, error) {
	r := &Reprise{Depuis: portable, Vers: dest}
	if vide(dest) {
		if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
			return nil, err
		}
		_ = os.Remove(dest) // vide : le déplacement le recrée
		if err := os.Rename(portable, dest); err != nil {
			if err := copier(portable, dest); err != nil {
				return nil, fmt.Errorf("données du mode portable non reprises : %w", err)
			}
			r.Avertissement = marquerRepris(portable)
		}
		if err := recalerChemins(filepath.Join(dest, "reglages.json"), portable, dest); err != nil {
			return r, err
		}
		return r, nil
	}
	r.Vers = filepath.Join(dest, "Repris du mode portable "+time.Now().Format("2006-01-02"))
	if _, err := os.Stat(r.Vers); err == nil {
		r.Vers += time.Now().Format(" 15h04m05")
	}
	if err := copier(portable, r.Vers); err != nil {
		return nil, fmt.Errorf("données du mode portable non reprises : %w", err)
	}
	if err := recalerChemins(filepath.Join(r.Vers, "reglages.json"), portable, r.Vers); err != nil {
		return r, err
	}
	if err := ajouterRecent(filepath.Join(dest, "reglages.json"), filepath.Join(r.Vers, "reglages.json")); err != nil {
		return r, err
	}
	r.Avertissement = fmt.Sprintf("Le mode portable n'existe plus : les données de « %s » ont été copiées dans « %s ». Leurs univers se rouvrent par Fichier → Récents ou Fichier → Ouvrir.", portable, r.Vers)
	if m := marquerRepris(portable); m != "" {
		r.Avertissement += " " + m
	}
	return r, nil
}

// marquerRepris renomme l'ancien dossier, recopié ailleurs ; renvoie un mot pour le MJ s'il reste
// sous son nom (un autre programme le tient).
func marquerRepris(portable string) string {
	cible := portable + " (repris)"
	for i := 2; ; i++ {
		if _, err := os.Lstat(cible); errors.Is(err, os.ErrNotExist) {
			break
		}
		cible = fmt.Sprintf("%s (repris %d)", portable, i)
	}
	if err := os.Rename(portable, cible); err != nil {
		return fmt.Sprintf("L'ancien dossier « %s » est resté en place : vous pouvez le supprimer, ses données sont copiées.", portable)
	}
	return ""
}

func vide(d string) bool {
	e, err := os.ReadDir(d)
	return errors.Is(err, os.ErrNotExist) || (err == nil && len(e) == 0)
}

// copier recopie un dossier entier (fichiers et sous-dossiers, avec leurs droits).
func copier(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		cible := filepath.Join(dst, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.MkdirAll(cible, info.Mode().Perm()|0o700)
		}
		if !info.Mode().IsRegular() {
			return nil // ni lien ni fichier spécial dans un dossier de données
		}
		return copierFichier(p, cible, info.Mode().Perm())
	})
}

func copierFichier(src, dst string, mode fs.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode|0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// recalerChemins : dans un fichier de réglages, les chemins sous « ancien » passent sous « nouveau »
// (valeur seule, ou liste JSON de chemins). Absent : rien à faire.
func recalerChemins(fichier, ancien, nouveau string) error {
	m, err := lireReglages(fichier)
	if err != nil || m == nil {
		return err
	}
	change := false
	for k, v := range m {
		if n, ok := sousDossier(v, ancien, nouveau); ok {
			m[k], change = n, true
			continue
		}
		var l []string
		if json.Unmarshal([]byte(v), &l) != nil || len(l) == 0 {
			continue
		}
		modifiee := false
		for i, c := range l {
			if n, ok := sousDossier(c, ancien, nouveau); ok {
				l[i], modifiee = n, true
			}
		}
		if modifiee {
			b, _ := json.Marshal(l)
			m[k], change = string(b), true
		}
	}
	if !change {
		return nil
	}
	return ecrireReglages(fichier, m)
}

// ajouterRecent met le dernier univers des réglages repris en tête des récents de dest.
func ajouterRecent(dest, repris string) error {
	r, err := lireReglages(repris)
	if err != nil || r[cleUnivers] == "" {
		return err
	}
	m, err := lireReglages(dest)
	if err != nil {
		return err
	}
	if m == nil {
		m = map[string]string{}
	}
	var l []string
	_ = json.Unmarshal([]byte(m[cleRecents]), &l)
	nouv := []string{r[cleUnivers]}
	for _, c := range l {
		if c != r[cleUnivers] && len(nouv) < recentsMax {
			nouv = append(nouv, c)
		}
	}
	b, _ := json.Marshal(nouv)
	m[cleRecents] = string(b)
	return ecrireReglages(dest, m)
}

func lireReglages(fichier string) (map[string]string, error) {
	b, err := os.ReadFile(fichier)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	m := map[string]string{}
	if json.Unmarshal(b, &m) != nil {
		return nil, nil // illisible : laissé tel quel, comme le fait reglages.Ouvrir
	}
	return m, nil
}

func ecrireReglages(fichier string, m map[string]string) error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(fichier, b, 0o600)
}

// sousDossier : « chemin » est-il « ancien » ou dedans ? Renvoie alors le même chemin sous « nouveau ».
func sousDossier(chemin, ancien, nouveau string) (string, bool) {
	if !filepath.IsAbs(chemin) {
		return "", false
	}
	rel, err := filepath.Rel(ancien, chemin)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.Join(nouveau, rel), true
}

// MemeChemin compare deux chemins comme le système de fichiers (sans la casse sous Windows).
func MemeChemin(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}
