package store

import (
	"testing"

	"github.com/Tchyze/cof_dm/internal/modele"
)

// Le fuseau horaire de l'agenda et les champs d'une séance voyagent avec la synchronisation ; un
// fuseau inconnu est refusé, d'où qu'il vienne.
func TestSynchronisationAgenda(t *testing.T) {
	p := nouvellePaire(t, nil)
	camp, err := p.copie.Creer("campagne", "La Marche", nil, nil, "copie")
	if err != nil {
		t.Fatal(err)
	}
	s, err := p.copie.Creer("seance", "Séance 1", map[string]any{"campagne": camp.ID, "date_reelle": "2027-03-06",
		"heure": "20h30", "duree": "3 h 30", "lieu_reel": "Chez Léa", "visio": "https://visio.example/table",
		"infos_joueurs": "Apportez vos dés", "nature": "séance zéro", "statut": "annulée"}, nil, "copie")
	if err != nil {
		t.Fatal(err)
	}
	if s.Data["heure"] != "20:30" {
		t.Fatalf("heure rangée : %v", s.Data["heure"])
	}
	if err := p.copie.ReglerFuseau("America/Montreal", "copie"); err != nil {
		t.Fatal(err)
	}
	if err := p.copie.ReglerFuseau("Mars/Olympus", "copie"); err == nil {
		t.Fatal("fuseau inconnu accepté")
	}
	p.synchroniser()
	if p.serveur.Fuseau() != "America/Montreal" {
		t.Fatalf("fuseau reçu : %q", p.serveur.Fuseau())
	}
	recue, err := p.serveur.Lire(s.ID)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range map[string]string{"heure": "20:30", "duree": "3 h 30", "lieu_reel": "Chez Léa", "statut": "annulée", "nature": "séance zéro"} {
		if recue.Data[k] != v {
			t.Errorf("%s reçu : %v", k, recue.Data[k])
		}
	}
	// revenu au fuseau par défaut, le réglage s'efface partout
	if err := p.serveur.ReglerFuseau(modele.FuseauParDefaut, "srv"); err != nil {
		t.Fatal(err)
	}
	p.synchroniser()
	if v, ok := p.copie.Meta(MetaFuseau); ok && v != "" {
		t.Fatalf("fuseau par défaut : %q gardé sur la copie", v)
	}
	if p.copie.Fuseau() != modele.FuseauParDefaut {
		t.Fatalf("fuseau : %q", p.copie.Fuseau())
	}
}

func TestHeureEtDureeValidees(t *testing.T) {
	s := ouvrir(t)
	for _, data := range []map[string]any{{"heure": "25:00"}, {"heure": "midi"}, {"duree": "45 min"}, {"statut": "reportée"}} {
		if _, err := s.Creer("seance", "Mauvaise", data, nil, "test"); err == nil {
			t.Errorf("%v accepté", data)
		}
	}
}

// Une séance qui passe à « jouée » garde les PJ actifs de sa campagne à ce moment (« prévus ») ; un
// PJ mort, archivé ou d'une autre campagne n'y est pas, et elle ne les recalcule plus ensuite.
func TestSeancePrevus(t *testing.T) {
	s := ouvrir(t)
	camp, _ := s.Creer("campagne", "La Marche", nil, nil, "test")
	autre, _ := s.Creer("campagne", "Ailleurs", nil, nil, "test")
	a, _ := s.Creer("pj", "Arwen", map[string]any{"campagne": camp.ID, "statut": "actif"}, nil, "test")
	b, _ := s.Creer("pj", "Brom", map[string]any{"campagne": camp.ID}, nil, "test")
	s.Creer("pj", "Cid", map[string]any{"campagne": camp.ID, "statut": "mort"}, nil, "test")
	s.Creer("pj", "Dara", map[string]any{"campagne": autre.ID}, nil, "test")
	e, _ := s.Creer("pj", "Eli", map[string]any{"campagne": camp.ID}, nil, "test")
	if _, err := s.Archiver(e.ID, true, "test"); err != nil {
		t.Fatal(err)
	}
	se, _ := s.Creer("seance", "Séance 1", map[string]any{"campagne": camp.ID, "statut": "prévue"}, nil, "test")
	if _, ok := se.Data["prevus"]; ok {
		t.Fatal("une séance prévue n'a pas encore de PJ prévus")
	}
	se, err := s.Modifier(se.ID, Maj{Data: map[string]any{"statut": "jouée"}}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(se.Data["prevus"]); len(got) != 2 || got[0] != a.ID || got[1] != b.ID {
		t.Fatalf("prévus : %v", se.Data["prevus"])
	}
	s.Modifier(b.ID, Maj{Data: map[string]any{"statut": "mort"}}, "test")
	se, _ = s.Modifier(se.ID, Maj{Data: map[string]any{"resume": "Ils ont gagné."}}, "test")
	if len(ids(se.Data["prevus"])) != 2 {
		t.Fatalf("prévus recalculés : %v", se.Data["prevus"])
	}
}

// Les réponses des joueurs : une par PJ (la nouvelle remplace l'ancienne), retirée par "" ; la date de
// modification de la séance ne bouge pas ; un PJ supprimé emporte sa réponse ; elles voyagent avec la
// synchronisation.
func TestReponsesSeance(t *testing.T) {
	p := nouvellePaire(t, nil)
	s := p.copie
	camp, _ := s.Creer("campagne", "La Marche", nil, nil, "test")
	a, _ := s.Creer("pj", "Arwen", map[string]any{"campagne": camp.ID}, nil, "test")
	b, _ := s.Creer("pj", "Brom", map[string]any{"campagne": camp.ID}, nil, "test")
	se, _ := s.Creer("seance", "S1", map[string]any{"campagne": camp.ID, "date_reelle": "2099-03-06"}, nil, "test")
	if _, err := s.Repondre(se.ID, a.ID, "peut-être", "lea"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Repondre(se.ID, b.ID, "absent", "hugo"); err != nil {
		t.Fatal(err)
	}
	e, err := s.Repondre(se.ID, a.ID, "présent", "lea")
	if err != nil {
		t.Fatal(err)
	}
	l := modele.LireReponses(e.Data["reponses"])
	if len(l) != 2 || l[0].PJ != b.ID || l[1].PJ != a.ID || l[1].Reponse != "présent" || l[1].Le == 0 {
		t.Fatalf("réponses : %+v", l)
	}
	if e.Modifie != se.Modifie {
		t.Fatal("la date de modification de la séance a bougé")
	}
	if _, err := s.Repondre(se.ID, a.ID, "bientôt", "lea"); err == nil {
		t.Fatal("réponse inconnue acceptée")
	}
	if _, err := s.Repondre(se.ID, camp.ID, "présent", "lea"); err == nil {
		t.Fatal("réponse pour une fiche qui n'est pas un PJ")
	}
	p.synchroniser()
	recue, _ := p.serveur.Lire(se.ID)
	if l := modele.LireReponses(recue.Data["reponses"]); len(l) != 2 {
		t.Fatalf("réponses reçues : %+v", recue.Data["reponses"])
	}
	if _, err := s.Supprimer(b.ID, "test"); err != nil {
		t.Fatal(err)
	}
	e, _ = s.Lire(se.ID)
	if l := modele.LireReponses(e.Data["reponses"]); len(l) != 1 || l[0].PJ != a.ID {
		t.Fatalf("après suppression du PJ : %+v", e.Data["reponses"])
	}
	e, _ = s.Repondre(se.ID, a.ID, "", "lea")
	if _, ok := e.Data["reponses"]; ok {
		t.Fatalf("réponse retirée : %v", e.Data["reponses"])
	}
}
