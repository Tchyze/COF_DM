package serveur

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// creer une fiche et renvoyer son identifiant.
func (b *banc) fiche(typ, nom string, data map[string]any) float64 {
	b.t.Helper()
	c, v := b.api("POST", "/api/entites", map[string]any{"type": typ, "nom": nom, "data": data})
	if c != 201 {
		b.t.Fatalf("création %s « %s » : %d %v", typ, nom, c, v)
	}
	return v["id"].(float64)
}

func TestTableVide(t *testing.T) {
	b := nouveau(t)
	c, v := b.local("GET", "/api/table", nil)
	if c != 200 || v["quete"] != nil || v["campagne"] != nil || len(v["pj"].([]any)) != 0 || len(v["scene"].([]any)) != 0 {
		t.Fatalf("monde vide : %d %v", c, v)
	}
	if c, _ := b.local("GET", "/api/table?quete=999", nil); c != 404 {
		t.Fatalf("quête inconnue : %d", c)
	}
}

func TestTable(t *testing.T) {
	b := nouveau(t)
	camp := b.fiche("campagne", "Les Terres d'Osgild", map[string]any{"statut": "en cours"})
	autre := b.fiche("campagne", "Ailleurs", map[string]any{"statut": "en pause"})
	lieu := b.fiche("lieu", "Mont-Veille", map[string]any{"impression": "Un fort battu par les vents."})
	baron := b.fiche("pnj", "Baron Hugues", map[string]any{"resume": "Seigneur ruiné.", "statut": "vivant", "secrets": "Il a vendu le fort."})
	b.fiche("pnj", "Sœur Aline", map[string]any{"statut": "vivant"})
	chef := b.fiche("pnj", "Rask le Borgne", map[string]any{"statut": "vivant"})
	oublie := b.fiche("pnj", "Vieux Tom", nil)
	b.fiche("pj", "Aldric", map[string]any{"campagne": camp, "statut": "actif", "joueur": "Léa"})
	b.fiche("pj", "Abel", map[string]any{"campagne": camp, "statut": "absent"})
	b.fiche("pj", "Torv", map[string]any{"campagne": camp, "statut": "parti"})
	b.fiche("pj", "Étranger", map[string]any{"campagne": autre, "statut": "actif"})
	b.fiche("quete", "Vieille affaire", map[string]any{"campagne": camp, "statut": "réussie"})
	quete := b.fiche("quete", "Le fort perdu", map[string]any{"campagne": camp, "statut": "en cours", "commanditaire": baron, "lieu": lieu,
		"objectif": "Reprendre le fort avec l'aide de [[Sœur Aline]] et de [[Vieux Tom]].", "secrets": "[[Baron Hugues]] ment."})
	b.fiche("rencontre", "Embuscade au col", map[string]any{"quete": quete, "campagne": camp, "statut": "préparée", "lieu": lieu, "pnj": []any{chef}})
	b.fiche("rencontre", "Sans rapport", map[string]any{"statut": "préparée"})
	b.fiche("secret", "Le fort est vendu", map[string]any{"statut": "caché", "concerne": []any{quete}})
	b.fiche("seance", "Séance 1", map[string]any{"campagne": camp, "numero": 1, "statut": "jouée"})
	prochaine := b.fiche("seance", "Séance 2", map[string]any{"campagne": camp, "numero": 2, "statut": "prévue", "preparation": "Ouvrir sur le col."})
	b.fiche("seance", "Séance 3", map[string]any{"campagne": camp, "numero": 3, "statut": "prévue"})
	if c, _ := b.api("POST", fmt.Sprintf("/api/entites/%v/archive", oublie), map[string]any{"archive": true}); c != 200 {
		t.Fatalf("archivage : %d", c)
	}

	c, v := b.local("GET", "/api/table", nil)
	if c != 200 {
		t.Fatalf("table : %d %v", c, v)
	}
	if q := v["quete"].(map[string]any); q["id"] != quete || len(v["quetes"].([]any)) != 1 {
		t.Fatalf("quête en cours : %v / %v", q["nom"], v["quetes"])
	}
	if v["campagne"].(map[string]any)["id"] != camp || v["seance"].(map[string]any)["id"] != prochaine {
		t.Fatalf("campagne ou séance : %v / %v", v["campagne"], v["seance"])
	}
	// PJ : ceux de la campagne, sans ceux qui sont partis
	pj := v["pj"].([]any)
	if len(pj) != 2 || pj[0].(map[string]any)["fiche"].(map[string]any)["nom"] != "Aldric" || pj[1].(map[string]any)["statut"] != "absent" {
		t.Fatalf("PJ : %v", pj)
	}
	// scène : chaque fiche une fois, avec sa raison ; ni fiche archivée, ni la quête elle-même
	raisons := map[string]string{}
	secrets := map[string]bool{}
	for _, x := range v["scene"].([]any) {
		m := x.(map[string]any)
		nom := m["fiche"].(map[string]any)["nom"].(string)
		if _, deja := raisons[nom]; deja {
			t.Fatalf("« %s » en double dans la scène", nom)
		}
		raisons[nom] = m["raison"].(string)
		secrets[nom], _ = m["secret"].(bool)
	}
	attendu := map[string]string{"Baron Hugues": "Commanditaire", "Mont-Veille": "Lieu", "Rask le Borgne": "rencontre « Embuscade au col »",
		"Le fort est vendu": "Concerne", "Sœur Aline": "cité"}
	if len(raisons) != len(attendu) {
		t.Fatalf("scène : %v", raisons)
	}
	for nom, r := range attendu {
		if raisons[nom] != r {
			t.Fatalf("scène, « %s » : raison %q, attendu %q (%v)", nom, raisons[nom], r, raisons)
		}
	}
	if !secrets["Baron Hugues"] || secrets["Mont-Veille"] {
		t.Fatalf("indicateur de secret : %v", secrets)
	}
	if r := v["rencontres"].([]any); len(r) != 1 || r[0].(map[string]any)["lieu"] != "Mont-Veille" {
		t.Fatalf("rencontres : %v", r)
	}
	// une quête choisie, même terminée ; une fiche d'un autre type n'est pas une quête
	if c, v := b.local("GET", fmt.Sprintf("/api/table?quete=%v", baron), nil); c != 404 {
		t.Fatalf("un PNJ pris pour une quête : %d %v", c, v)
	}
}

// L'état de la partie : les PV des PJ lus sur leur feuille, et le combat en cours.
func TestTableEtat(t *testing.T) {
	b := nouveau(t)
	s := b.scene()
	c, v := b.local("GET", fmt.Sprintf("/api/table/etat?campagne=%v", s.camp), nil)
	pj, _ := v["pj"].(map[string]any)
	if c != 200 || v["combat"] != nil || len(pj) != 1 {
		t.Fatalf("état sans combat : %d %v", c, v)
	}
	if a := pj[fmt.Sprint(s.aldric)].(map[string]any); a["pv"] != float64(21) || a["pv_max"] != float64(21) || a["pc"] != float64(2) {
		t.Fatalf("état d'Aldric : %v", a)
	}
	// l'écran lui-même donne l'état des PJ ; un PJ sans feuille n'en a pas
	_, v = b.local("GET", "/api/table", nil)
	vies := map[string]bool{}
	for _, x := range v["pj"].([]any) {
		p := x.(map[string]any)
		vies[p["fiche"].(map[string]any)["nom"].(string)] = p["vie"] != nil
	}
	if !vies["Aldric"] || vies["Mira"] || len(vies) != 3 {
		t.Fatalf("état des PJ sur l'écran : %v", vies)
	}
	// un combat lancé apparaît, et les PV perdus se lisent aussitôt
	_, v = b.local("POST", "/api/combats", map[string]any{"rencontre": s.renc})
	id := v["combat"].(map[string]any)["id"]
	_, v = b.local("GET", fmt.Sprintf("/api/combats/%v", id), nil)
	aldric := cbts(v)["Aldric"]["id"]
	if c, _ := b.local("POST", fmt.Sprintf("/api/combats/%v/actions", id), map[string]any{"type": "pv", "id": aldric, "sens": "degats", "valeur": 6}); c != 200 {
		t.Fatalf("dégâts : %d", c)
	}
	_, v = b.local("GET", fmt.Sprintf("/api/table/etat?campagne=%v", s.camp), nil)
	if cb := v["combat"].(map[string]any); cb["id"] != id || cb["nom"] != "Embuscade" || v["pj"].(map[string]any)[fmt.Sprint(s.aldric)].(map[string]any)["pv"] != float64(15) {
		t.Fatalf("état pendant le combat : %v", v)
	}
	// terminé, il n'est plus « en cours »
	b.local("POST", fmt.Sprintf("/api/combats/%v/actions", id), map[string]any{"type": "terminer"})
	if _, v = b.local("GET", "/api/table/etat", nil); v["combat"] != nil {
		t.Fatalf("combat terminé encore en cours : %v", v["combat"])
	}
	// les anciennes routes du pont vers le tracker ne répondent plus
	if c, _ = b.local("POST", "/api/table/combat", map[string]any{"rencontre": s.renc}); c != 404 && c != 405 {
		t.Fatalf("ancienne route de lancement : %d", c)
	}
	if c, _ = b.local("GET", "/api/table/tracker", nil); c != 404 {
		t.Fatalf("ancienne route du tracker : %d", c)
	}
	if c, _ = b.local("POST", fmt.Sprintf("/api/tracker/depuis/%v", s.renc), nil); c != 404 && c != 405 {
		t.Fatalf("envoi d'une rencontre au tracker : %d", c)
	}
}

// La page n'ouvre de WebSocket que vers l'application elle-même, et n'affiche plus aucun cadre.
func TestPolitiqueDeLaPage(t *testing.T) {
	b := nouveau(t)
	politique := func(hote string) string {
		r := httptest.NewRequest("GET", "/", nil)
		r.Host, r.RemoteAddr = hote, "127.0.0.1:5000"
		w := httptest.NewRecorder()
		b.s.ServeHTTP(w, r)
		return w.Header().Get("Content-Security-Policy")
	}
	p := politique("127.0.0.1:8100")
	if !strings.Contains(p, "default-src 'self'") || !strings.HasSuffix(p, "; connect-src 'self' ws://127.0.0.1:8100 wss://127.0.0.1:8100") || strings.Contains(p, "frame-src") {
		t.Fatalf("politique de la page : %q", p)
	}
	if p := socketsDeLaPage(&http.Request{Host: "pc; script-src *"}); p != "" {
		t.Fatalf("hôte recopié sans contrôle : %q", p)
	}
}
