// Package store range les fiches de COF DM dans SQLite : données structurées (JSON validé par le
// modèle), relations, références entre fiches, mentions [[Nom]] dans les textes, images, recherche
// plein texte et journal.
package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Tchyze/cof_dm/internal/modele"
	"github.com/mattn/go-sqlite3"
)

var (
	ErrIntrouvable = errors.New("fiche introuvable")
	ErrConflit     = errors.New("la fiche a été modifiée ailleurs depuis son ouverture")
	ErrPasUnivers  = errors.New("ce fichier n'est pas un univers COF DM")
)

// pilote : SQLite avec les précautions recommandées pour un fichier venu d'ailleurs (un univers se
// copie et s'échange). trusted_schema=OFF empêche les vues et déclencheurs d'un fichier d'appeler
// des fonctions à effet de bord ; cell_size_check repère plus tôt une page abîmée.
const pilote = "sqlite3_cofdm"

func init() {
	sql.Register(pilote, &sqlite3.SQLiteDriver{ConnectHook: func(c *sqlite3.SQLiteConn) error {
		_, err := c.Exec(`PRAGMA trusted_schema=OFF; PRAGMA cell_size_check=ON`, nil)
		return err
	}})
}

// ErrValidation signale une saisie refusée ; son message s'affiche tel quel.
type ErrValidation struct{ Msg string }

func (e ErrValidation) Error() string { return e.Msg }

func invalide(format string, a ...any) error { return ErrValidation{fmt.Sprintf(format, a...)} }

// Entite est une fiche complète.
type Entite struct {
	ID      int64          `json:"id"`
	Type    string         `json:"type"`
	Nom     string         `json:"nom"`
	Data    map[string]any `json:"data"`
	Tags    []string       `json:"tags"`
	Archive bool           `json:"archive"`
	Partage bool           `json:"partage"`         // visible des joueurs (sans ses champs réservés au MJ)
	Image   string         `json:"image,omitempty"` // image principale (nom de fichier)
	Cree    float64        `json:"cree"`
	Modifie float64        `json:"modifie"`
}

// Resume est une fiche dans une liste.
type Resume struct {
	ID      int64          `json:"id"`
	Type    string         `json:"type"`
	Nom     string         `json:"nom"`
	Image   string         `json:"image,omitempty"`
	Tags    []string       `json:"tags,omitempty"`
	Carte   map[string]any `json:"carte,omitempty"` // champs résumés (voir modele.Type.Carte)
	Archive bool           `json:"archive,omitempty"`
	Partage bool           `json:"partage,omitempty"`
	Modifie float64        `json:"modifie"`
	Extrait string         `json:"extrait,omitempty"`
}

// Store est un univers ouvert : un seul fichier, qui contient les fiches, leurs images et le journal.
type Store struct {
	db     *sql.DB
	mu     sync.Mutex
	cal    modele.Calendrier
	chemin string
	id     string
}

// Extension est celle des fichiers d'univers.
const Extension = ".cofdm"

// idApplication marque les fichiers d'univers (PRAGMA application_id) : « COFD ».
const idApplication = 0x434F4644

const schema = `
CREATE TABLE IF NOT EXISTS entites (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	type TEXT NOT NULL,
	nom TEXT NOT NULL,
	cle_nom TEXT NOT NULL,
	data TEXT NOT NULL DEFAULT '{}',
	tags TEXT NOT NULL DEFAULT '[]',
	archive_le REAL,
	cree REAL NOT NULL,
	modifie REAL NOT NULL
);
CREATE INDEX IF NOT EXISTS entites_type ON entites(type);
CREATE INDEX IF NOT EXISTS entites_nom ON entites(cle_nom);
CREATE TABLE IF NOT EXISTS relations (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	a INTEGER NOT NULL REFERENCES entites(id) ON DELETE CASCADE,
	b INTEGER NOT NULL REFERENCES entites(id) ON DELETE CASCADE,
	type TEXT NOT NULL,
	note TEXT NOT NULL DEFAULT '',
	cree REAL NOT NULL
);
CREATE INDEX IF NOT EXISTS relations_a ON relations(a);
CREATE INDEX IF NOT EXISTS relations_b ON relations(b);
CREATE TABLE IF NOT EXISTS refs (
	source INTEGER NOT NULL REFERENCES entites(id) ON DELETE CASCADE,
	champ TEXT NOT NULL,
	cible INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS refs_cible ON refs(cible);
CREATE INDEX IF NOT EXISTS refs_source ON refs(source);
CREATE TABLE IF NOT EXISTS mentions (
	source INTEGER NOT NULL REFERENCES entites(id) ON DELETE CASCADE,
	champ TEXT NOT NULL,
	cle TEXT NOT NULL,
	texte TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS mentions_cle ON mentions(cle);
CREATE INDEX IF NOT EXISTS mentions_source ON mentions(source);
CREATE TABLE IF NOT EXISTS medias (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	entite INTEGER NOT NULL REFERENCES entites(id) ON DELETE CASCADE,
	fichier TEXT NOT NULL UNIQUE,
	nom_original TEXT NOT NULL DEFAULT '',
	legende TEXT NOT NULL DEFAULT '',
	principal INTEGER NOT NULL DEFAULT 0,
	cree REAL NOT NULL,
	contenu BLOB
);
CREATE INDEX IF NOT EXISTS medias_entite ON medias(entite);
CREATE VIRTUAL TABLE IF NOT EXISTS recherche USING fts5(nom, texte, tokenize='unicode61 remove_diacritics 2');
CREATE TABLE IF NOT EXISTS journal (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	quand REAL NOT NULL,
	action TEXT NOT NULL,
	entite INTEGER,
	nom TEXT NOT NULL DEFAULT '',
	detail TEXT NOT NULL DEFAULT '',
	auteur TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS meta (cle TEXT PRIMARY KEY, valeur TEXT);
`

// Ouvrir ouvre un univers, ou le crée si le fichier n'existe pas. Le journal de SQLite est en mode
// DELETE (et non WAL) : en dehors d'une écriture, tout l'univers tient dans son seul fichier, qui se
// copie tel quel.
func Ouvrir(chemin string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(chemin), 0o700); err != nil {
		return nil, err
	}
	if err := reserver(chemin); err != nil {
		return nil, err
	}
	db, err := sql.Open(pilote, chemin+"?_journal_mode=DELETE&_busy_timeout=5000&_foreign_keys=on")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := reconnaitre(db); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(schema + schemaDocuments + schemaFeuilles + schemaSync + schemaConflits); err != nil {
		db.Close()
		return nil, fmt.Errorf("création du schéma : %w", err)
	}
	if err := migrer(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("mise à jour du schéma : %w", err)
	}
	if err := verifierVersions(db); err != nil {
		db.Close()
		return nil, err
	}
	s := &Store{db: db, cal: modele.CalendrierParDefaut(), chemin: chemin}
	if v, ok := s.Meta("calendrier"); ok {
		var c modele.Calendrier
		if json.Unmarshal([]byte(v), &c) == nil && c.Verifier() == nil {
			s.cal = c
		}
	}
	if s.id, _ = s.Meta("univers_id"); s.id == "" {
		b := make([]byte, 8)
		_, _ = rand.Read(b)
		s.id = hex.EncodeToString(b)
		if err := s.SetMeta("univers_id", s.id); err != nil {
			db.Close()
			return nil, err
		}
	}
	if err := s.reparer(); err != nil {
		db.Close()
		return nil, fmt.Errorf("mise à jour des fiches : %w", err)
	}
	if err := fixerFonctionnalites(s.db); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// reserver crée le fichier, s'il n'existe pas, lisible par son seul propriétaire : un univers
// contient les secrets du MJ. SQLite le remplit ensuite. Un fichier existant garde ses droits.
func reserver(chemin string) error {
	f, err := os.OpenFile(chemin, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return f.Close()
}

// reconnaitre refuse les fichiers qui ne sont pas des univers : autre chose qu'une base SQLite, ou la
// base d'un autre programme. Une base vide (nouvel univers) ou d'avant le marquage est acceptée.
func reconnaitre(db *sql.DB) error {
	var marque, tables, fiches int
	err := db.QueryRow(`PRAGMA application_id`).Scan(&marque)
	if err == nil {
		err = db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(name='entites'),0) FROM sqlite_master WHERE type='table'`).Scan(&tables, &fiches)
	}
	if err != nil {
		if strings.Contains(err.Error(), "not a database") {
			return ErrPasUnivers
		}
		return err
	}
	if marque == idApplication {
		return verifierFichier(db)
	}
	if marque != 0 || (tables > 0 && fiches == 0) {
		return ErrPasUnivers
	}
	if tables > 0 {
		if err := verifierFichier(db); err != nil {
			return err
		}
	}
	_, err = db.Exec(fmt.Sprintf(`PRAGMA application_id=%d`, idApplication))
	return err
}

// verifierFichier contrôle un univers existant avant de s'en servir, puisqu'il a pu venir d'un
// tiers : intégrité de la base, et aucun objet qu'un univers ne contient jamais (déclencheur, vue,
// table virtuelle autre que l'index de recherche), qui s'exécuterait à chaque écriture.
func verifierFichier(db *sql.DB) error {
	var etat string
	if err := db.QueryRow(`PRAGMA quick_check(1)`).Scan(&etat); err != nil || etat != "ok" {
		if err != nil {
			etat = err.Error()
		}
		return invalide("Ce fichier d'univers est abîmé (contrôle de SQLite : %s). Ouvrez plutôt une de ses sauvegardes.", etat)
	}
	var genre, nom string
	err := db.QueryRow(`SELECT type, name FROM sqlite_master
		WHERE type IN ('trigger', 'view')
		   OR (type = 'table' AND sql LIKE 'CREATE VIRTUAL TABLE%' AND name <> 'recherche')
		LIMIT 1`).Scan(&genre, &nom)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil
	case err != nil:
		return err
	}
	return invalide("Ce fichier n'est pas ouvert : il contient un objet qu'un univers COF DM n'a jamais (%s « %s »). Il a pu être modifié par un autre programme.", genre, nom)
}

// Chemin renvoie le fichier de l'univers.
func (s *Store) Chemin() string { return s.chemin }

// ID identifie l'univers, quel que soit le nom ou la place de son fichier (ses copies le partagent).
func (s *Store) ID() string { return s.id }

func (s *Store) Fermer() error { return s.db.Close() }

func maintenant() float64 { return float64(time.Now().UnixNano()) / 1e9 }

func cle(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// ---------------------------------------------------------------- calendrier

func (s *Store) Calendrier() modele.Calendrier {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cal
}

func (s *Store) DefinirCalendrier(c modele.Calendrier) error {
	if err := c.Verifier(); err != nil {
		return ErrValidation{err.Error()}
	}
	b, _ := json.Marshal(c)
	if err := s.SetMeta("calendrier", string(b)); err != nil {
		return err
	}
	s.mu.Lock()
	s.cal = c
	s.mu.Unlock()
	return nil
}

// ---------------------------------------------------------------- validation

// accesFiches répond à la validation sans reprendre le verrou (on est déjà dans une transaction).
type accesFiches struct {
	q interface{ QueryRow(string, ...any) *sql.Row }
}

func (a accesFiches) TypeDeFiche(id int64) (string, bool) {
	var t string
	if err := a.q.QueryRow(`SELECT type FROM entites WHERE id=?`, id).Scan(&t); err != nil {
		return "", false
	}
	return t, true
}

// TypeDeFiche (hors transaction).
func (s *Store) TypeDeFiche(id int64) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return accesFiches{s.db}.TypeDeFiche(id)
}

func erreurChamps(err error) error {
	var ec modele.ErrChamps
	if errors.As(err, &ec) {
		return ErrValidation{"Champs refusés — " + ec.Error()}
	}
	return err
}

func nettoyerTags(tags []string) []string {
	out := []string{}
	vu := map[string]bool{}
	for _, t := range tags {
		t = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(t), "#"))
		if t == "" || vu[cle(t)] {
			continue
		}
		vu[cle(t)] = true
		out = append(out, t)
	}
	return out
}

func nettoyerNom(n string) (string, error) {
	n = strings.Join(strings.Fields(n), " ")
	if n == "" {
		return "", invalide("Le nom est obligatoire.")
	}
	if len([]rune(n)) > 200 {
		return "", invalide("Nom trop long (200 caractères au plus).")
	}
	if strings.ContainsAny(n, "[]|#") {
		return "", invalide("Le nom ne peut pas contenir [ ] | # (ils servent aux liens [[Nom]]).")
	}
	return n, nil
}

// ---------------------------------------------------------------- index secondaires

type execer interface {
	Exec(string, ...any) (sql.Result, error)
	QueryRow(string, ...any) *sql.Row
	Query(string, ...any) (*sql.Rows, error)
}

var reMention = regexp.MustCompile(`\[\[([^\[\]\n]+?)\]\]`)

// Mentions extrait les [[Nom]] d'un texte (texte affiché après | ignoré).
func Mentions(texte string) []string {
	var out []string
	for _, m := range reMention.FindAllStringSubmatch(texte, -1) {
		cible := m[1]
		if i := strings.Index(cible, "|"); i >= 0 {
			cible = cible[:i]
		}
		cible = strings.TrimSuffix(strings.TrimSpace(cible), "\\")
		if i := strings.Index(cible, "#"); i >= 0 {
			cible = cible[:i]
		}
		if cible = strings.TrimSpace(cible); cible != "" {
			out = append(out, cible)
		}
	}
	return out
}

// indexer recalcule références, mentions et entrée de recherche d'une fiche.
func indexer(tx execer, e Entite) error {
	t, _ := modele.TypeDe(e.Type)
	for _, q := range []string{`DELETE FROM refs WHERE source=?`, `DELETE FROM mentions WHERE source=?`, `DELETE FROM recherche WHERE rowid=?`} {
		if _, err := tx.Exec(q, e.ID); err != nil {
			return err
		}
	}
	var texte strings.Builder
	for _, ch := range t.Champs {
		v, ok := e.Data[ch.Cle]
		if !ok {
			continue
		}
		switch ch.Genre {
		case modele.Ref, modele.Refs:
			for _, id := range ids(v) {
				if _, err := tx.Exec(`INSERT INTO refs (source, champ, cible) VALUES (?,?,?)`, e.ID, ch.Cle, id); err != nil {
					return err
				}
			}
		case modele.Texte, modele.Long, modele.Choix:
			s, _ := v.(string)
			texte.WriteString(s + "\n")
			for _, m := range Mentions(s) {
				if _, err := tx.Exec(`INSERT INTO mentions (source, champ, cle, texte) VALUES (?,?,?,?)`, e.ID, ch.Cle, cle(m), m); err != nil {
					return err
				}
			}
		case modele.Etapes:
			for _, et := range etapes(v) {
				texte.WriteString(et + "\n")
			}
		}
	}
	texte.WriteString(strings.Join(e.Tags, " "))
	_, err := tx.Exec(`INSERT INTO recherche (rowid, nom, texte) VALUES (?,?,?)`, e.ID, e.Nom, texte.String())
	return err
}

// ids lit une valeur Ref / Refs (stockée ou fraîchement validée).
func ids(v any) []int64 {
	switch x := v.(type) {
	case int64:
		return []int64{x}
	case float64:
		return []int64{int64(x)}
	case []int64:
		return x
	case []any:
		var out []int64
		for _, y := range x {
			out = append(out, ids(y)...)
		}
		return out
	}
	return nil
}

func etapes(v any) []string {
	var out []string
	switch x := v.(type) {
	case []modele.Etape:
		for _, e := range x {
			out = append(out, e.Texte)
		}
	case []any:
		for _, y := range x {
			if m, ok := y.(map[string]any); ok {
				if t, ok := m["texte"].(string); ok {
					out = append(out, t)
				}
			}
		}
	}
	return out
}

func noter(tx execer, action string, id int64, nom, detail, auteur string) {
	var e any
	if id > 0 {
		e = id
	}
	id, err := prochainID(tx, "journal")
	if err != nil {
		return
	}
	_, _ = tx.Exec(`INSERT INTO journal (id, quand, action, entite, nom, detail, auteur) VALUES (?,?,?,?,?,?,?)`, id, maintenant(), action, e, nom, detail, auteur)
}

// ---------------------------------------------------------------- lecture

const colonnes = `e.id, e.type, e.nom, e.data, e.tags, e.archive_le, e.partage, e.cree, e.modifie,
	COALESCE((SELECT fichier FROM medias m WHERE m.entite=e.id ORDER BY principal DESC, id LIMIT 1), '')`

type scanner interface{ Scan(...any) error }

func scanEntite(r scanner) (Entite, error) {
	var e Entite
	var data, tags string
	var arch sql.NullFloat64
	if err := r.Scan(&e.ID, &e.Type, &e.Nom, &data, &tags, &arch, &e.Partage, &e.Cree, &e.Modifie, &e.Image); err != nil {
		return e, err
	}
	e.Archive = arch.Valid
	_ = json.Unmarshal([]byte(data), &e.Data)
	_ = json.Unmarshal([]byte(tags), &e.Tags)
	if e.Data == nil {
		e.Data = map[string]any{}
	}
	if e.Tags == nil {
		e.Tags = []string{}
	}
	return e, nil
}

func lire(q execer, id int64) (Entite, error) {
	e, err := scanEntite(q.QueryRow(`SELECT `+colonnes+` FROM entites e WHERE e.id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return e, ErrIntrouvable
	}
	return e, err
}

func (s *Store) Lire(id int64) (Entite, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return lire(s.db, id)
}

// Toutes renvoie toutes les fiches, archivées comprises.
func (s *Store) Toutes() ([]Entite, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return toutes(s.db)
}

func toutes(q execer) ([]Entite, error) {
	rows, err := q.Query(`SELECT ` + colonnes + ` FROM entites e ORDER BY e.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Entite
	for rows.Next() {
		e, err := scanEntite(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------- écriture

// Creer ajoute une fiche après validation de ses champs.
func (s *Store) Creer(typ, nom string, data map[string]any, tags []string, auteur string) (Entite, error) {
	t, ok := modele.TypeDe(typ)
	if !ok {
		return Entite{}, invalide("Type de fiche inconnu : %q.", typ)
	}
	nom, err := nettoyerNom(nom)
	if err != nil {
		return Entite{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return Entite{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if data == nil {
		data = map[string]any{}
	}
	norm, err := modele.Normaliser(t, data, s.cal, accesFiches{tx})
	if err != nil {
		return Entite{}, erreurChamps(err)
	}
	e := Entite{Type: typ, Nom: nom, Data: norm, Tags: nettoyerTags(tags)}
	dj, _ := json.Marshal(e.Data)
	tj, _ := json.Marshal(e.Tags)
	now := maintenant()
	if e.ID, err = prochainID(tx, "entites"); err != nil {
		return Entite{}, err
	}
	if _, err := tx.Exec(`INSERT INTO entites (id, type, nom, cle_nom, data, tags, cree, modifie) VALUES (?,?,?,?,?,?,?,?)`,
		e.ID, typ, nom, cle(nom), string(dj), string(tj), now, now); err != nil {
		return Entite{}, err
	}
	if err := indexer(tx, e); err != nil {
		return Entite{}, err
	}
	noter(tx, "création", e.ID, nom, t.Nom, auteur)
	if err := tx.Commit(); err != nil {
		return Entite{}, err
	}
	return lire(s.db, e.ID)
}

// Maj décrit une modification. Data est fusionné dans les données existantes (une clé à null efface
// le champ), sauf si Remplacer : Data devient alors la totalité des champs.
type Maj struct {
	Nom       *string        `json:"nom"`
	Data      map[string]any `json:"data"`
	Remplacer bool           `json:"remplacer"`
	Tags      *[]string      `json:"tags"`
	Attendu   float64        `json:"modifie"` // date de modification connue du client (0 : pas de contrôle)
}

// Verifier valide les champs d'une modification partielle (data fusionné à celui de la fiche) sans
// rien écrire : pour contrôler un ensemble de modifications avant d'en appliquer aucune.
func (s *Store) Verifier(id int64, data map[string]any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, err := lire(s.db, id)
	if err != nil {
		return err
	}
	t, _ := modele.TypeDe(e.Type)
	fusion := map[string]any{}
	for k, v := range e.Data {
		fusion[k] = v
	}
	for k, v := range data {
		if v == nil {
			delete(fusion, k)
		} else {
			fusion[k] = v
		}
	}
	if _, err := modele.Normaliser(t, fusion, s.cal, accesFiches{s.db}); err != nil {
		return erreurChamps(err)
	}
	return nil
}

// Modifier applique une modification. Un changement de nom réécrit les [[Ancien nom]] des autres fiches.
func (s *Store) Modifier(id int64, m Maj, auteur string) (Entite, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return Entite{}, err
	}
	defer func() { _ = tx.Rollback() }()
	e, err := lire(tx, id)
	if err != nil {
		return Entite{}, err
	}
	if m.Attendu != 0 && e.Modifie != m.Attendu {
		return Entite{}, ErrConflit
	}
	t, _ := modele.TypeDe(e.Type)
	ancienNom := e.Nom
	if m.Nom != nil {
		if e.Nom, err = nettoyerNom(*m.Nom); err != nil {
			return Entite{}, err
		}
	}
	if m.Data != nil || m.Remplacer {
		data := map[string]any{}
		if !m.Remplacer {
			for k, v := range e.Data {
				data[k] = v
			}
		}
		for k, v := range m.Data {
			if v == nil {
				delete(data, k)
			} else {
				data[k] = v
			}
		}
		if e.Data, err = modele.Normaliser(t, data, s.cal, accesFiches{tx}); err != nil {
			return Entite{}, erreurChamps(err)
		}
	}
	if m.Tags != nil {
		e.Tags = nettoyerTags(*m.Tags)
	}
	dj, _ := json.Marshal(e.Data)
	tj, _ := json.Marshal(e.Tags)
	if _, err := tx.Exec(`UPDATE entites SET nom=?, cle_nom=?, data=?, tags=?, modifie=? WHERE id=?`,
		e.Nom, cle(e.Nom), string(dj), string(tj), maintenant(), id); err != nil {
		return Entite{}, err
	}
	if err := indexer(tx, e); err != nil {
		return Entite{}, err
	}
	detail := ""
	if ancienNom != e.Nom {
		n, err := renommerMentions(tx, ancienNom, e.Nom)
		if err != nil {
			return Entite{}, err
		}
		detail = fmt.Sprintf("renommée (ancien nom : %s ; %d fiche(s) mise(s) à jour)", ancienNom, n)
	}
	noter(tx, "modification", id, e.Nom, detail, auteur)
	if err := tx.Commit(); err != nil {
		return Entite{}, err
	}
	return lire(s.db, id)
}

// renommerMentions remplace [[Ancien]] (et [[Ancien|texte]], [[Ancien#ancre]]) par le nouveau nom
// dans les textes des autres fiches.
func renommerMentions(tx execer, ancien, nouveau string) (int, error) {
	rows, err := tx.Query(`SELECT DISTINCT source FROM mentions WHERE cle=?`, cle(ancien))
	if err != nil {
		return 0, err
	}
	var sources []int64
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			sources = append(sources, id)
		}
	}
	rows.Close()
	re := regexp.MustCompile(`(?i)\[\[\s*` + regexp.QuoteMeta(ancien) + `\s*((?:\\?\||#)[^\]\n]*)?\]\]`)
	for _, id := range sources {
		e, err := lire(tx, id)
		if err != nil {
			return 0, err
		}
		for k, v := range e.Data {
			if s, ok := v.(string); ok {
				e.Data[k] = re.ReplaceAllString(s, "[["+nouveau+"$1]]")
			}
		}
		dj, _ := json.Marshal(e.Data)
		if _, err := tx.Exec(`UPDATE entites SET data=? WHERE id=?`, string(dj), id); err != nil {
			return 0, err
		}
		if err := indexer(tx, e); err != nil {
			return 0, err
		}
	}
	return len(sources), nil
}

// Archiver masque (ou rétablit) une fiche sans la supprimer.
func (s *Store) Archiver(id int64, archiver bool, auteur string) (Entite, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var v any
	action := "restauration"
	if archiver {
		v, action = maintenant(), "archivage"
	}
	res, err := s.db.Exec(`UPDATE entites SET archive_le=? WHERE id=?`, v, id)
	if err != nil {
		return Entite{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Entite{}, ErrIntrouvable
	}
	e, err := lire(s.db, id)
	noter(s.db, action, id, e.Nom, "", auteur)
	return e, err
}

// Partager montre (ou cache) une fiche aux joueurs. Ses champs réservés au MJ ne leur sont jamais
// montrés, et une fiche « Secret » ne se partage pas.
func (s *Store) Partager(id int64, partager bool, auteur string) (Entite, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, err := lire(s.db, id)
	if err != nil {
		return Entite{}, err
	}
	if partager && e.Type == "secret" {
		return Entite{}, invalide("Une fiche « Secret » ne se partage pas avec les joueurs.")
	}
	if partager && e.Type == "aide" {
		return Entite{}, invalide("Une aide de jeu ne se partage pas : envoyez-la aux PJ, elle arrive dans leurs documents.")
	}
	if _, err := s.db.Exec(`UPDATE entites SET partage=? WHERE id=?`, partager, id); err != nil {
		return Entite{}, err
	}
	action := "fiche partagée avec les joueurs"
	if !partager {
		action = "fiche retirée aux joueurs"
	}
	noter(s.db, action, id, e.Nom, "", auteur)
	return lire(s.db, id)
}

// Supprimer efface définitivement une fiche, ses relations et ses images (leurs noms sont
// renvoyés), et la retire des champs des autres fiches qui la référencent.
func (s *Store) Supprimer(id int64, auteur string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	e, err := lire(tx, id)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(`SELECT fichier FROM medias WHERE entite=?`, id)
	if err != nil {
		return nil, err
	}
	var fichiers []string
	for rows.Next() {
		var f string
		if rows.Scan(&f) == nil {
			fichiers = append(fichiers, f)
		}
	}
	rows.Close()
	rows, err = tx.Query(`SELECT DISTINCT source FROM refs WHERE cible=? AND source<>?`, id, id)
	if err != nil {
		return nil, err
	}
	var sources []int64
	for rows.Next() {
		var src int64
		if rows.Scan(&src) == nil {
			sources = append(sources, src)
		}
	}
	rows.Close()
	for _, src := range sources {
		o, err := lire(tx, src)
		if err != nil {
			return nil, err
		}
		t, _ := modele.TypeDe(o.Type)
		for _, ch := range t.Champs {
			switch ch.Genre {
			case modele.Ref:
				if l := ids(o.Data[ch.Cle]); len(l) == 1 && l[0] == id {
					delete(o.Data, ch.Cle)
				}
			case modele.Refs:
				var reste []int64
				for _, x := range ids(o.Data[ch.Cle]) {
					if x != id {
						reste = append(reste, x)
					}
				}
				if len(reste) == 0 {
					delete(o.Data, ch.Cle)
				} else {
					o.Data[ch.Cle] = reste
				}
			}
		}
		dj, _ := json.Marshal(o.Data)
		if _, err := tx.Exec(`UPDATE entites SET data=? WHERE id=?`, string(dj), src); err != nil {
			return nil, err
		}
		if err := indexer(tx, o); err != nil {
			return nil, err
		}
	}
	for _, q := range []string{`DELETE FROM recherche WHERE rowid=?`, `DELETE FROM entites WHERE id=?`} {
		if _, err := tx.Exec(q, id); err != nil {
			return nil, err
		}
	}
	noter(tx, "suppression", 0, e.Nom, "fiche "+e.Type+" supprimée définitivement", auteur)
	return fichiers, tx.Commit()
}

// ---------------------------------------------------------------- listes et recherche

// Filtre restreint une liste de fiches.
type Filtre struct {
	Type     string
	Q        string // mots, "phrase exacte", #tag
	Tag      string
	Archives bool
	Campagne int64 // fiches rattachées à cette campagne (champ campagne / campagnes)
	Limite   int
}

func (s *Store) resume(e Entite) Resume {
	r := Resume{ID: e.ID, Type: e.Type, Nom: e.Nom, Image: e.Image, Tags: e.Tags, Archive: e.Archive, Partage: e.Partage, Modifie: e.Modifie}
	if t, ok := modele.TypeDe(e.Type); ok {
		r.Carte = map[string]any{}
		for _, k := range t.Carte {
			if v, ok := e.Data[k]; ok {
				r.Carte[k] = v
			}
		}
	}
	return r
}

// ResumeDe renvoie le résumé d'une fiche (carte des listes).
func (s *Store) ResumeDe(e Entite) Resume { return s.resume(e) }

func (s *Store) resumes(es []Entite) []Resume {
	out := make([]Resume, 0, len(es))
	for _, e := range es {
		out = append(out, s.resume(e))
	}
	return out
}

// Lister renvoie les fiches qui correspondent au filtre : les plus pertinentes d'abord s'il y a une
// recherche, sinon par nom (les quêtes par ordre, les séances par numéro).
func (s *Store) Lister(f Filtre) ([]Resume, error) {
	tout, err := s.Toutes()
	if err != nil {
		return nil, err
	}
	mots, phrases, tags := analyserRecherche(f.Q)
	if f.Tag != "" {
		tags = append(tags, f.Tag)
	}
	garder := func(e Entite) bool {
		if e.Archive && !f.Archives {
			return false
		}
		if f.Type != "" && e.Type != f.Type {
			return false
		}
		for _, t := range tags {
			if !aTag(e, t) {
				return false
			}
		}
		if f.Campagne > 0 && !rattache(e, f.Campagne) {
			return false
		}
		return true
	}
	var out []Resume
	if len(mots) == 0 && len(phrases) == 0 {
		var es []Entite
		for _, e := range tout {
			if garder(e) {
				es = append(es, e)
			}
		}
		trier(es)
		return limiter(s.resumes(es), f.Limite), nil
	}
	score := map[int64]float64{}
	extraits := map[int64]string{}
	if q := requeteFTS(mots, phrases); q != "" {
		s.mu.Lock()
		rows, err := s.db.Query(`SELECT rowid, bm25(recherche, 10.0, 1.0), snippet(recherche, 1, '«', '»', '…', 12)
			FROM recherche WHERE recherche MATCH ? ORDER BY 2 LIMIT 500`, q)
		if err == nil {
			for rows.Next() {
				var id int64
				var sc float64
				var ex string
				if rows.Scan(&id, &sc, &ex) == nil {
					score[id], extraits[id] = 100-sc, ex
				}
			}
			rows.Close()
		}
		s.mu.Unlock()
	}
	if len(phrases) == 0 {
		cible := Plier(strings.Join(mots, " "))
		for _, e := range tout {
			if _, deja := score[e.ID]; !deja && cible != "" {
				if d := distanceNom(cible, Plier(e.Nom)); d >= 0 {
					score[e.ID] = 50 - float64(d)*10
				}
			}
		}
	}
	for _, e := range tout {
		if sc, ok := score[e.ID]; ok && garder(e) {
			score[e.ID] = sc + bonusNom(Plier(e.Nom), mots)
			r := s.resume(e)
			r.Extrait = extraits[e.ID]
			out = append(out, r)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return score[out[i].ID] > score[out[j].ID] })
	return limiter(out, f.Limite), nil
}

func trier(es []Entite) {
	num := func(e Entite, k string) float64 {
		if n, ok := e.Data[k].(float64); ok {
			return n
		}
		return 1e9
	}
	sort.SliceStable(es, func(i, j int) bool {
		a, b := es[i], es[j]
		if a.Type == b.Type {
			switch a.Type {
			case "quete":
				if num(a, "ordre") != num(b, "ordre") {
					return num(a, "ordre") < num(b, "ordre")
				}
			case "seance":
				return num(a, "numero") > num(b, "numero") // la plus récente d'abord
			}
		}
		return Plier(a.Nom) < Plier(b.Nom)
	})
}

func rattache(e Entite, campagne int64) bool {
	if e.ID == campagne {
		return true
	}
	for _, k := range []string{"campagne", "campagnes"} {
		for _, id := range ids(e.Data[k]) {
			if id == campagne {
				return true
			}
		}
	}
	return false
}

func limiter(r []Resume, n int) []Resume {
	if r == nil {
		r = []Resume{}
	}
	if n > 0 && len(r) > n {
		return r[:n]
	}
	return r
}

func aTag(e Entite, tag string) bool {
	for _, t := range e.Tags {
		if Plier(t) == Plier(tag) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------- références et mentions

// Reference : une autre fiche pointe vers celle-ci, par un champ ou par une mention [[Nom]].
type Reference struct {
	Source Resume `json:"source"`
	Champ  string `json:"champ"` // nom lisible du champ
	Genre  string `json:"genre"` // "champ" ou "mention"
}

func nomChamp(typ, champ string) string {
	if t, ok := modele.TypeDe(typ); ok {
		if c, ok := t.Champ(champ); ok {
			return c.Nom
		}
	}
	return champ
}

// References liste les fiches qui pointent vers une fiche.
func (s *Store) References(id int64) ([]Reference, error) {
	e, err := s.Lire(id)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	type brut struct {
		src          int64
		champ, genre string
	}
	var bruts []brut
	for _, q := range []struct{ sql, genre string }{
		{`SELECT source, champ FROM refs WHERE cible=? AND source<>? ORDER BY source`, "champ"},
		{`SELECT DISTINCT source, champ FROM mentions WHERE cle=? AND source<>? ORDER BY source`, "mention"},
	} {
		var arg any = id
		if q.genre == "mention" {
			arg = cle(e.Nom)
		}
		rows, err := s.db.Query(q.sql, arg, id)
		if err != nil {
			s.mu.Unlock()
			return nil, err
		}
		for rows.Next() {
			var b brut
			if rows.Scan(&b.src, &b.champ) == nil {
				b.genre = q.genre
				bruts = append(bruts, b)
			}
		}
		rows.Close()
	}
	s.mu.Unlock()
	out := []Reference{}
	vu := map[string]bool{}
	for _, b := range bruts {
		src, err := s.Lire(b.src)
		if err != nil || src.Archive {
			continue
		}
		k := fmt.Sprint(b.src, b.champ, b.genre)
		if vu[k] {
			continue
		}
		vu[k] = true
		out = append(out, Reference{Source: s.resume(src), Champ: nomChamp(src.Type, b.champ), Genre: b.genre})
	}
	return out, nil
}

// Cible résolue d'une mention [[Nom]].
type CibleMention struct {
	Texte string `json:"texte"`
	ID    int64  `json:"id,omitempty"`
	Type  string `json:"type,omitempty"`
}

// ResoudreNoms associe chaque nom à une fiche (la première par ordre des types, puis par id).
func (s *Store) ResoudreNoms(noms []string) (map[string]CibleMention, error) {
	out := map[string]CibleMention{}
	if len(noms) == 0 {
		return out, nil
	}
	tout, err := s.Toutes()
	if err != nil {
		return nil, err
	}
	rang := map[string]int{}
	for i, t := range modele.Types {
		rang[t.Cle] = i
	}
	parNom := map[string]Entite{}
	for _, e := range tout {
		k := cle(e.Nom)
		if d, ok := parNom[k]; !ok || rang[e.Type] < rang[d.Type] || (d.Archive && !e.Archive) {
			parNom[k] = e
		}
	}
	for _, n := range noms {
		c := CibleMention{Texte: n}
		if e, ok := parNom[cle(n)]; ok {
			c.ID, c.Type = e.ID, e.Type
		}
		out[cle(n)] = c
	}
	return out, nil
}

// MentionsDe renvoie les mentions d'une fiche, résolues.
func (s *Store) MentionsDe(e Entite) (map[string]CibleMention, error) {
	var noms []string
	for _, v := range e.Data {
		if t, ok := v.(string); ok {
			noms = append(noms, Mentions(t)...)
		}
	}
	return s.ResoudreNoms(noms)
}

// LienMort : un [[Nom]] qui ne mène à aucune fiche.
type LienMort struct {
	Cible   string   `json:"cible"`
	Sources []Resume `json:"sources"`
}

func (s *Store) LiensMorts() ([]LienMort, error) {
	s.mu.Lock()
	rows, err := s.db.Query(`SELECT m.source, m.texte, m.cle FROM mentions m
		JOIN entites e ON e.id=m.source WHERE e.archive_le IS NULL
		AND NOT EXISTS (SELECT 1 FROM entites x WHERE x.cle_nom=m.cle) ORDER BY m.cle`)
	if err != nil {
		s.mu.Unlock()
		return nil, err
	}
	type brut struct {
		src        int64
		texte, cle string
	}
	var bruts []brut
	for rows.Next() {
		var b brut
		if rows.Scan(&b.src, &b.texte, &b.cle) == nil {
			bruts = append(bruts, b)
		}
	}
	rows.Close()
	s.mu.Unlock()
	parCle := map[string]*LienMort{}
	var ordre []string
	vu := map[string]bool{}
	for _, b := range bruts {
		m := parCle[b.cle]
		if m == nil {
			m = &LienMort{Cible: b.texte}
			parCle[b.cle] = m
			ordre = append(ordre, b.cle)
		}
		if k := fmt.Sprint(b.cle, b.src); !vu[k] {
			vu[k] = true
			if e, err := s.Lire(b.src); err == nil {
				m.Sources = append(m.Sources, s.resume(e))
			}
		}
	}
	out := []LienMort{}
	for _, k := range ordre {
		out = append(out, *parCle[k])
	}
	return out, nil
}

// ---------------------------------------------------------------- graphe

type Noeud struct {
	ID    int64  `json:"id"`
	Nom   string `json:"nom"`
	Type  string `json:"type"`
	Image string `json:"image,omitempty"`
}

type Arete struct {
	A       int64  `json:"a"`
	B       int64  `json:"b"`
	Libelle string `json:"libelle"`
	Genre   string `json:"genre"` // relation, champ, mention
}

// Graphe renvoie les fiches reliées à une fiche (relations, champs, mentions) jusqu'à la profondeur
// donnée ; sans centre, tout le monde (fiches actives).
func (s *Store) Graphe(centre int64, profondeur int) ([]Noeud, []Arete, error) {
	tout, err := s.Toutes()
	if err != nil {
		return nil, nil, err
	}
	parID := map[int64]Entite{}
	parNom := map[string]int64{}
	for _, e := range tout {
		if !e.Archive {
			parID[e.ID] = e
			if _, ok := parNom[cle(e.Nom)]; !ok {
				parNom[cle(e.Nom)] = e.ID
			}
		}
	}
	var aretes []Arete
	s.mu.Lock()
	lireAretes := func(q, genre string, f func(a, b int64, x string) Arete) error {
		rows, err := s.db.Query(q)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var a int64
			var bx any
			var x string
			if rows.Scan(&a, &bx, &x) != nil {
				continue
			}
			var b int64
			switch v := bx.(type) {
			case int64:
				b = v
			case string:
				b = parNom[v]
			}
			if _, ok := parID[a]; !ok {
				continue
			}
			if _, ok := parID[b]; !ok || a == b {
				continue
			}
			ar := f(a, b, x)
			ar.Genre = genre
			aretes = append(aretes, ar)
		}
		return rows.Err()
	}
	err = lireAretes(`SELECT a, b, type FROM relations`, "relation", func(a, b int64, x string) Arete {
		r, _ := modele.RelationDe(x)
		return Arete{A: a, B: b, Libelle: r.Nom}
	})
	if err == nil {
		err = lireAretes(`SELECT source, cible, champ FROM refs`, "champ", func(a, b int64, x string) Arete {
			return Arete{A: a, B: b, Libelle: nomChamp(parID[a].Type, x)}
		})
	}
	if err == nil {
		err = lireAretes(`SELECT DISTINCT source, cle, '' FROM mentions`, "mention", func(a, b int64, x string) Arete {
			return Arete{A: a, B: b, Libelle: "mentionne"}
		})
	}
	s.mu.Unlock()
	if err != nil {
		return nil, nil, err
	}
	// une seule arête par paire et par genre
	vu := map[string]bool{}
	var uniques []Arete
	for _, a := range aretes {
		x, y := a.A, a.B
		if x > y {
			x, y = y, x
		}
		k := fmt.Sprint(x, "-", y, a.Genre, a.Libelle)
		if !vu[k] {
			vu[k] = true
			uniques = append(uniques, a)
		}
	}
	garde := map[int64]bool{}
	if centre > 0 {
		if _, ok := parID[centre]; !ok {
			return nil, nil, ErrIntrouvable
		}
		garde[centre] = true
		front := []int64{centre}
		for d := 0; d < max(1, profondeur); d++ {
			var suivant []int64
			for _, n := range front {
				for _, a := range uniques {
					for _, p := range [][2]int64{{a.A, a.B}, {a.B, a.A}} {
						if p[0] == n && !garde[p[1]] {
							garde[p[1]] = true
							suivant = append(suivant, p[1])
						}
					}
				}
			}
			front = suivant
		}
	} else {
		for id := range parID {
			garde[id] = true
		}
	}
	var noeuds []Noeud
	for id := range garde {
		e := parID[id]
		noeuds = append(noeuds, Noeud{ID: id, Nom: e.Nom, Type: e.Type, Image: e.Image})
	}
	sort.Slice(noeuds, func(i, j int) bool { return noeuds[i].ID < noeuds[j].ID })
	var gardees []Arete
	for _, a := range uniques {
		if garde[a.A] && garde[a.B] {
			gardees = append(gardees, a)
		}
	}
	if noeuds == nil {
		noeuds = []Noeud{}
	}
	if gardees == nil {
		gardees = []Arete{}
	}
	return noeuds, gardees, nil
}

// ---------------------------------------------------------------- chronologie

// Moment de la chronologie : une fiche datée.
type Moment struct {
	Date   modele.DateJeu `json:"date"`
	Texte  string         `json:"date_texte"`
	Absolu int            `json:"absolu"`
	Champ  string         `json:"champ"`
	Fiche  Resume         `json:"fiche"`
}

// Chronologie renvoie toutes les dates en jeu (événements, séances, débuts de campagne…) dans l'ordre.
// Avec une campagne : celles de la campagne et l'histoire du monde (événements sans campagne).
func (s *Store) Chronologie(campagne int64) ([]Moment, error) {
	tout, err := s.Toutes()
	if err != nil {
		return nil, err
	}
	cal := s.Calendrier()
	out := []Moment{}
	for _, e := range tout {
		if e.Archive {
			continue
		}
		if campagne > 0 && !rattache(e, campagne) && !(e.Type == "evenement" && e.Data["campagne"] == nil) {
			continue
		}
		t, _ := modele.TypeDe(e.Type)
		for _, ch := range t.Champs {
			if ch.Genre != modele.DateMonde {
				continue
			}
			m, ok := e.Data[ch.Cle].(map[string]any)
			if !ok {
				continue
			}
			d := modele.DateJeu{An: entier(m["an"]), Mois: entier(m["mois"]), Jour: entier(m["jour"])}
			out = append(out, Moment{Date: d, Texte: cal.Texte(d), Absolu: cal.Absolu(d), Champ: ch.Nom, Fiche: s.resume(e)})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Absolu < out[j].Absolu })
	return out, nil
}

func entier(v any) int {
	if f, ok := v.(float64); ok {
		return int(f)
	}
	return 0
}

// ---------------------------------------------------------------- statistiques, journal, méta

type CompteType struct {
	Type   string `json:"type"`
	Nombre int    `json:"nombre"`
}

func (s *Store) Compter() (map[string]int, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT type, COUNT(*) FROM entites WHERE archive_le IS NULL GROUP BY type`)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := map[string]int{}
	total := 0
	for rows.Next() {
		var t string
		var n int
		if rows.Scan(&t, &n) == nil {
			out[t] = n
			total += n
		}
	}
	return out, total, rows.Err()
}

type LigneJournal struct {
	ID     int64   `json:"id"`
	Quand  float64 `json:"quand"`
	Action string  `json:"action"`
	Entite int64   `json:"entite,omitempty"`
	Nom    string  `json:"nom,omitempty"`
	Detail string  `json:"detail,omitempty"`
	Auteur string  `json:"auteur,omitempty"`
}

func (s *Store) Journal(limite int) ([]LigneJournal, error) {
	if limite <= 0 || limite > 1000 {
		limite = 200
	}
	return s.journal(limite)
}

// JournalEntier renvoie tout le journal, le plus récent en tête (export vers Obsidian).
func (s *Store) JournalEntier() ([]LigneJournal, error) { return s.journal(-1) }

// journal : les lignes du journal, le plus récent en tête ; limite négative : toutes.
func (s *Store) journal(limite int) ([]LigneJournal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT id, quand, action, COALESCE(entite,0), nom, detail, auteur FROM journal ORDER BY id DESC LIMIT ?`, limite)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LigneJournal{}
	for rows.Next() {
		var l LigneJournal
		if err := rows.Scan(&l.ID, &l.Quand, &l.Action, &l.Entite, &l.Nom, &l.Detail, &l.Auteur); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *Store) Noter(action, detail, auteur string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	noter(s.db, action, 0, "", detail, auteur)
}

func (s *Store) Meta(cle string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var v string
	if err := s.db.QueryRow(`SELECT valeur FROM meta WHERE cle=?`, cle).Scan(&v); err != nil {
		return "", false
	}
	return v, true
}

func (s *Store) SetMeta(cle, valeur string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`INSERT OR REPLACE INTO meta (cle, valeur) VALUES (?,?)`, cle, valeur)
	return err
}

// RetirerMeta efface des réglages de l'univers.
func (s *Store) RetirerMeta(cles ...string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range cles {
		if _, err := s.db.Exec(`DELETE FROM meta WHERE cle=?`, c); err != nil {
			return err
		}
	}
	return nil
}

// Sauvegarder copie l'univers entier (fiches, images, journal) dans un nouveau fichier.
func (s *Store) Sauvegarder(fichier string) error {
	if err := os.MkdirAll(filepath.Dir(fichier), 0o700); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.db.Exec(`VACUUM INTO ?`, fichier); err != nil {
		return err
	}
	return os.Chmod(fichier, 0o600)
}

// CopierBase copie une base SQLite dans un nouveau fichier sans toucher à l'originale (reprise des
// données d'avant les fichiers d'univers).
func CopierBase(source, cible string) error {
	if err := os.MkdirAll(filepath.Dir(cible), 0o700); err != nil {
		return err
	}
	db, err := sql.Open(pilote, source+"?_busy_timeout=5000")
	if err != nil {
		return err
	}
	defer db.Close()
	if _, err = db.Exec(`VACUUM INTO ?`, cible); err != nil {
		return err
	}
	return os.Chmod(cible, 0o600)
}
