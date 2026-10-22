package store

import (
	"database/sql"
	"encoding/json"
	"errors"
)

// Feuilles de personnage : une par fiche de PJ, au plus. La feuille suit sa fiche (elle disparaît
// avec elle) ; son contenu est validé par internal/fiches.

const schemaFeuilles = `
CREATE TABLE IF NOT EXISTS feuilles (
	entite INTEGER PRIMARY KEY REFERENCES entites(id) ON DELETE CASCADE,
	data TEXT NOT NULL,
	modifie REAL NOT NULL
);
`

// LireFeuille renvoie la feuille d'une fiche et sa date de modification (ErrIntrouvable sans feuille).
func (s *Store) LireFeuille(entite int64) ([]byte, float64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var data string
	var modifie float64
	err := s.db.QueryRow(`SELECT data, modifie FROM feuilles WHERE entite=?`, entite).Scan(&data, &modifie)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, 0, ErrIntrouvable
	}
	return []byte(data), modifie, err
}

// EcrireFeuille enregistre la feuille d'une fiche. attendu : date de modification connue du client
// (0 : pas de contrôle) ; si la feuille a changé depuis, ErrConflit. Auteur vide : rien n'est noté
// au journal (PV reportés pendant un combat, qui note son début et sa fin).
func (s *Store) EcrireFeuille(entite int64, data []byte, attendu float64, auteur string) (float64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, err := lire(s.db, entite)
	if err != nil {
		return 0, err
	}
	var actuel float64
	_ = s.db.QueryRow(`SELECT modifie FROM feuilles WHERE entite=?`, entite).Scan(&actuel)
	if attendu != 0 && actuel != 0 && actuel != attendu {
		return 0, ErrConflit
	}
	t := maintenant()
	if _, err := s.db.Exec(`INSERT INTO feuilles (entite, data, modifie) VALUES (?,?,?)
		ON CONFLICT(entite) DO UPDATE SET data=excluded.data, modifie=excluded.modifie`, entite, string(data), t); err != nil {
		return 0, err
	}
	if auteur != "" {
		noter(s.db, "feuille de personnage", entite, e.Nom, "", auteur)
	}
	return t, nil
}

// ChangerNiveau change en une fois le niveau d'un PJ (sur sa fiche) et sa feuille : une montée de
// niveau ou son annulation. attendu : date de modification de la feuille connue du client (0 : pas
// de contrôle ; ErrConflit si elle a changé). action et detail vont au journal.
func (s *Store) ChangerNiveau(entite int64, niveau int, data []byte, attendu float64, action, detail, auteur string) (float64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	e, err := lire(tx, entite)
	if err != nil {
		return 0, err
	}
	if e.Type != "pj" {
		return 0, invalide("Seuls les PJ ont un niveau.")
	}
	var actuel float64
	_ = tx.QueryRow(`SELECT modifie FROM feuilles WHERE entite=?`, entite).Scan(&actuel)
	if attendu != 0 && actuel != 0 && actuel != attendu {
		return 0, ErrConflit
	}
	if e.Data == nil {
		e.Data = map[string]any{}
	}
	e.Data["niveau"] = float64(niveau)
	dj, _ := json.Marshal(e.Data)
	t := maintenant()
	if _, err := tx.Exec(`UPDATE entites SET data=?, modifie=? WHERE id=?`, string(dj), t, entite); err != nil {
		return 0, err
	}
	if err := indexer(tx, e); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(`INSERT INTO feuilles (entite, data, modifie) VALUES (?,?,?)
		ON CONFLICT(entite) DO UPDATE SET data=excluded.data, modifie=excluded.modifie`, entite, string(data), t); err != nil {
		return 0, err
	}
	noter(tx, action, entite, e.Nom, detail, auteur)
	return t, tx.Commit()
}

// FeuilleExport : une feuille dans un export.
type FeuilleExport struct {
	Entite  int64           `json:"entite"`
	Data    json.RawMessage `json:"data"`
	Modifie float64         `json:"modifie"`
}

// feuilles lit toutes les feuilles de l'univers.
func feuilles(q execer) ([]FeuilleExport, error) {
	rows, err := q.Query(`SELECT entite, data, modifie FROM feuilles ORDER BY entite`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []FeuilleExport{}
	for rows.Next() {
		var f FeuilleExport
		var data string
		if err := rows.Scan(&f.Entite, &data, &f.Modifie); err != nil {
			return nil, err
		}
		f.Data = json.RawMessage(data)
		out = append(out, f)
	}
	return out, rows.Err()
}
