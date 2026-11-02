package serveur

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tchyze/cof_dm/internal/comptes"
	"github.com/Tchyze/cof_dm/internal/store"
)

const routeGestion = "/api/univers/gestion"

// creerFichier : un univers fermé, avec une fiche, là où le test le veut.
func creerFichier(t *testing.T, chemin string) string {
	t.Helper()
	st, err := store.Ouvrir(chemin)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Fermer()
	if _, err := st.Creer("lieu", "Le Gué", map[string]any{}, nil, "essai"); err != nil {
		t.Fatal(err)
	}
	return st.ID()
}

func (b *bancAppli) dossier() string { return b.a.DossierUnivers() }

// liste : les univers de la fenêtre, par nom.
func (b *bancAppli) liste() map[string]map[string]any {
	b.t.Helper()
	c, v := b.local("GET", routeGestion, nil)
	if c != 200 {
		b.t.Fatalf("liste : %d %v", c, v)
	}
	out := map[string]map[string]any{}
	for _, x := range v["univers"].([]any) {
		f := x.(map[string]any)
		out[f["nom"].(string)] = f
	}
	return out
}

func (b *bancAppli) cle(nom string) string {
	b.t.Helper()
	f, ok := b.liste()[nom]
	if !ok {
		b.t.Fatalf("« %s » n'est pas dans la liste : %v", nom, b.liste())
	}
	return f["cle"].(string)
}

func aAction(f map[string]any, action string) bool {
	for _, a := range f["actions"].([]any) {
		if a == action {
			return true
		}
	}
	return false
}

func lireTout(t *testing.T, chemin string) string {
	t.Helper()
	x, err := os.ReadFile(chemin)
	if err != nil {
		t.Fatal(err)
	}
	return string(x)
}

// La fenêtre liste l'univers ouvert, le dossier, les récents, les copies reliées et les archives.
func TestGestionListe(t *testing.T) {
	dir := t.TempDir()
	b := nouvelleAppli(t, dir)
	ailleurs := filepath.Join(dir, "ailleurs", "Clé USB.cofdm")
	creerFichier(t, ailleurs)
	disparu := filepath.Join(dir, "ailleurs", "Disparu.cofdm")
	creerFichier(t, filepath.Join(b.dossier(), "Essai.cofdm"))
	creerFichier(t, filepath.Join(b.a.DossierArchives(), "Vieux.cofdm"))
	if err := os.WriteFile(filepath.Join(b.dossier(), "Texte.cofdm"), []byte("ceci n'est pas un univers, et ce texte le montre"), 0o600); err != nil {
		t.Fatal(err)
	}
	copie := filepath.Join(b.dossier(), "Marches.cofdm")
	st, err := store.Ouvrir(copie)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.DefinirReplique(2); err != nil {
		t.Fatal(err)
	}
	idCopie := st.ID()
	st.Fermer()
	if err := b.a.ecrireLien(idCopie, &lien{URL: "https://jdr.example.org", Compte: "meneur", Copie: "Portable", Nom: "Les Marches", Chemin: copie}); err != nil {
		t.Fatal(err)
	}
	r, _ := json.Marshal([]string{ailleurs, disparu})
	if err := b.a.reg.SetMeta(regRecents, string(r)); err != nil {
		t.Fatal(err)
	}

	l := b.liste()
	attendu := map[string]string{"Mon univers": lieuDossier, "Essai": lieuDossier, "Texte": lieuDossier, "Marches": lieuDossier,
		"Clé USB": lieuAilleurs, "Disparu": lieuIntrouvable, "Vieux": lieuArchive}
	if len(l) != len(attendu) {
		t.Fatalf("liste : %v", l)
	}
	for nom, lieu := range attendu {
		if l[nom]["lieu"] != lieu {
			t.Errorf("%s : lieu %v, attendu %s", nom, l[nom]["lieu"], lieu)
		}
	}
	if l["Mon univers"]["ouvert"] != true || aAction(l["Mon univers"], "ouvrir") || !aAction(l["Mon univers"], "renommer") {
		t.Errorf("univers ouvert : %v", l["Mon univers"])
	}
	if !aAction(l["Essai"], "ouvrir") || !aAction(l["Essai"], "renommer") || aAction(l["Essai"], "oublier") {
		t.Errorf("univers du dossier : %v", l["Essai"])
	}
	if !aAction(l["Clé USB"], "oublier") || !aAction(l["Clé USB"], "ouvrir") {
		t.Errorf("fichier ailleurs : %v", l["Clé USB"])
	}
	if !aAction(l["Disparu"], "oublier") || aAction(l["Disparu"], "ouvrir") || aAction(l["Disparu"], "renommer") {
		t.Errorf("fichier disparu : %v", l["Disparu"])
	}
	if aAction(l["Vieux"], "ouvrir") {
		t.Errorf("un archivé ne s'ouvre pas : %v", l["Vieux"])
	}
	if l["Texte"]["gerable"] != false || len(l["Texte"]["actions"].([]any)) != 0 {
		t.Errorf("fichier qui n'est pas un univers : %v", l["Texte"])
	}
	c := l["Marches"]["copie"].(map[string]any)
	if c["url"] != "https://jdr.example.org" || c["nom"] != "Les Marches" || c["etat"] != "pause" {
		t.Errorf("copie reliée : %v", c)
	}
	if l["Essai"]["copie"] != nil {
		t.Errorf("univers local marqué copie : %v", l["Essai"])
	}
	if _, err := os.Stat(disparu); err == nil {
		t.Error("la liste a recréé le fichier disparu")
	}
}

// Une copie porte le même identifiant que l'original : la clé désigne un fichier, pas un identifiant.
func TestGestionClesParFichier(t *testing.T) {
	b := nouvelleAppli(t, t.TempDir())
	source := filepath.Join(b.dossier(), "Un.cofdm")
	creerFichier(t, source)
	if err := copierFichier(source, filepath.Join(b.dossier(), "Deux.cofdm")); err != nil {
		t.Fatal(err)
	}
	if b.cle("Un") == b.cle("Deux") {
		t.Fatal("deux fichiers, une seule clé")
	}
	if c, v := b.local("POST", routeGestion+"/"+b.cle("Deux")+"/renommer", map[string]any{"nom": "Trois"}); c != 200 {
		t.Fatalf("renommer : %d %v", c, v)
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatalf("l'original a bougé : %v", err)
	}
	if _, err := os.Stat(filepath.Join(b.dossier(), "Trois.cofdm")); err != nil {
		t.Fatal(err)
	}
}

// demande : une requête brute, pour les essais d'accès.
func (b *bancAppli) demande(methode, url, hote, distant string, entetes map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(methode, url, strings.NewReader(`{"nom":"Piraté"}`))
	r.Host, r.RemoteAddr = hote, distant
	for k, v := range entetes {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	b.a.ServeHTTP(w, r)
	return w
}

// Ces routes ne répondent qu'à l'ordinateur du MJ : ni tablette, ni session, ni jeton, d'où qu'il vienne.
func TestGestionAcces(t *testing.T) {
	b := nouvelleAppli(t, t.TempDir())
	cible := filepath.Join(b.dossier(), "Cible.cofdm")
	creerFichier(t, cible)
	cle := b.cle("Cible")
	jeton, _ := b.a.reg.Meta("api_jeton")
	if jeton == "" {
		jeton = "jeton-d-essai"
		if err := b.a.reg.SetMeta("api_jeton", jeton); err != nil {
			t.Fatal(err)
		}
	}
	const ici, mj, tablette = "127.0.0.1:8100", "127.0.0.1:5000", "192.168.1.50:5000"
	const reseau = "192.168.1.20:8100"
	essais := []struct {
		nom           string
		hote, distant string
		entetes       map[string]string
	}{
		{"tablette", reseau, tablette, map[string]string{"X-COF-DM": "1"}},
		{"tablette qui se dit locale", ici, tablette, map[string]string{"X-COF-DM": "1"}},
		{"session du réseau", reseau, tablette, map[string]string{"X-COF-DM": "1", "Cookie": "cofdm_session=une-session"}},
		{"jeton depuis le réseau", reseau, tablette, map[string]string{"Authorization": "Bearer " + jeton}},
		{"jeton depuis l'ordinateur", ici, mj, map[string]string{"Authorization": "Bearer " + jeton}},
		{"jeton et en-tête depuis l'ordinateur", ici, mj, map[string]string{"Authorization": "Bearer " + jeton, "X-COF-DM": "1"}},
		{"autre authentification", ici, mj, map[string]string{"Authorization": "Basic abc", "X-COF-DM": "1"}},
		{"nom d'hôte étranger sur cet ordinateur", "pirate.example.org:8100", mj, map[string]string{"X-COF-DM": "1"}},
		{"page d'un autre site", ici, mj, map[string]string{"X-COF-DM": "1", "Origin": "https://pirate.example.org"}},
		{"origine illisible", ici, mj, map[string]string{"X-COF-DM": "1", "Origin": "::"}},
	}
	routes := []struct{ methode, url string }{
		{"GET", routeGestion},
		{"POST", routeGestion + "/" + cle + "/renommer"},
		{"POST", routeGestion + "/" + cle + "/oublier"},
		{"POST", routeGestion + "/" + cle + "/bilan"},
		{"POST", routeGestion + "/" + cle + "/archiver"},
		{"POST", routeGestion + "/" + cle + "/desarchiver"},
		{"POST", routeGestion + "/" + cle + "/supprimer"},
	}
	for _, e := range essais {
		for _, rt := range routes {
			if w := b.demande(rt.methode, rt.url, e.hote, e.distant, e.entetes); w.Code != 401 && w.Code != 403 {
				t.Errorf("%s, %s %s : %d %s", e.nom, rt.methode, rt.url, w.Code, w.Body.String())
			}
		}
	}
	if w := b.demande("POST", routes[1].url, ici, mj, nil); w.Code != 403 {
		t.Errorf("écriture sans l'en-tête de l'application : %d", w.Code)
	}
	if w := b.demande("GET", routes[1].url, ici, mj, map[string]string{"X-COF-DM": "1"}); w.Code != 405 {
		t.Errorf("écriture derrière un GET : %d", w.Code)
	}
	if _, err := os.Stat(cible); err != nil {
		t.Fatalf("le fichier a bougé : %v", err)
	}
	// l'interface de l'application, elle, passe
	if w := b.demande("GET", routeGestion, ici, mj, map[string]string{"Origin": "http://127.0.0.1:8100"}); w.Code != 200 {
		t.Errorf("l'application elle-même : %d", w.Code)
	}
}

// Sur un serveur, ces routes n'existent pas et tout compte est refusé.
func TestGestionSurServeur(t *testing.T) {
	a, reg, connecter := serveurMulti(t)
	chef, meneur := connecter("chef"), connecter("meneur")
	id := a.st.ID()
	donner(reg, "meneur", id, comptes.MJ)
	chemin := a.st.Chemin()
	cle := cleFichier(chemin)
	for _, n := range []*navigateur{chef, meneur} {
		for _, rt := range []struct{ methode, url string }{
			{"GET", routeGestion},
			{"POST", routeGestion + "/" + cle + "/renommer"},
			{"POST", routeGestion + "/" + cle + "/oublier"},
			{"POST", routeGestion + "/" + cle + "/bilan"},
			{"POST", routeGestion + "/" + cle + "/archiver"},
			{"POST", routeGestion + "/" + cle + "/desarchiver"},
			{"POST", routeGestion + "/" + cle + "/supprimer"},
		} {
			if c, v := n.faire(rt.methode, rt.url, map[string]any{"nom": "Piraté"}, enTeteUnivers, id); c != 403 {
				t.Errorf("%s %s : %d %v", rt.methode, rt.url, c, v)
			}
		}
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", routeGestion, nil)
	a.mux.ServeHTTP(w, r)
	if w.Code != 404 {
		t.Errorf("route enregistrée sur un serveur : %d", w.Code)
	}
	if _, err := os.Stat(chemin); err != nil {
		t.Fatalf("le fichier a bougé : %v", err)
	}
}

// Renommer un univers fermé : fichier, place dans les récents, lien avec le serveur.
func TestRenommerFerme(t *testing.T) {
	b := nouvelleAppli(t, t.TempDir())
	ancien := filepath.Join(b.dossier(), "Ancien.cofdm")
	st, err := store.Ouvrir(ancien)
	if err != nil {
		t.Fatal(err)
	}
	id := st.ID()
	if err := st.DefinirReplique(2); err != nil {
		t.Fatal(err)
	}
	st.Fermer()
	if err := b.a.ecrireLien(id, &lien{URL: "https://jdr.example.org", Compte: "meneur", Copie: "Portable", Nom: "Sur le serveur", Chemin: ancien}); err != nil {
		t.Fatal(err)
	}
	premier := b.courant()["chemin"].(string)
	autre := filepath.Join(b.dossier(), "Autre.cofdm")
	r, _ := json.Marshal([]string{premier, ancien, autre})
	if err := b.a.reg.SetMeta(regRecents, string(r)); err != nil {
		t.Fatal(err)
	}
	contenu := lireTout(t, ancien)

	c, v := b.local("POST", routeGestion+"/"+b.cle("Ancien")+"/renommer", map[string]any{"nom": " Nouveau.cofdm "})
	if c != 200 {
		t.Fatalf("renommer : %d %v", c, v)
	}
	nouveau := filepath.Join(b.dossier(), "Nouveau.cofdm")
	if v["univers"].(map[string]any)["chemin"] != nouveau {
		t.Fatalf("réponse : %v", v)
	}
	if lireTout(t, nouveau) != contenu {
		t.Fatal("le contenu a changé : un univers fermé n'est pas ouvert pour le renommer")
	}
	if _, err := os.Stat(ancien); err == nil {
		t.Fatal("l'ancien fichier est resté")
	}
	if recents := b.a.recents(); len(recents) != 3 || recents[0] != premier || recents[1] != nouveau || recents[2] != autre {
		t.Fatalf("récents : %v", recents)
	}
	l := b.a.liens()[id]
	if l.Chemin != nouveau || l.Nom != "Sur le serveur" || l.URL != "https://jdr.example.org" {
		t.Fatalf("lien : %+v", l)
	}
	if b.a.parIdentifiant(id) != nouveau {
		t.Fatalf("la copie renommée paraît disparue : %q", b.a.parIdentifiant(id))
	}
	if b.courant()["chemin"] != premier {
		t.Fatalf("l'univers ouvert a changé : %v", b.courant())
	}
}

// Renommer l'univers ouvert : il reste ouvert, avec ses fiches, ses sauvegardes et sa place de dernier
// univers utilisé.
func TestRenommerOuvert(t *testing.T) {
	dir := t.TempDir()
	b := nouvelleAppli(t, dir)
	b.local("POST", "/api/entites", map[string]any{"type": "lieu", "nom": "Le Gué"})
	if c, v := b.local("POST", "/api/sauvegarde", nil); c != 200 {
		t.Fatalf("sauvegarde : %d %v", c, v)
	}
	id := b.courant()["id"]
	_, avant := b.local("GET", "/api/univers", nil)

	c, v := b.local("POST", routeGestion+"/"+b.cle("Mon univers")+"/renommer", map[string]any{"nom": "Les Marches"})
	if c != 200 {
		t.Fatalf("renommer : %d %v", c, v)
	}
	chemin := filepath.Join(b.dossier(), "Les Marches.cofdm")
	cour := b.courant()
	if cour["chemin"] != chemin || cour["id"] != id || b.fiches() != 1 {
		t.Fatalf("après le renommage : %v, %d fiches", cour, b.fiches())
	}
	if m, _ := b.a.reg.Meta(regUnivers); m != chemin {
		t.Fatalf("dernier univers : %s", m)
	}
	if _, err := os.Stat(filepath.Join(b.dossier(), "Mon univers.cofdm")); err == nil {
		t.Fatal("l'ancien fichier est resté")
	}
	_, apres := b.local("GET", "/api/univers", nil)
	if len(apres["sauvegardes"].([]any)) < len(avant["sauvegardes"].([]any)) || len(apres["sauvegardes"].([]any)) == 0 {
		t.Fatalf("sauvegardes perdues : %v puis %v", avant["sauvegardes"], apres["sauvegardes"])
	}
	for _, r := range b.a.recents() {
		if strings.Contains(r, "Mon univers") {
			t.Fatalf("récents : %v", b.a.recents())
		}
	}
	b.a.Fermer()
	b = nouvelleAppli(t, dir)
	if b.courant()["chemin"] != chemin {
		t.Fatalf("au lancement suivant : %v", b.courant())
	}
}

// Un nom déjà pris est refusé, sans rien écraser ; un nom impossible aussi.
func TestRenommerNomRefuse(t *testing.T) {
	b := nouvelleAppli(t, t.TempDir())
	cible := filepath.Join(b.dossier(), "Cible.cofdm")
	creerFichier(t, cible)
	pris := filepath.Join(b.dossier(), "Pris.cofdm")
	creerFichier(t, pris)
	creerFichier(t, filepath.Join(b.a.DossierArchives(), "Archivé.cofdm"))
	contenuPris, contenuCible := lireTout(t, pris), lireTout(t, cible)
	url := routeGestion + "/" + b.cle("Cible") + "/renommer"
	for nom, code := range map[string]int{"Pris": 409, "Pris.cofdm ": 409, "Archivé": 409, "": 422, "   ": 422, "...": 422, "///": 422, "NUL": 422, "com1.cofdm": 422, "Lpt9": 422} {
		if c, v := b.local("POST", url, map[string]any{"nom": nom}); c != code {
			t.Errorf("nom %q : %d %v, attendu %d", nom, c, v, code)
		}
	}
	if lireTout(t, pris) != contenuPris || lireTout(t, cible) != contenuCible {
		t.Fatal("un refus a écrit dans un fichier")
	}
	// un archivé ne prend pas non plus le nom d'un univers du dossier : il ne pourrait plus revenir
	if c, v := b.local("POST", routeGestion+"/"+b.cle("Archivé")+"/renommer", map[string]any{"nom": "Pris"}); c != 409 {
		t.Errorf("archivé renommé comme un univers du dossier : %d %v", c, v)
	}
	// le même nom : rien à faire
	if c, v := b.local("POST", url, map[string]any{"nom": "Cible"}); c != 200 {
		t.Errorf("même nom : %d %v", c, v)
	}
	noms := []string{}
	entrees, _ := os.ReadDir(b.dossier())
	for _, e := range entrees {
		noms = append(noms, e.Name())
	}
	if strings.Join(noms, "|") != "Cible.cofdm|Mon univers.cofdm|Pris.cofdm|archives" {
		t.Fatalf("dossier : %v", noms)
	}
}

// Une route de gestion ne vise qu'un univers connu, et n'atteint aucun autre fichier de l'ordinateur.
func TestGestionCheminsHostiles(t *testing.T) {
	dir := t.TempDir()
	b := nouvelleAppli(t, dir)
	dehors := filepath.Join(dir, "dehors")
	if err := os.MkdirAll(dehors, 0o700); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(dehors, "secret.txt")
	if err := os.WriteFile(secret, []byte("à ne pas toucher"), 0o600); err != nil {
		t.Fatal(err)
	}
	universDehors := filepath.Join(dehors, "Dehors.cofdm")
	creerFichier(t, universDehors)
	sousAutreNom := filepath.Join(dehors, "base.db")
	creerFichier(t, sousAutreNom)
	lienVersSecret := filepath.Join(b.dossier(), "Piège.cofdm")
	lienVersUnivers := filepath.Join(b.dossier(), "Raccourci.cofdm")
	if err := os.Symlink(secret, lienVersSecret); err != nil {
		t.Skip("pas de lien symbolique ici")
	}
	if err := os.Symlink(universDehors, lienVersUnivers); err != nil {
		t.Fatal(err)
	}
	// le dossier des archives ne suit pas un lien vers un autre dossier
	if err := os.Symlink(dehors, b.a.DossierArchives()); err != nil {
		t.Fatal(err)
	}
	r, _ := json.Marshal([]string{secret, sousAutreNom})
	if err := b.a.reg.SetMeta(regRecents, string(r)); err != nil {
		t.Fatal(err)
	}
	essai := func(url string, attendu ...int) {
		t.Helper()
		c, v := b.local("POST", url, map[string]any{"nom": "Piraté"})
		for _, a := range attendu {
			if c == a {
				return
			}
		}
		t.Errorf("%s : %d %v, attendu %v", url, c, v, attendu)
	}
	for _, cle := range []string{"inconnue", "..", "..%2f..%2fdehors%2fsecret.txt", "%2e%2e", cleFichier(universDehors) + "x", cleFichier(universDehors)} {
		// « .. » : le routeur redirige vers une route qui n'existe pas, sans rien exécuter
		essai(routeGestion+"/"+cle+"/renommer", 404, 400, 301, 307)
		essai(routeGestion+"/"+cle+"/oublier", 404, 400, 301, 307)
		essai(routeGestion+"/"+cle+"/bilan", 404, 400, 301, 307)
		essai(routeGestion+"/"+cle+"/archiver", 404, 400, 301, 307)
		essai(routeGestion+"/"+cle+"/desarchiver", 404, 400, 301, 307)
		essai(routeGestion+"/"+cle+"/supprimer", 404, 400, 301, 307)
	}
	// un chemin dans le corps ne vaut rien : la route ne connaît que la clé
	cible := filepath.Join(b.dossier(), "Vrai.cofdm")
	creerFichier(t, cible)
	if c, v := b.local("POST", routeGestion+"/"+b.cle("Vrai")+"/renommer", map[string]any{"nom": "X", "chemin": secret}); c != 422 {
		t.Errorf("chemin dans le corps : %d %v", c, v)
	}
	l := b.liste()
	if len(l) == 0 {
		t.Fatal("liste vide")
	}
	if _, listé := l["Dehors"]; listé {
		t.Error("le lien vers un dossier d'archives a fait entrer un fichier étranger dans la liste")
	}
	for _, nom := range []string{"Piège", "Raccourci", "secret", "base"} {
		f, ok := l[nom]
		if !ok {
			t.Errorf("« %s » devrait être listé : %v", nom, l)
			continue
		}
		if f["gerable"] != false || aAction(f, "renommer") || aAction(f, "archiver") || aAction(f, "supprimer") {
			t.Errorf("« %s » ne se gère pas : %v", nom, f)
		}
		cle := routeGestion + "/" + f["cle"].(string)
		essai(cle+"/renommer", 422)
		essai(cle+"/archiver", 422)
		essai(cle+"/desarchiver", 422)
		if c, v := b.local("POST", cle+"/supprimer", map[string]any{"nom": nom}); c != 422 {
			t.Errorf("supprimer « %s » : %d %v", nom, c, v)
		}
	}
	if lireTout(t, secret) != "à ne pas toucher" {
		t.Fatal("le fichier étranger a changé")
	}
	for _, c := range []string{secret, universDehors, sousAutreNom} {
		if _, err := os.Stat(c); err != nil {
			t.Fatalf("%s a bougé : %v", c, err)
		}
	}
	for _, c := range []string{lienVersSecret, lienVersUnivers} {
		if i, err := os.Lstat(c); err != nil || i.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("%s a changé : %v", c, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dehors, "Piraté.cofdm")); err == nil {
		t.Fatal("un renommage a atteint un autre dossier")
	}
	if _, err := os.Stat(filepath.Join(b.dossier(), "Piraté.cofdm")); err == nil {
		t.Fatal("un renommage a abouti à travers un lien")
	}
}

// requeteEnCours : une écriture de l'univers ouvert dont le corps n'arrive pas.
func (b *bancAppli) requeteEnCours() (liberer func() int) {
	b.t.Helper()
	lecteur, ecrivain := io.Pipe()
	r := httptest.NewRequest("POST", "/api/entites", lecteur)
	r.Host, r.RemoteAddr = "127.0.0.1:8100", "127.0.0.1:5000"
	r.Header.Set("X-COF-DM", "1")
	w := httptest.NewRecorder()
	fini := make(chan struct{})
	go func() {
		defer close(fini)
		b.a.ServeHTTP(w, r)
	}()
	if _, err := ecrivain.Write([]byte(`{"type":"lieu",`)); err != nil {
		b.t.Fatal(err)
	}
	libere := false
	liberer = func() int {
		if !libere {
			libere = true
			_, _ = ecrivain.Write([]byte(`"nom":"Fort Boueux"}`))
			_ = ecrivain.Close()
			<-fini
		}
		return w.Code
	}
	b.t.Cleanup(func() { liberer() })
	return liberer
}

// Des requêtes en cours qui durent : 409, et rien n'a changé ; une requête qui finit à temps : elle
// écrit, puis le renommage se fait.
func TestRenommerRequetesEnCours(t *testing.T) {
	b := nouvelleAppli(t, t.TempDir())
	chemin := b.courant()["chemin"].(string)
	cle := b.cle("Mon univers")
	url := routeGestion + "/" + cle + "/renommer"
	b.a.delaiFermeture = 50 * time.Millisecond
	liberer := b.requeteEnCours()
	attendreCompte(t, b.a.tous[b.a.st.ID()])
	if c, v := b.local("POST", url, map[string]any{"nom": "Trop tôt"}); c != 409 {
		t.Fatalf("requête en cours : %d %v", c, v)
	}
	if b.courant()["chemin"] != chemin {
		t.Fatalf("l'univers a changé : %v", b.courant())
	}
	if _, err := os.Stat(filepath.Join(b.dossier(), "Trop tôt.cofdm")); err == nil {
		t.Fatal("le fichier a été renommé malgré le refus")
	}
	if c := liberer(); c != http.StatusCreated && c != http.StatusOK {
		t.Fatalf("la requête en cours a échoué : %d", c)
	}

	b.a.delaiFermeture = 10 * time.Second
	liberer = b.requeteEnCours()
	id := b.a.st.ID()
	attendreCompte(t, b.a.tous[id])
	fini := make(chan int, 1)
	go func() {
		c, _ := b.local("POST", url, map[string]any{"nom": "À temps"})
		fini <- c
	}()
	attendreFermeture(t, b.a, id)
	if c := liberer(); c != http.StatusCreated && c != http.StatusOK {
		t.Fatalf("la requête attendue a échoué : %d", c)
	}
	if c := <-fini; c != 200 {
		t.Fatalf("renommage après la requête : %d", c)
	}
	if b.courant()["nom"] != "À temps" || b.fiches() != 2 {
		t.Fatalf("après : %v, %d fiches", b.courant(), b.fiches())
	}
}

// Un échange avec le serveur en cours se termine d'abord ; s'il dure : 409, et rien n'a changé.
func TestRenommerEchangeEnCours(t *testing.T) {
	b := nouvelleAppli(t, t.TempDir())
	chemin := b.courant()["chemin"].(string)
	b.a.delaiFermeture = 50 * time.Millisecond
	b.a.dist.mu.Lock()
	c, v := b.local("POST", routeGestion+"/"+b.cle("Mon univers")+"/renommer", map[string]any{"nom": "Pendant"})
	b.a.dist.mu.Unlock()
	if c != 409 {
		t.Fatalf("échange en cours : %d %v", c, v)
	}
	if b.courant()["chemin"] != chemin || b.fiches() != 0 {
		t.Fatalf("l'univers n'est plus servi comme avant : %v", b.courant())
	}
	if c, v := b.local("POST", routeGestion+"/"+b.cle("Mon univers")+"/renommer", map[string]any{"nom": "Après"}); c != 200 {
		t.Fatalf("échange fini : %d %v", c, v)
	}
}

// Un journal SQLite à côté d'un univers fermé porte des écritures interrompues : le fichier n'est pas
// séparé de son journal.
func TestRenommerAvecJournal(t *testing.T) {
	b := nouvelleAppli(t, t.TempDir())
	chemin := filepath.Join(b.dossier(), "Interrompu.cofdm")
	creerFichier(t, chemin)
	cle := b.cle("Interrompu")
	if err := os.WriteFile(chemin+"-journal", []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if c, v := b.local("POST", routeGestion+"/"+cle+"/renommer", map[string]any{"nom": "Autre"}); c != 409 {
		t.Fatalf("univers avec journal : %d %v", c, v)
	}
	if _, err := os.Stat(chemin); err != nil {
		t.Fatal(err)
	}
	// c'est en l'ouvrant que SQLite répare : l'action reste proposée
	f := b.liste()["Interrompu"]
	if f["interrompu"] != true || !aAction(f, "ouvrir") || aAction(f, "renommer") {
		t.Fatalf("fiche d'un univers interrompu : %v", f)
	}
}

// Oublier retire de la liste ce qui n'est que connu ; le fichier ne bouge jamais.
func TestOublier(t *testing.T) {
	dir := t.TempDir()
	b := nouvelleAppli(t, dir)
	ailleurs := filepath.Join(dir, "ailleurs", "Clé USB.cofdm")
	creerFichier(t, ailleurs)
	disparu := filepath.Join(dir, "ailleurs", "Disparu.cofdm")
	dossier := filepath.Join(b.dossier(), "Rangé.cofdm")
	creerFichier(t, dossier)
	relieDisparu := filepath.Join(dir, "ailleurs", "Copie perdue.cofdm")
	relieLa := filepath.Join(dir, "ailleurs", "Copie là.cofdm")
	st, err := store.Ouvrir(relieLa)
	if err != nil {
		t.Fatal(err)
	}
	idLa := st.ID()
	if err := st.DefinirReplique(2); err != nil {
		t.Fatal(err)
	}
	st.Fermer()
	if err := b.a.ecrireLien("perdu", &lien{URL: "https://jdr.example.org", Chemin: relieDisparu}); err != nil {
		t.Fatal(err)
	}
	if err := b.a.ecrireLien(idLa, &lien{URL: "https://jdr.example.org", Chemin: relieLa}); err != nil {
		t.Fatal(err)
	}
	r, _ := json.Marshal([]string{ailleurs, disparu})
	if err := b.a.reg.SetMeta(regRecents, string(r)); err != nil {
		t.Fatal(err)
	}
	contenu := lireTout(t, ailleurs)
	oublier := func(nom string) (int, map[string]any) {
		t.Helper()
		return b.local("POST", routeGestion+"/"+b.cle(nom)+"/oublier", nil)
	}
	if c, v := oublier("Mon univers"); c != 409 {
		t.Errorf("univers ouvert : %d %v", c, v)
	}
	if c, v := oublier("Rangé"); c != 409 {
		t.Errorf("univers du dossier : %d %v", c, v)
	}
	if c, v := oublier("Copie là"); c != 409 {
		t.Errorf("copie reliée présente : %d %v", c, v)
	}
	for _, nom := range []string{"Clé USB", "Disparu", "Copie perdue"} {
		if c, v := oublier(nom); c != 200 {
			t.Errorf("%s : %d %v", nom, c, v)
		}
	}
	l := b.liste()
	for _, nom := range []string{"Clé USB", "Disparu", "Copie perdue"} {
		if _, ok := l[nom]; ok {
			t.Errorf("« %s » est resté dans la liste", nom)
		}
	}
	if lireTout(t, ailleurs) != contenu {
		t.Fatal("oublier a touché au fichier")
	}
	if _, ok := b.a.liens()["perdu"]; ok {
		t.Error("le lien de la copie disparue est resté")
	}
	if _, ok := b.a.liens()[idLa]; !ok {
		t.Error("le lien d'une copie présente a été retiré")
	}
}

// Les récents et les liens d'une version d'avant se relisent sans perte : la liste n'y écrit rien.
func TestGestionListeNEcritRien(t *testing.T) {
	b := nouvelleAppli(t, t.TempDir())
	creerFichier(t, filepath.Join(b.dossier(), "Essai.cofdm"))
	avant := lireTout(t, filepath.Join(b.dir, "reglages.json"))
	fichier := lireTout(t, filepath.Join(b.dossier(), "Essai.cofdm"))
	b.liste()
	if !bytes.Equal([]byte(avant), []byte(lireTout(t, filepath.Join(b.dir, "reglages.json")))) {
		t.Fatal("la liste a écrit dans les réglages")
	}
	if fichier != lireTout(t, filepath.Join(b.dossier(), "Essai.cofdm")) {
		t.Fatal("la liste a écrit dans un univers")
	}
}
