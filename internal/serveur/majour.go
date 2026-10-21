package serveur

import (
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/Tchyze/cof_dm/internal/majour"
	"github.com/Tchyze/cof_dm/internal/store"
	"github.com/Tchyze/cof_dm/internal/synchro"
)

// Mises à jour de l'application de bureau (internal/majour) : elle cherche au lancement puis toutes
// les six heures, le signale, et n'installe que sur le clic du MJ. Le canal (stable ou test) tient à
// l'ordinateur : il est rangé dans les réglages. Routes du menu Fichier (/api/univers/…) : ordinateur
// du MJ seulement, et pas sur un serveur (l'image Docker se met à jour par docker compose).
//
// Un serveur cherche aussi les versions stables publiées, pour le dire à son administrateur
// (GET /api/univers/version), avec la version des postes reliés à l'univers ; il n'installe rien.

const (
	regCanal          = "maj_canal"
	intervalleMaj     = 6 * time.Hour
	premiereRecherche = 5 * time.Second
)

// Fonctions du paquet majour, remplaçables dans les tests.
var (
	chercherMaj  = majour.Chercher
	installerMaj = majour.Installer
)

type misesAJour struct {
	mu         sync.Mutex
	dispo      *majour.Disponible
	verifie    time.Time
	erreur     string
	installant bool
}

func (a *Application) routesMaj() {
	a.maj = &misesAJour{}
	a.mux.HandleFunc("GET /api/univers/mise-a-jour", a.routeEtatMaj)
	a.mux.HandleFunc("POST /api/univers/mise-a-jour/chercher", a.routeChercherMaj)
	a.mux.HandleFunc("PUT /api/univers/mise-a-jour/canal", a.routeCanalMaj)
	a.mux.HandleFunc("POST /api/univers/mise-a-jour/installer", a.routeInstallerMaj)
}

// commandeMajServeur : la mise à jour d'un serveur installé par docker compose (README).
const commandeMajServeur = "git pull && docker compose up -d --build"

func (a *Application) routesMajServeur() {
	a.maj = &misesAJour{}
	a.mux.HandleFunc("GET /api/univers/version", a.routeVersionServeur)
}

// routeVersionServeur : la version du serveur et ses numéros, la dernière version stable publiée
// si elle est plus récente, et les copies de l'univers visé avec leur version (administrateur).
func (a *Application) routeVersionServeur(w http.ResponseWriter, r *http.Request) {
	out := a.vueMaj()
	delete(out, "automatique")
	delete(out, "installable")
	out["protocole"], out["modele"], out["commande"] = store.VersionProtocole, store.VersionModele, commandeMajServeur
	copies := []map[string]any{}
	a.mu.RLock()
	o := a.vise(r)
	a.mu.RUnlock()
	if o != nil {
		out["univers"] = nomUnivers(o.st.Chemin())
		ici := synchro.Ici()
		for _, c := range o.st.Copies() {
			ecart := ""
			switch synchro.Comparer(synchro.Versions{Version: c.Version, Protocole: c.Protocole, Modele: c.Modele}, ici) {
			case synchro.ServeurRecent:
				ecart = ecartServeurRecent // ce poste est à mettre à jour
			case synchro.BureauRecent:
				ecart = ecartBureauRecent // ce poste attend la mise à jour du serveur
			}
			copies = append(copies, map[string]any{"numero": c.Numero, "nom": c.Nom, "compte": c.Compte, "vu": c.Vu,
				"version": c.Version, "ecart": ecart})
		}
	}
	out["copies"] = copies
	repondre(w, 200, out)
}

func (a *Application) canal() string {
	if v, _ := a.reg.Meta(regCanal); v == majour.Test {
		return majour.Test
	}
	return majour.Stable
}

// chercherMiseAJour interroge les versions publiées du canal.
func (a *Application) chercherMiseAJour() {
	d, err := chercherMaj(a.canal(), a.opt.Version)
	a.maj.mu.Lock()
	defer a.maj.mu.Unlock()
	a.maj.verifie = time.Now()
	if err != nil {
		a.maj.erreur = err.Error()
		return
	}
	a.maj.dispo, a.maj.erreur = d, ""
}

// verifierEnFond : une recherche peu après le lancement, puis toutes les six heures.
func (a *Application) verifierEnFond() {
	t := time.NewTimer(premiereRecherche)
	defer t.Stop()
	for {
		select {
		case <-a.fin:
			return
		case <-t.C:
		}
		a.chercherMiseAJour()
		t.Reset(intervalleMaj)
	}
}

func (a *Application) vueMaj() map[string]any {
	a.maj.mu.Lock()
	defer a.maj.mu.Unlock()
	chemin, _ := majour.Programme()
	_, publiee := majour.Lire(a.opt.Version)
	out := map[string]any{"version": a.opt.Version, "canal": a.canal(), "automatique": chemin != "" && publiee && a.opt.Relancer != nil,
		"erreur": a.maj.erreur, "installation": a.maj.installant}
	if !a.maj.verifie.IsZero() {
		out["verifie"] = float64(a.maj.verifie.UnixNano()) / 1e9
	}
	if d := a.maj.dispo; d != nil {
		out["disponible"] = d
		out["installable"] = d.Installable()
	}
	return out
}

func refuserMajServeur(a *Application, w http.ResponseWriter) bool {
	if a.multi() || a.maj == nil {
		erreur(w, 404, "Le serveur se met à jour par docker compose.")
		return true
	}
	return false
}

func (a *Application) routeEtatMaj(w http.ResponseWriter, r *http.Request) {
	if refuserMajServeur(a, w) {
		return
	}
	repondre(w, 200, a.vueMaj())
}

func (a *Application) routeChercherMaj(w http.ResponseWriter, r *http.Request) {
	if refuserMajServeur(a, w) {
		return
	}
	a.chercherMiseAJour()
	repondre(w, 200, a.vueMaj())
}

// routeCanalMaj : {"canal": "stable" | "test"}, puis une recherche dans ce canal.
func (a *Application) routeCanalMaj(w http.ResponseWriter, r *http.Request) {
	if refuserMajServeur(a, w) {
		return
	}
	var in struct {
		Canal string `json:"canal"`
	}
	if err := lireJSON(r, &in); err != nil {
		erreurStore(w, err)
		return
	}
	if in.Canal != majour.Stable && in.Canal != majour.Test {
		erreur(w, 422, "Canal inconnu : stable ou test.")
		return
	}
	v := ""
	if in.Canal == majour.Test {
		v = majour.Test
	}
	if err := a.reg.SetMeta(regCanal, v); err != nil {
		erreurStore(w, err)
		return
	}
	a.maj.mu.Lock()
	a.maj.dispo = nil
	a.maj.mu.Unlock()
	a.chercherMiseAJour()
	repondre(w, 200, a.vueMaj())
}

// routeInstallerMaj installe la version trouvée, puis relance l'application (l'univers est
// sauvegardé et fermé avant). La réponse part avant le redémarrage.
func (a *Application) routeInstallerMaj(w http.ResponseWriter, r *http.Request) {
	if refuserMajServeur(a, w) {
		return
	}
	a.maj.mu.Lock()
	d := a.maj.dispo
	if d == nil || a.maj.installant {
		a.maj.mu.Unlock()
		erreur(w, 409, "Aucune mise à jour à installer.")
		return
	}
	a.maj.installant = true
	a.maj.mu.Unlock()
	chemin, err := installerMaj(d)
	if err != nil {
		a.maj.mu.Lock()
		a.maj.installant = false
		a.maj.mu.Unlock()
		erreur(w, 422, majuscule(err.Error())+".")
		return
	}
	log.Printf("mise à jour installée : %s (%s)", d.Version, chemin)
	repondre(w, 200, map[string]any{"installee": d.Version, "relance": a.opt.Relancer != nil})
	if a.opt.Relancer != nil {
		go func() {
			time.Sleep(700 * time.Millisecond) // le temps que la réponse arrive à l'écran
			a.opt.Relancer(chemin)
		}()
	}
}
