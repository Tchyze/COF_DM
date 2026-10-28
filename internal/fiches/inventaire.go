package fiches

import (
	"encoding/json"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/Tchyze/cof_dm/internal/reference"
	"github.com/Tchyze/cof_dm/internal/regles"
)

// Inventaire d'un PJ : une ligne par objet (quantité, porté ou non). Un objet porté occupe un
// emplacement (regles.Feuille.LesEmplacements) ; ses bonus comptent comme ceux des capacités, avec
// le même détail (« DEF 12 = 8 + 4 Casque »), et une arme portée donne sa ligne d'attaque. Les
// chiffres d'un objet sont saisis par le MJ : aucun ne vient d'un livre.

// Objet : une ligne de l'inventaire. ID : propre à la feuille, donné à l'enregistrement.
type Objet struct {
	ID          int          `json:"id"`
	Nom         string       `json:"nom"`
	Quantite    int          `json:"quantite"`
	Porte       bool         `json:"porte,omitempty"`
	Emplacement string       `json:"emplacement,omitempty"`
	Bonus       []BonusObjet `json:"bonus,omitempty"`
	Arme        *Arme        `json:"arme,omitempty"`
	Notes       string       `json:"notes,omitempty"`
}

// BonusObjet : ce qu'un objet porté ajoute à une valeur (mêmes cibles que les bonus des capacités).
type BonusObjet struct {
	Cible  string `json:"cible"`
	Valeur int    `json:"valeur"`
}

// Arme : la ligne d'attaque qu'un objet porté ajoute à la feuille.
type Arme struct {
	Type   string `json:"type"` // C, D ou M
	Dm     string `json:"dm"`
	Mod    int    `json:"mod"`
	Portee string `json:"portee,omitempty"`
}

const (
	objetsMax = 200
	bonusMax  = 10
)

// verifierInventaire valide les lignes et leur donne un identifiant. emplacements : nil pour une
// feuille relue sans règles, dont on ne contrôle pas ce qui est porté ensemble.
func (f *Feuille) verifierInventaire(emplacements []regles.Emplacement) error {
	if f.Inventaire == nil {
		f.Inventaire = []Objet{}
	}
	if len(f.Inventaire) > objetsMax {
		return invalide("inventaire : %d objets au plus.", objetsMax)
	}
	vus, prochain := map[int]bool{}, 1
	for _, o := range f.Inventaire {
		prochain = max(prochain, o.ID+1)
	}
	cibles := append(slices.Clone(reference.CiblesBonus), Caracs...)
	for i := range f.Inventaire {
		o := &f.Inventaire[i]
		if o.ID <= 0 || vus[o.ID] {
			o.ID, prochain = prochain, prochain+1
		}
		vus[o.ID] = true
		o.Nom, o.Emplacement = strings.TrimSpace(o.Nom), strings.TrimSpace(o.Emplacement)
		if o.Nom == "" || len([]rune(o.Nom)) > 120 || len([]rune(o.Notes)) > 2000 || len(o.Emplacement) > 40 {
			return invalide("inventaire.%d : un nom de 1 à 120 caractères.", i)
		}
		if o.Quantite < 1 {
			o.Quantite = 1
		}
		if o.Quantite > 9999 {
			return invalide("inventaire.%d.quantite : 9999 au plus.", i)
		}
		if o.Porte && o.Emplacement == "" {
			return invalide("« %s » : sans emplacement, il ne se porte pas.", o.Nom)
		}
		if len(o.Bonus) > bonusMax {
			return invalide("« %s » : %d bonus au plus.", o.Nom, bonusMax)
		}
		for _, b := range o.Bonus {
			if !dans(cibles, b.Cible) || b.Valeur < -20 || b.Valeur > 20 {
				return invalide("« %s » : bonus de -20 à +20 sur %s.", o.Nom, strings.Join(cibles, ", "))
			}
		}
		if a := o.Arme; a != nil {
			if a.Type == "" {
				a.Type = "C"
			}
			if a.Type != "C" && a.Type != "D" && a.Type != "M" {
				return invalide("« %s » : arme de type C, D ou M.", o.Nom)
			}
			if len(a.Dm) > 40 || len(a.Portee) > 40 || a.Mod < -20 || a.Mod > 20 {
				return invalide("« %s » : DM et portée de 40 caractères au plus, bonus de -20 à +20.", o.Nom)
			}
		}
	}
	if emplacements != nil {
		return verifierPortes(f.Inventaire, emplacements)
	}
	return nil
}

// verifierPortes refuse deux objets portés au même emplacement (ou plus que son Nombre). Un
// emplacement que les règles ne connaissent plus compte pour lui-même, avec une place.
func verifierPortes(objets []Objet, emplacements []regles.Emplacement) error {
	parCle := map[string]regles.Emplacement{}
	for _, e := range emplacements {
		parCle[e.Cle] = e
	}
	nom := func(cle string) string {
		if e, ok := parCle[cle]; ok {
			return e.Nom
		}
		return cle
	}
	pris := map[string][]string{}
	for _, o := range objets {
		if !o.Porte {
			continue
		}
		occupe := []string{o.Emplacement}
		if e, ok := parCle[o.Emplacement]; ok && len(e.Occupe) > 0 {
			occupe = e.Occupe
		}
		for _, c := range occupe {
			pris[c] = append(pris[c], o.Nom)
			if places := max(1, parCle[c].Nombre); len(pris[c]) > places {
				return invalide("%s : « %s » ne se porte pas avec « %s ».", nom(c), o.Nom, strings.Join(pris[c][:len(pris[c])-1], " », « "))
			}
		}
	}
	return nil
}

// effetsObjets : les bonus des objets portés, à résoudre avec ceux des capacités.
func (f *Feuille) effetsObjets() []EffetVoie {
	var out []EffetVoie
	for _, o := range f.Inventaire {
		if !o.Porte {
			continue
		}
		for _, b := range o.Bonus {
			out = append(out, EffetVoie{Cible: b.Cible, Valeur: b.Valeur, Objet: o.Nom})
		}
	}
	return out
}

// attaquesObjets : une ligne d'attaque par arme portée (Objet : l'identifiant de la ligne).
func (f *Feuille) attaquesObjets() []Attaque {
	var out []Attaque
	for _, o := range f.Inventaire {
		if o.Porte && o.Arme != nil {
			out = append(out, Attaque{Nom: o.Nom, Type: o.Arme.Type, Dm: o.Arme.Dm, Mod: o.Arme.Mod, Portee: o.Arme.Portee, Objet: o.ID})
		}
	}
	return out
}

// sansAttaquesObjets : les attaques saisies, sans celles des armes portées (recalculées).
func sansAttaquesObjets(l []Attaque) []Attaque {
	return slices.DeleteFunc(l, func(a Attaque) bool { return a.Objet != 0 })
}

// IDsRecus : l'identifiant de chaque ligne de l'inventaire tel que le client l'a envoyé (0 : aucun),
// avant que l'enregistrement en donne aux nouvelles.
func IDsRecus(raw []byte) []int {
	var x struct {
		Inventaire []struct {
			ID int `json:"id"`
		} `json:"inventaire"`
	}
	_ = json.Unmarshal(raw, &x)
	ids := make([]int, len(x.Inventaire))
	for i, o := range x.Inventaire {
		ids[i] = o.ID
	}
	return ids
}

// GarderChiffres : ce qu'un joueur ne change pas dans l'inventaire de son PJ. recus : les
// identifiants qu'il a envoyés (IDsRecus). Une ligne qui existait garde ses bonus, son arme et son
// emplacement ; une ligne qu'il ajoute n'a ni bonus ni arme, et reçoit un nouvel identifiant.
func (f *Feuille) GarderChiffres(ancienne *Feuille, recus []int) {
	avant := map[int]Objet{}
	if ancienne != nil {
		for _, o := range ancienne.Inventaire {
			avant[o.ID] = o
		}
	}
	pris := map[int]bool{}
	for i := range f.Inventaire {
		o, id := &f.Inventaire[i], 0
		if i < len(recus) {
			id = recus[i]
		}
		if a, ok := avant[id]; ok && id > 0 && !pris[id] {
			pris[id] = true
			o.ID, o.Bonus, o.Arme, o.Emplacement = id, a.Bonus, a.Arme, a.Emplacement
		} else {
			o.ID, o.Bonus, o.Arme = 0, nil, nil
		}
	}
}

// ---------------------------------------------------------------- reprise de l'ancienne feuille

// ancienEquipement : ce que la feuille rangeait avant l'inventaire (modèle 5 et avant).
type ancienEquipement struct {
	Equipement string      `json:"equipement"`
	Armure     *Protection `json:"armure"`
	Bouclier   *Protection `json:"bouclier"`
}

// Protection : l'armure ou le bouclier d'une ancienne feuille.
type Protection struct {
	Nom   string `json:"nom"`
	Bonus int    `json:"bonus"`
}

var (
	reQteAvant = regexp.MustCompile(`^(\d{1,4})\s*[x×]?\s+(.+)$`)
	reQteApres = regexp.MustCompile(`^(.+?)\s*\(?[x×]\s*(\d{1,4})\)?$`)
	rePuce     = regexp.MustCompile(`^[-*•·]\s*`)
)

// reprendreEquipement : une feuille sans inventaire reçoit une ligne par ligne de son ancien texte
// « équipement » (« 3 torches » : trois torches), et son armure et son bouclier, portés.
func (f *Feuille) reprendreEquipement(raw []byte) {
	var champs map[string]json.RawMessage
	if json.Unmarshal(raw, &champs) != nil {
		return
	}
	if _, ok := champs["inventaire"]; ok {
		return
	}
	var a ancienEquipement
	if json.Unmarshal(raw, &a) != nil {
		return
	}
	porte := func(p *Protection, nom, emplacement string) {
		if p == nil || (strings.TrimSpace(p.Nom) == "" && p.Bonus == 0) {
			return
		}
		o := Objet{Nom: strings.TrimSpace(p.Nom), Quantite: 1, Porte: true, Emplacement: emplacement}
		if o.Nom == "" {
			o.Nom = nom
		}
		if p.Bonus != 0 {
			o.Bonus = []BonusObjet{{Cible: "def", Valeur: min(20, max(-20, p.Bonus))}}
		}
		f.Inventaire = append(f.Inventaire, o)
	}
	porte(a.Armure, "Armure", "corps")
	porte(a.Bouclier, "Bouclier", "main_secondaire")
	for _, l := range strings.Split(a.Equipement, "\n") {
		l = strings.TrimSpace(rePuce.ReplaceAllString(strings.TrimSpace(l), ""))
		if l == "" {
			continue
		}
		o := Objet{Nom: l, Quantite: 1}
		if m := reQteAvant.FindStringSubmatch(l); m != nil {
			o.Quantite, _ = strconv.Atoi(m[1])
			o.Nom = m[2]
		} else if m := reQteApres.FindStringSubmatch(l); m != nil {
			o.Quantite, _ = strconv.Atoi(m[2])
			o.Nom = m[1]
		}
		if r := []rune(o.Nom); len(r) > 120 {
			o.Nom, o.Notes = string(r[:120]), l
		}
		f.Inventaire = append(f.Inventaire, o)
	}
	for i := range f.Inventaire {
		f.Inventaire[i].ID = i + 1
	}
}
