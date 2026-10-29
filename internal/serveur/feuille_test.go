package serveur

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/Tchyze/cof_dm/internal/comptes"
)

// Règles d'essai pour la feuille de personnage, inventées : aucune valeur ne vient d'un livre.
const reglesFeuille = `{"format":"cof-dm-regles","personnage":{"niveau_max":9,"valeur_min":2,"valeur_max":24,"des_de_vie":["d6","d9"],"de_de_vie":"d9",
	"caracs_magie":["SAG","CHA"],"carac_magie":"SAG","defense_base":7,"pc_max":2,"pr_max":4,"rang_max":3,"origines_voie":["métier","sang"],
	"repos":[{"cle":"halte","libelle":"Halte","cout_pr":1,"pv":["dv","niveau"]}]}}`

func TestFeuille(t *testing.T) {
	b := nouveau(t)
	pj := b.fiche("pj", "Aldric", map[string]any{"niveau": 3})
	pnj := b.fiche("pnj", "Gautier", nil)
	u := "/api/entites/" + strconv.Itoa(int(pj)) + "/feuille"
	if c, _ := b.local("GET", "/api/entites/"+strconv.Itoa(int(pnj))+"/feuille", nil); c != 422 {
		t.Fatalf("feuille d'un PNJ : %d", c)
	}
	// sans règles : une feuille vierge, sans borne ni valeur de départ
	c, v := b.local("GET", u, nil)
	if c != 200 || v["existe"] != false || v["niveau"] != float64(3) || v["dv"] != "" || v["pc_max"] != float64(0) {
		t.Fatalf("feuille vierge sans règles : %d %v", c, v)
	}
	if c, v := b.local("PUT", u, map[string]any{"pv": 30, "dv": "d20", "valeurs": map[string]any{"FOR": 99}, "voies": []any{map[string]any{"nom": "Voie libre", "origine": "ce que je veux", "rang": 2, "capacites": []string{"a", "b"}}}}); c != 200 || v["existe"] != true {
		t.Fatalf("feuille libre sans règles : %d %v", c, v)
	}
	// avec des règles : valeurs de départ (caractéristiques neutres, dans les bornes) et bornes
	b.local("POST", "/api/regles/import", reglesFeuille)
	autre := b.fiche("pj", "Bérénice", map[string]any{"niveau": 2})
	ua := "/api/entites/" + strconv.Itoa(int(autre)) + "/feuille"
	if _, v := b.local("GET", ua, nil); v["dv"] != "d9" || v["carac_magie"] != "SAG" || v["pc_max"] != float64(2) || v["pr"] != float64(4) || v["valeurs"].(map[string]any)["FOR"] != float64(10) {
		t.Fatalf("feuille vierge avec règles : %v", v)
	}
	// le niveau, saisi sur la fiche, suit lui aussi les règles
	if c, _ := b.local("POST", "/api/entites", map[string]any{"type": "pj", "nom": "Trop fort", "data": map[string]any{"niveau": 10}}); c != 422 {
		t.Fatalf("PJ créé au-delà du niveau maximum : %d", c)
	}
	if c, _ := b.local("PATCH", "/api/entites/"+strconv.Itoa(int(autre)), map[string]any{"data": map[string]any{"niveau": 10}}); c != 422 {
		t.Fatalf("niveau au-delà du maximum : %d", c)
	}
	if c, _ := b.local("POST", "/api/entites", map[string]any{"type": "pnj", "nom": "Hors règles", "data": map[string]any{}}); c != 201 {
		t.Fatalf("la borne de niveau gêne les autres fiches : %d", c)
	}
	if _, v := b.local("GET", ua, nil); len(v["regles"].(map[string]any)["repos"].([]any)) != 1 {
		t.Fatalf("règles jointes à la feuille : %v", v["regles"])
	}
	// la feuille libre d'avant reste lisible, même hors des nouvelles bornes
	if c, v := b.local("GET", u, nil); c != 200 || v["dv"] != "d20" {
		t.Fatalf("feuille d'avant les règles : %d %v", c, v)
	}
	bonne := map[string]any{"valeurs": map[string]any{"FOR": 16, "DEX": 14, "CON": 12, "INT": 8, "SAG": 10, "CHA": 11}, "dv": "d6", "pv": 21, "pv_actuel": 99,
		"carac_magie": "CHA", "armure": map[string]any{"nom": "Cuir", "bonus": 2}, "bouclier": map[string]any{"bonus": 1}, "def_divers": 1, "init_divers": -1,
		"att_divers": map[string]any{"D": 1}, "pc": 9, "pc_max": 2, "pr": 1, "pr_max": 4, "notes": "note du MJ",
		"attaques": []any{map[string]any{"nom": "Épée", "type": "C", "mod": 1, "dm": "1d8+3"}, map[string]any{"nom": "Arc", "type": "D"}, map[string]any{"nom": "Trait", "type": "M"}},
		"voies":    []any{map[string]any{"nom": "Voie de la vigie", "origine": "sang", "rang": 2, "capacites": []string{"Œil ouvert"}}}}
	c, v = b.local("PUT", ua, bonne)
	if c != 200 {
		t.Fatalf("feuille valide : %d %v", c, v)
	}
	// valeurs dérivées : modificateurs, initiative, DEF, bonus d'attaque (niveau 2), ressources bornées
	car := v["carac"].(map[string]any)
	att := v["attaques"].([]any)
	bonus := func(i int) any { return att[i].(map[string]any)["bonus"] }
	if car["FOR"] != float64(3) || car["INT"] != float64(-1) || v["init"] != float64(13) || v["defense"] != float64(7+2+2+1+1) ||
		bonus(0) != float64(2+3+1) || bonus(1) != float64(2+2+1) || bonus(2) != float64(2+0) || v["pv_actuel"] != float64(21) || v["pc"] != float64(2) {
		t.Fatalf("valeurs dérivées : %v", v)
	}
	if caps := v["voies"].([]any)[0].(map[string]any)["capacites"].([]any); len(caps) != 3 {
		t.Fatalf("capacités d'une voie : %v", caps)
	}
	// le niveau vient de la fiche : le changer recalcule les attaques
	b.local("PATCH", "/api/entites/"+strconv.Itoa(int(autre)), map[string]any{"data": map[string]any{"niveau": 5}})
	if _, v := b.local("GET", ua, nil); v["niveau"] != float64(5) || v["attaques"].([]any)[0].(map[string]any)["bonus"] != float64(5+3+1) {
		t.Fatalf("attaques après un changement de niveau : %v", v)
	}
	// bornes des règles
	for nom, mauvais := range map[string]map[string]any{
		"valeur trop haute": {"valeurs": map[string]any{"FOR": 25, "DEX": 10, "CON": 10, "INT": 10, "SAG": 10, "CHA": 10}},
		"valeur trop basse": {"valeurs": map[string]any{"FOR": 1, "DEX": 10, "CON": 10, "INT": 10, "SAG": 10, "CHA": 10}},
		"dé de vie":         {"dv": "d8"}, "carac de magie": {"carac_magie": "INT"}, "pv": {"pv": 0}, "ressource négative": {"pm": -1},
		"origine de voie": {"voies": []any{map[string]any{"nom": "x", "origine": "autre"}}},
		"rang de voie":    {"voies": []any{map[string]any{"nom": "x", "rang": 4}}},
		"type d'attaque":  {"attaques": []any{map[string]any{"type": "Z"}}},
	} {
		if c, v := b.local("PUT", ua, mauvais); c != 422 {
			t.Fatalf("%s acceptée : %d %v", nom, c, v)
		}
	}
	// deux enregistrements concurrents : le second, parti d'une feuille périmée, est refusé
	_, v = b.local("GET", ua, nil)
	bonne["modifie"] = v["modifie"]
	if c, _ := b.local("PUT", ua, bonne); c != 200 {
		t.Fatalf("enregistrement à jour : %d", c)
	}
	if c, _ := b.local("PUT", ua, bonne); c != 409 {
		t.Fatalf("enregistrement périmé : %d", c)
	}
	// l'export emporte les feuilles ; un export qui n'en porte pas (ancien) laisse celles de l'univers
	x, _ := b.s.store.Exporter()
	if len(x.Feuilles) != 2 {
		t.Fatalf("feuilles exportées : %d", len(x.Feuilles))
	}
	x.Feuilles = nil
	if err := b.s.store.Restaurer(x, "test"); err != nil {
		t.Fatal(err)
	}
	if _, v := b.local("GET", ua, nil); v["existe"] != true || v["pv"] != float64(21) {
		t.Fatalf("feuille après la restauration d'un ancien export : %v", v)
	}
	// supprimer la fiche emporte sa feuille
	b.local("DELETE", "/api/entites/"+strconv.Itoa(int(autre))+"?definitif=1", nil)
	if _, _, err := b.s.store.LireFeuille(int64(autre)); err == nil {
		t.Fatal("la feuille a survécu à sa fiche")
	}
}

// Un joueur lit la feuille de son PJ, sans les notes du MJ ; il ne l'écrit que si l'édition est
// ouverte, et jamais celle d'un autre.
func TestFeuilleRoles(t *testing.T) {
	b, reg := bancServeur(t)
	for _, nom := range []string{"joueuse", "autre"} {
		reg.Creer(nom, "mot-de-passe-"+nom, false)
		reg.Modifier(nom, func(c *comptes.Compte) error { c.Roles[b.s.store.ID()] = comptes.Joueur; return nil })
	}
	chef, mj, audit, joueuse, autre := b.connecter("chef"), b.connecter("meneur"), b.connecter("audit"), b.connecter("joueuse"), b.connecter("autre")
	_, v, _ := b.proxy("POST", "/api/entites", map[string]any{"type": "pj", "nom": "Aldric"}, mj)
	id := strconv.Itoa(int(v["id"].(float64)))
	u := "/api/entites/" + id + "/feuille"
	feuille := map[string]any{"pv": 12, "notes": "SECRET-MJ : il est maudit", "equipement": "Une lanterne"}
	if c, _, _ := b.proxy("PUT", u, feuille, mj); c != 200 {
		t.Fatalf("feuille écrite par le MJ : %d", c)
	}
	if c, v, _ := b.proxy("GET", u, nil, audit); c != 200 || v["notes"] == "" {
		t.Fatalf("feuille lue par l'auditeur : %d %v", c, v)
	}
	if c, _, _ := b.proxy("PUT", u, feuille, audit); c != 403 {
		t.Fatalf("feuille écrite par l'auditeur : %d", c)
	}
	b.proxy("PATCH", "/api/comptes/joueuse", map[string]any{"pj": v["id"]}, chef)
	// tant que la fiche n'est pas partagée, le joueur ne voit rien
	if c, _, _ := b.proxy("GET", u, nil, joueuse); c != 404 {
		t.Fatalf("feuille d'un PJ non partagé : %d", c)
	}
	b.proxy("POST", "/api/entites/"+id+"/partage", map[string]any{"partage": true}, mj)
	c, v, _ := b.proxy("GET", u, nil, joueuse)
	if c != 200 || v["pv"] != float64(12) || strings.Contains(toJSON(v), "SECRET-MJ") {
		t.Fatalf("feuille lue par le joueur : %d %v", c, v)
	}
	if c, _, _ := b.proxy("GET", u, nil, autre); c != 404 {
		t.Fatalf("feuille lue par un autre joueur : %d", c)
	}
	// édition fermée par défaut
	if c, _, _ := b.proxy("PUT", u, map[string]any{"pv": 50}, joueuse); c != 403 {
		t.Fatalf("feuille écrite par le joueur, édition fermée : %d", c)
	}
	b.proxy("PUT", "/api/joueurs/edition", map[string]any{"active": true}, chef)
	c, v, _ = b.proxy("PUT", u, map[string]any{"pv": 15, "notes": "j'efface les notes", "inventaire": []any{map[string]any{"nom": "Une lanterne"}}}, joueuse)
	if c != 200 || v["pv"] != float64(15) || v["notes"] != "" {
		t.Fatalf("feuille écrite par le joueur : %d %v", c, v)
	}
	if c, _, _ := b.proxy("PUT", u, map[string]any{"pv": 1}, autre); c != 404 {
		t.Fatalf("feuille écrite par un autre joueur : %d", c)
	}
	// les chiffres d'un objet restent ceux du MJ : le joueur porte le casque, sans gonfler son bonus
	b.proxy("PUT", u, map[string]any{"pv": 15, "notes": "SECRET-MJ : il est maudit", "inventaire": []any{map[string]any{"nom": "Casque", "emplacement": "tete", "bonus": []any{map[string]any{"cible": "def", "valeur": 2}}}}}, mj)
	c, v, _ = b.proxy("PUT", u, map[string]any{"pv": 15, "inventaire": []any{map[string]any{"id": 1, "nom": "Casque", "porte": true, "emplacement": "corps", "bonus": []any{map[string]any{"cible": "def", "valeur": 20}}}}}, joueuse)
	if o, _ := v["inventaire"].([]any); c != 200 || len(o) != 1 || fmt.Sprint(o[0].(map[string]any)["bonus"]) != "[map[cible:def valeur:2]]" || o[0].(map[string]any)["emplacement"] != "tete" || o[0].(map[string]any)["porte"] != true {
		t.Fatalf("objet modifié par le joueur : %d %v", c, v["inventaire"])
	}
	// une ligne ajoutée sans identifiant n'hérite pas des chiffres du casque qu'elle remplace
	b.proxy("PUT", u, map[string]any{"pv": 15, "inventaire": []any{map[string]any{"nom": "Une lanterne"}}}, joueuse)
	// les notes du MJ sont intactes
	_, v, _ = b.proxy("GET", u, nil, mj)
	if inv, _ := v["inventaire"].([]any); v["notes"] != "SECRET-MJ : il est maudit" || v["pv"] != float64(15) || len(inv) != 1 || inv[0].(map[string]any)["nom"] != "Une lanterne" || inv[0].(map[string]any)["bonus"] != nil {
		t.Fatalf("feuille après l'édition du joueur : %v", v)
	}
}
