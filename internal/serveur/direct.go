package serveur

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"github.com/Tchyze/cof_dm/internal/synchro"
)

// Combat et dés en direct : quand l'univers ouvert est une copie reliée et connectée, le combat tourne
// sur le serveur et cette application y transmet chaque action ; l'écran ne change pas. Les joueurs
// suivent ainsi le combat aussitôt. Connexion perdue : rien ne continue ici, l'écran le dit et reprend
// seul au retour du serveur. Un combat lancé hors connexion reste local, invisible des joueurs, et part
// une fois terminé par la synchronisation (les combats en cours n'y entrent pas).

const (
	msgConnexionPerdue = "Connexion perdue : le combat reprendra dès le retour du serveur."
	attenteRetour      = 2 * time.Second
	finSuivi           = 40 * time.Second // sans nouvelles du serveur, le suivi est tenu pour mort
	pingSuivi          = 15 * time.Second
)

// clientDirect : le client vers le serveur, si cet univers y est relié et connecté ; nil sinon.
func (s *Serveur) clientDirect() *synchro.Client {
	c := s.connexionDirecte()
	if c == nil {
		return nil
	}
	return synchro.NouveauDirect(c.URL, c.Jeton, s.store.ID())
}

// connexionDirecte : la connexion au serveur par laquelle cet univers passe, s'il y en a une.
func (s *Serveur) connexionDirecte() *connexion {
	if s.opt.app == nil {
		return nil
	}
	_, _, c := s.opt.app.relieDe(s.store)
	return c
}

// noterDirect : ce qu'un appel dit du serveur ; la zone d'état de l'application le montre.
func (s *Serveur) noterDirect(err error) {
	s.opt.app.noterServeur(err)
}

func garderCorps(r *http.Request) []byte {
	b, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	r.Body = io.NopCloser(bytes.NewReader(b))
	return b
}

func repondreBrut(w http.ResponseWriter, code int, corps []byte) {
	sansCache(w)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_, _ = w.Write(corps)
}

func (h *salles) estDistant(id int64) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.distants[id]
}

// nombreDistants : les combats joués sur le serveur que ce poste suit.
func (h *salles) nombreDistants() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.distants)
}

func (h *salles) noterDistant(id int64, enCours bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if enCours {
		h.distants[id] = true
	} else {
		delete(h.distants, id)
	}
}

// combatLocal : le combat est-il rangé ici, et en cours ici (lancé hors connexion) ?
func (s *Serveur) combatLocal(id int64) (existe, enCours bool) {
	st, err := s.salles.lire(id)
	if err != nil {
		return false, false
	}
	return true, !st.Termine
}

// direct : la route d'un combat désigné par son numéro. Un combat en cours ici reste ici ; les autres
// se jouent sur le serveur. Celui-ci ne connaît pas un combat terminé ici, pas encore envoyé : il est
// lu ici, comme tout combat terminé quand le serveur ne répond pas.
func (s *Serveur) direct(local http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cl := s.clientDirect()
		if cl == nil {
			local(w, r)
			return
		}
		id, ok := idCombat(w, r)
		if !ok {
			return
		}
		existe, enCours := s.combatLocal(id)
		if enCours {
			local(w, r)
			return
		}
		corps := garderCorps(r)
		if r.Method == http.MethodPost && strings.Contains(string(corps), `"ajouter"`) {
			_, _ = s.opt.app.echangerDe(s.store) // la fiche ajoutée doit être sur le serveur, qui la lit
		}
		code, rep, err := cl.Relayer(r.Method, r.URL.RequestURI(), corps)
		switch {
		case err != nil:
			s.noterDirect(err)
			if existe && !s.salles.estDistant(id) {
				local(w, r)
				return
			}
			erreur(w, http.StatusServiceUnavailable, msgConnexionPerdue)
			return
		case code == http.StatusNotFound && existe:
			local(w, r)
			return
		case code == http.StatusUnauthorized:
			s.noterDirect(synchro.ErrServeur{Code: code, Msg: msgRefuse})
			erreur(w, http.StatusConflict, msgRefuse)
			return
		}
		s.noterDirect(nil)
		if code < 300 {
			s.apresDirect(r, id, rep)
		}
		repondreBrut(w, code, rep)
	}
}

// apresDirect : à la fin d'un combat (ou à sa suppression), la synchronisation part aussitôt : PV,
// chance et récapitulatif arrivent sur cet ordinateur.
func (s *Serveur) apresDirect(r *http.Request, id int64, rep []byte) {
	if r.Method == http.MethodDelete {
		s.salles.noterDistant(id, false)
		s.opt.app.reveiller()
		return
	}
	var c struct {
		Etat struct {
			Termine bool `json:"termine"`
		} `json:"etat"`
	}
	if json.Unmarshal(rep, &c) != nil {
		return
	}
	etaitEnCours := s.salles.estDistant(id)
	s.salles.noterDistant(id, !c.Etat.Termine)
	if c.Etat.Termine && (etaitEnCours || r.Method == http.MethodPost) {
		s.opt.app.reveiller()
	}
}

// combatCreerDirect lance le combat sur le serveur, après lui avoir envoyé les changements d'ici.
// Sans serveur joignable, ou de numéros différents, le combat reste local.
func (s *Serveur) combatCreerDirect(w http.ResponseWriter, r *http.Request) {
	cl := s.clientDirect()
	if cl == nil {
		s.combatCreer(w, r)
		return
	}
	corps := garderCorps(r)
	srv, err := cl.Versions()
	if err != nil {
		s.noterDirect(err)
		s.combatCreer(w, r)
		return
	}
	if synchro.Comparer(synchro.Ici(), srv) != synchro.Accord {
		s.combatCreer(w, r)
		return
	}
	if _, err := s.opt.app.echangerDe(s.store); err != nil {
		s.combatCreer(w, r)
		return
	}
	code, rep, err := cl.Relayer(r.Method, r.URL.RequestURI(), corps)
	if err != nil || code == http.StatusUnauthorized {
		if err != nil {
			s.noterDirect(err)
		}
		s.combatCreer(w, r)
		return
	}
	if code < 300 {
		var c struct {
			Combat struct {
				ID int64 `json:"id"`
			} `json:"combat"`
		}
		if json.Unmarshal(rep, &c) == nil && c.Combat.ID != 0 {
			s.salles.noterDistant(c.Combat.ID, true)
		}
	}
	repondreBrut(w, code, rep)
}

// combatsListerDirect : les combats du serveur et ceux qui ne sont qu'ici. Pour un même combat, le
// serveur fait foi : il l'a peut-être rouvert.
func (s *Serveur) combatsListerDirect(w http.ResponseWriter, r *http.Request) {
	cl := s.clientDirect()
	if cl == nil {
		s.combatsLister(w, r)
		return
	}
	code, rep, err := cl.Relayer(http.MethodGet, r.URL.RequestURI(), nil)
	if err != nil {
		s.noterDirect(err)
		s.combatsLister(w, r)
		return
	}
	var distants []resumeCombat
	if code != http.StatusOK || json.Unmarshal(rep, &distants) != nil {
		s.combatsLister(w, r)
		return
	}
	s.noterDirect(nil)
	locaux, err := s.listeCombats(r)
	if err != nil {
		erreurStore(w, err)
		return
	}
	vus := map[int64]bool{}
	for _, c := range distants {
		vus[c.ID] = true
		s.salles.noterDistant(c.ID, !c.Termine)
	}
	for _, c := range locaux {
		if !vus[c.ID] {
			distants = append(distants, c)
		}
	}
	trierCombats(distants)
	repondre(w, 200, distants)
}

func trierCombats(l []resumeCombat) {
	sort.SliceStable(l, func(i, j int) bool {
		if l[i].Termine != l[j].Termine {
			return !l[i].Termine
		}
		return l[i].Updated > l[j].Updated
	})
}

// combatSuivreDirect : le suivi en direct d'un combat du serveur. L'écran reste branché ici ; si le
// serveur se tait, il l'apprend, et le suivi se rétablit seul. Comme le suivi du serveur, il ne fait
// que recevoir : les actions passent par POST /api/combats/{id}/actions.
func (s *Serveur) combatSuivreDirect(w http.ResponseWriter, r *http.Request) {
	cl := s.clientDirect()
	if cl == nil {
		s.combatDirect(w, r)
		return
	}
	id, ok := idCombat(w, r)
	if !ok {
		return
	}
	existe, enCours := s.combatLocal(id)
	if enCours {
		s.combatDirect(w, r)
		return
	}
	chemin := "/api/combats/" + strconv.FormatInt(id, 10) + "/direct"
	amont, err := cl.Suivre(chemin)
	var refus synchro.ErrServeur
	switch {
	case err == nil:
	case errors.As(err, &refus) && refus.Code == http.StatusNotFound && existe:
		s.combatDirect(w, r)
		return
	case errors.As(err, &refus):
		erreur(w, refus.Code, refus.Msg)
		return
	case existe && !s.salles.estDistant(id):
		s.combatDirect(w, r)
		return
	}
	if err != nil {
		s.noterDirect(err)
	}
	droits, passage := s.droitsDuSuivi(r), s.connexionDirecte()
	conn, errMaj := versWebSocket.Upgrade(w, r, nil)
	if errMaj != nil {
		if amont != nil {
			amont.Close()
		}
		return
	}
	// le poste a quitté le serveur, ou en a changé : ce suivi n'a plus de raison d'être
	defer s.surveiller(conn, r, func() bool {
		c := s.connexionDirecte()
		return droits() && c != nil && passage != nil && *c == *passage
	})()
	s.salles.noterDistant(id, true)
	e := &ecran{conn: conn, envoi: make(chan []byte, 64)}
	go e.ecrire()
	s.salles.mu.Lock()
	s.salles.suivis[e] = true
	s.salles.mu.Unlock()
	// l'écran : ses pings, et la fin du suivi quand il s'en va
	fin := make(chan struct{})
	envoi := make(chan []byte, 8)
	go func() {
		defer close(fin)
		conn.SetReadLimit(4096)
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if strings.Contains(string(data), `"ping"`) {
				select {
				case envoi <- []byte(`{"type":"pong"}`):
				default:
				}
			}
		}
	}()
	perdue := err != nil
	if perdue {
		e.envoyer([]byte(`{"type":"connexion","etat":"perdue"}`))
	}
	defer func() {
		s.salles.mu.Lock()
		delete(s.salles.suivis, e)
		s.salles.mu.Unlock()
		close(e.envoi)
	}()
	for {
		if amont == nil {
			select {
			case <-fin:
				return
			case m := <-envoi:
				e.envoyer(m)
				continue
			case <-time.After(attenteRetour):
			}
			if amont, err = cl.Suivre(chemin); err != nil {
				amont = nil
				if errors.As(err, &refus) && refus.Code == http.StatusNotFound { // supprimé pendant la coupure
					s.salles.noterDistant(id, false)
					e.envoyer([]byte(`{"type":"supprime"}`))
					<-fin
					return
				}
				if errors.As(err, &refus) && (refus.Code == http.StatusUnauthorized || refus.Code == http.StatusForbidden) {
					s.noterDirect(err) // le serveur a retiré ses droits à ce poste : l'écran n'a plus à l'attendre
					return
				}
				continue
			}
		}
		if perdue {
			perdue = false
			s.noterDirect(nil)
			e.envoyer([]byte(`{"type":"connexion","etat":"retablie"}`))
		}
		amonts, stop, coupe := make(chan []byte, 8), make(chan struct{}), make(chan struct{})
		go relayerSuivi(amont, amonts, stop, coupe)
	suivi:
		for {
			select {
			case <-fin:
				close(stop)
				amont.Close()
				<-coupe
				return
			case m := <-envoi:
				e.envoyer(m)
			case m := <-amonts:
				e.envoyer(m)
			case <-coupe:
				break suivi
			}
		}
		amont.Close()
		amont, perdue = nil, true
		s.noterDirect(synchro.ErrInjoignable)
		e.envoyer([]byte(`{"type":"connexion","etat":"perdue"}`))
	}
}

// relayerSuivi lit le suivi du serveur jusqu'à sa coupure : ses messages vont à « sortie », sauf ses
// réponses aux pings, qui ne servent qu'à savoir qu'il vit.
func relayerSuivi(amont *websocket.Conn, sortie chan<- []byte, stop <-chan struct{}, coupe chan<- struct{}) {
	defer close(coupe)
	arret := make(chan struct{})
	defer close(arret)
	vivant := func() { _ = amont.SetReadDeadline(time.Now().Add(finSuivi)) }
	vivant()
	go func() {
		t := time.NewTicker(pingSuivi)
		defer t.Stop()
		for {
			select {
			case <-arret:
				return
			case <-t.C:
				_ = amont.SetWriteDeadline(time.Now().Add(pingSuivi))
				if err := amont.WriteMessage(websocket.TextMessage, []byte(`{"type":"ping"}`)); err != nil {
					_ = amont.Close()
					return
				}
			}
		}
	}()
	for {
		_, m, err := amont.ReadMessage()
		if err != nil {
			return
		}
		vivant()
		if bytes.Contains(m, []byte(`"type":"pong"`)) {
			continue
		}
		select {
		case sortie <- m:
		case <-stop:
			return
		}
	}
}

// desDirect : le lanceur de dés du MJ. Connecté, les jets vivent sur le serveur, et les joueurs les
// voient (un jet caché reste caché : c'est le serveur qui filtre). Sans serveur, ils restent ici.
func (s *Serveur) desDirect(local http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cl := s.clientDirect()
		if cl == nil || (r.Method == http.MethodGet && s.opt.app.etatServeur().Etat == etatHorsLigne) {
			local(w, r) // relu toutes les trois secondes : pas d'attente à chaque tour pendant une coupure
			return
		}
		corps := garderCorps(r)
		code, rep, err := cl.Relayer(r.Method, r.URL.RequestURI(), corps)
		if err != nil || code == http.StatusUnauthorized || code == http.StatusNotFound {
			if err != nil {
				s.noterDirect(err)
			}
			local(w, r)
			return
		}
		repondreBrut(w, code, rep)
	}
}
