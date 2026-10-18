package combat

import (
	"math/rand/v2"
	"regexp"
	"strconv"
	"strings"
)

var (
	reToken = regexp.MustCompile(`[+-]?[^+-]+`)
	reDe    = regexp.MustCompile(`^(\d*)d(\d+)$`)
	reNum   = regexp.MustCompile(`^\d+$`)
)

// De lance un dé à `faces` faces. Remplaçable dans les tests.
var De = func(faces int) int { return rand.IntN(faces) + 1 }

type Jet struct {
	Total  int
	Detail string
}

// Lancer évalue une formule comme « 2d6+3 » ou « 1d8+1d6-1 », et multiplie le total (dégâts d'un
// critique ; 1 ou moins : rien). ok=false si elle est illisible.
func Lancer(expr string, mult int) (Jet, bool) {
	s := strings.ToLower(strings.Join(strings.Fields(expr), ""))
	if s == "" {
		return Jet{}, false
	}
	toks := reToken.FindAllString(s, -1)
	if strings.Join(toks, "") != s {
		return Jet{}, false
	}
	total := 0
	var parts []string
	for _, t := range toks {
		sg := 1
		if t[0] == '-' {
			sg = -1
		}
		corps := strings.TrimLeft(t, "+-")
		if m := reDe.FindStringSubmatch(corps); m != nil {
			n := 1
			if m[1] != "" {
				n, _ = strconv.Atoi(m[1])
			}
			f, _ := strconv.Atoi(m[2])
			if n < 1 || n > 40 || f < 2 {
				return Jet{}, false
			}
			rs := make([]string, n)
			somme := 0
			for i := range n {
				r := De(f)
				somme += r
				rs[i] = strconv.Itoa(r)
			}
			total += sg * somme
			prefixe := ""
			if sg < 0 {
				prefixe = "− "
			} else if len(parts) > 0 {
				prefixe = "+ "
			}
			parts = append(parts, prefixe+strconv.Itoa(n)+"d"+strconv.Itoa(f)+"["+strings.Join(rs, ",")+"]")
		} else if reNum.MatchString(corps) {
			v, _ := strconv.Atoi(corps)
			total += sg * v
			if sg < 0 {
				parts = append(parts, "− "+corps)
			} else {
				parts = append(parts, "+ "+corps)
			}
		} else {
			return Jet{}, false
		}
	}
	detail := strings.Join(parts, " ")
	if mult > 1 {
		detail = "(" + detail + ") × " + strconv.Itoa(mult)
		total *= mult
	}
	return Jet{Total: max(0, total), Detail: detail}, true
}
