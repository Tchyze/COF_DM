package comptes

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"strings"
)

// Abonnement à l'agenda des séances. Un agenda en ligne (Google, Apple, Proton, Outlook…) relit
// l'adresse depuis ses propres serveurs, sans session ni cookie : le jeton secret dans l'adresse est
// sa seule clé. Un compte en a au plus un, qui ne donne que l'agenda (jamais l'API) ; il se change
// (l'ancien ne vaut plus) ou se retire, par son titulaire ou par l'administrateur.
//
// Le jeton est gardé deux fois : son empreinte, pour le reconnaître, et chiffré (clé des secrets du
// serveur, liée au compte), pour que la page du joueur puisse le lui remontrer quand il ajoute
// l'agenda sur un autre appareil.

const prefixeAgenda = "cofag_"

// ErrAgendaAdmin : un administrateur n'a rien à lire dans les univers.
var ErrAgendaAdmin = errors.New("un compte administrateur n'a pas d'agenda")

// JetonAgenda : l'adresse d'abonnement d'un compte.
type JetonAgenda struct {
	Empreinte string  `json:"empreinte"`
	Chiffre   string  `json:"chiffre"`
	Cree      float64 `json:"cree"`
	Utilise   float64 `json:"utilise,omitempty"` // dernière lecture, à une heure près
}

// NouvelAgenda tire un nouveau jeton d'agenda pour le compte (l'ancien ne vaut plus) et le rend en
// clair.
func (r *Registre) NouvelAgenda(nom string) (string, JetonAgenda, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", JetonAgenda{}, err
	}
	clair := prefixeAgenda + base64.RawURLEncoding.EncodeToString(b)
	var j JetonAgenda
	_, err := r.Modifier(nom, func(c *Compte) error {
		if c.Admin {
			return ErrAgendaAdmin
		}
		chiffre, err := r.chiffrer(c, []byte(clair))
		if err != nil {
			return err
		}
		j = JetonAgenda{Empreinte: empreinteJeton(clair), Chiffre: chiffre, Cree: maintenant()}
		c.Agenda = &j
		return nil
	})
	if err != nil {
		return "", JetonAgenda{}, err
	}
	return clair, j, nil
}

// LireAgenda rend le jeton d'agenda du compte en clair ("" : il n'en a pas, ou il est illisible).
func (r *Registre) LireAgenda(nom string) (string, JetonAgenda) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.m[Normaliser(nom)]
	if !ok || c.Agenda == nil {
		return "", JetonAgenda{}
	}
	clair, err := r.dechiffrer(c, c.Agenda.Chiffre)
	if err != nil || subtle.ConstantTimeCompare([]byte(empreinteJeton(string(clair))), []byte(c.Agenda.Empreinte)) != 1 {
		return "", *c.Agenda
	}
	return string(clair), *c.Agenda
}

// RetirerAgenda retire l'adresse d'abonnement du compte (rien s'il n'en a pas).
func (r *Registre) RetirerAgenda(nom string) error {
	_, err := r.Modifier(nom, func(c *Compte) error {
		c.Agenda = nil
		return nil
	})
	return err
}

// ParAgenda renvoie le compte actif auquel appartient ce jeton d'agenda.
func (r *Registre) ParAgenda(clair string) (Compte, bool) {
	if !strings.HasPrefix(clair, prefixeAgenda) || len(clair) > 100 {
		return Compte{}, false
	}
	e := []byte(empreinteJeton(clair))
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.m {
		if c.Agenda == nil || subtle.ConstantTimeCompare(e, []byte(c.Agenda.Empreinte)) != 1 {
			continue
		}
		if c.Inactif || c.Admin {
			return Compte{}, false
		}
		if t := maintenant(); t-c.Agenda.Utilise >= utiliseTous {
			c.Agenda.Utilise = t
			_ = r.ecrire() // une indication : le jeton reste valable sans elle
		}
		return copie(c), true
	}
	return Compte{}, false
}
