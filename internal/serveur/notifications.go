package serveur

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/Tchyze/cof_dm/internal/store"
)

// Notifications (projet 29) : la cloche du MJ et celle des joueurs. Ce qui les remplit :
//   - le veilleur (une fois par minute) : rappel de la séance la veille à 19 h (heure de l'univers),
//     aux attendus qui n'ont pas dit « absent » ; séance déplacée, annulée ou rétablie, à tous les
//     attendus ;
//   - la réponse d'un joueur à une séance (présent, peut-être, absent), pour le MJ ;
//   - la relance du MJ (bouton « Relancer »), aux attendus qui n'ont pas encore répondu.
//
// Elles naissent là où l'univers fait foi : sur un serveur, ou sur le bureau qui n'est relié à aucun
// serveur. Une copie reliée n'en fait aucune (le serveur s'en charge, une fois les changements
// reçus) et lit celles du MJ sur le serveur, par le jeton du poste.

const (
	heureRappel      = 19 // la veille de la séance, à 19 h, heure de l'univers
	maxNotifsLues    = 50 // ce que la cloche montre
	intervalleVeille = time.Minute
)

func (s *Serveur) routesNotifications() {
	m := s.mux
	m.HandleFunc("GET /api/notifications", s.parServeur(s.notificationsMJ, false))
	m.HandleFunc("POST /api/notifications/lues", s.parServeur(s.marquerLuesMJ, false))
	m.HandleFunc("POST /api/agenda/{id}/relance", s.parServeur(s.relancer, true))
}

func (s *Serveur) routesNotificationsJoueurs(m *http.ServeMux) {
	m.HandleFunc("GET /api/joueur/notifications", s.notificationsJoueur)
	m.HandleFunc("POST /api/joueur/notifications/lues", s.marquerLuesJoueur)
}

// estRelie : cet univers est la copie reliée d'un univers de serveur (connectée ou non).
func (s *Serveur) estRelie() bool {
	if s.opt.app == nil {
		return false
	}
	_, l, _ := s.opt.app.relieDe(s.store)
	return l != nil
}

const msgNotifsServeur = "Les notifications sont sur le serveur, qui ne répond pas : elles reviendront avec la connexion."

// parServeur : sur une copie reliée, la route est servie par le serveur (avec le jeton du poste) ;
// ailleurs, ici. envoyer : les changements d'ici partent d'abord (une séance qu'on vient de créer).
func (s *Serveur) parServeur(local http.HandlerFunc, envoyer bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.estRelie() {
			local(w, r)
			return
		}
		cl := s.clientDirect()
		if cl == nil {
			erreur(w, http.StatusServiceUnavailable, msgNotifsServeur)
			return
		}
		corps := garderCorps(r)
		if envoyer {
			_, _ = s.opt.app.echangerDe(s.store)
		}
		code, rep, err := cl.Relayer(r.Method, r.URL.RequestURI(), corps)
		if err != nil {
			s.noterDirect(err)
			erreur(w, http.StatusServiceUnavailable, msgNotifsServeur)
			return
		}
		s.noterDirect(nil)
		repondreBrut(w, code, rep)
	}
}

// ---------------------------------------------------------------- lire, marquer lues

func (s *Serveur) envoyerNotifs(w http.ResponseWriter, pour ...int64) {
	l, n, err := s.store.Notifications(maxNotifsLues, pour...)
	if err != nil {
		erreurStore(w, err)
		return
	}
	repondre(w, 200, map[string]any{"non_lues": n, "notifications": l})
}

// notificationsMJ : GET /api/notifications — la cloche du MJ (l'auditeur la lit aussi).
func (s *Serveur) notificationsMJ(w http.ResponseWriter, r *http.Request) {
	s.envoyerNotifs(w, store.PourMJ)
}

type demandeLues struct {
	IDs []int64 `json:"ids"` // vide : toutes
}

// marquerLuesMJ : POST /api/notifications/lues {"ids": [...]} (vide : toutes).
func (s *Serveur) marquerLuesMJ(w http.ResponseWriter, r *http.Request) {
	var in demandeLues
	if err := lireJSON(r, &in); err != nil {
		erreurStore(w, err)
		return
	}
	if err := s.store.MarquerLues(in.IDs, store.PourMJ); err != nil {
		erreurStore(w, err)
		return
	}
	s.envoyerNotifs(w, store.PourMJ)
}

// destinatairesJoueur : les PJ dont ce joueur lit les notifications. Sur un serveur, celui de son
// compte ; à la maison, le PJ choisi et les autres PJ actifs que joue la même personne (même champ
// « Joueur »), comme son agenda.
func (s *Serveur) destinatairesJoueur(w http.ResponseWriter, r *http.Request) ([]int64, bool) {
	id, ok := s.pjDocuments(w, r)
	if !ok {
		return nil, false
	}
	pour := []int64{id}
	if _, restreint := s.pjDuCompte(r); restreint {
		return pour, true
	}
	pj, err := s.store.Lire(id)
	if err != nil {
		erreurStore(w, err)
		return nil, false
	}
	if joueur := cleJoueur(pj); joueur != "" {
		tout, err := s.store.Toutes()
		if err != nil {
			erreurStore(w, err)
			return nil, false
		}
		for _, e := range tout {
			st, _ := e.Data["statut"].(string)
			if e.Type == "pj" && e.ID != id && !e.Archive && (st == "" || st == "actif") && cleJoueur(e) == joueur {
				pour = append(pour, e.ID)
			}
		}
	}
	return pour, true
}

// notificationsJoueur : GET /api/joueur/notifications[?pj=] — la cloche du joueur.
func (s *Serveur) notificationsJoueur(w http.ResponseWriter, r *http.Request) {
	if pour, ok := s.destinatairesJoueur(w, r); ok {
		s.envoyerNotifs(w, pour...)
	}
}

// marquerLuesJoueur : POST /api/joueur/notifications/lues[?pj=] {"ids": [...]} — seule écriture : ses
// propres notifications passent « lues ».
func (s *Serveur) marquerLuesJoueur(w http.ResponseWriter, r *http.Request) {
	var in demandeLues
	if err := lireJSON(r, &in); err != nil {
		erreurStore(w, err)
		return
	}
	pour, ok := s.destinatairesJoueur(w, r)
	if !ok {
		return
	}
	if err := s.store.MarquerLues(in.IDs, pour...); err != nil {
		erreurStore(w, err)
		return
	}
	s.envoyerNotifs(w, pour...)
}

// ---------------------------------------------------------------- déposer

// notifier dépose une notification et prévient aussitôt les écrans concernés.
func (s *Serveur) notifier(n store.NouvelleNotif, pour ...int64) {
	l, err := s.store.Notifier(n, pour...)
	if err != nil {
		log.Printf("notification : %v", err)
		return
	}
	pjs := []int64{}
	for _, x := range l {
		if x.Pour != store.PourMJ {
			pjs = append(pjs, x.Pour)
		}
	}
	if len(pjs) > 0 && s.joueurs != nil {
		s.joueurs.envoyer(map[string]any{"type": "notifications", "pj": pjs})
	}
}

// ---------------------------------------------------------------- textes

var (
	joursFR = []string{"dimanche", "lundi", "mardi", "mercredi", "jeudi", "vendredi", "samedi"}
	moisFR  = []string{"janvier", "février", "mars", "avril", "mai", "juin", "juillet", "août", "septembre", "octobre", "novembre", "décembre"}
)

// quand : « samedi 22 novembre à 20 h » (« à 20 h 30 » ; sans heure pour une séance sans heure).
func (x *seanceAgenda) quand(fz *time.Location) string {
	d := x.debut.In(fz)
	t := fmt.Sprintf("%s %d %s", joursFR[d.Weekday()], d.Day(), moisFR[d.Month()-1])
	if d.Day() == 1 {
		t = fmt.Sprintf("%s 1er %s", joursFR[d.Weekday()], moisFR[d.Month()-1])
	}
	if x.JourEntier {
		return t
	}
	return t + " à " + heureFR(d)
}

func heureFR(d time.Time) string {
	if d.Minute() == 0 {
		return fmt.Sprintf("%d h", d.Hour())
	}
	return fmt.Sprintf("%d h %02d", d.Hour(), d.Minute())
}

// details : campagne et lieu, sur une ligne.
func (x *seanceAgenda) details() string {
	l := []string{}
	if x.Campagne != nil {
		l = append(l, x.Campagne.Nom)
	}
	if x.Lieu != "" {
		l = append(l, x.Lieu)
	}
	if x.Visio != "" && x.Lieu == "" {
		l = append(l, "en visio")
	}
	return strings.Join(l, " · ")
}

func (x *seanceAgenda) attendus(sauf func(participant) bool) []int64 {
	out := []int64{}
	for _, p := range x.Participants {
		if sauf == nil || !sauf(p) {
			out = append(out, p.PJ)
		}
	}
	return out
}

const lienAgendaJoueur = "#/agenda"

// ---------------------------------------------------------------- le veilleur

// signature : ce qui, en changeant, fait prévenir les joueurs (date, heure, annulation).
func (x *seanceAgenda) signature() string {
	annulee := ""
	if x.Statut == "annulée" {
		annulee = "annulée"
	}
	return x.Date + "|" + x.Heure + "|" + annulee
}

// veillerNotifications passe sur les séances de l'univers : rappel de la veille, séance déplacée,
// annulée ou rétablie. Rien sur une copie reliée, ni quand l'agenda est coupé.
func (s *Serveur) veillerNotifications(maintenant time.Time) {
	if s.estRelie() || !s.store.Active("agenda") {
		return
	}
	l, err := s.seancesLocales()
	if err != nil {
		log.Printf("veilleur des notifications : %v", err)
		return
	}
	fz := s.fuseau()
	for _, x := range l {
		if x.Campagne == nil {
			continue
		}
		suivi, err := s.store.SuiviSeance(x.ID)
		if err != nil {
			continue
		}
		avant := suivi
		sig := x.signature()
		aVenir := x.fin.After(maintenant) && x.Statut != "jouée"
		if suivi.Connue && suivi.Signature != sig && aVenir {
			s.prevenirChangement(x, suivi.Signature, fz)
		}
		if suivi.Connue && strings.SplitN(suivi.Signature, "|", 2)[0] != x.Date {
			suivi.Rappel = "" // une nouvelle date, un nouveau rappel
		}
		suivi.Signature, suivi.Connue = sig, true
		if aVenir && x.Statut != "annulée" && suivi.Rappel != x.Date && x.debut.After(maintenant) {
			veille := x.debut.In(fz).AddDate(0, 0, -1)
			seuil := time.Date(veille.Year(), veille.Month(), veille.Day(), heureRappel, 0, 0, 0, fz)
			if !maintenant.Before(seuil) {
				s.notifier(store.NouvelleNotif{Genre: store.NotifRappel, Seance: x.ID, Lien: lienAgendaJoueur,
					Titre: "Rappel : " + x.Nom + ", " + x.quand(fz), Texte: x.details()},
					x.attendus(func(p participant) bool { return p.Reponse == "absent" })...)
				suivi.Rappel = x.Date
			}
		}
		if suivi != avant {
			_ = s.store.NoterSuiviSeance(x.ID, suivi)
		}
	}
}

func (s *Serveur) prevenirChangement(x *seanceAgenda, ancienne string, fz *time.Location) {
	etaitAnnulee := strings.HasSuffix(ancienne, "|annulée")
	n := store.NouvelleNotif{Seance: x.ID, Lien: lienAgendaJoueur, Texte: x.details()}
	switch {
	case x.Statut == "annulée" && !etaitAnnulee:
		n.Genre, n.Titre = store.NotifAnnulee, x.Nom+" est annulée ("+x.quand(fz)+")"
	case x.Statut != "annulée" && etaitAnnulee:
		n.Genre, n.Titre = store.NotifDeplacee, x.Nom+" est rétablie : "+x.quand(fz)
	case x.Statut != "annulée":
		n.Genre, n.Titre = store.NotifDeplacee, x.Nom+" est déplacée : "+x.quand(fz)
	default:
		return // annulée, et déplacée en plus : rien à dire de neuf
	}
	s.notifier(n, x.attendus(nil)...)
}

// veillerNotifications : une fois par minute, dans chaque univers servi.
func (a *Application) veillerNotifications() {
	t := time.NewTicker(intervalleVeille)
	defer t.Stop()
	for {
		select {
		case <-a.fin:
			return
		case maintenant := <-t.C:
			a.mu.RLock()
			l := a.ordre()
			a.mu.RUnlock()
			for _, o := range l {
				a.mu.RLock()
				servi := a.tous[o.st.ID()] == o
				if servi {
					o.requetes.entrer()
				}
				a.mu.RUnlock()
				if servi {
					o.srv.veillerNotifications(maintenant)
					o.requetes.sortir()
				}
			}
		}
	}
}

// ---------------------------------------------------------------- réponses et relance

// noterReponse : un joueur vient de répondre à une séance ; le MJ en est prévenu.
func (s *Serveur) noterReponse(x *seanceAgenda, pj int64, avant, apres string) {
	if avant == apres {
		return
	}
	qui := ""
	for _, p := range x.Participants {
		if p.PJ == pj {
			qui = p.Nom
			if p.Joueur != "" && p.Joueur != p.Nom {
				qui = p.Joueur + " (" + p.Nom + ")"
			}
		}
	}
	if qui == "" {
		return
	}
	titre := qui + " : " + apres + " à " + x.Nom
	if apres == "" {
		titre = qui + " retire sa réponse à " + x.Nom
	}
	s.notifier(store.NouvelleNotif{Genre: store.NotifReponse, Seance: x.ID, Lien: "#/agenda", Titre: titre, Texte: x.quand(s.fuseau())}, store.PourMJ)
}

// relancer : POST /api/agenda/{id}/relance — le MJ demande une réponse à ceux qui n'ont pas répondu
// (séance à venir et non annulée). Réponse : {"relances": n, "noms": [...]}.
func (s *Serveur) relancer(w http.ResponseWriter, r *http.Request) {
	id, err := idDe(r)
	if err != nil {
		erreurStore(w, err)
		return
	}
	l, err := s.seancesLocales()
	if err != nil {
		erreurStore(w, err)
		return
	}
	for _, x := range l {
		if x.ID != id {
			continue
		}
		if x.Statut == "annulée" || x.Statut == "jouée" || !x.fin.After(time.Now()) {
			erreur(w, 422, "Cette séance n'est plus à venir : rien à relancer.")
			return
		}
		pour, noms := []int64{}, []string{}
		for _, p := range x.Participants {
			if p.Reponse == "" {
				pour = append(pour, p.PJ)
				noms = append(noms, p.Nom)
			}
		}
		if len(pour) > 0 {
			s.notifier(store.NouvelleNotif{Genre: store.NotifRelance, Seance: x.ID, Lien: lienAgendaJoueur,
				Titre: "Le MJ attend votre réponse : " + x.Nom + ", " + x.quand(s.fuseau()), Texte: x.details()}, pour...)
			s.store.NoterFiche("agenda : relance", x.ID, x.Nom, strings.Join(noms, ", "), auteurDe(r))
		}
		repondre(w, 200, map[string]any{"relances": len(pour), "noms": noms})
		return
	}
	erreurStore(w, store.ErrIntrouvable)
}
