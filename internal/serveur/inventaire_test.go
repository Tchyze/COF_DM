package serveur

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/Tchyze/cof_dm/internal/comptes"
)

// Une fiche « Objet » et l'inventaire d'un PJ : le Détenteur suit dans les deux sens, la ligne prend
// le nom et les chiffres de la fiche, et une fiche effacée reste dans l'inventaire en objet courant.
func TestInventaireObjets(t *testing.T) {
	b := nouveau(t)
	aldric := b.fiche("pj", "Aldric", map[string]any{"statut": "actif"})
	brune := b.fiche("pj", "Brune", map[string]any{"statut": "actif"})
	feuille := func(pj float64) string { return fmt.Sprintf("/api/entites/%v/feuille", pj) }
	b.local("PUT", feuille(aldric), map[string]any{"pv": 10, "inventaire": []any{map[string]any{"nom": "Corde"}}})
	inventaire := func(pj float64) []map[string]any {
		_, f := b.local("GET", feuille(pj), nil)
		var l []map[string]any
		j, _ := json.Marshal(f["inventaire"])
		_ = json.Unmarshal(j, &l)
		return l
	}
	ligne := func(pj, objet float64) map[string]any {
		for _, o := range inventaire(pj) {
			if o["fiche"] == objet {
				return o
			}
		}
		return nil
	}
	detenteur := func(objet float64) any {
		_, v := b.local("GET", fmt.Sprintf("/api/entites/%v", objet), nil)
		return v["data"].(map[string]any)["detenteur"]
	}

	// l'emplacement d'une fiche se range par sa clé, qu'on donne la clé ou le nom ; inconnu : refusé
	c, v := b.local("POST", "/api/entites", map[string]any{"type": "objet", "nom": "Heaume", "data": map[string]any{"equipement": map[string]any{"emplacement": "Tête"}}})
	if c != 201 || v["data"].(map[string]any)["equipement"].(map[string]any)["emplacement"] != "tete" {
		t.Fatalf("emplacement par son nom : %d %v", c, v)
	}
	if c, v := b.local("PATCH", fmt.Sprintf("/api/entites/%v", v["id"]), map[string]any{"data": map[string]any{"equipement": map[string]any{"emplacement": "dos"}}}); c != 422 || !strings.Contains(fmt.Sprint(v["detail"]), "tete (Tête)") {
		t.Fatalf("emplacement inconnu : %d %v", c, v)
	}
	// donnée à Aldric : dans son inventaire, avec ses chiffres
	epee := b.fiche("objet", "Épée de givre", map[string]any{"detenteur": aldric,
		"equipement": map[string]any{"emplacement": "main_principale", "bonus": []any{map[string]any{"cible": "att", "valeur": 1}}, "arme": map[string]any{"type": "C", "dm": "1d8"}}})
	o := ligne(aldric, epee)
	if o == nil || o["nom"] != "Épée de givre" || o["emplacement"] != "main_principale" || o["arme"] == nil || len(inventaire(aldric)) != 2 {
		t.Fatalf("objet donné : %v", inventaire(aldric))
	}
	// portée : l'arme compte ; renommée, la ligne suit et reste portée
	l := inventaire(aldric)
	for _, x := range l {
		if x["fiche"] == epee {
			x["porte"] = true
		}
	}
	if c, v := b.local("PUT", feuille(aldric), map[string]any{"pv": 10, "inventaire": l}); c != 200 || !strings.Contains(fmt.Sprint(v["attaques"]), "Épée de givre") {
		t.Fatalf("portée : %d %v", c, v["attaques"])
	}
	b.local("PATCH", fmt.Sprintf("/api/entites/%v", epee), map[string]any{"nom": "Givrelame"})
	if o = ligne(aldric, epee); o["nom"] != "Givrelame" || o["porte"] != true {
		t.Fatalf("renommée : %v", o)
	}
	// un autre emplacement : elle est ôtée (sa nouvelle place peut être prise)
	b.local("PATCH", fmt.Sprintf("/api/entites/%v", epee), map[string]any{"data": map[string]any{"equipement": map[string]any{"emplacement": "deux_mains", "arme": map[string]any{"dm": "2d6"}}}})
	if o = ligne(aldric, epee); o["porte"] == true || o["emplacement"] != "deux_mains" || o["bonus"] != nil {
		t.Fatalf("chiffres changés : %v", o)
	}
	// donnée à Brune : elle quitte l'inventaire d'Aldric
	b.local("PATCH", fmt.Sprintf("/api/entites/%v", epee), map[string]any{"data": map[string]any{"detenteur": brune}})
	if ligne(aldric, epee) != nil || ligne(brune, epee) == nil {
		t.Fatalf("donnée à Brune : %v / %v", inventaire(aldric), inventaire(brune))
	}
	// reprise par Aldric depuis sa feuille : il en est le détenteur, Brune ne l'a plus
	if c, v := b.local("PUT", feuille(aldric), map[string]any{"pv": 10, "inventaire": []any{map[string]any{"nom": "Corde"}, map[string]any{"fiche": epee, "nom": "peu importe"}}}); c != 200 {
		t.Fatalf("liée depuis la feuille : %d %v", c, v)
	}
	if detenteur(epee) != aldric || ligne(brune, epee) != nil || ligne(aldric, epee)["nom"] != "Givrelame" {
		t.Fatalf("reprise : détenteur %v, Brune %v", detenteur(epee), inventaire(brune))
	}
	// ôtée de sa feuille : plus de détenteur
	b.local("PUT", feuille(aldric), map[string]any{"pv": 10, "inventaire": []any{map[string]any{"nom": "Corde"}}})
	if detenteur(epee) != nil {
		t.Fatalf("ôtée : détenteur %v", detenteur(epee))
	}
	// une fiche qui n'est pas un objet, ou deux fois le même objet : refusé
	if c, _ := b.local("PUT", feuille(aldric), map[string]any{"pv": 10, "inventaire": []any{map[string]any{"fiche": brune, "nom": "Brune"}}}); c != 422 {
		t.Fatalf("PJ dans l'inventaire : %d", c)
	}
	if c, _ := b.local("PUT", feuille(aldric), map[string]any{"pv": 10, "inventaire": []any{map[string]any{"fiche": epee, "nom": "a"}, map[string]any{"fiche": epee, "nom": "b"}}}); c != 422 {
		t.Fatalf("objet en double : %d", c)
	}
	// effacée : la ligne reste, en objet courant
	b.local("PATCH", fmt.Sprintf("/api/entites/%v", epee), map[string]any{"data": map[string]any{"detenteur": aldric}})
	if c, v := b.local("DELETE", fmt.Sprintf("/api/entites/%v?definitif=1", epee), nil); c != 200 {
		t.Fatalf("effacement : %d %v", c, v)
	}
	if l := inventaire(aldric); len(l) != 2 || l[1]["nom"] != "Givrelame" || l[1]["fiche"] != nil {
		t.Fatalf("après effacement : %v", l)
	}
}

// Le joueur porte un objet du monde, mais ne le retire pas de son inventaire et n'en lie pas un.
func TestInventaireObjetsJoueur(t *testing.T) {
	b, _ := bancReseau(t)
	pj := b.fiche("pj", "Aldric", map[string]any{"statut": "actif"})
	url := fmt.Sprintf("/api/entites/%v/feuille", pj)
	b.local("PUT", url, map[string]any{"pv": 10})
	bague := b.fiche("objet", "Bague du lac", map[string]any{"detenteur": pj, "equipement": map[string]any{"emplacement": "anneau", "bonus": []any{map[string]any{"cible": "def", "valeur": 1}}}})
	autre := b.fiche("objet", "Couronne", map[string]any{"equipement": map[string]any{"emplacement": "tete"}})
	b.local("POST", "/api/acces/joueurs", map[string]any{"active": true})
	b.local("PUT", "/api/joueurs/edition", map[string]any{"active": true})
	ecrire := func(inv ...any) (int, string) {
		return b.joueurLibre("PUT", url, map[string]any{"pv": 10, "inventaire": inv}, true)
	}
	if c, corps := ecrire(); c != 403 || !strings.Contains(corps, "Bague du lac") {
		t.Fatalf("objet du monde retiré par le joueur : %d %s", c, corps)
	}
	c, corps := ecrire(map[string]any{"id": 1, "nom": "Ma bague", "porte": true}, map[string]any{"fiche": autre, "nom": "Couronne"})
	if c != 200 || !strings.Contains(corps, `"defense":1`) || !strings.Contains(corps, `"nom":"Bague du lac"`) {
		t.Fatalf("bague portée par le joueur : %d %s", c, corps)
	}
	if strings.Contains(corps, fmt.Sprintf(`"fiche":%v`, autre)) {
		t.Fatalf("objet du monde lié par le joueur : %s", corps)
	}
	_, v := b.local("GET", fmt.Sprintf("/api/entites/%v", autre), nil)
	if v["data"].(map[string]any)["detenteur"] != nil {
		t.Fatalf("détenteur donné par le joueur : %v", v["data"])
	}
	_ = bague
}

// scenarioInventaire : ce qu'un joueur tente sur l'inventaire de son PJ, par une clé dont il change
// la casse, ou sans inventaire du tout. Rien ne doit passer : ni lecture d'un objet secret, ni vol à
// un autre PJ, ni bonus forgé.
func scenarioInventaire(t *testing.T, cle string, pj, autrePJ, secret, volable float64,
	ecrire func(corps map[string]any) (int, string), lireMJ func(url string) string) {
	t.Helper()
	feuille := fmt.Sprintf("/api/entites/%v/feuille", pj)
	lignes := func(l ...any) map[string]any { return map[string]any{"pv": 10, cle: l} }

	c, corps := ecrire(lignes(map[string]any{"nom": "x", "quantite": 1, "fiche": secret}))
	if c != 200 || strings.Contains(corps, "SECRET-NOM") || strings.Contains(corps, `"defense":17`) {
		t.Fatalf("%s : objet secret lu par le joueur : %d %s", cle, c, corps)
	}
	if s := lireMJ(fmt.Sprintf("/api/entites/%v", secret)); strings.Contains(s, "detenteur") {
		t.Fatalf("%s : le joueur devient détenteur de l'objet secret : %s", cle, s)
	}

	c, corps = ecrire(lignes(map[string]any{"nom": "x", "quantite": 1, "fiche": volable}))
	if c != 200 || strings.Contains(corps, "VOLABLE-NOM") {
		t.Fatalf("%s : objet d'un autre PJ pris par le joueur : %d %s", cle, c, corps)
	}
	if s := lireMJ(fmt.Sprintf("/api/entites/%v/feuille", autrePJ)); !strings.Contains(s, "VOLABLE-NOM") {
		t.Fatalf("%s : l'objet a quitté l'inventaire de l'autre PJ : %s", cle, s)
	}
	if s := lireMJ(fmt.Sprintf("/api/entites/%v", volable)); !strings.Contains(s, fmt.Sprintf(`"detenteur":%v`, autrePJ)) {
		t.Fatalf("%s : le détenteur a changé : %s", cle, s)
	}
	if s := lireMJ(feuille); strings.Contains(s, "VOLABLE-NOM") {
		t.Fatalf("%s : objet d'un autre PJ dans la feuille : %s", cle, s)
	}

	c, corps = ecrire(lignes(map[string]any{"nom": "forge", "quantite": 1, "bonus": []any{map[string]any{"cible": "def", "valeur": 9}},
		"arme": map[string]any{"dm": "1d4"}}))
	if c != 200 {
		t.Fatalf("%s : ligne libre refusée : %d %s", cle, c, corps)
	}
	if s := lireMJ(feuille); strings.Contains(s, `"valeur":9`) || strings.Contains(s, "1d4") || !strings.Contains(s, "forge") {
		t.Fatalf("%s : bonus forgé enregistré : %s", cle, s)
	}

	// un identifiant qui n'est pas un objet : aucune réponse ne dit ce qu'il désigne
	if c, corps = ecrire(lignes(map[string]any{"nom": "x", "quantite": 1, "fiche": pj})); c != 200 || strings.Contains(corps, "objet du monde") {
		t.Fatalf("%s : nature d'une fiche révélée : %d %s", cle, c, corps)
	}
}

// scenarioSansInventaire : une feuille sans aucune clé d'inventaire garde les lignes d'avant, et son
// ancien texte d'équipement n'y fait pas entrer de bonus.
func scenarioSansInventaire(t *testing.T, pj float64, ecrire func(corps map[string]any) (int, string), lireMJ func(url string) string) {
	t.Helper()
	feuille := fmt.Sprintf("/api/entites/%v/feuille", pj)
	if c, corps := ecrire(map[string]any{"pv": 10, "inventaire": []any{map[string]any{"nom": "gardee", "quantite": 1}}}); c != 200 {
		t.Fatalf("ligne libre : %d %s", c, corps)
	}
	if c, corps := ecrire(map[string]any{"pv": 10, "armure": map[string]any{"nom": "forgee", "bonus": 15}}); c != 200 {
		t.Fatalf("sans inventaire : %d %s", c, corps)
	}
	if s := lireMJ(feuille); strings.Contains(s, "forgee") || strings.Contains(s, `"valeur":15`) || !strings.Contains(s, "gardee") {
		t.Fatalf("bonus forgé par l'ancienne armure : %s", s)
	}
}

// Page libre de l'édition de bureau : même contrôle que pour un compte.
func TestInventaireCassePageLibre(t *testing.T) {
	for _, cle := range []string{"Inventaire", "INVENTAIRE", "inVentaire"} {
		t.Run(cle, func(t *testing.T) {
			b, _ := bancReseau(t)
			pj := b.fiche("pj", "Aldric", map[string]any{"statut": "actif"})
			autre := b.fiche("pj", "Brune", map[string]any{"statut": "actif"})
			b.local("PUT", fmt.Sprintf("/api/entites/%v/feuille", pj), map[string]any{"pv": 10})
			b.local("PUT", fmt.Sprintf("/api/entites/%v/feuille", autre), map[string]any{"pv": 10})
			chiffres := map[string]any{"emplacement": "tete", "bonus": []any{map[string]any{"cible": "def", "valeur": 7}}}
			secret := b.fiche("objet", "SECRET-NOM", map[string]any{"equipement": chiffres})
			volable := b.fiche("objet", "VOLABLE-NOM", map[string]any{"detenteur": autre})
			b.local("POST", "/api/acces/joueurs", map[string]any{"active": true})
			b.local("PUT", "/api/joueurs/edition", map[string]any{"active": true})
			url := fmt.Sprintf("/api/entites/%v/feuille", pj)
			ecrire := func(corps map[string]any) (int, string) { return b.joueurLibre("PUT", url, corps, true) }
			lireMJ := func(u string) string { _, v := b.local("GET", u, nil); return toJSON(v) }
			scenarioInventaire(t, cle, pj, autre, secret, volable, ecrire, lireMJ)
			scenarioSansInventaire(t, pj, ecrire, lireMJ)
		})
	}
}

// Compte joueur d'un serveur.
func TestInventaireCasseCompteJoueur(t *testing.T) {
	for _, cle := range []string{"Inventaire", "INVENTAIRE", "inVentaire"} {
		t.Run(cle, func(t *testing.T) {
			b, reg := bancServeur(t)
			mj := b.connecter("meneur")
			creer := func(typ, nom string, data map[string]any) float64 {
				c, v, _ := b.proxy("POST", "/api/entites", map[string]any{"type": typ, "nom": nom, "data": data}, mj)
				if c != 201 {
					t.Fatalf("création de %s : %d %v", nom, c, v)
				}
				return v["id"].(float64)
			}
			pj, autre := creer("pj", "Aldric", nil), creer("pj", "Brune", nil)
			for _, id := range []float64{pj, autre} {
				b.proxy("PUT", fmt.Sprintf("/api/entites/%v/feuille", id), map[string]any{"pv": 10}, mj)
			}
			chiffres := map[string]any{"emplacement": "tete", "bonus": []any{map[string]any{"cible": "def", "valeur": 7}}}
			secret := creer("objet", "SECRET-NOM", map[string]any{"equipement": chiffres})
			volable := creer("objet", "VOLABLE-NOM", map[string]any{"detenteur": autre})
			b.proxy("POST", fmt.Sprintf("/api/entites/%v/partage", pj), map[string]any{"partage": true}, mj)
			if _, err := reg.Creer("joueuse", "mot-de-passe-joueuse", false); err != nil {
				t.Fatal(err)
			}
			reg.Modifier("joueuse", func(c *comptes.Compte) error {
				c.Admis = append(c.Admis, b.s.store.ID())
				c.Roles[b.s.store.ID()] = comptes.Joueur
				c.Persos[b.s.store.ID()] = int64(pj)
				return nil
			})
			joueuse := b.connecter("joueuse")
			if c, _, _ := b.proxy("PUT", "/api/joueurs/edition", map[string]any{"active": true}, mj); c != 200 {
				t.Fatalf("ouverture de l'édition : %d", c)
			}
			url := fmt.Sprintf("/api/entites/%v/feuille", pj)
			ecrire := func(corps map[string]any) (int, string) {
				c, v, _ := b.proxy("PUT", url, corps, joueuse)
				return c, toJSON(v)
			}
			lireMJ := func(u string) string { _, v, _ := b.proxy("GET", u, nil, mj); return toJSON(v) }
			scenarioInventaire(t, cle, pj, autre, secret, volable, ecrire, lireMJ)
			scenarioSansInventaire(t, pj, ecrire, lireMJ)
		})
	}
}
