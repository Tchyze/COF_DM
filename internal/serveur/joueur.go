package serveur

import (
	"net/http"

	"github.com/Tchyze/cof_dm/internal/modele"
	"github.com/Tchyze/cof_dm/internal/store"
)

// Rôle « joueur » (mode serveur). Un compte joueur n'a que sa page de joueur (joueur.html,
// pagejoueurs.go) : sa feuille, ses voies, le combat en cours, ses documents, ses dés, ses hauts
// faits. Le reste de l'API lui est fermé, et une route ajoutée plus tard le reste tant qu'elle n'est
// pas inscrite ici.
//
// Le filtre des fiches partagées (mondeJoueur) sert à l'export vers Obsidian « joueurs » du MJ.

// genresJoueur : les genres de champs montrés aux joueurs. Le profil de combat d'un PNJ, les
// créatures d'une rencontre n'en font pas partie.
var genresJoueur = map[string]bool{
	modele.Texte: true, modele.Long: true, modele.Choix: true, modele.Nombre: true, modele.Booleen: true,
	modele.Ref: true, modele.Refs: true, modele.DateMonde: true, modele.DateReelle: true, modele.Etapes: true,
}

func champJoueur(c modele.Champ) bool { return !c.Secret && genresJoueur[c.Genre] }

// partagee : la fiche est visible des joueurs. Une aide de jeu ne l'est jamais : elle n'arrive aux
// joueurs que par un envoi, dans leurs documents (aides.go).
func partagee(e store.Entite) bool {
	return e.Partage && !e.Archive && e.Type != "secret" && e.Type != "aide"
}

// routesJoueur : la liste blanche du rôle « joueur ». Les routes de la page (/api/joueur/…) s'y
// ajoutent dans routesPageJoueurs.
func (s *Serveur) routesJoueur() {
	m := http.NewServeMux()
	m.HandleFunc("GET /api/sante", s.sante)
	m.HandleFunc("GET /api/reference", s.referenceJoueur(s.monPJ)) // ?pj= : les voies, le profil et le peuple de son PJ
	m.HandleFunc("GET /api/entites/{id}/feuille", s.feuilleLireJoueur)
	m.HandleFunc("PUT /api/entites/{id}/feuille", s.feuilleEcrireJoueur)
	m.HandleFunc("PATCH /api/joueur/pj/{id}", s.modifierJoueur)
	m.HandleFunc("POST /api/joueur/pj/{id}/niveau", s.monterNiveauJoueur(s.monPJ)) // le niveau accordé par le MJ
	m.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		erreur(w, 403, "Un compte joueur n'a accès qu'à sa page de joueur.")
	})
	s.joueur = m
}

// mondeJoueur : les fiches partagées, par identifiant.
func (s *Serveur) mondeJoueur() (map[int64]store.Entite, error) {
	tout, err := s.store.Toutes()
	if err != nil {
		return nil, err
	}
	vues := map[int64]store.Entite{}
	for _, e := range tout {
		if partagee(e) {
			vues[e.ID] = ficheJoueur(e, nil)
		}
	}
	// les champs qui citent d'autres fiches ne gardent que celles qui sont partagées
	for id, e := range vues {
		vues[id] = ficheJoueur(e, vues)
	}
	return vues, nil
}

// ficheJoueur renvoie la fiche telle qu'un joueur la voit : ses seuls champs montrables. Avec vues,
// les champs Ref / Refs sont réduits aux fiches partagées.
func ficheJoueur(e store.Entite, vues map[int64]store.Entite) store.Entite {
	data := map[string]any{}
	if t, ok := modele.TypeDe(e.Type); ok {
		for _, c := range t.Champs {
			v, present := e.Data[c.Cle]
			if !present || !champJoueur(c) {
				continue
			}
			if vues != nil && (c.Genre == modele.Ref || c.Genre == modele.Refs) {
				gardes := []any{}
				for _, id := range idsDe(v) {
					if _, ok := vues[id]; ok {
						gardes = append(gardes, float64(id))
					}
				}
				switch {
				case c.Genre == modele.Refs:
					v = gardes
				case len(gardes) == 1:
					v = gardes[0]
				default:
					continue
				}
			}
			data[c.Cle] = v
		}
	}
	e.Data = data
	return e
}

// partager : {"partage": true} montre la fiche aux joueurs (sans ses champs réservés au MJ).
func (s *Serveur) partager(w http.ResponseWriter, r *http.Request) {
	id, err := idDe(r)
	if err != nil {
		erreurStore(w, err)
		return
	}
	var in struct {
		Partage bool `json:"partage"`
	}
	if err := lireJSON(r, &in); err != nil {
		erreurStore(w, err)
		return
	}
	if _, err := s.store.Partager(id, in.Partage, auteurDe(r)); err != nil {
		erreurStore(w, err)
		return
	}
	s.repondreFiche(w, 200, id)
}

// ---------------------------------------------------------------- édition par les joueurs

// editionJoueurs : le MJ a ouvert l'édition aux joueurs dans cet univers.
func (s *Serveur) editionJoueurs() bool {
	v, _ := s.store.Meta(store.MetaEditionJoueurs)
	return v == "1"
}

// reglerEditionJoueurs : {"active": true} laisse chaque joueur tenir, depuis sa page, la feuille de
// son PJ (sauf les notes du MJ) et les champs de sa fiche que le modèle lui ouvre (Champ.Joueur).
// Réglage de l'univers, synchronisé. Il revient au MJ : à la maison, la route n'est que dans
// l'application ; sur un serveur, ni l'auditeur (écriture) ni le joueur (liste blanche) ne
// l'atteignent.
func (s *Serveur) reglerEditionJoueurs(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Active bool `json:"active"`
	}
	if err := lireJSON(r, &in); err != nil {
		erreurStore(w, err)
		return
	}
	v, detail := "", "édition par les joueurs fermée"
	if in.Active {
		v, detail = "1", "édition par les joueurs ouverte"
	}
	if err := s.store.SetMeta(store.MetaEditionJoueurs, v); err != nil {
		erreurStore(w, err)
		return
	}
	s.store.Noter("accès", detail, auteurDe(r))
	s.joueurs.envoyer(map[string]any{"type": "edition", "active": in.Active})
	repondre(w, 200, map[string]any{"edition_joueurs": in.Active})
}

// modifierJoueur : {"data": {…}, "modifie"?} sur la fiche de son PJ, pour les seuls champs ouverts
// aux joueurs (Champ.Joueur). Ni le nom, ni les tags, ni aucun autre champ. Réponse : le PJ tel que
// sa page le montre.
func (s *Serveur) modifierJoueur(w http.ResponseWriter, r *http.Request) {
	e, ok := s.monPJ(w, r) // une fiche cachée n'existe pas pour le joueur, même la sienne
	if !ok {
		return
	}
	if !s.editionJoueurs() {
		erreur(w, 403, "L'édition par les joueurs n'est pas ouverte dans cet univers.")
		return
	}
	var in struct {
		Data    map[string]any `json:"data"`
		Attendu float64        `json:"modifie"`
	}
	if err := lireJSON(r, &in); err != nil {
		erreurStore(w, err)
		return
	}
	t, _ := modele.TypeDe(e.Type)
	if len(in.Data) == 0 {
		erreur(w, 422, "Rien à modifier.")
		return
	}
	for k := range in.Data {
		if ch, ok := t.Champ(k); !ok || !ch.Joueur || !champJoueur(*ch) {
			erreur(w, 403, "Le champ « "+k+" » ne se modifie pas par le joueur.")
			return
		}
	}
	if _, err := s.store.Modifier(e.ID, store.Maj{Data: in.Data, Attendu: in.Attendu}, auteurDe(r)); err != nil {
		erreurStore(w, err)
		return
	}
	s.joueurs.fiche(e.ID) // ses autres appareils suivent
	if e, err := s.store.Lire(e.ID); err != nil {
		erreurStore(w, err)
	} else {
		repondre(w, 200, s.pjPourJoueur(e))
	}
}
