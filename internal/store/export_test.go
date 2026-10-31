package store

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
)

// copieExport : un export modifiable sans toucher à l'original.
func copieExport(t *testing.T, x Export) Export {
	t.Helper()
	b, err := json.Marshal(x)
	if err != nil {
		t.Fatal(err)
	}
	var c Export
	if err := json.Unmarshal(b, &c); err != nil {
		t.Fatal(err)
	}
	return c
}

// Un export restauré est validé comme une saisie : au moindre refus, rien n'est écrit.
func TestRestaurationValidee(t *testing.T) {
	s := ouvrir(t)
	camp := creer(t, s, "campagne", "Les Moissons", nil)
	seance := creer(t, s, "seance", "Séance 1", map[string]any{"campagne": float64(camp.ID)})
	fac := creer(t, s, "faction", "La Guilde", nil)
	chef := creer(t, s, "pnj", "Aldric", nil)
	second := creer(t, s, "pnj", "Brune", nil)
	pj := creer(t, s, "pj", "Céleste", nil)
	if _, err := s.Relier(chef.ID, fac.ID, "membre", DetailRelation{}, "t"); err != nil {
		t.Fatal(err)
	}
	sup := chef.ID
	if _, err := s.Relier(second.ID, fac.ID, "membre", DetailRelation{Superieur: &sup, Contexte: &seance.ID}, "t"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.EcrireFeuille(pj.ID, []byte(`{"pv":7}`), 0, "t"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AjouterMedia(chef.ID, "0123456789abcdef0123.png", "aldric.png", []byte("\x89PNG"), "t"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreerDoc("pots", []byte(`{"campagne":`+strconv.FormatInt(camp.ID, 10)+`,"pieces":{"po":4},"objets":[{"id":1,"nom":"Corde","quantite":2}]}`), "Pot", "t"); err != nil {
		t.Fatal(err)
	}
	x, err := s.Exporter()
	if err != nil {
		t.Fatal(err)
	}

	// un export valide se restaure, même quand une fiche en désigne une autre placée après elle
	bon := copieExport(t, x)
	for i, j := 0, len(bon.Entites)-1; i < j; i, j = i+1, j-1 {
		bon.Entites[i], bon.Entites[j] = bon.Entites[j], bon.Entites[i]
	}
	s2 := ouvrir(t)
	if err := s2.Restaurer(bon, "t"); err != nil {
		t.Fatalf("export valide refusé : %v", err)
	}
	if e, _ := s2.Lire(seance.ID); e.Data["campagne"] != float64(camp.ID) {
		t.Fatalf("référence perdue : %#v", e.Data)
	}
	if b, _, err := s2.LireFeuille(pj.ID); err != nil || !strings.Contains(string(b), `"pv":7`) {
		t.Fatalf("feuille après restauration : %v %s", err, b)
	}
	if pots, _ := s2.ListerDocs("pots"); len(pots) != 1 || !strings.Contains(string(pots[0].Data), `"Corde"`) {
		t.Fatalf("pot commun après restauration : %v", pots)
	}

	fiche := func(x *Export, id int64) *Entite {
		for i := range x.Entites {
			if x.Entites[i].ID == id {
				return &x.Entites[i]
			}
		}
		t.Fatalf("fiche n°%d absente de l'export", id)
		return nil
	}
	cas := []struct {
		nom     string
		abimer  func(x *Export)
		attendu string
	}{
		{"version plus récente", func(x *Export) { x.Donnees = "999" }, "plus récente"},
		{"identifiant en double", func(x *Export) { x.Entites = append(x.Entites, x.Entites[0]) }, "en double"},
		{"nom interdit", func(x *Export) { fiche(x, chef.ID).Nom = "[[Aldric]]" }, "Fiche n°"},
		{"nom vide", func(x *Export) { fiche(x, chef.ID).Nom = "  " }, "obligatoire"},
		{"champ inconnu", func(x *Export) { fiche(x, chef.ID).Data["pirate"] = "oui" }, "pirate"},
		{"référence vers une fiche absente", func(x *Export) { fiche(x, seance.ID).Data["campagne"] = 9999.0 }, "9999"},
		{"référence vers un mauvais type", func(x *Export) { fiche(x, seance.ID).Data["campagne"] = float64(fac.ID) }, "campagne"},
		{"relation vers une fiche absente", func(x *Export) { x.Relations[0].B = 9999 }, "absente"},
		{"relation sur elle-même", func(x *Export) { x.Relations[0].B = x.Relations[0].A }, "elle-même"},
		{"type de relation inconnu", func(x *Export) { x.Relations[0].Type = "pirate" }, "pirate"},
		{"supérieur hors de la faction", func(x *Export) { x.Relations[1].Superieur = pj.ID }, "supérieur"},
		{"boucle dans l'organigramme", func(x *Export) { x.Relations[0].Superieur = second.ID }, "boucle"},
		{"contexte absent", func(x *Export) { x.Relations[1].Contexte = 9999 }, "Contexte"},
		{"pot commun aux pièces négatives", func(x *Export) {
			for i := range x.Documents {
				if x.Documents[i].Coll == "pots" {
					x.Documents[i].Data = json.RawMessage(`{"campagne":1,"pieces":{"po":-3}}`)
				}
			}
		}, "négatif"},
		{"image d'une fiche absente", func(x *Export) { x.Medias[0].Entite = 9999 }, "absente"},
		{"nom d'image refusé", func(x *Export) { x.Medias[0].Fichier = "../../secret.png" }, "nom de fichier"},
		{"feuille invalide", func(x *Export) { x.Feuilles[0].Data = json.RawMessage(`{"pv":0}`) }, "Feuille"},
	}
	for _, c := range cas {
		t.Run(c.nom, func(t *testing.T) {
			y := copieExport(t, x)
			c.abimer(&y)
			err := s2.Restaurer(y, "t")
			var ev ErrValidation
			if err == nil || !errors.As(err, &ev) || !strings.Contains(err.Error(), c.attendu) {
				t.Fatalf("restauration : %v (attendu : refus mentionnant %q)", err, c.attendu)
			}
			// rien n'a changé
			if n, _, _ := s2.Compter(); n["pj"] != 1 || n["pnj"] != 2 {
				t.Fatalf("univers modifié par une restauration refusée : %v", n)
			}
			if e, _ := s2.Lire(seance.ID); e.Data["campagne"] != float64(camp.ID) {
				t.Fatalf("fiche modifiée par une restauration refusée : %#v", e.Data)
			}
		})
	}

	// une feuille rattachée à une fiche qui n'est pas un PJ est ignorée
	y := copieExport(t, x)
	y.Feuilles[0].Entite = chef.ID
	if err := s2.Restaurer(y, "t"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s2.LireFeuille(chef.ID); err == nil {
		t.Fatal("feuille restaurée sur un PNJ")
	}

	// un export plus ancien n'est pas refusé pour un champ qui a changé : il est recopié dans les
	// notes du MJ, comme à l'ouverture d'un ancien univers
	y = copieExport(t, x)
	y.Donnees = ""
	fiche(&y, chef.ID).Data["pirate"] = "Pavillon noir"
	if err := s2.Restaurer(y, "t"); err != nil {
		t.Fatalf("ancien export refusé : %v", err)
	}
	if e, _ := s2.Lire(chef.ID); e.Data["pirate"] != nil || !strings.Contains(e.Data["notes"].(string), "Pavillon noir") {
		t.Fatalf("ancien champ non recopié dans les notes : %#v", e.Data)
	}
}
