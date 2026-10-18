package store

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Media est une image rattachée à une fiche (portrait, carte, illustration). Son contenu est rangé
// dans l'univers ; il n'accompagne la fiche que dans un export.
type Media struct {
	ID          int64   `json:"id"`
	Entite      int64   `json:"entite"`
	Fichier     string  `json:"fichier"` // servi sous /medias/<fichier>
	NomOriginal string  `json:"nom_original,omitempty"`
	Legende     string  `json:"legende,omitempty"`
	Principal   bool    `json:"principal"`
	Cree        float64 `json:"cree"`
	Contenu     []byte  `json:"contenu,omitempty"`
}

var reNomMedia = regexp.MustCompile(`^[0-9a-f]{20}\.(png|jpg|webp|gif)$`)

// NomMediaValide : le nom sous lequel une image est rangée et servie (/medias/<fichier>).
func NomMediaValide(fichier string) bool { return reNomMedia.MatchString(fichier) }

// AjouterMedia range une image dans l'univers. La première devient l'image principale de la fiche.
func (s *Store) AjouterMedia(entite int64, fichier, nomOriginal string, contenu []byte, auteur string) (Media, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, err := lire(s.db, entite)
	if err != nil {
		return Media{}, err
	}
	var n int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM medias WHERE entite=?`, entite).Scan(&n)
	m := Media{Entite: entite, Fichier: fichier, NomOriginal: nomOriginal, Principal: n == 0, Cree: maintenant()}
	if m.ID, err = prochainID(s.db, "medias"); err != nil {
		return Media{}, err
	}
	if _, err := s.db.Exec(`INSERT INTO medias (id, entite, fichier, nom_original, principal, cree, contenu) VALUES (?,?,?,?,?,?,?)`,
		m.ID, entite, fichier, nomOriginal, m.Principal, m.Cree, contenu); err != nil {
		return Media{}, err
	}
	noter(s.db, "image", entite, e.Nom, "ajout de "+nomOriginal, auteur)
	return m, nil
}

func (s *Store) Medias(entite int64) ([]Media, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT id, entite, fichier, nom_original, legende, principal, cree FROM medias WHERE entite=? ORDER BY principal DESC, id`, entite)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Media{}
	for rows.Next() {
		var m Media
		if err := rows.Scan(&m.ID, &m.Entite, &m.Fichier, &m.NomOriginal, &m.Legende, &m.Principal, &m.Cree); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ModifierMedia change la légende et / ou fait de l'image l'image principale de sa fiche.
func (s *Store) ModifierMedia(id int64, legende *string, principal bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var entite int64
	if err := s.db.QueryRow(`SELECT entite FROM medias WHERE id=?`, id).Scan(&entite); err != nil {
		return invalide("Image introuvable.")
	}
	if legende != nil {
		if _, err := s.db.Exec(`UPDATE medias SET legende=? WHERE id=?`, strings.TrimSpace(*legende), id); err != nil {
			return err
		}
	}
	if principal {
		if _, err := s.db.Exec(`UPDATE medias SET principal=(id=?) WHERE entite=?`, id, entite); err != nil {
			return err
		}
	}
	return nil
}

// ContenuMedia renvoie une image et la date de son ajout.
func (s *Store) ContenuMedia(fichier string) ([]byte, float64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var contenu []byte
	var cree float64
	err := s.db.QueryRow(`SELECT contenu, cree FROM medias WHERE fichier=?`, fichier).Scan(&contenu, &cree)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && len(contenu) == 0) {
		return nil, 0, ErrIntrouvable
	}
	return contenu, cree, err
}

// EntiteDuMedia renvoie la fiche à laquelle appartient une image.
func (s *Store) EntiteDuMedia(fichier string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var id int64
	if err := s.db.QueryRow(`SELECT entite FROM medias WHERE fichier=?`, fichier).Scan(&id); err != nil {
		return 0, ErrIntrouvable
	}
	return id, nil
}

// SupprimerMedia retire une image de l'univers.
func (s *Store) SupprimerMedia(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var entite int64
	var principal bool
	if err := s.db.QueryRow(`SELECT entite, principal FROM medias WHERE id=?`, id).Scan(&entite, &principal); err != nil {
		return invalide("Image introuvable.")
	}
	if _, err := s.db.Exec(`DELETE FROM medias WHERE id=?`, id); err != nil {
		return err
	}
	if principal {
		_, _ = s.db.Exec(`UPDATE medias SET principal=1 WHERE id=(SELECT MIN(id) FROM medias WHERE entite=?)`, entite)
	}
	return nil
}

// ImporterMedias range dans l'univers les images qui étaient des fichiers du dossier donné (données
// d'avant les fichiers d'univers). Renvoie le nombre d'images reprises et les fichiers introuvables.
func (s *Store) ImporterMedias(dossier string) (int, []string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT fichier FROM medias WHERE contenu IS NULL ORDER BY id`)
	if err != nil {
		return 0, nil, err
	}
	var fichiers []string
	for rows.Next() {
		var f string
		if rows.Scan(&f) == nil {
			fichiers = append(fichiers, f)
		}
	}
	rows.Close()
	n := 0
	var absents []string
	for _, f := range fichiers {
		b, err := os.ReadFile(filepath.Join(dossier, filepath.Base(f)))
		if err != nil || len(b) == 0 {
			absents = append(absents, f)
			continue
		}
		if _, err := s.db.Exec(`UPDATE medias SET contenu=? WHERE fichier=?`, b, f); err != nil {
			return n, absents, err
		}
		n++
	}
	return n, absents, nil
}
