package fiches

import (
	"strings"

	"github.com/Tchyze/cof_dm/internal/reference"
	"github.com/Tchyze/cof_dm/internal/regles"
)

// Montée de niveau. Le niveau reste sur la fiche du PJ ; la feuille garde l'historique des montées
// (Niveaux), assez précis pour défaire la dernière : PV gagnés, rangs pris, points gardés avant.

// Montee : une ligne de l'historique des niveaux.
type Montee struct {
	Niveau int        `json:"niveau"` // le niveau atteint
	Quand  float64    `json:"quand"`
	Pv     int        `json:"pv"`               // PV max gagnés
	Jet    string     `json:"jet,omitempty"`    // le détail du jet (« 1d8[5] + 2 ») ; vide : PV saisis
	Cache  bool       `json:"cache,omitempty"`  // jet caché du MJ : le joueur n'en voit pas le détail
	Rangs  []RangPris `json:"rangs,omitempty"`  // rangs pris dans les voies de la feuille
	Points int        `json:"points,omitempty"` // points de capacité dépensés
	Reste  int        `json:"reste,omitempty"`  // points de capacité gardés avant cette montée
}

// RangPris : une voie passée du rang De au rang A, avec les capacités débloquées.
type RangPris struct {
	Voie      string   `json:"voie"`
	Ref       string   `json:"ref,omitempty"`
	De        int      `json:"de"`
	A         int      `json:"a"`
	Capacites []string `json:"capacites,omitempty"`
}

// ChoixRang : un rang demandé à la montée : la voie (son rang dans la liste de la feuille) et le
// rang qu'elle atteint.
type ChoixRang struct {
	Voie int `json:"voie"`
	Rang int `json:"rang"`
}

// rangMaxMontee : le rang qu'une voie peut atteindre (règles, sinon une borne large).
func rangMaxMontee(rg *regles.Feuille) int {
	if rg.RangMax > 0 {
		return rg.RangMax
	}
	return 20
}

// Monter fait passer la feuille au niveau donné (le nouveau) : pv gagnés (lancés ou saisis, jet
// en donne le détail), rangs choisis dans ses voies, au prix que disent les règles. Elle renvoie la
// ligne ajoutée à l'historique ; en cas d'erreur, la feuille n'a pas changé.
func (f *Feuille) Monter(niveau, pv int, jet string, cache bool, choix []ChoixRang, rg *regles.Feuille, quand float64) (*Montee, error) {
	if rg == nil {
		rg = &regles.Feuille{}
	}
	m := rg.Montee
	if pv < 0 || pv > 999 {
		return nil, invalide("PV gagnés : entre 0 et 999.")
	}
	ligne := &Montee{Niveau: niveau, Quand: quand, Pv: pv, Jet: jet, Cache: cache && jet != "", Reste: f.PointsCapacite}
	vues := map[int]bool{}
	cout := 0
	for _, c := range choix {
		if c.Voie < 0 || c.Voie >= len(f.Voies) || vues[c.Voie] {
			return nil, invalide("Rangs : une voie inconnue ou choisie deux fois.")
		}
		vues[c.Voie] = true
		v := f.Voies[c.Voie]
		if c.Rang <= v.Rang {
			return nil, invalide("%s : déjà au rang %d.", v.Nom, v.Rang)
		}
		if haut := rangMaxMontee(rg); c.Rang > haut {
			return nil, invalide("%s : rang %d au plus.", v.Nom, haut)
		}
		pris := RangPris{Voie: v.Nom, Ref: v.Ref, De: v.Rang, A: c.Rang}
		for r := v.Rang + 1; r <= c.Rang; r++ {
			if n := m.NiveauPour(r); n > niveau {
				return nil, invalide("%s : le rang %d demande le niveau %d.", v.Nom, r, n)
			}
			cout += m.Cout(r)
			if r <= len(v.Capacites) && strings.TrimSpace(v.Capacites[r-1]) != "" {
				pris.Capacites = append(pris.Capacites, v.Capacites[r-1])
			}
		}
		ligne.Rangs = append(ligne.Rangs, pris)
	}
	if m != nil && m.Points > 0 {
		dispo := m.Points + f.PointsCapacite
		if cout > dispo {
			return nil, invalide("Rangs : %d point%s de capacité, il y en a %d.", cout, pluriel(cout), dispo)
		}
		ligne.Points = cout
		f.PointsCapacite = 0
		if m.Report {
			f.PointsCapacite = dispo - cout
		}
	}
	for _, c := range choix {
		v := &f.Voies[c.Voie]
		for len(v.Capacites) < c.Rang {
			v.Capacites = append(v.Capacites, "")
		}
		v.Rang = c.Rang
	}
	f.Pv += pv
	if f.PvActuel != nil {
		*f.PvActuel += pv
	}
	f.Niveaux = append(f.Niveaux, *ligne)
	f.NiveauAccorde = 0
	return ligne, nil
}

// Annuler défait la dernière montée, si la fiche est encore au niveau qu'elle avait atteint et
// si ses voies n'ont pas bougé depuis. Elle renvoie la ligne retirée de l'historique.
func (f *Feuille) Annuler(niveau int) (*Montee, error) {
	if len(f.Niveaux) == 0 {
		return nil, invalide("Aucune montée de niveau à annuler.")
	}
	m := f.Niveaux[len(f.Niveaux)-1]
	if m.Niveau != niveau {
		return nil, invalide("La dernière montée menait au niveau %d, la fiche est au niveau %d : corrigez à la main.", m.Niveau, niveau)
	}
	if f.Pv-m.Pv < 1 {
		return nil, invalide("Les PV max ont changé depuis la montée : corrigez à la main.")
	}
	idx := make([]int, len(m.Rangs))
	for i, r := range m.Rangs {
		idx[i] = -1
		for j, v := range f.Voies {
			if (r.Ref != "" && v.Ref == r.Ref) || (r.Ref == "" && reference.Cle(v.Nom) == reference.Cle(r.Voie)) {
				idx[i] = j
				break
			}
		}
		if idx[i] < 0 || f.Voies[idx[i]].Rang != r.A {
			return nil, invalide("%s a changé depuis la montée : corrigez à la main.", r.Voie)
		}
	}
	for i, r := range m.Rangs {
		f.Voies[idx[i]].Rang = r.De
	}
	f.Pv -= m.Pv
	if f.PvActuel != nil {
		v := max(0, *f.PvActuel-m.Pv)
		f.PvActuel = &v
	}
	f.PointsCapacite = m.Reste
	f.Niveaux = f.Niveaux[:len(f.Niveaux)-1]
	if len(f.Niveaux) == 0 {
		f.Niveaux = nil
	}
	return &m, nil
}

// verifierNiveaux : l'historique et les points gardés d'une feuille reçue.
func (f *Feuille) verifierNiveaux() error {
	if f.PointsCapacite < 0 || f.PointsCapacite > 999 {
		return invalide("points_capacite : entre 0 et 999.")
	}
	if f.NiveauAccorde < 0 || f.NiveauAccorde > 999 {
		return invalide("niveau_accorde : entre 0 et 999.")
	}
	if len(f.Niveaux) > 100 {
		return invalide("niveaux : cent montées au plus.")
	}
	for i, m := range f.Niveaux {
		if m.Niveau < 1 || m.Pv < 0 || m.Points < 0 || m.Reste < 0 || len(m.Jet) > 200 || len(m.Rangs) > 40 {
			return invalide("niveaux.%d : invalide.", i)
		}
		for _, r := range m.Rangs {
			if r.De < 0 || r.A < r.De || len(r.Voie) > 200 || len(r.Capacites) > 20 {
				return invalide("niveaux.%d.rangs : invalide.", i)
			}
		}
	}
	if len(f.Niveaux) == 0 {
		f.Niveaux = nil
	}
	return nil
}

func pluriel(n int) string {
	if n > 1 {
		return "s"
	}
	return ""
}
