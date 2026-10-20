package serveur

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/Tchyze/cof_dm/internal/fiches"
	"github.com/Tchyze/cof_dm/internal/reference"
	"github.com/Tchyze/cof_dm/internal/store"
)

// Feuille de personnage d'un PJ (reprise de tracker_COF) : caractéristiques, ressources, attaques,
// voies. Elle complète la fiche du PJ, qui garde son nom, son niveau et ses textes. Ses bornes et
// ses valeurs de départ viennent du fichier de règles de l'univers.

func (s *Serveur) routesFeuille() {
	s.mux.HandleFunc("GET /api/entites/{id}/feuille", s.feuilleLire)
	s.mux.HandleFunc("PUT /api/entites/{id}/feuille", s.feuilleEcrire)
}

// niveauDe : le niveau du PJ, lu sur sa fiche (1 s'il n'est pas renseigné).
func niveauDe(e store.Entite) int {
	if n, ok := e.Data["niveau"].(float64); ok && n >= 1 {
		return int(n)
	}
	return 1
}

// feuilleDe renvoie la feuille d'un PJ, recalculée avec son niveau et les règles de l'univers ; une
// fiche qui n'en a pas encore reçoit une feuille vierge (existe : false).
func (s *Serveur) feuilleDe(e store.Entite) (f *fiches.Feuille, modifie float64, existe bool, err error) {
	rg := s.lireRegles().Personnage
	raw, modifie, err := s.store.LireFeuille(e.ID)
	if errors.Is(err, store.ErrIntrouvable) {
		f = fiches.FeuilleNeuve(rg)
		f.PrendreEffets(nil)
		f.Deriver(niveauDe(e), orVide(rg))
		return f, 0, false, nil
	}
	if err != nil {
		return nil, 0, false, err
	}
	// relue sans bornes : une feuille enregistrée reste lisible si les règles ont changé depuis
	if f, err = fiches.RelireFeuille(raw, niveauDe(e)); err != nil {
		return nil, 0, false, err
	}
	f.PrendreEffets(s.effetsDe(f.Voies))
	f.Deriver(niveauDe(e), orVide(rg))
	return f, modifie, true, nil
}

// effetsDe : les bonus permanents des capacités atteintes dans les voies d'une feuille qui sont
// liées à la bibliothèque (même ref, sinon même nom). Une voie libre n'en donne aucun.
func (s *Serveur) effetsDe(voies []fiches.Voie) []fiches.EffetVoie {
	if len(voies) == 0 {
		return nil
	}
	ref := s.lireReference()
	var out []fiches.EffetVoie
	for _, v := range voies {
		lib := voieLiee(ref, v)
		if lib == nil {
			continue
		}
		for r := 0; r < v.Rang && r < len(lib.Rangs); r++ {
			c := lib.Rangs[r]
			for _, b := range c.Bonus {
				out = append(out, fiches.EffetVoie{Cible: b.Cible, Valeur: b.Valeur, ParRang: b.ParRang, Niveau: b.Niveau, Carac: b.Carac,
					Rang: v.Rang, Capacite: c.Nom, Voie: lib.Nom, RangCapa: r + 1})
			}
		}
	}
	return out
}

// voieLiee : la voie de la bibliothèque que désigne une voie de feuille (nil : voie libre).
func voieLiee(ref *reference.Reference, v fiches.Voie) *reference.Voie {
	for i, x := range ref.Voies {
		if v.Ref != "" && x.Ref != "" {
			if v.Ref == x.Ref {
				return &ref.Voies[i]
			}
			continue
		}
		if reference.Cle(x.Nom) == reference.Cle(v.Nom) {
			return &ref.Voies[i]
		}
	}
	return nil
}

// repondreFeuille : la feuille, avec ce qu'il faut pour l'afficher et la modifier (niveau du PJ,
// règles de la feuille dans cet univers).
func (s *Serveur) repondreFeuille(w http.ResponseWriter, e store.Entite, f fiches.Feuille, modifie float64, existe bool) {
	var m map[string]any
	_ = json.Unmarshal(f.JSON(), &m)
	m["entite"], m["niveau"], m["modifie"], m["existe"] = e.ID, niveauDe(e), modifie, existe
	m["regles"] = orVide(s.lireRegles().Personnage)
	repondre(w, 200, m)
}

// niveauPermis refuse à un PJ un niveau au-delà de ce que disent les règles de l'univers.
func (s *Serveur) niveauPermis(typ string, data map[string]any) error {
	n, ok := data["niveau"].(float64)
	rg := s.lireRegles().Personnage
	if typ != "pj" || !ok || rg == nil || rg.NiveauMax <= 0 || int(n) <= rg.NiveauMax {
		return nil
	}
	return store.ErrValidation{Msg: "Niveau : " + strconv.Itoa(rg.NiveauMax) + " au plus."}
}

// pjDe lit la fiche d'un PJ ; toute autre fiche n'a pas de feuille.
func (s *Serveur) pjDe(w http.ResponseWriter, r *http.Request) (store.Entite, bool) {
	id, err := idDe(r)
	if err != nil {
		erreurStore(w, err)
		return store.Entite{}, false
	}
	e, err := s.store.Lire(id)
	if err != nil {
		erreurStore(w, err)
		return e, false
	}
	if e.Type != "pj" {
		erreur(w, 422, "Seuls les PJ ont une feuille de personnage.")
		return e, false
	}
	return e, true
}

func (s *Serveur) feuilleLire(w http.ResponseWriter, r *http.Request) {
	e, ok := s.pjDe(w, r)
	if !ok {
		return
	}
	f, modifie, existe, err := s.feuilleDe(e)
	if err != nil {
		erreurFiche(w, err)
		return
	}
	s.repondreFeuille(w, e, *f, modifie, existe)
}

// enregistrerFeuille valide et enregistre une feuille. garderNotes : les notes du MJ ne viennent pas
// de la requête (un joueur ne les voit ni ne les change).
func (s *Serveur) enregistrerFeuille(w http.ResponseWriter, r *http.Request, e store.Entite, garderNotes bool) {
	corps, err := lireCorps(r, tailleMaxJSON)
	if err != nil {
		erreurStore(w, err)
		return
	}
	var attendu struct {
		Modifie float64 `json:"modifie"`
	}
	_ = json.Unmarshal(corps, &attendu)
	// les bonus des capacités dépendent des voies de la feuille reçue
	var recue struct {
		Voies []fiches.Voie `json:"voies"`
	}
	_ = json.Unmarshal(corps, &recue)
	f, err := fiches.NormaliserFeuilleAvec(corps, niveauDe(e), s.lireRegles().Personnage, s.effetsDe(recue.Voies))
	if err != nil {
		erreurFiche(w, err)
		return
	}
	if garderNotes {
		f.Notes = ""
		if ancienne, _, existe, err := s.feuilleDe(e); err == nil && existe {
			f.Notes = ancienne.Notes
		}
	}
	modifie, err := s.store.EcrireFeuille(e.ID, f.Stockage(), attendu.Modifie, auteurDe(r))
	if err != nil {
		erreurStore(w, err)
		return
	}
	if garderNotes {
		*f = f.PourJoueur()
	}
	s.joueurs.fiche(e.ID)
	s.repondreFeuille(w, e, *f, modifie, true)
}

func (s *Serveur) feuilleEcrire(w http.ResponseWriter, r *http.Request) {
	if e, ok := s.pjDe(w, r); ok {
		s.enregistrerFeuille(w, r, e, false)
	}
}

// ---------------------------------------------------------------- vue des joueurs

// monPJ : la fiche partagée du PJ que joue le compte. Toute autre fiche répond comme une fiche
// inexistante : un joueur ne lit que sa propre feuille.
func (s *Serveur) monPJ(w http.ResponseWriter, r *http.Request) (store.Entite, bool) {
	id, _ := idDe(r)
	c, _ := compteDe(r)
	e, err := s.store.Lire(id)
	if id == 0 || err != nil || e.Type != "pj" || !partagee(e) || c.Persos[s.store.ID()] != id {
		erreurStore(w, store.ErrIntrouvable)
		return e, false
	}
	return e, true
}

func (s *Serveur) feuilleLireJoueur(w http.ResponseWriter, r *http.Request) {
	e, ok := s.monPJ(w, r)
	if !ok {
		return
	}
	f, modifie, existe, err := s.feuilleDe(e)
	if err != nil {
		erreurFiche(w, err)
		return
	}
	s.repondreFeuille(w, e, f.PourJoueur(), modifie, existe)
}

// feuilleEcrireJoueur : quand l'édition par les joueurs est ouverte, le joueur tient lui-même la
// feuille de son PJ (tout, sauf les notes du MJ).
func (s *Serveur) feuilleEcrireJoueur(w http.ResponseWriter, r *http.Request) {
	e, ok := s.monPJ(w, r)
	if !ok {
		return
	}
	if !s.editionJoueurs() {
		erreur(w, 403, "L'édition par les joueurs n'est pas ouverte dans cet univers.")
		return
	}
	s.enregistrerFeuille(w, r, e, true)
}
