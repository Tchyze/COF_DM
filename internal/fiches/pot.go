package fiches

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Tchyze/cof_dm/internal/modele"
)

// Pot commun d'une campagne : ce que le groupe a gagné et pas encore partagé. Le MJ y verse le butin
// d'une rencontre à la fin du combat et y ajoute ce qu'il veut (récompense de quête, trésor) ; les
// PJ s'en partagent le contenu, par des propositions que le MJ valide. Un partage validé met les
// pièces dans la bourse du PJ et les objets dans son inventaire ; le MJ peut l'annuler tant que le PJ
// a encore ce qu'il a reçu. Un document par campagne (collection « pots »).

type Pieces = modele.Pieces

// LignePot : un objet du pot, fiche « Objet » du monde (Fiche, quantité 1) ou objet courant.
type LignePot struct {
	ID       int    `json:"id"`
	Fiche    int64  `json:"fiche,omitempty"`
	Nom      string `json:"nom"`
	Quantite int    `json:"quantite"`
}

// Prise : ce qu'un partage prend d'une ligne du pot.
type Prise struct {
	Ligne    int `json:"ligne"`
	Quantite int `json:"quantite"`
}

// Proposition : un partage proposé pour un PJ, en attente du MJ.
type Proposition struct {
	ID     int     `json:"id"`
	Pj     int64   `json:"pj"`
	Par    string  `json:"par"`
	Le     float64 `json:"le"`
	Pieces Pieces  `json:"pieces"`
	Objets []Prise `json:"objets,omitempty"`
	Note   string  `json:"note,omitempty"`
}

// Mouvement : une entrée dans le pot (versement d'une rencontre, ajout du MJ), une sortie (retrait
// du MJ) ou un partage validé ; l'historique du pot.
type Mouvement struct {
	ID        int        `json:"id"`
	Genre     string     `json:"genre"` // entree, retrait ou partage
	Le        float64    `json:"le"`
	Par       string     `json:"par"`
	Libelle   string     `json:"libelle,omitempty"`
	Rencontre int64      `json:"rencontre,omitempty"`
	Pj        int64      `json:"pj,omitempty"`
	Pieces    Pieces     `json:"pieces"`
	Objets    []LignePot `json:"objets,omitempty"` // les objets tels qu'ils sont entrés ou sortis
	Annule    bool       `json:"annule,omitempty"` // partage annulé : revenu au pot
}

// Genres de mouvement.
const (
	Entree  = "entree"
	Retrait = "retrait"
	Partage = "partage"
)

type Pot struct {
	Campagne     int64         `json:"campagne"`
	Pieces       Pieces        `json:"pieces"`
	Objets       []LignePot    `json:"objets"`
	Propositions []Proposition `json:"propositions"`
	Historique   []Mouvement   `json:"historique"`
	Suivant      int           `json:"suivant"` // dernier numéro donné (lignes, propositions, mouvements)
}

// NouveauPot : le pot vide d'une campagne.
func NouveauPot(campagne int64) *Pot {
	return &Pot{Campagne: campagne, Objets: []LignePot{}, Propositions: []Proposition{}, Historique: []Mouvement{}}
}

// NormaliserPot relit un pot (restauration, synchronisation) : les listes vides sont des listes, les
// numéros ne se répètent pas.
func NormaliserPot(raw []byte) (*Pot, error) {
	p := NouveauPot(0)
	if err := decoder(raw, p); err != nil {
		return nil, err
	}
	if p.Campagne <= 0 {
		return nil, invalide("campagne : identifiant de la fiche attendu.")
	}
	if p.Pieces.Po < 0 || p.Pieces.Pa < 0 || p.Pieces.Pc < 0 {
		return nil, invalide("pièces : pas de nombre négatif.")
	}
	if p.Objets == nil {
		p.Objets = []LignePot{}
	}
	if p.Propositions == nil {
		p.Propositions = []Proposition{}
	}
	if p.Historique == nil {
		p.Historique = []Mouvement{}
	}
	for _, o := range p.Objets {
		p.Suivant = max(p.Suivant, o.ID)
		if o.Quantite < 1 {
			return nil, invalide("objets : quantité d'au moins 1.")
		}
	}
	for _, x := range p.Propositions {
		p.Suivant = max(p.Suivant, x.ID)
	}
	for _, x := range p.Historique {
		p.Suivant = max(p.Suivant, x.ID)
	}
	return p, nil
}

func (p *Pot) JSON() []byte {
	b, _ := json.Marshal(p)
	return b
}

func (p *Pot) numero() int {
	p.Suivant++
	return p.Suivant
}

// Vide : ni pièce ni objet.
func (p *Pot) Vide() bool { return p.Pieces.Vide() && len(p.Objets) == 0 }

// Ajouter met des pièces et des objets dans le pot et note l'entrée. Un objet courant rejoint la
// ligne du même nom ; un objet du monde déjà dans le pot est refusé.
func (p *Pot) Ajouter(m Mouvement, objets []modele.LigneButin) (*Mouvement, error) {
	if m.Pieces.Po < 0 || m.Pieces.Pa < 0 || m.Pieces.Pc < 0 {
		return nil, invalide("pièces : pas de nombre négatif.")
	}
	m.Objets = nil
	for _, o := range objets {
		l := LignePot{Fiche: o.Fiche, Nom: strings.TrimSpace(o.Nom), Quantite: o.Quantite}
		if l.Fiche != 0 || l.Quantite == 0 {
			l.Quantite = 1 // un objet du monde est unique ; sans quantité : un
		}
		if l.Quantite < 1 || l.Quantite > modele.MaxQuantite {
			return nil, invalide("« %s » : quantité de 1 à %d.", l.Nom, modele.MaxQuantite)
		}
		if l.Nom == "" {
			return nil, invalide("Un objet sans nom.")
		}
		m.Objets = append(m.Objets, l)
	}
	if m.Pieces.Vide() && len(m.Objets) == 0 {
		return nil, invalide("Rien à mettre dans le pot.")
	}
	for _, l := range m.Objets {
		if l.Fiche != 0 && p.ligneDeFiche(l.Fiche) != nil {
			return nil, invalide("« %s » est déjà dans le pot.", l.Nom)
		}
	}
	p.Pieces = plus(p.Pieces, m.Pieces)
	for _, l := range m.Objets {
		p.remettre(l)
	}
	m.ID = p.numero()
	p.Historique = append(p.Historique, m)
	return &p.Historique[len(p.Historique)-1], nil
}

// remettre : une ligne revient dans le pot (ajout, partage annulé).
func (p *Pot) remettre(l LignePot) {
	if l.Fiche == 0 {
		for i := range p.Objets {
			if p.Objets[i].Fiche == 0 && memeNom(p.Objets[i].Nom, l.Nom) {
				p.Objets[i].Quantite += l.Quantite
				return
			}
		}
	}
	l.ID = p.numero()
	p.Objets = append(p.Objets, l)
}

func (p *Pot) ligneDeFiche(fiche int64) *LignePot {
	for i := range p.Objets {
		if p.Objets[i].Fiche == fiche {
			return &p.Objets[i]
		}
	}
	return nil
}

func (p *Pot) ligne(id int) *LignePot {
	for i := range p.Objets {
		if p.Objets[i].ID == id {
			return &p.Objets[i]
		}
	}
	return nil
}

// Verifier : le pot contient ces pièces et ces objets.
func (p *Pot) Verifier(pieces Pieces, prises []Prise) error {
	if pieces.Po < 0 || pieces.Pa < 0 || pieces.Pc < 0 {
		return invalide("pièces : pas de nombre négatif.")
	}
	if pieces.Po > p.Pieces.Po || pieces.Pa > p.Pieces.Pa || pieces.Pc > p.Pieces.Pc {
		return invalide("Le pot n'a que %s.", TextePieces(p.Pieces))
	}
	voulu := map[int]int{}
	for _, x := range prises {
		voulu[x.Ligne] += x.Quantite
	}
	for id, q := range voulu {
		l := p.ligne(id)
		if l == nil {
			return invalide("Cet objet n'est plus dans le pot.")
		}
		if q < 1 || q > l.Quantite {
			return invalide("« %s » : le pot en a %d.", l.Nom, l.Quantite)
		}
	}
	if pieces.Vide() && len(prises) == 0 {
		return invalide("Rien n'est pris dans le pot.")
	}
	return nil
}

// Prendre retire du pot des pièces et des objets (vérifiés avant) et renvoie les objets pris.
func (p *Pot) Prendre(pieces Pieces, prises []Prise) ([]LignePot, error) {
	if err := p.Verifier(pieces, prises); err != nil {
		return nil, err
	}
	p.Pieces = moins(p.Pieces, pieces)
	var pris []LignePot
	for _, x := range prises {
		l := p.ligne(x.Ligne)
		pris = append(pris, LignePot{Fiche: l.Fiche, Nom: l.Nom, Quantite: x.Quantite})
		l.Quantite -= x.Quantite
	}
	reste := p.Objets[:0]
	for _, l := range p.Objets {
		if l.Quantite > 0 {
			reste = append(reste, l)
		}
	}
	p.Objets = reste
	return pris, nil
}

// Retirer : le MJ ôte du pot des pièces et des objets sans les donner (vendus, perdus…).
func (p *Pot) Retirer(m Mouvement, prises []Prise) (*Mouvement, error) {
	pris, err := p.Prendre(m.Pieces, prises)
	if err != nil {
		return nil, err
	}
	m.Genre, m.Objets, m.ID = Retrait, pris, p.numero()
	p.Historique = append(p.Historique, m)
	return &p.Historique[len(p.Historique)-1], nil
}

// Proposer ajoute une proposition de partage, si le pot a ce qu'elle demande.
func (p *Pot) Proposer(x Proposition) (*Proposition, error) {
	if x.Pj <= 0 {
		return nil, invalide("pj : le PJ qui reçoit le partage.")
	}
	if err := p.Verifier(x.Pieces, x.Objets); err != nil {
		return nil, err
	}
	x.Note = strings.TrimSpace(x.Note)
	x.ID = p.numero()
	p.Propositions = append(p.Propositions, x)
	return &p.Propositions[len(p.Propositions)-1], nil
}

// Proposition renvoie une proposition en attente.
func (p *Pot) Proposition(id int) (*Proposition, bool) {
	for i := range p.Propositions {
		if p.Propositions[i].ID == id {
			return &p.Propositions[i], true
		}
	}
	return nil, false
}

// Oublier retire une proposition (refusée ou validée).
func (p *Pot) Oublier(id int) {
	reste := p.Propositions[:0]
	for _, x := range p.Propositions {
		if x.ID != id {
			reste = append(reste, x)
		}
	}
	p.Propositions = reste
}

// Partager prend dans le pot ce que dit la proposition et note le partage ; les propositions qui
// ne tiennent plus restent, le MJ les refusera.
func (p *Pot) Partager(x Proposition, par string, le float64) (*Mouvement, error) {
	pris, err := p.Prendre(x.Pieces, x.Objets)
	if err != nil {
		return nil, err
	}
	p.Oublier(x.ID)
	m := Mouvement{ID: p.numero(), Genre: Partage, Le: le, Par: par, Pj: x.Pj, Pieces: x.Pieces, Objets: pris, Libelle: x.Note}
	p.Historique = append(p.Historique, m)
	return &p.Historique[len(p.Historique)-1], nil
}

// Mouvement renvoie un mouvement de l'historique.
func (p *Pot) Mouvement(id int) (*Mouvement, bool) {
	for i := range p.Historique {
		if p.Historique[i].ID == id {
			return &p.Historique[i], true
		}
	}
	return nil, false
}

// Annuler remet dans le pot ce qu'un partage avait donné (le PJ l'a rendu avant).
func (p *Pot) Annuler(id int) error {
	m, ok := p.Mouvement(id)
	if !ok || m.Genre != Partage {
		return invalide("Ce partage n'existe pas.")
	}
	if m.Annule {
		return invalide("Ce partage est déjà annulé.")
	}
	m.Annule = true
	p.Pieces = plus(p.Pieces, m.Pieces)
	for _, l := range m.Objets {
		p.remettre(l)
	}
	return nil
}

// ObjetEfface : la fiche « Objet » effacée reste dans le pot, en objet courant.
func (p *Pot) ObjetEfface(fiche int64) bool {
	change := false
	for i := range p.Objets {
		if p.Objets[i].Fiche == fiche {
			p.Objets[i].Fiche, change = 0, true
		}
	}
	return change
}

// ---------------------------------------------------------------- bourse et inventaire du PJ

// Recevoir met dans la feuille ce qu'un partage donne : les pièces dans la bourse, les objets dans
// l'inventaire (un objet courant rejoint la ligne libre du même nom).
func (f *Feuille) Recevoir(pieces Pieces, objets []LignePot) {
	f.Bourse.Po += pieces.Po
	f.Bourse.Pa += pieces.Pa
	f.Bourse.Pc += pieces.Pc
	for _, o := range objets {
		if o.Fiche != 0 {
			if f.ligneDeFiche(o.Fiche) < 0 {
				f.Inventaire = append(f.Inventaire, Objet{Fiche: o.Fiche, Nom: o.Nom, Quantite: 1})
			}
			continue
		}
		if i := f.ligneLibre(o.Nom); i >= 0 {
			f.Inventaire[i].Quantite += o.Quantite
			continue
		}
		f.Inventaire = append(f.Inventaire, Objet{Nom: o.Nom, Quantite: o.Quantite})
	}
}

// Rendre retire de la feuille ce qu'un partage avait donné, si le PJ l'a encore ; sinon rien ne
// change et l'erreur dit ce qui manque.
func (f *Feuille) Rendre(pieces Pieces, objets []LignePot) error {
	if f.Bourse.Po < pieces.Po || f.Bourse.Pa < pieces.Pa || f.Bourse.Pc < pieces.Pc {
		return invalide("La bourse n'a plus %s.", TextePieces(pieces))
	}
	for _, o := range objets {
		if o.Fiche != 0 {
			if f.ligneDeFiche(o.Fiche) < 0 {
				return invalide("« %s » n'est plus dans l'inventaire.", o.Nom)
			}
			continue
		}
		if i := f.ligneLibre(o.Nom); i < 0 || f.Inventaire[i].Quantite < o.Quantite {
			return invalide("L'inventaire n'a plus %d × « %s ».", o.Quantite, o.Nom)
		}
	}
	f.Bourse.Po -= pieces.Po
	f.Bourse.Pa -= pieces.Pa
	f.Bourse.Pc -= pieces.Pc
	for _, o := range objets {
		i := f.ligneDeFiche(o.Fiche)
		if o.Fiche == 0 {
			i = f.ligneLibre(o.Nom)
			f.Inventaire[i].Quantite -= o.Quantite
			if f.Inventaire[i].Quantite > 0 {
				continue
			}
		}
		f.Inventaire = append(f.Inventaire[:i], f.Inventaire[i+1:]...)
	}
	return nil
}

func (f *Feuille) ligneDeFiche(fiche int64) int {
	for i, o := range f.Inventaire {
		if fiche != 0 && o.Fiche == fiche {
			return i
		}
	}
	return -1
}

func (f *Feuille) ligneLibre(nom string) int {
	for i, o := range f.Inventaire {
		if o.Fiche == 0 && memeNom(o.Nom, nom) {
			return i
		}
	}
	return -1
}

// ---------------------------------------------------------------- textes

// TextePieces : « 12 po, 3 pa » (« rien » sans pièce).
func TextePieces(p Pieces) string {
	var l []string
	for _, x := range []struct {
		n   int
		nom string
	}{{p.Po, "po"}, {p.Pa, "pa"}, {p.Pc, "pc"}} {
		if x.n != 0 {
			l = append(l, fmt.Sprintf("%d %s", x.n, x.nom))
		}
	}
	if len(l) == 0 {
		return "rien"
	}
	return strings.Join(l, ", ")
}

// TexteButin : « 12 po, Épée longue, 3 × Torche » (« rien » sans pièce ni objet).
func TexteButin(p Pieces, objets []LignePot) string {
	var l []string
	if !p.Vide() {
		l = append(l, TextePieces(p))
	}
	for _, o := range objets {
		if o.Quantite > 1 {
			l = append(l, fmt.Sprintf("%d × %s", o.Quantite, o.Nom))
		} else {
			l = append(l, o.Nom)
		}
	}
	if len(l) == 0 {
		return "rien"
	}
	return strings.Join(l, ", ")
}

func plus(a, b Pieces) Pieces  { return Pieces{Po: a.Po + b.Po, Pa: a.Pa + b.Pa, Pc: a.Pc + b.Pc} }
func moins(a, b Pieces) Pieces { return Pieces{Po: a.Po - b.Po, Pa: a.Pa - b.Pa, Pc: a.Pc - b.Pc} }

func memeNom(a, b string) bool {
	return strings.EqualFold(strings.Join(strings.Fields(a), " "), strings.Join(strings.Fields(b), " "))
}
