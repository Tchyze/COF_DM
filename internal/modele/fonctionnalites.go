package modele

// Fonctionnalite : une partie de l'application que le MJ active ou coupe, univers par univers
// (Paramètres → Fonctionnalités). Coupée, elle disparaît de l'interface, ses routes et ses outils
// MCP sont refusés et ses types de fiches ne se créent plus ; ses données restent dans l'univers,
// intactes, et reviennent quand on la réactive. Le cœur (PNJ, lieux, factions, objets, lore,
// événements, campagnes, quêtes, séances, PJ, bestiaire) n'est jamais coupé.
type Fonctionnalite struct {
	Cle         string   `json:"cle"`
	Nom         string   `json:"nom"`
	Description string   `json:"description"`
	Groupe      string   `json:"groupe"`
	Defaut      bool     `json:"defaut"`          // état d'un univers neuf
	Types       []string `json:"types,omitempty"` // types de fiches qu'elle apporte
}

// Fonctionnalites, dans l'ordre du panneau.
var Fonctionnalites = []Fonctionnalite{
	{Cle: "ecran_mj", Nom: "Écran du MJ", Groupe: "Partie", Defaut: true,
		Description: "La partie en une page : joueurs, scène, quête, séance et combat."},
	{Cle: "seances", Nom: "Préparation et après-séance", Groupe: "Partie", Defaut: true,
		Description: "Rassembler ce qu'il faut pour une séance, puis la clore en une fois."},
	{Cle: "combat", Nom: "Combat", Groupe: "Partie", Defaut: true,
		Description: "Écran de combat (initiative, PV, attaques, états), combats des séances et hauts faits."},
	{Cle: "rencontres", Nom: "Rencontres", Groupe: "Partie", Defaut: true, Types: []string{"rencontre"},
		Description: "Fiches de rencontre avec leurs créatures du bestiaire, et leur difficulté."},
	{Cle: "butin", Nom: "Butin et pot commun", Groupe: "Partie", Defaut: true,
		Description: "Le butin chiffré des rencontres, versé au pot commun de la campagne, puis partagé entre les PJ."},
	{Cle: "secrets", Nom: "Secrets", Groupe: "Partie", Defaut: true, Types: []string{"secret"},
		Description: "Fiches de secret, « qui sait quoi » et révélations aux PJ."},
	{Cle: "feuilles", Nom: "Feuilles de personnage", Groupe: "Joueurs", Defaut: false,
		Description: "La feuille de chaque PJ (caractéristiques, PV, voies, équipement), ses repos et ses profils."},
	{Cle: "page_joueurs", Nom: "Page des joueurs", Groupe: "Joueurs", Defaut: false,
		Description: "La page que les joueurs suivent sur leur téléphone : leur feuille, le combat, leurs documents, les dés."},
	{Cle: "des", Nom: "Dés virtuels", Groupe: "Joueurs", Defaut: false,
		Description: "Le lanceur de dés du MJ (bouton 🎲, touche D) et celui de la page des joueurs."},
	{Cle: "aides", Nom: "Aides de jeu", Groupe: "Joueurs", Defaut: false, Types: []string{"aide"},
		Description: "Lettres, images et indices envoyés aux PJ, dans les documents de leur page."},
	{Cle: "regles", Nom: "Fiches de règles", Groupe: "Outils", Defaut: true, Types: []string{"regle"},
		Description: "Aide-mémoire des règles officielles et maison, en fiches."},
	{Cle: "graphe", Nom: "Graphe", Groupe: "Outils", Defaut: true,
		Description: "Les fiches et leurs liens, dessinés."},
	{Cle: "chronologie", Nom: "Chronologie", Groupe: "Outils", Defaut: true,
		Description: "Les événements et les séances sur le calendrier du monde."},
	{Cle: "organigramme", Nom: "Organigrammes", Groupe: "Outils", Defaut: true,
		Description: "La hiérarchie de chaque faction."},
	{Cle: "obsidian", Nom: "Export vers Obsidian", Groupe: "Outils", Defaut: true,
		Description: "Une note Markdown par fiche, pour un vault Obsidian."},
}

// FonctionnaliteDe renvoie une fonctionnalité.
func FonctionnaliteDe(cle string) (Fonctionnalite, bool) {
	for _, f := range Fonctionnalites {
		if f.Cle == cle {
			return f, true
		}
	}
	return Fonctionnalite{}, false
}

// FonctionnaliteDuType : la fonctionnalité qui apporte ce type de fiche ("" : le cœur).
func FonctionnaliteDuType(typ string) string {
	for _, f := range Fonctionnalites {
		for _, t := range f.Types {
			if t == typ {
				return f.Cle
			}
		}
	}
	return ""
}
