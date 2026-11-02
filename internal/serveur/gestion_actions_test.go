package serveur

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tchyze/cof_dm/internal/store"
)

func (b *bancAppli) action(nom, action string, corps any) (int, map[string]any) {
	b.t.Helper()
	return b.local("POST", routeGestion+"/"+b.cle(nom)+"/"+action, corps)
}

func entierDe(v any) int {
	f, _ := v.(float64)
	return int(f)
}

func plusLa(t *testing.T, chemin string) {
	t.Helper()
	if _, err := os.Lstat(chemin); err == nil {
		t.Fatalf("%s existe encore", chemin)
	}
}

// fichesDe : le nombre de fiches d'un fichier d'univers, pour relire une sauvegarde.
func fichesDe(t *testing.T, chemin string) int {
	t.Helper()
	copie := filepath.Join(t.TempDir(), "relue.cofdm")
	if err := copierFichier(chemin, copie); err != nil {
		t.Fatal(err)
	}
	st, err := store.Ouvrir(copie)
	if err != nil {
		t.Fatalf("%s ne se relit pas : %v", chemin, err)
	}
	defer st.Fermer()
	_, n, err := st.Compter()
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// Le nom retapé confirme ; une sauvegarde relue est faite avant ; la réponse dit où elle est.
func TestSupprimerFerme(t *testing.T) {
	b := nouvelleAppli(t, t.TempDir())
	chemin := filepath.Join(b.dossier(), "Vieux.cofdm")
	id := creerFichier(t, chemin)
	r, _ := json.Marshal([]string{chemin})
	if err := b.a.reg.SetMeta(regRecents, string(r)); err != nil {
		t.Fatal(err)
	}
	for _, nom := range []string{"", "vieux", "Mon univers", "Vieux.cofdm"} {
		if c, v := b.action("Vieux", "supprimer", map[string]any{"nom": nom}); c != 422 {
			t.Fatalf("confirmation %q : %d %v", nom, c, v)
		}
	}
	if _, err := os.Stat(chemin); err != nil {
		t.Fatal("une confirmation fausse a supprimé le fichier")
	}
	c, v := b.action("Vieux", "supprimer", map[string]any{"nom": " Vieux "})
	if c != 200 || v["supprime"] != true || v["verifiee"] != true {
		t.Fatalf("suppression : %d %v", c, v)
	}
	plusLa(t, chemin)
	sauvegarde := v["sauvegarde"].(string)
	if filepath.Dir(sauvegarde) != filepath.Join(b.dir, "sauvegardes", id) || v["dossier_sauvegardes"] != filepath.Dir(sauvegarde) {
		t.Fatalf("sauvegarde : %v", v)
	}
	if fichesDe(t, sauvegarde) != 1 {
		t.Fatal("la sauvegarde n'a pas la fiche")
	}
	if len(b.a.recents()) != 0 {
		t.Fatalf("récents : %v", b.a.recents())
	}
	if b.courant()["nom"] != "Mon univers" {
		t.Fatalf("l'univers ouvert a changé : %v", b.courant())
	}
}

// Une sauvegarde impossible : rien n'est supprimé, et l'univers ouvert reste servi.
func TestSupprimerSauvegardeEnEchec(t *testing.T) {
	b := nouvelleAppli(t, t.TempDir())
	ferme := filepath.Join(b.dossier(), "Vieux.cofdm")
	id := creerFichier(t, ferme)
	obstacle := filepath.Join(b.dir, "sauvegardes", id)
	if err := os.MkdirAll(filepath.Dir(obstacle), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(obstacle, []byte("un fichier à la place du dossier"), 0o600); err != nil {
		t.Fatal(err)
	}
	if c, v := b.action("Vieux", "supprimer", map[string]any{"nom": "Vieux"}); c != 500 || !strings.Contains(v["detail"].(string), "rien n'a été supprimé") {
		t.Fatalf("univers fermé : %d %v", c, v)
	}
	if _, err := os.Stat(ferme); err != nil {
		t.Fatal(err)
	}

	cour := b.courant()
	obstacle = filepath.Join(b.dir, "sauvegardes", cour["id"].(string))
	if err := os.RemoveAll(obstacle); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(obstacle, []byte("un fichier à la place du dossier"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(b.dir, "sauvegardes", id)); err != nil {
		t.Fatal(err)
	}
	c, v := b.action("Mon univers", "supprimer", map[string]any{"nom": "Mon univers", "suivant": b.cle("Vieux")})
	if c != 500 || !strings.Contains(v["detail"].(string), "rien n'a été supprimé") {
		t.Fatalf("univers ouvert : %d %v", c, v)
	}
	if b.courant()["chemin"] != cour["chemin"] || b.fiches() != 0 {
		t.Fatalf("l'univers ouvert n'est plus servi : %v", b.courant())
	}
}

// L'univers ouvert ne se supprime pas sans dire ce qui s'ouvre ensuite ; si le suivant ne s'ouvre pas,
// rien ne change.
func TestSupprimerOuvert(t *testing.T) {
	b := nouvelleAppli(t, t.TempDir())
	b.local("POST", "/api/entites", map[string]any{"type": "lieu", "nom": "Le Gué"})
	chemin := b.courant()["chemin"].(string)
	suivant := filepath.Join(b.dossier(), "Suivant.cofdm")
	creerFichier(t, suivant)
	piege := filepath.Join(b.dossier(), "Piège.cofdm")
	st, err := store.Ouvrir(piege)
	if err != nil {
		t.Fatal(err)
	}
	st.Fermer()
	db, err := sql.Open("sqlite3", piege)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TRIGGER piege AFTER INSERT ON meta BEGIN SELECT 1; END`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if err := os.WriteFile(filepath.Join(b.dossier(), "Texte.cofdm"), []byte("ceci n'est pas un univers, et ce texte le montre"), 0o600); err != nil {
		t.Fatal(err)
	}
	corps := func(s string) map[string]any { return map[string]any{"nom": "Mon univers", "suivant": s} }
	if c, v := b.action("Mon univers", "supprimer", map[string]any{"nom": "Mon univers"}); c != 422 {
		t.Fatalf("sans suivant : %d %v", c, v)
	}
	if c, v := b.action("Mon univers", "supprimer", corps("inconnue")); c != 422 {
		t.Fatalf("suivant inconnu : %d %v", c, v)
	}
	if c, v := b.action("Mon univers", "supprimer", corps(b.cle("Mon univers"))); c != 422 {
		t.Fatalf("suivant : lui-même : %d %v", c, v)
	}
	if c, v := b.action("Mon univers", "supprimer", corps(b.cle("Texte"))); c != 422 {
		t.Fatalf("suivant qui n'est pas un univers : %d %v", c, v)
	}
	if c, v := b.action("Mon univers", "supprimer", corps(b.cle("Piège"))); c != 422 {
		t.Fatalf("suivant qui ne s'ouvre pas : %d %v", c, v)
	}
	if b.courant()["chemin"] != chemin || b.fiches() != 1 {
		t.Fatalf("un refus a changé l'univers ouvert : %v", b.courant())
	}
	c, v := b.action("Mon univers", "supprimer", corps(b.cle("Suivant")))
	if c != 200 || v["ouvert"] != suivant || v["aucun_univers"] != false {
		t.Fatalf("suppression : %d %v", c, v)
	}
	plusLa(t, chemin)
	if b.courant()["chemin"] != suivant || b.fiches() != 1 {
		t.Fatalf("univers suivant : %v", b.courant())
	}
	if fichesDe(t, v["sauvegarde"].(string)) != 1 {
		t.Fatal("la sauvegarde n'a pas la fiche de l'univers supprimé")
	}
}

// Le dernier univers se supprime : l'application garde son interface et propose d'en créer un, y compris
// au lancement suivant. Seule une première installation crée un univers d'office.
func TestSupprimerDernier(t *testing.T) {
	dir := t.TempDir()
	b := nouvelleAppli(t, dir)
	chemin := b.courant()["chemin"].(string)
	c, v := b.action("Mon univers", "supprimer", map[string]any{"nom": "Mon univers"})
	if c != 200 || v["aucun_univers"] != true || v["ouvert"] != "" {
		t.Fatalf("dernier univers : %d %v", c, v)
	}
	plusLa(t, chemin)
	verifier := func(b *bancAppli) {
		t.Helper()
		if c, v := b.local("GET", "/api/univers", nil); c != 200 || v["courant"] != nil {
			t.Fatalf("état : %d %v", c, v)
		}
		if c, v := b.local("GET", "/api/stats", nil); c != 503 || v["aucun_univers"] != true {
			t.Fatalf("route d'un univers : %d %v", c, v)
		}
		if c, v := b.local("GET", routeGestion, nil); c != 200 || len(v["univers"].([]any)) != 0 {
			t.Fatalf("liste : %d %v", c, v)
		}
		if c, _ := b.local("GET", "/", nil); c != 200 {
			t.Fatalf("interface : %d", c)
		}
		// le réseau n'a rien de plus, et l'administration reste absente
		r := httptest.NewRequest("GET", "/", nil)
		r.Host, r.RemoteAddr = "192.168.1.20:8100", "192.168.1.50:5000"
		w := httptest.NewRecorder()
		b.a.ServeHTTP(w, r)
		if w.Code != 503 {
			t.Fatalf("depuis le réseau : %d %s", w.Code, w.Body.String())
		}
		if c, _ := b.local("GET", "/admin.html", nil); c != 404 {
			t.Fatalf("page d'administration : %d", c)
		}
		if c, _ := b.local("GET", "/js/", nil); c != 404 {
			t.Fatalf("liste d'un dossier : %d", c)
		}
	}
	verifier(b)
	b.a.Fermer()

	b = nouvelleAppli(t, dir)
	verifier(b)
	plusLa(t, chemin)
	if c, v := b.local("POST", "/api/univers/nouveau", map[string]any{"nom": "Le Retour"}); c != 200 {
		t.Fatalf("création : %d %v", c, v)
	}
	if b.courant()["nom"] != "Le Retour" || b.fiches() != 0 {
		t.Fatalf("après la création : %v", b.courant())
	}
	b.a.Fermer()

	b = nouvelleAppli(t, dir)
	if b.courant()["nom"] != "Le Retour" {
		t.Fatalf("relance : %v", b.courant())
	}
}

// Requêtes en cours qui durent : 409, rien ne change.
func TestSupprimerRequetesEnCours(t *testing.T) {
	b := nouvelleAppli(t, t.TempDir())
	chemin := b.courant()["chemin"].(string)
	suivant := filepath.Join(b.dossier(), "Suivant.cofdm")
	creerFichier(t, suivant)
	corps := map[string]any{"nom": "Mon univers", "suivant": b.cle("Suivant")}
	cle := b.cle("Mon univers")
	b.a.delaiFermeture = 50 * time.Millisecond
	liberer := b.requeteEnCours()
	attendreCompte(t, b.a.tous[b.a.st.ID()])
	if c, v := b.local("POST", routeGestion+"/"+cle+"/supprimer", corps); c != 409 {
		t.Fatalf("suppression, requête en cours : %d %v", c, v)
	}
	if c, v := b.local("POST", routeGestion+"/"+cle+"/archiver", map[string]any{"suivant": b.cle("Suivant")}); c != 409 {
		t.Fatalf("archivage, requête en cours : %d %v", c, v)
	}
	if _, err := os.Stat(chemin); err != nil {
		t.Fatal(err)
	}
	if liberer(); b.courant()["chemin"] != chemin || b.fiches() != 1 {
		t.Fatalf("l'univers n'est plus servi : %v, %d fiches", b.courant(), b.fiches())
	}
}

// Archiver et désarchiver : un seul fichier, qui revient tel quel.
func TestArchiverFerme(t *testing.T) {
	dir := t.TempDir()
	b := nouvelleAppli(t, dir)
	chemin := filepath.Join(b.dossier(), "Vieux.cofdm")
	creerFichier(t, chemin)
	r, _ := json.Marshal([]string{chemin})
	if err := b.a.reg.SetMeta(regRecents, string(r)); err != nil {
		t.Fatal(err)
	}
	contenu := lireTout(t, chemin)
	if c, v := b.action("Vieux", "archiver", map[string]any{}); c != 200 {
		t.Fatalf("archiver : %d %v", c, v)
	}
	archive := filepath.Join(b.a.DossierArchives(), "Vieux.cofdm")
	plusLa(t, chemin)
	if lireTout(t, archive) != contenu {
		t.Fatal("le fichier archivé a changé")
	}
	if entrees, _ := os.ReadDir(b.a.DossierArchives()); len(entrees) != 1 {
		t.Fatalf("archives : %v", entrees)
	}
	for _, x := range b.a.recents() {
		if strings.Contains(x, "Vieux") {
			t.Fatalf("récents : %v", b.a.recents())
		}
	}
	f := b.liste()["Vieux"]
	if f["lieu"] != lieuArchive || aAction(f, "ouvrir") || !aAction(f, "desarchiver") || aAction(f, "archiver") {
		t.Fatalf("fiche archivée : %v", f)
	}
	// le menu et la boîte du système ne contournent pas l'archivage
	if c, v := b.local("POST", "/api/univers/ouvrir", map[string]any{"chemin": archive}); c != 422 {
		t.Fatalf("ouvrir un archivé : %d %v", c, v)
	}
	if c, v := b.local("POST", "/api/univers/nouveau", map[string]any{"chemin": filepath.Join(b.a.DossierArchives(), "Neuf")}); c != 422 {
		t.Fatalf("créer dans les archives : %d %v", c, v)
	}

	// un autre univers a pris le nom : le désarchivage attend
	creerFichier(t, chemin)
	if c, v := b.action("Vieux", "desarchiver", map[string]any{}); c != 409 {
		t.Fatalf("nom pris : %d %v", c, v)
	}
	if lireTout(t, archive) != contenu {
		t.Fatal("le refus a touché au fichier")
	}
	if err := os.Remove(chemin); err != nil {
		t.Fatal(err)
	}
	if c, v := b.action("Vieux", "desarchiver", map[string]any{}); c != 200 {
		t.Fatalf("désarchiver : %d %v", c, v)
	}
	plusLa(t, archive)
	if lireTout(t, chemin) != contenu {
		t.Fatal("le fichier ne revient pas tel quel")
	}
	if b.liste()["Vieux"]["lieu"] != lieuDossier {
		t.Fatalf("après : %v", b.liste()["Vieux"])
	}
	// un nom déjà pris dans les archives : refusé, rien n'est écrasé
	if err := copierFichier(chemin, archive); err != nil {
		t.Fatal(err)
	}
	if c, v := b.local("POST", routeGestion+"/"+cleFichier(chemin)+"/archiver", map[string]any{}); c != 409 {
		t.Fatalf("nom pris aux archives : %d %v", c, v)
	}
}

// Archiver l'univers ouvert en ouvre un autre ; un fichier hors du dossier ne s'archive pas.
func TestArchiverOuvertEtAilleurs(t *testing.T) {
	dir := t.TempDir()
	b := nouvelleAppli(t, dir)
	b.local("POST", "/api/entites", map[string]any{"type": "lieu", "nom": "Le Gué"})
	chemin := b.courant()["chemin"].(string)
	suivant := filepath.Join(b.dossier(), "Suivant.cofdm")
	creerFichier(t, suivant)
	ailleurs := filepath.Join(dir, "ailleurs", "Clé USB.cofdm")
	creerFichier(t, ailleurs)
	r, _ := json.Marshal([]string{ailleurs})
	if err := b.a.reg.SetMeta(regRecents, string(r)); err != nil {
		t.Fatal(err)
	}
	f := b.liste()["Clé USB"]
	if aAction(f, "archiver") || !aAction(f, "supprimer") {
		t.Fatalf("fichier ailleurs : %v", f)
	}
	if c, v := b.action("Clé USB", "archiver", map[string]any{}); c != 422 {
		t.Fatalf("archiver ailleurs : %d %v", c, v)
	}
	if c, v := b.action("Mon univers", "archiver", map[string]any{}); c != 422 {
		t.Fatalf("archiver sans suivant : %d %v", c, v)
	}
	c, v := b.action("Mon univers", "archiver", map[string]any{"suivant": b.cle("Suivant")})
	if c != 200 || v["ouvert"] != suivant {
		t.Fatalf("archiver l'ouvert : %d %v", c, v)
	}
	plusLa(t, chemin)
	if b.courant()["chemin"] != suivant {
		t.Fatalf("suivant : %v", b.courant())
	}
	// il revient avec sa fiche
	if c, v := b.action("Mon univers", "desarchiver", map[string]any{}); c != 200 {
		t.Fatalf("désarchiver : %d %v", c, v)
	}
	if fichesDe(t, chemin) != 1 {
		t.Fatal("la fiche n'est pas revenue")
	}
	// un fichier d'ailleurs se supprime, avec sa sauvegarde
	if c, v := b.action("Clé USB", "supprimer", map[string]any{"nom": "Clé USB"}); c != 200 || fichesDe(t, v["sauvegarde"].(string)) != 1 {
		t.Fatalf("supprimer ailleurs : %d %v", c, v)
	}
	plusLa(t, ailleurs)
	// le dernier univers s'archive aussi : il reste à désarchiver depuis l'écran d'accueil
	b.action("Mon univers", "supprimer", map[string]any{"nom": "Mon univers"})
	c, v = b.action("Suivant", "archiver", map[string]any{})
	if c != 200 || v["aucun_univers"] != true {
		t.Fatalf("archiver le dernier : %d %v", c, v)
	}
	if f := b.liste()["Suivant"]; !aAction(f, "desarchiver") {
		t.Fatalf("écran d'accueil : %v", f)
	}
}

// Un combat lancé ici et pas terminé ne voyage pas : il se perd à la suppression, sauf confirmation.
func TestCombatEnCoursAvertit(t *testing.T) {
	b := nouvelleAppli(t, t.TempDir())
	suivant := filepath.Join(b.dossier(), "Suivant.cofdm")
	creerFichier(t, suivant)
	if c, v := b.local("POST", "/api/combats", map[string]any{"nom": "Au gué"}); c != 201 {
		t.Fatalf("combat : %d %v", c, v)
	}
	c, v := b.action("Mon univers", "bilan", nil)
	if c != 200 || entierDe(v["bilan"].(map[string]any)["combats"]) != 1 {
		t.Fatalf("bilan de l'univers ouvert : %d %v", c, v)
	}
	if s := v["suivants"].([]any); len(s) != 1 || s[0].(map[string]any)["nom"] != "Suivant" {
		t.Fatalf("suivants : %v", v["suivants"])
	}
	vu := v["bilan"]
	suiv := b.cle("Suivant")
	corps := map[string]any{"nom": "Mon univers", "suivant": suiv}
	c, v = b.action("Mon univers", "supprimer", corps)
	if c != 409 || entierDe(v["bilan"].(map[string]any)["combats"]) != 1 {
		t.Fatalf("suppression sans avertissement : %d %v", c, v)
	}
	if c, v = b.action("Mon univers", "archiver", map[string]any{"suivant": suiv}); c != 409 {
		t.Fatalf("archivage sans avertissement : %d %v", c, v)
	}
	if b.courant()["nom"] != "Mon univers" {
		t.Fatalf("un refus a changé l'univers : %v", b.courant())
	}
	// fermé aussi : le bilan lit une copie du fichier, qui n'est pas touché
	if c, v = b.action("Mon univers", "archiver", map[string]any{"suivant": suiv, "bilan_vu": vu}); c != 200 {
		t.Fatalf("archivage confirmé : %d %v", c, v)
	}
	archive := filepath.Join(b.a.DossierArchives(), "Mon univers.cofdm")
	contenu := lireTout(t, archive)
	c, v = b.action("Mon univers", "bilan", nil)
	if c != 200 || entierDe(v["bilan"].(map[string]any)["combats"]) != 1 {
		t.Fatalf("bilan d'un fichier fermé : %d %v", c, v)
	}
	if lireTout(t, archive) != contenu {
		t.Fatal("le bilan a touché au fichier")
	}
	if entrees, _ := os.ReadDir(b.dir); strings.Contains(fmt.Sprint(entrees), ".bilan") {
		t.Fatalf("le bilan laisse un fichier : %v", entrees)
	}
	if c, v = b.action("Mon univers", "supprimer", map[string]any{"nom": "Mon univers"}); c != 409 {
		t.Fatalf("suppression sans avertissement : %d %v", c, v)
	}
	if c, v = b.action("Mon univers", "supprimer", map[string]any{"nom": "Mon univers", "bilan_vu": vu}); c != 200 {
		t.Fatalf("suppression confirmée : %d %v", c, v)
	}
}

// Un fichier abîmé ne s'ouvre plus : il se supprime quand même, sa sauvegarde n'étant qu'une copie non
// vérifiée, et le risque se confirme. Ce qui n'est pas un univers reste intouchable.
func TestSupprimerAbime(t *testing.T) {
	b := nouvelleAppli(t, t.TempDir())
	chemin := filepath.Join(b.dossier(), "Abîmé.cofdm")
	creerFichier(t, chemin)
	f, err := os.OpenFile(chemin, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteAt([]byte(strings.Repeat("\xff", 3000)), 100); err != nil {
		t.Fatal(err)
	}
	f.Close()
	fiche := b.liste()["Abîmé"]
	if fiche["abime"] != true || fiche["gerable"] != false || aAction(fiche, "ouvrir") || aAction(fiche, "renommer") || aAction(fiche, "archiver") || !aAction(fiche, "supprimer") {
		t.Fatalf("fiche : %v", fiche)
	}
	c, v := b.action("Abîmé", "bilan", nil)
	if c != 200 || v["bilan"].(map[string]any)["inconnu"] != true {
		t.Fatalf("bilan : %d %v", c, v)
	}
	vu := v["bilan"]
	if c, v := b.action("Abîmé", "supprimer", map[string]any{"nom": "Abîmé"}); c != 409 {
		t.Fatalf("sans confirmer le risque : %d %v", c, v)
	}
	c, v = b.action("Abîmé", "supprimer", map[string]any{"nom": "Abîmé", "bilan_vu": vu})
	if c != 200 || v["verifiee"] != false {
		t.Fatalf("suppression : %d %v", c, v)
	}
	plusLa(t, chemin)
	if _, err := os.Stat(v["sauvegarde"].(string)); err != nil {
		t.Fatal(err)
	}

	texte := filepath.Join(b.dossier(), "Texte.cofdm")
	if err := os.WriteFile(texte, []byte("ceci n'est pas un univers, et ce texte le montre"), 0o600); err != nil {
		t.Fatal(err)
	}
	if c, v := b.action("Texte", "supprimer", map[string]any{"nom": "Texte"}); c != 422 {
		t.Fatalf("fichier qui n'est pas un univers : %d %v", c, v)
	}
	if _, err := os.Stat(texte); err != nil {
		t.Fatal(err)
	}
}

// Une écriture interrompue : ni archivé ni supprimé, on l'ouvre d'abord.
func TestInterrompuNiArchiveNiSupprime(t *testing.T) {
	b := nouvelleAppli(t, t.TempDir())
	chemin := filepath.Join(b.dossier(), "Interrompu.cofdm")
	creerFichier(t, chemin)
	cle := b.cle("Interrompu")
	if err := os.WriteFile(chemin+suffixeJournal, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if c, v := b.local("POST", routeGestion+"/"+cle+"/archiver", map[string]any{}); c != 409 {
		t.Fatalf("archiver : %d %v", c, v)
	}
	if c, v := b.local("POST", routeGestion+"/"+cle+"/supprimer", map[string]any{"nom": "Interrompu"}); c != 409 {
		t.Fatalf("supprimer : %d %v", c, v)
	}
	if _, err := os.Stat(chemin); err != nil {
		t.Fatal(err)
	}
}

// Une copie reliée renommée continue de se synchroniser, sans toucher au nom du serveur.
func TestCopieRenommeeSeSynchronise(t *testing.T) {
	d := nouveauBancDirect(t)
	id := d.b.courant()["id"].(string)
	nomServeur := nomUnivers(d.a.st.Chemin())
	nom := d.b.courant()["nom"].(string)
	f := d.b.liste()[nom]
	if c := f["copie"].(map[string]any); c["etat"] != "actif" || c["url"] != d.ts.URL {
		t.Fatalf("copie : %v", f)
	}
	if c, v := d.b.action(nom, "renommer", map[string]any{"nom": "Mon exemplaire"}); c != 200 {
		t.Fatalf("renommer : %d %v", c, v)
	}
	if d.b.a.liens()[id].Chemin != d.b.courant()["chemin"] {
		t.Fatalf("lien : %+v", d.b.a.liens()[id])
	}
	if nomUnivers(d.a.st.Chemin()) != nomServeur {
		t.Fatalf("le serveur a changé de nom : %s", d.a.st.Chemin())
	}
	d.b.local("POST", "/api/entites", map[string]any{"type": "lieu", "nom": "La Tour"})
	if c, v := d.b.local("POST", "/api/univers/serveur/synchroniser", nil); c != 200 || v["etat"] != "a_jour" {
		t.Fatalf("échange : %d %v", c, v)
	}
	if n, _, _ := d.a.st.Compter(); n["lieu"] != 1 {
		t.Fatalf("le serveur n'a pas reçu la fiche : %v", n)
	}
	// le combat passe toujours par le serveur
	c, v := d.b.local("POST", "/api/combats", map[string]any{"nom": "Au gué"})
	if c != 201 {
		t.Fatalf("combat : %d %v", c, v)
	}
	if d.combatLocalExiste(idLance(v)) {
		t.Fatal("après le renommage, le combat ne passe plus par le serveur")
	}
	if c, _ := d.mj.faire("GET", fmt.Sprintf("/api/combats/%d", idLance(v)), nil); c != 200 {
		t.Fatalf("combat sur le serveur : %d", c)
	}
	_, v = d.b.action("Mon exemplaire", "bilan", nil)
	if entierDe(v["bilan"].(map[string]any)["direct"]) != 1 || entierDe(v["bilan"].(map[string]any)["combats"]) != 0 {
		t.Fatalf("bilan d'un combat en direct : %v", v)
	}
}

// Supprimer une copie reliée : seule la copie locale part, ses changements en attente se confirment, et
// le serveur est intact ; elle peut être tirée de nouveau.
func TestSupprimerCopieReliee(t *testing.T) {
	d := nouveauBancDirect(t)
	id := d.b.courant()["id"].(string)
	nom := d.b.courant()["nom"].(string)
	chemin := d.b.courant()["chemin"].(string)
	d.mj.faire("POST", "/api/entites", map[string]any{"type": "lieu", "nom": "Sur le serveur"})
	d.couper() // hors connexion : ce qui change ici attend
	d.b.local("POST", "/api/entites", map[string]any{"type": "lieu", "nom": "Pas envoyée"})
	c, v := d.b.action(nom, "bilan", nil)
	if c != 200 || entierDe(v["bilan"].(map[string]any)["attente"]) == 0 {
		t.Fatalf("bilan : %d %v", c, v)
	}
	vu := v["bilan"]
	suivant := d.b.cle("Mon univers")
	if c, v := d.b.action(nom, "supprimer", map[string]any{"nom": nom, "suivant": suivant}); c != 409 || !strings.Contains(v["detail"].(string), "attente") {
		t.Fatalf("sans confirmation : %d %v", c, v)
	}
	if _, err := os.Stat(chemin); err != nil {
		t.Fatal(err)
	}
	// un changement de plus depuis l'avertissement : le MJ doit le revoir
	d.b.local("POST", "/api/entites", map[string]any{"type": "lieu", "nom": "Encore une"})
	if c, v := d.b.action(nom, "supprimer", map[string]any{"nom": nom, "suivant": suivant, "bilan_vu": vu}); c != 409 {
		t.Fatalf("bilan dépassé : %d %v", c, v)
	}
	_, v = d.b.action(nom, "bilan", nil)
	c, v = d.b.action(nom, "supprimer", map[string]any{"nom": nom, "suivant": suivant, "bilan_vu": v["bilan"]})
	if c != 200 {
		t.Fatalf("suppression confirmée : %d %v", c, v)
	}
	plusLa(t, chemin)
	if fichesDe(t, v["sauvegarde"].(string)) == 0 {
		t.Fatal("la sauvegarde n'a pas les fiches")
	}
	if _, ok := d.b.a.liens()[id]; ok {
		t.Fatal("le lien de la copie supprimée est resté")
	}
	d.demarrer()
	if n, _, _ := d.a.st.Compter(); n["lieu"] != 1 {
		t.Fatalf("le serveur a changé : %v", n)
	}
	if c, _ := d.mj.faire("GET", "/api/mes-univers", nil); c != 200 {
		t.Fatalf("le serveur ne répond plus : %d", c)
	}
	if c, v := d.b.local("POST", "/api/univers/serveur/ouvrir", map[string]any{"univers": id}); c != 200 {
		t.Fatalf("tirer de nouveau : %d %v", c, v)
	}
	if b := d.b.fiches(); b != 1 {
		t.Fatalf("nouvelle copie : %d fiches", b)
	}
}

// Archiver une copie reliée : elle ne se synchronise plus et n'est pas tirée une seconde fois ; désarchivée,
// ses changements repartent.
func TestArchiverCopieReliee(t *testing.T) {
	d := nouveauBancDirect(t)
	id := d.b.courant()["id"].(string)
	nom := d.b.courant()["nom"].(string)
	suivant := filepath.Join(d.b.dossier(), "Local.cofdm")
	creerFichier(t, suivant)
	d.b.local("POST", "/api/entites", map[string]any{"type": "lieu", "nom": "Pas envoyée"})
	_, v := d.b.action(nom, "bilan", nil)
	c, v := d.b.action(nom, "archiver", map[string]any{"suivant": d.b.cle("Local"), "bilan_vu": v["bilan"]})
	if c != 200 {
		t.Fatalf("archiver : %d %v", c, v)
	}
	if n, _, _ := d.a.st.Compter(); n["lieu"] != 0 {
		t.Fatalf("le serveur a reçu la fiche à l'archivage : %v", n)
	}
	l := d.b.a.liens()[id]
	if !l.Archive || !dansDossier(l.Chemin, d.b.a.DossierArchives()) {
		t.Fatalf("lien : %+v", l)
	}
	if c, v := d.b.local("POST", "/api/univers/serveur/ouvrir", map[string]any{"univers": id}); c != 409 || !strings.Contains(v["detail"].(string), "archivée") {
		t.Fatalf("ouvrir une copie archivée depuis le serveur : %d %v", c, v)
	}
	if c, v := d.b.local("POST", "/api/univers/serveur/relier", map[string]any{"univers": id}); c != 409 {
		t.Fatalf("tirer une seconde copie : %d %v", c, v)
	}
	_, v = d.b.local("GET", "/api/univers/serveur/univers", nil)
	if u := v["univers"].([]any)[0].(map[string]any); u["archivee"] != true {
		t.Fatalf("liste du serveur : %v", u)
	}
	if c, v := d.b.action(nom, "desarchiver", map[string]any{}); c != 200 {
		t.Fatalf("désarchiver : %d %v", c, v)
	}
	if d.b.a.liens()[id].Archive {
		t.Fatalf("lien : %+v", d.b.a.liens()[id])
	}
	if c, v := d.b.local("POST", "/api/univers/serveur/ouvrir", map[string]any{"univers": id}); c != 200 {
		t.Fatalf("ouvrir après désarchivage : %d %v", c, v)
	}
	if c, v := d.b.local("POST", "/api/univers/serveur/synchroniser", nil); c != 200 || v["etat"] != "a_jour" {
		t.Fatalf("échange : %d %v", c, v)
	}
	if n, _, _ := d.a.st.Compter(); n["lieu"] != 1 {
		t.Fatalf("la fiche n'est pas repartie : %v", n)
	}
}
