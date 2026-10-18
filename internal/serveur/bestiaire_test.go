package serveur

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Tchyze/cof_dm/internal/comptes"
)

// Fiches et règles d'essai, inventées pour les tests : aucune valeur ne vient d'un livre.
const gobelin = `{"nom":"Rampant des dunes","source":"officiel","ref":"essai-rampant","livre":"Livre d'essai","page":12,"nc":0.5,"categorie":"de chair","taille":"nain",
	"defense":13,"pv":5,"init":14,"carac":{"FOR":-1,"DEX":2},"attaques":[{"nom":"Dard","type":"C","bonus":1,"dm":"1d6"}]}`

const reglesEssai = `{"format":"cof-dm-regles","version":1,"titre":"Règles d'essai",
 "listes":{"peuples":["Sélénite","Marcheur"],"peuples_pnj":["Sélénite","Marcheur","Spectre"],"profils":["Veilleur"],"tailles":["nain","géant"],"categories":["de chair"]},
 "creature":{"archetypes":[{"cle":"trapu","description":"Solide."}],
  "table":{"géant":{"trapu":{"nc":4,"FOR":6,"DEX":-1,"CON":6,"forSup":true,"def":15,"pv":40,"init":8,"att":7,"dm":"2d6+6"}}},
  "boss":[{"cle":"meneur","libelle":"Meneur","att":1,"dm":1,"def":1,"pv":7}],
  "nc_par_rang_boss":1,"nc_par_rang_voie":1,"rangs_max_voie":2}}`

func TestBestiaire(t *testing.T) {
	b := nouveau(t)
	// création d'un monstre maison, validée
	c, v := b.local("POST", "/api/bestiaire", map[string]any{"nom": "Loup des brumes", "nc": 1, "pv": 12, "defense": 14})
	if c != 201 || v["source"] != "maison" || v["id"] == nil || v["crit"] != float64(20) {
		t.Fatalf("création : %d %v", c, v)
	}
	loup := "/api/bestiaire/" + strconv.Itoa(int(v["id"].(float64)))
	for _, mauvais := range []map[string]any{{"nom": ""}, {"nom": "X", "pv": 0}, {"nom": "X", "source": "volé"}, {"nom": "X", "attaques": []any{map[string]any{"type": "Z"}}}} {
		if c, _ := b.local("POST", "/api/bestiaire", mauvais); c != 422 {
			t.Fatalf("monstre invalide %v accepté : %d", mauvais, c)
		}
	}
	if c, v := b.local("PUT", loup, map[string]any{"nom": "Loup des brumes", "pv": 20}); c != 200 || v["pv"] != float64(20) {
		t.Fatalf("modification : %d %v", c, v)
	}
	// import : les fiches officielles sont ajoutées, puis mises à jour par leur référence
	c, v = b.local("POST", "/api/bestiaire/import", `{"monstres":[`+gobelin+`,{"nom":""}]}`)
	if c != 200 || v["crees"] != float64(1) || len(v["rejets"].([]any)) != 1 {
		t.Fatalf("import : %d %v", c, v)
	}
	if c, v = b.local("POST", "/api/bestiaire/import", `[`+strings.Replace(gobelin, `"pv":5`, `"pv":7`, 1)+`]`); v["mis_a_jour"] != float64(1) || v["crees"] != float64(0) {
		t.Fatalf("réimport : %d %v", c, v)
	}
	_, l := b.local("GET", "/api/bestiaire", nil)
	var liste []map[string]any
	json.Unmarshal([]byte(l["_liste"].(string)), &liste)
	if len(liste) != 2 || liste[1]["pv"] != float64(7) {
		t.Fatalf("bestiaire : %v", liste)
	}
	gob := "/api/bestiaire/" + strconv.Itoa(int(liste[1]["id"].(float64)))
	// une fiche officielle est verrouillée : on la duplique
	if c, _ := b.local("PUT", gob, map[string]any{"nom": "Rampant des dunes", "pv": 99}); c != 409 {
		t.Fatalf("fiche officielle modifiée : %d", c)
	}
	if c, v := b.local("POST", gob+"/dupliquer", nil); c != 201 || v["source"] != "maison" || v["ref"] != nil || v["nom"] != "Rampant des dunes (maison)" {
		t.Fatalf("duplication : %d %v", c, v)
	}
	// sans fichier de règles : pas de créateur de monstre, et des champs en saisie libre
	if c, _ := b.local("POST", "/api/bestiaire/calculer", map[string]any{"taille": "géant", "archetype": "trapu"}); c != 404 {
		t.Fatalf("créateur sans règles : %d", c)
	}
	if c, _ := b.local("GET", "/api/bestiaire/tables", nil); c != 404 {
		t.Fatalf("tables sans règles : %d", c)
	}
	champ := func(typ, cle string) map[string]any {
		_, v := b.local("GET", "/api/schema", nil)
		for _, x := range v["types"].([]any) {
			if x.(map[string]any)["cle"] != typ {
				continue
			}
			for _, c := range x.(map[string]any)["champs"].([]any) {
				if c.(map[string]any)["cle"] == cle {
					return c.(map[string]any)
				}
			}
		}
		t.Fatalf("champ %s.%s absent du schéma", typ, cle)
		return nil
	}
	if c := champ("pj", "peuple"); c["genre"] != "texte" || c["options"] != nil {
		t.Fatalf("champ peuple sans règles : %v", c)
	}
	if c, _ := b.local("POST", "/api/entites", map[string]any{"type": "pj", "nom": "Aldric", "data": map[string]any{"peuple": "Peuple de mon cru"}}); c != 201 {
		t.Fatalf("peuple en saisie libre : %d", c)
	}
	// le fichier de règles (ici inventé : aucune valeur ne vient d'un livre) apporte listes et tables
	if c, _ := b.local("POST", "/api/regles/import", `{"format":"autre"}`); c != 422 {
		t.Fatalf("fichier de règles d'un autre format : %d", c)
	}
	if c, v := b.local("POST", "/api/regles/import", reglesEssai); c != 200 || v["createur"] != true || v["listes"].(map[string]any)["peuples"] != float64(2) {
		t.Fatalf("import des règles : %d %v", c, v)
	}
	if c := champ("pj", "peuple"); len(c["options"].([]any)) != 2 {
		t.Fatalf("champ peuple avec règles : %v", c)
	}
	if c := champ("pnj", "peuple"); len(c["options"].([]any)) != 3 {
		t.Fatalf("champ peuple des PNJ : %v", c)
	}
	// créateur de monstre : un calcul, rien n'est enregistré
	if c, v := b.local("POST", "/api/bestiaire/calculer", map[string]any{"taille": "géant", "archetype": "trapu", "type_boss": "meneur", "rangs_boss": 1}); c != 200 || v["pv"] != float64(47) || v["nc"] != float64(5) {
		t.Fatalf("calcul d'une créature : %d %v", c, v)
	}
	if c, v := b.local("GET", "/api/bestiaire/tables", nil); c != 200 || v["creature"] == nil || len(v["listes"].(map[string]any)["tailles"].([]any)) != 2 {
		t.Fatalf("tables : %d %v", c, v)
	}
	// un second fichier ne retire pas ce qu'il ne dit pas
	if _, v := b.local("POST", "/api/regles/import", `{"format":"cof-dm-regles","listes":{"profils":["Veilleur"]}}`); v["createur"] != true || v["listes"].(map[string]any)["peuples"] != float64(2) {
		t.Fatalf("second fichier de règles : %v", v)
	}
	// profils, peuples et voies
	ref := `{"profils":[{"ref":"essai-veilleur","nom":"Veilleur","voies":["Voie de la vigie"]}],"voies":[{"ref":"essai-vigie","nom":"Voie de la vigie","type":"profil","rangs":[{"nom":"Œil ouvert"}]}]}`
	if c, v := b.local("POST", "/api/reference/import", ref); c != 200 || v["total"].(map[string]any)["voies"] != float64(1) {
		t.Fatalf("import des règles : %d %v", c, v)
	}
	if _, v := b.local("GET", "/api/reference", nil); len(v["profils"].([]any)) != 1 {
		t.Fatalf("règles : %v", v)
	}
	// l'export de l'univers emporte bestiaire et règles, et la restauration les rend
	x, err := b.s.store.Exporter()
	if err != nil || len(x.Documents) != 3 || len(x.Reference) == 0 || len(x.Regles) == 0 {
		t.Fatalf("export : %d documents, %v", len(x.Documents), err)
	}
	b.local("DELETE", loup, nil)
	b.local("DELETE", "/api/reference", nil)
	b.local("DELETE", "/api/regles", nil)
	if c := champ("pj", "peuple"); c["options"] != nil {
		t.Fatalf("champ peuple après le retrait des règles : %v", c)
	}
	if err := b.s.store.Restaurer(x, "test"); err != nil {
		t.Fatal(err)
	}
	if c := champ("pj", "peuple"); len(c["options"].([]any)) != 2 {
		t.Fatalf("règles après restauration : %v", c)
	}
	if c, _ := b.local("GET", loup, nil); c != 200 {
		t.Fatalf("monstre après restauration : %d", c)
	}
	if _, v := b.local("GET", "/api/reference", nil); len(v["voies"].([]any)) != 1 {
		t.Fatalf("règles après restauration : %v", v)
	}
	// un document invalide dans un export est refusé, et rien n'est modifié
	x.Documents[0].Data = json.RawMessage(`{"nom":"","pv":-3}`)
	if err := b.s.store.Restaurer(x, "test"); err == nil {
		t.Fatal("export au monstre invalide accepté")
	}
	if c, _ := b.local("GET", loup, nil); c != 200 {
		t.Fatalf("monstre après une restauration refusée : %d", c)
	}
	if c, _ := b.local("DELETE", loup, nil); c != 200 {
		t.Fatalf("suppression : %d", c)
	}
	if c, _ := b.local("GET", loup, nil); c != 404 {
		t.Fatalf("monstre supprimé encore lu : %d", c)
	}
}

// Le bestiaire suit les rôles : le MJ le modifie, l'auditeur le lit, le joueur ne le voit pas.
func TestBestiaireRoles(t *testing.T) {
	b, reg := bancServeur(t)
	reg.Creer("joueuse", "mot-de-passe-joueuse", false)
	reg.Modifier("joueuse", func(c *comptes.Compte) error { c.Roles[b.s.store.ID()] = comptes.Joueur; return nil })
	mj, audit, joueuse, passant := b.connecter("meneur"), b.connecter("audit"), b.connecter("joueuse"), b.connecter("passant")
	monstre := map[string]any{"nom": "Ogre", "pv": 30}
	if c, _, _ := b.proxy("POST", "/api/bestiaire", monstre, mj); c != 201 {
		t.Fatalf("création par le MJ : %d", c)
	}
	for nom, qui := range map[string]func(*http.Request){"audit": audit, "joueuse": joueuse, "passant": passant} {
		for _, x := range [][2]string{{"POST", "/api/bestiaire"}, {"POST", "/api/bestiaire/import"}, {"DELETE", "/api/bestiaire/1"}, {"POST", "/api/reference/import"}, {"DELETE", "/api/reference"},
			{"POST", "/api/regles/import"}, {"DELETE", "/api/regles"}, {"POST", "/api/bibliotheque"}} {
			if c, _, _ := b.proxy(x[0], x[1], monstre, qui); c != 403 {
				t.Fatalf("%s %s par %s : %d", x[0], x[1], nom, c)
			}
		}
	}
	// profils et voies : lus aussi par le joueur, qui en a besoin pour tenir sa feuille
	if c, _, _ := b.proxy("GET", "/api/reference", nil, joueuse); c != 200 {
		t.Fatalf("profils et voies lus par le joueur : %d", c)
	}
	if c, _, _ := b.proxy("GET", "/api/reference", nil, passant); c != 403 {
		t.Fatalf("profils et voies lus sans rôle : %d", c)
	}
	for _, u := range []string{"/api/bestiaire", "/api/bestiaire/1", "/api/bibliotheque", "/api/regles"} {
		if c, _, _ := b.proxy("GET", u, nil, audit); c != 200 {
			t.Fatalf("GET %s par l'auditeur : %d", u, c)
		}
		for nom, qui := range map[string]func(*http.Request){"joueuse": joueuse, "passant": passant} {
			if c, _, _ := b.proxy("GET", u, nil, qui); c != 403 {
				t.Fatalf("GET %s par %s : %d", u, nom, c)
			}
		}
	}
}

// Le bestiaire et les règles d'un univers se reprennent dans un autre, sans doublon.
func TestBibliothequeEntreUnivers(t *testing.T) {
	b := nouvelleAppli(t, t.TempDir())
	b.appel(false, "POST", "/api/bestiaire/import", json.RawMessage(`[`+gobelin+`,{"nom":"Loup des brumes"}]`))
	b.appel(false, "POST", "/api/regles/import", json.RawMessage(reglesEssai))
	b.appel(false, "POST", "/api/reference/import", json.RawMessage(`{"voies":[{"nom":"Voie de la vigie","type":"profil"}]}`))
	premier := b.a.st.Chemin()
	if c, v := b.appel(false, "POST", "/api/univers/nouveau", map[string]any{"nom": "Second"}); c != 200 {
		t.Fatalf("second univers : %d %v", c, v)
	}
	if c, v := b.appel(false, "POST", "/api/univers/bibliotheque", map[string]any{"source": b.a.st.Chemin()}); c != 422 {
		t.Fatalf("reprise depuis l'univers ouvert : %d %v", c, v)
	}
	if c, v := b.appel(false, "POST", "/api/univers/bibliotheque", map[string]any{"source": filepath.Join(b.dir, "absent.cofdm")}); c != 422 {
		t.Fatalf("reprise depuis un fichier absent : %d %v", c, v)
	}
	b.appel(false, "POST", "/api/bestiaire", map[string]any{"nom": "loup des brumes", "pv": 3}) // déjà là, version de cet univers
	c, v := b.appel(false, "POST", "/api/univers/bibliotheque", map[string]any{"source": premier})
	if c != 200 || v["monstres"].(map[string]any)["crees"] != float64(1) || v["deja_presents"] != float64(1) || v["total"].(map[string]any)["voies"] != float64(1) || v["regles"].(map[string]any)["createur"] != true {
		t.Fatalf("reprise : %d %v", c, v)
	}
	// une seconde reprise ne double rien
	if _, v := b.appel(false, "POST", "/api/univers/bibliotheque", map[string]any{"source": premier}); v["monstres"].(map[string]any)["crees"] != float64(0) {
		t.Fatalf("seconde reprise : %v", v)
	}
	if l, _ := b.a.st.ListerDocs("monstres"); len(l) != 2 {
		t.Fatalf("bestiaire du second univers : %d monstres", len(l))
	}
	if c, _ := b.appel(true, "POST", "/api/univers/bibliotheque", map[string]any{"source": premier}); c != 403 {
		t.Fatalf("reprise depuis le réseau : %d", c)
	}
}
