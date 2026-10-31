package store

import (
	"database/sql"
	"errors"
)

// Documents : ce que l'univers range en dehors des fiches du monde, par collection — le bestiaire
// (« monstres »), les combats (« combats », en cours ou terminés), les profils, peuples et voies
// (« profils », « peuples », « voies », un document par élément), le pot commun de chaque campagne
// (« pots », internal/fiches/pot.go). Un document est un objet JSON
// validé par son paquet (internal/fiches, internal/combat, internal/reference).

// Collections de documents connues.
var CollectionsDocs = []string{"monstres", "combats", "profils", "peuples", "voies", "pots"}

func collectionConnue(coll string) bool {
	for _, c := range CollectionsDocs {
		if c == coll {
			return true
		}
	}
	return false
}

type Document struct {
	ID      int64   `json:"id"`
	Coll    string  `json:"coll"`
	Data    []byte  `json:"data"`
	Cree    float64 `json:"cree"`
	Modifie float64 `json:"modifie"`
}

const schemaDocuments = `
CREATE TABLE IF NOT EXISTS documents (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	coll TEXT NOT NULL,
	data TEXT NOT NULL,
	cree REAL NOT NULL,
	modifie REAL NOT NULL
);
CREATE INDEX IF NOT EXISTS documents_coll ON documents (coll, id);
`

// ListerDocs renvoie les documents d'une collection, par identifiant.
func (s *Store) ListerDocs(coll string) ([]Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT id, coll, data, cree, modifie FROM documents WHERE coll=? ORDER BY id`, coll)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Document{}
	for rows.Next() {
		var d Document
		var data string
		if err := rows.Scan(&d.ID, &d.Coll, &data, &d.Cree, &d.Modifie); err != nil {
			return nil, err
		}
		d.Data = []byte(data)
		out = append(out, d)
	}
	return out, rows.Err()
}

func lireDoc(q execer, coll string, id int64) (Document, error) {
	d := Document{ID: id, Coll: coll}
	var data string
	err := q.QueryRow(`SELECT data, cree, modifie FROM documents WHERE id=? AND coll=?`, id, coll).Scan(&data, &d.Cree, &d.Modifie)
	if errors.Is(err, sql.ErrNoRows) {
		return d, ErrIntrouvable
	}
	d.Data = []byte(data)
	return d, err
}

func (s *Store) LireDoc(coll string, id int64) (Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return lireDoc(s.db, coll, id)
}

// CreerDoc ajoute un document ; nom et auteur vont au journal (auteur vide : rien n'est noté, pour
// les imports en nombre, qui notent leur bilan).
func (s *Store) CreerDoc(coll string, data []byte, nom, auteur string) (Document, error) {
	if !collectionConnue(coll) {
		return Document{}, invalide("Collection inconnue : %s.", coll)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	t := maintenant()
	id, err := prochainID(s.db, "documents")
	if err != nil {
		return Document{}, err
	}
	if _, err := s.db.Exec(`INSERT INTO documents (id, coll, data, cree, modifie) VALUES (?,?,?,?,?)`, id, coll, string(data), t, t); err != nil {
		return Document{}, err
	}
	if auteur != "" {
		noter(s.db, coll+" : création", 0, nom, "", auteur)
	}
	return Document{ID: id, Coll: coll, Data: data, Cree: t, Modifie: t}, nil
}

func (s *Store) MajDoc(coll string, id int64, data []byte, nom, auteur string) (Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.Exec(`UPDATE documents SET data=?, modifie=? WHERE id=? AND coll=?`, string(data), maintenant(), id, coll)
	if err != nil {
		return Document{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Document{}, ErrIntrouvable
	}
	if auteur != "" {
		noter(s.db, coll+" : modification", 0, nom, "", auteur)
	}
	return lireDoc(s.db, coll, id)
}

func (s *Store) SupprimerDoc(coll string, id int64, nom, auteur string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.Exec(`DELETE FROM documents WHERE id=? AND coll=?`, id, coll)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrIntrouvable
	}
	noter(s.db, coll+" : suppression", 0, nom, "", auteur)
	return nil
}
