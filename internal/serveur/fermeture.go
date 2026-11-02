package serveur

import (
	"errors"
	"log"
	"sync"
	"time"
)

// Les verrous de l'application :
//   - a.mu protège a.tous, a.st, a.srv et a.fermetures, et n'est tenu que quelques instructions :
//     jamais pendant une requête, une lecture du réseau, une sauvegarde ou une ouverture de base ;
//   - a.gestion : une seule opération à la fois sur les univers (ouvrir, fermer, renommer, archiver,
//     supprimer, restaurer), tenu d'un bout à l'autre de l'opération. Toujours pris avant a.mu.
//
// Pour changer ces champs, il faut tenir les deux ; pour les lire, l'un des deux suffit. Une requête
// servie par un Serveur ne prend jamais a.gestion : une fermeture attend les requêtes de l'univers,
// et elles ne l'attendent pas.

// delaiFermeture : ce qu'une opération attend les requêtes en cours de l'univers à fermer.
const delaiFermeture = 30 * time.Second

// delaiArret : à l'arrêt d'un serveur, docker stop ne laisse que 10 secondes par défaut.
const delaiArret = 5 * time.Second

const (
	msgOccupe    = "Des requêtes sont encore en cours sur cet univers : rien n'a été modifié, réessayez dans un instant."
	msgFermeture = "Cet univers est momentanément indisponible : réessayez dans un instant."
)

// errOccupe : l'univers à fermer sert encore des requêtes, il reste ouvert (voir erreurStore).
var errOccupe = errors.New("requêtes encore en cours sur l'univers")

// requetes compte le travail en cours sur un univers. entrer se fait sous a.mu, pendant que l'univers
// est servi : une fois retiré de a.tous, le compte ne peut plus que descendre.
type requetes struct {
	mu   sync.Mutex
	n    int
	vide chan struct{} // fermé quand le compte retombe à zéro ; nil si personne n'attend
}

func (q *requetes) entrer() {
	q.mu.Lock()
	q.n++
	q.mu.Unlock()
}

func (q *requetes) sortir() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.n--
	if q.n == 0 && q.vide != nil {
		close(q.vide)
		q.vide = nil
	}
}

// attendre : vrai dès que le compte est à zéro, faux après le délai ou si abandon se ferme.
func (q *requetes) attendre(delai time.Duration, abandon <-chan struct{}) bool {
	q.mu.Lock()
	if q.n == 0 {
		q.mu.Unlock()
		return true
	}
	if q.vide == nil {
		q.vide = make(chan struct{})
	}
	vide := q.vide
	q.mu.Unlock()
	t := time.NewTimer(delai)
	defer t.Stop()
	select {
	case <-vide:
		return true
	case <-t.C:
		return false
	case <-abandon:
		return false
	}
}

// suspendre retire un univers du service et attend la fin de ses requêtes (a.gestion tenu). Faux si
// elles durent : il est remis en service et rien n'a changé. Vrai : plus personne ne s'en sert ; à
// l'appelant de le fermer (clore) ou de le remettre (remettre), puis d'appeler fermetureFinie. Si
// c'était l'univers par défaut, un autre le devient en attendant.
func (a *Application) suspendre(o *universOuvert) bool {
	a.mu.Lock()
	o.defaut = a.st == o.st
	delete(a.tous, o.st.ID())
	a.fermetures[o.st.ID()] = true
	if o.defaut {
		a.st, a.srv = nil, nil
		if l := a.ordre(); len(l) > 0 {
			a.st, a.srv = l[0].st, l[0].srv
		}
	}
	st := a.st
	a.mu.Unlock()
	if o.defaut && st != nil {
		_ = a.reg.SetMeta(regUnivers, st.Chemin())
	}
	if o.requetes.attendre(a.delaiFermeture, a.fin) {
		return true
	}
	a.remettre(o)
	return false
}

// remettre : un univers suspendu est de nouveau servi, comme avant (a.gestion tenu).
func (a *Application) remettre(o *universOuvert) {
	a.mu.Lock()
	a.tous[o.st.ID()] = o
	delete(a.fermetures, o.st.ID())
	if o.defaut {
		a.st, a.srv = o.st, o.srv
	}
	a.mu.Unlock()
	if o.defaut {
		_ = a.reg.SetMeta(regUnivers, o.st.Chemin())
	}
}

// fermetureFinie : l'opération est terminée, l'univers n'est plus « momentanément indisponible » :
// il a été rouvert, ou il n'existe plus (a.gestion tenu).
func (a *Application) fermetureFinie(id string) {
	a.mu.Lock()
	delete(a.fermetures, id)
	a.mu.Unlock()
}

// redevenirDefaut : l'univers rouvert reprend sa place d'univers par défaut (a.gestion tenu).
func (a *Application) redevenirDefaut(o *universOuvert) {
	a.mu.Lock()
	a.st, a.srv = o.st, o.srv
	a.mu.Unlock()
	_ = a.reg.SetMeta(regUnivers, o.st.Chemin())
}

// clore ferme pour de bon un univers suspendu : ses écrans sont coupés, puis sa base.
func (a *Application) clore(o *universOuvert) {
	if o.srv != nil {
		o.srv.Fermer()
	}
	_ = o.st.Fermer()
}

// retirer ferme un univers servi, après sa sauvegarde automatique, faite quand plus rien ne
// l'écrit (a.gestion tenu). Faux : il sert encore des requêtes, il reste ouvert.
func (a *Application) retirer(o *universOuvert) bool {
	if !a.suspendre(o) {
		return false
	}
	if _, err := sauvegardeAuto(o.st, a.dossierSauvegardes(o.st)); err != nil {
		log.Printf("sauvegarde automatique : %v", err)
	}
	a.clore(o)
	return true
}
