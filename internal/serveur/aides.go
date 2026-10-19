package serveur

import (
	"bytes"
	"encoding/json"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Tchyze/cof_dm/internal/modele"
	"github.com/Tchyze/cof_dm/internal/store"
)

// Aides de jeu (projet 9) : une fiche « aide » (lettre, image, indice) que le MJ envoie à un,
// plusieurs ou tous les PJ. Chaque envoi est une relation « recu » d'un PJ vers l'aide, avec la
// séance pour contexte : elle se synchronise, se journalise et disparaît avec l'une des deux fiches.
// Les joueurs retrouvent leurs aides dans l'onglet « Documents » de leur page, à jour de ce que le
// MJ y corrige, réduites au nom, à la forme, au texte remis et aux images : jamais le secret dont
// c'est un indice ni les notes du MJ. Une aide n'est jamais une fiche partagée (partagee).

const relRecu = "recu"

func (s *Serveur) routesAides() {
	s.mux.HandleFunc("GET /api/aides", s.listerAides)
	s.mux.HandleFunc("POST /api/aides/{id}/envoyer", s.envoyerAide)
}

// routesDocumentsJoueurs : les aides reçues, sur la page des joueurs (application, rôle « joueur »,
// page libre).
func (s *Serveur) routesDocumentsJoueurs(m *http.ServeMux) {
	m.HandleFunc("GET /api/joueur/documents", s.documentsJoueur)
	m.HandleFunc("GET /api/joueur/documents/{id}", s.documentJoueur)
	m.HandleFunc("GET /api/joueur/documents/{id}/images/{fichier}", s.imageDocumentJoueur)
}

// ---------------------------------------------------------------- côté MJ

type envoiAide struct {
	Relation int64         `json:"relation"`
	PJ       store.Resume  `json:"pj"`
	Seance   *store.Resume `json:"seance,omitempty"`
	Date     float64       `json:"date"`
}

type aideMJ struct {
	Fiche    store.Resume  `json:"fiche"`
	IndiceDe *store.Resume `json:"indice_de,omitempty"`
	Prevue   int64         `json:"prevue,omitempty"`
	Envois   []envoiAide   `json:"envois"`
}

// listerAides : les aides de jeu (non archivées), d'une campagne et celles sans campagne avec
// ?campagne=, et à qui chacune a été envoyée.
func (s *Serveur) listerAides(w http.ResponseWriter, r *http.Request) {
	campagne := entierParam(r, "campagne")
	tout, err := s.store.Toutes()
	if err != nil {
		erreurStore(w, err)
		return
	}
	envois, err := s.store.Envois()
	if err != nil {
		erreurStore(w, err)
		return
	}
	parID := map[int64]store.Entite{}
	for _, e := range tout {
		parID[e.ID] = e
	}
	resume := func(id int64) *store.Resume {
		e, ok := parID[id]
		if !ok || id == 0 {
			return nil
		}
		x := s.store.ResumeDe(e)
		return &x
	}
	parAide := map[int64][]envoiAide{}
	for _, v := range envois {
		pj := resume(v.PJ)
		if pj == nil {
			continue
		}
		parAide[v.Aide] = append(parAide[v.Aide], envoiAide{Relation: v.Relation, PJ: *pj, Seance: resume(v.Seance), Date: v.Date})
	}
	out := []aideMJ{}
	for _, e := range tout {
		if e.Type != "aide" || e.Archive {
			continue
		}
		if c := idsDe(e.Data["campagne"]); campagne > 0 && len(c) > 0 && c[0] != campagne {
			continue
		}
		a := aideMJ{Fiche: s.store.ResumeDe(e), Envois: parAide[e.ID]}
		if a.Envois == nil {
			a.Envois = []envoiAide{}
		}
		if sec := idsDe(e.Data["indice_de"]); len(sec) > 0 {
			a.IndiceDe = resume(sec[0])
		}
		if p := idsDe(e.Data["prevue"]); len(p) > 0 {
			a.Prevue = p[0]
		}
		out = append(out, a)
	}
	sort.SliceStable(out, func(i, j int) bool { return store.Plier(out[i].Fiche.Nom) < store.Plier(out[j].Fiche.Nom) })
	repondre(w, 200, out)
}

// envoyerAide : {"qui": [PJ…]} ou {"tous": true} (les PJ de la campagne de l'aide, ou tous sans
// campagne, ni morts ni partis), "seance" facultative. Un PJ qui l'a déjà reçue est passé. Un
// secret « caché » dont l'aide est un indice devient « soupçonné » ; la révélation reste un geste
// du MJ.
func (s *Serveur) envoyerAide(w http.ResponseWriter, r *http.Request) {
	id, err := idDe(r)
	if err != nil {
		erreurStore(w, err)
		return
	}
	var in struct {
		Qui    []int64 `json:"qui"`
		Tous   bool    `json:"tous"`
		Seance int64   `json:"seance"`
	}
	if err := lireJSON(r, &in); err != nil {
		erreurStore(w, err)
		return
	}
	aide, err := s.store.Lire(id)
	if err != nil {
		erreurStore(w, err)
		return
	}
	if aide.Type != "aide" {
		erreurStore(w, store.ErrValidation{Msg: "Cette fiche n'est pas une aide de jeu."})
		return
	}
	if aide.Archive {
		erreurStore(w, store.ErrValidation{Msg: "Cette aide de jeu est archivée : restaurez-la avant de l'envoyer."})
		return
	}
	if in.Seance != 0 {
		if t, ok := s.store.TypeDeFiche(in.Seance); !ok || t != "seance" {
			erreurStore(w, store.ErrValidation{Msg: "La séance indiquée n'existe pas."})
			return
		}
	}
	qui := in.Qui
	if in.Tous {
		filtre := store.Filtre{Type: "pj"}
		if c := idsDe(aide.Data["campagne"]); len(c) > 0 {
			filtre.Campagne = c[0]
		}
		pjs, err := s.store.Lister(filtre)
		if err != nil {
			erreurStore(w, err)
			return
		}
		qui = nil
		for _, p := range pjs {
			if e, err := s.store.Lire(p.ID); err == nil && !absent(e.Data["statut"]) {
				qui = append(qui, p.ID)
			}
		}
	}
	if len(qui) == 0 {
		erreurStore(w, store.ErrValidation{Msg: "Choisissez à qui envoyer l'aide de jeu."})
		return
	}
	for _, p := range qui {
		e, err := s.store.Lire(p)
		if err != nil || e.Type != "pj" || e.Archive {
			erreurStore(w, store.ErrValidation{Msg: "On n'envoie une aide de jeu qu'à des PJ."})
			return
		}
	}
	deja := map[int64]bool{}
	envois, err := s.store.Envois()
	if err != nil {
		erreurStore(w, err)
		return
	}
	for _, v := range envois {
		if v.Aide == id {
			deja[v.PJ] = true
		}
	}
	d := store.DetailRelation{}
	if in.Seance != 0 {
		d.Contexte = &in.Seance
	}
	var recus []int64
	for _, p := range qui {
		if deja[p] {
			continue
		}
		if _, err := s.store.Relier(p, id, relRecu, d, auteurDe(r)); err != nil {
			erreurStore(w, err)
			return
		}
		deja[p] = true
		recus = append(recus, p)
	}
	var statut any
	if sec := idsDe(aide.Data["indice_de"]); len(sec) > 0 && len(recus) > 0 {
		if e, err := s.store.Lire(sec[0]); err == nil && e.Type == "secret" {
			if v, _ := e.Data["statut"].(string); v == "" || v == "caché" {
				if _, err := s.store.Modifier(e.ID, store.Maj{Data: map[string]any{"statut": "soupçonné"}}, auteurDe(r)); err != nil {
					erreurStore(w, err)
					return
				}
				statut = "soupçonné"
			}
		}
	}
	if len(recus) > 0 {
		s.joueurs.document(recus, aide)
	}
	if recus == nil {
		recus = []int64{}
	}
	repondre(w, 200, map[string]any{"envoyes": recus, "statut_secret": statut})
}

// indicesRecus : pour « qui sait quoi », les aides envoyées qui sont l'indice d'un secret.
type indiceRecu struct {
	Relation int64        `json:"relation"`
	Secret   int64        `json:"secret"`
	Qui      int64        `json:"qui"`
	Aide     store.Resume `json:"aide"`
	Seance   int64        `json:"seance,omitempty"`
}

func (s *Serveur) indicesRecus() ([]indiceRecu, error) {
	envois, err := s.store.Envois()
	if err != nil {
		return nil, err
	}
	out := []indiceRecu{}
	aides := map[int64]*store.Entite{}
	for _, v := range envois {
		a, vu := aides[v.Aide]
		if !vu {
			if e, err := s.store.Lire(v.Aide); err == nil && !e.Archive {
				a = &e
			}
			aides[v.Aide] = a
		}
		if a == nil {
			continue
		}
		if sec := idsDe(a.Data["indice_de"]); len(sec) > 0 {
			out = append(out, indiceRecu{Relation: v.Relation, Secret: sec[0], Qui: v.PJ, Aide: s.store.ResumeDe(*a), Seance: v.Seance})
		}
	}
	return out, nil
}

// ---------------------------------------------------------------- côté joueurs

// pjDocuments : le PJ dont on lit les documents. Sur un serveur, celui du compte « joueur » (0 :
// aucun) ; sinon (MJ, page libre à la maison) celui de ?pj=.
func (s *Serveur) pjDocuments(w http.ResponseWriter, r *http.Request) (int64, bool) {
	if pj, restreint := s.pjDuCompte(r); restreint {
		return pj, true
	}
	pj := entierParam(r, "pj")
	if e, err := s.store.Lire(pj); pj == 0 || err != nil || e.Type != "pj" || e.Archive {
		erreurStore(w, store.ErrIntrouvable)
		return 0, false
	}
	return pj, true
}

// aideRecue : l'aide de jeu n°id, si elle a été envoyée à ce PJ et n'est pas archivée.
func (s *Serveur) aideRecue(pj, id int64) (store.Entite, float64, bool) {
	if pj == 0 {
		return store.Entite{}, 0, false
	}
	envois, err := s.store.Envois()
	if err != nil {
		return store.Entite{}, 0, false
	}
	for _, v := range envois {
		if v.PJ == pj && v.Aide == id {
			e, err := s.store.Lire(id)
			if err != nil || e.Type != "aide" || e.Archive {
				return store.Entite{}, 0, false
			}
			return e, v.Date, true
		}
	}
	return store.Entite{}, 0, false
}

// documentResume : une aide dans la liste des documents d'un joueur.
type documentResume struct {
	ID    int64   `json:"id"`
	Nom   string  `json:"nom"`
	Forme string  `json:"forme,omitempty"`
	Image string  `json:"image,omitempty"` // image principale (fichier)
	Recu  float64 `json:"recu"`
}

// documentComplet : une aide telle que la lit un joueur. Rien d'autre de la fiche.
type documentComplet struct {
	documentResume
	Contenu string          `json:"contenu"`
	Images  []documentImage `json:"images"`
}

type documentImage struct {
	Fichier string `json:"fichier"`
	Legende string `json:"legende,omitempty"`
}

func formeDe(e store.Entite) string {
	if t, ok := modele.TypeDe(e.Type); ok {
		if c, ok := t.Champ("forme"); ok && champJoueur(*c) {
			v, _ := e.Data["forme"].(string)
			return v
		}
	}
	return ""
}

// documentsJoueur : les aides de jeu reçues par le PJ, de la plus récente à la plus ancienne.
func (s *Serveur) documentsJoueur(w http.ResponseWriter, r *http.Request) {
	pj, ok := s.pjDocuments(w, r)
	if !ok {
		return
	}
	out := []documentResume{}
	envois, err := s.store.Envois()
	if err != nil {
		erreurStore(w, err)
		return
	}
	for _, v := range envois {
		if pj == 0 || v.PJ != pj {
			continue
		}
		e, err := s.store.Lire(v.Aide)
		if err != nil || e.Type != "aide" || e.Archive {
			continue
		}
		out = append(out, documentResume{ID: e.ID, Nom: e.Nom, Forme: formeDe(e), Image: e.Image, Recu: v.Date})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Recu > out[j].Recu })
	repondre(w, 200, out)
}

// reLien : [[cible#ancre|texte]] et ![[image]] dans un texte Markdown.
var reLien = regexp.MustCompile(`!?\[\[([^\]\n]*?)\]\]`)

// sansLiens remplace les liens vers les fiches du monde par leur texte : un joueur n'y a pas accès,
// et l'identifiant d'une fiche cachée ne doit pas lui parvenir.
func sansLiens(texte string) string {
	return reLien.ReplaceAllStringFunc(texte, func(m string) string {
		if strings.HasPrefix(m, "!") {
			return ""
		}
		in := strings.TrimSuffix(strings.TrimPrefix(m, "[["), "]]")
		in = strings.ReplaceAll(in, `\|`, "|")
		if i := strings.Index(in, "|"); i >= 0 {
			return strings.TrimSpace(in[i+1:])
		}
		if i := strings.Index(in, "#"); i >= 0 {
			in = in[:i]
		}
		return strings.TrimSpace(in)
	})
}

// documentJoueur : une aide reçue, réduite à ce que les joueurs lisent.
func (s *Serveur) documentJoueur(w http.ResponseWriter, r *http.Request) {
	pj, ok := s.pjDocuments(w, r)
	if !ok {
		return
	}
	id, _ := idDe(r)
	e, recu, ok := s.aideRecue(pj, id)
	if !ok { // pas reçue, archivée ou inexistante : la même réponse
		erreurStore(w, store.ErrIntrouvable)
		return
	}
	d := documentComplet{documentResume: documentResume{ID: e.ID, Nom: e.Nom, Forme: formeDe(e), Image: e.Image, Recu: recu}, Images: []documentImage{}}
	if c, ok := modele.TypeDe("aide"); ok {
		if ch, ok := c.Champ("contenu"); ok && champJoueur(*ch) {
			v, _ := e.Data["contenu"].(string)
			d.Contenu = sansLiens(v)
		}
	}
	medias, err := s.store.Medias(e.ID)
	if err != nil {
		erreurStore(w, err)
		return
	}
	for _, m := range medias {
		d.Images = append(d.Images, documentImage{Fichier: m.Fichier, Legende: m.Legende})
	}
	repondre(w, 200, d)
}

// imageDocumentJoueur : une image d'une aide reçue par ce PJ, et d'aucune autre fiche.
func (s *Serveur) imageDocumentJoueur(w http.ResponseWriter, r *http.Request) {
	pj, ok := s.pjDocuments(w, r)
	if !ok {
		return
	}
	id, _ := idDe(r)
	f := r.PathValue("fichier")
	if _, _, ok := s.aideRecue(pj, id); !ok || !store.NomMediaValide(f) {
		http.NotFound(w, r)
		return
	}
	if de, err := s.store.EntiteDuMedia(f); err != nil || de != id {
		http.NotFound(w, r)
		return
	}
	contenu, cree, err := s.store.ContenuMedia(f)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	// pas de cache partagé : l'envoi peut être retiré
	w.Header().Set("Cache-Control", "private, no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, f, time.Unix(int64(cree), 0), bytes.NewReader(contenu))
}

// ---------------------------------------------------------------- en direct

// document prévient les appareils de ces PJ qu'une aide vient d'arriver (nom seulement : la page la
// demande ensuite par sa route). Un compte « joueur » n'apprend que ce qui arrive à son PJ.
func (d *diffusion) document(pjs []int64, aide store.Entite) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for e, filtre := range d.ecrans {
		pour := pjs
		if filtre != sansRestriction {
			pour = nil
			for _, p := range pjs {
				if p == filtre {
					pour = []int64{p}
				}
			}
			if pour == nil {
				continue
			}
		}
		msg, _ := json.Marshal(map[string]any{"type": "document", "id": aide.ID, "nom": aide.Nom, "pj": pour})
		e.envoyer(msg)
	}
}

// documents prévient que des envois ont été retirés : chaque page relit sa liste.
func (d *diffusion) documents() { d.envoyer(map[string]any{"type": "documents"}) }
