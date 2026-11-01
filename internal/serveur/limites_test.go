package serveur

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tchyze/cof_dm/internal/comptes"
)

// bancLimites : un serveur et le MJ de son univers.
func bancLimites(t *testing.T, l *Limites) (*Application, *navigateur, string) {
	t.Helper()
	a, reg, connecter := serveurMulti(t)
	a.opt.Limites = l
	a.srv.opt.Limites = l
	mj := connecter("meneur")
	donner(reg, "meneur", a.st.ID(), comptes.MJ)
	return a, mj, a.st.ID()
}

func fichiersSauvegardes(a *Application, id string) (noms []string) {
	for _, s := range listerSauvegardes(a.dossierSauvegardes(a.tous[id].st)) {
		noms = append(noms, s.Fichier)
	}
	return noms
}

// Une seule place : le deuxième traitement lourd reçoit 429, et la place revient ensuite.
func TestTraitementsLourdsLimites(t *testing.T) {
	l := NouvellesLimites(1, 0, 0, 0)
	a, mj, id := bancLimites(t, l)
	libere, ok := l.prendre()
	if !ok {
		t.Fatal("la place doit être libre")
	}
	for _, x := range [][2]string{{"POST", "/api/sauvegarde"}, {"GET", "/api/export"}} {
		if c, _ := mj.faire(x[0], x[1], nil, enTeteUnivers, id); c != 429 {
			t.Fatalf("%s %s, place prise : %d", x[0], x[1], c)
		}
	}
	if c, _ := mj.faire("POST", "/api/import/univers", []byte("SQLite format 3\x00 rien"), enTeteUnivers, id); c != 429 {
		t.Fatalf("import, place prise : %d", c)
	}
	libere()
	if c, _ := mj.faire("POST", "/api/sauvegarde", nil, enTeteUnivers, id); c != 200 {
		t.Fatalf("sauvegarde, place libre : %d", c)
	}
	if len(a.tous) != 1 {
		t.Fatal("univers")
	}
}

// Un envoi lent n'occupe pas la place : elle n'est prise qu'une fois le fichier reçu.
func TestEnvoiLentNOccupePasLaPlace(t *testing.T) {
	l := NouvellesLimites(1, 0, 0, 0)
	_, mj, id := bancLimites(t, l)
	lecteur, ecrivain := io.Pipe()
	defer ecrivain.Close()
	r := httptest.NewRequest("POST", "/api/restaurer", lecteur)
	r.Host, r.RemoteAddr = "127.0.0.1:8100", "127.0.0.1:5000"
	r.Header.Set("X-COF-DM", "1")
	r.Header.Set(enTeteUnivers, id)
	for _, c := range mj.cookies {
		r.AddCookie(c)
	}
	fini := make(chan struct{})
	go func() {
		defer close(fini)
		mj.a.ServeHTTP(httptest.NewRecorder(), r)
	}()
	if _, err := ecrivain.Write([]byte(`{"format":`)); err != nil {
		t.Fatal(err)
	}
	if c, _ := mj.faire("POST", "/api/sauvegarde", nil, enTeteUnivers, id); c != 200 {
		t.Fatalf("sauvegarde pendant un envoi lent : %d", c)
	}
	ecrivain.Close()
	select {
	case <-fini:
	case <-time.After(5 * time.Second):
		t.Fatal("la requête doit finir")
	}
}

// La restauration d'un export reste possible, et un corps qui n'en est pas un est refusé.
func TestRestaurerExportReste(t *testing.T) {
	_, mj, id := bancLimites(t, NouvellesLimites(1, 0, 0, 0))
	mj.faire("POST", "/api/entites", map[string]any{"type": "lieu", "nom": "Fort Boueux"}, enTeteUnivers, id)
	w := mj.brut("GET", "/api/export", nil, enTeteUnivers, id)
	if w.Code != 200 {
		t.Fatalf("export : %d", w.Code)
	}
	var x map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &x)
	mj.faire("POST", "/api/entites", map[string]any{"type": "lieu", "nom": "En trop"}, enTeteUnivers, id)
	if c, v := mj.faire("POST", "/api/restaurer", x, enTeteUnivers, id); c != 200 || v["fiches"] != float64(1) {
		t.Fatalf("restauration : %d %v", c, v)
	}
	if c, _ := mj.faire("POST", "/api/restaurer", []byte("pas du json"), enTeteUnivers, id); c != 422 {
		t.Fatalf("corps illisible : %d", c)
	}
}

// Les sauvegardes demandées et celles d'avant une opération sont plafonnées ; les plus anciennes
// partent, jamais celle qui vient d'être faite. Celle d'avant une suppression ne part jamais.
func TestPurgeDesSauvegardes(t *testing.T) {
	dir := t.TempDir()
	poser := func(jour string, motif string, taille int) string {
		nom := "2026-10-" + jour + "_10-00-00_" + motif + ".cofdm"
		if err := os.WriteFile(filepath.Join(dir, nom), make([]byte, taille), 0o600); err != nil {
			t.Fatal(err)
		}
		return nom
	}
	reste := func(nom string) bool {
		_, err := os.Stat(filepath.Join(dir, nom))
		return err == nil
	}
	m1, m2, m3 := poser("01", "manuelle", 10), poser("02", "manuelle", 10), poser("03", "manuelle", 10)
	s1, s2 := poser("04", "avant-import", 10), poser("05", "avant-restauration", 10)
	s3 := poser("06", "avant-import", 10)
	sup := poser("07", "avant-suppression", 10)
	auto := poser("08", motifAuto, 10)
	l := NouvellesLimites(1, 0, 0, 2)
	if !l.ranger(dir, s3, false) {
		t.Fatal("rien ne limite le volume")
	}
	if reste(m1) || !reste(m2) || !reste(m3) || reste(s1) || !reste(s2) || !reste(s3) || !reste(sup) || !reste(auto) {
		t.Fatal("plafond par famille : les plus anciennes, et elles seules")
	}

	// le volume : les plus anciennes partent, sauf la dernière de chaque famille
	l = NouvellesLimites(1, 0, 45, 2)
	if !l.ranger(dir, m3, true) {
		t.Fatal("il y a de quoi faire de la place")
	}
	if reste(m2) || !reste(m3) || reste(s2) || !reste(s3) || !reste(sup) || !reste(auto) {
		t.Fatal("volume : les plus anciennes d'abord")
	}

	// plus de place sans toucher aux dernières : une sauvegarde demandée est refusée sans rien effacer
	l = NouvellesLimites(1, 0, 15, 2)
	m4 := poser("09", "manuelle", 10)
	if l.ranger(dir, m4, true) {
		t.Fatal("pas de place : refus attendu")
	}
	if !reste(m3) || !reste(s3) || !reste(sup) || !reste(auto) {
		t.Fatal("un refus n'efface rien")
	}
	_ = os.Remove(filepath.Join(dir, m4))
	// une sauvegarde de sécurité passe toujours : elle protège l'opération qui suit
	s4 := poser("10", "avant-import", 10)
	if l.ranger(dir, s4, false) {
		t.Fatal("le volume reste dépassé, et le dit")
	}
	if !reste(s4) {
		t.Fatal("jamais la sauvegarde qui vient d'être faite")
	}
}

// Au-delà du quota de sauvegardes, le MJ reçoit 507 pour une sauvegarde demandée ; l'import, lui,
// garde sa sauvegarde de sécurité et se fait.
func TestQuotaSauvegardesServeur(t *testing.T) {
	a, mj, id := bancLimites(t, NouvellesLimites(1, 0, 1, 3))
	mj.faire("POST", "/api/entites", map[string]any{"type": "lieu", "nom": "Fort Boueux"}, enTeteUnivers, id)
	c, v := mj.faire("POST", "/api/sauvegarde", nil, enTeteUnivers, id)
	if c != 507 || !strings.Contains(v["detail"].(string), "quota") {
		t.Fatalf("sauvegarde au-delà du quota : %d %v", c, v)
	}
	if n := len(fichiersSauvegardes(a, id)); n != 0 {
		t.Fatalf("la sauvegarde refusée ne reste pas : %d", n)
	}
	w := mj.brut("GET", "/api/export", nil, enTeteUnivers, id)
	var x map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &x)
	if c, v := mj.faire("POST", "/api/restaurer", x, enTeteUnivers, id); c != 200 {
		t.Fatalf("restauration au-delà du quota : %d %v", c, v)
	}
	l := fichiersSauvegardes(a, id)
	if len(l) != 1 || !strings.Contains(l[0], "avant-restauration") {
		t.Fatalf("sauvegarde de sécurité : %v", l)
	}
}

// Le bureau n'a ni quota ni purge : ses sauvegardes restent toutes.
func TestBureauSansLimites(t *testing.T) {
	b := nouvelleAppli(t, t.TempDir())
	if b.a.opt.Limites != nil {
		t.Fatal("pas de limites sur le bureau")
	}
	var l *Limites
	if libere, ok := l.prendre(); !ok {
		t.Fatal("sans limites, toujours une place")
	} else {
		libere()
	}
	if !l.ranger(t.TempDir(), "x", true) {
		t.Fatal("sans limites, rien à refuser")
	}
}

// Le quota d'un univers : ni image de plus, ni import trop gros.
func TestQuotaUnivers(t *testing.T) {
	a, mj, id := bancLimites(t, NouvellesLimites(1, 1, 0, 0))
	_, v := mj.faire("POST", "/api/entites", map[string]any{"type": "lieu", "nom": "Fort Boueux"}, enTeteUnivers, id)
	png := []byte("\x89PNG\r\n\x1a\n0000000000000000")
	u := "/api/entites/" + jsonNombre(v["id"]) + "/medias?nom=a.png"
	if c, v := mj.faire("POST", u, png, enTeteUnivers, id); c != 507 {
		t.Fatalf("image au-delà du quota : %d %v", c, v)
	}
	if c, _ := mj.faire("POST", "/api/import/univers?remplacer=1", []byte("SQLite format 3\x00 rien"), enTeteUnivers, id); c != 413 {
		t.Fatalf("import au-delà du quota : %d", c)
	}
	if n, _, _ := a.tous[id].st.Compter(); n["lieu"] != 1 {
		t.Fatal("rien n'a changé")
	}
}

func jsonNombre(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
