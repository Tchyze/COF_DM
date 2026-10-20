// Package synchro est le client de la synchronisation : l'application de bureau s'en sert pour se
// relier à un serveur COF DM, en tirer une copie d'un univers et la tenir à jour (protocole décrit
// dans internal/store/protocole.go, routes dans docs/API.md, « Synchronisation »).
package synchro

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Tchyze/cof_dm/internal/store"
)

// Client parle à un serveur, pour un univers, avec le jeton d'API d'un compte.
type Client struct {
	Base    string // https://jdr.example.org
	Jeton   string
	Univers string // identifiant de l'univers sur le serveur
	http    *http.Client
}

// ErrInjoignable : le serveur ne répond pas (hors ligne).
var ErrInjoignable = errors.New("serveur injoignable")

// ErrServeur : le serveur a répondu par un refus ; son message est lisible.
type ErrServeur struct {
	Code int
	Msg  string
}

func (e ErrServeur) Error() string { return e.Msg }

// Adresse vérifie et nettoie l'adresse d'un serveur. Le HTTP sans chiffrement n'est accepté que pour
// cet ordinateur ou le réseau local : ailleurs, le mot de passe et le jeton passeraient en clair.
func Adresse(brute string) (string, error) {
	brute = strings.TrimRight(strings.TrimSpace(brute), "/")
	if !strings.Contains(brute, "://") {
		brute = "https://" + brute
	}
	u, err := url.Parse(brute)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("adresse de serveur invalide (exemple : https://jdr.example.org)")
	}
	if u.Scheme == "http" && !hoteLocal(u.Hostname()) {
		return "", errors.New("un serveur sur Internet se joint en HTTPS : le mot de passe passerait en clair")
	}
	return strings.TrimRight(u.String(), "/"), nil
}

func hoteLocal(h string) bool {
	if h == "localhost" {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast())
}

// Nouveau : un client pour un univers d'un serveur.
func Nouveau(base, jeton, univers string) *Client {
	return &Client{Base: base, Jeton: jeton, Univers: univers, http: &http.Client{Timeout: 2 * time.Minute}}
}

// Univers : un univers du serveur où le compte a un rôle.
type Univers struct {
	ID   string `json:"id"`
	Nom  string `json:"nom"`
	Role string `json:"role"`
}

// Connecter ouvre une session avec le compte, crée un jeton d'API au nom de ce poste, puis ferme la
// session : seul le jeton est gardé (jamais le mot de passe). Les jetons que ce poste aurait laissés
// (serveur injoignable quand il l'a quitté) sont retirés d'abord : un poste n'en a qu'un. Renvoie
// aussi les univers du compte.
func Connecter(base, nom, mdp, poste string) (jeton string, univers []Univers, err error) {
	jar, _ := cookiejar.New(nil)
	c := &Client{Base: base, http: &http.Client{Timeout: time.Minute, Jar: jar}}
	if err := c.appel("POST", "/api/acces/connexion", map[string]string{"nom": nom, "mdp": mdp}, nil); err != nil {
		return "", nil, err
	}
	libelle := LibelleJeton(poste)
	var compte struct {
		Jetons []struct {
			ID  string `json:"id"`
			Nom string `json:"nom"`
		} `json:"jetons"`
	}
	if c.appel("GET", "/api/compte", nil, &compte) == nil {
		for _, ancien := range compte.Jetons {
			if ancien.Nom == libelle {
				_ = c.appel("DELETE", "/api/compte/jetons/"+url.PathEscape(ancien.ID), nil, nil)
			}
		}
	}
	var j struct {
		Jeton string `json:"jeton"`
	}
	err = c.appel("POST", "/api/compte/jetons", map[string]string{"nom": libelle}, &j)
	_ = c.appel("POST", "/api/acces/deconnexion", nil, nil)
	if err != nil {
		return "", nil, err
	}
	if j.Jeton == "" {
		return "", nil, errors.New("le serveur n'a pas donné de jeton d'API")
	}
	if univers, err = Nouveau(base, j.Jeton, "").MesUnivers(); err != nil {
		_ = Rendre(base, j.Jeton)
		return "", nil, err
	}
	return j.Jeton, univers, nil
}

// LibelleJeton : le nom du jeton d'API de ce poste sur le serveur, tel que « Mon compte » le montre
// (60 caractères au plus, comme le serveur le garde).
func LibelleJeton(poste string) string {
	l := "Synchronisation — " + poste
	if r := []rune(l); len(r) > 60 {
		l = string(r[:60])
	}
	return l
}

// MesUnivers : les univers du serveur où le compte du jeton a un rôle. Sert aussi à vérifier que le
// serveur répond et reconnaît encore ce poste.
func (c *Client) MesUnivers() ([]Univers, error) {
	l, _, err := c.mesUnivers()
	return l, err
}

// mesUnivers : les univers du compte, et les numéros du serveur s'il les y publie (nil : un serveur
// d'avant la 1.2.3).
func (c *Client) mesUnivers() ([]Univers, *Versions, error) {
	var m struct {
		Univers   []Univers `json:"univers"`
		Version   *string   `json:"version"`
		Protocole int       `json:"protocole"`
		Modele    int       `json:"modele"`
	}
	if err := c.appel("GET", "/api/mes-univers", nil, &m); err != nil {
		return nil, nil, err
	}
	if m.Univers == nil {
		m.Univers = []Univers{}
	}
	if m.Version == nil {
		return m.Univers, nil, nil
	}
	return m.Univers, &Versions{Version: *m.Version, Protocole: m.Protocole, Modele: m.Modele}, nil
}

// Rendre retire du serveur le jeton de ce poste, qui se déconnecte. Un jeton que le serveur ne
// connaît plus est déjà rendu. Sans réponse du serveur, le jeton y reste valable : il se retire à la
// connexion suivante de ce poste, ou à la main, dans « Mon compte ».
func Rendre(base, jeton string) error {
	c := Nouveau(base, jeton, "")
	c.http.Timeout = 10 * time.Second
	err := c.appel("DELETE", "/api/compte/jeton", nil, nil)
	var se ErrServeur
	if errors.As(err, &se) && se.Code == http.StatusUnauthorized {
		return nil
	}
	return err
}

// requete prépare une requête : jeton, univers visé, en-tête des écritures.
func (c *Client) requete(methode, chemin string, corps io.Reader) (*http.Request, error) {
	r, err := http.NewRequest(methode, c.Base+chemin, corps)
	if err != nil {
		return nil, err
	}
	if c.Jeton != "" {
		r.Header.Set("Authorization", "Bearer "+c.Jeton)
	}
	if c.Univers != "" {
		r.Header.Set("X-COF-Univers", c.Univers)
	}
	r.Header.Set("X-COF-DM", "1")
	r.Header.Set("X-COF-Auteur", "synchronisation")
	ici := Ici()
	r.Header.Set(EnteteVersion, ici.Version)
	r.Header.Set(EnteteProtocole, strconv.Itoa(ici.Protocole))
	r.Header.Set(EnteteModele, strconv.Itoa(ici.Modele))
	return r, nil
}

func (c *Client) faire(r *http.Request) (*http.Response, error) {
	rep, err := c.http.Do(r)
	if err != nil {
		return nil, fmt.Errorf("%w : %v", ErrInjoignable, err)
	}
	if rep.StatusCode >= 400 {
		defer rep.Body.Close()
		var e struct {
			Detail string `json:"detail"`
		}
		b, _ := io.ReadAll(io.LimitReader(rep.Body, 1<<20))
		if json.Unmarshal(b, &e) != nil || e.Detail == "" {
			e.Detail = fmt.Sprintf("le serveur répond %d", rep.StatusCode)
		}
		return nil, ErrServeur{rep.StatusCode, e.Detail}
	}
	return rep, nil
}

// appel : une requête JSON, réponse décodée dans sortie (nil : ignorée).
func (c *Client) appel(methode, chemin string, entree, sortie any) error {
	var corps io.Reader
	if entree != nil {
		b, err := json.Marshal(entree)
		if err != nil {
			return err
		}
		corps = bytes.NewReader(b)
	}
	r, err := c.requete(methode, chemin, corps)
	if err != nil {
		return err
	}
	if entree != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	rep, err := c.faire(r)
	if err != nil {
		return err
	}
	defer rep.Body.Close()
	if sortie == nil {
		return nil
	}
	return json.NewDecoder(rep.Body).Decode(sortie)
}

// Copier inscrit une copie de l'univers au nom de ce poste et l'écrit dans fichier (qui ne doit pas
// exister). Renvoie son numéro.
func (c *Client) Copier(nom, fichier string) (int64, error) {
	b, _ := json.Marshal(map[string]string{"nom": nom})
	r, err := c.requete("POST", "/api/sync/copie", bytes.NewReader(b))
	if err != nil {
		return 0, err
	}
	r.Header.Set("Content-Type", "application/json")
	rep, err := c.faire(r)
	if err != nil {
		return 0, err
	}
	defer rep.Body.Close()
	n, _ := strconv.ParseInt(rep.Header.Get("X-COF-Replique"), 10, 64)
	if n <= 0 {
		return 0, errors.New("le serveur n'a pas donné de numéro de copie")
	}
	f, err := os.OpenFile(fichier, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return 0, err
	}
	if _, err := io.Copy(f, rep.Body); err != nil {
		f.Close()
		os.Remove(fichier)
		return 0, fmt.Errorf("%w : %v", ErrInjoignable, err)
	}
	if err := f.Close(); err != nil {
		os.Remove(fichier)
		return 0, err
	}
	return n, nil
}

// Deposer ajoute au serveur l'univers du fichier donné, sous ce nom (compte administrateur), et
// renvoie son identifiant sur le serveur.
func (c *Client) Deposer(nom, fichier string) (string, error) {
	f, err := os.Open(fichier)
	if err != nil {
		return "", err
	}
	defer f.Close()
	r, err := c.requete("POST", "/api/univers/deposer?nom="+url.QueryEscape(nom), f)
	if err != nil {
		return "", err
	}
	r.Header.Del("X-COF-Univers")
	r.Header.Set("Content-Type", "application/octet-stream")
	rep, err := c.faire(r)
	if err != nil {
		return "", err
	}
	defer rep.Body.Close()
	var etat struct {
		Courant struct {
			ID string `json:"id"`
		} `json:"courant"`
	}
	if err := json.NewDecoder(rep.Body).Decode(&etat); err != nil || etat.Courant.ID == "" {
		return "", errors.New("réponse du serveur illisible après le dépôt")
	}
	return etat.Courant.ID, nil
}

// Bilan d'un échange.
type Bilan struct {
	Envoyes  int `json:"envoyes"`
	Recus    int `json:"recus"`
	Conflits int `json:"conflits"`
	Images   int `json:"images"`
	// EnAttente : les objets changés sur cette copie qui attendent la mise à jour du serveur (échange
	// en réception seule) ; RecusEnAttente : les objets reçus que cette copie n'a pas pu appliquer.
	EnAttente      int `json:"en_attente"`
	RecusEnAttente int `json:"recus_en_attente"`
}

// Echanger : un échange complet pour la copie st (voir le protocole), puis les images : celles que le
// serveur attend partent, celles qui manquent ici arrivent. envoyer false : réception seule (le
// serveur est plus ancien que ce bureau) ; les changements de la copie attendent, rien n'est perdu.
func (c *Client) Echanger(st *store.Store, envoyer bool) (Bilan, error) {
	var b Bilan
	curseur, chs, err := st.Preparer()
	if err != nil {
		return b, err
	}
	envoye, _ := st.Meta(store.MetaSyncEnvoye)
	avant, _ := strconv.ParseInt(envoye, 10, 64)
	partent, jusqua := chs, store.EnvoyeJusqua(chs, avant)
	if !envoyer {
		partent, jusqua = []store.Changement{}, avant
		b.EnAttente = store.ObjetsChanges(chs)
	}
	var rep store.Reponse
	if err := c.appel("POST", "/api/sync", map[string]any{"replique": st.Replique(), "curseur": curseur, "changements": partent}, &rep); err != nil {
		return b, err
	}
	if err := st.Integrer(rep, jusqua); err != nil {
		return b, err
	}
	b.RecusEnAttente = st.RecusEnAttente()
	for _, ch := range partent {
		if ch.Objet != store.ObjJournal {
			b.Envoyes++
		}
	}
	for _, ch := range rep.Changements {
		if ch.Objet != store.ObjJournal {
			b.Recus++
		}
	}
	b.Conflits = len(rep.Conflits)
	for _, f := range rep.ImagesAttendues {
		contenu, _, err := st.ContenuMedia(f)
		if err != nil || len(contenu) == 0 {
			continue // ni ici non plus : une autre copie l'enverra
		}
		r, err := c.requete("PUT", "/api/sync/images/"+url.PathEscape(f), bytes.NewReader(contenu))
		if err != nil {
			return b, err
		}
		rep, err := c.faire(r)
		if err != nil {
			return b, err
		}
		rep.Body.Close()
		b.Images++
	}
	manquantes, err := st.ImagesSansContenu()
	if err != nil {
		return b, err
	}
	for _, f := range manquantes {
		r, err := c.requete("GET", "/medias/"+url.PathEscape(f), nil)
		if err != nil {
			return b, err
		}
		rep, err := c.faire(r)
		var se ErrServeur
		if errors.As(err, &se) && se.Code == 404 {
			continue // le serveur ne l'a pas encore
		}
		if err != nil {
			return b, err
		}
		contenu, err := io.ReadAll(io.LimitReader(rep.Body, 32<<20))
		rep.Body.Close()
		if err != nil {
			return b, fmt.Errorf("%w : %v", ErrInjoignable, err)
		}
		if err := st.ContenuAttendu(f, contenu); err != nil {
			return b, err
		}
		b.Images++
	}
	return b, nil
}
