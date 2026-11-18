package store

import (
	"database/sql"
	"errors"
	"strings"
)

// Notifications : ce que la cloche montre au MJ et aux joueurs (rappel de séance, séance déplacée ou
// annulée, relance, réponse d'un joueur, message du MJ). Elles naissent là où l'univers fait foi (le
// serveur, ou le bureau qui n'est relié à aucun serveur) et y restent : elles n'entrent pas dans la
// synchronisation (ni aplatir, ni appliquer), comme le journal de travail d'un seul côté. D'où des
// numéros locaux (auto-incrément), qui ne voyagent jamais.
//
// Destinataire (« pour ») : 0 pour le MJ (tous les MJ de l'univers la partagent), sinon le numéro du PJ
// à qui elle s'adresse ; son joueur la lit sur sa page.

const schemaNotifications = `
CREATE TABLE IF NOT EXISTS notifications (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	cree REAL NOT NULL,
	genre TEXT NOT NULL,
	pour INTEGER NOT NULL,
	titre TEXT NOT NULL,
	texte TEXT NOT NULL DEFAULT '',
	lien TEXT NOT NULL DEFAULT '',
	seance INTEGER NOT NULL DEFAULT 0,
	lue REAL NOT NULL DEFAULT 0,
	poussee INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS notifications_pour ON notifications (pour, id);
CREATE TABLE IF NOT EXISTS notif_seances (
	seance INTEGER PRIMARY KEY,
	signature TEXT NOT NULL,
	rappel TEXT NOT NULL DEFAULT ''
);
`

// PourMJ : le destinataire « le MJ ».
const PourMJ int64 = 0

// Genres de notification.
const (
	NotifRappel   = "rappel"   // la séance de demain
	NotifDeplacee = "deplacee" // la séance change de date ou d'heure
	NotifAnnulee  = "annulee"  // la séance est annulée
	NotifRelance  = "relance"  // le MJ attend une réponse
	NotifReponse  = "reponse"  // un joueur a répondu (pour le MJ)
	NotifMessage  = "message"  // un mot du MJ à la campagne
)

// gardeNotifications : ce qu'on garde au plus par destinataire, les plus anciennes partent.
const gardeNotifications = 100

type Notification struct {
	ID     int64   `json:"id"`
	Cree   float64 `json:"cree"`
	Genre  string  `json:"genre"`
	Pour   int64   `json:"pour"`
	Titre  string  `json:"titre"`
	Texte  string  `json:"texte,omitempty"`
	Lien   string  `json:"lien,omitempty"`
	Seance int64   `json:"seance,omitempty"`
	Lue    bool    `json:"lue"`
}

// NouvelleNotif : ce qu'on dépose dans la cloche, pour chacun des destinataires.
type NouvelleNotif struct {
	Genre, Titre, Texte, Lien string
	Seance                    int64
}

// Notifier dépose la même notification pour chaque destinataire (les doublons sont ignorés) et
// renvoie ce qui a été créé.
func (s *Store) Notifier(n NouvelleNotif, pour ...int64) ([]Notification, error) {
	if strings.TrimSpace(n.Titre) == "" || n.Genre == "" {
		return nil, invalide("Notification sans titre.")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	t, vus, out := maintenant(), map[int64]bool{}, []Notification{}
	for _, p := range pour {
		if vus[p] || p < 0 {
			continue
		}
		vus[p] = true
		res, err := tx.Exec(`INSERT INTO notifications (cree, genre, pour, titre, texte, lien, seance) VALUES (?,?,?,?,?,?,?)`,
			t, n.Genre, p, n.Titre, n.Texte, n.Lien, n.Seance)
		if err != nil {
			return nil, err
		}
		id, _ := res.LastInsertId()
		if _, err := tx.Exec(`DELETE FROM notifications WHERE pour=? AND id NOT IN (SELECT id FROM notifications WHERE pour=? ORDER BY id DESC LIMIT ?)`,
			p, p, gardeNotifications); err != nil {
			return nil, err
		}
		out = append(out, Notification{ID: id, Cree: t, Genre: n.Genre, Pour: p, Titre: n.Titre, Texte: n.Texte, Lien: n.Lien, Seance: n.Seance})
	}
	return out, tx.Commit()
}

// Notifications : celles de ces destinataires, des plus récentes aux plus anciennes (au plus
// « max »), et combien ne sont pas lues.
func (s *Store) Notifications(max int, pour ...int64) ([]Notification, int, error) {
	out := []Notification{}
	if len(pour) == 0 {
		return out, 0, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	marques, args := dans(pour)
	rows, err := s.db.Query(`SELECT id, cree, genre, pour, titre, texte, lien, seance, lue FROM notifications WHERE pour IN (`+marques+`) ORDER BY id DESC LIMIT ?`,
		append(args, max)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	for rows.Next() {
		var n Notification
		var lue float64
		if err := rows.Scan(&n.ID, &n.Cree, &n.Genre, &n.Pour, &n.Titre, &n.Texte, &n.Lien, &n.Seance, &lue); err != nil {
			return nil, 0, err
		}
		n.Lue = lue > 0
		out = append(out, n)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	var nonLues int
	err = s.db.QueryRow(`SELECT COUNT(*) FROM notifications WHERE lue=0 AND pour IN (`+marques+`)`, args...).Scan(&nonLues)
	return out, nonLues, err
}

// MarquerLues : ces notifications (toutes celles des destinataires si ids est vide) sont lues. Une
// notification d'un autre destinataire n'est jamais touchée.
func (s *Store) MarquerLues(ids []int64, pour ...int64) error {
	if len(pour) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	marques, args := dans(pour)
	q := `UPDATE notifications SET lue=? WHERE lue=0 AND pour IN (` + marques + `)`
	args = append([]any{maintenant()}, args...)
	if len(ids) > 0 {
		m, a := dans(ids)
		q += ` AND id IN (` + m + `)`
		args = append(args, a...)
	}
	_, err := s.db.Exec(q, args...)
	return err
}

func dans(ids []int64) (string, []any) {
	m := make([]string, len(ids))
	a := make([]any, len(ids))
	for i, id := range ids {
		m[i], a[i] = "?", id
	}
	return strings.Join(m, ","), a
}

// SuiviSeance : ce que le veilleur a retenu d'une séance (sa date et son statut à la dernière
// visite, et la date du dernier rappel envoyé).
type SuiviSeance struct {
	Signature, Rappel string
	Connue            bool
}

func (s *Store) SuiviSeance(seance int64) (SuiviSeance, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var v SuiviSeance
	err := s.db.QueryRow(`SELECT signature, rappel FROM notif_seances WHERE seance=?`, seance).Scan(&v.Signature, &v.Rappel)
	if errors.Is(err, sql.ErrNoRows) {
		return v, nil
	}
	v.Connue = err == nil
	return v, err
}

func (s *Store) NoterSuiviSeance(seance int64, v SuiviSeance) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`INSERT OR REPLACE INTO notif_seances (seance, signature, rappel) VALUES (?,?,?)`, seance, v.Signature, v.Rappel)
	return err
}

// APousser : les notifications pas encore envoyées aux appareils, créées depuis « depuis » (temps
// Unix) ; elles passent aussitôt « envoyées », celles d'avant aussi, sans partir (le serveur était
// arrêté : trop tard pour sonner).
func (s *Store) APousser(depuis float64) ([]Notification, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.Query(`SELECT id, cree, genre, pour, titre, texte, lien, seance FROM notifications WHERE poussee=0 AND cree>=? ORDER BY id`, depuis)
	if err != nil {
		return nil, err
	}
	out := []Notification{}
	for rows.Next() {
		var n Notification
		if err := rows.Scan(&n.ID, &n.Cree, &n.Genre, &n.Pour, &n.Titre, &n.Texte, &n.Lien, &n.Seance); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, n)
	}
	rows.Close()
	if _, err := tx.Exec(`UPDATE notifications SET poussee=1 WHERE poussee=0`); err != nil {
		return nil, err
	}
	return out, tx.Commit()
}
