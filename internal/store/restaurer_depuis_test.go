package store

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// Un univers reçu arrive avec ses images, lues une à une.
func TestRestaurerDepuisGardeLesImages(t *testing.T) {
	source, err := Ouvrir(filepath.Join(t.TempDir(), "venu.cofdm"))
	if err != nil {
		t.Fatal(err)
	}
	defer source.Fermer()
	e, err := source.Creer("lieu", "Fort Boueux", nil, nil, "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.AjouterMedia(e.ID, strings.Repeat("a", 20)+".png", "a.png", []byte("contenu-de-l-image"), "test"); err != nil {
		t.Fatal(err)
	}
	cible, err := Ouvrir(filepath.Join(t.TempDir(), "ici.cofdm"))
	if err != nil {
		t.Fatal(err)
	}
	defer cible.Fermer()
	if n, _, err := cible.RestaurerDepuis(source, "test"); err != nil || n != 1 {
		t.Fatalf("restauration : %d %v", n, err)
	}
	x, err := cible.Exporter()
	if err != nil || len(x.Medias) != 1 || string(x.Medias[0].Contenu) != "contenu-de-l-image" {
		t.Fatalf("images : %+v %v", x.Medias, err)
	}
}

// Un fichier reçu avec une image démesurée est refusé avant d'être chargé, et rien ne change ici.
func TestRestaurerDepuisRefuseLeDemesure(t *testing.T) {
	chemin := filepath.Join(t.TempDir(), "venu.cofdm")
	source, err := Ouvrir(chemin)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Fermer()
	e, err := source.Creer("lieu", "Fort Boueux", nil, nil, "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.AjouterMedia(e.ID, strings.Repeat("a", 20)+".png", "a.png", []byte("x"), "test"); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open(pilote, chemin)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`UPDATE medias SET contenu = zeroblob(?)`, imageMaxImport+1); err != nil {
		t.Fatal(err)
	}
	cible, err := Ouvrir(filepath.Join(t.TempDir(), "ici.cofdm"))
	if err != nil {
		t.Fatal(err)
	}
	defer cible.Fermer()
	if _, err := cible.Creer("lieu", "Déjà là", nil, nil, "test"); err != nil {
		t.Fatal(err)
	}
	_, _, err = cible.RestaurerDepuis(source, "test")
	var ev ErrValidation
	if !asValidation(err, &ev) {
		t.Fatalf("refus attendu : %v", err)
	}
	if _, fiches, _ := cible.Compter(); fiches != 1 {
		t.Fatalf("rien ne doit changer : %d fiches", fiches)
	}
	// la même chose, avec tout le texte
	if _, err := db.Exec(`UPDATE medias SET contenu = x'00'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE entites SET data = zeroblob(?)`, texteMaxImport+1); err != nil {
		t.Fatal(err)
	}
	if _, _, err = cible.RestaurerDepuis(source, "test"); !asValidation(err, &ev) {
		t.Fatalf("texte démesuré : %v", err)
	}
}

func asValidation(err error, ev *ErrValidation) bool {
	e, ok := err.(ErrValidation)
	if ok {
		*ev = e
	}
	return ok
}

// Un export se lit tel qu'il a été écrit, images comprises ; ce qui se déplierait en mémoire est refusé
// avant de l'être.
func TestLireExport(t *testing.T) {
	st, err := Ouvrir(filepath.Join(t.TempDir(), "venu.cofdm"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Fermer()
	e, err := st.Creer("lieu", "Fort Boueux", nil, nil, "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AjouterMedia(e.ID, strings.Repeat("a", 20)+".png", "a.png", []byte("contenu"), "test"); err != nil {
		t.Fatal(err)
	}
	x, err := st.Exporter()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(x)
	lu, err := LireExport(bytes.NewReader(b))
	if err != nil || lu.Format != formatExport || len(lu.Entites) != 1 || lu.Entites[0].Nom != "Fort Boueux" ||
		len(lu.Medias) != 1 || string(lu.Medias[0].Contenu) != "contenu" {
		t.Fatalf("relecture : %+v %v", lu, err)
	}
	if err := st.Restaurer(lu, "test"); err != nil {
		t.Fatalf("restauration : %v", err)
	}
	var ev ErrValidation
	for quoi, corps := range map[string]string{
		"fiches":   `{"format":"cof-dm","entites":[` + strings.Repeat(`{},`, texteMaxImport/3) + `{}]}`,
		"inconnue": `{"autre":"` + strings.Repeat("x", texteMaxImport) + `"}`,
	} {
		if _, err := LireExport(strings.NewReader(corps)); !asValidation(err, &ev) {
			t.Fatalf("%s : refus attendu, %v", quoi, err)
		}
	}
	for _, corps := range []string{"pas du json", "[]", `{"medias":3}`, `{"format":`} {
		if _, err := LireExport(strings.NewReader(corps)); err == nil || asValidation(err, &ev) {
			t.Fatalf("%q : erreur de lecture attendue, %v", corps, err)
		}
	}
}
