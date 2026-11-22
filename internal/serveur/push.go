package serveur

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"

	"github.com/Tchyze/cof_dm/internal/comptes"
	"github.com/Tchyze/cof_dm/internal/store"
)

// Notifications sur les appareils (Web Push, serveur seulement) : quand une notification entre dans
// une cloche, les appareils inscrits du compte concerné sonnent, même écran éteint. Il faut l'adresse
// publique du serveur (-origine, en HTTPS) : les navigateurs n'acceptent un service worker et un
// abonnement que sur une page sûre.
//
// Le message part chiffré pour l'appareil (seul le navigateur le lit) chez Google, Apple ou Mozilla,
// qui le relaient ; il reste neutre (« COF DM — Rappel de séance ») : ni nom, ni date, ni lieu. Le
// détail se lit en ouvrant l'application, dans la cloche.
//
// Heures calmes : de 22 h à 8 h (fuseau de l'univers), rien ne sonne ; la cloche se remplit tout de
// suite et les appareils sont prévenus à 8 h, en un seul message s'il y en a plusieurs.

const (
	regVapidPrive  = "push_vapid_prive"
	regVapidPublic = "push_vapid_public"
	debutCalme     = 22
	finCalme       = 8
	retardMax      = 12 * time.Hour // une notification plus vieille ne sonne plus (serveur arrêté)
	ttlPush        = 12 * 3600      // ce que le relais garde un message pour un appareil éteint
	delaiPush      = 15 * time.Second
)

// Ce que l'appareil affiche, par genre : jamais un détail de la partie.
var textesPush = map[string]string{
	store.NotifRappel:   "Rappel de séance",
	store.NotifDeplacee: "Séance déplacée",
	store.NotifAnnulee:  "Séance annulée",
	store.NotifRelance:  "Le MJ attend votre réponse",
	store.NotifReponse:  "Un joueur a répondu",
	store.NotifMessage:  "Message du MJ",
}

// GenresPush : les genres qu'un compte peut couper sur ses appareils (l'ordre de l'écran).
var GenresPush = []string{store.NotifRappel, store.NotifDeplacee, store.NotifAnnulee, store.NotifRelance, store.NotifReponse, store.NotifMessage}

// envoiPush : un message pour un appareil d'un compte.
type envoiPush struct {
	compte   string
	appareil comptes.Appareil
	charge   []byte
}

type pousseur struct {
	prive, public, sujet string
	comptes              *comptes.Registre
	// envoyer : l'envoi réel (remplacé dans les tests) ; code HTTP du relais.
	envoyer func(ctx context.Context, a comptes.Appareil, charge []byte) (int, error)
	file    chan []envoiPush
	horloge func() time.Time // l'heure, pour les heures calmes (remplacée dans les tests)
}

// nouveauPousseur : nil sans comptes ou sans adresse publique. Les clés VAPID du serveur sont tirées
// une fois et gardées dans ses réglages (jamais dans un univers).
func nouveauPousseur(reg Reglages, c *comptes.Registre, origine string) *pousseur {
	locale := strings.HasPrefix(origine, "http://localhost") || strings.HasPrefix(origine, "http://127.0.0.1") // essais sur cette machine
	if c == nil || reg == nil || !(strings.HasPrefix(origine, "https://") || locale) {
		return nil
	}
	prive, ok1 := reg.Meta(regVapidPrive)
	public, ok2 := reg.Meta(regVapidPublic)
	if !ok1 || !ok2 || prive == "" || public == "" {
		var err error
		if prive, public, err = webpush.GenerateVAPIDKeys(); err != nil {
			log.Printf("notifications sur les appareils : clés impossibles à tirer : %v", err)
			return nil
		}
		if reg.SetMeta(regVapidPrive, prive) != nil || reg.SetMeta(regVapidPublic, public) != nil {
			log.Printf("notifications sur les appareils : clés impossibles à garder")
			return nil
		}
	}
	p := &pousseur{prive: prive, public: public, sujet: origine, comptes: c, file: make(chan []envoiPush, 64), horloge: time.Now}
	p.envoyer = p.envoyerVraiment
	go p.boucle()
	return p
}

func (p *pousseur) envoyerVraiment(ctx context.Context, a comptes.Appareil, charge []byte) (int, error) {
	rep, err := webpush.SendNotificationWithContext(ctx, charge, &webpush.Subscription{Endpoint: a.Adresse, Keys: webpush.Keys{Auth: a.Auth, P256dh: a.P256dh}},
		&webpush.Options{Subscriber: p.sujet, VAPIDPublicKey: p.public, VAPIDPrivateKey: p.prive, TTL: ttlPush, Urgency: webpush.UrgencyNormal,
			HTTPClient: &http.Client{Timeout: delaiPush}})
	if err != nil {
		return 0, err
	}
	defer rep.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(rep.Body, 4096))
	return rep.StatusCode, nil
}

// boucle : les envois partent l'un après l'autre, hors de toute requête et de tout univers.
func (p *pousseur) boucle() {
	for lot := range p.file {
		for _, e := range lot {
			p.un(e)
		}
	}
}

func (p *pousseur) un(e envoiPush) {
	ctx, fin := context.WithTimeout(context.Background(), delaiPush)
	defer fin()
	code, err := p.envoyer(ctx, e.appareil, e.charge)
	switch {
	case err != nil:
		p.comptes.NoterEnvoi(e.compte, e.appareil.ID, false, false, "relais injoignable")
	case code == http.StatusNotFound || code == http.StatusGone:
		p.comptes.NoterEnvoi(e.compte, e.appareil.ID, false, true, "abonnement terminé")
	case code >= 300:
		p.comptes.NoterEnvoi(e.compte, e.appareil.ID, false, false, fmt.Sprintf("refusé par le relais (%d)", code))
	default:
		p.comptes.NoterEnvoi(e.compte, e.appareil.ID, true, false, "")
	}
}

// deposer : un lot d'envois, sans attendre (plein : il est perdu, la cloche reste).
func (p *pousseur) deposer(lot []envoiPush) {
	if len(lot) == 0 {
		return
	}
	select {
	case p.file <- lot:
	default:
		log.Printf("notifications sur les appareils : file pleine, %d envois perdus", len(lot))
	}
}

// chargePush : ce que lit le service worker (sw.js).
func chargePush(titre, texte, lien, etiquette string) []byte {
	b, _ := json.Marshal(map[string]string{"titre": titre, "texte": texte, "lien": lien, "etiquette": etiquette})
	return b
}

// ---------------------------------------------------------------- la file d'un univers

// heuresCalmes : de 22 h à 8 h dans le fuseau de l'univers.
func heuresCalmes(t time.Time, fz *time.Location) bool {
	h := t.In(fz).Hour()
	return h >= debutCalme || h < finCalme
}

// pousser envoie aux appareils les notifications qui ne l'ont pas encore été (sauf en heures
// calmes). Appelé après chaque dépôt et par le veilleur, chaque minute.
func (s *Serveur) pousser(maintenant time.Time) {
	p := s.opt.push
	if p == nil || s.acces.comptes == nil || heuresCalmes(maintenant, s.fuseau()) {
		return
	}
	s.pushMu.Lock()
	defer s.pushMu.Unlock()
	// les dates de création viennent de l'horloge du store : la fraîcheur se juge à la même horloge
	l, err := s.store.APousser(float64(time.Now().Add(-retardMax).UnixNano()) / 1e9)
	if err != nil || len(l) == 0 {
		return
	}
	// pour chaque compte : les genres qui l'attendent, dans l'ordre
	univers := s.store.ID()
	parCompte := map[string][]string{}
	ordre := []string{}
	ajouter := func(nom, genre string) {
		if _, ok := parCompte[nom]; !ok {
			ordre = append(ordre, nom)
		}
		parCompte[nom] = append(parCompte[nom], genre)
	}
	tous := s.acces.comptes.Liste()
	for _, n := range l {
		for _, c := range tous {
			if c.Inactif || len(c.Appareils) == 0 || slices.Contains(c.NotifsCoupees, n.Genre) {
				continue
			}
			role := c.Role(univers)
			if (n.Pour == store.PourMJ && role == comptes.MJ) || (n.Pour != store.PourMJ && role == comptes.Joueur && c.Persos[univers] == n.Pour) {
				ajouter(c.Nom, n.Genre)
			}
		}
	}
	nomUniv := url.QueryEscape(univers)
	lot := []envoiPush{}
	for _, nom := range ordre {
		c, ok := s.acces.comptes.Lire(nom)
		if !ok {
			continue
		}
		genres := parCompte[nom]
		texte := textesPush[genres[0]]
		if len(genres) > 1 {
			texte = fmt.Sprintf("%d nouvelles notifications", len(genres))
		}
		lien := "/joueur.html?univers=" + nomUniv + "#/agenda"
		if c.Role(univers) == comptes.MJ {
			lien = "/?univers=" + nomUniv + "#/agenda"
		}
		charge := chargePush("COF DM", texte, lien, "cofdm-"+univers)
		for _, a := range c.Appareils {
			lot = append(lot, envoiPush{compte: c.Nom, appareil: a, charge: charge})
		}
	}
	p.deposer(lot)
}

// ---------------------------------------------------------------- Mon compte : mes appareils

func (s *Serveur) routesPush() {
	m := s.mux
	m.HandleFunc("GET /api/compte/notifications", s.mesAppareils)
	m.HandleFunc("POST /api/compte/notifications/appareils", s.inscrireAppareil)
	m.HandleFunc("DELETE /api/compte/notifications/appareils/{id}", s.retirerMonAppareil)
	m.HandleFunc("PUT /api/compte/notifications/reglages", s.reglerMesNotifs)
	m.HandleFunc("POST /api/compte/notifications/essai", s.essaiNotif)
}

// vueAppareils : ce que le titulaire voit de ses appareils (jamais l'adresse ni les clés).
func vueAppareils(c comptes.Compte) []map[string]any {
	out := []map[string]any{}
	for _, a := range c.Appareils {
		v := map[string]any{"id": a.ID, "nom": a.Nom, "cree": a.Cree}
		if a.Recu != 0 {
			v["recu"] = a.Recu
		}
		if a.Derniere != "" {
			v["erreur"] = a.Derniere
		}
		out = append(out, v)
	}
	return out
}

func (s *Serveur) vueNotifsCompte(c comptes.Compte) map[string]any {
	v := map[string]any{"possible": s.opt.push != nil && !c.Admin, "cle": nil, "appareils": vueAppareils(c),
		"coupees": append([]string{}, c.NotifsCoupees...), "genres": GenresPush, "calme": []int{debutCalme, finCalme}}
	if s.opt.push != nil {
		v["cle"] = s.opt.push.public
	}
	return v
}

func (s *Serveur) compteNotifiable(w http.ResponseWriter, r *http.Request) (comptes.Compte, bool) {
	c, ok := s.compteAbonnable(w, r)
	if !ok {
		return c, false
	}
	if c.Admin {
		erreur(w, 403, "Un compte administrateur ne reçoit pas de notifications.")
		return c, false
	}
	return c, true
}

// mesAppareils : GET /api/compte/notifications.
func (s *Serveur) mesAppareils(w http.ResponseWriter, r *http.Request) {
	if c, ok := s.compteAbonnable(w, r); ok {
		repondre(w, 200, s.vueNotifsCompte(c))
	}
}

// inscrireAppareil : POST /api/compte/notifications/appareils {"nom", "abonnement": {"endpoint",
// "keys": {"p256dh", "auth"}}} — l'appareil d'où vient la demande recevra les notifications.
func (s *Serveur) inscrireAppareil(w http.ResponseWriter, r *http.Request) {
	c, ok := s.compteNotifiable(w, r)
	if !ok {
		return
	}
	if s.opt.push == nil {
		erreur(w, 409, "Ce serveur n'envoie pas de notifications : il lui faut son adresse publique en HTTPS (COF_DM_ORIGINE).")
		return
	}
	var in struct {
		Nom        string `json:"nom"`
		Abonnement struct {
			Endpoint       string   `json:"endpoint"`
			ExpirationTime *float64 `json:"expirationTime"` // donné par le navigateur, sans usage ici
			Keys           struct {
				P256dh string `json:"p256dh"`
				Auth   string `json:"auth"`
			} `json:"keys"`
		} `json:"abonnement"`
	}
	if err := lireJSON(r, &in); err != nil {
		erreurStore(w, err)
		return
	}
	ab := in.Abonnement
	u, err := url.Parse(ab.Endpoint)
	if err != nil || u.Scheme != "https" || u.Host == "" || len(ab.Endpoint) > 1024 || ab.Keys.P256dh == "" || ab.Keys.Auth == "" ||
		len(ab.Keys.P256dh) > 200 || len(ab.Keys.Auth) > 100 || !relaisConnu(u.Hostname()) {
		erreur(w, 422, "Abonnement du navigateur illisible.")
		return
	}
	a, err := s.acces.comptes.InscrireAppareil(c.Nom, comptes.Appareil{Nom: in.Nom, Adresse: ab.Endpoint, P256dh: ab.Keys.P256dh, Auth: ab.Keys.Auth})
	if err != nil {
		erreurCompte(w, err)
		return
	}
	s.noterCompte(c, "appareil inscrit aux notifications : "+a.Nom)
	s.acces.noter(r, "notifications_appareil", c.Nom)
	c, _ = s.acces.comptes.Lire(c.Nom)
	v := s.vueNotifsCompte(c)
	v["ici"] = a.ID
	repondre(w, 200, v)
}

// relaisConnu : l'adresse d'envoi est chez un relais de navigateur (Google, Mozilla, Apple, Microsoft).
// Le serveur n'envoie rien ailleurs : un abonnement inventé n'en fait pas un outil pour joindre une
// adresse de son choix.
func relaisConnu(hote string) bool {
	hote = strings.ToLower(hote)
	for _, d := range []string{"fcm.googleapis.com", "android.googleapis.com", "updates.push.services.mozilla.com", "push.apple.com", "notify.windows.com"} {
		if hote == d || strings.HasSuffix(hote, "."+d) {
			return true
		}
	}
	return false
}

// retirerMonAppareil : DELETE /api/compte/notifications/appareils/{id}.
func (s *Serveur) retirerMonAppareil(w http.ResponseWriter, r *http.Request) {
	c, ok := s.compteAbonnable(w, r)
	if !ok {
		return
	}
	if err := s.acces.comptes.RetirerAppareil(c.Nom, r.PathValue("id")); err != nil {
		erreurCompte(w, err)
		return
	}
	s.noterCompte(c, "appareil retiré des notifications")
	c, _ = s.acces.comptes.Lire(c.Nom)
	repondre(w, 200, s.vueNotifsCompte(c))
}

// reglerMesNotifs : PUT /api/compte/notifications/reglages {"coupees": ["rappel", …]}.
func (s *Serveur) reglerMesNotifs(w http.ResponseWriter, r *http.Request) {
	c, ok := s.compteNotifiable(w, r)
	if !ok {
		return
	}
	var in struct {
		Coupees []string `json:"coupees"`
	}
	if err := lireJSON(r, &in); err != nil {
		erreurStore(w, err)
		return
	}
	in.Coupees = slices.DeleteFunc(in.Coupees, func(g string) bool { return !slices.Contains(GenresPush, g) })
	if err := s.acces.comptes.ReglerNotifications(c.Nom, in.Coupees); err != nil {
		erreurCompte(w, err)
		return
	}
	c, _ = s.acces.comptes.Lire(c.Nom)
	repondre(w, 200, s.vueNotifsCompte(c))
}

// essaiNotif : POST /api/compte/notifications/essai — un message d'essai à tous ses appareils, tout
// de suite (même en heures calmes : c'est lui qui le demande).
func (s *Serveur) essaiNotif(w http.ResponseWriter, r *http.Request) {
	c, ok := s.compteNotifiable(w, r)
	if !ok {
		return
	}
	if s.opt.push == nil || len(c.Appareils) == 0 {
		erreur(w, 409, "Aucun appareil inscrit.")
		return
	}
	charge := chargePush("COF DM", "Les notifications arrivent bien sur cet appareil.", "/", "cofdm-essai")
	lot := []envoiPush{}
	for _, a := range c.Appareils {
		lot = append(lot, envoiPush{compte: c.Nom, appareil: a, charge: charge})
	}
	s.opt.push.deposer(lot)
	repondre(w, 200, map[string]any{"envoyes": len(lot)})
}

// adminRetirerAppareil : DELETE /api/admin/comptes/{nom}/appareils/{id} — l'administrateur retire un
// appareil d'un compte (perdu, volé), sans rien lire d'un univers.
func (a *Application) adminRetirerAppareil(w http.ResponseWriter, r *http.Request) {
	nom := comptes.Normaliser(r.PathValue("nom"))
	if _, ok := a.opt.Comptes.Lire(nom); !ok {
		erreurCompte(w, comptes.ErrInconnu)
		return
	}
	if err := a.opt.Comptes.RetirerAppareil(nom, r.PathValue("id")); err != nil {
		erreurCompte(w, err)
		return
	}
	a.opt.acces.noter(r, "notifications_appareil_retire", nom, "par="+auteurDe(r))
	c, _ := a.opt.Comptes.Lire(nom)
	repondre(w, 200, vueAdmin(c))
}
