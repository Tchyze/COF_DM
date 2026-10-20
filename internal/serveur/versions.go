package serveur

import (
	"net/http"

	"github.com/Tchyze/cof_dm/internal/store"
	"github.com/Tchyze/cof_dm/internal/synchro"
)

// Versions de l'échange et du modèle (internal/store/versions.go) : publiées par GET /api/sante, et
// comparées par le bureau avant de tirer une copie, d'envoyer un univers ou d'échanger.

// sante : l'application répond, avec sa version et ses numéros.
func (s *Serveur) sante(w http.ResponseWriter, r *http.Request) {
	repondre(w, 200, map[string]any{"ok": true, "version": s.opt.Version,
		"protocole": store.VersionProtocole, "modele": store.VersionModele})
}

// msgServeurRecent : le serveur a un modèle (ou un protocole) que ce bureau ne connaît pas.
func msgServeurRecent(srv synchro.Versions) string {
	return "Le serveur est en " + srv.Affichee() + ", plus récent que cette application (" + synchro.Ici().Affichee() +
		") : mettez-la à jour (Paramètres → Mises à jour) pour reprendre la synchronisation."
}

// msgBureauRecent : le serveur ne connaît pas encore le modèle de ce bureau.
func msgBureauRecent(srv synchro.Versions) string {
	return "Le serveur est en " + srv.Affichee() + ", plus ancien que cette application (" + synchro.Ici().Affichee() +
		") : demandez à son administrateur de le mettre à jour."
}

// verifierServeur : avant de tirer une copie (envoi false) ou d'envoyer un univers (envoi true), le
// serveur et ce bureau doivent pouvoir échanger. Une copie se tire d'un serveur plus ancien (le
// bureau en reçoit les changements) ; un univers ne s'envoie qu'à un serveur au même modèle, sans
// quoi l'un des deux côtés ne pourrait pas ouvrir l'autre. Répond et renvoie false sinon.
func (a *Application) verifierServeur(w http.ResponseWriter, c *connexion, envoi bool) bool {
	srv, err := synchro.Nouveau(c.URL, c.Jeton, "").Versions()
	a.noterServeur(err)
	if err != nil {
		erreurDistante(w, err)
		return false
	}
	switch synchro.Comparer(synchro.Ici(), srv) {
	case synchro.ServeurRecent:
		erreur(w, 409, msgServeurRecent(srv))
		return false
	case synchro.BureauRecent:
		if envoi {
			erreur(w, 409, msgBureauRecent(srv)+" Un univers ne s'envoie qu'à un serveur à jour.")
			return false
		}
	}
	return true
}
