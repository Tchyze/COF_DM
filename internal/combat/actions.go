package combat

import (
	"strings"
)

// Budget d'actions d'un tour, quand les règles de l'univers le demandent (budget_actions) :
//   - 1 action limitée (L) ;
//   - OU 1 action de mouvement (M) et 1 action d'attaque (A) ;
//   - OU 2 actions de mouvement.
// Un état « une_action » ne laisse qu'une action, attaque ou mouvement ; un état « bloque », aucune.
// Les actions gratuites (riposte, attaque offerte par une capacité…) ne consomment rien.
// Sans budget, seuls les états bloquants et l'inconscience empêchent d'agir.
//
// Clés de Combattant.Act : "A", "M", "M2" (second mouvement), "L", "LA" (action limitée
// consommée par une attaque : les attaques suivantes de la même capacité restent permises).

func nouvellesActions() map[string]bool {
	return map[string]bool{"A": false, "M": false, "M2": false, "L": false, "LA": false}
}

func (c *Combattant) mouvements() int {
	n := 0
	if c.Act["M"] {
		n++
	}
	if c.Act["M2"] {
		n++
	}
	return n
}

// empecheAgir : raison pour laquelle le combattant ne peut rien faire ce tour, ou "".
func (st *Etat) empecheAgir(c *Combattant) string {
	if c.Pv <= 0 {
		if c.Camp == "pj" {
			return c.Nom + " est inconscient : il ne peut pas agir."
		}
		return c.Nom + " est hors de combat."
	}
	for _, e := range st.etatsTries(c) {
		if st.rg.Etat(e).Bloque {
			return c.Nom + " est " + strings.ToLower(e) + " : aucune action possible ce tour."
		}
	}
	return ""
}

// Possible indique si le combattant courant peut encore faire l'action k ("A", "M" ou "L").
// Le message explique le refus.
func (st *Etat) Possible(c *Combattant, k string) (bool, string) {
	if r := st.empecheAgir(c); r != "" {
		return false, r
	}
	if k != "A" && k != "M" && k != "L" {
		return false, "Type d'action inconnu."
	}
	if st.rg == nil || !st.rg.BudgetActions {
		return true, ""
	}
	uneAction := false
	for e := range c.Etats {
		uneAction = uneAction || st.rg.Etat(e).UneAction
	}
	m := c.mouvements()
	switch k {
	case "L":
		if uneAction {
			return false, c.Nom + " n'a qu'une action ce tour, attaque ou mouvement : pas d'action limitée."
		}
		if c.Act["L"] {
			return false, c.Nom + " a déjà utilisé son action limitée ce tour."
		}
		if c.Act["A"] || m > 0 {
			return false, "Une action limitée occupe tout le tour : " + c.Nom + " a déjà agi (" + resumeActions(c) + ")."
		}
	case "A":
		if c.Act["L"] {
			return false, c.Nom + " a utilisé une action limitée : plus d'attaque possible ce tour."
		}
		if c.Act["A"] {
			return false, c.Nom + " a déjà utilisé son action d'attaque ce tour."
		}
		if m >= 2 {
			return false, c.Nom + " a fait deux mouvements : plus d'attaque possible ce tour."
		}
		if uneAction && m > 0 {
			return false, c.Nom + " s'est déjà déplacé et n'a qu'une action ce tour."
		}
	case "M":
		if c.Act["L"] {
			return false, c.Nom + " a utilisé une action limitée : plus de mouvement possible ce tour."
		}
		if m >= 2 {
			return false, c.Nom + " a déjà fait ses deux mouvements."
		}
		if m == 1 && c.Act["A"] {
			return false, c.Nom + " a déjà attaqué et bougé : son tour est complet."
		}
		if uneAction && (m > 0 || c.Act["A"]) {
			return false, c.Nom + " n'a qu'une action ce tour."
		}
	default:
		return false, "Type d'action inconnu."
	}
	return true, ""
}

func resumeActions(c *Combattant) string {
	var p []string
	if c.Act["A"] {
		p = append(p, "attaque")
	}
	switch c.mouvements() {
	case 1:
		p = append(p, "un mouvement")
	case 2:
		p = append(p, "deux mouvements")
	}
	if c.Act["L"] {
		p = append(p, "action limitée")
	}
	if len(p) == 0 {
		return "rien"
	}
	return strings.Join(p, ", ")
}

// consommer réserve l'action k pour le combattant courant, ou renvoie une erreur.
func (st *Etat) consommer(c *Combattant, k string) error {
	if cur := st.Courant(); cur == nil || cur.ID != c.ID {
		return ErrAction("Ce n'est pas le tour de " + c.Nom + ".")
	}
	if ok, raison := st.Possible(c, k); !ok {
		return ErrAction(raison)
	}
	if st.rg == nil || !st.rg.BudgetActions {
		return nil
	}
	if k == "M" && c.Act["M"] {
		c.Act["M2"] = true
	} else {
		c.Act[k] = true
	}
	return nil
}

// actionAttaque vérifie et consomme l'action correspondant à une attaque.
// mode : "A" (attaque normale), "L" (capacité limitée), "LS" (attaque suivante de la même capacité
// limitée, ex. Double attaque) ou "G" (action gratuite : riposte, capacité, hors de son tour).
func (st *Etat) actionAttaque(a *Combattant, mode string) (string, error) {
	if mode == "" {
		mode = "A"
	}
	if mode == "G" {
		if a.Pv <= 0 {
			return "", ErrAction(st.empecheAgir(a))
		}
		for _, e := range st.etatsTries(a) {
			if st.rg.Etat(e).Bloque {
				return "", ErrAction(a.Nom + " est " + strings.ToLower(e) + " : il ne peut pas attaquer, même en action gratuite.")
			}
		}
		return " (action gratuite)", nil
	}
	if cur := st.Courant(); cur == nil || cur.ID != a.ID {
		return "", ErrAction("Ce n'est pas le tour de " + a.Nom + ". Une attaque hors de son tour (riposte, capacité) se déclare en « action gratuite ».")
	}
	switch mode {
	case "A":
		return "", st.consommer(a, "A")
	case "L":
		if err := st.consommer(a, "L"); err != nil {
			return "", err
		}
		a.Act["LA"] = true
		return " (action limitée)", nil
	case "LS":
		if !a.Act["LA"] {
			return "", ErrAction("Aucune capacité limitée en cours : choisissez d'abord « Capacité limitée (L) » pour la première attaque.")
		}
		if r := st.empecheAgir(a); r != "" {
			return "", ErrAction(r)
		}
		return " (suite de l'action limitée)", nil
	}
	return "", ErrAction("Type d'action inconnu.")
}
