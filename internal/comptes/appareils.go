package comptes

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"slices"
	"strings"
)

// Appareils qui reçoivent les notifications d'un compte sur un serveur (Web Push) : un téléphone,
// une tablette, un navigateur, chacun inscrit par son titulaire depuis cet appareil. Le navigateur
// donne une adresse d'envoi (chez Google, Apple ou Mozilla) et deux clés ; le serveur y dépose des
// messages chiffrés. L'administrateur voit les appareils d'un compte et peut les retirer.

// MaxAppareils : au-delà, le plus ancien laisse sa place.
const MaxAppareils = 10

// ErrAppareilAdmin : un administrateur ne reçoit pas les notifications des univers.
var ErrAppareilAdmin = errors.New("un compte administrateur ne reçoit pas de notifications")

type Appareil struct {
	ID       string  `json:"id"`
	Nom      string  `json:"nom"` // « Android · Chrome », tel que la page l'a décrit
	Adresse  string  `json:"adresse"`
	P256dh   string  `json:"p256dh"`
	Auth     string  `json:"auth"`
	Cree     float64 `json:"cree"`
	Recu     float64 `json:"recu,omitempty"`     // dernier envoi accepté
	Echecs   int     `json:"echecs,omitempty"`   // envois refusés d'affilée
	Derniere string  `json:"derniere,omitempty"` // dernière erreur, pour l'administrateur
}

// InscrireAppareil ajoute l'appareil au compte, ou le remplace s'il est déjà inscrit (même adresse).
func (r *Registre) InscrireAppareil(nom string, a Appareil) (Appareil, error) {
	a.Nom = strings.TrimSpace(a.Nom)
	if len([]rune(a.Nom)) > 60 {
		a.Nom = string([]rune(a.Nom)[:60])
	}
	if a.Nom == "" {
		a.Nom = "Appareil"
	}
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return a, err
	}
	_, err := r.Modifier(nom, func(c *Compte) error {
		if c.Admin {
			return ErrAppareilAdmin
		}
		c.Appareils = slices.DeleteFunc(c.Appareils, func(x Appareil) bool {
			if x.Adresse == a.Adresse {
				b, _ = hex.DecodeString(x.ID) // le même appareil garde son numéro
				return true
			}
			return false
		})
		a.ID, a.Cree = hex.EncodeToString(b), maintenant()
		c.Appareils = append(c.Appareils, a)
		if len(c.Appareils) > MaxAppareils {
			c.Appareils = c.Appareils[len(c.Appareils)-MaxAppareils:]
		}
		return nil
	})
	return a, err
}

// RetirerAppareil retire un appareil du compte (rien s'il n'y est pas).
func (r *Registre) RetirerAppareil(nom, id string) error {
	_, err := r.Modifier(nom, func(c *Compte) error {
		c.Appareils = slices.DeleteFunc(c.Appareils, func(x Appareil) bool { return x.ID == id })
		return nil
	})
	return err
}

// NoterEnvoi : ce que le service d'envoi a répondu pour cet appareil. perime : l'abonnement n'existe
// plus (désinstallé, refusé) et l'appareil est retiré ; sinon, cinq refus d'affilée le retirent aussi.
func (r *Registre) NoterEnvoi(nom, id string, ok, perime bool, erreur string) {
	_, _ = r.Modifier(nom, func(c *Compte) error {
		for i := range c.Appareils {
			x := &c.Appareils[i]
			if x.ID != id {
				continue
			}
			if ok {
				x.Recu, x.Echecs, x.Derniere = maintenant(), 0, ""
				return nil
			}
			x.Echecs++
			x.Derniere = erreur
			if perime || x.Echecs >= 5 {
				c.Appareils = slices.Delete(c.Appareils, i, i+1)
			}
			return nil
		}
		return nil
	})
}

// ReglerNotifications : les genres de notification que le compte ne veut pas sur ses appareils (la
// cloche, elle, les garde toutes).
func (r *Registre) ReglerNotifications(nom string, coupees []string) error {
	_, err := r.Modifier(nom, func(c *Compte) error {
		c.NotifsCoupees = nil
		for _, g := range coupees {
			if g = strings.TrimSpace(g); g != "" && len(g) <= 20 && !slices.Contains(c.NotifsCoupees, g) {
				c.NotifsCoupees = append(c.NotifsCoupees, g)
			}
		}
		return nil
	})
	return err
}
