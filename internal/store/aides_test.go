package store

import (
	"encoding/json"
	"testing"
)

// « a reçu » ne va que d'un PJ vers une aide de jeu, quel que soit le chemin (saisie, échange,
// restauration).
func TestRelationRecu(t *testing.T) {
	s := ouvrir(t)
	pj := creer(t, s, "pj", "Aelis", nil)
	pnj := creer(t, s, "pnj", "Brom", nil)
	aide := creer(t, s, "aide", "Lettre", map[string]any{"contenu": "Venez seuls."})
	if _, err := s.Relier(pnj.ID, aide.ID, "recu", DetailRelation{}, "mj"); err == nil {
		t.Fatal("un PNJ reçoit une aide de jeu")
	}
	if _, err := s.Relier(aide.ID, pj.ID, "recu", DetailRelation{}, "mj"); err == nil {
		t.Fatal("relation « a reçu » à l'envers")
	}
	id, err := s.Relier(pj.ID, aide.ID, "recu", DetailRelation{}, "mj")
	if err != nil {
		t.Fatal(err)
	}
	ami, err := s.Relier(pj.ID, pnj.ID, "ami", DetailRelation{}, "mj")
	if err != nil {
		t.Fatal(err)
	}
	typ := "recu"
	if err := s.ModifierRelation(ami, DetailRelation{Type: &typ}, "mj"); err == nil {
		t.Fatal("relation changée en « a reçu » vers un PNJ")
	}
	envois, err := s.Envois()
	if err != nil || len(envois) != 1 || envois[0].Relation != id || envois[0].PJ != pj.ID || envois[0].Aide != aide.ID {
		t.Fatalf("envois : %v %v", envois, err)
	}
	// un échange qui pose « a reçu » d'un PNJ est refusé
	refus, err := s.Appliquer([]Changement{
		{Objet: ObjRelation, Cle: "900", Champ: ChampExiste, Valeur: json.RawMessage(`true`)},
		{Objet: ObjRelation, Cle: "900", Champ: "a", Valeur: json.RawMessage(itoa(pnj.ID))},
		{Objet: ObjRelation, Cle: "900", Champ: "b", Valeur: json.RawMessage(itoa(aide.ID))},
		{Objet: ObjRelation, Cle: "900", Champ: "type", Valeur: json.RawMessage(`"recu"`)},
	}, nil, true)
	if err != nil || len(refus) == 0 {
		t.Fatalf("échange refusé attendu : %v %v", refus, err)
	}
	if envois, _ := s.Envois(); len(envois) != 1 {
		t.Fatalf("envois après le refus : %v", envois)
	}
	// supprimer l'aide retire ses envois
	if _, err := s.Supprimer(aide.ID, "mj"); err != nil {
		t.Fatal(err)
	}
	if envois, _ := s.Envois(); len(envois) != 0 {
		t.Fatalf("envois après la suppression de l'aide : %v", envois)
	}
}

// Une aide de jeu, son image et ses envois voyagent avec la synchronisation, dans les deux sens.
func TestSynchronisationAides(t *testing.T) {
	var pj, seance Entite
	p := nouvellePaire(t, func(s *Store) {
		pj = creer(t, s, "pj", "Aelis", nil)
		seance = creer(t, s, "seance", "Séance 2", nil)
	})
	// créée et envoyée sur la copie, hors ligne
	aide := creer(t, p.copie, "aide", "Plan du moulin", map[string]any{"forme": "image", "contenu": "Au nord du [[Séance 2]]."})
	if _, err := p.copie.AjouterMedia(aide.ID, "0123456789abcdef0123.png", "plan.png", []byte("png"), "mj"); err != nil {
		t.Fatal(err)
	}
	ctx := seance.ID
	if _, err := p.copie.Relier(pj.ID, aide.ID, "recu", DetailRelation{Contexte: &ctx}, "mj"); err != nil {
		t.Fatal(err)
	}
	if _, refus := p.synchroniser(); len(refus) != 0 {
		t.Fatalf("refus : %v", refus)
	}
	if e := lireFiche(t, p.serveur, aide.ID); e.Type != "aide" || e.Data["forme"] != "image" {
		t.Fatalf("aide sur le serveur : %v", e)
	}
	envois, _ := p.serveur.Envois()
	if len(envois) != 1 || envois[0].PJ != pj.ID || envois[0].Aide != aide.ID || envois[0].Seance != seance.ID {
		t.Fatalf("envoi sur le serveur : %v", envois)
	}
	if m, _ := p.serveur.Medias(aide.ID); len(m) != 1 {
		t.Fatalf("image de l'aide sur le serveur : %v", m)
	}
	// retiré sur le serveur : retiré sur la copie
	if err := p.serveur.Delier(envois[0].Relation, "mj"); err != nil {
		t.Fatal(err)
	}
	p.synchroniser()
	if envois, _ := p.copie.Envois(); len(envois) != 0 {
		t.Fatalf("envoi retiré, encore sur la copie : %v", envois)
	}
}
