package fiches

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Tchyze/cof_dm/internal/reference"
	"github.com/Tchyze/cof_dm/internal/regles"
)

// Montée de niveau. Le niveau reste sur la fiche du PJ ; la feuille garde l'historique des montées
// (Niveaux), assez précis pour défaire la dernière : PV gagnés, rangs pris, points gardés avant.

// Montee : une ligne de l'historique des niveaux.
type Montee struct {
	Niveau int          `json:"niveau"` // le niveau atteint
	Quand  float64      `json:"quand"`
	Pv     int          `json:"pv"`               // PV max gagnés
	Jet    string       `json:"jet,omitempty"`    // le détail des PV (« d8[5] + 2 (CON) ») ; vide : PV saisis
	Pm     int          `json:"pm,omitempty"`     // PM max gagnés
	Rangs  []RangPris   `json:"rangs,omitempty"`  // rangs pris dans les voies de la feuille
	Points int          `json:"points,omitempty"` // points de capacité dépensés
	Reste  int          `json:"reste,omitempty"`  // points de capacité gardés avant cette montée
	Etapes []EtapeFaite `json:"etapes,omitempty"` // ce que le joueur a noté aux étapes en plus
}

// EtapeFaite : une étape en plus de la montée et ce que le joueur y a noté (ajouté aux capacités).
type EtapeFaite struct {
	Cle     string `json:"cle"`
	Libelle string `json:"libelle"`
	Texte   string `json:"texte"`
}

// ChoixMontee : ce que le joueur (ou le MJ) choisit en passant un niveau : les dés de vie lancés en
// vrai (Des), ou les PV gagnés quand les règles n'en donnent pas la formule (Pv), les rangs pris,
// et ce qu'il note aux étapes en plus (par clé d'étape).
type ChoixMontee struct {
	Des    []int             `json:"des"`
	Pv     *int              `json:"pv"`
	Rangs  []ChoixRang       `json:"rangs"`
	Etapes map[string]string `json:"etapes"`
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

// pvGagnes : les PV d'une montée au niveau donné, selon la formule des règles et les dés saisis,
// avec leur détail ; sans formule, ceux saisis.
func (f *Feuille) pvGagnes(m *regles.Montee, niveau int, c ChoixMontee) (int, string, error) {
	if !m.Formule() {
		if c.Pv == nil || *c.Pv < 0 || *c.Pv > 999 {
			return 0, "", invalide("PV gagnés : entre 0 et 999.")
		}
		return *c.Pv, "", nil
	}
	if len(c.Des) != m.NbDes() {
		return 0, "", invalide("Dés de vie : %d résultat%s attendu%s.", m.NbDes(), pluriel(m.NbDes()), pluriel(m.NbDes()))
	}
	faces := 0
	if m.NbDes() > 0 {
		if !reDe.MatchString(f.Dv) {
			return 0, "", invalide("La feuille n'a pas de dé de vie : choisissez-le d'abord.")
		}
		faces, _ = strconv.Atoi(f.Dv[1:])
	}
	total, termes, de := 0, []string{}, 0
	for _, t := range m.Pv {
		switch t {
		case "dv":
			v := c.Des[de]
			de++
			if v < 1 || v > faces {
				return 0, "", invalide("Dé de vie (%s) : entre 1 et %d.", f.Dv, faces)
			}
			total += v
			termes = append(termes, fmt.Sprintf("%s[%d]", f.Dv, v))
		case "con":
			total += f.Carac.CON
			termes = append(termes, fmt.Sprintf("%+d (CON)", f.Carac.CON))
		case "niveau":
			total += niveau
			termes = append(termes, fmt.Sprintf("%+d (niveau)", niveau))
		}
	}
	if m.PvFixe != 0 {
		total += m.PvFixe
		termes = append(termes, fmt.Sprintf("%+d", m.PvFixe))
	}
	detail := strings.TrimPrefix(strings.Join(termes, " "), "+")
	detail = strings.NewReplacer(" +", " + ", " -", " − ").Replace(detail)
	return max(0, total, m.PvMin), detail, nil
}

// ligneEtape : la ligne qu'une étape ajoute aux capacités de la feuille.
func ligneEtape(e EtapeFaite, niveau int) string {
	return fmt.Sprintf("%s (niveau %d) : %s", e.Libelle, niveau, e.Texte)
}

// Monter fait passer la feuille au niveau donné (le nouveau), d'un PJ de ce profil : PV gagnés (dés
// saisis ou PV saisis), PM du profil, rangs choisis dans ses voies au prix que disent les règles,
// étapes en plus. Elle renvoie la ligne ajoutée à l'historique ; en cas d'erreur, la feuille n'a
// pas changé.
func (f *Feuille) Monter(niveau int, c ChoixMontee, profil string, rg *regles.Feuille, quand float64) (*Montee, error) {
	if rg == nil {
		rg = &regles.Feuille{}
	}
	m := rg.Montee
	pv, jet, err := f.pvGagnes(m, niveau, c)
	if err != nil {
		return nil, err
	}
	ligne := &Montee{Niveau: niveau, Quand: quand, Pv: pv, Jet: jet, Pm: m.GainPmDe(profil), Reste: f.PointsCapacite}
	for _, e := range m.EtapesDu(niveau) {
		t := strings.Join(strings.Fields(c.Etapes[e.Cle]), " ")
		if t == "" || len([]rune(t)) > 200 {
			return nil, invalide("%s : à noter (200 caractères au plus).", e.Libelle)
		}
		ligne.Etapes = append(ligne.Etapes, EtapeFaite{Cle: e.Cle, Libelle: e.Libelle, Texte: t})
	}
	vues := map[int]bool{}
	cout := 0
	for _, ch := range c.Rangs {
		if ch.Voie < 0 || ch.Voie >= len(f.Voies) || vues[ch.Voie] {
			return nil, invalide("Rangs : une voie inconnue ou choisie deux fois.")
		}
		vues[ch.Voie] = true
		v := f.Voies[ch.Voie]
		if ch.Rang <= v.Rang {
			return nil, invalide("%s : déjà au rang %d.", v.Nom, v.Rang)
		}
		if haut := rangMaxMontee(rg); ch.Rang > haut {
			return nil, invalide("%s : rang %d au plus.", v.Nom, haut)
		}
		pris := RangPris{Voie: v.Nom, Ref: v.Ref, De: v.Rang, A: ch.Rang}
		for r := v.Rang + 1; r <= ch.Rang; r++ {
			if n := m.NiveauPour(r); n > niveau {
				return nil, invalide("%s : le rang %d demande le niveau %d.", v.Nom, r, n)
			}
			cout += m.Cout(v.Origine, r)
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
	for _, ch := range c.Rangs {
		v := &f.Voies[ch.Voie]
		for len(v.Capacites) < ch.Rang {
			v.Capacites = append(v.Capacites, "")
		}
		v.Rang = ch.Rang
	}
	f.Pv += pv
	if f.PvActuel != nil {
		*f.PvActuel += pv
	}
	f.PmMax += ligne.Pm
	f.Pm += ligne.Pm
	for _, e := range ligne.Etapes {
		if f.Capacites != "" {
			f.Capacites += "\n"
		}
		f.Capacites += ligneEtape(e, niveau)
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
	if f.Pv-m.Pv < 1 || f.PmMax-m.Pm < 0 {
		return nil, invalide("Les PV ou les PM max ont changé depuis la montée : corrigez à la main.")
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
	f.PmMax -= m.Pm
	f.Pm = max(0, f.Pm-m.Pm)
	// les lignes que ses étapes ont ajoutées aux capacités, si elles y sont encore
	for i := len(m.Etapes) - 1; i >= 0; i-- {
		l := ligneEtape(m.Etapes[i], m.Niveau)
		if k := strings.LastIndex(f.Capacites, l); k >= 0 {
			f.Capacites = strings.TrimRight(f.Capacites[:k], "\n") + f.Capacites[k+len(l):]
		}
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
		if m.Niveau < 1 || m.Pv < 0 || m.Pm < 0 || m.Points < 0 || m.Reste < 0 || len(m.Jet) > 200 || len(m.Rangs) > 40 || len(m.Etapes) > 12 {
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
