package store

import (
	"strconv"
)

// Identifiants stables entre un univers et ses copies synchronisées.
//
// Chaque copie d'un univers (le serveur, chaque application de bureau qui s'y relie) a un numéro, et
// tire les identifiants de ce qu'elle crée (fiches, relations, images, documents, lignes du journal)
// dans sa propre plage : [numéro × taillePlage + 1, (numéro + 1) × taillePlage[. Une fiche créée hors
// ligne garde ainsi le même identifiant partout, et rien de ce qui la désigne (champs, relations,
// feuille, combats) n'est à traduire. La plage 0 est celle du serveur et de tout univers qui n'a jamais
// été copié : les identifiants existants y sont déjà.
//
// taillePlage laisse 2^13 copies sous 2^53, la limite des entiers exacts en JavaScript.

const taillePlage = int64(1) << 40

// RepliqueMax : le plus grand numéro de copie possible.
const RepliqueMax = int64(1)<<13 - 1

// MetaReplique : clé du réglage qui garde le numéro de copie de ce fichier (absent : 0).
const MetaReplique = "replique"

// tablesAIdentifiant : les tables dont les lignes reçoivent un identifiant de la plage.
var tablesAIdentifiant = map[string]bool{"entites": true, "relations": true, "medias": true, "documents": true, "journal": true}

// replique lit le numéro de copie de ce fichier.
func replique(q execer) int64 {
	var v string
	if q.QueryRow(`SELECT valeur FROM meta WHERE cle=?`, MetaReplique).Scan(&v) != nil {
		return 0
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 0 || n > RepliqueMax {
		return 0
	}
	return n
}

// prochainID : l'identifiant suivant de la plage de cette copie, dans cette table.
func prochainID(q execer, table string) (int64, error) {
	if !tablesAIdentifiant[table] {
		panic("prochainID : table inconnue " + table)
	}
	base := replique(q) * taillePlage
	var max int64
	if err := q.QueryRow(`SELECT COALESCE(MAX(id), ?) FROM `+table+` WHERE id > ? AND id < ?`, base, base, base+taillePlage).Scan(&max); err != nil {
		return 0, err
	}
	if max+1 >= base+taillePlage {
		return 0, invalide("Plus aucun identifiant libre dans la plage de cette copie (table %s).", table)
	}
	return max + 1, nil
}

// Replique : le numéro de copie de cet univers (0 : le serveur, ou un univers jamais copié).
func (s *Store) Replique() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return replique(s.db)
}

// DefinirReplique donne à ce fichier son numéro de copie : ce qu'il crée ensuite prend ses
// identifiants dans la plage de ce numéro.
func (s *Store) DefinirReplique(n int64) error {
	if n < 0 || n > RepliqueMax {
		return invalide("Numéro de copie hors limites (0 à %d).", RepliqueMax)
	}
	return s.SetMeta(MetaReplique, strconv.FormatInt(n, 10))
}
