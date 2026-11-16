package serveur

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Tchyze/cof_dm/internal/comptes"
)

// banc d'agenda : deux campagnes, Léa joue dans les deux (son nom écrit deux fois, à la casse près),
// Hugo dans la première seulement.
func bancAgenda(t *testing.T) (b *banc, marche, ailleurs float64) {
	b = nouveau(t)
	marche = b.fiche("campagne", "La Marche", nil)
	ailleurs = b.fiche("campagne", "Ailleurs", nil)
	b.fiche("pj", "Arwen", map[string]any{"campagne": marche, "joueur": "Léa"})
	b.fiche("pj", "Cid", map[string]any{"campagne": marche, "joueur": "Hugo", "statut": "actif"})
	b.fiche("pj", "Brom", map[string]any{"campagne": ailleurs, "joueur": "léa "})
	b.fiche("pj", "Dara", map[string]any{"campagne": ailleurs, "joueur": "Hugo", "statut": "mort"})
	return
}

func seancesDe(t *testing.T, v map[string]any) map[string]map[string]any {
	t.Helper()
	out := map[string]map[string]any{}
	l, _ := v["seances"].([]any)
	for _, x := range l {
		m := x.(map[string]any)
		out[m["nom"].(string)] = m
	}
	return out
}

func resumeConflits(m map[string]any) string {
	var l []string
	for _, x := range m["conflits"].([]any) {
		c := x.(map[string]any)
		var p []string
		for _, y := range c["personnes"].([]any) {
			p = append(p, y.(string))
		}
		l = append(l, c["nom"].(string)+" "+c["genre"].(string)+" "+strings.Join(p, ","))
	}
	return strings.Join(l, " | ")
}

func TestAgendaConflits(t *testing.T) {
	b, marche, ailleurs := bancAgenda(t)
	b.fiche("seance", "M1", map[string]any{"campagne": marche, "date_reelle": "2027-03-06", "heure": "14:00", "duree": "4 h"})
	b.fiche("seance", "A1", map[string]any{"campagne": ailleurs, "date_reelle": "2027-03-06", "heure": "17:00", "duree": "3 h"})
	b.fiche("seance", "A2", map[string]any{"campagne": ailleurs, "date_reelle": "2027-03-06", "heure": "15:00", "duree": "1 h", "statut": "annulée"})
	b.fiche("seance", "M2", map[string]any{"campagne": marche, "date_reelle": "2027-03-06", "heure": "20:30", "duree": "2 h"})
	b.fiche("seance", "Sans date", map[string]any{"campagne": marche})
	c, v := b.local("GET", "/api/agenda", nil)
	if c != 200 || v["fuseau"] != "Europe/Paris" {
		t.Fatalf("agenda : %d %v", c, v)
	}
	s := seancesDe(t, v)
	if len(s) != 4 || s["Sans date"] != nil {
		t.Fatalf("séances datées : %v", v["seances"])
	}
	// le MJ et Léa à M1 et A1 ; Hugo n'est attendu qu'à M1 (son PJ d'Ailleurs est mort)
	attendu := map[string]string{
		"M1": "A1 chevauchement Léa,le MJ",
		"A1": "M1 chevauchement léa,le MJ | M2 rapproche léa,le MJ", // le nom tel qu'il est écrit dans sa campagne
		"M2": "A1 rapproche Léa,le MJ",
		"A2": "",
	}
	for nom, a := range attendu {
		if got := resumeConflits(s[nom]); got != a {
			t.Errorf("%s : %q, attendu %q", nom, got, a)
		}
	}
	if s["M1"]["debut"] != "2027-03-06T14:00:00+01:00" || s["M1"]["fin"] != "2027-03-06T18:00:00+01:00" {
		t.Errorf("horaires : %v → %v", s["M1"]["debut"], s["M1"]["fin"])
	}
	if p := s["M1"]["participants"].([]any); len(p) != 2 {
		t.Errorf("participants : %v", p)
	}
	// le numéro qui suit, pour planifier la suivante
	if _, v := b.local("GET", "/api/agenda?campagne="+ftoa(marche), nil); v["numero_suivant"] != float64(1) {
		t.Errorf("numéro suivant : %v", v["numero_suivant"])
	}
	// jouée, M1 cite ceux qui y étaient, plus les PJ actifs d'aujourd'hui
	cid := s["M1"]["participants"].([]any)[1].(map[string]any)["pj"]
	if c, v := b.local("PATCH", "/api/entites/"+ftoa(s["M1"]["id"].(float64)), map[string]any{"data": map[string]any{"statut": "jouée", "presents": []any{cid}}}); c != 200 {
		t.Fatalf("séance jouée : %d %v", c, v)
	}
	_, v = b.local("GET", "/api/agenda", nil)
	if p := seancesDe(t, v)["M1"]["participants"].([]any); len(p) != 1 || p[0].(map[string]any)["nom"] != "Cid" {
		t.Errorf("présents d'une séance jouée : %v", p)
	}
	// avant d'enregistrer : une séance d'Ailleurs essayée à 13 h gêne M1 (Léa, le MJ)
	c, v = b.local("GET", "/api/agenda/conflits?date=2027-03-06&heure=13:00&duree=2%20h&campagne="+ftoa(ailleurs), nil)
	l := []map[string]any{}
	json.Unmarshal([]byte(v["_liste"].(string)), &l)
	if c != 200 || len(l) != 1 || l[0]["nom"] != "M1" || l[0]["genre"] != "chevauchement" {
		t.Fatalf("conflits essayés : %d %v", c, v)
	}
	// déplacée (elle-même mise de côté), M1 ne se gêne pas
	c, v = b.local("GET", "/api/agenda/conflits?date=2027-03-06&heure=14:00&duree=4%20h&campagne="+ftoa(marche)+"&seance="+ftoa(s["M1"]["id"].(float64)), nil)
	if c != 200 || !strings.Contains(v["_liste"].(string), `"A1"`) || strings.Contains(v["_liste"].(string), `"M1"`) {
		t.Fatalf("séance déplacée : %d %v", c, v)
	}
	if c, _ := b.local("GET", "/api/agenda/conflits?heure=13:00", nil); c != 400 {
		t.Fatalf("sans date : %d", c)
	}
}

func ftoa(f float64) string { b, _ := json.Marshal(f); return string(b) }

// Le fichier .ics : ce qu'il faut pour s'organiser, jamais une note du MJ ; les heures en UTC.
func TestAgendaICS(t *testing.T) {
	b, marche, _ := bancAgenda(t)
	b.fiche("seance", "Séance 3", map[string]any{"campagne": marche, "date_reelle": "2027-07-10", "heure": "20:00",
		"duree": "4 h", "lieu_reel": "Chez Léa", "visio": "https://visio.example/table", "infos_joueurs": "Apportez vos dés",
		"notes": "SECRET-NOTES", "preparation": "SECRET-PREPA", "resume": "RESUME-MJ"})
	b.fiche("seance", "Zéro", map[string]any{"campagne": marche, "date_reelle": "2027-07-11", "nature": "séance zéro", "statut": "annulée"})
	r := httptest.NewRequest("GET", "/api/agenda/ics", nil)
	r.Host, r.RemoteAddr = "127.0.0.1:8100", "127.0.0.1:5000"
	w := httptest.NewRecorder()
	b.s.ServeHTTP(w, r)
	corps := strings.ReplaceAll(w.Body.String(), "\r\n ", "")
	if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/calendar") {
		t.Fatalf("ics : %d %s", w.Code, w.Header().Get("Content-Type"))
	}
	for _, x := range []string{"SUMMARY:Séance 3 · La Marche", "DTSTART:20270710T180000Z", "DTEND:20270710T220000Z",
		"LOCATION:Chez Léa", "URL:https://visio.example/table", "Apportez vos dés", "Léa (Arwen)", "Hugo (Cid)",
		"SUMMARY:ANNULÉE — Zéro · La Marche", "STATUS:CANCELLED", "DTSTART;VALUE=DATE:20270711", "Séance zéro"} {
		if !strings.Contains(corps, x) {
			t.Errorf("manque %q", x)
		}
	}
	for _, interdit := range []string{"SECRET-NOTES", "SECRET-PREPA", "RESUME-MJ"} {
		if strings.Contains(corps, interdit) {
			t.Errorf("%s dans le fichier .ics", interdit)
		}
	}
}

func TestAgendaFuseau(t *testing.T) {
	b, marche, _ := bancAgenda(t)
	b.fiche("seance", "S", map[string]any{"campagne": marche, "date_reelle": "2027-07-10", "heure": "20:00", "duree": "1 h"})
	if c, _ := b.local("PUT", "/api/agenda/reglages", map[string]any{"fuseau": "Lune/Mer"}); c != 422 {
		t.Fatalf("fuseau inconnu : %d", c)
	}
	if c, v := b.local("PUT", "/api/agenda/reglages", map[string]any{"fuseau": "America/Montreal"}); c != 200 || v["fuseau"] != "America/Montreal" {
		t.Fatalf("fuseau : %d %v", c, v)
	}
	_, v := b.local("GET", "/api/agenda", nil)
	if s := seancesDe(t, v)["S"]; s["debut"] != "2027-07-10T20:00:00-04:00" {
		t.Fatalf("heure à Montréal : %v", s["debut"])
	}
}

// Coupé, l'agenda répond 404 ; les séances, elles, restent (cœur).
func TestFonctionnalitesAgendaCoupe(t *testing.T) {
	b, marche, _ := bancAgenda(t)
	if c, _ := b.local("PUT", "/api/fonctionnalites", map[string]bool{"agenda": false}); c != 200 {
		t.Fatal(c)
	}
	for _, u := range []string{"/api/agenda", "/api/agenda/ics", "/api/agenda/conflits?date=2027-01-01", "/api/agenda/reglages"} {
		if c, _ := b.local("GET", u, nil); c != 404 {
			t.Errorf("%s : %d", u, c)
		}
	}
	b.fiche("seance", "S", map[string]any{"campagne": marche, "date_reelle": "2027-07-10"})
}

// Page des joueurs, à la maison : un joueur voit les séances à venir de ses campagnes (celles de tous
// les PJ qu'il joue, à son nom), en lecture, sans rien de réservé au MJ ni conflit ; sans PJ, rien.
func TestAgendaJoueursLibre(t *testing.T) {
	b, _ := bancReseau(t)
	marche := b.fiche("campagne", "La Marche", nil)
	ailleurs := b.fiche("campagne", "Ailleurs", nil)
	autre := b.fiche("campagne", "Sans Léa", nil)
	arwen := b.fiche("pj", "Arwen", map[string]any{"campagne": marche, "joueur": "Léa"})
	b.fiche("pj", "Brom", map[string]any{"campagne": ailleurs, "joueur": "léa"})
	b.fiche("pj", "Vieux", map[string]any{"campagne": autre, "joueur": "Léa", "statut": "mort"})
	cid := b.fiche("pj", "Cid", map[string]any{"campagne": marche, "joueur": "Hugo"})
	b.fiche("seance", "M1", map[string]any{"campagne": marche, "date_reelle": "2099-03-06", "heure": "14:00", "duree": "4 h",
		"lieu_reel": "Chez Léa", "infos_joueurs": "Apportez vos dés ; on part de [[Château secret|la tour]]", "notes": "SECRET-NOTES", "preparation": "SECRET-PREPA"})
	b.fiche("seance", "A1", map[string]any{"campagne": ailleurs, "date_reelle": "2099-03-06", "heure": "16:00", "duree": "3 h"})
	b.fiche("seance", "S1", map[string]any{"campagne": autre, "date_reelle": "2099-03-07"})
	b.fiche("seance", "Passée", map[string]any{"campagne": marche, "date_reelle": "2020-01-04", "heure": "20:00"})
	if c, _ := b.joueurLibre("GET", "/api/joueur/agenda?pj="+ftoa(arwen), nil, false); c != 401 {
		t.Fatalf("page fermée : %d", c)
	}
	if c, _ := b.local("POST", "/api/acces/joueurs", map[string]any{"active": true}); c != 200 {
		t.Fatal(c)
	}
	c, corps := b.joueurLibre("GET", "/api/joueur/agenda?pj="+ftoa(arwen), nil, false)
	if c != 200 || !strings.Contains(corps, `"M1"`) || !strings.Contains(corps, `"A1"`) || strings.Contains(corps, `"S1"`) ||
		strings.Contains(corps, "Passée") || !strings.Contains(corps, "Apportez vos dés") || !strings.Contains(corps, "Chez Léa") {
		t.Fatalf("agenda de Léa : %d %s", c, corps)
	}
	for _, interdit := range []string{"SECRET", "conflits", "chevauchement", "Château", "[["} {
		if strings.Contains(corps, interdit) {
			t.Errorf("%q dans l'agenda d'un joueur : %s", interdit, corps)
		}
	}
	if c, corps := b.joueurLibre("GET", "/api/joueur/agenda?pj="+ftoa(cid), nil, false); c != 200 || !strings.Contains(corps, `"M1"`) || strings.Contains(corps, `"A1"`) {
		t.Fatalf("agenda de Hugo : %d %s", c, corps)
	}
	if c, _ := b.joueurLibre("GET", "/api/joueur/agenda", nil, false); c != 404 {
		t.Fatalf("sans PJ : %d", c)
	}
	c, corps = b.joueurLibre("GET", "/api/joueur/agenda/ics?pj="+ftoa(arwen), nil, false)
	corps = strings.ReplaceAll(corps, "\r\n ", "")
	if c != 200 || !strings.Contains(corps, "SUMMARY:M1 · La Marche") || !strings.Contains(corps, "SUMMARY:A1 · Ailleurs") || strings.Contains(corps, "S1") || strings.Contains(corps, "SECRET") {
		t.Fatalf("ics de Léa : %d %s", c, corps)
	}
	// l'agenda coupé, la page ne le sert plus
	b.local("PUT", "/api/fonctionnalites", map[string]bool{"agenda": false})
	if c, _ := b.joueurLibre("GET", "/api/joueur/agenda?pj="+ftoa(arwen), nil, false); c != 404 {
		t.Fatalf("agenda coupé : %d", c)
	}
}

// Sur un serveur, un compte joueur ne voit que les séances de la campagne de son PJ, quel que soit le
// PJ demandé ; il n'atteint pas l'agenda du MJ.
func TestAgendaCompteJoueur(t *testing.T) {
	b, reg := bancServeur(t)
	mj := b.connecter("meneur")
	creer := func(typ, nom string, data map[string]any) float64 {
		c, v, _ := b.proxy("POST", "/api/entites", map[string]any{"type": typ, "nom": nom, "data": data}, mj)
		if c != 201 {
			t.Fatalf("création de %s : %d %v", nom, c, v)
		}
		return v["id"].(float64)
	}
	marche := creer("campagne", "La Marche", nil)
	ailleurs := creer("campagne", "Ailleurs", nil)
	arwen := creer("pj", "Arwen", map[string]any{"campagne": marche, "joueur": "Léa"})
	brom := creer("pj", "Brom", map[string]any{"campagne": ailleurs, "joueur": "Léa"})
	b.proxy("POST", "/api/entites/"+ftoa(arwen)+"/partage", map[string]any{"partage": true}, mj)
	creer("seance", "M1", map[string]any{"campagne": marche, "date_reelle": "2099-03-06", "heure": "14:00", "notes": "SECRET-NOTES"})
	creer("seance", "A1", map[string]any{"campagne": ailleurs, "date_reelle": "2099-03-06", "heure": "20:00"})
	reg.Creer("joueuse", "mot-de-passe-esueuoj", false)
	reg.Modifier("joueuse", func(c *comptes.Compte) error {
		c.Admis = append(c.Admis, b.s.store.ID())
		c.Roles[b.s.store.ID()] = comptes.Joueur
		c.Persos[b.s.store.ID()] = int64(arwen)
		return nil
	})
	joueuse := b.connecter("joueuse")
	for _, u := range []string{"/api/joueur/agenda", "/api/joueur/agenda?pj=" + ftoa(brom)} {
		c, v, _ := b.proxy("GET", u, nil, joueuse)
		corps, _ := json.Marshal(v)
		if c != 200 || !strings.Contains(string(corps), `"M1"`) || strings.Contains(string(corps), `"A1"`) || strings.Contains(string(corps), "SECRET") {
			t.Fatalf("%s : %d %s", u, c, corps)
		}
	}
	if c, _, _ := b.proxy("GET", "/api/agenda", nil, joueuse); c != 403 {
		t.Fatalf("agenda du MJ par un joueur : %d", c)
	}
}
