package modele

import (
	"fmt"
	"strings"
)

// Mois du calendrier du monde. Des « jours complémentaires » sont un mois comme les autres, plus court.
type Mois struct {
	Nom   string `json:"nom"`
	Jours int    `json:"jours"`
}

// Calendrier du monde : mois, semaine et nom de l'ère.
type Calendrier struct {
	Ere     string   `json:"ere"` // « de la Couronne » : « 1 Vendémiaire 1216 de la Couronne »
	Mois    []Mois   `json:"mois"`
	Semaine []string `json:"semaine"` // noms des jours ; la semaine court sans interruption d'une année à l'autre
}

// CalendrierParDefaut : douze mois de trente jours (noms du calendrier républicain), une décade de dix
// jours et cinq jours de fête en fin d'année. Tout se modifie dans les paramètres.
func CalendrierParDefaut() Calendrier {
	noms := []string{"Vendémiaire", "Brumaire", "Frimaire", "Nivôse", "Pluviôse", "Ventôse",
		"Germinal", "Floréal", "Prairial", "Messidor", "Thermidor", "Fructidor"}
	c := Calendrier{Ere: "", Semaine: []string{"primidi", "duodi", "tridi", "quartidi", "quintidi", "sextidi", "septidi", "octidi", "nonidi", "décadi"}}
	for _, n := range noms {
		c.Mois = append(c.Mois, Mois{n, 30})
	}
	c.Mois = append(c.Mois, Mois{"Jours complémentaires", 5})
	return c
}

// Verifier contrôle un calendrier saisi.
func (c Calendrier) Verifier() error {
	if len(c.Mois) == 0 || len(c.Mois) > 40 {
		return fmt.Errorf("le calendrier doit avoir entre 1 et 40 mois")
	}
	for i, m := range c.Mois {
		if strings.TrimSpace(m.Nom) == "" {
			return fmt.Errorf("le mois %d n'a pas de nom", i+1)
		}
		if m.Jours < 1 || m.Jours > 400 {
			return fmt.Errorf("%s : nombre de jours invalide (%d)", m.Nom, m.Jours)
		}
	}
	if len(c.Semaine) > 20 {
		return fmt.Errorf("une semaine de plus de 20 jours ? Vérifiez la liste")
	}
	return nil
}

func (c Calendrier) joursAn() int {
	n := 0
	for _, m := range c.Mois {
		n += m.Jours
	}
	return n
}

// DateJeu est une date du monde. Mois et jour commencent à 1.
type DateJeu struct {
	An   int `json:"an"`
	Mois int `json:"mois"`
	Jour int `json:"jour"`
}

// Verifier contrôle une date par rapport au calendrier.
func (c Calendrier) VerifierDate(d DateJeu) error {
	if d.Mois < 1 || d.Mois > len(c.Mois) {
		return fmt.Errorf("mois %d inexistant (1 à %d)", d.Mois, len(c.Mois))
	}
	if n := c.Mois[d.Mois-1].Jours; d.Jour < 1 || d.Jour > n {
		return fmt.Errorf("%s n'a que %d jours", c.Mois[d.Mois-1].Nom, n)
	}
	return nil
}

// Absolu numérote les jours depuis l'an 0, pour trier et calculer des écarts.
func (c Calendrier) Absolu(d DateJeu) int {
	n := d.An * c.joursAn()
	for i := 0; i < d.Mois-1 && i < len(c.Mois); i++ {
		n += c.Mois[i].Jours
	}
	return n + d.Jour - 1
}

// Texte écrit une date : « quintidi 5 Vendémiaire 1216 de la Couronne ».
func (c Calendrier) Texte(d DateJeu) string {
	if c.VerifierDate(d) != nil {
		return fmt.Sprintf("%d/%d/%d", d.Jour, d.Mois, d.An)
	}
	s := fmt.Sprintf("%d %s %d", d.Jour, c.Mois[d.Mois-1].Nom, d.An)
	if c.Ere != "" {
		s += " " + c.Ere
	}
	return s
}
