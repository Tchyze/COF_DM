package store

import (
	"strings"
	"unicode"
)

// Plier retire accents et casse pour comparer des noms.
func Plier(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if p, ok := plis[r]; ok {
			b.WriteString(p)
		} else if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			b.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

var plis = map[rune]string{
	'à': "a", 'â': "a", 'ä': "a", 'á': "a", 'ã': "a", 'å': "a",
	'ç': "c", 'é': "e", 'è': "e", 'ê': "e", 'ë': "e",
	'î': "i", 'ï': "i", 'í': "i", 'ì': "i",
	'ô': "o", 'ö': "o", 'ó': "o", 'ò': "o", 'õ': "o",
	'ù': "u", 'û': "u", 'ü': "u", 'ú': "u",
	'ÿ': "y", 'ñ': "n", 'œ': "oe", 'æ': "ae",
}

// Distance d'édition (Levenshtein, une inversion de deux lettres comptant pour une faute).
func Distance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	d := make([][]int, len(ra)+1)
	for i := range d {
		d[i] = make([]int, len(rb)+1)
		d[i][0] = i
	}
	for j := range d[0] {
		d[0][j] = j
	}
	for i := 1; i <= len(ra); i++ {
		for j := 1; j <= len(rb); j++ {
			c := 1
			if ra[i-1] == rb[j-1] {
				c = 0
			}
			d[i][j] = min(d[i-1][j]+1, d[i][j-1]+1, d[i-1][j-1]+c)
			if i > 1 && j > 1 && ra[i-1] == rb[j-2] && ra[i-2] == rb[j-1] {
				d[i][j] = min(d[i][j], d[i-2][j-2]+1)
			}
		}
	}
	return d[len(ra)][len(rb)]
}

// distanceNom tolère une faute par tranche de quatre lettres, sur le nom entier ou une suite de ses
// mots. Renvoie -1 si le nom est trop éloigné.
func distanceNom(q, nom string) int {
	n := len([]rune(q))
	if n < 4 {
		return -1
	}
	tol := max(1, n/4)
	best := -1
	essai := func(c string) {
		if d := Distance(q, c); d <= tol && (best < 0 || d < best) {
			best = d
		}
	}
	essai(nom)
	mots := strings.Fields(nom)
	k := len(strings.Fields(q))
	for i := 0; i+k <= len(mots); i++ {
		essai(strings.Join(mots[i:i+k], " "))
	}
	return best
}

// bonusNom favorise le nom exact, puis les noms qui contiennent chaque mot cherché en début de mot.
func bonusNom(nom string, mots []string) float64 {
	q := Plier(strings.Join(mots, " "))
	if q == "" {
		return 0
	}
	if nom == q {
		return 1000
	}
	for _, m := range strings.Fields(q) {
		ok := false
		for _, n := range strings.Fields(nom) {
			if strings.HasPrefix(n, m) {
				ok = true
				break
			}
		}
		if !ok {
			return 0
		}
	}
	return 500
}

// analyserRecherche sépare mots, "phrases exactes" et #tags.
func analyserRecherche(q string) (mots, phrases, tags []string) {
	for {
		i := strings.IndexByte(q, '"')
		if i < 0 {
			break
		}
		j := strings.IndexByte(q[i+1:], '"')
		if j < 0 {
			q = q[:i] + q[i+1:]
			break
		}
		if p := strings.TrimSpace(q[i+1 : i+1+j]); p != "" {
			phrases = append(phrases, p)
		}
		q = q[:i] + " " + q[i+2+j:]
	}
	for _, m := range strings.Fields(q) {
		if strings.HasPrefix(m, "#") && len(m) > 1 {
			tags = append(tags, m[1:])
		} else {
			mots = append(mots, m)
		}
	}
	return
}

// requeteFTS construit une requête FTS5 sûre : chaque mot devient un préfixe entre guillemets.
func requeteFTS(mots, phrases []string) string {
	var parts []string
	for _, p := range phrases {
		parts = append(parts, `"`+strings.ReplaceAll(p, `"`, ``)+`"`)
	}
	for _, m := range mots {
		for _, t := range strings.Fields(Plier(m)) {
			parts = append(parts, `"`+t+`"*`)
		}
	}
	return strings.Join(parts, " AND ")
}
