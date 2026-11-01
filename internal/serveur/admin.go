package serveur

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Tchyze/cof_dm/internal/comptes"
	"github.com/Tchyze/cof_dm/internal/store"
)

// Administration du serveur (mode serveur) : /api/admin/… L'administrateur crée, renomme, archive
// et supprime les univers, crée les comptes et les lie aux univers, sans jamais rien voir du
// contenu : ces routes ne lisent d'un univers que son identifiant, son nom (celui du fichier), sa
// taille et sa date, jamais le store. Elles ne passent pas par le Serveur d'un univers, et un
// administrateur n'entre dans aucun univers (comptes.Compte.Role).

const (
	regArchives    = "serveur_archives" // {"identifiant": "fichier"} : les univers archivés, dans DossierArchives
	dossierArchive = "archives"
)

// pageAdmin : les fichiers de la page d'administration, que l'application de bureau ne sert pas.
func pageAdmin(chemin string) bool {
	chemin = path.Clean(chemin)
	return chemin == "/admin.html" || chemin == "/js/admin.js"
}

// DossierArchives : les fichiers des univers archivés, hors du dossier ouvert au démarrage.
func (a *Application) DossierArchives() string {
	return filepath.Join(a.DossierUnivers(), dossierArchive)
}

func (a *Application) archives() map[string]string {
	m := map[string]string{}
	if v, ok := a.reg.Meta(regArchives); ok {
		_ = json.Unmarshal([]byte(v), &m)
	}
	return m
}

func (a *Application) noterArchives(m map[string]string) error {
	b, _ := json.Marshal(m)
	return a.reg.SetMeta(regArchives, string(b))
}

func (a *Application) routesAdmin() *http.ServeMux {
	m := http.NewServeMux()
	m.HandleFunc("GET /api/admin/univers", a.adminUnivers)
	m.HandleFunc("POST /api/admin/univers", a.adminCreerUnivers)
	m.HandleFunc("PATCH /api/admin/univers/{id}", a.adminModifierUnivers)
	m.HandleFunc("DELETE /api/admin/univers/{id}", a.adminSupprimerUnivers)
	m.HandleFunc("GET /api/admin/comptes", a.adminComptes)
	m.HandleFunc("POST /api/admin/comptes", a.adminCreerCompte)
	m.HandleFunc("PATCH /api/admin/comptes/{nom}", a.adminModifierCompte)
	m.HandleFunc("DELETE /api/admin/comptes/{nom}", a.adminSupprimerCompte)
	return m
}

// servirAdmin : un compte administrateur, connecté par son mot de passe (jamais par un jeton).
func (a *Application) servirAdmin(w http.ResponseWriter, r *http.Request) {
	enTetes(w)
	if !a.multi() {
		erreur(w, 404, "L'administration n'existe que sur un serveur.")
		return
	}
	r, ok := a.opt.acces.entrer(w, r)
	if !ok {
		return
	}
	if c, connu := compteDe(r); !connu || !c.Admin || parJeton(r) {
		erreur(w, 403, "Réservé à un compte administrateur, connecté par son mot de passe.")
		return
	}
	a.admin.ServeHTTP(w, r)
}

// ficheAdmin : un univers tel que l'administrateur le voit, sans rien de son contenu.
type ficheAdmin struct {
	ID      string         `json:"id"`
	Nom     string         `json:"nom"`
	Taille  int64          `json:"taille"`
	Modifie float64        `json:"modifie"`
	Archive bool           `json:"archive"`
	Comptes []compteLie    `json:"comptes"`
	chemin  string         // le fichier, gardé côté serveur
	ouvert  *universOuvert // nil : archivé
}

type compteLie struct {
	Nom  string `json:"nom"`
	Role string `json:"role,omitempty"` // mj, auditeur, joueur ; vide : admis comme joueur, pas encore joint
}

// fichesAdmin : les univers servis puis archivés, par nom (verrou tenu).
func (a *Application) fichesAdmin() []ficheAdmin {
	var l []ficheAdmin
	for _, o := range a.ordre() {
		l = append(l, ficheAdmin{ID: o.st.ID(), Nom: nomUnivers(o.st.Chemin()), chemin: o.st.Chemin(), ouvert: o})
	}
	var arch []ficheAdmin
	for id, f := range a.archives() {
		arch = append(arch, ficheAdmin{ID: id, Nom: nomUnivers(f), Archive: true, chemin: filepath.Join(a.DossierArchives(), f)})
	}
	sort.Slice(arch, func(i, j int) bool { return strings.ToLower(arch[i].Nom) < strings.ToLower(arch[j].Nom) })
	l = append(l, arch...)
	tous := a.opt.Comptes.Liste()
	for i := range l {
		if info, err := os.Stat(l[i].chemin); err == nil {
			l[i].Taille, l[i].Modifie = info.Size(), float64(info.ModTime().Unix())
		}
		l[i].Comptes = []compteLie{}
		for _, c := range tous {
			if role := c.Role(l[i].ID); role != "" {
				l[i].Comptes = append(l[i].Comptes, compteLie{c.Nom, role})
			} else if c.PeutJouer(l[i].ID) {
				l[i].Comptes = append(l[i].Comptes, compteLie{Nom: c.Nom})
			}
		}
	}
	return l
}

func (a *Application) ficheAdmin(id string) (ficheAdmin, bool) {
	for _, f := range a.fichesAdmin() {
		if f.ID == id {
			return f, true
		}
	}
	return ficheAdmin{}, false
}

func (a *Application) adminUnivers(w http.ResponseWriter, r *http.Request) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	repondre(w, 200, a.fichesAdmin())
}

// adminCreerUnivers : {"nom": "…"} crée un univers vide dans le dossier du serveur. Son contenu
// vient ensuite de son MJ (saisie, import, bureau relié).
func (a *Application) adminCreerUnivers(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Nom string `json:"nom"`
	}
	if err := lireJSON(r, &in); err != nil {
		erreurStore(w, err)
		return
	}
	chemin, code, msg := a.cheminLibre(in.Nom)
	if code != 0 {
		erreur(w, code, msg)
		return
	}
	o, err := a.ouvrir(chemin, true)
	if err != nil {
		erreurStore(w, err)
		return
	}
	o.st.Noter("univers", "univers créé sur le serveur", auteurDe(r))
	a.mu.RLock()
	f, _ := a.ficheAdmin(o.st.ID())
	a.mu.RUnlock()
	repondre(w, 201, f)
}

// cheminLibre : le fichier d'un univers nommé « nom » dans le dossier du serveur, s'il n'est pris ni
// par un univers servi ni par un archivé.
func (a *Application) cheminLibre(nom string) (string, int, string) {
	nom = nomDeFichier(nom)
	if nom == "" {
		return "", 422, "Donnez un nom à l'univers."
	}
	chemin := avecExtension(filepath.Join(a.DossierUnivers(), nom))
	if _, err := os.Stat(chemin); err == nil {
		return "", 409, "Un univers porte déjà ce nom sur le serveur."
	}
	if _, err := os.Stat(filepath.Join(a.DossierArchives(), filepath.Base(chemin))); err == nil {
		return "", 409, "Un univers archivé porte déjà ce nom."
	}
	return chemin, 0, ""
}

// adminModifierUnivers : {"nom"?: "…", "archive"?: true|false}. Renommer change le nom du fichier ;
// archiver ferme l'univers et le range à part : il sort de toutes les listes et n'est plus
// accessible à personne, fichier et rôles gardés, jusqu'à ce qu'un administrateur le désarchive.
func (a *Application) adminModifierUnivers(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Nom     *string `json:"nom"`
		Archive *bool   `json:"archive"`
	}
	if err := lireJSON(r, &in); err != nil {
		erreurStore(w, err)
		return
	}
	id := r.PathValue("id")
	a.mu.Lock()
	defer a.mu.Unlock()
	f, ok := a.ficheAdmin(id)
	if !ok {
		erreur(w, 404, "Univers inconnu.")
		return
	}
	if in.Archive != nil && *in.Archive != f.Archive {
		var code int
		var msg string
		if *in.Archive {
			code, msg = a.archiver(f)
		} else {
			code, msg = a.desarchiver(f)
		}
		if code != 0 {
			erreur(w, code, msg)
			return
		}
		log.Printf("univers « %s » %s par %s", f.Nom, map[bool]string{true: "archivé", false: "désarchivé"}[*in.Archive], auteurDe(r))
		f, _ = a.ficheAdmin(id)
	}
	if in.Nom != nil && nomDeFichier(*in.Nom) != f.Nom {
		if code, msg := a.renommer(f, *in.Nom, auteurDe(r)); code != 0 {
			erreur(w, code, msg)
			return
		}
		f, _ = a.ficheAdmin(id)
	}
	repondre(w, 200, f)
}

// retirer ferme un univers servi, après sa sauvegarde automatique (verrou d'écriture tenu). Si
// c'était l'univers par défaut, un autre le devient.
func (a *Application) retirer(o *universOuvert) {
	if _, err := sauvegardeAuto(o.st, a.dossierSauvegardes(o.st)); err != nil {
		log.Printf("sauvegarde automatique : %v", err)
	}
	if o.srv != nil {
		o.srv.Fermer()
	}
	_ = o.st.Fermer()
	delete(a.tous, o.st.ID())
	if a.st == o.st {
		a.st, a.srv = nil, nil
		if l := a.ordre(); len(l) > 0 {
			a.st, a.srv = l[0].st, l[0].srv
			_ = a.reg.SetMeta(regUnivers, a.st.Chemin())
		}
	}
}

// dernier : c'est le seul univers servi ; il reste, pour que le serveur ait toujours où répondre.
func (a *Application) dernier(f ficheAdmin) bool {
	return f.ouvert != nil && len(a.tous) == 1
}

const msgDernierUnivers = "C'est le dernier univers servi : créez-en un autre d'abord."

func (a *Application) archiver(f ficheAdmin) (int, string) {
	if a.dernier(f) {
		return 409, msgDernierUnivers
	}
	if err := os.MkdirAll(a.DossierArchives(), 0o700); err != nil {
		return 500, "Archivage impossible : " + err.Error()
	}
	cible := filepath.Join(a.DossierArchives(), filepath.Base(f.chemin))
	if _, err := os.Stat(cible); err == nil {
		return 409, "Un univers archivé porte déjà ce nom : renommez celui-ci d'abord."
	}
	a.retirer(f.ouvert)
	if err := os.Rename(f.chemin, cible); err != nil {
		a.rouvrir(f.chemin)
		return 500, "Archivage impossible : " + err.Error()
	}
	m := a.archives()
	m[f.ID] = filepath.Base(cible)
	if err := a.noterArchives(m); err != nil {
		log.Printf("univers archivés : %v", err)
	}
	a.noterServis()
	return 0, ""
}

func (a *Application) desarchiver(f ficheAdmin) (int, string) {
	cible := filepath.Join(a.DossierUnivers(), filepath.Base(f.chemin))
	if _, err := os.Stat(cible); err == nil {
		return 409, "Un univers servi porte déjà ce nom : renommez-le d'abord."
	}
	if err := os.Rename(f.chemin, cible); err != nil {
		return 500, "Désarchivage impossible : " + err.Error()
	}
	m := a.archives()
	delete(m, f.ID)
	if err := a.noterArchives(m); err != nil {
		log.Printf("univers archivés : %v", err)
	}
	if o := a.rouvrir(cible); o == nil {
		return 500, "L'univers est revenu dans le dossier du serveur, mais ne s'ouvre pas (voir le journal du serveur)."
	}
	return 0, ""
}

// rouvrir ouvre un fichier d'univers du dossier du serveur (verrou d'écriture tenu).
func (a *Application) rouvrir(chemin string) *universOuvert {
	st, err := store.Ouvrir(chemin)
	if err != nil {
		log.Printf("univers %s non rouvert : %v", filepath.Base(chemin), err)
		return nil
	}
	if _, pris := a.tous[st.ID()]; pris {
		_ = st.Fermer()
		log.Printf("univers %s non rouvert : une copie du même univers est déjà servie", filepath.Base(chemin))
		return nil
	}
	return a.installer(st)
}

func (a *Application) renommer(f ficheAdmin, nom, auteur string) (int, string) {
	chemin, code, msg := a.cheminLibre(nom)
	if code != 0 {
		return code, msg
	}
	if f.Archive {
		chemin = filepath.Join(a.DossierArchives(), filepath.Base(chemin))
		if err := os.Rename(f.chemin, chemin); err != nil {
			return 500, "Renommage impossible : " + err.Error()
		}
		m := a.archives()
		m[f.ID] = filepath.Base(chemin)
		_ = a.noterArchives(m)
		return 0, ""
	}
	defaut := a.st == f.ouvert.st
	a.retirer(f.ouvert)
	echec := os.Rename(f.chemin, chemin)
	if echec != nil {
		chemin = f.chemin // rouvert tel quel
	}
	o := a.rouvrir(chemin)
	if o != nil && defaut {
		a.st, a.srv = o.st, o.srv
		_ = a.reg.SetMeta(regUnivers, chemin)
	}
	a.noterServis()
	if echec != nil {
		return 500, "Renommage impossible : " + echec.Error()
	}
	if o == nil {
		return 500, "L'univers est renommé, mais ne s'ouvre plus (voir le journal du serveur)."
	}
	o.st.Noter("univers", "univers renommé « "+nomUnivers(chemin)+" » (avant : « "+f.Nom+" »)", auteur)
	return 0, ""
}

// adminSupprimerUnivers : {"nom": "<nom de l'univers>"}, retapé pour confirmer. Une sauvegarde est
// faite avant (rangée avec les autres sauvegardes de l'univers, que la suppression garde) ; les
// rôles des comptes dans cet univers sont retirés.
func (a *Application) adminSupprimerUnivers(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Nom string `json:"nom"`
	}
	if err := lireJSON(r, &in); err != nil {
		erreurStore(w, err)
		return
	}
	id := r.PathValue("id")
	a.mu.Lock()
	defer a.mu.Unlock()
	f, ok := a.ficheAdmin(id)
	if !ok {
		erreur(w, 404, "Univers inconnu.")
		return
	}
	if strings.TrimSpace(in.Nom) != f.Nom {
		erreur(w, 422, "Retapez le nom de l'univers (« "+f.Nom+" ») pour confirmer sa suppression.")
		return
	}
	if a.dernier(f) {
		erreur(w, 409, msgDernierUnivers)
		return
	}
	dossier := filepath.Join(a.opt.DossierDon, "sauvegardes", id)
	if err := os.MkdirAll(dossier, 0o700); err != nil {
		erreur(w, 500, "Sauvegarde impossible, rien n'a été supprimé : "+err.Error())
		return
	}
	sauvegarde := ""
	if f.ouvert != nil {
		s, err := sauvegarder(f.ouvert.st, dossier, "avant-suppression")
		if err != nil {
			erreur(w, 500, "Sauvegarde impossible, rien n'a été supprimé : "+err.Error())
			return
		}
		sauvegarde = s
		a.retirer(f.ouvert)
	} else {
		sauvegarde = filepath.Join(dossier, nomSauvegarde("avant-suppression"))
		if err := copierFichier(f.chemin, sauvegarde); err != nil {
			_ = os.Remove(sauvegarde)
			erreur(w, 500, "Sauvegarde impossible, rien n'a été supprimé : "+err.Error())
			return
		}
	}
	if err := os.Remove(f.chemin); err != nil && !errors.Is(err, os.ErrNotExist) {
		if f.ouvert != nil {
			a.rouvrir(f.chemin)
		}
		erreur(w, 500, "Suppression impossible : "+err.Error())
		return
	}
	if f.Archive {
		m := a.archives()
		delete(m, id)
		_ = a.noterArchives(m)
	}
	a.noterServis()
	if err := a.opt.Comptes.OublierUnivers(id); err != nil {
		log.Printf("rôles de l'univers supprimé : %v", err)
	}
	log.Printf("univers « %s » (%s) supprimé par %s ; sauvegarde : %s", f.Nom, id, auteurDe(r), sauvegarde)
	repondre(w, 200, map[string]any{"supprime": true, "sauvegarde": filepath.Base(sauvegarde)})
}

// ---------------------------------------------------------------- comptes

// vueAdmin : un compte tel que l'administrateur le voit : ses rôles et ses admissions, sans
// empreinte ni PJ (le PJ est le contenu d'un univers, qui regarde son MJ).
func vueAdmin(c comptes.Compte) map[string]any {
	roles := map[string]string{}
	for u, role := range c.Roles {
		roles[u] = role
	}
	return map[string]any{"nom": c.Nom, "admin": c.Admin, "inactif": c.Inactif, "roles": roles,
		"admis": append([]string{}, c.Admis...), "jetons": len(c.Jetons), "cree": c.Cree}
}

func (a *Application) adminComptes(w http.ResponseWriter, r *http.Request) {
	out := []map[string]any{}
	for _, c := range a.opt.Comptes.Liste() {
		out = append(out, vueAdmin(c))
	}
	repondre(w, 200, out)
}

// adminCreerCompte : {"nom", "mdp", "admin"?}.
func (a *Application) adminCreerCompte(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Nom   string `json:"nom"`
		Mdp   string `json:"mdp"`
		Admin bool   `json:"admin"`
	}
	if err := lireJSON(r, &in); err != nil {
		erreurStore(w, err)
		return
	}
	c, err := a.opt.Comptes.Creer(in.Nom, in.Mdp, in.Admin)
	if err != nil {
		erreurCompte(w, err)
		return
	}
	log.Printf("compte « %s » créé par %s", c.Nom, auteurDe(r))
	repondre(w, 201, vueAdmin(c))
}

// adminModifierCompte : {"mdp"?, "admin"?, "inactif"?, "roles"?: {"<univers>": "mj"|"auditeur"|""},
// "admis"?: {"<univers>": true|false}}. L'administrateur donne les rôles MJ et auditeur ; pour un
// joueur, il l'admet dans un univers, et c'est le MJ qui l'y joint. Ne plus l'admettre l'en retire.
func (a *Application) adminModifierCompte(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Mdp     *string           `json:"mdp"`
		Admin   *bool             `json:"admin"`
		Inactif *bool             `json:"inactif"`
		Roles   map[string]string `json:"roles"`
		Admis   map[string]bool   `json:"admis"`
	}
	if err := lireJSON(r, &in); err != nil {
		erreurStore(w, err)
		return
	}
	a.mu.RLock()
	connus := map[string]bool{}
	for _, f := range a.fichesAdmin() {
		connus[f.ID] = true
	}
	a.mu.RUnlock()
	for u, role := range in.Roles {
		switch {
		case !connus[u]:
			erreur(w, 404, "Univers inconnu : "+u)
			return
		case role != "" && role != comptes.MJ && role != comptes.Auditeur:
			erreur(w, 422, "L'administrateur donne les rôles mj et auditeur ; un joueur est admis dans l'univers (« admis »), puis joint par son MJ.")
			return
		}
	}
	for u := range in.Admis {
		if !connus[u] {
			erreur(w, 404, "Univers inconnu : "+u)
			return
		}
	}
	nom := r.PathValue("nom")
	c, err := a.opt.Comptes.Modifier(nom, func(x *comptes.Compte) error {
		if in.Admin != nil {
			x.Admin = *in.Admin
		}
		if in.Inactif != nil {
			x.Inactif = *in.Inactif
		}
		for u, role := range in.Roles {
			if role != "" || x.Roles[u] != comptes.Joueur { // retirer un joueur passe par « admis »
				x.Roles[u] = role
			}
		}
		for u, oui := range in.Admis {
			if i := indexDe(x.Admis, u); oui && i < 0 {
				x.Admis = append(x.Admis, u)
			} else if !oui && i >= 0 {
				x.Admis = append(x.Admis[:i], x.Admis[i+1:]...)
			}
		}
		return nil
	})
	if err == nil && in.Mdp != nil {
		if err = a.opt.Comptes.ReinitialiserMdp(nom, *in.Mdp); err == nil {
			c, _ = a.opt.Comptes.Lire(nom)
		}
	}
	if err != nil {
		erreurCompte(w, err)
		return
	}
	log.Printf("compte « %s » modifié par %s", c.Nom, auteurDe(r))
	repondre(w, 200, vueAdmin(c))
}

func indexDe(l []string, v string) int {
	for i, x := range l {
		if x == v {
			return i
		}
	}
	return -1
}

func (a *Application) adminSupprimerCompte(w http.ResponseWriter, r *http.Request) {
	nom := comptes.Normaliser(r.PathValue("nom"))
	if err := a.opt.Comptes.Supprimer(nom); err != nil {
		erreurCompte(w, err)
		return
	}
	log.Printf("compte « %s » supprimé par %s", nom, auteurDe(r))
	repondre(w, 200, map[string]any{"supprime": true})
}
