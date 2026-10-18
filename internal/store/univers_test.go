package store

import (
	"bytes"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Un univers fermé tient dans un seul fichier, images comprises, et se rouvre tel quel.
func TestUniversFichierUnique(t *testing.T) {
	dir := t.TempDir()
	chemin := filepath.Join(dir, "Les Marches.cofdm")
	s, err := Ouvrir(chemin)
	if err != nil {
		t.Fatal(err)
	}
	e := creer(t, s, "pnj", "Dame Ysolde", nil)
	image := []byte("\x89PNG contenu de l'image")
	m, err := s.AjouterMedia(e.ID, "0123456789abcdef0123.png", "ysolde.png", image, "test")
	if err != nil || !m.Principal {
		t.Fatalf("image : %v %+v", err, m)
	}
	id := s.ID()
	if err := s.Fermer(); err != nil {
		t.Fatal(err)
	}
	if entrees, _ := os.ReadDir(dir); len(entrees) != 1 || entrees[0].Name() != "Les Marches.cofdm" {
		t.Fatalf("l'univers fermé ne tient pas dans un seul fichier : %v", entrees)
	}

	s, err = Ouvrir(chemin)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Fermer()
	if s.ID() != id || id == "" {
		t.Fatalf("identifiant de l'univers : %q puis %q", id, s.ID())
	}
	if b, _, err := s.ContenuMedia(m.Fichier); err != nil || !bytes.Equal(b, image) {
		t.Fatalf("image relue : %v %q", err, b)
	}
	if l, _ := s.Medias(e.ID); len(l) != 1 || len(l[0].Contenu) != 0 {
		t.Fatalf("la liste des images ne doit pas porter leur contenu : %+v", l)
	}
	if _, err := s.Supprimer(e.ID, "test"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.ContenuMedia(m.Fichier); !errors.Is(err, ErrIntrouvable) {
		t.Fatalf("image d'une fiche supprimée : %v", err)
	}
}

// Un fichier qui n'est pas un univers est refusé et laissé intact.
func TestPasUnUnivers(t *testing.T) {
	dir := t.TempDir()
	texte := filepath.Join(dir, "notes.cofdm")
	contenu := []byte("ceci n'est pas une base de données, mais un texte assez long pour être lu")
	if err := os.WriteFile(texte, contenu, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Ouvrir(texte); !errors.Is(err, ErrPasUnivers) {
		t.Fatalf("fichier texte : %v", err)
	}
	if b, _ := os.ReadFile(texte); !bytes.Equal(b, contenu) {
		t.Fatal("le fichier refusé a été modifié")
	}

	autre := filepath.Join(dir, "autre.cofdm")
	db, err := sql.Open("sqlite3", autre)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE factures (id INTEGER)`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if _, err := Ouvrir(autre); !errors.Is(err, ErrPasUnivers) {
		t.Fatalf("base d'un autre programme : %v", err)
	}
	db, _ = sql.Open("sqlite3", autre)
	defer db.Close()
	var n int
	if db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table'`).Scan(&n); n != 1 {
		t.Fatalf("la base refusée a été modifiée : %d tables", n)
	}
}

// L'export emporte les images ; un export d'avant (sans leur contenu) garde celles de l'univers.
func TestExportAvecImages(t *testing.T) {
	s := ouvrir(t)
	e := creer(t, s, "lieu", "Le Gué", nil)
	image := []byte("\x89PNG carte du gué")
	m, err := s.AjouterMedia(e.ID, "aaaaaaaaaaaaaaaaaaaa.png", "gue.png", image, "test")
	if err != nil {
		t.Fatal(err)
	}
	x, err := s.Exporter()
	if err != nil || len(x.Medias) != 1 || !bytes.Equal(x.Medias[0].Contenu, image) {
		t.Fatalf("export : %v %+v", err, x.Medias)
	}
	s2 := ouvrir(t)
	if err := s2.Restaurer(x, "t"); err != nil {
		t.Fatal(err)
	}
	if b, _, err := s2.ContenuMedia(m.Fichier); err != nil || !bytes.Equal(b, image) {
		t.Fatalf("image restaurée ailleurs : %v", err)
	}
	if f, _ := s2.Lire(e.ID); f.Image != m.Fichier {
		t.Fatalf("image principale après restauration : %q", f.Image)
	}

	x.Medias[0].Contenu = nil // export d'avant les fichiers d'univers
	if err := s.Restaurer(x, "t"); err != nil {
		t.Fatal(err)
	}
	if b, _, err := s.ContenuMedia(m.Fichier); err != nil || !bytes.Equal(b, image) {
		t.Fatalf("image perdue par un ancien export : %v", err)
	}
}

// Les images d'avant les fichiers d'univers étaient des fichiers d'un dossier.
func TestImporterMedias(t *testing.T) {
	s := ouvrir(t)
	e := creer(t, s, "pnj", "Brann", nil)
	if _, err := s.AjouterMedia(e.ID, "bbbbbbbbbbbbbbbbbbbb.png", "brann.png", nil, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AjouterMedia(e.ID, "cccccccccccccccccccc.png", "perdue.png", nil, "test"); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bbbbbbbbbbbbbbbbbbbb.png"), []byte("portrait"), 0o644); err != nil {
		t.Fatal(err)
	}
	n, absentes, err := s.ImporterMedias(dir)
	if err != nil || n != 1 || len(absentes) != 1 || absentes[0] != "cccccccccccccccccccc.png" {
		t.Fatalf("reprise : %d %v %v", n, absentes, err)
	}
	if b, _, err := s.ContenuMedia("bbbbbbbbbbbbbbbbbbbb.png"); err != nil || string(b) != "portrait" {
		t.Fatalf("image reprise : %v %q", err, b)
	}
	if _, _, err := s.ContenuMedia("cccccccccccccccccccc.png"); !errors.Is(err, ErrIntrouvable) {
		t.Fatalf("image sans contenu : %v", err)
	}
}

// Un univers et ses copies contiennent les secrets du MJ : seul leur propriétaire les lit.
func TestFichiersPrives(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("droits Unix")
	}
	dir := t.TempDir()
	st, err := Ouvrir(filepath.Join(dir, "sous", "u.cofdm"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Fermer()
	copie := filepath.Join(dir, "copies", "c.cofdm")
	if err := st.Sauvegarder(copie); err != nil {
		t.Fatal(err)
	}
	for f, attendu := range map[string]os.FileMode{st.Chemin(): 0o600, copie: 0o600, filepath.Dir(copie): 0o700, filepath.Dir(st.Chemin()): 0o700} {
		if fi, err := os.Stat(f); err != nil || fi.Mode().Perm() != attendu {
			t.Fatalf("%s : droits %v, attendu %v (%v)", f, fi.Mode().Perm(), attendu, err)
		}
	}
}

// Un univers reçu d'un tiers est contrôlé avant usage : un objet qu'un univers n'a jamais
// (déclencheur, vue) ou une base abîmée le fait refuser, sans le modifier.
func TestUniversSuspect(t *testing.T) {
	dir := t.TempDir()
	modele := func(nom string) string {
		chemin := filepath.Join(dir, nom)
		s, err := Ouvrir(chemin)
		if err != nil {
			t.Fatal(err)
		}
		creer(t, s, "lieu", "Le Gué", nil)
		s.Fermer()
		return chemin
	}
	for _, c := range []struct{ nom, sql, attendu string }{
		{"declencheur.cofdm", `CREATE TRIGGER piege AFTER INSERT ON journal BEGIN DELETE FROM entites; END`, "trigger « piege »"},
		{"vue.cofdm", `CREATE VIEW piege AS SELECT * FROM entites`, "view « piege »"},
	} {
		chemin := modele(c.nom)
		db, err := sql.Open("sqlite3", chemin)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(c.sql); err != nil {
			t.Fatal(err)
		}
		db.Close()
		avant, _ := os.ReadFile(chemin)
		if _, err := Ouvrir(chemin); err == nil || !strings.Contains(err.Error(), c.attendu) {
			t.Fatalf("%s : %v", c.nom, err)
		}
		if apres, _ := os.ReadFile(chemin); !bytes.Equal(avant, apres) {
			t.Fatalf("%s : le fichier refusé a été modifié", c.nom)
		}
	}

	// une page abîmée au milieu du fichier
	chemin := modele("abime.cofdm")
	b, _ := os.ReadFile(chemin)
	taille := 4096
	for i := 2 * taille; i < 3*taille && i < len(b); i++ {
		b[i] = 0xA5
	}
	if err := os.WriteFile(chemin, b, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Ouvrir(chemin); err == nil {
		t.Fatal("univers abîmé ouvert")
	} else if !strings.Contains(err.Error(), "abîmé") {
		t.Fatalf("univers abîmé : %v", err)
	}
}

// Les connexions de l'univers ne font pas confiance au schéma du fichier.
func TestPiloteDefensif(t *testing.T) {
	s := ouvrir(t)
	var confiance, controle int
	if err := s.db.QueryRow(`PRAGMA trusted_schema`).Scan(&confiance); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow(`PRAGMA cell_size_check`).Scan(&controle); err != nil {
		t.Fatal(err)
	}
	if confiance != 0 || controle != 1 {
		t.Fatalf("trusted_schema=%d cell_size_check=%d", confiance, controle)
	}
}
