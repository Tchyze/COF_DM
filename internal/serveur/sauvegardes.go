package serveur

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"

	"github.com/Tchyze/cof_dm/internal/store"
)

// Sauvegardes d'un univers : des copies complètes du fichier (fiches, images, journal), rangées dans
// le dossier de l'application et non à côté de l'univers, qui reste un seul fichier.
//
// Les sauvegardes automatiques se font à l'ouverture de l'univers, toutes les demi-heures et à la
// fermeture, seulement si l'univers a changé depuis la dernière. Les plus récentes sont gardées,
// puis une par jour ; les sauvegardes demandées par le MJ et celles d'avant une restauration ne
// sont jamais effacées.
const (
	motifAuto           = "auto"
	sauvegardesRecentes = 5
	sauvegardesJours    = 14
	intervalleAuto      = 30 * time.Minute
	formatSauvegarde    = "2006-01-02_15-04-05"
)

var reSauvegarde = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}_\d{2}-\d{2}-\d{2})_([a-z-]+)\.cofdm$`)

// Sauvegarde décrit un fichier de sauvegarde.
type Sauvegarde struct {
	Fichier string  `json:"fichier"`
	Quand   float64 `json:"quand"`
	Motif   string  `json:"motif"`
	Taille  int64   `json:"taille"`

	modifie time.Time
}

// sauvegarder copie l'univers dans dossier/<date>_<motif>.cofdm et renvoie le fichier écrit.
func sauvegarder(st *store.Store, dossier, motif string) (string, error) {
	f := filepath.Join(dossier, time.Now().Format(formatSauvegarde)+"_"+motif+store.Extension)
	if _, err := os.Stat(f); err == nil {
		return f, nil // déjà faite dans la même seconde
	}
	if err := st.Sauvegarder(f); err != nil {
		_ = os.Remove(f)
		return "", err
	}
	return f, nil
}

// listerSauvegardes renvoie les sauvegardes du dossier, la plus récente d'abord.
func listerSauvegardes(dossier string) []Sauvegarde {
	out := []Sauvegarde{}
	entrees, _ := os.ReadDir(dossier)
	for _, e := range entrees {
		m := reSauvegarde.FindStringSubmatch(e.Name())
		info, err := e.Info()
		if m == nil || err != nil || !info.Mode().IsRegular() {
			continue
		}
		quand, err := time.ParseInLocation(formatSauvegarde, m[1], time.Local)
		if err != nil {
			continue
		}
		out = append(out, Sauvegarde{Fichier: e.Name(), Quand: float64(quand.Unix()), Motif: m[2], Taille: info.Size(), modifie: info.ModTime()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Fichier > out[j].Fichier })
	return out
}

// aSauvegarder : l'univers a été modifié depuis sa dernière sauvegarde.
func aSauvegarder(st *store.Store, dossier string) bool {
	info, err := os.Stat(st.Chemin())
	if err != nil {
		return false
	}
	for _, s := range listerSauvegardes(dossier) {
		if !s.modifie.Before(info.ModTime()) {
			return false
		}
	}
	return true
}

// sauvegardeAuto fait une sauvegarde si l'univers a des fiches et a changé, puis efface les
// sauvegardes automatiques périmées.
func sauvegardeAuto(st *store.Store, dossier string) (string, error) {
	if _, fiches, _ := st.Compter(); fiches == 0 || !aSauvegarder(st, dossier) {
		return "", nil // rien à protéger, ou rien de neuf
	}
	f, err := sauvegarder(st, dossier, motifAuto)
	if err != nil {
		return "", err
	}
	purgerSauvegardes(dossier, time.Now())
	return f, nil
}

// purgerSauvegardes ne garde, parmi les sauvegardes automatiques, que les plus récentes et la
// dernière de chaque jour sur la période de conservation.
func purgerSauvegardes(dossier string, maintenant time.Time) {
	limite := maintenant.AddDate(0, 0, -sauvegardesJours)
	gardees, jours := 0, map[string]bool{}
	for _, s := range listerSauvegardes(dossier) {
		if s.Motif != motifAuto {
			continue
		}
		if gardees < sauvegardesRecentes {
			gardees++
			jours[s.Fichier[:10]] = true
			continue
		}
		jour := s.Fichier[:10]
		if !jours[jour] && time.Unix(int64(s.Quand), 0).After(limite) {
			jours[jour] = true
			continue
		}
		_ = os.Remove(filepath.Join(dossier, s.Fichier))
	}
}
