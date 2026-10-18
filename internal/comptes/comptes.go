// Package comptes garde les comptes du mode serveur : nom d'utilisateur, mot de passe haché, rôle
// dans chaque univers, jetons d'API. Ils tiennent au serveur, pas à un univers.
package comptes

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Rôles d'un compte dans un univers. L'administrateur, lui, l'est pour tout le serveur.
const (
	MJ       = "mj"       // lit et modifie l'univers
	Auditeur = "auditeur" // lit tout l'univers, secrets du MJ et journal compris, sans rien modifier
	Joueur   = "joueur"   // ne lit que les fiches partagées par le MJ, sans leurs champs réservés au MJ
)

// RoleValide : c'est un rôle qu'un compte peut avoir dans un univers.
func RoleValide(role string) bool { return role == MJ || role == Auditeur || role == Joueur }

const (
	iterations = 210000
	MdpMin     = 10
	prefixe    = "cofdm_"
	jetonsMax  = 20
)

var (
	ErrInconnu      = errors.New("compte inconnu")
	ErrExiste       = errors.New("ce nom est déjà pris")
	ErrNom          = errors.New("nom invalide : 3 à 32 caractères parmi a-z, 0-9, « - » et « _ »")
	ErrMdp          = errors.New("le mot de passe doit faire au moins 10 caractères")
	ErrRole         = errors.New("rôle inconnu : mj, auditeur ou joueur")
	ErrDernierAdmin = errors.New("il doit rester un administrateur actif")
	ErrTropDeJetons = errors.New("trop de jetons : retirez-en un d'abord")

	reNom = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{2,31}$`)
)

// Jeton d'API d'un compte. Seule son empreinte est gardée : il ne s'affiche qu'à sa création.
type Jeton struct {
	ID        string  `json:"id"`
	Nom       string  `json:"nom"`
	Empreinte string  `json:"empreinte"`
	Cree      float64 `json:"cree"`
}

type Compte struct {
	Nom     string            `json:"nom"`
	Mdp     string            `json:"mdp"` // « sel:empreinte » (PBKDF2-SHA256)
	Admin   bool              `json:"admin,omitempty"`
	Inactif bool              `json:"inactif,omitempty"`
	Roles   map[string]string `json:"roles,omitempty"`  // identifiant d'univers → rôle
	Persos  map[string]int64  `json:"persos,omitempty"` // identifiant d'univers → fiche du PJ que joue ce compte
	Jetons  []Jeton           `json:"jetons,omitempty"`
	Version int               `json:"version"` // change avec le mot de passe : les sessions ouvertes tombent
	Cree    float64           `json:"cree"`
}

// Role renvoie le rôle du compte dans un univers ("" : aucun accès). Un administrateur y est MJ.
func (c Compte) Role(univers string) string {
	if c.Admin {
		return MJ
	}
	return c.Roles[univers]
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
	for _, c := range l {
		r.m[c.Nom] = c
	}
	return r, nil
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
	x.Jetons = append([]Jeton(nil), c.Jetons...)
	return x
}

func hacher(mdp string, sel []byte) []byte {
	k, _ := pbkdf2.Key(sha256.New, mdp, sel, iterations, 32)
	return k
}

func empreinteMdp(mdp string) string {
	sel := make([]byte, 16)
	_, _ = rand.Read(sel)
	return hex.EncodeToString(sel) + ":" + hex.EncodeToString(hacher(mdp, sel))
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

func (r *Registre) Creer(nom, mdp string, admin bool) (Compte, error) {
	nom = Normaliser(nom)
	if !reNom.MatchString(nom) {
		return Compte{}, ErrNom
	}
	if len([]rune(mdp)) < MdpMin {
		return Compte{}, ErrMdp
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.m[nom]; ok {
		return Compte{}, ErrExiste
	}
	c := &Compte{Nom: nom, Mdp: empreinteMdp(mdp), Admin: admin, Version: 1, Cree: maintenant()}
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
	sel, attendu := r.leurre, []byte(nil)
	if ok {
		s, h, _ := strings.Cut(x.Mdp, ":")
		sel, _ = hex.DecodeString(s)
		attendu, _ = hex.DecodeString(h)
	}
	bon := subtle.ConstantTimeCompare(hacher(mdp, sel), attendu) == 1
	if !ok || !bon || x.Inactif || len(attendu) == 0 {
		return Compte{}, false
	}
	return x, true
}

// Modifier applique f au compte et l'enregistre. Le dernier administrateur actif ne peut ni perdre
// son rôle, ni être désactivé.
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
	for u, role := range x.Roles {
		switch role {
		case MJ, Auditeur, Joueur:
		case "":
			delete(x.Roles, u)
		default:
			return Compte{}, ErrRole
		}
	}
	for u, pj := range x.Persos {
		if pj <= 0 {
			delete(x.Persos, u)
		}
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
	if len([]rune(mdp)) < MdpMin {
		return ErrMdp
	}
	_, err := r.Modifier(nom, func(c *Compte) error {
		c.Mdp = empreinteMdp(mdp)
		c.Version++
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
		for _, j := range c.Jetons {
			if subtle.ConstantTimeCompare(e, []byte(j.Empreinte)) == 1 {
				if c.Inactif {
					return Compte{}, false
				}
				return copie(c), true
			}
		}
	}
	return Compte{}, false
}
