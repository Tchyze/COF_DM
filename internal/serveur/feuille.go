package serveur

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/Tchyze/cof_dm/internal/combat"
	"github.com/Tchyze/cof_dm/internal/fiches"
	"github.com/Tchyze/cof_dm/internal/reference"
	"github.com/Tchyze/cof_dm/internal/store"
)

// Feuille de personnage d'un PJ (reprise de tracker_COF) : caractéristiques, ressources, attaques,
// voies. Elle complète la fiche du PJ, qui garde son nom, son niveau et ses textes. Ses bornes et
// ses valeurs de départ viennent du fichier de règles de l'univers.

func (s *Serveur) routesFeuille() {
	s.mux.HandleFunc("GET /api/entites/{id}/feuille", s.feuilleLire)
	s.mux.HandleFunc("PUT /api/entites/{id}/feuille", s.feuilleEcrire)
	s.mux.HandleFunc("POST /api/feuilles/nouveau-jour", s.nouveauJour)
	s.mux.HandleFunc("POST /api/feuilles/{id}/niveau", s.monterNiveau)
	s.mux.HandleFunc("POST /api/feuilles/{id}/niveau/annuler", s.annulerNiveau)
	s.mux.HandleFunc("POST /api/feuilles/{id}/niveau/accorder", s.accorderNiveau)
}

// niveauDe : le niveau du PJ, lu sur sa fiche (1 s'il n'est pas renseigné).
func niveauDe(e store.Entite) int {
	if n, ok := e.Data["niveau"].(float64); ok && n >= 1 {
		return int(n)
	}
	return 1
}

// feuilleDe renvoie la feuille d'un PJ, recalculée avec son niveau et les règles de l'univers ; une
// fiche qui n'en a pas encore reçoit une feuille vierge (existe : false).
func (s *Serveur) feuilleDe(e store.Entite) (f *fiches.Feuille, modifie float64, existe bool, err error) {
	rg := s.lireRegles().Personnage
	raw, modifie, err := s.store.LireFeuille(e.ID)
	if errors.Is(err, store.ErrIntrouvable) {
		f = fiches.FeuilleNeuve(rg)
		f.PrendreEffets(nil)
		f.Deriver(niveauDe(e), orVide(rg))
		return f, 0, false, nil
	}
	if err != nil {
		return nil, 0, false, err
	}
	// relue sans bornes : une feuille enregistrée reste lisible si les règles ont changé depuis
	if f, err = fiches.RelireFeuille(raw, niveauDe(e)); err != nil {
		return nil, 0, false, err
	}
	f.PrendreEffets(s.effetsDe(f.Voies))
	f.Deriver(niveauDe(e), orVide(rg))
	return f, modifie, true, nil
}

// nouveauJour : les capacités « par jour » des PJ redeviennent utilisables. {"pj": [ids]} : ces PJ
// seulement (absent : tous ceux qui ont une feuille).
func (s *Serveur) nouveauJour(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Pj []int64 `json:"pj"`
	}
	if err := lireJSON(r, &in); err != nil {
		erreurStore(w, err)
		return
	}
	ids := in.Pj
	if len(ids) == 0 {
		liste, err := s.store.Lister(store.Filtre{Type: "pj"})
		if err != nil {
			erreurStore(w, err)
			return
		}
		for _, x := range liste {
			ids = append(ids, x.ID)
		}
	}
	n := 0
	for _, id := range ids {
		e, err := s.store.Lire(id)
		if err != nil || e.Type != "pj" {
			continue
		}
		raw, _, err := s.store.LireFeuille(id)
		if err != nil {
			continue
		}
		f, err := fiches.RelireFeuille(raw, niveauDe(e))
		if err != nil || f.RemettreAZero("jour") == 0 {
			continue
		}
		if _, err := s.store.EcrireFeuille(id, f.Stockage(), 0, auteurDe(r)); err != nil {
			erreurStore(w, err)
			return
		}
		s.joueurs.fiche(id)
		n++
	}
	repondre(w, 200, map[string]int{"feuilles": n})
}

// effetsDe : les bonus permanents des capacités atteintes dans les voies d'une feuille qui sont
// liées à la bibliothèque (même ref, sinon même nom). Une voie libre n'en donne aucun.
func (s *Serveur) effetsDe(voies []fiches.Voie) []fiches.EffetVoie {
	if len(voies) == 0 {
		return nil
	}
	ref := s.lireReference()
	var out []fiches.EffetVoie
	for _, v := range voies {
		lib := voieLiee(ref, v)
		if lib == nil {
			continue
		}
		for r := 0; r < v.Rang && r < len(lib.Rangs); r++ {
			c := lib.Rangs[r]
			for _, b := range c.Bonus {
				out = append(out, fiches.EffetVoie{Cible: b.Cible, Valeur: b.Valeur, ParRang: b.ParRang, Niveau: b.Niveau, Carac: b.Carac,
					Rang: v.Rang, Capacite: c.Nom, Voie: lib.Nom, RangCapa: r + 1})
			}
		}
	}
	return out
}

// capacitesActives : les capacités de la bibliothèque atteintes par le PJ qui ont une action en
// combat ou des utilisations comptées, avec ce qui en a déjà servi (sur la feuille).
func (s *Serveur) capacitesActives(f *fiches.Feuille) []combat.CapaciteActive {
	if len(f.Voies) == 0 {
		return nil
	}
	ref := s.lireReference()
	var out []combat.CapaciteActive
	for _, v := range f.Voies {
		lib := voieLiee(ref, v)
		if lib == nil {
			continue
		}
		for r := 0; r < v.Rang && r < len(lib.Rangs); r++ {
			c := lib.Rangs[r]
			if c.Nom == "" || (c.Action == nil && c.Utilisations == nil) {
				continue
			}
			a := combat.CapaciteActive{Cle: fiches.CleCapacite(lib.Nom, r+1), Nom: c.Nom, Voie: lib.Nom, Rang: r + 1, Texte: c.Texte, Limitee: c.Limitee, Action: c.Action}
			if u := c.Utilisations; u != nil {
				a.Fois, a.Par = u.Fois, u.Par
				if u.Par != "combat" {
					a.Deja = f.Utilisations[a.Cle].N
				}
			}
			out = append(out, a)
		}
	}
	return out
}

// voieAjoutee : le nom d'une voie de la bibliothèque que la nouvelle feuille a et que l'ancienne
// n'avait pas (vide : aucune). Un joueur monte de rang dans ses voies ou en écrit une libre ; ajouter
// une voie de la bibliothèque reste au MJ.
func (s *Serveur) voieAjoutee(avant, apres []fiches.Voie) string {
	ref := s.lireReference()
	deja := map[string]bool{}
	for _, v := range avant {
		if lib := voieLiee(ref, v); lib != nil {
			deja[reference.Ident(lib.Ref, lib.Nom)] = true
		}
	}
	for _, v := range apres {
		if lib := voieLiee(ref, v); lib != nil && !deja[reference.Ident(lib.Ref, lib.Nom)] {
			return lib.Nom
		}
	}
	return ""
}

// voieLiee : la voie de la bibliothèque que désigne une voie de feuille (nil : voie libre).
func voieLiee(ref *reference.Reference, v fiches.Voie) *reference.Voie {
	for i, x := range ref.Voies {
		if v.Ref != "" && x.Ref != "" {
			if v.Ref == x.Ref {
				return &ref.Voies[i]
			}
			continue
		}
		if reference.Cle(x.Nom) == reference.Cle(v.Nom) {
			return &ref.Voies[i]
		}
	}
	return nil
}

// repondreFeuille : la feuille, avec ce qu'il faut pour l'afficher et la modifier (niveau du PJ,
// règles de la feuille dans cet univers).
func (s *Serveur) repondreFeuille(w http.ResponseWriter, e store.Entite, f fiches.Feuille, modifie float64, existe bool) {
	repondre(w, 200, s.vueFeuille(e, f, modifie, existe))
}

func (s *Serveur) vueFeuille(e store.Entite, f fiches.Feuille, modifie float64, existe bool) map[string]any {
	var m map[string]any
	_ = json.Unmarshal(f.JSON(), &m)
	m["entite"], m["niveau"], m["modifie"], m["existe"] = e.ID, niveauDe(e), modifie, existe
	rg := orVide(s.lireRegles().Personnage)
	m["regles"] = rg
	m["emplacements"] = rg.LesEmplacements() // ceux des règles, sinon ceux par défaut
	// les PM max que ce PJ gagne à chaque niveau, selon son profil (montée de niveau)
	if pm := rg.Montee.GainPmDe(profilDe(e)); pm > 0 {
		m["montee_pm"] = pm
	}
	// les capacités atteintes dont les utilisations se comptent, avec ce qui en a servi
	comptees := []combat.CapaciteActive{}
	for _, a := range s.capacitesActives(&f) {
		if a.Fois > 0 {
			a.Action, a.Texte = nil, ""
			comptees = append(comptees, a)
		}
	}
	m["comptees"] = comptees
	return m
}

// niveauPermis refuse à un PJ un niveau au-delà de ce que disent les règles de l'univers.
func (s *Serveur) niveauPermis(typ string, data map[string]any) error {
	n, ok := data["niveau"].(float64)
	rg := s.lireRegles().Personnage
	if typ != "pj" || !ok || rg == nil || rg.NiveauMax <= 0 || int(n) <= rg.NiveauMax {
		return nil
	}
	return store.ErrValidation{Msg: "Niveau : " + strconv.Itoa(rg.NiveauMax) + " au plus."}
}

// pjDe lit la fiche d'un PJ ; toute autre fiche n'a pas de feuille.
func (s *Serveur) pjDe(w http.ResponseWriter, r *http.Request) (store.Entite, bool) {
	id, err := idDe(r)
	if err != nil {
		erreurStore(w, err)
		return store.Entite{}, false
	}
	e, err := s.store.Lire(id)
	if err != nil {
		erreurStore(w, err)
		return e, false
	}
	if e.Type != "pj" {
		erreur(w, 422, "Seuls les PJ ont une feuille de personnage.")
		return e, false
	}
	return e, true
}

func (s *Serveur) feuilleLire(w http.ResponseWriter, r *http.Request) {
	e, ok := s.pjDe(w, r)
	if !ok {
		return
	}
	f, modifie, existe, err := s.feuilleDe(e)
	if err != nil {
		erreurFiche(w, err)
		return
	}
	s.repondreFeuille(w, e, *f, modifie, existe)
}

// enregistrerFeuille valide et enregistre une feuille. garderNotes : les notes du MJ ne viennent pas
// de la requête (un joueur ne les voit ni ne les change).
func (s *Serveur) enregistrerFeuille(w http.ResponseWriter, r *http.Request, e store.Entite, garderNotes bool) {
	corps, err := lireCorps(r, tailleMaxJSON)
	if err != nil {
		erreurStore(w, err)
		return
	}
	var attendu struct {
		Modifie float64 `json:"modifie"`
	}
	_ = json.Unmarshal(corps, &attendu)
	// les bonus des capacités dépendent des voies de la feuille reçue
	var recue struct {
		Voies []fiches.Voie `json:"voies"`
	}
	_ = json.Unmarshal(corps, &recue)
	f, err := fiches.NormaliserFeuilleAvec(corps, niveauDe(e), s.lireRegles().Personnage, s.effetsDe(recue.Voies))
	if err != nil {
		erreurFiche(w, err)
		return
	}
	if garderNotes {
		// ni les notes du MJ, ni l'historique des niveaux, ni le niveau accordé (qui passent par les
		// routes de la montée de niveau)
		f.Notes, f.Niveaux, f.PointsCapacite, f.NiveauAccorde = "", nil, 0, 0
		ancienne, _, existe, err := s.feuilleDe(e)
		if err == nil && existe {
			f.Notes, f.Niveaux, f.PointsCapacite, f.NiveauAccorde = ancienne.Notes, ancienne.Niveaux, ancienne.PointsCapacite, ancienne.NiveauAccorde
		}
		if err == nil {
			if nom := s.voieAjoutee(ancienne.Voies, f.Voies); nom != "" {
				erreur(w, 403, "Seul le MJ ajoute une voie de la bibliothèque à une feuille : « "+nom+" ».")
				return
			}
			// les chiffres des objets restent ceux du MJ ; la feuille se revalide sans eux
			f.GarderChiffres(ancienne, fiches.IDsRecus(corps))
			if f, err = fiches.NormaliserFeuilleAvec(f.JSON(), niveauDe(e), s.lireRegles().Personnage, s.effetsDe(f.Voies)); err != nil {
				erreurFiche(w, err)
				return
			}
			f.Notes, f.Niveaux, f.PointsCapacite, f.NiveauAccorde = ancienne.Notes, ancienne.Niveaux, ancienne.PointsCapacite, ancienne.NiveauAccorde
		}
	}
	modifie, err := s.store.EcrireFeuille(e.ID, f.Stockage(), attendu.Modifie, auteurDe(r))
	if err != nil {
		erreurStore(w, err)
		return
	}
	if garderNotes {
		*f = f.PourJoueur()
	}
	s.joueurs.fiche(e.ID)
	s.repondreFeuille(w, e, *f, modifie, true)
}

func (s *Serveur) feuilleEcrire(w http.ResponseWriter, r *http.Request) {
	if e, ok := s.pjDe(w, r); ok {
		s.enregistrerFeuille(w, r, e, false)
	}
}

// ---------------------------------------------------------------- vue des joueurs

// monPJ : la fiche partagée du PJ que joue le compte. Toute autre fiche répond comme une fiche
// inexistante : un joueur ne lit que sa propre feuille.
func (s *Serveur) monPJ(w http.ResponseWriter, r *http.Request) (store.Entite, bool) {
	id, _ := idDe(r)
	c, _ := compteDe(r)
	e, err := s.store.Lire(id)
	if id == 0 || err != nil || e.Type != "pj" || !partagee(e) || c.Persos[s.store.ID()] != id {
		erreurStore(w, store.ErrIntrouvable)
		return e, false
	}
	return e, true
}

func (s *Serveur) feuilleLireJoueur(w http.ResponseWriter, r *http.Request) {
	e, ok := s.monPJ(w, r)
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

// feuilleEcrireJoueur : quand l'édition par les joueurs est ouverte, le joueur tient lui-même la
// feuille de son PJ (tout, sauf les notes du MJ).
func (s *Serveur) feuilleEcrireJoueur(w http.ResponseWriter, r *http.Request) {
	e, ok := s.monPJ(w, r)
	if !ok {
		return
	}
	if !s.editionJoueurs() {
		erreur(w, 403, "L'édition par les joueurs n'est pas ouverte dans cet univers.")
		return
	}
	s.enregistrerFeuille(w, r, e, true)
}

// ---------------------------------------------------------------- montée de niveau

// pjAvecFeuille : la fiche d'un PJ (id du chemin) et sa feuille, qui doit déjà exister.
func (s *Serveur) pjAvecFeuille(w http.ResponseWriter, r *http.Request) (store.Entite, *fiches.Feuille, bool) {
	e, ok := s.pjDe(w, r)
	if !ok {
		return e, nil, false
	}
	f, _, existe, err := s.feuilleDe(e)
	if err != nil {
		erreurFiche(w, err)
		return e, nil, false
	}
	if !existe {
		erreur(w, 422, "Ce PJ n'a pas encore de feuille : remplissez-la d'abord.")
		return e, nil, false
	}
	return e, f, true
}

// demandeMontee : ce qu'envoie l'écran de la montée de niveau (les choix, et la date de la feuille).
type demandeMontee struct {
	fiches.ChoixMontee
	Modifie float64 `json:"modifie"`
}

// monterNiveau : le MJ fait passer le PJ au niveau suivant, avec les dés lancés en vrai (ou les PV
// saisis) et les rangs choisis dans les voies de la feuille.
func (s *Serveur) monterNiveau(w http.ResponseWriter, r *http.Request) {
	var in demandeMontee
	if err := lireJSON(r, &in); err != nil {
		erreurStore(w, err)
		return
	}
	if e, f, ok := s.pjAvecFeuille(w, r); ok {
		s.monter(w, r, e, f, in, false)
	}
}

// monterNiveauJoueur : le joueur prend le niveau que le MJ lui a accordé, en faisant lui-même ses
// choix. pj : le PJ visé et le droit de le jouer (compte, ou page libre).
func (s *Serveur) monterNiveauJoueur(pj func(http.ResponseWriter, *http.Request) (store.Entite, bool)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in demandeMontee
		if err := lireJSON(r, &in); err != nil {
			erreurStore(w, err)
			return
		}
		e, ok := pj(w, r)
		if !ok {
			return
		}
		f, _, existe, err := s.feuilleDe(e)
		if err != nil {
			erreurFiche(w, err)
			return
		}
		if !existe || f.NiveauAccorde != niveauDe(e)+1 {
			erreur(w, 403, "Le MJ ne vous a pas accordé de nouveau niveau.")
			return
		}
		s.monter(w, r, e, f, in, true)
	}
}

// profilDe : le profil du PJ, lu sur sa fiche.
func profilDe(e store.Entite) string {
	p, _ := e.Data["profil"].(string)
	return p
}

// monter : la montée elle-même, pour le MJ ou pour le joueur (réponse sans les notes du MJ).
func (s *Serveur) monter(w http.ResponseWriter, r *http.Request, e store.Entite, f *fiches.Feuille, in demandeMontee, joueur bool) {
	rg := orVide(s.lireRegles().Personnage)
	avant := niveauDe(e)
	niveau := avant + 1
	if rg.NiveauMax > 0 && niveau > rg.NiveauMax {
		erreur(w, 422, "Niveau : "+strconv.Itoa(rg.NiveauMax)+" au plus.")
		return
	}
	ligne, err := f.Monter(niveau, in.ChoixMontee, profilDe(e), rg, float64(time.Now().UnixMilli())/1000)
	if err != nil {
		erreurFiche(w, err)
		return
	}
	f.PrendreEffets(s.effetsDe(f.Voies))
	f.Deriver(niveau, rg)
	detail := fmt.Sprintf("niveau %d → %d ; %+d PV", avant, niveau, ligne.Pv)
	if ligne.Jet != "" {
		detail += " (" + ligne.Jet + ")"
	}
	if ligne.Pm != 0 {
		detail += fmt.Sprintf(" ; %+d PM", ligne.Pm)
	}
	for _, x := range ligne.Rangs {
		detail += fmt.Sprintf(" ; %s rang %d → %d", x.Voie, x.De, x.A)
	}
	for _, x := range ligne.Etapes {
		detail += fmt.Sprintf(" ; %s : %s", x.Libelle, x.Texte)
	}
	modifie, err := s.store.ChangerNiveau(e.ID, niveau, f.Stockage(), in.Modifie, "montée de niveau", detail, auteurDe(r))
	if err != nil {
		erreurStore(w, err)
		return
	}
	if e.Data == nil {
		e.Data = map[string]any{}
	}
	e.Data["niveau"] = float64(niveau)
	vue := *f
	if joueur {
		vue = f.PourJoueur()
	}
	out := s.vueFeuille(e, vue, modifie, true)
	out["montee"] = ligne
	s.joueurs.fiche(e.ID)
	repondre(w, 200, out)
}

// accorderNiveau : {"accorde": true} accorde au PJ le niveau suivant, que son joueur prendra
// lui-même depuis sa page (PV, rangs) ; false retire l'accord.
func (s *Serveur) accorderNiveau(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Accorde bool    `json:"accorde"`
		Modifie float64 `json:"modifie"`
	}
	if err := lireJSON(r, &in); err != nil {
		erreurStore(w, err)
		return
	}
	e, f, ok := s.pjAvecFeuille(w, r)
	if !ok {
		return
	}
	rg := orVide(s.lireRegles().Personnage)
	f.NiveauAccorde = 0
	if in.Accorde {
		f.NiveauAccorde = niveauDe(e) + 1
		if rg.NiveauMax > 0 && f.NiveauAccorde > rg.NiveauMax {
			erreur(w, 422, "Niveau : "+strconv.Itoa(rg.NiveauMax)+" au plus.")
			return
		}
	}
	modifie, err := s.store.EcrireFeuille(e.ID, f.Stockage(), in.Modifie, auteurDe(r))
	if err != nil {
		erreurStore(w, err)
		return
	}
	s.joueurs.fiche(e.ID)
	s.repondreFeuille(w, e, *f, modifie, true)
}

// annulerNiveau défait la dernière montée de niveau (MJ seulement) : la seule façon de baisser un
// niveau autrement qu'à la main.
func (s *Serveur) annulerNiveau(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Modifie float64 `json:"modifie"`
	}
	if err := lireJSON(r, &in); err != nil {
		erreurStore(w, err)
		return
	}
	e, f, ok := s.pjAvecFeuille(w, r)
	if !ok {
		return
	}
	avant := niveauDe(e)
	ligne, err := f.Annuler(avant)
	if err != nil {
		erreurFiche(w, err)
		return
	}
	niveau := max(1, ligne.Niveau-1)
	rg := orVide(s.lireRegles().Personnage)
	f.PrendreEffets(s.effetsDe(f.Voies))
	f.Deriver(niveau, rg)
	detail := fmt.Sprintf("niveau %d → %d ; %+d PV", avant, niveau, -ligne.Pv)
	for _, x := range ligne.Rangs {
		detail += fmt.Sprintf(" ; %s rang %d → %d", x.Voie, x.A, x.De)
	}
	modifie, err := s.store.ChangerNiveau(e.ID, niveau, f.Stockage(), in.Modifie, "montée de niveau annulée", detail, auteurDe(r))
	if err != nil {
		erreurStore(w, err)
		return
	}
	if e.Data == nil {
		e.Data = map[string]any{}
	}
	e.Data["niveau"] = float64(niveau)
	out := s.vueFeuille(e, *f, modifie, true)
	out["annulee"] = ligne
	s.joueurs.fiche(e.ID)
	repondre(w, 200, out)
}
