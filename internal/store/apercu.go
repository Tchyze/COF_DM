package store

import (
	"database/sql"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// ApercuUnivers : ce qu'on peut dire d'un univers fermé sans l'ouvrir.
type ApercuUnivers struct {
	ID    string
	Copie bool
}

// Apercu lit un univers fermé en lecture seule : contrairement à Ouvrir, il n'y écrit jamais rien
// (ni schéma, ni migration, ni identifiant) et ne crée aucun fichier. ErrPasUnivers : ce fichier n'est
// pas un univers COF DM.
func Apercu(chemin string) (ApercuUnivers, error) {
	info, err := os.Stat(chemin)
	if err != nil {
		return ApercuUnivers{}, err
	}
	if !info.Mode().IsRegular() {
		return ApercuUnivers{}, ErrPasUnivers
	}
	abs, err := filepath.Abs(chemin)
	if err != nil {
		return ApercuUnivers{}, err
	}
	db, err := sql.Open(pilote, adresseLecture(abs))
	if err != nil {
		return ApercuUnivers{}, err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	var marque, fiches int
	err = db.QueryRow(`PRAGMA application_id`).Scan(&marque)
	if err == nil {
		err = db.QueryRow(`SELECT COALESCE(SUM(name='entites'),0) FROM sqlite_master WHERE type='table'`).Scan(&fiches)
	}
	if err != nil {
		if strings.Contains(err.Error(), "not a database") {
			return ApercuUnivers{}, ErrPasUnivers
		}
		return ApercuUnivers{}, err
	}
	// sans marque : un univers d'une ancienne version, qu'Ouvrir marquera
	if marque != idApplication && (marque != 0 || fiches == 0) {
		return ApercuUnivers{}, ErrPasUnivers
	}
	var a ApercuUnivers
	if err := db.QueryRow(`SELECT valeur FROM meta WHERE cle='univers_id'`).Scan(&a.ID); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return ApercuUnivers{}, err
	}
	a.Copie = replique(db) != 0
	return a, nil
}

// adresseLecture : l'adresse SQLite d'un fichier ouvert en lecture seule. Un chemin brut couperait au
// premier « ? » et lirait mal « # » ou « % ».
func adresseLecture(abs string) string {
	p := filepath.ToSlash(abs)
	if !strings.HasPrefix(p, "/") { // C:/… sous Windows
		p = "/" + p
	}
	u := url.URL{Scheme: "file", Path: p, RawQuery: "mode=ro&_query_only=1&_busy_timeout=200"}
	return u.String()
}
