package serveur

import (
	"net/http"
	"strings"
	"time"

	"github.com/Tchyze/cof_dm/internal/agenda"
	"github.com/Tchyze/cof_dm/internal/comptes"
)

// Abonnement à l'agenda des séances (mode serveur seulement). Google Agenda, Apple Calendrier, Proton
// Calendar, Outlook… relisent l'adresse depuis leurs propres serveurs, sans session ni cookie : le
// jeton secret de l'adresse (/agenda/<jeton>.ics) est sa seule clé. Un par compte, pour tous ses
// univers : comme joueur, les séances de la campagne de son PJ ; comme MJ, toutes celles de
// l'univers ; comme auditeur, rien (il voit l'agenda sans s'y abonner). Rien de réservé au MJ n'y
// entre : le flux d'un MJ est celui d'un joueur de chaque campagne.
//
// Le jeton n'est jamais écrit dans un journal ; il se change ou se retire depuis « Mon compte », et
// l'administrateur peut le retirer (DELETE /api/admin/comptes/{nom}/agenda) sans rien lire d'un
// univers. Le journal du reverse proxy, lui, garde les adresses demandées : hors de notre portée.

const (
	prefixeFlux = "/agenda/"
	// essaisFlux : les adresses inconnues tolérées par adresse IP, sur la durée des essais de connexion
	essaisFlux = 30
	// historiqueFlux : les séances finies depuis moins longtemps restent dans le flux
	historiqueFlux = 60 * 24 * time.Hour
)

// servirFlux : GET /agenda/<jeton>.ics.
func (a *Application) servirFlux(w http.ResponseWriter, r *http.Request) {
	enTetes(w)
	if !a.multi() {
		erreur(w, 404, "L'abonnement à l'agenda n'existe que sur un serveur.")
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		erreur(w, 405, "Lecture seule.")
		return
	}
	jeton, ok := strings.CutSuffix(strings.TrimPrefix(r.URL.Path, prefixeFlux), ".ics")
	cle := "agenda:" + cleAdresse(a.opt.acces.adresse(r))
	if !a.opt.acces.reserverEssai(cle, essaisFlux) {
		a.opt.acces.noter(r, "limite_atteinte", "", "portee=agenda")
		erreur(w, 429, "Trop d'adresses inconnues : patientez quelques minutes.")
		return
	}
	var c comptes.Compte
	if ok {
		c, ok = a.opt.Comptes.ParAgenda(jeton)
	}
	if !ok {
		// la même réponse pour un jeton inconnu, retiré ou d'un compte désactivé ; rien n'est noté
		// qui contiendrait l'adresse
		erreur(w, 404, "Adresse d'agenda inconnue ou retirée.")
		return
	}
	a.opt.acces.rendreEssai(cle)
	ev := a.evenementsDuCompte(c)
	h := w.Header()
	h.Set("Content-Type", "text/calendar; charset=utf-8")
	h.Set("Cache-Control", "private, no-store")
	w.WriteHeader(200)
	if r.Method == http.MethodGet {
		_, _ = w.Write(agenda.ICS("Séances de jeu — COF DM", ev, time.Now()))
	}
}

// evenementsDuCompte : les séances de tous les univers où le compte joue ou mène, chaque univers
// compté pendant sa lecture (une fermeture l'attend).
func (a *Application) evenementsDuCompte(c comptes.Compte) []agenda.Evenement {
	a.mu.RLock()
	var ouverts []*universOuvert
	for _, o := range a.ordre() {
		if role := c.Role(o.st.ID()); role == comptes.MJ || role == comptes.Joueur {
			o.requetes.entrer()
			ouverts = append(ouverts, o)
		}
	}
	a.mu.RUnlock()
	ev := []agenda.Evenement{}
	for _, o := range ouverts {
		ev = append(ev, o.srv.evenementsPour(c)...)
		o.requetes.sortir()
	}
	return ev
}

// evenementsPour : les séances de cet univers que le compte suit (agenda activé), sans les séances
// finies depuis plus de deux mois.
func (s *Serveur) evenementsPour(c comptes.Compte) []agenda.Evenement {
	if !s.store.Active("agenda") {
		return nil
	}
	id := s.store.ID()
	var campagnes map[int64]bool // nil : toutes (MJ)
	switch c.Role(id) {
	case comptes.MJ:
	case comptes.Joueur:
		pj, err := s.store.Lire(c.Persos[id])
		if c.Persos[id] == 0 || err != nil || pj.Type != "pj" || !partagee(pj) {
			return nil
		}
		campagnes = map[int64]bool{}
		for _, x := range idsDe(pj.Data["campagne"]) {
			campagnes[x] = true
		}
	default:
		return nil
	}
	l, err := s.seancesLocales()
	if err != nil {
		return nil
	}
	limite := time.Now().Add(-historiqueFlux)
	var gardees []*seanceAgenda
	for _, x := range l {
		if x.fin.Before(limite) {
			continue
		}
		if campagnes != nil && (x.Campagne == nil || !campagnes[x.Campagne.ID]) {
			continue
		}
		gardees = append(gardees, x)
	}
	return s.evenements(gardees)
}

// ---------------------------------------------------------------- Mon compte

func (s *Serveur) routesAbonnement() {
	m := s.mux
	m.HandleFunc("GET /api/compte/agenda", s.monAgenda)
	m.HandleFunc("POST /api/compte/agenda", s.nouvelAgenda)
	m.HandleFunc("DELETE /api/compte/agenda", s.retirerMonAgenda)
}

// abonnable : le compte mène ou joue dans au moins un univers (un auditeur voit l'agenda sans s'y
// abonner ; un administrateur n'a rien à y lire).
func abonnable(c comptes.Compte) bool {
	if c.Admin {
		return false
	}
	for _, role := range c.Roles {
		if role == comptes.MJ || role == comptes.Joueur {
			return true
		}
	}
	return false
}

// adressePublique : l'origine du serveur vue d'Internet (-origine si elle est donnée, sinon celle de la
// requête, telle que le proxy déclaré la transmet).
func (s *Serveur) adressePublique(r *http.Request) string {
	if s.acces.cles != nil && s.acces.cles.Origine != "" {
		return s.acces.cles.Origine
	}
	schema := "http"
	if s.acces.https(r) {
		schema = "https"
	}
	return schema + "://" + r.Host
}

// vueAgendaCompte : {"abonnable", "adresse", "webcal", "cree", "utilise"} pour son titulaire.
func (s *Serveur) vueAgendaCompte(r *http.Request, c comptes.Compte) map[string]any {
	out := map[string]any{"abonnable": abonnable(c), "adresse": nil, "webcal": nil}
	clair, j := s.acces.comptes.LireAgenda(c.Nom)
	if clair == "" || !abonnable(c) {
		return out
	}
	adresse := s.adressePublique(r) + prefixeFlux + clair + ".ics"
	_, reste, _ := strings.Cut(adresse, "://")
	out["adresse"], out["webcal"], out["cree"] = adresse, "webcal://"+reste, j.Cree
	if j.Utilise != 0 {
		out["utilise"] = j.Utilise
	}
	return out
}

func (s *Serveur) compteAbonnable(w http.ResponseWriter, r *http.Request) (comptes.Compte, bool) {
	c, ok := compteDe(r)
	if !ok {
		erreur(w, 401, "Connectez-vous.")
		return c, false
	}
	return c, true
}

// monAgenda : GET /api/compte/agenda.
func (s *Serveur) monAgenda(w http.ResponseWriter, r *http.Request) {
	if c, ok := s.compteAbonnable(w, r); ok {
		repondre(w, 200, s.vueAgendaCompte(r, c))
	}
}

// nouvelAgenda : POST /api/compte/agenda — crée l'adresse d'abonnement, ou la change (l'ancienne ne
// vaut plus).
func (s *Serveur) nouvelAgenda(w http.ResponseWriter, r *http.Request) {
	c, ok := s.compteAbonnable(w, r)
	if !ok {
		return
	}
	if !abonnable(c) {
		erreur(w, 403, "Seuls un MJ et un joueur s'abonnent à l'agenda.")
		return
	}
	if _, _, err := s.acces.comptes.NouvelAgenda(c.Nom); err != nil {
		erreurCompte(w, err)
		return
	}
	s.noterCompte(c, "adresse d'abonnement à l'agenda créée ou changée")
	s.acces.noter(r, "agenda_adresse", c.Nom)
	repondre(w, 200, s.vueAgendaCompte(r, c))
}

// retirerMonAgenda : DELETE /api/compte/agenda.
func (s *Serveur) retirerMonAgenda(w http.ResponseWriter, r *http.Request) {
	c, ok := s.compteAbonnable(w, r)
	if !ok {
		return
	}
	if err := s.acces.comptes.RetirerAgenda(c.Nom); err != nil {
		erreurCompte(w, err)
		return
	}
	s.noterCompte(c, "adresse d'abonnement à l'agenda retirée")
	s.acces.noter(r, "agenda_retire", c.Nom)
	repondre(w, 200, s.vueAgendaCompte(r, c))
}

// adminRetirerAgenda : DELETE /api/admin/comptes/{nom}/agenda — l'administrateur coupe l'adresse
// d'abonnement d'un compte (compte volé, appareil perdu), sans rien lire d'un univers.
func (a *Application) adminRetirerAgenda(w http.ResponseWriter, r *http.Request) {
	nom := comptes.Normaliser(r.PathValue("nom"))
	if _, ok := a.opt.Comptes.Lire(nom); !ok {
		erreurCompte(w, comptes.ErrInconnu)
		return
	}
	if err := a.opt.Comptes.RetirerAgenda(nom); err != nil {
		erreurCompte(w, err)
		return
	}
	a.opt.acces.noter(r, "agenda_retire", nom, "par="+auteurDe(r))
	c, _ := a.opt.Comptes.Lire(nom)
	repondre(w, 200, vueAdmin(c))
}
