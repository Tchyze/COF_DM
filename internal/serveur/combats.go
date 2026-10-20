package serveur

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/Tchyze/cof_dm/internal/combat"
	"github.com/Tchyze/cof_dm/internal/fiches"
	"github.com/Tchyze/cof_dm/internal/store"
)

// Combats (repris de tracker_COF) : ils vivent dans l'univers, en cours ou terminés. Le serveur est
// la seule source de vérité : une action arrive par POST /api/combats/{id}/actions, le moteur
// l'applique avec les règles de l'univers, l'état est enregistré puis diffusé aux écrans ouverts
// sur le combat (GET /api/combats/{id}/direct, un WebSocket qui ne fait que recevoir). Les PV et
// les points de chance des PJ sont reportés sur leur feuille de personnage à chaque action.

const collCombats = "combats"

func (s *Serveur) routesCombats() {
	s.salles = &salles{s: s, ouvertes: map[int64]*salle{}}
	m := s.mux
	m.HandleFunc("GET /api/combats", s.combatsLister)
	m.HandleFunc("POST /api/combats", s.combatCreer)
	m.HandleFunc("GET /api/combats/{id}", s.combatLire)
	m.HandleFunc("PATCH /api/combats/{id}", s.combatModifier)
	m.HandleFunc("DELETE /api/combats/{id}", s.combatSupprimer)
	m.HandleFunc("POST /api/combats/{id}/actions", s.combatAgir)
	m.HandleFunc("GET /api/combats/{id}/direct", s.combatDirect)
}

// Fermer coupe les écrans qui suivent un combat : l'univers va être fermé.
func (s *Serveur) Fermer() {
	if s.salles != nil {
		s.salles.fermer()
	}
	if s.joueurs != nil {
		s.joueurs.fermer()
	}
}

// ---------------------------------------------------------------- combats ouverts

// salles garde en mémoire les combats suivis ou joués, pour les annulations et la diffusion.
type salles struct {
	s        *Serveur
	mu       sync.Mutex
	ouvertes map[int64]*salle
}

type salle struct {
	mu      sync.Mutex
	etat    *combat.Etat
	hist    [][]byte
	ecrans  map[*ecran]bool
	reporte map[int64]string // dernières valeurs (PV, PC, PM, utilisations) reportées sur les feuilles des PJ
}

// ecran : un appareil qui suit le combat.
type ecran struct {
	conn  *websocket.Conn
	envoi chan []byte
}

const histMax = 100

func (h *salles) lire(id int64) (*combat.Etat, error) {
	d, err := h.s.store.LireDoc(collCombats, id)
	if err != nil {
		return nil, err
	}
	var st combat.Etat
	if err := json.Unmarshal(d.Data, &st); err != nil {
		return nil, err
	}
	st.Normaliser()
	return &st, nil
}

func (h *salles) salle(id int64) (*salle, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if sa, ok := h.ouvertes[id]; ok {
		return sa, nil
	}
	st, err := h.lire(id)
	if err != nil {
		return nil, err
	}
	sa := &salle{etat: st, ecrans: map[*ecran]bool{}, reporte: map[int64]string{}}
	h.ouvertes[id] = sa
	return sa, nil
}

func (e *ecran) envoyer(msg []byte) {
	select {
	case e.envoi <- msg:
	default: // écran trop lent : on le déconnecte, il se reconnectera
		_ = e.conn.Close()
	}
}

func (sa *salle) message(id int64) []byte {
	b, _ := json.Marshal(map[string]any{"type": "etat", "id": id, "etat": sa.etat, "annulable": len(sa.hist)})
	return b
}

func (sa *salle) diffuser(msg []byte) {
	for e := range sa.ecrans {
		e.envoyer(msg)
	}
}

// oublier retire un combat supprimé et prévient ses écrans.
func (h *salles) oublier(id int64) {
	h.mu.Lock()
	sa, ok := h.ouvertes[id]
	delete(h.ouvertes, id)
	h.mu.Unlock()
	if !ok {
		return
	}
	sa.mu.Lock()
	defer sa.mu.Unlock()
	sa.diffuser([]byte(`{"type":"supprime"}`))
}

func (h *salles) fermer() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for id, sa := range h.ouvertes {
		sa.mu.Lock()
		for e := range sa.ecrans {
			_ = e.conn.Close()
		}
		sa.mu.Unlock()
		delete(h.ouvertes, id)
	}
}

func restaurer(instantane []byte) *combat.Etat {
	var st combat.Etat
	_ = json.Unmarshal(instantane, &st)
	st.Normaliser()
	return &st
}

// agir applique une action à un combat, l'enregistre et le diffuse. « annuler » revient à l'état
// d'avant la dernière action.
func (h *salles) agir(id int64, a combat.Action, auteur string) (sa *salle, err error) {
	if sa, err = h.salle(id); err != nil {
		return nil, err
	}
	sa.mu.Lock()
	defer sa.mu.Unlock()
	avant, _ := json.Marshal(sa.etat)
	// une erreur imprévue du moteur ne doit pas laisser un état à moitié modifié
	defer func() {
		if e := recover(); e != nil {
			log.Printf("panique sur l'action %q du combat %d : %v\n%s", a.Type, id, e, debug.Stack())
			sa.etat = restaurer(avant)
			err = errors.New("erreur interne")
		}
	}()
	if a.Type == "annuler" {
		if len(sa.hist) == 0 {
			return nil, combat.ErrAction("Rien à annuler.")
		}
		sa.etat = restaurer(sa.hist[len(sa.hist)-1])
		sa.hist = sa.hist[:len(sa.hist)-1]
	} else {
		if err := h.s.preparerAction(&a); err != nil {
			return nil, err
		}
		sa.etat.Regler(h.s.lireRegles().Combat)
		if err := sa.etat.Appliquer(a); err != nil {
			sa.etat = restaurer(avant)
			return nil, err
		}
		if sa.hist = append(sa.hist, avant); len(sa.hist) > histMax {
			sa.hist = sa.hist[1:]
		}
	}
	h.s.reporterSurFeuilles(sa.etat, sa.reporte)
	data, _ := json.Marshal(sa.etat)
	if _, err := h.s.store.MajDoc(collCombats, id, data, sa.etat.Nom, ""); err != nil {
		log.Printf("enregistrement du combat %d : %v", id, err)
		return nil, err
	}
	switch a.Type {
	case "terminer":
		h.s.store.Noter("combat", fmt.Sprintf("combat « %s » terminé", sa.etat.Nom), auteur)
	case "rouvrir":
		h.s.store.Noter("combat", fmt.Sprintf("combat « %s » rouvert", sa.etat.Nom), auteur)
	}
	sa.diffuser(sa.message(id))
	go h.s.joueurs.combat() // après libération du verrou de la salle
	if a.Type == "terminer" || a.Type == "rouvrir" || a.Type == "annuler" {
		go h.s.joueurs.envoyer(map[string]any{"type": "hauts_faits"})
	}
	return sa, nil
}

// reporterSurFeuilles recopie les PV, les points de chance, les points de mana et les utilisations
// des capacités (hors « par combat ») des PJ du combat sur leur feuille de personnage (annulation
// comprise). deja : dernières valeurs écrites, pour ne réécrire que les feuilles qui changent.
func (s *Serveur) reporterSurFeuilles(st *combat.Etat, deja map[int64]string) {
	for _, c := range st.Cbts {
		if c.Ref == nil || c.Ref.Kind != "pj" || c.Ref.ID == 0 {
			continue
		}
		usages := map[string]fiches.Utilisation{}
		for _, a := range c.Actives {
			if a.Fois > 0 && a.Par != "combat" {
				usages[a.Cle] = fiches.Utilisation{Par: a.Par, N: a.Deja + c.Usages[a.Cle]} // N = 0 : rendue par « Annuler »
			}
		}
		u, _ := json.Marshal(usages)
		valeurs := fmt.Sprintf("%d %d %d %s", c.Pv, c.Pc, c.Pm, u)
		if v, ok := deja[c.Ref.ID]; ok && v == valeurs {
			continue
		}
		raw, _, err := s.store.LireFeuille(c.Ref.ID)
		if err != nil {
			continue
		}
		var m map[string]any
		if json.Unmarshal(raw, &m) != nil {
			continue
		}
		deja[c.Ref.ID] = valeurs
		pv, avecPv := m["pv_actuel"].(float64)
		pc, _ := m["pc"].(float64)
		pm, _ := m["pm"].(float64)
		ancien, _ := m["utilisations"].(map[string]any)
		if ancien == nil {
			ancien = map[string]any{}
		}
		changeUsages := false
		for k, x := range usages {
			a, _ := ancien[k].(map[string]any)
			switch {
			case x.N == 0 && a != nil:
				delete(ancien, k)
				changeUsages = true
			case x.N > 0 && (a == nil || a["n"] != float64(x.N) || a["par"] != x.Par):
				ancien[k], changeUsages = map[string]any{"par": x.Par, "n": x.N}, true
			}
		}
		if avecPv && int(pv) == c.Pv && int(pc) == c.Pc && (c.PmMax == 0 || int(pm) == c.Pm) && !changeUsages {
			continue // la feuille est déjà à jour
		}
		m["pv_actuel"], m["pc"] = c.Pv, c.Pc
		if c.PmMax > 0 { // un PJ sans mana garde sa feuille telle quelle
			m["pm"] = c.Pm
		}
		if len(ancien) > 0 {
			m["utilisations"] = ancien
		} else {
			delete(m, "utilisations")
		}
		data, _ := json.Marshal(m)
		if _, err := s.store.EcrireFeuille(c.Ref.ID, data, 0, ""); err != nil {
			log.Printf("report des PV sur la feuille n°%d : %v", c.Ref.ID, err)
			continue
		}
		s.joueurs.fiche(c.Ref.ID)
	}
}

// ---------------------------------------------------------------- fiches de combat

func ficheDeMonstre(id int64, m *fiches.Monstre) combat.FicheCombat {
	return combat.FicheCombat{ID: id, Nom: m.Nom, Init: m.Init, Defense: m.Defense, Pv: m.Pv, Rd: m.Rd, Crit: m.Crit,
		Attaques: m.Attaques, Capacites: m.Capacites, Carac: m.Carac, Superieures: m.Superieures}
}

// ficheCombat : ce qu'il faut pour combattre, d'après un monstre du bestiaire (« monstre »), la
// feuille d'un PJ (« pj ») ou le profil de combat d'un PNJ (« pnj », qui garde son nom).
func (s *Serveur) ficheCombat(kind string, id int64) (*combat.FicheCombat, error) {
	switch kind {
	case "monstre":
		m, err := s.monstre(id)
		if err != nil {
			return nil, combat.ErrAction("Ce monstre n'est plus dans le bestiaire.")
		}
		f := ficheDeMonstre(id, m)
		return &f, nil
	case "pnj":
		e, err := s.store.Lire(id)
		if err != nil || e.Type != "pnj" {
			return nil, combat.ErrAction("PNJ introuvable.")
		}
		m, err := s.monstre(premierID(e.Data["profil_combat"]))
		if err != nil {
			return nil, combat.ErrAction(e.Nom + " n'a pas de profil de combat.")
		}
		f := ficheDeMonstre(id, m)
		f.Nom = e.Nom
		return &f, nil
	case "pj":
		e, err := s.store.Lire(id)
		if err != nil || e.Type != "pj" {
			return nil, combat.ErrAction("PJ introuvable.")
		}
		fe, _, existe, err := s.feuilleDe(e)
		if err != nil || !existe {
			return nil, combat.ErrAction(e.Nom + " n'a pas encore de feuille de personnage.")
		}
		// avec les bonus des capacités atteintes
		f := combat.FicheCombat{ID: id, Nom: e.Nom, Init: fe.Init, Defense: fe.Defense, Pv: fe.PvMax(), PvActuel: fe.PvActuel,
			Rd: fe.RdTotal(), Crit: fe.Crit, Pc: fe.Pc, Pm: fe.Pm, PmMax: fe.PmMaxTotal(), Attaques: fe.Attaques, Capacites: fe.Capacites,
			Voies: capacitesAcquises(fe.Voies), Carac: fe.Carac, Actives: s.capacitesActives(fe)}
		f.Valeurs = &struct {
			SAG int `json:"SAG"`
		}{fe.ValeurTotale("SAG")}
		return &f, nil
	}
	return nil, combat.ErrAction("Fiche inconnue.")
}

// capacitesAcquises : les capacités des voies d'un PJ jusqu'à son rang dans chacune.
func capacitesAcquises(voies []fiches.Voie) []string {
	var out []string
	for _, v := range voies {
		for i, c := range v.Capacites {
			c = strings.Join(strings.Fields(c), " ")
			if i >= v.Rang || c == "" {
				continue
			}
			if r := []rune(c); len(r) > 80 {
				c = string(r[:80])
			}
			out = append(out, c)
		}
	}
	return out
}

// preparerAction résout côté serveur la fiche d'une action « ajouter » : un écran ne fournit que
// la référence, jamais les statistiques d'un monstre, d'un PNJ ou d'un PJ de l'univers.
func (s *Serveur) preparerAction(a *combat.Action) error {
	if a.Type != "ajouter" {
		return nil
	}
	switch a.Kind {
	case "monstre", "pnj", "pj":
		f, err := s.ficheCombat(a.Kind, a.RefID)
		if err != nil {
			return err
		}
		a.Fiche = f
		if a.Kind != "monstre" {
			a.Qte = 1
		}
	default:
		a.Kind = "" // saisie libre : la fiche vient de l'écran
	}
	return nil
}

// ---------------------------------------------------------------- API

// resumeCombat : ligne de la liste des combats.
type resumeCombat struct {
	combat.Resume
	Rencontre int64 `json:"rencontre,omitempty"`
	Campagne  int64 `json:"campagne,omitempty"`
	Seance    int64 `json:"seance,omitempty"`
}

func resumer(id int64, modifie float64, st *combat.Etat) resumeCombat {
	r := resumeCombat{Resume: st.Resume(id, modifie), Campagne: st.CampagneID, Seance: st.SeanceID}
	if st.RencontreID != nil {
		r.Rencontre = *st.RencontreID
	}
	return r
}

// combatsLister : les combats de l'univers, ceux en cours d'abord, le plus récent en tête.
// ?seance=12 : ceux d'une séance ; ?en_cours=1 : ceux qui ne sont pas terminés.
func (s *Serveur) combatsLister(w http.ResponseWriter, r *http.Request) {
	docs, err := s.store.ListerDocs(collCombats)
	if err != nil {
		erreurStore(w, err)
		return
	}
	seance, enCours := entierParam(r, "seance"), r.URL.Query().Get("en_cours") != ""
	out := []resumeCombat{}
	for _, d := range docs {
		var st combat.Etat
		if json.Unmarshal(d.Data, &st) != nil {
			continue
		}
		if (seance != 0 && st.SeanceID != seance) || (enCours && st.Termine) {
			continue
		}
		out = append(out, resumer(d.ID, d.Modifie, &st))
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Termine != out[j].Termine {
			return !out[i].Termine
		}
		return out[i].Updated > out[j].Updated
	})
	repondre(w, 200, out)
}

// combatCreer lance un combat. Avec « rencontre » : ses créatures, ses PNJ présents qui ont un
// profil de combat, et les PJ en jeu de sa campagne (ou ceux de « pj »). Sans rencontre : les PJ
// de « pj », ou un combat vide à remplir. Il est rattaché à la séance de la campagne que montre
// l'écran du MJ.
func (s *Serveur) combatCreer(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Nom       string  `json:"nom"`
		Rencontre int64   `json:"rencontre"`
		Campagne  int64   `json:"campagne"`
		Pj        []int64 `json:"pj"` // absent : les PJ en jeu de la campagne
	}
	if err := lireJSON(r, &in); err != nil {
		erreurStore(w, err)
		return
	}
	nom := strings.TrimSpace(in.Nom)
	if len([]rune(nom)) > 120 {
		erreur(w, 422, "Nom du combat : 120 caractères au plus.")
		return
	}
	var cbts []*combat.Combattant
	deja := map[string]*combat.Combattant{}
	ajouter := func(l ...*combat.Combattant) {
		for _, c := range l {
			cbts = append(cbts, c)
			deja[c.ID] = c
		}
	}
	absents := []string{} // ce qui n'a pas pu entrer en combat, et pourquoi
	campagne := in.Campagne
	var rencontre *int64
	if in.Rencontre != 0 {
		e, err := s.store.Lire(in.Rencontre)
		if err != nil || e.Type != "rencontre" {
			erreur(w, 404, "Rencontre introuvable.")
			return
		}
		rencontre = &e.ID
		if nom == "" {
			nom = e.Nom
		}
		if campagne == 0 {
			campagne = s.campagneDeRencontre(e)
		}
		creatures, sans := s.creaturesDe(e)
		for _, c := range creatures {
			kind, id := "monstre", c.Monstre
			if c.Pnj != nil {
				kind, id = "pnj", c.Pnj.ID
			}
			f, err := s.ficheCombat(kind, id)
			if err != nil {
				absents = append(absents, c.Nom+" : "+strings.TrimSuffix(err.Error(), "."))
				continue
			}
			ajouter(combat.InstancierGroupe(*f, c.Qte, c.Camp, kind, deja)...)
		}
		for _, p := range sans {
			absents = append(absents, p.Nom+" : pas de profil de combat")
		}
	}
	pj := in.Pj
	if pj == nil && (in.Rencontre != 0 || in.Campagne != 0) {
		for _, p := range s.pjDeTable(campagne) {
			if p.Statut != "absent" {
				pj = append(pj, p.Fiche.ID)
			}
		}
	}
	for _, id := range pj {
		f, err := s.ficheCombat("pj", id)
		if err != nil {
			absents = append(absents, strings.TrimSuffix(err.Error(), "."))
			continue
		}
		ajouter(combat.DepuisFiche(*f, "pj", "", "pj"))
	}
	if nom == "" {
		nom = "Combat"
	}
	st := combat.Nouveau(nom, cbts, s.lireRegles().Combat)
	st.RencontreID, st.CampagneID, st.SeanceID = rencontre, campagne, s.seanceDeTable(campagne)
	data, _ := json.Marshal(st)
	d, err := s.store.CreerDoc(collCombats, data, nom, "")
	if err != nil {
		erreurStore(w, err)
		return
	}
	s.store.Noter("combat", fmt.Sprintf("combat « %s » lancé", nom), auteurDe(r))
	go s.joueurs.combat()
	repondre(w, 201, map[string]any{"combat": resumer(d.ID, d.Modifie, st), "absents": absents})
}

// campagneDeRencontre : la campagne d'une rencontre, sinon celle de sa quête, sinon celle de
// l'écran du MJ.
func (s *Serveur) campagneDeRencontre(e store.Entite) int64 {
	if c := premierID(e.Data["campagne"]); c != 0 {
		return c
	}
	if q, err := s.store.Lire(premierID(e.Data["quete"])); err == nil {
		return s.campagneDeTable(&q)
	}
	return s.campagneDeTable(nil)
}

func idCombat(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		erreur(w, 404, "Combat introuvable.")
		return 0, false
	}
	return id, true
}

// repondreCombat : l'état d'un combat, tel que les écrans le reçoivent aussi en direct.
func repondreCombat(w http.ResponseWriter, id int64, sa *salle) {
	repondre(w, 200, map[string]any{"id": id, "etat": sa.etat, "annulable": len(sa.hist)})
}

func (s *Serveur) combatLire(w http.ResponseWriter, r *http.Request) {
	id, ok := idCombat(w, r)
	if !ok {
		return
	}
	sa, err := s.salles.salle(id)
	if err != nil {
		erreur(w, 404, "Combat introuvable.")
		return
	}
	sa.mu.Lock()
	defer sa.mu.Unlock()
	repondreCombat(w, id, sa)
}

// combatModifier renomme un combat ou le rattache à une autre séance (0 : aucune).
func (s *Serveur) combatModifier(w http.ResponseWriter, r *http.Request) {
	id, ok := idCombat(w, r)
	if !ok {
		return
	}
	var in struct {
		Nom    *string `json:"nom"`
		Seance *int64  `json:"seance"`
	}
	if err := lireJSON(r, &in); err != nil {
		erreurStore(w, err)
		return
	}
	if in.Nom != nil {
		if *in.Nom = strings.TrimSpace(*in.Nom); *in.Nom == "" || len([]rune(*in.Nom)) > 120 {
			erreur(w, 422, "Nom du combat : de 1 à 120 caractères.")
			return
		}
	}
	if in.Seance != nil && *in.Seance != 0 {
		if e, err := s.store.Lire(*in.Seance); err != nil || e.Type != "seance" {
			erreur(w, 422, "Séance introuvable.")
			return
		}
	}
	sa, err := s.salles.salle(id)
	if err != nil {
		erreur(w, 404, "Combat introuvable.")
		return
	}
	sa.mu.Lock()
	defer sa.mu.Unlock()
	if in.Nom != nil {
		sa.etat.Nom = *in.Nom
	}
	if in.Seance != nil {
		sa.etat.SeanceID = *in.Seance
	}
	data, _ := json.Marshal(sa.etat)
	if _, err := s.store.MajDoc(collCombats, id, data, sa.etat.Nom, ""); err != nil {
		erreurStore(w, err)
		return
	}
	sa.diffuser(sa.message(id))
	repondreCombat(w, id, sa)
}

func (s *Serveur) combatSupprimer(w http.ResponseWriter, r *http.Request) {
	id, ok := idCombat(w, r)
	if !ok {
		return
	}
	nom := "combat n°" + strconv.FormatInt(id, 10)
	if st, err := s.salles.lire(id); err == nil {
		nom = st.Nom
	}
	if err := s.store.SupprimerDoc(collCombats, id, nom, auteurDe(r)); err != nil {
		erreur(w, 404, "Combat introuvable.")
		return
	}
	s.salles.oublier(id)
	go s.joueurs.combat()
	go s.joueurs.envoyer(map[string]any{"type": "hauts_faits"})
	repondre(w, 200, map[string]any{"supprime": true})
}

// combatAgir applique une action au combat et renvoie son nouvel état. Une action refusée par les
// règles répond 422, avec la raison.
func (s *Serveur) combatAgir(w http.ResponseWriter, r *http.Request) {
	id, ok := idCombat(w, r)
	if !ok {
		return
	}
	var a combat.Action
	if err := lireJSON(r, &a); err != nil {
		erreurStore(w, err)
		return
	}
	sa, err := s.salles.agir(id, a, auteurDe(r))
	var refus combat.ErrAction
	switch {
	case errors.As(err, &refus):
		erreur(w, 422, refus.Error())
	case errors.Is(err, store.ErrIntrouvable):
		erreur(w, 404, "Combat introuvable.")
	case err != nil:
		log.Printf("combat n°%d : %v", id, err)
		erreur(w, 500, "Action impossible. "+msgInterne)
	default:
		sa.mu.Lock()
		defer sa.mu.Unlock()
		repondreCombat(w, id, sa)
	}
}

// ---------------------------------------------------------------- suivi en direct

// memeOrigine : la page qui ouvre le WebSocket est servie par cette application. Un navigateur
// envoie toujours Origin ; un client qui n'en envoie pas n'est pas une page d'un autre site.
func memeOrigine(r *http.Request) bool {
	o := r.Header.Get("Origin")
	if o == "" {
		return true
	}
	u, err := url.Parse(o)
	return err == nil && strings.EqualFold(u.Host, r.Host)
}

var versWebSocket = websocket.Upgrader{ReadBufferSize: 1024, WriteBufferSize: 16384, CheckOrigin: memeOrigine}

// combatDirect : l'état du combat, renvoyé à chaque changement. La connexion ne fait que recevoir :
// les actions passent par POST /api/combats/{id}/actions, qui suit les rôles.
func (s *Serveur) combatDirect(w http.ResponseWriter, r *http.Request) {
	id, ok := idCombat(w, r)
	if !ok {
		return
	}
	sa, err := s.salles.salle(id)
	if err != nil {
		erreur(w, 404, "Combat introuvable.")
		return
	}
	conn, err := versWebSocket.Upgrade(w, r, nil)
	if err != nil {
		return // la réponse d'erreur est déjà partie
	}
	e := &ecran{conn: conn, envoi: make(chan []byte, 64)}
	go e.ecrire()
	sa.mu.Lock()
	sa.ecrans[e] = true
	e.envoyer(sa.message(id))
	sa.mu.Unlock()
	defer func() {
		sa.mu.Lock()
		delete(sa.ecrans, e)
		sa.mu.Unlock()
		close(e.envoi)
	}()
	conn.SetReadLimit(4096)
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if strings.Contains(string(data), `"ping"`) { // l'écran vérifie que la connexion vit
			sa.mu.Lock()
			e.envoyer([]byte(`{"type":"pong"}`))
			sa.mu.Unlock()
		}
	}
}

func (e *ecran) ecrire() {
	ping := time.NewTicker(30 * time.Second)
	defer ping.Stop()
	for {
		select {
		case msg, ok := <-e.envoi:
			if !ok {
				_ = e.conn.Close()
				return
			}
			_ = e.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := e.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				_ = e.conn.Close()
				return
			}
		case <-ping.C:
			_ = e.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := e.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				_ = e.conn.Close()
				return
			}
		}
	}
}
