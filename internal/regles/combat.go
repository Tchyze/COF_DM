package regles

import (
	"fmt"
	"strings"
)

// Combat : ce que le fichier de règles dit du combat. Le code garde le déroulement (initiative,
// tours, attaque contre la DEF, dégâts) ; les valeurs et les états viennent d'ici. Sans cette
// partie, le combat reste jouable, sans état prédéfini, critique, point de chance ni budget
// d'actions.
type Combat struct {
	// De : dé des attaques et des tests (0 : 20).
	De int `json:"de"`
	// EchecNaturel : résultat du dé qui fait toujours échouer (0 : aucun).
	EchecNaturel int `json:"echec_naturel"`
	// CritiqueDm : par combien un critique multiplie les dégâts (0 : pas de critique). Un critique
	// est aussi une réussite automatique ; il n'existe qu'avec le dé normal.
	CritiqueDm int `json:"critique_dm"`
	// Chance : ce qu'un point de chance ajoute à un jet déjà lancé (0 : pas de point de chance).
	Chance int `json:"chance"`
	// BudgetActions : chaque tour se limite à une action limitée, ou un mouvement et une attaque,
	// ou deux mouvements.
	BudgetActions bool         `json:"budget_actions"`
	Etats         []EtatCombat `json:"etats"`
}

// EtatCombat : un état que peut porter un combattant, et ses effets.
type EtatCombat struct {
	Nom          string `json:"nom"`
	Description  string `json:"description"`
	Att          int    `json:"att,omitempty"`          // modificateur aux attaques
	AttDistance  int    `json:"att_distance,omitempty"` // aux attaques à distance (remplace att)
	Def          int    `json:"def,omitempty"`
	Init         int    `json:"init,omitempty"`
	De           int    `json:"de,omitempty"`                   // dé qui remplace le dé normal
	Bloque       bool   `json:"bloque,omitempty"`               // aucune action possible
	CritiqueAuto bool   `json:"critique_automatique,omitempty"` // toute attaque contre lui est un critique
	UneAction    bool   `json:"une_action,omitempty"`           // une seule action par tour, pas d'action limitée
	// Fin automatique : « round » à la fin du premier round, « tour » au début du prochain tour du
	// combattant.
	Fin string `json:"fin,omitempty"`
	// Action : l'état se prend à son propre tour et coûte cette action (« A » attaque, « L »
	// limitée) : les postures défensives.
	Action string `json:"action,omitempty"`
}

func (c *Combat) normaliser() error {
	if c.De < 0 || c.De == 1 || c.De > 1000 || c.EchecNaturel < 0 || c.CritiqueDm < 0 || c.CritiqueDm > 10 || c.Chance < 0 {
		return fmt.Errorf("combat : dé d'au moins 2 faces, valeurs positives attendues")
	}
	if len(c.Etats) > 60 {
		return fmt.Errorf("combat.etats : soixante au plus")
	}
	vus := map[string]bool{}
	for i := range c.Etats {
		e := &c.Etats[i]
		e.Nom = strings.TrimSpace(e.Nom)
		if e.Nom == "" || len([]rune(e.Nom)) > 40 || vus[strings.ToLower(e.Nom)] {
			return fmt.Errorf("combat.etats.%d : nom absent, trop long ou en double", i)
		}
		vus[strings.ToLower(e.Nom)] = true
		if e.Fin != "" && e.Fin != "round" && e.Fin != "tour" {
			return fmt.Errorf("combat.etats.%d.fin : round ou tour", i)
		}
		if e.Action != "" && e.Action != "A" && e.Action != "L" {
			return fmt.Errorf("combat.etats.%d.action : A ou L", i)
		}
		if e.De < 0 || e.De == 1 || e.De > 1000 {
			return fmt.Errorf("combat.etats.%d.de : dé d'au moins 2 faces", i)
		}
	}
	return nil
}

// DeNormal : le dé des attaques et des tests.
func (c *Combat) DeNormal() int {
	if c == nil || c.De == 0 {
		return 20
	}
	return c.De
}

// Etat renvoie les effets d'un état ; un état que les règles ne connaissent pas (saisie libre) n'en
// a aucun.
func (c *Combat) Etat(nom string) EtatCombat {
	if c != nil {
		for _, e := range c.Etats {
			if e.Nom == nom {
				return e
			}
		}
	}
	return EtatCombat{Nom: nom}
}

// Connu : l'état est décrit par les règles.
func (c *Combat) Connu(nom string) bool {
	if c != nil {
		for _, e := range c.Etats {
			if e.Nom == nom {
				return true
			}
		}
	}
	return false
}

// Rang d'un état dans l'ordre des règles (les états libres viennent après).
func (c *Combat) Rang(nom string) int {
	if c != nil {
		for i, e := range c.Etats {
			if e.Nom == nom {
				return i
			}
		}
		return len(c.Etats)
	}
	return 0
}
