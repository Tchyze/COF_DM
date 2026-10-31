// Package serveur expose l'API de COF DM (voir docs/API.md) et sert l'interface web.
package serveur

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Tchyze/cof_dm/internal/comptes"
	"github.com/Tchyze/cof_dm/internal/modele"
	"github.com/Tchyze/cof_dm/internal/store"
)

// Reglages garde ce qui tient à cet ordinateur et non à un univers : mot de passe MJ, jeton d'API,
// univers ouvert.
type Reglages interface {
	Meta(cle string) (string, bool)
	SetMeta(cle, valeur string) error
}

type Options struct {
	Version     string
	DossierDon  string   // dossier de l'application : réglages, univers par défaut, sauvegardes
	Reglages    Reglages // nil : les réglages sont rangés dans l'univers
	Sauvegardes string   // dossier des sauvegardes de l'univers ; vide : DossierDon/sauvegardes/<id>
	Frontend    fs.FS
	Adresse     string           // adresse d'écoute, affichée dans les paramètres
	Port        int              // port d'écoute
	Reseau      bool             // l'application écoute sur le réseau local
	ReseauForce bool             // … parce que -reseau l'impose
	ReseauVoulu bool             // réglage enregistré (appliqué au lancement)
	Config      func(bool) error // enregistre le réglage ; nil : non modifiable
	// Relie : l'application de bureau est reliée à un serveur ; elle ne répond alors plus au réseau
	// local (ni tablette, ni page des joueurs), même si elle y écoute. nil : jamais reliée.
	Relie func() bool
	// Relancer : après une mise à jour, l'application ferme l'univers et relance ce programme
	// (application de bureau seulement ; nil : pas de relance automatique).
	Relancer func(programme string)
	// Comptes : mode serveur. L'accès passe par un compte (nom, mot de passe) ou son jeton d'API, et
	// plus rien n'est accordé d'office aux requêtes de cet ordinateur (un reverse proxy en vient).
	Comptes *comptes.Registre
	acces   *acces // partagé par l'Application et les Serveurs des univers ouverts
	// Dialogue ouvre la boîte « Ouvrir » ou « Enregistrer » du système et renvoie le fichier choisi
	// ("" : annulé) ; nil : le chemin se saisit dans l'interface.
	Dialogue func(enregistrer bool, nom, dossier string) (string, error)
	// Programme : ce programme (l'exe, l'AppImage), et ArgsMCP les arguments à ajouter à -mcp, pour
	// l'inscrire dans Claude Desktop ou Claude Code (claude.go). Vide : pas d'inscription.
	Programme string
	ArgsMCP   []string
	// VeilleMaj : mode serveur, chercher les versions publiées (au lancement puis toutes les six
	// heures) pour le signaler à l'administrateur ; le serveur ne s'installe rien (docker compose).
	VeilleMaj bool
}

type Serveur struct {
	opt         Options
	store       *store.Store
	reg         Reglages
	mux         *http.ServeMux
	acces       *acces
	joueur      *http.ServeMux // mode serveur : les seules routes ouvertes au rôle « joueur »
	salles      *salles        // combats suivis ou joués
	joueurs     *diffusion     // appareils qui suivent la page des joueurs
	libre       *http.ServeMux // à la maison : les seules routes de la page des joueurs, sans mot de passe
	jets        *tableJets     // lanceur de dés : les derniers jets de la partie
	potMu       sync.Mutex     // les écritures du pot commun, l'une après l'autre (butin.go)
	reseauVoulu atomic.Bool
}

func Nouveau(st *store.Store, opt Options) *Serveur {
	if opt.Reglages == nil {
		opt.Reglages = st
	}
	if opt.Sauvegardes == "" {
		opt.Sauvegardes = filepath.Join(opt.DossierDon, "sauvegardes", st.ID())
	}
	s := &Serveur{opt: opt, store: st, reg: opt.Reglages, mux: http.NewServeMux()}
	if _, ok := s.reg.Meta("api_jeton"); !ok && opt.Comptes == nil {
		_ = s.reg.SetMeta("api_jeton", nouveauJeton())
	}
	if s.acces = opt.acces; s.acces == nil {
		s.acces = nouvelAcces(s.reg, opt.Comptes)
	}
	s.reseauVoulu.Store(opt.ReseauVoulu)
	s.routes()
	return s
}

// relie : l'application de bureau est reliée à un serveur (voir distant.go).
func (s *Serveur) relie() bool { return s.opt.Relie != nil && s.opt.Relie() }

func nouveauJeton() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ---------------------------------------------------------------- accès

type cleContexte struct{}

// auteur de la requête, pour le journal : « mj » (navigateur local) ou le nom donné par le client API.
func auteurDe(r *http.Request) string {
	if a, ok := r.Context().Value(cleContexte{}).(string); ok {
		return a
	}
	return "mj"
}

// ServeHTTP contrôle l'accès avant de router :
//   - avec « Authorization: Bearer <jeton> » (clients de l'API, Claude) : accepté, d'où qu'il vienne ;
//   - depuis cet ordinateur, avec un nom d'hôte local (contre le « DNS rebinding ») : accepté ;
//   - depuis le réseau, avec la session ouverte par le mot de passe MJ : accepté (voir acces.go) ;
//   - sinon, seuls l'interface et la connexion répondent.
//
// Sans jeton, les écritures doivent porter l'en-tête X-COF-DM qu'une page d'un autre site ne peut
// pas envoyer.
func (s *Serveur) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	enTetes(w)
	if s.acces.comptes != nil {
		s.servirComptes(w, r)
		return
	}
	if s.relie() && !boucleLocale(r) {
		erreur(w, 403, msgReseauRelie)
		return
	}
	if j := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "); j != r.Header.Get("Authorization") {
		attendu, _ := s.reg.Meta("api_jeton")
		if attendu == "" || subtle.ConstantTimeCompare([]byte(j), []byte(attendu)) != 1 {
			erreur(w, 401, "Jeton d'API invalide.")
			return
		}
		auteur := strings.TrimSpace(r.Header.Get("X-COF-Auteur"))
		if auteur == "" {
			auteur = "api"
		}
		if len(auteur) > 40 {
			auteur = auteur[:40]
		}
		s.servir(s.mux, w, r.WithContext(contexte(r, auteur)))
		return
	}
	switch {
	case local(r):
	case !s.opt.Reseau || !hoteDuReseau(r):
		erreur(w, 401, "Accès réservé : utilisez l'application sur l'ordinateur du MJ, ou un jeton d'API.")
		return
	case s.acces.session(r):
		r = avecSession(r)
	case s.pageJoueursOuverte() && routeLibre(r):
		// la page des joueurs, ouverte par le MJ : ses seules routes, sans mot de passe
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Header.Get("X-COF-DM") != "1" {
			erreur(w, 403, "Requête refusée.")
			return
		}
		s.servir(s.libre, w, r.WithContext(contexte(sansSession(r), "joueur")))
		return
	case ouvert(r):
		r = sansSession(r)
	default:
		erreur(w, 401, "Accès réservé au MJ : connectez-vous avec le mot de passe MJ, ou utilisez un jeton d'API.")
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Header.Get("X-COF-DM") != "1" {
		erreur(w, 403, "Requête refusée.")
		return
	}
	s.servir(s.mux, w, r)
}

// enTetes : ce que toute réponse dit au navigateur. L'application ne s'affiche dans le cadre
// d'aucun autre site (une page hostile y piégerait les clics du MJ) et ses réponses ne sont pas
// réinterprétées.
func enTetes(w http.ResponseWriter) {
	h := w.Header()
	h.Set("X-Frame-Options", "SAMEORIGIN")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
}

func contexte(r *http.Request, auteur string) context.Context {
	return context.WithValue(r.Context(), cleContexte{}, auteur)
}

func local(r *http.Request) bool {
	hote := r.Host
	if h, _, err := net.SplitHostPort(hote); err == nil {
		hote = h
	}
	if hote != "localhost" && hote != "127.0.0.1" && hote != "::1" && hote != "[::1]" {
		return false
	}
	ip := r.RemoteAddr
	if h, _, err := net.SplitHostPort(ip); err == nil {
		ip = h
	}
	p := net.ParseIP(ip)
	return p == nil || p.IsLoopback() // nil : requête de test sans adresse
}

func (s *Serveur) routes() {
	m := s.mux
	m.HandleFunc("GET /api/sante", s.sante)
	m.HandleFunc("GET /api/schema", s.schema)
	m.HandleFunc("GET /api/stats", s.stats)

	m.HandleFunc("GET /api/entites", s.lister)
	m.HandleFunc("POST /api/entites", s.creer)
	m.HandleFunc("GET /api/entites/{id}", s.lire)
	m.HandleFunc("PATCH /api/entites/{id}", s.modifier)
	m.HandleFunc("PUT /api/entites/{id}", s.remplacer)
	m.HandleFunc("DELETE /api/entites/{id}", s.supprimer)
	m.HandleFunc("POST /api/entites/{id}/archive", s.archiver)
	m.HandleFunc("POST /api/entites/{id}/partage", s.partager)
	m.HandleFunc("POST /api/entites/{id}/medias", s.ajouterMedia)
	m.HandleFunc("GET /api/entites/{id}/organigramme", s.organigramme)
	m.HandleFunc("GET /api/secrets", s.secrets)
	m.HandleFunc("POST /api/secrets/{id}/reveler", s.reveler)
	m.HandleFunc("POST /api/import/fiches", s.importerFiches)
	s.routesBestiaire()
	s.routesRencontres()
	s.routesButin()
	s.routesCombats()
	s.routesFeuille()
	s.routesTable()
	s.routesPreparation()
	s.routesApres()
	s.routesAides()
	s.routesFonctionnalites()
	s.routesClaude()
	s.routesAcces()
	s.routesDes()
	s.routesPageJoueurs() // après routesAcces, qui crée la liste blanche du rôle « joueur »

	m.HandleFunc("POST /api/relations", s.relier)
	m.HandleFunc("PATCH /api/relations/{id}", s.modifierRelation)
	m.HandleFunc("DELETE /api/relations/{id}", s.delier)

	m.HandleFunc("PATCH /api/medias/{id}", s.modifierMedia)
	m.HandleFunc("DELETE /api/medias/{id}", s.supprimerMedia)
	m.HandleFunc("GET /medias/{fichier}", s.servirMedia)

	m.HandleFunc("GET /api/resoudre", s.resoudre)
	m.HandleFunc("GET /api/liens-morts", s.liensMorts)
	m.HandleFunc("GET /api/graphe", s.graphe)
	m.HandleFunc("GET /api/chronologie", s.chronologie)
	m.HandleFunc("GET /api/calendrier", s.lireCalendrier)
	m.HandleFunc("PUT /api/calendrier", s.ecrireCalendrier)
	m.HandleFunc("GET /api/journal", s.journal)

	m.HandleFunc("GET /api/export", s.exporter)
	m.HandleFunc("GET /api/export/obsidian", s.exporterObsidian)
	m.HandleFunc("POST /api/restaurer", s.restaurer)
	m.HandleFunc("POST /api/sauvegarde", s.sauvegarder)
	s.routesSync(m)
	m.HandleFunc("GET /api/parametres", s.parametres)
	m.HandleFunc("POST /api/parametres/jeton", s.nouveauJeton)

	statique := http.FileServer(http.FS(s.opt.Frontend))
	m.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data: blob:; style-src 'self' 'unsafe-inline'; object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'self'"+socketsDeLaPage(r))
		statique.ServeHTTP(w, r)
	}))
}

// ---------------------------------------------------------------- utilitaires HTTP

func repondre(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func erreur(w http.ResponseWriter, code int, msg string) {
	repondre(w, code, map[string]string{"detail": msg})
}

// msgInterne : la réponse à une erreur imprévue, dont le détail ne sort pas du journal.
const msgInterne = "Erreur interne (détails dans le journal de l'application)."

func erreurStore(w http.ResponseWriter, err error) {
	var ev store.ErrValidation
	switch {
	case errors.As(err, &ev):
		erreur(w, 422, ev.Error())
	case errors.Is(err, store.ErrIntrouvable):
		erreur(w, 404, "Fiche introuvable.")
	case errors.Is(err, store.ErrConflit):
		erreur(w, 409, "Cette fiche a été modifiée ailleurs depuis que vous l'avez ouverte. Rechargez-la avant d'enregistrer.")
	default:
		// le détail (chemins de fichiers, requêtes) reste dans le journal : la réponse peut partir
		// sur le réseau, vers un joueur
		log.Printf("erreur : %v", err)
		erreur(w, 500, msgInterne)
	}
}

// tailleMaxJSON : une fiche, même longue, pèse quelques dizaines de kilo-octets.
const tailleMaxJSON = 4 << 20

func lireJSON(r *http.Request, v any) error {
	b, err := io.ReadAll(io.LimitReader(r.Body, tailleMaxJSON+1))
	if err != nil {
		return err
	}
	if len(b) > tailleMaxJSON {
		return store.ErrValidation{Msg: "Requête trop lourde (4 Mo au plus)."}
	}
	d := json.NewDecoder(strings.NewReader(string(b)))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return store.ErrValidation{Msg: "Requête illisible : " + err.Error()}
	}
	return nil
}

func idDe(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return 0, store.ErrIntrouvable
	}
	return id, nil
}

func entierParam(r *http.Request, nom string) int64 {
	n, _ := strconv.ParseInt(r.URL.Query().Get(nom), 10, 64)
	return n
}

// ---------------------------------------------------------------- schéma, statistiques

func (s *Serveur) schema(w http.ResponseWriter, r *http.Request) {
	etat := s.store.Fonctionnalites()
	repondre(w, 200, map[string]any{
		"types": typesActifs(etat, s.typesDeLUnivers()), "relations": modele.Relations, "calendrier": s.store.Calendrier(),
		"fonctionnalites": etat,
		"genres": map[string]string{
			modele.Texte: "chaîne d'une ligne", modele.Long: "Markdown ; [[Nom]] crée un lien vers la fiche de ce nom",
			modele.Choix: "une des options", modele.Nombre: "nombre", modele.Booleen: "true / false",
			modele.Ref: "identifiant (id) d'une fiche d'un des types « cibles »", modele.Refs: "liste d'identifiants",
			modele.DateMonde: "{\"an\":1216,\"mois\":1,\"jour\":1} selon le calendrier", modele.DateReelle: "\"AAAA-MM-JJ\"",
			modele.Etapes: "[{\"texte\":\"…\",\"fait\":false}]",
			modele.Butin:  "{\"pieces\":{\"po\":30,\"pa\":0,\"pc\":0},\"objets\":[{\"fiche\":12},{\"nom\":\"Torche\",\"quantite\":4}]}",
		},
	})
}

func (s *Serveur) stats(w http.ResponseWriter, r *http.Request) {
	compte, total, err := s.store.Compter()
	if err != nil {
		erreurStore(w, err)
		return
	}
	morts, _ := s.store.LiensMorts()
	journal, _ := s.store.Journal(12)
	campagnes, _ := s.store.Lister(store.Filtre{Type: "campagne"})
	seances, _ := s.store.Lister(store.Filtre{Type: "seance", Limite: 5})
	quetes, _ := s.store.Lister(store.Filtre{Type: "quete"})
	enCours := []store.Resume{}
	for _, q := range quetes {
		if q.Carte["statut"] == "en cours" {
			enCours = append(enCours, q)
		}
	}
	repondre(w, 200, map[string]any{
		"compte": compte, "total": total, "liens_morts": len(morts), "journal": journal,
		"campagnes": campagnes, "seances": seances, "quetes_en_cours": enCours,
	})
}

// ---------------------------------------------------------------- fiches

func (s *Serveur) lister(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	lim, _ := strconv.Atoi(q.Get("limite"))
	l, err := s.store.Lister(store.Filtre{
		Type: q.Get("type"), Q: q.Get("q"), Tag: q.Get("tag"), Archives: q.Get("archives") == "1",
		Campagne: entierParam(r, "campagne"), Limite: lim,
	})
	if err != nil {
		erreurStore(w, err)
		return
	}
	// les fiches d'une fonctionnalité coupée sortent des listes (elles restent lisibles par leur lien)
	etat, gardes := s.store.Fonctionnalites(), l[:0]
	for _, x := range l {
		if typeActif(etat, x.Type) {
			gardes = append(gardes, x)
		}
	}
	repondre(w, 200, gardes)
}

// ficheComplete : la fiche et tout ce qui l'entoure.
type ficheComplete struct {
	store.Entite
	Relations  []store.Relation              `json:"relations"`
	References []store.Reference             `json:"references"`
	Mentions   map[string]store.CibleMention `json:"mentions"`
	RefsNoms   map[string]store.Resume       `json:"refs"` // fiches citées par les champs Ref / Refs
	Medias     []store.Media                 `json:"medias"`
	Dates      map[string]string             `json:"dates"` // champs date en jeu, mis en texte
}

func (s *Serveur) complete(id int64) (ficheComplete, error) {
	e, err := s.store.Lire(id)
	if err != nil {
		return ficheComplete{}, err
	}
	c := ficheComplete{Entite: e, RefsNoms: map[string]store.Resume{}, Dates: map[string]string{}}
	if c.Relations, err = s.store.Relations(id); err != nil {
		return c, err
	}
	if c.References, err = s.store.References(id); err != nil {
		return c, err
	}
	if c.Mentions, err = s.store.MentionsDe(e); err != nil {
		return c, err
	}
	if c.Medias, err = s.store.Medias(id); err != nil {
		return c, err
	}
	t, _ := modele.TypeDe(e.Type)
	cal := s.store.Calendrier()
	for _, ch := range t.Champs {
		v, ok := e.Data[ch.Cle]
		if !ok {
			continue
		}
		switch ch.Genre {
		case modele.Ref, modele.Refs:
			for _, x := range idsDe(v) {
				if o, err := s.store.Lire(x); err == nil {
					c.RefsNoms[strconv.FormatInt(x, 10)] = store.Resume{ID: o.ID, Type: o.Type, Nom: o.Nom, Image: o.Image, Archive: o.Archive}
				}
			}
		case modele.Butin: // les objets du monde d'un butin, pour les montrer sous leur nom du moment
			for _, o := range modele.LireButin(v).Objets {
				if o.Fiche == 0 {
					continue
				}
				if x, err := s.store.Lire(o.Fiche); err == nil {
					c.RefsNoms[strconv.FormatInt(x.ID, 10)] = store.Resume{ID: x.ID, Type: x.Type, Nom: x.Nom, Image: x.Image, Archive: x.Archive}
				}
			}
		case modele.DateMonde:
			if m, ok := v.(map[string]any); ok {
				d := modele.DateJeu{An: int(num(m["an"])), Mois: int(num(m["mois"])), Jour: int(num(m["jour"]))}
				c.Dates[ch.Cle] = cal.Texte(d)
			}
		}
	}
	return c, nil
}

func num(v any) float64 { f, _ := v.(float64); return f }

func idsDe(v any) []int64 {
	switch x := v.(type) {
	case float64:
		return []int64{int64(x)}
	case []any:
		var out []int64
		for _, y := range x {
			out = append(out, idsDe(y)...)
		}
		return out
	}
	return nil
}

func (s *Serveur) lire(w http.ResponseWriter, r *http.Request) {
	id, err := idDe(r)
	if err == nil {
		var c ficheComplete
		if c, err = s.complete(id); err == nil {
			repondre(w, 200, c)
			return
		}
	}
	erreurStore(w, err)
}

func (s *Serveur) repondreFiche(w http.ResponseWriter, code int, id int64) {
	c, err := s.complete(id)
	if err != nil {
		erreurStore(w, err)
		return
	}
	repondre(w, code, c)
}

func (s *Serveur) creer(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Type string         `json:"type"`
		Nom  string         `json:"nom"`
		Data map[string]any `json:"data"`
		Tags []string       `json:"tags"`
	}
	if err := lireJSON(r, &in); err != nil {
		erreurStore(w, err)
		return
	}
	if err := s.typeCoupe(in.Type); err != nil {
		erreurStore(w, err)
		return
	}
	if err := s.niveauPermis(in.Type, in.Data); err != nil {
		erreurStore(w, err)
		return
	}
	if in.Type == "objet" {
		if err := s.verifierChiffres(in.Data); err != nil {
			erreur(w, 422, err.Error())
			return
		}
	}
	e, err := s.store.Creer(in.Type, in.Nom, in.Data, in.Tags, auteurDe(r))
	if err != nil {
		erreurStore(w, err)
		return
	}
	s.suivreObjet(nil, e, auteurDe(r))
	s.repondreFiche(w, 201, e.ID)
}

func (s *Serveur) modifier(w http.ResponseWriter, r *http.Request) {
	s.maj(w, r, false)
}

func (s *Serveur) remplacer(w http.ResponseWriter, r *http.Request) {
	s.maj(w, r, true)
}

func (s *Serveur) maj(w http.ResponseWriter, r *http.Request, remplacer bool) {
	id, err := idDe(r)
	if err != nil {
		erreurStore(w, err)
		return
	}
	var m store.Maj
	if err := lireJSON(r, &m); err != nil {
		erreurStore(w, err)
		return
	}
	if remplacer {
		m.Remplacer = true
	}
	switch t, _ := s.store.TypeDeFiche(id); t {
	case "pj":
		if err := s.niveauPermis(t, m.Data); err != nil {
			erreurStore(w, err)
			return
		}
	case "objet":
		if err := s.verifierChiffres(m.Data); err != nil {
			erreur(w, 422, err.Error())
			return
		}
	}
	avant, _ := s.store.Lire(id)
	apres, err := s.store.Modifier(id, m, auteurDe(r))
	if err != nil {
		erreurStore(w, err)
		return
	}
	s.suivreObjet(&avant, apres, auteurDe(r))
	s.repondreFiche(w, 200, id)
}

// supprimer archive la fiche ; avec ?definitif=1, l'efface avec ses relations et ses images.
func (s *Serveur) supprimer(w http.ResponseWriter, r *http.Request) {
	id, err := idDe(r)
	if err != nil {
		erreurStore(w, err)
		return
	}
	if r.URL.Query().Get("definitif") != "1" {
		if _, err := s.store.Archiver(id, true, auteurDe(r)); err != nil {
			erreurStore(w, err)
			return
		}
		repondre(w, 200, map[string]any{"archive": true})
		return
	}
	avant, _ := s.store.Lire(id)
	if _, err := s.store.Supprimer(id, auteurDe(r)); err != nil {
		erreurStore(w, err)
		return
	}
	s.objetSupprime(avant, auteurDe(r))
	s.butinSupprime(avant, auteurDe(r))
	repondre(w, 200, map[string]any{"supprime": true})
}

func (s *Serveur) archiver(w http.ResponseWriter, r *http.Request) {
	id, err := idDe(r)
	if err != nil {
		erreurStore(w, err)
		return
	}
	var in struct {
		Archive bool `json:"archive"`
	}
	if err := lireJSON(r, &in); err != nil {
		erreurStore(w, err)
		return
	}
	if _, err := s.store.Archiver(id, in.Archive, auteurDe(r)); err != nil {
		erreurStore(w, err)
		return
	}
	s.repondreFiche(w, 200, id)
}

// ---------------------------------------------------------------- relations

func (s *Serveur) relier(w http.ResponseWriter, r *http.Request) {
	var in struct {
		A         int64   `json:"a"`
		B         int64   `json:"b"`
		Type      string  `json:"type"`
		Note      *string `json:"note"`
		Rang      *string `json:"rang"`
		Superieur *int64  `json:"superieur"`
		Secrete   *bool   `json:"secrete"`
		Contexte  *int64  `json:"contexte"`
	}
	if err := lireJSON(r, &in); err != nil {
		erreurStore(w, err)
		return
	}
	if t, _ := modele.RelationDe(in.Type); t.Reservee {
		erreurStore(w, store.ErrValidation{Msg: msgRelationReservee})
		return
	}
	id, err := s.store.Relier(in.A, in.B, in.Type, store.DetailRelation{Note: in.Note, Rang: in.Rang, Superieur: in.Superieur, Secrete: in.Secrete, Contexte: in.Contexte}, auteurDe(r))
	if err != nil {
		erreurStore(w, err)
		return
	}
	repondre(w, 201, map[string]any{"id": id})
}

func (s *Serveur) modifierRelation(w http.ResponseWriter, r *http.Request) {
	id, err := idDe(r)
	if err != nil {
		erreurStore(w, err)
		return
	}
	var d store.DetailRelation
	if err := lireJSON(r, &d); err != nil {
		erreurStore(w, err)
		return
	}
	if d.Type != nil {
		avant, err := s.store.TypeDeRelation(id)
		if err != nil {
			erreurStore(w, err)
			return
		}
		t1, _ := modele.RelationDe(avant)
		t2, _ := modele.RelationDe(*d.Type)
		if avant != *d.Type && (t1.Reservee || t2.Reservee) {
			erreurStore(w, store.ErrValidation{Msg: msgRelationReservee})
			return
		}
	}
	if err := s.store.ModifierRelation(id, d, auteurDe(r)); err != nil {
		erreurStore(w, err)
		return
	}
	repondre(w, 200, map[string]any{"id": id})
}

func (s *Serveur) organigramme(w http.ResponseWriter, r *http.Request) {
	id, err := idDe(r)
	if err != nil {
		erreurStore(w, err)
		return
	}
	o, err := s.store.Organigramme(id)
	if err != nil {
		erreurStore(w, err)
		return
	}
	repondre(w, 200, o)
}

// msgRelationReservee : une relation posée par l'application (envoi d'une aide de jeu).
const msgRelationReservee = "Cette relation se pose par l'application : une aide de jeu s'envoie depuis sa fiche ou l'écran du MJ."

func (s *Serveur) delier(w http.ResponseWriter, r *http.Request) {
	id, err := idDe(r)
	typ := ""
	if err == nil {
		typ, _ = s.store.TypeDeRelation(id)
		err = s.store.Delier(id, auteurDe(r))
	}
	if err != nil {
		erreurStore(w, err)
		return
	}
	if typ == relRecu {
		s.joueurs.documents() // envoi retiré : les pages des joueurs relisent leurs documents
	}
	repondre(w, 200, map[string]any{"supprime": true})
}

// ---------------------------------------------------------------- images

var extensions = map[string]string{"image/png": ".png", "image/jpeg": ".jpg", "image/webp": ".webp", "image/gif": ".gif"}

const tailleMaxImage = 20 << 20

// ajouterMedia accepte un formulaire multipart (champ « fichier ») ou le contenu brut de l'image
// (Content-Type image/…, nom dans ?nom=).
func (s *Serveur) ajouterMedia(w http.ResponseWriter, r *http.Request) {
	id, err := idDe(r)
	if err != nil {
		erreurStore(w, err)
		return
	}
	if _, err := s.store.Lire(id); err != nil {
		erreurStore(w, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, tailleMaxImage+1<<20)
	var data []byte
	nom := r.URL.Query().Get("nom")
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		f, h, err := r.FormFile("fichier")
		if err != nil {
			erreur(w, 422, "Envoyez l'image dans le champ « fichier ».")
			return
		}
		defer f.Close()
		data, err = io.ReadAll(f)
		if err != nil {
			erreur(w, 413, "Image trop lourde (20 Mo au plus).")
			return
		}
		nom = h.Filename
	} else if data, err = io.ReadAll(r.Body); err != nil {
		erreur(w, 413, "Image trop lourde (20 Mo au plus).")
		return
	}
	if len(data) == 0 || len(data) > tailleMaxImage {
		erreur(w, 422, "Image vide ou trop lourde (20 Mo au plus).")
		return
	}
	ext, ok := extensions[http.DetectContentType(data)]
	if !ok {
		erreur(w, 415, "Format non pris en charge : PNG, JPEG, WebP ou GIF.")
		return
	}
	m, err := s.store.AjouterMedia(id, nouveauJeton()[:20]+ext, filepath.Base(nom), data, auteurDe(r))
	if err != nil {
		erreurStore(w, err)
		return
	}
	repondre(w, 201, m)
}

func (s *Serveur) modifierMedia(w http.ResponseWriter, r *http.Request) {
	id, err := idDe(r)
	if err != nil {
		erreurStore(w, err)
		return
	}
	var in struct {
		Legende   *string `json:"legende"`
		Principal bool    `json:"principal"`
	}
	if err := lireJSON(r, &in); err != nil {
		erreurStore(w, err)
		return
	}
	if err := s.store.ModifierMedia(id, in.Legende, in.Principal); err != nil {
		erreurStore(w, err)
		return
	}
	repondre(w, 200, map[string]any{"id": id})
}

func (s *Serveur) supprimerMedia(w http.ResponseWriter, r *http.Request) {
	id, err := idDe(r)
	if err != nil {
		erreurStore(w, err)
		return
	}
	if err := s.store.SupprimerMedia(id); err != nil {
		erreurStore(w, err)
		return
	}
	repondre(w, 200, map[string]any{"supprime": true})
}

func (s *Serveur) servirMedia(w http.ResponseWriter, r *http.Request) {
	f := r.PathValue("fichier")
	if !store.NomMediaValide(f) {
		http.NotFound(w, r)
		return
	}
	contenu, cree, err := s.store.ContenuMedia(f)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "max-age=31536000, immutable")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, f, time.Unix(int64(cree), 0), bytes.NewReader(contenu))
}

// ---------------------------------------------------------------- liens, graphe, temps

// resoudre renvoie un aperçu de la fiche visée par [[Nom]] (survol, clic).
func (s *Serveur) resoudre(w http.ResponseWriter, r *http.Request) {
	nom := r.URL.Query().Get("nom")
	res, err := s.store.ResoudreNoms([]string{nom})
	if err != nil {
		erreurStore(w, err)
		return
	}
	c := res[strings.ToLower(strings.TrimSpace(nom))]
	if c.ID == 0 {
		erreur(w, 404, "Aucune fiche « "+nom+" ».")
		return
	}
	e, err := s.store.Lire(c.ID)
	if err != nil {
		erreurStore(w, err)
		return
	}
	repondre(w, 200, map[string]any{"id": e.ID, "nom": e.Nom, "type": e.Type, "image": e.Image, "resume": apercu(e)})
}

// apercu : la phrase de présentation d'une fiche.
func apercu(e store.Entite) string {
	for _, k := range []string{"resume", "impression", "but", "pitch", "contenu", "publique", "accroche", "description", "apparence"} {
		if v, ok := e.Data[k].(string); ok && strings.TrimSpace(v) != "" {
			v = strings.TrimSpace(strings.SplitN(v, "\n\n", 2)[0])
			if rs := []rune(v); len(rs) > 240 {
				v = string(rs[:240]) + "…"
			}
			return v
		}
	}
	return ""
}

func (s *Serveur) liensMorts(w http.ResponseWriter, r *http.Request) {
	l, err := s.store.LiensMorts()
	if err != nil {
		erreurStore(w, err)
		return
	}
	repondre(w, 200, l)
}

func (s *Serveur) graphe(w http.ResponseWriter, r *http.Request) {
	prof, _ := strconv.Atoi(r.URL.Query().Get("profondeur"))
	n, a, err := s.store.Graphe(entierParam(r, "id"), max(1, min(prof, 3)))
	if err != nil {
		erreurStore(w, err)
		return
	}
	repondre(w, 200, map[string]any{"noeuds": n, "aretes": a})
}

func (s *Serveur) chronologie(w http.ResponseWriter, r *http.Request) {
	l, err := s.store.Chronologie(entierParam(r, "campagne"))
	if err != nil {
		erreurStore(w, err)
		return
	}
	repondre(w, 200, l)
}

func (s *Serveur) lireCalendrier(w http.ResponseWriter, r *http.Request) {
	repondre(w, 200, s.store.Calendrier())
}

func (s *Serveur) ecrireCalendrier(w http.ResponseWriter, r *http.Request) {
	var c modele.Calendrier
	if err := lireJSON(r, &c); err != nil {
		erreurStore(w, err)
		return
	}
	if err := s.store.DefinirCalendrier(c); err != nil {
		erreurStore(w, err)
		return
	}
	s.store.Noter("calendrier", "calendrier modifié", auteurDe(r))
	repondre(w, 200, s.store.Calendrier())
}

func (s *Serveur) journal(w http.ResponseWriter, r *http.Request) {
	lim, _ := strconv.Atoi(r.URL.Query().Get("limite"))
	l, err := s.store.Journal(lim)
	if err != nil {
		erreurStore(w, err)
		return
	}
	repondre(w, 200, l)
}

// ---------------------------------------------------------------- données

func (s *Serveur) exporter(w http.ResponseWriter, r *http.Request) {
	x, err := s.store.Exporter()
	if err != nil {
		erreurStore(w, err)
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="cof-dm-%s.json"`, time.Now().Format("2006-01-02")))
	repondre(w, 200, x)
}

func (s *Serveur) sauvegarder(w http.ResponseWriter, r *http.Request) {
	f, err := sauvegarder(s.store, s.opt.Sauvegardes, "manuelle")
	if err != nil {
		erreurStore(w, err)
		return
	}
	s.store.Noter("sauvegarde", f, auteurDe(r))
	repondre(w, 200, map[string]any{"fichier": f})
}

func (s *Serveur) restaurer(w http.ResponseWriter, r *http.Request) {
	var x store.Export
	b, err := io.ReadAll(io.LimitReader(r.Body, 256<<20))
	if err == nil {
		err = json.Unmarshal(b, &x)
	}
	if err != nil {
		erreur(w, 422, "Fichier d'export illisible.")
		return
	}
	d, err := sauvegarder(s.store, s.opt.Sauvegardes, "avant-restauration")
	if err != nil {
		log.Printf("sauvegarde avant restauration : %v", err)
		erreur(w, 500, "Sauvegarde impossible, rien n'a été modifié. "+msgInterne)
		return
	}
	if err := s.store.Restaurer(x, auteurDe(r)); err != nil {
		erreurStore(w, err)
		return
	}
	s.Fermer() // les combats suivis viennent d'être remplacés
	repondre(w, 200, map[string]any{"fiches": len(x.Entites), "relations": len(x.Relations), "sauvegarde": d})
}

func (s *Serveur) parametres(w http.ResponseWriter, r *http.Request) {
	j, _ := s.reg.Meta("api_jeton")
	prive := !parSession(r) // le jeton d'API et les chemins ne se lisent que sur l'ordinateur du MJ
	if modeServeur(r) {     // … ou, sur un serveur, par l'administrateur ; chaque compte a ses jetons
		c, _ := compteDe(r)
		j, prive = "", c.Admin
	}
	if !prive {
		j = ""
	}
	univers := map[string]any{"nom": nomUnivers(s.store.Chemin()), "id": s.store.ID()}
	out := map[string]any{"jeton": j, "adresse": s.opt.Adresse, "version": s.opt.Version, "univers": univers}
	if prive {
		univers["chemin"] = s.store.Chemin()
		out["donnees"], out["sauvegardes"] = s.opt.DossierDon, s.opt.Sauvegardes
	}
	repondre(w, 200, out)
}

func (s *Serveur) nouveauJeton(w http.ResponseWriter, r *http.Request) {
	if modeServeur(r) {
		erreur(w, 404, "Sur un serveur, chaque compte a ses jetons : POST /api/compte/jetons.")
		return
	}
	if !surPlace(w, r) {
		return
	}
	j := nouveauJeton()
	if err := s.reg.SetMeta("api_jeton", j); err != nil {
		erreurStore(w, err)
		return
	}
	s.store.Noter("jeton", "nouveau jeton d'API (l'ancien ne marche plus)", auteurDe(r))
	repondre(w, 200, map[string]any{"jeton": j})
}
