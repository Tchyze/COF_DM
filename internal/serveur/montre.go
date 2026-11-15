package serveur

import (
	"bytes"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Tchyze/cof_dm/internal/modele"
	"github.com/Tchyze/cof_dm/internal/store"
)

// Afficheur de table, hors combat : l'écran d'attente (campagne, date en jeu, lieu du groupe) et
// « Montrer à la table ». Le MJ montre l'image d'une fiche ou une aide de jeu sur l'afficheur et sur
// la page des joueurs, jusqu'à ce qu'il la retire. Ce qui est montré est tenu en mémoire, comme les
// jets de dés : rien n'est rangé dans l'univers, et un redémarrage l'efface.
//
// Les joueurs n'en reçoivent que ce que le MJ a choisi de montrer : le titre (le nom de la fiche par
// défaut, qu'il peut changer ou vider), la légende de l'image, le texte remis d'une aide (jamais ses
// notes ni son secret), et cette seule image, servie par /api/joueur/montre/image. Ni la fiche, ni
// son identifiant, ni le nom du fichier.
//
// Quand le bureau est relié à un serveur, ces routes y passent (desDirect) : c'est le serveur qui
// tient ce qui est montré, et ses joueurs le voient.

type montre struct {
	Seq     int64  `json:"seq"`
	Titre   string `json:"titre"`
	Legende string `json:"legende,omitempty"`
	Texte   string `json:"texte,omitempty"`
	Image   bool   `json:"image"` // une image est montrée : …/montre/image?seq=<seq>
	fichier string // l'image, pour la servir
	fiche   int64  // la fiche montrée, pour le MJ
}

// montreMJ : ce que l'afficheur et l'écran du MJ en lisent, avec la fiche et le fichier de l'image.
type montreMJ struct {
	montre
	Fiche   int64  `json:"fiche"`
	Fichier string `json:"fichier,omitempty"`
}

type tableMontre struct {
	mu  sync.Mutex
	m   *montre
	seq int64
}

func (t *tableMontre) lire() *montre {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.m == nil {
		return nil
	}
	c := *t.m
	return &c
}

func (t *tableMontre) poser(m *montre) *montre {
	t.mu.Lock()
	defer t.mu.Unlock()
	if m != nil {
		t.seq++
		m.Seq = t.seq
	}
	t.m = m
	if m == nil {
		return nil
	}
	c := *m
	return &c
}

func (s *Serveur) routesAfficheur() {
	s.montre = &tableMontre{}
	s.mux.HandleFunc("GET /api/afficheur/attente", s.afficheurAttente)
	s.mux.HandleFunc("GET /api/afficheur/montre", s.desDirect(s.montreLire))
	s.mux.HandleFunc("POST /api/afficheur/montrer", s.desDirect(s.montrer))
	s.mux.HandleFunc("POST /api/afficheur/retirer", s.desDirect(s.montreRetirer))
}

// routesMontreJoueurs : l'image montrée, sur la page des joueurs (application, rôle « joueur »,
// page libre).
func (s *Serveur) routesMontreJoueurs(m *http.ServeMux) {
	m.HandleFunc("GET /api/joueur/montre/image", s.montreImage)
}

// pourJoueurs : ce qui est montré, tel que les joueurs le reçoivent (nil : rien).
func (s *Serveur) pourJoueurs() *montre {
	if s.montre == nil {
		return nil
	}
	return s.montre.lire()
}

func (s *Serveur) montreLire(w http.ResponseWriter, r *http.Request) {
	m := s.montre.lire()
	if m == nil {
		repondre(w, 200, map[string]any{"montre": nil})
		return
	}
	repondre(w, 200, map[string]any{"montre": montreMJ{montre: *m, Fiche: m.fiche, Fichier: m.fichier}})
}

type demandeMontrer struct {
	Fiche int64   `json:"fiche"`
	Image string  `json:"image"` // une image de la fiche ; vide : son image principale
	Titre *string `json:"titre"` // absent : le nom de la fiche ; "" : aucun titre
	Texte *bool   `json:"texte"` // une aide : montrer aussi son texte remis (oui par défaut)
}

// montrer : le MJ montre l'image d'une fiche, ou une aide de jeu (image et texte remis), à la table.
func (s *Serveur) montrer(w http.ResponseWriter, r *http.Request) {
	var in demandeMontrer
	if err := lireJSON(r, &in); err != nil {
		erreurStore(w, err)
		return
	}
	e, err := s.store.Lire(in.Fiche)
	if err != nil {
		erreur(w, 404, "Fiche introuvable.")
		return
	}
	m := &montre{Titre: e.Nom, fiche: e.ID}
	if in.Titre != nil {
		m.Titre = strings.TrimSpace(*in.Titre)
	}
	medias, err := s.store.Medias(e.ID)
	if err != nil {
		erreurStore(w, err)
		return
	}
	for _, x := range medias { // Medias : la principale d'abord
		if in.Image == "" || x.Fichier == in.Image {
			m.fichier, m.Legende, m.Image = x.Fichier, strings.TrimSpace(x.Legende), true
			break
		}
	}
	if in.Image != "" && !m.Image {
		erreur(w, 422, "Cette image n'est pas sur la fiche.")
		return
	}
	if e.Type == "aide" && (in.Texte == nil || *in.Texte) {
		if c, ok := modele.TypeDe("aide"); ok {
			if ch, ok := c.Champ("contenu"); ok && champJoueur(*ch) {
				v, _ := e.Data["contenu"].(string)
				m.Texte = strings.TrimSpace(sansLiens(v))
			}
		}
	}
	if !m.Image && m.Texte == "" {
		msg := "Rien à montrer : cette fiche n'a pas d'image."
		if e.Type == "aide" {
			msg = "Rien à montrer : cette aide n'a ni image ni texte remis."
		}
		erreur(w, 422, msg)
		return
	}
	montre := s.montre.poser(m)
	go s.joueurs.envoyer(map[string]any{"type": "montre", "montre": montre})
	repondre(w, 200, map[string]any{"montre": montreMJ{montre: *montre, Fiche: m.fiche, Fichier: m.fichier}})
}

func (s *Serveur) montreRetirer(w http.ResponseWriter, r *http.Request) {
	s.montre.poser(nil)
	go s.joueurs.envoyer(map[string]any{"type": "montre", "montre": nil})
	repondre(w, 200, map[string]any{"montre": nil})
}

// montreImage : l'image montrée à la table, et elle seule.
func (s *Serveur) montreImage(w http.ResponseWriter, r *http.Request) {
	m := s.montre.lire()
	if m == nil || !m.Image || !store.NomMediaValide(m.fichier) {
		http.NotFound(w, r)
		return
	}
	contenu, cree, err := s.store.ContenuMedia(m.fichier)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	// pas de cache partagé : le MJ la retire quand il veut
	w.Header().Set("Cache-Control", "private, no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, "image", time.Unix(int64(cree), 0), bytes.NewReader(contenu))
}

// ---------------------------------------------------------------- écran d'attente

type ficheAttente struct {
	Nom   string `json:"nom"`
	Image string `json:"image,omitempty"` // fichier de son image principale (servi par /medias/)
}

// afficheurAttente : ce que l'afficheur montre hors combat. La campagne : celle de l'écran du MJ
// (?campagne=, sinon celle de la quête en cours, sinon la campagne en cours) ; sa date en jeu en
// toutes lettres, et le lieu où se trouve le groupe, avec son image. Rien d'autre de la campagne.
func (s *Serveur) afficheurAttente(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{"campagne": nil, "date": "", "lieu": nil}
	id := entierParam(r, "campagne")
	if id == 0 {
		var quete *store.Entite
		if l, err := s.store.Lister(store.Filtre{Type: "quete"}); err == nil {
			var plusRecente store.Resume
			for _, q := range l {
				if q.Carte["statut"] == "en cours" && q.Modifie > plusRecente.Modifie {
					plusRecente = q
				}
			}
			if plusRecente.ID != 0 {
				if q, err := s.store.Lire(plusRecente.ID); err == nil {
					quete = &q
				}
			}
		}
		id = s.campagneDeTable(quete)
	}
	c, err := s.store.Lire(id)
	if err != nil || c.Type != "campagne" || c.Archive {
		repondre(w, 200, out)
		return
	}
	out["campagne"] = ficheAttente{Nom: c.Nom, Image: s.imagePrincipale(c.ID)}
	if m, ok := c.Data["date_actuelle"].(map[string]any); ok {
		out["date"] = s.store.Calendrier().Texte(modele.DateJeu{An: int(num(m["an"])), Mois: int(num(m["mois"])), Jour: int(num(m["jour"]))})
	}
	if l, err := s.store.Lire(premierID(c.Data["lieu_groupe"])); err == nil && !l.Archive {
		out["lieu"] = ficheAttente{Nom: l.Nom, Image: s.imagePrincipale(l.ID)}
	}
	repondre(w, 200, out)
}

// imagePrincipale : le fichier de l'image principale d'une fiche ("" : aucune).
func (s *Serveur) imagePrincipale(id int64) string {
	l, err := s.store.Medias(id)
	if err != nil || len(l) == 0 {
		return ""
	}
	return l[0].Fichier
}
