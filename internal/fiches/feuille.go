package fiches

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"github.com/Tchyze/cof_dm/internal/reference"
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
	// Utilisations : ce qui a été utilisé des capacités à utilisations comptées (par jour, par repos,
	// à remettre à zéro à la main ; celles « par combat » se comptent dans le combat), par CleCapacite.
	Utilisations map[string]Utilisation `json:"utilisations,omitempty"`
	// Niveaux : l'historique des montées de niveau (niveaux.go) ; PointsCapacite : les points de
	// capacité gardés pour le niveau suivant, quand les règles le permettent.
	Niveaux        []Montee `json:"niveaux,omitempty"`
	PointsCapacite int      `json:"points_capacite,omitempty"`
	// Effets : ce que les capacités atteintes ajoutent (« A + b »), calculé à la lecture, jamais
	// rangé (Stockage).
	Effets *Effets `json:"effets,omitempty"`

	effets       []EffetVoie    // bonus des capacités atteintes, donnés par PrendreEffets
	effetsConnus bool           // false : feuille relue sans la bibliothèque (synchronisation, restauration)
	bonus        map[string]int // ce que les effets ajoutent, par clé de Effets.Base
}

// Utilisation : combien de fois une capacité a servi depuis sa dernière remise à zéro, et quand
// celle-ci vient (jour, repos:<clé>, main).
type Utilisation struct {
	Par string `json:"par"`
	N   int    `json:"n"`
}

// CleCapacite : la clé d'une capacité dans Utilisations, tirée du nom de sa voie et de son rang.
func CleCapacite(voie string, rang int) string {
	return reference.Cle(voie) + "#" + strconv.Itoa(rang)
}

// RemettreAZero oublie les utilisations dont la période est l'une de celles données.
func (f *Feuille) RemettreAZero(pars ...string) int {
	n := 0
	for k, u := range f.Utilisations {
		if dans(pars, u.Par) {
			delete(f.Utilisations, k)
			n++
		}
	}
	return n
}

// EffetVoie : un bonus permanent d'une capacité que le PJ a atteinte, repris de la bibliothèque. Il
// vaut Valeur (× Rang, le rang atteint dans la voie, si ParRang), + le niveau si Niveau, + le
// modificateur de Carac.
type EffetVoie struct {
	Cible    string
	Valeur   int
	ParRang  bool
	Niveau   bool
	Carac    string
	Rang     int
	Capacite string
	Voie     string
	RangCapa int // rang de la capacité dans sa voie
}

// Effets : pour chaque valeur que les capacités changent (FOR…CHA, def, init, pv, pm, rd, att_C,
// att_D, att_M), sa valeur sans elles (Base), ce qu'elles ajoutent (Bonus, non nul), et le détail.
type Effets struct {
	Base   map[string]int `json:"base"`
	Bonus  map[string]int `json:"bonus"`
	Detail []EffetDetail  `json:"detail"`
}

type EffetDetail struct {
	Cible    string `json:"cible"`
	Valeur   int    `json:"valeur"`
	Capacite string `json:"capacite"`
	Voie     string `json:"voie"`
	Rang     int    `json:"rang"`
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
	return normaliser(raw, niveau, rg, nil, true)
}

// NormaliserFeuilleAvec : de même, avec les bonus des capacités que le PJ a atteintes.
func NormaliserFeuilleAvec(raw []byte, niveau int, rg *regles.Feuille, effets []EffetVoie) (*Feuille, error) {
	return normaliser(raw, niveau, rg, effets, true)
}

// RelireFeuille : une feuille rangée, relue sans règles ni bibliothèque. Les PV et les PM actuels ne
// sont pas ramenés sous leur maximum, que les bonus des capacités peuvent dépasser.
func RelireFeuille(raw []byte, niveau int) (*Feuille, error) {
	return normaliser(raw, niveau, nil, nil, false)
}

func normaliser(raw []byte, niveau int, rg *regles.Feuille, effets []EffetVoie, connus bool) (*Feuille, error) {
	f := FeuilleNeuve(rg)
	if err := decoder(raw, f); err != nil {
		return nil, err
	}
	f.effets, f.effetsConnus = effets, connus
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
	if err := f.verifierNiveaux(); err != nil {
		return nil, err
	}
	if len(f.Utilisations) > 200 {
		return nil, invalide("utilisations : trop de capacités.")
	}
	for k, u := range f.Utilisations {
		if u.N <= 0 || u.Par == "" || u.Par == "combat" {
			delete(f.Utilisations, k)
			continue
		}
		if len(k) > 160 || len(u.Par) > 80 || u.N > 999 {
			return nil, invalide("utilisations.%s : invalide.", k)
		}
	}
	if len(f.Utilisations) == 0 {
		f.Utilisations = nil
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

// PrendreEffets donne à la feuille les bonus des capacités que le PJ a atteintes ; Deriver les
// applique ensuite.
func (f *Feuille) PrendreEffets(effets []EffetVoie) {
	f.effets, f.effetsConnus = effets, true
}

// calcul : les valeurs dérivées d'une feuille, avec les bonus donnés (par clé de Effets.Base).
type calcul struct {
	valeurs    Valeurs
	carac      Carac
	def, init  int
	pv, pm, rd int
	attaque    map[string]int
}

func (f *Feuille) calculer(niveau int, rg *regles.Feuille, bonus map[string]int) calcul {
	c := calcul{}
	for _, k := range Caracs {
		c.valeurs.Set(k, f.Valeurs.Get(k)+bonus[k])
		c.carac.Set(k, Modificateur(c.valeurs.Get(k)))
	}
	c.init = c.valeurs.DEX + f.InitDivers + bonus["init"]
	c.def = rg.DefenseBase + c.carac.DEX + f.Armure.Bonus + f.Bouclier.Bonus + f.DefDivers + bonus["def"]
	c.pv, c.pm, c.rd = f.Pv+bonus["pv"], f.PmMax+bonus["pm"], f.Rd+bonus["rd"]
	c.attaque = map[string]int{
		"C": niveau + c.carac.FOR + f.AttDivers.C + bonus["att_C"],
		"D": niveau + c.carac.DEX + f.AttDivers.D + bonus["att_D"],
		"M": niveau + c.carac.Get(f.CaracMagie) + f.AttDivers.M + bonus["att_M"],
	}
	return c
}

// carte : les valeurs que les capacités peuvent changer, par clé.
func (c calcul) carte() map[string]int {
	m := map[string]int{"def": c.def, "init": c.init, "pv": c.pv, "pm": c.pm, "rd": c.rd}
	for _, k := range Caracs {
		m[k] = c.valeurs.Get(k)
	}
	for t, v := range c.attaque {
		m["att_"+t] = v
	}
	return m
}

// clesCible : les clés de Effets.Base qu'une cible d'effet augmente.
func clesCible(cible string) []string {
	switch cible {
	case "att":
		return []string{"att_C", "att_D", "att_M"}
	case "att_c", "att_d", "att_m":
		return []string{"att_" + strings.ToUpper(cible[4:])}
	}
	return []string{cible}
}

// resoudre : la valeur de chaque effet. Ceux qui changent une caractéristique d'abord, avec les
// modificateurs sans effet ; les autres avec les caractéristiques changées.
func (f *Feuille) resoudre(niveau int) (map[string]int, []EffetDetail) {
	bonus, detail := map[string]int{}, []EffetDetail{}
	valeur := func(e EffetVoie, carac Carac) int {
		v := e.Valeur
		if e.ParRang {
			v *= e.Rang
		}
		if e.Niveau {
			v += niveau
		}
		if e.Carac != "" {
			v += carac.Get(e.Carac)
		}
		return v
	}
	var carac Carac
	for _, k := range Caracs {
		carac.Set(k, Modificateur(f.Valeurs.Get(k)))
	}
	for passe := range 2 {
		for _, e := range f.effets {
			if dans(Caracs, e.Cible) != (passe == 0) {
				continue
			}
			v := valeur(e, carac)
			for _, k := range clesCible(e.Cible) {
				bonus[k] += v
			}
			detail = append(detail, EffetDetail{Cible: e.Cible, Valeur: v, Capacite: e.Capacite, Voie: e.Voie, Rang: e.RangCapa})
		}
		for _, k := range Caracs {
			carac.Set(k, Modificateur(f.Valeurs.Get(k)+bonus[k]))
		}
	}
	return bonus, detail
}

// Deriver recalcule modificateurs, initiative, DEF et bonus d'attaque, avec les bonus des
// capacités, et borne les ressources. Les valeurs saisies (caractéristiques, PV max, PM max, RD)
// ne changent pas : leurs totaux se lisent par PvMax, PmMaxTotal, RdTotal et ValeurTotale.
func (f *Feuille) Deriver(niveau int, rg *regles.Feuille) {
	bonus, detail := f.resoudre(niveau)
	t := f.calculer(niveau, rg, bonus)
	f.Carac, f.Init, f.Defense = t.carac, t.init, t.def
	for i := range f.Attaques {
		f.Attaques[i].Bonus = t.attaque[f.Attaques[i].Type] + f.Attaques[i].Mod
	}
	f.Effets, f.bonus = nil, map[string]int{}
	if len(detail) > 0 {
		base, total := f.calculer(niveau, rg, nil).carte(), t.carte()
		for k, v := range total {
			if d := v - base[k]; d != 0 {
				f.bonus[k] = d
			}
		}
		f.Effets = &Effets{Base: base, Bonus: f.bonus, Detail: detail}
	}
	// relue sans la bibliothèque, une feuille garde ses PV et ses PM actuels : ils peuvent dépasser
	// le maximum saisi, grâce aux capacités
	if f.PvActuel != nil {
		v := max(0, *f.PvActuel)
		if f.effetsConnus {
			v = min(t.pv, v)
		}
		f.PvActuel = &v
	}
	if f.effetsConnus {
		f.Pm = min(f.Pm, t.pm)
	}
	f.Pc, f.Pr = min(f.Pc, f.PcMax), min(f.Pr, f.PrMax)
}

// PvMax, PmMaxTotal, RdTotal, ValeurTotale : les valeurs saisies, avec les bonus des capacités.
func (f *Feuille) PvMax() int                { return f.Pv + f.bonus["pv"] }
func (f *Feuille) PmMaxTotal() int           { return f.PmMax + f.bonus["pm"] }
func (f *Feuille) RdTotal() int              { return f.Rd + f.bonus["rd"] }
func (f *Feuille) ValeurTotale(k string) int { return f.Valeurs.Get(k) + f.bonus[k] }

// Stockage : la feuille telle qu'elle se range, sans ce que la bibliothèque y ajoute.
func (f Feuille) Stockage() []byte {
	f.Effets = nil
	return f.JSON()
}

// PourJoueur renvoie la feuille telle que le joueur la voit : sans les notes du MJ, ni le détail
// des jets cachés de ses montées de niveau.
func (f Feuille) PourJoueur() Feuille {
	f.Notes = ""
	if len(f.Niveaux) > 0 {
		f.Niveaux = append([]Montee(nil), f.Niveaux...)
		for i := range f.Niveaux {
			if f.Niveaux[i].Cache {
				f.Niveaux[i].Jet = ""
			}
		}
	}
	return f
}

// JSON de la feuille.
func (f *Feuille) JSON() []byte {
	b, _ := json.Marshal(f)
	return b
}
