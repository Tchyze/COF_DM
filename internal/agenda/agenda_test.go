package agenda

import (
	"strings"
	"testing"
	"time"

	"github.com/Tchyze/cof_dm/internal/modele"
)

func paris(t *testing.T) *time.Location {
	t.Helper()
	l, err := modele.Fuseau("")
	if err != nil {
		t.Fatal(err)
	}
	return l
}

// Une heure écrite à Paris part en UTC avec le bon décalage, l'été comme l'hiver.
func TestPlacerHeureEte(t *testing.T) {
	l := paris(t)
	d, f, entier, ok := Placer("2027-07-10", "20:00", "4 h", l)
	if !ok || entier || d.UTC().Format("15:04") != "18:00" || f.UTC().Format("15:04") != "22:00" {
		t.Fatalf("été : %v → %v (%v, %v)", d.UTC(), f.UTC(), entier, ok)
	}
	d, _, _, _ = Placer("2027-12-10", "20h00", "4 h", l)
	if d.UTC().Format("15:04") != "19:00" {
		t.Fatalf("hiver : %v", d.UTC())
	}
	// la nuit du passage à l'heure d'hiver (31 octobre 2027 à 3 h) : 4 h écoulées, pas 4 h d'horloge
	d, f, _, _ = Placer("2027-10-30", "23:00", "4 h", l)
	if f.Sub(d) != 4*time.Hour || f.In(l).Format("15:04") != "02:00" {
		t.Fatalf("changement d'heure : %v → %v", d, f.In(l))
	}
}

func TestPlacerSansHeureNiDuree(t *testing.T) {
	l := paris(t)
	d, f, entier, ok := Placer("2027-03-06", "", "", l)
	if !ok || !entier || f.Sub(d) != 24*time.Hour {
		t.Fatalf("journée entière : %v → %v (%v)", d, f, entier)
	}
	d, f, _, _ = Placer("2027-03-06", "14:00", "", l)
	if f.Sub(d) != time.Duration(modele.DureeParDefaut)*time.Minute {
		t.Fatalf("durée par défaut : %v", f.Sub(d))
	}
	if _, _, _, ok := Placer("", "14:00", "1 h", l); ok {
		t.Fatal("sans date, pas de créneau")
	}
}

func TestDurees(t *testing.T) {
	if modele.DureesSeance[0] != "30 min" || modele.DureesSeance[2] != "1 h 30" || modele.DureesSeance[len(modele.DureesSeance)-1] != "12 h" {
		t.Fatalf("menu des durées : %v", modele.DureesSeance)
	}
	for i, s := range modele.DureesSeance {
		if m, ok := modele.MinutesDuree(s); !ok || m != 30*(i+1) {
			t.Fatalf("%q → %d", s, m)
		}
	}
}

func TestConflits(t *testing.T) {
	l := paris(t)
	c := func(id int64, date, heure, duree string, qui ...string) Creneau {
		d, f, e, _ := Placer(date, heure, duree, l)
		return Creneau{ID: id, Debut: d, Fin: f, JourEntier: e, Personnes: qui}
	}
	cs := []Creneau{
		c(1, "2027-03-06", "14:00", "4 h", "lea", "mj"),
		c(2, "2027-03-06", "17:00", "3 h", "lea"),         // chevauche 1 (Léa)
		c(3, "2027-03-06", "19:00", "2 h", "mj", "hugo"),  // une heure tout juste après 1 : rien ; chevauche 2 sans personne commune
		c(4, "2027-03-06", "20:30", "2 h", "lea"),         // 30 min après 2 : rapproché (Léa)
		c(5, "2027-03-06", "", "", "hugo"),                // toute la journée : chevauche 3 (Hugo)
		c(6, "2027-03-07", "20:00", "2 h", "lea", "hugo"), // le lendemain : rien
	}
	got := Conflits(cs)
	attendu := map[int64]string{1: "2 chevauchement lea", 2: "1 chevauchement lea|4 rapproche lea", 3: "5 chevauchement hugo",
		4: "2 rapproche lea", 5: "3 chevauchement hugo"}
	for id, a := range attendu {
		var l []string
		for _, x := range got[id] {
			l = append(l, strings.Join([]string{itoa(x.Autre), x.Genre, strings.Join(x.Personnes, ",")}, " "))
		}
		if strings.Join(l, "|") != a {
			t.Errorf("séance %d : %q, attendu %q", id, strings.Join(l, "|"), a)
		}
	}
	if len(got[6]) != 0 {
		t.Errorf("séance 6 : %v", got[6])
	}
}

func TestICS(t *testing.T) {
	l := paris(t)
	d, f, _, _ := Placer("2027-07-10", "20:00", "4 h", l)
	j, jf, _, _ := Placer("2027-07-11", "", "", l)
	long := strings.Repeat("Apportez vos dés, vos crayons ; et de quoi grignoter. ", 4)
	b := string(ICS("Ma table", []Evenement{
		{UID: "a@x", Titre: "Séance 3 · La campagne", Debut: d, Fin: f, Lieu: "Chez Léa", Description: long + "\nÀ bientôt"},
		{UID: "b@x", Titre: "Création", Debut: j, Fin: jf, JourEntier: true, Annulee: true},
	}, time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)))
	for _, x := range []string{"BEGIN:VCALENDAR\r\n", "DTSTART:20270710T180000Z\r\n", "DTEND:20270710T220000Z\r\n",
		"SUMMARY:Séance 3 · La campagne\r\n", "LOCATION:Chez Léa\r\n", "DTSTART;VALUE=DATE:20270711\r\n",
		"DTEND;VALUE=DATE:20270712\r\n", "STATUS:CANCELLED\r\n", "SUMMARY:ANNULÉE — Création\r\n", "END:VCALENDAR\r\n"} {
		if !strings.Contains(b, x) {
			t.Errorf("manque %q", x)
		}
	}
	for _, ligne := range strings.Split(b, "\r\n") {
		if len(ligne) > 75 {
			t.Errorf("ligne de %d octets : %q", len(ligne), ligne)
		}
	}
	// une fois dépliée, la description revient entière, échappée
	deplie := strings.ReplaceAll(b, "\r\n ", "")
	if !strings.Contains(deplie, `vos crayons \; et de quoi grignoter. `) || !strings.Contains(deplie, `\nÀ bientôt`) {
		t.Errorf("description : %s", deplie)
	}
}
