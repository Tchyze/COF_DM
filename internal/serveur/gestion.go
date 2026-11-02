package serveur

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/Tchyze/cof_dm/internal/store"
)

// La fenêtre « Mes univers » de l'application de bureau : /api/univers/gestion/… Ces routes touchent
// aux fichiers de l'ordinateur du MJ, sans jamais recevoir un chemin : elles désignent un univers
// par une clé, l'empreinte de son chemin, retrouvée à chaque requête parmi les univers que
// l'application connaît. Une clé qui n'y est pas ne mène à rien. Elles n'existent pas sur un serveur,
// où l'administrateur a sa page (admin.go).
//
// Verrous (fermeture.go) : a.gestion d'un bout à l'autre de l'opération, puis a.dist.mu, le temps de
// laisser finir un échange en cours avec le serveur, jamais l'inverse.

const (
	lieuDossier     = "dossier"     // dans le dossier des univers
	lieuAilleurs    = "ailleurs"    // connu par les récents ou un lien, rangé ailleurs
	lieuArchive     = "archive"     // dans le dossier des archives
	lieuIntrouvable = "introuvable" // connu, mais le fichier n'est plus là

	actionOuvrir   = "ouvrir"
	actionRenommer = "renommer"
	actionOublier  = "oublier"

	suffixeJournal = "-journal" // le journal SQLite d'une écriture interrompue, à ne pas séparer de son fichier
	msgInterrompu  = "Une écriture de cet univers a été interrompue : ouvrez-le une fois, puis réessayez."
)

// copieGeree : ce qui relie un univers de cet ordinateur à son serveur. Etat : actif (il se synchronise
// quand il est ouvert) ou pause (il attend son serveur).
type copieGeree struct {
	URL    string `json:"url"`
	Compte string `json:"compte,omitempty"`
	Copie  string `json:"copie,omitempty"`
	Nom    string `json:"nom,omitempty"` // son nom sur le serveur
	Etat   string `json:"etat"`
}

// ficheGeree : un univers tel que la fenêtre le montre. Gerable : un fichier d'univers ordinaire, sur
// lequel les actions sont possibles (ni lien symbolique, ni autre extension, ni autre chose qu'un univers).
// Interrompu : une écriture s'est arrêtée en route (un journal SQLite est à côté) ; l'ouvrir la répare.
type ficheGeree struct {
	Cle        string      `json:"cle"`
	Nom        string      `json:"nom"`
	Chemin     string      `json:"chemin"`
	Taille     int64       `json:"taille"`
	Modifie    float64     `json:"modifie"`
	Ouvert     bool        `json:"ouvert"`
	Lieu       string      `json:"lieu"`
	Gerable    bool        `json:"gerable"`
	Interrompu bool        `json:"interrompu,omitempty"`
	Copie      *copieGeree `json:"copie,omitempty"`
	Actions    []string    `json:"actions"`
	id         string
}

func (f ficheGeree) peut(action string) bool {
	for _, x := range f.Actions {
		if x == action {
			return true
		}
	}
	return false
}

func (a *Application) routesGestion() {
	m := a.mux
	m.HandleFunc("GET /api/univers/gestion", a.gardeGestion(a.routeGestion))
	m.HandleFunc("POST /api/univers/gestion/{cle}/renommer", a.gardeGestion(a.routeRenommerUnivers))
	m.HandleFunc("POST /api/univers/gestion/{cle}/oublier", a.gardeGestion(a.routeOublierUnivers))
}

// gardeGestion : l'ordinateur du MJ, par l'interface de l'application et rien d'autre. Plus strict que
// servirUnivers, qui a déjà vérifié l'origine : aucun jeton, même sur cet ordinateur, puisque Claude et
// les outils qui en ont un ne suppriment jamais rien définitivement.
func (a *Application) gardeGestion(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.multi() {
			erreur(w, 404, "Sur un serveur, les univers se gèrent dans l'administration.")
			return
		}
		if !local(r) {
			erreur(w, 403, "Les univers se gèrent sur l'ordinateur du MJ.")
			return
		}
		if r.Header.Get("Authorization") != "" {
			erreur(w, 403, "Les univers se gèrent dans l'application, pas avec un jeton.")
			return
		}
		if o := r.Header.Get("Origin"); o != "" {
			if u, err := url.Parse(o); err != nil || u.Host != r.Host {
				erreur(w, 403, "Requête refusée.")
				return
			}
		}
		if r.Method != http.MethodGet && r.Header.Get("X-COF-DM") != "1" {
			erreur(w, 403, "Requête refusée.")
			return
		}
		h(w, r)
	}
}

// cheminCle : un chemin sous sa forme de comparaison (Windows ne distingue pas les majuscules).
func cheminCle(chemin string) string {
	chemin = filepath.Clean(chemin)
	if runtime.GOOS == "windows" {
		chemin = strings.ToLower(chemin)
	}
	return chemin
}

func cleFichier(chemin string) string {
	s := sha256.Sum256([]byte(cheminCle(chemin)))
	return hex.EncodeToString(s[:12])
}

// dansDossier : le fichier est réellement dans ce dossier, quel que soit le chemin qui y mène.
func dansDossier(chemin, dossier string) bool {
	return memeFichier(filepath.Dir(chemin), dossier)
}

// entreesUnivers : les fichiers .cofdm d'un dossier, par nom.
func entreesUnivers(dossier string) []string {
	entrees, _ := os.ReadDir(dossier)
	var l []string
	for _, e := range entrees {
		if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), store.Extension) {
			l = append(l, filepath.Join(dossier, e.Name()))
		}
	}
	return l
}

// archivesSures : le dossier des archives, s'il en est bien un et pas un lien vers un autre dossier.
func (a *Application) archivesSures() (string, bool) {
	d := a.DossierArchives()
	info, err := os.Lstat(d)
	return d, err == nil && info.IsDir()
}

// fichesGerees : les univers que l'application connaît, l'ouvert d'abord, puis par nom : l'univers
// ouvert, le dossier des univers, les récents, les copies reliées, les archives. Rien n'y est écrit.
func (a *Application) fichesGerees() []ficheGeree {
	a.mu.RLock()
	ouvert := a.st
	a.mu.RUnlock()
	liens := a.liens()
	cx := a.cnx()
	vus := map[string]bool{}
	out := []ficheGeree{}
	ajouter := func(chemin string) {
		cle := cleFichier(chemin)
		if chemin == "" || vus[cle] {
			return
		}
		vus[cle] = true
		out = append(out, a.ficheGeree(chemin, ouvert, liens, cx))
	}
	if ouvert != nil {
		ajouter(ouvert.Chemin())
	}
	for _, c := range entreesUnivers(a.DossierUnivers()) {
		ajouter(c)
	}
	for _, c := range a.recents() {
		ajouter(c)
	}
	for _, l := range liens {
		ajouter(l.Chemin)
	}
	if d, ok := a.archivesSures(); ok {
		for _, c := range entreesUnivers(d) {
			ajouter(c)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Ouvert != out[j].Ouvert {
			return out[i].Ouvert
		}
		return strings.ToLower(out[i].Nom) < strings.ToLower(out[j].Nom)
	})
	return out
}

func (a *Application) ficheGeree(chemin string, ouvert *store.Store, liens map[string]lien, cx *connexion) ficheGeree {
	f := ficheGeree{Cle: cleFichier(chemin), Nom: nomUnivers(chemin), Chemin: chemin, Actions: []string{}, Lieu: lieuAilleurs}
	f.Ouvert = ouvert != nil && cheminCle(ouvert.Chemin()) == cheminCle(chemin)
	info, err := os.Lstat(chemin)
	if err != nil {
		f.Lieu = lieuIntrouvable
		f.Actions = append(f.Actions, actionOublier)
		f.Copie = copieDuChemin(chemin, liens, cx)
		return f
	}
	f.Taille, f.Modifie = info.Size(), float64(info.ModTime().Unix())
	if dansDossier(chemin, a.DossierUnivers()) {
		f.Lieu = lieuDossier
	} else if d, ok := a.archivesSures(); ok && dansDossier(chemin, d) {
		f.Lieu = lieuArchive
	}
	estCopie, lisible := false, true
	switch {
	case f.Ouvert:
		f.id, estCopie = ouvert.ID(), ouvert.EstCopie()
	case existe(chemin + suffixeJournal):
		f.Interrompu = true
	default:
		ap, err := store.Apercu(chemin)
		f.id, estCopie, lisible = ap.ID, ap.Copie, err == nil
	}
	f.Gerable = lisible && !f.Interrompu && info.Mode().IsRegular() && strings.EqualFold(filepath.Ext(chemin), store.Extension)
	if estCopie {
		if l, ok := liens[f.id]; ok {
			f.Copie = vueCopie(l, cx)
		}
	}
	if f.Copie == nil {
		f.Copie = copieDuChemin(chemin, liens, cx)
	}
	if !f.Ouvert && f.Lieu != lieuArchive && lisible {
		f.Actions = append(f.Actions, actionOuvrir)
	}
	if f.Gerable {
		f.Actions = append(f.Actions, actionRenommer)
	}
	if !f.Ouvert && f.Lieu == lieuAilleurs && f.Copie == nil {
		f.Actions = append(f.Actions, actionOublier)
	}
	return f
}

func vueCopie(l lien, cx *connexion) *copieGeree {
	etat := "pause"
	if cx != nil && cx.URL == l.URL {
		etat = "actif"
	}
	return &copieGeree{URL: l.URL, Compte: l.Compte, Copie: l.Copie, Nom: l.Nom, Etat: etat}
}

func copieDuChemin(chemin string, liens map[string]lien, cx *connexion) *copieGeree {
	for _, l := range liens {
		if l.Chemin != "" && cheminCle(l.Chemin) == cheminCle(chemin) {
			return vueCopie(l, cx)
		}
	}
	return nil
}

// ficheParCle : l'univers connu que désigne cette clé (a.gestion tenu pour agir dessus).
func (a *Application) ficheParCle(cle string) (ficheGeree, bool) {
	for _, f := range a.fichesGerees() {
		if f.Cle == cle {
			return f, true
		}
	}
	return ficheGeree{}, false
}

// routeGestion : la liste de la fenêtre.
func (a *Application) routeGestion(w http.ResponseWriter, r *http.Request) {
	repondre(w, 200, map[string]any{"univers": a.fichesGerees(), "dossier": a.DossierUnivers(), "dossier_archives": a.DossierArchives(),
		"parcourir": a.opt.Dialogue != nil})
}

// attendreEchange : un échange en cours avec le serveur se termine d'abord, car il écrit dans l'univers
// sans passer par le compte des requêtes. Faux après le délai : il dure, rien ne doit changer.
func (a *Application) attendreEchange() bool {
	limite := time.Now().Add(a.delaiFermeture)
	for !a.dist.mu.TryLock() {
		if time.Now().After(limite) {
			return false
		}
		select {
		case <-a.fin:
			return false
		case <-time.After(10 * time.Millisecond):
		}
	}
	a.dist.mu.Unlock()
	return true
}

// nomReserve : un nom que Windows garde pour ses périphériques, quelle qu'en soit l'extension. Refusé
// partout : un univers se copie d'un ordinateur à l'autre.
func nomReserve(nom string) bool {
	base, _, _ := strings.Cut(strings.ToUpper(strings.TrimSpace(nom)), ".")
	switch base {
	case "CON", "PRN", "AUX", "NUL":
		return true
	}
	return len(base) == 4 && (base[:3] == "COM" || base[:3] == "LPT") && base[3] >= '0' && base[3] <= '9'
}

// nomUniversSaisi : le nom d'univers saisi, sans extension ni ce qu'un nom de fichier ne peut contenir ;
// vide s'il n'en reste rien de valable.
func nomUniversSaisi(saisi string) string {
	nom := strings.TrimSpace(saisi)
	if strings.EqualFold(filepath.Ext(nom), store.Extension) {
		nom = nom[:len(nom)-len(store.Extension)]
	}
	nom = nomDeFichier(nom)
	if nomReserve(nom) {
		return ""
	}
	return nom
}

// existe : quelque chose porte ce nom, même un lien cassé.
func existe(chemin string) bool {
	_, err := os.Lstat(chemin)
	return err == nil
}

// nomPris : le fichier cible est déjà pris, y compris par un archivé (ou par un univers du dossier pour
// un archivé), qui ne pourrait plus revenir. Un même fichier écrit autrement (la casse) ne l'est pas.
func (a *Application) nomPris(f ficheGeree, cible string) bool {
	if memeFichier(f.Chemin, cible) {
		return false
	}
	if existe(cible) || existe(cible+suffixeJournal) {
		return true
	}
	switch f.Lieu {
	case lieuDossier:
		return existe(filepath.Join(a.DossierArchives(), filepath.Base(cible)))
	case lieuArchive:
		return existe(filepath.Join(a.DossierUnivers(), filepath.Base(cible)))
	}
	return false
}

// routeRenommerUnivers : {"nom": "…"} renomme le fichier, sur place. Les récents, le lien avec le serveur
// et le dernier univers ouvert le suivent ; les sauvegardes tiennent à l'identifiant et ne bougent pas.
func (a *Application) routeRenommerUnivers(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Nom string `json:"nom"`
	}
	if err := lireJSON(r, &in); err != nil {
		erreurStore(w, err)
		return
	}
	nom := nomUniversSaisi(in.Nom)
	if nom == "" {
		erreur(w, 422, "Ce nom ne convient pas à un univers.")
		return
	}
	a.gestion.Lock()
	defer a.gestion.Unlock()
	f, ok := a.ficheParCle(r.PathValue("cle"))
	if !ok {
		erreur(w, 404, "Univers inconnu.")
		return
	}
	if f.Interrompu {
		erreur(w, 409, msgInterrompu)
		return
	}
	if !f.peut(actionRenommer) {
		erreur(w, 422, "Ce fichier ne se renomme pas ici : ce n'est pas un fichier d'univers ordinaire.")
		return
	}
	cible := filepath.Join(filepath.Dir(f.Chemin), nom+store.Extension)
	if cible == f.Chemin {
		repondre(w, 200, map[string]any{"univers": f})
		return
	}
	if a.nomPris(f, cible) {
		erreur(w, 409, "Un univers porte déjà ce nom : rien n'a été modifié.")
		return
	}
	var code int
	var msg string
	if f.Ouvert {
		code, msg = a.renommerOuvert(f, cible, auteurDe(r))
	} else {
		code, msg = a.renommerFerme(f, cible)
	}
	if code != 0 {
		erreur(w, code, msg)
		return
	}
	log.Printf("univers « %s » renommé « %s »", f.Nom, nom)
	n, _ := a.ficheParCle(cleFichier(cible))
	repondre(w, 200, map[string]any{"univers": n})
}

// renommerFerme : le fichier n'est pas ouvert pour autant, il reste identique (a.gestion tenu).
func (a *Application) renommerFerme(f ficheGeree, cible string) (int, string) {
	if err := os.Rename(f.Chemin, cible); err != nil {
		return 500, auJournal("Renommage impossible", err)
	}
	a.suivreRenommage(f.Chemin, cible)
	return 0, ""
}

// renommerOuvert : l'univers quitte le service le temps de changer de nom, puis reprend. S'il ne rouvre
// pas sous son nouveau nom, il revient à l'ancien (a.gestion tenu).
func (a *Application) renommerOuvert(f ficheGeree, cible, auteur string) (int, string) {
	a.mu.RLock()
	o := a.tous[f.id]
	a.mu.RUnlock()
	if o == nil {
		return 404, "Univers inconnu."
	}
	if !a.suspendre(o) {
		return 409, msgOccupe
	}
	defer a.fermetureFinie(f.id)
	if !a.attendreEchange() {
		a.remettre(o)
		return 409, msgOccupe
	}
	if _, err := sauvegardeAuto(o.st, a.dossierSauvegardes(o.st)); err != nil {
		log.Printf("sauvegarde automatique : %v", err)
	}
	a.clore(o)
	if err := os.Rename(f.Chemin, cible); err != nil {
		if !a.reprendreOuvert(f.Chemin) {
			return 500, "L'univers ne se rouvre plus : voyez le journal de l'application."
		}
		return 500, auJournal("Renommage impossible, rien n'a été modifié", err)
	}
	a.suivreRenommage(f.Chemin, cible)
	st, err := store.Ouvrir(cible)
	if err != nil {
		log.Printf("univers renommé, non rouvert : %v", err)
		if e := os.Rename(cible, f.Chemin); e != nil {
			return 500, auJournal("L'univers est renommé, mais ne s'ouvre plus", e)
		}
		a.suivreRenommage(cible, f.Chemin)
		if !a.reprendreOuvert(f.Chemin) {
			return 500, "L'univers ne se rouvre plus : voyez le journal de l'application."
		}
		return 500, auJournal("Renommage impossible, rien n'a été modifié", err)
	}
	n := a.installer(st)
	n.st.Noter("univers", "univers renommé « "+nomUnivers(cible)+" » (avant : « "+f.Nom+" »)", auteur)
	return 0, ""
}

// reprendreOuvert rouvre un univers resté à sa place (a.gestion tenu).
func (a *Application) reprendreOuvert(chemin string) bool {
	st, err := store.Ouvrir(chemin)
	if err != nil {
		log.Printf("univers non rouvert : %v", err)
		return false
	}
	a.installer(st)
	return true
}

// suivreRenommage : ce que l'ordinateur retient de ce fichier suit son nouveau nom, à la même place
// (a.gestion tenu). Sans cela, une copie reliée paraîtrait disparue et serait tirée une seconde fois.
func (a *Application) suivreRenommage(ancien, cible string) {
	a.remplacerRecent(ancien, cible)
	if d, ok := a.reg.Meta(regUnivers); ok && cheminCle(d) == cheminCle(ancien) {
		_ = a.reg.SetMeta(regUnivers, cible)
	}
	for id, l := range a.liens() {
		if l.Chemin != "" && cheminCle(l.Chemin) == cheminCle(ancien) {
			l.Chemin = cible
			if err := a.ecrireLien(id, &l); err != nil {
				log.Printf("lien de la copie renommée : %v", err)
			}
		}
	}
}

// routeOublierUnivers : retire de la liste un univers rangé ailleurs ou disparu. Le fichier ne bouge
// jamais. Une copie reliée présente reste connue : la détacher se fait dans le panneau du serveur.
func (a *Application) routeOublierUnivers(w http.ResponseWriter, r *http.Request) {
	a.gestion.Lock()
	defer a.gestion.Unlock()
	f, ok := a.ficheParCle(r.PathValue("cle"))
	if !ok {
		erreur(w, 404, "Univers inconnu.")
		return
	}
	if !f.peut(actionOublier) {
		erreur(w, 409, "Cet univers reste dans la liste : il est ouvert, rangé dans le dossier des univers, ou c'est une copie reliée.")
		return
	}
	if err := a.retirerRecent(f.Chemin); err != nil {
		erreurStore(w, err)
		return
	}
	if f.Lieu == lieuIntrouvable { // sa copie a disparu : le lien ne mène plus à rien, et elle peut être tirée de nouveau
		for id, l := range a.liens() {
			if l.Chemin != "" && cheminCle(l.Chemin) == cheminCle(f.Chemin) {
				if err := a.ecrireLien(id, nil); err != nil {
					erreurStore(w, err)
					return
				}
			}
		}
	}
	repondre(w, 200, map[string]any{"oublie": true})
}

func (a *Application) ecrireRecents(l []string) error {
	b, err := json.Marshal(l)
	if err != nil {
		return err
	}
	return a.reg.SetMeta(regRecents, string(b))
}

func (a *Application) remplacerRecent(ancien, nouveau string) {
	l := a.recents()
	changer := false
	for i, c := range l {
		if cheminCle(c) == cheminCle(ancien) {
			l[i], changer = nouveau, true
		}
	}
	if changer {
		if err := a.ecrireRecents(l); err != nil {
			log.Printf("univers récents : %v", err)
		}
	}
}

func (a *Application) retirerRecent(chemin string) error {
	l := []string{}
	for _, c := range a.recents() {
		if cheminCle(c) != cheminCle(chemin) {
			l = append(l, c)
		}
	}
	return a.ecrireRecents(l)
}
