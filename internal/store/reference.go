package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Tchyze/cof_dm/internal/reference"
)

// Profils, peuples et voies : un document par élément (collections reference.CollProfils,
// CollPeuples, CollVoies), pour que la synchronisation les fusionne un par un. Avant le modèle 2,
// ils tenaient en un seul réglage (MetaReference), repris à l'ouverture (reprendreReference).

var collsReference = []string{reference.CollProfils, reference.CollPeuples, reference.CollVoies}

// Reference rassemble les profils, peuples et voies de l'univers, triés par nom, avec leur
// identifiant.
func (s *Store) Reference() (*reference.Reference, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return lireReference(s.db)
}

func lireReference(q execer) (*reference.Reference, error) {
	r := reference.Vide()
	rows, err := q.Query(`SELECT id, coll, data FROM documents WHERE coll IN (?,?,?) ORDER BY id`,
		reference.CollProfils, reference.CollPeuples, reference.CollVoies)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var coll, data string
		if err := rows.Scan(&id, &coll, &data); err != nil {
			return nil, err
		}
		switch coll {
		case reference.CollProfils:
			var p reference.Profil
			if json.Unmarshal([]byte(data), &p) == nil {
				p.ID = id
				r.Profils = append(r.Profils, p)
			}
		case reference.CollPeuples:
			var p reference.Peuple
			if json.Unmarshal([]byte(data), &p) == nil {
				p.ID = id
				r.Peuples = append(r.Peuples, p)
			}
		case reference.CollVoies:
			var v reference.Voie
			if json.Unmarshal([]byte(data), &v) == nil {
				v.ID = id
				r.Voies = append(r.Voies, v)
			}
		}
	}
	r.Trier()
	return r, rows.Err()
}

// FusionnerReference ajoute ou remplace (même référence, ou même nom sans référence) les éléments
// de n, en une fois ; auteur note le bilan au journal.
func (s *Store) FusionnerReference(n *reference.Reference, auteur string) (reference.Bilan, error) {
	if err := n.Normaliser(); err != nil {
		return reference.Bilan{}, invalide("Profils et voies : %v", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return reference.Bilan{}, err
	}
	defer func() { _ = tx.Rollback() }()
	b, err := fusionnerReference(tx, n, func(coll, _ string) (int64, error) { return prochainID(tx, "documents") })
	if err != nil {
		return b, err
	}
	if auteur != "" {
		noter(tx, "règles", 0, "", fmt.Sprintf("profils, peuples et voies importés : %d nouveaux, %d mis à jour", b.Nouveaux, b.MisAJour), auteur)
	}
	return b, tx.Commit()
}

// ViderReference retire tous les profils, peuples et voies (les feuilles ne changent pas).
func (s *Store) ViderReference(auteur string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.db.Exec(`DELETE FROM documents WHERE coll IN (?,?,?)`, reference.CollProfils, reference.CollPeuples, reference.CollVoies); err != nil {
		return err
	}
	noter(s.db, "règles", 0, "", "profils, peuples et voies retirés", auteur)
	return nil
}

// fusionnerReference range les éléments de n (déjà normalisés) : un élément reconnu (même
// identité) est remplacé, les autres sont créés avec l'identifiant que donne nouvelID.
func fusionnerReference(tx execer, n *reference.Reference, nouvelID func(coll, ident string) (int64, error)) (reference.Bilan, error) {
	b := reference.Bilan{Profils: len(n.Profils), Peuples: len(n.Peuples), Voies: len(n.Voies)}
	existants := map[string]int64{} // coll + identité → document
	rows, err := tx.Query(`SELECT id, coll, data FROM documents WHERE coll IN (?,?,?)`, reference.CollProfils, reference.CollPeuples, reference.CollVoies)
	if err != nil {
		return b, err
	}
	for rows.Next() {
		var id int64
		var coll, data string
		if err := rows.Scan(&id, &coll, &data); err != nil {
			rows.Close()
			return b, err
		}
		var x struct{ Ref, Nom string }
		if json.Unmarshal([]byte(data), &x) == nil {
			existants[coll+"\x00"+reference.Ident(x.Ref, x.Nom)] = id
		}
	}
	rows.Close()
	ranger := func(coll, ref, nom string, el any) error {
		data, err := json.Marshal(el)
		if err != nil {
			return err
		}
		if data, err = reference.NormaliserDoc(coll, data); err != nil {
			return invalide("%s : %v", nom, err)
		}
		ident := reference.Ident(ref, nom)
		now := maintenant()
		if id, ok := existants[coll+"\x00"+ident]; ok {
			b.MisAJour++
			_, err = tx.Exec(`UPDATE documents SET data=?, modifie=? WHERE id=?`, string(data), now, id)
			return err
		}
		id, err := nouvelID(coll, ident)
		if err != nil {
			return err
		}
		b.Nouveaux++
		existants[coll+"\x00"+ident] = id
		_, err = tx.Exec(`INSERT INTO documents (id, coll, data, cree, modifie) VALUES (?,?,?,?,?)`, id, coll, string(data), now, now)
		return err
	}
	for _, p := range n.Profils {
		if err := ranger(reference.CollProfils, p.Ref, p.Nom, p); err != nil {
			return b, err
		}
	}
	for _, p := range n.Peuples {
		if err := ranger(reference.CollPeuples, p.Ref, p.Nom, p); err != nil {
			return b, err
		}
	}
	for _, v := range n.Voies {
		if err := ranger(reference.CollVoies, v.Ref, v.Nom, v); err != nil {
			return b, err
		}
	}
	return b, nil
}

// repliqueReprise : la plage d'identifiants réservée à la reprise de l'ancien réglage, qu'aucune
// copie ne reçoit. Chaque copie d'un univers reprend ce réglage de son côté (à l'ouverture, ou en le
// recevant d'un serveur plus ancien) : l'identifiant d'un élément y est tiré de son identité, pour
// que toutes les copies rangent le même élément sous le même identifiant, sans doublon à la
// synchronisation.
const repliqueReprise = RepliqueMax

// idReprise : l'identifiant d'un élément repris, dans la plage réservée ; s'il est déjà pris par
// un autre document, le suivant.
func idReprise(q execer, coll, ident string) (int64, error) {
	h := sha256.Sum256([]byte(coll + "\x00" + ident))
	base := repliqueReprise * taillePlage
	n := int64(binary.BigEndian.Uint64(h[:8]) & uint64(taillePlage-1))
	for range 1000 {
		if n == 0 {
			n = 1
		}
		var x int
		err := q.QueryRow(`SELECT 1 FROM documents WHERE id=?`, base+n).Scan(&x)
		if errors.Is(err, sql.ErrNoRows) {
			return base + n, nil
		}
		if err != nil {
			return 0, err
		}
		n = (n + 1) & (taillePlage - 1)
	}
	return 0, invalide("Plus d'identifiant libre pour reprendre les profils et voies.")
}

// reprendreReference range un par un les profils, peuples et voies de l'ancien réglage, puis le
// retire. Un réglage illisible est laissé en place.
func reprendreReference(tx execer, auteur string) error {
	var v string
	err := tx.QueryRow(`SELECT valeur FROM meta WHERE cle=?`, MetaReference).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	ref, err := reference.Lire([]byte(v))
	if err != nil {
		return nil
	}
	if err := reprendre(tx, ref, auteur); err != nil {
		return err
	}
	_, err = tx.Exec(`DELETE FROM meta WHERE cle=?`, MetaReference)
	return err
}

// reprendre range ref dans les collections, avec les identifiants de la reprise.
func reprendre(tx execer, ref *reference.Reference, auteur string) error {
	b, err := fusionnerReference(tx, ref, func(coll, ident string) (int64, error) { return idReprise(tx, coll, ident) })
	if err != nil {
		return err
	}
	if b.Nouveaux+b.MisAJour > 0 {
		noter(tx, "mise à jour", 0, "", fmt.Sprintf("profils, peuples et voies rangés un par un : %d profils, %d peuples, %d voies", b.Profils, b.Peuples, b.Voies), auteur)
	}
	return nil
}

// reprendreALOuverture : la reprise, dans sa transaction, à l'ouverture de l'univers.
func (s *Store) reprendreALOuverture() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := reprendreReference(tx, "cof-dm"); err != nil {
		return err
	}
	return tx.Commit()
}
