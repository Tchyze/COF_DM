package store

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

// Un univers neuf part des valeurs par défaut : dés, aides de jeu, feuilles et page des joueurs
// coupés ; le reste activé.
func TestFonctionnalitesParDefaut(t *testing.T) {
	s := ouvrir(t)
	etat := s.Fonctionnalites()
	for cle, attendu := range map[string]bool{"des": false, "aides": false, "feuilles": false, "page_joueurs": false,
		"combat": true, "rencontres": true, "secrets": true, "ecran_mj": true, "graphe": true, "obsidian": true} {
		if etat[cle] != attendu {
			t.Fatalf("%s : %v, attendu %v", cle, etat[cle], attendu)
		}
	}
	if _, err := s.ReglerFonctionnalites(map[string]bool{"inconnue": true}, "mj"); err == nil {
		t.Fatal("fonctionnalité inconnue acceptée")
	}
	etat, err := s.ReglerFonctionnalites(map[string]bool{"des": true, "combat": false}, "mj")
	if err != nil || !etat["des"] || etat["combat"] || !etat["secrets"] {
		t.Fatalf("réglage : %v %v", etat, err)
	}
	if !s.Active("des") || s.Active("combat") {
		t.Fatal("état relu")
	}
	if j, _ := s.Journal(1); len(j) != 1 || j[0].Action != "fonctionnalités" {
		t.Fatalf("journal : %v", j)
	}
}

// Un univers d'avant les fonctionnalités garde ce dont il se sert : des feuilles de personnage (et
// donc la page des joueurs), des aides de jeu. Ce qui est déjà réglé n'est plus touché.
func TestFonctionnalitesReprise(t *testing.T) {
	chemin := filepath.Join(t.TempDir(), "ancien.cofdm")
	s, err := Ouvrir(chemin)
	if err != nil {
		t.Fatal(err)
	}
	pj := creer(t, s, "pj", "Aelis", nil)
	if _, err := s.EcrireFeuille(pj.ID, []byte(`{"pv":10}`), 0, "mj"); err != nil {
		t.Fatal(err)
	}
	if err := s.RetirerMeta(MetaFonctions); err != nil { // comme avant cette version
		t.Fatal(err)
	}
	s.Fermer()
	s, err = Ouvrir(chemin)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Fermer()
	etat := s.Fonctionnalites()
	if !etat["feuilles"] || !etat["page_joueurs"] || etat["aides"] || etat["des"] || !etat["combat"] {
		t.Fatalf("reprise : %v", etat)
	}
	if _, err := s.ReglerFonctionnalites(map[string]bool{"feuilles": false}, "mj"); err != nil {
		t.Fatal(err)
	}
	s.Fermer()
	s, _ = Ouvrir(chemin)
	if s.Active("feuilles") {
		t.Fatal("le choix du MJ est repris à l'ouverture suivante")
	}
}

// Les fonctionnalités activées voyagent avec la synchronisation ; une valeur invalide est refusée.
func TestSynchronisationFonctionnalites(t *testing.T) {
	p := nouvellePaire(t, func(s *Store) { creer(t, s, "pnj", "Brom", nil) })
	if _, err := p.copie.ReglerFonctionnalites(map[string]bool{"des": true, "secrets": false}, "mj"); err != nil {
		t.Fatal(err)
	}
	if _, refus := p.synchroniser(); len(refus) != 0 {
		t.Fatalf("refus : %v", refus)
	}
	if !p.serveur.Active("des") || p.serveur.Active("secrets") {
		t.Fatalf("serveur : %v", p.serveur.Fonctionnalites())
	}
	refus, err := p.serveur.Appliquer([]Changement{{Objet: ObjReglage, Cle: MetaFonctions, Champ: "valeur", Valeur: json.RawMessage(`{"pirate": true}`)}}, nil, true)
	if err != nil || len(refus) != 1 {
		t.Fatalf("réglage invalide : %v %v", refus, err)
	}
	if !p.serveur.Active("des") {
		t.Fatal("réglage invalide appliqué")
	}
}
