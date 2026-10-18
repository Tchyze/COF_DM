package serveur

import (
	"context"
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Tchyze/cof_dm/internal/comptes"
	qrcode "github.com/skip2/go-qrcode"
)

// Accès à l'interface depuis un autre appareil (tablette du MJ), comme dans tracker_COF.
//
// Rien ne change par défaut : l'application n'écoute que sur l'ordinateur du MJ. L'ouverture au
// réseau local est un choix (Paramètres ou -reseau) ; l'interface y demande alors le mot de passe
// MJ, qui ne se définit que depuis l'ordinateur du MJ. Sans mot de passe, aucun navigateur du
// réseau n'entre. Le mot de passe est gardé haché (PBKDF2-SHA256) ; la session est un cookie signé.

const (
	cookieMJ     = "cof_dm_mj"
	dureeSession = 30 * 24 * time.Hour
	iterations   = 210000
	metaMdp      = "mj_mdp"
	metaSecret   = "mj_secret"
	metaFermees  = "mj_fermees" // sessions fermées avant leur terme : {"identifiant": échéance}
	mdpMin       = 8
	essaisMax    = 8 // essais ratés par adresse…
	essaisDuree  = 5 * time.Minute
)

type acces struct {
	reg     Reglages
	comptes *comptes.Registre // mode serveur : nil sinon
	mu      sync.Mutex
	echecs  map[string][]time.Time // limitation des essais par adresse
	secret  []byte
	fermees map[string]int64 // sessions fermées (déconnexion) : identifiant → échéance
	pause   time.Duration    // attente après un mot de passe faux
	hachage sync.Mutex       // un seul calcul de mot de passe à la fois
}

func nouvelAcces(reg Reglages, c *comptes.Registre) *acces {
	a := &acces{reg: reg, comptes: c, echecs: map[string][]time.Time{}, fermees: map[string]int64{}, pause: 400 * time.Millisecond}
	if v, ok := reg.Meta(metaSecret); ok {
		a.secret, _ = hex.DecodeString(v)
	}
	if v, ok := reg.Meta(metaFermees); ok {
		_ = json.Unmarshal([]byte(v), &a.fermees)
	}
	if len(a.secret) < 32 {
		_ = a.nouveauSecret()
	}
	return a
}

// nouveauSecret : les sessions ouvertes ne valent plus rien.
func (a *acces) nouveauSecret() error {
	secret := make([]byte, 32)
	_, _ = rand.Read(secret)
	a.mu.Lock()
	a.secret = secret
	a.fermees = map[string]int64{} // signées avec l'ancien secret : déjà sans valeur
	a.mu.Unlock()
	_ = a.reg.SetMeta(metaFermees, "")
	return a.reg.SetMeta(metaSecret, hex.EncodeToString(secret))
}

func hacher(mdp string, sel []byte) []byte {
	k, _ := pbkdf2.Key(sha256.New, mdp, sel, iterations, 32)
	return k
}

func (a *acces) mdpDefini() bool {
	v, ok := a.reg.Meta(metaMdp)
	return ok && v != ""
}

// definirMdp enregistre le mot de passe (sel aléatoire + PBKDF2-SHA256) ; vide : il est retiré.
// Dans les deux cas, les appareils connectés sont déconnectés.
func (a *acces) definirMdp(mdp string) error {
	v := ""
	if mdp != "" {
		sel := make([]byte, 16)
		_, _ = rand.Read(sel)
		v = hex.EncodeToString(sel) + ":" + hex.EncodeToString(hacher(mdp, sel))
	}
	if err := a.reg.SetMeta(metaMdp, v); err != nil {
		return err
	}
	return a.nouveauSecret()
}

// verifier compare le mot de passe. Le calcul est coûteux exprès : il n'y en a qu'un à la fois, pour
// que des essais envoyés en rafale n'occupent pas tout l'ordinateur.
func (a *acces) verifier(mdp string) bool {
	a.hachage.Lock()
	defer a.hachage.Unlock()
	v, _ := a.reg.Meta(metaMdp)
	sel, attendu, trouve := strings.Cut(v, ":")
	if !trouve {
		return false
	}
	s, err1 := hex.DecodeString(sel)
	h, err2 := hex.DecodeString(attendu)
	if err1 != nil || err2 != nil {
		return false
	}
	return subtle.ConstantTimeCompare(hacher(mdp, s), h) == 1
}

// jeton de session : « échéance.identifiant.signature ». L'identifiant, tiré au hasard, permet de
// fermer cette session-là à la déconnexion.
func (a *acces) jeton(exp time.Time) string {
	id := make([]byte, 9)
	_, _ = rand.Read(id)
	return a.signer(strconv.FormatInt(exp.Unix(), 10) + "." + base64.RawURLEncoding.EncodeToString(id))
}

func (a *acces) signer(contenu string) string {
	a.mu.Lock()
	m := hmac.New(sha256.New, a.secret)
	a.mu.Unlock()
	m.Write([]byte(contenu))
	return contenu + "." + base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// lireSession vérifie un jeton de session (signature, échéance) et renvoie son identifiant, son
// échéance et ce qu'il porte en plus (mode serveur : le nom du compte et sa version). Une session
// fermée reste « authentique » : voir ouverte.
func (a *acces) lireSession(v string) (id string, exp int64, plus []string, ok bool) {
	p := strings.Split(v, ".")
	if len(p) < 3 {
		return "", 0, nil, false
	}
	n, err := strconv.ParseInt(p[0], 10, 64)
	contenu := strings.Join(p[:len(p)-1], ".")
	if err != nil || time.Now().Unix() > n || !hmac.Equal([]byte(a.signer(contenu)), []byte(v)) {
		return "", 0, nil, false
	}
	return p[1], n, p[2 : len(p)-1], true
}

func (a *acces) ouverte(id string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	_, fermee := a.fermees[id]
	return !fermee
}

func (a *acces) jetonValide(v string) bool {
	id, _, plus, ok := a.lireSession(v)
	return ok && len(plus) == 0 && a.ouverte(id)
}

// fermer met fin à une session : son jeton ne vaut plus rien, même recopié ailleurs.
func (a *acces) fermer(v string) {
	id, exp, _, ok := a.lireSession(v)
	if !ok {
		return
	}
	a.mu.Lock()
	maintenant := time.Now().Unix()
	for k, e := range a.fermees {
		if e < maintenant {
			delete(a.fermees, k)
		}
	}
	a.fermees[id] = exp
	b, _ := json.Marshal(a.fermees)
	a.mu.Unlock()
	_ = a.reg.SetMeta(metaFermees, string(b))
}

// session : l'appareil s'est connecté avec le mot de passe MJ (qui existe toujours).
func (a *acces) session(r *http.Request) bool {
	c, err := r.Cookie(cookieMJ)
	return err == nil && a.mdpDefini() && a.jetonValide(c.Value)
}

func adresseDe(r *http.Request) string {
	hote, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return hote
}

// reserverEssai compte un essai pour cette adresse avant de vérifier le mot de passe, pour que des
// essais envoyés en même temps ne passent pas tous sous la limite. Renvoie false quand elle est
// atteinte. Au passage, les essais périmés de toutes les adresses sont oubliés.
func (a *acces) reserverEssai(ip string, limite int) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	for k, l := range a.echecs {
		recents := l[:0]
		for _, t := range l {
			if time.Since(t) < essaisDuree {
				recents = append(recents, t)
			}
		}
		if len(recents) == 0 {
			delete(a.echecs, k)
		} else {
			a.echecs[k] = recents
		}
	}
	if len(a.echecs[ip]) >= limite {
		return false
	}
	a.echecs[ip] = append(a.echecs[ip], time.Now())
	return true
}

// rendreEssai : le mot de passe était bon, l'essai ne compte pas.
func (a *acces) rendreEssai(ip string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if l := a.echecs[ip]; len(l) > 1 {
		a.echecs[ip] = l[:len(l)-1]
	} else {
		delete(a.echecs, ip)
	}
}

// temoin : le cookie de session. Il n'est renvoyé qu'en HTTPS quand la page est servie ainsi.
func (a *acces) temoin(r *http.Request, valeur string, exp time.Time) *http.Cookie {
	c := &http.Cookie{Name: cookieMJ, Value: valeur, Path: "/", Expires: exp, HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: a.https(r)}
	if valeur == "" {
		c.MaxAge = -1
	}
	return c
}

// ---------------------------------------------------------------- qui demande

type cleSession struct{}
type cleAnonyme struct{}

// parSession : la requête vient d'un appareil du réseau connecté avec le mot de passe MJ.
func parSession(r *http.Request) bool {
	v, _ := r.Context().Value(cleSession{}).(bool)
	return v
}

// anonyme : un navigateur du réseau qui n'est pas connecté (il n'atteint que ce qui est « ouvert »).
func anonyme(r *http.Request) bool {
	v, _ := r.Context().Value(cleAnonyme{}).(bool)
	return v
}

// hoteDuReseau : la page est demandée par une adresse IP ou un nom du réseau local (« pc-du-mj »,
// « pc-du-mj.local »). Un nom de domaine d'internet est refusé : c'est ainsi qu'une page web hostile
// tenterait de parler à l'application à travers le navigateur d'un appareil du réseau.
func hoteDuReseau(r *http.Request) bool {
	hote := r.Host
	if h, _, err := net.SplitHostPort(hote); err == nil {
		hote = h
	}
	hote = strings.ToLower(strings.Trim(hote, "[]"))
	return hote != "" && (net.ParseIP(hote) != nil || !strings.Contains(hote, ".") || strings.HasSuffix(hote, ".local"))
}

// ouvert : ce qu'un navigateur du réseau reçoit sans être connecté. Les fichiers de l'interface
// (pour afficher la page de connexion ; les données restent derrière l'API) et la connexion.
func ouvert(r *http.Request) bool {
	chemin := r.URL.Path
	if !strings.HasPrefix(chemin, "/api/") && !strings.HasPrefix(chemin, "/medias/") {
		return r.Method == http.MethodGet || r.Method == http.MethodHead
	}
	return (r.Method == http.MethodGet && chemin == "/api/acces") || (r.Method == http.MethodPost && chemin == "/api/acces/connexion")
}

// surPlace réserve à l'ordinateur du MJ les réglages qui gardent la porte : le mot de passe,
// l'ouverture au réseau, le jeton d'API. Depuis le réseau, ni la session ni le
// jeton ne les ouvrent.
//
// En mode serveur, il n'y a pas d'« ordinateur du MJ » : ces réglages reviennent à l'administrateur.
func surPlace(w http.ResponseWriter, r *http.Request) bool {
	if modeServeur(r) {
		if c, ok := compteDe(r); !ok || !c.Admin {
			erreur(w, 403, "Réservé à l'administrateur.")
			return false
		}
		return true
	}
	if !local(r) {
		erreur(w, 403, "Ce réglage se fait sur l'ordinateur du MJ.")
		return false
	}
	return true
}

// ---------------------------------------------------------------- routes

func (s *Serveur) routesAcces() {
	m := s.mux
	m.HandleFunc("POST /api/acces/deconnexion", s.deconnexion)
	if s.acces.comptes != nil {
		s.routesComptes()
	} else {
		m.HandleFunc("GET /api/acces", s.etatAcces)
		m.HandleFunc("POST /api/acces/connexion", s.connexion)
		m.HandleFunc("POST /api/acces/mdp", s.changerMdp)
		m.HandleFunc("POST /api/acces/reseau", s.changerReseau)
	}
	m.HandleFunc("GET /api/acces/qr", s.qr)
}

// Adresses renvoie les adresses de l'application sur le réseau local (IPv4 privées d'abord).
func Adresses(port int) []string {
	var privees, autres []string
	ifaces, _ := net.Interfaces()
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok || ipn.IP.To4() == nil {
				continue
			}
			u := fmt.Sprintf("http://%s:%d", ipn.IP.String(), port)
			if ipn.IP.IsPrivate() {
				privees = append(privees, u)
			} else if !ipn.IP.IsLinkLocalUnicast() {
				autres = append(autres, u)
			}
		}
	}
	return append(privees, autres...)
}

// etatAcces : qui est en face. Un inconnu n'apprend que s'il peut se connecter ; le MJ voit aussi
// l'état du réseau.
func (s *Serveur) etatAcces(w http.ResponseWriter, r *http.Request) {
	mj := !anonyme(r)
	out := map[string]any{"mj": mj, "local": local(r), "mdp_defini": s.acces.mdpDefini()}
	if mj {
		out["reseau"] = s.opt.Reseau               // l'application écoute sur le réseau en ce moment
		out["reseau_voulu"] = s.reseauVoulu.Load() // réglage enregistré, appliqué au prochain lancement
		out["reseau_force"] = s.opt.ReseauForce    // imposé par -reseau
		out["reglable"] = s.opt.Config != nil      // le réglage peut s'enregistrer
		out["adresses"] = Adresses(s.opt.Port)     // à ouvrir sur la tablette
		out["page_joueurs"] = s.pageJoueursOuverte()
		out["relie"] = s.relie() // reliée à un serveur : le réseau local reste fermé
		out["edition_joueurs"] = s.editionJoueurs()
	}
	repondre(w, 200, out)
}

func (s *Serveur) connexion(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Mdp string `json:"mdp"`
	}
	_ = json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&req)
	ip := adresseDe(r)
	if !s.acces.mdpDefini() {
		erreur(w, 409, "Aucun mot de passe MJ n'est défini : créez-le sur l'ordinateur du MJ, dans Paramètres.")
		return
	}
	if !s.acces.reserverEssai(ip, essaisMax) {
		erreur(w, 429, "Trop d'essais : patientez quelques minutes.")
		return
	}
	if !s.acces.verifier(req.Mdp) {
		time.Sleep(s.acces.pause)
		erreur(w, 401, "Mot de passe incorrect.")
		return
	}
	s.acces.rendreEssai(ip)
	exp := time.Now().Add(dureeSession)
	http.SetCookie(w, s.acces.temoin(r, s.acces.jeton(exp), exp))
	repondre(w, 200, map[string]bool{"mj": true})
}

// deconnexion ferme la session de l'appareil : son témoin est effacé et ne sera plus accepté.
func (s *Serveur) deconnexion(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieMJ); err == nil {
		s.acces.fermer(c.Value)
	}
	http.SetCookie(w, s.acces.temoin(r, "", time.Unix(0, 0)))
	repondre(w, 200, map[string]bool{"ok": true})
}

// changerMdp : {"nouveau": "…"} définit ou change le mot de passe MJ ; "" le retire.
func (s *Serveur) changerMdp(w http.ResponseWriter, r *http.Request) {
	if !surPlace(w, r) {
		return
	}
	var req struct {
		Nouveau string `json:"nouveau"`
	}
	if err := lireJSON(r, &req); err != nil {
		erreurStore(w, err)
		return
	}
	if n := len([]rune(req.Nouveau)); n > 0 && n < mdpMin {
		erreur(w, 422, fmt.Sprintf("Le mot de passe doit faire au moins %d caractères.", mdpMin))
		return
	}
	if err := s.acces.definirMdp(req.Nouveau); err != nil {
		erreurStore(w, err)
		return
	}
	detail := "mot de passe MJ défini (les appareils connectés sont déconnectés)"
	if req.Nouveau == "" {
		detail = "mot de passe MJ retiré (plus aucun appareil du réseau n'entre)"
	}
	s.store.Noter("accès", detail, auteurDe(r))
	repondre(w, 200, map[string]any{"ok": true, "mdp_defini": req.Nouveau != ""})
}

// changerReseau : {"reseau": true} ouvre l'application au réseau local au prochain lancement.
func (s *Serveur) changerReseau(w http.ResponseWriter, r *http.Request) {
	if !surPlace(w, r) {
		return
	}
	var req struct {
		Reseau bool `json:"reseau"`
	}
	if err := lireJSON(r, &req); err != nil {
		erreurStore(w, err)
		return
	}
	if s.opt.Config == nil {
		erreur(w, 409, "Ce réglage ne peut pas s'enregistrer ici.")
		return
	}
	if req.Reseau && s.relie() {
		erreur(w, 409, msgReseauRelie)
		return
	}
	if err := s.opt.Config(req.Reseau); err != nil {
		erreur(w, 500, "Enregistrement impossible : "+err.Error())
		return
	}
	s.reseauVoulu.Store(req.Reseau)
	detail := "ouverture au réseau local activée"
	if !req.Reseau {
		detail = "ouverture au réseau local désactivée"
	}
	s.store.Noter("accès", detail+" (au prochain lancement)", auteurDe(r))
	s.etatAcces(w, r)
}

// qr : le QR code d'une adresse de l'application, à scanner avec la tablette.
func (s *Serveur) qr(w http.ResponseWriter, r *http.Request) {
	u := r.URL.Query().Get("url")
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") || len(u) > 200 {
		erreur(w, 400, "Adresse invalide.")
		return
	}
	png, err := qrcode.Encode(u, qrcode.Medium, 320)
	if err != nil {
		erreur(w, 500, "QR code impossible à générer.")
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "max-age=3600")
	_, _ = w.Write(png)
}

func avecSession(r *http.Request) *http.Request {
	return r.WithContext(context.WithValue(contexte(r, "mj (réseau)"), cleSession{}, true))
}

func sansSession(r *http.Request) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), cleAnonyme{}, true))
}
