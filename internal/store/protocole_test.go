package store

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// liaison : un serveur et une copie inscrite auprès de lui, qui échangent par le protocole.
type liaison struct {
	t              *testing.T
	serveur, copie *Store
}

func nouvelleLiaison(t *testing.T, preparer func(s *Store)) *liaison {
	t.Helper()
	srv := ouvrir(t)
	if preparer != nil {
		preparer(srv)
	}
	fichier := filepath.Join(t.TempDir(), "copie.cofdm")
	n, err := srv.Inscrire("Portable", "marion", fichier)
	if err != nil || n != 1 {
		t.Fatalf("inscription : %d %v", n, err)
	}
	cp, err := Ouvrir(fichier)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cp.Fermer() })
	if cp.Replique() != 1 || !cp.EstCopie() || srv.EstCopie() {
		t.Fatalf("numéros : copie %d", cp.Replique())
	}
	if len(cp.Copies()) != 0 || len(srv.Copies()) != 1 {
		t.Fatal("la liste des copies doit rester au serveur")
	}
	return &liaison{t: t, serveur: srv, copie: cp}
}

// echanger : un échange complet ; renvoie la réponse du serveur.
func (l *liaison) echanger() Reponse {
	l.t.Helper()
	curseur, chs, err := l.copie.Preparer()
	if err != nil {
		l.t.Fatal(err)
	}
	envoye := EnvoyeJusqua(chs, metaEntier(l.copie.db, MetaSyncEnvoye))
	// comme sur le réseau : JSON aller et retour
	b, _ := json.Marshal(chs)
	var recus []Changement
	_ = json.Unmarshal(b, &recus)
	rep, err := l.serveur.Recevoir(curseur, recus)
	if err != nil {
		l.t.Fatal(err)
	}
	b, _ = json.Marshal(rep)
	var r Reponse
	_ = json.Unmarshal(b, &r)
	if err := l.copie.Integrer(r, envoye); err != nil {
		l.t.Fatal(err)
	}
	return r
}

// stable : un échange de plus ne change rien.
func (l *liaison) stable() {
	l.t.Helper()
	_, chs, _ := l.copie.Preparer()
	for _, c := range chs {
		// un champ réécrit à l'identique (conflit déjà tranché par le serveur) peut repartir une fois
		if c.Objet != ObjJournal {
			l.echanger()
			break
		}
	}
	rep := l.echanger()
	for _, c := range rep.Changements {
		if c.Objet != ObjJournal {
			l.t.Fatalf("échange de trop : %v", rep.Changements)
		}
	}
	if len(rep.Conflits) != 0 {
		l.t.Fatalf("conflits de trop : %v", rep.Conflits)
	}
}

func TestProtocole(t *testing.T) {
	var gue, baron, ombre Entite
	l := nouvelleLiaison(t, func(s *Store) {
		gue = creer(t, s, "lieu", "Le Gué", nil)
		baron = creer(t, s, "pnj", "Baron", map[string]any{"resume": "Un noble."})
		ombre = creer(t, s, "pnj", "Ombre", nil)
	})
	srv, cp := l.serveur, l.copie

	// des deux côtés, puis un échange : tout se rejoint, journal compris
	tour := creer(t, cp, "lieu", "La Tour", nil)
	cp.Modifier(baron.ID, Maj{Data: map[string]any{"resume": "Un traître."}}, "marion")
	srv.Modifier(baron.ID, Maj{Data: map[string]any{"resume": "Un noble ruiné."}}, "luc")
	srv.Modifier(baron.ID, Maj{Data: map[string]any{"age": "50 ans"}}, "luc")
	rep := l.echanger()
	if len(rep.Conflits) != 1 || rep.Conflits[0].Genre != ConflitChamp || rep.Conflits[0].Champ != "data.resume" {
		t.Fatalf("conflits : %v", rep.Conflits)
	}
	for _, s := range []*Store{srv, cp} {
		b := lireFiche(t, s, baron.ID)
		if b.Data["resume"] != "Un noble ruiné." || b.Data["age"] != "50 ans" {
			t.Fatalf("après échange : %#v", b.Data)
		}
		lireFiche(t, s, tour.ID)
	}
	journal := func(s *Store) string {
		j, _ := s.Journal(100)
		b, _ := json.Marshal(j)
		return string(b)
	}
	if !strings.Contains(journal(srv), "marion") || !strings.Contains(journal(cp), "luc") {
		t.Fatal("journal non fusionné")
	}
	l.stable()

	// le conflit attend dans la liste ; garder la valeur locale la renvoie au serveur
	cs, _ := cp.Conflits()
	if len(cs) != 1 || cs[0].Nom != "Baron" || string(cs[0].Local) != `"Un traître."` {
		t.Fatalf("liste des conflits : %v", cs)
	}
	if err := cp.ResoudreConflit(cs[0].ID, true, "marion"); err != nil {
		t.Fatal(err)
	}
	l.echanger()
	if r := lireFiche(t, srv, baron.ID).Data["resume"]; r != "Un traître." {
		t.Fatalf("valeur gardée : %v", r)
	}
	if cs, _ := cp.Conflits(); len(cs) != 0 {
		t.Fatalf("conflit resté : %v", cs)
	}

	// supprimé sur la copie, modifié sur le serveur : gardé, rendu à la copie en entier
	cp.Supprimer(gue.ID, "marion")
	srv.Modifier(gue.ID, Maj{Data: map[string]any{"rumeurs": "On y a vu un loup."}}, "luc")
	rep = l.echanger()
	if len(rep.Conflits) != 1 || rep.Conflits[0].Genre != ConflitSupprimeIci {
		t.Fatalf("suppression ici : %v", rep.Conflits)
	}
	for _, s := range []*Store{srv, cp} {
		if g := lireFiche(t, s, gue.ID); g.Nom != "Le Gué" || g.Data["rumeurs"] != "On y a vu un loup." {
			t.Fatalf("fiche gardée : %#v", g)
		}
	}
	l.stable()

	// supprimé sur le serveur, modifié sur la copie : gardé, renvoyé en entier au serveur
	srv.Supprimer(ombre.ID, "luc")
	cp.Modifier(ombre.ID, Maj{Data: map[string]any{"resume": "Il sait tout."}}, "marion")
	rep = l.echanger()
	if len(rep.Conflits) != 1 || rep.Conflits[0].Genre != ConflitSupprimeLa {
		t.Fatalf("suppression là-bas : %v", rep.Conflits)
	}
	if o := lireFiche(t, cp, ombre.ID); o.Data["resume"] != "Il sait tout." {
		t.Fatal("la copie a perdu la fiche")
	}
	l.echanger()
	if o := lireFiche(t, srv, ombre.ID); o.Nom != "Ombre" || o.Data["resume"] != "Il sait tout." {
		t.Fatalf("fiche rendue au serveur : %#v", o)
	}
	l.stable()
	// le MJ accepte finalement la suppression : elle part
	cs, _ = cp.Conflits()
	for _, c := range cs {
		if c.Genre == ConflitSupprimeLa {
			if err := cp.ResoudreConflit(c.ID, false, "marion"); err != nil {
				t.Fatal(err)
			}
		}
	}
	l.echanger()
	if _, err := srv.Lire(ombre.ID); err == nil {
		t.Fatal("suppression acceptée, fiche encore sur le serveur")
	}

	// une suppression sans conflit se propage dans les deux sens
	srv.Supprimer(tour.ID, "luc")
	l.echanger()
	if _, err := cp.Lire(tour.ID); err == nil {
		t.Fatal("suppression du serveur non reçue")
	}
	l.stable()
}

// Une valeur refusée par le serveur devient un conflit ; la copie reprend la valeur du serveur.
func TestProtocoleRefus(t *testing.T) {
	var gue Entite
	l := nouvelleLiaison(t, func(s *Store) { gue = creer(t, s, "lieu", "Le Gué", nil) })
	// une écriture que l'interface n'aurait pas permise, faite directement dans le fichier
	if _, err := l.copie.db.Exec(`UPDATE entites SET data='{"pirate":"oui"}' WHERE id=?`, gue.ID); err != nil {
		t.Fatal(err)
	}
	rep := l.echanger()
	if len(rep.Conflits) != 1 || rep.Conflits[0].Genre != ConflitRefus || rep.Conflits[0].Raison == "" {
		t.Fatalf("refus : %v", rep.Conflits)
	}
	if e := lireFiche(t, l.copie, gue.ID); e.Data["pirate"] != nil {
		t.Fatalf("la copie garde la valeur refusée : %#v", e.Data)
	}
	l.stable()
}

// Les images voyagent : la fiche de l'image d'abord, son contenu à part.
func TestProtocoleImages(t *testing.T) {
	var pj Entite
	l := nouvelleLiaison(t, func(s *Store) { pj = creer(t, s, "pj", "Céleste", nil) })
	image := []byte("\x89PNG portrait")
	m, err := l.copie.AjouterMedia(pj.ID, "abcdefabcdefabcdef01.png", "celeste.png", image, "marion")
	if err != nil {
		t.Fatal(err)
	}
	rep := l.echanger()
	if len(rep.ImagesAttendues) != 1 || rep.ImagesAttendues[0] != m.Fichier {
		t.Fatalf("images attendues : %v", rep.ImagesAttendues)
	}
	if err := l.serveur.ContenuAttendu(m.Fichier, image); err != nil {
		t.Fatal(err)
	}
	if b, _, err := l.serveur.ContenuMedia(m.Fichier); err != nil || string(b) != string(image) {
		t.Fatalf("contenu reçu : %v", err)
	}
	if err := l.serveur.ContenuAttendu("ffffffffffffffffffff.png", image); err == nil {
		t.Fatal("contenu d'une image inconnue accepté")
	}
	// dans l'autre sens : la copie reçoit la fiche de l'image, et sait qu'il lui manque le contenu
	m2, _ := l.serveur.AjouterMedia(pj.ID, "abcdefabcdefabcdef02.png", "carte.png", image, "luc")
	l.echanger()
	if manque, _ := l.copie.ImagesSansContenu(); len(manque) != 1 || manque[0] != m2.Fichier {
		t.Fatalf("images sans contenu : %v", manque)
	}
}

// Une copie ne sert pas elle-même de serveur.
func TestCopieNInscritPas(t *testing.T) {
	l := nouvelleLiaison(t, nil)
	if _, err := l.copie.Inscrire("Autre", "x", filepath.Join(t.TempDir(), "x.cofdm")); err == nil {
		t.Fatal("une copie a inscrit une copie")
	}
	if _, err := l.serveur.Inscrire("  ", "x", filepath.Join(t.TempDir(), "y.cofdm")); err == nil {
		t.Fatal("copie sans nom inscrite")
	}
}
