package serveur

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/Tchyze/cof_dm/internal/comptes"
)

// surveillance : les droits de cet univers sont revus souvent, le temps du test.
const surveillance = 20 * time.Millisecond

// ecranEssai : un écran branché, dont un lecteur guette la coupure.
type ecranEssai struct {
	conn  *websocket.Conn
	coupe chan struct{}
}

func suivre(t *testing.T, conn *websocket.Conn) *ecranEssai {
	e := &ecranEssai{conn, make(chan struct{})}
	go func() {
		defer close(e.coupe)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()
	t.Cleanup(func() { conn.Close() })
	return e
}

// ecranDirect : un écran branché avec la session (ou le jeton) de ce navigateur.
func ecranDirect(t *testing.T, ts *httptest.Server, n *navigateur, chemin string) *ecranEssai {
	t.Helper()
	h := http.Header{}
	if n.jeton != "" {
		h.Set("Authorization", "Bearer "+n.jeton)
	}
	for _, c := range n.cookies {
		h.Add("Cookie", c.Name+"="+c.Value)
	}
	conn, rep, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(ts.URL, "http")+chemin, h)
	if err != nil {
		code := 0
		if rep != nil {
			code = rep.StatusCode
		}
		t.Fatalf("branchement de %s : %v (%d)", chemin, err, code)
	}
	return suivre(t, conn)
}

// coupe : l'écran est coupé en quelques secondes, sans avoir rien envoyé.
func coupe(t *testing.T, e *ecranEssai, quoi string) {
	t.Helper()
	select {
	case <-e.coupe:
	case <-time.After(6 * time.Second):
		t.Fatalf("%s : l'écran n'a pas été coupé", quoi)
	}
}

// resteBranche : rien ne le coupe tant que rien ne change.
func resteBranche(t *testing.T, e *ecranEssai) {
	t.Helper()
	select {
	case <-e.coupe:
		t.Fatal("écran coupé sans raison")
	case <-time.After(150 * time.Millisecond):
	}
}

func bancSuivi(t *testing.T) (*Application, *comptes.Registre, func(string) *navigateur, *httptest.Server, string) {
	t.Helper()
	a, reg, connecter := serveurMulti(t)
	a.srv.intervalleDroits.Store(int64(surveillance))
	ts := httptest.NewServer(a)
	t.Cleanup(ts.Close)
	return a, reg, connecter, ts, a.st.ID()
}

func combatEnCours(t *testing.T, mj *navigateur) int64 {
	t.Helper()
	c, v := mj.faire("POST", "/api/combats", map[string]any{"nom": "Au gué"})
	if c != 201 {
		t.Fatalf("combat : %d %v", c, v)
	}
	return idLance(v)
}

// Le suivi de la page des joueurs : coupé dès que le compte perd son accès à l'univers, change de PJ
// ou de mot de passe, ferme sa session, ou que son compte est désactivé.
func TestDirectJoueurDroitsRevus(t *testing.T) {
	_, reg, connecter, ts, id := bancSuivi(t)
	const chemin = "/api/joueur/direct"
	joueur := func(nom string) *navigateur {
		n := connecter(nom)
		donner(reg, nom, id, comptes.Joueur)
		return n
	}
	for quoi, retirer := range map[string]func(string){
		"retiré de l'univers": func(nom string) {
			reg.Modifier(nom, func(c *comptes.Compte) error { delete(c.Roles, id); return nil })
		},
		"compte désactivé": func(nom string) {
			reg.Modifier(nom, func(c *comptes.Compte) error { c.Inactif = true; return nil })
		},
		"autre PJ": func(nom string) {
			reg.Modifier(nom, func(c *comptes.Compte) error {
				if c.Persos == nil {
					c.Persos = map[string]int64{}
				}
				c.Persos[id] = 4242
				return nil
			})
		},
		"mot de passe changé": func(nom string) { _, _ = reg.ChangerMdp(nom, "un-tout-autre-secret", false) },
	} {
		t.Run(quoi, func(t *testing.T) {
			nom := "joueur" + fmt.Sprint(len(quoi))
			conn := ecranDirect(t, ts, joueur(nom), chemin)
			resteBranche(t, conn)
			retirer(nom)
			coupe(t, conn, quoi)
		})
	}
}

// Le suivi d'un combat : un ancien MJ ou auditeur ne garde pas les chiffres du combat.
func TestDirectCombatDroitsRevus(t *testing.T) {
	_, reg, connecter, ts, id := bancSuivi(t)
	mj, lectrice := connecter("meneur"), connecter("lectrice")
	donner(reg, "meneur", id, comptes.MJ)
	donner(reg, "lectrice", id, comptes.Auditeur)
	combat := combatEnCours(t, mj)
	chemin := fmt.Sprintf("/api/combats/%d/direct", combat)
	conn := ecranDirect(t, ts, lectrice, chemin)
	resteBranche(t, conn)
	reg.Modifier("lectrice", func(c *comptes.Compte) error { delete(c.Roles, id); return nil })
	coupe(t, conn, "auditeur retiré")

	// par un jeton : retiré, l'écran tombe
	j, cree, err := reg.NouveauJeton("meneur", "poste")
	if err != nil {
		t.Fatal(err)
	}
	conn = ecranDirect(t, ts, &navigateur{t: t, a: mj.a, jeton: j, cookies: map[string]*http.Cookie{}}, chemin)
	resteBranche(t, conn)
	if err := reg.RetirerJeton("meneur", cree.ID); err != nil {
		t.Fatal(err)
	}
	coupe(t, conn, "jeton retiré")

	// la session fermée
	conn = ecranDirect(t, ts, mj, chemin)
	resteBranche(t, conn)
	mj.faire("POST", "/api/acces/deconnexion", nil)
	coupe(t, conn, "session fermée")
}

// À la maison : la page des joueurs refermée, ou le réseau, coupe qui la suivait sans compte.
func TestDirectPageLibreDroitsRevus(t *testing.T) {
	b := nouvelleAppli(t, t.TempDir())
	s := b.a.srv
	s.intervalleDroits.Store(int64(surveillance))
	if err := s.reg.SetMeta(metaPageJoueurs, "1"); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(b.a)
	defer ts.Close()
	h := http.Header{}
	u := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/joueur/direct"
	conn, _, err := websocket.DefaultDialer.Dial(u, h)
	if err != nil {
		t.Fatal(err)
	}
	resteBranche(t, suivre(t, conn))

	// la page reste ouverte, mais vue comme un appareil du réseau : mêmes règles qu'à l'entrée
	r := httptest.NewRequest("GET", "/api/joueur/direct", nil)
	r.Host, r.RemoteAddr = "192.168.1.20:8100", "192.168.1.50:5000"
	encore := s.droitsDuSuivi(r)
	if !encore() {
		t.Fatal("page ouverte : l'appareil garde son écran")
	}
	if err := s.reg.SetMeta(metaPageJoueurs, "0"); err != nil {
		t.Fatal(err)
	}
	if encore() {
		t.Fatal("page refermée : l'appareil perd son écran")
	}
}

// Le relais du bureau : il tombe si le poste quitte le serveur, ou si le serveur lui retire ses droits.
func TestRelaisDroitsRevus(t *testing.T) {
	relais := func(t *testing.T) (*bancDirect, *ecranEssai) {
		d := nouveauBancDirect(t)
		d.a.srv.intervalleDroits.Store(int64(surveillance))
		d.b.a.srv.intervalleDroits.Store(int64(surveillance))
		_, v := d.b.local("POST", "/api/entites", map[string]any{"type": "pj", "nom": "Aldric"})
		pj := int64(v["id"].(float64))
		d.pjPret(pj)
		c, v := d.b.local("POST", "/api/combats", map[string]any{"nom": "Au gué", "pj": []int64{pj}})
		if c != 201 {
			t.Fatalf("combat : %d %v", c, v)
		}
		ecran := httptest.NewServer(d.b.a)
		t.Cleanup(ecran.Close)
		conn := ecranDirect(t, ecran, &navigateur{t: t, cookies: map[string]*http.Cookie{}}, fmt.Sprintf("/api/combats/%d/direct", idLance(v)))
		resteBranche(t, conn)
		return d, conn
	}
	t.Run("le poste quitte le serveur", func(t *testing.T) {
		d, conn := relais(t)
		if c, v := d.b.local("POST", "/api/univers/serveur/local", nil); c != 200 {
			t.Fatalf("mode local : %d %v", c, v)
		}
		coupe(t, conn, "relais après le départ du poste")
	})
	t.Run("le serveur retire ses droits", func(t *testing.T) {
		d, conn := relais(t)
		d.a.opt.Comptes.Modifier("meneur", func(c *comptes.Compte) error {
			delete(c.Roles, d.a.st.ID())
			return nil
		})
		coupe(t, conn, "relais après le retrait")
	})
}
