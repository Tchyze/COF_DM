package serveur

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/Tchyze/cof_dm/internal/reglages"
	"github.com/Tchyze/cof_dm/internal/store"
)

type bancAppli struct {
	t   *testing.T
	a   *Application
	dir string
}

func nouvelleAppli(t *testing.T, dir string) *bancAppli {
	t.Helper()
	a := NouvelleApplication(Options{Version: "test", DossierDon: dir, Port: 8100, Reseau: true,
		Frontend: fstest.MapFS{"index.html": {Data: []byte("<html>")}}})
	if err := a.Demarrer(""); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.Fermer)
	return &bancAppli{t, a, dir}
}

// appel depuis l'ordinateur du MJ, ou depuis le réseau avec le jeton d'API.
func (b *bancAppli) appel(reseau bool, methode, url string, corps any) (int, map[string]any) {
	b.t.Helper()
	j, _ := json.Marshal(corps)
	r := httptest.NewRequest(methode, url, bytes.NewReader(j))
	r.Host, r.RemoteAddr = "127.0.0.1:8100", "127.0.0.1:5000"
	r.Header.Set("X-COF-DM", "1")
	if reseau {
		jeton, _ := b.a.reg.Meta("api_jeton")
		r.Host, r.RemoteAddr = "192.168.1.20:8100", "192.168.1.50:5000"
		r.Header.Set("Authorization", "Bearer "+jeton)
	}
	w := httptest.NewRecorder()
	b.a.ServeHTTP(w, r)
	var v map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &v)
	if v == nil {
		v = map[string]any{"_brut": w.Body.String()}
	}
	return w.Code, v
}

func (b *bancAppli) local(methode, url string, corps any) (int, map[string]any) {
	b.t.Helper()
	return b.appel(false, methode, url, corps)
}

func (b *bancAppli) courant() map[string]any {
	b.t.Helper()
	c, v := b.local("GET", "/api/univers", nil)
	if c != 200 {
		b.t.Fatalf("état des univers : %d %v", c, v)
	}
	return v["courant"].(map[string]any)
}

func (b *bancAppli) fiches() int {
	b.t.Helper()
	_, v := b.local("GET", "/api/stats", nil)
	return int(v["total"].(float64))
}

// Fichier > Nouvel univers, Ouvrir, Enregistrer une copie : chaque univers est un fichier à part.
func TestMenuFichier(t *testing.T) {
	b := nouvelleAppli(t, t.TempDir())
	premier := b.courant()
	if premier["nom"] != "Mon univers" || !strings.HasSuffix(premier["chemin"].(string), filepath.Join("univers", "Mon univers.cofdm")) {
		t.Fatalf("univers par défaut : %v", premier)
	}
	if c, v := b.local("POST", "/api/entites", map[string]any{"type": "lieu", "nom": "Mont-Veille"}); c != 201 {
		t.Fatalf("création : %d %v", c, v)
	}

	// nouvel univers : vide, et devenu l'univers courant
	if c, v := b.local("POST", "/api/univers/nouveau", map[string]any{"nom": "Terres : du/Nord?"}); c != 200 {
		t.Fatalf("nouvel univers : %d %v", c, v)
	}
	second := b.courant()
	if second["nom"] != "Terres  duNord" || second["id"] == premier["id"] || b.fiches() != 0 {
		t.Fatalf("nouvel univers : %v, %d fiches", second, b.fiches())
	}
	if c, _ := b.local("POST", "/api/univers/nouveau", map[string]any{"nom": "Terres : du/Nord?"}); c != 409 {
		t.Fatalf("un univers existant ne doit pas être remplacé : %d", c)
	}
	_, etat := b.local("GET", "/api/univers", nil)
	if recents := etat["recents"].([]any); len(recents) != 1 || recents[0].(map[string]any)["chemin"] != premier["chemin"] {
		t.Fatalf("univers récents : %v", recents)
	}

	// retour au premier : ses fiches sont là
	if c, v := b.local("POST", "/api/univers/ouvrir", map[string]any{"chemin": premier["chemin"]}); c != 200 {
		t.Fatalf("ouverture : %d %v", c, v)
	}
	if b.courant()["id"] != premier["id"] || b.fiches() != 1 {
		t.Fatalf("premier univers rouvert : %v, %d fiches", b.courant(), b.fiches())
	}

	// un fichier absent ou qui n'est pas un univers est refusé, et l'univers courant reste ouvert
	if c, _ := b.local("POST", "/api/univers/ouvrir", map[string]any{"chemin": filepath.Join(b.dir, "absent.cofdm")}); c != 422 {
		t.Fatalf("fichier absent : %d", c)
	}
	faux := filepath.Join(b.dir, "faux.cofdm")
	os.WriteFile(faux, []byte("un simple texte, assez long pour ne pas passer pour une base vide"), 0o644)
	if c, v := b.local("POST", "/api/univers/ouvrir", map[string]any{"chemin": faux}); c != 422 || !strings.Contains(v["detail"].(string), "pas un univers") {
		t.Fatalf("faux univers : %d %v", c, v)
	}
	if b.courant()["id"] != premier["id"] || b.fiches() != 1 {
		t.Fatal("l'univers courant a été perdu après un refus")
	}

	// copie : un fichier complet, sans changer d'univers
	copie := filepath.Join(b.dir, "copie")
	if c, v := b.local("POST", "/api/univers/copie", map[string]any{"chemin": copie}); c != 200 || v["chemin"] != copie+".cofdm" {
		t.Fatalf("copie : %d %v", c, v)
	}
	if c, _ := b.local("POST", "/api/univers/copie", map[string]any{"chemin": copie}); c != 409 {
		t.Fatalf("copie sur un fichier existant : %d", c)
	}
	if c, _ := b.local("POST", "/api/univers/copie", map[string]any{"chemin": premier["chemin"], "remplacer": true}); c != 422 {
		t.Fatalf("copie sur l'univers ouvert : %d", c)
	}
	st, err := store.Ouvrir(copie + ".cofdm")
	if err != nil {
		t.Fatal(err)
	}
	if _, n, _ := st.Compter(); n != 1 || st.ID() != premier["id"] {
		t.Fatalf("contenu de la copie : %d fiches, id %s", n, st.ID())
	}
	st.Fermer()

	// rien de tout cela depuis le réseau, même avec le jeton d'API
	for _, x := range [][2]string{{"GET", "/api/univers"}, {"POST", "/api/univers/ouvrir"}, {"POST", "/api/univers/nouveau"}, {"POST", "/api/univers/copie"}, {"POST", "/api/univers/restaurer"}, {"POST", "/api/univers/parcourir"}} {
		if c, _ := b.appel(true, x[0], x[1], map[string]any{"chemin": faux, "nom": "pirate"}); c != 403 {
			t.Fatalf("%s %s depuis le réseau : %d", x[0], x[1], c)
		}
	}
	if c, _ := b.appel(true, "GET", "/api/stats", nil); c != 200 {
		t.Fatalf("l'API reste ouverte au jeton : %d", c)
	}
	// sans l'en-tête de l'interface, une page d'un autre site ne peut rien demander
	r := httptest.NewRequest("POST", "/api/univers/nouveau", strings.NewReader(`{"nom":"x"}`))
	r.Host, r.RemoteAddr = "127.0.0.1:8100", "127.0.0.1:5000"
	w := httptest.NewRecorder()
	b.a.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("écriture sans en-tête : %d", w.Code)
	}
}

// Au lancement suivant, le dernier univers est rouvert ; s'il a disparu, l'univers par défaut l'est
// à sa place et le MJ est prévenu.
func TestDernierUnivers(t *testing.T) {
	dir := t.TempDir()
	b := nouvelleAppli(t, dir)
	if c, v := b.local("POST", "/api/univers/nouveau", map[string]any{"chemin": filepath.Join(dir, "ailleurs", "Cendres")}); c != 200 {
		t.Fatalf("nouvel univers ailleurs : %d %v", c, v)
	}
	cendres := b.courant()
	b.a.Fermer()

	b = nouvelleAppli(t, dir)
	if b.courant()["id"] != cendres["id"] {
		t.Fatalf("dernier univers non rouvert : %v", b.courant())
	}
	b.a.Fermer()

	if err := os.Remove(cendres["chemin"].(string)); err != nil {
		t.Fatal(err)
	}
	b = nouvelleAppli(t, dir)
	_, etat := b.local("GET", "/api/univers", nil)
	if b.courant()["nom"] != "Mon univers" || !strings.Contains(etat["avertissement"].(string), "Cendres") {
		t.Fatalf("univers disparu : %v", etat)
	}
	recents := etat["recents"].([]any)
	if len(recents) != 1 || recents[0].(map[string]any)["present"] != false {
		t.Fatalf("un univers disparu reste dans les récents, marqué absent : %v", recents)
	}
}

// Les données d'avant les fichiers d'univers (cof-dm.db, dossier medias, réglages dans la base)
// deviennent un univers, sans rien effacer.
func TestRepriseAnciennesDonnees(t *testing.T) {
	dir := t.TempDir()
	ancienne := filepath.Join(dir, "cof-dm.db")
	st, err := store.Ouvrir(ancienne)
	if err != nil {
		t.Fatal(err)
	}
	e, err := st.Creer("pnj", "Gautier Roussel", nil, nil, "mj")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AjouterMedia(e.ID, "0123456789abcdef0123.png", "roussel.png", nil, "mj"); err != nil {
		t.Fatal(err)
	}
	st.SetMeta("api_jeton", "ancien-jeton")
	st.SetMeta("tracker_url", "http://10.0.0.5:8000")
	st.Fermer()
	os.MkdirAll(filepath.Join(dir, "medias"), 0o755)
	os.WriteFile(filepath.Join(dir, "medias", "0123456789abcdef0123.png"), []byte("\x89PNG\r\n\x1a\nportrait"), 0o644)

	b := nouvelleAppli(t, dir)
	if b.courant()["nom"] != "Mon univers" || b.fiches() != 1 {
		t.Fatalf("univers repris : %v, %d fiches", b.courant(), b.fiches())
	}
	r := httptest.NewRequest("GET", "/medias/0123456789abcdef0123.png", nil)
	r.Host, r.RemoteAddr = "127.0.0.1:8100", "127.0.0.1:5000"
	w := httptest.NewRecorder()
	b.a.ServeHTTP(w, r)
	if w.Code != 200 || !strings.HasSuffix(w.Body.String(), "portrait") || w.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("image reprise : %d %q %s", w.Code, w.Body.String(), w.Header().Get("Content-Type"))
	}
	// les réglages quittent l'univers pour l'application
	if j, _ := b.a.reg.Meta("api_jeton"); j != "ancien-jeton" {
		t.Fatalf("jeton d'API repris : %q", j)
	}
	// l'adresse de l'ancien tracker ne suit ni l'univers ni l'application
	if _, garde := b.a.reg.Meta("tracker_url"); garde {
		t.Fatal("adresse du tracker gardée dans les réglages")
	}
	if c, _ := b.local("GET", "/api/tracker", nil); c != 404 {
		t.Fatalf("route du tracker encore servie : %d", c)
	}
	b.a.mu.RLock()
	_, reste := b.a.st.Meta("api_jeton")
	b.a.mu.RUnlock()
	if reste {
		t.Fatal("le jeton d'API est resté dans le fichier d'univers")
	}
	// rien n'est effacé
	if _, err := os.Stat(ancienne + ".ancien"); err != nil {
		t.Fatalf("ancienne base : %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "medias", "0123456789abcdef0123.png")); err != nil {
		t.Fatalf("ancienne image : %v", err)
	}
	// et la reprise ne se refait pas
	b.a.Fermer()
	b = nouvelleAppli(t, dir)
	if b.fiches() != 1 {
		t.Fatalf("après relance : %d fiches", b.fiches())
	}
}

// Une sauvegarde automatique n'est faite que si l'univers a changé ; restaurer en remet une en place.
func TestSauvegardesEtRestauration(t *testing.T) {
	b := nouvelleAppli(t, t.TempDir())
	sauvegardes := func() []any {
		_, v := b.local("GET", "/api/univers", nil)
		return v["sauvegardes"].([]any)
	}
	if n := len(sauvegardes()); n != 0 {
		t.Fatalf("un univers vide n'a pas à être sauvegardé : %d", n)
	}
	b.a.mu.RLock()
	st, dossier := b.a.st, b.a.dossierSauvegardes(b.a.st)
	b.a.mu.RUnlock()

	if c, _ := b.local("POST", "/api/entites", map[string]any{"type": "lieu", "nom": "Le Gué"}); c != 201 {
		t.Fatal("création")
	}
	f, err := sauvegardeAuto(st, dossier)
	if err != nil || f == "" {
		t.Fatalf("sauvegarde d'un univers modifié : %q %v", f, err)
	}
	avecLeGue := filepath.Base(f)
	if f, err := sauvegardeAuto(st, dossier); err != nil || f != "" {
		t.Fatalf("sauvegarde d'un univers inchangé : %q %v", f, err)
	}

	if c, _ := b.local("POST", "/api/entites", map[string]any{"type": "lieu", "nom": "Le Pont"}); c != 201 || b.fiches() != 2 {
		t.Fatal("seconde création")
	}
	time.Sleep(1100 * time.Millisecond)
	if c, v := b.local("POST", "/api/univers/restaurer", map[string]any{"fichier": avecLeGue}); c != 200 {
		t.Fatalf("restauration : %d %v", c, v)
	}
	if b.fiches() != 1 {
		t.Fatalf("après restauration : %d fiches", b.fiches())
	}
	avant := ""
	for _, s := range sauvegardes() {
		if m := s.(map[string]any); m["motif"] == "avant-restauration" {
			avant = m["fichier"].(string)
		}
	}
	if avant == "" {
		t.Fatalf("pas de sauvegarde d'avant la restauration : %v", sauvegardes())
	}
	// l'état d'avant la restauration se retrouve de la même façon
	time.Sleep(1100 * time.Millisecond)
	if c, v := b.local("POST", "/api/univers/restaurer", map[string]any{"fichier": avant}); c != 200 || b.fiches() != 2 {
		t.Fatalf("retour à l'état d'avant : %d %v, %d fiches", c, v, b.fiches())
	}
	for _, mauvais := range []string{"../../reglages.json", "2020-01-01_00-00-00_auto.cofdm", ""} {
		if c, _ := b.local("POST", "/api/univers/restaurer", map[string]any{"fichier": mauvais}); c != 422 && c != 404 {
			t.Fatalf("restauration de %q : %d", mauvais, c)
		}
	}
	if b.fiches() != 2 {
		t.Fatal("univers abîmé par une restauration refusée")
	}
}

// Parmi les sauvegardes automatiques, restent les plus récentes puis la dernière de chaque jour ;
// celles que le MJ a demandées ne sont jamais effacées.
func TestRotationDesSauvegardes(t *testing.T) {
	dir := t.TempDir()
	maintenant := time.Date(2026, 10, 12, 18, 0, 0, 0, time.Local)
	poser := func(quand time.Time, motif string) string {
		nom := quand.Format(formatSauvegarde) + "_" + motif + store.Extension
		os.WriteFile(filepath.Join(dir, nom), []byte("x"), 0o644)
		return nom
	}
	var aujourdhui []string
	for h := 0; h < 8; h++ { // huit sauvegardes aujourd'hui, de 10 h à 17 h
		aujourdhui = append(aujourdhui, poser(maintenant.Add(time.Duration(h-8)*time.Hour), motifAuto))
	}
	hierMatin := poser(maintenant.AddDate(0, 0, -1).Add(-9*time.Hour), motifAuto)
	hierSoir := poser(maintenant.AddDate(0, 0, -1), motifAuto)
	ancienne := poser(maintenant.AddDate(0, 0, -40), motifAuto)
	manuelle := poser(maintenant.AddDate(0, 0, -200), "manuelle")
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o644)

	purgerSauvegardes(dir, maintenant)
	reste := map[string]bool{}
	for _, s := range listerSauvegardes(dir) {
		reste[s.Fichier] = true
	}
	attendu := append([]string{hierSoir, manuelle}, aujourdhui[3:]...)
	if len(reste) != len(attendu) {
		t.Fatalf("sauvegardes gardées : %v", reste)
	}
	for _, f := range attendu {
		if !reste[f] {
			t.Fatalf("%s aurait dû être gardée : %v", f, reste)
		}
	}
	for _, f := range []string{hierMatin, ancienne, aujourdhui[0]} {
		if reste[f] {
			t.Fatalf("%s aurait dû être effacée", f)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "notes.txt")); err != nil {
		t.Fatal("un fichier étranger aux sauvegardes a été effacé")
	}
}

// Le mot de passe MJ et le jeton tiennent à l'ordinateur : ils ne changent pas avec l'univers.
func TestReglagesHorsUnivers(t *testing.T) {
	dir := t.TempDir()
	b := nouvelleAppli(t, dir)
	jeton, _ := b.a.reg.Meta("api_jeton")
	if c, v := b.local("POST", "/api/acces/mdp", map[string]any{"nouveau": "sésame-ouvre"}); c != 200 {
		t.Fatalf("mot de passe : %d %v", c, v)
	}
	if c, _ := b.local("POST", "/api/univers/nouveau", map[string]any{"nom": "Autre"}); c != 200 {
		t.Fatal("nouvel univers")
	}
	if j, _ := b.a.reg.Meta("api_jeton"); j != jeton || jeton == "" {
		t.Fatalf("jeton d'API changé avec l'univers : %q puis %q", jeton, j)
	}
	if _, v := b.local("GET", "/api/acces", nil); v["mdp_defini"] != true {
		t.Fatalf("mot de passe perdu en changeant d'univers : %v", v)
	}
	if _, ok := reglages.Ouvrir(filepath.Join(dir, "reglages.json")).Meta("mj_mdp"); !ok {
		t.Fatal("mot de passe absent des réglages de l'application")
	}
	c, v := b.local("GET", "/api/parametres", nil)
	if u, _ := v["univers"].(map[string]any); c != 200 || u["nom"] != "Autre" || u["chemin"] == nil {
		t.Fatalf("univers dans les paramètres : %d %v", c, v)
	}
}
