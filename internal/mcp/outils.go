package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"
)

// outil : un outil MCP, traduit en un ou plusieurs appels de l'API (docs/API.md).
type outil struct {
	Nom         string
	Titre       string
	Description string
	Proprietes  map[string]any // propriétés du schéma JSON des arguments
	Requis      []string
	Lecture     bool   // ne modifie rien
	Destructif  bool   // peut effacer ou remplacer quelque chose
	Fonction    string // fonctionnalité optionnelle dont il dépend (internal/modele) ; coupée, il n'est pas proposé
	Faire       func(c *Client, a arguments) (any, error)
}

func (o outil) description() map[string]any {
	schema := map[string]any{"type": "object", "properties": o.Proprietes}
	if o.Proprietes == nil {
		schema["properties"] = map[string]any{}
	}
	if len(o.Requis) > 0 {
		schema["required"] = o.Requis
	}
	return map[string]any{
		"name": o.Nom, "title": o.Titre, "description": o.Description, "inputSchema": schema,
		"annotations": map[string]any{"title": o.Titre, "readOnlyHint": o.Lecture,
			"destructiveHint": o.Destructif, "idempotentHint": o.Lecture, "openWorldHint": false},
	}
}

// Propriétés du schéma des arguments.
func entier(desc string) map[string]any {
	return map[string]any{"type": "integer", "description": desc}
}
func texte(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }
func booleen(desc string) map[string]any {
	return map[string]any{"type": "boolean", "description": desc}
}
func objet(desc string) map[string]any { return map[string]any{"type": "object", "description": desc} }
func entiers(desc string) map[string]any {
	return map[string]any{"type": "array", "items": map[string]any{"type": "integer"}, "description": desc}
}
func textes(desc string) map[string]any {
	return map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": desc}
}
func choix(desc string, valeurs ...string) map[string]any {
	return map[string]any{"type": "string", "enum": valeurs, "description": desc}
}

// arguments : les arguments d'un appel d'outil, tels que le client les envoie.
type arguments map[string]any

func (a arguments) a(cle string) bool { v, ok := a[cle]; return ok && v != nil }

// entier : un nombre entier (un nombre écrit en chaîne est accepté).
func (a arguments) entier(cle string) (int64, bool, error) {
	v, ok := a[cle]
	if !ok || v == nil {
		return 0, false, nil
	}
	switch n := v.(type) {
	case float64:
		if n != math.Trunc(n) {
			return 0, true, fmt.Errorf("%s : un nombre entier est attendu", cle)
		}
		return int64(n), true, nil
	case string:
		i, err := strconv.ParseInt(strings.TrimSpace(n), 10, 64)
		if err != nil {
			return 0, true, fmt.Errorf("%s : un nombre entier est attendu", cle)
		}
		return i, true, nil
	}
	return 0, true, fmt.Errorf("%s : un nombre entier est attendu", cle)
}

func (a arguments) id(cle string) (int64, error) {
	n, ok, err := a.entier(cle)
	if err != nil {
		return 0, err
	}
	if !ok || n <= 0 {
		return 0, fmt.Errorf("%s : identifiant obligatoire", cle)
	}
	return n, nil
}

func (a arguments) texte(cle string) string {
	switch v := a[cle].(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	return ""
}

func (a arguments) booleen(cle string) (bool, bool) {
	switch v := a[cle].(type) {
	case bool:
		return v, true
	case string:
		return v == "true" || v == "1" || v == "oui", true
	}
	return false, false
}

// params : la requête d'une liste, avec les arguments donnés.
func (a arguments) params(cles ...string) url.Values {
	q := url.Values{}
	for _, c := range cles {
		if v, estBool := a[c].(bool); estBool {
			if v {
				q.Set(c, "1")
			}
			continue
		}
		if s := a.texte(c); s != "" {
			q.Set(c, s)
		}
	}
	return q
}

// copier : les clés données des arguments, telles quelles, pour le corps d'une requête.
func (a arguments) copier(cles ...string) map[string]any {
	m := map[string]any{}
	for _, c := range cles {
		if a.a(c) {
			m[c] = a[c]
		}
	}
	return m
}

func chemin(format string, ids ...int64) string {
	v := make([]any, len(ids))
	for i, id := range ids {
		v[i] = id
	}
	return fmt.Sprintf(format, v...)
}

// idFiche : la fiche désignée par « id », sinon par son nom exact (« nom »).
func idFiche(c *Client, a arguments) (int64, error) {
	if a.a("id") {
		return a.id("id")
	}
	nom := a.texte("nom")
	if nom == "" {
		return 0, errors.New("donner l'identifiant de la fiche (id) ou son nom exact (nom)")
	}
	b, err := c.appel("GET", "/api/resoudre", url.Values{"nom": {nom}}, nil)
	if err != nil {
		return 0, err
	}
	var r struct {
		ID int64 `json:"id"`
	}
	if json.Unmarshal(b, &r) != nil || r.ID == 0 {
		return 0, fmt.Errorf("aucune fiche « %s »", nom)
	}
	return r.ID, nil
}

var proprieteFiche = map[string]any{
	"id":  entier("Identifiant de la fiche"),
	"nom": texte("Nom exact de la fiche, à défaut de l'identifiant"),
}

func outils() []outil {
	lire := func(chemin string, cles ...string) func(*Client, arguments) (any, error) {
		return func(c *Client, a arguments) (any, error) { return c.appel("GET", chemin, a.params(cles...), nil) }
	}
	return []outil{
		// Lecture
		{
			Nom: "schema", Titre: "Schéma de l'univers", Lecture: true,
			Description: "Les types de fiches, leurs champs (genre, valeurs permises, types de fiches visés par une référence, champs réservés au MJ), les types de relations et le calendrier. À lire avant de créer ou de modifier une fiche.",
			Faire:       lire("/api/schema"),
		},
		{
			Nom: "statistiques", Titre: "Vue d'ensemble", Lecture: true,
			Description: "L'univers en chiffres : fiches par type, campagnes, séances, quêtes en cours, liens morts, dernières modifications.",
			Faire:       lire("/api/stats"),
		},
		{
			Nom: "chercher", Titre: "Chercher des fiches", Lecture: true,
			Description: "Liste des fiches (résumés), filtrée. q : recherche plein texte sans accents, tolérante aux fautes, \"phrase exacte\", #tag.",
			Proprietes: map[string]any{
				"q":        texte("Texte cherché dans les noms et tous les textes"),
				"type":     texte("Type de fiche : pnj, lieu, faction, objet, lore, evenement, campagne, quete, rencontre, seance, pj, secret, regle"),
				"tag":      texte("Tag"),
				"campagne": entier("Identifiant d'une campagne : ses fiches seulement"),
				"archives": booleen("Inclure les fiches archivées"),
				"limite":   entier("Nombre maximum de résultats (50 par défaut)"),
			},
			Faire: func(c *Client, a arguments) (any, error) {
				q := a.params("q", "type", "tag", "campagne", "archives", "limite")
				if q.Get("limite") == "" {
					q.Set("limite", "50")
				}
				return c.appel("GET", "/api/entites", q, nil)
			},
		},
		{
			Nom: "lire_fiche", Titre: "Lire une fiche", Lecture: true,
			Description: "Une fiche complète : ses champs, ses relations, ce qui la cite, les fiches qu'elle mentionne, ses images, ses dates en toutes lettres.",
			Proprietes:  proprieteFiche,
			Faire: func(c *Client, a arguments) (any, error) {
				id, err := idFiche(c, a)
				if err != nil {
					return nil, err
				}
				return c.appel("GET", chemin("/api/entites/%d", id), nil, nil)
			},
		},
		{
			Nom: "graphe", Fonction: "graphe", Titre: "Fiches liées", Lecture: true,
			Description: "Les fiches liées à une fiche (relations, champs de référence, mentions), jusqu'à 3 niveaux : nœuds et arêtes.",
			Proprietes: map[string]any{
				"id":         entier("Fiche de départ"),
				"profondeur": entier("1 à 3 (2 par défaut)"),
			},
			Requis: []string{"id"},
			Faire:  lire("/api/graphe", "id", "profondeur"),
		},
		{
			Nom: "chronologie", Fonction: "chronologie", Titre: "Chronologie", Lecture: true,
			Description: "Toutes les dates en jeu, dans l'ordre. Avec une campagne : la campagne et l'histoire du monde.",
			Proprietes:  map[string]any{"campagne": entier("Identifiant d'une campagne")},
			Faire:       lire("/api/chronologie", "campagne"),
		},
		{
			Nom: "qui_sait_quoi", Fonction: "secrets", Titre: "Qui sait quoi", Lecture: true,
			Description: "Les secrets, et qui (PJ, PNJ, faction) sait chacun : séance où il l'a appris et comment ; les indices (aides de jeu envoyées) que chaque PJ a reçus.",
			Proprietes:  map[string]any{"campagne": entier("Identifiant d'une campagne")},
			Faire:       lire("/api/secrets", "campagne"),
		},
		{
			Nom: "organigramme", Fonction: "organigramme", Titre: "Organigramme d'une faction", Lecture: true,
			Description: "L'arbre d'une faction : dirigeants, membres et leurs rangs, qui répond à qui, appartenances secrètes.",
			Proprietes:  map[string]any{"id": entier("Identifiant de la faction")},
			Requis:      []string{"id"},
			Faire: func(c *Client, a arguments) (any, error) {
				id, err := a.id("id")
				if err != nil {
					return nil, err
				}
				return c.appel("GET", chemin("/api/entites/%d/organigramme", id), nil, nil)
			},
		},
		{
			Nom: "partie_en_cours", Fonction: "ecran_mj", Titre: "Partie en cours", Lecture: true,
			Description: "Ce que montre l'écran du MJ : quêtes en cours, la quête choisie, sa campagne, la prochaine séance, les PJ et leur santé, la scène (fiches liées à la quête), les rencontres.",
			Proprietes:  map[string]any{"quete": entier("Identifiant d'une quête en cours (la dernière modifiée par défaut)")},
			Faire:       lire("/api/table", "quete"),
		},
		{
			Nom: "preparer_seance", Fonction: "seances", Titre: "Préparer une séance", Lecture: true,
			Description: "Ce que la campagne d'une séance a en cours : séance précédente et son résumé, où est le groupe, PJ, fils rouges, quêtes en cours et étapes restantes, secrets non révélés avec leurs indices et les PJ qui les ignorent, PNJ probables, rencontres prêtes. Pour écrire la préparation, modifier le champ « preparation » de la séance (réservé au MJ).",
			Proprietes:  proprieteFiche,
			Faire: func(c *Client, a arguments) (any, error) {
				id, err := idFiche(c, a)
				if err != nil {
					return nil, err
				}
				return c.appel("GET", chemin("/api/entites/%d/preparation", id), nil, nil)
			},
		},
		{
			Nom: "feuille_personnage", Fonction: "feuilles", Titre: "Feuille de personnage", Lecture: true,
			Description: "La feuille de personnage d'un PJ : caractéristiques, PV, DEF, ressources, attaques, voies, équipement, bourse, notes du MJ.",
			Proprietes:  proprieteFiche,
			Faire: func(c *Client, a arguments) (any, error) {
				id, err := idFiche(c, a)
				if err != nil {
					return nil, err
				}
				return c.appel("GET", chemin("/api/entites/%d/feuille", id), nil, nil)
			},
		},
		{
			Nom: "bestiaire", Titre: "Bestiaire", Lecture: true,
			Description: "Les monstres du bestiaire de l'univers, en résumé (id, nom, NC, catégorie, taille, DEF, PV, source). Lire un monstre en entier avec « monstre ».",
			Proprietes: map[string]any{
				"q":      texte("Texte cherché dans le nom, la catégorie et les tags"),
				"nc_min": entier("NC minimum"),
				"nc_max": entier("NC maximum"),
			},
			Faire: bestiaire,
		},
		{
			Nom: "monstre", Titre: "Lire un monstre", Lecture: true,
			Description: "Un monstre du bestiaire en entier : caractéristiques, DEF, PV, attaques, capacités, description.",
			Proprietes:  map[string]any{"id": entier("Identifiant du monstre")},
			Requis:      []string{"id"},
			Faire: func(c *Client, a arguments) (any, error) {
				id, err := a.id("id")
				if err != nil {
					return nil, err
				}
				return c.appel("GET", chemin("/api/bestiaire/%d", id), nil, nil)
			},
		},
		{
			Nom: "difficulte_rencontre", Fonction: "rencontres", Titre: "Difficulté d'une rencontre", Lecture: true,
			Description: "Les créatures d'une rencontre et sa difficulté face aux PJ de sa campagne (selon le fichier de règles de l'univers).",
			Proprietes: map[string]any{
				"id":        entier("Identifiant de la rencontre"),
				"condition": texte("Clé d'une condition des règles (terrain, obscurité…)"),
				"nb_pj":     entier("Évaluer face à ce nombre de PJ"),
				"niveau_pj": entier("Évaluer face à ce niveau moyen"),
			},
			Requis: []string{"id"},
			Faire: func(c *Client, a arguments) (any, error) {
				id, err := a.id("id")
				if err != nil {
					return nil, err
				}
				return c.appel("GET", chemin("/api/entites/%d/difficulte", id), a.params("condition", "nb_pj", "niveau_pj"), nil)
			},
		},
		{
			Nom: "combats", Fonction: "combat", Titre: "Combats", Lecture: true,
			Description: "Les combats de l'univers (en cours d'abord) : round, PJ, ennemis, issue, rencontre, séance.",
			Proprietes: map[string]any{
				"seance":   entier("Ceux d'une séance"),
				"en_cours": booleen("Seulement ceux qui ne sont pas terminés"),
			},
			Faire: lire("/api/combats", "seance", "en_cours"),
		},
		{
			Nom: "combat", Fonction: "combat", Titre: "Lire un combat", Lecture: true,
			Description: "Un combat en entier : ordre d'initiative, combattants, journal, récapitulatif s'il est terminé.",
			Proprietes:  map[string]any{"id": entier("Identifiant du combat")},
			Requis:      []string{"id"},
			Faire: func(c *Client, a arguments) (any, error) {
				id, err := a.id("id")
				if err != nil {
					return nil, err
				}
				return c.appel("GET", chemin("/api/combats/%d", id), nil, nil)
			},
		},
		{
			Nom: "journal", Titre: "Journal des modifications", Lecture: true,
			Description: "Les dernières modifications de l'univers, avec leur auteur.",
			Proprietes:  map[string]any{"limite": entier("Nombre de lignes (100 par défaut)")},
			Faire:       lire("/api/journal", "limite"),
		},
		{
			Nom: "jets", Fonction: "des", Titre: "Jets de dés de la table", Lecture: true,
			Description: "Les derniers jets de dés de la partie (MJ et joueurs, jets cachés du MJ compris), gardés tant que l'application tourne.",
			Proprietes:  map[string]any{"depuis": entier("Seulement les jets après cet identifiant")},
			Faire:       lire("/api/des", "depuis"),
		},
		{
			Nom: "liens_morts", Titre: "Liens morts", Lecture: true,
			Description: "Les [[Nom]] qui ne mènent à aucune fiche, et les fiches où ils sont.",
			Faire:       lire("/api/liens-morts"),
		},

		// Écriture
		{
			Nom: "creer_fiche", Titre: "Créer une fiche",
			Description: "Crée une fiche. data : les champs du type (voir « schema ») ; une référence se donne par l'identifiant de la fiche visée ; dans un texte long, [[Nom]] crée un lien. Rien n'est créé si un champ est refusé : l'erreur dit lequel et pourquoi.",
			Proprietes: map[string]any{
				"type": texte("Type de fiche (voir « schema »)"),
				"nom":  texte("Nom de la fiche"),
				"data": objet("Champs du type"),
				"tags": textes("Tags"),
			},
			Requis: []string{"type", "nom"},
			Faire: func(c *Client, a arguments) (any, error) {
				return c.appel("POST", "/api/entites", nil, a.copier("type", "nom", "data", "tags"))
			},
		},
		{
			Nom: "modifier_fiche", Titre: "Modifier une fiche",
			Description: "Modifie une fiche en partie : seuls les champs donnés dans data changent ; un champ à null est effacé. Renommer une fiche réécrit les [[Ancien nom]] des autres fiches. tags remplace tous les tags. modifie (facultatif) : la valeur lue, pour refuser si la fiche a changé entre-temps.",
			Proprietes: map[string]any{
				"id":      entier("Identifiant de la fiche"),
				"nom":     texte("Nouveau nom"),
				"data":    objet("Champs à changer"),
				"tags":    textes("Tous les tags de la fiche"),
				"modifie": map[string]any{"type": "number", "description": "Champ « modifie » de la fiche lue"},
			},
			Requis: []string{"id"},
			Faire: func(c *Client, a arguments) (any, error) {
				id, err := a.id("id")
				if err != nil {
					return nil, err
				}
				corps := a.copier("nom", "data", "tags", "modifie")
				if len(corps) == 0 {
					return nil, errors.New("rien à modifier : donner nom, data ou tags")
				}
				return c.appel("PATCH", chemin("/api/entites/%d", id), nil, corps)
			},
		},
		{
			Nom: "archiver_fiche", Titre: "Archiver une fiche", Destructif: true,
			Description: "Archive une fiche (masquée, récupérable) ou la restaure (archive: false). Une fiche ne s'efface pas d'ici : l'effacement définitif reste au MJ, dans l'application.",
			Proprietes: map[string]any{
				"id":      entier("Identifiant de la fiche"),
				"archive": booleen("true : archiver (par défaut) ; false : restaurer"),
			},
			Requis: []string{"id"},
			Faire: func(c *Client, a arguments) (any, error) {
				id, err := a.id("id")
				if err != nil {
					return nil, err
				}
				archive, ok := a.booleen("archive")
				if !ok {
					archive = true
				}
				return c.appel("POST", chemin("/api/entites/%d/archive", id), nil, map[string]any{"archive": archive})
			},
		},
		{
			Nom: "partager_fiche", Titre: "Partager avec les joueurs",
			Description: "Rend une fiche visible des joueurs (sans ses champs réservés au MJ), ou la leur retire (partage: false). Une fiche « Secret » ne se partage pas.",
			Proprietes: map[string]any{
				"id":      entier("Identifiant de la fiche"),
				"partage": booleen("true : partager (par défaut) ; false : retirer"),
			},
			Requis: []string{"id"},
			Faire: func(c *Client, a arguments) (any, error) {
				id, err := a.id("id")
				if err != nil {
					return nil, err
				}
				partage, ok := a.booleen("partage")
				if !ok {
					partage = true
				}
				return c.appel("POST", chemin("/api/entites/%d/partage", id), nil, map[string]any{"partage": partage})
			},
		},
		{
			Nom: "relier", Titre: "Créer une relation",
			Description: "Crée une relation de la fiche a vers la fiche b (types dans « schema » : membre, allie, ennemi, famille…). rang et superieur servent aux membres d'une faction ; secrete : connue du MJ seulement ; contexte : une fiche qui situe la relation (pour « sait » : la séance) ; note : précision, ou libellé pour le type « autre ».",
			Proprietes: map[string]any{
				"a":         entier("Fiche de départ"),
				"b":         entier("Fiche d'arrivée"),
				"type":      texte("Type de relation"),
				"rang":      texte("Rang dans la faction"),
				"superieur": entier("Supérieur dans la faction (fiche)"),
				"secrete":   booleen("Connue du MJ seulement"),
				"contexte":  entier("Fiche qui situe la relation"),
				"note":      texte("Note"),
			},
			Requis: []string{"a", "b", "type"},
			Faire: func(c *Client, a arguments) (any, error) {
				return c.appel("POST", "/api/relations", nil, a.copier("a", "b", "type", "rang", "superieur", "secrete", "contexte", "note"))
			},
		},
		{
			Nom: "modifier_relation", Titre: "Modifier une relation",
			Description: "Modifie une relation (identifiant lu dans les relations d'une fiche). superieur ou contexte à 0 les retire.",
			Proprietes: map[string]any{
				"id":        entier("Identifiant de la relation"),
				"type":      texte("Type de relation"),
				"rang":      texte("Rang"),
				"superieur": entier("Supérieur (0 : aucun)"),
				"secrete":   booleen("Connue du MJ seulement"),
				"contexte":  entier("Fiche qui situe la relation (0 : aucune)"),
				"note":      texte("Note"),
			},
			Requis: []string{"id"},
			Faire: func(c *Client, a arguments) (any, error) {
				id, err := a.id("id")
				if err != nil {
					return nil, err
				}
				corps := a.copier("type", "rang", "superieur", "secrete", "contexte", "note")
				if len(corps) == 0 {
					return nil, errors.New("rien à modifier")
				}
				return c.appel("PATCH", chemin("/api/relations/%d", id), nil, corps)
			},
		},
		{
			Nom: "supprimer_relation", Titre: "Supprimer une relation", Destructif: true,
			Description: "Supprime une relation. Pour « ne sait plus un secret », supprimer la relation « sait ». Les subordonnés d'un membre retiré passent sous son propre supérieur.",
			Proprietes:  map[string]any{"id": entier("Identifiant de la relation")},
			Requis:      []string{"id"},
			Faire: func(c *Client, a arguments) (any, error) {
				id, err := a.id("id")
				if err != nil {
					return nil, err
				}
				return c.appel("DELETE", chemin("/api/relations/%d", id), nil, nil)
			},
		},
		{
			Nom: "reveler_secret", Fonction: "secrets", Titre: "Révéler un secret",
			Description: "Ces fiches (PJ, PNJ, factions) apprennent le secret. Le statut du secret passe à « révélé » quand tous les PJ de la campagne le savent, « en partie révélé » sinon.",
			Proprietes: map[string]any{
				"id":     entier("Identifiant du secret"),
				"qui":    entiers("Fiches qui l'apprennent"),
				"seance": entier("Séance de la révélation"),
				"note":   texte("Comment ils l'ont appris"),
			},
			Requis: []string{"id", "qui"},
			Faire: func(c *Client, a arguments) (any, error) {
				id, err := a.id("id")
				if err != nil {
					return nil, err
				}
				return c.appel("POST", chemin("/api/secrets/%d/reveler", id), nil, a.copier("qui", "seance", "note"))
			},
		},
		{
			Nom: "aides_de_jeu", Fonction: "aides", Titre: "Aides de jeu", Lecture: true,
			Description: "Les aides de jeu (lettres, images, indices à remettre aux joueurs), avec le secret dont chacune est l'indice, la séance pour laquelle elle est prévue et les PJ qui l'ont déjà reçue. Le texte se lit avec lire_fiche.",
			Proprietes:  map[string]any{"campagne": entier("Identifiant d'une campagne (ses aides et celles sans campagne)")},
			Faire:       lire("/api/aides", "campagne"),
		},
		{
			Nom: "envoyer_aide", Fonction: "aides", Titre: "Envoyer une aide de jeu",
			Description: "Envoie une aide de jeu à des PJ (ou à tous ceux de sa campagne) : elle arrive aussitôt sur leur page des joueurs, onglet Documents. Un PJ qui l'a déjà reçue est passé. Si l'aide est l'indice d'un secret « caché », il passe à « soupçonné ». À ne faire que si le MJ le demande.",
			Proprietes: map[string]any{
				"id":     entier("Identifiant de l'aide de jeu"),
				"qui":    entiers("PJ qui la reçoivent"),
				"tous":   booleen("Tous les PJ de la campagne de l'aide (au lieu de qui)"),
				"seance": entier("Séance de l'envoi"),
			},
			Requis: []string{"id"},
			Faire: func(c *Client, a arguments) (any, error) {
				id, err := a.id("id")
				if err != nil {
					return nil, err
				}
				return c.appel("POST", chemin("/api/aides/%d/envoyer", id), nil, a.copier("qui", "tous", "seance"))
			},
		},
		{
			Nom: "lancer_des", Fonction: "des", Titre: "Lancer des dés",
			Description: "Lance des dés pour le MJ : formule comme 1d20+3, 2d6-1, 2d20kh1 (garder le meilleur de deux), 2d20kl1 (le pire). Le jet s'affiche sur la page des joueurs, sauf s'il est caché.",
			Proprietes: map[string]any{
				"formule": texte("Formule de dés"),
				"libelle": texte("Pour quoi (facultatif)"),
				"cache":   booleen("Jet caché : vu du MJ seulement (true par défaut)"),
			},
			Requis: []string{"formule"},
			Faire: func(c *Client, a arguments) (any, error) {
				cache, ok := a.booleen("cache")
				if !ok {
					cache = true // un assistant lance pour le MJ : rien ne s'affiche à la table sans le demander
				}
				return c.appel("POST", "/api/des", nil, map[string]any{"formule": a.texte("formule"), "libelle": a.texte("libelle"), "cache": cache})
			},
		},
		{
			Nom: "clore_seance", Fonction: "seances", Titre: "Clore une séance",
			Description: "Reporte en une fois ce qui s'est passé pendant une séance : la séance passe « jouée », avec son résumé, ses PJ présents et sa date en jeu (qui devient aussi celle de la campagne) ; le lieu du groupe ; les étapes faites (une valeur par étape, dans l'ordre de la fiche) et le statut des quêtes ; les secrets appris (qui) ; les PNJ morts ; les rencontres jouées ou évitées ; et, si demandé, la séance suivante est créée (prévue). Tout est contrôlé avant d'écrire : une valeur refusée n'en laisse passer aucune.",
			Proprietes: map[string]any{
				"id":          entier("Identifiant de la séance"),
				"resume":      texte("Résumé de la séance"),
				"presents":    entiers("PJ présents"),
				"date_jeu":    objet("Date en jeu à la fin : {\"an\", \"mois\", \"jour\"}"),
				"lieu_groupe": entier("Lieu où se trouve le groupe"),
				"quetes":      map[string]any{"type": "array", "description": "[{\"id\", \"etapes\": [true, false…], \"statut\"}]", "items": map[string]any{"type": "object"}},
				"secrets":     map[string]any{"type": "array", "description": "[{\"id\", \"qui\": [ids], \"note\"}]", "items": map[string]any{"type": "object"}},
				"morts":       entiers("PNJ morts"),
				"rencontres":  map[string]any{"type": "array", "description": "[{\"id\", \"statut\": \"jouée\" ou \"évitée\"}]", "items": map[string]any{"type": "object"}},
				"suivante":    booleen("Créer la séance suivante"),
			},
			Requis: []string{"id"},
			Faire: func(c *Client, a arguments) (any, error) {
				id, err := a.id("id")
				if err != nil {
					return nil, err
				}
				return c.appel("POST", chemin("/api/entites/%d/apres-seance", id), nil,
					a.copier("resume", "presents", "date_jeu", "lieu_groupe", "quetes", "secrets", "morts", "rencontres", "suivante"))
			},
		},
		{
			Nom: "modifier_feuille", Fonction: "feuilles", Titre: "Modifier une feuille de personnage",
			Description: "Modifie la feuille de personnage d'un PJ : seuls les champs donnés changent (pv_actuel, pc, pr, pm, equipement, bourse, attaques, voies…). Une liste (attaques, voies) se donne en entier. Le niveau se change sur la fiche du PJ, pas ici. Les valeurs dérivées (modificateurs, DEF, initiative) sont recalculées.",
			Proprietes: map[string]any{
				"id":     entier("Identifiant du PJ"),
				"champs": objet("Champs de la feuille à changer"),
			},
			Requis: []string{"id", "champs"},
			Faire:  modifierFeuille,
		},
	}
}

// bestiaire : les monstres en résumé, filtrés ici (l'API les rend tous, en entier).
func bestiaire(c *Client, a arguments) (any, error) {
	b, err := c.appel("GET", "/api/bestiaire", nil, nil)
	if err != nil {
		return nil, err
	}
	var monstres []map[string]any
	if err := json.Unmarshal(b, &monstres); err != nil {
		return nil, errors.New("bestiaire illisible")
	}
	q := sansAccents(a.texte("q"))
	ncMin, avecMin, err := a.entier("nc_min")
	if err != nil {
		return nil, err
	}
	ncMax, avecMax, err := a.entier("nc_max")
	if err != nil {
		return nil, err
	}
	out := []map[string]any{}
	for _, m := range monstres {
		nc, _ := m["nc"].(float64)
		if (avecMin && nc < float64(ncMin)) || (avecMax && nc > float64(ncMax)) {
			continue
		}
		if q != "" {
			tags, _ := json.Marshal(m["tags"])
			foin := sansAccents(fmt.Sprint(m["nom"], " ", m["categorie"], " ", string(tags)))
			if !strings.Contains(foin, q) {
				continue
			}
		}
		r := map[string]any{}
		for _, k := range []string{"id", "nom", "nc", "categorie", "taille", "defense", "pv", "source"} {
			if v, ok := m[k]; ok {
				r[k] = v
			}
		}
		out = append(out, r)
	}
	return out, nil
}

var accents = strings.NewReplacer("à", "a", "â", "a", "ä", "a", "é", "e", "è", "e", "ê", "e", "ë", "e",
	"î", "i", "ï", "i", "ô", "o", "ö", "o", "ù", "u", "û", "u", "ü", "u", "ç", "c", "œ", "oe", "æ", "ae")

func sansAccents(s string) string { return accents.Replace(strings.ToLower(strings.TrimSpace(s))) }

// modifierFeuille lit la feuille, y reporte les champs donnés et l'enregistre, en refusant si elle
// a changé entre la lecture et l'écriture (combat en cours, joueur qui tient sa feuille).
func modifierFeuille(c *Client, a arguments) (any, error) {
	id, err := a.id("id")
	if err != nil {
		return nil, err
	}
	champs, ok := a["champs"].(map[string]any)
	if !ok || len(champs) == 0 {
		return nil, errors.New("champs : les champs de la feuille à changer")
	}
	b, err := c.appel("GET", chemin("/api/entites/%d/feuille", id), nil, nil)
	if err != nil {
		return nil, err
	}
	var f map[string]any
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, errors.New("feuille illisible")
	}
	for _, k := range []string{"entite", "niveau", "existe", "regles"} {
		delete(f, k)
	}
	for k, v := range champs {
		switch k {
		case "entite", "niveau", "existe", "regles", "modifie":
			return nil, fmt.Errorf("%s : ne se change pas par la feuille", k)
		}
		f[k] = v
	}
	return c.appel("PUT", chemin("/api/entites/%d/feuille", id), nil, f)
}
