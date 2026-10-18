package store

import (
	"testing"
)

// Chaque copie tire ses identifiants dans sa plage ; la plage 0 n'est pas déplacée par les
// identifiants élevés qu'une copie apporte.
func TestIdentifiantsParPlage(t *testing.T) {
	s := ouvrir(t)
	a := creer(t, s, "lieu", "Le Gué", nil)
	if a.ID != 1 || s.Replique() != 0 {
		t.Fatalf("plage 0 : fiche n°%d, copie %d", a.ID, s.Replique())
	}

	if err := s.DefinirReplique(5); err != nil {
		t.Fatal(err)
	}
	base := 5 * taillePlage
	b := creer(t, s, "lieu", "La Tour", nil)
	c := creer(t, s, "pnj", "Ysolde", map[string]any{"lieu": float64(b.ID)})
	if b.ID != base+1 || c.ID != base+2 {
		t.Fatalf("plage 5 : fiches n°%d et n°%d", b.ID, c.ID)
	}
	// un identifiant élevé reste exact d'un bout à l'autre (champ de référence, relu du fichier)
	if e, _ := s.Lire(c.ID); e.Data["lieu"] != float64(b.ID) || float64(b.ID) != float64(base+1) {
		t.Fatalf("référence vers n°%d : %#v", b.ID, e.Data)
	}
	r, err := s.Relier(a.ID, b.ID, "allie", DetailRelation{}, "t")
	if err != nil || r != base+1 {
		t.Fatalf("relation : n°%d %v", r, err)
	}
	m, err := s.AjouterMedia(b.ID, "0123456789abcdef0123.png", "tour.png", []byte("\x89PNG"), "t")
	if err != nil || m.ID != base+1 {
		t.Fatalf("image : n°%d %v", m.ID, err)
	}
	d, err := s.CreerDoc("combats", []byte(`{}`), "", "t")
	if err != nil || d.ID != base+1 {
		t.Fatalf("document : n°%d %v", d.ID, err)
	}
	var j int64
	s.db.QueryRow(`SELECT MAX(id) FROM journal`).Scan(&j)
	if j <= base || j >= base+taillePlage {
		t.Fatalf("journal : n°%d hors de la plage 5", j)
	}

	// de retour dans la plage 0 (le serveur, qui a reçu les fiches de la copie 5)
	if err := s.DefinirReplique(0); err != nil {
		t.Fatal(err)
	}
	if e := creer(t, s, "lieu", "Le Moulin", nil); e.ID != 2 {
		t.Fatalf("plage 0 après une copie : fiche n°%d", e.ID)
	}
	if err := s.DefinirReplique(RepliqueMax + 1); err == nil {
		t.Fatal("numéro de copie hors limites accepté")
	}
}
