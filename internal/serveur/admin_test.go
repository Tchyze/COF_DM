package serveur

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Tchyze/cof_dm/internal/comptes"
)

// marqueContenu : ce qui est écrit dans un univers et qu'aucune réponse faite à l'administrateur ne
// doit porter.
const marqueContenu = "CONTENU-DU-MJ"

// L'administrateur gère le serveur sans rien voir du contenu des univers : aucune route d'un univers
// ne lui répond, et ce que l'administration lui montre ne porte rien de ce que le MJ a écrit.
func TestAdminNeLitAucuneFiche(t *testing.T) {
	a, reg, connecter := serveurMulti(t)
	chef, meneur := connecter("chef"), connecter("meneur")
	id := a.st.ID()
	donner(reg, "meneur", id, comptes.MJ)
	creer := func(typ string) string {
		t.Helper()
		c, v := meneur.faire("POST", "/api/entites", map[string]any{"type": typ, "nom": marqueContenu + " " + typ,
			"data": map[string]any{"secrets": marqueContenu}, "tags": []string{"marque"}})
		if c != 201 {
			t.Fatalf("fiche %s : %d %v", typ, c, v)
		}
		return strconv.Itoa(int(v["id"].(float64)))
	}
	pnj, pj := creer("pnj"), creer("pj")
	meneur.faire("POST", "/api/entites/"+pj+"/partage", map[string]any{"partage": true})
	meneur.faire("PUT", "/api/entites/"+pj+"/feuille", map[string]any{"pv": 9, "notes": marqueContenu})
	w := meneur.brut("POST", "/api/entites/"+pnj+"/medias?nom=a.png", []byte("\x89PNG\r\n\x1a\n"+strings.Repeat("\x00", 32)), "Content-Type", "image/png")
	if w.Code != 201 {
		t.Fatalf("image : %d %s", w.Code, w.Body.String())
	}
	media := strings.Split(strings.Split(w.Body.String(), `"fichier":"`)[1], `"`)[0]
	meneur.faire("POST", "/api/sauvegarde", nil)

	// ni par l'univers par défaut, ni en le désignant : rien
	lectures := []string{"/api/entites", "/api/entites/" + pnj, "/api/entites/" + pj + "/feuille", "/api/entites?q=contenu",
		"/api/stats", "/api/schema", "/api/calendrier", "/api/parametres", "/api/resoudre?nom=" + marqueContenu, "/medias/" + media,
		"/api/journal", "/api/export", "/api/export/obsidian", "/api/secrets", "/api/graphe", "/api/chronologie", "/api/table",
		"/api/combats", "/api/liens-morts", "/api/bestiaire", "/api/regles", "/api/reference", "/api/fonctionnalites",
		"/api/joueurs/comptes", "/api/joueur/pj", "/api/joueur/etat", "/api/sync/copies", "/api/conflits", "/api/univers",
		"/api/bibliotheque", "/api/reference/export"}
	for _, u := range lectures {
		for _, x := range [][]string{nil, {enTeteUnivers, id}} {
			w := chef.brut("GET", u, nil, x...)
			if w.Code != 403 || strings.Contains(w.Body.String(), marqueContenu) {
				t.Fatalf("GET %s %v par l'administrateur : %d %s", u, x, w.Code, w.Body.String())
			}
		}
		if w := chef.brut("GET", u+map[bool]string{true: "&", false: "?"}[strings.Contains(u, "?")]+"univers="+id, nil); w.Code != 403 {
			t.Fatalf("GET %s?univers= par l'administrateur : %d", u, w.Code)
		}
	}
	for _, x := range [][2]string{{"POST", "/api/entites"}, {"PATCH", "/api/entites/" + pnj}, {"DELETE", "/api/entites/" + pnj},
		{"POST", "/api/restaurer"}, {"POST", "/api/import/univers"}, {"POST", "/api/sauvegarde"}, {"PUT", "/api/joueurs/edition"},
		{"POST", "/api/univers/restaurer"}, {"POST", "/api/univers/bibliotheque"}, {"POST", "/api/sync/copie"}} {
		if c, _ := chef.faire(x[0], x[1], map[string]any{"type": "pnj", "nom": "Intrus"}, enTeteUnivers, id); c != 403 {
			t.Fatalf("%s %s par l'administrateur : %d", x[0], x[1], c)
		}
	}
	if n, _, _ := a.st.Compter(); n["pnj"] != 1 {
		t.Fatalf("l'administrateur a écrit dans l'univers : %v", n)
	}
	// il n'entre dans aucun univers, et ne peut pas s'en donner les moyens
	if _, v := chef.faire("GET", "/api/mes-univers", nil); len(v["univers"].([]any)) != 0 {
		t.Fatalf("univers de l'administrateur : %v", v)
	}
	if c, _ := chef.faire("POST", "/api/mes-univers/courant", map[string]any{"id": id}); c != 404 {
		t.Fatalf("entrée de l'administrateur dans un univers : %d", c)
	}
	if c, _ := chef.faire("POST", "/api/compte/jetons", map[string]any{"nom": "script"}); c != 409 {
		t.Fatalf("jeton d'API d'un administrateur : %d", c)
	}
	if c, _ := chef.faire("PATCH", "/api/admin/comptes/chef", map[string]any{"roles": map[string]string{id: comptes.MJ}}); c != 409 {
		t.Fatalf("rôle de MJ donné à un administrateur : %d", c)
	}
	// ce que l'administration lui montre : rien du contenu
	for _, u := range []string{"/api/admin/univers", "/api/admin/comptes", "/api/univers/version", "/api/acces", "/api/compte", "/api/sante"} {
		w := chef.brut("GET", u, nil)
		if w.Code != 200 || strings.Contains(w.Body.String(), marqueContenu) || strings.Contains(w.Body.String(), `"pj"`) {
			t.Fatalf("GET %s : %d %s", u, w.Code, w.Body.String())
		}
	}
	// et son passage n'est pas au journal de l'univers
	l, _ := a.st.Journal(200)
	for _, x := range l {
		if x.Auteur == "chef" {
			t.Fatalf("l'administrateur au journal de l'univers : %+v", x)
		}
	}
}

// L'administration est au seul administrateur, connecté par son mot de passe.
func TestAdminReserve(t *testing.T) {
	a, reg, connecter := serveurMulti(t)
	meneur := connecter("meneur")
	donner(reg, "meneur", a.st.ID(), comptes.MJ)
	_, v := meneur.faire("POST", "/api/compte/jetons", map[string]any{"nom": "script"})
	script := &navigateur{t: t, a: a, cookies: map[string]*http.Cookie{}, jeton: v["jeton"].(string)}
	anonyme := &navigateur{t: t, a: a, cookies: map[string]*http.Cookie{}}
	for _, u := range []string{"/api/admin/univers", "/api/admin/comptes"} {
		if c, _ := anonyme.faire("GET", u, nil); c != 401 {
			t.Fatalf("GET %s sans compte : %d", u, c)
		}
		for _, n := range []*navigateur{meneur, script} {
			if c, _ := n.faire("GET", u, nil); c != 403 {
				t.Fatalf("GET %s par un MJ : %d", u, c)
			}
		}
	}
	if c, _ := meneur.faire("POST", "/api/admin/univers", map[string]any{"nom": "Pirate"}); c != 403 || len(a.tous) != 1 {
		t.Fatalf("univers créé par un MJ : %d", c)
	}
	// sur le bureau, l'administration n'existe pas
	b := nouvelleAppli(t, t.TempDir())
	if c, _ := b.local("GET", "/api/admin/univers", nil); c != 404 {
		t.Fatalf("administration sur le bureau : %d", c)
	}
}

// Créer, renommer, archiver, désarchiver et supprimer un univers : le fichier suit, les rôles
// restent (sauf à la suppression), et rien ne s'ouvre à côté d'un univers archivé.
func TestAdminUnivers(t *testing.T) {
	a, reg, connecter := serveurMulti(t)
	chef, meneur := connecter("chef"), connecter("meneur")
	premier := a.st.ID()
	if c, _ := chef.faire("POST", "/api/admin/univers", map[string]any{"nom": "  "}); c != 422 {
		t.Fatalf("univers sans nom : %d", c)
	}
	if c, _ := chef.faire("POST", "/api/admin/univers", map[string]any{"nom": nomParDefaut}); c != 409 {
		t.Fatalf("nom déjà pris : %d", c)
	}
	id := creerUnivers(t, chef, "Les Marches")
	donner(reg, "meneur", id, comptes.MJ)
	meneur.faire("POST", "/api/entites", map[string]any{"type": "lieu", "nom": "Fort Boueux"}, enTeteUnivers, id)
	fiche := func(id string) map[string]any {
		t.Helper()
		w := chef.brut("GET", "/api/admin/univers", nil)
		var l []map[string]any
		if err := jsonDe(w, &l); err != nil {
			t.Fatal(err)
		}
		for _, f := range l {
			if f["id"] == id {
				return f
			}
		}
		return nil
	}
	if f := fiche(id); f["nom"] != "Les Marches" || f["archive"] != false || f["taille"].(float64) == 0 || toJSON(f["comptes"]) != `[{"nom":"meneur","role":"mj"}]` {
		t.Fatalf("fiche de l'univers : %v", f)
	}
	// renommer : le fichier change de nom, le MJ y reste
	if c, v := chef.faire("PATCH", "/api/admin/univers/"+id, map[string]any{"nom": nomParDefaut}); c != 409 {
		t.Fatalf("renommage sur un nom pris : %d %v", c, v)
	}
	if c, v := chef.faire("PATCH", "/api/admin/univers/"+id, map[string]any{"nom": "Les Marches du Nord"}); c != 200 || v["nom"] != "Les Marches du Nord" {
		t.Fatalf("renommage : %d %v", c, v)
	}
	if _, err := os.Stat(filepath.Join(a.DossierUnivers(), "Les Marches du Nord.cofdm")); err != nil {
		t.Fatal(err)
	}
	if c, v := meneur.faire("GET", "/api/stats", nil, enTeteUnivers, id); c != 200 || v["compte"].(map[string]any)["lieu"] != float64(1) {
		t.Fatalf("univers renommé, pour son MJ : %d %v", c, v)
	}
	// archiver : il n'est plus servi, ni listé, ni rouvert au démarrage
	if c, v := chef.faire("PATCH", "/api/admin/univers/"+id, map[string]any{"archive": true}); c != 200 || v["archive"] != true {
		t.Fatalf("archivage : %d %v", c, v)
	}
	if c, _ := meneur.faire("GET", "/api/stats", nil, enTeteUnivers, id); c != 404 {
		t.Fatalf("univers archivé, pour son MJ : %d", c)
	}
	if _, v := meneur.faire("GET", "/api/mes-univers", nil); len(v["univers"].([]any)) != 0 {
		t.Fatalf("univers archivé listé : %v", v)
	}
	if _, err := os.Stat(filepath.Join(a.DossierArchives(), "Les Marches du Nord.cofdm")); err != nil {
		t.Fatal(err)
	}
	if c, _ := chef.faire("PATCH", "/api/admin/univers/"+premier, map[string]any{"archive": true}); c != 409 {
		t.Fatalf("dernier univers servi archivé : %d", c)
	}
	a.Fermer()
	a = NouvelleApplication(Options{Version: "test", DossierDon: a.opt.DossierDon, Port: 8100, Comptes: reg,
		Frontend: fstest.MapFS{"index.html": {Data: []byte("<html>")}}})
	if err := a.Demarrer(""); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.Fermer)
	a.opt.acces.pause = 0
	if len(a.tous) != 1 || a.tous[id] != nil {
		t.Fatalf("univers archivé rouvert au démarrage : %d univers", len(a.tous))
	}
	chef, meneur = reconnecter(t, a, "chef"), reconnecter(t, a, "meneur")
	if f := fiche(id); f == nil || f["archive"] != true || toJSON(f["comptes"]) != `[{"nom":"meneur","role":"mj"}]` {
		t.Fatalf("univers archivé, après redémarrage : %v", f)
	}
	// désarchiver : de nouveau servi, avec ses rôles
	if c, v := chef.faire("PATCH", "/api/admin/univers/"+id, map[string]any{"archive": false}); c != 200 || v["archive"] != false {
		t.Fatalf("désarchivage : %d %v", c, v)
	}
	if c, _ := meneur.faire("GET", "/api/stats", nil, enTeteUnivers, id); c != 200 {
		t.Fatalf("univers désarchivé, pour son MJ : %d", c)
	}
	// supprimer : le nom retapé, une sauvegarde avant, les rôles retirés
	if c, _ := chef.faire("DELETE", "/api/admin/univers/"+id, map[string]any{"nom": "Les Marches"}); c != 422 || len(a.tous) != 2 {
		t.Fatalf("suppression sans le bon nom : %d", c)
	}
	c, v := chef.faire("DELETE", "/api/admin/univers/"+id, map[string]any{"nom": "Les Marches du Nord"})
	if c != 200 || len(a.tous) != 1 || !strings.Contains(toJSON(v), "avant-suppression") {
		t.Fatalf("suppression : %d %v", c, v)
	}
	if _, err := os.Stat(filepath.Join(a.DossierUnivers(), "Les Marches du Nord.cofdm")); !os.IsNotExist(err) {
		t.Fatal("fichier de l'univers supprimé toujours là")
	}
	if l, _ := filepath.Glob(filepath.Join(a.opt.DossierDon, "sauvegardes", id, "*_avant-suppression.cofdm")); len(l) != 1 {
		t.Fatalf("sauvegarde d'avant la suppression : %v", l)
	}
	if c, _ := reg.Lire("meneur"); c.Role(id) != "" {
		t.Fatal("rôle gardé dans un univers supprimé")
	}
	if c, _ := chef.faire("DELETE", "/api/admin/univers/"+premier, map[string]any{"nom": nomParDefaut}); c != 409 {
		t.Fatalf("dernier univers supprimé : %d", c)
	}
	// un univers archivé se supprime aussi
	autre := creerUnivers(t, chef, "Brouillon")
	chef.faire("PATCH", "/api/admin/univers/"+autre, map[string]any{"archive": true})
	if c, _ := chef.faire("DELETE", "/api/admin/univers/"+autre, map[string]any{"nom": "Brouillon"}); c != 200 || fiche(autre) != nil {
		t.Fatalf("suppression d'un univers archivé : %d", c)
	}
	if l, _ := os.ReadDir(a.DossierArchives()); len(l) != 0 {
		t.Fatalf("archives après suppression : %v", l)
	}
}

// reconnecter : une nouvelle session sur l'application a.
func reconnecter(t *testing.T, a *Application, nom string) *navigateur {
	t.Helper()
	n := &navigateur{t: t, a: a, cookies: map[string]*http.Cookie{}}
	if c, _ := n.faire("POST", "/api/acces/connexion", map[string]any{"nom": nom, "mdp": "mot-de-passe-" + nom}); c != 200 {
		t.Fatalf("connexion de %s : %d", nom, c)
	}
	return n
}

// Les comptes : l'administrateur donne les rôles MJ et auditeur, et admet un joueur dans un univers ;
// c'est le MJ qui l'y joint.
func TestAdminComptes(t *testing.T) {
	a, reg, connecter := serveurMulti(t)
	chef := connecter("chef")
	id := a.st.ID()
	if c, v := chef.faire("POST", "/api/admin/comptes", map[string]any{"nom": "Lea", "mdp": "mot-de-passe-lea"}); c != 201 || v["nom"] != "lea" || v["mdp"] != nil {
		t.Fatalf("création de compte : %d %v", c, v)
	}
	if c, _ := chef.faire("POST", "/api/admin/comptes", map[string]any{"nom": "court", "mdp": "court"}); c != 422 {
		t.Fatalf("mot de passe trop court : %d", c)
	}
	if c, _ := chef.faire("PATCH", "/api/admin/comptes/lea", map[string]any{"roles": map[string]string{id: comptes.Joueur}}); c != 422 {
		t.Fatalf("rôle joueur donné par l'administrateur : %d", c)
	}
	if c, _ := chef.faire("PATCH", "/api/admin/comptes/lea", map[string]any{"roles": map[string]string{"inconnu": comptes.MJ}}); c != 404 {
		t.Fatalf("rôle dans un univers inconnu : %d", c)
	}
	if c, v := chef.faire("PATCH", "/api/admin/comptes/lea", map[string]any{"roles": map[string]string{id: comptes.Auditeur}}); c != 200 || toJSON(v["roles"]) != `{"`+id+`":"auditeur"}` {
		t.Fatalf("rôle d'auditeur : %d %v", c, v)
	}
	lea := reconnecter(t, a, "lea")
	if c, _ := lea.faire("GET", "/api/entites", nil); c != 200 {
		t.Fatalf("lecture par l'auditrice : %d", c)
	}
	if c, v := chef.faire("PATCH", "/api/admin/comptes/lea", map[string]any{"roles": map[string]string{id: ""}, "admis": map[string]bool{id: true}}); c != 200 || toJSON(v["admis"]) != `["`+id+`"]` {
		t.Fatalf("admission : %d %v", c, v)
	}
	if c, _ := lea.faire("GET", "/api/entites", nil); c != 403 {
		t.Fatalf("admise, pas encore jointe : %d", c)
	}
	if c, _ := chef.faire("PATCH", "/api/admin/comptes/lea", map[string]any{"mdp": "un-autre-mot-de-passe", "inactif": true}); c != 200 {
		t.Fatalf("mot de passe et désactivation : %d", c)
	}
	if c, _ := lea.faire("GET", "/api/compte", nil); c != 401 {
		t.Fatalf("session d'un compte désactivé : %d", c)
	}
	if c, _ := chef.faire("PATCH", "/api/admin/comptes/chef", map[string]any{"admin": false}); c != 409 {
		t.Fatalf("dernier administrateur retiré : %d", c)
	}
	if c, _ := chef.faire("DELETE", "/api/admin/comptes/lea", nil); c != 200 {
		t.Fatalf("suppression du compte : %d", c)
	}
	if _, ok := reg.Lire("lea"); ok {
		t.Fatal("compte supprimé toujours là")
	}
}

// Plusieurs groupes sur un même serveur : le MJ ne voit que les joueurs que l'administrateur a admis
// dans son univers, les y joint ou non, et leur donne leur PJ.
func TestJoindreJoueurs(t *testing.T) {
	a, reg, connecter := serveurMulti(t)
	chef := connecter("chef")
	premier := a.st.ID()
	second := creerUnivers(t, chef, "Les Marches")
	mj1, mj2 := connecter("mj1"), connecter("mj2")
	donner(reg, "mj1", premier, comptes.MJ)
	donner(reg, "mj2", second, comptes.MJ)
	for _, nom := range []string{"lea", "tom", "zoe"} {
		connecter(nom)
	}
	chef.faire("PATCH", "/api/admin/comptes/lea", map[string]any{"admis": map[string]bool{premier: true}})
	chef.faire("PATCH", "/api/admin/comptes/tom", map[string]any{"admis": map[string]bool{second: true}})
	chef.faire("PATCH", "/api/admin/comptes/zoe", map[string]any{"admis": map[string]bool{premier: true}, "roles": map[string]string{premier: comptes.Auditeur}})
	noms := func(n *navigateur) string {
		t.Helper()
		var l []map[string]any
		if err := jsonDe(n.brut("GET", "/api/joueurs/comptes", nil), &l); err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, x := range l {
			out = append(out, x["nom"].(string)+map[bool]string{true: "+", false: ""}[x["joint"] == true])
		}
		return strings.Join(out, ",")
	}
	if n := noms(mj1); n != "lea,zoe" {
		t.Fatalf("joueurs admis dans le premier : %s", n)
	}
	if n := noms(mj2); n != "tom" {
		t.Fatalf("joueurs admis dans le second : %s", n)
	}
	// un compte admis ailleurs n'existe pas pour ce MJ
	if c, _ := mj2.faire("PATCH", "/api/joueurs/comptes/lea", map[string]any{"joint": true}); c != 404 {
		t.Fatalf("joueur d'un autre groupe : %d", c)
	}
	if c, _ := mj1.faire("PATCH", "/api/joueurs/comptes/inconnu", map[string]any{"joint": true}); c != 404 {
		t.Fatalf("compte inconnu : %d", c)
	}
	// un rôle donné par l'administrateur ne se change pas ici
	if c, _ := mj1.faire("PATCH", "/api/joueurs/comptes/zoe", map[string]any{"joint": true}); c != 409 {
		t.Fatalf("auditrice jointe comme joueuse : %d", c)
	}
	_, v := mj1.faire("POST", "/api/entites", map[string]any{"type": "pj", "nom": "Aldric"})
	pj := int64(v["id"].(float64))
	if c, _ := mj1.faire("PATCH", "/api/joueurs/comptes/lea", map[string]any{"pj": pj}); c != 409 {
		t.Fatalf("PJ donné à un compte pas encore joint : %d", c)
	}
	if c, v := mj1.faire("PATCH", "/api/joueurs/comptes/lea", map[string]any{"joint": true, "pj": pj}); c != 200 || v["joint"] != true || v["pj"] != float64(pj) {
		t.Fatalf("joueuse jointe : %d %v", c, v)
	}
	if n := noms(mj1); n != "lea+,zoe" {
		t.Fatalf("après la jonction : %s", n)
	}
	lea := reconnecter(t, a, "lea")
	if c, v := lea.faire("GET", "/api/joueur/etat", nil); c != 200 || v["pj"] == nil {
		t.Fatalf("page de la joueuse : %d %v", c, v)
	}
	if c, _ := lea.faire("GET", "/api/entites", nil); c != 403 {
		t.Fatalf("la joueuse hors de sa page : %d", c)
	}
	// le MJ ne touche à rien d'autre : ni à un autre univers, ni aux droits
	if c, _ := lea.faire("PATCH", "/api/joueurs/comptes/lea", map[string]any{"joint": true}); c != 403 {
		t.Fatalf("une joueuse qui se joint elle-même : %d", c)
	}
	if c, _ := reg.Lire("lea"); c.Role(second) != "" || c.Admin {
		t.Fatalf("compte après la jonction : %+v", c)
	}
	// le MJ la retire ; l'administrateur ne l'admet plus : elle n'y entre plus, son PJ s'en va
	mj1.faire("PATCH", "/api/joueurs/comptes/lea", map[string]any{"joint": false})
	if c, _ := reg.Lire("lea"); c.Role(premier) != "" || c.Persos[premier] != 0 || !c.PeutJouer(premier) {
		t.Fatalf("joueuse retirée par le MJ : %+v", c)
	}
	mj1.faire("PATCH", "/api/joueurs/comptes/lea", map[string]any{"joint": true})
	chef.faire("PATCH", "/api/admin/comptes/lea", map[string]any{"admis": map[string]bool{premier: false}})
	if c, _ := reg.Lire("lea"); c.Role(premier) != "" || c.PeutJouer(premier) {
		t.Fatalf("joueuse plus admise : %+v", c)
	}
	if n := noms(mj1); n != "zoe" {
		t.Fatalf("après le retrait de l'admission : %s", n)
	}
}

func jsonDe(w *httptest.ResponseRecorder, v any) error {
	return json.Unmarshal(w.Body.Bytes(), v)
}
