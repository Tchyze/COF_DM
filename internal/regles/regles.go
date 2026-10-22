// Package regles applique les règles du jeu à partir d'un fichier de règles importé par le MJ et
// rangé dans l'univers. Le code ne contient aucune donnée des livres : ni liste, ni table, ni nom,
// ni texte. Sans fichier, les outils qui en dépendent (créateur de monstre) ne sont pas proposés et
// les fiches se saisissent en texte libre.
package regles

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
)

const (
	Format  = "cof-dm-regles"
	Version = 1
)

// Regles est le fichier de règles d'un univers.
type Regles struct {
	Format   string    `json:"format"`
	Version  int       `json:"version"`
	Titre    string    `json:"titre,omitempty"`
	Listes   Listes    `json:"listes"`
	Creature *Creation `json:"creature,omitempty"`
	// Personnage : bornes et valeurs de départ de la feuille de personnage.
	Personnage *Feuille `json:"personnage,omitempty"`
	// Rencontre : difficulté des rencontres.
	Rencontre *Rencontres `json:"rencontre,omitempty"`
	// Combat : valeurs du combat et états des combattants.
	Combat *Combat `json:"combat,omitempty"`
}

// Listes de choix proposés dans les fiches.
type Listes struct {
	Peuples    []string `json:"peuples"`
	Profils    []string `json:"profils"`
	PeuplesPNJ []string `json:"peuples_pnj"`
	ProfilsPNJ []string `json:"profils_pnj"`
	Tailles    []string `json:"tailles"`
	Categories []string `json:"categories"`
}

// Profil de base d'une créature, par taille et par archétype.
type Profil struct {
	Nc     float64 `json:"nc"`
	For    int     `json:"FOR"`
	Dex    int     `json:"DEX"`
	Con    int     `json:"CON"`
	DexSup bool    `json:"dexSup"`
	ForSup bool    `json:"forSup"`
	Def    int     `json:"def"`
	Pv     int     `json:"pv"`
	Init   int     `json:"init"`
	Att    int     `json:"att"`
	Dm     string  `json:"dm"`
}

type Archetype struct {
	Cle         string `json:"cle"`
	Description string `json:"description"`
}

// Boss : ce que chaque rang ajoute.
type Boss struct {
	Cle         string `json:"cle"`
	Libelle     string `json:"libelle"`
	Att         int    `json:"att"`
	Dm          int    `json:"dm"`
	DmDes       int    `json:"dmDes"` // d6 supplémentaires
	Def         int    `json:"def"`
	Pv          int    `json:"pv"`
	Description string `json:"description"`
}

// Voie de créature : capacités par rang, et ce qu'elle apporte au profil.
type Voie struct {
	Cle       string   `json:"cle"`
	Nom       string   `json:"nom"`
	Page      int      `json:"page"`
	Sup       []string `json:"sup"`   // caractéristiques supérieures accordées
	SupChoix  bool     `json:"choix"` // INT, SAG ou CHA au choix
	Rangs     []string `json:"rangs"` // nom des capacités, par rang
	Rd        []int    `json:"rd"`    // RD selon le rang atteint
	InitBonus []int    `json:"init"`  // bonus d'initiative selon le rang
	Remarque  string   `json:"remarque"`
}

type MotCle struct {
	Nom    string `json:"nom"`
	Valeur int    `json:"valeur"`
}

// Arme : dé de dégâts qui remplace celui de l'archétype (vide : celui de l'archétype).
type Arme struct {
	Cle     string `json:"cle"`
	Libelle string `json:"libelle"`
	De      string `json:"de"`
}

// Creation : tables et paramètres du créateur de monstre.
type Creation struct {
	Archetypes      []Archetype                  `json:"archetypes"`
	Table           map[string]map[string]Profil `json:"table"` // taille → archétype (« unique » : un seul archétype)
	Boss            []Boss                       `json:"boss"`
	Voies           []Voie                       `json:"voies"`
	MotsCles        map[string][]MotCle          `json:"mots_cles"`
	Armes           []Arme                       `json:"armes"`
	TaillesSansArme []string                     `json:"tailles_sans_arme"`
	NcRangBoss      float64                      `json:"nc_par_rang_boss"`
	NcRangVoie      float64                      `json:"nc_par_rang_voie"`
	NcRangVoieBas   float64                      `json:"nc_par_rang_voie_faible"` // pour une créature de NC faible, sans rang de boss
	NcFaible        float64                      `json:"nc_faible"`
	NcAttMultiple   float64                      `json:"nc_attaque_multiple"`
	RangsMaxVoie    int                          `json:"rangs_max_voie"`
	VoiesConseil    int                          `json:"voies_conseillees"`
	Textes          map[string]string            `json:"textes"`
}

// Feuille : ce que les règles disent de la feuille de personnage. Sans elles, la feuille n'impose
// aucune borne et ne propose aucune valeur de départ.
type Feuille struct {
	NiveauMax    int               `json:"niveau_max"`   // 0 : pas de limite
	ValeurMin    int               `json:"valeur_min"`   // caractéristiques
	ValeurMax    int               `json:"valeur_max"`   // 0 : pas de limite
	DesDeVie     []string          `json:"des_de_vie"`   // vide : tout dé « dN »
	DeDeVie      string            `json:"de_de_vie"`    // proposé sur une feuille neuve
	CaracsMagie  []string          `json:"caracs_magie"` // vide : n'importe quelle caractéristique
	CaracMagie   string            `json:"carac_magie"`  // proposée sur une feuille neuve
	DefenseBase  int               `json:"defense_base"` // DEF sans rien : base + mod. DEX + armure…
	PcMax        int               `json:"pc_max"`       // valeurs de départ des ressources
	PrMax        int               `json:"pr_max"`
	RangMax      int               `json:"rang_max"`      // 0 : pas de limite
	OriginesVoie []string          `json:"origines_voie"` // vide : texte libre
	Repos        []Repos           `json:"repos,omitempty"`
	Montee       *Montee           `json:"montee,omitempty"` // absente : montée de niveau en saisie libre
	Textes       map[string]string `json:"textes,omitempty"`
}

// Montee : ce que gagne un PJ qui passe un niveau. PV gagnés : somme de Pv et de PvFixe, chaque dé
// de vie étant lancé en vrai et saisi ; sans formule, les PV se saisissent. Points : les points de
// capacité gagnés, que coûtent les rangs de voie (Couts : le coût de chaque rang, le dernier valant
// pour les suivants ; vide : 1 ; CoutsOrigine : de même pour les voies d'une origine donnée) ;
// NiveauMin : le niveau qu'il faut pour prendre chaque rang (0 : aucun). Sans points, les rangs se
// prennent librement. Pm : les PM max gagnés selon le profil du PJ. Etapes : ce que le joueur note
// en plus à certains niveaux.
type Montee struct {
	Pv           []string         `json:"pv,omitempty"` // somme de « dv » (un dé de vie, saisi), « con » (mod. CON), « niveau » (le nouveau)
	PvFixe       int              `json:"pv_fixe,omitempty"`
	PvMin        int              `json:"pv_min,omitempty"` // PV gagnés au moins
	Points       int              `json:"points,omitempty"`
	Couts        []int            `json:"couts,omitempty"`
	CoutsOrigine map[string][]int `json:"couts_origine,omitempty"`
	NiveauMin    []int            `json:"niveau_min,omitempty"`
	Report       bool             `json:"report,omitempty"` // les points non dépensés se gardent pour le niveau suivant
	Pm           []GainPm         `json:"pm,omitempty"`
	Etapes       []Etape          `json:"etapes,omitempty"`
	Aide         string           `json:"aide,omitempty"`
}

// GainPm : les PM max que gagne à chaque niveau un PJ de l'un de ces profils.
type GainPm struct {
	Profils []string `json:"profils"`
	Gain    int      `json:"gain"`
}

// Etape : un écran de plus de la montée, aux niveaux donnés : le joueur y note un choix (une
// capacité…), ajouté aux capacités de sa feuille.
type Etape struct {
	Cle     string `json:"cle"`
	Libelle string `json:"libelle"`
	Aide    string `json:"aide,omitempty"`
	Niveaux []int  `json:"niveaux"`
}

// Formule : les PV gagnés se calculent (dés saisis compris) ; sinon, ils se saisissent.
func (m *Montee) Formule() bool { return m != nil && (len(m.Pv) > 0 || m.PvFixe != 0) }

// NbDes : le nombre de dés de vie à saisir.
func (m *Montee) NbDes() int {
	n := 0
	if m != nil {
		for _, t := range m.Pv {
			if t == "dv" {
				n++
			}
		}
	}
	return n
}

// Cout : ce que coûte le rang r (à partir de 1) d'une voie de cette origine.
func (m *Montee) Cout(origine string, r int) int {
	if m == nil {
		return 1
	}
	couts := m.Couts
	if c, ok := m.CoutsOrigine[origine]; ok && len(c) > 0 {
		couts = c
	}
	switch {
	case len(couts) == 0:
		return 1
	case r <= len(couts):
		return couts[r-1]
	}
	return couts[len(couts)-1]
}

// NiveauPour : le niveau qu'il faut pour prendre le rang r (0 : aucun).
func (m *Montee) NiveauPour(r int) int {
	if m == nil || r < 1 || r > len(m.NiveauMin) {
		return 0
	}
	return m.NiveauMin[r-1]
}

// GainPmDe : les PM max gagnés par niveau par un PJ de ce profil (0 : aucun).
func (m *Montee) GainPmDe(profil string) int {
	if m == nil || strings.TrimSpace(profil) == "" {
		return 0
	}
	for _, g := range m.Pm {
		for _, p := range g.Profils {
			if strings.EqualFold(strings.TrimSpace(p), strings.TrimSpace(profil)) {
				return g.Gain
			}
		}
	}
	return 0
}

// EtapesDu : les étapes en plus d'une montée à ce niveau.
func (m *Montee) EtapesDu(niveau int) []Etape {
	var out []Etape
	if m != nil {
		for _, e := range m.Etapes {
			if slices.Contains(e.Niveaux, niveau) {
				out = append(out, e)
			}
		}
	}
	return out
}

// termePv : un terme d'une formule de PV (repos, montée de niveau).
func termePv(t string) bool { return t == "dv" || t == "con" || t == "niveau" }

// Repos : une façon de récupérer proposée sur la feuille (un bouton). Ce qu'elle coûte et ce
// qu'elle rend est décrit ici, pas dans le code.
type Repos struct {
	Cle     string   `json:"cle"`
	Libelle string   `json:"libelle"`
	Aide    string   `json:"aide,omitempty"`
	CoutPr  int      `json:"cout_pr,omitempty"` // points de récupération dépensés
	GainPr  int      `json:"gain_pr,omitempty"` // points de récupération rendus
	Pv      []string `json:"pv,omitempty"`      // PV rendus : somme de « dv » (un dé de vie lancé), « con » (mod. CON), « niveau »
	PvMax   bool     `json:"pv_max,omitempty"`  // PV remis au maximum
	PmMax   bool     `json:"pm_max,omitempty"`  // PM remis au maximum
	// NouveauJour : ce repos vaut un nouveau jour (les capacités « par jour » redeviennent
	// utilisables) ; il remet toujours à zéro celles « par » ce repos.
	NouveauJour bool `json:"nouveau_jour,omitempty"`
}

// Vide : un univers sans fichier de règles.
func Vide() *Regles { return &Regles{Format: Format, Version: Version} }

// Lire décode un fichier de règles et le vérifie.
func Lire(data []byte) (*Regles, error) {
	r := &Regles{}
	if err := json.Unmarshal(data, r); err != nil {
		return nil, fmt.Errorf("fichier de règles illisible : %v", err)
	}
	if r.Format != Format {
		return nil, fmt.Errorf("ce fichier n'est pas un fichier de règles (format « %s » attendu)", Format)
	}
	return r, r.Normaliser()
}

// Relire lit des règles déjà rangées dans un univers ou dans un export. Une partie « rencontre »
// d'une forme inconnue (rangée avant qu'elle ait une structure) est laissée de côté : le reste des
// règles reste utilisable.
func Relire(data []byte) (*Regles, error) {
	r := &Regles{}
	if err := json.Unmarshal(data, r); err != nil {
		return nil, fmt.Errorf("fichier de règles illisible : %v", err)
	}
	if r.Rencontre != nil && r.Rencontre.illisible != nil {
		r.Rencontre = nil
	}
	if r.Format != Format {
		return nil, fmt.Errorf("ce fichier n'est pas un fichier de règles (format « %s » attendu)", Format)
	}
	return r, r.Normaliser()
}

func propres(l []string) []string {
	out := []string{}
	vus := map[string]bool{}
	for _, s := range l {
		if s = strings.TrimSpace(s); s != "" && !vus[s] && len([]rune(s)) <= 80 {
			vus[s] = true
			out = append(out, s)
		}
	}
	return out
}

// Normaliser met les règles en ordre et refuse celles qui ne tiendraient pas debout.
func (r *Regles) Normaliser() error {
	r.Format, r.Version = Format, Version
	l := &r.Listes
	l.Peuples, l.Profils, l.PeuplesPNJ, l.ProfilsPNJ = propres(l.Peuples), propres(l.Profils), propres(l.PeuplesPNJ), propres(l.ProfilsPNJ)
	l.Tailles, l.Categories = propres(l.Tailles), propres(l.Categories)
	if r.Combat != nil {
		if err := r.Combat.normaliser(); err != nil {
			return err
		}
	}
	if x := r.Rencontre; x != nil {
		if x.illisible == nil && len(x.Points) == 0 {
			r.Rencontre = nil // partie vide : comme absente
		} else if err := x.normaliser(); err != nil {
			return err
		}
	}
	if f := r.Personnage; f != nil {
		f.DesDeVie, f.CaracsMagie, f.OriginesVoie = propres(f.DesDeVie), propres(f.CaracsMagie), propres(f.OriginesVoie)
		for i, x := range f.Repos {
			if strings.TrimSpace(x.Libelle) == "" || x.CoutPr < 0 || x.GainPr < 0 {
				return fmt.Errorf("personnage.repos.%d : libellé attendu, coût et gain positifs", i)
			}
			for _, t := range x.Pv {
				if !termePv(t) {
					return fmt.Errorf("personnage.repos.%d.pv : dv, con ou niveau", i)
				}
			}
		}
		if len(f.Repos) > 12 {
			return fmt.Errorf("personnage.repos : douze au plus")
		}
		if m := f.Montee; m != nil {
			for _, t := range m.Pv {
				if !termePv(t) {
					return fmt.Errorf("personnage.montee.pv : dv, con ou niveau")
				}
			}
			if len(m.Pv) > 6 || len(m.Couts) > 20 || len(m.NiveauMin) > 20 || len(m.CoutsOrigine) > 12 || len(m.Pm) > 20 || len(m.Etapes) > 12 || len([]rune(m.Aide)) > 400 {
				return fmt.Errorf("personnage.montee : trop long")
			}
			coutMauvais := func(c int) bool { return c < 1 || c > 99 }
			if m.PvMin < 0 || m.Points < 0 || slices.ContainsFunc(m.Couts, coutMauvais) || slices.ContainsFunc(m.NiveauMin, func(n int) bool { return n < 0 }) {
				return fmt.Errorf("personnage.montee : PV minimum et points positifs, coûts de 1 à 99, niveaux positifs")
			}
			for o, c := range m.CoutsOrigine {
				if strings.TrimSpace(o) == "" || len(c) > 20 || slices.ContainsFunc(c, coutMauvais) {
					return fmt.Errorf("personnage.montee.couts_origine.%s : coûts de 1 à 99", o)
				}
			}
			for i, g := range m.Pm {
				if g.Gain < 0 || g.Gain > 99 || len(g.Profils) == 0 {
					return fmt.Errorf("personnage.montee.pm.%d : des profils et un gain de 0 à 99", i)
				}
			}
			vues := map[string]bool{}
			for i := range m.Etapes {
				e := &m.Etapes[i]
				e.Cle, e.Libelle = strings.TrimSpace(e.Cle), strings.TrimSpace(e.Libelle)
				if e.Cle == "" || e.Libelle == "" || vues[e.Cle] || len(e.Niveaux) == 0 || len([]rune(e.Aide)) > 400 {
					return fmt.Errorf("personnage.montee.etapes.%d : clé unique, libellé et niveaux attendus", i)
				}
				vues[e.Cle] = true
			}
		}
		if f.NiveauMax < 0 || f.ValeurMax < 0 || f.RangMax < 0 || f.PcMax < 0 || f.PrMax < 0 || (f.ValeurMax > 0 && f.ValeurMin > f.ValeurMax) {
			return fmt.Errorf("personnage : bornes incohérentes")
		}
	}
	c := r.Creature
	if c == nil {
		return nil
	}
	if len(c.Table) == 0 {
		return fmt.Errorf("creature.table : aucune taille")
	}
	for taille, t := range c.Table {
		if len(t) == 0 {
			return fmt.Errorf("creature.table.%s : aucun archétype", taille)
		}
		for arch, p := range t {
			if p.Pv < 1 || p.Nc < 0 {
				return fmt.Errorf("creature.table.%s.%s : PV d'au moins 1 et NC positif attendus", taille, arch)
			}
		}
	}
	vues := map[string]bool{}
	for i := range c.Voies {
		v := &c.Voies[i]
		if v.Cle = strings.TrimSpace(v.Cle); v.Cle == "" || vues[v.Cle] {
			return fmt.Errorf("creature.voies.%d : clé absente ou en double", i)
		}
		vues[v.Cle] = true
	}
	for i, b := range c.Boss {
		if strings.TrimSpace(b.Cle) == "" {
			return fmt.Errorf("creature.boss.%d : clé absente", i)
		}
	}
	if c.RangsMaxVoie < 1 {
		c.RangsMaxVoie = 1
	}
	if c.Textes == nil {
		c.Textes = map[string]string{}
	}
	return nil
}

// Fusionner remplace les parties que n apporte : une liste vide ou une partie absente laisse celle
// de r en place.
func (r *Regles) Fusionner(n *Regles) {
	if n.Titre != "" {
		r.Titre = n.Titre
	}
	garder := func(dst *[]string, src []string) {
		if len(src) > 0 {
			*dst = src
		}
	}
	garder(&r.Listes.Peuples, n.Listes.Peuples)
	garder(&r.Listes.Profils, n.Listes.Profils)
	garder(&r.Listes.PeuplesPNJ, n.Listes.PeuplesPNJ)
	garder(&r.Listes.ProfilsPNJ, n.Listes.ProfilsPNJ)
	garder(&r.Listes.Tailles, n.Listes.Tailles)
	garder(&r.Listes.Categories, n.Listes.Categories)
	if n.Creature != nil {
		r.Creature = n.Creature
	}
	if n.Personnage != nil {
		r.Personnage = n.Personnage
	}
	if n.Combat != nil {
		r.Combat = n.Combat
	}
	if n.Rencontre != nil && n.Rencontre.illisible == nil {
		r.Rencontre = n.Rencontre
	}
}

// EstVide : aucune règle n'a été importée.
func (r *Regles) EstVide() bool {
	l := r.Listes
	return r.Creature == nil && r.Personnage == nil && r.Rencontre == nil && r.Combat == nil &&
		len(l.Peuples)+len(l.Profils)+len(l.PeuplesPNJ)+len(l.ProfilsPNJ)+len(l.Tailles)+len(l.Categories) == 0
}

// ---------------------------------------------------------------- calcul d'une créature

type DemandeCreature struct {
	Taille      string         `json:"taille"`
	Archetype   string         `json:"archetype"`
	Categorie   string         `json:"categorie"`
	TypeBoss    string         `json:"type_boss"`
	RangsBoss   int            `json:"rangs_boss"`
	Voies       map[string]int `json:"voies"` // clé → rang
	SupMagie    string         `json:"sup_magie"`
	Int         int            `json:"INT"`
	Sag         int            `json:"SAG"`
	Cha         int            `json:"CHA"`
	AttMultiple bool           `json:"attaque_multiple"`
}

type Creature struct {
	Nc          float64        `json:"nc"`
	Carac       map[string]int `json:"carac"`
	Superieures []string       `json:"superieures"`
	Defense     int            `json:"defense"`
	Pv          int            `json:"pv"`
	Init        int            `json:"init"`
	Rd          int            `json:"rd"`
	Att         int            `json:"att"`
	Dm          string         `json:"dm"`
	Conception  string         `json:"conception"`
	Capacites   string         `json:"capacites"`
	Detail      []string       `json:"detail"`
	Alertes     []string       `json:"alertes"`
}

// ajouterDm : "2d6+5" + 2 → "2d6+7" ; + 1d6 → "3d6+7" quand le dé de base est un d6.
func ajouterDm(dm string, bonus, des int) string {
	nb, faces, fixe := 1, 0, 0
	var n1, f1, b1 int
	if k, _ := fmt.Sscanf(dm, "%dd%d%d", &n1, &f1, &b1); k >= 2 {
		nb, faces, fixe = n1, f1, b1
	} else if k, _ := fmt.Sscanf(dm, "%d", &b1); k == 1 && !strings.Contains(dm, "d") {
		fixe = b1
		faces = 0
	}
	fixe += bonus
	extra := ""
	if des > 0 {
		if faces == 6 {
			nb += des
		} else {
			extra = fmt.Sprintf("+%dd6", des)
		}
	}
	var out string
	if faces == 0 {
		out = fmt.Sprintf("%d", max(1, fixe))
	} else {
		out = fmt.Sprintf("%dd%d", nb, faces)
		if fixe > 0 {
			out += fmt.Sprintf("+%d", fixe)
		} else if fixe < 0 {
			out += fmt.Sprintf("%d", fixe)
		}
	}
	return out + extra
}

func signe(n int) string {
	if n >= 0 {
		return fmt.Sprintf("+%d", n)
	}
	return fmt.Sprint(n)
}

func FmtNC(nc float64) string {
	if nc == 0.5 {
		return "½"
	}
	if nc == math.Trunc(nc) {
		return fmt.Sprint(int(nc))
	}
	return strings.Replace(fmt.Sprintf("%.1f", nc), ".", ",", 1)
}

func (c *Creation) voie(cle string) (Voie, bool) {
	for _, v := range c.Voies {
		if v.Cle == cle {
			return v, true
		}
	}
	return Voie{}, false
}

func (c *Creation) boss(cle string) (Boss, bool) {
	for _, b := range c.Boss {
		if b.Cle == cle {
			return b, true
		}
	}
	if len(c.Boss) > 0 {
		return c.Boss[0], true
	}
	return Boss{}, false
}

// auRang renvoie la valeur d'une liste « par rang » au rang r (0 au-delà de ce que la liste donne).
func auRang(l []int, r int) int {
	if r >= 1 && r <= len(l) {
		return l[r-1]
	}
	return 0
}

// Calculer conçoit une créature : le profil de sa taille et de son archétype, puis ce qu'ajoutent
// ses rangs de boss et ses voies de créature.
func (t *Creation) Calculer(d DemandeCreature) (Creature, error) {
	tab, ok := t.Table[d.Taille]
	if !ok {
		return Creature{}, fmt.Errorf("taille inconnue : %s", d.Taille)
	}
	arch := d.Archetype
	if _, ok := tab["unique"]; ok {
		arch = "unique"
	}
	p, ok := tab[arch]
	if !ok {
		return Creature{}, fmt.Errorf("archétype inconnu : %s", d.Archetype)
	}
	c := Creature{Nc: p.Nc, Defense: p.Def, Pv: p.Pv, Init: p.Init, Att: p.Att, Dm: p.Dm,
		Carac:  map[string]int{"FOR": p.For, "DEX": p.Dex, "CON": p.Con, "INT": d.Int, "SAG": d.Sag, "CHA": d.Cha},
		Detail: []string{}, Alertes: []string{}}
	sup := map[string]bool{"DEX": p.DexSup, "FOR": p.ForSup}
	nomArch := "Archétype " + d.Archetype
	if arch == "unique" {
		nomArch = "Archétype " + d.Taille
	}
	c.Detail = append(c.Detail, fmt.Sprintf("%s, taille %s : NC %s, DEF %d, PV %d, Init %d, attaque %s, DM %s",
		nomArch, d.Taille, FmtNC(p.Nc), p.Def, p.Pv, p.Init, signe(p.Att), p.Dm))
	conception := []string{nomArch}

	if b, ok := t.boss(d.TypeBoss); ok && d.RangsBoss > 0 {
		r := d.RangsBoss
		c.Nc += t.NcRangBoss * float64(r)
		c.Att += b.Att * r
		c.Defense += b.Def * r
		c.Pv += b.Pv * r
		c.Dm = ajouterDm(c.Dm, b.Dm*r, b.DmDes*r)
		c.Detail = append(c.Detail, fmt.Sprintf("%s rang %d : NC +%s, attaque +%d, DEF +%d, PV +%d, DM %s",
			b.Libelle, r, FmtNC(t.NcRangBoss*float64(r)), b.Att*r, b.Def*r, b.Pv*r, c.Dm))
		conception = append(conception, fmt.Sprintf("%s rang %d", b.Libelle, r))
	}

	cles := make([]string, 0, len(d.Voies))
	for k := range d.Voies {
		cles = append(cles, k)
	}
	sort.Strings(cles)
	var caps []string
	rangsVoies, nbVoies := 0, 0
	for _, k := range cles {
		r := d.Voies[k]
		v, ok := t.voie(k)
		if !ok || r <= 0 {
			continue
		}
		r = min(r, t.RangsMaxVoie)
		rangsVoies += r
		nbVoies++
		parRang := t.NcRangVoie
		if p.Nc <= t.NcFaible && d.RangsBoss == 0 {
			parRang = t.NcRangVoieBas
		}
		c.Nc += parRang * float64(r)
		for _, s := range v.Sup {
			sup[s] = true
		}
		if v.SupChoix {
			s := d.SupMagie
			if s != "INT" && s != "SAG" && s != "CHA" {
				s = "INT"
			}
			sup[s] = true
		}
		c.Rd = max(c.Rd, auRang(v.Rd, r))
		c.Init += auRang(v.InitBonus, r)
		noms := v.Rangs[:min(r, len(v.Rangs))]
		ligne := fmt.Sprintf("%s rang %d", v.Nom, r)
		if v.Page > 0 {
			ligne += fmt.Sprintf(" (livre p. %d)", v.Page)
		}
		caps = append(caps, ligne+" : "+strings.Join(noms, ", ")+".")
		if v.Remarque != "" {
			caps = append(caps, "  "+v.Remarque)
		}
		c.Detail = append(c.Detail, fmt.Sprintf("%s rang %d : NC +%s", v.Nom, r, FmtNC(parRang*float64(r))))
		conception = append(conception, fmt.Sprintf("%s rang %d", v.Nom, r))
	}
	if d.AttMultiple {
		c.Nc += t.NcAttMultiple
		c.Detail = append(c.Detail, fmt.Sprintf("Attaque multiple : NC +%s", FmtNC(t.NcAttMultiple)))
		if x := t.Textes["capacite_attaque_multiple"]; x != "" {
			caps = append(caps, x)
		}
	}
	if c.Rd > 0 {
		c.Detail = append(c.Detail, fmt.Sprintf("RD %d apportée par les voies", c.Rd))
	}
	if x := t.Textes["alerte_voies_nc"]; x != "" && rangsVoies > 0 && float64(rangsVoies) > c.Nc/2 {
		c.Alertes = append(c.Alertes, x)
	}
	if x := t.Textes["alerte_trop_de_voies"]; x != "" && t.VoiesConseil > 0 && nbVoies > t.VoiesConseil {
		c.Alertes = append(c.Alertes, x)
	}
	for _, k := range []string{"FOR", "DEX", "CON", "INT", "SAG", "CHA"} {
		if sup[k] {
			c.Superieures = append(c.Superieures, k)
		}
	}
	c.Conception = strings.Join(conception, ", ")
	c.Capacites = strings.Join(caps, "\n")
	return c, nil
}
