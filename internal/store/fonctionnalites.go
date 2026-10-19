package store

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/Tchyze/cof_dm/internal/modele"
)

// MetaFonctions : les fonctionnalités activées de l'univers ({"des": true, …}), réglage de
// l'univers qui voyage avec la synchronisation. Une clé absente prend la valeur par défaut.
const MetaFonctions = "fonctionnalites"

// Fonctionnalites renvoie l'état de chaque fonctionnalité de l'univers.
func (s *Store) Fonctionnalites() map[string]bool {
	v, _ := s.Meta(MetaFonctions)
	return etatFonctions(v)
}

// Active : la fonctionnalité est activée dans cet univers.
func (s *Store) Active(cle string) bool { return s.Fonctionnalites()[cle] }

func etatFonctions(brut string) map[string]bool {
	choix := map[string]bool{}
	if brut != "" {
		_ = json.Unmarshal([]byte(brut), &choix)
	}
	out := map[string]bool{}
	for _, f := range modele.Fonctionnalites {
		v, ok := choix[f.Cle]
		if !ok {
			v = f.Defaut
		}
		out[f.Cle] = v
	}
	return out
}

// lireFonctions valide un réglage reçu : des clés connues, des booléens.
func lireFonctions(brut []byte) (map[string]bool, error) {
	var choix map[string]bool
	if err := json.Unmarshal(brut, &choix); err != nil {
		return nil, invalide("fonctionnalités : un objet {\"clé\": true|false} est attendu")
	}
	for k := range choix {
		if _, ok := modele.FonctionnaliteDe(k); !ok {
			return nil, invalide("fonctionnalité inconnue %q", k)
		}
	}
	return choix, nil
}

// ReglerFonctionnalites active ou coupe des fonctionnalités ; les autres gardent leur état. Rien
// n'est effacé : les données d'une fonctionnalité coupée restent dans l'univers.
func (s *Store) ReglerFonctionnalites(choix map[string]bool, auteur string) (map[string]bool, error) {
	brut, _ := json.Marshal(choix)
	if _, err := lireFonctions(brut); err != nil {
		return nil, err
	}
	etat := s.Fonctionnalites()
	var changes []string
	for k, v := range choix {
		if etat[k] != v {
			f, _ := modele.FonctionnaliteDe(k)
			mot := "coupée"
			if v {
				mot = "activée"
			}
			changes = append(changes, f.Nom+" "+mot)
		}
		etat[k] = v
	}
	b, _ := json.Marshal(etat) // clés triées : la même valeur partout
	if err := s.SetMeta(MetaFonctions, string(b)); err != nil {
		return nil, err
	}
	if len(changes) > 0 {
		sort.Strings(changes)
		s.Noter("fonctionnalités", strings.Join(changes, ", "), auteur)
	}
	return etat, nil
}

// fixerFonctionnalites : un univers d'avant les fonctionnalités optionnelles garde ce dont il se
// sert déjà. Une fonctionnalité coupée par défaut reste active s'il en a les données (des aides de
// jeu, des feuilles de personnage : alors aussi la page des joueurs). Un univers vide part des
// valeurs par défaut.
func fixerFonctionnalites(q execer) error {
	var v string
	if q.QueryRow(`SELECT valeur FROM meta WHERE cle=?`, MetaFonctions).Scan(&v) == nil {
		return nil
	}
	compte := func(req string) int {
		var n int
		_ = q.QueryRow(req).Scan(&n)
		return n
	}
	if compte(`SELECT COUNT(*) FROM entites`) == 0 {
		return nil
	}
	etat := etatFonctions("")
	if compte(`SELECT COUNT(*) FROM entites WHERE type='aide'`) > 0 {
		etat["aides"] = true
	}
	if compte(`SELECT COUNT(*) FROM feuilles`) > 0 {
		etat["feuilles"], etat["page_joueurs"] = true, true
	}
	b, _ := json.Marshal(etat)
	if _, err := q.Exec(`INSERT INTO meta (cle, valeur) VALUES (?, ?)`, MetaFonctions, string(b)); err != nil {
		return fmt.Errorf("fonctionnalités : %w", err)
	}
	return nil
}
