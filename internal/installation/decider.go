package installation

import "github.com/Tchyze/cof_dm/internal/majour"

// Action : ce que fait un programme lancé hors de son dossier d'installation.
type Action int

const (
	Continuer      Action = iota // c'est le programme installé : il s'ouvre
	Installer                    // il s'installe (première fois, ou plus récent que l'installé), puis lance l'installé
	LancerInstalle               // l'installé est aussi récent : il le lance, sans rien remplacer
)

// Decider : que faire au lancement de « exe », en version « ici », quand le programme installé est
// « installe » (présent ou non, en version « versionInstallee », "" si elle est inconnue). Une
// copie plus ancienne (l'exe resté sur le Bureau quand l'installé s'est mis à jour) ne remplace
// jamais une version plus récente.
func Decider(exe, installe string, present bool, ici, versionInstallee string) Action {
	switch {
	case MemeChemin(exe, installe):
		return Continuer
	case !present:
		return Installer
	}
	a, ok := majour.Lire(ici)
	b, okb := majour.Lire(versionInstallee)
	if !ok || (okb && majour.Comparer(a, b) <= 0) {
		return LancerInstalle
	}
	return Installer
}
