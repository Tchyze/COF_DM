package serveur

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Tchyze/cof_dm/internal/comptes"
	"github.com/Tchyze/cof_dm/internal/store"
)

// Serveur multi-univers. En mode serveur, tous les univers du dossier du serveur sont ouverts en
// même temps, et chaque compte a son rôle dans chacun. Une requête vise un univers :
//   - par l'en-tête « X-COF-Univers: <identifiant> » (clients de l'API, et chaque onglet de
//     l'interface, qui garde son univers) ; pour une lecture, aussi par ?univers=<identifiant> ;
//   - sinon par le choix de ce navigateur (cookie posé par POST /api/mes-univers/courant) ;
//   - sinon le premier univers où le compte a un rôle.
//
// Un jeton d'API qui a accès à plusieurs univers doit dire lequel : sans l'en-tête, la requête est
// refusée plutôt que d'écrire dans le mauvais monde. Les routes du compte lui-même (accès, mot de
// passe, jetons), qui ne touchent à aucun monde, n'ont pas à le dire.

const (
	cookieUnivers = "cof_dm_univers"
	enTeteUnivers = "X-COF-Univers"
	paramUnivers  = "univers"
)

const regServis = "serveur_univers" // {"identifiant": "fichier"} : le fichier qui tient chaque univers servi

func (a *Application) servis() map[string]string {
	m := map[string]string{}
	if v, ok := a.reg.Meta(regServis); ok {
		_ = json.Unmarshal([]byte(v), &m)
	}
	return m
}

// noterServis retient quel fichier tient chaque univers (verrou tenu). Ce qui était noté pour un
// univers pas encore rouvert est gardé ; un fichier disparu est oublié.
func (a *Application) noterServis() {
	m := a.servis()
	for id, chemin := range m {
		if _, err := os.Stat(chemin); err != nil {
			delete(m, id)
		}
	}
	for id, o := range a.tous {
		m[id] = o.st.Chemin()
	}
	b, _ := json.Marshal(m)
	_ = a.reg.SetMeta(regServis, string(b))
}

// ouvrirDossier ouvre les univers du dossier du serveur qui ne le sont pas encore. Les fichiers
// déjà servis passent d'abord : si une copie d'un univers traîne dans le dossier, c'est toujours
// l'original qui est servi, et non celui des deux que l'ordre alphabétique place en tête.
func (a *Application) ouvrirDossier() {
	for _, chemin := range a.servis() {
		if _, err := os.Stat(chemin); err == nil {
			if _, err := a.ouvrir(chemin, false); err != nil {
				log.Printf("univers %s non ouvert : %v", filepath.Base(chemin), err)
			}
		}
	}
	entrees, _ := os.ReadDir(a.DossierUnivers())
	for _, e := range entrees {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), store.Extension) {
			continue
		}
		if _, err := a.ouvrir(filepath.Join(a.DossierUnivers(), e.Name()), false); err != nil {
			log.Printf("univers %s non ouvert : %v", e.Name(), err)
		}
	}
	a.mu.Lock()
	a.noterServis()
	a.mu.Unlock()
}

// ordre : les univers ouverts, par nom (verrou tenu).
func (a *Application) ordre() []*universOuvert {
	l := make([]*universOuvert, 0, len(a.tous))
	for _, o := range a.tous {
		l = append(l, o)
	}
	sort.Slice(l, func(i, j int) bool {
		x, y := strings.ToLower(nomUnivers(l[i].st.Chemin())), strings.ToLower(nomUnivers(l[j].st.Chemin()))
		if x != y {
			return x < y
		}
		return l[i].st.ID() < l[j].st.ID()
	})
	return l
}

// reconnaitre : le compte qui fait la requête, sans rien répondre (voir entrer pour le contrôle).
func (a *acces) reconnaitre(r *http.Request) (c comptes.Compte, connu, jeton bool) {
	if j := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "); j != r.Header.Get("Authorization") {
		c, connu = a.comptes.ParJeton(j)
		return c, connu, true
	}
	c, connu = a.compteDeSession(r)
	return c, connu, false
}

// accessibles : les univers où le compte a un rôle (verrou tenu).
func (a *Application) accessibles(c comptes.Compte) []*universOuvert {
	var l []*universOuvert
	for _, o := range a.ordre() {
		if c.Role(o.st.ID()) != "" {
			l = append(l, o)
		}
	}
	return l
}

// choisir renvoie l'univers que vise la requête (verrou tenu). En cas de refus, code n'est pas nul.
func (a *Application) choisir(r *http.Request) (o *universOuvert, code int, msg string) {
	if id := universDemande(r); id != "" {
		if o := a.tous[id]; o != nil {
			return o, 0, ""
		}
		return nil, 404, "Univers inconnu : " + id
	}
	c, connu, jeton := a.opt.acces.reconnaitre(r)
	if t, err := r.Cookie(cookieUnivers); err == nil && !jeton {
		if o := a.tous[t.Value]; o != nil && (!connu || c.Role(t.Value) != "") {
			return o, 0, ""
		}
	}
	if connu {
		l := a.accessibles(c)
		if jeton && len(l) > 1 && !proprePerso(r.URL.Path) {
			return nil, 400, "Ce compte a accès à plusieurs univers : précisez lequel avec l'en-tête " + enTeteUnivers + " (identifiants : GET /api/mes-univers)."
		}
		if len(l) > 0 {
			return l[0], 0, ""
		}
	}
	return a.tous[a.st.ID()], 0, ""
}

// universDemande : l'univers que la requête désigne elle-même, par l'en-tête X-COF-Univers, ou, pour
// une lecture, par le paramètre ?univers= (ce que l'onglet ne peut pas munir d'un en-tête : images,
// WebSocket, téléchargements). Vide : le choix du navigateur (cookie), sinon le premier univers.
func universDemande(r *http.Request) string {
	if id := strings.TrimSpace(r.Header.Get(enTeteUnivers)); id != "" {
		return id
	}
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		return strings.TrimSpace(r.URL.Query().Get(paramUnivers))
	}
	return ""
}

// vise : l'univers sur lequel porte une requête déjà admise (verrou tenu). Hors serveur, le seul
// univers ouvert.
func (a *Application) vise(r *http.Request) *universOuvert {
	if a.st == nil {
		return nil
	}
	if a.multi() {
		if o, code, _ := a.choisir(r); code == 0 {
			return o
		}
	}
	return a.tous[a.st.ID()]
}

// viser fait de o l'univers de ce navigateur, et de la requête en cours.
func (a *Application) viser(w http.ResponseWriter, r *http.Request, o *universOuvert) *http.Request {
	http.SetCookie(w, &http.Cookie{Name: cookieUnivers, Value: o.st.ID(), Path: "/", Expires: time.Now().Add(365 * 24 * time.Hour),
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: a.opt.acces.https(r)})
	r = r.Clone(r.Context())
	r.Header.Set(enTeteUnivers, o.st.ID())
	return r
}

// servirMulti route une requête du mode serveur vers l'univers qu'elle vise (verrou de lecture tenu).
func (a *Application) servirMulti(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api/mes-univers" || strings.HasPrefix(r.URL.Path, "/api/mes-univers/") {
		a.mesUnivers(w, r)
		return
	}
	o, code, msg := a.choisir(r)
	if code != 0 {
		enTetes(w)
		erreur(w, code, msg)
		return
	}
	o.srv.ServeHTTP(w, r)
}

// mesUnivers : GET /api/mes-univers liste les univers où le compte a un rôle ;
// POST /api/mes-univers/courant {"id": "…"} choisit celui de ce navigateur.
func (a *Application) mesUnivers(w http.ResponseWriter, r *http.Request) {
	enTetes(w)
	r, ok := a.opt.acces.entrer(w, r)
	if !ok {
		return
	}
	c, connu := compteDe(r)
	if !connu {
		erreur(w, 401, "Connectez-vous avec votre compte, ou utilisez un jeton d'API.")
		return
	}
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/mes-univers":
		liste := []map[string]any{}
		for _, o := range a.accessibles(c) {
			liste = append(liste, map[string]any{"id": o.st.ID(), "nom": nomUnivers(o.st.Chemin()), "role": c.Role(o.st.ID())})
		}
		courant := ""
		if o, code, _ := a.choisir(r); code == 0 && c.Role(o.st.ID()) != "" {
			courant = o.st.ID()
		}
		repondre(w, 200, map[string]any{"univers": liste, "courant": courant})
	case r.Method == http.MethodPost && r.URL.Path == "/api/mes-univers/courant":
		var in struct {
			ID string `json:"id"`
		}
		if err := lireJSON(r, &in); err != nil {
			erreurStore(w, err)
			return
		}
		o := a.tous[in.ID]
		if o == nil || c.Role(in.ID) == "" { // inconnu ou fermé à ce compte : la même réponse
			erreur(w, 404, "Univers inconnu, ou auquel votre compte n'a pas accès.")
			return
		}
		a.viser(w, r, o)
		repondre(w, 200, map[string]any{"courant": in.ID, "nom": nomUnivers(o.st.Chemin())})
	default:
		erreur(w, 404, "Route inconnue.")
	}
}

// ouvrirSurServeur : le menu Fichier d'un administrateur. Un serveur n'ouvre que les univers de son
// dossier ; « ouvrir » y fait passer ce navigateur dans l'univers choisi.
func (a *Application) ouvrirSurServeur(w http.ResponseWriter, r *http.Request, chemin string) {
	chemin, err := filepath.Abs(strings.TrimSpace(chemin))
	dossier, _ := filepath.Abs(a.DossierUnivers())
	if err != nil || filepath.Dir(chemin) != dossier {
		erreur(w, 422, "Un serveur n'ouvre que les univers de son dossier : "+dossier)
		return
	}
	o, err := a.ouvrir(chemin, false)
	if err != nil {
		erreurStore(w, err)
		return
	}
	a.etat(w, a.viser(w, r, o))
}
