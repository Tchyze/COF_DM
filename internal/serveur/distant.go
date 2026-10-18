package serveur

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Tchyze/cof_dm/internal/store"
	"github.com/Tchyze/cof_dm/internal/synchro"
)

// L'application de bureau travaille dans l'un de deux modes :
//
//   - local (client seul) : ses univers ne sont qu'à elle. Elle peut alors s'ouvrir au réseau local
//     (tablette du MJ, page des joueurs), si le MJ le veut.
//   - serveur : elle est reliée à un serveur COF DM (l'image Docker) par un compte. Un univers du
//     serveur y est une copie synchronisée, tenue à jour en arrière-plan ; hors connexion, les
//     changements attendent le retour du serveur. Elle ne répond alors plus au réseau local : la
//     tablette et les joueurs passent par le serveur.
//
// La connexion se fait avec le nom d'utilisateur et le mot de passe du compte, échangés aussitôt
// contre un jeton d'API propre à ce poste : le mot de passe n'est jamais gardé, et le jeton se
// retire du serveur quand le poste s'en déconnecte (ou à la main, dans « Mon compte »). Le mode, la
// connexion et les liens des univers tiennent à cet ordinateur : ils sont rangés dans les réglages,
// jamais dans un univers. Une copie d'un autre serveur, ou ouverte en mode local, est en pause : ses
// changements attendent qu'on se reconnecte à son serveur.
//
// Ces routes (/api/univers/serveur/…) ne répondent qu'à l'ordinateur du MJ, et pas sur un serveur.

const (
	regServeurs         = "serveurs"       // {identifiant d'univers : lien}
	regConnexion        = "serveur"        // la connexion au serveur ; absente : mode local
	intervalleSync      = 30 * time.Second // entre deux échanges, en arrière-plan
	modeLocal           = "local"
	modeServeurDistant  = "serveur"
	etatAJour           = "a_jour"
	etatHorsLigne       = "hors_ligne"
	etatErreur          = "erreur"
	etatJamais          = "jamais"
	etatRefuse          = "refuse"   // le serveur ne reconnaît plus ce poste (jeton retiré)
	etatConnecte        = "connecte" // le serveur répond
	etatInconnu         = "inconnu"  // pas encore essayé
	motifAvantEnvoi     = "avant-envoi"
	nomCopieParDefaut   = "Application de bureau"
	tailleMaxNomPoste   = 60
	erreurDejaCopie     = "Cet univers est déjà une copie d'un serveur."
	erreurSansConnexion = "Connectez d'abord l'application à un serveur."
	msgRefuse           = "Le serveur ne reconnaît plus ce poste (son jeton a été retiré) : reconnectez-vous."
	msgHorsLigne        = "Le serveur ne répond pas : les changements partiront au retour de la connexion."
	msgReseauRelie      = "L'application est reliée à un serveur : la tablette et la page des joueurs passent par lui. Passez en mode local pour ouvrir le réseau local."
)

// connexion : le serveur auquel l'application est reliée (mode serveur).
type connexion struct {
	URL    string `json:"url"`
	Compte string `json:"compte"`
	Jeton  string `json:"jeton"`
}

// lien : ce qui relie un univers de cet ordinateur à son serveur. Le jeton est celui de la
// connexion ; Jeton ne sert qu'à reprendre les liens d'avant les modes.
type lien struct {
	URL    string `json:"url"`
	Compte string `json:"compte"`
	Copie  string `json:"copie"`            // nom de cette copie sur le serveur
	Nom    string `json:"nom,omitempty"`    // nom de l'univers sur le serveur
	Chemin string `json:"chemin,omitempty"` // fichier de la copie sur cet ordinateur
	Jeton  string `json:"jeton,omitempty"`
}

// etatSync : où en est la synchronisation d'un univers.
type etatSync struct {
	Etat     string        `json:"etat"`
	Derniere float64       `json:"derniere,omitempty"` // dernier échange réussi
	Erreur   string        `json:"erreur,omitempty"`
	Bilan    synchro.Bilan `json:"bilan"`
}

// etatServeur : le serveur répond-il ?
type etatServeur struct {
	Etat    string  `json:"etat"`
	Verifie float64 `json:"verifie,omitempty"` // dernière réponse
	Erreur  string  `json:"erreur,omitempty"`
}

// distant : l'état de la synchronisation de l'application de bureau.
type distant struct {
	mu      sync.Mutex // un échange à la fois
	etatMu  sync.Mutex
	etats   map[string]*etatSync
	serveur etatServeur
	reveil  chan struct{}
}

func (a *Application) routesDistant() {
	a.dist = &distant{etats: map[string]*etatSync{}, serveur: etatServeur{Etat: etatInconnu}, reveil: make(chan struct{}, 1)}
	a.reprendreLiens()
	m := a.mux
	m.HandleFunc("GET /api/univers/serveur", a.routeDistant)
	m.HandleFunc("POST /api/univers/serveur/connexion", a.routeConnexionDistante)
	m.HandleFunc("POST /api/univers/serveur/local", a.routeModeLocal)
	m.HandleFunc("GET /api/univers/serveur/univers", a.routeUniversDistants)
	m.HandleFunc("POST /api/univers/serveur/relier", a.routeRelier)
	m.HandleFunc("POST /api/univers/serveur/ouvrir", a.routeOuvrirDistant)
	m.HandleFunc("POST /api/univers/serveur/envoyer", a.routeEnvoyer)
	m.HandleFunc("POST /api/univers/serveur/synchroniser", a.routeSynchroniser)
	m.HandleFunc("POST /api/univers/serveur/delier", a.routeDelier)
}

// ---------------------------------------------------------------- mode et connexion

func lireLiens(reg Reglages) map[string]lien {
	l := map[string]lien{}
	if v, ok := reg.Meta(regServeurs); ok {
		_ = json.Unmarshal([]byte(v), &l)
	}
	return l
}

func lireConnexion(reg Reglages) *connexion {
	v, ok := reg.Meta(regConnexion)
	if !ok || v == "" {
		return nil
	}
	var c connexion
	if json.Unmarshal([]byte(v), &c) != nil || c.URL == "" || c.Jeton == "" {
		return nil
	}
	return &c
}

// BureauRelie : l'application de bureau dont voici les réglages est reliée à un serveur (mode
// serveur). Elle ne doit alors pas s'ouvrir au réseau local. Les liens d'avant les modes, qui
// portaient chacun leur jeton, valent une connexion.
func BureauRelie(reg Reglages) bool {
	if lireConnexion(reg) != nil {
		return true
	}
	for _, l := range lireLiens(reg) {
		if l.Jeton != "" {
			return true
		}
	}
	return false
}

// reprendreLiens : avant les modes, chaque univers relié gardait son propre jeton et se
// synchronisait toujours. L'application passe en mode serveur avec la connexion d'un de ces liens ;
// les jetons quittent les liens.
func (a *Application) reprendreLiens() {
	if a.multi() {
		return
	}
	ls := lireLiens(a.reg)
	ids := make([]string, 0, len(ls))
	for id, l := range ls {
		if l.Jeton != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return
	}
	sort.Strings(ids)
	if lireConnexion(a.reg) == nil {
		l := ls[ids[0]]
		if err := a.ecrireConnexion(&connexion{URL: l.URL, Compte: l.Compte, Jeton: l.Jeton}); err != nil {
			log.Printf("reprise des liens : %v", err)
			return
		}
	}
	for _, id := range ids {
		l := ls[id]
		l.Jeton = ""
		ls[id] = l
	}
	b, _ := json.Marshal(ls)
	_ = a.reg.SetMeta(regServeurs, string(b))
}

// cnx : la connexion au serveur ; nil en mode local (et sur un serveur).
func (a *Application) cnx() *connexion {
	if a.multi() {
		return nil
	}
	return lireConnexion(a.reg)
}

// Relie : l'application de bureau est reliée à un serveur.
func (a *Application) Relie() bool { return a.cnx() != nil }

func (a *Application) ecrireConnexion(c *connexion) error {
	if c == nil {
		return a.reg.SetMeta(regConnexion, "")
	}
	b, _ := json.Marshal(c)
	return a.reg.SetMeta(regConnexion, string(b))
}

func (a *Application) liens() map[string]lien { return lireLiens(a.reg) }

// noterFichierCopie retient où est rangée la copie qui vient d'ouvrir, pour la rouvrir depuis la
// liste des univers du serveur (verrou d'écriture tenu).
func (a *Application) noterFichierCopie(st *store.Store) {
	if !st.EstCopie() {
		return
	}
	if l, ok := a.liens()[st.ID()]; ok && l.Chemin != st.Chemin() {
		l.Chemin = st.Chemin()
		_ = a.ecrireLien(st.ID(), &l)
	}
}

// fichier : le fichier de la copie de cet univers sur cet ordinateur, s'il existe encore.
func (l lien) fichier() string {
	if l.Chemin == "" {
		return ""
	}
	if info, err := os.Stat(l.Chemin); err != nil || info.IsDir() {
		return ""
	}
	return l.Chemin
}

func (a *Application) ecrireLien(univers string, l *lien) error {
	ls := a.liens()
	if l == nil {
		delete(ls, univers)
	} else {
		ls[univers] = *l
	}
	if len(ls) == 0 {
		return a.reg.SetMeta(regServeurs, "")
	}
	b, _ := json.Marshal(ls)
	return a.reg.SetMeta(regServeurs, string(b))
}

// courantRelie : l'univers ouvert, son lien s'il est une copie reliée, et la connexion qui le
// synchronise ; nil si la copie est en pause (mode local, ou autre serveur).
func (a *Application) courantRelie() (*store.Store, *lien, *connexion) {
	a.mu.RLock()
	st := a.st
	a.mu.RUnlock()
	if st == nil || !st.EstCopie() {
		return st, nil, nil
	}
	l, ok := a.liens()[st.ID()]
	if !ok {
		return st, nil, nil
	}
	if c := a.cnx(); c != nil && c.URL == l.URL {
		return st, &l, c
	}
	return st, &l, nil
}

func (a *Application) etatDe(univers string) etatSync {
	a.dist.etatMu.Lock()
	defer a.dist.etatMu.Unlock()
	if e, ok := a.dist.etats[univers]; ok {
		return *e
	}
	return etatSync{Etat: etatJamais}
}

func (a *Application) noterEtat(univers string, e etatSync) {
	a.dist.etatMu.Lock()
	defer a.dist.etatMu.Unlock()
	a.dist.etats[univers] = &e
}

func (a *Application) etatServeur() etatServeur {
	a.dist.etatMu.Lock()
	defer a.dist.etatMu.Unlock()
	return a.dist.serveur
}

// noterServeur : ce qu'un appel au serveur dit de la connexion. Un refus de validation ou d'univers
// vient d'un serveur qui répond : il reste « connecté ».
func (a *Application) noterServeur(err error) {
	e := etatServeur{Etat: etatConnecte, Verifie: float64(time.Now().UnixNano()) / 1e9}
	var se synchro.ErrServeur
	switch {
	case err == nil:
	case errors.Is(err, synchro.ErrInjoignable):
		e = a.etatServeur()
		e.Etat, e.Erreur = etatHorsLigne, msgHorsLigne
	case errors.As(err, &se) && se.Code == http.StatusUnauthorized:
		e.Etat, e.Erreur = etatRefuse, msgRefuse
	case errors.As(err, &se):
	default:
		return
	}
	a.dist.etatMu.Lock()
	a.dist.serveur = e
	a.dist.etatMu.Unlock()
}

// echanger : un échange de l'univers ouvert avec son serveur, s'il y est relié et pas en pause.
func (a *Application) echanger() (etatSync, error) {
	a.dist.mu.Lock()
	defer a.dist.mu.Unlock()
	st, l, c := a.courantRelie()
	if l == nil {
		return etatSync{}, store.ErrValidation{Msg: "Cet univers n'est relié à aucun serveur."}
	}
	if c == nil {
		return etatSync{}, store.ErrValidation{Msg: "Cette copie est en pause : connectez l'application à " + l.URL + " pour la synchroniser."}
	}
	e := a.etatDe(st.ID())
	bilan, err := synchro.Nouveau(c.URL, c.Jeton, st.ID()).Echanger(st)
	a.noterServeur(err)
	var se synchro.ErrServeur
	switch {
	case err == nil:
		e = etatSync{Etat: etatAJour, Derniere: float64(time.Now().UnixNano()) / 1e9, Bilan: bilan}
	case errors.Is(err, synchro.ErrInjoignable):
		e.Etat, e.Erreur = etatHorsLigne, msgHorsLigne
	case errors.As(err, &se) && se.Code == http.StatusUnauthorized:
		e.Etat, e.Erreur = etatErreur, msgRefuse
	case errors.As(err, &se):
		e.Etat, e.Erreur = etatErreur, se.Msg
	default:
		e.Etat, e.Erreur = etatErreur, err.Error()
		log.Printf("synchronisation : %v", err)
	}
	a.noterEtat(st.ID(), e)
	return e, err
}

// sonder : le serveur répond-il ? (quand l'univers ouvert n'est pas une copie de ce serveur)
func (a *Application) sonder() {
	if c := a.cnx(); c != nil {
		_, err := synchro.Nouveau(c.URL, c.Jeton, "").MesUnivers()
		a.noterServeur(err)
	}
}

// synchroniserEnFond tient la copie ouverte à jour : un échange toutes les 30 secondes, ou dès
// qu'on le demande. Sans copie à tenir, il vérifie seulement que le serveur répond.
func (a *Application) synchroniserEnFond() {
	t := time.NewTicker(intervalleSync)
	defer t.Stop()
	a.reveiller() // un premier échange dès le lancement
	for {
		select {
		case <-a.fin:
			return
		case <-t.C:
		case <-a.dist.reveil:
		}
		if _, _, c := a.courantRelie(); c != nil {
			_, _ = a.echanger()
		} else {
			a.sonder()
		}
	}
}

// reveiller demande un échange sans attendre.
func (a *Application) reveiller() {
	select {
	case a.dist.reveil <- struct{}{}:
	default:
	}
}

func refuserSurServeur(a *Application, w http.ResponseWriter) bool {
	if a.multi() {
		erreur(w, 404, "Un serveur ne se relie pas à un autre serveur.")
		return true
	}
	return false
}

// routeDistant : le mode de l'application, son serveur, et où en est l'univers ouvert.
func (a *Application) routeDistant(w http.ResponseWriter, r *http.Request) {
	if refuserSurServeur(a, w) {
		return
	}
	repondre(w, 200, a.vueDistante())
}

func (a *Application) vueDistante() map[string]any {
	st, l, c := a.courantRelie()
	out := map[string]any{"mode": modeLocal, "relie": false, "actif": false, "pause": false,
		"copie": st != nil && st.EstCopie(), "reseau": a.opt.Reseau}
	if cx := a.cnx(); cx != nil {
		out["mode"] = modeServeurDistant
		e := a.etatServeur()
		out["serveur"] = map[string]any{"url": cx.URL, "compte": cx.Compte, "etat": e.Etat, "verifie": e.Verifie, "erreur": e.Erreur}
	}
	if l != nil {
		out["relie"], out["actif"], out["pause"] = true, c != nil, c == nil
		out["url"], out["compte"], out["nom_copie"] = l.URL, l.Compte, l.Copie
		out["synchro"] = a.etatDe(st.ID())
		cs, _ := st.Conflits()
		out["conflits"] = len(cs)
	}
	return out
}

// passerDeconnecte : l'application quitte son serveur. Le jeton de ce poste est rendu au serveur ;
// s'il ne répond pas, il y reste valable (renvoie false) et se retire dans « Mon compte ».
func (a *Application) passerDeconnecte(c *connexion) bool {
	if c == nil {
		return true
	}
	if err := synchro.Rendre(c.URL, c.Jeton); err != nil {
		log.Printf("jeton non rendu à %s : %v", c.URL, err)
		return false
	}
	return true
}

// routeConnexionDistante : {"url", "nom", "mdp"} relie l'application au serveur (mode serveur) et
// renvoie les univers du compte. Seul un jeton d'API est gardé. Une connexion d'avant (autre
// serveur, ou autre compte) est remplacée et son jeton rendu ; les copies d'un autre serveur
// passent en pause.
func (a *Application) routeConnexionDistante(w http.ResponseWriter, r *http.Request) {
	if refuserSurServeur(a, w) {
		return
	}
	var in struct {
		URL string `json:"url"`
		Nom string `json:"nom"`
		Mdp string `json:"mdp"`
	}
	if err := lireJSON(r, &in); err != nil {
		erreurStore(w, err)
		return
	}
	base, err := synchro.Adresse(in.URL)
	if err != nil {
		erreur(w, 422, majuscule(err.Error())+".")
		return
	}
	nom := strings.TrimSpace(in.Nom)
	jeton, univers, err := synchro.Connecter(base, nom, in.Mdp, nomDuPoste())
	if err != nil {
		erreurDistante(w, err)
		return
	}
	a.dist.mu.Lock()
	ancienne := a.cnx()
	err = a.ecrireConnexion(&connexion{URL: base, Compte: nom, Jeton: jeton})
	a.dist.mu.Unlock()
	if err != nil {
		_ = synchro.Rendre(base, jeton)
		erreurStore(w, err)
		return
	}
	a.noterServeur(nil)
	a.fermerReseau()
	rendu := a.passerDeconnecte(ancienne)
	a.reveiller()
	out := a.vueDistante()
	out["url"], out["univers"] = base, a.marquerIci(univers)
	if !rendu {
		out["avertissement"] = "Le jeton de l'ancienne connexion n'a pas pu être retiré de " + ancienne.URL + " : il le sera à votre prochaine connexion à ce serveur, ou retirez-le dans « Mon compte »."
	}
	repondre(w, 200, out)
}

// fermerReseau : en passant en mode serveur, les appareils du réseau qui suivaient la page des
// joueurs sont coupés ; plus rien du réseau n'entre (voir Serveur.ServeHTTP).
func (a *Application) fermerReseau() {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.srv != nil && a.srv.joueurs != nil {
		a.srv.joueurs.fermer()
	}
}

// routeModeLocal : l'application quitte son serveur et travaille seule (client seul). Les copies
// restent sur l'ordinateur, en pause ; elles reprendront si l'on se reconnecte au même serveur.
func (a *Application) routeModeLocal(w http.ResponseWriter, r *http.Request) {
	if refuserSurServeur(a, w) {
		return
	}
	a.dist.mu.Lock()
	c := a.cnx()
	err := a.ecrireConnexion(nil)
	a.dist.mu.Unlock()
	if err != nil {
		erreurStore(w, err)
		return
	}
	a.dist.etatMu.Lock()
	a.dist.serveur = etatServeur{Etat: etatInconnu}
	a.dist.etatMu.Unlock()
	out := a.vueDistante()
	if !a.passerDeconnecte(c) {
		out["avertissement"] = "Le serveur ne répond pas : le jeton de ce poste y reste valable. Il sera retiré à votre prochaine connexion à " + c.URL + ", ou retirez-le dans « Mon compte »."
	}
	repondre(w, 200, out)
}

// univDistant : un univers du serveur ; Ici : le fichier de sa copie s'il est déjà sur cet
// ordinateur ; Ouvert : c'est l'univers ouvert.
type univDistant struct {
	synchro.Univers
	Ici    string `json:"ici,omitempty"`
	Ouvert bool   `json:"ouvert"`
}

func (a *Application) marquerIci(us []synchro.Univers) []univDistant {
	a.mu.RLock()
	courant := ""
	if a.st != nil {
		courant = a.st.ID()
	}
	a.mu.RUnlock()
	out := make([]univDistant, 0, len(us))
	for _, u := range us {
		out = append(out, univDistant{u, a.parIdentifiant(u.ID), u.ID == courant})
	}
	sort.SliceStable(out, func(i, j int) bool { return strings.ToLower(out[i].Nom) < strings.ToLower(out[j].Nom) })
	return out
}

// copiesDe : les copies de ce serveur rangées sur cet ordinateur, quand il ne répond pas.
func (a *Application) copiesDe(url string) []synchro.Univers {
	us := []synchro.Univers{}
	for id, l := range a.liens() {
		if l.URL == url && l.fichier() != "" {
			nom := l.Nom
			if nom == "" {
				nom = nomUnivers(l.Chemin)
			}
			us = append(us, synchro.Univers{ID: id, Nom: nom, Role: "mj"})
		}
	}
	return us
}

// routeUniversDistants : les univers du serveur où le compte a un rôle.
func (a *Application) routeUniversDistants(w http.ResponseWriter, r *http.Request) {
	if refuserSurServeur(a, w) {
		return
	}
	c := a.cnx()
	if c == nil {
		erreur(w, 409, erreurSansConnexion)
		return
	}
	us, err := synchro.Nouveau(c.URL, c.Jeton, "").MesUnivers()
	a.noterServeur(err)
	if errors.Is(err, synchro.ErrInjoignable) { // hors connexion : les copies de cet ordinateur
		repondre(w, 200, map[string]any{"url": c.URL, "hors_ligne": true, "univers": a.marquerIci(a.copiesDe(c.URL))})
		return
	}
	if err != nil {
		erreurDistante(w, err)
		return
	}
	repondre(w, 200, map[string]any{"url": c.URL, "univers": a.marquerIci(us)})
}

func erreurDistante(w http.ResponseWriter, err error) {
	var se synchro.ErrServeur
	switch {
	case errors.Is(err, synchro.ErrInjoignable):
		erreur(w, 502, "Le serveur ne répond pas : vérifiez l'adresse et la connexion.")
	case errors.As(err, &se):
		erreur(w, 422, "Le serveur refuse : "+se.Msg)
	default:
		erreurStore(w, err)
	}
}

func nomDuPoste() string {
	h, err := os.Hostname()
	if err != nil || strings.TrimSpace(h) == "" {
		return nomCopieParDefaut
	}
	if r := []rune(h); len(r) > tailleMaxNomPoste {
		h = string(r[:tailleMaxNomPoste])
	}
	return h
}

// routeRelier : {"univers", "copie"?} tire une copie d'un univers du serveur, l'ouvre et la relie.
func (a *Application) routeRelier(w http.ResponseWriter, r *http.Request) {
	if refuserSurServeur(a, w) {
		return
	}
	var in struct {
		Univers string `json:"univers"`
		Copie   string `json:"copie"`
	}
	if err := lireJSON(r, &in); err != nil {
		erreurStore(w, err)
		return
	}
	a.copierEtOuvrir(w, r, in.Univers, in.Copie)
}

// routeOuvrirDistant : {"univers", "copie"?} passe à un univers du serveur : sa copie sur cet
// ordinateur s'ouvre (même hors connexion) ; s'il n'y en a pas, elle est tirée d'abord.
func (a *Application) routeOuvrirDistant(w http.ResponseWriter, r *http.Request) {
	if refuserSurServeur(a, w) {
		return
	}
	var in struct {
		Univers string `json:"univers"`
		Copie   string `json:"copie"`
	}
	if err := lireJSON(r, &in); err != nil {
		erreurStore(w, err)
		return
	}
	c := a.cnx()
	if c == nil {
		erreur(w, 409, erreurSansConnexion)
		return
	}
	if l, ok := a.liens()[in.Univers]; ok && l.fichier() != "" {
		if _, err := a.ouvrir(l.Chemin, false); err != nil {
			erreurStore(w, err)
			return
		}
		a.choisi()
		a.reveiller() // un échange aussitôt, si c'est une copie de ce serveur
		a.etat(w, r)
		return
	}
	a.copierEtOuvrir(w, r, in.Univers, in.Copie)
}

// copierEtOuvrir tire une copie d'un univers du serveur, l'ouvre et la relie.
func (a *Application) copierEtOuvrir(w http.ResponseWriter, r *http.Request, univers, nomCopie string) {
	c := a.cnx()
	if c == nil {
		erreur(w, 409, erreurSansConnexion)
		return
	}
	us, err := synchro.Nouveau(c.URL, c.Jeton, "").MesUnivers()
	a.noterServeur(err)
	if err != nil {
		erreurDistante(w, err)
		return
	}
	var choisi *synchro.Univers
	for i := range us {
		if us[i].ID == univers {
			choisi = &us[i]
		}
	}
	if choisi == nil {
		erreur(w, 404, "Univers inconnu sur ce serveur.")
		return
	}
	if choisi.Role != "mj" {
		erreur(w, 403, "La synchronisation est réservée au MJ de l'univers et à l'administrateur.")
		return
	}
	if o := a.parIdentifiant(choisi.ID); o != "" {
		erreur(w, 409, "Cet univers est déjà sur cet ordinateur : "+o)
		return
	}
	copie := strings.TrimSpace(nomCopie)
	if copie == "" {
		copie = nomDuPoste()
	}
	fichier := fichierLibre(a.DossierUnivers(), nomDeFichier(choisi.Nom))
	if err := os.MkdirAll(filepath.Dir(fichier), 0o700); err != nil {
		erreurStore(w, err)
		return
	}
	if _, err := synchro.Nouveau(c.URL, c.Jeton, choisi.ID).Copier(copie, fichier); err != nil {
		erreurDistante(w, err)
		return
	}
	if err := a.ecrireLien(choisi.ID, &lien{URL: c.URL, Compte: c.Compte, Copie: copie, Nom: choisi.Nom, Chemin: fichier}); err != nil {
		_ = os.Remove(fichier)
		erreurStore(w, err)
		return
	}
	if _, err := a.ouvrir(fichier, false); err != nil {
		erreurStore(w, err)
		return
	}
	a.choisi()
	a.noterEtat(choisi.ID, etatSync{Etat: etatAJour, Derniere: float64(time.Now().UnixNano()) / 1e9})
	a.etat(w, r)
}

// parIdentifiant : le fichier d'un univers connu de cet ordinateur (ouvert ou récent) qui porte cet
// identifiant ; vide s'il n'y en a pas.
func (a *Application) parIdentifiant(id string) string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if o, ok := a.tous[id]; ok {
		return o.st.Chemin()
	}
	if l, ok := a.liens()[id]; ok {
		if f := l.fichier(); f != "" {
			return f
		}
		if l.Chemin == "" {
			return "une copie déjà reliée"
		}
		// le fichier de la copie a disparu : elle peut être tirée de nouveau
	}
	return ""
}

// fichierLibre : un fichier d'univers qui n'existe pas encore, « Nom », puis « Nom (2) »…
func fichierLibre(dossier, nom string) string {
	if nom == "" {
		nom = "Univers du serveur"
	}
	f := filepath.Join(dossier, nom+store.Extension)
	for i := 2; ; i++ {
		if _, err := os.Stat(f); errors.Is(err, os.ErrNotExist) {
			return f
		}
		f = filepath.Join(dossier, fmt.Sprintf("%s (%d)%s", nom, i, store.Extension))
	}
}

// routeEnvoyer : {"nom"} publie l'univers ouvert sur le serveur (compte administrateur), puis le
// remplace ici par sa copie reliée. L'univers d'avant est gardé dans ses sauvegardes.
func (a *Application) routeEnvoyer(w http.ResponseWriter, r *http.Request) {
	if refuserSurServeur(a, w) {
		return
	}
	var in struct {
		Nom   string `json:"nom"`
		Copie string `json:"copie"`
	}
	if err := lireJSON(r, &in); err != nil {
		erreurStore(w, err)
		return
	}
	c := a.cnx()
	if c == nil {
		erreur(w, 409, erreurSansConnexion)
		return
	}
	a.mu.RLock()
	st := a.st
	a.mu.RUnlock()
	if st == nil {
		erreur(w, 409, "Aucun univers n'est ouvert.")
		return
	}
	if st.EstCopie() {
		erreur(w, 409, erreurDejaCopie)
		return
	}
	nom := strings.TrimSpace(in.Nom)
	if nom == "" {
		nom = nomUnivers(st.Chemin())
	}
	copie := strings.TrimSpace(in.Copie)
	if copie == "" {
		copie = nomDuPoste()
	}
	dossier := filepath.Dir(st.Chemin())
	instantane, err := os.CreateTemp(dossier, ".envoi-*.tmp")
	if err != nil {
		erreurStore(w, err)
		return
	}
	instantane.Close()
	os.Remove(instantane.Name())
	defer os.Remove(instantane.Name())
	if err := st.Sauvegarder(instantane.Name()); err != nil {
		erreurStore(w, err)
		return
	}
	cl := synchro.Nouveau(c.URL, c.Jeton, "")
	id, err := cl.Deposer(nom, instantane.Name())
	if err != nil {
		erreurDistante(w, err)
		return
	}
	cl.Univers = id
	recue := instantane.Name() + ".copie"
	defer os.Remove(recue)
	if _, err := cl.Copier(copie, recue); err != nil {
		erreurDistante(w, err)
		return
	}
	if _, err := sauvegarder(st, a.dossierSauvegardes(st), motifAvantEnvoi); err != nil {
		erreur(w, 500, "L'univers est sur le serveur, mais sa copie n'a pas remplacé l'original (sauvegarde impossible). Copiez-le depuis le serveur (zone en bas à gauche). "+msgInterne)
		log.Printf("sauvegarde avant envoi : %v", err)
		return
	}
	chemin := st.Chemin()
	a.mu.Lock()
	if a.st == st {
		a.fermerCourant()
	}
	err = os.Rename(recue, chemin)
	a.mu.Unlock()
	if err != nil {
		// l'original est resté en place : il est rouvert tel quel
		log.Printf("remplacement par la copie : %v", err)
		_, _ = a.ouvrir(chemin, false)
		erreur(w, 500, "L'univers est sur le serveur, mais sa copie n'a pas remplacé l'original. Copiez-le depuis le serveur (zone en bas à gauche). "+msgInterne)
		return
	}
	if err := a.ecrireLien(id, &lien{URL: c.URL, Compte: c.Compte, Copie: copie, Nom: nom, Chemin: chemin}); err != nil {
		erreurStore(w, err)
		return
	}
	if _, err := a.ouvrir(chemin, false); err != nil {
		erreurStore(w, err)
		return
	}
	a.noterEtat(id, etatSync{Etat: etatAJour, Derniere: float64(time.Now().UnixNano()) / 1e9})
	a.etat(w, r)
}

// routeSynchroniser : un échange tout de suite.
func (a *Application) routeSynchroniser(w http.ResponseWriter, r *http.Request) {
	if refuserSurServeur(a, w) {
		return
	}
	e, err := a.echanger()
	var ev store.ErrValidation
	if errors.As(err, &ev) {
		erreur(w, 409, ev.Msg)
		return
	}
	repondre(w, 200, e) // hors ligne ou refus : l'état le dit
}

// routeDelier : l'univers ouvert n'est plus synchronisé (la copie reste, avec ses données).
func (a *Application) routeDelier(w http.ResponseWriter, r *http.Request) {
	if refuserSurServeur(a, w) {
		return
	}
	st, l, _ := a.courantRelie()
	if l == nil {
		erreur(w, 409, "Cet univers n'est relié à aucun serveur.")
		return
	}
	if err := a.ecrireLien(st.ID(), nil); err != nil {
		erreurStore(w, err)
		return
	}
	st.Noter("synchronisation", "copie déliée de "+l.URL, "MJ")
	repondre(w, 200, map[string]any{"relie": false})
}
