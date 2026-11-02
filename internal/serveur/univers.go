package serveur

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/Tchyze/cof_dm/internal/comptes"
	"github.com/Tchyze/cof_dm/internal/reglages"
	"github.com/Tchyze/cof_dm/internal/store"
)

// Application tient l'univers ouvert et permet d'en changer (menu Fichier) : un univers est un
// fichier .cofdm, où qu'il soit sur l'ordinateur. Les routes /api/univers ne répondent qu'à
// l'ordinateur du MJ ; tout le reste est servi par le Serveur de l'univers ouvert.
type Application struct {
	opt Options
	reg Reglages
	mux *http.ServeMux
	fin chan struct{}

	gestion        sync.Mutex    // une seule opération à la fois sur les univers (fermeture.go)
	delaiFermeture time.Duration // ce qu'une fermeture attend les requêtes en cours

	mu            sync.RWMutex // quelques instructions seulement, jamais pendant une requête (fermeture.go)
	st            *store.Store // l'univers courant ; sur un serveur, celui qui sert par défaut
	srv           *Serveur
	tous          map[string]*universOuvert // les univers ouverts, par identifiant : un seul, sauf sur un serveur
	fermetures    map[string]bool           // les univers qu'une opération a retirés du service, et qui répondent 503
	avertissement string                    // à montrer au MJ (univers introuvable au lancement…)
	dist          *distant                  // application de bureau : le lien avec un serveur
	maj           *misesAJour               // application de bureau : les mises à jour
	admin         *http.ServeMux            // serveur : l'administration (admin.go)
}

// universOuvert : un univers ouvert et le serveur de son API.
type universOuvert struct {
	st       *store.Store
	srv      *Serveur
	requetes requetes // le travail en cours : une fermeture l'attend (fermeture.go)
	defaut   bool     // il servait par défaut quand il a été suspendu
}

const (
	regUnivers     = "univers"
	regRecents     = "recents"
	regSansUnivers = "sans_univers" // le MJ a retiré son dernier univers : aucun ne se crée d'office au lancement
	recentsMax     = 8
	nomParDefaut   = "Mon univers"
)

// reglagesAppli : réglages rangés autrefois dans la base, qui ne doivent pas suivre un univers.
var reglagesAppli = []string{metaMdp, metaSecret, "api_jeton"}

// reglagesPerimes : réglages de l'ancien pont vers tracker_COF, retirés des univers qui les portent.
var reglagesPerimes = []string{"tracker_url", "tracker_mdp"}

func NouvelleApplication(opt Options) *Application {
	if opt.Reglages == nil {
		opt.Reglages = reglages.Ouvrir(filepath.Join(opt.DossierDon, "reglages.json"))
	}
	if opt.Comptes != nil {
		opt.acces = nouvelAcces(opt.Reglages, opt.Comptes, opt.Proxies)
		opt.acces.cles = opt.Cles
	}
	if opt.Comptes != nil && opt.Limites == nil {
		opt.Limites = LimitesParDefaut()
	}
	opt.partagerVoulu()
	a := &Application{opt: opt, reg: opt.Reglages, mux: http.NewServeMux(), fin: make(chan struct{}), tous: map[string]*universOuvert{},
		fermetures: map[string]bool{}, delaiFermeture: delaiFermeture}
	if opt.Comptes == nil {
		a.opt.Relie = a.Relie
		a.opt.app = a
	}
	m := a.mux
	m.HandleFunc("GET /api/univers", a.etat)
	m.HandleFunc("POST /api/univers/ouvrir", a.routeOuvrir)
	m.HandleFunc("POST /api/univers/nouveau", a.routeNouveau)
	m.HandleFunc("POST /api/univers/copie", a.routeCopie)
	m.HandleFunc("POST /api/univers/parcourir", a.routeParcourir)
	m.HandleFunc("POST /api/univers/restaurer", a.routeRestaurer)
	m.HandleFunc("POST /api/univers/bibliotheque", a.routeBibliotheque)
	a.routesDistant()
	if !a.multi() {
		a.routesMaj()
		a.routesGestion()
	} else {
		a.routesMajServeur()
		a.admin = a.routesAdmin()
	}
	return a
}

// DossierUnivers est le dossier proposé pour les nouveaux univers.
func (a *Application) DossierUnivers() string { return filepath.Join(a.opt.DossierDon, "univers") }

func (a *Application) dossierSauvegardes(st *store.Store) string {
	return filepath.Join(a.opt.DossierDon, "sauvegardes", st.ID())
}

func nomUnivers(chemin string) string {
	return strings.TrimSuffix(filepath.Base(chemin), filepath.Ext(chemin))
}

// Demarrer ouvre l'univers demandé, sinon le dernier ouvert, sinon l'univers par défaut (créé au
// besoin), après avoir repris les données d'une version d'avant les fichiers d'univers.
func (a *Application) Demarrer(demande string) error {
	if err := a.reprendre(); err != nil {
		return fmt.Errorf("reprise des anciennes données : %w", err)
	}
	if demande != "" {
		if err := a.Ouvrir(demande, true); err != nil {
			return err
		}
	} else if !a.resteSansUnivers() {
		dernier, _ := a.reg.Meta(regUnivers)
		if dernier == "" || a.Ouvrir(dernier, false) != nil {
			if err := a.Ouvrir(filepath.Join(a.DossierUnivers(), nomParDefaut+store.Extension), true); err != nil {
				return err
			}
			if dernier != "" {
				a.mu.Lock()
				a.avertissement = fmt.Sprintf("L'univers « %s » est introuvable (%s) : « %s » a été ouvert à la place.", nomUnivers(dernier), dernier, nomParDefaut)
				a.mu.Unlock()
				log.Print(a.avertissement)
			}
		}
	}
	if a.multi() {
		a.ouvrirDossier()
		if a.opt.VeilleMaj {
			go a.verifierEnFond()
		}
	}
	go a.veiller()
	if !a.multi() {
		go a.synchroniserEnFond()
		if a.opt.Relancer != nil { // l'application lancée par un MJ, pas un banc d'essai
			go a.verifierEnFond()
		}
	}
	return nil
}

// multi : mode serveur. Tous les univers du dossier du serveur sont ouverts en même temps, et
// chaque requête dit lequel elle vise (multi.go).
func (a *Application) multi() bool { return a.opt.Comptes != nil }

// reprendre transforme les données d'avant les fichiers d'univers (cof-dm.db et le dossier medias)
// en un univers. Rien n'est effacé : l'ancienne base est renommée, les images restent en place.
func (a *Application) reprendre() error {
	ancienne := filepath.Join(a.opt.DossierDon, "cof-dm.db")
	if _, deja := a.reg.Meta(regUnivers); deja {
		return nil
	}
	if _, err := os.Stat(ancienne); err != nil {
		return nil
	}
	cible := filepath.Join(a.DossierUnivers(), nomParDefaut+store.Extension)
	if _, err := os.Stat(cible); err == nil {
		return nil // déjà repris
	}
	if err := store.CopierBase(ancienne, cible); err != nil {
		_ = os.Remove(cible)
		return err
	}
	st, err := store.Ouvrir(cible)
	if err != nil {
		_ = os.Remove(cible)
		return err
	}
	n, absentes, err := st.ImporterMedias(filepath.Join(a.opt.DossierDon, "medias"))
	if err != nil {
		st.Fermer()
		_ = os.Remove(cible)
		return err
	}
	for _, k := range reglagesAppli {
		if v, ok := st.Meta(k); ok && v != "" {
			if _, deja := a.reg.Meta(k); !deja {
				_ = a.reg.SetMeta(k, v)
			}
		}
	}
	detail := fmt.Sprintf("données reprises dans le fichier %s (%d images)", filepath.Base(cible), n)
	if len(absentes) > 0 {
		detail += fmt.Sprintf(" ; %d images introuvables : %s", len(absentes), strings.Join(absentes, ", "))
		a.avertissement = fmt.Sprintf("%d images n'ont pas été retrouvées pendant la reprise des données (voir le journal).", len(absentes))
	}
	st.Noter("univers", detail, "cof-dm")
	if err := st.Fermer(); err != nil {
		return err
	}
	if err := os.Rename(ancienne, ancienne+".ancien"); err != nil {
		log.Printf("ancienne base non renommée : %v", err)
	}
	return a.reg.SetMeta(regUnivers, cible)
}

// Ouvrir ouvre un fichier d'univers et en fait l'univers courant. Avec creer, le fichier est créé
// s'il n'existe pas ; sinon il doit exister. En cas d'échec, l'univers courant reste ouvert.
func (a *Application) Ouvrir(chemin string, creer bool) error {
	_, err := a.ouvrir(chemin, creer)
	return err
}

// ouvrir : comme Ouvrir, et renvoie l'univers ouvert. Sur un serveur, les univers déjà ouverts le
// restent.
func (a *Application) ouvrir(chemin string, creer bool) (*universOuvert, error) {
	chemin, err := filepath.Abs(strings.TrimSpace(chemin))
	if err != nil || strings.TrimSpace(chemin) == "" {
		return nil, store.ErrValidation{Msg: "Chemin d'univers invalide."}
	}
	info, err := os.Stat(chemin)
	switch {
	case err == nil && info.IsDir():
		return nil, store.ErrValidation{Msg: "C'est un dossier, pas un fichier d'univers."}
	case err != nil && !creer:
		return nil, store.ErrValidation{Msg: "Fichier introuvable : " + chemin}
	}
	a.gestion.Lock()
	defer a.gestion.Unlock()
	if o := a.parChemin(chemin); o != nil {
		return o, nil
	}
	nouveau := err != nil
	st, err := store.Ouvrir(chemin)
	if err != nil {
		if nouveau {
			_ = os.Remove(chemin)
		}
		if errors.Is(err, store.ErrPasUnivers) {
			return nil, store.ErrValidation{Msg: "Ce fichier n'est pas un univers COF DM."}
		}
		return nil, err
	}
	_ = st.RetirerMeta(reglagesAppli...)
	_ = st.RetirerMeta(reglagesPerimes...)
	if !a.multi() {
		if !a.fermerCourant() {
			_ = st.Fermer()
			if nouveau {
				_ = os.Remove(chemin)
			}
			return nil, errOccupe
		}
	} else if deja, pris := a.tous[st.ID()]; pris { // la copie d'un univers porte son identifiant
		_ = st.Fermer()
		return nil, store.ErrValidation{Msg: "Cet univers est déjà ouvert sous le nom « " + nomUnivers(deja.st.Chemin()) + " » : ce fichier en est une copie."}
	}
	return a.installer(st), nil
}

// parChemin : l'univers ouvert qui tient dans ce fichier (a.mu ou a.gestion tenu).
func (a *Application) parChemin(chemin string) *universOuvert {
	for _, o := range a.tous {
		if memeFichier(o.st.Chemin(), chemin) {
			return o
		}
	}
	return nil
}

// Avertir ajoute un mot pour le MJ, montré avec l'univers ouvert au lancement (données reprises du
// mode portable, installation…).
func (a *Application) Avertir(msg string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.avertissement != "" {
		a.avertissement += " "
	}
	a.avertissement += msg
}

// choisi : le MJ vient de choisir son univers, l'avertissement du lancement n'a plus lieu d'être.
func (a *Application) choisi() {
	a.mu.Lock()
	a.avertissement = ""
	a.mu.Unlock()
}

func memeFichier(x, y string) bool {
	ix, err1 := os.Stat(x)
	iy, err2 := os.Stat(y)
	return err1 == nil && err2 == nil && os.SameFile(ix, iy)
}

// installer sert l'univers st, et en fait l'univers courant s'il n'y en a pas d'autre (a.gestion
// tenu). Sa sauvegarde automatique se fait une fois a.mu rendu.
func (a *Application) installer(st *store.Store) *universOuvert {
	opt := a.opt
	opt.Reglages, opt.Sauvegardes = a.reg, a.dossierSauvegardes(st)
	o := &universOuvert{st: st, srv: Nouveau(st, opt)}
	a.mu.Lock()
	a.tous[st.ID()] = o
	courant := !a.multi() || a.st == nil
	if courant {
		a.st, a.srv = o.st, o.srv
	}
	a.mu.Unlock()
	if courant {
		_ = a.reg.SetMeta(regUnivers, st.Chemin())
		_ = a.reg.SetMeta(regSansUnivers, "")
		a.noterRecent(st.Chemin())
		a.noterFichierCopie(st)
	}
	if a.multi() {
		a.noterServis()
	}
	if _, err := sauvegardeAuto(st, opt.Sauvegardes); err != nil {
		log.Printf("sauvegarde automatique : %v", err)
	}
	return o
}

// fermerCourant sauvegarde l'univers courant s'il a changé, puis le ferme (a.gestion tenu). Faux : il
// sert encore des requêtes, il reste ouvert.
func (a *Application) fermerCourant() bool {
	if a.st == nil {
		return true
	}
	id := a.st.ID()
	if !a.retirer(a.tous[id]) {
		return false
	}
	a.fermetureFinie(id)
	return true
}

// Fermer sauvegarde et ferme les univers ouverts, à l'arrêt de l'application : les requêtes en cours
// ont un court délai pour finir (une écriture interrompue est annulée par SQLite), une opération
// sur un univers renonce à attendre les siennes.
func (a *Application) Fermer() {
	a.mu.Lock()
	select {
	case <-a.fin:
	default:
		close(a.fin)
	}
	a.mu.Unlock()
	a.gestion.Lock()
	defer a.gestion.Unlock()
	a.mu.Lock()
	ouverts := a.tous
	a.tous, a.st, a.srv = map[string]*universOuvert{}, nil, nil
	a.mu.Unlock()
	delai := a.delaiFermeture
	if a.multi() {
		delai = min(delai, delaiArret)
	}
	limite := time.Now().Add(delai)
	for _, o := range ouverts {
		if !o.requetes.attendre(time.Until(limite), nil) {
			log.Printf("arrêt : univers « %s » fermé malgré des requêtes en cours", nomUnivers(o.st.Chemin()))
		}
		if _, err := sauvegardeAuto(o.st, a.dossierSauvegardes(o.st)); err != nil {
			log.Printf("sauvegarde automatique : %v", err)
		}
		a.clore(o)
	}
}

// veiller fait les sauvegardes automatiques tant que l'application tourne.
func (a *Application) veiller() {
	t := time.NewTicker(intervalleAuto)
	defer t.Stop()
	for {
		select {
		case <-a.fin:
			return
		case <-t.C:
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
				if !servi {
					continue
				}
				if _, err := sauvegardeAuto(o.st, a.dossierSauvegardes(o.st)); err != nil {
					log.Printf("sauvegarde automatique : %v", err)
				}
				o.requetes.sortir()
			}
		}
	}
}

func (a *Application) recents() []string {
	var l []string
	if v, ok := a.reg.Meta(regRecents); ok {
		_ = json.Unmarshal([]byte(v), &l)
	}
	return l
}

func (a *Application) noterRecent(chemin string) {
	l := []string{chemin}
	for _, c := range a.recents() {
		if c != chemin && len(l) < recentsMax {
			l = append(l, c)
		}
	}
	b, _ := json.Marshal(l)
	_ = a.reg.SetMeta(regRecents, string(b))
}

// ---------------------------------------------------------------- HTTP

func (a *Application) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api/univers" || strings.HasPrefix(r.URL.Path, "/api/univers/") {
		a.servirUnivers(w, r)
		return
	}
	if r.URL.Path == "/api/admin" || strings.HasPrefix(r.URL.Path, "/api/admin/") {
		a.servirAdmin(w, r)
		return
	}
	if websocket.IsWebSocketUpgrade(r) {
		a.servirSuivi(w, r)
		return
	}
	a.mu.RLock()
	if a.srv == nil {
		sans := !a.multi() && len(a.fermetures) == 0
		a.mu.RUnlock()
		if sans && local(r) {
			a.servirSansUnivers(w, r)
			return
		}
		erreur(w, 503, "Aucun univers n'est ouvert.")
		return
	}
	if a.multi() && (r.URL.Path == "/api/mes-univers" || strings.HasPrefix(r.URL.Path, "/api/mes-univers/")) {
		a.mu.RUnlock()
		a.mesUnivers(w, r)
		return
	}
	// le verrou ne sert qu'à choisir l'univers : la requête, compte tenu, se déroule sans lui
	o, code, msg := a.tous[a.st.ID()], 0, ""
	if a.multi() {
		o, code, msg = a.choisir(r)
	}
	if code == 0 {
		o.requetes.entrer()
	}
	a.mu.RUnlock()
	if code != 0 {
		enTetes(w)
		erreur(w, code, msg)
		return
	}
	defer o.requetes.sortir()
	o.srv.ServeHTTP(w, r)
}

// servirSuivi : un écran en direct reste branché longtemps. Il n'entre pas dans le compte des requêtes
// de l'univers : fermer ou changer d'univers l'attendrait sans fin. La fermeture d'un univers coupe
// ses écrans (Serveur.Fermer).
func (a *Application) servirSuivi(w http.ResponseWriter, r *http.Request) {
	a.mu.RLock()
	srv, code, msg := a.srv, 0, ""
	if srv != nil && a.multi() {
		var o *universOuvert
		if o, code, msg = a.choisir(r); code == 0 {
			srv = o.srv
		}
	}
	a.mu.RUnlock()
	switch {
	case srv == nil:
		erreur(w, 503, "Aucun univers n'est ouvert.")
	case code != 0:
		enTetes(w)
		erreur(w, code, msg)
	default:
		srv.ServeHTTP(w, r)
	}
}

// servirUnivers : le menu Fichier touche aux fichiers de l'ordinateur du MJ, il ne répond donc qu'à
// lui, jamais à un appareil du réseau, même connecté ou porteur du jeton.
func (a *Application) servirUnivers(w http.ResponseWriter, r *http.Request) {
	enTetes(w)
	if a.opt.acces != nil { // mode serveur : voir permisSurServeur
		r, ok := a.opt.acces.entrer(w, r)
		if !ok {
			return
		}
		c, connu := compteDe(r)
		if !connu {
			erreur(w, 401, "Connectez-vous avec votre compte, ou utilisez un jeton d'API.")
			return
		}
		if code, msg := a.permisSurServeur(c, r); code != 0 {
			erreur(w, code, msg)
			return
		}
		a.mux.ServeHTTP(w, r)
		return
	}
	if !local(r) {
		erreur(w, 403, "Les univers se gèrent sur l'ordinateur du MJ.")
		return
	}
	if j := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "); j != r.Header.Get("Authorization") {
		attendu, _ := a.reg.Meta("api_jeton")
		if attendu == "" || subtle.ConstantTimeCompare([]byte(j), []byte(attendu)) != 1 {
			erreur(w, 401, "Jeton d'API invalide.")
			return
		}
	} else if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Header.Get("X-COF-DM") != "1" {
		erreur(w, 403, "Requête refusée.")
		return
	}
	a.mux.ServeHTTP(w, r)
}

// permisSurServeur : sur un serveur, /api/univers ne crée ni n'ouvre plus rien (l'administration
// s'en charge, admin.go). Il reste au MJ de l'univers visé l'état de son univers, ses sauvegardes
// et la reprise d'une bibliothèque ; à l'administrateur, la version du serveur.
func (a *Application) permisSurServeur(c comptes.Compte, r *http.Request) (int, string) {
	chemin := r.URL.Path
	switch {
	case c.Admin:
		if chemin == "/api/univers/version" && !parJeton(r) {
			return 0, ""
		}
		return 403, "L'administrateur gère les univers depuis l'administration, sans y entrer."
	case chemin == "/api/univers/deposer":
		return 410, "Ce serveur ne reçoit plus d'univers déposé : l'administrateur y crée l'univers, puis son MJ y importe ses données. Mettez à jour l'application de bureau."
	case (chemin == "/api/univers" && r.Method == http.MethodGet) || chemin == "/api/univers/restaurer" || chemin == "/api/univers/bibliotheque":
		a.mu.RLock()
		o, code, msg := a.choisir(r)
		a.mu.RUnlock()
		if code != 0 {
			return code, msg
		}
		if c.Role(o.st.ID()) != comptes.MJ {
			return 403, "Réservé au MJ de cet univers."
		}
		return 0, ""
	}
	return 403, "Sur un serveur, les univers se créent et se renomment dans l'administration."
}

type ficheUnivers struct {
	Nom     string `json:"nom"`
	Chemin  string `json:"chemin"`
	ID      string `json:"id,omitempty"`
	Taille  int64  `json:"taille,omitempty"`
	Present bool   `json:"present"`
}

func decrire(chemin string) ficheUnivers {
	f := ficheUnivers{Nom: nomUnivers(chemin), Chemin: chemin}
	if info, err := os.Stat(chemin); err == nil && !info.IsDir() {
		f.Present, f.Taille = true, info.Size()
	}
	return f
}

func (a *Application) etat(w http.ResponseWriter, r *http.Request) {
	a.mu.RLock()
	o := a.vise(r)
	avertissement := a.avertissement
	var accessibles []*universOuvert
	if a.multi() {
		c, _ := compteDe(r)
		accessibles = a.accessibles(c)
	}
	a.mu.RUnlock()
	if a.multi() {
		a.etatSurServeur(w, r, o, accessibles)
		return
	}
	out := map[string]any{"dossier": a.DossierUnivers(), "parcourir": a.opt.Dialogue != nil, "avertissement": avertissement, "serveur": a.multi()}
	courant := ""
	if o != nil {
		courant = o.st.Chemin()
		c := decrire(courant)
		c.ID = o.st.ID()
		out["courant"] = c
		out["dossier_sauvegardes"] = a.dossierSauvegardes(o.st)
		out["sauvegardes"] = listerSauvegardes(a.dossierSauvegardes(o.st))
	}
	recents := []ficheUnivers{}
	vus := map[string]bool{courant: true}
	for _, c := range a.recents() {
		if !vus[c] && !a.multi() { // un serveur n'ouvre que les univers de son dossier
			vus[c] = true
			recents = append(recents, decrire(c))
		}
	}
	out["recents"] = recents
	// les univers du dossier par défaut qui ne sont pas déjà dans la liste
	autres := []ficheUnivers{}
	entrees, _ := os.ReadDir(a.DossierUnivers())
	for _, e := range entrees {
		c := filepath.Join(a.DossierUnivers(), e.Name())
		if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), store.Extension) && !vus[c] {
			autres = append(autres, decrire(c))
		}
	}
	sort.Slice(autres, func(i, j int) bool { return strings.ToLower(autres[i].Nom) < strings.ToLower(autres[j].Nom) })
	out["autres"] = autres
	repondre(w, 200, out)
}

// etatSurServeur : l'univers visé et ses sauvegardes, pour son MJ, sans chemin du serveur ; les
// autres univers du compte (d'où reprendre une bibliothèque). Rien n'y lit la base : ni verrou ni compte.
func (a *Application) etatSurServeur(w http.ResponseWriter, r *http.Request, o *universOuvert, accessibles []*universOuvert) {
	c, _ := compteDe(r)
	if o == nil {
		erreur(w, 503, msgFermeture)
		return
	}
	courant := decrire(o.st.Chemin())
	courant.Chemin, courant.ID = "", o.st.ID()
	autres := []ficheUnivers{}
	for _, x := range accessibles {
		if role := c.Role(x.st.ID()); x != o && (role == comptes.MJ || role == comptes.Auditeur) {
			f := decrire(x.st.Chemin())
			f.Chemin, f.ID = "", x.st.ID()
			autres = append(autres, f)
		}
	}
	repondre(w, 200, map[string]any{"serveur": true, "parcourir": false, "courant": courant, "recents": []ficheUnivers{},
		"autres": autres, "sauvegardes": listerSauvegardes(a.dossierSauvegardes(o.st))})
}

func (a *Application) routeOuvrir(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Chemin string `json:"chemin"`
	}
	if err := lireJSON(r, &in); err != nil {
		erreurStore(w, err)
		return
	}
	if a.dansArchives(in.Chemin) {
		erreur(w, 422, msgDansArchives)
		return
	}
	if err := a.Ouvrir(in.Chemin, false); err != nil {
		erreurStore(w, err)
		return
	}
	a.choisi()
	a.etat(w, r)
}

// avecExtension ajoute .cofdm à un nom de fichier qui ne l'a pas.
func avecExtension(chemin string) string {
	if !strings.EqualFold(filepath.Ext(chemin), store.Extension) {
		chemin += store.Extension
	}
	return chemin
}

// nomDeFichier retire d'un nom d'univers ce qu'un nom de fichier ne peut pas contenir.
func nomDeFichier(nom string) string {
	nom = strings.Map(func(r rune) rune {
		if r < 32 || strings.ContainsRune(`<>:"/\|?*`, r) {
			return -1
		}
		return r
	}, nom)
	nom = strings.Trim(strings.TrimSpace(nom), ".")
	if rs := []rune(nom); len(rs) > 80 {
		nom = strings.TrimSpace(string(rs[:80]))
	}
	return nom
}

// routeNouveau : {"nom": "…"} crée l'univers dans le dossier par défaut ; {"chemin": "…"} là où le
// MJ l'a choisi. Un fichier existant n'est jamais remplacé.
func (a *Application) routeNouveau(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Nom    string `json:"nom"`
		Chemin string `json:"chemin"`
	}
	if err := lireJSON(r, &in); err != nil {
		erreurStore(w, err)
		return
	}
	chemin := strings.TrimSpace(in.Chemin)
	if chemin == "" {
		nom := nomDeFichier(in.Nom)
		if nom == "" {
			erreur(w, 422, "Donnez un nom à l'univers.")
			return
		}
		chemin = filepath.Join(a.DossierUnivers(), nom)
	}
	chemin = avecExtension(chemin)
	if a.dansArchives(chemin) {
		erreur(w, 422, msgDansArchives)
		return
	}
	if _, err := os.Stat(chemin); err == nil {
		erreur(w, 409, "Un fichier porte déjà ce nom : "+chemin)
		return
	}
	o, err := a.ouvrir(chemin, true)
	if err != nil {
		erreurStore(w, err)
		return
	}
	a.choisi()
	o.st.Noter("univers", "univers créé", auteurDe(r))
	a.etat(w, r)
}

// tailleMaxUnivers : un univers porte ses images ; au-delà, mieux vaut copier le fichier à la main.
const tailleMaxUnivers = 1 << 30

// enTeteSQLite : le fichier commence comme une base SQLite (un fichier vide ou d'un autre format
// deviendrait sinon un univers vide).
func enTeteSQLite(chemin string) bool {
	f, err := os.Open(chemin)
	if err != nil {
		return false
	}
	defer f.Close()
	b := make([]byte, 16)
	_, err = io.ReadFull(f, b)
	return err == nil && string(b) == "SQLite format 3\x00"
}

// routeCopie : {"chemin": "…"} enregistre une copie complète de l'univers ouvert, qui reste ouvert.
// "remplacer": true écrase un fichier existant.
func (a *Application) routeCopie(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Chemin    string `json:"chemin"`
		Remplacer bool   `json:"remplacer"`
	}
	if err := lireJSON(r, &in); err != nil {
		erreurStore(w, err)
		return
	}
	if strings.TrimSpace(in.Chemin) == "" {
		erreur(w, 422, "Indiquez où enregistrer la copie.")
		return
	}
	chemin, err := filepath.Abs(avecExtension(strings.TrimSpace(in.Chemin)))
	if err != nil {
		erreur(w, 422, "Chemin invalide.")
		return
	}
	a.mu.RLock()
	o := a.vise(r)
	if o == nil {
		a.mu.RUnlock()
		erreur(w, 503, "Aucun univers n'est ouvert.")
		return
	}
	if a.parChemin(chemin) != nil {
		a.mu.RUnlock()
		erreur(w, 422, "C'est le fichier de l'univers ouvert : choisissez un autre nom.")
		return
	}
	o.requetes.entrer()
	a.mu.RUnlock()
	defer o.requetes.sortir()
	if _, err := os.Stat(chemin); err == nil && !in.Remplacer {
		erreur(w, 409, "Un fichier porte déjà ce nom : "+chemin)
		return
	}
	tmp := chemin + ".copie"
	_ = os.Remove(tmp)
	if err := o.st.Sauvegarder(tmp); err != nil {
		_ = os.Remove(tmp)
		erreur(w, 500, "Copie impossible : "+err.Error())
		return
	}
	if err := os.Rename(tmp, chemin); err != nil {
		_ = os.Remove(tmp)
		erreur(w, 500, "Copie impossible : "+err.Error())
		return
	}
	o.st.Noter("univers", "copie enregistrée dans "+chemin, auteurDe(r))
	repondre(w, 200, map[string]any{"chemin": chemin})
}

// routeParcourir ouvre la boîte de dialogue du système : {"enregistrer": false, "nom": "…"}.
// Répond {"chemin": ""} si le MJ annule.
func (a *Application) routeParcourir(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Enregistrer bool   `json:"enregistrer"`
		Nom         string `json:"nom"`
	}
	if err := lireJSON(r, &in); err != nil {
		erreurStore(w, err)
		return
	}
	if a.opt.Dialogue == nil {
		erreur(w, 501, "Aucune boîte de dialogue sur ce système : saisissez le chemin du fichier.")
		return
	}
	dossier := a.DossierUnivers()
	a.mu.RLock()
	if o := a.vise(r); o != nil {
		dossier = filepath.Dir(o.st.Chemin())
	}
	a.mu.RUnlock()
	_ = os.MkdirAll(dossier, 0o700)
	chemin, err := a.opt.Dialogue(in.Enregistrer, nomDeFichier(in.Nom), dossier)
	if err != nil {
		erreur(w, 501, "Boîte de dialogue indisponible : saisissez le chemin du fichier.")
		return
	}
	if chemin != "" && in.Enregistrer {
		chemin = avecExtension(chemin)
	}
	repondre(w, 200, map[string]any{"chemin": chemin})
}

// routeRestaurer : {"fichier": "<date>_<motif>.cofdm"} remet l'univers ouvert dans l'état d'une de
// ses sauvegardes. L'état actuel est sauvegardé avant.
func (a *Application) routeRestaurer(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Fichier string `json:"fichier"`
	}
	if err := lireJSON(r, &in); err != nil {
		erreurStore(w, err)
		return
	}
	if !reSauvegarde.MatchString(in.Fichier) {
		erreur(w, 422, "Sauvegarde inconnue.")
		return
	}
	a.gestion.Lock()
	defer a.gestion.Unlock()
	o := a.vise(r)
	if o == nil {
		erreur(w, 503, "Aucun univers n'est ouvert.")
		return
	}
	chemin, dossier := o.st.Chemin(), a.dossierSauvegardes(o.st)
	// la sauvegarde est d'abord copiée et vérifiée à côté de l'univers, qui n'est remplacé qu'ensuite
	tmp := chemin + ".restauration"
	if err := copierFichier(filepath.Join(dossier, in.Fichier), tmp); err != nil {
		_ = os.Remove(tmp)
		log.Printf("restauration : %v", err)
		erreur(w, 404, "Sauvegarde illisible.")
		return
	}
	essai, err := store.Ouvrir(tmp)
	if err != nil {
		_ = os.Remove(tmp)
		erreur(w, 422, "Cette sauvegarde est abîmée : rien n'a été modifié.")
		return
	}
	essai.Noter("restauration", "univers remis dans l'état de la sauvegarde "+in.Fichier, auteurDe(r))
	_ = essai.Fermer()
	if !a.suspendre(o) {
		_ = os.Remove(tmp)
		erreur(w, 409, msgOccupe)
		return
	}
	defer a.fermetureFinie(o.st.ID())
	// quand plus rien n'écrit : la sauvegarde d'avant a tout ce que l'univers contenait
	avant, err := sauvegarder(o.st, dossier, "avant-restauration")
	if err != nil {
		a.remettre(o)
		_ = os.Remove(tmp)
		erreur(w, 500, auJournal("Sauvegarde impossible, rien n'a été modifié", err))
		return
	}
	if !a.opt.Limites.ranger(dossier, filepath.Base(avant), false) {
		log.Printf("sauvegardes de l'univers « %s » : quota dépassé", nomUnivers(chemin))
	}
	a.clore(o)
	echec := os.Rename(tmp, chemin)
	if echec != nil {
		_ = os.Remove(tmp) // l'univers n'a pas été touché : il est rouvert tel quel
	}
	st, err := store.Ouvrir(chemin)
	if err != nil {
		log.Printf("restauration, état précédent dans %s", avant)
		erreur(w, 500, auJournal("L'univers ne s'ouvre plus", err))
		return
	}
	if rouvert := a.installer(st); o.defaut {
		a.redevenirDefaut(rouvert)
	}
	if echec != nil {
		erreur(w, 500, auJournal("Restauration impossible, rien n'a été modifié", echec))
		return
	}
	repondre(w, 200, map[string]any{"restaure": in.Fichier, "sauvegarde": filepath.Base(avant)})
}

// bibliothequeSurServeur : la reprise depuis un autre univers du serveur, que le compte lit (MJ
// ou auditeur).
func (a *Application) bibliothequeSurServeur(w http.ResponseWriter, r *http.Request, id string) {
	c, _ := compteDe(r)
	a.mu.RLock()
	cible, source := a.vise(r), a.tous[id]
	if role := c.Role(id); source == nil || (role != comptes.MJ && role != comptes.Auditeur) {
		a.mu.RUnlock()
		erreur(w, 404, "Univers inconnu, ou que votre compte ne lit pas.")
		return
	}
	if cible == nil {
		a.mu.RUnlock()
		erreur(w, 503, msgFermeture)
		return
	}
	if source == cible {
		a.mu.RUnlock()
		erreur(w, 422, "C'est l'univers ouvert : choisissez-en un autre.")
		return
	}
	cible.requetes.entrer()
	source.requetes.entrer()
	a.mu.RUnlock()
	defer cible.requetes.sortir()
	defer source.requetes.sortir()
	b, err := source.srv.bibliotheque()
	if err != nil {
		erreurStore(w, err)
		return
	}
	out, err := cible.srv.recevoirBibliotheque(b, auteurDe(r))
	if err != nil {
		erreurStore(w, err)
		return
	}
	repondre(w, 200, out)
}

func copierFichier(source, cible string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(cible, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// routeBibliotheque : {"source": "<fichier d'univers>"} reprend dans l'univers visé le bestiaire et
// les règles (profils, peuples, voies) d'un autre univers. Rien n'est retiré, ni ici ni là-bas.
func (a *Application) routeBibliotheque(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Source  string `json:"source"`
		Univers string `json:"univers"` // sur un serveur : l'identifiant de l'autre univers
	}
	if err := lireJSON(r, &in); err != nil {
		erreurStore(w, err)
		return
	}
	if a.multi() {
		a.bibliothequeSurServeur(w, r, in.Univers)
		return
	}
	chemin, err := filepath.Abs(strings.TrimSpace(in.Source))
	if err != nil || strings.TrimSpace(in.Source) == "" {
		erreur(w, 422, "Indiquez l'univers dont reprendre le bestiaire et les règles.")
		return
	}
	a.mu.RLock()
	cible := a.vise(r)
	if cible == nil {
		a.mu.RUnlock()
		erreur(w, 503, "Aucun univers n'est ouvert.")
		return
	}
	if memeFichier(cible.st.Chemin(), chemin) {
		a.mu.RUnlock()
		erreur(w, 422, "C'est l'univers ouvert : choisissez-en un autre.")
		return
	}
	autre := a.parChemin(chemin)
	cible.requetes.entrer()
	if autre != nil {
		autre.requetes.entrer()
		defer autre.requetes.sortir()
	}
	a.mu.RUnlock()
	defer cible.requetes.sortir()
	var b Bibliotheque
	switch o := autre; {
	case o != nil:
		b, err = o.srv.bibliotheque()
	default: // sur l'ordinateur du MJ : l'autre univers est lu, puis refermé
		if info, e := os.Stat(chemin); e != nil || info.IsDir() {
			erreur(w, 422, "Fichier introuvable : "+chemin)
			return
		}
		st, e := store.Ouvrir(chemin)
		if e != nil {
			erreur(w, 422, "Ce fichier n'est pas un univers COF DM.")
			return
		}
		b, err = bibliothequeDe(st)
		_ = st.Fermer()
	}
	if err != nil {
		erreurStore(w, err)
		return
	}
	out, err := cible.srv.recevoirBibliotheque(b, auteurDe(r))
	if err != nil {
		erreurStore(w, err)
		return
	}
	repondre(w, 200, out)
}
