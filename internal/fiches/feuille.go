package fiches

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/Tchyze/cof_dm/internal/regles"
)

// Feuille de personnage d'un PJ : ses caractéristiques, ses ressources, ses attaques, ses voies.
// Elle complète la fiche du PJ, qui garde son nom, son joueur, son peuple, son profil, son niveau
// et ses textes : rien n'est saisi deux fois.
//
// Les bornes et les valeurs de départ viennent du fichier de règles de l'univers (regles.Feuille) ;
// sans lui, la feuille n'impose rien. Les valeurs dérivées (modificateurs, initiative, DEF, bonus
// d'attaque) sont recalculées à chaque enregistrement.

type Valeurs = Carac

type Protection struct {
	Nom   string `json:"nom"`
	Bonus int    `json:"bonus"`
}

type AttDivers struct {
	C int `json:"C"`
	D int `json:"D"`
	M int `json:"M"`
}

type Voie struct {
	Nom       string   `json:"nom"`
	Origine   string   `json:"origine"`
	Rang      int      `json:"rang"`
	Capacites []string `json:"capacites"`
	Ref       string   `json:"ref,omitempty"` // voie du fichier de profils et voies
}

type Bourse struct {
	Po int `json:"po"`
	Pa int `json:"pa"`
	Pc int `json:"pc"`
}

type Feuille struct {
	Sexe           string     `json:"sexe"`
	Age            string     `json:"age"`
	TaillePhysique string     `json:"taille_physique"`
	Poids          string     `json:"poids"`
	Divinite       string     `json:"divinite"`
	Langues        string     `json:"langues"`
	Valeurs        Valeurs    `json:"valeurs"`
	Carac          Carac      `json:"carac"` // modificateurs, dérivés des valeurs
	Dv             string     `json:"dv"`
	Pv             int        `json:"pv"`
	PvActuel       *int       `json:"pv_actuel"`
	Defense        int        `json:"defense"` // dérivée
	Init           int        `json:"init"`    // dérivée
	Rd             int        `json:"rd"`
	Crit           int        `json:"crit"`
	Armure         Protection `json:"armure"`
	Bouclier       Protection `json:"bouclier"`
	DefDivers      int        `json:"def_divers"`
	InitDivers     int        `json:"init_divers"`
	AttDivers      AttDivers  `json:"att_divers"`
	CaracMagie     string     `json:"carac_magie"`
	Attaques       []Attaque  `json:"attaques"`
	Pc             int        `json:"pc"`
	PcMax          int        `json:"pc_max"`
	Pr             int        `json:"pr"`
	PrMax          int        `json:"pr_max"`
	Pm             int        `json:"pm"`
	PmMax          int        `json:"pm_max"`
	Voies          []Voie     `json:"voies"`
	Capacites      string     `json:"capacites"`
	Equipement     string     `json:"equipement"`
	Bourse         Bourse     `json:"bourse"`
	Notes          string     `json:"notes"` // notes du MJ : jamais montrées au joueur
}

var reDe = regexp.MustCompile(`^d\d{1,3}$`)

func dans(liste []string, v string) bool {
	for _, x := range liste {
		if x == v {
			return true
		}
	}
	return false
}

// valeurNeutre : la valeur de caractéristique dont le modificateur est nul.
const valeurNeutre = 10

// FeuilleNeuve : une feuille vierge, avec les valeurs de départ que donnent les règles. Les
// caractéristiques partent de la valeur neutre (modificateur nul), ramenée dans les bornes des règles.
func FeuilleNeuve(rg *regles.Feuille) *Feuille {
	f := &Feuille{Pv: 1, Crit: 20, Attaques: []Attaque{}, Voies: []Voie{}}
	v := valeurNeutre
	if rg != nil {
		f.Dv, f.CaracMagie = rg.DeDeVie, rg.CaracMagie
		f.PcMax, f.Pc, f.PrMax, f.Pr = rg.PcMax, rg.PcMax, rg.PrMax, rg.PrMax
		v = max(v, rg.ValeurMin)
		if rg.ValeurMax > 0 {
			v = min(v, rg.ValeurMax)
		}
	}
	f.Valeurs = Valeurs{v, v, v, v, v, v}
	return f
}

// NormaliserFeuille valide une feuille et recalcule ses valeurs dérivées. niveau est celui de la
// fiche du PJ ; rg, les règles de l'univers (nil : aucune borne).
func NormaliserFeuille(raw []byte, niveau int, rg *regles.Feuille) (*Feuille, error) {
	f := FeuilleNeuve(rg)
	if err := decoder(raw, f); err != nil {
		return nil, err
	}
	if rg == nil {
		rg = &regles.Feuille{}
	}
	for _, k := range Caracs {
		if v := f.Valeurs.Get(k); v < rg.ValeurMin || (rg.ValeurMax > 0 && v > rg.ValeurMax) {
			if rg.ValeurMax > 0 {
				return nil, invalide("valeurs.%s : entre %d et %d.", k, rg.ValeurMin, rg.ValeurMax)
			}
			return nil, invalide("valeurs.%s : au moins %d.", k, rg.ValeurMin)
		}
	}
	f.Dv = strings.ToLower(strings.TrimSpace(f.Dv))
	switch {
	case len(rg.DesDeVie) > 0 && !dans(rg.DesDeVie, f.Dv):
		return nil, invalide("dv : %s.", strings.Join(rg.DesDeVie, ", "))
	case f.Dv != "" && !reDe.MatchString(f.Dv):
		return nil, invalide("dv : un dé comme d8.")
	}
	switch {
	case len(rg.CaracsMagie) > 0 && !dans(rg.CaracsMagie, f.CaracMagie):
		return nil, invalide("carac_magie : %s.", strings.Join(rg.CaracsMagie, ", "))
	case f.CaracMagie != "" && !dans(Caracs, f.CaracMagie):
		return nil, invalide("carac_magie : %s.", strings.Join(Caracs, ", "))
	}
	if f.Pv < 1 {
		return nil, invalide("pv : au moins 1.")
	}
	if f.Rd < 0 {
		return nil, invalide("rd : ne peut pas être négative.")
	}
	if f.Crit < 2 || f.Crit > 20 {
		return nil, invalide("crit : entre 2 et 20.")
	}
	if f.Armure.Bonus < 0 || f.Bouclier.Bonus < 0 {
		return nil, invalide("bonus d'armure ou de bouclier : ne peut pas être négatif.")
	}
	for _, v := range []int{f.Pc, f.PcMax, f.Pr, f.PrMax, f.Pm, f.PmMax, f.Bourse.Po, f.Bourse.Pa, f.Bourse.Pc} {
		if v < 0 {
			return nil, invalide("les ressources et la bourse ne peuvent pas être négatives.")
		}
	}
	if f.Attaques == nil {
		f.Attaques = []Attaque{}
	}
	if len(f.Attaques) > 40 || len(f.Voies) > 40 {
		return nil, invalide("trop d'attaques ou de voies.")
	}
	for i := range f.Attaques {
		a := &f.Attaques[i]
		if a.Type == "" {
			a.Type = "C"
		}
		if a.Type != "C" && a.Type != "D" && a.Type != "M" {
			return nil, invalide("attaques.%d.type : C, D ou M.", i)
		}
		if strings.TrimSpace(a.Nom) == "" {
			a.Nom = "Attaque"
		}
	}
	if f.Voies == nil {
		f.Voies = []Voie{}
	}
	for i := range f.Voies {
		v := &f.Voies[i]
		if len(rg.OriginesVoie) > 0 {
			if v.Origine == "" {
				v.Origine = rg.OriginesVoie[0]
			}
			if !dans(rg.OriginesVoie, v.Origine) {
				return nil, invalide("voies.%d.origine : %s.", i, strings.Join(rg.OriginesVoie, ", "))
			}
		}
		if len(v.Capacites) > 20 {
			return nil, invalide("voies.%d : trop de capacités.", i)
		}
		rangMax := len(v.Capacites)
		if rg.RangMax > 0 {
			rangMax = rg.RangMax
			for len(v.Capacites) < rg.RangMax { // une ligne par rang, même vide
				v.Capacites = append(v.Capacites, "")
			}
			v.Capacites = v.Capacites[:rg.RangMax]
		}
		if v.Capacites == nil {
			v.Capacites = []string{}
		}
		if v.Rang < 0 || v.Rang > rangMax {
			return nil, invalide("voies.%d.rang : entre 0 et %d.", i, rangMax)
		}
	}
	f.Deriver(niveau, rg)
	return f, nil
}

// BasesAttaque : niveau + modificateur + divers, pour chaque type d'attaque.
func (f *Feuille) BasesAttaque(niveau int) map[string]int {
	return map[string]int{
		"C": niveau + f.Carac.FOR + f.AttDivers.C,
		"D": niveau + f.Carac.DEX + f.AttDivers.D,
		"M": niveau + f.Carac.Get(f.CaracMagie) + f.AttDivers.M,
	}
}

// Deriver recalcule modificateurs, initiative, DEF et bonus d'attaque, et borne les ressources.
func (f *Feuille) Deriver(niveau int, rg *regles.Feuille) {
	for _, k := range Caracs {
		f.Carac.Set(k, Modificateur(f.Valeurs.Get(k)))
	}
	f.Init = f.Valeurs.DEX + f.InitDivers
	f.Defense = rg.DefenseBase + f.Carac.DEX + f.Armure.Bonus + f.Bouclier.Bonus + f.DefDivers
	b := f.BasesAttaque(niveau)
	for i := range f.Attaques {
		f.Attaques[i].Bonus = b[f.Attaques[i].Type] + f.Attaques[i].Mod
	}
	if f.PvActuel != nil {
		v := max(0, min(f.Pv, *f.PvActuel))
		f.PvActuel = &v
	}
	f.Pc, f.Pr, f.Pm = min(f.Pc, f.PcMax), min(f.Pr, f.PrMax), min(f.Pm, f.PmMax)
}

// PourJoueur renvoie la feuille telle que le joueur la voit : sans les notes du MJ.
func (f Feuille) PourJoueur() Feuille {
	f.Notes = ""
	return f
}

// JSON de la feuille.
func (f *Feuille) JSON() []byte {
	b, _ := json.Marshal(f)
	return b
}
