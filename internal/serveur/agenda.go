package serveur

import (
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Tchyze/cof_dm/internal/agenda"
	"github.com/Tchyze/cof_dm/internal/comptes"
	"github.com/Tchyze/cof_dm/internal/modele"
	"github.com/Tchyze/cof_dm/internal/store"
)

// Agenda des séances (fonctionnalité « agenda ») : les dates RÉELLES des séances, à ne pas
// confondre avec le calendrier du monde (/api/calendrier). Une séance de l'agenda est une fiche
// « séance » qui a une date réelle ; son heure, sa durée, son lieu, son lien de visio et ce qui est
// dit aux joueurs sont des champs de la fiche. Le calcul (créneaux, conflits, fichier .ics) est dans
// internal/agenda.
//
// Qui est attendu à une séance : les PJ actifs de sa campagne (un PJ mort est remplacé par le
// suivant, sans rien changer à l'agenda) et le MJ. Une personne se reconnaît à son compte sur un
// serveur (le compte dont c'est le PJ), sinon au champ « Joueur » du PJ : la même personne qui joue
// dans deux campagnes de l'univers y est la même.

func (s *Serveur) routesAgenda() {
	m := s.mux
	m.HandleFunc("GET /api/agenda", s.lireAgenda)
	m.HandleFunc("GET /api/agenda/conflits", s.conflitsAgenda)
	m.HandleFunc("GET /api/agenda/ics", s.icsAgenda)
	m.HandleFunc("GET /api/agenda/reglages", s.lireReglagesAgenda)
	m.HandleFunc("PUT /api/agenda/reglages", s.reglerAgenda)
}

// personneMJ : la clé du MJ dans les créneaux (tout l'univers).
const personneMJ = "mj"

// seanceAgenda : une séance telle que l'agenda la montre au MJ.
type seanceAgenda struct {
	ID           int64             `json:"id"`
	Nom          string            `json:"nom"`
	Campagne     *store.Resume     `json:"campagne"`
	Numero       float64           `json:"numero,omitempty"`
	Nature       string            `json:"nature,omitempty"`
	Statut       string            `json:"statut,omitempty"`
	Date         string            `json:"date"`
	Heure        string            `json:"heure,omitempty"`
	Duree        string            `json:"duree,omitempty"`
	Debut        string            `json:"debut"` // RFC 3339, avec le décalage du fuseau de l'univers
	Fin          string            `json:"fin"`
	JourEntier   bool              `json:"jour_entier,omitempty"`
	Lieu         string            `json:"lieu,omitempty"`
	Visio        string            `json:"visio,omitempty"`
	Infos        string            `json:"infos,omitempty"`
	Participants []participant     `json:"participants"`
	Conflits     []conflitAffiche  `json:"conflits"`
	Modifie      float64           `json:"modifie"`
	debut, fin   time.Time         // pour trier et pour le .ics
	personnes    map[string]string // clé → nom affiché
}

// participant : un PJ attendu à la séance, et qui le joue.
type participant struct {
	PJ      int64  `json:"pj"`
	Nom     string `json:"nom"`
	Joueur  string `json:"joueur,omitempty"`
	Reponse string `json:"reponse,omitempty"` // présent, peut-être, absent ; vide : pas encore répondu
}

// conflitAffiche : une autre séance qui gêne, et qui est concerné (noms affichés).
type conflitAffiche struct {
	Seance    int64    `json:"seance,omitempty"` // absent pour un autre univers : rien n'en sort
	Nom       string   `json:"nom,omitempty"`
	Campagne  string   `json:"campagne,omitempty"`
	Debut     string   `json:"debut,omitempty"`
	Genre     string   `json:"genre"`
	Personnes []string `json:"personnes"`
}

// fuseau : le fuseau horaire de l'univers (celui par défaut s'il était illisible).
func (s *Serveur) fuseau() *time.Location {
	l, err := modele.Fuseau(s.store.Fuseau())
	if err != nil {
		l, _ = modele.Fuseau("")
	}
	return l
}

// joueursDesPJ : sur un serveur, le compte qui joue chaque PJ de l'univers.
func (s *Serveur) joueursDesPJ() map[int64]string {
	out := map[int64]string{}
	if s.acces.comptes == nil {
		return out
	}
	id := s.store.ID()
	for _, c := range s.acces.comptes.Liste() {
		if pj := c.Persos[id]; pj != 0 && !c.Inactif {
			out[pj] = c.Nom
		}
	}
	return out
}

// seancesAgenda : toutes les séances datées de l'univers (non archivées), placées dans le temps,
// avec leurs participants et leurs conflits, ceux des autres univers du serveur compris.
func (s *Serveur) seancesAgenda() ([]*seanceAgenda, error) { return s.seancesAvec(true) }

// seancesLocales : les mêmes, sans regarder les autres univers (ce que ceux-ci en lisent).
func (s *Serveur) seancesLocales() ([]*seanceAgenda, error) { return s.seancesAvec(false) }

func (s *Serveur) seancesAvec(ailleurs bool) ([]*seanceAgenda, error) {
	tout, err := s.store.Toutes()
	if err != nil {
		return nil, err
	}
	fiches := map[int64]store.Entite{}
	for _, e := range tout {
		fiches[e.ID] = e
	}
	parCompte := s.joueursDesPJ()
	// les PJ actifs de chaque campagne
	actifs := map[int64][]store.Entite{}
	for _, e := range tout {
		if e.Type != "pj" || e.Archive {
			continue
		}
		if st, _ := e.Data["statut"].(string); st != "" && st != "actif" {
			continue
		}
		for _, c := range idsDe(e.Data["campagne"]) {
			actifs[c] = append(actifs[c], e)
		}
	}
	l, fz := []*seanceAgenda{}, s.fuseau()
	texte := func(e store.Entite, k string) string { v, _ := e.Data[k].(string); return strings.TrimSpace(v) }
	for _, e := range tout {
		if e.Type != "seance" || e.Archive {
			continue
		}
		debut, fin, entier, ok := agenda.Placer(texte(e, "date_reelle"), texte(e, "heure"), texte(e, "duree"), fz)
		if !ok {
			continue
		}
		x := &seanceAgenda{ID: e.ID, Nom: e.Nom, Numero: num(e.Data["numero"]), Nature: texte(e, "nature"),
			Statut: texte(e, "statut"), Date: texte(e, "date_reelle"), Heure: texte(e, "heure"), Duree: texte(e, "duree"),
			JourEntier: entier, Lieu: texte(e, "lieu_reel"), Visio: texte(e, "visio"), Infos: texte(e, "infos_joueurs"),
			Participants: []participant{}, Conflits: []conflitAffiche{}, Modifie: e.Modifie,
			debut: debut, fin: fin, personnes: map[string]string{personneMJ: "le MJ"}}
		x.Debut, x.Fin = debut.Format(time.RFC3339), fin.Format(time.RFC3339)
		if ids := idsDe(e.Data["campagne"]); len(ids) > 0 {
			if c, ok := fiches[ids[0]]; ok && c.Type == "campagne" {
				x.Campagne = &store.Resume{ID: c.ID, Type: c.Type, Nom: c.Nom}
				attendus := actifs[c.ID]
				// une séance jouée : ceux qui y étaient (présents cochés), sinon ceux qui étaient prévus
				if x.Statut == "jouée" {
					for _, cle := range []string{"presents", "prevus"} {
						if ids := idsDe(e.Data[cle]); len(ids) > 0 {
							attendus = nil
							for _, id := range ids {
								if pj, ok := fiches[id]; ok && pj.Type == "pj" {
									attendus = append(attendus, pj)
								}
							}
							break
						}
					}
				}
				reponses := map[int64]string{}
				for _, rj := range modele.LireReponses(e.Data["reponses"]) {
					reponses[rj.PJ] = rj.Reponse
				}
				for _, pj := range attendus {
					x.Participants = append(x.Participants, participant{PJ: pj.ID, Nom: pj.Nom, Joueur: texte(pj, "joueur"), Reponse: reponses[pj.ID]})
					if reponses[pj.ID] == "absent" {
						continue // il a dit qu'il ne viendrait pas : cette séance ne le gêne en rien
					}
					joueur, cle := texte(pj, "joueur"), ""
					switch {
					case parCompte[pj.ID] != "": // la personne est son compte ; son nom reste celui écrit sur le PJ
						cle = "compte:" + parCompte[pj.ID]
						if joueur == "" {
							joueur = parCompte[pj.ID]
						}
					case joueur != "":
						cle = "joueur:" + strings.ToLower(joueur)
					default:
						cle = "pj:" + strconv.FormatInt(pj.ID, 10)
					}
					x.personnes[cle] = joueur
					if joueur == "" {
						x.personnes[cle] = pj.Nom
					}
					if p := &x.Participants[len(x.Participants)-1]; p.Joueur == "" {
						p.Joueur = joueur // le nom de son compte, faute d'un nom écrit sur le PJ
					}
				}
			}
		}
		l = append(l, x)
	}
	sort.Slice(l, func(i, j int) bool {
		if !l[i].debut.Equal(l[j].debut) {
			return l[i].debut.Before(l[j].debut)
		}
		return l[i].ID < l[j].ID
	})
	parID := map[int64]*seanceAgenda{}
	var creneaux []agenda.Creneau
	for _, x := range l {
		parID[x.ID] = x
		if x.Statut == "annulée" {
			continue // une séance annulée ne gêne personne
		}
		creneaux = append(creneaux, x.creneau())
	}
	for id, cs := range agenda.Conflits(creneaux) {
		x := parID[id]
		for _, c := range cs {
			x.Conflits = append(x.Conflits, parID[c.Autre].conflitPour(x, c))
		}
	}
	if !ailleurs {
		return l, nil
	}
	if ext := s.creneauxAilleurs(); len(ext) > 0 {
		mjs := s.comptesMJ()
		for _, x := range l {
			if x.Statut == "annulée" {
				continue
			}
			if c, ok := x.conflitAilleurs(mjs, ext); ok {
				x.Conflits = append(x.Conflits, c)
			}
		}
	}
	return l, nil
}

// ---------------------------------------------------------------- conflits entre univers (serveur)

// Sur un serveur, la même personne (un compte) peut jouer ou mener dans plusieurs univers. Le MJ de
// l'univers A apprend seulement qu'une de ses séances gêne « quelque chose » ailleurs pour tel joueur :
// ni l'univers, ni la séance, ni la campagne, ni même l'heure. Les autres univers ne livrent à A que
// des créneaux anonymes (début, fin, comptes attendus), jamais un nom ni un identifiant de fiche.

// genreAilleurs : une séance d'un autre univers du serveur gêne (chevauchement ou moins d'une heure).
const genreAilleurs = "ailleurs"

// creneauExterne : une séance d'un autre univers, réduite à ce qui permet de voir un conflit.
type creneauExterne struct {
	debut, fin time.Time
	jourEntier bool
	comptes    []string // « compte:<nom> » : ceux qui y sont attendus (joueurs et MJ de cet univers)
}

// comptesMJ : les comptes MJ de cet univers, en clés de personne.
func (s *Serveur) comptesMJ() []string {
	if s.acces.comptes == nil {
		return nil
	}
	var l []string
	for _, c := range s.acces.comptes.Liste() {
		if !c.Inactif && c.Role(s.store.ID()) == comptes.MJ {
			l = append(l, "compte:"+c.Nom)
		}
	}
	return l
}

// creneauxAilleurs : sur un serveur, les créneaux des autres univers (nil sur le bureau).
func (s *Serveur) creneauxAilleurs() []creneauExterne {
	if s.opt.ailleurs == nil {
		return nil
	}
	return s.opt.ailleurs(s.store.ID())
}

// creneauxPourAilleurs : les séances à venir de cet univers (agenda activé, non annulées), réduites à
// leurs créneaux et à leurs comptes.
func (s *Serveur) creneauxPourAilleurs() []creneauExterne {
	if !s.store.Active("agenda") {
		return nil
	}
	l, err := s.seancesLocales()
	if err != nil {
		return nil
	}
	mjs := s.comptesMJ()
	hier := time.Now().Add(-24 * time.Hour)
	var out []creneauExterne
	for _, x := range l {
		if x.Statut == "annulée" || x.fin.Before(hier) {
			continue
		}
		c := creneauExterne{debut: x.debut, fin: x.fin, jourEntier: x.JourEntier, comptes: append([]string(nil), mjs...)}
		for k := range x.personnes {
			if strings.HasPrefix(k, "compte:") {
				c.comptes = append(c.comptes, k)
			}
		}
		out = append(out, c)
	}
	return out
}

// creneauxAilleurs : ceux de tous les univers servis sauf celui-ci, chacun compté pendant sa lecture.
func (a *Application) creneauxAilleurs(sauf string) []creneauExterne {
	a.mu.RLock()
	var ouverts []*universOuvert
	for _, o := range a.tous {
		if o.st.ID() != sauf {
			o.requetes.entrer()
			ouverts = append(ouverts, o)
		}
	}
	a.mu.RUnlock()
	var out []creneauExterne
	for _, o := range ouverts {
		out = append(out, o.srv.creneauxPourAilleurs()...)
		o.requetes.sortir()
	}
	return out
}

// conflitAilleurs : un seul conflit pour tous les créneaux d'ailleurs qui gênent cette séance, avec
// les personnes concernées telles que cet univers les nomme.
func (x *seanceAgenda) conflitAilleurs(mjs []string, ext []creneauExterne) (conflitAffiche, bool) {
	ici := map[string]string{}
	for k, nom := range x.personnes {
		if strings.HasPrefix(k, "compte:") {
			ici[k] = nom
		}
	}
	for _, k := range mjs {
		if _, joue := ici[k]; !joue {
			ici[k] = "le MJ"
		}
	}
	moi := agenda.Creneau{Debut: x.debut, Fin: x.fin, JourEntier: x.JourEntier}
	qui := map[string]bool{}
	for _, e := range ext {
		if agenda.Relation(moi, agenda.Creneau{Debut: e.debut, Fin: e.fin, JourEntier: e.jourEntier}) == "" {
			continue
		}
		for _, k := range e.comptes {
			if nom, ok := ici[k]; ok {
				qui[nom] = true
			}
		}
	}
	if len(qui) == 0 {
		return conflitAffiche{}, false
	}
	c := conflitAffiche{Genre: genreAilleurs, Personnes: []string{}}
	for nom := range qui {
		c.Personnes = append(c.Personnes, nom)
	}
	sort.Strings(c.Personnes)
	return c, true
}

func (x *seanceAgenda) creneau() agenda.Creneau {
	c := agenda.Creneau{ID: x.ID, Debut: x.debut, Fin: x.fin, JourEntier: x.JourEntier}
	for k := range x.personnes {
		c.Personnes = append(c.Personnes, k)
	}
	return c
}

// conflitPour : cette séance (l'autre), vue depuis la séance x qu'elle gêne.
func (autre *seanceAgenda) conflitPour(x *seanceAgenda, c agenda.Conflit) conflitAffiche {
	a := conflitAffiche{Seance: autre.ID, Nom: autre.Nom, Debut: autre.Debut, Genre: c.Genre, Personnes: []string{}}
	if autre.Campagne != nil {
		a.Campagne = autre.Campagne.Nom
	}
	for _, k := range c.Personnes {
		nom := x.personnes[k]
		if nom == "" {
			nom = autre.personnes[k]
		}
		a.Personnes = append(a.Personnes, nom)
	}
	return a
}

// lireAgenda : GET /api/agenda[?campagne=<id>][&depuis=AAAA-MM-JJ] — les séances datées, de la plus
// proche à la plus lointaine, avec leurs participants et leurs conflits.
func (s *Serveur) lireAgenda(w http.ResponseWriter, r *http.Request) {
	l, err := s.seancesAgenda()
	if err != nil {
		erreurStore(w, err)
		return
	}
	camp := entierParam(r, "campagne")
	l = filtrerAgenda(l, camp, r.URL.Query().Get("depuis"))
	out := map[string]any{"fuseau": s.store.Fuseau(), "seances": l}
	if camp != 0 {
		n, err := s.numeroSuivant(camp)
		if err != nil {
			erreurStore(w, err)
			return
		}
		out["numero_suivant"] = n
	}
	repondre(w, 200, out)
}

// numeroSuivant : le numéro qui suit la plus haute séance de la campagne (datée ou non).
func (s *Serveur) numeroSuivant(campagne int64) (int, error) {
	tout, err := s.store.Toutes()
	if err != nil {
		return 0, err
	}
	n := 0
	for _, e := range tout {
		if e.Type != "seance" {
			continue
		}
		if ids := idsDe(e.Data["campagne"]); len(ids) > 0 && ids[0] == campagne && int(num(e.Data["numero"])) > n {
			n = int(num(e.Data["numero"]))
		}
	}
	return n + 1, nil
}

func filtrerAgenda(l []*seanceAgenda, campagne int64, depuis string) []*seanceAgenda {
	out := []*seanceAgenda{}
	for _, x := range l {
		if campagne != 0 && (x.Campagne == nil || x.Campagne.ID != campagne) {
			continue
		}
		if depuis != "" && x.Date < depuis {
			continue
		}
		out = append(out, x)
	}
	return out
}

// conflitsAgenda : GET /api/agenda/conflits?date=&heure=&duree=&campagne=[&seance=] — les conflits
// qu'aurait une séance placée là, avant de l'enregistrer (seance : celle qu'on déplace, mise de côté).
func (s *Serveur) conflitsAgenda(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	debut, fin, entier, ok := agenda.Placer(q.Get("date"), q.Get("heure"), q.Get("duree"), s.fuseau())
	if !ok {
		erreur(w, 400, "Date AAAA-MM-JJ attendue.")
		return
	}
	l, err := s.seancesLocales()
	if err != nil {
		erreurStore(w, err)
		return
	}
	// la séance essayée : les personnes de sa campagne, prises dans une séance de la même campagne
	// si elle en a, sinon recalculées
	essai := &seanceAgenda{ID: -1, debut: debut, fin: fin, JourEntier: entier, personnes: map[string]string{personneMJ: "le MJ"}}
	if c := entierParam(r, "campagne"); c != 0 {
		pers, err := s.personnesDeCampagne(c)
		if err != nil {
			erreurStore(w, err)
			return
		}
		for k, v := range pers {
			essai.personnes[k] = v
		}
	}
	sauf := entierParam(r, "seance")
	creneaux := []agenda.Creneau{essai.creneau()}
	parID := map[int64]*seanceAgenda{}
	for _, x := range l {
		if x.ID == sauf || x.Statut == "annulée" {
			continue
		}
		parID[x.ID] = x
		creneaux = append(creneaux, x.creneau())
	}
	out := []conflitAffiche{}
	for _, c := range agenda.Conflits(creneaux)[-1] {
		out = append(out, parID[c.Autre].conflitPour(essai, c))
	}
	if ext := s.creneauxAilleurs(); len(ext) > 0 {
		if c, ok := essai.conflitAilleurs(s.comptesMJ(), ext); ok {
			out = append(out, c)
		}
	}
	repondre(w, 200, out)
}

// personnesDeCampagne : les personnes attendues à une séance de cette campagne (hors MJ).
func (s *Serveur) personnesDeCampagne(campagne int64) (map[string]string, error) {
	ids, err := s.store.PJActifs(campagne)
	if err != nil {
		return nil, err
	}
	parCompte := s.joueursDesPJ()
	out := map[string]string{}
	for _, id := range ids {
		pj, err := s.store.Lire(id)
		if err != nil {
			continue
		}
		joueur, _ := pj.Data["joueur"].(string)
		joueur = strings.TrimSpace(joueur)
		switch {
		case parCompte[id] != "":
			if joueur == "" {
				joueur = parCompte[id]
			}
			out["compte:"+parCompte[id]] = joueur
		case joueur != "":
			out["joueur:"+strings.ToLower(joueur)] = joueur
		default:
			out["pj:"+strconv.FormatInt(id, 10)] = pj.Nom
		}
	}
	return out, nil
}

// icsAgenda : GET /api/agenda/ics[?campagne=<id>] — l'agenda en fichier iCalendar, à importer dans
// Google Agenda, Apple Calendrier, Proton Calendar, Outlook… Il ne contient que ce qu'il faut pour
// s'organiser, jamais une note du MJ.
func (s *Serveur) icsAgenda(w http.ResponseWriter, r *http.Request) {
	l, err := s.seancesLocales()
	if err != nil {
		erreurStore(w, err)
		return
	}
	l = filtrerAgenda(l, entierParam(r, "campagne"), "")
	nom := nomUnivers(s.store.Chemin())
	s.envoyerICS(w, nom, s.evenements(l))
}

// evenements : les séances, telles qu'elles partent dans un fichier .ics.
func (s *Serveur) evenements(l []*seanceAgenda) []agenda.Evenement {
	out := make([]agenda.Evenement, 0, len(l))
	for _, x := range l {
		e := agenda.Evenement{UID: "seance-" + strconv.FormatInt(x.ID, 10) + "-" + s.store.ID() + "@cof-dm",
			Titre: x.Nom, Debut: x.debut, Fin: x.fin, JourEntier: x.JourEntier, Annulee: x.Statut == "annulée",
			Modifie: time.Unix(int64(x.Modifie), 0)}
		if x.Campagne != nil {
			e.Titre += " · " + x.Campagne.Nom
		}
		e.Lieu = x.Lieu
		if x.Visio != "" {
			if e.Lieu == "" {
				e.Lieu = x.Visio
			}
			if u, err := url.Parse(x.Visio); err == nil && (u.Scheme == "https" || u.Scheme == "http") {
				e.URL = x.Visio
			}
		}
		var d []string
		if x.Nature != "" && x.Nature != "séance" {
			r, n := utf8.DecodeRuneInString(x.Nature)
			d = append(d, string(unicode.ToUpper(r))+x.Nature[n:])
		}
		if len(x.Participants) > 0 {
			var p []string
			for _, pt := range x.Participants {
				nom := pt.Nom
				if pt.Joueur != "" {
					nom = pt.Joueur + " (" + pt.Nom + ")"
				}
				if pt.Reponse != "" {
					nom += " : " + pt.Reponse
				}
				p = append(p, nom)
			}
			d = append(d, "Participants : "+strings.Join(p, ", "))
		}
		if x.Visio != "" {
			d = append(d, "Visio : "+x.Visio)
		}
		if x.Infos != "" {
			d = append(d, sansLiens(x.Infos)) // un lien [[Nom]] vers une fiche ne dit rien hors de l'application
		}
		e.Description = strings.Join(d, "\n\n")
		out = append(out, e)
	}
	return out
}

func (s *Serveur) envoyerICS(w http.ResponseWriter, nom string, ev []agenda.Evenement) {
	fichier := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`\/:*?"<>|`, r) || r < 32 {
			return '-'
		}
		return r
	}, nom)
	h := w.Header()
	h.Set("Content-Type", "text/calendar; charset=utf-8")
	h.Set("Content-Disposition", `attachment; filename="agenda.ics"; filename*=UTF-8''`+url.PathEscape("Agenda - "+fichier+".ics"))
	h.Set("Cache-Control", "no-store")
	w.WriteHeader(200)
	_, _ = w.Write(agenda.ICS("Agenda — "+nom, ev, time.Now()))
}

// lireReglagesAgenda : GET /api/agenda/reglages — le fuseau horaire de l'univers.
func (s *Serveur) lireReglagesAgenda(w http.ResponseWriter, r *http.Request) {
	repondre(w, 200, map[string]any{"fuseau": s.store.Fuseau(), "defaut": modele.FuseauParDefaut,
		"durees": modele.DureesSeance, "duree_defaut": modele.TexteDuree(modele.DureeParDefaut)})
}

// reglerAgenda : PUT /api/agenda/reglages {"fuseau": "Europe/Paris"}.
func (s *Serveur) reglerAgenda(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Fuseau string `json:"fuseau"`
	}
	if err := lireJSON(r, &in); err != nil {
		erreurStore(w, err)
		return
	}
	if err := s.store.ReglerFuseau(strings.TrimSpace(in.Fuseau), auteurDe(r)); err != nil {
		erreurStore(w, err)
		return
	}
	s.lireReglagesAgenda(w, r)
}

// ---------------------------------------------------------------- page des joueurs

// Un joueur voit les séances de SES campagnes, sans pouvoir les modifier : date, heures, lieu, visio,
// ce que le MJ leur dit et qui est attendu. Jamais un conflit (il citerait une autre campagne), une
// note ni une préparation du MJ. Sur un serveur, ses campagnes sont celles du PJ de son compte ; à la
// maison, celles de tous les PJ que joue la même personne (même champ « Joueur »).

func (s *Serveur) routesAgendaJoueurs(m *http.ServeMux) {
	m.HandleFunc("GET /api/joueur/agenda", s.agendaJoueur)
	m.HandleFunc("GET /api/joueur/agenda/ics", s.icsJoueur)
	m.HandleFunc("POST /api/joueur/agenda/{id}/reponse", s.repondreJoueur)
}

// seanceJoueur : une séance telle qu'un joueur la voit.
type seanceJoueur struct {
	ID           int64         `json:"id"`
	Nom          string        `json:"nom"`
	Campagne     string        `json:"campagne,omitempty"`
	Nature       string        `json:"nature,omitempty"`
	Statut       string        `json:"statut,omitempty"`
	Date         string        `json:"date"`
	Debut        string        `json:"debut"`
	Fin          string        `json:"fin"`
	JourEntier   bool          `json:"jour_entier,omitempty"`
	Lieu         string        `json:"lieu,omitempty"`
	Visio        string        `json:"visio,omitempty"`
	Infos        string        `json:"infos,omitempty"`
	Participants []participant `json:"participants"`
	MonPJ        int64         `json:"mon_pj"` // le PJ qui répond pour lui à cette séance
}

// seancesDuJoueur : les séances à venir (pas encore finies) des campagnes du PJ visé.
// mesPJ : pour chaque campagne du joueur, le PJ qui y répond pour lui.
func (s *Serveur) seancesDuJoueur(w http.ResponseWriter, r *http.Request) (l []*seanceAgenda, pj store.Entite, mesPJ map[int64]int64, ok bool) {
	id, ok := s.pjDocuments(w, r)
	if !ok {
		return nil, store.Entite{}, nil, false
	}
	pj, err := s.store.Lire(id)
	if err != nil {
		erreurStore(w, err)
		return nil, pj, nil, false
	}
	_, restreint := s.pjDuCompte(r)
	campagnes := map[int64]int64{}
	for _, c := range idsDe(pj.Data["campagne"]) {
		campagnes[c] = pj.ID
	}
	if joueur := cleJoueur(pj); !restreint && joueur != "" {
		tout, err := s.store.Toutes()
		if err != nil {
			erreurStore(w, err)
			return nil, pj, nil, false
		}
		for _, e := range tout {
			st, _ := e.Data["statut"].(string)
			if e.Type == "pj" && !e.Archive && (st == "" || st == "actif") && cleJoueur(e) == joueur {
				for _, c := range idsDe(e.Data["campagne"]) {
					if campagnes[c] == 0 {
						campagnes[c] = e.ID
					}
				}
			}
		}
	}
	tous, err := s.seancesLocales()
	if err != nil {
		erreurStore(w, err)
		return nil, pj, nil, false
	}
	maintenant := time.Now()
	l = []*seanceAgenda{}
	for _, x := range tous {
		if x.Campagne != nil && campagnes[x.Campagne.ID] != 0 && x.fin.After(maintenant) {
			l = append(l, x)
		}
	}
	return l, pj, campagnes, true
}

func cleJoueur(pj store.Entite) string {
	v, _ := pj.Data["joueur"].(string)
	return strings.ToLower(strings.TrimSpace(v))
}

// agendaJoueur : GET /api/joueur/agenda[?pj=] — les séances à venir des campagnes du joueur.
func (s *Serveur) agendaJoueur(w http.ResponseWriter, r *http.Request) {
	l, _, mesPJ, ok := s.seancesDuJoueur(w, r)
	if !ok {
		return
	}
	out := make([]seanceJoueur, 0, len(l))
	for _, x := range l {
		v := seanceJoueur{ID: x.ID, Nom: x.Nom, Nature: x.Nature, Statut: x.Statut, Date: x.Date, Debut: x.Debut, Fin: x.Fin,
			JourEntier: x.JourEntier, Lieu: x.Lieu, Visio: x.Visio, Infos: sansLiens(x.Infos), Participants: x.Participants}
		if x.Campagne != nil {
			v.Campagne, v.MonPJ = x.Campagne.Nom, mesPJ[x.Campagne.ID]
		}
		out = append(out, v)
	}
	repondre(w, 200, map[string]any{"fuseau": s.store.Fuseau(), "seances": out})
}

// icsJoueur : GET /api/joueur/agenda/ics[?pj=] — ces mêmes séances en fichier iCalendar.
func (s *Serveur) icsJoueur(w http.ResponseWriter, r *http.Request) {
	l, pj, _, ok := s.seancesDuJoueur(w, r)
	if !ok {
		return
	}
	s.envoyerICS(w, nomUnivers(s.store.Chemin())+" — "+pj.Nom, s.evenements(l))
}

// repondreJoueur : POST /api/joueur/agenda/{id}/reponse?pj= {"reponse": "présent" | "peut-être" |
// "absent" | ""} — le joueur dit s'il vient à une séance à venir de ses campagnes, pour le PJ qu'il y
// joue (sur un serveur, celui de son compte). Une séance annulée, finie ou d'une autre campagne :
// 404. Seule écriture de l'agenda ouverte aux joueurs ; elle ne touche qu'à sa réponse.
func (s *Serveur) repondreJoueur(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Reponse string `json:"reponse"`
	}
	if err := lireJSON(r, &in); err != nil {
		erreurStore(w, err)
		return
	}
	permise := in.Reponse == ""
	for _, o := range modele.ReponsesSeance {
		permise = permise || in.Reponse == o
	}
	if !permise {
		erreur(w, 422, "Réponse attendue : "+strings.Join(modele.ReponsesSeance, ", ")+".")
		return
	}
	seance, err := idDe(r)
	if err != nil {
		erreurStore(w, err)
		return
	}
	l, _, mesPJ, ok := s.seancesDuJoueur(w, r)
	if !ok {
		return
	}
	for _, x := range l {
		if x.ID != seance || x.Statut == "annulée" {
			continue
		}
		if _, err := s.store.Repondre(seance, mesPJ[x.Campagne.ID], in.Reponse, auteurDe(r)); err != nil {
			erreurStore(w, err)
			return
		}
		s.agendaJoueur(w, r)
		return
	}
	erreurStore(w, store.ErrIntrouvable)
}
