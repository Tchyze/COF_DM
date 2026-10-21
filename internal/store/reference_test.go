package store

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/Tchyze/cof_dm/internal/reference"
)

// ancienReglage : des profils, peuples et voies (inventés) tels que les rangeait le modèle 1.
const ancienReglage = `{"format":"tracker-cof-reference","version":1,
	"profils":[{"ref":"essai-passeur","nom":"Passeur","voies":["Voie du gué"]}],
	"peuples":[{"nom":"Gens des marais","caracs":"+1 SAG"}],
	"voies":[{"ref":"essai-gue","nom":"Voie du gué","rangs":[{"nom":"Pas sûr"}]},{"nom":"Voie de la lanterne"}]}`

func nombreDocs(t *testing.T, s *Store) map[string]int {
	t.Helper()
	ref, err := s.Reference()
	if err != nil {
		t.Fatal(err)
	}
	return map[string]int{"profils": len(ref.Profils), "peuples": len(ref.Peuples), "voies": len(ref.Voies)}
}

// À l'ouverture, l'ancien réglage est rangé un par un dans la plage réservée, puis retiré ; une
// seconde ouverture n'y change rien.
func TestReferenceReprise(t *testing.T) {
	chemin := filepath.Join(t.TempDir(), "u.cofdm")
	s, err := Ouvrir(chemin)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetMeta(MetaReference, ancienReglage); err != nil {
		t.Fatal(err)
	}
	s.Fermer()
	for range 2 {
		if s, err = Ouvrir(chemin); err != nil {
			t.Fatal(err)
		}
		if n := nombreDocs(t, s); n["profils"] != 1 || n["peuples"] != 1 || n["voies"] != 2 {
			t.Fatalf("après reprise : %v", n)
		}
		if _, ok := s.Meta(MetaReference); ok {
			t.Fatal("ancien réglage gardé")
		}
		ref, _ := s.Reference()
		for _, v := range ref.Voies {
			if v.ID/taillePlage != repliqueReprise {
				t.Fatalf("voie %s hors de la plage réservée : %d", v.Nom, v.ID)
			}
		}
		if ref.Profils[0].Etiquette != reference.Officielle || ref.Peuples[0].Modificateurs["SAG"] != 1 {
			t.Fatalf("éléments repris : %+v %+v", ref.Profils[0], ref.Peuples[0])
		}
		s.Fermer()
	}
	if err := s.DefinirReplique(RepliqueMax); err == nil {
		t.Fatal("numéro de copie de la plage réservée accepté")
	}
}

// Le serveur et sa copie reprennent l'ancien réglage chacun de son côté (la copie à son ouverture, le
// serveur à sa mise à jour) : ils rangent les mêmes éléments sous les mêmes identifiants, et
// l'échange se fait sans doublon ni conflit.
func TestReferenceRepriseDesDeuxCotes(t *testing.T) {
	l := nouvelleLiaison(t, func(s *Store) {
		if err := s.SetMeta(MetaReference, ancienReglage); err != nil {
			t.Fatal(err)
		}
	})
	if _, ok := l.copie.Meta(MetaReference); ok {
		t.Fatal("copie : ancien réglage gardé")
	}
	if _, ok := l.serveur.Meta(MetaReference); !ok {
		t.Fatal("serveur : l'ancien réglage devait attendre sa mise à jour")
	}
	if err := l.serveur.reprendreALOuverture(); err != nil {
		t.Fatal(err)
	}
	l.echanger()
	l.stable()
	a, _ := l.serveur.Reference()
	b, _ := l.copie.Reference()
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	if string(ja) != string(jb) || len(a.Voies) != 2 {
		t.Fatalf("serveur et copie diffèrent :\n%s\n%s", ja, jb)
	}
	if c, _ := l.serveur.Conflits(); len(c) != 0 {
		t.Fatalf("conflits : %v", c)
	}
	// ensuite, une voie modifiée d'un côté passe seule
	n := reference.Vide()
	n.Voies = []reference.Voie{{Ref: "essai-gue", Nom: "Voie du gué", Description: "Revue."}}
	if _, err := l.copie.FusionnerReference(n, "marion"); err != nil {
		t.Fatal(err)
	}
	l.echanger()
	a, _ = l.serveur.Reference()
	if len(a.Voies) != 2 || a.Voies[1].Description != "Revue." { // triées par nom
		t.Fatalf("voie modifiée sur la copie : %+v", a.Voies)
	}
}

// Une copie qui reçoit l'ancien réglage d'un serveur d'avant le modèle 2 le range aussitôt.
func TestReferenceRecueDUnAncienServeur(t *testing.T) {
	s := ouvrir(t)
	ch := []Changement{{Objet: ObjReglage, Cle: MetaReference, Champ: "valeur", Valeur: json.RawMessage(ancienReglage)}}
	refus, err := s.Appliquer(ch, nil, true)
	if err != nil || len(refus) != 0 {
		t.Fatalf("réception : %v %v", refus, err)
	}
	if n := nombreDocs(t, s); n["voies"] != 2 {
		t.Fatalf("après réception : %v", n)
	}
	if _, ok := s.Meta(MetaReference); ok {
		t.Fatal("ancien réglage rangé tel quel")
	}
}

func TestReferenceFusion(t *testing.T) {
	s := ouvrir(t)
	r, err := reference.Lire([]byte(ancienReglage))
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.FusionnerReference(r, "test")
	if err != nil || b.Nouveaux != 4 || b.MisAJour != 0 {
		t.Fatalf("premier import : %+v %v", b, err)
	}
	// même référence, ou même nom sans référence : remplacé
	r.Voies[1].Description = "Une lueur."
	if b, _ = s.FusionnerReference(r, "test"); b.Nouveaux != 0 || b.MisAJour != 4 {
		t.Fatalf("second import : %+v", b)
	}
	ref, _ := s.Reference()
	if len(ref.Voies) != 2 || ref.Voies[0].Description != "Une lueur." || ref.Voies[0].ID == 0 {
		t.Fatalf("voies : %+v", ref.Voies)
	}
	if ref.Voies[0].ID/taillePlage != 0 {
		t.Fatalf("un import tire ses identifiants dans la plage de la copie : %d", ref.Voies[0].ID)
	}
	if err := s.ViderReference("test"); err != nil {
		t.Fatal(err)
	}
	if n := nombreDocs(t, s); n["voies"]+n["profils"]+n["peuples"] != 0 {
		t.Fatalf("après retrait : %v", n)
	}
}

// Un export d'avant le modèle 2 porte encore l'ancien réglage : sa restauration le range un par un.
func TestReferenceRestaurationAncienExport(t *testing.T) {
	s := ouvrir(t)
	x, err := s.Exporter()
	if err != nil {
		t.Fatal(err)
	}
	x.Modele, x.Reference = 1, json.RawMessage(ancienReglage)
	if err := s.Restaurer(x, "test"); err != nil {
		t.Fatal(err)
	}
	if n := nombreDocs(t, s); n["voies"] != 2 || n["profils"] != 1 {
		t.Fatalf("après restauration : %v", n)
	}
	// et le nouvel export les porte dans ses documents
	if x, _ = s.Exporter(); len(x.Reference) != 0 || len(x.Documents) != 4 {
		t.Fatalf("nouvel export : %d documents, réglage %s", len(x.Documents), x.Reference)
	}
}
