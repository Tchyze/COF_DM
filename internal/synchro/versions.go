package synchro

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/Tchyze/cof_dm/internal/majour"
	"github.com/Tchyze/cof_dm/internal/store"
)

// VersionApplication : la version de cette application, annoncée au serveur (réglée au lancement).
var VersionApplication = "dev"

// En-têtes par lesquels le bureau annonce ses numéros au serveur, à chaque requête.
const (
	EnteteVersion   = "X-COF-Version"
	EnteteProtocole = "X-COF-Protocole"
	EnteteModele    = "X-COF-Modele"
)

// Versions : ce qu'une application publie d'elle-même (GET /api/sante, en-têtes du bureau).
// Protocole et Modele : voir internal/store/versions.go.
type Versions struct {
	Version   string `json:"version"`
	Protocole int    `json:"protocole"`
	Modele    int    `json:"modele"`
}

// Ici : les numéros de cette application.
func Ici() Versions {
	return Versions{Version: VersionApplication, Protocole: store.VersionProtocole, Modele: store.VersionModele}
}

// Lire : les numéros publiés par l'autre côté, textes vides s'il ne les publie pas. Une version
// d'avant les numéros a le protocole 1 et le modèle de sa version : 1 depuis la 1.2.2, 0 avant.
// Une version de développement sans numéros est prise pour la 1.2.2.
func Lire(version, protocole, modele string) Versions {
	v := Versions{Version: version, Protocole: 1, Modele: 1}
	if n, err := strconv.Atoi(strings.TrimSpace(protocole)); err == nil && n > 0 {
		v.Protocole = n
	}
	if n, err := strconv.Atoi(strings.TrimSpace(modele)); err == nil && n >= 0 {
		v.Modele = n
		return v
	}
	if lue, ok := majour.Lire(version); ok {
		if base, _ := majour.Lire("1.2.2"); majour.Comparer(lue, base) < 0 {
			v.Modele = 0
		}
	}
	return v
}

// DeLaRequete : les numéros qu'un bureau annonce dans ses en-têtes (vides : un bureau d'avant les
// numéros, ou un navigateur).
func DeLaRequete(r *http.Request) Versions {
	return Lire(r.Header.Get(EnteteVersion), r.Header.Get(EnteteProtocole), r.Header.Get(EnteteModele))
}

// Versions : la version et les numéros du serveur. Ils viennent de GET /api/mes-univers, qui ne
// vise aucun univers ; un serveur d'avant la 1.2.3 ne les y publie pas : sa version se lit alors
// dans GET /api/sante d'un de ses univers (un compte qui en a plusieurs doit en viser un).
func (c *Client) Versions() (Versions, error) {
	us, v, err := c.mesUnivers()
	if err != nil {
		return Versions{}, err
	}
	if v != nil {
		return *v, nil
	}
	vise := *c
	if vise.Univers == "" && len(us) > 0 {
		vise.Univers = us[0].ID
	}
	return vise.sante()
}

// sante : les numéros publiés par GET /api/sante.
func (c *Client) sante() (Versions, error) {
	var s struct {
		Version   string `json:"version"`
		Protocole *int   `json:"protocole"`
		Modele    *int   `json:"modele"`
	}
	if err := c.appel("GET", "/api/sante", nil, &s); err != nil {
		return Versions{}, err
	}
	texte := func(p *int) string {
		if p == nil {
			return ""
		}
		return strconv.Itoa(*p)
	}
	return Lire(s.Version, texte(s.Protocole), texte(s.Modele)), nil
}

// Ecart : comment deux applications se situent l'une par rapport à l'autre pour l'échange.
type Ecart int

const (
	Accord        Ecart = iota // même protocole, même modèle
	ServeurRecent              // le serveur a un protocole ou un modèle plus récent : mettre le bureau à jour
	BureauRecent               // le bureau est plus récent : il attend la mise à jour du serveur
)

// Comparer : l'écart entre ce bureau et un serveur. Un protocole différent l'emporte sur le modèle.
func Comparer(bureau, serveur Versions) Ecart {
	switch {
	case serveur.Protocole > bureau.Protocole:
		return ServeurRecent
	case serveur.Protocole < bureau.Protocole:
		return BureauRecent
	case serveur.Modele > bureau.Modele:
		return ServeurRecent
	case serveur.Modele < bureau.Modele:
		return BureauRecent
	}
	return Accord
}

// Affichee : la version à montrer (« 1.2.2 », ou « une version plus ancienne » quand elle est
// inconnue).
func (v Versions) Affichee() string {
	if lue, ok := majour.Lire(v.Version); ok {
		return lue.String()
	}
	if strings.TrimSpace(v.Version) != "" {
		return v.Version
	}
	return "version inconnue"
}
