package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/Tchyze/cof_dm/internal/modele"
)

// empreinteModele : l'empreinte du modèle synchronisé à VersionModele. Quand TestVersionModele
// échoue, le modèle a changé : augmenter VersionModele (versions.go), puis recopier ici la nouvelle
// empreinte.
const empreinteModele = "053478957c4ceed3"

// empreinte : ce que la synchronisation transporte et que l'autre côté doit connaître pour
// l'appliquer. La forme d'une feuille et celle du fichier de règles n'y sont pas : un changement de
// ces formes demande aussi d'augmenter VersionModele, sans que ce test le voie.
func empreinte() string {
	type champ struct {
		Cle, Genre      string
		Options, Cibles []string
	}
	type typ struct {
		Cle    string
		Champs []champ
	}
	var types []typ
	for _, t := range modele.Types {
		x := typ{Cle: t.Cle}
		for _, c := range t.Champs {
			x.Champs = append(x.Champs, champ{c.Cle, c.Genre, c.Options, c.Cibles})
		}
		types = append(types, x)
	}
	var relations [][]any
	for _, r := range modele.Relations {
		relations = append(relations, []any{r.Cle, r.De, r.Vers, r.Reservee})
	}
	var fonctions []string
	for _, f := range modele.Fonctionnalites {
		fonctions = append(fonctions, f.Cle)
	}
	b, _ := json.Marshal(map[string]any{
		"donnees": versionDonnees, "types": types, "relations": relations, "fonctionnalites": fonctions,
		"collections": CollectionsDocs, "reglages": reglagesSync,
		"objets": []string{ObjReglage, ObjFiche, ObjDocument, ObjRelation, ObjImage, ObjFeuille, ObjJournal, ChampExiste},
	})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:8])
}

func TestVersionModele(t *testing.T) {
	if e := empreinte(); e != empreinteModele {
		t.Fatalf("le modèle synchronisé a changé (empreinte %s) : augmentez VersionModele (aujourd'hui %d) "+
			"dans versions.go, puis recopiez l'empreinte dans empreinteModele", e, VersionModele)
	}
}

// Un univers ouvert par une application plus récente ne s'ouvre plus dans une plus ancienne, et
// l'ouverture refusée n'y touche pas.
func TestUniversTropRecent(t *testing.T) {
	chemin := filepath.Join(t.TempDir(), "u.cofdm")
	s, err := Ouvrir(chemin)
	if err != nil {
		t.Fatal(err)
	}
	if s.Modele() != VersionModele {
		t.Fatalf("modèle noté : %d", s.Modele())
	}
	s.Fermer()
	for _, cas := range []struct{ cle, valeur string }{
		{MetaModele, strconv.Itoa(VersionModele + 1)},
		{"version_donnees", strconv.FormatInt(donneesCourantes()+1, 10)},
	} {
		db, err := sql.Open("sqlite3", chemin)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT OR REPLACE INTO meta (cle, valeur) VALUES (?, ?)`, cas.cle, cas.valeur); err != nil {
			t.Fatal(err)
		}
		db.Close()
		_, err = Ouvrir(chemin)
		var ev ErrValidation
		if !errors.As(err, &ev) || ev.Msg != MsgTropRecent {
			t.Fatalf("%s plus récent : %v", cas.cle, err)
		}
		db, _ = sql.Open("sqlite3", chemin)
		var v string
		_ = db.QueryRow(`SELECT valeur FROM meta WHERE cle=?`, cas.cle).Scan(&v)
		if v != cas.valeur {
			t.Fatalf("%s changé par l'ouverture refusée : %q", cas.cle, v)
		}
		// remis au niveau de l'application : l'univers s'ouvre de nouveau
		courant := map[string]string{MetaModele: strconv.Itoa(VersionModele), "version_donnees": versionDonnees}[cas.cle]
		_, _ = db.Exec(`UPDATE meta SET valeur=? WHERE cle=?`, courant, cas.cle)
		db.Close()
		s, err := Ouvrir(chemin)
		if err != nil {
			t.Fatal(err)
		}
		s.Fermer()
	}
}

func TestExportTropRecent(t *testing.T) {
	s := ouvrir(t)
	x, err := s.Exporter()
	if err != nil || x.Modele != VersionModele {
		t.Fatalf("export : %d %v", x.Modele, err)
	}
	x.Modele++
	var ev ErrValidation
	if err := s.Restaurer(x, "mj"); !errors.As(err, &ev) {
		t.Fatalf("un export d'un modèle plus récent doit être refusé : %v", err)
	}
	x.Modele = 0 // export d'avant les numéros : accepté
	if err := s.Restaurer(x, "mj"); err != nil {
		t.Fatal(err)
	}
}
