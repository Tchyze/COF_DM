package serveur

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Tchyze/cof_dm/internal/comptes"
)

func titresNotifs(t *testing.T, corps string) []string {
	t.Helper()
	var v struct {
		NonLues int `json:"non_lues"`
		L       []struct {
			Titre string `json:"titre"`
		} `json:"notifications"`
	}
	if err := json.Unmarshal([]byte(corps), &v); err != nil {
		t.Fatalf("%v : %s", err, corps)
	}
	out := []string{}
	for _, n := range v.L {
		out = append(out, n.Titre)
	}
	return out
}

func heureParis(t *testing.T, s string) time.Time {
	t.Helper()
	fz, _ := time.LoadLocation("Europe/Paris")
	d, err := time.ParseInLocation("2006-01-02 15:04", s, fz)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// Le veilleur : rappel la veille à 19 h (pas aux absents, une seule fois), séance déplacée, annulée,
// rétablie ; rien à la première visite d'une séance, ni pour une séance passée.
func TestNotificationsVeilleur(t *testing.T) {
	b, _ := bancReseau(t)
	marche := b.fiche("campagne", "La Marche", nil)
	arwen := b.fiche("pj", "Arwen", map[string]any{"campagne": marche, "joueur": "Léa"})
	hugo := b.fiche("pj", "Brom", map[string]any{"campagne": marche, "joueur": "Hugo"})
	m1 := b.fiche("seance", "Séance 12", map[string]any{"campagne": marche, "date_reelle": "2099-03-06", "heure": "20:00", "duree": "4 h", "lieu_reel": "Chez Marc", "notes": "SECRET-MJ"})
	b.local("POST", "/api/acces/joueurs", map[string]any{"active": true})
	cloche := func(pj float64) []string {
		c, corps := b.joueurLibre("GET", "/api/joueur/notifications?pj="+ftoa(pj), nil, true)
		if c != 200 {
			t.Fatalf("cloche : %d %s", c, corps)
		}
		if strings.Contains(corps, "SECRET") {
			t.Fatalf("une note du MJ dans la cloche : %s", corps)
		}
		return titresNotifs(t, corps)
	}
	// Hugo ne viendra pas
	if c, corps := b.joueurLibre("POST", "/api/joueur/agenda/"+ftoa(m1)+"/reponse?pj="+ftoa(hugo), map[string]any{"reponse": "absent"}, true); c != 200 {
		t.Fatalf("réponse : %d %s", c, corps)
	}
	b.s.veillerNotifications(heureParis(t, "2099-03-04 12:00")) // première visite : rien
	b.s.veillerNotifications(heureParis(t, "2099-03-05 18:59"))
	if l := cloche(arwen); len(l) != 0 {
		t.Fatalf("avant 19 h : %v", l)
	}
	b.s.veillerNotifications(heureParis(t, "2099-03-05 19:00"))
	b.s.veillerNotifications(heureParis(t, "2099-03-05 19:01"))
	if l := cloche(arwen); len(l) != 1 || l[0] != "Rappel : Séance 12, vendredi 6 mars à 20 h" {
		t.Fatalf("rappel : %v", l)
	}
	if l := cloche(hugo); len(l) != 0 {
		t.Fatalf("un absent reçoit le rappel : %v", l)
	}
	// déplacée : tout le monde est prévenu, et un nouveau rappel viendra pour la nouvelle date
	b.local("PATCH", "/api/entites/"+ftoa(m1), map[string]any{"data": map[string]any{"date_reelle": "2099-03-13", "heure": "20:30"}})
	b.s.veillerNotifications(heureParis(t, "2099-03-05 19:10"))
	if l := cloche(hugo); len(l) != 1 || l[0] != "Séance 12 est déplacée : vendredi 13 mars à 20 h 30" {
		t.Fatalf("déplacée : %v", l)
	}
	b.s.veillerNotifications(heureParis(t, "2099-03-12 19:00"))
	if l := cloche(arwen); len(l) != 3 || !strings.HasPrefix(l[0], "Rappel : Séance 12, vendredi 13 mars") {
		t.Fatalf("second rappel : %v", l)
	}
	// annulée, puis rétablie
	b.local("PATCH", "/api/entites/"+ftoa(m1), map[string]any{"data": map[string]any{"statut": "annulée"}})
	b.s.veillerNotifications(heureParis(t, "2099-03-12 20:00"))
	b.local("PATCH", "/api/entites/"+ftoa(m1), map[string]any{"data": map[string]any{"statut": "prévue"}})
	b.s.veillerNotifications(heureParis(t, "2099-03-12 21:00"))
	if l := cloche(hugo); len(l) != 3 || !strings.Contains(l[1], "annulée") || !strings.Contains(l[0], "rétablie") {
		t.Fatalf("annulée puis rétablie : %v", l)
	}
	// une séance passée ne prévient plus personne
	b.local("PATCH", "/api/entites/"+ftoa(m1), map[string]any{"data": map[string]any{"heure": "10:00"}})
	b.s.veillerNotifications(heureParis(t, "2099-03-20 12:00"))
	if l := cloche(hugo); len(l) != 3 {
		t.Fatalf("séance passée : %v", l)
	}
	// le MJ a été prévenu de la réponse de Hugo
	_, v := b.local("GET", "/api/notifications", nil)
	corps, _ := json.Marshal(v)
	if l := titresNotifs(t, string(corps)); len(l) != 1 || l[0] != "Hugo (Brom) : absent à Séance 12" {
		t.Fatalf("cloche du MJ : %v", l)
	}
	// marquer lues : les siennes seulement
	c, lu := b.joueurLibre("POST", "/api/joueur/notifications/lues?pj="+ftoa(arwen), map[string]any{}, true)
	if c != 200 || !strings.Contains(lu, `"non_lues":0`) {
		t.Fatalf("lues : %d %s", c, lu)
	}
	if _, corps := b.joueurLibre("GET", "/api/joueur/notifications?pj="+ftoa(hugo), nil, true); !strings.Contains(corps, `"non_lues":3`) {
		t.Fatalf("celles de Hugo touchées : %s", corps)
	}
	if _, v := b.local("GET", "/api/notifications", nil); v["non_lues"] != float64(1) {
		t.Fatalf("celles du MJ touchées : %v", v)
	}
}

// Agenda coupé : le veilleur ne dit rien.
func TestNotificationsAgendaCoupe(t *testing.T) {
	b, _ := bancReseau(t)
	marche := b.fiche("campagne", "La Marche", nil)
	arwen := b.fiche("pj", "Arwen", map[string]any{"campagne": marche})
	b.fiche("seance", "S", map[string]any{"campagne": marche, "date_reelle": "2099-03-06", "heure": "20:00"})
	b.local("PUT", "/api/fonctionnalites", map[string]any{"agenda": false})
	b.s.veillerNotifications(heureParis(t, "2099-03-05 20:00"))
	if l, _, _ := b.s.store.Notifications(10, int64(arwen)); len(l) != 0 {
		t.Fatalf("agenda coupé : %v", l)
	}
}

// Relance : seulement à ceux qui n'ont pas répondu ; une séance annulée ou passée se refuse.
func TestNotificationsRelance(t *testing.T) {
	b, _ := bancReseau(t)
	marche := b.fiche("campagne", "La Marche", nil)
	arwen := b.fiche("pj", "Arwen", map[string]any{"campagne": marche, "joueur": "Léa"})
	brom := b.fiche("pj", "Brom", map[string]any{"campagne": marche, "joueur": "Hugo"})
	m1 := b.fiche("seance", "M1", map[string]any{"campagne": marche, "date_reelle": "2099-03-06", "heure": "20:00"})
	annulee := b.fiche("seance", "M2", map[string]any{"campagne": marche, "date_reelle": "2099-03-20", "statut": "annulée"})
	passee := b.fiche("seance", "M0", map[string]any{"campagne": marche, "date_reelle": "2020-03-20"})
	b.local("POST", "/api/acces/joueurs", map[string]any{"active": true})
	b.joueurLibre("POST", "/api/joueur/agenda/"+ftoa(m1)+"/reponse?pj="+ftoa(arwen), map[string]any{"reponse": "présent"}, true)
	c, v := b.local("POST", "/api/agenda/"+ftoa(m1)+"/relance", nil)
	if c != 200 || v["relances"] != float64(1) {
		t.Fatalf("relance : %d %v", c, v)
	}
	if l, _, _ := b.s.store.Notifications(10, int64(brom)); len(l) != 1 || !strings.HasPrefix(l[0].Titre, "Le MJ attend votre réponse : M1") {
		t.Fatalf("relance reçue : %v", l)
	}
	if l, _, _ := b.s.store.Notifications(10, int64(arwen)); len(l) != 0 {
		t.Fatalf("relance à qui a répondu : %v", l)
	}
	for _, x := range []float64{annulee, passee} {
		if c, _ := b.local("POST", "/api/agenda/"+ftoa(x)+"/relance", nil); c != 422 {
			t.Fatalf("relance d'une séance qui n'est plus à venir : %d", c)
		}
	}
	if c, _ := b.local("POST", "/api/agenda/999999/relance", nil); c != 404 {
		t.Fatalf("relance d'une séance inconnue : %d", c)
	}
	// la page libre ne relance pas, ni ne lit la cloche du MJ
	for _, u := range []string{"/api/agenda/" + ftoa(m1) + "/relance", "/api/notifications/lues"} {
		if c, _ := b.joueurLibre("POST", u, map[string]any{}, true); c != 401 {
			t.Fatalf("%s par la page libre : %d", u, c)
		}
	}
	if c, _ := b.joueurLibre("GET", "/api/notifications", nil, true); c != 401 {
		t.Fatalf("cloche du MJ par la page libre : %d", c)
	}
}

// Sur un serveur : un joueur ne lit que la cloche du PJ de son compte, quel que soit celui qu'il
// nomme ; ni celle du MJ, ni la relance. L'auditeur lit la cloche du MJ sans pouvoir la marquer lue.
func TestNotificationsCompteJoueur(t *testing.T) {
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
	arwen := creer("pj", "Arwen", map[string]any{"campagne": marche, "joueur": "Léa"})
	brom := creer("pj", "Brom", map[string]any{"campagne": marche, "joueur": "Hugo"})
	b.proxy("POST", "/api/entites/"+ftoa(arwen)+"/partage", map[string]any{"partage": true}, mj)
	m1 := creer("seance", "M1", map[string]any{"campagne": marche, "date_reelle": "2099-03-06", "heure": "20:00"})
	reg.Creer("joueuse", "mot-de-passe-esueuoj", false)
	reg.Modifier("joueuse", func(c *comptes.Compte) error {
		c.Admis = append(c.Admis, b.s.store.ID())
		c.Roles[b.s.store.ID()] = comptes.Joueur
		c.Persos[b.s.store.ID()] = int64(arwen)
		return nil
	})
	joueuse := b.connecter("joueuse")
	if c, v, _ := b.proxy("POST", "/api/agenda/"+ftoa(m1)+"/relance", nil, mj); c != 200 || v["relances"] != float64(2) {
		t.Fatalf("relance du MJ : %d %v", c, v)
	}
	c, v, _ := b.proxy("GET", "/api/joueur/notifications?pj="+ftoa(brom), nil, joueuse)
	if c != 200 || len(v["notifications"].([]any)) != 1 || v["notifications"].([]any)[0].(map[string]any)["pour"] != arwen {
		t.Fatalf("cloche de la joueuse : %d %v", c, v)
	}
	if c, v, _ := b.proxy("POST", "/api/joueur/notifications/lues?pj="+ftoa(brom), map[string]any{}, joueuse); c != 200 || v["non_lues"] != float64(0) {
		t.Fatalf("lues : %d %v", c, v)
	}
	if l, n, _ := b.s.store.Notifications(10, int64(brom)); len(l) != 1 || n != 1 {
		t.Fatalf("celle de Brom touchée : %v %d", l, n)
	}
	for _, x := range []struct{ m, u string }{{"GET", "/api/notifications"}, {"POST", "/api/notifications/lues"}, {"POST", "/api/agenda/" + ftoa(m1) + "/relance"}, {"POST", "/api/notifications/message"}} {
		if c, _, _ := b.proxy(x.m, x.u, map[string]any{}, joueuse); c != 403 {
			t.Fatalf("%s %s par une joueuse : %d", x.m, x.u, c)
		}
	}
}

// Message du MJ : à tous les PJ actifs de la campagne, à sens unique ; ni vide, ni trop long ; la page
// libre ne l'envoie pas.
func TestNotificationsMessage(t *testing.T) {
	b, _ := bancReseau(t)
	marche := b.fiche("campagne", "La Marche", nil)
	autre := b.fiche("campagne", "Ailleurs", nil)
	arwen := b.fiche("pj", "Arwen", map[string]any{"campagne": marche})
	brom := b.fiche("pj", "Brom", map[string]any{"campagne": marche})
	mort := b.fiche("pj", "Mort", map[string]any{"campagne": marche, "statut": "mort"})
	loin := b.fiche("pj", "Loin", map[string]any{"campagne": autre})
	b.local("POST", "/api/acces/joueurs", map[string]any{"active": true})
	c, v := b.local("POST", "/api/notifications/message", map[string]any{"campagne": marche, "texte": "  Pensez à vos dés !\nÀ jeudi.  "})
	if c != 200 || v["envoyes"] != float64(2) {
		t.Fatalf("message : %d %v", c, v)
	}
	for _, pj := range []float64{arwen, brom} {
		l, _, _ := b.s.store.Notifications(10, int64(pj))
		if len(l) != 1 || l[0].Titre != "Message du MJ — La Marche" || l[0].Texte != "Pensez à vos dés !\nÀ jeudi." || l[0].Genre != "message" {
			t.Fatalf("reçu par %v : %+v", pj, l)
		}
	}
	for _, pj := range []float64{mort, loin} {
		if l, _, _ := b.s.store.Notifications(10, int64(pj)); len(l) != 0 {
			t.Fatalf("reçu par %v : %+v", pj, l)
		}
	}
	if l, _, _ := b.s.store.Notifications(10, 0); len(l) != 0 {
		t.Fatalf("copie dans la cloche du MJ : %+v", l)
	}
	for _, x := range []map[string]any{{"campagne": marche, "texte": "  "}, {"campagne": marche, "texte": strings.Repeat("é", 501)}} {
		if c, _ := b.local("POST", "/api/notifications/message", x); c != 422 {
			t.Fatalf("message refusé attendu : %d", c)
		}
	}
	if c, _ := b.local("POST", "/api/notifications/message", map[string]any{"campagne": arwen, "texte": "x"}); c != 404 {
		t.Fatalf("message à une fiche qui n'est pas une campagne : %d", c)
	}
	if c, _ := b.joueurLibre("POST", "/api/notifications/message", map[string]any{"campagne": marche, "texte": "x"}, true); c != 401 {
		t.Fatalf("message par la page libre : %d", c)
	}
}
