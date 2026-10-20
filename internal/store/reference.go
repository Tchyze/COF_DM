package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

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

// ---------------------------------------------------------------- éditeur, élément par élément

// majusculeInitiale : un message d'erreur qui commence par une majuscule et finit par un point.
func majusculeInitiale(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}
	return strings.ToUpper(string(r[0])) + string(r[1:]) + "."
}

// collReference : la collection est-elle celle de profils, de peuples ou de voies ?
func collReference(coll string) bool {
	return coll == reference.CollProfils || coll == reference.CollPeuples || coll == reference.CollVoies
}

// identite : la référence et le nom d'un élément rangé.
type identite struct{ Ref, Nom string }

func identiteDe(data []byte) identite {
	var x identite
	_ = json.Unmarshal(data, &x)
	return x
}

// verifierUnique refuse un élément qui a la même identité (référence, ou nom sans référence)
// qu'un autre de sa collection.
func verifierUnique(tx execer, coll string, id int64, x identite) error {
	rows, err := tx.Query(`SELECT id, data FROM documents WHERE coll=? AND id<>?`, coll, id)
	if err != nil {
		return err
	}
	defer rows.Close()
	cible := reference.Ident(x.Ref, x.Nom)
	for rows.Next() {
		var autre int64
		var data string
		if err := rows.Scan(&autre, &data); err != nil {
			return err
		}
		y := identiteDe([]byte(data))
		if reference.Ident(y.Ref, y.Nom) == cible || (x.Ref != "" && y.Ref == x.Ref) {
			return invalide("« %s » existe déjà dans la bibliothèque.", y.Nom)
		}
	}
	return rows.Err()
}

// CreerElement ajoute un profil, un peuple ou une voie à la bibliothèque.
func (s *Store) CreerElement(coll string, data []byte, auteur string) (int64, []byte, error) {
	if !collReference(coll) {
		return 0, nil, ErrIntrouvable
	}
	norm, err := reference.NormaliserDoc(coll, data)
	if err != nil {
		return 0, nil, invalide("%s", majusculeInitiale(err.Error()))
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = tx.Rollback() }()
	x := identiteDe(norm)
	if err := verifierUnique(tx, coll, 0, x); err != nil {
		return 0, nil, err
	}
	id, err := prochainID(tx, "documents")
	if err != nil {
		return 0, nil, err
	}
	t := maintenant()
	if _, err := tx.Exec(`INSERT INTO documents (id, coll, data, cree, modifie) VALUES (?,?,?,?,?)`, id, coll, string(norm), t, t); err != nil {
		return 0, nil, err
	}
	noter(tx, coll+" : création", 0, x.Nom, "", auteur)
	return id, norm, tx.Commit()
}

// ModifierElement remplace un profil, un peuple ou une voie. Une voie renommée l'est aussi sur les
// feuilles qui la désignaient par son nom, et dans les voies des profils et des peuples.
func (s *Store) ModifierElement(coll string, id int64, data []byte, auteur string) ([]byte, error) {
	if !collReference(coll) {
		return nil, ErrIntrouvable
	}
	norm, err := reference.NormaliserDoc(coll, data)
	if err != nil {
		return nil, invalide("%s", majusculeInitiale(err.Error()))
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	ancien, err := lireDoc(tx, coll, id)
	if err != nil {
		return nil, err
	}
	avant, apres := identiteDe(ancien.Data), identiteDe(norm)
	if err := verifierUnique(tx, coll, id, apres); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`UPDATE documents SET data=?, modifie=? WHERE id=?`, string(norm), maintenant(), id); err != nil {
		return nil, err
	}
	detail := ""
	if coll == reference.CollVoies && reference.Cle(avant.Nom) != reference.Cle(apres.Nom) {
		n, err := renommerVoie(tx, avant, apres)
		if err != nil {
			return nil, err
		}
		detail = fmt.Sprintf("renommée « %s » → « %s » (%d feuilles)", avant.Nom, apres.Nom, n)
	}
	noter(tx, coll+" : modification", 0, apres.Nom, detail, auteur)
	return norm, tx.Commit()
}

// SupprimerElement retire un profil, un peuple ou une voie. Les feuilles gardent leur copie des
// noms : la voie y devient une voie libre.
func (s *Store) SupprimerElement(coll string, id int64, auteur string) error {
	if !collReference(coll) {
		return ErrIntrouvable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := lireDoc(s.db, coll, id)
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(`DELETE FROM documents WHERE id=? AND coll=?`, id, coll); err != nil {
		return err
	}
	noter(s.db, coll+" : suppression", 0, identiteDe(d.Data).Nom, "", auteur)
	return nil
}

// designe : la voie d'une feuille (nom, ref) désigne-t-elle la voie x de la bibliothèque ?
func designe(nom, ref string, x identite) bool {
	if ref != "" && x.Ref != "" {
		return ref == x.Ref
	}
	return reference.Cle(nom) == reference.Cle(x.Nom)
}

// renommerVoie suit le renommage d'une voie sur les feuilles et dans les profils et les peuples.
func renommerVoie(tx execer, avant, apres identite) (int, error) {
	rows, err := tx.Query(`SELECT entite, data FROM feuilles`)
	if err != nil {
		return 0, err
	}
	maj := map[int64]string{}
	for rows.Next() {
		var id int64
		var data string
		if err := rows.Scan(&id, &data); err != nil {
			rows.Close()
			return 0, err
		}
		var f map[string]any
		if json.Unmarshal([]byte(data), &f) != nil {
			continue
		}
		voies, _ := f["voies"].([]any)
		change := false
		for _, x := range voies {
			v, _ := x.(map[string]any)
			nom, _ := v["nom"].(string)
			ref, _ := v["ref"].(string)
			if v != nil && designe(nom, ref, avant) {
				v["nom"], change = apres.Nom, true
			}
		}
		if change {
			b, _ := json.Marshal(f)
			maj[id] = string(b)
		}
	}
	rows.Close()
	for id, data := range maj {
		if _, err := tx.Exec(`UPDATE feuilles SET data=?, modifie=? WHERE entite=?`, data, maintenant(), id); err != nil {
			return 0, err
		}
	}
	// les voies d'un profil ou d'un peuple sont désignées par leur nom ou leur référence
	rows, err = tx.Query(`SELECT id, data FROM documents WHERE coll IN (?,?)`, reference.CollProfils, reference.CollPeuples)
	if err != nil {
		return 0, err
	}
	docs := map[int64]string{}
	for rows.Next() {
		var id int64
		var data string
		if err := rows.Scan(&id, &data); err != nil {
			rows.Close()
			return 0, err
		}
		var el map[string]any
		if json.Unmarshal([]byte(data), &el) != nil {
			continue
		}
		voies, _ := el["voies"].([]any)
		change := false
		for i, x := range voies {
			if nom, _ := x.(string); avant.Ref == "" || nom != avant.Ref {
				if reference.Cle(nom) == reference.Cle(avant.Nom) {
					voies[i], change = apres.Nom, true
				}
			}
		}
		if change {
			b, _ := json.Marshal(el)
			docs[id] = string(b)
		}
	}
	rows.Close()
	for id, data := range docs {
		if _, err := tx.Exec(`UPDATE documents SET data=?, modifie=? WHERE id=?`, data, maintenant(), id); err != nil {
			return 0, err
		}
	}
	return len(maj), nil
}

// Utilisation : un PJ dont la feuille a une voie de la bibliothèque, à tel rang, avec ses
// « divers » (bonus saisis à la main).
type Utilisation struct {
	ID         int64          `json:"id"`
	Nom        string         `json:"nom"`
	Rang       int            `json:"rang"`
	DefDivers  int            `json:"def_divers"`
	InitDivers int            `json:"init_divers"`
	AttDivers  map[string]int `json:"att_divers"`
}

// UtilisationsVoie : les PJ dont la feuille a la voie id.
func (s *Store) UtilisationsVoie(id int64) ([]Utilisation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := lireDoc(s.db, reference.CollVoies, id)
	if err != nil {
		return nil, err
	}
	x := identiteDe(d.Data)
	rows, err := s.db.Query(`SELECT f.entite, e.nom, f.data FROM feuilles f JOIN entites e ON e.id = f.entite ORDER BY e.nom`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Utilisation{}
	for rows.Next() {
		var u Utilisation
		var data string
		if err := rows.Scan(&u.ID, &u.Nom, &data); err != nil {
			return nil, err
		}
		var f struct {
			Voies []struct {
				Nom, Ref string
				Rang     int
			} `json:"voies"`
			DefDivers  int            `json:"def_divers"`
			InitDivers int            `json:"init_divers"`
			AttDivers  map[string]int `json:"att_divers"`
		}
		if json.Unmarshal([]byte(data), &f) != nil {
			continue
		}
		for _, v := range f.Voies {
			if designe(v.Nom, v.Ref, x) {
				u.Rang, u.DefDivers, u.InitDivers, u.AttDivers = v.Rang, f.DefDivers, f.InitDivers, f.AttDivers
				out = append(out, u)
				break
			}
		}
	}
	return out, rows.Err()
}
