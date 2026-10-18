// Package reference gère les profils, peuples et voies (officiels ou maison) importés par le MJ.
// Le contenu officiel n'est pas embarqué : il s'importe depuis un fichier JSON du vault.
package reference

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode"
)

var sansAccents = strings.NewReplacer(
	"à", "a", "â", "a", "ä", "a", "á", "a", "ç", "c", "é", "e", "è", "e", "ê", "e", "ë", "e",
	"î", "i", "ï", "i", "í", "i", "ô", "o", "ö", "o", "ó", "o", "ù", "u", "û", "u", "ü", "u", "ú", "u",
	"ÿ", "y", "œ", "oe", "æ", "ae",
)

const Format = "tracker-cof-reference"

type Capacite struct {
	Nom     string `json:"nom"`
	Texte   string `json:"texte"`
	Limitee bool   `json:"limitee"` // (L) : action limitée
	Sort    bool   `json:"sort"`    // * : sort
}

type Voie struct {
	Ref         string     `json:"ref"`
	Nom         string     `json:"nom"`
	Type        string     `json:"type"` // profil, peuple, prestige, autre
	Profils     []string   `json:"profils"`
	Prerequis   string     `json:"prerequis"`
	Description string     `json:"description"`
	Rangs       []Capacite `json:"rangs"`
	Livre       string     `json:"livre"`
	Page        int        `json:"page"`
}

type Profil struct {
	Ref         string   `json:"ref"`
	Nom         string   `json:"nom"`
	Famille     string   `json:"famille"`
	Dv          string   `json:"dv"`
	CaracMagie  string   `json:"carac_magie"`
	Armes       string   `json:"armes"`
	Armures     string   `json:"armures"`
	Equipement  string   `json:"equipement"`
	Description string   `json:"description"`
	Voies       []string `json:"voies"`
	Livre       string   `json:"livre"`
	Page        int      `json:"page"`
}

type Peuple struct {
	Ref         string   `json:"ref"`
	Nom         string   `json:"nom"`
	Caracs      string   `json:"caracs"` // ex. « +2 DEX, −2 CON »
	Description string   `json:"description"`
	Voies       []string `json:"voies"`
	Livre       string   `json:"livre"`
	Page        int      `json:"page"`
}

type Reference struct {
	Format  string   `json:"format"`
	Version int      `json:"version"`
	Source  string   `json:"source,omitempty"`
	Profils []Profil `json:"profils"`
	Peuples []Peuple `json:"peuples"`
	Voies   []Voie   `json:"voies"`
}

func Vide() *Reference {
	return &Reference{Format: Format, Version: 1, Profils: []Profil{}, Peuples: []Peuple{}, Voies: []Voie{}}
}

// Cle : nom sans accents ni majuscules, pour rapprocher « Voie du prêtre » et « voie du pretre ».
func Cle(s string) string {
	var b strings.Builder
	for _, r := range sansAccents.Replace(strings.ToLower(strings.TrimSpace(s))) {
		if unicode.IsSpace(r) || r == '\'' || r == '’' || r == '-' || r == ',' {
			r = ' '
		}
		b.WriteRune(r)
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// Lire décode un fichier de référence (objet complet ou simple tableau de voies) et le valide.
func Lire(data []byte) (*Reference, error) {
	r := Vide()
	if err := json.Unmarshal(data, r); err != nil {
		var voies []Voie
		if json.Unmarshal(data, &voies) != nil {
			return nil, fmt.Errorf("format non reconnu : attendu un objet {\"profils\", \"peuples\", \"voies\"}")
		}
		r.Voies = voies
	}
	if r.Format != "" && r.Format != Format {
		return nil, fmt.Errorf("ce fichier n'est pas un fichier de profils et voies (format %q)", r.Format)
	}
	r.Format, r.Version = Format, 1
	if err := r.Normaliser(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *Reference) Normaliser() error {
	if r.Profils == nil {
		r.Profils = []Profil{}
	}
	if r.Peuples == nil {
		r.Peuples = []Peuple{}
	}
	if r.Voies == nil {
		r.Voies = []Voie{}
	}
	for i := range r.Voies {
		v := &r.Voies[i]
		v.Nom = strings.TrimSpace(v.Nom)
		if v.Nom == "" {
			return fmt.Errorf("voie n° %d : nom manquant", i+1)
		}
		switch v.Type {
		case "":
			v.Type = "profil"
		case "profil", "peuple", "prestige", "autre":
		default:
			return fmt.Errorf("voie « %s » : type %q inconnu (profil, peuple, prestige ou autre)", v.Nom, v.Type)
		}
		if len(v.Rangs) > 5 {
			return fmt.Errorf("voie « %s » : 5 rangs au maximum", v.Nom)
		}
		for len(v.Rangs) < 5 {
			v.Rangs = append(v.Rangs, Capacite{})
		}
		if v.Profils == nil {
			v.Profils = []string{}
		}
	}
	for i := range r.Profils {
		p := &r.Profils[i]
		p.Nom = strings.TrimSpace(p.Nom)
		if p.Nom == "" {
			return fmt.Errorf("profil n° %d : nom manquant", i+1)
		}
		switch p.Dv {
		case "", "d4", "d6", "d8", "d10", "d12":
		default:
			return fmt.Errorf("profil « %s » : dé de vie %q invalide", p.Nom, p.Dv)
		}
		switch p.CaracMagie {
		case "", "INT", "SAG", "CHA":
		default:
			return fmt.Errorf("profil « %s » : carac. de magie %q invalide (INT, SAG ou CHA)", p.Nom, p.CaracMagie)
		}
		if p.Voies == nil {
			p.Voies = []string{}
		}
	}
	for i := range r.Peuples {
		p := &r.Peuples[i]
		p.Nom = strings.TrimSpace(p.Nom)
		if p.Nom == "" {
			return fmt.Errorf("peuple n° %d : nom manquant", i+1)
		}
		if p.Voies == nil {
			p.Voies = []string{}
		}
	}
	return nil
}

// cle d'identité d'un élément : sa référence, ou à défaut son nom.
func ident(ref, nom string) string {
	if ref != "" {
		return "ref:" + ref
	}
	return "nom:" + Cle(nom)
}

// Bilan d'une fusion.
type Bilan struct {
	Profils  int `json:"profils"`
	Peuples  int `json:"peuples"`
	Voies    int `json:"voies"`
	Nouveaux int `json:"nouveaux"`
	MisAJour int `json:"mis_a_jour"`
}

// Fusionner ajoute ou remplace (même référence, ou même nom sans référence) les éléments de n.
func (r *Reference) Fusionner(n *Reference) Bilan {
	b := Bilan{Profils: len(n.Profils), Peuples: len(n.Peuples), Voies: len(n.Voies)}
	compter := func(remplace bool) {
		if remplace {
			b.MisAJour++
		} else {
			b.Nouveaux++
		}
	}
	idx := map[string]int{}
	for i, p := range r.Profils {
		idx[ident(p.Ref, p.Nom)] = i
	}
	for _, p := range n.Profils {
		i, ok := idx[ident(p.Ref, p.Nom)]
		compter(ok)
		if ok {
			r.Profils[i] = p
		} else {
			idx[ident(p.Ref, p.Nom)] = len(r.Profils)
			r.Profils = append(r.Profils, p)
		}
	}
	idx = map[string]int{}
	for i, p := range r.Peuples {
		idx[ident(p.Ref, p.Nom)] = i
	}
	for _, p := range n.Peuples {
		i, ok := idx[ident(p.Ref, p.Nom)]
		compter(ok)
		if ok {
			r.Peuples[i] = p
		} else {
			idx[ident(p.Ref, p.Nom)] = len(r.Peuples)
			r.Peuples = append(r.Peuples, p)
		}
	}
	idx = map[string]int{}
	for i, v := range r.Voies {
		idx[ident(v.Ref, v.Nom)] = i
	}
	for _, v := range n.Voies {
		i, ok := idx[ident(v.Ref, v.Nom)]
		compter(ok)
		if ok {
			r.Voies[i] = v
		} else {
			idx[ident(v.Ref, v.Nom)] = len(r.Voies)
			r.Voies = append(r.Voies, v)
		}
	}
	r.Trier()
	return b
}

func (r *Reference) Trier() {
	sort.SliceStable(r.Profils, func(i, j int) bool { return Cle(r.Profils[i].Nom) < Cle(r.Profils[j].Nom) })
	sort.SliceStable(r.Peuples, func(i, j int) bool { return Cle(r.Peuples[i].Nom) < Cle(r.Peuples[j].Nom) })
	sort.SliceStable(r.Voies, func(i, j int) bool { return Cle(r.Voies[i].Nom) < Cle(r.Voies[j].Nom) })
}

// Problemes signale les voies citées par un profil ou un peuple mais absentes du fichier.
func (r *Reference) Problemes() []string {
	connues := map[string]bool{}
	for _, v := range r.Voies {
		connues[Cle(v.Nom)] = true
		if v.Ref != "" {
			connues[v.Ref] = true
		}
	}
	out := []string{}
	for _, p := range r.Profils {
		for _, v := range p.Voies {
			if !connues[Cle(v)] && !connues[v] {
				out = append(out, fmt.Sprintf("profil %s : voie « %s » introuvable", p.Nom, v))
			}
		}
	}
	for _, p := range r.Peuples {
		for _, v := range p.Voies {
			if !connues[Cle(v)] && !connues[v] {
				out = append(out, fmt.Sprintf("peuple %s : voie « %s » introuvable", p.Nom, v))
			}
		}
	}
	return out
}
