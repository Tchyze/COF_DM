package serveur

import (
	"bytes"
	"fmt"
	"log"

	"github.com/Tchyze/cof_dm/internal/fiches"
	"github.com/Tchyze/cof_dm/internal/store"
)

// Inventaire et fiches « Objet » : une ligne de l'inventaire d'un PJ peut désigner une fiche
// « Objet » du monde. La fiche fait foi pour le nom et les chiffres, que la ligne copie ; le
// Détenteur de la fiche suit dans les deux sens (donné au PJ : dans son inventaire ; ôté de son
// inventaire, ou donné ailleurs : il n'y est plus). Ces règles se jouent ici, à l'écriture par l'API
// (Claude compris) ; la synchronisation transporte leurs résultats sans les rejouer. Feuilles de
// personnage coupées : rien ne suit.

func (s *Serveur) suiviInventaire() bool { return s.store.Fonctionnalites()["feuilles"] }

// detenteurDe : l'identifiant du détenteur d'une fiche « Objet » (0 : aucun).
func detenteurDe(e store.Entite) int64 {
	switch v := e.Data["detenteur"].(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	case int:
		return int64(v)
	}
	return 0
}

// lierInventaire : chaque ligne liée prend le nom et les chiffres de sa fiche « Objet ».
func (s *Serveur) lierInventaire(f *fiches.Feuille) error {
	for i := range f.Inventaire {
		o := &f.Inventaire[i]
		if o.Fiche == 0 {
			continue
		}
		e, err := s.store.Lire(o.Fiche)
		if err != nil || e.Type != "objet" {
			return fmt.Errorf("« %s » : la fiche n°%d n'est pas un objet du monde.", o.Nom, o.Fiche)
		}
		o.PrendreFiche(e.Nom, fiches.ChiffresDe(e.Data["equipement"]))
	}
	return nil
}

// suivreDetenteurs : après l'écriture de la feuille du PJ pj, les objets du monde entrés dans son
// inventaire l'ont pour détenteur (et quittent celui d'un autre PJ) ; ceux qui en sont sortis n'en ont
// plus, s'ils l'avaient encore.
func (s *Serveur) suivreDetenteurs(pj int64, avant, apres []fiches.Objet, auteur string) {
	if !s.suiviInventaire() {
		return
	}
	dans := func(l []fiches.Objet) map[int64]bool {
		m := map[int64]bool{}
		for _, o := range l {
			if o.Fiche != 0 {
				m[o.Fiche] = true
			}
		}
		return m
	}
	a, b := dans(avant), dans(apres)
	for id := range b {
		if a[id] {
			continue
		}
		e, err := s.store.Lire(id)
		if err != nil || detenteurDe(e) == pj {
			continue
		}
		if ancien := detenteurDe(e); ancien != 0 {
			s.retirerDeInventaire(ancien, id, false, auteur)
		}
		s.donnerDetenteur(id, pj, auteur)
	}
	for id := range a {
		if b[id] {
			continue
		}
		if e, err := s.store.Lire(id); err == nil && detenteurDe(e) == pj {
			s.donnerDetenteur(id, 0, auteur)
		}
	}
}

func (s *Serveur) donnerDetenteur(objet, detenteur int64, auteur string) {
	var v any
	if detenteur != 0 {
		v = detenteur
	}
	if _, err := s.store.Modifier(objet, store.Maj{Data: map[string]any{"detenteur": v}}, auteur); err != nil {
		log.Printf("détenteur de l'objet n°%d : %v", objet, err)
	}
}

// suivreObjet : après l'écriture d'une fiche « Objet » (avant : nil à la création), l'objet quitte
// l'inventaire de son ancien détenteur et entre dans celui du nouveau, s'ils sont des PJ ; s'il ne
// change pas de mains, la ligne de son PJ reprend son nom et ses chiffres.
func (s *Serveur) suivreObjet(avant *store.Entite, apres store.Entite, auteur string) {
	if apres.Type != "objet" || !s.suiviInventaire() {
		return
	}
	var ancien int64
	if avant != nil {
		ancien = detenteurDe(*avant)
	}
	nouveau := detenteurDe(apres)
	if ancien != 0 && ancien != nouveau {
		s.retirerDeInventaire(ancien, apres.ID, false, auteur)
	}
	if nouveau != 0 {
		s.mettreDansInventaire(nouveau, apres, auteur)
	}
}

// objetSupprime : une fiche « Objet » effacée reste dans l'inventaire de son PJ, en objet courant.
func (s *Serveur) objetSupprime(e store.Entite, auteur string) {
	if e.Type == "objet" && detenteurDe(e) != 0 && s.suiviInventaire() {
		s.retirerDeInventaire(detenteurDe(e), e.ID, true, auteur)
	}
}

// retirerDeInventaire ôte de l'inventaire du PJ pj la ligne de la fiche objet ; garder : elle reste,
// en objet courant (fiche effacée).
func (s *Serveur) retirerDeInventaire(pj, objet int64, garder bool, auteur string) {
	s.changerInventaire(pj, auteur, func(f *fiches.Feuille) {
		for i := range f.Inventaire {
			if f.Inventaire[i].Fiche != objet {
				continue
			}
			if garder {
				f.Inventaire[i].Fiche = 0
			} else {
				f.Inventaire = append(f.Inventaire[:i], f.Inventaire[i+1:]...)
			}
			return
		}
	})
}

// mettreDansInventaire : l'objet e est dans l'inventaire du PJ pj, avec son nom et ses chiffres.
func (s *Serveur) mettreDansInventaire(pj int64, e store.Entite, auteur string) {
	s.changerInventaire(pj, auteur, func(f *fiches.Feuille) {
		i := -1
		for k, o := range f.Inventaire {
			if o.Fiche == e.ID {
				i = k
			}
		}
		if i < 0 {
			f.Inventaire = append(f.Inventaire, fiches.Objet{Fiche: e.ID, Quantite: 1})
			i = len(f.Inventaire) - 1
		}
		f.Inventaire[i].PrendreFiche(e.Nom, fiches.ChiffresDe(e.Data["equipement"]))
	})
}

// changerInventaire applique changer à la feuille du PJ pj et l'enregistre si elle a changé (une
// fiche qui n'est pas un PJ n'a pas d'inventaire).
func (s *Serveur) changerInventaire(pj int64, auteur string, changer func(*fiches.Feuille)) {
	e, err := s.store.Lire(pj)
	if err != nil || e.Type != "pj" {
		return
	}
	f, _, _, err := s.feuilleDe(e)
	if err != nil {
		log.Printf("inventaire du PJ n°%d : %v", pj, err)
		return
	}
	avant := f.Stockage()
	changer(f)
	f, err = fiches.NormaliserFeuilleAvec(f.JSON(), niveauDe(e), s.lireRegles().Personnage, s.effetsDe(f.Voies))
	if err != nil {
		log.Printf("inventaire du PJ n°%d : %v", pj, err)
		return
	}
	if bytes.Equal(avant, f.Stockage()) {
		return
	}
	if _, err := s.store.EcrireFeuille(pj, f.Stockage(), 0, auteur); err != nil {
		log.Printf("inventaire du PJ n°%d : %v", pj, err)
		return
	}
	s.joueurs.fiche(pj)
}
