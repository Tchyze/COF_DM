// Package mcp est le serveur MCP (Model Context Protocol) de COF DM : il laisse un assistant comme
// Claude lire et tenir à jour l'univers. Il tourne à part (cof-dm -mcp), parle JSON-RPC sur son
// entrée et sa sortie standard, et ne touche à l'univers que par l'API REST de l'application, avec
// un jeton : les droits, la validation et le journal sont ceux de tout client de l'API.
package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"sync"
)

// Versions du protocole que ce serveur sait parler, de la plus récente à la plus ancienne.
var versions = []string{"2025-11-25", "2025-06-18", "2025-03-26", "2024-11-05"}

const instructions = `COF DM tient un univers de jeu de rôle (Chroniques Oubliées Fantasy) : fiches de PNJ, lieux, factions, objets, lore, événements, campagnes, quêtes, rencontres, séances, PJ, secrets et règles, reliées entre elles.

- Le monde est unique et persistant : il n'y a pas d'état par campagne (un PNJ mort l'est partout).
- Lire d'abord « schema » pour connaître les types, leurs champs et les valeurs permises.
- Dans un texte long, [[Nom]] crée un lien vers la fiche de ce nom.
- Les champs du groupe « MJ » et les fiches « Secret » sont réservés au MJ : ne pas les recopier dans un texte destiné aux joueurs.
- Chaque écriture est validée (erreur explicite si un champ est refusé) et inscrite au journal de l'univers.
- Avant de créer une fiche, chercher si elle existe déjà (« chercher ») pour ne pas la doubler.`

// requete : un message JSON-RPC reçu (requête ou notification).
type requete struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Methode string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type erreurRPC struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type reponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *erreurRPC      `json:"error,omitempty"`
}

// Serveur : le serveur MCP, au-dessus d'un client de l'API.
type Serveur struct {
	api     *Client
	version string
	outils  []outil
	index   map[string]*outil
	mu      sync.Mutex // une réponse à la fois sur la sortie
	sortie  io.Writer
}

// Nouveau : un serveur MCP qui parle à l'API par ce client.
func Nouveau(api *Client, version string) *Serveur {
	s := &Serveur{api: api, version: version, outils: outils(), index: map[string]*outil{}}
	for i := range s.outils {
		s.index[s.outils[i].Nom] = &s.outils[i]
	}
	return s
}

// Servir lit les messages (un objet JSON par ligne) jusqu'à la fin de l'entrée et répond sur la
// sortie. Rien d'autre ne doit écrire sur cette sortie : les traces vont au journal (stderr).
func (s *Serveur) Servir(entree io.Reader, sortie io.Writer) error {
	s.sortie = sortie
	lecteur := bufio.NewReaderSize(entree, 1<<16)
	for {
		ligne, err := lecteur.ReadBytes('\n')
		if ligne = bytes.TrimSpace(ligne); len(ligne) > 0 {
			s.traiterLigne(ligne)
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func (s *Serveur) traiterLigne(ligne []byte) {
	if ligne[0] == '[' { // lot (versions anciennes du protocole)
		var lot []json.RawMessage
		if json.Unmarshal(ligne, &lot) != nil {
			s.ecrire(reponse{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &erreurRPC{-32700, "JSON illisible"}})
			return
		}
		var reponses []reponse
		for _, m := range lot {
			if r, ok := s.traiter(m); ok {
				reponses = append(reponses, r)
			}
		}
		if len(reponses) > 0 {
			s.ecrire(reponses)
		}
		return
	}
	if r, ok := s.traiter(ligne); ok {
		s.ecrire(r)
	}
}

// traiter répond à un message ; ok est faux pour une notification (sans réponse).
func (s *Serveur) traiter(brut []byte) (reponse, bool) {
	var q requete
	if err := json.Unmarshal(brut, &q); err != nil {
		return reponse{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &erreurRPC{-32700, "JSON illisible"}}, true
	}
	if len(q.ID) == 0 || string(q.ID) == "null" {
		return reponse{}, false // notification (initialized, cancelled…) : rien à répondre
	}
	r := reponse{JSONRPC: "2.0", ID: q.ID}
	res, e := s.methode(q.Methode, q.Params)
	if e != nil {
		r.Error = e
	} else {
		r.Result = res
	}
	return r, true
}

func (s *Serveur) methode(nom string, params json.RawMessage) (any, *erreurRPC) {
	switch nom {
	case "initialize":
		var p struct {
			Version string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(params, &p)
		version := versions[0]
		for _, v := range versions {
			if v == p.Version {
				version = v
			}
		}
		return map[string]any{
			"protocolVersion": version,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "cof-dm", "title": "COF DM", "version": s.version},
			"instructions":    instructions,
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		actives := s.fonctionsActives()
		l := make([]map[string]any, 0, len(s.outils))
		for _, o := range s.outils {
			if o.Fonction == "" || actives == nil || actives[o.Fonction] {
				l = append(l, o.description())
			}
		}
		return map[string]any{"tools": l}, nil
	case "tools/call":
		var p struct {
			Nom  string         `json:"name"`
			Args map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &erreurRPC{-32602, "paramètres illisibles"}
		}
		o := s.index[p.Nom]
		if o == nil {
			return nil, &erreurRPC{-32602, "outil inconnu : " + p.Nom}
		}
		if p.Args == nil {
			p.Args = map[string]any{}
		}
		return s.appeler(o, p.Args), nil
	}
	return nil, &erreurRPC{-32601, "méthode inconnue : " + nom}
}

// tailleMax : au-delà, la réponse d'un outil est coupée (l'assistant affine sa demande).
const tailleMax = 120_000

// appeler exécute un outil. Une erreur de l'API (fiche refusée, introuvable…) est rendue comme
// résultat en erreur, lisible par l'assistant, et non comme une erreur du protocole.
// fonctionsActives : les fonctionnalités activées dans l'univers visé (nil : inconnues, application
// d'une version d'avant ou injoignable ; tous les outils sont alors proposés, l'API refusera).
func (s *Serveur) fonctionsActives() map[string]bool {
	brut, err := s.api.appel("GET", "/api/fonctionnalites", nil, nil)
	if err != nil {
		return nil
	}
	var l []struct {
		Cle    string `json:"cle"`
		Active bool   `json:"active"`
	}
	if json.Unmarshal(brut, &l) != nil {
		return nil
	}
	out := map[string]bool{}
	for _, f := range l {
		out[f.Cle] = f.Active
	}
	return out
}

func (s *Serveur) appeler(o *outil, args map[string]any) map[string]any {
	res, err := o.Faire(s.api, arguments(args))
	if err != nil {
		return map[string]any{"isError": true, "content": []map[string]any{{"type": "text", "text": err.Error()}}}
	}
	var texte string
	switch v := res.(type) {
	case string:
		texte = v
	case json.RawMessage:
		texte = indenter(v)
	default:
		b, _ := json.Marshal(v)
		texte = indenter(b)
	}
	if r := []rune(texte); len(r) > tailleMax {
		texte = string(r[:tailleMax]) + "\n… (réponse coupée : affinez la demande, avec un filtre ou une limite)"
	}
	return map[string]any{"content": []map[string]any{{"type": "text", "text": texte}}}
}

func indenter(b []byte) string {
	var out bytes.Buffer
	if json.Indent(&out, b, "", " ") != nil {
		return string(b)
	}
	return out.String()
}

func (s *Serveur) ecrire(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		log.Printf("mcp : réponse impossible à écrire : %v", err)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := fmt.Fprintf(s.sortie, "%s\n", b); err != nil {
		log.Printf("mcp : sortie fermée : %v", err)
	}
}
