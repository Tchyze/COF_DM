package serveur

import (
	"crypto/subtle"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"github.com/Tchyze/cof_dm/internal/comptes"
)

// Un écran en direct reste branché longtemps : les droits vus à son ouverture ne valent pas pour
// toujours. Ils sont revus à intervalles réguliers, par une tâche à part, pour qu'un écran qui ne
// dit jamais rien n'y échappe pas. S'ils ont changé, l'écran est coupé : il se rebranche seul s'il
// en a encore le droit, et il repart alors de ce qu'il a le droit de voir (PJ d'un joueur compris).

// intervalleDroits : ce qu'un écran en direct garde au plus ses droits après leur retrait.
const intervalleDroits = 30 * time.Second

// droitsDuSuivi : la question à reposer pour cette requête, qui vient d'être admise. Rien qui écrive
// ou compte un essai : seulement le même raisonnement qu'à l'entrée.
func (s *Serveur) droitsDuSuivi(r *http.Request) func() bool {
	if s.acces.comptes == nil {
		return func() bool { return s.admisSansCompte(r) && s.fonctionsActives(r) }
	}
	id := s.store.ID()
	c, _ := compteDe(r)
	role, pj := c.Role(id), c.Persos[id]
	return func() bool {
		maintenant, connu, _ := s.acces.reconnaitre(r)
		return connu && maintenant.Role(id) == role && (role != comptes.Joueur || maintenant.Persos[id] == pj) && s.fonctionsActives(r)
	}
}

// admisSansCompte : hors mode serveur, les règles d'entrée de Serveur.ServeHTTP.
func (s *Serveur) admisSansCompte(r *http.Request) bool {
	if s.relie() && !boucleLocale(r) {
		return false
	}
	if j := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "); j != r.Header.Get("Authorization") {
		attendu, _ := s.reg.Meta("api_jeton")
		return attendu != "" && subtle.ConstantTimeCompare([]byte(j), []byte(attendu)) == 1
	}
	switch {
	case local(r):
		return true
	case !s.opt.ecouteReseau() || !hoteDuReseau(r):
		return false
	}
	return s.acces.session(r) || (s.pageJoueursOuverte() && routeLibre(r))
}

func (s *Serveur) fonctionsActives(r *http.Request) bool {
	for _, cle := range fonctionsDeRoute(r) {
		if !s.store.Active(cle) {
			return false
		}
	}
	return true
}

// surveiller coupe l'écran dès que encore répond non. La fonction rendue arrête la surveillance.
func (s *Serveur) surveiller(conn *websocket.Conn, r *http.Request, encore func() bool) (arreter func()) {
	fin := make(chan struct{})
	go func() {
		t := time.NewTicker(time.Duration(s.intervalleDroits.Load()))
		defer t.Stop()
		for {
			select {
			case <-fin:
				return
			case <-t.C:
				if !encore() {
					log.Printf("securite evenement=direct_coupe nom=%q ip=%s", auteurDe(r), s.acces.adresse(r))
					_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "droits retirés"), time.Now().Add(time.Second))
					_ = conn.Close()
					return
				}
			}
		}
	}()
	return func() { close(fin) }
}
