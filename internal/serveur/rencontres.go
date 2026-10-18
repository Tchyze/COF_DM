package serveur

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/Tchyze/cof_dm/internal/regles"
	"github.com/Tchyze/cof_dm/internal/store"
)

// Rencontres : leurs créatures sont des monstres du bestiaire de l'univers, et leur difficulté se
// calcule ici, avec la partie « rencontre » du fichier de règles et les PJ de la campagne.

func (s *Serveur) routesRencontres() {
	s.mux.HandleFunc("GET /api/entites/{id}/difficulte", s.rencontreDifficulte)
}

// creatureRencontre : une ligne de la rencontre, avec ce que le bestiaire en dit.
type creatureRencontre struct {
	Monstre     int64         `json:"monstre"`
	Nom         string        `json:"nom"`
	Qte         int           `json:"qte"`
	Camp        string        `json:"camp"`
	Nc          float64       `json:"nc"`
	Defense     int           `json:"defense,omitempty"`
	Pv          int           `json:"pv,omitempty"`
	Pnj         *store.Resume `json:"pnj,omitempty"`         // PNJ présent qui porte ce profil
	Introuvable bool          `json:"introuvable,omitempty"` // monstre retiré du bestiaire
}

// adversairesDe lit le champ « adversaires » d'une rencontre.
func adversairesDe(e store.Entite) []creatureRencontre {
	var out []creatureRencontre
	l, _ := e.Data["adversaires"].([]any)
	for _, x := range l {
		a, _ := x.(map[string]any)
		m, _ := a["monstre"].(float64)
		q, _ := a["qte"].(float64)
		c := creatureRencontre{Monstre: int64(m), Qte: max(1, int(q)), Camp: "ennemi"}
		c.Nom, _ = a["nom"].(string)
		if camp, _ := a["camp"].(string); camp == "allie" {
			c.Camp = camp
		}
		if c.Monstre > 0 {
			out = append(out, c)
		}
	}
	return out
}

// creaturesEnClair : « 4 × Brigand, 1 × Chef », d'après les noms gardés dans la rencontre.
func creaturesEnClair(e store.Entite) string {
	var l []string
	for _, c := range adversairesDe(e) {
		nom := c.Nom
		if nom == "" {
			nom = "créature"
		}
		l = append(l, fmt.Sprintf("%d × %s", c.Qte, nom))
	}
	return strings.Join(l, ", ")
}

// creaturesDe : les créatures de la rencontre, puis ses PNJ présents qui ont un profil de combat.
// sans : les PNJ présents qui n'en ont pas.
func (s *Serveur) creaturesDe(e store.Entite) (creatures []creatureRencontre, sans []store.Resume) {
	creatures = []creatureRencontre{}
	completer := func(c *creatureRencontre) {
		m, err := s.monstre(c.Monstre)
		if err != nil {
			c.Introuvable = true
			return
		}
		c.Nc, c.Defense, c.Pv = m.Nc, m.Defense, m.Pv
		if c.Pnj == nil {
			c.Nom = m.Nom
		}
	}
	for _, c := range adversairesDe(e) {
		completer(&c)
		creatures = append(creatures, c)
	}
	for _, pid := range idsDe(e.Data["pnj"]) {
		p, err := s.store.Lire(pid)
		if err != nil {
			continue
		}
		res := store.Resume{ID: p.ID, Type: p.Type, Nom: p.Nom, Image: p.Image}
		m := premierID(p.Data["profil_combat"])
		if m == 0 {
			sans = append(sans, res)
			continue
		}
		c := creatureRencontre{Monstre: m, Nom: p.Nom, Qte: 1, Camp: "ennemi", Pnj: &res}
		completer(&c)
		creatures = append(creatures, c)
	}
	return creatures, sans
}

// groupeDe : combien de PJ jouent la rencontre et leur niveau moyen. Ce sont les PJ de sa campagne
// (ou de celle de sa quête) qui ne sont ni morts, ni partis, ni absents ; un PJ sans niveau compte
// pour le niveau 1.
func (s *Serveur) groupeDe(e store.Entite) (nb int, niveau float64) {
	somme := 0.0
	for _, p := range s.pjDeTable(s.campagneDeRencontre(e)) {
		if p.Statut == "absent" {
			continue
		}
		n := 1.0
		if f, err := s.store.Lire(p.Fiche.ID); err == nil {
			if v, ok := f.Data["niveau"].(float64); ok && v >= 1 {
				n = v
			}
		}
		somme += n
		nb++
	}
	if nb > 0 {
		niveau = somme / float64(nb)
	}
	return nb, niveau
}

// rencontreDifficulte : les créatures d'une rencontre et sa difficulté. ?condition= applique une
// condition du fichier de règles ; ?nb_pj= et ?niveau_pj= remplacent le groupe de la campagne.
// Sans partie « rencontre » dans les règles, « evaluation » est absente.
func (s *Serveur) rencontreDifficulte(w http.ResponseWriter, r *http.Request) {
	id, err := idDe(r)
	if err != nil {
		erreurStore(w, err)
		return
	}
	e, err := s.store.Lire(id)
	if err != nil || e.Type != "rencontre" {
		erreur(w, 404, "Rencontre introuvable.")
		return
	}
	creatures, sans := s.creaturesDe(e)
	if sans == nil {
		sans = []store.Resume{}
	}
	nb, niveau := s.groupeDe(e)
	q := r.URL.Query()
	if n, err := strconv.Atoi(q.Get("nb_pj")); err == nil && n >= 1 && n <= 50 {
		nb = n
	}
	if n, err := strconv.ParseFloat(q.Get("niveau_pj"), 64); err == nil && n > 0 && n <= 100 {
		niveau = n
	}
	out := map[string]any{"creatures": creatures, "pnj_sans_profil": sans, "nb_pj": nb, "niveau_pj": niveau,
		"conditions": []regles.Condition{}, "evaluation": nil}
	if rg := s.lireRegles().Rencontre; rg != nil {
		d := regles.DemandeRencontre{NbPj: nb, NiveauPj: niveau, Condition: q.Get("condition")}
		for _, c := range creatures {
			if c.Camp != "allie" && !c.Introuvable {
				d.Groupes = append(d.Groupes, regles.GroupeRencontre{Nom: c.Nom, Nc: c.Nc, Qte: c.Qte})
			}
		}
		if rg.Conditions != nil {
			out["conditions"] = rg.Conditions
		}
		out["evaluation"] = rg.Evaluer(d)
	}
	repondre(w, 200, out)
}

// usagesMonstre : les fiches qui s'appuient sur un monstre du bestiaire (profil de combat d'un PNJ,
// créature d'une rencontre).
func (s *Serveur) usagesMonstre(id int64) []string {
	var noms []string
	tout, _ := s.store.Toutes()
	for _, e := range tout {
		utilise := e.Type == "pnj" && premierID(e.Data["profil_combat"]) == id
		if e.Type == "rencontre" {
			for _, c := range adversairesDe(e) {
				utilise = utilise || c.Monstre == id
			}
		}
		if utilise {
			noms = append(noms, e.Nom)
		}
	}
	return noms
}
