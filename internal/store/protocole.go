package store

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

// Protocole de synchronisation entre le serveur et une copie (application de bureau).
//
// La copie retient deux numéros : son curseur (le dernier numéro d'ordre du serveur qu'elle a reçu)
// et le dernier numéro d'ordre local qu'elle a envoyé. Un échange :
//
//  1. la copie relève ses changements et envoie ceux d'après son dernier envoi (Preparer) ;
//  2. le serveur les reçoit dans une seule transaction (Recevoir) : un champ changé aussi sur le
//     serveur depuis le curseur garde la valeur du serveur (conflit) ; une suppression ne passe pas si
//     l'objet a été modifié de l'autre côté, il est alors gardé et signalé ; le reste est appliqué,
//     validé comme une saisie ;
//  3. la copie reçoit tout ce que le serveur a de nouveau depuis son curseur, et range les conflits
//     dans sa liste « Conflits » (Integrer).

// Genres de conflit.
const (
	ConflitChamp       = "champ"        // le même champ a changé des deux côtés : le serveur garde le sien
	ConflitSupprimeIci = "supprime_ici" // supprimé sur la copie, modifié sur le serveur : gardé
	ConflitSupprimeLa  = "supprime_la"  // supprimé sur le serveur, modifié sur la copie : gardé, renvoyé
	ConflitRefus       = "refus"        // refusé par le serveur (validation) : la valeur du serveur reste
)

// Conflit : ce qu'un échange n'a pas pu fusionner seul, pour que le MJ choisisse.
type Conflit struct {
	ID      int64           `json:"id,omitempty"`
	Genre   string          `json:"genre"`
	Objet   string          `json:"objet"`
	Cle     string          `json:"cle"`
	Champ   string          `json:"champ,omitempty"`
	Local   json.RawMessage `json:"local,omitempty"`
	Serveur json.RawMessage `json:"serveur,omitempty"`
	Raison  string          `json:"raison,omitempty"`
	Nom     string          `json:"nom,omitempty"`  // nom de la fiche concernée, pour l'affichage
	Type    string          `json:"type,omitempty"` // et son type
	Quand   float64         `json:"quand,omitempty"`
}

// Reponse du serveur à un échange.
type Reponse struct {
	Curseur     int64        `json:"curseur"`
	Changements []Changement `json:"changements"`
	Conflits    []Conflit    `json:"conflits"`
	// ImagesAttendues : les images dont le serveur n'a pas encore le contenu.
	ImagesAttendues []string `json:"images_attendues"`
}

// Recevoir : côté serveur, un échange avec une copie (voir plus haut).
func (s *Store) Recevoir(curseur int64, chs []Changement) (Reponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return Reponse{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := relever(tx); err != nil {
		return Reponse{}, err
	}
	distants, err := changementsDepuis(tx, curseur)
	if err != nil {
		return Reponse{}, err
	}
	changeLa := map[string]Changement{}
	modifieLa, supprimeLa := map[string]bool{}, map[string]bool{}
	for _, c := range distants {
		changeLa[c.adresse()] = c
		switch {
		case c.Champ == ChampExiste && c.Supprime():
			supprimeLa[c.objet()] = true
		case c.Champ != ChampExiste:
			modifieLa[c.objet()] = true
		}
	}
	parObjet := map[string][]Changement{}
	var ordre []string
	for _, c := range chs {
		if _, vu := parObjet[c.objet()]; !vu {
			ordre = append(ordre, c.objet())
		}
		parObjet[c.objet()] = append(parObjet[c.objet()], c)
	}
	var rep Reponse
	var aAppliquer []Changement
	entier, retenu := map[string]bool{}, map[string]bool{} // objets à renvoyer en entier ; à ne pas renvoyer
	for _, o := range ordre {
		l := parObjet[o]
		supprime := false
		for _, c := range l {
			if c.Champ == ChampExiste && c.Supprime() {
				supprime = true
			}
		}
		switch {
		case supprime && supprimeLa[o]: // supprimé des deux côtés
			continue
		case supprime && modifieLa[o]:
			rep.Conflits = append(rep.Conflits, Conflit{Genre: ConflitSupprimeIci, Objet: l[0].Objet, Cle: l[0].Cle})
			entier[o] = true
			continue
		case !supprime && supprimeLa[o]:
			rep.Conflits = append(rep.Conflits, Conflit{Genre: ConflitSupprimeLa, Objet: l[0].Objet, Cle: l[0].Cle})
			retenu[o] = true
			continue
		}
		for _, c := range l {
			if d, ok := changeLa[c.adresse()]; ok && !supprime {
				if string(d.Valeur) != string(c.Valeur) {
					rep.Conflits = append(rep.Conflits, Conflit{Genre: ConflitChamp, Objet: c.Objet, Cle: c.Cle, Champ: c.Champ, Local: c.Valeur, Serveur: d.Valeur})
				}
				continue
			}
			aAppliquer = append(aAppliquer, c)
		}
	}
	refus, _, err := appliquer(tx, aAppliquer, nil)
	if err != nil {
		return Reponse{}, err
	}
	if rep.Curseur, err = relever(tx); err != nil {
		return Reponse{}, err
	}
	etat, err := lireEtat(tx)
	if err != nil {
		return Reponse{}, err
	}
	// un champ refusé que le serveur n'a pas reste sur la copie (rien ne lui est renvoyé) : il peut
	// venir d'un modèle plus récent et serait perdu ; un objet refusé que le serveur n'a pas du tout
	// n'est signalé qu'une fois, par son champ « * »
	present := map[string]bool{}
	for _, c := range etat {
		if !c.Supprime() {
			present[c.objet()] = true
		}
	}
	signale := map[string]bool{}
	for _, r := range refus {
		if !present[r.objet()] {
			if !signale[r.objet()] {
				signale[r.objet()] = true
				rep.Conflits = append(rep.Conflits, Conflit{Genre: ConflitRefus, Objet: r.Objet, Cle: r.Cle, Champ: ChampExiste,
					Local: jsonVrai, Serveur: jsonNull, Raison: r.Raison})
			}
			continue
		}
		rep.Conflits = append(rep.Conflits, Conflit{Genre: ConflitRefus, Objet: r.Objet, Cle: r.Cle, Champ: r.Champ, Local: r.Valeur,
			Serveur: valeurEtat(etat, r.adresse()), Raison: r.Raison})
	}
	// la réponse : tout ce qui a changé depuis le curseur, sauf les objets que la copie garde ; plus,
	// en entier, les objets qu'elle a supprimés et que le serveur garde ; plus la valeur du serveur
	// des champs refusés
	vus := map[string]int{}
	ajouter := func(c Changement) {
		if i, ok := vus[c.adresse()]; ok {
			rep.Changements[i] = c
			return
		}
		vus[c.adresse()] = len(rep.Changements)
		rep.Changements = append(rep.Changements, c)
	}
	recents, err := changementsDepuis(tx, curseur)
	if err != nil {
		return Reponse{}, err
	}
	for _, c := range recents {
		if !retenu[c.objet()] {
			ajouter(c)
		}
	}
	for _, c := range etat {
		if entier[c.objet()] && !c.Supprime() {
			ajouter(c)
		}
	}
	for _, r := range refus {
		if _, connu := etat[r.adresse()]; connu && !retenu[r.objet()] {
			ajouter(Changement{Objet: r.Objet, Cle: r.Cle, Champ: r.Champ, Valeur: valeurEtat(etat, r.adresse())})
		}
	}
	if rep.Changements == nil {
		rep.Changements = []Changement{}
	}
	if rep.Conflits == nil {
		rep.Conflits = []Conflit{}
	}
	rep.ImagesAttendues = []string{}
	rows, err := tx.Query(`SELECT fichier FROM medias WHERE contenu IS NULL ORDER BY id`)
	if err != nil {
		return Reponse{}, err
	}
	for rows.Next() {
		var f string
		if rows.Scan(&f) == nil {
			rep.ImagesAttendues = append(rep.ImagesAttendues, f)
		}
	}
	rows.Close()
	return rep, tx.Commit()
}

func changementsDepuis(q execer, seq int64) ([]Changement, error) {
	rows, err := q.Query(`SELECT objet, cle, champ, valeur, seq FROM sync_etat WHERE seq > ? ORDER BY seq`, seq)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Changement
	for rows.Next() {
		var c Changement
		var v string
		if err := rows.Scan(&c.Objet, &c.Cle, &c.Champ, &v, &c.Seq); err != nil {
			return nil, err
		}
		c.Valeur = json.RawMessage(v)
		out = append(out, c)
	}
	return out, rows.Err()
}

// lireEtat : tout l'état relevé, par adresse.
func lireEtat(q execer) (map[string]Changement, error) {
	rows, err := q.Query(`SELECT objet, cle, champ, valeur, seq FROM sync_etat`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]Changement{}
	for rows.Next() {
		var c Changement
		var v string
		if err := rows.Scan(&c.Objet, &c.Cle, &c.Champ, &v, &c.Seq); err != nil {
			return nil, err
		}
		c.Valeur = json.RawMessage(v)
		out[c.adresse()] = c
	}
	return out, rows.Err()
}

func valeurEtat(etat map[string]Changement, adresse string) json.RawMessage {
	if c, ok := etat[adresse]; ok {
		return c.Valeur
	}
	return jsonNull
}

// ---------------------------------------------------------------- côté copie

const schemaConflits = `
CREATE TABLE IF NOT EXISTS sync_conflits (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	quand REAL NOT NULL,
	genre TEXT NOT NULL,
	objet TEXT NOT NULL,
	cle TEXT NOT NULL,
	champ TEXT NOT NULL DEFAULT '',
	local TEXT NOT NULL DEFAULT 'null',
	serveur TEXT NOT NULL DEFAULT 'null',
	raison TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS sync_recus (
	objet TEXT NOT NULL,
	cle TEXT NOT NULL,
	champ TEXT NOT NULL,
	valeur TEXT NOT NULL,
	raison TEXT NOT NULL DEFAULT '',
	PRIMARY KEY (objet, cle, champ)
);
`

// Clés des réglages d'une copie synchronisée.
const (
	MetaSyncCurseur = "sync_curseur" // dernier numéro d'ordre du serveur reçu
	MetaSyncEnvoye  = "sync_envoye"  // dernier numéro d'ordre local envoyé
)

// Preparer : côté copie, relève et renvoie les changements locaux à envoyer, avec le curseur.
func (s *Store) Preparer() (curseur int64, chs []Changement, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := relever(tx); err != nil {
		return 0, nil, err
	}
	envoye := metaEntier(tx, MetaSyncEnvoye)
	if chs, err = changementsDepuis(tx, envoye); err != nil {
		return 0, nil, err
	}
	if chs == nil {
		chs = []Changement{}
	}
	return metaEntier(tx, MetaSyncCurseur), chs, tx.Commit()
}

// Integrer : côté copie, la réponse du serveur. envoye : le dernier numéro d'ordre local envoyé dans
// cet échange (celui de Preparer). Les conflits rejoignent la liste ; un objet supprimé sur le
// serveur et modifié ici est gardé et sera renvoyé en entier au prochain échange.
func (s *Store) Integrer(rep Reponse, envoye int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	now := maintenant()
	// ce qui a changé ici pendant l'échange : un champ que le serveur renvoie aussi est un conflit,
	// comme sur le serveur (la valeur du serveur s'applique, la valeur locale attend)
	if _, err := relever(tx); err != nil {
		return err
	}
	pendants, err := changementsDepuis(tx, envoye)
	if err != nil {
		return err
	}
	enCours := map[string]Changement{}
	for _, c := range pendants {
		enCours[c.adresse()] = c
	}
	for _, c := range rep.Changements {
		if l, ok := enCours[c.adresse()]; ok && string(l.Valeur) != string(c.Valeur) && c.Champ != ChampExiste {
			rep.Conflits = append(rep.Conflits, Conflit{Genre: ConflitChamp, Objet: c.Objet, Cle: c.Cle, Champ: c.Champ, Local: l.Valeur, Serveur: c.Valeur})
		}
	}
	for _, c := range rep.Conflits {
		if c.Genre == ConflitChamp || c.Genre == ConflitRefus {
			// la valeur locale est celle qui part ; si elle n'a pas changé depuis, rien à garder
			if string(c.Local) == string(c.Serveur) {
				continue
			}
		}
		local, serveur := string(c.Local), string(c.Serveur)
		if local == "" {
			local = "null"
		}
		if serveur == "" {
			serveur = "null"
		}
		if _, err := tx.Exec(`INSERT INTO sync_conflits (quand, genre, objet, cle, champ, local, serveur, raison) VALUES (?,?,?,?,?,?,?,?)`,
			now, c.Genre, c.Objet, c.Cle, c.Champ, local, serveur, c.Raison); err != nil {
			return err
		}
	}
	// les objets supprimés ici et gardés par le serveur reviennent en entier : ils sont recréés.
	// Ce que cette copie n'a pas pu appliquer (un modèle plus récent que le sien) attend dans
	// sync_recus et repasse à chaque échange, avant ce qui arrive : un changement plus récent du
	// même champ le remplace.
	attente, err := recusEnAttente(tx)
	if err != nil {
		return err
	}
	arrive := map[string]bool{}
	for _, c := range rep.Changements {
		arrive[c.adresse()] = true
	}
	var tous []Changement
	for _, c := range attente {
		if !arrive[c.adresse()] {
			tous = append(tous, c)
		}
	}
	tous = append(tous, rep.Changements...)
	refus, appliques, err := appliquer(tx, tous, nil)
	if err != nil {
		return err
	}
	if err := retenir(tx, appliques); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM sync_recus`); err != nil {
		return err
	}
	for _, r := range refus {
		if _, err := tx.Exec(`INSERT OR REPLACE INTO sync_recus (objet, cle, champ, valeur, raison) VALUES (?,?,?,?,?)`,
			r.Objet, r.Cle, r.Champ, string(r.Valeur), r.Raison); err != nil {
			return err
		}
	}
	for _, c := range rep.Conflits {
		if c.Genre == ConflitSupprimeLa {
			// tout l'objet repartira : son état relevé est oublié
			if _, err := tx.Exec(`DELETE FROM sync_etat WHERE objet=? AND cle=?`, c.Objet, c.Cle); err != nil {
				return err
			}
		}
	}
	for cle, v := range map[string]int64{MetaSyncCurseur: rep.Curseur, MetaSyncEnvoye: envoye} {
		if _, err := tx.Exec(`INSERT OR REPLACE INTO meta (cle, valeur) VALUES (?, ?)`, cle, strconv.FormatInt(v, 10)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func recusEnAttente(q execer) ([]Changement, error) {
	rows, err := q.Query(`SELECT objet, cle, champ, valeur FROM sync_recus ORDER BY rowid`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Changement
	for rows.Next() {
		var c Changement
		var v string
		if err := rows.Scan(&c.Objet, &c.Cle, &c.Champ, &v); err != nil {
			return nil, err
		}
		c.Valeur = json.RawMessage(v)
		out = append(out, c)
	}
	return out, rows.Err()
}

// RecusEnAttente : le nombre d'objets reçus du serveur que cette copie n'a pas pu appliquer ; ils
// le seront à un échange suivant, une fois l'application mise à jour.
func (s *Store) RecusEnAttente() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	var n int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM (SELECT DISTINCT objet, cle FROM sync_recus)`).Scan(&n)
	return n
}

// AEnvoyer : le nombre d'objets (fiches, relations, images…) que cette copie a changés et pas
// encore envoyés au serveur.
func (s *Store) AEnvoyer() (int, error) {
	_, chs, err := s.Preparer()
	return ObjetsChanges(chs), err
}

// ObjetsChanges : le nombre d'objets que touchent ces changements, hors journal.
func ObjetsChanges(chs []Changement) int {
	vus := map[string]bool{}
	for _, c := range chs {
		if c.Objet != ObjJournal {
			vus[c.objet()] = true
		}
	}
	return len(vus)
}

// EnvoyeJusqua : le numéro d'ordre local qu'atteint un envoi (le dernier changement préparé).
func EnvoyeJusqua(chs []Changement, avant int64) int64 {
	for _, c := range chs {
		if c.Seq > avant {
			avant = c.Seq
		}
	}
	return avant
}

func metaEntier(q execer, cle string) int64 {
	var v string
	_ = q.QueryRow(`SELECT valeur FROM meta WHERE cle=?`, cle).Scan(&v)
	n, _ := strconv.ParseInt(v, 10, 64)
	return n
}

// Conflits : la liste « Conflits » de la copie, des plus anciens aux plus récents.
func (s *Store) Conflits() ([]Conflit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT id, quand, genre, objet, cle, champ, local, serveur, raison FROM sync_conflits ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Conflit{}
	for rows.Next() {
		var c Conflit
		var local, serveur string
		if err := rows.Scan(&c.ID, &c.Quand, &c.Genre, &c.Objet, &c.Cle, &c.Champ, &local, &serveur, &c.Raison); err != nil {
			return nil, err
		}
		c.Local, c.Serveur = json.RawMessage(local), json.RawMessage(serveur)
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close() // une seule connexion : la lecture des noms vient après
	for i, c := range out {
		if c.Objet == ObjFiche || c.Objet == ObjFeuille {
			var nom, typ string
			if s.db.QueryRow(`SELECT nom, type FROM entites WHERE id=?`, c.Cle).Scan(&nom, &typ) == nil {
				out[i].Nom, out[i].Type = nom, typ
			}
		}
	}
	return out, nil
}

// ResoudreConflit : le MJ choisit. garderLocal : la valeur locale (ou la suppression, pour un objet
// supprimé ici) est réécrite et partira au prochain échange ; sinon la valeur du serveur, déjà en
// place, reste (pour un objet supprimé sur le serveur : il est supprimé ici aussi).
func (s *Store) ResoudreConflit(id int64, garderLocal bool, auteur string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var c Conflit
	var local string
	err = tx.QueryRow(`SELECT genre, objet, cle, champ, local FROM sync_conflits WHERE id=?`, id).Scan(&c.Genre, &c.Objet, &c.Cle, &c.Champ, &local)
	if err != nil {
		return invalide("Conflit introuvable.")
	}
	var ch []Changement
	switch {
	case c.Genre == ConflitRefus && garderLocal:
		// renvoyé au prochain échange : l'état relevé du champ (ou de tout l'objet, pour un objet que
		// le serveur n'a pas) est oublié
		q, args := `DELETE FROM sync_etat WHERE objet=? AND cle=? AND champ=?`, []any{c.Objet, c.Cle, c.Champ}
		if c.Champ == ChampExiste {
			q, args = `DELETE FROM sync_etat WHERE objet=? AND cle=?`, args[:2]
		}
		if _, err := tx.Exec(q, args...); err != nil {
			return err
		}
		if c.Champ != ChampExiste {
			ch = []Changement{{Objet: c.Objet, Cle: c.Cle, Champ: c.Champ, Valeur: json.RawMessage(local)}}
		}
	case c.Genre == ConflitRefus:
		// la valeur du serveur (absente : le champ ou l'objet est retiré ici), sans repartir
		var serveur string
		_ = tx.QueryRow(`SELECT serveur FROM sync_conflits WHERE id=?`, id).Scan(&serveur)
		v := []Changement{{Objet: c.Objet, Cle: c.Cle, Champ: c.Champ, Valeur: json.RawMessage(serveur)}}
		refus, appliques, err := appliquer(tx, v, nil)
		if err != nil {
			return err
		}
		if len(refus) > 0 {
			return invalide("Valeur refusée : %s", refus[0].Raison)
		}
		if err := retenir(tx, appliques); err != nil {
			return err
		}
	case c.Genre == ConflitChamp && garderLocal:
		ch = []Changement{{Objet: c.Objet, Cle: c.Cle, Champ: c.Champ, Valeur: json.RawMessage(local)}}
	case c.Genre == ConflitSupprimeIci && garderLocal, c.Genre == ConflitSupprimeLa && !garderLocal:
		ch = []Changement{{Objet: c.Objet, Cle: c.Cle, Champ: ChampExiste, Valeur: jsonNull}}
	}
	if len(ch) > 0 {
		// écrit sans être retenu : le prochain relevé le verra comme un changement local
		refus, _, err := appliquer(tx, ch, nil)
		if err != nil {
			return err
		}
		if len(refus) > 0 {
			return invalide("Valeur refusée : %s", refus[0].Raison)
		}
	}
	if _, err := tx.Exec(`DELETE FROM sync_conflits WHERE id=?`, id); err != nil {
		return err
	}
	choix := "valeur du serveur"
	if garderLocal {
		choix = "valeur de cette copie"
	}
	noter(tx, "conflit résolu", entiteDuConflit(c), "", fmt.Sprintf("%s %s %s : %s", c.Objet, c.Cle, c.Champ, choix), auteur)
	return tx.Commit()
}

func entiteDuConflit(c Conflit) int64 {
	if c.Objet != ObjFiche && c.Objet != ObjFeuille {
		return 0
	}
	n, _ := strconv.ParseInt(c.Cle, 10, 64)
	return n
}

// ---------------------------------------------------------------- copies inscrites (serveur)

// MetaRepliques : les copies inscrites auprès de ce serveur, {numéro: {nom, compte, cree, vu}}.
const MetaRepliques = "sync_repliques"

// CopieInscrite : une copie de l'univers tenue à jour depuis ce serveur.
type CopieInscrite struct {
	Numero int64   `json:"numero"`
	Nom    string  `json:"nom"`
	Compte string  `json:"compte"`
	Cree   float64 `json:"cree"`
	Vu     float64 `json:"vu"`
	// à son dernier passage : la version de l'application (vide avant la 1.2.3, qui ne l'annonçait
	// pas), son protocole et son modèle (voir versions.go)
	Version   string `json:"version"`
	Protocole int    `json:"protocole"`
	Modele    int    `json:"modele"`
}

func lireCopies(q execer) map[int64]CopieInscrite {
	var v string
	out := map[int64]CopieInscrite{}
	if q.QueryRow(`SELECT valeur FROM meta WHERE cle=?`, MetaRepliques).Scan(&v) == nil {
		_ = json.Unmarshal([]byte(v), &out)
	}
	return out
}

func ecrireCopies(q execer, l map[int64]CopieInscrite) error {
	b, _ := json.Marshal(l)
	_, err := q.Exec(`INSERT OR REPLACE INTO meta (cle, valeur) VALUES (?, ?)`, MetaRepliques, string(b))
	return err
}

// Copies : les copies inscrites, par numéro.
func (s *Store) Copies() []CopieInscrite {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []CopieInscrite
	for _, c := range lireCopies(s.db) {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Numero < out[j].Numero })
	return out
}

// CopieConnue : la copie n est inscrite ; son passage est noté, avec les numéros de l'application
// qui la tient.
func (s *Store) CopieConnue(n int64, version string, protocole, modele int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	l := lireCopies(s.db)
	c, ok := l[n]
	if !ok {
		return false
	}
	c.Vu = maintenant()
	c.Version, c.Protocole, c.Modele = version, protocole, modele
	l[n] = c
	_ = ecrireCopies(s.db, l)
	return true
}

// Inscrire : côté serveur, une nouvelle copie reçoit son numéro et un instantané de l'univers, écrit
// dans fichier, avec les réglages qui en font une copie synchronisée (numéro, curseur). Renvoie le
// numéro.
func (s *Store) Inscrire(nom, compte, fichier string) (int64, error) {
	nom = strings.TrimSpace(nom)
	if nom == "" || len([]rune(nom)) > 80 {
		return 0, invalide("Donnez un nom à cette copie (80 caractères au plus).")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if replique(s.db) != 0 {
		return 0, invalide("Cet univers est lui-même une copie : il ne peut pas en servir d'autres.")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	seq, err := relever(tx)
	if err != nil {
		return 0, err
	}
	l := lireCopies(tx)
	n := int64(1)
	for k := range l {
		if k >= n {
			n = k + 1
		}
	}
	if n >= repliqueReprise {
		return 0, invalide("Plus aucun numéro de copie libre pour cet univers.")
	}
	l[n] = CopieInscrite{Numero: n, Nom: nom, Compte: compte, Cree: maintenant(), Vu: maintenant()}
	if err := ecrireCopies(tx, l); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	if _, err := s.db.Exec(`VACUUM INTO ?`, fichier); err != nil {
		return 0, err
	}
	if err := os.Chmod(fichier, 0o600); err != nil {
		return 0, err
	}
	// l'instantané devient une copie : son numéro, son curseur ; la liste des copies reste au serveur
	cp, err := Ouvrir(fichier)
	if err != nil {
		return 0, err
	}
	defer cp.Fermer()
	for cle, v := range map[string]string{MetaReplique: strconv.FormatInt(n, 10), MetaSyncCurseur: strconv.FormatInt(seq, 10), MetaSyncEnvoye: strconv.FormatInt(seq, 10)} {
		if err := cp.SetMeta(cle, v); err != nil {
			return 0, err
		}
	}
	if err := cp.RetirerMeta(MetaRepliques); err != nil {
		return 0, err
	}
	return n, nil
}

// ContenuAttendu : côté serveur, le contenu d'une image dont il n'a que la fiche (envoyée par une
// copie). Une image qui a déjà son contenu ne change pas.
func (s *Store) ContenuAttendu(fichier string, contenu []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.Exec(`UPDATE medias SET contenu=? WHERE fichier=? AND contenu IS NULL`, contenu, fichier)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		var existe int
		if s.db.QueryRow(`SELECT COUNT(*) FROM medias WHERE fichier=?`, fichier).Scan(&existe); existe == 0 {
			return ErrIntrouvable
		}
	}
	return nil
}

// ImagesSansContenu : côté copie, les images reçues dont le contenu reste à télécharger.
func (s *Store) ImagesSansContenu() ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT fichier FROM medias WHERE contenu IS NULL ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var f string
		if rows.Scan(&f) == nil {
			out = append(out, f)
		}
	}
	return out, nil
}

// EstCopie : cet univers est une copie synchronisée d'un serveur.
func (s *Store) EstCopie() bool { return s.Replique() != 0 }
