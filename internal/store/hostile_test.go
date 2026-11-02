package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fabrique part d'un univers valide et applique des ordres SQL, comme un fichier préparé par un tiers.
func fabrique(t testing.TB, fiches int, ordres ...string) string {
	t.Helper()
	chemin := filepath.Join(t.TempDir(), "venu.cofdm")
	st, err := Ouvrir(chemin)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < fiches; i++ {
		if _, err := st.Creer("lieu", fmt.Sprintf("Lieu %d", i), nil, nil, "t"); err != nil {
			t.Fatal(err)
		}
	}
	st.Fermer()
	db, err := sql.Open("sqlite3", chemin)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, o := range ordres {
		if _, err := db.Exec(o); err != nil {
			t.Fatalf("%s : %v", o, err)
		}
	}
	return chemin
}

// Une colonne calculée n'existe dans aucun univers : quelques octets y coûteraient des centaines de Mo.
func TestColonneGenereeRefusee(t *testing.T) {
	for _, o := range [][]string{
		{`ALTER TABLE entites RENAME COLUMN data TO data0`, `ALTER TABLE entites ADD COLUMN data TEXT GENERATED ALWAYS AS (printf('%100000000d',1)) VIRTUAL`},
		{`ALTER TABLE entites ADD COLUMN piege TEXT GENERATED ALWAYS AS (zeroblob(100)) VIRTUAL`},
		{`CREATE TABLE autre (a TEXT, b TEXT GENERATED ALWAYS AS (a) STORED)`},
	} {
		st, err := Ouvrir(fabrique(t, 1, o...))
		if err == nil {
			st.Fermer()
			t.Fatalf("%v : ouvert", o)
		}
		if !strings.Contains(err.Error(), "colonne calculée") {
			t.Fatalf("%v : %v", o, err)
		}
	}
}

// Une valeur ne dépasse pas ce qu'un univers contient : un index ou un CHECK piégé échoue au lieu
// d'occuper presque toute la mémoire du serveur.
func TestValeurPlafonnee(t *testing.T) {
	st, err := Ouvrir(filepath.Join(t.TempDir(), "t.cofdm"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Fermer()
	var n int
	if err := st.db.QueryRow(`SELECT length(printf('%1000000d', 1))`).Scan(&n); err != nil || n != 1000000 {
		t.Fatalf("une valeur raisonnable : %d %v", n, err)
	}
	if err := st.db.QueryRow(`SELECT length(printf('%900000000d', 1))`).Scan(&n); err == nil {
		t.Fatalf("une valeur démesurée passe : %d", n)
	}
	// plus lourde qu'une image reçue, elle s'écrit et se relit
	e, err := st.Creer("lieu", "Fort Boueux", nil, nil, "t")
	if err != nil {
		t.Fatal(err)
	}
	gros := make([]byte, imageMaxImport)
	if _, err := st.AjouterMedia(e.ID, strings.Repeat("a", 20)+".png", "a.png", gros, "t"); err != nil {
		t.Fatalf("une image au plafond : %v", err)
	}
	if err := st.Sauvegarder(filepath.Join(t.TempDir(), "copie.cofdm")); err != nil {
		t.Fatalf("sa sauvegarde : %v", err)
	}
}

// Un fichier reçu, quel qu'il soit : refusé proprement ou repris, jamais de plantage. Les graines se
// rejouent à chaque go test ; `go test -fuzz=FuzzFichierRecu ./internal/store` en cherche d'autres.
func FuzzFichierRecu(f *testing.F) {
	seul := fabrique(f, 2)
	b, err := os.ReadFile(seul)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(b)
	f.Add([]byte("SQLite format 3\x00"))
	f.Add(b[:len(b)/2])
	for _, o := range []string{
		`CREATE TRIGGER piege AFTER INSERT ON journal BEGIN DELETE FROM entites; END`,
		`CREATE VIEW piege AS SELECT 1`,
		`CREATE TABLE piege (x TEXT CHECK (length(x) > 0))`,
		`CREATE INDEX piege ON entites (length(nom))`,
		`PRAGMA writable_schema=ON; UPDATE sqlite_master SET sql = sql || ' ' WHERE name='meta'; PRAGMA writable_schema=OFF`,
	} {
		v, err := os.ReadFile(fabrique(f, 1, o))
		if err != nil {
			f.Fatal(err)
		}
		f.Add(v)
	}
	f.Fuzz(func(t *testing.T, contenu []byte) {
		dir := t.TempDir()
		chemin := filepath.Join(dir, "venu.cofdm")
		if err := os.WriteFile(chemin, contenu, 0o600); err != nil {
			t.Fatal(err)
		}
		source, err := Ouvrir(chemin)
		if err != nil {
			return
		}
		defer source.Fermer()
		cible, err := Ouvrir(filepath.Join(dir, "ici.cofdm"))
		if err != nil {
			t.Fatal(err)
		}
		defer cible.Fermer()
		_, _, _ = cible.RestaurerDepuis(source, "t")
	})
}
