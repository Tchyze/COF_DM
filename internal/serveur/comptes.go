package serveur

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"crypto/rand"

	"github.com/Tchyze/cof_dm/internal/comptes"
)

// Mode serveur : l'application tourne sur une machine distante, derrière un reverse proxy qui porte
// le HTTPS. Chacun entre avec son compte (nom d'utilisateur, mot de passe) ou un de ses jetons
// d'API. Rien n'est accordé aux requêtes venues de la machine elle-même : celles du proxy en
// viennent toutes.
//
// Droits : l'administrateur gère les comptes, les univers et les réglages du serveur (admin.go),
// sans entrer dans aucun univers ; dans un univers, le MJ lit et modifie, l'auditeur lit tout sans
// rien modifier, le joueur n'a que sa page. Un compte sans rôle dans l'univers ouvert n'y entre pas.

const (
	essaisMaxMdp     = 20              // essais ratés de l'ancien mot de passe, par compte
	essaisMaxAdresse = 30              // essais ratés d'une adresse, tous noms confondus
	essaisSeuilNom   = 20              // essais ratés sur un même nom, d'où qu'ils viennent, avant d'y ralentir
	essaisPlafondNom = 1000            // au-delà, ils ne sont plus retenus : l'attente est déjà au plus haut
	attenteNomMax    = 5 * time.Second // ralentissement maximal d'un essai
)

type cleCompte struct{}
type cleServeur struct{}
type cleJeton struct{}

// compteDe : le compte qui fait la requête (mode serveur).
func compteDe(r *http.Request) (comptes.Compte, bool) {
	c, ok := r.Context().Value(cleCompte{}).(comptes.Compte)
	return c, ok
}

func modeServeur(r *http.Request) bool {
	v, _ := r.Context().Value(cleServeur{}).(bool)
	return v
}

// parJeton : la requête porte un jeton d'API, et non la session d'un navigateur.
func parJeton(r *http.Request) bool {
	v, _ := r.Context().Value(cleJeton{}).(bool)
	return v
}

func boucleLocale(r *http.Request) bool {
	hote, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		hote = r.RemoteAddr
	}
	ip := net.ParseIP(hote)
	return ip != nil && ip.IsLoopback()
}

// sessionCompte : jeton de session d'un compte, « échéance.identifiant.nom.version.graine.signature ».
func (a *acces) sessionCompte(c comptes.Compte, exp time.Time) string {
	id := make([]byte, 9)
	_, _ = rand.Read(id)
	return a.signer(strings.Join([]string{strconv.FormatInt(exp.Unix(), 10), base64.RawURLEncoding.EncodeToString(id), c.Nom, strconv.Itoa(c.Version), c.Graine}, "."))
}

// compteDeSession : le compte actif dont c'est la session, tant que son mot de passe n'a pas changé
// et que c'est bien ce compte, pas un autre recréé sous le même nom.
func (a *acces) compteDeSession(r *http.Request) (comptes.Compte, bool) {
	t, err := r.Cookie(cookieMJ)
	if err != nil {
		return comptes.Compte{}, false
	}
	id, _, plus, ok := a.lireSession(t.Value)
	if !ok || len(plus) != 3 || !a.ouverte(id) {
		return comptes.Compte{}, false
	}
	c, ok := a.comptes.Lire(plus[0])
	if !ok || c.Inactif || strconv.Itoa(c.Version) != plus[1] || c.Graine == "" || c.Graine != plus[2] {
		return comptes.Compte{}, false
	}
	return c, true
}

// entrer identifie qui fait la requête, en mode serveur. Elle renvoie la requête marquée du compte,
// ou sans compte pour ce qui est « ouvert » (la page de connexion). En cas de refus, la réponse est
// déjà écrite.
func (a *acces) entrer(w http.ResponseWriter, r *http.Request) (*http.Request, bool) {
	ctx := context.WithValue(r.Context(), cleServeur{}, true)
	var c comptes.Compte
	connu, jeton := false, false
	if j := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "); j != r.Header.Get("Authorization") {
		if c, connu = a.comptes.ParJeton(j); !connu {
			erreur(w, 401, "Jeton d'API invalide.")
			return nil, false
		}
		jeton = true
		ctx = context.WithValue(ctx, cleJeton{}, true)
	} else {
		c, connu = a.compteDeSession(r)
	}
	// sans jeton, les écritures portent l'en-tête qu'une page d'un autre site ne peut pas envoyer
	if !jeton && r.Method != http.MethodGet && r.Method != http.MethodHead && r.Header.Get("X-COF-DM") != "1" {
		erreur(w, 403, "Requête refusée.")
		return nil, false
	}
	if !connu {
		if !ouvert(r) {
			erreur(w, 401, "Connectez-vous avec votre compte, ou utilisez un jeton d'API.")
			return nil, false
		}
		return r.WithContext(context.WithValue(ctx, cleAnonyme{}, true)), true
	}
	ctx = context.WithValue(ctx, cleCompte{}, c)
	return r.WithContext(context.WithValue(ctx, cleContexte{}, c.Nom)), true
}

// proprePerso : ce qui touche au compte lui-même, ouvert à tout compte quel que soit son rôle.
func proprePerso(chemin string) bool {
	return chemin == "/api/acces" || strings.HasPrefix(chemin, "/api/acces/") || chemin == "/api/compte" || strings.HasPrefix(chemin, "/api/compte/")
}

func (s *Serveur) servirComptes(w http.ResponseWriter, r *http.Request) {
	r, ok := s.acces.entrer(w, r)
	if !ok {
		return
	}
	c, connu := compteDe(r)
	chemin := r.URL.Path
	api := strings.HasPrefix(chemin, "/api/") || strings.HasPrefix(chemin, "/medias/")
	lecture := r.Method == http.MethodGet || r.Method == http.MethodHead
	switch role := c.Role(s.store.ID()); {
	case !connu, !api, proprePerso(chemin):
	case c.Admin: // l'administrateur n'entre dans aucun univers (admin.go) ; la version du serveur lui répond
		if chemin != "/api/sante" {
			erreur(w, 403, "L'administrateur gère le serveur sans entrer dans les univers.")
			return
		}
	case role == "":
		erreur(w, 403, "Votre compte n'a pas accès à cet univers.")
		return
	case role == comptes.Joueur: // liste blanche, contenu filtré (joueur.go)
		s.servir(s.joueur, w, r)
		return
	case !lecture && role != comptes.MJ:
		erreur(w, 403, "Votre compte lit cet univers sans pouvoir le modifier.")
		return
	}
	s.servir(s.mux, w, r)
}

// ---------------------------------------------------------------- routes

func (s *Serveur) routesComptes() {
	m := s.mux
	s.routesJoueur()
	m.HandleFunc("GET /api/acces", s.etatCompte)
	m.HandleFunc("POST /api/acces/connexion", s.connexionCompte)
	m.HandleFunc("GET /api/compte", s.monCompte)
	m.HandleFunc("POST /api/compte/mdp", s.monMdp)
	m.HandleFunc("POST /api/compte/jetons", s.monJeton)
	m.HandleFunc("DELETE /api/compte/jetons/{id}", s.retirerMonJeton)
	m.HandleFunc("DELETE /api/compte/jeton", s.rendreJeton)
	m.HandleFunc("GET /api/joueurs/comptes", s.joueursAdmis)
	m.HandleFunc("PATCH /api/joueurs/comptes/{nom}", s.joindreJoueur)
	m.HandleFunc("PUT /api/joueurs/edition", s.reglerEditionJoueurs)
}

// vueCompte : un compte tel que l'API le montre à son titulaire, sans empreinte de mot de passe ni
// de jeton. Pour un administrateur, rien de l'univers où passe la requête.
func (s *Serveur) vueCompte(c comptes.Compte) map[string]any {
	jetons := []map[string]any{}
	for _, j := range c.Jetons {
		v := map[string]any{"id": j.ID, "nom": j.Nom, "cree": j.Cree}
		if j.Utilise != 0 {
			v["utilise"] = j.Utilise
		}
		jetons = append(jetons, v)
	}
	out := map[string]any{"nom": c.Nom, "admin": c.Admin, "inactif": c.Inactif, "jetons": jetons, "cree": c.Cree}
	if !c.Admin {
		out["role"], out["roles"], out["pj"] = c.Role(s.store.ID()), c.Roles, c.Persos[s.store.ID()]
	}
	return out
}

func erreurCompte(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, comptes.ErrInconnu):
		erreur(w, 404, "Compte ou jeton introuvable.")
	case errors.Is(err, comptes.ErrExiste), errors.Is(err, comptes.ErrDernierAdmin), errors.Is(err, comptes.ErrAdmin), errors.Is(err, comptes.ErrNonAdmis):
		erreur(w, 409, majuscule(err.Error())+".")
	case errors.Is(err, comptes.ErrNom), errors.Is(err, comptes.ErrMdp), errors.Is(err, comptes.ErrMdpAdmin), errors.Is(err, comptes.ErrMdpFaible), errors.Is(err, comptes.ErrRole), errors.Is(err, comptes.ErrTropDeJetons):
		erreur(w, 422, majuscule(err.Error())+".")
	default:
		erreurStore(w, err)
	}
}

func majuscule(s string) string {
	if s == "" {
		return s
	}
	rs := []rune(s)
	return strings.ToUpper(string(rs[:1])) + string(rs[1:])
}

// etatCompte : qui est en face. « mj » dit si l'interface peut s'ouvrir sur l'univers.
func (s *Serveur) etatCompte(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{"serveur": true, "local": false, "mdp_defini": true, "mj": false, "connecte": false}
	if c, ok := compteDe(r); ok {
		role := c.Role(s.store.ID())
		out["connecte"], out["mj"], out["compte"] = true, role != "", s.vueCompte(c)
		out["lecture_seule"] = role == comptes.Auditeur || role == comptes.Joueur
		out["joueur"] = role == comptes.Joueur
		if role != "" {
			out["edition_joueurs"] = s.editionJoueurs()
		}
	}
	repondre(w, 200, out)
}

func (s *Serveur) connexionCompte(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Nom string `json:"nom"`
		Mdp string `json:"mdp"`
	}
	_ = json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&req)
	nom := comptes.Normaliser(req.Nom)
	if rs := []rune(nom); len(rs) > 32 {
		nom = string(rs[:32])
	}
	adresse := cleAdresse(s.acces.adresse(r))
	parIP, parCouple, parNom := "ip:"+adresse, "couple:"+nom+"|"+adresse, "nom:"+nom
	if !s.acces.reserverEssai(parIP, essaisMaxAdresse) {
		s.acces.noter(r, "limite_atteinte", nom, "portee=adresse")
		erreur(w, 429, "Trop d'essais : patientez quelques minutes.")
		return
	}
	if !s.acces.reserverEssai(parCouple, essaisMax) {
		s.acces.rendreEssai(parIP)
		s.acces.noter(r, "limite_atteinte", nom, "portee=couple")
		erreur(w, 429, "Trop d'essais sur ce compte : patientez quelques minutes.")
		return
	}
	attente := s.acces.attenteNom(parNom)
	if s.acces.ralenti(parNom) {
		s.acces.noter(r, "limite_atteinte", nom, "portee=nom")
	}
	s.acces.reserverEssai(parNom, essaisPlafondNom)
	time.Sleep(attente)
	s.acces.hachage.Lock()
	c, ok := s.acces.comptes.Verifier(nom, req.Mdp)
	s.acces.hachage.Unlock()
	if !ok {
		s.acces.noter(r, "connexion_echec", nom)
		time.Sleep(s.acces.pause)
		erreur(w, 401, "Nom d'utilisateur ou mot de passe incorrect.")
		return
	}
	s.acces.rendreEssai(parIP)
	s.acces.rendreEssai(parCouple)
	s.acces.rendreEssai(parNom)
	s.acces.noter(r, "connexion_ok", c.Nom)
	exp := time.Now().Add(dureeSession)
	http.SetCookie(w, s.acces.temoin(r, s.acces.sessionCompte(c, exp), exp))
	repondre(w, 200, map[string]any{"mj": c.Role(s.store.ID()) != "", "compte": s.vueCompte(c)})
}

func (s *Serveur) monCompte(w http.ResponseWriter, r *http.Request) {
	c, _ := compteDe(r)
	repondre(w, 200, s.vueCompte(c))
}

// noterCompte : ce que fait un compte sur lui-même entre au journal de l'univers où il se trouve,
// s'il y a un rôle (pas celui d'un univers où il n'entre pas, ni pour un administrateur).
func (s *Serveur) noterCompte(c comptes.Compte, msg string) {
	if c.Role(s.store.ID()) != "" {
		s.store.Noter("compte", msg, c.Nom)
	}
}

// parMdp refuse à un jeton d'API ce qui garde le compte : son mot de passe et ses jetons.
func parMdp(w http.ResponseWriter, r *http.Request) bool {
	if parJeton(r) {
		erreur(w, 403, "Connectez-vous avec votre mot de passe pour ce réglage.")
		return false
	}
	return true
}

// monMdp : {"ancien": "…", "nouveau": "…"}. Les autres sessions du compte tombent.
func (s *Serveur) monMdp(w http.ResponseWriter, r *http.Request) {
	if !parMdp(w, r) {
		return
	}
	var req struct {
		Ancien  string `json:"ancien"`
		Nouveau string `json:"nouveau"`
		Retirer bool   `json:"retirer_jetons"`
	}
	if err := lireJSON(r, &req); err != nil {
		erreurStore(w, err)
		return
	}
	c, _ := compteDe(r)
	cle := "mdp:" + c.Nom // pas le compteur de la connexion : un anonyme n'empêche pas ce changement
	if !s.acces.reserverEssai(cle, essaisMaxMdp) {
		s.acces.noter(r, "limite_atteinte", c.Nom, "portee=mdp")
		erreur(w, 429, "Trop d'essais : patientez quelques minutes.")
		return
	}
	s.acces.hachage.Lock()
	_, ok := s.acces.comptes.Verifier(c.Nom, req.Ancien)
	s.acces.hachage.Unlock()
	if !ok {
		s.acces.noter(r, "mdp_echec", c.Nom)
		erreur(w, 401, "Mot de passe actuel incorrect.")
		return
	}
	s.acces.rendreEssai(cle)
	retires, err := s.acces.comptes.ChangerMdp(c.Nom, req.Nouveau, req.Retirer)
	if err != nil {
		erreurCompte(w, err)
		return
	}
	c, _ = s.acces.comptes.Lire(c.Nom)
	exp := time.Now().Add(dureeSession)
	http.SetCookie(w, s.acces.temoin(r, s.acces.sessionCompte(c, exp), exp))
	s.noterCompte(c, "mot de passe changé")
	s.acces.noter(r, "mdp_change", c.Nom, "jetons_retires="+strconv.Itoa(retires))
	repondre(w, 200, map[string]any{"ok": true, "jetons_retires": retires})
}

// monJeton : {"nom": "…"} crée un jeton d'API, montré cette seule fois.
func (s *Serveur) monJeton(w http.ResponseWriter, r *http.Request) {
	if !parMdp(w, r) {
		return
	}
	var req struct {
		Nom string `json:"nom"`
	}
	if err := lireJSON(r, &req); err != nil {
		erreurStore(w, err)
		return
	}
	c, _ := compteDe(r)
	clair, j, err := s.acces.comptes.NouveauJeton(c.Nom, req.Nom)
	if err != nil {
		erreurCompte(w, err)
		return
	}
	s.noterCompte(c, "jeton d'API créé")
	s.acces.noter(r, "jeton_cree", c.Nom, "jeton="+j.ID)
	repondre(w, 201, map[string]any{"jeton": clair, "id": j.ID, "nom": j.Nom, "cree": j.Cree})
}

func (s *Serveur) retirerMonJeton(w http.ResponseWriter, r *http.Request) {
	if !parMdp(w, r) {
		return
	}
	c, _ := compteDe(r)
	if err := s.acces.comptes.RetirerJeton(c.Nom, r.PathValue("id")); err != nil {
		erreurCompte(w, err)
		return
	}
	s.noterCompte(c, "jeton d'API retiré")
	s.acces.noter(r, "jeton_retire", c.Nom, "jeton="+strconv.Quote(r.PathValue("id")))
	repondre(w, 200, map[string]any{"supprime": true})
}

// rendreJeton : le jeton d'API qui fait la requête se retire lui-même (un poste qui se déconnecte
// du serveur). Il ne touche à rien d'autre : ni aux autres jetons, ni au compte.
func (s *Serveur) rendreJeton(w http.ResponseWriter, r *http.Request) {
	if !parJeton(r) {
		erreur(w, 409, "Cette route retire le jeton d'API qui l'appelle : appelez-la avec ce jeton.")
		return
	}
	c, _ := compteDe(r)
	if err := s.acces.comptes.RetirerParJeton(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")); err != nil {
		erreurCompte(w, err)
		return
	}
	s.noterCompte(c, "jeton d'API rendu par son poste")
	s.acces.noter(r, "jeton_rendu", c.Nom)
	repondre(w, 200, map[string]any{"supprime": true})
}

// joueursAdmis : GET /api/joueurs/comptes, les comptes que l'administrateur a admis comme joueurs
// de cet univers, joints ou non par le MJ, avec leur PJ.
func (s *Serveur) joueursAdmis(w http.ResponseWriter, r *http.Request) {
	id := s.store.ID()
	out := []map[string]any{}
	for _, c := range s.acces.comptes.Liste() {
		if c.PeutJouer(id) {
			out = append(out, map[string]any{"nom": c.Nom, "joint": c.Role(id) == comptes.Joueur, "pj": c.Persos[id],
				"inactif": c.Inactif, "autre_role": c.Role(id) != "" && c.Role(id) != comptes.Joueur})
		}
	}
	repondre(w, 200, out)
}

// joindreJoueur : PATCH /api/joueurs/comptes/{nom} {"joint"?: true|false, "pj"?: <fiche>} — le MJ
// joint à son univers (ou en retire) un compte que l'administrateur y a admis, et lui donne son PJ
// (0 : aucun). Il ne touche à rien d'autre du compte, ni aux autres univers.
func (s *Serveur) joindreJoueur(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Joint *bool  `json:"joint"`
		PJ    *int64 `json:"pj"`
	}
	if err := lireJSON(r, &in); err != nil {
		erreurStore(w, err)
		return
	}
	if in.PJ != nil && *in.PJ != 0 {
		if t, ok := s.store.TypeDeFiche(*in.PJ); !ok || t != "pj" {
			erreur(w, 422, "Cette fiche n'est pas un PJ de cet univers.")
			return
		}
	}
	id, nom := s.store.ID(), r.PathValue("nom")
	if c, ok := s.acces.comptes.Lire(nom); !ok || !c.PeutJouer(id) { // inconnu ou non admis : la même réponse
		erreur(w, 404, "Compte inconnu, ou que l'administrateur n'a pas admis comme joueur de cet univers.")
		return
	}
	c, err := s.acces.comptes.Modifier(nom, func(x *comptes.Compte) error {
		if role := x.Roles[id]; role != "" && role != comptes.Joueur {
			return errAutreRole
		}
		if in.Joint != nil {
			x.Roles[id] = map[bool]string{true: comptes.Joueur, false: ""}[*in.Joint]
		}
		if in.PJ != nil {
			if x.Roles[id] != comptes.Joueur && *in.PJ != 0 {
				return errNonJoint
			}
			x.Persos[id] = *in.PJ
		}
		return nil
	})
	switch {
	case errors.Is(err, errAutreRole), errors.Is(err, errNonJoint):
		erreur(w, 409, majuscule(err.Error())+".")
		return
	case err != nil:
		erreurCompte(w, err)
		return
	}
	s.store.Noter("compte", "joueur « "+c.Nom+" » "+map[bool]string{true: "joint à l'univers", false: "hors de l'univers"}[c.Role(id) == comptes.Joueur], auteurDe(r))
	repondre(w, 200, map[string]any{"nom": c.Nom, "joint": c.Role(id) == comptes.Joueur, "pj": c.Persos[id], "inactif": c.Inactif})
}

var (
	errAutreRole = errors.New("ce compte a déjà un autre rôle dans cet univers (donné par l'administrateur)")
	errNonJoint  = errors.New("joignez d'abord ce compte à l'univers")
)
