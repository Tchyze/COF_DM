package store

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// L'aperçu d'un univers fermé dit ce qu'il est sans jamais y écrire : ni migration, ni journal.
func TestApercuNeTouchePasAuFichier(t *testing.T) {
	racine := t.TempDir()
	simple := filepath.Join(racine, "simple")
	st, err := Ouvrir(filepath.Join(simple, "Les Marches.cofdm"))
	if err != nil {
		t.Fatal(err)
	}
	id := st.ID()
	if err := st.Fermer(); err != nil {
		t.Fatal(err)
	}
	// des caractères qu'une adresse de fichier SQLite pourrait mal lire
	dossier := filepath.Join(racine, "Clé #1 ?100% & co")
	if err := os.Rename(simple, dossier); err != nil {
		t.Fatal(err)
	}
	chemin := filepath.Join(dossier, "Les Marches.cofdm")
	ancienne := time.Now().Add(-time.Hour).Truncate(time.Second)
	if err := os.Chtimes(chemin, ancienne, ancienne); err != nil {
		t.Fatal(err)
	}
	avant, _ := os.ReadFile(chemin)

	a, err := Apercu(chemin)
	if err != nil {
		t.Fatal(err)
	}
	if a.ID != id || a.Copie {
		t.Fatalf("aperçu : %+v, identifiant attendu %s", a, id)
	}
	apres, _ := os.ReadFile(chemin)
	info, _ := os.Stat(chemin)
	if string(avant) != string(apres) || !info.ModTime().Equal(ancienne) {
		t.Fatal("l'aperçu a touché au fichier")
	}
	entrees, _ := os.ReadDir(filepath.Dir(chemin))
	if len(entrees) != 1 {
		t.Fatalf("l'aperçu laisse des fichiers à côté : %v", entrees)
	}
}

func TestApercuCopie(t *testing.T) {
	chemin := filepath.Join(t.TempDir(), "Copie.cofdm")
	st, err := Ouvrir(chemin)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.DefinirReplique(3); err != nil {
		t.Fatal(err)
	}
	st.Fermer()
	if a, err := Apercu(chemin); err != nil || !a.Copie {
		t.Fatalf("copie : %+v, %v", a, err)
	}
}

// Rien n'est créé pour un fichier absent, et ce qui n'est pas un univers est reconnu comme tel.
func TestApercuRefuse(t *testing.T) {
	dir := t.TempDir()
	absent := filepath.Join(dir, "absent.cofdm")
	if _, err := Apercu(absent); err == nil {
		t.Fatal("fichier absent accepté")
	}
	if _, err := os.Stat(absent); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("l'aperçu a créé le fichier absent")
	}

	texte := filepath.Join(dir, "texte.cofdm")
	if err := os.WriteFile(texte, []byte("ceci n'est pas une base, et ce texte est assez long pour le montrer"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Apercu(texte); !errors.Is(err, ErrPasUnivers) {
		t.Fatalf("fichier de texte : %v", err)
	}

	vide := filepath.Join(dir, "vide.cofdm")
	if err := os.WriteFile(vide, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Apercu(vide); !errors.Is(err, ErrPasUnivers) {
		t.Fatalf("fichier vide : %v", err)
	}

	autre := filepath.Join(dir, "autre.cofdm")
	db, err := sql.Open("sqlite3", autre)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE photos (id INTEGER)`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if _, err := Apercu(autre); !errors.Is(err, ErrPasUnivers) {
		t.Fatalf("base d'une autre application : %v", err)
	}
}
