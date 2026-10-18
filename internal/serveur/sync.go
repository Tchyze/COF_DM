package serveur

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/Tchyze/cof_dm/internal/store"
)

// Synchronisation hors ligne, côté serveur (voir internal/store/protocole.go). Réservée au mode
// serveur, et aux comptes qui écrivent dans l'univers (MJ, administrateur) : ces routes ne sont ni en
// lecture pour l'auditeur, ni dans la liste blanche du joueur.

// tailleMaxEchange : un échange porte les changements faits hors ligne, sans les images.
const tailleMaxEchange = 64 << 20

func (s *Serveur) routesSync(m *http.ServeMux) {
	m.HandleFunc("POST /api/sync/copie", s.syncCopie)
	m.HandleFunc("POST /api/sync", s.syncEchanger)
	m.HandleFunc("PUT /api/sync/images/{fichier}", s.syncImage)
	m.HandleFunc("GET /api/sync/copies", s.syncCopies)
	m.HandleFunc("GET /api/conflits", s.conflits)
	m.HandleFunc("POST /api/conflits/{id}", s.resoudreConflit)
}

func syncPermise(w http.ResponseWriter, r *http.Request) bool {
	if !modeServeur(r) {
		erreur(w, 404, "La synchronisation se fait avec un serveur COF DM.")
		return false
	}
	return true
}

// syncCopie : {"nom": "Portable de Marion"} inscrit une copie et renvoie le fichier de l'univers
// tel qu'elle le gardera (en-têtes X-COF-Replique : son numéro, X-COF-Univers : l'univers).
func (s *Serveur) syncCopie(w http.ResponseWriter, r *http.Request) {
	if !syncPermise(w, r) {
		return
	}
	var in struct {
		Nom string `json:"nom"`
	}
	if err := lireJSON(r, &in); err != nil {
		erreurStore(w, err)
		return
	}
	dir, err := os.MkdirTemp("", "cofdm-copie-")
	if err != nil {
		erreurStore(w, err)
		return
	}
	defer os.RemoveAll(dir)
	fichier := filepath.Join(dir, "copie"+store.Extension)
	c, _ := compteDe(r)
	n, err := s.store.Inscrire(in.Nom, c.Nom, fichier)
	if err != nil {
		erreurStore(w, err)
		return
	}
	f, err := os.Open(fichier)
	if err != nil {
		erreurStore(w, err)
		return
	}
	defer f.Close()
	s.store.Noter("synchronisation", "copie « "+in.Nom+" » inscrite (n°"+strconv.FormatInt(n, 10)+")", auteurDe(r))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("X-COF-Replique", strconv.FormatInt(n, 10))
	w.Header().Set("X-COF-Univers", s.store.ID())
	_, _ = f.WriteTo(w)
}

// syncEchanger : {"replique", "curseur", "changements"} → store.Reponse.
func (s *Serveur) syncEchanger(w http.ResponseWriter, r *http.Request) {
	if !syncPermise(w, r) {
		return
	}
	corps, err := lireCorps(r, tailleMaxEchange)
	if err != nil {
		erreurStore(w, err)
		return
	}
	var in struct {
		Replique    int64              `json:"replique"`
		Curseur     int64              `json:"curseur"`
		Changements []store.Changement `json:"changements"`
	}
	if err := json.Unmarshal(corps, &in); err != nil {
		erreur(w, 422, "Échange illisible.")
		return
	}
	if in.Replique <= 0 || !s.store.CopieConnue(in.Replique) {
		erreur(w, 409, "Cette copie n'est pas inscrite auprès de ce serveur : reliez-la de nouveau.")
		return
	}
	rep, err := s.store.Recevoir(in.Curseur, in.Changements)
	if err != nil {
		erreurStore(w, err)
		return
	}
	repondre(w, 200, rep)
}

// syncImage : le contenu d'une image que le serveur attend (corps brut).
func (s *Serveur) syncImage(w http.ResponseWriter, r *http.Request) {
	if !syncPermise(w, r) {
		return
	}
	f := r.PathValue("fichier")
	if !store.NomMediaValide(f) {
		erreur(w, 404, "Image inconnue.")
		return
	}
	data, err := lireCorps(r, tailleMaxImage)
	if err != nil {
		erreur(w, 413, "Image trop lourde (20 Mo au plus).")
		return
	}
	if ext, ok := extensions[http.DetectContentType(data)]; !ok || filepath.Ext(f) != ext {
		erreur(w, 415, "Le contenu ne correspond pas au format de l'image.")
		return
	}
	if err := s.store.ContenuAttendu(f, data); err != nil {
		if errors.Is(err, store.ErrIntrouvable) {
			erreur(w, 404, "Image inconnue.")
			return
		}
		erreurStore(w, err)
		return
	}
	w.WriteHeader(204)
}

// syncCopies : les copies inscrites de l'univers.
func (s *Serveur) syncCopies(w http.ResponseWriter, r *http.Request) {
	if !syncPermise(w, r) {
		return
	}
	l := s.store.Copies()
	if l == nil {
		l = []store.CopieInscrite{}
	}
	repondre(w, 200, map[string]any{"copies": l})
}

// conflits : la liste « Conflits » d'une copie synchronisée (vide ailleurs).
func (s *Serveur) conflits(w http.ResponseWriter, r *http.Request) {
	l, err := s.store.Conflits()
	if err != nil {
		erreurStore(w, err)
		return
	}
	repondre(w, 200, map[string]any{"conflits": l})
}

// resoudreConflit : {"garder": "local" | "serveur"}.
func (s *Serveur) resoudreConflit(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		erreur(w, 404, "Conflit introuvable.")
		return
	}
	var in struct {
		Garder string `json:"garder"`
	}
	if err := lireJSON(r, &in); err != nil {
		erreurStore(w, err)
		return
	}
	if in.Garder != "local" && in.Garder != "serveur" {
		erreur(w, 422, "Choisissez « local » ou « serveur ».")
		return
	}
	if err := s.store.ResoudreConflit(id, in.Garder == "local", auteurDe(r)); err != nil {
		erreurStore(w, err)
		return
	}
	s.conflits(w, r)
}
