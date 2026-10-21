package store

import (
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Tchyze/cof_dm/internal/modele"
)

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// paire : un univers « serveur » et une copie faite de son fichier, avec ce que la copie sait de la
// synchronisation (curseur : dernier numéro d'ordre du serveur reçu ; envoye : dernier numéro
// d'ordre local envoyé).
type paire struct {
	t               *testing.T
	serveur, copie  *Store
	curseur, envoye int64
}

func nouvellePaire(t *testing.T, preparer func(s *Store)) *paire {
	t.Helper()
	srv := ouvrir(t)
	if preparer != nil {
		preparer(srv)
	}
	seq, err := srv.Relever()
	if err != nil {
		t.Fatal(err)
	}
	chemin := filepath.Join(t.TempDir(), "copie.cofdm")
	if err := srv.Sauvegarder(chemin); err != nil {
		t.Fatal(err)
	}
	cp, err := Ouvrir(chemin)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cp.Fermer() })
	if err := cp.DefinirReplique(1); err != nil {
		t.Fatal(err)
	}
	envoye, err := cp.Relever()
	if err != nil {
		t.Fatal(err)
	}
	return &paire{t: t, serveur: srv, copie: cp, curseur: seq, envoye: envoye}
}

// synchroniser joue un échange, comme le fera le protocole : la copie envoie ses changements, un
// champ changé aussi sur le serveur garde la valeur du serveur (conflits rendus), puis la copie reçoit
// tout ce que le serveur a de nouveau.
func (p *paire) synchroniser() (conflits []Changement, refus []Refus) {
	p.t.Helper()
	if _, err := p.copie.Relever(); err != nil {
		p.t.Fatal(err)
	}
	locaux, err := p.copie.ChangementsDepuis(p.envoye)
	if err != nil {
		p.t.Fatal(err)
	}
	if _, err := p.serveur.Relever(); err != nil {
		p.t.Fatal(err)
	}
	distants, _ := p.serveur.ChangementsDepuis(p.curseur)
	changeLa := map[string]bool{}
	for _, c := range distants {
		changeLa[c.adresse()] = true
	}
	var envoi []Changement
	for _, c := range locaux {
		if changeLa[c.adresse()] {
			conflits = append(conflits, c)
			continue
		}
		envoi = append(envoi, c)
	}
	if refus, err = p.serveur.Appliquer(envoi, nil, true); err != nil {
		p.t.Fatal(err)
	}
	recus, _ := p.serveur.ChangementsDepuis(p.curseur)
	if _, err := p.copie.Appliquer(recus, nil, false); err != nil {
		p.t.Fatal(err)
	}
	if p.curseur, err = p.serveur.Relever(); err != nil {
		p.t.Fatal(err)
	}
	if p.envoye, err = p.copie.Relever(); err != nil {
		p.t.Fatal(err)
	}
	// rien ne doit plus bouger : ni de nouveaux changements locaux, ni sur le serveur
	if l, _ := p.copie.ChangementsDepuis(p.envoye); len(l) != 0 {
		p.t.Fatalf("la copie relève encore des changements après l'échange : %v", l)
	}
	if again, _ := p.serveur.Relever(); again != p.curseur {
		p.t.Fatal("le serveur relève encore des changements après l'échange")
	}
	return conflits, refus
}

func lireFiche(t *testing.T, s *Store, id int64) Entite {
	t.Helper()
	e, err := s.Lire(id)
	if err != nil {
		t.Fatalf("fiche n°%d : %v", id, err)
	}
	return e
}

// Les deux côtés écrivent ; un échange les rend identiques, champ par champ.
func TestSynchronisationFusion(t *testing.T) {
	var gue, baron Entite
	p := nouvellePaire(t, func(s *Store) {
		gue = creer(t, s, "lieu", "Le Gué", nil)
		baron = creer(t, s, "pnj", "Baron", map[string]any{"resume": "Un noble.", "lieu": float64(gue.ID)})
	})
	// sur la copie : une fiche nouvelle qui en désigne une autre, nouvelle aussi ; une relation ; un champ
	tour := creer(t, p.copie, "lieu", "La Tour", nil)
	garde := creer(t, p.copie, "pnj", "Garde", map[string]any{"lieu": float64(tour.ID), "resume": "Voir [[Baron]]."})
	if _, err := p.copie.Relier(garde.ID, baron.ID, "allie", DetailRelation{}, "copie"); err != nil {
		t.Fatal(err)
	}
	if _, err := p.copie.Modifier(baron.ID, Maj{Data: map[string]any{"age": "50 ans"}}, "copie"); err != nil {
		t.Fatal(err)
	}
	// sur le serveur : un autre champ de la même fiche, et une fiche nouvelle
	if _, err := p.serveur.Modifier(baron.ID, Maj{Data: map[string]any{"statut": "vivant"}}, "srv"); err != nil {
		t.Fatal(err)
	}
	moulin := creer(t, p.serveur, "lieu", "Le Moulin", nil)

	conflits, refus := p.synchroniser()
	if len(conflits) != 0 || len(refus) != 0 {
		t.Fatalf("conflits %v, refus %v", conflits, refus)
	}
	if garde.ID <= taillePlage {
		t.Fatalf("la copie n'a pas tiré dans sa plage : n°%d", garde.ID)
	}
	for _, s := range []*Store{p.serveur, p.copie} {
		b := lireFiche(t, s, baron.ID)
		if b.Data["age"] != "50 ans" || b.Data["statut"] != "vivant" || b.Data["resume"] != "Un noble." {
			t.Fatalf("fusion des champs : %#v", b.Data)
		}
		g := lireFiche(t, s, garde.ID)
		if g.Data["lieu"] != float64(tour.ID) {
			t.Fatalf("référence vers une fiche reçue : %#v", g.Data)
		}
		if r, _ := s.Relations(baron.ID); len(r) != 1 {
			t.Fatalf("relation : %v", r)
		}
		lireFiche(t, s, moulin.ID)
		// les index suivent : recherche et mentions
		if l, _ := s.Lister(Filtre{Q: "tour"}); len(l) != 1 {
			t.Fatalf("recherche après échange : %v", l)
		}
		if refs, _ := s.References(baron.ID); len(refs) == 0 {
			t.Fatal("la mention [[Baron]] n'est pas indexée")
		}
	}

	// le même champ changé des deux côtés : la valeur du serveur reste, la copie le sait
	p.copie.Modifier(baron.ID, Maj{Data: map[string]any{"resume": "Un traître."}}, "copie")
	p.serveur.Modifier(baron.ID, Maj{Data: map[string]any{"resume": "Un noble ruiné."}}, "srv")
	conflits, _ = p.synchroniser()
	if len(conflits) != 1 || conflits[0].Champ != "data.resume" || string(conflits[0].Valeur) != `"Un traître."` {
		t.Fatalf("conflit : %v", conflits)
	}
	for _, s := range []*Store{p.serveur, p.copie} {
		if r := lireFiche(t, s, baron.ID).Data["resume"]; r != "Un noble ruiné." {
			t.Fatalf("après conflit : %v", r)
		}
	}

	// renommer une fiche réécrit ses mentions : les deux changements passent
	nom := "Baron Noir"
	if _, err := p.copie.Modifier(baron.ID, Maj{Nom: &nom}, "copie"); err != nil {
		t.Fatal(err)
	}
	p.synchroniser()
	if g := lireFiche(t, p.serveur, garde.ID); !strings.Contains(g.Data["resume"].(string), "[[Baron Noir]]") {
		t.Fatalf("mention renommée : %v", g.Data["resume"])
	}

	// une suppression se propage, avec ses relations
	if _, err := p.copie.Supprimer(garde.ID, "copie"); err != nil {
		t.Fatal(err)
	}
	p.synchroniser()
	if _, err := p.serveur.Lire(garde.ID); err == nil {
		t.Fatal("fiche supprimée sur la copie, encore sur le serveur")
	}
	if r, _ := p.serveur.Relations(baron.ID); len(r) != 0 {
		t.Fatalf("relation d'une fiche supprimée : %v", r)
	}
}

// Images, feuilles, documents et réglages suivent ; un combat en cours reste de son côté.
func TestSynchronisationObjets(t *testing.T) {
	var pj Entite
	p := nouvellePaire(t, func(s *Store) { pj = creer(t, s, "pj", "Céleste", map[string]any{"niveau": 2.0}) })
	if _, err := p.copie.EcrireFeuille(pj.ID, []byte(`{"pv":9}`), 0, "copie"); err != nil {
		t.Fatal(err)
	}
	image := []byte("\x89PNG portrait")
	m, err := p.copie.AjouterMedia(pj.ID, "abcdefabcdefabcdef01.png", "celeste.png", image, "copie")
	if err != nil {
		t.Fatal(err)
	}
	monstre, err := p.copie.CreerDoc("monstres", []byte(`{"nom":"Rat géant","nc":0,"pv":4,"defense":12}`), "Rat géant", "copie")
	if err != nil {
		t.Fatal(err)
	}
	enCours, _ := p.serveur.CreerDoc("combats", []byte(`{"nom":"Embuscade","termine":false}`), "", "srv")
	fini, _ := p.serveur.CreerDoc("combats", []byte(`{"nom":"Escarmouche","termine":true}`), "", "srv")
	cal := modele.CalendrierParDefaut()
	cal.Ere = "de la Couronne"
	if err := p.copie.DefinirCalendrier(cal); err != nil {
		t.Fatal(err)
	}
	if err := p.copie.SetMeta(MetaEditionJoueurs, "1"); err != nil { // le MJ ouvre l'édition sur son bureau
		t.Fatal(err)
	}

	// le contenu des images voyage à part : ici, donné directement
	if _, err := p.copie.Relever(); err != nil {
		t.Fatal(err)
	}
	locaux, _ := p.copie.ChangementsDepuis(p.envoye)
	if refus, err := p.serveur.Appliquer(locaux, map[string][]byte{m.Fichier: image}, true); err != nil || len(refus) != 0 {
		t.Fatalf("application : %v %v", err, refus)
	}
	p.synchroniser()

	if b, _, err := p.serveur.LireFeuille(pj.ID); err != nil || !strings.Contains(string(b), `"pv":9`) {
		t.Fatalf("feuille : %v %s", err, b)
	}
	if b, _, err := p.serveur.ContenuMedia(m.Fichier); err != nil || string(b) != string(image) {
		t.Fatalf("image : %v", err)
	}
	if _, err := p.serveur.LireDoc("monstres", monstre.ID); err != nil {
		t.Fatalf("monstre : %v", err)
	}
	if p.serveur.Calendrier().Ere != "de la Couronne" {
		t.Fatal("calendrier non reçu")
	}
	if v, _ := p.serveur.Meta(MetaEditionJoueurs); v != "1" {
		t.Fatalf("édition par les joueurs non reçue : %q", v)
	}
	// refermée sur le serveur, elle se referme sur la copie
	if err := p.serveur.SetMeta(MetaEditionJoueurs, ""); err != nil {
		t.Fatal(err)
	}
	p.synchroniser()
	if v, _ := p.copie.Meta(MetaEditionJoueurs); v != "" {
		t.Fatalf("édition par les joueurs refermée : %q sur la copie", v)
	}
	if _, err := p.copie.LireDoc("combats", fini.ID); err != nil {
		t.Fatalf("combat terminé non reçu : %v", err)
	}
	if _, err := p.copie.LireDoc("combats", enCours.ID); err == nil {
		t.Fatal("combat en cours reçu")
	}
	// le combat en cours, terminé ensuite, part à son tour ; il n'a jamais passé pour supprimé
	if _, err := p.serveur.MajDoc("combats", enCours.ID, []byte(`{"nom":"Embuscade","termine":true}`), "", "srv"); err != nil {
		t.Fatal(err)
	}
	p.synchroniser()
	if _, err := p.copie.LireDoc("combats", enCours.ID); err != nil {
		t.Fatalf("combat terminé ensuite : %v", err)
	}
}

// Un changement invalide est refusé seul ; les autres passent.
func TestSynchronisationRefus(t *testing.T) {
	var gue Entite
	p := nouvellePaire(t, func(s *Store) { gue = creer(t, s, "lieu", "Le Gué", nil) })
	cle := json.RawMessage(`"Une rumeur."`)
	chs := []Changement{
		{Objet: ObjFiche, Cle: itoa(gue.ID), Champ: "data.pirate", Valeur: json.RawMessage(`"oui"`)},
		{Objet: ObjFiche, Cle: itoa(gue.ID), Champ: "data.rumeurs", Valeur: cle},
		{Objet: ObjFiche, Cle: "77", Champ: ChampExiste, Valeur: json.RawMessage(`true`)},
		{Objet: ObjFiche, Cle: "77", Champ: "type", Valeur: json.RawMessage(`"pirate"`)},
		{Objet: ObjRelation, Cle: "5", Champ: "a", Valeur: json.RawMessage(`9999`)},
		{Objet: ObjReglage, Cle: "page_joueurs", Champ: "valeur", Valeur: json.RawMessage(`"1"`)},
		{Objet: "inconnu", Cle: "1", Champ: "x", Valeur: json.RawMessage(`1`)},
	}
	refus, err := p.serveur.Appliquer(chs, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	raisons := map[string]bool{}
	for _, r := range refus {
		raisons[r.Objet+"/"+r.Cle+"/"+r.Champ] = true
	}
	for _, attendu := range []string{ObjFiche + "/" + itoa(gue.ID) + "/data.pirate", ObjFiche + "/77/type", ObjRelation + "/5/a", ObjReglage + "/page_joueurs/valeur", "inconnu/1/x"} {
		if !raisons[attendu] {
			t.Fatalf("refus attendu pour %s : %v", attendu, refus)
		}
	}
	e := lireFiche(t, p.serveur, gue.ID)
	if e.Data["rumeurs"] != "Une rumeur." || e.Data["pirate"] != nil {
		t.Fatalf("champs après refus : %#v", e.Data)
	}
	if _, err := p.serveur.Lire(77); err == nil {
		t.Fatal("fiche de type inconnu créée")
	}
}
