package serveur

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Tchyze/cof_dm/internal/combat"
	"github.com/Tchyze/cof_dm/internal/comptes"
	"github.com/Tchyze/cof_dm/internal/store"
)

// Page des joueurs (reprise de tracker_COF) : la feuille de son personnage, le combat en cours
// réduit à l'ordre d'initiative et aux états (aucun PV, aucune DEF, aucune attaque), et les hauts
// faits. Les joueurs regardent le combat, ils n'y agissent pas.
//
// Deux façons d'y entrer :
//   - sur un serveur, par son compte (rôle « joueur ») : il ne voit que le PJ qu'il joue ;
//   - à la maison, sans compte : la page est libre sur le réseau local quand le MJ l'ouvre
//     (réglage de l'ordinateur, éteint par défaut), chacun y choisit son PJ. Seules les routes de
//     « libre » répondent alors sans mot de passe ; tout le reste demande toujours le MJ.

const metaPageJoueurs = "page_joueurs"

// routesPageJoueurs inscrit les routes de la page : dans l'application (le MJ la voit comme un
// joueur), dans la liste blanche du rôle « joueur », et dans celle de la page libre.
func (s *Serveur) routesPageJoueurs() {
	s.joueurs = &diffusion{s: s, ecrans: map[*ecran]int64{}}
	for _, m := range []*http.ServeMux{s.mux, s.joueur} {
		if m == nil {
			continue
		}
		m.HandleFunc("GET /api/joueur/etat", s.joueurEtat)
		m.HandleFunc("GET /api/joueur/pj", s.joueurPJ)
		m.HandleFunc("GET /api/joueur/hauts-faits", s.joueurHautsFaits)
		m.HandleFunc("GET /api/joueur/direct", s.joueurDirect)
		s.routesDesJoueurs(m)
	}
	if s.acces.comptes != nil {
		return
	}
	// à la maison : le verrou des feuilles et l'ouverture de la page se règlent ici
	s.mux.HandleFunc("PUT /api/joueurs/edition", s.reglerEditionJoueurs)
	s.mux.HandleFunc("POST /api/acces/joueurs", s.changerPageJoueurs)
	l := http.NewServeMux()
	l.HandleFunc("GET /api/joueur/etat", s.joueurEtat)
	l.HandleFunc("GET /api/joueur/pj", s.joueurPJ)
	l.HandleFunc("GET /api/joueur/hauts-faits", s.joueurHautsFaits)
	l.HandleFunc("GET /api/joueur/direct", s.joueurDirect)
	s.routesDesJoueurs(l) // la seule écriture hors de la feuille : un jet, qui ne touche pas à l'univers
	l.HandleFunc("GET /api/entites/{id}/feuille", s.feuilleLireLibre)
	l.HandleFunc("PUT /api/entites/{id}/feuille", s.feuilleEcrireLibre)
	l.HandleFunc("GET /api/reference", s.referenceLire) // profils et voies : utiles à qui tient sa feuille
	l.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		erreur(w, 401, "Accès réservé au MJ : connectez-vous avec le mot de passe MJ, ou utilisez un jeton d'API.")
	})
	s.libre = l
}

// pageJoueursOuverte : à la maison, le MJ a ouvert la page des joueurs au réseau local.
func (s *Serveur) pageJoueursOuverte() bool {
	if s.acces.comptes != nil || s.libre == nil {
		return false
	}
	v, _ := s.reg.Meta(metaPageJoueurs)
	return v == "1"
}

// routeLibre : la requête vise la page des joueurs (ses données ; les fichiers de l'interface sont
// servis à tous).
func routeLibre(r *http.Request) bool {
	p := r.URL.Path
	return strings.HasPrefix(p, "/api/joueur/") || p == "/api/reference" ||
		(strings.HasPrefix(p, "/api/entites/") && strings.HasSuffix(p, "/feuille"))
}

// changerPageJoueurs : {"active": true} ouvre la page des joueurs au réseau local, sans mot de
// passe. Comme tout ce qui garde la porte, cela ne se règle que sur l'ordinateur du MJ.
func (s *Serveur) changerPageJoueurs(w http.ResponseWriter, r *http.Request) {
	if !surPlace(w, r) {
		return
	}
	var in struct {
		Active bool `json:"active"`
	}
	if err := lireJSON(r, &in); err != nil {
		erreurStore(w, err)
		return
	}
	if in.Active && s.relie() {
		erreur(w, 409, msgReseauRelie)
		return
	}
	v, detail := "", "page des joueurs fermée"
	if in.Active {
		v, detail = "1", "page des joueurs ouverte au réseau local"
	}
	if err := s.reg.SetMeta(metaPageJoueurs, v); err != nil {
		erreurStore(w, err)
		return
	}
	s.store.Noter("accès", detail, auteurDe(r))
	if !in.Active {
		s.joueurs.fermer() // les appareils qui suivaient la page sont coupés
	}
	repondre(w, 200, map[string]any{"page_joueurs": in.Active})
}

// pjDuCompte : sur un serveur, le PJ que joue le compte « joueur » ; restreint : la requête ne doit
// voir que lui (0 : il n'en joue aucun).
func (s *Serveur) pjDuCompte(r *http.Request) (pj int64, restreint bool) {
	c, ok := compteDe(r)
	if !ok || c.Role(s.store.ID()) != comptes.Joueur {
		return 0, false
	}
	return c.Persos[s.store.ID()], true
}

// ---------------------------------------------------------------- combat vu par les joueurs

type placeJoueur struct {
	Nom     string   `json:"nom"`
	Camp    string   `json:"camp"`
	Pj      int64    `json:"pj,omitempty"`      // fiche du PJ : son joueur reconnaît son tour
	ATerre  bool     `json:"a_terre,omitempty"` // PJ inconscient
	Courant bool     `json:"courant"`
	Suivant bool     `json:"suivant"`
	Etats   []string `json:"etats"`
}

type combatJoueur struct {
	Nom   string        `json:"nom"`
	Round int           `json:"round"`
	Ordre []placeJoueur `json:"ordre"`
}

// reduire ne garde d'un combat que ce qu'un joueur en voit : l'ordre d'initiative, qui joue, qui
// joue ensuite, et les états. Les adversaires et les alliés hors de combat disparaissent ; aucun
// chiffre n'est transmis.
func reduire(st *combat.Etat) *combatJoueur {
	cj := &combatJoueur{Nom: st.Nom, Round: st.Round, Ordre: []placeJoueur{}}
	for i, id := range st.Ordre {
		c := st.Cbts[id]
		if c == nil || (c.Camp != "pj" && c.Pv <= 0) {
			continue
		}
		p := placeJoueur{Nom: c.Nom, Camp: c.Camp, ATerre: c.Camp == "pj" && c.Pv <= 0, Courant: i == st.Tour, Etats: []string{}}
		if c.Ref != nil && c.Ref.Kind == "pj" {
			p.Pj = c.Ref.ID
		}
		for e := range c.Etats {
			p.Etats = append(p.Etats, e)
		}
		sort.Strings(p.Etats)
		cj.Ordre = append(cj.Ordre, p)
	}
	// prochain à jouer : le premier combattant en état d'agir après le courant
	cur := -1
	for i, p := range cj.Ordre {
		if p.Courant {
			cur = i
		}
	}
	if cur >= 0 {
		for k := 1; k < len(cj.Ordre); k++ {
			if i := (cur + k) % len(cj.Ordre); !cj.Ordre[i].ATerre {
				cj.Ordre[i].Suivant = true
				break
			}
		}
	}
	return cj
}

func participe(st *combat.Etat, pj int64) bool {
	for _, c := range st.Cbts {
		if c.Ref != nil && c.Ref.Kind == "pj" && c.Ref.ID == pj {
			return true
		}
	}
	return false
}

// combatJoueurs : le combat en cours le plus récemment joué, réduit. restreint : seulement un
// combat où ce PJ se bat.
func (s *Serveur) combatJoueurs(pj int64, restreint bool) *combatJoueur {
	docs, _ := s.store.ListerDocs(collCombats)
	sort.SliceStable(docs, func(i, j int) bool { return docs[i].Modifie > docs[j].Modifie })
	for _, d := range docs {
		var st combat.Etat
		if json.Unmarshal(d.Data, &st) != nil || st.Termine || len(st.Ordre) == 0 {
			continue
		}
		if restreint && (pj == 0 || !participe(&st, pj)) {
			continue
		}
		return reduire(&st)
	}
	return nil
}

// ---------------------------------------------------------------- diffusion en direct

// diffusion : les appareils qui suivent la page des joueurs, chacun avec le PJ auquel il est
// restreint (0 : aucun, −1 : pas de restriction).
type diffusion struct {
	s      *Serveur
	mu     sync.Mutex
	ecrans map[*ecran]int64
}

const sansRestriction = -1

func (d *diffusion) envoyer(v any) {
	b, _ := json.Marshal(v)
	d.mu.Lock()
	defer d.mu.Unlock()
	for e := range d.ecrans {
		e.envoyer(b)
	}
}

// combat renvoie à chaque appareil le combat qu'il a le droit de voir.
func (d *diffusion) combat() {
	d.mu.Lock()
	defer d.mu.Unlock()
	vus := map[int64][]byte{}
	for e, pj := range d.ecrans {
		msg, ok := vus[pj]
		if !ok {
			msg, _ = json.Marshal(map[string]any{"type": "combat", "combat": d.s.combatJoueurs(pj, pj != sansRestriction)})
			vus[pj] = msg
		}
		e.envoyer(msg)
	}
}

// fiche prévient qu'une feuille de personnage a changé.
func (d *diffusion) fiche(id int64) { d.envoyer(map[string]any{"type": "fiche", "id": id}) }

func (d *diffusion) fermer() {
	d.mu.Lock()
	defer d.mu.Unlock()
	for e := range d.ecrans {
		_ = e.conn.Close()
	}
}

// joueurDirect : WebSocket de la page des joueurs. Il ne fait que recevoir : l'état à la connexion,
// puis le combat à chaque action, les feuilles qui changent, le verrou des feuilles, les combats
// qui se terminent.
func (s *Serveur) joueurDirect(w http.ResponseWriter, r *http.Request) {
	pj, restreint := s.pjDuCompte(r)
	filtre := int64(sansRestriction)
	if restreint {
		filtre = pj
	}
	conn, err := versWebSocket.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	e := &ecran{conn: conn, envoi: make(chan []byte, 32)}
	go e.ecrire()
	debut, _ := json.Marshal(map[string]any{"type": "etat", "edition": s.editionJoueurs(), "combat": s.combatJoueurs(pj, restreint)})
	s.joueurs.mu.Lock()
	s.joueurs.ecrans[e] = filtre
	e.envoyer(debut)
	s.joueurs.mu.Unlock()
	defer func() {
		s.joueurs.mu.Lock()
		delete(s.joueurs.ecrans, e)
		s.joueurs.mu.Unlock()
		close(e.envoi)
	}()
	conn.SetReadLimit(4096)
	for {
		_ = conn.SetReadDeadline(time.Now().Add(90 * time.Second))
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if strings.Contains(string(data), `"ping"`) {
			s.joueurs.mu.Lock()
			e.envoyer([]byte(`{"type":"pong"}`))
			s.joueurs.mu.Unlock()
		}
	}
}

// ---------------------------------------------------------------- routes

// joueurEtat : ce que la page affiche en arrivant. « serveur » : on y entre par un compte, et « pj »
// est alors le PJ que joue ce compte.
func (s *Serveur) joueurEtat(w http.ResponseWriter, r *http.Request) {
	pj, restreint := s.pjDuCompte(r)
	de := 20
	if c := s.lireRegles().Combat; c != nil && c.De > 0 {
		de = c.De
	}
	repondre(w, 200, map[string]any{"serveur": s.acces.comptes != nil, "pj": pj, "edition": s.editionJoueurs(),
		"combat": s.combatJoueurs(pj, restreint), "univers": nomUnivers(s.store.Chemin()), "de": de})
}

// pjJoueur : un PJ dans la liste de la page.
type pjJoueur struct {
	ID      int64  `json:"id"`
	Nom     string `json:"nom"`
	Joueur  string `json:"joueur,omitempty"`
	Peuple  string `json:"peuple,omitempty"`
	Profil  string `json:"profil,omitempty"`
	Niveau  int    `json:"niveau"`
	Statut  string `json:"statut,omitempty"`
	Feuille bool   `json:"feuille"` // sa feuille de personnage existe
}

// joueurPJ : les PJ parmi lesquels choisir le sien (ni morts, ni partis, ni archivés). Sur un
// serveur, un compte « joueur » n'y trouve que le sien.
func (s *Serveur) joueurPJ(w http.ResponseWriter, r *http.Request) {
	pj, restreint := s.pjDuCompte(r)
	liste, err := s.store.Lister(store.Filtre{Type: "pj"})
	if err != nil {
		erreurStore(w, err)
		return
	}
	out := []pjJoueur{}
	for _, x := range liste {
		e, err := s.store.Lire(x.ID)
		if err != nil || e.Archive || absent(e.Data["statut"]) || (restreint && (e.ID != pj || !partagee(e))) {
			continue
		}
		texte := func(k string) string { v, _ := e.Data[k].(string); return v }
		_, _, existe, _ := s.feuilleDe(e)
		out = append(out, pjJoueur{ID: e.ID, Nom: e.Nom, Joueur: texte("joueur"), Peuple: texte("peuple"), Profil: texte("profil"),
			Niveau: niveauDe(e), Statut: texte("statut"), Feuille: existe})
	}
	sort.SliceStable(out, func(i, j int) bool { return store.Plier(out[i].Nom) < store.Plier(out[j].Nom) })
	repondre(w, 200, out)
}

// pjLibre : le PJ visé par une route de la page libre.
func (s *Serveur) pjLibre(w http.ResponseWriter, r *http.Request) (store.Entite, bool) {
	id, _ := idDe(r)
	e, err := s.store.Lire(id)
	if id == 0 || err != nil || e.Type != "pj" || e.Archive {
		erreurStore(w, store.ErrIntrouvable)
		return e, false
	}
	return e, true
}

// feuilleLireLibre : la feuille d'un PJ sur la page libre, sans les notes du MJ.
func (s *Serveur) feuilleLireLibre(w http.ResponseWriter, r *http.Request) {
	e, ok := s.pjLibre(w, r)
	if !ok {
		return
	}
	f, modifie, existe, err := s.feuilleDe(e)
	if err != nil {
		erreurFiche(w, err)
		return
	}
	s.repondreFeuille(w, e, f.PourJoueur(), modifie, existe)
}

// feuilleEcrireLibre : quand le MJ a déverrouillé les feuilles, un joueur tient la sienne (tout,
// sauf les notes du MJ).
func (s *Serveur) feuilleEcrireLibre(w http.ResponseWriter, r *http.Request) {
	e, ok := s.pjLibre(w, r)
	if !ok {
		return
	}
	if !s.editionJoueurs() {
		erreur(w, 403, "Les feuilles sont verrouillées par le MJ.")
		return
	}
	s.enregistrerFeuille(w, r, e, true)
}

// ---------------------------------------------------------------- hauts faits

// combatPasse : un combat terminé, par son récapitulatif.
type combatPasse struct {
	ID    int64         `json:"id"`
	Recap *combat.Recap `json:"recap"`
}

// records d'un PJ sur les combats passés.
type records struct {
	Pj           int64          `json:"pj,omitempty"`
	Nom          string         `json:"nom"`
	Combats      int            `json:"combats"`
	Victoires    int            `json:"victoires"`
	Infliges     int            `json:"infliges"`
	Abattus      int            `json:"abattus"`
	Critiques    int            `json:"critiques"`
	Touches      int            `json:"touches"`
	Tentatives   int            `json:"tentatives"`
	Tombe        int            `json:"tombe"`
	MeilleurCoup int            `json:"meilleur_coup"`
	MeilleurTxt  string         `json:"meilleur_txt"`
	MeilleurOu   string         `json:"meilleur_ou"` // nom du combat
	Titres       map[string]int `json:"titres"`
}

// hautsFaits : les récapitulatifs des combats terminés, du plus récent au plus ancien, et les
// records de chaque PJ. restreint : seulement les combats où ce PJ s'est battu.
func (s *Serveur) hautsFaits(pj int64, restreint bool) ([]combatPasse, []*records) {
	passes := []combatPasse{}
	docs, _ := s.store.ListerDocs(collCombats)
	for _, d := range docs {
		var st combat.Etat
		if json.Unmarshal(d.Data, &st) != nil || !st.Termine || st.Recap == nil {
			continue
		}
		if restreint && (pj == 0 || !participe(&st, pj)) {
			continue
		}
		passes = append(passes, combatPasse{ID: d.ID, Recap: st.Recap})
	}
	sort.SliceStable(passes, func(i, j int) bool { return passes[i].Recap.Date > passes[j].Recap.Date })

	parPj := map[string]*records{}
	var ordre []string
	for i := len(passes) - 1; i >= 0; i-- { // du plus ancien au plus récent
		r := passes[i].Recap
		for _, b := range r.Pjs {
			k := b.Nom
			if b.RefID > 0 {
				k = "#" + strconv.FormatInt(b.RefID, 10)
			}
			rec, ok := parPj[k]
			if !ok {
				rec = &records{Pj: b.RefID, Titres: map[string]int{}}
				parPj[k] = rec
				ordre = append(ordre, k)
			}
			rec.Nom = b.Nom // dernier nom connu
			rec.Combats++
			if r.Issue == "victoire" {
				rec.Victoires++
			}
			rec.Infliges += b.Infliges
			rec.Abattus += b.Abattus
			rec.Critiques += b.Critiques
			rec.Touches += b.Touches
			rec.Tentatives += b.Tentatives
			if b.Tombe {
				rec.Tombe++
			}
			if b.MeilleurCoup > rec.MeilleurCoup {
				rec.MeilleurCoup, rec.MeilleurTxt, rec.MeilleurOu = b.MeilleurCoup, b.MeilleurTxt, r.Nom
			}
			if b.Titre != "" {
				rec.Titres[b.Titre]++
			}
		}
	}
	recs := make([]*records, 0, len(ordre))
	for _, k := range ordre {
		recs = append(recs, parPj[k])
	}
	sort.SliceStable(recs, func(i, j int) bool { return store.Plier(recs[i].Nom) < store.Plier(recs[j].Nom) })
	return passes, recs
}

func (s *Serveur) joueurHautsFaits(w http.ResponseWriter, r *http.Request) {
	passes, recs := s.hautsFaits(s.pjDuCompte(r))
	repondre(w, 200, map[string]any{"combats": passes, "records": recs})
}
