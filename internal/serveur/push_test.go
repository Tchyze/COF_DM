package serveur

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Tchyze/cof_dm/internal/comptes"
)

// pousseurEssai : un pousseur sans envoi réel ni boucle ; les lots restent dans la file, à lire.
func pousseurEssai(reg *comptes.Registre, heure *time.Time) *pousseur {
	return &pousseur{prive: "prive", public: "cle-publique", sujet: "https://cof.exemple", comptes: reg,
		file: make(chan []envoiPush, 64), horloge: func() time.Time { return *heure }}
}

func lotsDe(p *pousseur) [][]envoiPush {
	var out [][]envoiPush
	for {
		select {
		case l := <-p.file:
			out = append(out, l)
		default:
			return out
		}
	}
}

func textePush(t *testing.T, e envoiPush) map[string]string {
	t.Helper()
	var v map[string]string
	if err := json.Unmarshal(e.charge, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func abonnementEssai(n string) map[string]any {
	return map[string]any{"nom": "Android · Chrome", "abonnement": map[string]any{"endpoint": "https://fcm.googleapis.com/fcm/send/" + n,
		"expirationTime": nil, "keys": map[string]any{"p256dh": "BCle" + n, "auth": "auth" + n}}}
}

// Notifications sur les appareils : inscription (relais connus seulement), envoi neutre au bon compte,
// heures calmes (tout part à 8 h, en un message), genres coupés, abonnement périmé retiré.
func TestNotificationsAppareils(t *testing.T) {
	b, reg := bancServeur(t)
	heure := heureParis(t, "2099-03-04 15:00")
	b.s.opt.push = pousseurEssai(reg, &heure)
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
	creer("pj", "Brom", map[string]any{"campagne": marche, "joueur": "Hugo"})
	b.proxy("POST", "/api/entites/"+ftoa(arwen)+"/partage", map[string]any{"partage": true}, mj)
	m1 := creer("seance", "Séance secrète", map[string]any{"campagne": marche, "date_reelle": "2099-03-06", "heure": "20:00", "lieu_reel": "Chez Marc"})
	reg.Creer("joueuse", "mot-de-passe-esueuoj", false)
	reg.Modifier("joueuse", func(c *comptes.Compte) error {
		c.Admis = append(c.Admis, b.s.store.ID())
		c.Roles[b.s.store.ID()] = comptes.Joueur
		c.Persos[b.s.store.ID()] = int64(arwen)
		return nil
	})
	joueuse := b.connecter("joueuse")

	// inscription : un relais de navigateur, en HTTPS, rien d'autre
	for _, mauvais := range []string{"http://fcm.googleapis.com/x", "https://exemple.org/x", "https://fcm.googleapis.com.exemple.org/x"} {
		ab := abonnementEssai("x")
		ab["abonnement"].(map[string]any)["endpoint"] = mauvais
		if c, _, _ := b.proxy("POST", "/api/compte/notifications/appareils", ab, joueuse); c != 422 {
			t.Fatalf("abonnement %s : %d", mauvais, c)
		}
	}
	c, v, _ := b.proxy("POST", "/api/compte/notifications/appareils", abonnementEssai("lea"), joueuse)
	if c != 200 || v["ici"] == "" || v["cle"] != "cle-publique" {
		t.Fatalf("inscription : %d %v", c, v)
	}
	if corps, _ := json.Marshal(v); strings.Contains(string(corps), "fcm.googleapis") || strings.Contains(string(corps), "authlea") {
		t.Fatalf("l'adresse ou les clés de l'appareil reviennent : %s", corps)
	}
	if c, _, _ := b.proxy("POST", "/api/compte/notifications/appareils", abonnementEssai("mj"), mj); c != 200 {
		t.Fatalf("inscription du MJ : %d", c)
	}
	if c, _, _ := b.proxy("POST", "/api/compte/notifications/appareils", abonnementEssai("chef"), b.connecter("chef")); c != 403 {
		t.Fatalf("inscription d'un administrateur : %d", c)
	}
	lotsDe(b.s.opt.push)

	// la relance du MJ sonne chez la joueuse, avec un texte neutre et un lien vers sa page
	if c, _, _ := b.proxy("POST", "/api/agenda/"+ftoa(m1)+"/relance", nil, mj); c != 200 {
		t.Fatalf("relance : %d", c)
	}
	lots := lotsDe(b.s.opt.push)
	if len(lots) != 1 || len(lots[0]) != 1 || lots[0][0].compte != "joueuse" {
		t.Fatalf("envois de la relance : %+v", lots)
	}
	m := textePush(t, lots[0][0])
	if m["titre"] != "COF DM" || m["texte"] != "Le MJ attend votre réponse" || !strings.HasPrefix(m["lien"], "/joueur.html?univers=") {
		t.Fatalf("message : %v", m)
	}
	if strings.Contains(string(lots[0][0].charge), "secrète") || strings.Contains(string(lots[0][0].charge), "Marc") {
		t.Fatalf("un détail de la séance part chez le relais : %s", lots[0][0].charge)
	}
	// sa réponse sonne chez le MJ
	b.proxy("POST", "/api/joueur/agenda/"+ftoa(m1)+"/reponse", map[string]any{"reponse": "présent"}, joueuse)
	lots = lotsDe(b.s.opt.push)
	if len(lots) != 1 || lots[0][0].compte != "meneur" || textePush(t, lots[0][0])["texte"] != "Un joueur a répondu" || !strings.HasPrefix(textePush(t, lots[0][0])["lien"], "/?univers=") {
		t.Fatalf("envoi de la réponse : %+v", lots)
	}

	// heures calmes : rien ne part à 23 h ; à 8 h, un seul message pour les deux
	heure = heureParis(t, "2099-03-04 23:00")
	b.proxy("POST", "/api/agenda/"+ftoa(m1)+"/relance", nil, mj)
	b.proxy("POST", "/api/joueur/agenda/"+ftoa(m1)+"/reponse", map[string]any{"reponse": ""}, joueuse)
	b.proxy("POST", "/api/agenda/"+ftoa(m1)+"/relance", nil, mj)
	b.proxy("POST", "/api/agenda/"+ftoa(m1)+"/relance", nil, mj)
	b.s.pousser(heureParis(t, "2099-03-05 07:59"))
	if lots := lotsDe(b.s.opt.push); len(lots) != 0 {
		t.Fatalf("envoi en heures calmes : %+v", lots)
	}
	b.s.pousser(heureParis(t, "2099-03-05 08:00"))
	lots = lotsDe(b.s.opt.push)
	textes := map[string]string{}
	for _, l := range lots {
		for _, e := range l {
			textes[e.compte] = textePush(t, e)["texte"]
		}
	}
	if textes["joueuse"] != "2 nouvelles notifications" || textes["meneur"] != "Un joueur a répondu" {
		t.Fatalf("après les heures calmes : %v", textes)
	}

	// la joueuse coupe les relances : la cloche les garde, l'appareil ne sonne plus
	heure = heureParis(t, "2099-03-04 15:00")
	if c, v, _ := b.proxy("PUT", "/api/compte/notifications/reglages", map[string]any{"coupees": []string{"relance", "inconnu"}}, joueuse); c != 200 || len(v["coupees"].([]any)) != 1 {
		t.Fatalf("réglages : %d %v", c, v)
	}
	b.proxy("POST", "/api/agenda/"+ftoa(m1)+"/relance", nil, mj)
	if lots := lotsDe(b.s.opt.push); len(lots) != 0 {
		t.Fatalf("relance coupée : %+v", lots)
	}

	// l'essai, à la demande
	if c, v, _ := b.proxy("POST", "/api/compte/notifications/essai", nil, joueuse); c != 200 || v["envoyes"] != float64(1) {
		t.Fatalf("essai : %d %v", c, v)
	}
	essai := lotsDe(b.s.opt.push)
	// le relais dit que l'abonnement n'existe plus : l'appareil est retiré
	p := b.s.opt.push
	p.envoyer = func(context.Context, comptes.Appareil, []byte) (int, error) { return http.StatusGone, nil }
	p.un(essai[0][0])
	if c, _ := reg.Lire("joueuse"); len(c.Appareils) != 0 {
		t.Fatalf("appareil périmé gardé : %+v", c.Appareils)
	}
	// le MJ retire le sien
	cm, _ := reg.Lire("meneur")
	if c, v, _ := b.proxy("DELETE", "/api/compte/notifications/appareils/"+cm.Appareils[0].ID, nil, mj); c != 200 || len(v["appareils"].([]any)) != 0 {
		t.Fatalf("retrait : %d %v", c, v)
	}
	// l'administrateur voit les appareils d'un compte, jamais leur adresse
	b.proxy("POST", "/api/compte/notifications/appareils", abonnementEssai("mj2"), mj)
	cm, _ = reg.Lire("meneur")
	if corps, _ := json.Marshal(vueAdmin(cm)); !strings.Contains(string(corps), "Android · Chrome") || strings.Contains(string(corps), "googleapis") {
		t.Fatalf("vue de l'administrateur : %s", corps)
	}
}

// Sans adresse publique en HTTPS, pas de notifications sur les appareils (la cloche reste).
func TestNotificationsAppareilsSansOrigine(t *testing.T) {
	if nouveauPousseur(nil, nil, "") != nil {
		t.Fatal("pousseur sans comptes")
	}
	b, reg := bancServeur(t)
	if p := nouveauPousseur(b.s.reg, reg, "http://cof.exemple"); p != nil {
		t.Fatal("pousseur sans HTTPS")
	}
	mj := b.connecter("meneur")
	c, v, _ := b.proxy("GET", "/api/compte/notifications", nil, mj)
	if c != 200 || v["possible"] != false {
		t.Fatalf("sans origine : %d %v", c, v)
	}
	if c, _, _ := b.proxy("POST", "/api/compte/notifications/appareils", abonnementEssai("x"), mj); c != 409 {
		t.Fatalf("inscription sans origine : %d", c)
	}
}

func TestHeuresCalmes(t *testing.T) {
	fz, _ := time.LoadLocation("Europe/Paris")
	for h, calme := range map[string]bool{"21:59": false, "22:00": true, "03:00": true, "07:59": true, "08:00": false, "12:00": false} {
		if heuresCalmes(heureParis(t, "2099-03-04 "+h), fz) != calme {
			t.Errorf("%s : %v attendu", h, calme)
		}
	}
}
