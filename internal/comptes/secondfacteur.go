package comptes

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Second facteur des administrateurs. Les autres comptes n'en ont pas.
//
// TOTP (RFC 6238 : SHA-1, 6 chiffres, 30 secondes) : le secret est chiffré avec une clé rangée à côté
// du fichier des comptes (fichierCle). Cela protège d'une fuite de ce seul fichier, pas de qui lit tout
// le dossier : celui-là a aussi le secret qui signe les sessions (README, « Double authentification »).
//
// Codes de secours : tirés au hasard, assez longs pour qu'un calcul coûteux n'ajoute rien ; seule leur
// empreinte est gardée.
//
// Clés de sécurité (WebAuthn) : leur vérification est dans internal/serveur ; il ne reste ici que leur
// clé publique et leur compteur, rien de secret.

const (
	totpPeriode  = 30
	totpChiffres = 6
	totpFenetre  = 1 // pas acceptés de part et d'autre de l'heure du serveur
	totpOctets   = 20

	secoursNombre   = 10
	secoursLongueur = 16
	secoursAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ" // base 32 de Crockford : ni I, L, O, U

	clesMax   = 10
	nomCleMax = 60

	fichierCle     = "comptes.cle"
	fichierVerrou  = "comptes.verrou"
	formatChiffre  = "aes256gcm"
	longueurCleAES = 32
)

var (
	ErrCode            = errors.New("code incorrect")
	ErrSansFacteur     = errors.New("ce compte n'a pas de second facteur")
	ErrDernierFacteur  = errors.New("c'est le dernier second facteur de ce compte : enregistrez-en un autre d'abord")
	ErrPasAdmin        = errors.New("ce compte n'est pas administrateur")
	ErrSecretIllisible = errors.New("secret du second facteur illisible")
	ErrCompteurCle     = errors.New("compteur de la clé incohérent : elle a peut-être été copiée")
	ErrCleConnue       = errors.New("cette clé est déjà enregistrée")
	ErrNomCle          = errors.New("donnez un nom à cette clé")
	ErrTropDeCles      = errors.New("trop de clés : retirez-en une d'abord")
	ErrOccupe          = errors.New("un autre programme tient déjà le fichier des comptes")
)

// SecondFacteur : ce qui prouve le second facteur d'un compte. Les codes de secours n'en sont pas un à
// eux seuls : ils dépannent, et l'obligation demande un TOTP ou une clé.
type SecondFacteur struct {
	TOTP    *FacteurTOTP  `json:"totp,omitempty"`
	Cles    []CleSecurite `json:"cles,omitempty"`
	Secours []CodeSecours `json:"secours,omitempty"`
}

type FacteurTOTP struct {
	Secret     string  `json:"secret"`      // chiffré : voir chiffrer
	DernierPas int64   `json:"dernier_pas"` // aucun code de ce pas, ni d'un plus ancien, n'est accepté
	Active     float64 `json:"active"`
}

// CleSecurite : une clé WebAuthn enregistrée. Rien de secret : sa clé publique et son identifiant.
type CleSecurite struct {
	ID          string   `json:"id"` // court, pour la désigner dans l'API et au journal
	Nom         string   `json:"nom"`
	Identifiant []byte   `json:"identifiant"`
	ClePublique []byte   `json:"cle_publique"`
	Type        string   `json:"type,omitempty"`
	Format      string   `json:"format,omitempty"`
	Transports  []string `json:"transports,omitempty"`
	AAGUID      []byte   `json:"aaguid,omitempty"`
	Compteur    uint32   `json:"compteur"`
	// Eligible, Sauvegardee : indications de la clé ; une clé ne change jamais d'éligibilité
	Eligible    bool    `json:"sauvegarde_eligible,omitempty"`
	Sauvegardee bool    `json:"sauvegardee,omitempty"`
	Cree        float64 `json:"cree"`
	Utilise     float64 `json:"utilise,omitempty"`
}

type CodeSecours struct {
	Empreinte string  `json:"empreinte"`
	Utilise   float64 `json:"utilise,omitempty"`
}

// AFacteur : ce compte a un facteur qui répond à l'obligation.
func (c Compte) AFacteur() bool {
	return c.ATOTP() || c.NombreDeCles() > 0
}

// ATOTP : ce compte a un TOTP actif.
func (c Compte) ATOTP() bool { return c.SecondFacteur != nil && c.SecondFacteur.TOTP != nil }

// NombreDeCles : ses clés de sécurité.
func (c Compte) NombreDeCles() int {
	if c.SecondFacteur == nil {
		return 0
	}
	return len(c.SecondFacteur.Cles)
}

// SecoursRestants : ses codes de secours encore valables.
func (c Compte) SecoursRestants() int {
	if c.SecondFacteur == nil {
		return 0
	}
	n := 0
	for _, s := range c.SecondFacteur.Secours {
		if s.Utilise == 0 {
			n++
		}
	}
	return n
}

// PeutRetirerTOTP : il lui reste un autre facteur, ou l'obligation ne le concerne pas.
func (c Compte) PeutRetirerTOTP() bool { return !c.Admin || c.NombreDeCles() > 0 }

// PeutRetirerCle : il lui reste un autre facteur, ou l'obligation ne le concerne pas.
func (c Compte) PeutRetirerCle() bool {
	return !c.Admin || c.ATOTP() || c.NombreDeCles() > 1
}

func copieSecondFacteur(s *SecondFacteur) *SecondFacteur {
	if s == nil {
		return nil
	}
	x := *s
	if s.TOTP != nil {
		t := *s.TOTP
		x.TOTP = &t
	}
	x.Secours = append([]CodeSecours(nil), s.Secours...)
	x.Cles = append([]CleSecurite(nil), s.Cles...)
	for i := range x.Cles {
		x.Cles[i].Identifiant = append([]byte(nil), x.Cles[i].Identifiant...)
		x.Cles[i].ClePublique = append([]byte(nil), x.Cles[i].ClePublique...)
		x.Cles[i].AAGUID = append([]byte(nil), x.Cles[i].AAGUID...)
		x.Cles[i].Transports = append([]string(nil), x.Cles[i].Transports...)
	}
	return &x
}

// HorlogePourTests : l'heure des codes, à la main. Rien d'autre ne l'appelle.
func (r *Registre) HorlogePourTests(f func() time.Time) { r.horloge = f }

func (r *Registre) pasCourant() int64 {
	if r.horloge != nil {
		return r.horloge().Unix() / totpPeriode
	}
	return time.Now().Unix() / totpPeriode
}

// ---------------------------------------------------------------- TOTP

var base32Totp = base32.StdEncoding.WithPadding(base32.NoPadding)

// NouveauSecretTOTP tire un secret, en base 32 (ce qu'on saisit dans une application d'authentification).
func NouveauSecretTOTP() string {
	b := make([]byte, totpOctets)
	_, _ = rand.Read(b)
	return base32Totp.EncodeToString(b)
}

func hotp(cle []byte, compteur uint64, chiffres int) string {
	m := hmac.New(sha1.New, cle)
	_ = binary.Write(m, binary.BigEndian, compteur)
	s := m.Sum(nil)
	o := s[len(s)-1] & 0x0f
	v := uint64(binary.BigEndian.Uint32(s[o:o+4]) & 0x7fffffff)
	mod := uint64(1)
	for range chiffres {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", chiffres, v%mod)
}

// CodeTOTP : le code de ce secret à cette heure ; "" si le secret est illisible.
func CodeTOTP(secret string, t time.Time) string {
	cle, err := base32Totp.DecodeString(secret)
	if err != nil {
		return ""
	}
	return hotp(cle, uint64(t.Unix()/totpPeriode), totpChiffres)
}

// lireCodeTOTP : les chiffres saisis, sans les espaces qu'on y met pour les lire.
func lireCodeTOTP(code string) (string, bool) {
	code = strings.ReplaceAll(code, " ", "")
	if len(code) != totpChiffres {
		return "", false
	}
	for _, x := range code {
		if x < '0' || x > '9' {
			return "", false
		}
	}
	return code, true
}

// pasAccepte : le pas de ce code dans la fenêtre, plus récent que dernier. Tous les pas de la fenêtre
// sont comparés, à temps constant, quel que soit le résultat.
func pasAccepte(cle []byte, code string, courant, dernier int64) (int64, bool) {
	code, ok := lireCodeTOTP(code)
	if !ok {
		return 0, false
	}
	var trouve int64 = -1
	for p := courant - totpFenetre; p <= courant+totpFenetre; p++ {
		egal := subtle.ConstantTimeCompare([]byte(hotp(cle, uint64(p), totpChiffres)), []byte(code)) == 1
		if egal && p > dernier && p > trouve {
			trouve = p
		}
	}
	return trouve, trouve >= 0
}

// ActiverTOTP enregistre ce secret si ce code en vient, à la place du précédent s'il y en a un. Les
// codes de secours sont tirés s'il n'en reste aucun : ils sont renvoyés cette seule fois.
func (r *Registre) ActiverTOTP(nom, secret, code string) ([]string, error) {
	var codes []string
	_, err := r.Modifier(nom, func(c *Compte) error {
		cle, err := base32Totp.DecodeString(secret)
		if err != nil {
			return ErrCode
		}
		pas, ok := pasAccepte(cle, code, r.pasCourant(), -1)
		if !ok {
			return ErrCode
		}
		chiffre, err := r.chiffrer(c, cle)
		if err != nil {
			return err
		}
		if c.SecondFacteur == nil {
			c.SecondFacteur = &SecondFacteur{}
		}
		c.SecondFacteur.TOTP = &FacteurTOTP{Secret: chiffre, DernierPas: pas, Active: maintenant()}
		if c.SecoursRestants() == 0 {
			codes, c.SecondFacteur.Secours = nouveauxSecours(c)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return codes, nil
}

// VerifierTOTP accepte ce code une seule fois : le pas accepté est écrit avant de répondre oui, et si
// l'écriture échoue, c'est non.
func (r *Registre) VerifierTOTP(nom, code string) error {
	_, err := r.Modifier(nom, func(c *Compte) error {
		if !c.ATOTP() {
			return ErrCode
		}
		cle, err := r.dechiffrer(c, c.SecondFacteur.TOTP.Secret)
		if err != nil {
			return err
		}
		pas, ok := pasAccepte(cle, code, r.pasCourant(), c.SecondFacteur.TOTP.DernierPas)
		if !ok {
			return ErrCode
		}
		c.SecondFacteur.TOTP.DernierPas = pas
		return nil
	})
	return err
}

// RetirerTOTP : les sessions ouvertes du compte tombent.
func (r *Registre) RetirerTOTP(nom string) error {
	_, err := r.Modifier(nom, func(c *Compte) error {
		if !c.ATOTP() {
			return ErrInconnu
		}
		if !c.PeutRetirerTOTP() {
			return ErrDernierFacteur
		}
		c.SecondFacteur.TOTP = nil
		c.Version++
		return nil
	})
	return err
}

// ---------------------------------------------------------------- clés de sécurité

func lireNomCle(nom string) (string, error) {
	nom = strings.TrimSpace(nom)
	if nom == "" {
		return "", ErrNomCle
	}
	if rs := []rune(nom); len(rs) > nomCleMax {
		nom = string(rs[:nomCleMax])
	}
	return nom, nil
}

// AjouterCle enregistre une clé déjà vérifiée. Une clé qui sert déjà, sur ce compte ou un autre, est
// refusée. Les codes de secours sont tirés s'il n'en reste aucun : ils sont renvoyés cette seule fois.
func (r *Registre) AjouterCle(nom string, cle CleSecurite) (codes []string, id string, err error) {
	if cle.Nom, err = lireNomCle(cle.Nom); err != nil {
		return nil, "", err
	}
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	cle.ID, cle.Cree = hex.EncodeToString(b), maintenant()
	_, err = r.Modifier(nom, func(c *Compte) error {
		for _, autre := range r.m { // r.mu est tenu
			if autre.SecondFacteur == nil {
				continue
			}
			for _, k := range autre.SecondFacteur.Cles {
				if bytes.Equal(k.Identifiant, cle.Identifiant) {
					return ErrCleConnue
				}
			}
		}
		if c.NombreDeCles() >= clesMax {
			return ErrTropDeCles
		}
		if c.SecondFacteur == nil {
			c.SecondFacteur = &SecondFacteur{}
		}
		c.SecondFacteur.Cles = append(c.SecondFacteur.Cles, cle)
		if c.SecoursRestants() == 0 {
			codes, c.SecondFacteur.Secours = nouveauxSecours(c)
		}
		return nil
	})
	if err != nil {
		return nil, "", err
	}
	return codes, cle.ID, nil
}

// UtiliserCle note l'usage d'une clé dont la signature vient d'être vérifiée. Le compteur est comparé
// sous le verrou : s'il ne monte pas, alors qu'il compte, la clé a pu être copiée, et c'est non. Une clé
// sans compteur reste à zéro. Si l'écriture échoue, c'est non aussi.
func (r *Registre) UtiliserCle(nom string, identifiant []byte, compteur uint32, sauvegardee bool) error {
	_, err := r.Modifier(nom, func(c *Compte) error {
		if c.SecondFacteur == nil {
			return ErrCode
		}
		for i := range c.SecondFacteur.Cles {
			k := &c.SecondFacteur.Cles[i]
			if !bytes.Equal(k.Identifiant, identifiant) {
				continue
			}
			if (compteur != 0 || k.Compteur != 0) && compteur <= k.Compteur {
				return ErrCompteurCle
			}
			k.Compteur, k.Sauvegardee, k.Utilise = compteur, sauvegardee, maintenant()
			return nil
		}
		return ErrCode
	})
	return err
}

// RenommerCle : son nom seul change.
func (r *Registre) RenommerCle(nom, id, libelle string) error {
	libelle, err := lireNomCle(libelle)
	if err != nil {
		return err
	}
	_, err = r.Modifier(nom, func(c *Compte) error {
		if c.SecondFacteur != nil {
			for i := range c.SecondFacteur.Cles {
				if c.SecondFacteur.Cles[i].ID == id {
					c.SecondFacteur.Cles[i].Nom = libelle
					return nil
				}
			}
		}
		return ErrInconnu
	})
	return err
}

// RetirerCle : jamais le dernier facteur. Les sessions ouvertes du compte tombent.
func (r *Registre) RetirerCle(nom, id string) error {
	_, err := r.Modifier(nom, func(c *Compte) error {
		if c.SecondFacteur == nil {
			return ErrInconnu
		}
		for i := range c.SecondFacteur.Cles {
			if c.SecondFacteur.Cles[i].ID != id {
				continue
			}
			if !c.PeutRetirerCle() {
				return ErrDernierFacteur
			}
			c.SecondFacteur.Cles = slices.Delete(c.SecondFacteur.Cles, i, i+1)
			c.Version++
			return nil
		}
		return ErrInconnu
	})
	return err
}

// ---------------------------------------------------------------- codes de secours

func empreinteSecours(c *Compte, code string) string {
	h := sha256.Sum256([]byte(c.Graine + ":" + code))
	return hex.EncodeToString(h[:])
}

// nouveauxSecours : les codes à montrer, formés par groupes de quatre, et ce qu'on en garde.
func nouveauxSecours(c *Compte) ([]string, []CodeSecours) {
	codes := make([]string, secoursNombre)
	gardes := make([]CodeSecours, secoursNombre)
	for i := range codes {
		b := make([]byte, secoursLongueur)
		_, _ = rand.Read(b)
		for j := range b {
			b[j] = secoursAlphabet[b[j]&31] // 256 est un multiple de 32 : tirage uniforme
		}
		gardes[i].Empreinte = empreinteSecours(c, string(b))
		codes[i] = string(b[:4]) + "-" + string(b[4:8]) + "-" + string(b[8:12]) + "-" + string(b[12:])
	}
	return codes, gardes
}

// lireSecours : un code saisi, sous sa forme gardée (minuscules, tirets et lettres ambiguës admis).
func lireSecours(code string) (string, bool) {
	code = strings.NewReplacer("-", "", " ", "", "O", "0", "I", "1", "L", "1").Replace(strings.ToUpper(code))
	if len(code) != secoursLongueur {
		return "", false
	}
	for _, x := range code {
		if !strings.ContainsRune(secoursAlphabet, x) {
			return "", false
		}
	}
	return code, true
}

// UtiliserSecours consomme ce code et renvoie ce qu'il en reste. Si l'écriture échoue, il n'a pas servi.
func (r *Registre) UtiliserSecours(nom, code string) (int, error) {
	x, err := r.Modifier(nom, func(c *Compte) error {
		lu, ok := lireSecours(code)
		if !ok || c.SecondFacteur == nil {
			return ErrCode
		}
		e := []byte(empreinteSecours(c, lu))
		trouve := -1
		for i, s := range c.SecondFacteur.Secours { // tous comparés, à temps constant
			if subtle.ConstantTimeCompare(e, []byte(s.Empreinte)) == 1 && s.Utilise == 0 {
				trouve = i
			}
		}
		if trouve < 0 {
			return ErrCode
		}
		c.SecondFacteur.Secours[trouve].Utilise = maintenant()
		return nil
	})
	if err != nil {
		return 0, err
	}
	return x.SecoursRestants(), nil
}

// RegenererSecours remplace tous les codes de secours : les anciens ne valent plus rien.
func (r *Registre) RegenererSecours(nom string) ([]string, error) {
	var codes []string
	_, err := r.Modifier(nom, func(c *Compte) error {
		if !c.AFacteur() {
			return ErrSansFacteur
		}
		codes, c.SecondFacteur.Secours = nouveauxSecours(c)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return codes, nil
}

// ---------------------------------------------------------------- réinitialisation

// ReinitialiserSecondFacteur retire tous les facteurs et les codes de secours : le compte repasse par
// l'enrôlement, et ses sessions tombent. Le mot de passe ne change pas.
func (r *Registre) ReinitialiserSecondFacteur(nom string) error {
	_, err := r.Modifier(nom, func(c *Compte) error {
		c.SecondFacteur = nil
		c.Version++
		return nil
	})
	return err
}

// ReinitialiserLocalement : le dernier recours, depuis la machine (voir main.go). Une note reste dans le
// fichier : le journal d'un conteneur jetable disparaît avec lui, le serveur la reprend à son démarrage.
func (r *Registre) ReinitialiserLocalement(nom string) error {
	_, err := r.Modifier(nom, func(c *Compte) error {
		if !c.Admin {
			return ErrPasAdmin
		}
		c.SecondFacteur = nil
		c.Version++
		c.ReinitLocale = maintenant()
		return nil
	})
	return err
}

// PrendreReinitLocales : les comptes réinitialisés par la commande locale depuis le dernier appel.
func (r *Registre) PrendreReinitLocales() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var l []string
	for _, c := range r.m {
		if c.ReinitLocale != 0 {
			c.ReinitLocale = 0
			l = append(l, c.Nom)
		}
	}
	if len(l) > 0 {
		_ = r.ecrire() // perdue, la note reviendrait au démarrage suivant : rien de grave
	}
	return l
}

// ---------------------------------------------------------------- chiffrement du secret

// cleSecrets : la clé de chiffrement (r.mu tenu). Absente, elle n'est tirée que pour écrire : une clé
// neuve ne relirait rien, et ferait croire à une clé valable. Illisible, elle n'est jamais remplacée.
func (r *Registre) cleSecrets(creer bool) ([]byte, error) {
	if r.cle != nil {
		return r.cle, nil
	}
	chemin := filepath.Join(filepath.Dir(r.chemin), fichierCle)
	b, err := os.ReadFile(chemin)
	if errors.Is(err, os.ErrNotExist) {
		if !creer {
			return nil, ErrSecretIllisible
		}
		cle := make([]byte, longueurCleAES)
		if _, err := rand.Read(cle); err != nil {
			return nil, err
		}
		if err := os.MkdirAll(filepath.Dir(chemin), 0o700); err != nil {
			return nil, err
		}
		tmp := chemin + ".tmp"
		if err := os.WriteFile(tmp, []byte(hex.EncodeToString(cle)+"\n"), 0o600); err != nil {
			return nil, err
		}
		if err := os.Rename(tmp, chemin); err != nil {
			return nil, err
		}
		r.cle = cle
		return cle, nil
	}
	if err != nil {
		return nil, err
	}
	cle, err := hex.DecodeString(strings.TrimSpace(string(b)))
	if err != nil || len(cle) != longueurCleAES {
		return nil, ErrSecretIllisible
	}
	r.cle = cle
	return cle, nil
}

func aeadSecrets(cle []byte) (cipher.AEAD, error) {
	bloc, err := aes.NewCipher(cle)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(bloc)
}

// liaison : un secret chiffré ne vaut que pour son compte, et pas pour un homonyme recréé.
func liaison(c *Compte) []byte { return []byte(c.Nom + "\x00" + c.Graine) }

func (r *Registre) chiffrer(c *Compte, clair []byte) (string, error) {
	cle, err := r.cleSecrets(true)
	if err != nil {
		return "", err
	}
	a, err := aeadSecrets(cle)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, a.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return formatChiffre + "$" + base64.RawStdEncoding.EncodeToString(a.Seal(nonce, nonce, clair, liaison(c))), nil
}

func (r *Registre) dechiffrer(c *Compte, texte string) ([]byte, error) {
	format, valeur, ok := strings.Cut(texte, "$")
	if !ok || format != formatChiffre {
		return nil, ErrSecretIllisible
	}
	b, err := base64.RawStdEncoding.DecodeString(valeur)
	if err != nil {
		return nil, ErrSecretIllisible
	}
	cle, err := r.cleSecrets(false)
	if err != nil {
		return nil, ErrSecretIllisible
	}
	a, err := aeadSecrets(cle)
	if err != nil || len(b) < a.NonceSize() {
		return nil, ErrSecretIllisible
	}
	clair, err := a.Open(nil, b[:a.NonceSize()], b[a.NonceSize():], liaison(c))
	if err != nil {
		return nil, ErrSecretIllisible
	}
	return clair, nil
}
