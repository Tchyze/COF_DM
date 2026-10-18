// Package des lit et lance une formule de dés, hors combat : « 1d20+3 », « 2d6-1 », « 2d20kh1+5 »
// (garder le meilleur de deux dés), « 3d6kl2 » (garder les deux plus faibles). Aucune règle du jeu
// n'y est écrite : le résultat donne le dé « naturel » quand un seul dé compte, et l'écran en tire
// ce qu'il veut (réussite critique, échec).
package des

import (
	"errors"
	"math/rand/v2"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Bornes d'une formule : assez pour tout jet de table, trop peu pour encombrer.
const (
	LongueurMax = 60
	TermesMax   = 12
	DesMax      = 100
	FacesMax    = 1000
)

// De lance un dé à « faces » faces. Remplaçable dans les tests.
var De = func(faces int) int { return rand.IntN(faces) + 1 }

// Resultat : un jet.
type Resultat struct {
	Formule string `json:"formule"` // la formule, écrite proprement
	Total   int    `json:"total"`
	Detail  string `json:"detail"`          // « 2d20kh1[17,(4)] + 3 » : les dés écartés entre parenthèses
	Nat     int    `json:"nat,omitempty"`   // le dé qui compte, quand le premier groupe n'en garde qu'un
	Faces   int    `json:"faces,omitempty"` // … et son nombre de faces
}

var (
	reTerme = regexp.MustCompile(`[+-]?[^+-]+`)
	reDes   = regexp.MustCompile(`^(\d*)d(\d+)(?:k([hl]?)(\d*))?$`)
	reNb    = regexp.MustCompile(`^\d{1,6}$`)
)

// ErrFormule : la formule ne se lit pas.
var ErrFormule = errors.New("formule illisible : par exemple 1d20+3, 2d6-1, 2d20kh1 (garder le meilleur), 2d20kl1 (le pire)")

// Lancer lit la formule et lance ses dés.
func Lancer(formule string) (Resultat, error) {
	s := strings.ToLower(strings.Join(strings.Fields(formule), ""))
	if s == "" || len(s) > LongueurMax {
		return Resultat{}, ErrFormule
	}
	termes := reTerme.FindAllString(s, -1)
	if strings.Join(termes, "") != s || len(termes) > TermesMax {
		return Resultat{}, ErrFormule
	}
	var r Resultat
	var propre, detail []string
	nbDes, premier := 0, true
	for _, t := range termes {
		signe, sg := 1, "+"
		if t[0] == '-' {
			signe, sg = -1, "−"
		}
		corps := strings.TrimLeft(t, "+-")
		if corps == "" || len(t)-len(corps) > 1 {
			return Resultat{}, ErrFormule
		}
		sep := sg + " "
		if len(propre) == 0 && signe > 0 {
			sep = ""
		}
		if m := reDes.FindStringSubmatch(corps); m != nil {
			n := 1
			if m[1] != "" {
				n, _ = strconv.Atoi(m[1])
			}
			f, _ := strconv.Atoi(m[2])
			garde, haut := n, true
			if strings.Contains(corps, "k") {
				haut = m[3] != "l"
				garde = 1
				if m[4] != "" {
					garde, _ = strconv.Atoi(m[4])
				}
			}
			nbDes += n
			if n < 1 || f < 2 || f > FacesMax || nbDes > DesMax || garde < 1 || garde > n {
				return Resultat{}, ErrFormule
			}
			valeurs := make([]int, n)
			for i := range valeurs {
				valeurs[i] = De(f)
			}
			gardes := garder(valeurs, garde, haut)
			somme := 0
			parts := make([]string, n)
			for i, v := range valeurs {
				parts[i] = strconv.Itoa(v)
				if gardes[i] {
					somme += v
				} else {
					parts[i] = "(" + parts[i] + ")"
				}
			}
			r.Total += signe * somme
			ecrit := strconv.Itoa(n) + "d" + strconv.Itoa(f)
			if garde < n {
				ecrit += "k" + map[bool]string{true: "h", false: "l"}[haut] + strconv.Itoa(garde)
			}
			if premier && garde == 1 && signe > 0 {
				for i, v := range valeurs {
					if gardes[i] {
						r.Nat, r.Faces = v, f
					}
				}
			}
			premier = false
			propre = append(propre, sep+ecrit)
			detail = append(detail, sep+ecrit+"["+strings.Join(parts, ",")+"]")
		} else if reNb.MatchString(corps) {
			v, _ := strconv.Atoi(corps)
			r.Total += signe * v
			propre = append(propre, sep+corps)
			detail = append(detail, sep+corps)
		} else {
			return Resultat{}, ErrFormule
		}
	}
	if nbDes == 0 {
		return Resultat{}, errors.New("la formule ne lance aucun dé")
	}
	r.Formule = strings.ReplaceAll(strings.Join(propre, " "), "−", "-")
	r.Detail = strings.Join(detail, " ")
	return r, nil
}

// garder marque les « garde » dés les plus hauts (ou les plus bas) ; à égalité, les premiers lancés.
func garder(valeurs []int, garde int, haut bool) []bool {
	idx := make([]int, len(valeurs))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool {
		if haut {
			return valeurs[idx[a]] > valeurs[idx[b]]
		}
		return valeurs[idx[a]] < valeurs[idx[b]]
	})
	out := make([]bool, len(valeurs))
	for _, i := range idx[:garde] {
		out[i] = true
	}
	return out
}
