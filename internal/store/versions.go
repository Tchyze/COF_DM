package store

import "strconv"

// Versions de l'échange et du modèle, distinctes de la version de l'application : deux versions de
// COF DM qui ont les mêmes numéros se synchronisent sans rien perdre, quelles que soient leurs
// versions.
//
// VersionProtocole change quand la forme de l'échange change (routes /api/sync, Reponse, Conflit).
//
// VersionModele change à chaque ajout ou changement de ce qu'un univers contient et que la
// synchronisation transporte : type de fiche, champ, options d'un choix, relation, fonctionnalité,
// réglage synchronisé, collection de documents, forme d'une feuille, et toute hausse de
// versionDonnees. Le test TestVersionModele garde une empreinte du modèle : il échoue tant qu'elle
// change sans que VersionModele augmente.
//
// Modèle 1 : celui de la version 1.2.2 (aides de jeu, fonctionnalités optionnelles). Un serveur ou
// un bureau qui ne publie pas ses numéros est plus ancien que cette règle : 1 depuis la 1.2.2, 0
// avant (internal/synchro).
//
// Modèle 2 : les profils, peuples et voies sont rangés un par un (collections « profils »,
// « peuples », « voies ») et non plus dans le réglage « reference » ; ils portent une étiquette
// (officielle ou maison), et les capacités leurs bonus, leur action en combat et leurs utilisations.
//
// Modèle 3 : la feuille compte les utilisations des capacités (« utilisations ») ; un repos des
// règles peut valoir un nouveau jour (« nouveau_jour »).
//
// Modèle 4 : l'édition par les joueurs (réglage « edition_joueurs ») se synchronise, pour que le MJ
// la règle depuis un bureau relié au serveur.
//
// Modèle 5 : la feuille garde l'historique des montées de niveau (« niveaux ») et les points de
// capacité en réserve (« points_capacite ») ; le fichier de règles décrit la montée de niveau
// (« personnage.montee »).
const (
	VersionProtocole = 1
	VersionModele    = 5
)

// MetaModele : le modèle le plus récent qui a ouvert cet univers. Une application plus ancienne
// refuse de l'ouvrir : elle perdrait ce qu'elle ne connaît pas.
const MetaModele = "version_modele"

// MsgTropRecent : un univers ou un export venu d'une version plus récente de COF DM.
const MsgTropRecent = "Cet univers vient d'une version plus récente de COF DM : mettez l'application à jour pour l'ouvrir."

// verifierVersions refuse un univers plus récent que l'application (modèle ou forme des données),
// puis y note le modèle de l'application s'il est plus ancien.
func verifierVersions(q execer) error {
	if metaEntier(q, MetaModele) > VersionModele || metaEntier(q, "version_donnees") > donneesCourantes() {
		return ErrValidation{Msg: MsgTropRecent}
	}
	if metaEntier(q, MetaModele) == VersionModele {
		return nil
	}
	_, err := q.Exec(`INSERT OR REPLACE INTO meta (cle, valeur) VALUES (?, ?)`, MetaModele, strconv.Itoa(VersionModele))
	return err
}

func donneesCourantes() int64 {
	n, _ := strconv.ParseInt(versionDonnees, 10, 64)
	return n
}

// Modele : le modèle noté dans cet univers.
func (s *Store) Modele() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return metaEntier(s.db, MetaModele)
}

// verifierExport refuse un export plus récent que l'application.
func verifierExport(x Export) error {
	donnees, _ := strconv.ParseInt(x.Donnees, 10, 64)
	if x.Modele > VersionModele || donnees > donneesCourantes() {
		return invalide("Cet export vient d'une version plus récente de COF DM : mettez l'application à jour avant de le restaurer.")
	}
	return nil
}
