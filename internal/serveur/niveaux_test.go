package serveur

import (
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/Tchyze/cof_dm/internal/comptes"
	"github.com/Tchyze/cof_dm/internal/des"
)

// Règles d'essai pour la montée de niveau, inventées : aucune valeur ne vient d'un livre.
const reglesMontee = `{"format":"cof-dm-regles","personnage":{"niveau_max":7,"rang_max":4,
	"montee":{"pv":["dv","con"],"pv_fixe":1,"points":3,"couts":[1,2],"niveau_min":[0,0,0,6],"report":true}}}`

func niveauFiche(b *banc, id string) any {
	_, v := b.local("GET", "/api/entites/"+id, nil)
	return v["data"].(map[string]any)["niveau"]
}

// Sans règles, la montée se fait en saisie libre ; l'annulation défait exactement la dernière.
func TestMonteeLibre(t *testing.T) {
	b := nouveau(t)
	id := strconv.Itoa(int(b.fiche("pj", "Aldric", map[string]any{"niveau": 2})))
	u, un := "/api/feuilles/"+id+"/niveau", "/api/feuilles/"+id+"/niveau/annuler"
	if c, _ := b.local("POST", u, map[string]any{"pv": 4}); c != 422 {
		t.Fatalf("montée sans feuille : %d", c)
	}
	b.local("PUT", "/api/entites/"+id+"/feuille", map[string]any{"pv": 10, "pv_actuel": 6,
		"voies": []any{map[string]any{"nom": "Voie du gué", "rang": 1, "capacites": []string{"Pied sûr", "Passeur", "Rive"}}}})
	if c, _ := b.local("POST", u, map[string]any{}); c != 422 {
		t.Fatalf("montée sans PV saisis ni formule : %d", c)
	}
	for nom, rangs := range map[string][]any{
		"voie inconnue":  {map[string]any{"voie": 3, "rang": 2}},
		"rang déjà pris": {map[string]any{"voie": 0, "rang": 1}},
		"voie en double": {map[string]any{"voie": 0, "rang": 2}, map[string]any{"voie": 0, "rang": 3}},
	} {
		if c, _ := b.local("POST", u, map[string]any{"pv": 4, "rangs": rangs}); c != 422 {
			t.Fatalf("%s acceptée : %d", nom, c)
		}
	}
	if niveauFiche(b, id) != float64(2) {
		t.Fatal("une montée refusée a changé le niveau")
	}
	// sans règles, les rangs sont libres : deux d'un coup
	c, v := b.local("POST", u, map[string]any{"pv": 4, "rangs": []any{map[string]any{"voie": 0, "rang": 3}}})
	if c != 200 || v["niveau"] != float64(3) || v["pv"] != float64(14) || v["pv_actuel"] != float64(10) || v["voies"].([]any)[0].(map[string]any)["rang"] != float64(3) {
		t.Fatalf("montée libre : %d %v", c, v)
	}
	m := v["montee"].(map[string]any)
	pris := m["rangs"].([]any)[0].(map[string]any)
	if m["niveau"] != float64(3) || m["pv"] != float64(4) || m["jet"] != nil || pris["de"] != float64(1) || pris["a"] != float64(3) ||
		toJSON(pris["capacites"]) != `["Passeur","Rive"]` || len(v["niveaux"].([]any)) != 1 {
		t.Fatalf("ligne de l'historique : %v", v)
	}
	if niveauFiche(b, id) != float64(3) {
		t.Fatal("le niveau de la fiche n'a pas suivi")
	}
	// le journal note la montée
	if _, j := b.local("GET", "/api/journal", nil); !strings.Contains(toJSON(j), "montée de niveau") {
		t.Fatalf("journal : %v", j)
	}
	// annuler la dernière
	c, v = b.local("POST", un, map[string]any{})
	if c != 200 || v["niveau"] != float64(2) || v["pv"] != float64(10) || v["pv_actuel"] != float64(6) || v["voies"].([]any)[0].(map[string]any)["rang"] != float64(1) || v["niveaux"] != nil {
		t.Fatalf("annulation : %d %v", c, v)
	}
	if niveauFiche(b, id) != float64(2) {
		t.Fatal("le niveau de la fiche n'a pas été rendu")
	}
	if c, _ := b.local("POST", un, map[string]any{}); c != 422 {
		t.Fatalf("annulation sans montée : %d", c)
	}
	// une voie changée à la main depuis la montée : l'annulation est refusée, rien ne bouge
	b.local("POST", u, map[string]any{"pv": 2, "rangs": []any{map[string]any{"voie": 0, "rang": 2}}})
	_, f := b.local("GET", "/api/entites/"+id+"/feuille", nil)
	f["voies"].([]any)[0].(map[string]any)["rang"] = 1
	b.local("PUT", "/api/entites/"+id+"/feuille", f)
	if c, _ := b.local("POST", un, map[string]any{}); c != 422 || niveauFiche(b, id) != float64(3) {
		t.Fatalf("annulation après un changement de voie : %d", c)
	}
	// le niveau changé à la main sur la fiche : de même
	b.local("PATCH", "/api/entites/"+id, map[string]any{"data": map[string]any{"niveau": 5}})
	if c, _ := b.local("POST", un, map[string]any{}); c != 422 {
		t.Fatalf("annulation après un changement de niveau : %d", c)
	}
	// une feuille périmée est refusée
	if c, _ := b.local("POST", u, map[string]any{"pv": 1, "modifie": 1}); c != 409 {
		t.Fatalf("montée sur une feuille périmée : %d", c)
	}
}

// Avec des règles : PV lancés (jet à la table, caché si demandé), points de capacité, coût et
// niveau minimal des rangs, report des points, niveau maximum.
func TestMonteeRegles(t *testing.T) {
	b := nouveau(t)
	ancien := des.De
	des.De = func(faces int) int { return faces } // le dé donne toujours son maximum
	t.Cleanup(func() { des.De = ancien })
	b.local("POST", "/api/regles/import", reglesMontee)
	id := strconv.Itoa(int(b.fiche("pj", "Bérénice", map[string]any{"niveau": 4})))
	u := "/api/feuilles/" + id + "/niveau"
	b.local("PUT", "/api/entites/"+id+"/feuille", map[string]any{"pv": 20, "dv": "",
		"valeurs": map[string]any{"FOR": 10, "DEX": 10, "CON": 14, "INT": 10, "SAG": 10, "CHA": 10},
		"voies":   []any{map[string]any{"nom": "Voie du gué", "rang": 0}, map[string]any{"nom": "Voie libre", "rang": 2}}})
	if c, v := b.local("POST", u, map[string]any{}); c != 422 || !strings.Contains(toJSON(v), "dé de vie") {
		t.Fatalf("montée sans dé de vie : %d %v", c, v)
	}
	_, f := b.local("GET", "/api/entites/"+id+"/feuille", nil)
	f["dv"] = "d9"
	b.local("PUT", "/api/entites/"+id+"/feuille", f)
	con := int(f["carac"].(map[string]any)["CON"].(float64))
	for nom, rangs := range map[string][]any{
		"trop de points":         {map[string]any{"voie": 0, "rang": 2}, map[string]any{"voie": 1, "rang": 3}}, // 1 + 2 + 2
		"rang au-delà du niveau": {map[string]any{"voie": 1, "rang": 4}},
		"rang au-delà du max":    {map[string]any{"voie": 0, "rang": 5}},
	} {
		if c, _ := b.local("POST", u, map[string]any{"rangs": rangs}); c != 422 {
			t.Fatalf("%s accepté : %d", nom, c)
		}
	}
	if _, j := b.local("GET", "/api/des", nil); len(j["jets"].([]any)) != 0 {
		t.Fatalf("une montée refusée a lancé les PV : %v", j)
	}
	// les PV saisis sont ignorés quand les règles donnent la formule
	c, v := b.local("POST", u, map[string]any{"pv": 99, "rangs": []any{map[string]any{"voie": 0, "rang": 1}}})
	gain := 9 + con + 1
	if c != 200 || v["niveau"] != float64(5) || v["pv"] != float64(20+gain) || v["points_capacite"] != float64(2) {
		t.Fatalf("montée avec les règles : %d %v", c, v)
	}
	m := v["montee"].(map[string]any)
	if m["pv"] != float64(gain) || !strings.Contains(m["jet"].(string), "1d9[9]") || m["points"] != float64(1) {
		t.Fatalf("ligne de l'historique : %v", m)
	}
	j := v["jet"].(map[string]any)
	if j["total"] != float64(gain) || j["cache"] != nil || !strings.Contains(j["libelle"].(string), "Bérénice") {
		t.Fatalf("jet des PV à la table : %v", j)
	}
	// les points gardés s'ajoutent à ceux du niveau : 2 + 3 = 5, dont 2 + 2 dépensés
	c, v = b.local("POST", u, map[string]any{"cache": true, "rangs": []any{map[string]any{"voie": 0, "rang": 2}, map[string]any{"voie": 1, "rang": 3}}})
	if c != 200 || v["niveau"] != float64(6) || v["points_capacite"] != float64(1) || v["jet"].(map[string]any)["cache"] != true {
		t.Fatalf("seconde montée, jet caché : %d %v", c, v)
	}
	// annuler rend les points gardés avant la montée
	if _, v = b.local("POST", u+"/annuler", map[string]any{}); v["points_capacite"] != float64(2) || v["niveau"] != float64(5) {
		t.Fatalf("annulation : %v", v)
	}
	// le niveau maximum des règles
	b.local("PATCH", "/api/entites/"+id, map[string]any{"data": map[string]any{"niveau": 7}})
	if c, _ := b.local("POST", u, map[string]any{}); c != 422 {
		t.Fatalf("montée au-delà du niveau maximum : %d", c)
	}
	// fonctionnalité « feuilles » coupée : plus de montée
	b.local("PUT", "/api/fonctionnalites", map[string]any{"feuilles": false})
	if c, _ := b.local("POST", u+"/annuler", map[string]any{}); c != 404 {
		t.Fatalf("montée, feuilles coupées : %d", c)
	}
}

// Seul le MJ fait monter un PJ de niveau ; le joueur voit l'historique sans le détail d'un jet
// caché, et ne peut pas le réécrire.
func TestMonteeRoles(t *testing.T) {
	b, reg := bancServeur(t)
	reg.Creer("joueuse", "mot-de-passe-joueuse", false)
	reg.Modifier("joueuse", func(c *comptes.Compte) error { c.Roles[b.s.store.ID()] = comptes.Joueur; return nil })
	chef, mj, audit, joueuse := b.connecter("chef"), b.connecter("meneur"), b.connecter("audit"), b.connecter("joueuse")
	_, v, _ := b.proxy("POST", "/api/entites", map[string]any{"type": "pj", "nom": "Aldric", "data": map[string]any{"niveau": 1}}, mj)
	id := strconv.Itoa(int(v["id"].(float64)))
	b.proxy("PATCH", "/api/comptes/joueuse", map[string]any{"pj": v["id"]}, chef)
	b.proxy("POST", "/api/entites/"+id+"/partage", map[string]any{"partage": true}, mj)
	b.proxy("PUT", "/api/joueurs/edition", map[string]any{"active": true}, chef)
	u := "/api/entites/" + id + "/feuille"
	b.proxy("PUT", u, map[string]any{"pv": 8, "voies": []any{map[string]any{"nom": "Voie du gué", "rang": 0, "capacites": []string{"Pied sûr"}}}}, mj)
	for nom, qui := range map[string]func(*http.Request){"auditeur": audit, "joueur": joueuse} {
		if c, _, _ := b.proxy("POST", "/api/feuilles/"+id+"/niveau", map[string]any{"pv": 3}, qui); c != 403 {
			t.Fatalf("montée par le %s : %d", nom, c)
		}
	}
	if c, _, _ := b.proxy("POST", "/api/feuilles/"+id+"/niveau", map[string]any{"pv": 3, "cache": true, "rangs": []any{map[string]any{"voie": 0, "rang": 1}}}, mj); c != 200 {
		t.Fatalf("montée par le MJ : %d", c)
	}
	// sans formule, pas de jet : rien de caché ; on en fabrique un pour vérifier le filtre
	_, f, _ := b.proxy("GET", u, nil, mj)
	f["niveaux"].([]any)[0].(map[string]any)["jet"], f["niveaux"].([]any)[0].(map[string]any)["cache"] = "JET-CACHE", true
	b.proxy("PUT", u, f, mj)
	c, v, _ := b.proxy("GET", u, nil, joueuse)
	if c != 200 || len(v["niveaux"].([]any)) != 1 || strings.Contains(toJSON(v), "JET-CACHE") {
		t.Fatalf("historique vu par le joueur : %d %v", c, v)
	}
	// le joueur réécrit sa feuille : l'historique et les points restent ceux du MJ
	v["niveaux"], v["points_capacite"] = []any{}, 50
	if c, v, _ = b.proxy("PUT", u, v, joueuse); c != 200 || len(v["niveaux"].([]any)) != 1 || v["points_capacite"] != nil {
		t.Fatalf("historique réécrit par le joueur : %d %v", c, v)
	}
	if _, v, _ = b.proxy("GET", u, nil, mj); v["niveaux"].([]any)[0].(map[string]any)["jet"] != "JET-CACHE" {
		t.Fatalf("historique après l'écriture du joueur : %v", v)
	}
}

// Le MJ accorde un niveau ; le joueur le prend lui-même (PV, rangs), une seule fois, même feuilles
// verrouillées. Il ne peut ni s'accorder un niveau, ni prendre celui d'un autre.
func TestMonteeAccordee(t *testing.T) {
	b, reg := bancServeur(t)
	for _, nom := range []string{"joueuse", "autre"} {
		reg.Creer(nom, "mot-de-passe-"+nom, false)
		reg.Modifier(nom, func(c *comptes.Compte) error { c.Roles[b.s.store.ID()] = comptes.Joueur; return nil })
	}
	chef, mj, joueuse, autre := b.connecter("chef"), b.connecter("meneur"), b.connecter("joueuse"), b.connecter("autre")
	_, v, _ := b.proxy("POST", "/api/entites", map[string]any{"type": "pj", "nom": "Aldric", "data": map[string]any{"niveau": 2}}, mj)
	id := strconv.Itoa(int(v["id"].(float64)))
	b.proxy("PATCH", "/api/comptes/joueuse", map[string]any{"pj": v["id"]}, chef)
	b.proxy("POST", "/api/entites/"+id+"/partage", map[string]any{"partage": true}, mj)
	u, uj := "/api/entites/"+id+"/feuille", "/api/joueur/pj/"+id+"/niveau"
	b.proxy("PUT", u, map[string]any{"pv": 8, "notes": "SECRET-MJ", "voies": []any{map[string]any{"nom": "Voie du gué", "rang": 1, "capacites": []string{"Pied sûr", "Passeur"}}}}, mj)
	monter := map[string]any{"pv": 3, "cache": true, "rangs": []any{map[string]any{"voie": 0, "rang": 2}}}
	if c, _, _ := b.proxy("POST", uj, monter, joueuse); c != 403 {
		t.Fatalf("montée sans accord du MJ : %d", c)
	}
	if c, _, _ := b.proxy("POST", "/api/feuilles/"+id+"/niveau/accorder", map[string]any{"accorde": true}, joueuse); c != 403 {
		t.Fatalf("niveau accordé par le joueur lui-même : %d", c)
	}
	if c, v, _ := b.proxy("POST", "/api/feuilles/"+id+"/niveau/accorder", map[string]any{"accorde": true}, mj); c != 200 || v["niveau_accorde"] != float64(3) || v["niveau"] != float64(2) {
		t.Fatalf("niveau accordé : %d %v", c, v)
	}
	// le joueur ne peut pas réécrire l'accord par sa feuille
	b.proxy("PUT", "/api/joueurs/edition", map[string]any{"active": true}, chef)
	_, f, _ := b.proxy("GET", u, nil, joueuse)
	if f["niveau_accorde"] != float64(3) {
		t.Fatalf("accord vu par le joueur : %v", f)
	}
	f["niveau_accorde"] = 9
	if _, f, _ = b.proxy("PUT", u, f, joueuse); f["niveau_accorde"] != float64(3) {
		t.Fatalf("accord réécrit par le joueur : %v", f)
	}
	b.proxy("PUT", "/api/joueurs/edition", map[string]any{"active": false}, chef)
	if c, _, _ := b.proxy("POST", uj, monter, autre); c != 404 {
		t.Fatalf("montée prise par un autre joueur : %d", c)
	}
	// feuilles verrouillées : l'accord suffit ; le jet du joueur n'est jamais caché, les notes restent au MJ
	c, v, _ := b.proxy("POST", uj, monter, joueuse)
	if c != 200 || v["niveau"] != float64(3) || v["pv"] != float64(11) || v["niveau_accorde"] != nil || strings.Contains(toJSON(v), "SECRET-MJ") ||
		v["montee"].(map[string]any)["cache"] != nil {
		t.Fatalf("montée par le joueur : %d %v", c, v)
	}
	if c, _, _ := b.proxy("POST", uj, monter, joueuse); c != 403 {
		t.Fatalf("niveau accordé pris deux fois : %d", c)
	}
	// le MJ peut retirer un accord qu'il a donné
	b.proxy("POST", "/api/feuilles/"+id+"/niveau/accorder", map[string]any{"accorde": true}, mj)
	if _, v, _ := b.proxy("POST", "/api/feuilles/"+id+"/niveau/accorder", map[string]any{"accorde": false}, mj); v["niveau_accorde"] != nil {
		t.Fatalf("accord retiré : %v", v)
	}
	if c, _, _ := b.proxy("POST", uj, monter, joueuse); c != 403 {
		t.Fatalf("montée après le retrait de l'accord : %d", c)
	}
}

// À la maison, la page des joueurs ouverte : le joueur prend le niveau accordé, et rien d'autre.
func TestMonteeAccordeeMaison(t *testing.T) {
	b, _ := bancReseau(t)
	id := strconv.Itoa(int(b.fiche("pj", "Bérénice", map[string]any{"niveau": 1})))
	b.local("PUT", "/api/entites/"+id+"/feuille", map[string]any{"pv": 6})
	b.local("POST", "/api/acces/joueurs", map[string]any{"active": true})
	uj := "/api/joueur/pj/" + id + "/niveau"
	if c, _ := b.joueurLibre("POST", uj, map[string]any{"pv": 2}, true); c != 403 {
		t.Fatalf("montée libre sans accord : %d", c)
	}
	if c, _ := b.joueurLibre("POST", "/api/feuilles/"+id+"/niveau/accorder", map[string]any{"accorde": true}, true); c != 401 {
		t.Fatalf("accord depuis la page libre : %d", c)
	}
	b.local("POST", "/api/feuilles/"+id+"/niveau/accorder", map[string]any{"accorde": true})
	if c, corps := b.joueurLibre("POST", uj, map[string]any{"pv": 2}, true); c != 200 || !strings.Contains(corps, `"niveau":2`) {
		t.Fatalf("montée libre : %d %s", c, corps)
	}
}
