// Package modele décrit les types de fiches de COF DM (PNJ, lieux, quêtes…), leurs champs, les types
// de relations et le calendrier. C'est la seule source de vérité : le serveur valide avec, l'interface
// construit ses formulaires avec, et l'API la publie (GET /api/schema) pour les clients comme Claude.
package modele

// Genres de champ.
const (
	Texte       = "texte"       // une ligne
	Long        = "long"        // Markdown, liens [[Nom]] reconnus
	Choix       = "choix"       // une valeur parmi Options
	Nombre      = "nombre"      // entier ou décimal
	Booleen     = "booleen"     // oui / non
	Ref         = "ref"         // une autre fiche (id), d'un des types Cibles
	Refs        = "refs"        // plusieurs fiches (ids)
	DateMonde   = "date_jeu"    // date du calendrier du monde : {an, mois, jour}
	DateReelle  = "date"        // date réelle AAAA-MM-JJ
	Etapes      = "etapes"      // liste de cases à cocher : [{texte, fait}]
	Adversaires = "adversaires" // [{monstre, nom, qte, camp}] : monstres du bestiaire de l'univers
	Monstre     = "monstre"     // identifiant d'un monstre du bestiaire de l'univers
	Equipement  = "equipement"  // chiffres d'un objet porté : {emplacement, bonus: [{cible, valeur}], arme: {type, dm, mod, portee}}
	Butin       = "butin"       // pièces et objets : {pieces: {po, pa, pc}, objets: [{fiche, nom, quantite}]}
)

// Champ d'un type de fiche.
type Champ struct {
	Cle     string   `json:"cle"`
	Nom     string   `json:"nom"`
	Genre   string   `json:"genre"`
	Options []string `json:"options,omitempty"` // Choix ; Texte : suggestions
	Cibles  []string `json:"cibles,omitempty"`  // Ref / Refs : types acceptés
	Aide    string   `json:"aide,omitempty"`
	// Liste : les choix proposés viennent de cette liste du fichier de règles de l'univers
	// (internal/regles), et non du code ; sans fichier, le champ se saisit librement.
	Liste  string `json:"liste,omitempty"`
	Secret bool   `json:"secret,omitempty"` // réservé au MJ (jamais montré aux joueurs)
	Joueur bool   `json:"joueur,omitempty"` // modifiable par le joueur du PJ, quand l'édition par les joueurs est ouverte
	Groupe string `json:"groupe,omitempty"` // regroupement dans le formulaire
}

// Type de fiche.
type Type struct {
	Cle     string   `json:"cle"`
	Nom     string   `json:"nom"`
	Pluriel string   `json:"pluriel"`
	Icone   string   `json:"icone"`
	Champs  []Champ  `json:"champs"`
	Carte   []string `json:"carte"` // champs résumés sur les cartes de liste
}

var (
	tousTypes = []string{"pnj", "lieu", "faction", "objet", "lore", "evenement", "campagne", "quete", "seance", "pj", "regle", "rencontre"}
)

// Types dans l'ordre de la navigation.
var Types = []Type{
	{
		Cle: "pnj", Nom: "PNJ", Pluriel: "PNJ", Icone: "👤", Carte: []string{"statut", "profil", "lieu"},
		Champs: []Champ{
			{Cle: "resume", Nom: "En une phrase", Genre: Texte, Aide: "Qui est-ce ?"},
			{Cle: "motivation", Nom: "Ce qu'il veut", Genre: Texte, Aide: "Son but, ce qui le fait agir"},
			{Cle: "alias", Nom: "Alias", Genre: Texte, Aide: "Faux nom, surnom"},
			{Cle: "statut", Nom: "Statut", Genre: Choix, Options: []string{"vivant", "mort", "disparu", "inconnu"}},
			{Cle: "attitude", Nom: "Attitude envers les PJ", Genre: Choix, Options: []string{"hostile", "méfiante", "neutre", "amicale", "alliée"}},
			{Cle: "peuple", Nom: "Peuple", Genre: Texte, Liste: "peuples_pnj"},
			{Cle: "profil", Nom: "Profil", Genre: Texte, Liste: "profils_pnj"},
			{Cle: "sexe", Nom: "Sexe", Genre: Texte},
			{Cle: "age", Nom: "Âge", Genre: Texte},
			{Cle: "metier", Nom: "Métier", Genre: Texte, Aide: "Ce qu'il fait dans le monde (le profil est celui des règles)"},
			{Cle: "allegeance", Nom: "Allégeance apparente", Genre: Ref, Cibles: []string{"faction"}, Aide: "Ce qu'il affiche ; sa vraie appartenance (même secrète) se règle dans l'organigramme"},
			{Cle: "responsable", Nom: "Responsable", Genre: Ref, Cibles: []string{"pnj", "pj"}, Aide: "À qui il répond : il se place sous lui dans les organigrammes"},
			{Cle: "lieu", Nom: "Où le trouver", Genre: Ref, Cibles: []string{"lieu"}},
			{Cle: "campagnes", Nom: "Campagnes", Genre: Refs, Cibles: []string{"campagne"}},
			{Cle: "apparence", Nom: "Apparence", Genre: Long, Groupe: "Portrait"},
			{Cle: "personnalite", Nom: "Personnalité & manières", Genre: Long, Groupe: "Portrait"},
			{Cle: "voix", Nom: "Voix & tics", Genre: Texte, Groupe: "Portrait", Aide: "Pour le jouer à la table : accent, expression, geste"},
			{Cle: "role", Nom: "Rôle dans l'histoire", Genre: Long, Groupe: "Portrait"},
			{Cle: "profil_combat", Nom: "Profil de combat", Genre: Monstre, Groupe: "Combat", Aide: "Monstre du bestiaire de l'univers"},
			{Cle: "responsable_secret", Nom: "Responsable secret", Genre: Ref, Cibles: []string{"pnj", "pj"}, Secret: true, Groupe: "MJ", Aide: "Son vrai maître, connu du MJ seulement"},
			{Cle: "secrets", Nom: "Secrets", Genre: Long, Secret: true, Groupe: "MJ"},
			{Cle: "notes", Nom: "Notes du MJ", Genre: Long, Secret: true, Groupe: "MJ"},
		},
	},
	{
		Cle: "lieu", Nom: "Lieu", Pluriel: "Lieux", Icone: "🏰", Carte: []string{"categorie", "parent"},
		Champs: []Champ{
			{Cle: "impression", Nom: "Impression", Genre: Texte, Aide: "Ce que les PJ voient en arrivant"},
			{Cle: "categorie", Nom: "Catégorie", Genre: Choix, Options: []string{"royaume", "région", "ville", "village", "quartier", "fort", "bâtiment", "auberge", "temple", "ferme", "donjon", "repaire", "site", "route", "cours d'eau", "forêt", "montagne", "côte", "autre"}},
			{Cle: "statut", Nom: "État", Genre: Choix, Options: []string{"florissant", "ordinaire", "en déclin", "abandonné", "en ruine", "détruit"}},
			{Cle: "population", Nom: "Population", Genre: Texte, Aide: "Combien, et qui"},
			{Cle: "danger", Nom: "Danger", Genre: Texte, Aide: "Niveau conseillé, menaces"},
			{Cle: "parent", Nom: "Fait partie de", Genre: Ref, Cibles: []string{"lieu"}},
			{Cle: "dirigeant", Nom: "Dirigeant", Genre: Ref, Cibles: []string{"pnj", "pj", "faction"}},
			{Cle: "campagnes", Nom: "Campagnes", Genre: Refs, Cibles: []string{"campagne"}},
			{Cle: "description", Nom: "Description", Genre: Long},
			{Cle: "lieux_notables", Nom: "Lieux notables", Genre: Long},
			{Cle: "rumeurs", Nom: "Rumeurs & accroches", Genre: Long},
			{Cle: "secrets", Nom: "Secrets", Genre: Long, Secret: true, Groupe: "MJ"},
		},
	},
	{
		Cle: "faction", Nom: "Faction", Pluriel: "Factions", Icone: "⚑", Carte: []string{"categorie", "attitude"},
		Champs: []Champ{
			{Cle: "but", Nom: "But", Genre: Texte, Aide: "En une phrase"},
			{Cle: "categorie", Nom: "Catégorie", Genre: Choix, Options: []string{"pouvoir & institution", "guilde & compagnie", "ordre religieux", "culte", "hors-la-loi & rebelles", "tribu & peuple", "autre"}},
			{Cle: "statut", Nom: "Statut", Genre: Choix, Options: []string{"active", "en essor", "affaiblie", "en sommeil", "dissoute", "détruite"}},
			{Cle: "parent", Nom: "Fait partie de", Genre: Ref, Cibles: []string{"faction"}, Aide: "La faction dont elle est une branche"},
			{Cle: "siege", Nom: "Siège", Genre: Ref, Cibles: []string{"lieu"}},
			{Cle: "dirigeant", Nom: "Dirigeant", Genre: Ref, Cibles: []string{"pnj", "pj"}},
			{Cle: "attitude", Nom: "Attitude envers les PJ", Genre: Choix, Options: []string{"hostile", "méfiante", "neutre", "amicale", "alliée"}},
			{Cle: "campagnes", Nom: "Campagnes", Genre: Refs, Cibles: []string{"campagne"}},
			{Cle: "description", Nom: "Description", Genre: Long},
			{Cle: "symbole", Nom: "Symbole & devise", Genre: Texte},
			{Cle: "effectif", Nom: "Effectif", Genre: Texte, Aide: "Combien de membres, et lesquels"},
			{Cle: "ressources", Nom: "Ressources & influence", Genre: Long},
			{Cle: "secrets", Nom: "Secrets", Genre: Long, Secret: true, Groupe: "MJ"},
		},
	},
	{
		Cle: "objet", Nom: "Objet", Pluriel: "Objets", Icone: "🗝", Carte: []string{"categorie", "rarete", "detenteur"},
		Champs: []Champ{
			{Cle: "resume", Nom: "En une phrase", Genre: Texte},
			{Cle: "categorie", Nom: "Catégorie", Genre: Choix, Options: []string{"arme", "armure", "bouclier", "potion", "parchemin", "anneau & bijou", "objet merveilleux", "document", "trésor", "autre"}},
			{Cle: "rarete", Nom: "Rareté", Genre: Choix, Options: []string{"courant", "peu courant", "rare", "très rare", "légendaire", "unique"}},
			{Cle: "prix", Nom: "Prix", Genre: Texte, Aide: "En pc, pa ou po"},
			{Cle: "magique", Nom: "Magique", Genre: Booleen},
			{Cle: "identifie", Nom: "Identifié par les PJ", Genre: Booleen, Aide: "Les PJ connaissent ses pouvoirs"},
			{Cle: "charges", Nom: "Charges", Genre: Texte, Aide: "Utilisations restantes : 3 / 5, une dose…"},
			{Cle: "poids", Nom: "Poids", Genre: Texte},
			{Cle: "createur", Nom: "Créateur", Genre: Ref, Cibles: []string{"pnj", "pj", "faction"}},
			{Cle: "detenteur", Nom: "Détenteur", Genre: Ref, Cibles: []string{"pnj", "pj", "faction", "lieu"}, Aide: "Un PJ : l'objet est dans son inventaire"},
			{Cle: "campagnes", Nom: "Campagnes", Genre: Refs, Cibles: []string{"campagne"}},
			{Cle: "equipement", Nom: "Chiffres", Genre: Equipement, Aide: "Emplacement, bonus et arme : ils comptent quand un PJ le porte"},
			{Cle: "effets", Nom: "Effets (règles)", Genre: Long},
			{Cle: "histoire", Nom: "Histoire", Genre: Long},
			{Cle: "secrets", Nom: "Secrets / malédiction", Genre: Long, Secret: true, Groupe: "MJ"},
		},
	},
	{
		Cle: "lore", Nom: "Lore", Pluriel: "Lore", Icone: "📜", Carte: []string{"categorie"},
		Champs: []Champ{
			{Cle: "categorie", Nom: "Catégorie", Genre: Choix, Options: []string{"histoire", "religion", "magie", "légende", "géographie", "peuple & culture", "calendrier", "économie", "autre"}},
			{Cle: "campagnes", Nom: "Campagnes", Genre: Refs, Cibles: []string{"campagne"}},
			{Cle: "contenu", Nom: "Contenu", Genre: Long},
			{Cle: "secrets", Nom: "Vérité (MJ)", Genre: Long, Secret: true, Groupe: "MJ"},
		},
	},
	{
		Cle: "evenement", Nom: "Événement", Pluriel: "Événements", Icone: "⏳", Carte: []string{"date_jeu", "lieu"},
		Champs: []Champ{
			{Cle: "date_jeu", Nom: "Date", Genre: DateMonde},
			{Cle: "date_fin", Nom: "Jusqu'au", Genre: DateMonde, Aide: "Pour ce qui dure : guerre, siège, règne"},
			{Cle: "statut", Nom: "Statut", Genre: Choix, Options: []string{"passé", "en cours", "prévu"}, Aide: "Prévu : ce qui arrivera si les PJ n'agissent pas"},
			{Cle: "categorie", Nom: "Catégorie", Genre: Choix, Options: []string{"bataille & guerre", "politique", "catastrophe", "fondation", "naissance & mort", "religion & prodige", "crime", "autre"}},
			{Cle: "lieu", Nom: "Lieu", Genre: Ref, Cibles: []string{"lieu"}},
			{Cle: "campagne", Nom: "Campagne", Genre: Ref, Cibles: []string{"campagne"}, Aide: "Vide : histoire du monde"},
			{Cle: "acteurs", Nom: "Acteurs", Genre: Refs, Cibles: []string{"pnj", "pj", "faction"}},
			{Cle: "quetes", Nom: "Quêtes liées", Genre: Refs, Cibles: []string{"quete"}},
			{Cle: "publique", Nom: "Ce qu'on en raconte", Genre: Long},
			{Cle: "consequences", Nom: "Conséquences", Genre: Long},
			{Cle: "verite", Nom: "Vérité (MJ)", Genre: Long, Secret: true, Groupe: "MJ"},
		},
	},
	{
		Cle: "campagne", Nom: "Campagne", Pluriel: "Campagnes", Icone: "📖", Carte: []string{"statut", "date_actuelle"},
		Champs: []Champ{
			{Cle: "statut", Nom: "Statut", Genre: Choix, Options: []string{"en préparation", "en cours", "en pause", "terminée"}},
			{Cle: "pitch", Nom: "Pitch", Genre: Long},
			{Cle: "themes", Nom: "Thèmes & ton", Genre: Texte},
			{Cle: "date_debut", Nom: "Début (en jeu)", Genre: DateMonde},
			{Cle: "date_actuelle", Nom: "Date actuelle (en jeu)", Genre: DateMonde},
			{Cle: "date_fin", Nom: "Fin (en jeu)", Genre: DateMonde},
			{Cle: "niveaux", Nom: "Niveaux des PJ", Genre: Texte, Aide: "Au départ et visés : 1 à 5…"},
			{Cle: "lieu_groupe", Nom: "Où est le groupe", Genre: Ref, Cibles: []string{"lieu"}},
			{Cle: "arc", Nom: "Arc principal", Genre: Long},
			{Cle: "regles_maison", Nom: "Règles de la campagne", Genre: Refs, Cibles: []string{"regle"}},
			{Cle: "fils_rouges", Nom: "Fils rouges", Genre: Long, Secret: true, Groupe: "MJ"},
			{Cle: "notes", Nom: "Notes du MJ", Genre: Long, Secret: true, Groupe: "MJ"},
		},
	},
	{
		Cle: "quete", Nom: "Quête", Pluriel: "Quêtes", Icone: "✦", Carte: []string{"statut", "categorie", "campagne"},
		Champs: []Champ{
			{Cle: "campagne", Nom: "Campagne", Genre: Ref, Cibles: []string{"campagne"}},
			{Cle: "categorie", Nom: "Catégorie", Genre: Choix, Options: []string{"arc principal", "mission", "secondaire"}},
			{Cle: "ordre", Nom: "Ordre", Genre: Nombre, Aide: "Pour trier l'arc principal"},
			{Cle: "parent", Nom: "Fait partie de", Genre: Ref, Cibles: []string{"quete"}, Aide: "La quête dont c'est une étape"},
			{Cle: "statut", Nom: "Statut", Genre: Choix, Options: []string{"à venir", "en cours", "réussie", "échouée", "abandonnée"}},
			{Cle: "commanditaire", Nom: "Commanditaire", Genre: Ref, Cibles: []string{"pnj", "faction"}},
			{Cle: "lieu", Nom: "Lieu", Genre: Ref, Cibles: []string{"lieu"}},
			{Cle: "pnj", Nom: "PNJ impliqués", Genre: Refs, Cibles: []string{"pnj"}},
			{Cle: "echeance", Nom: "Échéance (en jeu)", Genre: DateMonde, Aide: "Passé ce jour, c'est trop tard"},
			{Cle: "niveau", Nom: "Niveau conseillé", Genre: Texte},
			{Cle: "accroche", Nom: "Accroche", Genre: Long},
			{Cle: "objectif", Nom: "Objectif", Genre: Long},
			{Cle: "etapes", Nom: "Étapes", Genre: Etapes},
			{Cle: "indices", Nom: "Indices", Genre: Long},
			{Cle: "recompense", Nom: "Récompense", Genre: Long, Aide: "Argent, objets ([[Nom]] d'une fiche), faveurs"},
			{Cle: "consequences", Nom: "Conséquences", Genre: Long},
			{Cle: "secrets", Nom: "Secrets", Genre: Long, Secret: true, Groupe: "MJ"},
		},
	},
	{
		Cle: "seance", Nom: "Séance", Pluriel: "Séances", Icone: "🎲", Carte: []string{"statut", "date_reelle", "date_jeu"},
		Champs: []Champ{
			{Cle: "campagne", Nom: "Campagne", Genre: Ref, Cibles: []string{"campagne"}},
			{Cle: "numero", Nom: "Numéro", Genre: Nombre},
			{Cle: "statut", Nom: "Statut", Genre: Choix, Options: []string{"prévue", "jouée"}},
			{Cle: "date_reelle", Nom: "Date réelle", Genre: DateReelle},
			{Cle: "date_jeu", Nom: "Date en jeu", Genre: DateMonde},
			{Cle: "date_jeu_fin", Nom: "Jusqu'au (en jeu)", Genre: DateMonde, Aide: "Quand la séance couvre plusieurs jours"},
			{Cle: "presents", Nom: "PJ présents", Genre: Refs, Cibles: []string{"pj"}},
			{Cle: "quetes", Nom: "Quêtes avancées", Genre: Refs, Cibles: []string{"quete"}},
			{Cle: "lieux", Nom: "Lieux visités", Genre: Refs, Cibles: []string{"lieu"}},
			{Cle: "pnj", Nom: "PNJ rencontrés", Genre: Refs, Cibles: []string{"pnj"}},
			{Cle: "resume", Nom: "Résumé", Genre: Long},
			{Cle: "butin", Nom: "Butin & récompenses", Genre: Long},
			{Cle: "preparation", Nom: "Préparation", Genre: Long, Secret: true, Groupe: "MJ", Aide: "Réservée au MJ : elle peut nommer des secrets et leurs indices"},
			{Cle: "notes", Nom: "Notes du MJ", Genre: Long, Secret: true, Groupe: "MJ"},
		},
	},
	{
		Cle: "rencontre", Nom: "Rencontre", Pluriel: "Rencontres", Icone: "⚔", Carte: []string{"statut", "quete", "lieu"},
		Champs: []Champ{
			{Cle: "resume", Nom: "En une phrase", Genre: Texte},
			{Cle: "statut", Nom: "Statut", Genre: Choix, Options: []string{"préparée", "jouée", "évitée"}},
			{Cle: "campagne", Nom: "Campagne", Genre: Ref, Cibles: []string{"campagne"}},
			{Cle: "quete", Nom: "Quête", Genre: Ref, Cibles: []string{"quete"}},
			{Cle: "seance", Nom: "Séance", Genre: Ref, Cibles: []string{"seance"}},
			{Cle: "lieu", Nom: "Lieu", Genre: Ref, Cibles: []string{"lieu"}},
			{Cle: "adversaires", Nom: "Créatures", Genre: Adversaires, Aide: "Monstres du bestiaire de l'univers"},
			{Cle: "pnj", Nom: "PNJ présents", Genre: Refs, Cibles: []string{"pnj"}, Aide: "Ceux qui ont un profil de combat comptent dans la difficulté"},
			{Cle: "mise_en_scene", Nom: "Mise en scène", Genre: Long},
			{Cle: "butin", Nom: "Butin", Genre: Long},
			{Cle: "butin_chiffre", Nom: "Pièces et objets", Genre: Butin, Aide: "Versés au pot commun de la campagne à la fin du combat"},
			{Cle: "butin_verse", Nom: "Butin versé au pot", Genre: Booleen, Aide: "Coché au versement : le butin ne se verse pas deux fois"},
			{Cle: "tactique", Nom: "Tactique", Genre: Long, Secret: true, Groupe: "MJ"},
			{Cle: "notes", Nom: "Notes du MJ", Genre: Long, Secret: true, Groupe: "MJ"},
		},
	},
	{
		Cle: "pj", Nom: "PJ", Pluriel: "PJ", Icone: "🛡", Carte: []string{"joueur", "profil", "niveau"},
		Champs: []Champ{
			{Cle: "joueur", Nom: "Joueur", Genre: Texte},
			{Cle: "campagne", Nom: "Campagne", Genre: Ref, Cibles: []string{"campagne"}},
			{Cle: "statut", Nom: "Statut", Genre: Choix, Options: []string{"actif", "absent", "mort", "parti"}},
			{Cle: "peuple", Nom: "Peuple", Genre: Texte, Liste: "peuples"},
			{Cle: "profil", Nom: "Profil", Genre: Texte, Liste: "profils"},
			{Cle: "niveau", Nom: "Niveau", Genre: Nombre},
			{Cle: "apparence", Nom: "Apparence", Genre: Long, Joueur: true},
			{Cle: "personnalite", Nom: "Personnalité", Genre: Long, Joueur: true},
			{Cle: "historique", Nom: "Historique", Genre: Long, Joueur: true},
			{Cle: "liens", Nom: "Liens & attaches", Genre: Long, Joueur: true, Aide: "Famille, amis, dettes, rivaux"},
			{Cle: "objectifs", Nom: "Objectifs", Genre: Long, Joueur: true},
			{Cle: "secrets", Nom: "Ce que le MJ sait", Genre: Long, Secret: true, Groupe: "MJ"},
		},
	},
	{
		Cle: "secret", Nom: "Secret", Pluriel: "Secrets", Icone: "🔒", Carte: []string{"statut", "importance", "campagne"},
		Champs: []Champ{
			{Cle: "resume", Nom: "Le secret en une phrase", Genre: Texte},
			{Cle: "statut", Nom: "Statut", Genre: Choix, Options: []string{"caché", "soupçonné", "en partie révélé", "révélé"}},
			{Cle: "importance", Nom: "Importance", Genre: Choix, Options: []string{"détail", "important", "capital"}},
			{Cle: "categorie", Nom: "Catégorie", Genre: Choix, Options: []string{"identité", "complot", "trahison", "passé", "lieu caché", "trésor", "relation", "prophétie", "autre"}},
			{Cle: "campagne", Nom: "Campagne", Genre: Ref, Cibles: []string{"campagne"}},
			{Cle: "concerne", Nom: "Concerne", Genre: Refs, Cibles: tousTypes},
			{Cle: "verite", Nom: "La vérité", Genre: Long},
			{Cle: "indices", Nom: "Comment le découvrir", Genre: Long, Aide: "Indices, tests, PNJ qui peuvent parler"},
			{Cle: "consequences", Nom: "Si c'est révélé", Genre: Long},
			{Cle: "revelation", Nom: "Révélé aux PJ lors de", Genre: Ref, Cibles: []string{"seance"}},
		},
	},
	{
		// Aide de jeu : lettre, image ou indice que le MJ envoie aux joueurs (relation « recu » d'un PJ
		// vers l'aide). Les joueurs n'en voient que le nom, la forme, le texte remis et les images,
		// dans l'onglet « Documents » de leur page, et seulement si elle a été envoyée à leur PJ.
		Cle: "aide", Nom: "Aide de jeu", Pluriel: "Aides de jeu", Icone: "✉", Carte: []string{"forme", "indice_de", "campagne"},
		Champs: []Champ{
			{Cle: "forme", Nom: "Forme", Genre: Choix, Options: []string{"lettre", "image", "indice", "document", "autre"}},
			{Cle: "campagne", Nom: "Campagne", Genre: Ref, Cibles: []string{"campagne"}},
			{Cle: "contenu", Nom: "Texte remis aux joueurs", Genre: Long, Aide: "Ce que lisent les joueurs, tel quel ; les images de la fiche l'accompagnent"},
			{Cle: "indice_de", Nom: "Indice de", Genre: Ref, Cibles: []string{"secret"}, Secret: true, Groupe: "MJ", Aide: "Le secret dont c'est un indice : « qui sait quoi » note qui l'a reçu"},
			{Cle: "expediteur", Nom: "De la part de", Genre: Ref, Cibles: []string{"pnj", "pj", "faction"}, Secret: true, Groupe: "MJ"},
			{Cle: "trouvee_a", Nom: "Trouvée à", Genre: Ref, Cibles: []string{"lieu"}, Secret: true, Groupe: "MJ"},
			{Cle: "prevue", Nom: "Prévue pour", Genre: Ref, Cibles: []string{"seance"}, Secret: true, Groupe: "MJ"},
			{Cle: "notes", Nom: "Notes du MJ", Genre: Long, Secret: true, Groupe: "MJ"},
		},
	},
	{
		Cle: "regle", Nom: "Règle", Pluriel: "Règles", Icone: "⚖", Carte: []string{"categorie", "domaine", "source"},
		Champs: []Champ{
			{Cle: "resume", Nom: "En une phrase", Genre: Texte},
			{Cle: "categorie", Nom: "Origine", Genre: Choix, Options: []string{"officielle", "maison"}},
			{Cle: "domaine", Nom: "Domaine", Genre: Choix, Options: []string{"tests", "combat", "états", "dommages & soins", "magie", "création", "progression", "équipement", "exploration", "autre"}},
			{Cle: "source", Nom: "Livre", Genre: Texte},
			{Cle: "page", Nom: "Page", Genre: Texte},
			{Cle: "remplace", Nom: "Remplace", Genre: Texte, Aide: "Règle officielle modifiée (règle maison)"},
			{Cle: "contenu", Nom: "Règle", Genre: Long},
		},
	},
}

var parCle = func() map[string]*Type {
	m := map[string]*Type{}
	for i := range Types {
		m[Types[i].Cle] = &Types[i]
	}
	return m
}()

// TypeDe renvoie la description d'un type de fiche.
func TypeDe(cle string) (*Type, bool) {
	t, ok := parCle[cle]
	return t, ok
}

// Champ renvoie un champ d'un type.
func (t *Type) Champ(cle string) (*Champ, bool) {
	for i := range t.Champs {
		if t.Champs[i].Cle == cle {
			return &t.Champs[i], true
		}
	}
	return nil, false
}

// TypeRelation décrit un lien entre deux fiches, lu dans les deux sens.
type TypeRelation struct {
	Cle     string `json:"cle"`
	Nom     string `json:"nom"`     // de A vers B : « A est membre de B »
	Inverse string `json:"inverse"` // de B vers A : « B a pour membre A »
	// De / Vers : types de fiches permis pour A et pour B (vides : tous).
	De   []string `json:"de,omitempty"`
	Vers []string `json:"vers,omitempty"`
	// Reservee : posée par une action de l'application, pas proposée parmi les relations à créer.
	Reservee bool `json:"reservee,omitempty"`
}

// Permise : la relation peut aller d'une fiche de type a vers une fiche de type b.
func (r TypeRelation) Permise(a, b string) bool {
	dans := func(l []string, t string) bool {
		if len(l) == 0 {
			return true
		}
		for _, x := range l {
			if x == t {
				return true
			}
		}
		return false
	}
	return dans(r.De, a) && dans(r.Vers, b)
}

// Relations proposées. « autre » prend son libellé dans la note de la relation.
var Relations = []TypeRelation{
	{Cle: "allie", Nom: "allié de", Inverse: "allié de"},
	{Cle: "ennemi", Nom: "ennemi de", Inverse: "ennemi de"},
	{Cle: "rival", Nom: "rival de", Inverse: "rival de"},
	{Cle: "ami", Nom: "ami de", Inverse: "ami de"},
	{Cle: "famille", Nom: "de la famille de", Inverse: "de la famille de"},
	{Cle: "parent", Nom: "parent de", Inverse: "enfant de"},
	{Cle: "conjoint", Nom: "conjoint de", Inverse: "conjoint de"},
	{Cle: "amour", Nom: "amoureux de", Inverse: "aimé de"},
	{Cle: "mentor", Nom: "mentor de", Inverse: "élève de"},
	{Cle: "membre", Nom: "membre de", Inverse: "a pour membre"},
	{Cle: "dirige", Nom: "dirige", Inverse: "dirigé par"},
	{Cle: "sert", Nom: "travaille pour", Inverse: "emploie"},
	{Cle: "informateur", Nom: "informateur de", Inverse: "a pour informateur"},
	{Cle: "dette", Nom: "a une dette envers", Inverse: "créancier de"},
	{Cle: "connait", Nom: "connaît", Inverse: "connu de"},
	{Cle: "craint", Nom: "craint", Inverse: "craint par"},
	{Cle: "cherche", Nom: "cherche", Inverse: "recherché par"},
	{Cle: "habite", Nom: "habite à", Inverse: "habité par"},
	{Cle: "frequente", Nom: "fréquente", Inverse: "fréquenté par"},
	{Cle: "possede", Nom: "possède", Inverse: "possédé par"},
	{Cle: "garde", Nom: "garde", Inverse: "gardé par"},
	{Cle: "cache", Nom: "cache", Inverse: "caché par"},
	{Cle: "trahit", Nom: "trahit", Inverse: "trahi par"},
	{Cle: "sait", Nom: "sait", Inverse: "est su de"},
	{Cle: "autre", Nom: "lié à", Inverse: "lié à"},
	// envoi d'une aide de jeu à un PJ : posée par l'envoi (POST /api/aides/{id}/envoyer), jamais à la main
	{Cle: "recu", Nom: "a reçu", Inverse: "reçu par", De: []string{"pj"}, Vers: []string{"aide"}, Reservee: true},
}

// RelationDe renvoie un type de relation.
func RelationDe(cle string) (TypeRelation, bool) {
	for _, r := range Relations {
		if r.Cle == cle {
			return r, true
		}
	}
	return TypeRelation{}, false
}
