package serveur

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Tchyze/cof_dm/internal/fiches"
	"github.com/Tchyze/cof_dm/internal/modele"
	"github.com/Tchyze/cof_dm/internal/store"
)

// Butin et pot commun (fonctionnalité « butin ») : le butin chiffré d'une rencontre (champ
// « butin_chiffre ») se verse au pot commun de sa campagne à la fin du combat ; le MJ y ajoute ce
// qu'il veut ; les PJ se le partagent par des propositions que le MJ valide (internal/fiches/pot.go).
// Un partage validé passe par la feuille du PJ : sans les feuilles de personnage, le pot garde tout.
// Comme l'inventaire, ces règles se jouent ici, à l'écriture par l'API ; la synchronisation
// transporte leurs résultats.

func (s *Serveur) routesButin() {
	m := s.mux
	m.HandleFunc("GET /api/campagnes/{id}/pot", s.lirePot)
	m.HandleFunc("POST /api/campagnes/{id}/pot/ajouter", s.ajouterAuPot)
	m.HandleFunc("POST /api/campagnes/{id}/pot/retirer", s.retirerDuPot)
	m.HandleFunc("POST /api/campagnes/{id}/pot/propositions", s.proposerPartage)
	m.HandleFunc("POST /api/campagnes/{id}/pot/propositions/{n}/valider", s.validerPartage)
	m.HandleFunc("POST /api/campagnes/{id}/pot/propositions/{n}/refuser", s.refuserPartage)
	m.HandleFunc("POST /api/campagnes/{id}/pot/partager", s.partagerPot)
	m.HandleFunc("POST /api/campagnes/{id}/pot/partages/{n}/annuler", s.annulerPartage)
	m.HandleFunc("POST /api/entites/{id}/verser-butin", s.verserButin)
}

// msgSansFeuilles : un partage n'a nulle part où aller quand les feuilles sont coupées.
const msgSansFeuilles = "Les feuilles de personnage sont désactivées dans cet univers : le partage n'a pas de bourse ni d'inventaire où aller (Paramètres → Fonctionnalités). Le pot garde tout."

// potDe : le pot de la campagne (neuf s'il n'existe pas encore : doc 0) et son document.
func (s *Serveur) potDe(campagne int64) (*fiches.Pot, int64, error) {
	docs, err := s.store.ListerDocs("pots")
	if err != nil {
		return nil, 0, err
	}
	for _, d := range docs {
		p, err := fiches.NormaliserPot(d.Data)
		if err == nil && p.Campagne == campagne {
			return p, d.ID, nil
		}
	}
	return fiches.NouveauPot(campagne), 0, nil
}

func (s *Serveur) enregistrerPot(p *fiches.Pot, doc int64, nom, auteur string) error {
	var err error
	if doc == 0 {
		_, err = s.store.CreerDoc("pots", p.JSON(), nom, auteur)
	} else {
		_, err = s.store.MajDoc("pots", doc, p.JSON(), nom, auteur)
	}
	return err
}

// campagneDe : la fiche de campagne du chemin.
func (s *Serveur) campagneDe(w http.ResponseWriter, r *http.Request) (store.Entite, bool) {
	id, _ := idDe(r)
	e, err := s.store.Lire(id)
	if err != nil || e.Type != "campagne" {
		erreur(w, 404, "Campagne introuvable.")
		return e, false
	}
	return e, true
}

func nomDuPot(campagne store.Entite) string { return "Pot commun de " + campagne.Nom }

// potVu : le pot tel que l'API le montre, avec les noms des fiches qu'il désigne (PJ, rencontres,
// objets du monde) et les objets du monde sous leur nom du moment.
type potVu struct {
	*fiches.Pot
	Doc  int64            `json:"doc,omitempty"`
	Noms map[int64]string `json:"noms"`
}

func (s *Serveur) voirPot(p *fiches.Pot, doc int64) potVu {
	v := potVu{Pot: p, Doc: doc, Noms: map[int64]string{}}
	nom := func(id int64) {
		if id == 0 {
			return
		}
		if _, ok := v.Noms[id]; ok {
			return
		}
		if e, err := s.store.Lire(id); err == nil {
			v.Noms[id] = e.Nom
		}
	}
	for i, o := range p.Objets {
		if o.Fiche != 0 {
			nom(o.Fiche)
			if n, ok := v.Noms[o.Fiche]; ok {
				p.Objets[i].Nom = n
			}
		}
	}
	for _, x := range p.Propositions {
		nom(x.Pj)
	}
	for _, m := range p.Historique {
		nom(m.Pj)
		nom(m.Rencontre)
	}
	return v
}

func (s *Serveur) lirePot(w http.ResponseWriter, r *http.Request) {
	c, ok := s.campagneDe(w, r)
	if !ok {
		return
	}
	p, doc, err := s.potDe(c.ID)
	if err != nil {
		erreurStore(w, err)
		return
	}
	repondre(w, 200, s.voirPot(p, doc))
}

// changerPot : lit le pot de la campagne du chemin, le change et l'enregistre, l'un après l'autre.
// changer renvoie l'action et le détail notés au journal de la campagne.
func (s *Serveur) changerPot(w http.ResponseWriter, r *http.Request, changer func(c store.Entite, p *fiches.Pot) (string, string, error)) {
	c, ok := s.campagneDe(w, r)
	if !ok {
		return
	}
	s.potMu.Lock()
	defer s.potMu.Unlock()
	p, doc, err := s.potDe(c.ID)
	if err != nil {
		erreurStore(w, err)
		return
	}
	action, detail, err := changer(c, p)
	if err != nil {
		erreurPot(w, err)
		return
	}
	if err := s.enregistrerPot(p, doc, nomDuPot(c), ""); err != nil {
		erreurStore(w, err)
		return
	}
	s.store.NoterFiche(action, c.ID, c.Nom, detail, auteurDe(r))
	s.joueurs.pot()
	p, doc, _ = s.potDe(c.ID)
	repondre(w, 200, s.voirPot(p, doc))
}

// errConflitPot : une action refusée par l'état de l'univers (409), et non par sa demande.
type errConflitPot struct{ msg string }

func (e errConflitPot) Error() string { return e.msg }

func erreurPot(w http.ResponseWriter, err error) {
	var c errConflitPot
	if errors.As(err, &c) {
		erreur(w, 409, c.msg)
		return
	}
	erreurFiche(w, err)
}

func numeroDe(r *http.Request) int {
	n, _ := strconv.Atoi(r.PathValue("n"))
	return n
}

func maintenantPot() float64 { return float64(time.Now().UnixNano()) / 1e9 }

// ---------------------------------------------------------------- entrées et sorties du MJ

type demandeAjout struct {
	Pieces  fiches.Pieces       `json:"pieces"`
	Objets  []modele.LigneButin `json:"objets"`
	Libelle string              `json:"libelle"`
}

func (s *Serveur) ajouterAuPot(w http.ResponseWriter, r *http.Request) {
	var in demandeAjout
	if err := lireJSON(r, &in); err != nil {
		erreurStore(w, err)
		return
	}
	s.changerPot(w, r, func(c store.Entite, p *fiches.Pot) (string, string, error) {
		objets, err := s.objetsDuMonde(in.Objets)
		if err != nil {
			return "", "", err
		}
		m, err := p.Ajouter(fiches.Mouvement{Genre: fiches.Entree, Le: maintenantPot(), Par: auteurDe(r), Libelle: strings.TrimSpace(in.Libelle), Pieces: in.Pieces}, objets)
		if err != nil {
			return "", "", err
		}
		return "pot commun : ajout", libelle(m.Libelle, fiches.TexteButin(m.Pieces, m.Objets)), nil
	})
}

// objetsDuMonde : les objets du monde désignés par un butin existent et prennent leur nom du moment.
func (s *Serveur) objetsDuMonde(l []modele.LigneButin) ([]modele.LigneButin, error) {
	out := make([]modele.LigneButin, 0, len(l))
	for _, o := range l {
		if o.Fiche != 0 {
			e, err := s.store.Lire(o.Fiche)
			if err != nil || e.Type != "objet" {
				return nil, store.ErrValidation{Msg: fmt.Sprintf("La fiche n°%d n'est pas un objet du monde.", o.Fiche)}
			}
			o.Nom, o.Quantite = e.Nom, 1
		}
		out = append(out, o)
	}
	return out, nil
}

type demandeRetrait struct {
	Pieces  fiches.Pieces  `json:"pieces"`
	Objets  []fiches.Prise `json:"objets"`
	Libelle string         `json:"libelle"`
}

func (s *Serveur) retirerDuPot(w http.ResponseWriter, r *http.Request) {
	var in demandeRetrait
	if err := lireJSON(r, &in); err != nil {
		erreurStore(w, err)
		return
	}
	s.changerPot(w, r, func(c store.Entite, p *fiches.Pot) (string, string, error) {
		m, err := p.Retirer(fiches.Mouvement{Le: maintenantPot(), Par: auteurDe(r), Libelle: strings.TrimSpace(in.Libelle), Pieces: in.Pieces}, in.Objets)
		if err != nil {
			return "", "", err
		}
		return "pot commun : retrait", libelle(m.Libelle, fiches.TexteButin(m.Pieces, m.Objets)), nil
	})
}

func libelle(l, texte string) string {
	if l == "" {
		return texte
	}
	return l + " : " + texte
}

// ---------------------------------------------------------------- partages

type demandePartage struct {
	Pj     int64          `json:"pj"`
	Pieces fiches.Pieces  `json:"pieces"`
	Objets []fiches.Prise `json:"objets"`
	Note   string         `json:"note"`
}

// pjDuPartage : la fiche du PJ qui reçoit un partage.
func (s *Serveur) pjDuPartage(id int64) (store.Entite, error) {
	e, err := s.store.Lire(id)
	if err != nil || e.Type != "pj" {
		return e, store.ErrValidation{Msg: "pj : la fiche d'un PJ est attendue."}
	}
	return e, nil
}

// proposerPartage : une proposition de partage, que le MJ validera ou refusera.
func (s *Serveur) proposerPartage(w http.ResponseWriter, r *http.Request) {
	var in demandePartage
	if err := lireJSON(r, &in); err != nil {
		erreurStore(w, err)
		return
	}
	s.changerPot(w, r, func(c store.Entite, p *fiches.Pot) (string, string, error) {
		pj, err := s.pjDuPartage(in.Pj)
		if err != nil {
			return "", "", err
		}
		x, err := p.Proposer(fiches.Proposition{Pj: pj.ID, Par: auteurDe(r), Le: maintenantPot(), Pieces: in.Pieces, Objets: in.Objets, Note: in.Note})
		if err != nil {
			return "", "", err
		}
		return "pot commun : partage proposé", pj.Nom + " : " + texteProposition(p, *x), nil
	})
}

// texteProposition : ce que demande une proposition, en clair.
func texteProposition(p *fiches.Pot, x fiches.Proposition) string {
	var objets []fiches.LignePot
	for _, pr := range x.Objets {
		for _, l := range p.Objets {
			if l.ID == pr.Ligne {
				objets = append(objets, fiches.LignePot{Nom: l.Nom, Quantite: pr.Quantite})
			}
		}
	}
	return fiches.TexteButin(x.Pieces, objets)
}

func (s *Serveur) refuserPartage(w http.ResponseWriter, r *http.Request) {
	s.changerPot(w, r, func(c store.Entite, p *fiches.Pot) (string, string, error) {
		x, ok := p.Proposition(numeroDe(r))
		if !ok {
			return "", "", store.ErrValidation{Msg: "Cette proposition n'existe plus."}
		}
		detail := s.nomDe(x.Pj) + " : " + texteProposition(p, *x)
		p.Oublier(x.ID)
		return "pot commun : partage refusé", detail, nil
	})
}

func (s *Serveur) nomDe(id int64) string {
	if e, err := s.store.Lire(id); err == nil {
		return e.Nom
	}
	return fmt.Sprintf("fiche n°%d", id)
}

func (s *Serveur) validerPartage(w http.ResponseWriter, r *http.Request) {
	s.changerPot(w, r, func(c store.Entite, p *fiches.Pot) (string, string, error) {
		x, ok := p.Proposition(numeroDe(r))
		if !ok {
			return "", "", store.ErrValidation{Msg: "Cette proposition n'existe plus."}
		}
		return s.donner(r, p, *x)
	})
}

// partagerPot : le MJ partage directement, sans proposition.
func (s *Serveur) partagerPot(w http.ResponseWriter, r *http.Request) {
	var in demandePartage
	if err := lireJSON(r, &in); err != nil {
		erreurStore(w, err)
		return
	}
	s.changerPot(w, r, func(c store.Entite, p *fiches.Pot) (string, string, error) {
		if err := p.Verifier(in.Pieces, in.Objets); err != nil {
			return "", "", err
		}
		return s.donner(r, p, fiches.Proposition{Pj: in.Pj, Pieces: in.Pieces, Objets: in.Objets, Note: strings.TrimSpace(in.Note)})
	})
}

// donner : le partage x sort du pot et entre dans la feuille du PJ (bourse et inventaire), qui
// s'enregistre d'abord : si elle est refusée, le pot ne change pas.
func (s *Serveur) donner(r *http.Request, p *fiches.Pot, x fiches.Proposition) (string, string, error) {
	if !s.store.Active("feuilles") {
		return "", "", errConflitPot{msgSansFeuilles}
	}
	pj, err := s.pjDuPartage(x.Pj)
	if err != nil {
		return "", "", err
	}
	m, err := p.Partager(x, auteurDe(r), maintenantPot())
	if err != nil {
		return "", "", err
	}
	if err := s.changerFeuille(pj, auteurDe(r), func(f *fiches.Feuille) error {
		f.Recevoir(m.Pieces, m.Objets)
		return nil
	}); err != nil {
		return "", "", err
	}
	texte := fiches.TexteButin(m.Pieces, m.Objets)
	s.store.NoterFiche("butin reçu", pj.ID, pj.Nom, texte, auteurDe(r))
	return "pot commun : partage", pj.Nom + " : " + texte, nil
}

// annulerPartage : ce qu'un partage avait donné quitte la feuille du PJ et revient au pot, si le PJ
// l'a encore.
func (s *Serveur) annulerPartage(w http.ResponseWriter, r *http.Request) {
	s.changerPot(w, r, func(c store.Entite, p *fiches.Pot) (string, string, error) {
		m, ok := p.Mouvement(numeroDe(r))
		if !ok || m.Genre != fiches.Partage {
			return "", "", store.ErrValidation{Msg: "Ce partage n'existe pas."}
		}
		if m.Annule {
			return "", "", store.ErrValidation{Msg: "Ce partage est déjà annulé."}
		}
		if !s.store.Active("feuilles") {
			return "", "", errConflitPot{msgSansFeuilles}
		}
		pj, err := s.pjDuPartage(m.Pj)
		if err != nil {
			return "", "", errConflitPot{"Le PJ de ce partage n'existe plus : rien ne peut revenir au pot."}
		}
		if err := s.changerFeuille(pj, auteurDe(r), func(f *fiches.Feuille) error {
			if err := f.Rendre(m.Pieces, m.Objets); err != nil {
				return errConflitPot{pj.Nom + " : " + strings.TrimPrefix(err.Error(), "Fiche invalide. ")}
			}
			return nil
		}); err != nil {
			return "", "", err
		}
		if err := p.Annuler(m.ID); err != nil {
			return "", "", err
		}
		texte := fiches.TexteButin(m.Pieces, m.Objets)
		s.store.NoterFiche("butin rendu au pot", pj.ID, pj.Nom, texte, auteurDe(r))
		return "pot commun : partage annulé", pj.Nom + " : " + texte, nil
	})
}

// changerFeuille applique changer à la feuille du PJ, la revalide et l'enregistre ; les objets du
// monde reçus ou rendus suivent (Détenteur), comme à l'enregistrement de la feuille.
func (s *Serveur) changerFeuille(pj store.Entite, auteur string, changer func(*fiches.Feuille) error) error {
	f, modifie, _, err := s.feuilleDe(pj)
	if err != nil {
		return err
	}
	avant := append([]fiches.Objet(nil), f.Inventaire...)
	if err := changer(f); err != nil {
		return err
	}
	if err := s.lierInventaire(f); err != nil {
		return store.ErrValidation{Msg: err.Error()}
	}
	if f, err = fiches.NormaliserFeuilleAvec(f.JSON(), niveauDe(pj), s.lireRegles().Personnage, s.effetsDe(f.Voies)); err != nil {
		return err
	}
	if _, err := s.store.EcrireFeuille(pj.ID, f.Stockage(), modifie, auteur); err != nil {
		return err
	}
	s.suivreDetenteurs(pj.ID, avant, f.Inventaire, auteur)
	s.joueurs.fiche(pj.ID)
	return nil
}

// ---------------------------------------------------------------- versement d'une rencontre

type demandeVersement struct {
	Seance int64 `json:"seance"` // la séance du combat (0 : celle de la rencontre)
}

// verserButin : le butin chiffré de la rencontre entre dans le pot commun de sa campagne ; la
// rencontre est jouée, son butin versé (il ne se verse pas deux fois), et la séance le note.
func (s *Serveur) verserButin(w http.ResponseWriter, r *http.Request) {
	var in demandeVersement
	if err := lireJSON(r, &in); err != nil {
		erreurStore(w, err)
		return
	}
	id, _ := idDe(r)
	renc, err := s.store.Lire(id)
	if err != nil || renc.Type != "rencontre" {
		erreur(w, 404, "Rencontre introuvable.")
		return
	}
	s.potMu.Lock()
	defer s.potMu.Unlock()
	if renc, err = s.store.Lire(id); err != nil {
		erreurStore(w, err)
		return
	}
	if v, _ := renc.Data["butin_verse"].(bool); v {
		erreur(w, 409, "Le butin de cette rencontre est déjà versé au pot.")
		return
	}
	b := modele.LireButin(renc.Data["butin_chiffre"])
	if b.Vide() {
		erreur(w, 422, "Cette rencontre n'a pas de butin chiffré (pièces et objets) à verser.")
		return
	}
	camp, err := s.store.Lire(premierID(renc.Data["campagne"]))
	if err != nil || camp.Type != "campagne" {
		erreur(w, 422, "Cette rencontre n'a pas de campagne : le pot commun est celui d'une campagne.")
		return
	}
	seance := in.Seance
	if seance == 0 {
		seance = premierID(renc.Data["seance"])
	}
	if seance != 0 {
		if e, err := s.store.Lire(seance); err != nil || e.Type != "seance" {
			erreur(w, 422, "seance : la fiche d'une séance est attendue.")
			return
		}
	}
	p, doc, err := s.potDe(camp.ID)
	if err != nil {
		erreurStore(w, err)
		return
	}
	objets, err := s.objetsDuMonde(b.Objets)
	if err != nil {
		erreurStore(w, err)
		return
	}
	m, err := p.Ajouter(fiches.Mouvement{Genre: fiches.Entree, Le: maintenantPot(), Par: auteurDe(r), Libelle: renc.Nom, Rencontre: renc.ID, Pieces: b.Pieces}, objets)
	if err != nil {
		erreurPot(w, err)
		return
	}
	// la rencontre d'abord : si elle refuse, le pot ne reçoit rien
	if _, err := s.store.Modifier(renc.ID, store.Maj{Data: map[string]any{"butin_verse": true, "statut": "jouée"}}, auteurDe(r)); err != nil {
		erreurStore(w, err)
		return
	}
	if err := s.enregistrerPot(p, doc, nomDuPot(camp), ""); err != nil {
		erreurStore(w, err)
		return
	}
	texte := fiches.TexteButin(m.Pieces, m.Objets)
	s.store.NoterFiche("pot commun : butin versé", camp.ID, camp.Nom, renc.Nom+" : "+texte, auteurDe(r))
	if seance != 0 {
		s.noterButinSeance(seance, "- "+renc.Nom+" : "+texte, auteurDe(r))
	}
	s.joueurs.pot()
	p, doc, _ = s.potDe(camp.ID)
	repondre(w, 200, s.voirPot(p, doc))
}

// noterButinSeance ajoute une ligne au champ « Butin & récompenses » de la séance.
func (s *Serveur) noterButinSeance(seance int64, ligne, auteur string) {
	e, err := s.store.Lire(seance)
	if err != nil {
		return
	}
	texte, _ := e.Data["butin"].(string)
	texte = strings.TrimSpace(strings.TrimSpace(texte) + "\n" + ligne)
	_, _ = s.store.Modifier(seance, store.Maj{Data: map[string]any{"butin": texte}}, auteur)
}

// ---------------------------------------------------------------- suppressions

// butinSupprime : une campagne effacée emporte son pot ; un objet du monde effacé reste dans les
// pots, en objet courant.
func (s *Serveur) butinSupprime(e store.Entite, auteur string) {
	if e.Type != "campagne" && e.Type != "objet" {
		return
	}
	s.potMu.Lock()
	defer s.potMu.Unlock()
	docs, err := s.store.ListerDocs("pots")
	if err != nil {
		return
	}
	for _, d := range docs {
		p, err := fiches.NormaliserPot(d.Data)
		if err != nil {
			continue
		}
		switch {
		case e.Type == "campagne" && p.Campagne == e.ID:
			_ = s.store.SupprimerDoc("pots", d.ID, nomDuPot(e), auteur)
		case e.Type == "objet":
			for i := range p.Objets {
				if p.Objets[i].Fiche == e.ID {
					p.Objets[i].Nom = e.Nom
				}
			}
			if p.ObjetEfface(e.ID) {
				_, _ = s.store.MajDoc("pots", d.ID, p.JSON(), "", "")
			}
		}
	}
}

// ---------------------------------------------------------------- page des joueurs

// Le pot commun vu par les joueurs : celui de la campagne de leur PJ, ce qu'il contient, les
// propositions en attente, et ce qui a été partagé. Ils y proposent un partage pour leur PJ, et
// retirent leur proposition tant que le MJ ne l'a pas traitée ; rien ne change dans l'univers avant
// la décision du MJ. Ni les fiches du monde (aucun identifiant d'objet), ni qui a écrit quoi.

// routesPotJoueurs : application, rôle « joueur » et page libre.
func (s *Serveur) routesPotJoueurs(m *http.ServeMux) {
	m.HandleFunc("GET /api/joueur/pot", s.potJoueur)
	m.HandleFunc("POST /api/joueur/pot/propositions", s.proposerJoueur)
	m.HandleFunc("POST /api/joueur/pot/propositions/{n}/retirer", s.retirerPropositionJoueur)
}

// pjDuPot : le PJ du joueur (celui du compte sur un serveur, sinon ?pj=) et sa campagne.
func (s *Serveur) pjDuPot(w http.ResponseWriter, r *http.Request, pj int64) (store.Entite, store.Entite, bool) {
	if p, restreint := s.pjDuCompte(r); restreint {
		pj = p
	}
	e, err := s.store.Lire(pj)
	if pj == 0 || err != nil || e.Type != "pj" || e.Archive {
		erreurStore(w, store.ErrIntrouvable)
		return e, store.Entite{}, false
	}
	c, err := s.store.Lire(premierID(e.Data["campagne"]))
	if err != nil || c.Type != "campagne" {
		erreur(w, 404, "Ce personnage n'a pas de campagne : il n'a pas de pot commun.")
		return e, c, false
	}
	return e, c, true
}

type objetJoueur struct {
	Ligne    int    `json:"ligne"`
	Nom      string `json:"nom"`
	Quantite int    `json:"quantite"`
}

type propositionJoueur struct {
	ID      int           `json:"id"`
	Pj      int64         `json:"pj"`
	PjNom   string        `json:"pj_nom"`
	Mienne  bool          `json:"mienne"`
	Le      float64       `json:"le"`
	Pieces  fiches.Pieces `json:"pieces"`
	Objets  []objetJoueur `json:"objets"`
	Note    string        `json:"note,omitempty"`
	Valable bool          `json:"valable"` // le pot a encore ce qu'elle demande
}

type mouvementJoueur struct {
	Genre   string  `json:"genre"`
	Le      float64 `json:"le"`
	PjNom   string  `json:"pj_nom,omitempty"`
	Libelle string  `json:"libelle,omitempty"`
	Texte   string  `json:"texte"`
	Annule  bool    `json:"annule,omitempty"`
}

type potJoueur struct {
	Campagne     string              `json:"campagne"`
	Pieces       fiches.Pieces       `json:"pieces"`
	Objets       []objetJoueur       `json:"objets"`
	Propositions []propositionJoueur `json:"propositions"`
	Historique   []mouvementJoueur   `json:"historique"` // du plus récent au plus ancien, 30 au plus
	Partage      bool                `json:"partage"`    // un partage peut aboutir (feuilles de personnage activées)
}

func (s *Serveur) vuePotJoueur(pj, camp store.Entite, p *fiches.Pot) potJoueur {
	v := potJoueur{Campagne: camp.Nom, Pieces: p.Pieces, Objets: []objetJoueur{}, Propositions: []propositionJoueur{},
		Historique: []mouvementJoueur{}, Partage: s.store.Active("feuilles")}
	vu := s.voirPot(p, 0) // les objets du monde sous leur nom du moment
	for _, o := range vu.Objets {
		v.Objets = append(v.Objets, objetJoueur{Ligne: o.ID, Nom: o.Nom, Quantite: o.Quantite})
	}
	for _, x := range p.Propositions {
		pr := propositionJoueur{ID: x.ID, Pj: x.Pj, PjNom: vu.Noms[x.Pj], Mienne: x.Pj == pj.ID, Le: x.Le, Pieces: x.Pieces,
			Objets: []objetJoueur{}, Note: x.Note, Valable: p.Verifier(x.Pieces, x.Objets) == nil}
		for _, o := range x.Objets {
			nom := "objet parti du pot"
			for _, l := range vu.Objets {
				if l.ID == o.Ligne {
					nom = l.Nom
				}
			}
			pr.Objets = append(pr.Objets, objetJoueur{Ligne: o.Ligne, Nom: nom, Quantite: o.Quantite})
		}
		v.Propositions = append(v.Propositions, pr)
	}
	for i := len(p.Historique) - 1; i >= 0 && len(v.Historique) < 30; i-- {
		m := p.Historique[i]
		libelle := m.Libelle
		if m.Rencontre != 0 {
			libelle = "Butin d'un combat" // le nom d'une rencontre est celui que lui donne le MJ
		}
		v.Historique = append(v.Historique, mouvementJoueur{Genre: m.Genre, Le: m.Le, PjNom: vu.Noms[m.Pj],
			Libelle: libelle, Texte: fiches.TexteButin(m.Pieces, m.Objets), Annule: m.Annule})
	}
	return v
}

// potJoueur : GET /api/joueur/pot?pj= : le pot commun de la campagne du PJ.
func (s *Serveur) potJoueur(w http.ResponseWriter, r *http.Request) {
	pj, camp, ok := s.pjDuPot(w, r, entierParam(r, "pj"))
	if !ok {
		return
	}
	p, _, err := s.potDe(camp.ID)
	if err != nil {
		erreurStore(w, err)
		return
	}
	repondre(w, 200, s.vuePotJoueur(pj, camp, p))
}

// proposerJoueur : {"pj", "pieces", "objets": [{"ligne", "quantite"}], "note"} : une proposition de
// partage pour son PJ, que le MJ validera ou refusera.
func (s *Serveur) proposerJoueur(w http.ResponseWriter, r *http.Request) {
	var in demandePartage
	if err := lireJSON(r, &in); err != nil {
		erreurStore(w, err)
		return
	}
	s.changerPotJoueur(w, r, in.Pj, func(pj store.Entite, p *fiches.Pot) (string, string, error) {
		if len([]rune(in.Note)) > 200 {
			return "", "", store.ErrValidation{Msg: "note : 200 caractères au plus."}
		}
		x, err := p.Proposer(fiches.Proposition{Pj: pj.ID, Par: "joueur de " + pj.Nom, Le: maintenantPot(), Pieces: in.Pieces, Objets: in.Objets, Note: in.Note})
		if err != nil {
			return "", "", err
		}
		return "pot commun : partage proposé", pj.Nom + " : " + texteProposition(p, *x), nil
	})
}

// retirerPropositionJoueur : POST …/propositions/{n}/retirer {"pj"} : le joueur retire sa proposition.
func (s *Serveur) retirerPropositionJoueur(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Pj int64 `json:"pj"`
	}
	if err := lireJSON(r, &in); err != nil {
		erreurStore(w, err)
		return
	}
	s.changerPotJoueur(w, r, in.Pj, func(pj store.Entite, p *fiches.Pot) (string, string, error) {
		x, ok := p.Proposition(numeroDe(r))
		if !ok || x.Pj != pj.ID {
			return "", "", store.ErrValidation{Msg: "Cette proposition n'existe plus."}
		}
		detail := pj.Nom + " : " + texteProposition(p, *x)
		p.Oublier(x.ID)
		return "pot commun : proposition retirée", detail, nil
	})
}

// changerPotJoueur : comme changerPot, pour le pot de la campagne du PJ du joueur.
func (s *Serveur) changerPotJoueur(w http.ResponseWriter, r *http.Request, pj int64, changer func(pj store.Entite, p *fiches.Pot) (string, string, error)) {
	e, camp, ok := s.pjDuPot(w, r, pj)
	if !ok {
		return
	}
	s.potMu.Lock()
	defer s.potMu.Unlock()
	p, doc, err := s.potDe(camp.ID)
	if err != nil {
		erreurStore(w, err)
		return
	}
	action, detail, err := changer(e, p)
	if err != nil {
		erreurPot(w, err)
		return
	}
	if err := s.enregistrerPot(p, doc, nomDuPot(camp), ""); err != nil {
		erreurStore(w, err)
		return
	}
	s.store.NoterFiche(action, camp.ID, camp.Nom, detail, "joueur de "+e.Nom)
	s.joueurs.pot()
	repondre(w, 200, s.vuePotJoueur(e, camp, p))
}

// pot : le pot commun a changé ; les pages des joueurs le relisent.
func (d *diffusion) pot() { d.envoyer(map[string]any{"type": "pot"}) }
