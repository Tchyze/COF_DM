package serveur

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Tchyze/cof_dm/internal/comptes"
)

// requeteLente : une écriture dont le corps n'arrive pas, tant que le test ne libère pas sa réponse.
func requeteLente(t *testing.T, n *navigateur, univers string) (liberer func() int) {
	t.Helper()
	lecteur, ecrivain := io.Pipe()
	r := httptest.NewRequest("POST", "/api/entites", lecteur)
	r.Host, r.RemoteAddr = "127.0.0.1:8100", "127.0.0.1:5000"
	r.Header.Set("X-COF-DM", "1")
	r.Header.Set(enTeteUnivers, univers)
	for _, c := range n.cookies {
		r.AddCookie(c)
	}
	w := httptest.NewRecorder()
	fini := make(chan struct{})
	go func() {
		defer close(fini)
		n.a.ServeHTTP(w, r)
	}()
	if _, err := ecrivain.Write([]byte(`{"type":"lieu",`)); err != nil { // le serveur a commencé à lire
		t.Fatal(err)
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
	t.Cleanup(func() { liberer() })
	return liberer
}

// avant : l'appel répond dans le délai ; sinon quelque chose le bloque.
func avant(t *testing.T, delai time.Duration, quoi string, f func()) {
	t.Helper()
	fini := make(chan struct{})
	go func() {
		defer close(fini)
		f()
	}()
	select {
	case <-fini:
	case <-time.After(delai):
		t.Fatalf("%s : pas de réponse après %s", quoi, delai)
	}
}

// Une requête dont le corps n'arrive pas ne bloque ni les autres requêtes ni l'administration.
func TestRequeteLenteNeBloquePasLeServeur(t *testing.T) {
	a, reg, connecter := serveurMulti(t)
	chef, meneur := connecter("chef"), connecter("meneur")
	univers := a.st.ID()
	donner(reg, "meneur", univers, comptes.MJ)
	liberer := requeteLente(t, meneur, univers)

	// la création d'un univers demande le verrou d'écriture : elle attendrait, et tout le monde après elle
	avant(t, 3*time.Second, "création d'un univers", func() { creerUnivers(t, chef, "Les Marches") })
	avant(t, 3*time.Second, "lecture", func() {
		if c, _ := meneur.faire("GET", "/api/mes-univers", nil); c != 200 {
			t.Errorf("mes univers : %d", c)
		}
		if c, _ := meneur.faire("GET", "/api/entites?type=lieu", nil, enTeteUnivers, univers); c != 200 {
			t.Errorf("lecture : %d", c)
		}
	})
	if c := liberer(); c != http.StatusOK && c != http.StatusCreated {
		t.Fatalf("la requête lente aboutit : %d", c)
	}
	if n, _, _ := a.st.Compter(); n["lieu"] != 1 {
		t.Fatalf("écriture de la requête lente : %v", n)
	}
}

// attendreCompte : la requête lente est bien comptée dans son univers.
func attendreCompte(t *testing.T, o *universOuvert) {
	t.Helper()
	for i := 0; i < 500; i++ {
		o.requetes.mu.Lock()
		n := o.requetes.n
		o.requetes.mu.Unlock()
		if n > 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("requête non comptée")
}

// attendreFermeture : l'opération a retiré l'univers du service et attend ses requêtes.
func attendreFermeture(t *testing.T, a *Application, id string) {
	t.Helper()
	for i := 0; i < 2000; i++ {
		a.mu.RLock()
		en := a.fermetures[id]
		a.mu.RUnlock()
		if en {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("l'univers n'est jamais passé en fermeture")
}

// Une fermeture attend les requêtes de l'univers : la requête lente écrit dans une base encore
// ouverte. En attendant, l'univers répond 503, à l'en-tête comme au cookie, jamais un autre à sa
// place, et les autres univers ne sentent rien.
func TestFermetureAttendLesRequetes(t *testing.T) {
	a, reg, connecter := serveurMulti(t)
	chef, meneur, voisine := connecter("chef"), connecter("meneur"), connecter("voisine")
	premier := a.st.ID()
	id := creerUnivers(t, chef, "Les Marches")
	donner(reg, "meneur", id, comptes.MJ)
	donner(reg, "meneur", premier, comptes.MJ)
	donner(reg, "voisine", premier, comptes.MJ)
	if c, _ := meneur.faire("POST", "/api/mes-univers/courant", map[string]any{"id": id}); c != 200 {
		t.Fatalf("choix de l'univers : %d", c)
	}
	o := a.tous[id]
	liberer := requeteLente(t, meneur, id)
	attendreCompte(t, o)

	fin := make(chan int)
	go func() {
		c, _ := chef.faire("PATCH", "/api/admin/univers/"+id, map[string]any{"archive": true})
		fin <- c
	}()
	attendreFermeture(t, a, id)

	if c, _ := meneur.faire("GET", "/api/entites?type=lieu", nil, enTeteUnivers, id); c != 503 {
		t.Fatalf("par l'en-tête, pendant la fermeture : %d", c)
	}
	// par le cookie : ce navigateur écrirait dans l'autre univers de son compte
	if c, _ := meneur.faire("POST", "/api/entites", map[string]any{"type": "lieu", "nom": "Ailleurs"}); c != 503 {
		t.Fatalf("par le cookie, pendant la fermeture : %d", c)
	}
	if n, _, _ := a.tous[premier].st.Compter(); n["lieu"] != 0 {
		t.Fatalf("écrit dans un autre univers : %v", n)
	}
	if c, _ := voisine.faire("POST", "/api/entites", map[string]any{"type": "lieu", "nom": "Fort Voisin"}); c >= 300 {
		t.Fatalf("un autre univers pendant la fermeture : %d", c)
	}
	select {
	case c := <-fin:
		t.Fatalf("la fermeture n'a pas attendu la requête : %d", c)
	case <-time.After(50 * time.Millisecond):
	}

	if c := liberer(); c >= 300 {
		t.Fatalf("la requête en cours pendant la fermeture : %d", c)
	}
	if c := <-fin; c != 200 {
		t.Fatalf("archivage : %d", c)
	}
	if c, _ := meneur.faire("GET", "/api/entites?type=lieu", nil, enTeteUnivers, id); c != 404 {
		t.Fatalf("univers archivé : %d", c)
	}
	// rien de perdu : ce que la requête a écrit est dans le fichier archivé
	if c, _ := chef.faire("PATCH", "/api/admin/univers/"+id, map[string]any{"archive": false}); c != 200 {
		t.Fatalf("désarchivage : %d", c)
	}
	if n, _, _ := a.tous[id].st.Compter(); n["lieu"] != 1 {
		t.Fatalf("écriture de la requête en cours : %v", n)
	}
}

// Si les requêtes durent, rien n'est fermé ni déplacé : 409, l'univers reste servi, et toujours par
// défaut s'il l'était.
func TestFermetureDelaiDepasse(t *testing.T) {
	a, reg, connecter := serveurMulti(t)
	chef, meneur := connecter("chef"), connecter("meneur")
	id := a.st.ID()
	autre := creerUnivers(t, chef, "Les Marches")
	donner(reg, "meneur", id, comptes.MJ)
	o, chemin := a.tous[id], a.tous[id].st.Chemin()
	if a.st != o.st {
		t.Fatal("univers par défaut")
	}
	a.delaiFermeture = 50 * time.Millisecond
	liberer := requeteLente(t, meneur, id)
	attendreCompte(t, o)

	nom := nomUnivers(chemin)
	for quoi, essai := range map[string]func() int{
		"archiver": func() int {
			c, _ := chef.faire("PATCH", "/api/admin/univers/"+id, map[string]any{"archive": true})
			return c
		},
		"renommer": func() int {
			c, _ := chef.faire("PATCH", "/api/admin/univers/"+id, map[string]any{"nom": "Autre nom"})
			return c
		},
		"supprimer": func() int {
			c, _ := chef.faire("DELETE", "/api/admin/univers/"+id, map[string]any{"nom": nom})
			return c
		},
	} {
		if c := essai(); c != 409 {
			t.Fatalf("%s pendant une requête qui dure : %d", quoi, c)
		}
		if a.tous[id] != o || a.st != o.st || a.srv != o.srv || len(a.fermetures) != 0 {
			t.Fatalf("%s : univers remis tel quel", quoi)
		}
		if c, _ := meneur.faire("GET", "/api/entites?type=lieu", nil, enTeteUnivers, id); c != 200 {
			t.Fatalf("%s : univers encore servi : %d", quoi, c)
		}
	}
	if c := liberer(); c >= 300 {
		t.Fatalf("la requête lente : %d", c)
	}
	if n, _, _ := o.st.Compter(); n["lieu"] != 1 {
		t.Fatalf("la base doit être restée ouverte : %v", n)
	}
	// les requêtes finies, l'opération passe ; l'univers renommé reste celui par défaut
	if c, _ := chef.faire("PATCH", "/api/admin/univers/"+id, map[string]any{"nom": "Autre nom"}); c != 200 {
		t.Fatalf("renommer ensuite : %d", c)
	}
	if a.st.ID() != id || a.tous[autre] == nil {
		t.Fatalf("univers par défaut après renommage : %s", a.st.ID())
	}
}

// La restauration attend aussi, et l'univers restauré reste celui par défaut. Pendant qu'elle
// attend, une autre requête admise ne tombe jamais sur un autre univers.
func TestRestaurationPendantUneRequete(t *testing.T) {
	a, reg, connecter := serveurMulti(t)
	chef, meneur := connecter("chef"), connecter("meneur")
	id := a.st.ID()
	creerUnivers(t, chef, "Les Marches")
	donner(reg, "meneur", id, comptes.MJ)
	meneur.faire("POST", "/api/entites", map[string]any{"type": "lieu", "nom": "Avant"}, enTeteUnivers, id)
	o := a.tous[id]
	s, err := sauvegarder(o.st, a.dossierSauvegardes(o.st), "manuelle")
	if err != nil {
		t.Fatal(err)
	}
	fichier := filepath.Base(s)

	a.delaiFermeture = 50 * time.Millisecond
	liberer := requeteLente(t, meneur, id)
	attendreCompte(t, o)
	if c, v := meneur.faire("POST", "/api/univers/restaurer", map[string]any{"fichier": fichier}, enTeteUnivers, id); c != 409 {
		t.Fatalf("restauration pendant une requête qui dure : %d %v", c, v)
	}
	if a.tous[id] != o {
		t.Fatal("l'univers doit rester ouvert")
	}
	if c := liberer(); c >= 300 {
		t.Fatalf("la requête lente : %d", c)
	}
	if n, _, _ := o.st.Compter(); n["lieu"] != 2 {
		t.Fatalf("avant la restauration : %v", n)
	}
	a.delaiFermeture = delaiFermeture
	if c, v := meneur.faire("POST", "/api/univers/restaurer", map[string]any{"fichier": fichier}, enTeteUnivers, id); c != 200 {
		t.Fatalf("restauration : %d %v", c, v)
	}
	if n, _, _ := a.tous[id].st.Compter(); n["lieu"] != 1 || a.st.ID() != id {
		t.Fatalf("après la restauration : %v, par défaut %s", n, a.st.ID())
	}
	// la sauvegarde d'avant a ce que la requête lente avait écrit
	var avant string
	for _, x := range listerSauvegardes(a.dossierSauvegardes(a.st)) {
		if x.Motif == "avant-restauration" {
			avant = x.Fichier
		}
	}
	if avant == "" {
		t.Fatal("pas de sauvegarde d'avant la restauration")
	}
	if c, _ := meneur.faire("POST", "/api/univers/restaurer", map[string]any{"fichier": avant}, enTeteUnivers, id); c != 200 {
		t.Fatalf("retour en arrière : %d", c)
	}
	if n, _, _ := a.tous[id].st.Compter(); n["lieu"] != 2 {
		t.Fatalf("sauvegarde d'avant la restauration : %v", n)
	}
}

// Sur le bureau : changer d'univers pendant une requête qui dure laisse l'univers ouvert à sa place.
func TestBureauChangerPendantUneRequete(t *testing.T) {
	b := nouvelleAppli(t, t.TempDir())
	id := b.a.st.ID()
	ici := &navigateur{t: t, a: b.a, cookies: map[string]*http.Cookie{}}
	o := b.a.tous[id]
	b.a.delaiFermeture = 50 * time.Millisecond
	liberer := requeteLente(t, ici, id)
	attendreCompte(t, o)
	chemin := filepath.Join(b.a.DossierUnivers(), "Autre.cofdm")
	if c, v := b.local("POST", "/api/univers/nouveau", map[string]any{"chemin": chemin}); c != 409 {
		t.Fatalf("changer d'univers pendant une requête : %d %v", c, v)
	}
	if _, err := os.Stat(chemin); err == nil {
		t.Fatal("le fichier du nouvel univers ne doit pas rester")
	}
	if b.a.st != o.st || b.a.srv != o.srv {
		t.Fatal("l'univers ouvert doit rester")
	}
	if c := liberer(); c >= 300 {
		t.Fatalf("la requête : %d", c)
	}
	if c, _ := b.local("POST", "/api/univers/nouveau", map[string]any{"chemin": chemin}); c != 200 {
		t.Fatalf("changer d'univers ensuite : %d", c)
	}
}

// À l'arrêt, une requête qui dure ne retient pas l'application indéfiniment.
func TestArretMalgreUneRequete(t *testing.T) {
	a, reg, connecter := serveurMulti(t)
	meneur := connecter("meneur")
	id := a.st.ID()
	donner(reg, "meneur", id, comptes.MJ)
	a.delaiFermeture = 50 * time.Millisecond
	requeteLente(t, meneur, id)
	attendreCompte(t, a.tous[id])
	avant(t, 3*time.Second, "arrêt", a.Fermer)
	if len(a.tous) != 0 || a.st != nil {
		t.Fatal("tout doit être fermé")
	}
	if c, _ := meneur.faire("GET", "/api/entites?type=lieu", nil); c != 503 {
		t.Fatalf("après l'arrêt : %d", c)
	}
}

func TestCompteDesRequetes(t *testing.T) {
	var q requetes
	if !q.attendre(0, nil) {
		t.Fatal("rien en cours")
	}
	q.entrer()
	q.entrer()
	if q.attendre(5*time.Millisecond, nil) {
		t.Fatal("délai dépassé attendu")
	}
	abandon := make(chan struct{})
	close(abandon)
	if q.attendre(time.Minute, abandon) {
		t.Fatal("abandon attendu")
	}
	fini := make(chan bool)
	go func() { fini <- q.attendre(time.Minute, nil) }()
	q.sortir()
	select {
	case <-fini:
		t.Fatal("encore une requête en cours")
	case <-time.After(5 * time.Millisecond):
	}
	q.sortir()
	if !<-fini {
		t.Fatal("le compte est à zéro")
	}
	// reprendre après une attente abandonnée
	q.entrer()
	q.sortir()
	if !q.attendre(0, nil) {
		t.Fatal("rien en cours")
	}
}

// Requêtes concurrentes pendant qu'on archive, désarchive et renomme : aucune ne doit tomber sur une
// base fermée (500), ni rester bloquée. À lancer avec -race.
func TestStressFermetures(t *testing.T) {
	a, reg, connecter := serveurMulti(t)
	chef := connecter("chef")
	premier := a.st.ID()
	id := creerUnivers(t, chef, "Les Marches")
	var nav []*navigateur
	for i := 0; i < 6; i++ {
		nom := fmt.Sprintf("meneur%d", i)
		n := connecter(nom)
		donner(reg, nom, premier, comptes.MJ)
		donner(reg, nom, id, comptes.MJ)
		nav = append(nav, n)
	}
	stop := make(chan struct{})
	var wg sync.WaitGroup
	var mu sync.Mutex
	codes := map[int]int{}
	for i, n := range nav {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for k := 0; ; k++ {
				select {
				case <-stop:
					return
				default:
				}
				var c int
				switch {
				case k%3 == 0:
					w := n.brut("POST", "/api/entites", map[string]any{"type": "lieu", "nom": fmt.Sprintf("Lieu %d-%d", i, k)}, enTeteUnivers, id)
					c = w.Code
				case k%3 == 1:
					c = n.brut("GET", "/api/entites?type=lieu", nil, enTeteUnivers, id).Code
				default:
					c = n.brut("GET", "/api/mes-univers", nil).Code
				}
				mu.Lock()
				codes[c]++
				mu.Unlock()
			}
		}()
	}
	nom := "Les Marches"
	for tour := 0; tour < 8; tour++ {
		if c, v := chef.faire("PATCH", "/api/admin/univers/"+id, map[string]any{"archive": true}); c != 200 {
			t.Errorf("archivage %d : %d %v", tour, c, v)
		}
		if c, v := chef.faire("PATCH", "/api/admin/univers/"+id, map[string]any{"archive": false}); c != 200 {
			t.Errorf("désarchivage %d : %d %v", tour, c, v)
		}
		nom = fmt.Sprintf("Les Marches %d", tour)
		if c, v := chef.faire("PATCH", "/api/admin/univers/"+id, map[string]any{"nom": nom}); c != 200 {
			t.Errorf("renommage %d : %d %v", tour, c, v)
		}
		if c, _ := chef.faire("GET", "/api/admin/univers", nil); c != 200 {
			t.Errorf("liste : %d", c)
		}
	}
	close(stop)
	avant(t, 30*time.Second, "fin des requêtes", wg.Wait)
	t.Logf("réponses : %v", codes)
	for c, n := range codes {
		if c >= 500 && c != 503 {
			t.Errorf("%d réponses %d : une requête a vu une base fermée", n, c)
		}
	}
	if codes[200]+codes[201] == 0 {
		t.Error("aucune requête n'a abouti")
	}
}
