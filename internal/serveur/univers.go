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

	mu            sync.RWMutex // en écriture : le temps d'ouvrir, de fermer ou de remplacer un univers
	st            *store.Store // l'univers courant ; sur un serveur, celui qui sert par défaut
	srv           *Serveur
	tous          map[string]*universOuvert // les univers ouverts, par identifiant : un seul, sauf sur un serveur
	avertissement string                    // à montrer au MJ (univers introuvable au lancement…)
	dist          *distant                  // application de bureau : le lien avec un serveur
	maj           *misesAJour               // application de bureau : les mises à jour
}

// universOuvert : un univers ouvert et le serveur de son API.
type universOuvert struct {
	st  *store.Store
	srv *Serveur
}

const (
	regUnivers   = "univers"
	regRecents   = "recents"
	recentsMax   = 8
	nomParDefaut = "Mon univers"
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
		opt.acces = nouvelAcces(opt.Reglages, opt.Comptes)
	}
	a := &Application{opt: opt, reg: opt.Reglages, mux: http.NewServeMux(), fin: make(chan struct{}), tous: map[string]*universOuvert{}}
	if opt.Comptes == nil {
		a.opt.Relie = a.Relie
	}
	m := a.mux
	m.HandleFunc("GET /api/univers", a.etat)
	m.HandleFunc("POST /api/univers/ouvrir", a.routeOuvrir)
	m.HandleFunc("POST /api/univers/nouveau", a.routeNouveau)
	m.HandleFunc("POST /api/univers/deposer", a.routeDeposer)
	m.HandleFunc("POST /api/univers/copie", a.routeCopie)
	m.HandleFunc("POST /api/univers/parcourir", a.routeParcourir)
	m.HandleFunc("POST /api/univers/restaurer", a.routeRestaurer)
	m.HandleFunc("POST /api/univers/bibliotheque", a.routeBibliotheque)
	a.routesDistant()
	if !a.multi() {
		a.routesMaj()
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
	} else {
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
	a.mu.Lock()
	defer a.mu.Unlock()
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
		a.fermerCourant()
	} else if deja, pris := a.tous[st.ID()]; pris { // la copie d'un univers porte son identifiant
		_ = st.Fermer()
		return nil, store.ErrValidation{Msg: "Cet univers est déjà ouvert sous le nom « " + nomUnivers(deja.st.Chemin()) + " » : ce fichier en est une copie."}
	}
	return a.installer(st), nil
}

// parChemin : l'univers ouvert qui tient dans ce fichier (verrou tenu).
func (a *Application) parChemin(chemin string) *universOuvert {
	for _, o := range a.tous {
		if memeFichier(o.st.Chemin(), chemin) {
			return o
		}
	}
	return nil
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

// installer fait de st l'univers courant (verrou d'écriture tenu).
func (a *Application) installer(st *store.Store) *universOuvert {
	opt := a.opt
	opt.Reglages, opt.Sauvegardes = a.reg, a.dossierSauvegardes(st)
	o := &universOuvert{st, Nouveau(st, opt)}
	a.tous[st.ID()] = o
	if !a.multi() || a.st == nil {
		a.st, a.srv = o.st, o.srv
		_ = a.reg.SetMeta(regUnivers, st.Chemin())
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

// fermerCourant sauvegarde l'univers courant s'il a changé, puis le ferme (verrou d'écriture tenu).
func (a *Application) fermerCourant() {
	if a.st == nil {
		return
	}
	if _, err := sauvegardeAuto(a.st, a.dossierSauvegardes(a.st)); err != nil {
		log.Printf("sauvegarde automatique : %v", err)
	}
	if a.srv != nil {
		a.srv.Fermer()
	}
	_ = a.st.Fermer()
	delete(a.tous, a.st.ID())
	a.st, a.srv = nil, nil
}

// Fermer sauvegarde et ferme l'univers ouvert, à l'arrêt de l'application.
func (a *Application) Fermer() {
	a.mu.Lock()
	defer a.mu.Unlock()
	select {
	case <-a.fin:
	default:
		close(a.fin)
	}
	a.fermerCourant()
	for id, o := range a.tous { // sur un serveur : les autres univers
		if _, err := sauvegardeAuto(o.st, a.dossierSauvegardes(o.st)); err != nil {
			log.Printf("sauvegarde automatique : %v", err)
		}
		if o.srv != nil {
			o.srv.Fermer()
		}
		_ = o.st.Fermer()
		delete(a.tous, id)
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
			for _, o := range a.tous {
				if _, err := sauvegardeAuto(o.st, a.dossierSauvegardes(o.st)); err != nil {
					log.Printf("sauvegarde automatique : %v", err)
				}
			}
			a.mu.RUnlock()
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
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.srv == nil {
		erreur(w, 503, "Aucun univers n'est ouvert.")
		return
	}
	if a.multi() {
		a.servirMulti(w, r)
		return
	}
	a.srv.ServeHTTP(w, r)
}

// servirUnivers : le menu Fichier touche aux fichiers de l'ordinateur du MJ, il ne répond donc qu'à
// lui, jamais à un appareil du réseau, même connecté ou porteur du jeton.
func (a *Application) servirUnivers(w http.ResponseWriter, r *http.Request) {
	enTetes(w)
	if a.opt.acces != nil { // mode serveur : les fichiers du serveur regardent l'administrateur
		r, ok := a.opt.acces.entrer(w, r)
		if !ok {
			return
		}
		if c, connu := compteDe(r); !connu || !c.Admin {
			erreur(w, 403, "Les univers se gèrent avec un compte administrateur.")
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
	defer a.mu.RUnlock()
	out := map[string]any{"dossier": a.DossierUnivers(), "parcourir": a.opt.Dialogue != nil, "avertissement": a.avertissement, "serveur": a.multi()}
	courant := ""
	if o := a.vise(r); o != nil {
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

func (a *Application) routeOuvrir(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Chemin string `json:"chemin"`
	}
	if err := lireJSON(r, &in); err != nil {
		erreurStore(w, err)
		return
	}
	if a.multi() {
		a.ouvrirSurServeur(w, r, in.Chemin)
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
	if chemin != "" && a.multi() {
		erreur(w, 422, "Sur un serveur, un univers se crée par son nom, dans le dossier du serveur.")
		return
	}
	if chemin == "" {
		nom := nomDeFichier(in.Nom)
		if nom == "" {
			erreur(w, 422, "Donnez un nom à l'univers.")
			return
		}
		chemin = filepath.Join(a.DossierUnivers(), nom)
	}
	chemin = avecExtension(chemin)
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
	if a.multi() { // l'administrateur passe dans l'univers qu'il vient de créer
		r = a.viser(w, r, o)
	}
	a.etat(w, r)
}

// tailleMaxUnivers : un univers porte ses images ; au-delà, mieux vaut copier le fichier à la main.
const tailleMaxUnivers = 1 << 30

// routeDeposer : sur un serveur, l'administrateur y ajoute un univers existant (le fichier .cofdm,
// brut, dans le corps ; ?nom= pour le nommer). Le fichier est écrit à part et ouvert pour contrôle
// (univers reconnu, intact, sans objet étranger, remis au format du modèle) avant d'entrer dans le
// dossier des univers ; la copie d'un univers déjà présent est refusée.
func (a *Application) routeDeposer(w http.ResponseWriter, r *http.Request) {
	if !a.multi() {
		erreur(w, 422, "Sur cet ordinateur, ouvrez le fichier directement (menu Fichier, « Ouvrir un univers »).")
		return
	}
	nom := nomDeFichier(strings.TrimSuffix(r.URL.Query().Get("nom"), store.Extension))
	if nom == "" {
		erreur(w, 422, "Donnez un nom à l'univers.")
		return
	}
	chemin := avecExtension(filepath.Join(a.DossierUnivers(), nom))
	if _, err := os.Stat(chemin); err == nil {
		erreur(w, 409, "Un univers porte déjà ce nom sur le serveur : choisissez-en un autre.")
		return
	}
	tmp, err := os.CreateTemp(a.DossierUnivers(), ".depot-*.tmp")
	if err != nil {
		erreurStore(w, err)
		return
	}
	defer os.Remove(tmp.Name()) // sans effet une fois le fichier rangé
	_ = tmp.Chmod(0o600)
	n, err := io.Copy(tmp, http.MaxBytesReader(w, r.Body, tailleMaxUnivers))
	if errFermer := tmp.Close(); err == nil {
		err = errFermer
	}
	var tropGros *http.MaxBytesError
	switch {
	case errors.As(err, &tropGros):
		erreur(w, 413, "Fichier trop lourd (1 Go au plus).")
		return
	case err != nil:
		erreurStore(w, err)
		return
	case !enTeteSQLite(tmp.Name()):
		erreur(w, 422, "Ce fichier n'est pas un univers COF DM.")
		return
	}
	essai, err := store.Ouvrir(tmp.Name())
	if errors.Is(err, store.ErrPasUnivers) {
		erreur(w, 422, "Ce fichier n'est pas un univers COF DM.")
		return
	}
	if err != nil {
		erreurStore(w, err)
		return
	}
	id := essai.ID()
	_ = essai.Fermer()
	a.mu.RLock()
	deja, pris := a.tous[id]
	a.mu.RUnlock()
	if pris {
		erreur(w, 409, "Cet univers est déjà sur le serveur, sous le nom « "+nomUnivers(deja.st.Chemin())+" » : ce fichier en est une copie. Pour remettre l'univers du serveur dans l'état de ce fichier, ouvrez-le sur votre ordinateur, exportez-le en JSON (Paramètres) et restaurez cet export sur le serveur.")
		return
	}
	if err := os.Link(tmp.Name(), chemin); err != nil { // ne remplace jamais un fichier arrivé entre-temps
		if errors.Is(err, os.ErrExist) {
			erreur(w, 409, "Un univers porte déjà ce nom sur le serveur : choisissez-en un autre.")
			return
		}
		erreurStore(w, err)
		return
	}
	o, err := a.ouvrir(chemin, false)
	if err != nil {
		_ = os.Remove(chemin)
		erreurStore(w, err)
		return
	}
	o.st.Noter("univers", fmt.Sprintf("univers déposé sur le serveur (%d octets)", n), auteurDe(r))
	a.etat(w, a.viser(w, r, o))
}

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
	defer a.mu.RUnlock()
	o := a.vise(r)
	if o == nil {
		erreur(w, 503, "Aucun univers n'est ouvert.")
		return
	}
	if a.parChemin(chemin) != nil {
		erreur(w, 422, "C'est le fichier de l'univers ouvert : choisissez un autre nom.")
		return
	}
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
	a.mu.Lock()
	defer a.mu.Unlock()
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
		erreur(w, 404, "Sauvegarde illisible : "+err.Error())
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
	avant, err := sauvegarder(o.st, dossier, "avant-restauration")
	if err != nil {
		_ = os.Remove(tmp)
		erreur(w, 500, "Sauvegarde impossible, rien n'a été modifié : "+err.Error())
		return
	}
	if o.srv != nil {
		o.srv.Fermer()
	}
	_ = o.st.Fermer()
	delete(a.tous, o.st.ID())
	if a.st == o.st {
		a.st, a.srv = nil, nil
	}
	echec := os.Rename(tmp, chemin)
	if echec != nil {
		_ = os.Remove(tmp) // l'univers n'a pas été touché : il est rouvert tel quel
	}
	st, err := store.Ouvrir(chemin)
	if err != nil {
		erreur(w, 500, "L'univers ne s'ouvre plus : "+err.Error()+" (état précédent : "+avant+")")
		return
	}
	a.installer(st)
	if echec != nil {
		erreur(w, 500, "Restauration impossible, rien n'a été modifié : "+echec.Error())
		return
	}
	repondre(w, 200, map[string]any{"restaure": in.Fichier, "sauvegarde": filepath.Base(avant)})
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
		Source string `json:"source"`
	}
	if err := lireJSON(r, &in); err != nil {
		erreurStore(w, err)
		return
	}
	chemin, err := filepath.Abs(strings.TrimSpace(in.Source))
	if err != nil || strings.TrimSpace(in.Source) == "" {
		erreur(w, 422, "Indiquez l'univers dont reprendre le bestiaire et les règles.")
		return
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	cible := a.vise(r)
	if cible == nil {
		erreur(w, 503, "Aucun univers n'est ouvert.")
		return
	}
	if memeFichier(cible.st.Chemin(), chemin) {
		erreur(w, 422, "C'est l'univers ouvert : choisissez-en un autre.")
		return
	}
	var b Bibliotheque
	switch o := a.parChemin(chemin); {
	case o != nil:
		b, err = o.srv.bibliotheque()
	case a.multi():
		erreur(w, 422, "Sur un serveur, la reprise se fait depuis un univers du serveur.")
		return
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
