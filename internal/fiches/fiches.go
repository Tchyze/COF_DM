// Package fiches définit les fiches de combat (les monstres du bestiaire, pour l'instant) et leur
// validation. Repris de tracker_COF. Aucune donnée des livres : les listes de choix (tailles,
// catégories) viennent du fichier de règles de l'univers (internal/regles).
package fiches

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ErrValidation est renvoyée quand une fiche est refusée ; son message s'affiche tel quel.
type ErrValidation struct{ Msg string }

func (e ErrValidation) Error() string { return "Fiche invalide. " + e.Msg }

func invalide(format string, a ...any) error { return ErrValidation{fmt.Sprintf(format, a...)} }

var Caracs = []string{"FOR", "DEX", "CON", "INT", "SAG", "CHA"}

// ---------------------------------------------------------------- types communs

type Attaque struct {
	Nom    string `json:"nom"`
	Type   string `json:"type"` // C contact, D distance, M magique
	Bonus  int    `json:"bonus"`
	Dm     string `json:"dm"`
	Notes  string `json:"notes"`
	Mod    int    `json:"mod"`             // bonus propre à l'arme (personnages)
	Portee string `json:"portee"`          // personnages
	Objet  int    `json:"objet,omitempty"` // personnages : l'arme portée de l'inventaire qui donne cette ligne (calculée)
}

type Carac struct {
	FOR int `json:"FOR"`
	DEX int `json:"DEX"`
	CON int `json:"CON"`
	INT int `json:"INT"`
	SAG int `json:"SAG"`
	CHA int `json:"CHA"`
}

func (c Carac) Get(k string) int {
	switch k {
	case "FOR":
		return c.FOR
	case "DEX":
		return c.DEX
	case "CON":
		return c.CON
	case "INT":
		return c.INT
	case "SAG":
		return c.SAG
	case "CHA":
		return c.CHA
	}
	return 0
}

func (c *Carac) Set(k string, v int) {
	switch k {
	case "FOR":
		c.FOR = v
	case "DEX":
		c.DEX = v
	case "CON":
		c.CON = v
	case "INT":
		c.INT = v
	case "SAG":
		c.SAG = v
	case "CHA":
		c.CHA = v
	}
}

// Modificateur selon la table COF : 10-11 → +0, 12-13 → +1, 8-9 → −1…
func Modificateur(valeur int) int {
	m := valeur - 10
	if m < 0 {
		return (m - 1) / 2
	}
	return m / 2
}

type base struct {
	Nom       string    `json:"nom"`
	Defense   int       `json:"defense"`
	Pv        int       `json:"pv"`
	Init      int       `json:"init"`
	Rd        int       `json:"rd"`
	Crit      int       `json:"crit"`
	Carac     Carac     `json:"carac"`
	Attaques  []Attaque `json:"attaques"`
	Capacites string    `json:"capacites"`
	Notes     string    `json:"notes"`
}

func baseParDefaut() base {
	return base{Defense: 10, Pv: 1, Init: 10, Crit: 20, Attaques: []Attaque{}}
}

func (b *base) valider() error {
	b.Nom = strings.TrimSpace(b.Nom)
	if b.Nom == "" {
		return invalide("nom : Le nom est obligatoire.")
	}
	if len([]rune(b.Nom)) > 120 {
		return invalide("nom : 120 caractères au maximum.")
	}
	if b.Pv < 1 {
		return invalide("pv : au moins 1.")
	}
	if b.Rd < 0 {
		return invalide("rd : ne peut pas être négative.")
	}
	if b.Crit < 2 || b.Crit > 20 {
		return invalide("crit : entre 2 et 20.")
	}
	if b.Attaques == nil {
		b.Attaques = []Attaque{}
	}
	for i := range b.Attaques {
		a := &b.Attaques[i]
		if a.Type == "" {
			a.Type = "C"
		}
		if a.Type != "C" && a.Type != "D" && a.Type != "M" {
			return invalide("attaques.%d.type : C, D ou M.", i)
		}
		if strings.TrimSpace(a.Nom) == "" {
			a.Nom = "Attaque"
		}
	}
	return nil
}

// ---------------------------------------------------------------- monstres

type Monstre struct {
	base
	Source      string   `json:"source"`
	Nc          float64  `json:"nc"`
	Categorie   string   `json:"categorie"`
	Taille      string   `json:"taille"`
	Tags        []string `json:"tags"`
	Superieures []string `json:"superieures"` // Carac. supérieures (*) : 2 dés, on garde le meilleur
	Description string   `json:"description"`
	// Pour le jouer : comment il se bat et réagit, où on le croise, ce qu'il porte, ce qu'il parle.
	Comportement string `json:"comportement,omitempty"`
	Habitat      string `json:"habitat,omitempty"`
	Butin        string `json:"butin,omitempty"`
	Langues      string `json:"langues,omitempty"`
	// Fiches officielles : identifiant fixe (mise à jour par import) et référence au livre.
	Ref   string `json:"ref,omitempty"`
	Livre string `json:"livre,omitempty"`
	Page  int    `json:"page,omitempty"`
}

// Verrouillee : une fiche officielle importée ne se modifie pas, on la duplique.
func (m *Monstre) Verrouillee() bool { return m.Source == "officiel" && m.Ref != "" }

func NormaliserMonstre(raw []byte) (*Monstre, error) {
	m := &Monstre{base: baseParDefaut(), Source: "maison", Tags: []string{}}
	if err := decoder(raw, m); err != nil {
		return nil, err
	}
	if err := m.valider(); err != nil {
		return nil, err
	}
	if m.Source != "officiel" && m.Source != "maison" {
		return nil, invalide("source : officiel ou maison.")
	}
	if m.Nc < 0 {
		return nil, invalide("nc : ne peut pas être négatif.")
	}
	if m.Tags == nil {
		m.Tags = []string{}
	}
	sup := []string{}
	for _, c := range m.Superieures {
		for _, k := range Caracs {
			if c == k {
				sup = append(sup, c)
			}
		}
	}
	m.Superieures = sup
	m.Ref = strings.TrimSpace(m.Ref)
	return m, nil
}

// ---------------------------------------------------------------- générique

func decoder(raw []byte, v any) error {
	if err := json.Unmarshal(raw, v); err != nil {
		var te *json.UnmarshalTypeError
		if errors.As(err, &te) {
			return invalide("%s : type de valeur incorrect.", te.Field)
		}
		return invalide("JSON illisible.")
	}
	return nil
}
