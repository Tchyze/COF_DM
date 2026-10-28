package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/Tchyze/cof_dm/internal/fiches"
	"github.com/Tchyze/cof_dm/internal/modele"
)

// monstresParNom : les monstres du bestiaire de l'univers, par nom plié.
func monstresParNom(q execer) (map[string]int64, error) {
	rows, err := q.Query(`SELECT id, data FROM documents WHERE coll='monstres' ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var id int64
		var data string
		var m struct {
			Nom string `json:"nom"`
		}
		if rows.Scan(&id, &data) == nil && json.Unmarshal([]byte(data), &m) == nil && m.Nom != "" {
			out[Plier(m.Nom)] = id
		}
	}
	return out, rows.Err()
}

// delierTracker retire d'une fiche ce qui la liait au tracker externe.
//
// Toujours : la fiche de personnage d'un PJ dans le tracker et les combats du tracker rattachés à
// une séance (version 4), retirés sans bruit — la feuille de personnage et les combats sont dans
// l'univers.
//
// avantBestiaire (jusqu'à la version 2) : le profil de combat d'un PNJ et les créatures d'une
// rencontre portaient des identifiants du tracker, qui ne désignent rien dans le bestiaire de
// l'univers. Les créatures sont rattachées au monstre de l'univers qui porte leur nom ; ce qui ne
// se retrouve pas est retiré et signalé.
func delierTracker(e *Entite, bestiaire map[string]int64, avantBestiaire bool) (perdus []string) {
	switch e.Type {
	case "pj":
		delete(e.Data, "fiche_tracker")
	case "seance":
		delete(e.Data, "combats")
	case "rencontre":
		delete(e.Data, "tracker_rencontre")
	}
	if !avantBestiaire {
		return nil
	}
	switch e.Type {
	case "pnj":
		if v, ok := e.Data["profil_combat"]; ok {
			perdus = append(perdus, fmt.Sprintf("- Profil de combat : fiche n°%v de l'ancien tracker", v))
			delete(e.Data, "profil_combat")
		}
	case "rencontre":
		liste, ok := e.Data["adversaires"].([]any)
		if !ok {
			return nil
		}
		gardes := []any{}
		for _, x := range liste {
			a, _ := x.(map[string]any)
			nom, _ := a["nom"].(string)
			if id, ok := bestiaire[Plier(nom)]; ok && nom != "" {
				a["monstre"] = float64(id)
				gardes = append(gardes, a)
				continue
			}
			perdus = append(perdus, fmt.Sprintf("- Créature : %v × %s (ancien tracker)", a["qte"], nom))
		}
		if len(gardes) > 0 {
			e.Data["adversaires"] = gardes
		} else {
			delete(e.Data, "adversaires")
		}
	}
	return perdus
}

// versionDonnees augmente quand des champs changent de forme (texte devenu liste, champ supprimé…).
// Version 3 : le profil de combat d'un PNJ et les créatures d'une rencontre désignent le bestiaire de
// l'univers, et non plus des fiches du tracker externe. Version 4 : les derniers champs du pont vers
// le tracker sont retirés (fiche de personnage d'un PJ, combats d'une séance). Version 5 : la
// feuille de personnage range son équipement dans un inventaire ; le texte « equipement », l'armure
// et le bouclier en deviennent les lignes (reprendreFeuilles).
const versionDonnees = "5"

// reparer remet les fiches au format du modèle après une mise à jour de l'application. Rien n'est
// perdu : une valeur qui n'a plus sa place est recopiée dans les notes du MJ de la fiche.
func (s *Store) reparer() error {
	v, _ := s.Meta("version_donnees")
	if v == versionDonnees {
		return nil
	}
	ancienne, _ := strconv.Atoi(v)
	tout, err := s.Toutes()
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	bestiaire, err := monstresParNom(tx)
	if err != nil {
		return err
	}
	for _, e := range tout {
		t, ok := modele.TypeDe(e.Type)
		if !ok {
			continue
		}
		garde := map[string]any{}
		avant, _ := json.Marshal(e.Data)
		perdus := delierTracker(&e, bestiaire, ancienne < 3)
		apres, _ := json.Marshal(e.Data)
		rattache := string(avant) != string(apres)
		cles := make([]string, 0, len(e.Data))
		for k := range e.Data {
			cles = append(cles, k)
		}
		sort.Strings(cles)
		for _, k := range cles {
			v := e.Data[k]
			if _, err := modele.Normaliser(t, map[string]any{k: v}, s.cal, accesFiches{tx}); err != nil {
				nom := k
				if c, ok := t.Champ(k); ok {
					nom = c.Nom
				}
				b, _ := json.Marshal(v)
				texte := string(b)
				if str, ok := v.(string); ok {
					texte = str
				}
				perdus = append(perdus, fmt.Sprintf("- %s : %s", nom, texte))
				continue
			}
			garde[k] = v
		}
		if len(perdus) == 0 && !rattache {
			continue
		}
		cible := ""
		for _, c := range t.Champs {
			if c.Genre == modele.Long && c.Secret {
				cible = c.Cle // notes ou secrets du MJ
			}
		}
		if cible != "" && len(perdus) > 0 {
			ancien, _ := garde[cible].(string)
			garde[cible] = strings.TrimSpace(ancien + "\n\n**Ancien format (mise à jour de COF DM)**\n" + strings.Join(perdus, "\n"))
		}
		dj, _ := json.Marshal(garde)
		if _, err := tx.Exec(`UPDATE entites SET data=? WHERE id=?`, string(dj), e.ID); err != nil {
			return err
		}
		e.Data = garde
		if err := indexer(tx, e); err != nil {
			return err
		}
		if len(perdus) > 0 {
			noter(tx, "mise à jour", e.ID, e.Nom, "champs recopiés dans les notes : "+strings.Join(perdus, " ; "), "cof-dm")
		}
	}
	if ancienne < 5 {
		if err := reprendreFeuilles(tx); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`INSERT OR REPLACE INTO meta (cle, valeur) VALUES ('version_donnees', ?)`, versionDonnees); err != nil {
		return err
	}
	return tx.Commit()
}

// reprendreFeuilles range au format de l'inventaire les feuilles qui ont encore leur ancien
// équipement (fiches.RelireFeuille en fait les lignes).
func reprendreFeuilles(tx *sql.Tx) error {
	rows, err := tx.Query(`SELECT f.entite, f.data, e.data FROM feuilles f JOIN entites e ON e.id = f.entite`)
	if err != nil {
		return err
	}
	type feuille struct {
		id   int64
		data string
	}
	var aRanger []feuille
	for rows.Next() {
		var id int64
		var data, fiche string
		if err := rows.Scan(&id, &data, &fiche); err != nil {
			rows.Close()
			return err
		}
		var champs map[string]json.RawMessage
		if json.Unmarshal([]byte(data), &champs) != nil {
			continue
		}
		_, eq := champs["equipement"]
		_, ar := champs["armure"]
		_, bo := champs["bouclier"]
		if !eq && !ar && !bo {
			continue
		}
		var e struct {
			Niveau float64 `json:"niveau"`
		}
		_ = json.Unmarshal([]byte(fiche), &e)
		f, err := fiches.RelireFeuille([]byte(data), max(1, int(e.Niveau)))
		if err != nil {
			continue // illisible : laissée telle quelle
		}
		aRanger = append(aRanger, feuille{id, string(f.Stockage())})
	}
	rows.Close()
	for _, f := range aRanger {
		if _, err := tx.Exec(`UPDATE feuilles SET data=? WHERE entite=?`, f.data, f.id); err != nil {
			return err
		}
	}
	return nil
}
