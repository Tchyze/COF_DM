package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/Tchyze/cof_dm/internal/comptes"
)

// Un autre programme tient 127.0.0.1 sur le port voulu (comme le relais de WSL sous Windows) :
// l'application ouverte au réseau prend le port suivant, pour que sa fenêtre ne l'ouvre pas.
func TestEcouterFenetreLibre(t *testing.T) {
	autre, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer autre.Close()
	pris := autre.Addr().(*net.TCPAddr).Port
	ln, port, err := ecouter("0.0.0.0", pris, 5)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if port == pris {
		t.Fatalf("port %d pris sur 127.0.0.1, gardé quand même", pris)
	}
	// la fenêtre joint bien cette application
	c, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
}

// L'application a dû prendre un autre port que celui de config.json : cof-dm -mcp le lit dans
// le port publié, et retombe sur config.json quand elle ne tourne plus.
func TestPortPublie(t *testing.T) {
	dossier := t.TempDir()
	if err := ecrireConfig(dossier, config{Port: 8100}); err != nil {
		t.Fatal(err)
	}
	if p := portApplication(dossier); p != 8100 {
		t.Fatalf("sans port publié : %d, attendu 8100 (config.json)", p)
	}
	if err := publierPort(dossier, 8101); err != nil {
		t.Fatal(err)
	}
	if p := portApplication(dossier); p != 8101 {
		t.Fatalf("port publié : %d, attendu 8101", p)
	}
	// l'application relancée par une mise à jour a publié le sien : l'ancienne n'y touche pas
	if err := os.WriteFile(filepath.Join(dossier, fichierPort), []byte(`{"port":8102,"pid":-1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	retirerPort(dossier)
	if p := portApplication(dossier); p != 8102 {
		t.Fatalf("port d'une autre instance effacé : %d", p)
	}
	if err := publierPort(dossier, 8101); err != nil {
		t.Fatal(err)
	}
	retirerPort(dossier)
	if p := portApplication(dossier); p != 8100 {
		t.Fatalf("après l'arrêt : %d, attendu 8100", p)
	}
	// un fichier abîmé ne compte pas
	if err := os.WriteFile(filepath.Join(dossier, fichierPort), []byte(`{"port":80}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if p := portApplication(dossier); p != 8100 {
		t.Fatalf("port publié invalide : %d", p)
	}
}

// adresseDuReseau : une adresse de cet ordinateur sur un réseau, pour jouer l'appareil du réseau.
func adresseDuReseau(t *testing.T) string {
	t.Helper()
	as, err := net.InterfaceAddrs()
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range as {
		if n, ok := a.(*net.IPNet); ok && !n.IP.IsLoopback() && !n.IP.IsLinkLocalUnicast() && n.IP.To4() != nil {
			return n.IP.String()
		}
	}
	t.Skip("pas d'adresse réseau sur cet ordinateur")
	return ""
}

func porteDeTest(t *testing.T) (*ecoute, int) {
	t.Helper()
	ln, port, err := ecouter("127.0.0.1", 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	port = ln.Addr().(*net.TCPAddr).Port
	e := nouvelleEcoute(ln, port, false)
	mux := http.NewServeMux()
	mux.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "pong") })
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) { // comme un WebSocket : la connexion quitte le serveur HTTP
		c, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		fmt.Fprint(c, "ok")
		go func() {
			defer c.Close()
			_, _ = io.Copy(io.Discard, c)
		}()
	})
	e.srv = &http.Server{Handler: mux}
	e.servir(ln)
	t.Cleanup(func() { e.srv.Close() })
	return e, port
}

func ping(c *http.Client, hote string, port int) error {
	r, err := c.Get(fmt.Sprintf("http://%s:%d/ping", hote, port))
	if err != nil {
		return err
	}
	defer r.Body.Close()
	_, err = io.Copy(io.Discard, r.Body)
	return err
}

// Cocher ouvre sans relance et sans couper la fenêtre ; décocher referme, coupe les appareils du
// réseau (WebSocket compris) et laisse la fenêtre tranquille.
func TestEcouteBascule(t *testing.T) {
	lan := adresseDuReseau(t)
	e, port := porteDeTest(t)
	adr := net.JoinHostPort(lan, strconv.Itoa(port))
	if c, err := net.DialTimeout("tcp", adr, time.Second); err == nil {
		c.Close()
		t.Fatal("fermée, l'application répond au réseau")
	}
	fenetre, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatal(err)
	}
	defer fenetre.Close()
	_ = fenetre.SetDeadline(time.Now().Add(5 * time.Second))
	lecteur := bufio.NewReader(fenetre)
	requeteFenetre := func() {
		t.Helper()
		fmt.Fprintf(fenetre, "GET /ping HTTP/1.1\r\nHost: x\r\n\r\n")
		r, err := http.ReadResponse(lecteur, nil)
		if err != nil {
			t.Fatalf("connexion de la fenêtre : %v", err)
		}
		_, _ = io.Copy(io.Discard, r.Body)
		r.Body.Close()
	}
	requeteFenetre()
	if err := e.changer(true); err != nil {
		t.Fatal(err)
	}
	if !e.estOuvert() {
		t.Fatal("état après ouverture")
	}
	if err := ping(http.DefaultClient, lan, port); err != nil {
		t.Fatalf("ouverte, le réseau ne répond pas : %v", err)
	}
	if err := ping(http.DefaultClient, "127.0.0.1", port); err != nil {
		t.Fatalf("ouverte, la fenêtre ne répond plus : %v", err)
	}
	requeteFenetre()
	tablette, err := net.Dial("tcp", adr)
	if err != nil {
		t.Fatal(err)
	}
	defer tablette.Close()
	fmt.Fprintf(tablette, "GET /ws HTTP/1.1\r\nHost: x\r\n\r\n")
	lu := make([]byte, 2)
	if _, err := io.ReadFull(tablette, lu); err != nil {
		t.Fatal(err)
	}
	if err := e.changer(false); err != nil {
		t.Fatal(err)
	}
	_ = tablette.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, err := tablette.Read(lu); err == nil || os.IsTimeout(err) {
		t.Fatalf("la tablette n'est pas coupée : %v", err)
	}
	if c, err := net.DialTimeout("tcp", adr, time.Second); err == nil {
		c.Close()
		t.Fatal("refermée, l'application répond encore au réseau")
	}
	// la connexion de la fenêtre a traversé les deux bascules
	requeteFenetre()
	e.mu.Lock()
	reste := len(e.reseau)
	e.mu.Unlock()
	if reste != 0 {
		t.Fatalf("connexions encore suivies : %d", reste)
	}
}

// Le port est pris sur le réseau (ou par le relais de WSL) : l'application le dit et reste comme avant.
func TestEcoutePortPris(t *testing.T) {
	lan := adresseDuReseau(t)
	e, port := porteDeTest(t)
	autre, err := net.Listen("tcp", net.JoinHostPort(lan, strconv.Itoa(port)))
	if err != nil {
		t.Fatal(err)
	}
	defer autre.Close()
	if err := e.changer(true); err == nil {
		if runtime.GOOS == "linux" {
			t.Fatal("ouverture acceptée, port pris")
		}
		return
	}
	if e.estOuvert() {
		t.Fatal("dite ouverte après l'échec")
	}
	if err := ping(http.DefaultClient, "127.0.0.1", port); err != nil {
		t.Fatalf("la fenêtre ne répond plus après l'échec : %v", err)
	}
	autre.Close()
	if err := e.changer(true); err != nil {
		t.Fatalf("second essai, port libre : %v", err)
	}
}

func TestProxiesDemandes(t *testing.T) {
	t.Setenv("COF_DM_PROXIES", "")
	if l, err := proxiesDemandes(""); err != nil || len(l) != 2 {
		t.Fatalf("défaut : %v %v", l, err)
	}
	t.Setenv("COF_DM_PROXIES", "10.0.0.0/8")
	if l, err := proxiesDemandes(""); err != nil || len(l) != 1 || l[0].String() != "10.0.0.0/8" {
		t.Fatalf("variable : %v %v", l, err)
	}
	if l, err := proxiesDemandes("172.30.0.0/24"); err != nil || len(l) != 1 || l[0].String() != "172.30.0.0/24" {
		t.Fatalf("l'option passe avant la variable : %v %v", l, err)
	}
	for _, v := range []string{"pas-une-adresse", "0.0.0.0/0", ",,"} {
		if _, err := proxiesDemandes(v); err == nil {
			t.Errorf("%q accepté : le serveur doit refuser de démarrer", v)
		}
	}
}

// Le premier compte : un mot de passe tiré, affiché et à changer ; ou celui d'un secret, jamais affiché.
func TestPremierAdmin(t *testing.T) {
	ouvrir := func() *comptes.Registre {
		r, err := comptes.Ouvrir(filepath.Join(t.TempDir(), "comptes.json"))
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	t.Setenv(envMdpAdmin, "")
	r := ouvrir()
	if err := premierAdmin(r); err != nil {
		t.Fatal(err)
	}
	if c, ok := r.Lire("admin"); !ok || !c.Admin || !c.DoitChanger {
		t.Fatalf("mot de passe tiré : %+v", c)
	}

	secret := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(secret, []byte("une-phrase-bien-longue\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(envMdpAdmin, secret)
	r = ouvrir()
	if err := premierAdmin(r); err != nil {
		t.Fatal(err)
	}
	if c, ok := r.Verifier("admin", "une-phrase-bien-longue"); !ok || c.DoitChanger {
		t.Fatalf("mot de passe du secret : %+v %v", c, ok)
	}
	// un compte existe déjà : le secret n'y change rien
	if err := os.WriteFile(secret, []byte("une-tout-autre-phrase\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := premierAdmin(r); err != nil {
		t.Fatal(err)
	}
	if _, ok := r.Verifier("admin", "une-phrase-bien-longue"); !ok {
		t.Fatal("le compte existant ne change pas")
	}
	// un secret faible ou absent : le serveur le dit et ne crée rien
	if err := os.WriteFile(secret, []byte("1234567890\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r = ouvrir()
	if err := premierAdmin(r); err == nil || !r.Vide() {
		t.Fatalf("secret faible : %v", err)
	}
	if err := os.Remove(secret); err != nil {
		t.Fatal(err)
	}
	if err := premierAdmin(r); err == nil || !r.Vide() {
		t.Fatalf("secret absent : %v", err)
	}
}
