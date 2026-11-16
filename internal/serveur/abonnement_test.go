package serveur

import (
	"bytes"
	"log"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/Tchyze/cof_dm/internal/comptes"
	"github.com/Tchyze/cof_dm/internal/store"
)

// lireFlux : l'adresse d'abonnement lue comme le fait Google ou Apple : sans session ni cookie, d'une
// autre machine.
func lireFlux(a *Application, chemin string) (int, string) {
	r := httptest.NewRequest("GET", chemin, nil)
	r.Host, r.RemoteAddr = "jdr.exemple.org", "203.0.113.9:4000"
	w := httptest.NewRecorder()
	a.ServeHTTP(w, r)
	return w.Code, strings.ReplaceAll(w.Body.String(), "\r\n ", "")
}

func cheminDe(t *testing.T, adresse any) string {
	t.Helper()
	s, _ := adresse.(string)
	u, err := url.Parse(s)
	if err != nil || !strings.HasPrefix(u.Path, prefixeFlux) || !strings.HasSuffix(u.Path, ".ics") {
		t.Fatalf("adresse d'abonnement : %v", adresse)
	}
	return u.Path
}

// L'abonnement : une adresse par compte, lue sans session, qui réunit les séances de tous ses univers
// (joueur : la campagne de son PJ ; MJ : tout l'univers), sans rien de réservé au MJ. Ni l'auditeur
// ni l'administrateur ne s'abonnent ; l'administrateur coupe une adresse sans lire d'univers. Le jeton
// n'est écrit dans aucun journal.
func TestAbonnementAgenda(t *testing.T) {
	var journal bytes.Buffer
	log.SetOutput(&journal)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	a, reg, connecter := serveurMulti(t)
	chef, meneur, lea, lectrice := connecter("chef"), connecter("meneur"), connecter("lea"), connecter("lectrice")
	premier := a.st.ID()
	second := creerUnivers(t, chef, "Les Îles")
	donner(reg, "meneur", premier, comptes.MJ)
	donner(reg, "lectrice", premier, comptes.Auditeur)
	donner(reg, "lea", premier, comptes.Joueur)
	donner(reg, "lea", second, comptes.Joueur)
	creer := func(st *store.Store, typ, nom string, data map[string]any) int64 {
		e, err := st.Creer(typ, nom, data, nil, "test")
		if err != nil {
			t.Fatal(err)
		}
		return e.ID
	}
	s1, s2 := a.tous[premier].st, a.tous[second].st
	marche, autre, iles := creer(s1, "campagne", "La Marche", nil), creer(s1, "campagne", "Sans Léa", nil), creer(s2, "campagne", "Les Îles", nil)
	arwen := creer(s1, "pj", "Arwen", map[string]any{"campagne": marche, "joueur": "Léa"})
	brom := creer(s2, "pj", "Brom", map[string]any{"campagne": iles, "joueur": "Léa"})
	s1.Partager(arwen, true, "test")
	s2.Partager(brom, true, "test")
	reg.Modifier("lea", func(c *comptes.Compte) error { c.Persos[premier], c.Persos[second] = arwen, brom; return nil })
	creer(s1, "seance", "M1", map[string]any{"campagne": marche, "date_reelle": "2099-03-06", "heure": "20:00", "notes": "SECRET-NOTES", "preparation": "SECRET-PREPA"})
	creer(s1, "seance", "X1", map[string]any{"campagne": autre, "date_reelle": "2099-03-07", "heure": "20:00"})
	creer(s2, "seance", "I1", map[string]any{"campagne": iles, "date_reelle": "2099-03-08", "heure": "14:00"})
	creer(s1, "seance", "Vieille", map[string]any{"campagne": marche, "date_reelle": "2020-01-04", "heure": "20:00"})

	if c, v := lea.faire("GET", "/api/compte/agenda", nil); c != 200 || v["abonnable"] != true || v["adresse"] != nil {
		t.Fatalf("avant : %d %v", c, v)
	}
	c, v := lea.faire("POST", "/api/compte/agenda", nil)
	if c != 200 || !strings.HasPrefix(v["webcal"].(string), "webcal://") {
		t.Fatalf("création : %d %v", c, v)
	}
	chemin := cheminDe(t, v["adresse"])
	jeton := strings.TrimSuffix(strings.TrimPrefix(chemin, prefixeFlux), ".ics")
	if _, v2 := lea.faire("GET", "/api/compte/agenda", nil); v2["adresse"] != v["adresse"] {
		t.Fatalf("adresse remontrée : %v", v2)
	}
	code, corps := lireFlux(a, chemin)
	if code != 200 || !strings.Contains(corps, "SUMMARY:M1 · La Marche") || !strings.Contains(corps, "SUMMARY:I1 · Les Îles") ||
		strings.Contains(corps, "X1") || strings.Contains(corps, "Vieille") || strings.Contains(corps, "SECRET") {
		t.Fatalf("flux de Léa : %d %s", code, corps)
	}
	// le MJ : tout son univers, rien de l'autre, rien de secret
	_, v = meneur.faire("POST", "/api/compte/agenda", nil)
	code, corps = lireFlux(a, cheminDe(t, v["adresse"]))
	if code != 200 || !strings.Contains(corps, "M1") || !strings.Contains(corps, "X1") || strings.Contains(corps, "I1") || strings.Contains(corps, "SECRET") {
		t.Fatalf("flux du MJ : %d %s", code, corps)
	}
	// ni l'auditeur, ni l'administrateur
	if c, v := lectrice.faire("GET", "/api/compte/agenda", nil); c != 200 || v["abonnable"] != false {
		t.Fatalf("auditeur : %d %v", c, v)
	}
	if c, _ := lectrice.faire("POST", "/api/compte/agenda", nil); c != 403 {
		t.Fatalf("abonnement d'un auditeur : %d", c)
	}
	if c, _ := chef.faire("POST", "/api/compte/agenda", nil); c != 403 {
		t.Fatalf("abonnement de l'administrateur : %d", c)
	}
	// l'agenda coupé dans un univers, ses séances quittent le flux
	s2.ReglerFonctionnalites(map[string]bool{"agenda": false}, "test")
	if _, corps := lireFlux(a, chemin); strings.Contains(corps, "I1") || !strings.Contains(corps, "M1") {
		t.Fatalf("agenda coupé : %s", corps)
	}
	// l'administrateur voit qu'une adresse existe, jamais le jeton, et la coupe
	w := chef.brut("GET", "/api/admin/comptes", nil)
	if !strings.Contains(w.Body.String(), `"agenda"`) || strings.Contains(w.Body.String(), jeton) || strings.Contains(w.Body.String(), "empreinte") {
		t.Fatalf("comptes vus par l'administrateur : %s", w.Body.String())
	}
	if c, _ := meneur.faire("DELETE", "/api/admin/comptes/lea/agenda", nil); c != 403 {
		t.Fatalf("un MJ coupe l'adresse d'un joueur : %d", c)
	}
	if c, _ := chef.faire("DELETE", "/api/admin/comptes/lea/agenda", nil); c != 200 {
		t.Fatalf("retrait par l'administrateur : %d", c)
	}
	if code, _ := lireFlux(a, chemin); code != 404 {
		t.Fatalf("adresse retirée : %d", code)
	}
	// changée par son titulaire, l'ancienne ne vaut plus ; compte désactivé, plus rien
	_, v = lea.faire("POST", "/api/compte/agenda", nil)
	nouveau := cheminDe(t, v["adresse"])
	lea.faire("POST", "/api/compte/agenda", nil)
	if code, _ := lireFlux(a, nouveau); code != 404 {
		t.Fatalf("ancienne adresse : %d", code)
	}
	_, v = lea.faire("GET", "/api/compte/agenda", nil)
	courant := cheminDe(t, v["adresse"])
	reg.Modifier("lea", func(c *comptes.Compte) error { c.Inactif = true; return nil })
	if code, _ := lireFlux(a, courant); code != 404 {
		t.Fatalf("compte désactivé : %d", code)
	}
	for _, faux := range []string{"/agenda/", "/agenda/x.ics", "/agenda/cofag_abc.ics", "/agenda/" + strings.TrimPrefix(courant, prefixeFlux) + "/x"} {
		if code, _ := lireFlux(a, faux); code != 404 {
			t.Fatalf("%s : %d", faux, code)
		}
	}
	// aucun jeton dans les journaux
	for _, j := range []string{jeton, strings.TrimSuffix(strings.TrimPrefix(courant, prefixeFlux), ".ics")} {
		if strings.Contains(journal.String(), j) || strings.Contains(journal.String(), strings.TrimPrefix(j, "cofag_")) {
			t.Fatalf("jeton dans les journaux : %s", journal.String())
		}
	}
	// trop d'adresses inconnues depuis la même machine : 429
	code = 0
	for i := 0; i < essaisFlux+2 && code != 429; i++ {
		code, _ = lireFlux(a, "/agenda/cofag_inconnu.ics")
	}
	if code != 429 {
		t.Fatalf("limite des adresses inconnues : %d", code)
	}
}

// Le bureau n'a pas d'abonnement : le flux n'existe que sur un serveur.
func TestAbonnementBureau(t *testing.T) {
	b := nouveau(t)
	r := httptest.NewRequest("GET", "/agenda/cofag_x.ics", nil)
	r.Host, r.RemoteAddr = "127.0.0.1:8100", "127.0.0.1:5000"
	w := httptest.NewRecorder()
	b.s.ServeHTTP(w, r)
	if w.Code == 200 && strings.Contains(w.Body.String(), "VCALENDAR") {
		t.Fatalf("flux sur le bureau : %d", w.Code)
	}
}
