package modele

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Probleme décrit une valeur refusée.
type Probleme struct {
	Champ string `json:"champ"`
	Msg   string `json:"message"`
}

// ErrChamps regroupe les problèmes de validation d'une fiche.
type ErrChamps []Probleme

func (e ErrChamps) Error() string {
	var p []string
	for _, x := range e {
		if x.Champ != "" {
			p = append(p, x.Champ+" : "+x.Msg)
		} else {
			p = append(p, x.Msg)
		}
	}
	return strings.Join(p, " ; ")
}

// Fiches donne à la validation l'accès aux fiches existantes (pour les champs Ref).
type Fiches interface {
	TypeDeFiche(id int64) (string, bool)
}

// Adversaire d'une rencontre : un monstre du bestiaire de l'univers.
type Adversaire struct {
	Monstre int64  `json:"monstre"`
	Nom     string `json:"nom"`
	Qte     int    `json:"qte"`
	Camp    string `json:"camp"` // ennemi ou allie
}

// Etape d'une quête.
type Etape struct {
	Texte string `json:"texte"`
	Fait  bool   `json:"fait"`
}

var reDate = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// Normaliser vérifie les champs d'une fiche et les met au format de stockage. Les valeurs vides sont
// retirées. Toutes les erreurs sont renvoyées ensemble.
func Normaliser(t *Type, data map[string]any, cal Calendrier, f Fiches) (map[string]any, error) {
	out := map[string]any{}
	var errs ErrChamps
	cles := make([]string, 0, len(data))
	for k := range data {
		cles = append(cles, k)
	}
	sort.Strings(cles)
	for _, k := range cles {
		v := data[k]
		ch, ok := t.Champ(k)
		if !ok {
			var noms []string
			for _, c := range t.Champs {
				noms = append(noms, c.Cle)
			}
			errs = append(errs, Probleme{k, "champ inconnu pour « " + t.Nom + " » (champs : " + strings.Join(noms, ", ") + ")"})
			continue
		}
		if vide(v) {
			continue
		}
		val, err := normaliserValeur(ch, v, cal, f)
		if err != nil {
			errs = append(errs, Probleme{k, err.Error()})
			continue
		}
		if !vide(val) {
			out[k] = val
		}
	}
	if len(errs) > 0 {
		return nil, errs
	}
	return out, nil
}

func vide(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(x) == ""
	case []any:
		return len(x) == 0
	case []int64:
		return len(x) == 0
	case []Etape:
		return len(x) == 0
	case []Adversaire:
		return len(x) == 0
	}
	return false
}

func normaliserValeur(ch *Champ, v any, cal Calendrier, f Fiches) (any, error) {
	switch ch.Genre {
	case Texte:
		switch x := v.(type) {
		case string:
			return strings.TrimSpace(strings.ReplaceAll(x, "\n", " ")), nil
		case float64, int, int64, bool:
			return fmt.Sprint(x), nil
		}
		return nil, fmt.Errorf("texte attendu")
	case Long:
		if s, ok := v.(string); ok {
			return strings.TrimRight(strings.ReplaceAll(s, "\r\n", "\n"), " \n\t"), nil
		}
		return nil, fmt.Errorf("texte attendu")
	case Choix:
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("une valeur parmi : %s", strings.Join(ch.Options, ", "))
		}
		for _, o := range ch.Options {
			if strings.EqualFold(strings.TrimSpace(s), o) {
				return o, nil
			}
		}
		return nil, fmt.Errorf("« %s » n'est pas permis (valeurs : %s)", s, strings.Join(ch.Options, " · "))
	case Nombre:
		n, err := nombre(v)
		if err != nil {
			return nil, err
		}
		return n, nil
	case Booleen:
		switch x := v.(type) {
		case bool:
			return x, nil
		case string:
			switch strings.ToLower(strings.TrimSpace(x)) {
			case "oui", "true", "vrai", "1":
				return true, nil
			case "non", "false", "faux", "0":
				return false, nil
			}
		}
		return nil, fmt.Errorf("oui ou non attendu")
	case Ref:
		return reference(ch, v, f)
	case Refs:
		liste, ok := v.([]any)
		if !ok {
			if _, err := reference(ch, v, f); err == nil { // une seule valeur : tolérée
				liste = []any{v}
			} else {
				return nil, fmt.Errorf("liste d'identifiants de fiches attendue")
			}
		}
		var out []int64
		vu := map[int64]bool{}
		for _, x := range liste {
			id, err := reference(ch, x, f)
			if err != nil {
				return nil, err
			}
			if !vu[id] {
				vu[id] = true
				out = append(out, id)
			}
		}
		return out, nil
	case DateMonde:
		m, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("date attendue sous la forme {\"an\":1216,\"mois\":1,\"jour\":1}")
		}
		var d DateJeu
		for cle, dst := range map[string]*int{"an": &d.An, "mois": &d.Mois, "jour": &d.Jour} {
			n, err := nombre(m[cle])
			if err != nil || n != math.Trunc(n) {
				return nil, fmt.Errorf("%s : entier attendu", cle)
			}
			*dst = int(n)
		}
		if err := cal.VerifierDate(d); err != nil {
			return nil, err
		}
		return d, nil
	case DateReelle:
		s, ok := v.(string)
		s = strings.TrimSpace(s)
		if ok && reDate.MatchString(s) {
			if _, err := time.Parse("2006-01-02", s); err == nil {
				return s, nil
			}
		}
		return nil, fmt.Errorf("date AAAA-MM-JJ attendue")
	case Monstre:
		n, err := nombre(v)
		if err != nil || n != math.Trunc(n) || n <= 0 {
			return nil, fmt.Errorf("identifiant d'un monstre du bestiaire attendu")
		}
		return int64(n), nil
	case Adversaires:
		liste, ok := v.([]any)
		if !ok {
			return nil, fmt.Errorf("liste attendue : [{\"monstre\":12,\"nom\":\"Créature\",\"qte\":3,\"camp\":\"ennemi\"}]")
		}
		var out []Adversaire
		for _, x := range liste {
			m, ok := x.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("adversaire illisible")
			}
			id, err := nombre(m["monstre"])
			if err != nil || id <= 0 || id != math.Trunc(id) {
				return nil, fmt.Errorf("monstre : identifiant du bestiaire attendu")
			}
			a := Adversaire{Monstre: int64(id), Qte: 1, Camp: "ennemi"}
			a.Nom, _ = m["nom"].(string)
			if q, err := nombre(m["qte"]); err == nil && q >= 1 && q <= 99 {
				a.Qte = int(q)
			}
			if c, _ := m["camp"].(string); c == "allie" || c == "allié" {
				a.Camp = "allie"
			}
			out = append(out, a)
		}
		return out, nil
	case Etapes:
		liste, ok := v.([]any)
		if !ok {
			return nil, fmt.Errorf("liste d'étapes attendue : [{\"texte\":\"…\",\"fait\":false}]")
		}
		var out []Etape
		for _, x := range liste {
			switch e := x.(type) {
			case string:
				if strings.TrimSpace(e) != "" {
					out = append(out, Etape{Texte: strings.TrimSpace(e)})
				}
			case map[string]any:
				t, _ := e["texte"].(string)
				fait, _ := e["fait"].(bool)
				if strings.TrimSpace(t) != "" {
					out = append(out, Etape{Texte: strings.TrimSpace(t), Fait: fait})
				}
			default:
				return nil, fmt.Errorf("étape illisible")
			}
		}
		return out, nil
	}
	return nil, fmt.Errorf("genre de champ inconnu %s", ch.Genre)
}

func nombre(v any) (float64, error) {
	switch x := v.(type) {
	case float64:
		return x, nil
	case int:
		return float64(x), nil
	case int64:
		return float64(x), nil
	case string:
		if n, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(x), ",", "."), 64); err == nil {
			return n, nil
		}
	}
	return 0, fmt.Errorf("nombre attendu")
}

func reference(ch *Champ, v any, f Fiches) (int64, error) {
	n, err := nombre(v)
	if err != nil || n != math.Trunc(n) || n <= 0 {
		return 0, fmt.Errorf("identifiant de fiche attendu (%s)", strings.Join(ch.Cibles, " ou "))
	}
	id := int64(n)
	typ, ok := f.TypeDeFiche(id)
	if !ok {
		return 0, fmt.Errorf("aucune fiche n°%d", id)
	}
	for _, c := range ch.Cibles {
		if c == typ {
			return id, nil
		}
	}
	return 0, fmt.Errorf("la fiche n°%d est de type %s ; attendu : %s", id, typ, strings.Join(ch.Cibles, " ou "))
}
