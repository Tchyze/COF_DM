package serveur

import (
	"fmt"
	"testing"

	"github.com/Tchyze/cof_dm/internal/comptes"
)

// Règles de rencontre d'essai, inventées pour les tests : aucune valeur ne vient d'un livre.
const reglesRencontre = `{"format":"cof-dm-regles","rencontre":{
 "points":[{"nc":0,"points":2},{"nc":1,"points":10},{"nc":2,"points":20},{"nc":3,"points":40}],
 "points_au_dela":30,"groupe_reference":2,
 "conditions":[{"cle":"brume","libelle":"Brume","niveau":1,"detail":"On n'y voit rien."}],
 "verdicts":[{"cle":"promenade","texte":"Une promenade.","jusqu_a":-1,"inclus":true,"ton":"ok"},
             {"cle":"serre","texte":"Serré.","jusqu_a":1,"ton":"warn"},
             {"cle":"fuite","texte":"Fuyez.","ton":"danger"}],
 "alertes":[{"quand":"nc","seuil":3,"texte":"Gros morceau."}]}}`

// Une rencontre prend ses créatures dans le bestiaire de l'univers et calcule sa difficulté avec les
// règles de l'univers et les PJ de sa campagne, sans tracker.
func TestRencontreDifficulte(t *testing.T) {
	b := nouveau(t)
	monstre := func(nom string, nc float64) float64 {
		c, v := b.local("POST", "/api/bestiaire", map[string]any{"nom": nom, "nc": nc, "pv": 9, "defense": 12})
		if c != 201 {
			t.Fatalf("monstre %s : %d %v", nom, c, v)
		}
		return v["id"].(float64)
	}
	gluon, titan, chef := monstre("Gluon", 1), monstre("Titan", 3), monstre("Meneur gluon", 2)
	camp := b.fiche("campagne", "Les Marches", map[string]any{"statut": "en cours"})
	b.fiche("pj", "Aldric", map[string]any{"campagne": camp, "statut": "actif", "niveau": 3})
	b.fiche("pj", "Mira", map[string]any{"campagne": camp, "statut": "actif", "niveau": 1})
	b.fiche("pj", "Abel", map[string]any{"campagne": camp, "statut": "absent", "niveau": 9})
	b.fiche("pj", "Feu Odon", map[string]any{"campagne": camp, "statut": "mort", "niveau": 9})
	baron := b.fiche("pnj", "Baron Hugues", map[string]any{"profil_combat": chef})
	valet := b.fiche("pnj", "Le valet", nil)
	renc := b.fiche("rencontre", "Embuscade", map[string]any{"campagne": camp, "pnj": []any{baron, valet},
		"adversaires": []any{map[string]any{"monstre": gluon, "nom": "Gluon", "qte": 2}, map[string]any{"monstre": titan, "nom": "Titan", "qte": 1, "camp": "allie"}}})
	url := fmt.Sprintf("/api/entites/%v/difficulte", renc)

	// sans règles : les créatures et le groupe, pas d'évaluation
	c, v := b.local("GET", url, nil)
	cr, _ := v["creatures"].([]any)
	if c != 200 || v["evaluation"] != nil || len(cr) != 3 || v["nb_pj"] != float64(2) || v["niveau_pj"] != float64(2) || len(v["pnj_sans_profil"].([]any)) != 1 {
		t.Fatalf("sans règles : %d %v", c, v)
	}
	if g := cr[0].(map[string]any); g["nom"] != "Gluon" || g["nc"] != float64(1) || g["pv"] != float64(9) || g["qte"] != float64(2) {
		t.Fatalf("créature : %v", g)
	}
	if p := cr[2].(map[string]any); p["nom"] != "Baron Hugues" || p["nc"] != float64(2) || p["pnj"].(map[string]any)["id"] != baron {
		t.Fatalf("PNJ présent : %v", p)
	}
	if c, v := b.local("POST", "/api/regles/import", reglesRencontre); c != 200 || v["rencontre"] != true {
		t.Fatalf("import des règles : %d %v", c, v)
	}
	// 2 Gluon (20) + le baron (20) = 40 points, niveau 3 ; l'allié ne compte pas ; groupe de niveau 2
	_, v = b.local("GET", url, nil)
	e, _ := v["evaluation"].(map[string]any)
	if e["points"] != float64(40) || e["niveau"] != float64(3) || e["ecart"] != float64(1) || e["cle"] != "fuite" || e["ton"] != "danger" || len(e["alertes"].([]any)) != 0 {
		t.Fatalf("évaluation : %v", e)
	}
	if len(v["conditions"].([]any)) != 1 {
		t.Fatalf("conditions : %v", v["conditions"])
	}
	// condition et groupe hypothétique
	_, v = b.local("GET", url+"?condition=brume&nb_pj=4&niveau_pj=6", nil)
	if e = v["evaluation"].(map[string]any); e["points_ajustes"] != float64(20) || e["niveau"] != float64(3) || e["cle"] != "promenade" || v["nb_pj"] != float64(4) {
		t.Fatalf("condition et groupe : %v", v)
	}
	// un monstre qui sert à des fiches ne se supprime pas ; libéré, il se supprime
	if c, v := b.local("DELETE", fmt.Sprintf("/api/bestiaire/%v", gluon), nil); c != 409 {
		t.Fatalf("suppression d'un monstre utilisé : %d %v", c, v)
	}
	if c, _ := b.local("DELETE", fmt.Sprintf("/api/bestiaire/%v", chef), nil); c != 409 {
		t.Fatalf("suppression d'un profil de combat utilisé : %d", c)
	}
	if c, _ := b.local("PATCH", fmt.Sprintf("/api/entites/%v", baron), map[string]any{"data": map[string]any{"profil_combat": nil}}); c != 200 {
		t.Fatalf("retrait du profil de combat : %d", c)
	}
	if c, _ := b.local("DELETE", fmt.Sprintf("/api/bestiaire/%v", chef), nil); c != 200 {
		t.Fatalf("suppression d'un monstre libre : %d", c)
	}
	// le profil de combat est un identifiant du bestiaire
	if c, _ := b.local("PATCH", fmt.Sprintf("/api/entites/%v", baron), map[string]any{"data": map[string]any{"profil_combat": "Meneur"}}); c != 422 {
		t.Fatalf("profil de combat en texte : %d", c)
	}
	// une fiche qui n'est pas une rencontre
	if c, _ := b.local("GET", fmt.Sprintf("/api/entites/%v/difficulte", baron), nil); c != 404 {
		t.Fatalf("difficulté d'un PNJ : %d", c)
	}
	// sans campagne ni PJ : le niveau de la rencontre, sans verdict
	seule := b.fiche("rencontre", "Au hasard", map[string]any{"campagne": b.fiche("campagne", "Vide", nil),
		"adversaires": []any{map[string]any{"monstre": titan, "qte": 1}, map[string]any{"monstre": 999, "nom": "Disparu", "qte": 2}}})
	_, v = b.local("GET", fmt.Sprintf("/api/entites/%v/difficulte", seule), nil)
	e = v["evaluation"].(map[string]any)
	if e["cle"] != "inconnu" || e["niveau"] != float64(3) || len(e["alertes"].([]any)) != 1 || v["creatures"].([]any)[1].(map[string]any)["introuvable"] != true {
		t.Fatalf("sans groupe : %v", v)
	}
}

// La difficulté suit les rôles : le MJ et l'auditeur la lisent, le joueur non, même sur une
// rencontre partagée.
func TestRencontreRoles(t *testing.T) {
	b, reg := bancServeur(t)
	reg.Creer("joueuse", "mot-de-passe-joueuse", false)
	reg.Modifier("joueuse", func(c *comptes.Compte) error { c.Roles[b.s.store.ID()] = comptes.Joueur; return nil })
	mj := b.connecter("meneur")
	_, m, _ := b.proxy("POST", "/api/bestiaire", map[string]any{"nom": "Gluon", "nc": 1, "pv": 9}, mj)
	c, v, _ := b.proxy("POST", "/api/entites", map[string]any{"type": "rencontre", "nom": "Embuscade",
		"data": map[string]any{"adversaires": []any{map[string]any{"monstre": m["id"], "nom": "Gluon", "qte": 2}}}}, mj)
	if c != 201 {
		t.Fatalf("rencontre : %d %v", c, v)
	}
	fiche := fmt.Sprintf("/api/entites/%v", v["id"])
	if c, _, _ := b.proxy("POST", fiche+"/partage", map[string]any{"partage": true}, mj); c != 200 {
		t.Fatalf("partage : %d", c)
	}
	for nom, attendu := range map[string]int{"meneur": 200, "audit": 200, "joueuse": 403, "passant": 403} {
		if c, v, _ := b.proxy("GET", fiche+"/difficulte", nil, b.connecter(nom)); c != attendu {
			t.Fatalf("difficulté lue par %s : %d %v", nom, c, v)
		}
	}
	// la fiche partagée ne montre aux joueurs ni ses créatures ni le profil de combat d'un PNJ
	vues, err := b.s.mondeJoueur()
	if r, ok := vues[int64(v["id"].(float64))]; err != nil || !ok || r.Data["adversaires"] != nil {
		t.Fatalf("rencontre vue des joueurs : %v %v", err, r)
	}
}
