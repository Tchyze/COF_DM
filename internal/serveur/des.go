package serveur

import (
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Tchyze/cof_dm/internal/des"
	"github.com/Tchyze/cof_dm/internal/store"
)

// Lanceur de dés, hors combat. Les jets des joueurs et les jets publics du MJ se voient de toute la
// table (page des joueurs, en direct) ; le MJ a aussi des jets cachés, que lui seul voit. Les jets
// ne sont pas rangés dans l'univers : ils vivent le temps d'une partie, comme un combat en cours
// sur une table, et ne se synchronisent pas.

const jetsGardes = 100 // les derniers jets de la partie

// jet : un jet de dés à la table.
type jet struct {
	ID      int64   `json:"id"`
	Quand   float64 `json:"quand"`
	Qui     string  `json:"qui"`          // « MJ », ou le nom du PJ
	Pj      int64   `json:"pj,omitempty"` // fiche du PJ qui a lancé
	MJ      bool    `json:"mj,omitempty"`
	Cache   bool    `json:"cache,omitempty"` // jet caché du MJ : jamais montré aux joueurs
	Libelle string  `json:"libelle,omitempty"`
	des.Resultat
}

type tableJets struct {
	mu      sync.Mutex
	liste   []jet
	dernier int64
}

func (t *tableJets) ajouter(j jet) jet {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.dernier++
	j.ID = t.dernier
	j.Quand = float64(time.Now().UnixMilli()) / 1000
	t.liste = append(t.liste, j)
	if len(t.liste) > jetsGardes {
		t.liste = append([]jet(nil), t.liste[len(t.liste)-jetsGardes:]...)
	}
	return j
}

// depuis : les jets après « id », sans les jets cachés si « caches » est faux.
func (t *tableJets) depuis(id int64, caches bool) ([]jet, int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := []jet{}
	for _, j := range t.liste {
		if j.ID > id && (caches || !j.Cache) {
			out = append(out, j)
		}
	}
	return out, t.dernier
}

func (s *Serveur) routesDes() {
	s.jets = &tableJets{}
	s.mux.HandleFunc("GET /api/des", s.desLire)
	s.mux.HandleFunc("POST /api/des", s.desLancerMJ)
}

// routesDesJoueurs : les jets vus et lancés depuis la page des joueurs (mêmes routes partout).
func (s *Serveur) routesDesJoueurs(m *http.ServeMux) {
	m.HandleFunc("GET /api/joueur/des", s.desLireJoueur)
	m.HandleFunc("POST /api/joueur/des", s.desLancerJoueur)
}

type demandeJet struct {
	Formule string `json:"formule"`
	Libelle string `json:"libelle"`
	Cache   bool   `json:"cache"`
	Pj      int64  `json:"pj"`
}

func libelleJet(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > 60 {
		s = string(r[:60])
	}
	return s
}

func lireDepuis(r *http.Request) int64 {
	n, _ := strconv.ParseInt(r.URL.Query().Get("depuis"), 10, 64)
	return n
}

// desLire : le MJ voit tous les jets, cachés compris.
func (s *Serveur) desLire(w http.ResponseWriter, r *http.Request) {
	l, dernier := s.jets.depuis(lireDepuis(r), true)
	repondre(w, 200, map[string]any{"jets": l, "dernier": dernier})
}

func (s *Serveur) desLireJoueur(w http.ResponseWriter, r *http.Request) {
	l, dernier := s.jets.depuis(lireDepuis(r), false)
	repondre(w, 200, map[string]any{"jets": l, "dernier": dernier})
}

func (s *Serveur) desLancerMJ(w http.ResponseWriter, r *http.Request) {
	var in demandeJet
	if err := lireJSON(r, &in); err != nil {
		erreurStore(w, err)
		return
	}
	res, err := des.Lancer(in.Formule)
	if err != nil {
		erreur(w, 422, err.Error())
		return
	}
	s.publierJet(w, jet{Qui: "MJ", MJ: true, Cache: in.Cache, Libelle: libelleJet(in.Libelle), Resultat: res})
}

// desLancerJoueur : un joueur lance pour son PJ. Sur un serveur, c'est le PJ de son compte ; à la
// maison, celui qu'il a choisi sur la page. Un joueur ne fait jamais de jet caché.
func (s *Serveur) desLancerJoueur(w http.ResponseWriter, r *http.Request) {
	var in demandeJet
	if err := lireJSON(r, &in); err != nil {
		erreurStore(w, err)
		return
	}
	pj, restreint := s.pjDuCompte(r)
	if !restreint {
		pj = in.Pj
	}
	if pj == 0 {
		erreur(w, 422, "Choisissez d'abord votre personnage.")
		return
	}
	e, err := s.store.Lire(pj)
	if err != nil || e.Type != "pj" || e.Archive || (restreint && !partagee(e)) {
		erreurStore(w, store.ErrIntrouvable)
		return
	}
	res, err := des.Lancer(in.Formule)
	if err != nil {
		erreur(w, 422, err.Error())
		return
	}
	s.publierJet(w, jet{Qui: e.Nom, Pj: e.ID, Libelle: libelleJet(in.Libelle), Resultat: res})
}

func (s *Serveur) publierJet(w http.ResponseWriter, j jet) {
	repondre(w, 201, s.annoncerJet(j))
}

// annoncerJet range un jet à la table et le montre aux joueurs, sauf s'il est caché.
func (s *Serveur) annoncerJet(j jet) jet {
	j = s.jets.ajouter(j)
	if !j.Cache {
		s.joueurs.envoyer(map[string]any{"type": "jet", "jet": j})
	}
	return j
}
