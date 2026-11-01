// Package comptes garde les comptes du mode serveur : nom d'utilisateur, mot de passe haché, rôle
// dans chaque univers, jetons d'API. Ils tiennent au serveur, pas à un univers.
//
// Un administrateur gère le serveur (univers, comptes) sans rien voir du contenu : il n'a de rôle
// dans aucun univers, ni jeton d'API. Il décide à quels univers un compte peut être joint comme
// joueur (Admis) ; c'est ensuite le MJ de l'univers qui l'y joint, ou non, et lui donne son PJ.
package comptes

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Rôles d'un compte dans un univers. L'administrateur, lui, l'est pour tout le serveur, et n'en a
// aucun dans les univers.
const (
	MJ       = "mj"       // lit et modifie l'univers
	Auditeur = "auditeur" // lit tout l'univers, secrets du MJ et journal compris, sans rien modifier
	Joueur   = "joueur"   // ne lit que les fiches partagées par le MJ, sans leurs champs réservés au MJ
)

// RoleValide : c'est un rôle qu'un compte peut avoir dans un univers.
func RoleValide(role string) bool { return role == MJ || role == Auditeur || role == Joueur }

const (
	MdpMin      = 10
	MdpMinAdmin = 12
	prefixe     = "cofdm_"
	jetonsMax   = 20
	// utiliseTous : le fichier des comptes ne s'écrit pas à chaque requête d'un poste relié
	utiliseTous = 3600.0
)

var (
	ErrInconnu      = errors.New("compte inconnu")
	ErrExiste       = errors.New("ce nom est déjà pris")
	ErrNom          = errors.New("nom invalide : 3 à 32 caractères parmi a-z, 0-9, « - » et « _ »")
	ErrMdp          = errors.New("le mot de passe doit faire au moins 10 caractères")
	ErrMdpAdmin     = errors.New("le mot de passe d'un administrateur doit faire au moins 12 caractères")
	ErrMdpFaible    = errors.New("ce mot de passe est trop facile à deviner : évitez le nom du compte, les répétitions et les mots de passe courants")
	ErrRole         = errors.New("rôle inconnu : mj, auditeur ou joueur")
	ErrDernierAdmin = errors.New("il doit rester un administrateur actif")
	ErrTropDeJetons = errors.New("trop de jetons : retirez-en un d'abord")
	ErrAdmin        = errors.New("un compte administrateur n'a ni rôle dans les univers ni jeton d'API : retirez-les d'abord, ou utilisez un autre compte")
	ErrNonAdmis     = errors.New("ce compte n'est pas admis comme joueur dans cet univers")

	reNom = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{2,31}$`)
)

// Jeton d'API d'un compte. Seule son empreinte est gardée : il ne s'affiche qu'à sa création.
type Jeton struct {
	ID        string  `json:"id"`
	Nom       string  `json:"nom"`
	Empreinte string  `json:"empreinte"`
	Cree      float64 `json:"cree"`
	Utilise   float64 `json:"utilise,omitempty"` // dernière utilisation, à une heure près : voir ParJeton
}

type Compte struct {
	Nom     string            `json:"nom"`
	Mdp     string            `json:"mdp"` // « sel:empreinte » (PBKDF2-SHA256)
	Admin   bool              `json:"admin,omitempty"`
	Inactif bool              `json:"inactif,omitempty"`
	Roles   map[string]string `json:"roles,omitempty"`  // identifiant d'univers → rôle
	Admis   []string          `json:"admis,omitempty"`  // univers où le MJ peut joindre ce compte comme joueur
	Persos  map[string]int64  `json:"persos,omitempty"` // identifiant d'univers → fiche du PJ que joue ce compte
	Jetons  []Jeton           `json:"jetons,omitempty"`
	Version int               `json:"version"` // change avec le mot de passe : les sessions ouvertes tombent
	Cree    float64           `json:"cree"`
	Graine  string            `json:"graine"` // tirée à la création : une session n'ouvre que ce compte-là, pas un homonyme recréé
	// DoitChanger : son mot de passe est passé par les journaux ; il n'a accès à rien d'autre qu'à son
	// compte tant qu'il ne l'a pas changé.
	DoitChanger bool `json:"doit_changer,omitempty"`
}

// Role renvoie le rôle du compte dans un univers ("" : aucun accès, toujours pour un administrateur).
func (c Compte) Role(univers string) string {
	if c.Admin {
		return ""
	}
	return c.Roles[univers]
}

// PeutJouer : l'administrateur a admis ce compte comme joueur de l'univers.
func (c Compte) PeutJouer(univers string) bool {
	return !c.Admin && slices.Contains(c.Admis, univers)
}

// Registre est le fichier des comptes, lisible par son seul propriétaire.
type Registre struct {
	chemin string
	mu     sync.Mutex
	m      map[string]*Compte
	leurre []byte // sel du calcul fait pour un compte inconnu, pour répondre dans le même temps
}

// Ouvrir lit le fichier des comptes ; absent, le registre est vide.
func Ouvrir(chemin string) (*Registre, error) {
	r := &Registre{chemin: chemin, m: map[string]*Compte{}, leurre: make([]byte, 16)}
	_, _ = rand.Read(r.leurre)
	b, err := os.ReadFile(chemin)
	if errors.Is(err, os.ErrNotExist) {
		return r, nil
	}
	if err != nil {
		return nil, err
	}
	var l []*Compte
	if err := json.Unmarshal(b, &l); err != nil {
		return nil, errors.New("fichier des comptes illisible : " + err.Error())
	}
	change := false
	for _, c := range l {
		r.m[c.Nom] = c
		change = reprendre(c) || change
	}
	if change {
		if err := r.ecrire(); err != nil {
			return nil, err
		}
	}
	return r, nil
}

// reprendre met un compte d'avant la séparation des rôles dans la forme actuelle : un
// administrateur perd ses rôles et ses jetons (il n'entre plus dans les univers), un compte joueur
// est admis dans les univers où il joue déjà. Un compte sans graine en reçoit une : ses sessions
// d'avant tombent, une seule fois.
func reprendre(c *Compte) bool {
	change := false
	if c.Graine == "" {
		c.Graine, change = nouvelleGraine(), true
	}
	if c.Admin {
		change = len(c.Roles)+len(c.Admis)+len(c.Persos)+len(c.Jetons) > 0 || change
		c.Roles, c.Admis, c.Persos, c.Jetons = nil, nil, nil, nil
		return change
	}
	for u, role := range c.Roles {
		if role == Joueur && !slices.Contains(c.Admis, u) {
			c.Admis, change = append(c.Admis, u), true
		}
	}
	slices.Sort(c.Admis)
	return change
}

func nouvelleGraine() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (r *Registre) ecrire() error {
	l := make([]*Compte, 0, len(r.m))
	for _, c := range r.m {
		l = append(l, c)
	}
	sort.Slice(l, func(i, j int) bool { return l[i].Nom < l[j].Nom })
	b, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(r.chemin), 0o700); err != nil {
		return err
	}
	tmp := r.chemin + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, r.chemin)
}

// Normaliser met un nom d'utilisateur sous sa forme enregistrée.
func Normaliser(nom string) string { return strings.ToLower(strings.TrimSpace(nom)) }

func copie(c *Compte) Compte {
	x := *c
	x.Roles = map[string]string{}
	for k, v := range c.Roles {
		x.Roles[k] = v
	}
	x.Persos = map[string]int64{}
	for k, v := range c.Persos {
		x.Persos[k] = v
	}
	x.Admis = append([]string(nil), c.Admis...)
	x.Jetons = append([]Jeton(nil), c.Jetons...)
	return x
}

// Le coût d'une empreinte de mot de passe (recommandation OWASP pour PBKDF2-SHA256). Une empreinte
// porte le sien : un compte passe au coût courant à sa prochaine connexion réussie.
const (
	formatEmpreinte     = "pbkdf2-sha256"
	iterationsAnciennes = 210000 // « sel:empreinte », la forme d'avant
	iterationsMax       = 100000000
)

var iterationsMdp = 600000

// CoutReduitPourTests : les tests créent des centaines de comptes. Rien d'autre ne l'appelle.
func CoutReduitPourTests() { iterationsMdp = 1000 }

func deriver(mdp string, sel []byte, iterations int) []byte {
	k, _ := pbkdf2.Key(sha256.New, mdp, sel, iterations, 32)
	return k
}

func empreinteMdp(mdp string) string {
	sel := make([]byte, 16)
	_, _ = rand.Read(sel)
	return fmt.Sprintf("%s$%d$%s$%s", formatEmpreinte, iterationsMdp, hex.EncodeToString(sel), hex.EncodeToString(deriver(mdp, sel, iterationsMdp)))
}

// empreinte : une empreinte de mot de passe relue. Illisible : aucun mot de passe n'y correspond.
type empreinte struct {
	iterations  int
	sel, valeur []byte
}

func lireEmpreinte(texte string) empreinte {
	var e empreinte
	var sel, valeur string
	if p := strings.Split(texte, "$"); len(p) == 4 && p[0] == formatEmpreinte {
		n, err := strconv.Atoi(p[1])
		if err != nil || n <= 0 || n > iterationsMax {
			return empreinte{}
		}
		e.iterations, sel, valeur = n, p[2], p[3]
	} else if s, h, ok := strings.Cut(texte, ":"); ok && !strings.Contains(texte, "$") {
		e.iterations, sel, valeur = iterationsAnciennes, s, h
	} else {
		return empreinte{}
	}
	var err1, err2 error
	e.sel, err1 = hex.DecodeString(sel)
	e.valeur, err2 = hex.DecodeString(valeur)
	if err1 != nil || err2 != nil || len(e.valeur) == 0 {
		return empreinte{}
	}
	return e
}

func (e empreinte) correspond(mdp string) bool {
	return e.iterations > 0 && subtle.ConstantTimeCompare(deriver(mdp, e.sel, e.iterations), e.valeur) == 1
}

func maintenant() float64 { return float64(time.Now().UnixNano()) / 1e9 }

// Vide : aucun compte n'existe encore.
func (r *Registre) Vide() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.m) == 0
}

// Liste renvoie les comptes, par nom.
func (r *Registre) Liste() []Compte {
	r.mu.Lock()
	defer r.mu.Unlock()
	l := make([]Compte, 0, len(r.m))
	for _, c := range r.m {
		l = append(l, copie(c))
	}
	sort.Slice(l, func(i, j int) bool { return l[i].Nom < l[j].Nom })
	return l
}

func (r *Registre) Lire(nom string) (Compte, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.m[Normaliser(nom)]
	if !ok {
		return Compte{}, false
	}
	return copie(c), true
}

// mdpCourants : les plus essayés de ceux qui passent la longueur minimale (en minuscules).
var mdpCourants = map[string]bool{
	"0123456789": true, "1234567890": true, "12345678910": true, "123456789a": true, "a123456789": true, "0987654321": true,
	"1234512345": true, "1122334455": true, "1q2w3e4r5t": true, "1qaz2wsx3edc": true, "qwertyuiop": true, "azertyuiop": true,
	"azerty1234": true, "azerty12345": true, "azerty123456": true, "qwerty1234": true, "qwerty12345": true, "qwerty123456": true,
	"password12": true, "password123": true, "password1234": true, "passw0rd123": true, "motdepasse": true, "motdepasse1": true,
	"motdepasse12": true, "motdepasse123": true, "motdepasse1234": true, "administrateur": true, "administrator": true,
	"adminadmin": true, "admin12345": true, "admin123456": true, "changeme123": true, "letmein1234": true, "iloveyou12": true,
	"iloveyou123": true, "jetaime1234": true, "bonjour1234": true, "soleil1234": true, "welcome123": true, "football123": true,
	"dragon1234": true, "dragon12345": true, "chroniques": true, "chroniquesoubliees": true, "maitredujeu": true, "donjons1234": true,
}

// MdpAcceptable : la règle d'un nouveau mot de passe. Elle ne vaut qu'à la création et au changement :
// un compte existant se connecte toujours avec le sien.
func MdpAcceptable(nom, mdp string, admin bool) error {
	n := len([]rune(mdp))
	switch {
	case n < MdpMin:
		return ErrMdp
	case admin && n < MdpMinAdmin:
		return ErrMdpAdmin
	}
	bas := strings.ToLower(mdp)
	repete := true
	for _, x := range bas {
		if x != []rune(bas)[0] {
			repete = false
			break
		}
	}
	if repete || mdpCourants[bas] || (nom != "" && strings.Contains(bas, strings.ToLower(nom))) {
		return ErrMdpFaible
	}
	return nil
}

func (r *Registre) Creer(nom, mdp string, admin bool) (Compte, error) {
	nom = Normaliser(nom)
	if !reNom.MatchString(nom) {
		return Compte{}, ErrNom
	}
	if err := MdpAcceptable(nom, mdp, admin); err != nil {
		return Compte{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.m[nom]; ok {
		return Compte{}, ErrExiste
	}
	c := &Compte{Nom: nom, Mdp: empreinteMdp(mdp), Admin: admin, Version: 1, Cree: maintenant(), Graine: nouvelleGraine()}
	r.m[nom] = c
	if err := r.ecrire(); err != nil {
		delete(r.m, nom)
		return Compte{}, err
	}
	return copie(c), nil
}

// Verifier renvoie le compte actif dont c'est le mot de passe. Pour un compte inconnu ou inactif,
// le même calcul est fait : le temps de réponse ne dit pas si le nom existe.
func (r *Registre) Verifier(nom, mdp string) (Compte, bool) {
	r.mu.Lock()
	c, ok := r.m[Normaliser(nom)]
	var x Compte
	if ok {
		x = copie(c)
	}
	r.mu.Unlock()
	e := empreinte{}
	if ok {
		e = lireEmpreinte(x.Mdp)
	}
	bon := e.correspond(mdp)
	// un échec coûte le même calcul quel que soit le compte, inconnu ou d'avant la migration : le temps
	// de réponse ne dit pas lequel existe
	if reste := iterationsMdp - e.iterations; !bon && reste > 0 {
		deriver(mdp, r.leurre, reste)
	}
	if !ok || !bon || x.Inactif {
		return Compte{}, false
	}
	if e.iterations < iterationsMdp {
		x.Mdp = r.migrer(x.Nom, x.Mdp, mdp)
	}
	return x, true
}

// migrer passe au coût courant l'empreinte d'un mot de passe qui vient d'être vérifié. Ni la version ni
// la graine ne changent : les sessions ouvertes restent. Si l'écriture échoue, l'ancienne empreinte
// reste, et la connexion réussit quand même.
func (r *Registre) migrer(nom, avant, mdp string) string {
	neuve := empreinteMdp(mdp)
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.m[nom]
	if !ok || c.Mdp != avant { // changé entre-temps : pas d'une ancienne valeur sur une nouvelle
		return avant
	}
	c.Mdp = neuve
	if err := r.ecrire(); err != nil {
		c.Mdp = avant
		return avant
	}
	return neuve
}

// Modifier applique f au compte et l'enregistre. Le dernier administrateur actif ne peut ni perdre
// son rôle, ni être désactivé. Un administrateur n'a ni rôle dans les univers ni jeton ; un compte
// n'est joueur que d'un univers où il est admis (ne plus l'y admettre l'en retire).
func (r *Registre) Modifier(nom string, f func(*Compte) error) (Compte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.m[Normaliser(nom)]
	if !ok {
		return Compte{}, ErrInconnu
	}
	avant := copie(c)
	x := copie(c)
	if err := f(&x); err != nil {
		return Compte{}, err
	}
	x.Nom = avant.Nom
	slices.Sort(x.Admis)
	x.Admis = slices.Compact(x.Admis)
	x.Admis = slices.DeleteFunc(x.Admis, func(u string) bool { return u == "" })
	for u, role := range x.Roles {
		switch role {
		case MJ, Auditeur:
		case Joueur:
			if !slices.Contains(x.Admis, u) {
				if avant.Roles[u] == Joueur { // l'administrateur ne l'admet plus : il quitte l'univers
					delete(x.Roles, u)
					continue
				}
				return Compte{}, ErrNonAdmis
			}
		case "":
			delete(x.Roles, u)
		default:
			return Compte{}, ErrRole
		}
	}
	for u, pj := range x.Persos {
		if pj <= 0 || x.Roles[u] != Joueur {
			delete(x.Persos, u)
		}
	}
	if x.Admin && len(x.Roles)+len(x.Admis)+len(x.Persos)+len(x.Jetons) > 0 {
		return Compte{}, ErrAdmin
	}
	*c = x
	if avant.Admin && !avant.Inactif && !r.resteUnAdmin() {
		*c = avant
		return Compte{}, ErrDernierAdmin
	}
	if err := r.ecrire(); err != nil {
		*c = avant
		return Compte{}, err
	}
	return copie(c), nil
}

func (r *Registre) resteUnAdmin() bool {
	for _, c := range r.m {
		if c.Admin && !c.Inactif {
			return true
		}
	}
	return false
}

// DefinirMdp change le mot de passe : les sessions ouvertes du compte tombent.
func (r *Registre) DefinirMdp(nom, mdp string) error {
	_, err := r.ChangerMdp(nom, mdp, false)
	return err
}

// ReinitialiserMdp : le mot de passe est changé par un autre que le titulaire. Tous les jetons du
// compte tombent : qui a volé un jeton ne garde pas l'accès.
func (r *Registre) ReinitialiserMdp(nom, mdp string) error {
	_, err := r.ChangerMdp(nom, mdp, true)
	return err
}

// ChangerMdp change le mot de passe, et retire tous les jetons du compte si retirerJetons. Renvoie le
// nombre de jetons retirés ; en cas d'erreur, rien n'a changé.
func (r *Registre) ChangerMdp(nom, mdp string, retirerJetons bool) (int, error) {
	retires := 0
	_, err := r.Modifier(nom, func(c *Compte) error {
		if err := MdpAcceptable(c.Nom, mdp, c.Admin); err != nil {
			return err
		}
		c.Mdp = empreinteMdp(mdp)
		c.Version++
		c.DoitChanger = false
		if retirerJetons {
			retires, c.Jetons = len(c.Jetons), nil
		}
		return nil
	})
	return retires, err
}

// ExigerChangement : ce compte devra changer son mot de passe avant tout le reste.
func (r *Registre) ExigerChangement(nom string) error {
	_, err := r.Modifier(nom, func(c *Compte) error {
		c.DoitChanger = true
		return nil
	})
	return err
}

func (r *Registre) Supprimer(nom string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	nom = Normaliser(nom)
	c, ok := r.m[nom]
	if !ok {
		return ErrInconnu
	}
	delete(r.m, nom)
	if c.Admin && !c.Inactif && !r.resteUnAdmin() {
		r.m[nom] = c
		return ErrDernierAdmin
	}
	if err := r.ecrire(); err != nil {
		r.m[nom] = c
		return err
	}
	return nil
}

// OublierUnivers retire de tous les comptes ce qui tenait à un univers supprimé : rôles, admission,
// PJ.
func (r *Registre) OublierUnivers(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	change := false
	for _, c := range r.m {
		if _, ok := c.Roles[id]; ok {
			delete(c.Roles, id)
			change = true
		}
		if _, ok := c.Persos[id]; ok {
			delete(c.Persos, id)
			change = true
		}
		if i := slices.Index(c.Admis, id); i >= 0 {
			c.Admis = slices.Delete(c.Admis, i, i+1)
			change = true
		}
	}
	if !change {
		return nil
	}
	return r.ecrire()
}

func empreinteJeton(clair string) string {
	h := sha256.Sum256([]byte(clair))
	return hex.EncodeToString(h[:])
}

// NouveauJeton crée un jeton d'API pour le compte et le renvoie en clair, une seule fois.
func (r *Registre) NouveauJeton(nom, libelle string) (string, Jeton, error) {
	b := make([]byte, 28)
	_, _ = rand.Read(b)
	clair := prefixe + hex.EncodeToString(b[4:])
	libelle = strings.TrimSpace(libelle)
	if rs := []rune(libelle); len(rs) > 60 {
		libelle = string(rs[:60])
	}
	j := Jeton{ID: hex.EncodeToString(b[:4]), Nom: libelle, Empreinte: empreinteJeton(clair), Cree: maintenant()}
	_, err := r.Modifier(nom, func(c *Compte) error {
		if len(c.Jetons) >= jetonsMax {
			return ErrTropDeJetons
		}
		c.Jetons = append(c.Jetons, j)
		return nil
	})
	if err != nil {
		return "", Jeton{}, err
	}
	return clair, j, nil
}

func (r *Registre) RetirerJeton(nom, id string) error {
	_, err := r.Modifier(nom, func(c *Compte) error {
		for i, j := range c.Jetons {
			if j.ID == id {
				c.Jetons = append(c.Jetons[:i], c.Jetons[i+1:]...)
				return nil
			}
		}
		return ErrInconnu
	})
	return err
}

// RetirerParJeton retire le jeton d'API donné en clair, quel que soit son compte : un poste qui se
// déconnecte du serveur rend ainsi son propre jeton.
func (r *Registre) RetirerParJeton(clair string) error {
	c, ok := r.ParJeton(clair)
	if !ok {
		return ErrInconnu
	}
	e := []byte(empreinteJeton(clair))
	_, err := r.Modifier(c.Nom, func(c *Compte) error {
		for i, j := range c.Jetons {
			if subtle.ConstantTimeCompare(e, []byte(j.Empreinte)) == 1 {
				c.Jetons = append(c.Jetons[:i], c.Jetons[i+1:]...)
				return nil
			}
		}
		return ErrInconnu
	})
	return err
}

// ParJeton renvoie le compte actif auquel appartient ce jeton d'API.
func (r *Registre) ParJeton(clair string) (Compte, bool) {
	if !strings.HasPrefix(clair, prefixe) {
		return Compte{}, false
	}
	e := []byte(empreinteJeton(clair))
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.m {
		for i := range c.Jetons {
			j := &c.Jetons[i]
			if subtle.ConstantTimeCompare(e, []byte(j.Empreinte)) == 1 {
				if c.Inactif {
					return Compte{}, false
				}
				if t := maintenant(); t-j.Utilise >= utiliseTous {
					j.Utilise = t
					_ = r.ecrire() // cette date n'est qu'une indication : le jeton reste valable sans elle
				}
				return copie(c), true
			}
		}
	}
	return Compte{}, false
}
