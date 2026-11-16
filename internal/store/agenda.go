package store

import (
	"encoding/json"
	"time"

	"github.com/Tchyze/cof_dm/internal/modele"
)

// MetaFuseau : le fuseau horaire de l'agenda des séances (nom IANA, « Europe/Paris » par défaut).
// Réglage de l'univers, synchronisé : les heures des séances y sont écrites.
const MetaFuseau = "fuseau"

// Fuseau : le fuseau horaire de l'univers.
func (s *Store) Fuseau() string {
	if v, ok := s.Meta(MetaFuseau); ok && v != "" {
		return v
	}
	return modele.FuseauParDefaut
}

// ReglerFuseau change le fuseau horaire de l'univers.
func (s *Store) ReglerFuseau(nom, auteur string) error {
	if _, err := modele.Fuseau(nom); err != nil {
		return invalide("%v", err)
	}
	if nom == modele.FuseauParDefaut {
		nom = ""
	}
	if err := s.SetMeta(MetaFuseau, nom); err != nil {
		return err
	}
	if nom == "" {
		nom = modele.FuseauParDefaut
	}
	s.Noter("agenda", "fuseau horaire : "+nom, auteur)
	return nil
}

// noterPrevus : une séance qui passe à « jouée » garde les PJ qui étaient prévus, c'est-à-dire les
// PJ actifs de sa campagne à ce moment (les présents, eux, se cochent à part). Rien n'est changé
// si elle les a déjà.
func noterPrevus(tx execer, avant string, e *Entite) error {
	if e.Type != "seance" || avant == "jouée" {
		return nil
	}
	if st, _ := e.Data["statut"].(string); st != "jouée" {
		return nil
	}
	if _, deja := e.Data["prevus"]; deja {
		return nil
	}
	ids := ids(e.Data["campagne"])
	if len(ids) == 0 {
		return nil
	}
	pjs, err := pjActifs(tx, ids[0])
	if err != nil || len(pjs) == 0 {
		return err
	}
	l := make([]any, len(pjs))
	for i, id := range pjs {
		l[i] = id
	}
	e.Data["prevus"] = l
	return nil
}

// pjActifs : les PJ non archivés d'une campagne dont le statut est « actif » ou vide.
func pjActifs(q execer, campagne int64) ([]int64, error) {
	rows, err := q.Query(`SELECT id, data FROM entites WHERE type='pj' AND archive_le IS NULL ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		var data string
		if err := rows.Scan(&id, &data); err != nil {
			return nil, err
		}
		var d map[string]any
		_ = json.Unmarshal([]byte(data), &d)
		if st, _ := d["statut"].(string); st != "" && st != "actif" {
			continue
		}
		for _, c := range ids(d["campagne"]) {
			if c == campagne {
				out = append(out, id)
				break
			}
		}
	}
	return out, rows.Err()
}

// PJActifs : les PJ actifs d'une campagne (ceux qu'une séance prévue attend).
func (s *Store) PJActifs(campagne int64) ([]int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return pjActifs(s.db, campagne)
}

// Repondre range la réponse d'un joueur (pour son PJ) à une séance : « présent », « peut-être »,
// « absent », ou "" pour la retirer. Lue et écrite sous le même verrou : deux joueurs qui répondent
// en même temps gardent chacun la sienne. La date de modification de la séance ne bouge pas (un MJ
// qui la modifie au même moment n'en est pas gêné) ; la synchronisation, elle, voit le changement.
func (s *Store) Repondre(seance, pj int64, reponse, auteur string) (Entite, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return Entite{}, err
	}
	defer func() { _ = tx.Rollback() }()
	e, err := lire(tx, seance)
	if err != nil {
		return Entite{}, err
	}
	if e.Type != "seance" {
		return Entite{}, ErrIntrouvable
	}
	p, err := lire(tx, pj)
	if err != nil || p.Type != "pj" {
		return Entite{}, invalide("PJ introuvable.")
	}
	var l []any
	for _, x := range modele.LireReponses(e.Data["reponses"]) {
		if x.PJ != pj {
			l = append(l, map[string]any{"pj": float64(x.PJ), "reponse": x.Reponse, "le": x.Le})
		}
	}
	if reponse != "" {
		l = append(l, map[string]any{"pj": float64(pj), "reponse": reponse, "le": float64(time.Now().Unix())})
	}
	data := map[string]any{}
	for k, v := range e.Data {
		data[k] = v
	}
	delete(data, "reponses")
	if len(l) > 0 {
		data["reponses"] = l
	}
	t, _ := modele.TypeDe(e.Type)
	if e.Data, err = modele.Normaliser(t, data, s.cal, accesFiches{tx}); err != nil {
		return Entite{}, erreurChamps(err)
	}
	dj, _ := json.Marshal(e.Data)
	if _, err := tx.Exec(`UPDATE entites SET data=? WHERE id=?`, string(dj), seance); err != nil {
		return Entite{}, err
	}
	if err := indexer(tx, e); err != nil {
		return Entite{}, err
	}
	detail := p.Nom + " : " + reponse
	if reponse == "" {
		detail = p.Nom + " : réponse retirée"
	}
	noter(tx, "réponse", seance, e.Nom, detail, auteur)
	if err := tx.Commit(); err != nil {
		return Entite{}, err
	}
	return lire(s.db, seance)
}
