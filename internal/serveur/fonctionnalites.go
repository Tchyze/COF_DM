package serveur

import (
	"net/http"
	"strings"

	"github.com/Tchyze/cof_dm/internal/modele"
	"github.com/Tchyze/cof_dm/internal/store"
)

// Fonctionnalités optionnelles (internal/modele/fonctionnalites.go) : réglées univers par univers.
// Coupée, une fonctionnalité disparaît du schéma et de l'interface, ses routes répondent 404
// (servir), ses types de fiches ne se créent plus et sortent des listes ; ses données restent
// intactes dans l'univers et continuent de voyager avec la synchronisation.

func (s *Serveur) routesFonctionnalites() {
	s.mux.HandleFunc("GET /api/fonctionnalites", s.lireFonctionnalites)
	s.mux.HandleFunc("PUT /api/fonctionnalites", s.reglerFonctionnalites)
}

type etatFonction struct {
	modele.Fonctionnalite
	Active bool `json:"active"`
}

func (s *Serveur) etatFonctions() []etatFonction {
	etat := s.store.Fonctionnalites()
	out := make([]etatFonction, 0, len(modele.Fonctionnalites))
	for _, f := range modele.Fonctionnalites {
		out = append(out, etatFonction{Fonctionnalite: f, Active: etat[f.Cle]})
	}
	return out
}

func (s *Serveur) lireFonctionnalites(w http.ResponseWriter, r *http.Request) {
	repondre(w, 200, s.etatFonctions())
}

// reglerFonctionnalites : {"des": true, "combat": false} ; les autres gardent leur état.
func (s *Serveur) reglerFonctionnalites(w http.ResponseWriter, r *http.Request) {
	var choix map[string]bool
	if err := lireJSON(r, &choix); err != nil {
		erreurStore(w, err)
		return
	}
	etat, err := s.store.ReglerFonctionnalites(choix, auteurDe(r))
	if err != nil {
		erreurStore(w, err)
		return
	}
	if !etat["page_joueurs"] {
		s.joueurs.fermer() // les appareils qui suivaient la page sont coupés
	}
	repondre(w, 200, s.etatFonctions())
}

// fonctionsDeRoute : les fonctionnalités dont une route a besoin (aucune : le cœur).
func fonctionsDeRoute(r *http.Request) []string {
	p := r.URL.Path
	debut := func(l ...string) bool {
		for _, x := range l {
			if p == x || strings.HasPrefix(p, x+"/") {
				return true
			}
		}
		return false
	}
	fiche := func(suffixe string) bool {
		return strings.HasPrefix(p, "/api/entites/") && strings.HasSuffix(p, "/"+suffixe)
	}
	var f []string
	if debut("/api/joueur") || (p == "/api/acces/joueurs" && r.Method != http.MethodGet) {
		f = append(f, "page_joueurs")
	}
	switch {
	case debut("/api/des", "/api/joueur/des"):
		f = append(f, "des")
	case debut("/api/aides", "/api/joueur/documents"):
		f = append(f, "aides")
	case fiche("feuille"), debut("/api/feuilles"):
		f = append(f, "feuilles")
	case debut("/api/combats", "/api/joueur/hauts-faits"):
		f = append(f, "combat")
	case fiche("difficulte"):
		f = append(f, "rencontres")
	case debut("/api/secrets"):
		f = append(f, "secrets")
	case debut("/api/table"):
		f = append(f, "ecran_mj")
	case fiche("preparation"), fiche("apres-seance"):
		f = append(f, "seances")
	case debut("/api/graphe"):
		f = append(f, "graphe")
	case debut("/api/chronologie"):
		f = append(f, "chronologie")
	case fiche("organigramme"):
		f = append(f, "organigramme")
	case debut("/api/export/obsidian"):
		f = append(f, "obsidian")
	}
	return f
}

// servir passe la requête au routeur, sauf si elle vise une fonctionnalité coupée dans l'univers.
func (s *Serveur) servir(m http.Handler, w http.ResponseWriter, r *http.Request) {
	for _, cle := range fonctionsDeRoute(r) {
		if !s.store.Active(cle) {
			f, _ := modele.FonctionnaliteDe(cle)
			repondre(w, 404, map[string]any{"detail": "Fonctionnalité désactivée dans cet univers : " + f.Nom +
				". Elle s'active dans Paramètres → Fonctionnalités.", "fonctionnalite": cle})
			return
		}
	}
	m.ServeHTTP(w, r)
}

// typeActif : le type de fiche n'appartient pas à une fonctionnalité coupée.
func typeActif(etat map[string]bool, typ string) bool {
	f := modele.FonctionnaliteDuType(typ)
	return f == "" || etat[f]
}

// typeCoupe : l'erreur d'une création de fiche d'un type coupé.
func (s *Serveur) typeCoupe(typ string) error {
	if f := modele.FonctionnaliteDuType(typ); f != "" && !s.store.Active(f) {
		x, _ := modele.FonctionnaliteDe(f)
		return store.ErrValidation{Msg: "Fonctionnalité désactivée dans cet univers : " + x.Nom + " (Paramètres → Fonctionnalités)."}
	}
	return nil
}

// typesActifs : les types du schéma sans ceux des fonctionnalités coupées ; un champ qui ne
// désigne que des types coupés disparaît, les autres perdent ces cibles.
func typesActifs(etat map[string]bool, types []modele.Type) []modele.Type {
	out := []modele.Type{}
	for _, t := range types {
		if !typeActif(etat, t.Cle) {
			continue
		}
		champs := []modele.Champ{}
		for _, c := range t.Champs {
			if len(c.Cibles) > 0 {
				cibles := []string{}
				for _, x := range c.Cibles {
					if typeActif(etat, x) {
						cibles = append(cibles, x)
					}
				}
				if len(cibles) == 0 {
					continue
				}
				c.Cibles = cibles
			}
			champs = append(champs, c)
		}
		t.Champs = champs
		carte := []string{}
		for _, k := range t.Carte {
			for _, c := range champs {
				if c.Cle == k {
					carte = append(carte, k)
				}
			}
		}
		t.Carte = carte
		out = append(out, t)
	}
	return out
}
