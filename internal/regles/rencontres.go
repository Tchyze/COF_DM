package regles

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
)

// Rencontres : ce que le fichier de règles dit de la difficulté d'une rencontre. Le code n'en garde
// que le calcul : la table des points, les conditions, les verdicts et les alertes viennent du
// fichier.
type Rencontres struct {
	// Points : ce que vaut une créature selon son NC. La même table donne le niveau d'une rencontre
	// (la ligne dont les points sont les plus proches du total).
	Points []PointsNC `json:"points"`
	// PointsAuDela : points par niveau au-delà de la dernière ligne (0 : la dernière ligne vaut pour
	// tout ce qui la dépasse).
	PointsAuDela float64 `json:"points_au_dela"`
	// GroupeReference : taille du groupe pour laquelle la table est écrite (0 : pas d'ajustement).
	GroupeReference int         `json:"groupe_reference"`
	Conditions      []Condition `json:"conditions"`
	// Verdicts, du plus facile au plus dur : le premier dont la borne est respectée l'emporte.
	Verdicts []Verdict         `json:"verdicts"`
	Alertes  []AlerteRencontre `json:"alertes"`

	illisible error // partie d'une forme inconnue : refusée à l'import, ignorée à la relecture
}

type PointsNC struct {
	Nc     float64 `json:"nc"`
	Points int     `json:"points"`
}

// Condition de la rencontre : ce qu'elle ajoute à son niveau.
type Condition struct {
	Cle     string  `json:"cle"`
	Libelle string  `json:"libelle"`
	Niveau  float64 `json:"niveau"`
	Detail  string  `json:"detail"`
}

// Verdict selon l'écart entre le niveau de la rencontre et celui du groupe. Sans borne (JusquA
// absent), il vaut pour tout écart : c'est le dernier de la liste.
type Verdict struct {
	Cle    string   `json:"cle"`
	Texte  string   `json:"texte"`
	JusquA *float64 `json:"jusqu_a"`
	Inclus bool     `json:"inclus"`
	Ton    string   `json:"ton"` // ok, warn, danger
}

// AlerteRencontre : avertissement quand le plus fort NC (« nc ») ou l'écart entre le plus fort et le
// plus faible (« ecart_nc ») atteint le seuil.
type AlerteRencontre struct {
	Quand string  `json:"quand"`
	Seuil float64 `json:"seuil"`
	Texte string  `json:"texte"`
}

// UnmarshalJSON garde l'erreur de côté au lieu de faire échouer tout le fichier : un univers dont
// les règles ont été rangées avant que cette partie ait une structure doit rester lisible.
func (x *Rencontres) UnmarshalJSON(data []byte) error {
	type brut Rencontres
	var b brut
	if err := json.Unmarshal(data, &b); err != nil {
		*x = Rencontres{illisible: err}
		return nil
	}
	*x = Rencontres(b)
	return nil
}

func (x *Rencontres) normaliser() error {
	if x.illisible != nil {
		return fmt.Errorf("rencontre : partie illisible (%v)", x.illisible)
	}
	if len(x.Points) > 200 || len(x.Conditions) > 30 || len(x.Verdicts) > 30 || len(x.Alertes) > 30 {
		return fmt.Errorf("rencontre : trop de lignes")
	}
	sort.SliceStable(x.Points, func(i, j int) bool { return x.Points[i].Nc < x.Points[j].Nc })
	for i, p := range x.Points {
		if p.Nc < 0 || p.Points < 0 || (i > 0 && (p.Nc == x.Points[i-1].Nc || p.Points < x.Points[i-1].Points)) {
			return fmt.Errorf("rencontre.points : NC distincts et points croissants attendus")
		}
	}
	if x.PointsAuDela < 0 || x.GroupeReference < 0 || x.GroupeReference > 50 {
		return fmt.Errorf("rencontre : points_au_dela et groupe_reference positifs attendus")
	}
	vues := map[string]bool{}
	for i := range x.Conditions {
		c := &x.Conditions[i]
		c.Cle, c.Libelle = strings.TrimSpace(c.Cle), strings.TrimSpace(c.Libelle)
		if c.Cle == "" || vues[c.Cle] {
			return fmt.Errorf("rencontre.conditions.%d : clé absente ou en double", i)
		}
		vues[c.Cle] = true
		if c.Libelle == "" {
			c.Libelle = c.Cle
		}
	}
	for i := range x.Verdicts {
		v := &x.Verdicts[i]
		if v.Cle = strings.TrimSpace(v.Cle); v.Cle == "" {
			return fmt.Errorf("rencontre.verdicts.%d : clé absente", i)
		}
		if v.Ton != "" && v.Ton != "ok" && v.Ton != "warn" && v.Ton != "danger" {
			return fmt.Errorf("rencontre.verdicts.%d.ton : ok, warn ou danger", i)
		}
	}
	for i, a := range x.Alertes {
		if (a.Quand != "nc" && a.Quand != "ecart_nc") || strings.TrimSpace(a.Texte) == "" {
			return fmt.Errorf("rencontre.alertes.%d : « quand » vaut nc ou ecart_nc, avec un texte", i)
		}
	}
	return nil
}

// PointsDe : points d'une créature selon son NC (la ligne la plus proche ; au-delà de la table,
// extrapolation).
func (x *Rencontres) PointsDe(nc float64) int {
	n := len(x.Points)
	if n == 0 {
		return 0
	}
	if der := x.Points[n-1]; nc > der.Nc {
		return der.Points + int(math.Round((nc-der.Nc)*x.PointsAuDela))
	}
	meilleur, ecart := x.Points[0].Points, math.MaxFloat64
	for _, l := range x.Points {
		if e := math.Abs(l.Nc - nc); e < ecart {
			meilleur, ecart = l.Points, e
		}
	}
	return meilleur
}

// NiveauDe : niveau de la rencontre, c'est-à-dire le NC dont les points sont les plus proches du
// total.
func (x *Rencontres) NiveauDe(points float64) float64 {
	n := len(x.Points)
	if n == 0 {
		return 0
	}
	if der := x.Points[n-1]; points > float64(der.Points) {
		if x.PointsAuDela <= 0 {
			return der.Nc
		}
		return der.Nc + math.Round((points-float64(der.Points))/x.PointsAuDela)
	}
	meilleur, ecart := 0.0, math.MaxFloat64
	for _, l := range x.Points {
		if e := math.Abs(float64(l.Points) - points); e < ecart {
			meilleur, ecart = l.Nc, e
		}
	}
	return meilleur
}

// GroupeRencontre : des créatures identiques d'une rencontre.
type GroupeRencontre struct {
	Nom string  `json:"nom"`
	Nc  float64 `json:"nc"`
	Qte int     `json:"qte"`
}

type DemandeRencontre struct {
	Groupes   []GroupeRencontre `json:"groupes"`
	NbPj      int               `json:"nb_pj"`     // 0 : le groupe de référence
	NiveauPj  float64           `json:"niveau_pj"` // 0 : inconnu, pas de verdict
	Condition string            `json:"condition"` // clé d'une condition du fichier, ou vide
}

// Evaluation d'une rencontre. Cle : celle du verdict, « vide » sans créature, « inconnu » sans
// niveau de groupe.
type Evaluation struct {
	Points        int      `json:"points"`
	PointsAjustes float64  `json:"points_ajustes"`
	Niveau        float64  `json:"niveau"`
	Ecart         float64  `json:"ecart"`
	Cle           string   `json:"cle"`
	Verdict       string   `json:"verdict"`
	Ton           string   `json:"ton"`
	Detail        []string `json:"detail"`
	Alertes       []string `json:"alertes"`
}

// Evaluer calcule la difficulté d'une rencontre : points des créatures, ramenés au groupe de
// référence, niveau de la rencontre, puis verdict selon l'écart avec le niveau du groupe.
func (x *Rencontres) Evaluer(d DemandeRencontre) Evaluation {
	e := Evaluation{Detail: []string{}, Alertes: []string{}}
	ncMin, ncMax, sortes := math.MaxFloat64, 0.0, 0
	plafond := -1.0
	if n := len(x.Points); n > 0 {
		plafond = x.Points[n-1].Nc
	}
	for _, g := range d.Groupes {
		if g.Qte <= 0 {
			continue
		}
		p := x.PointsDe(g.Nc)
		e.Points += p * g.Qte
		e.Detail = append(e.Detail, fmt.Sprintf("%d × %s (NC %s) : %d × %d = %d points", g.Qte, g.Nom, FmtNC(g.Nc), g.Qte, p, p*g.Qte))
		ncMin, ncMax = math.Min(ncMin, g.Nc), math.Max(ncMax, g.Nc)
		sortes++
		if plafond >= 0 && g.Nc > plafond {
			e.Alertes = append(e.Alertes, fmt.Sprintf("%s : NC au-delà de la table (%s), points extrapolés.", g.Nom, FmtNC(plafond)))
		}
	}
	e.PointsAjustes = float64(e.Points)
	if ref := x.GroupeReference; ref > 0 && d.NbPj > 0 && d.NbPj != ref {
		e.PointsAjustes = float64(e.Points) * float64(ref) / float64(d.NbPj)
		e.Detail = append(e.Detail, fmt.Sprintf("Groupe de %d PJ : %d × %d / %d = %.0f points pour un groupe de référence de %d", d.NbPj, e.Points, ref, d.NbPj, e.PointsAjustes, ref))
	}
	if e.Points == 0 {
		e.Cle, e.Verdict = "vide", "Aucune créature."
		return e
	}
	e.Niveau = x.NiveauDe(e.PointsAjustes)
	for _, c := range x.Conditions {
		if c.Cle != d.Condition {
			continue
		}
		e.Niveau += c.Niveau
		ligne := c.Libelle
		if c.Niveau != 0 {
			ligne += fmt.Sprintf(" : niveau %s", signe(int(c.Niveau)))
		}
		if c.Detail != "" {
			ligne += " — " + c.Detail
		}
		e.Detail = append(e.Detail, ligne)
	}
	for _, a := range x.Alertes {
		if (a.Quand == "nc" && ncMax >= a.Seuil) || (a.Quand == "ecart_nc" && sortes > 1 && ncMax-ncMin >= a.Seuil) {
			e.Alertes = append(e.Alertes, a.Texte)
		}
	}
	if d.NiveauPj <= 0 {
		e.Cle, e.Verdict = "inconnu", fmt.Sprintf("Rencontre de niveau %s.", FmtNC(e.Niveau))
		return e
	}
	e.Ecart = e.Niveau - d.NiveauPj
	for _, v := range x.Verdicts {
		if v.JusquA == nil || e.Ecart < *v.JusquA || (v.Inclus && e.Ecart == *v.JusquA) {
			e.Cle, e.Verdict, e.Ton = v.Cle, v.Texte, v.Ton
			return e
		}
	}
	e.Cle, e.Verdict = "inconnu", fmt.Sprintf("Rencontre de niveau %s.", FmtNC(e.Niveau))
	return e
}
