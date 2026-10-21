package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/Tchyze/cof_dm/internal/fiches"
	"github.com/Tchyze/cof_dm/internal/modele"
	"github.com/Tchyze/cof_dm/internal/reference"
	"github.com/Tchyze/cof_dm/internal/regles"
)

// État de synchronisation : ce qui permet à deux copies d'un univers de se dire ce qui a changé.
//
// L'univers est vu comme un ensemble de champs : (objet, clé, champ) → valeur JSON. Les objets sont
// les fiches, relations, images, documents (bestiaire, combats terminés), feuilles de personnage et
// trois réglages (calendrier, règles, profils et voies). Le champ « * » d'un objet dit qu'il existe ;
// il vaut null quand l'objet est supprimé.
//
// La table sync_etat garde la dernière valeur relevée de chaque champ et le numéro d'ordre (seq) du
// relevé qui l'a vue changer. Relever compare l'univers à cette table : rien n'a à être noté au moment
// d'une écriture, et aucune écriture ne peut être oubliée. Un combat en cours n'est pas relevé : il
// n'existe que de son côté, et n'est pas pris pour une suppression.

const schemaSync = `
CREATE TABLE IF NOT EXISTS sync_etat (
	objet TEXT NOT NULL,
	cle TEXT NOT NULL,
	champ TEXT NOT NULL,
	valeur TEXT NOT NULL,
	seq INTEGER NOT NULL,
	PRIMARY KEY (objet, cle, champ)
);
CREATE INDEX IF NOT EXISTS sync_etat_seq ON sync_etat (seq);
`

// metaSyncSeq : le dernier numéro d'ordre donné par un relevé.
const metaSyncSeq = "sync_seq"

// Objets synchronisés.
const (
	ObjFiche    = "entite"
	ObjRelation = "relation"
	ObjImage    = "media"
	ObjDocument = "document"
	ObjFeuille  = "feuille"
	ObjReglage  = "meta"
	ObjJournal  = "journal"
	ChampExiste = "*"
)

// reglagesSync : les réglages de l'univers qui se synchronisent (dont les fonctionnalités
// activées) ; les autres (édition par les joueurs, page des joueurs…) tiennent à chaque côté.
var reglagesSync = []string{"calendrier", MetaRegles, MetaReference, MetaFonctions}

// Changement : la valeur d'un champ, telle qu'un relevé l'a vue à son numéro d'ordre.
type Changement struct {
	Objet  string          `json:"objet"`
	Cle    string          `json:"cle"`
	Champ  string          `json:"champ"`
	Valeur json.RawMessage `json:"valeur"` // null : champ retiré, ou objet supprimé (champ « * »)
	Seq    int64           `json:"seq,omitempty"`
}

func (c Changement) adresse() string { return c.Objet + "\x00" + c.Cle + "\x00" + c.Champ }
func (c Changement) objet() string   { return c.Objet + "\x00" + c.Cle }

// Supprime : le changement retire le champ (ou l'objet, pour « * »).
func (c Changement) Supprime() bool { return len(c.Valeur) == 0 || string(c.Valeur) == "null" }

var jsonNull = json.RawMessage("null")

var jsonVrai = json.RawMessage("true")

// canonique : la forme JSON stable d'une valeur (clés triées), pour comparer deux relevés.
func canonique(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "null"
	}
	return string(b)
}

// canoniqueJSON : la forme stable d'un texte JSON.
func canoniqueJSON(s string) string {
	var v any
	if json.Unmarshal([]byte(s), &v) != nil {
		return canonique(s) // pas du JSON : gardé comme texte
	}
	return canonique(v)
}

// aplatir lit tout l'univers champ par champ. masques : les objets à ne pas relever (combats en
// cours), qui ne doivent pas non plus passer pour supprimés.
func aplatir(q execer) (champs map[string]string, masques map[string]bool, err error) {
	champs, masques = map[string]string{}, map[string]bool{}
	mettre := func(objet string, cle int64, champ string, v string) {
		champs[Changement{Objet: objet, Cle: strconv.FormatInt(cle, 10), Champ: champ}.adresse()] = v
	}
	rows, err := q.Query(`SELECT id, type, nom, data, tags, archive_le IS NOT NULL, partage, cree FROM entites`)
	if err != nil {
		return nil, nil, err
	}
	for rows.Next() {
		var id int64
		var typ, nom, data, tags string
		var archive, partage bool
		var cree float64
		if err := rows.Scan(&id, &typ, &nom, &data, &tags, &archive, &partage, &cree); err != nil {
			rows.Close()
			return nil, nil, err
		}
		mettre(ObjFiche, id, ChampExiste, "true")
		mettre(ObjFiche, id, "type", canonique(typ))
		mettre(ObjFiche, id, "nom", canonique(nom))
		mettre(ObjFiche, id, "tags", canoniqueJSON(tags))
		mettre(ObjFiche, id, "archive", canonique(archive))
		mettre(ObjFiche, id, "partage", canonique(partage))
		mettre(ObjFiche, id, "cree", canonique(cree))
		var d map[string]any
		_ = json.Unmarshal([]byte(data), &d)
		for k, v := range d {
			mettre(ObjFiche, id, "data."+k, canonique(v))
		}
	}
	rows.Close()

	rows, err = q.Query(`SELECT id, a, b, type, note, rang, COALESCE(superieur,0), secrete, COALESCE(contexte,0) FROM relations`)
	if err != nil {
		return nil, nil, err
	}
	for rows.Next() {
		var r RelationBrute
		if err := rows.Scan(&r.ID, &r.A, &r.B, &r.Type, &r.Note, &r.Rang, &r.Superieur, &r.Secrete, &r.Contexte); err != nil {
			rows.Close()
			return nil, nil, err
		}
		mettre(ObjRelation, r.ID, ChampExiste, "true")
		for champ, v := range map[string]any{"a": r.A, "b": r.B, "type": r.Type, "note": r.Note, "rang": r.Rang, "superieur": r.Superieur, "secrete": r.Secrete, "contexte": r.Contexte} {
			mettre(ObjRelation, r.ID, champ, canonique(v))
		}
	}
	rows.Close()

	rows, err = q.Query(`SELECT id, entite, fichier, nom_original, legende, principal, cree FROM medias`)
	if err != nil {
		return nil, nil, err
	}
	for rows.Next() {
		var m Media
		if err := rows.Scan(&m.ID, &m.Entite, &m.Fichier, &m.NomOriginal, &m.Legende, &m.Principal, &m.Cree); err != nil {
			rows.Close()
			return nil, nil, err
		}
		mettre(ObjImage, m.ID, ChampExiste, "true")
		for champ, v := range map[string]any{"entite": m.Entite, "fichier": m.Fichier, "nom_original": m.NomOriginal, "legende": m.Legende, "principal": m.Principal, "cree": m.Cree} {
			mettre(ObjImage, m.ID, champ, canonique(v))
		}
	}
	rows.Close()

	rows, err = q.Query(`SELECT id, coll, data, cree FROM documents`)
	if err != nil {
		return nil, nil, err
	}
	for rows.Next() {
		var id int64
		var coll, data string
		var cree float64
		if err := rows.Scan(&id, &coll, &data, &cree); err != nil {
			rows.Close()
			return nil, nil, err
		}
		if coll == "combats" && !combatTermine(data) {
			masques[Changement{Objet: ObjDocument, Cle: strconv.FormatInt(id, 10)}.objet()] = true
			continue
		}
		mettre(ObjDocument, id, ChampExiste, "true")
		mettre(ObjDocument, id, "coll", canonique(coll))
		mettre(ObjDocument, id, "data", canoniqueJSON(data))
		mettre(ObjDocument, id, "cree", canonique(cree))
	}
	rows.Close()

	rows, err = q.Query(`SELECT entite, data FROM feuilles`)
	if err != nil {
		return nil, nil, err
	}
	for rows.Next() {
		var id int64
		var data string
		if err := rows.Scan(&id, &data); err != nil {
			rows.Close()
			return nil, nil, err
		}
		mettre(ObjFeuille, id, ChampExiste, "true")
		var d map[string]any
		_ = json.Unmarshal([]byte(data), &d)
		for k, v := range d {
			mettre(ObjFeuille, id, "data."+k, canonique(v))
		}
	}
	rows.Close()

	// le journal : une ligne, un champ ; il ne fait que s'allonger
	rows, err = q.Query(`SELECT id, quand, action, COALESCE(entite,0), nom, detail, auteur FROM journal`)
	if err != nil {
		return nil, nil, err
	}
	for rows.Next() {
		var l ligneSync
		if err := rows.Scan(&l.ID, &l.Quand, &l.Action, &l.Entite, &l.Nom, &l.Detail, &l.Auteur); err != nil {
			rows.Close()
			return nil, nil, err
		}
		mettre(ObjJournal, l.ID, "ligne", canonique(l))
	}
	rows.Close()

	for _, cle := range reglagesSync {
		var v string
		if q.QueryRow(`SELECT valeur FROM meta WHERE cle=?`, cle).Scan(&v) == nil && v != "" {
			champs[Changement{Objet: ObjReglage, Cle: cle, Champ: "valeur"}.adresse()] = canoniqueJSON(v)
		}
	}
	return champs, masques, nil
}

// ligneSync : une ligne du journal, telle qu'elle voyage.
type ligneSync struct {
	ID     int64   `json:"-"`
	Quand  float64 `json:"quand"`
	Action string  `json:"action"`
	Entite int64   `json:"entite,omitempty"`
	Nom    string  `json:"nom,omitempty"`
	Detail string  `json:"detail,omitempty"`
	Auteur string  `json:"auteur,omitempty"`
}

// combatTermine : un combat ne se synchronise qu'une fois terminé.
func combatTermine(data string) bool {
	var c struct {
		Termine bool `json:"termine"`
	}
	return json.Unmarshal([]byte(data), &c) == nil && c.Termine
}

// Relever compare l'univers à son dernier relevé et donne un nouveau numéro d'ordre à chaque champ
// qui a changé (null pour ce qui a disparu). Renvoie le dernier numéro d'ordre.
func (s *Store) Relever() (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	seq, err := relever(tx)
	if err != nil {
		return 0, err
	}
	return seq, tx.Commit()
}

func relever(tx execer) (int64, error) {
	actuel, masques, err := aplatir(tx)
	if err != nil {
		return 0, err
	}
	connu := map[string]string{}
	rows, err := tx.Query(`SELECT objet, cle, champ, valeur FROM sync_etat`)
	if err != nil {
		return 0, err
	}
	for rows.Next() {
		var c Changement
		var v string
		if err := rows.Scan(&c.Objet, &c.Cle, &c.Champ, &v); err != nil {
			rows.Close()
			return 0, err
		}
		connu[c.adresse()] = v
	}
	rows.Close()
	seq := seqCourant(tx)
	ecrire := func(adresse, v string) error {
		p := strings.SplitN(adresse, "\x00", 3)
		seq++
		_, err := tx.Exec(`INSERT INTO sync_etat (objet, cle, champ, valeur, seq) VALUES (?,?,?,?,?)
			ON CONFLICT(objet, cle, champ) DO UPDATE SET valeur=excluded.valeur, seq=excluded.seq`, p[0], p[1], p[2], v, seq)
		return err
	}
	// un ordre stable : les numéros suivent les objets, puis les champs
	cles := make([]string, 0, len(actuel))
	for k := range actuel {
		cles = append(cles, k)
	}
	sort.Strings(cles)
	for _, k := range cles {
		if v, ok := connu[k]; !ok || v != actuel[k] {
			if err := ecrire(k, actuel[k]); err != nil {
				return 0, err
			}
		}
	}
	cles = cles[:0]
	for k, v := range connu {
		if _, ok := actuel[k]; !ok && v != "null" {
			p := strings.SplitN(k, "\x00", 3)
			if !masques[p[0]+"\x00"+p[1]] {
				cles = append(cles, k)
			}
		}
	}
	sort.Strings(cles)
	for _, k := range cles {
		if err := ecrire(k, "null"); err != nil {
			return 0, err
		}
	}
	_, err = tx.Exec(`INSERT OR REPLACE INTO meta (cle, valeur) VALUES (?, ?)`, metaSyncSeq, strconv.FormatInt(seq, 10))
	return seq, err
}

func seqCourant(q execer) int64 {
	var v string
	_ = q.QueryRow(`SELECT valeur FROM meta WHERE cle=?`, metaSyncSeq).Scan(&v)
	n, _ := strconv.ParseInt(v, 10, 64)
	return n
}

// ChangementsDepuis : les champs relevés après le numéro d'ordre seq, dans l'ordre.
func (s *Store) ChangementsDepuis(seq int64) ([]Changement, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT objet, cle, champ, valeur, seq FROM sync_etat WHERE seq > ? ORDER BY seq`, seq)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Changement{}
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

// Refus : un changement reçu qui n'a pas pu être appliqué, et pourquoi.
type Refus struct {
	Changement
	Raison string `json:"raison"`
}

// Appliquer écrit dans l'univers des changements venus d'une autre copie, validés comme une saisie :
// un champ refusé n'empêche pas les autres (il est rendu dans les refus). contenus : le contenu des
// images nouvelles, par nom de fichier (une image sans contenu attend le sien).
//
// nouveaux : sur le serveur, ce qui vient d'être appliqué reçoit de nouveaux numéros d'ordre (les
// autres copies le recevront) ; sur une copie, l'état relevé prend les valeurs reçues sans les
// marquer comme changements locaux.
func (s *Store) Appliquer(chs []Changement, contenus map[string][]byte, nouveaux bool) ([]Refus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if nouveaux {
		// ce qui a changé ici avant l'application garde son propre numéro d'ordre
		if _, err := relever(tx); err != nil {
			return nil, err
		}
	}
	refus, appliques, err := appliquer(tx, chs, contenus)
	if err != nil {
		return nil, err
	}
	if nouveaux {
		if _, err := relever(tx); err != nil {
			return nil, err
		}
	} else if err := retenir(tx, appliques); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	if c, ok := lireCalendrier(s.db); ok {
		s.cal = c
	}
	return refus, nil
}

// retenir : sur une copie, l'état relevé des objets reçus prend leur valeur actuelle, sans nouveau
// numéro d'ordre (ce ne sont pas des changements locaux).
func retenir(tx execer, objets map[string]bool) error {
	actuel, _, err := aplatir(tx)
	if err != nil {
		return err
	}
	connu := map[string]bool{}
	rows, err := tx.Query(`SELECT objet, cle, champ FROM sync_etat`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var c Changement
		if rows.Scan(&c.Objet, &c.Cle, &c.Champ) == nil && objets[c.objet()] {
			connu[c.adresse()] = true
		}
	}
	rows.Close()
	for k, v := range actuel {
		p := strings.SplitN(k, "\x00", 3)
		if !objets[p[0]+"\x00"+p[1]] {
			continue
		}
		delete(connu, k)
		if _, err := tx.Exec(`INSERT INTO sync_etat (objet, cle, champ, valeur, seq) VALUES (?,?,?,?,0)
			ON CONFLICT(objet, cle, champ) DO UPDATE SET valeur=excluded.valeur`, p[0], p[1], p[2], v); err != nil {
			return err
		}
	}
	for k := range connu { // champs retirés par les changements reçus
		p := strings.SplitN(k, "\x00", 3)
		if _, err := tx.Exec(`UPDATE sync_etat SET valeur='null' WHERE objet=? AND cle=? AND champ=?`, p[0], p[1], p[2]); err != nil {
			return err
		}
	}
	return nil
}

// objetRecu : les changements reçus pour un objet, champ par champ.
type objetRecu struct {
	objet, cle string
	champs     map[string]Changement
	ordre      int
}

func (o *objetRecu) supprime() bool {
	c, ok := o.champs[ChampExiste]
	return ok && c.Supprime()
}

// appliquer fait le travail d'Appliquer dans la transaction. Ordre : réglages, fiches (toutes
// inscrites avant que leurs champs soient vérifiés, comme à la restauration), documents, relations,
// images, feuilles ; les suppressions en dernier, dans l'ordre inverse.
func appliquer(tx execer, chs []Changement, contenus map[string][]byte) (refus []Refus, appliques map[string]bool, err error) {
	appliques = map[string]bool{}
	objets := map[string]*objetRecu{}
	for i, c := range chs {
		o := objets[c.objet()]
		if o == nil {
			o = &objetRecu{objet: c.Objet, cle: c.Cle, champs: map[string]Changement{}, ordre: i}
			objets[c.objet()] = o
		}
		o.champs[c.Champ] = c
	}
	rang := map[string]int{ObjReglage: 0, ObjFiche: 1, ObjDocument: 2, ObjRelation: 3, ObjImage: 4, ObjFeuille: 5, ObjJournal: 6}
	liste := make([]*objetRecu, 0, len(objets))
	for _, o := range objets {
		if _, ok := rang[o.objet]; !ok {
			for _, c := range o.champs {
				refus = append(refus, Refus{c, "objet inconnu"})
			}
			continue
		}
		liste = append(liste, o)
	}
	sort.SliceStable(liste, func(i, j int) bool {
		if rang[liste[i].objet] != rang[liste[j].objet] {
			return rang[liste[i].objet] < rang[liste[j].objet]
		}
		return liste[i].ordre < liste[j].ordre
	})
	refuser := func(o *objetRecu, raison string) {
		for _, c := range o.champs {
			refus = append(refus, Refus{c, raison})
		}
	}
	cal := modele.CalendrierParDefaut()
	if c, ok := lireCalendrier(tx); ok {
		cal = c
	}

	// 1. créations et modifications ; les fiches sont toutes écrites avant d'être vérifiées
	var fichesRecues []*objetRecu
	for _, o := range liste {
		if o.supprime() {
			continue
		}
		var e error
		switch o.objet {
		case ObjReglage:
			e = appliquerReglage(tx, o)
			if e == nil && o.cle == "calendrier" {
				if c, ok := lireCalendrier(tx); ok {
					cal = c
				}
			}
		case ObjFiche:
			e = ecrireFiche(tx, o)
			if e == nil {
				fichesRecues = append(fichesRecues, o)
			}
		}
		if e != nil {
			refuser(o, e.Error())
			continue
		}
		if o.objet != ObjFiche {
			appliques[o.objet+"\x00"+o.cle] = true
		}
	}
	for _, o := range fichesRecues {
		r, e := verifierFiche(tx, o, cal)
		refus = append(refus, r...)
		if e != nil {
			return nil, nil, e
		}
		appliques[o.objet+"\x00"+o.cle] = true
	}
	for _, o := range liste {
		if o.supprime() || o.objet == ObjReglage || o.objet == ObjFiche {
			continue
		}
		var e error
		switch o.objet {
		case ObjDocument:
			e = ecrireDocument(tx, o)
		case ObjRelation:
			e = ecrireRelation(tx, o)
		case ObjImage:
			e = ecrireImage(tx, o, contenus)
		case ObjFeuille:
			e = ecrireFeuille(tx, o)
		case ObjJournal:
			e = ecrireLigne(tx, o)
		}
		if e != nil {
			var ev ErrValidation
			if !errors.As(e, &ev) {
				return nil, nil, e
			}
			refuser(o, e.Error())
			continue
		}
		appliques[o.objet+"\x00"+o.cle] = true
	}

	// 2. suppressions, dans l'ordre inverse (une relation avant sa fiche)
	tables := map[string]string{ObjFiche: "entites", ObjRelation: "relations", ObjImage: "medias", ObjDocument: "documents", ObjFeuille: "feuilles", ObjJournal: "journal"}
	for i := len(liste) - 1; i >= 0; i-- {
		o := liste[i]
		if !o.supprime() {
			continue
		}
		id, e := strconv.ParseInt(o.cle, 10, 64)
		if o.objet == ObjReglage || e != nil {
			refuser(o, "suppression impossible")
			continue
		}
		col := "id"
		if o.objet == ObjFeuille {
			col = "entite"
		}
		if o.objet == ObjFiche {
			for _, q := range []string{`DELETE FROM refs WHERE source=?`, `DELETE FROM mentions WHERE source=?`, `DELETE FROM recherche WHERE rowid=?`} {
				if _, err := tx.Exec(q, id); err != nil {
					return nil, nil, err
				}
			}
		}
		if _, err := tx.Exec(`DELETE FROM `+tables[o.objet]+` WHERE `+col+`=?`, id); err != nil {
			return nil, nil, err
		}
		appliques[o.objet+"\x00"+o.cle] = true
	}
	return refus, appliques, nil
}

func lireCalendrier(q execer) (modele.Calendrier, bool) {
	var v string
	if q.QueryRow(`SELECT valeur FROM meta WHERE cle='calendrier'`).Scan(&v) != nil {
		return modele.Calendrier{}, false
	}
	var c modele.Calendrier
	if json.Unmarshal([]byte(v), &c) != nil || c.Verifier() != nil {
		return modele.Calendrier{}, false
	}
	return c, true
}

// valeurRecue décode la valeur d'un champ reçu.
func valeurRecue(c Changement, v any) error {
	if err := json.Unmarshal(c.Valeur, v); err != nil {
		return invalide("%s : valeur illisible", c.Champ)
	}
	return nil
}

func appliquerReglage(tx execer, o *objetRecu) error {
	c, ok := o.champs["valeur"]
	if !ok || len(o.champs) != 1 {
		return invalide("réglage : seul le champ « valeur » se synchronise")
	}
	var texte string
	switch {
	case c.Supprime():
	case o.cle == "calendrier":
		var cal modele.Calendrier
		if err := valeurRecue(c, &cal); err != nil {
			return err
		}
		if err := cal.Verifier(); err != nil {
			return invalide("calendrier : %v", err)
		}
		texte = canonique(cal)
	case o.cle == MetaRegles:
		rg, err := regles.Relire(c.Valeur)
		if err != nil {
			return invalide("règles : %v", err)
		}
		texte = canonique(rg)
	case o.cle == MetaReference:
		ref, err := reference.Lire(c.Valeur)
		if err != nil {
			return invalide("profils et voies : %v", err)
		}
		texte = canonique(ref)
	case o.cle == MetaFonctions:
		choix, err := lireFonctions(c.Valeur)
		if err != nil {
			return err
		}
		texte = canonique(choix)
	default:
		return invalide("réglage %q : il ne se synchronise pas", o.cle)
	}
	if texte == "" {
		_, err := tx.Exec(`DELETE FROM meta WHERE cle=?`, o.cle)
		return err
	}
	_, err := tx.Exec(`INSERT OR REPLACE INTO meta (cle, valeur) VALUES (?, ?)`, o.cle, texte)
	return err
}

// ecrireFiche inscrit une fiche reçue (création ou modification) sans vérifier ses champs, qui peuvent
// désigner une fiche reçue plus loin : verifierFiche le fait ensuite.
func ecrireFiche(tx execer, o *objetRecu) error {
	id, err := strconv.ParseInt(o.cle, 10, 64)
	if err != nil || id <= 0 {
		return invalide("identifiant de fiche invalide")
	}
	e, err := lire(tx, id)
	nouvelle := errors.Is(err, ErrIntrouvable)
	if err != nil && !nouvelle {
		return err
	}
	if e.Data == nil {
		e.Data = map[string]any{}
	}
	e.ID = id
	for champ, c := range o.champs {
		switch {
		case champ == ChampExiste:
		case champ == "type":
			if err := valeurRecue(c, &e.Type); err != nil {
				return err
			}
		case champ == "nom":
			if err := valeurRecue(c, &e.Nom); err != nil {
				return err
			}
		case champ == "tags":
			e.Tags = nil
			if !c.Supprime() {
				if err := valeurRecue(c, &e.Tags); err != nil {
					return err
				}
			}
		case champ == "archive":
			_ = valeurRecue(c, &e.Archive)
		case champ == "partage":
			_ = valeurRecue(c, &e.Partage)
		case champ == "cree":
			_ = valeurRecue(c, &e.Cree)
		case strings.HasPrefix(champ, "data."):
			k := strings.TrimPrefix(champ, "data.")
			if c.Supprime() {
				delete(e.Data, k)
				continue
			}
			var v any
			if err := valeurRecue(c, &v); err != nil {
				return err
			}
			e.Data[k] = v
		default:
			return invalide("champ de fiche inconnu %q", champ)
		}
	}
	if _, ok := modele.TypeDe(e.Type); !ok {
		return invalide("type de fiche inconnu %q", e.Type)
	}
	if e.Nom, err = nettoyerNom(e.Nom); err != nil {
		return err
	}
	e.Tags = nettoyerTags(e.Tags)
	now := maintenant()
	if e.Cree == 0 {
		e.Cree = now
	}
	var arch any
	if e.Archive {
		arch = now
		var avant sql.NullFloat64
		if tx.QueryRow(`SELECT archive_le FROM entites WHERE id=?`, id).Scan(&avant) == nil && avant.Valid {
			arch = avant.Float64
		}
	}
	dj, _ := json.Marshal(e.Data)
	tj, _ := json.Marshal(e.Tags)
	partage := e.Partage && e.Type != "secret"
	if nouvelle {
		_, err = tx.Exec(`INSERT INTO entites (id, type, nom, cle_nom, data, tags, archive_le, partage, cree, modifie) VALUES (?,?,?,?,?,?,?,?,?,?)`,
			id, e.Type, e.Nom, cle(e.Nom), string(dj), string(tj), arch, partage, e.Cree, now)
	} else {
		_, err = tx.Exec(`UPDATE entites SET type=?, nom=?, cle_nom=?, data=?, tags=?, archive_le=?, partage=?, cree=?, modifie=? WHERE id=?`,
			e.Type, e.Nom, cle(e.Nom), string(dj), string(tj), arch, partage, e.Cree, now, id)
	}
	return err
}

// verifierFiche valide les champs d'une fiche reçue, une fois toutes les fiches inscrites. Un champ
// refusé est retiré de la fiche et rendu dans les refus ; la fiche est ensuite réindexée.
func verifierFiche(tx execer, o *objetRecu, cal modele.Calendrier) ([]Refus, error) {
	id, _ := strconv.ParseInt(o.cle, 10, 64)
	e, err := lire(tx, id)
	if err != nil {
		return nil, err
	}
	t, _ := modele.TypeDe(e.Type)
	var refus []Refus
	garde := map[string]any{}
	for k, v := range e.Data {
		norm, err := modele.Normaliser(t, map[string]any{k: v}, cal, accesFiches{tx})
		if err != nil {
			c, recu := o.champs["data."+k]
			if !recu {
				c = Changement{Objet: o.objet, Cle: o.cle, Champ: "data." + k, Valeur: json.RawMessage(canonique(v))}
			}
			refus = append(refus, Refus{c, fmt.Sprint(erreurChamps(err))})
			continue
		}
		for nk, nv := range norm {
			garde[nk] = nv
		}
	}
	// relu comme après un enregistrement
	dj, _ := json.Marshal(garde)
	e.Data = map[string]any{}
	_ = json.Unmarshal(dj, &e.Data)
	if _, err := tx.Exec(`UPDATE entites SET data=? WHERE id=?`, string(dj), id); err != nil {
		return nil, err
	}
	return refus, indexer(tx, e)
}

func ecrireDocument(tx execer, o *objetRecu) error {
	id, err := strconv.ParseInt(o.cle, 10, 64)
	if err != nil || id <= 0 {
		return invalide("identifiant de document invalide")
	}
	var coll, data string
	var cree float64
	err = tx.QueryRow(`SELECT coll, data, cree FROM documents WHERE id=?`, id).Scan(&coll, &data, &cree)
	nouveau := errors.Is(err, sql.ErrNoRows)
	if err != nil && !nouveau {
		return err
	}
	for champ, c := range o.champs {
		switch champ {
		case ChampExiste:
		case "coll":
			if err := valeurRecue(c, &coll); err != nil {
				return err
			}
		case "data":
			data = string(c.Valeur)
		case "cree":
			_ = valeurRecue(c, &cree)
		default:
			return invalide("champ de document inconnu %q", champ)
		}
	}
	if !collectionConnue(coll) {
		return invalide("collection inconnue %q", coll)
	}
	norm, err := normaliserDoc(coll, []byte(data))
	if err != nil {
		return invalide("document n°%d : %v", id, err)
	}
	now := maintenant()
	if cree == 0 {
		cree = now
	}
	if nouveau {
		_, err = tx.Exec(`INSERT INTO documents (id, coll, data, cree, modifie) VALUES (?,?,?,?,?)`, id, coll, string(norm), cree, now)
	} else {
		_, err = tx.Exec(`UPDATE documents SET coll=?, data=?, cree=?, modifie=? WHERE id=?`, coll, string(norm), cree, now, id)
	}
	return err
}

func ecrireRelation(tx execer, o *objetRecu) error {
	id, err := strconv.ParseInt(o.cle, 10, 64)
	if err != nil || id <= 0 {
		return invalide("identifiant de relation invalide")
	}
	var r RelationBrute
	err = tx.QueryRow(`SELECT a, b, type, note, rang, COALESCE(superieur,0), secrete, COALESCE(contexte,0) FROM relations WHERE id=?`, id).
		Scan(&r.A, &r.B, &r.Type, &r.Note, &r.Rang, &r.Superieur, &r.Secrete, &r.Contexte)
	nouvelle := errors.Is(err, sql.ErrNoRows)
	if err != nil && !nouvelle {
		return err
	}
	cibles := map[string]any{"a": &r.A, "b": &r.B, "type": &r.Type, "note": &r.Note, "rang": &r.Rang, "superieur": &r.Superieur, "secrete": &r.Secrete, "contexte": &r.Contexte}
	for champ, c := range o.champs {
		if champ == ChampExiste {
			continue
		}
		cible, ok := cibles[champ]
		if !ok {
			return invalide("champ de relation inconnu %q", champ)
		}
		if err := valeurRecue(c, cible); err != nil {
			return err
		}
	}
	if err := typeRelationValide(r.Type); err != nil {
		return err
	}
	var types [2]string
	for i, f := range []int64{r.A, r.B} {
		e, err := lire(tx, f)
		if err != nil {
			return invalide("relation n°%d : elle relie une fiche absente (n°%d)", id, f)
		}
		types[i] = e.Type
	}
	if err := relationPermise(r.Type, types[0], types[1]); err != nil {
		return invalide("relation n°%d : %v", id, err)
	}
	if r.A == r.B {
		return invalide("relation n°%d : une fiche ne peut pas être reliée à elle-même", id)
	}
	if nouvelle {
		_, err = tx.Exec(`INSERT INTO relations (id, a, b, type, note, rang, secrete, cree) VALUES (?,?,?,?,?,?,?,?)`,
			id, r.A, r.B, r.Type, r.Note, r.Rang, r.Secrete, maintenant())
	} else {
		_, err = tx.Exec(`UPDATE relations SET a=?, b=?, type=?, note=?, rang=?, secrete=?, superieur=NULL, contexte=NULL WHERE id=?`,
			r.A, r.B, r.Type, r.Note, r.Rang, r.Secrete, id)
	}
	if err != nil {
		return err
	}
	var d DetailRelation
	if r.Superieur != 0 {
		d.Superieur = &r.Superieur
	}
	if r.Contexte != 0 {
		d.Contexte = &r.Contexte
	}
	return appliquerDetail(tx, id, r.A, r.B, r.Type, d)
}

func ecrireImage(tx execer, o *objetRecu, contenus map[string][]byte) error {
	id, err := strconv.ParseInt(o.cle, 10, 64)
	if err != nil || id <= 0 {
		return invalide("identifiant d'image invalide")
	}
	var m Media
	err = tx.QueryRow(`SELECT entite, fichier, nom_original, legende, principal, cree FROM medias WHERE id=?`, id).
		Scan(&m.Entite, &m.Fichier, &m.NomOriginal, &m.Legende, &m.Principal, &m.Cree)
	nouvelle := errors.Is(err, sql.ErrNoRows)
	if err != nil && !nouvelle {
		return err
	}
	cibles := map[string]any{"entite": &m.Entite, "fichier": &m.Fichier, "nom_original": &m.NomOriginal, "legende": &m.Legende, "principal": &m.Principal, "cree": &m.Cree}
	for champ, c := range o.champs {
		if champ == ChampExiste {
			continue
		}
		cible, ok := cibles[champ]
		if !ok {
			return invalide("champ d'image inconnu %q", champ)
		}
		if err := valeurRecue(c, cible); err != nil {
			return err
		}
	}
	if !NomMediaValide(m.Fichier) {
		return invalide("image n°%d : nom de fichier refusé", id)
	}
	if _, err := lire(tx, m.Entite); err != nil {
		return invalide("image n°%d : rattachée à une fiche absente", id)
	}
	if nouvelle {
		var contenu []byte
		if c := contenus[m.Fichier]; len(c) > 0 {
			contenu = c
		}
		_, err = tx.Exec(`INSERT INTO medias (id, entite, fichier, nom_original, legende, principal, cree, contenu) VALUES (?,?,?,?,?,?,?,?)`,
			id, m.Entite, m.Fichier, m.NomOriginal, m.Legende, m.Principal, m.Cree, contenu)
	} else {
		_, err = tx.Exec(`UPDATE medias SET entite=?, fichier=?, nom_original=?, legende=?, principal=? WHERE id=?`,
			m.Entite, m.Fichier, m.NomOriginal, m.Legende, m.Principal, id)
		if c := contenus[m.Fichier]; err == nil && len(c) > 0 {
			_, err = tx.Exec(`UPDATE medias SET contenu=? WHERE id=? AND contenu IS NULL`, c, id)
		}
	}
	if err != nil && strings.Contains(err.Error(), "UNIQUE") {
		return invalide("image n°%d : un autre fichier porte déjà ce nom", id)
	}
	return err
}

func ecrireFeuille(tx execer, o *objetRecu) error {
	id, err := strconv.ParseInt(o.cle, 10, 64)
	if err != nil || id <= 0 {
		return invalide("identifiant de feuille invalide")
	}
	e, err := lire(tx, id)
	if err != nil {
		return invalide("feuille de la fiche n°%d : fiche absente", id)
	}
	if e.Type != "pj" {
		return invalide("feuille de la fiche n°%d : ce n'est pas un PJ", id)
	}
	d := map[string]any{}
	var data string
	if tx.QueryRow(`SELECT data FROM feuilles WHERE entite=?`, id).Scan(&data) == nil {
		_ = json.Unmarshal([]byte(data), &d)
	}
	for champ, c := range o.champs {
		switch {
		case champ == ChampExiste:
		case strings.HasPrefix(champ, "data."):
			k := strings.TrimPrefix(champ, "data.")
			if c.Supprime() {
				delete(d, k)
				continue
			}
			var v any
			if err := valeurRecue(c, &v); err != nil {
				return err
			}
			d[k] = v
		default:
			return invalide("champ de feuille inconnu %q", champ)
		}
	}
	niveau := 1
	if n, ok := e.Data["niveau"].(float64); ok && n >= 1 {
		niveau = int(n)
	}
	brut, _ := json.Marshal(d)
	f, err := fiches.NormaliserFeuille(brut, niveau, nil)
	if err != nil {
		return invalide("feuille de la fiche n°%d : %v", id, err)
	}
	_, err = tx.Exec(`INSERT INTO feuilles (entite, data, modifie) VALUES (?,?,?)
		ON CONFLICT(entite) DO UPDATE SET data=excluded.data, modifie=excluded.modifie`, id, string(f.JSON()), maintenant())
	return err
}

// ecrireLigne ajoute au journal une ligne venue d'une autre copie (une ligne ne change jamais).
func ecrireLigne(tx execer, o *objetRecu) error {
	id, err := strconv.ParseInt(o.cle, 10, 64)
	c, ok := o.champs["ligne"]
	if err != nil || id <= 0 || !ok {
		return invalide("ligne de journal invalide")
	}
	var l ligneSync
	if err := valeurRecue(c, &l); err != nil {
		return err
	}
	var e any
	if l.Entite > 0 {
		e = l.Entite
	}
	_, err = tx.Exec(`INSERT OR IGNORE INTO journal (id, quand, action, entite, nom, detail, auteur) VALUES (?,?,?,?,?,?,?)`,
		id, l.Quand, l.Action, e, l.Nom, l.Detail, l.Auteur)
	return err
}
