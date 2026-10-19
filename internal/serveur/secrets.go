package serveur

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Tchyze/cof_dm/internal/modele"
	"github.com/Tchyze/cof_dm/internal/store"
)

// secrets : tableau « qui sait quoi » (secrets × PJ, et les PNJ ou factions qui savent).
func (s *Serveur) secrets(w http.ResponseWriter, r *http.Request) {
	sec, pj, pnj, conn, err := s.store.QuiSaitQuoi(entierParam(r, "campagne"))
	if err != nil {
		erreurStore(w, err)
		return
	}
	indices, err := s.indicesRecus()
	if err != nil {
		erreurStore(w, err)
		return
	}
	repondre(w, 200, map[string]any{"secrets": sec, "pj": pj, "pnj": pnj, "connaissances": conn, "indices": indices})
}

// reveler : des PJ apprennent un secret (lors d'une séance). Le statut du secret suit : « révélé »
// quand tous les PJ de sa campagne le savent, « en partie révélé » sinon.
func (s *Serveur) reveler(w http.ResponseWriter, r *http.Request) {
	id, err := idDe(r)
	if err != nil {
		erreurStore(w, err)
		return
	}
	var in struct {
		Qui    []int64 `json:"qui"`
		Seance int64   `json:"seance"`
		Note   string  `json:"note"`
	}
	if err := lireJSON(r, &in); err != nil {
		erreurStore(w, err)
		return
	}
	ajoutes, statut, err := s.revelerSecret(id, in.Qui, in.Seance, in.Note, auteurDe(r))
	if err != nil {
		erreurStore(w, err)
		return
	}
	repondre(w, 200, map[string]any{"ajoutes": ajoutes, "statut": statut})
}

// revelerSecret : ces fiches apprennent le secret (relation « sait », avec la séance) ; le statut du
// secret suit (révélé quand tous les PJ de sa campagne savent, en partie révélé sinon).
func (s *Serveur) revelerSecret(id int64, qui []int64, seance int64, note, auteur string) (int, any, error) {
	sec, err := s.store.Lire(id)
	if err != nil {
		return 0, nil, err
	}
	if sec.Type != "secret" {
		return 0, nil, store.ErrValidation{Msg: "Cette fiche n'est pas un secret."}
	}
	if len(qui) == 0 {
		return 0, nil, store.ErrValidation{Msg: "Indiquez qui apprend le secret."}
	}
	deja := map[int64]bool{}
	rels, _ := s.store.Relations(id)
	for _, rel := range rels {
		if rel.Type == "sait" {
			deja[rel.Autre.ID] = true
		}
	}
	ajoutes := 0
	for _, q := range qui {
		if deja[q] {
			continue
		}
		d := store.DetailRelation{Note: &note}
		if seance > 0 {
			d.Contexte = &seance
		}
		if _, err := s.store.Relier(q, id, "sait", d, auteur); err != nil {
			return ajoutes, nil, err
		}
		deja[q] = true
		ajoutes++
	}
	// statut : tous les PJ (de la campagne du secret) savent-ils ?
	filtre := store.Filtre{Type: "pj"}
	if c := idsDe(sec.Data["campagne"]); len(c) > 0 {
		filtre.Campagne = c[0]
	}
	pjs, _ := s.store.Lister(filtre)
	tous, un := len(pjs) > 0, false
	for _, p := range pjs {
		if deja[p.ID] {
			un = true
		} else {
			tous = false
		}
	}
	maj := map[string]any{}
	switch {
	case tous:
		maj["statut"] = "révélé"
	case un && sec.Data["statut"] != "révélé":
		maj["statut"] = "en partie révélé"
	}
	if seance > 0 && un && sec.Data["revelation"] == nil {
		if t, ok := s.store.TypeDeFiche(seance); ok && t == "seance" {
			maj["revelation"] = seance
		}
	}
	if len(maj) > 0 {
		if _, err := s.store.Modifier(id, store.Maj{Data: maj}, auteur); err != nil {
			return ajoutes, nil, err
		}
	}
	return ajoutes, maj["statut"], nil
}

// importerFiches crée des fiches depuis un fichier JSON : {"fiches": [{"type","nom","data","tags"}]}
// (ou directement la liste). Une fiche dont le nom existe déjà pour ce type est ignorée.
func (s *Serveur) importerFiches(w http.ResponseWriter, r *http.Request) {
	b, err := io.ReadAll(io.LimitReader(r.Body, 64<<20))
	if err != nil {
		erreur(w, 400, "Requête illisible.")
		return
	}
	type nouvelle struct {
		Type string         `json:"type"`
		Nom  string         `json:"nom"`
		Data map[string]any `json:"data"`
		Tags []string       `json:"tags"`
	}
	var liste []nouvelle
	if err := json.Unmarshal(b, &liste); err != nil {
		var obj struct {
			Fiches []nouvelle `json:"fiches"`
		}
		if err := json.Unmarshal(b, &obj); err != nil || obj.Fiches == nil {
			erreur(w, 422, "Fichier illisible : attendu {\"fiches\": [{\"type\", \"nom\", \"data\", \"tags\"}]}.")
			return
		}
		liste = obj.Fiches
	}
	existants := map[string]bool{}
	tout, _ := s.store.Toutes()
	for _, e := range tout {
		existants[e.Type+"\x00"+strings.ToLower(e.Nom)] = true
	}
	var crees, ignores int
	var erreurs []string
	for i, f := range liste {
		if _, ok := modele.TypeDe(f.Type); !ok {
			erreurs = append(erreurs, fmt.Sprintf("n°%d : type inconnu %q", i+1, f.Type))
			continue
		}
		k := f.Type + "\x00" + strings.ToLower(strings.Join(strings.Fields(f.Nom), " "))
		if existants[k] {
			ignores++
			continue
		}
		if _, err := s.store.Creer(f.Type, f.Nom, f.Data, f.Tags, auteurDe(r)); err != nil {
			erreurs = append(erreurs, fmt.Sprintf("%s : %v", f.Nom, err))
			continue
		}
		existants[k] = true
		crees++
	}
	if erreurs == nil {
		erreurs = []string{}
	}
	repondre(w, 200, map[string]any{"crees": crees, "ignores": ignores, "erreurs": erreurs})
}
