package serveur

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Tchyze/cof_dm/internal/comptes"
	qrcode "github.com/skip2/go-qrcode"
)

// Second facteur des administrateurs (mode serveur). Le mot de passe seul n'ouvre aucune session :
//
//   - l'administrateur qui a un facteur reçoit une preuve, cookie signé de cinq minutes qui n'ouvre que
//     POST /api/acces/second-facteur ; la session vient ensuite, et la preuve ne ressert pas ;
//   - celui qui n'en a aucun reçoit une session d'enrôlement, qui n'ouvre que son compte (voir
//     ouvertEnEnrolement), le temps d'en enregistrer un. Le mot de passe initial passe avant.
//
// La session d'un administrateur porte son niveau : une session sans niveau (d'avant cette version, ou
// d'un compte devenu administrateur depuis) ne vaut rien. Celle des autres comptes ne change pas.
//
// Personne ne bloque le titulaire : sans la preuve, donc sans le mot de passe, on n'atteint aucun des
// compteurs de cette étape. Celui qui a le mot de passe peut saturer le quota du TOTP par compte, mais pas
// les codes de secours, qui ne se devinent pas. Les limites sont en mémoire, comme celles du mot de passe.

const (
	cookiePreuve         = "cof_dm_2f"
	routeSecondFacteur   = "/api/acces/second-facteur"
	prefixeSecondFacteur = "/api/compte/second-facteur/"

	dureePreuve      = 5 * time.Minute
	dureeEnrolement  = 30 * time.Minute
	dureeAttenteTOTP = 10 * time.Minute

	niveauComplet    = "2f"
	niveauEnrolement = "enrol"

	facteurTOTP    = "totp"
	facteurSecours = "secours"

	essaisMaxPreuve       = 5  // erreurs par preuve : au-delà, retour au mot de passe
	essaisMax2FCouple     = 5  // par compte et par adresse, tous facteurs
	essaisMax2FAdresse    = 20 // par adresse, tous comptes et tous facteurs
	essaisMaxTOTPCompte   = 10 // par compte, d'où qu'ils viennent : TOTP seulement
	essaisMaxTOTPJour     = 50 // idem, sur une journée : un attaquant réparti reste sous 0,02 % par jour
	dureeTOTPJour         = 24 * time.Hour
	essaisMaxConfirmer    = essaisMaxMdp
	libelleEmetteurTOTP   = "COF DM"
	tailleQRSecondFacteur = 256
)

type cleNiveau struct{}

// niveauSession : le niveau de la session d'un administrateur ("" pour tout autre compte).
func niveauSession(r *http.Request) string {
	v, _ := r.Context().Value(cleNiveau{}).(string)
	return v
}

// ouvertEnEnrolement : ce qu'atteint une session d'enrôlement. Tout le reste est refusé, y compris ce
// qui sera ajouté plus tard : TestEnrolementFerme essaie toutes les routes déclarées.
func ouvertEnEnrolement(r *http.Request) bool {
	chemin := r.URL.Path
	if !strings.HasPrefix(chemin, "/api/") && !strings.HasPrefix(chemin, "/medias/") {
		return true
	}
	switch r.Method + " " + chemin {
	case "GET /api/acces", "POST /api/acces/connexion", "POST /api/acces/deconnexion", "POST " + routeSecondFacteur,
		"GET /api/compte", "POST /api/compte/mdp", "GET /api/sante":
		return true
	}
	return strings.HasPrefix(chemin, prefixeSecondFacteur)
}

// ---------------------------------------------------------------- preuve du mot de passe

// signerPreuve : la clé des preuves n'est pas celle des sessions, tirée au démarrage et jamais écrite :
// une preuve n'ouvre aucune session, une session ne passe pas pour une preuve, et un redémarrage les
// annule toutes.
func (a *acces) signerPreuve(contenu string) string {
	m := hmac.New(sha256.New, a.clePreuve)
	m.Write([]byte(contenu))
	return contenu + "." + base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// preuve : « échéance.identifiant.nom.version.graine.signature ».
func (a *acces) preuve(c comptes.Compte, exp time.Time) string {
	id := make([]byte, 9)
	_, _ = rand.Read(id)
	return a.signerPreuve(strings.Join([]string{strconv.FormatInt(exp.Unix(), 10), base64.RawURLEncoding.EncodeToString(id), c.Nom, strconv.Itoa(c.Version), c.Graine}, "."))
}

// lirePreuve : le compte que cette preuve désigne, tant qu'elle n'a pas expiré ni servi, et que le compte
// est toujours le même (version, graine) et a toujours un facteur à donner.
func (a *acces) lirePreuve(v string) (c comptes.Compte, id string, exp int64, ok bool) {
	p := strings.Split(v, ".")
	if len(p) != 6 || !hmac.Equal([]byte(a.signerPreuve(strings.Join(p[:5], "."))), []byte(v)) {
		return comptes.Compte{}, "", 0, false
	}
	n, err := strconv.ParseInt(p[0], 10, 64)
	if err != nil || time.Now().Unix() > n {
		return comptes.Compte{}, "", 0, false
	}
	a.mu.Lock()
	_, servie := a.preuvesServies[p[1]]
	a.mu.Unlock()
	if servie {
		return comptes.Compte{}, "", 0, false
	}
	c, ok = a.comptes.Lire(p[2])
	if !ok || c.Inactif || !c.Admin || !c.AFacteur() || strconv.Itoa(c.Version) != p[3] || c.Graine != p[4] {
		return comptes.Compte{}, "", 0, false
	}
	return c, p[1], n, true
}

// consommerPreuve : false si elle a déjà servi, même à l'instant, dans une autre requête.
func (a *acces) consommerPreuve(id string, exp int64) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	t := time.Now().Unix()
	for k, e := range a.preuvesServies {
		if e < t {
			delete(a.preuvesServies, k)
		}
	}
	if _, servie := a.preuvesServies[id]; servie {
		return false
	}
	a.preuvesServies[id] = exp
	return true
}

// temoinPreuve : le cookie de la preuve, envoyé seulement à la route de cette étape.
func (a *acces) temoinPreuve(r *http.Request, valeur string, exp time.Time) *http.Cookie {
	c := &http.Cookie{Name: cookiePreuve, Value: valeur, Path: routeSecondFacteur, Expires: exp, HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: a.https(r)}
	if valeur == "" {
		c.MaxAge = -1
	}
	return c
}

// facteursDe : ce que ce compte peut donner à l'étape du second facteur.
func facteursDe(c comptes.Compte) []string {
	l := []string{}
	if c.AFacteur() {
		l = append(l, facteurTOTP)
	}
	if c.SecoursRestants() > 0 {
		l = append(l, facteurSecours)
	}
	return l
}

// suiteDuMdp : le mot de passe d'un administrateur est bon. Ni session complète ni rôle pour autant.
func (s *Serveur) suiteDuMdp(w http.ResponseWriter, r *http.Request, c comptes.Compte) {
	if c.AFacteur() {
		exp := time.Now().Add(dureePreuve)
		http.SetCookie(w, s.acces.temoinPreuve(r, s.acces.preuve(c, exp), exp))
		s.acces.noter(r, "second_facteur_demande", c.Nom)
		repondre(w, 200, map[string]any{"mj": false, "etape": "second_facteur", "facteurs": facteursDe(c)})
		return
	}
	exp := time.Now().Add(dureeEnrolement)
	http.SetCookie(w, s.acces.temoin(r, s.acces.sessionCompte(c, niveauEnrolement, exp), exp))
	s.acces.noter(r, "enrolement_ouvert", c.Nom)
	repondre(w, 200, map[string]any{"mj": false, "etape": "enrolement", "compte": s.vueCompte(c)})
}

// ---------------------------------------------------------------- donner un facteur

// essayerFacteur vérifie un code de ce compte, après l'avoir compté. En cas de refus, la réponse est
// déjà écrite. Jamais le code au journal ni dans une réponse.
func (a *acces) essayerFacteur(w http.ResponseWriter, r *http.Request, c comptes.Compte, facteur, code string) bool {
	if facteur != facteurTOTP && facteur != facteurSecours {
		erreur(w, 422, "Second facteur inconnu.")
		return false
	}
	adresse := cleAdresse(a.adresse(r))
	parIP, parCouple := "2f-ip:"+adresse, "2f-couple:"+c.Nom+"|"+adresse
	parCompte, parJour := "2f-compte:"+c.Nom, "2f-jour:"+c.Nom
	if !a.reserverEssai(parIP, essaisMax2FAdresse) {
		a.noter(r, "limite_atteinte", c.Nom, "portee=2f_adresse")
		erreur(w, 429, "Trop d'essais : patientez quelques minutes.")
		return false
	}
	if !a.reserverEssai(parCouple, essaisMax2FCouple) {
		a.rendreEssai(parIP)
		a.noter(r, "limite_atteinte", c.Nom, "portee=2f_couple")
		erreur(w, 429, "Trop d'essais : patientez quelques minutes.")
		return false
	}
	var err error
	restants := 0
	switch facteur {
	case facteurTOTP:
		sur5min := a.reserverEssai(parCompte, essaisMaxTOTPCompte)
		if !sur5min || !a.reserverEssaiSur(parJour, essaisMaxTOTPJour, dureeTOTPJour) {
			if sur5min {
				a.rendreEssai(parCompte)
			}
			a.rendreEssai(parIP)
			a.rendreEssai(parCouple)
			a.noter(r, "limite_atteinte", c.Nom, "portee=2f_compte")
			erreur(w, 429, "Trop d'essais de code sur ce compte : utilisez un code de secours, ou patientez.")
			return false
		}
		err = a.comptes.VerifierTOTP(c.Nom, code)
	case facteurSecours:
		restants, err = a.comptes.UtiliserSecours(c.Nom, code)
	}
	if err != nil {
		if !errors.Is(err, comptes.ErrCode) {
			log.Printf("second facteur de %q : %v", c.Nom, err)
		}
		a.noter(r, "second_facteur_echec", c.Nom, "facteur="+facteur)
		time.Sleep(a.pause)
		if facteur == facteurTOTP {
			erreur(w, 401, "Code incorrect, ou déjà utilisé : attendez le suivant.")
		} else {
			erreur(w, 401, "Code de secours incorrect, ou déjà utilisé.")
		}
		return false
	}
	a.rendreEssai(parIP)
	a.rendreEssai(parCouple)
	a.oublierEssais(parCompte)
	a.oublierEssais(parJour)
	if facteur == facteurSecours {
		a.noter(r, "secours_utilise", c.Nom, "restants="+strconv.Itoa(restants))
	}
	return true
}

// connexionSecondFacteur : POST /api/acces/second-facteur {"facteur", "code"}, avec la preuve. Sans
// preuve valable, toujours la même réponse, et aucun compteur n'est touché.
func (s *Serveur) connexionSecondFacteur(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Facteur string `json:"facteur"`
		Code    string `json:"code"`
	}
	_ = json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&req)
	a := s.acces
	const expiree = "Cette étape a expiré : reprenez la connexion."
	t, err := r.Cookie(cookiePreuve)
	if err != nil {
		erreur(w, 401, expiree)
		return
	}
	c, id, exp, ok := a.lirePreuve(t.Value)
	if !ok {
		http.SetCookie(w, a.temoinPreuve(r, "", time.Unix(0, 0)))
		erreur(w, 401, expiree)
		return
	}
	if !a.reserverEssai("2f-preuve:"+id, essaisMaxPreuve) {
		a.consommerPreuve(id, exp)
		http.SetCookie(w, a.temoinPreuve(r, "", time.Unix(0, 0)))
		a.noter(r, "limite_atteinte", c.Nom, "portee=2f_preuve")
		erreur(w, 401, expiree)
		return
	}
	if !a.essayerFacteur(w, r, c, req.Facteur, req.Code) {
		return
	}
	http.SetCookie(w, a.temoinPreuve(r, "", time.Unix(0, 0)))
	if !a.consommerPreuve(id, exp) {
		erreur(w, 401, expiree)
		return
	}
	a.oublierEssais("2f-preuve:" + id)
	c, ok = a.comptes.Lire(c.Nom)
	if !ok {
		erreur(w, 401, expiree)
		return
	}
	a.noter(r, "connexion_ok", c.Nom, "facteur="+req.Facteur)
	fin := time.Now().Add(dureeSessionAdmin)
	http.SetCookie(w, a.temoin(r, a.sessionCompte(c, niveauComplet, fin), fin))
	repondre(w, 200, map[string]any{"mj": false, "compte": s.vueCompte(c)})
}

// ---------------------------------------------------------------- confirmer une action

// confirmation : ce que demande une action sensible, dans sa requête même. Sans état à garder, et une
// session volée ne l'a pas.
type confirmation struct {
	Mdp     string `json:"mdp"`
	Facteur string `json:"facteur"`
	Code    string `json:"code"`
}

// confirmer vérifie le mot de passe et, si le compte en a un, un second facteur. Pendant l'enrôlement,
// il n'y a encore que le mot de passe à donner. En cas de refus, la réponse est déjà écrite.
func (a *acces) confirmer(w http.ResponseWriter, r *http.Request, c comptes.Compte, conf *confirmation) bool {
	if conf == nil {
		repondre(w, 403, map[string]any{"detail": "Confirmez cette action avec votre mot de passe et un second facteur.", "confirmation_requise": true})
		return false
	}
	cle := "confirmer:" + c.Nom // pas les compteurs de la connexion : un anonyme n'y touche pas, ni l'inverse
	if !a.reserverEssai(cle, essaisMaxConfirmer) {
		a.noter(r, "limite_atteinte", c.Nom, "portee=confirmation")
		erreur(w, 429, "Trop d'essais : patientez quelques minutes.")
		return false
	}
	a.hachage.Lock()
	_, ok := a.comptes.Verifier(c.Nom, conf.Mdp)
	a.hachage.Unlock()
	if !ok {
		a.noter(r, "confirmation_echec", c.Nom)
		time.Sleep(a.pause)
		erreur(w, 401, "Mot de passe incorrect.")
		return false
	}
	a.rendreEssai(cle)
	c, ok = a.comptes.Lire(c.Nom)
	if !ok {
		erreur(w, 401, "Connectez-vous avec votre compte.")
		return false
	}
	return !c.AFacteur() || a.essayerFacteur(w, r, c, conf.Facteur, conf.Code)
}

// ---------------------------------------------------------------- ses facteurs

func (s *Serveur) routesSecondFacteur() {
	m := s.mux
	m.HandleFunc("POST "+routeSecondFacteur, s.connexionSecondFacteur)
	m.HandleFunc("POST "+prefixeSecondFacteur+"totp/debut", s.debutTOTP)
	m.HandleFunc("POST "+prefixeSecondFacteur+"totp/activer", s.activerTOTP)
	m.HandleFunc("DELETE "+prefixeSecondFacteur+"totp", s.retirerTOTP)
	m.HandleFunc("POST "+prefixeSecondFacteur+"secours", s.regenererSecours)
}

// vueSecondFacteur : ce que le titulaire voit de ses facteurs. Jamais un secret.
func vueSecondFacteur(c comptes.Compte) map[string]any {
	v := map[string]any{"requis": !c.AFacteur(), "totp": c.AFacteur(), "secours_restants": c.SecoursRestants()}
	if c.AFacteur() {
		v["totp_active"] = c.SecondFacteur.TOTP.Active
	}
	return v
}

// adminDeSession : ces routes ne concernent qu'un administrateur, par sa session. Rien ne change pour
// les autres comptes.
func adminDeSession(w http.ResponseWriter, r *http.Request) (comptes.Compte, bool) {
	c, _ := compteDe(r)
	if !c.Admin || parJeton(r) {
		erreur(w, 403, "Le second facteur est réservé aux administrateurs, connectés par leur mot de passe.")
		return comptes.Compte{}, false
	}
	return c, true
}

// idSession : l'identifiant de la session de cette requête.
func (a *acces) idSession(r *http.Request) string {
	t, err := r.Cookie(cookieMJ)
	if err != nil {
		return ""
	}
	id, _, _, _ := a.lireSession(t.Value)
	return id
}

// totpEnAttente : un secret montré, pas encore prouvé. Il reste en mémoire : rien sur le disque tant que
// le compte n'a pas donné un code valable, et il ne vaut que pour cette session.
type totpEnAttente struct {
	secret  string
	session string
	version int
	exp     time.Time
}

// debutTOTP : {"confirmation": {…}} tire un secret et le montre, avec son QR code fait ici, cette seule
// fois. Rien n'est activé tant que POST …/totp/activer n'a pas reçu un code. Avec un TOTP déjà actif, le
// nouveau le remplacera : c'est ainsi qu'on change de téléphone.
func (s *Serveur) debutTOTP(w http.ResponseWriter, r *http.Request) {
	c, ok := adminDeSession(w, r)
	if !ok {
		return
	}
	var req struct {
		Confirmation *confirmation `json:"confirmation"`
	}
	if err := lireJSON(r, &req); err != nil {
		erreurStore(w, err)
		return
	}
	if !s.acces.confirmer(w, r, c, req.Confirmation) {
		return
	}
	secret := comptes.NouveauSecretTOTP()
	u := url.URL{Scheme: "otpauth", Host: "totp", Path: "/" + libelleEmetteurTOTP + ":" + c.Nom,
		RawQuery: url.Values{"secret": {secret}, "issuer": {libelleEmetteurTOTP}, "algorithm": {"SHA1"}, "digits": {"6"}, "period": {"30"}}.Encode()}
	png, err := qrcode.Encode(u.String(), qrcode.Medium, tailleQRSecondFacteur)
	if err != nil {
		erreur(w, 500, "QR code impossible à générer.")
		return
	}
	a := s.acces
	session := a.idSession(r) // avant le verrou : signer le prend
	a.mu.Lock()
	for k, e := range a.totpAttente {
		if time.Now().After(e.exp) {
			delete(a.totpAttente, k)
		}
	}
	a.totpAttente[c.Nom] = totpEnAttente{secret, session, c.Version, time.Now().Add(dureeAttenteTOTP)}
	a.mu.Unlock()
	repondre(w, 200, map[string]any{"secret": secret, "otpauth": u.String(), "qr": "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)})
}

// activerTOTP : {"code"} active le secret montré par debutTOTP si ce code en vient. Les codes de secours,
// s'il n'en restait aucun, sont dans la réponse, cette seule fois. Une session d'enrôlement devient une
// session complète, sous un autre identifiant : l'ancienne ne vaut plus rien.
func (s *Serveur) activerTOTP(w http.ResponseWriter, r *http.Request) {
	c, ok := adminDeSession(w, r)
	if !ok {
		return
	}
	var req struct {
		Code string `json:"code"`
	}
	if err := lireJSON(r, &req); err != nil {
		erreurStore(w, err)
		return
	}
	a := s.acces
	a.mu.Lock()
	e, ok := a.totpAttente[c.Nom]
	a.mu.Unlock()
	if !ok || time.Now().After(e.exp) || e.session != a.idSession(r) || e.version != c.Version {
		erreur(w, 409, "Recommencez l'enregistrement : ce secret n'est plus valable.")
		return
	}
	codes, err := a.comptes.ActiverTOTP(c.Nom, e.secret, req.Code)
	switch {
	case errors.Is(err, comptes.ErrCode):
		erreur(w, 422, "Code incorrect : vérifiez l'heure de votre appareil, et saisissez le code affiché.")
		return
	case err != nil:
		erreurCompte(w, err)
		return
	}
	a.mu.Lock()
	delete(a.totpAttente, c.Nom)
	a.mu.Unlock()
	a.noter(r, "second_facteur_active", c.Nom, "facteur="+facteurTOTP)
	if niveauSession(r) == niveauEnrolement {
		if t, err := r.Cookie(cookieMJ); err == nil {
			a.fermer(t.Value)
		}
		c, _ = a.comptes.Lire(c.Nom)
		fin := time.Now().Add(dureeSessionAdmin)
		http.SetCookie(w, a.temoin(r, a.sessionCompte(c, niveauComplet, fin), fin))
		a.noter(r, "connexion_ok", c.Nom, "facteur="+facteurTOTP)
	}
	out := map[string]any{"ok": true}
	if len(codes) > 0 {
		out["secours"] = codes
	}
	repondre(w, 200, out)
}

// retirerTOTP : {"confirmation": {…}}. Jamais le dernier facteur : 409, avant de consommer un code. Les
// autres sessions du compte tombent.
func (s *Serveur) retirerTOTP(w http.ResponseWriter, r *http.Request) {
	c, ok := adminDeSession(w, r)
	if !ok {
		return
	}
	var req struct {
		Confirmation *confirmation `json:"confirmation"`
	}
	if err := lireJSON(r, &req); err != nil {
		erreurStore(w, err)
		return
	}
	if !c.AFacteur() {
		erreur(w, 404, "Aucun TOTP sur ce compte.")
		return
	}
	if !c.PeutRetirerTOTP() {
		erreurCompte(w, comptes.ErrDernierFacteur)
		return
	}
	if !s.acces.confirmer(w, r, c, req.Confirmation) {
		return
	}
	if err := s.acces.comptes.RetirerTOTP(c.Nom); err != nil {
		erreurCompte(w, err)
		return
	}
	s.acces.noter(r, "second_facteur_retire", c.Nom, "facteur="+facteurTOTP)
	c, _ = s.acces.comptes.Lire(c.Nom)
	fin := time.Now().Add(dureeSessionAdmin)
	http.SetCookie(w, s.acces.temoin(r, s.acces.sessionCompte(c, niveauSession(r), fin), fin))
	repondre(w, 200, map[string]any{"ok": true})
}

// regenererSecours : {"confirmation": {…}} remplace tous les codes de secours, montrés cette seule fois.
func (s *Serveur) regenererSecours(w http.ResponseWriter, r *http.Request) {
	c, ok := adminDeSession(w, r)
	if !ok {
		return
	}
	var req struct {
		Confirmation *confirmation `json:"confirmation"`
	}
	if err := lireJSON(r, &req); err != nil {
		erreurStore(w, err)
		return
	}
	if !c.AFacteur() {
		erreurCompte(w, comptes.ErrSansFacteur)
		return
	}
	if !s.acces.confirmer(w, r, c, req.Confirmation) {
		return
	}
	codes, err := s.acces.comptes.RegenererSecours(c.Nom)
	if err != nil {
		erreurCompte(w, err)
		return
	}
	s.acces.noter(r, "secours_regeneres", c.Nom)
	repondre(w, 200, map[string]any{"secours": codes})
}
