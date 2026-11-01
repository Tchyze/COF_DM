package serveur

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/protocol/webauthncose"
	"github.com/go-webauthn/webauthn/webauthn"

	"github.com/Tchyze/cof_dm/internal/comptes"
)

// Clés de sécurité (WebAuthn, FIDO2), second facteur d'un administrateur. La bibliothèque vérifie
// l'origine, le domaine, le défi et la signature ; ici, le défi à usage unique et le refus d'un compteur
// incohérent, qu'elle ne fait que signaler.
//
// Origine et domaine viennent de la configuration (-origine, -rp-id), jamais d'un en-tête : derrière un
// proxy, Host peut perdre son port, et rien de ce qu'envoie le visiteur ne doit décider à quel site une
// clé répond. Une clé enregistrée reste liée à son domaine : s'il change, elle ne sert plus.

const (
	dureeDefi           = 2 * time.Minute
	tailleMaxReponseCle = 16 << 10
	nomDuSite           = "COF DM"
)

// ConfigCles : l'adresse publique du serveur, seule source de l'origine et du domaine.
type ConfigCles struct {
	wa      *webauthn.WebAuthn
	Origine string
	RPID    string
}

// LireConfigCles vérifie l'adresse publique du serveur. HTTPS, sauf sur cette machine (les navigateurs
// n'offrent pas WebAuthn autrement). rpID vide : l'hôte de l'origine. Sinon, cet hôte ou un domaine parent,
// ce qui permet de changer de sous-domaine sans perdre ses clés.
func LireConfigCles(origine, rpID string) (*ConfigCles, error) {
	u, err := url.Parse(strings.TrimSpace(origine))
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, errors.New("attendu sous la forme https://jdr.exemple.org, sans chemin")
	}
	hote := strings.ToLower(u.Hostname())
	switch {
	case u.Scheme == "https":
	case u.Scheme == "http" && hote == "localhost":
	default:
		return nil, errors.New("l'adresse doit être en https (http seulement pour localhost)")
	}
	if net.ParseIP(hote) != nil {
		return nil, errors.New("les clés de sécurité demandent un nom de domaine, pas une adresse IP")
	}
	rpID = strings.ToLower(strings.TrimSpace(rpID))
	if rpID == "" {
		rpID = hote
	}
	if rpID != hote && !strings.HasSuffix(hote, "."+rpID) {
		return nil, errors.New("-rp-id doit être l'hôte de -origine ou un de ses domaines parents")
	}
	if rpID != "localhost" && !strings.Contains(rpID, ".") {
		return nil, errors.New("-rp-id : un domaine complet, comme exemple.org")
	}
	origine = u.Scheme + "://" + strings.ToLower(u.Host)
	delai := webauthn.TimeoutConfig{Enforce: true, Timeout: dureeDefi, TimeoutUVD: dureeDefi}
	wa, err := webauthn.New(&webauthn.Config{RPID: rpID, RPDisplayName: nomDuSite, RPOrigins: []string{origine},
		Timeouts: webauthn.TimeoutsConfig{Login: delai, Registration: delai}})
	if err != nil {
		return nil, err
	}
	return &ConfigCles{wa: wa, Origine: origine, RPID: rpID}, nil
}

// utilisateurCle : un compte, tel que la bibliothèque le lit. Son identifiant ne dit rien du compte et ne
// sert à personne d'autre : tiré de sa graine.
type utilisateurCle struct{ c comptes.Compte }

func (u utilisateurCle) WebAuthnID() []byte {
	h := sha256.Sum256([]byte("cof-dm-webauthn:" + u.c.Graine))
	return h[:]
}
func (u utilisateurCle) WebAuthnName() string        { return u.c.Nom }
func (u utilisateurCle) WebAuthnDisplayName() string { return u.c.Nom }
func (u utilisateurCle) WebAuthnCredentials() []webauthn.Credential {
	if u.c.SecondFacteur == nil {
		return nil
	}
	l := make([]webauthn.Credential, 0, len(u.c.SecondFacteur.Cles))
	for _, k := range u.c.SecondFacteur.Cles {
		x := webauthn.Credential{ID: k.Identifiant, PublicKey: k.ClePublique, AttestationType: k.Type, AttestationFormat: k.Format,
			Flags:         webauthn.CredentialFlags{UserPresent: true, BackupEligible: k.Eligible, BackupState: k.Sauvegardee},
			Authenticator: webauthn.Authenticator{AAGUID: k.AAGUID, SignCount: k.Compteur}}
		for _, t := range k.Transports {
			x.Transport = append(x.Transport, protocol.AuthenticatorTransport(t))
		}
		l = append(l, x)
	}
	return l
}

// defiCle : un défi en attente de sa réponse, en mémoire, pour une seule réponse. Il attend sous la preuve
// ou la session qui l'a demandé : une autre n'y répond pas.
type defiCle struct {
	session webauthn.SessionData
	nom     string
	version int
	libelle string // à l'enregistrement : le nom donné à la clé
	exp     time.Time
}

func (a *acces) poserDefi(cle string, d defiCle) {
	d.exp = time.Now().Add(dureeDefi)
	a.mu.Lock()
	defer a.mu.Unlock()
	for k, e := range a.defis {
		if time.Now().After(e.exp) {
			delete(a.defis, k)
		}
	}
	a.defis[cle] = d // un nouveau défi remplace le précédent
}

// prendreDefi le retire : réussie ou non, une réponse ne le laisse pas derrière elle.
func (a *acces) prendreDefi(cle string) (defiCle, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	d, ok := a.defis[cle]
	delete(a.defis, cle)
	if !ok || time.Now().After(d.exp) {
		return defiCle{}, false
	}
	return d, true
}

// optionsPreuve : le défi à présenter aux clés de ce compte, rangé sous cleDefi.
func (a *acces) optionsPreuve(c comptes.Compte, cleDefi string) (*protocol.PublicKeyCredentialRequestOptions, error) {
	o, session, err := a.cles.wa.BeginLogin(utilisateurCle{c}, webauthn.WithUserVerification(protocol.VerificationDiscouraged),
		webauthn.WithAssertionPublicKeyCredentialHints([]protocol.PublicKeyCredentialHints{protocol.PublicKeyCredentialHintSecurityKey}))
	if err != nil {
		return nil, err
	}
	a.poserDefi(cleDefi, defiCle{session: *session, nom: c.Nom, version: c.Version})
	return &o.Response, nil
}

// verifierCle : la réponse d'une clé au défi rangé sous cleDefi. L'erreur de la bibliothèque n'est ni
// montrée ni écrite : elle peut citer le défi.
func (a *acces) verifierCle(c comptes.Compte, cleDefi string, reponse json.RawMessage) error {
	d, ok := a.prendreDefi(cleDefi)
	if !ok || d.nom != c.Nom || d.version != c.Version {
		return comptes.ErrCode
	}
	lu, err := protocol.ParseCredentialRequestResponseBytes(reponse)
	if err != nil {
		return comptes.ErrCode
	}
	x, err := a.cles.wa.ValidateLogin(utilisateurCle{c}, d.session, lu)
	if err != nil {
		return comptes.ErrCode
	}
	if x.Authenticator.CloneWarning { // la bibliothèque le signale sans refuser
		return comptes.ErrCompteurCle
	}
	return a.comptes.UtiliserCle(c.Nom, x.ID, x.Authenticator.SignCount, x.Flags.BackupState)
}

// clesDispo : ces routes n'existent que si le serveur connaît son adresse publique.
func (a *acces) clesDispo(w http.ResponseWriter) bool {
	if a.cles == nil {
		erreur(w, 409, "Les clés de sécurité ne sont pas configurées sur ce serveur : voir l'option -origine (README).")
		return false
	}
	return true
}

// defiConnexion : POST /api/acces/second-facteur/defi, avec la preuve : le défi pour les clés du compte.
func (s *Serveur) defiConnexion(w http.ResponseWriter, r *http.Request) {
	a := s.acces
	expiree := func() {
		repondre(w, 401, map[string]any{"detail": "Cette étape a expiré : reprenez la connexion.", "reprendre": true})
	}
	t, err := r.Cookie(cookiePreuve)
	if err != nil {
		expiree()
		return
	}
	c, id, _, ok := a.lirePreuve(t.Value)
	if !ok {
		expiree()
		return
	}
	if !a.clesDispo(w) {
		return
	}
	if c.NombreDeCles() == 0 {
		erreurCompte(w, comptes.ErrSansFacteur)
		return
	}
	o, err := a.optionsPreuve(c, "preuve:"+id)
	if err != nil {
		erreur(w, 500, "Défi impossible à préparer.")
		return
	}
	repondre(w, 200, o)
}

// defiConfirmation : POST /api/compte/second-facteur/defi, le défi pour confirmer une action par une clé.
func (s *Serveur) defiConfirmation(w http.ResponseWriter, r *http.Request) {
	c, ok := adminDeSession(w, r)
	if !ok || !s.acces.clesDispo(w) {
		return
	}
	if c.NombreDeCles() == 0 {
		erreurCompte(w, comptes.ErrSansFacteur)
		return
	}
	o, err := s.acces.optionsPreuve(c, "confirmer:"+s.acces.idSession(r))
	if err != nil {
		erreur(w, 500, "Défi impossible à préparer.")
		return
	}
	repondre(w, 200, o)
}

// debutCle : {"nom", "confirmation"} prépare l'enregistrement d'une clé. Rien n'est enregistré avant la
// réponse de la clé. Toute clé FIDO2 convient : aucune attestation demandée.
func (s *Serveur) debutCle(w http.ResponseWriter, r *http.Request) {
	c, ok := adminDeSession(w, r)
	if !ok || !s.acces.clesDispo(w) {
		return
	}
	var req struct {
		Nom          string        `json:"nom"`
		Confirmation *confirmation `json:"confirmation"`
	}
	if err := lireJSON(r, &req); err != nil {
		erreurStore(w, err)
		return
	}
	if strings.TrimSpace(req.Nom) == "" {
		erreurCompte(w, comptes.ErrNomCle)
		return
	}
	a := s.acces
	if !a.confirmer(w, r, c, req.Confirmation) {
		return
	}
	c, _ = a.comptes.Lire(c.Nom)
	u := utilisateurCle{c}
	var exclues []protocol.CredentialDescriptor
	for _, x := range u.WebAuthnCredentials() {
		exclues = append(exclues, x.Descriptor())
	}
	o, session, err := a.cles.wa.BeginRegistration(u,
		webauthn.WithExclusions(exclues),
		webauthn.WithConveyancePreference(protocol.PreferNoAttestation),
		webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{ResidentKey: protocol.ResidentKeyRequirementDiscouraged, UserVerification: protocol.VerificationDiscouraged}),
		webauthn.WithCredentialParameters([]protocol.CredentialParameter{
			{Type: protocol.PublicKeyCredentialType, Algorithm: webauthncose.AlgES256},
			{Type: protocol.PublicKeyCredentialType, Algorithm: webauthncose.AlgEdDSA},
		}),
		webauthn.WithPublicKeyCredentialHints([]protocol.PublicKeyCredentialHints{protocol.PublicKeyCredentialHintSecurityKey}))
	if err != nil {
		erreur(w, 500, "Défi impossible à préparer.")
		return
	}
	a.poserDefi("ajout:"+a.idSession(r), defiCle{session: *session, nom: c.Nom, version: c.Version, libelle: req.Nom})
	repondre(w, 200, o.Response)
}

// activerCle : {"reponse"} enregistre la clé qui vient de répondre au défi de debutCle. Les codes de
// secours, s'il n'en restait aucun, sont dans la réponse, cette seule fois.
func (s *Serveur) activerCle(w http.ResponseWriter, r *http.Request) {
	c, ok := adminDeSession(w, r)
	if !ok || !s.acces.clesDispo(w) {
		return
	}
	var req struct {
		Reponse json.RawMessage `json:"reponse"`
	}
	if err := lireJSON(r, &req); err != nil {
		erreurStore(w, err)
		return
	}
	a := s.acces
	d, ok := a.prendreDefi("ajout:" + a.idSession(r))
	if !ok || d.nom != c.Nom || d.version != c.Version {
		erreur(w, 409, "Recommencez l'enregistrement : ce défi n'est plus valable.")
		return
	}
	lu, err := protocol.ParseCredentialCreationResponseBytes(req.Reponse)
	if err != nil {
		erreur(w, 422, "Clé refusée : réponse illisible.")
		return
	}
	x, err := a.cles.wa.CreateCredential(utilisateurCle{c}, d.session, lu)
	if err != nil {
		erreur(w, 422, "Clé refusée : sa réponse ne correspond pas à ce serveur.")
		return
	}
	k := comptes.CleSecurite{Nom: d.libelle, Identifiant: x.ID, ClePublique: x.PublicKey, Type: x.AttestationType, Format: x.AttestationFormat,
		AAGUID: x.Authenticator.AAGUID, Compteur: x.Authenticator.SignCount, Eligible: x.Flags.BackupEligible, Sauvegardee: x.Flags.BackupState}
	for _, t := range x.Transport {
		k.Transports = append(k.Transports, string(t))
	}
	codes, id, err := a.comptes.AjouterCle(c.Nom, k)
	if err != nil {
		erreurCompte(w, err)
		return
	}
	a.apresActivation(w, r, c.Nom, facteurCle)
	out := map[string]any{"ok": true, "id": id}
	if len(codes) > 0 {
		out["secours"] = codes
	}
	repondre(w, 200, out)
}

// renommerCle : {"nom"}. Son nom seul change : rien à confirmer.
func (s *Serveur) renommerCle(w http.ResponseWriter, r *http.Request) {
	c, ok := adminDeSession(w, r)
	if !ok {
		return
	}
	var req struct {
		Nom string `json:"nom"`
	}
	if err := lireJSON(r, &req); err != nil {
		erreurStore(w, err)
		return
	}
	if err := s.acces.comptes.RenommerCle(c.Nom, r.PathValue("id"), req.Nom); err != nil {
		erreurCompte(w, err)
		return
	}
	repondre(w, 200, map[string]any{"ok": true})
}

// retirerCle : {"confirmation"}. Jamais le dernier facteur : 409, avant de consommer un code. Les autres
// sessions du compte tombent.
func (s *Serveur) retirerCle(w http.ResponseWriter, r *http.Request) {
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
	id := r.PathValue("id")
	trouvee := false
	if c.SecondFacteur != nil {
		for _, k := range c.SecondFacteur.Cles {
			trouvee = trouvee || k.ID == id
		}
	}
	if !trouvee {
		erreurCompte(w, comptes.ErrInconnu)
		return
	}
	if !c.PeutRetirerCle() {
		erreurCompte(w, comptes.ErrDernierFacteur)
		return
	}
	if !s.acces.confirmer(w, r, c, req.Confirmation) {
		return
	}
	if err := s.acces.comptes.RetirerCle(c.Nom, id); err != nil {
		erreurCompte(w, err)
		return
	}
	s.acces.noter(r, "second_facteur_retire", c.Nom, "facteur="+facteurCle, "cle="+id)
	s.acces.renouvelerSession(w, r, c.Nom)
	repondre(w, 200, map[string]any{"ok": true})
}
