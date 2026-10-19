// COF DM : gestionnaire de monde et de campagnes pour Chroniques Oubliées Fantasy.
//
// Par défaut, l'application ouvre sa fenêtre. Avec -headless, l'interface s'ouvre dans un navigateur.
// Elle n'écoute que sur cet ordinateur (127.0.0.1), sauf avec -reseau : l'API est alors joignable
// depuis le réseau local, avec le jeton d'API (voir docs/API.md). Une application reliée à un
// serveur ne s'ouvre jamais au réseau.
//
// Le mode serveur (comptes, plusieurs univers, derrière un reverse proxy) n'existe que dans
// l'édition serveur, compilée avec l'étiquette « serveur » pour l'image Docker.
//
// Un univers est un fichier .cofdm ; l'application rouvre le dernier, ou celui donné par -univers.
package main

import (
	"crypto/rand"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/Tchyze/cof_dm/internal/comptes"
	"github.com/Tchyze/cof_dm/internal/majour"
	"github.com/Tchyze/cof_dm/internal/reglages"
	"github.com/Tchyze/cof_dm/internal/serveur"
)

var version = "dev"

//go:embed frontend
var frontend embed.FS

type config struct {
	Port   int  `json:"port"`
	Reseau bool `json:"reseau,omitempty"` // écouter sur le réseau local (tablette du MJ)
}

// dossierPortable : un dossier « COF DM - données » posé à côté du programme (ou de l'AppImage)
// est utilisé à la place du dossier de l'utilisateur, pour emporter l'application sur une clé USB.
func dossierPortable() string {
	exe := os.Getenv("APPIMAGE")
	if exe == "" {
		exe, _ = os.Executable()
	}
	if exe == "" {
		return ""
	}
	d := filepath.Join(filepath.Dir(exe), "COF DM - données")
	if fi, err := os.Stat(d); err == nil && fi.IsDir() {
		return d
	}
	return ""
}

func dossierParDefaut() string {
	if d := dossierPortable(); d != "" {
		return d
	}
	switch runtime.GOOS {
	case "windows":
		if d := os.Getenv("APPDATA"); d != "" {
			return filepath.Join(d, "COF DM")
		}
	default:
		if d := os.Getenv("XDG_DATA_HOME"); d != "" {
			return filepath.Join(d, "cof-dm")
		}
		if h, err := os.UserHomeDir(); err == nil {
			return filepath.Join(h, ".local", "share", "cof-dm")
		}
	}
	return "donnees"
}

func lireConfig(dossier string) config {
	c := config{Port: 8100}
	if b, err := os.ReadFile(filepath.Join(dossier, "config.json")); err == nil {
		_ = json.Unmarshal(b, &c)
	}
	if c.Port < 1024 || c.Port > 65535 {
		c.Port = 8100
	}
	return c
}

func ecrireConfig(dossier string, c config) error {
	b, _ := json.MarshalIndent(c, "", "  ")
	return os.WriteFile(filepath.Join(dossier, "config.json"), b, 0o600)
}

// fichierPort : le port que l'application lancée a réellement pris, publié dans son dossier de
// données. Il diffère de config.json quand le port voulu était pris (sous Windows, le relais de WSL
// tient 127.0.0.1:8100 pour un serveur Docker) : cof-dm -mcp le lit pour joindre cette application.
const fichierPort = "port.json"

type portPublie struct {
	Port int `json:"port"`
	PID  int `json:"pid"`
}

func publierPort(dossier string, port int) error {
	b, _ := json.Marshal(portPublie{Port: port, PID: os.Getpid()})
	return os.WriteFile(filepath.Join(dossier, fichierPort), b, 0o600)
}

// retirerPort efface le port publié en quittant, s'il est encore le sien : l'application relancée
// par une mise à jour a déjà publié le sien.
func retirerPort(dossier string) {
	chemin := filepath.Join(dossier, fichierPort)
	b, err := os.ReadFile(chemin)
	if err != nil {
		return
	}
	var p portPublie
	if json.Unmarshal(b, &p) == nil && p.PID == os.Getpid() {
		_ = os.Remove(chemin)
	}
}

// portApplication : le port où joindre l'application de cet ordinateur. Celui qu'elle a publié en
// se lançant, sinon celui de config.json.
func portApplication(dossier string) int {
	if b, err := os.ReadFile(filepath.Join(dossier, fichierPort)); err == nil {
		var p portPublie
		if json.Unmarshal(b, &p) == nil && p.Port >= 1024 && p.Port <= 65535 {
			return p.Port
		}
	}
	return lireConfig(dossier).Port
}

// resserrer : les données d'une installation d'avant étaient lisibles par les autres comptes de
// l'ordinateur. Les univers contiennent les secrets du MJ : seul leur propriétaire les lit.
func resserrer(dossier string) {
	for _, d := range []string{"univers", "sauvegardes", "medias"} {
		_ = filepath.WalkDir(filepath.Join(dossier, d), func(chemin string, e os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if e.IsDir() {
				_ = os.Chmod(chemin, 0o700)
			} else if e.Type().IsRegular() {
				_ = os.Chmod(chemin, 0o600)
			}
			return nil
		})
	}
	for _, f := range []string{"config.json", "cof-dm.log", "cof-dm.db", "cof-dm.db.ancien"} {
		if fi, err := os.Lstat(filepath.Join(dossier, f)); err == nil && fi.Mode().IsRegular() {
			_ = os.Chmod(filepath.Join(dossier, f), 0o600)
		}
	}
}

// envRelance : présent quand l'application vient d'être relancée par une mise à jour.
const envRelance = "COF_DM_RELANCE"

// attendrePort laisse jusqu'à dix secondes à l'instance d'avant pour libérer le port, afin que
// l'application relancée garde la même adresse.
func attendrePort(hote string, port int) {
	for fin := time.Now().Add(10 * time.Second); time.Now().Before(fin); time.Sleep(200 * time.Millisecond) {
		if ln, _, err := ecouter(hote, port, 1); err == nil {
			ln.Close()
			return
		}
	}
}

// ecouter ouvre le port demandé, ou le premier libre parmi les suivants. Ouverte au réseau, elle
// exige aussi que 127.0.0.1 soit libre sur ce port : c'est l'adresse de sa fenêtre. Sous Windows,
// WSL y relaie les serveurs de Linux (un serveur COF DM en Docker, par exemple) sans empêcher
// l'écoute sur toutes les adresses : la fenêtre ouvrirait alors l'autre programme.
func ecouter(hote string, port, essais int) (net.Listener, int, error) {
	var dernier error
	for p := port; p < port+essais; p++ {
		if hote != "127.0.0.1" {
			sonde, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
			if err != nil {
				dernier = err
				continue
			}
			sonde.Close()
		}
		ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", hote, p))
		if err == nil {
			return ln, p, nil
		}
		dernier = err
	}
	return nil, 0, dernier
}

func main() {
	headless := flag.Bool("headless", false, "serveur seul, sans fenêtre (l'interface s'ouvre dans le navigateur)")
	portFlag := flag.Int("port", 0, "port d'écoute (par défaut : config.json, sinon 8100)")
	dataFlag := flag.String("data", "", "dossier de l'application (réglages, univers par défaut, sauvegardes)")
	universFlag := flag.String("univers", "", "fichier d'univers (.cofdm) à ouvrir ; créé s'il n'existe pas")
	reseau := flag.Bool("reseau", false, "écouter sur le réseau local (API avec jeton, interface avec le mot de passe MJ)")
	modeServeur := flag.Bool("serveur", editionServeur, "mode serveur, derrière un reverse proxy : sans fenêtre, accès par comptes (édition serveur, image Docker)")
	modeMCP := flag.Bool("mcp", false, "serveur MCP pour Claude, sur l'entrée et la sortie standard : joint l'application lancée par son API (COF_DM_URL, COF_DM_JETON, COF_DM_UNIVERS)")
	flag.Parse()
	if *modeMCP {
		lancerMCP(*dataFlag, *portFlag)
		return
	}
	if *modeServeur && !editionServeur {
		fatal("Le mode serveur n'existe que dans l'image Docker (voir README, « Avec Docker »). L'application de bureau se relie à un serveur depuis la zone en bas à gauche de sa fenêtre.")
	}
	*modeServeur = editionServeur // l'édition serveur n'a pas d'autre mode
	if *modeServeur {
		*headless = true
	}

	dossier := *dataFlag
	if dossier == "" {
		dossier = dossierParDefaut()
	}
	if err := os.MkdirAll(dossier, 0o700); err != nil {
		fatal("Impossible de créer le dossier des données %s : %v", dossier, err)
	}
	resserrer(dossier)
	if logf, err := os.OpenFile(filepath.Join(dossier, "cof-dm.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil && !*headless {
		log.SetOutput(logf)
	}

	cfg := lireConfig(dossier)
	if *portFlag > 0 {
		cfg.Port = *portFlag
	}
	reg := reglages.Ouvrir(filepath.Join(dossier, "reglages.json"))
	// une application reliée à un serveur ne s'ouvre pas au réseau local : tablette et joueurs passent
	// par le serveur
	relie := !*modeServeur && serveur.BureauRelie(reg)
	hote := "127.0.0.1"
	switch {
	case relie && (*reseau || cfg.Reseau):
		log.Print("application reliée à un serveur : le réseau local reste fermé (passez en mode local pour l'ouvrir)")
	case *reseau || (cfg.Reseau && !*modeServeur): // un serveur reste derrière son reverse proxy, sauf -reseau
		hote = "0.0.0.0"
	}
	if os.Getenv(envRelance) != "" {
		attendrePort(hote, cfg.Port) // l'instance d'avant la mise à jour libère son port
	}
	ln, port, err := ecouter(hote, cfg.Port, 10)
	if err != nil {
		fatal("Le port %d est déjà utilisé (%v). Fermez l'autre programme ou changez de port.", cfg.Port, err)
	}

	sub, _ := fs.Sub(frontend, "frontend")
	opt := serveur.Options{Version: version, DossierDon: dossier, Reglages: reg, Frontend: sub, Adresse: fmt.Sprintf("%s:%d", hote, port),
		Port: port, Reseau: hote != "127.0.0.1", ReseauForce: *reseau && hote != "127.0.0.1", ReseauVoulu: cfg.Reseau}
	if dialogueDisponible() {
		opt.Dialogue = dialogueFichier
	}
	opt.Config = func(ouvrir bool) error {
		c := lireConfig(dossier) // le port enregistré, pas celui de -port
		c.Reseau = ouvrir
		return ecrireConfig(dossier, c)
	}
	if *modeServeur {
		opt.Dialogue, opt.Config = nil, nil
		if opt.Comptes, err = comptes.Ouvrir(filepath.Join(dossier, "comptes.json")); err != nil {
			fatal("Impossible de lire les comptes : %v", err)
		}
		if err := premierAdmin(opt.Comptes); err != nil {
			fatal("Impossible de créer le compte administrateur : %v", err)
		}
	}
	var app *serveur.Application
	if !*modeServeur {
		majour.Nettoyer() // le programme d'avant la dernière mise à jour
		opt.Relancer = func(programme string) {
			cmd := exec.Command(programme, os.Args[1:]...)
			cmd.Env = append(os.Environ(), envRelance+"=1")
			if err := cmd.Start(); err != nil {
				log.Printf("relance après la mise à jour impossible : %v", err)
				return
			}
			app.Fermer() // l'univers est sauvegardé et fermé
			os.Exit(0)
		}
	}
	app = serveur.NouvelleApplication(opt)
	if err := app.Demarrer(*universFlag); err != nil {
		fatal("Impossible d'ouvrir l'univers : %v", err)
	}
	defer app.Fermer()
	// délais : une connexion lente ou muette ne reste pas ouverte indéfiniment (les plus longs
	// laissent passer un export ou une restauration avec images sur un réseau lent)
	srv := &http.Server{Handler: app, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 5 * time.Minute,
		WriteTimeout: 10 * time.Minute, IdleTimeout: 2 * time.Minute, MaxHeaderBytes: 64 << 10}
	if err := publierPort(dossier, port); err != nil {
		log.Printf("port non publié pour cof-dm -mcp : %v", err)
	}
	defer retirerPort(dossier)
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("serveur arrêté : %v", err)
		}
	}()
	url := fmt.Sprintf("http://127.0.0.1:%d/", port)
	log.Printf("COF DM %s sur %s, données dans %s", version, url, dossier)
	if *modeServeur && hote != "127.0.0.1" {
		log.Print("  attention : -serveur avec -reseau écoute sans HTTPS ; placez un reverse proxy devant")
	}
	if hote != "127.0.0.1" {
		for _, a := range serveur.Adresses(port) {
			log.Printf("  réseau local : %s", a)
		}
	}

	if *headless || !ouvrirFenetre(url) {
		if !*headless {
			ouvrirNavigateur(url)
		}
		fmt.Printf("COF DM %s : %s (données : %s)\n", version, url, dossier)
		arret := make(chan os.Signal, 1)
		signal.Notify(arret, os.Interrupt, syscall.SIGTERM)
		<-arret // l'univers est sauvegardé et fermé avant de quitter
	}
	_ = srv.Close()
}

// premierAdmin crée le compte « admin » d'un serveur qui n'a encore aucun compte. Son mot de passe,
// tiré au hasard, n'est affiché qu'ici (et pas dans le fichier journal) : il est à changer aussitôt.
func premierAdmin(reg *comptes.Registre) error {
	if !reg.Vide() {
		return nil
	}
	b := make([]byte, 15)
	if _, err := rand.Read(b); err != nil {
		return err
	}
	mdp := base64.RawURLEncoding.EncodeToString(b)
	if _, err := reg.Creer("admin", mdp, true); err != nil {
		return err
	}
	fmt.Printf("\nPremier lancement du serveur : compte administrateur créé.\n  nom d'utilisateur : admin\n  mot de passe      : %s\nNotez-le et changez-le après la première connexion : il ne sera plus affiché.\n\n", mdp)
	return nil
}

func fatal(format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	log.Print(msg)
	fmt.Fprintln(os.Stderr, msg)
	alerte(msg)
	os.Exit(1)
}
