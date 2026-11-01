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
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Tchyze/cof_dm/internal/comptes"
	"github.com/Tchyze/cof_dm/internal/installation"
	"github.com/Tchyze/cof_dm/internal/majour"
	"github.com/Tchyze/cof_dm/internal/reglages"
	"github.com/Tchyze/cof_dm/internal/serveur"
	"github.com/Tchyze/cof_dm/internal/synchro"
)

var version = "dev"

//go:embed frontend
var frontend embed.FS

type config struct {
	Port   int  `json:"port"`
	Reseau bool `json:"reseau,omitempty"` // écouter sur le réseau local (tablette du MJ)
}

// dossierParDefaut : le dossier des données de l'utilisateur. L'ancien mode portable (un dossier
// « COF DM - données » à côté du programme) n'existe plus : reprendrePortable y range ses données.
func dossierParDefaut() string {
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

// programmeCourant : le chemin de ce programme, tel qu'un autre programme le relance (l'AppImage
// plutôt que son point de montage temporaire).
func programmeCourant() string {
	if p := os.Getenv("APPIMAGE"); p != "" {
		return p
	}
	p, err := os.Executable()
	if err != nil {
		return ""
	}
	if r, err := filepath.EvalSymlinks(p); err == nil {
		p = r
	}
	return p
}

// envRelance : présent quand l'application vient d'être relancée par une mise à jour.
const envRelance = "COF_DM_RELANCE"

// envAvertir : un mot pour le MJ, transmis au programme installé qui s'ouvre à la place de celui-ci.
const envAvertir = "COF_DM_AVERTIR"

// reprendrePortable : le mode portable n'existe plus ; un dossier « COF DM - données » posé à côté
// du programme (ou de l'AppImage) passe dans le dossier de l'utilisateur, sans rien perdre. Renvoie
// un mot pour le MJ, s'il y a lieu.
func reprendrePortable() string {
	portable := installation.PortableDe(programmeCourant())
	if portable == "" {
		return ""
	}
	r, err := installation.ReprendrePortable(portable, dossierParDefaut())
	if err != nil {
		log.Printf("mode portable : %v", err)
		return fmt.Sprintf("Les données de « %s » n'ont pas pu rejoindre votre dossier de données (%v) : elles y sont toujours. Fermez ce qui les utilise, puis relancez COF DM.", portable, err)
	}
	log.Printf("mode portable : données de %s reprises dans %s", r.Depuis, r.Vers)
	return r.Avertissement
}

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

// ecoute : la porte de l'application, que le MJ ouvre au réseau local et referme sans relance.
// Passer de 127.0.0.1 à toutes les adresses demande de fermer l'une pour ouvrir l'autre (Linux
// refuse les deux à la fois) ; les connexions déjà établies, dont celle de la fenêtre, n'en souffrent
// pas. Celles du réseau sont suivies, WebSocket compris, pour être coupées quand le MJ referme.
type ecoute struct {
	srv    *http.Server
	port   int
	mu     sync.Mutex
	ln     net.Listener // nil : ni l'ouverture ni le retour n'ont pu reprendre le port
	ouvert bool
	reseau map[*connReseau]struct{}
}

func nouvelleEcoute(ln net.Listener, port int, ouvert bool) *ecoute {
	return &ecoute{ln: ln, port: port, ouvert: ouvert, reseau: map[*connReseau]struct{}{}}
}

func hoteEcoute(ouvert bool) string {
	if ouvert {
		return "0.0.0.0"
	}
	return "127.0.0.1"
}

func (e *ecoute) estOuvert() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.ouvert
}

// servir sert ln jusqu'à ce qu'il soit fermé (bascule ou fin de l'application).
func (e *ecoute) servir(ln net.Listener) {
	go func() {
		err := e.srv.Serve(&ecouteSuivie{ln, e})
		if err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
			log.Printf("serveur arrêté : %v", err)
		}
	}()
}

// changer ouvre ou referme aussitôt. Si le port ne peut pas s'ouvrir au réseau, l'application reste
// comme avant et le dit ; refermer coupe tous les appareils du réseau.
func (e *ecoute) changer(ouvrir bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if ouvrir == e.ouvert && e.ln != nil {
		return nil
	}
	if e.ln != nil {
		e.ln.Close()
		e.ln = nil
	}
	if !ouvrir {
		for c := range e.reseau {
			c.Conn.Close()
		}
		clear(e.reseau)
		e.ouvert = false
	}
	ln, _, err := ecouter(hoteEcoute(ouvrir), e.port, 1)
	if err != nil && ouvrir {
		log.Printf("ouverture au réseau impossible : %v", err)
		ln, err = e.reprendre(e.ouvert)
		if err == nil {
			err = fmt.Errorf("le port %d est déjà pris sur cet ordinateur : fermez l'autre programme, puis réessayez", e.port)
		}
		e.ln = ln
		if ln != nil {
			e.servir(ln)
		}
		return err
	}
	if err != nil {
		ln, err = e.reprendre(false)
	}
	if err != nil {
		return fmt.Errorf("le port %d n'a pas pu être repris (%v)", e.port, err)
	}
	e.ln, e.ouvert = ln, ouvrir
	e.servir(ln)
	return nil
}

// reprendre : le port vient d'être lâché, un autre programme a pu s'y glisser un instant.
func (e *ecoute) reprendre(ouvert bool) (net.Listener, error) {
	var err error
	for range 10 {
		var ln net.Listener
		if ln, _, err = ecouter(hoteEcoute(ouvert), e.port, 1); err == nil {
			return ln, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil, err
}

// ecouteSuivie repère les connexions du réseau à leur arrivée.
type ecouteSuivie struct {
	net.Listener
	e *ecoute
}

func (l *ecouteSuivie) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	a, ok := c.RemoteAddr().(*net.TCPAddr)
	if ok && a.IP.IsLoopback() {
		return c, nil
	}
	s := &connReseau{c, l.e}
	l.e.mu.Lock()
	defer l.e.mu.Unlock()
	if !l.e.ouvert { // arrivée pendant que le MJ refermait
		c.Close()
		return nil, net.ErrClosed
	}
	l.e.reseau[s] = struct{}{}
	return s, nil
}

// connReseau : une connexion d'un appareil du réseau. Le serveur HTTP et les WebSocket la ferment
// par cette méthode, ce qui la retire du suivi.
type connReseau struct {
	net.Conn
	e *ecoute
}

func (c *connReseau) Close() error {
	c.e.mu.Lock()
	delete(c.e.reseau, c)
	c.e.mu.Unlock()
	return c.Conn.Close()
}

func main() {
	headless := flag.Bool("headless", false, "serveur seul, sans fenêtre (l'interface s'ouvre dans le navigateur)")
	portFlag := flag.Int("port", 0, "port d'écoute (par défaut : config.json, sinon 8100)")
	dataFlag := flag.String("data", "", "dossier de l'application (réglages, univers par défaut, sauvegardes)")
	universFlag := flag.String("univers", "", "fichier d'univers (.cofdm) à ouvrir ; créé s'il n'existe pas")
	reseau := flag.Bool("reseau", false, "écouter sur le réseau local (API avec jeton, interface avec le mot de passe MJ)")
	modeServeur := flag.Bool("serveur", editionServeur, "mode serveur, derrière un reverse proxy : sans fenêtre, accès par comptes (édition serveur, image Docker)")
	importsFlag := flag.Int("imports-simultanes", serveur.ImportsSimultanesParDefaut, "mode serveur : traitements lourds simultanés sur tout le serveur (imports, exports, sauvegardes demandées)")
	quotaUniversFlag := flag.Int64("quota-univers", serveur.QuotaUniversParDefaut>>20, "mode serveur : taille d'un univers au-delà de laquelle il ne reçoit plus d'image ni d'import, en Mo (0 : aucune)")
	quotaSauvegardesFlag := flag.Int64("quota-sauvegardes", serveur.QuotaSauvegardesParDefaut>>20, "mode serveur : volume des sauvegardes de chaque univers, en Mo (0 : aucun)")
	sauvegardesMaxFlag := flag.Int("sauvegardes-max", serveur.SauvegardesMaxParDefaut, "mode serveur : sauvegardes gardées par univers, demandées d'une part, d'avant une opération de l'autre (0 : pas de plafond)")
	proxiesFlag := flag.String("proxies", "", "mode serveur : réseaux des proxys de confiance, en CIDR séparés par des virgules (ou COF_DM_PROXIES ; par défaut "+serveur.ProxiesParDefaut+")")
	desinstallerFlag := flag.Bool("desinstaller", false, "désinstaller l'application (Windows, lancé par « Applications installées »)")
	modeMCP := flag.Bool("mcp", false, "serveur MCP pour Claude, sur l'entrée et la sortie standard : joint l'application lancée par son API (COF_DM_URL, COF_DM_JETON, COF_DM_UNIVERS)")
	flag.Parse()
	if *desinstallerFlag {
		desinstaller()
		return
	}
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

	// application de bureau : reprise de l'ancien mode portable, puis installation dans le dossier de
	// l'utilisateur (Windows) ; le programme installé est alors lancé à la place de celui-ci
	var avertissements []string
	if m := os.Getenv(envAvertir); m != "" {
		avertissements = append(avertissements, m)
		os.Unsetenv(envAvertir)
	}
	if !*modeServeur && *dataFlag == "" && !*headless {
		if m := reprendrePortable(); m != "" {
			avertissements = append(avertissements, m)
		}
		sortir, m := installerAuLancement(strings.Join(avertissements, " "))
		if sortir {
			return
		}
		if m != "" {
			avertissements = append(avertissements, m)
		}
	}

	dossier := *dataFlag
	if dossier == "" {
		dossier = dossierParDefaut()
	}
	if err := os.MkdirAll(dossier, 0o700); err != nil {
		fatal("Impossible de créer le dossier des données %s : %v", dossier, err)
	}
	resserrer(dossier)
	if !*modeServeur && !*headless {
		rangerCacheFenetre(dossier)
	}
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
	porte := nouvelleEcoute(ln, port, hote != "127.0.0.1")

	sub, _ := fs.Sub(frontend, "frontend")
	synchro.VersionApplication = version
	opt := serveur.Options{Version: version, DossierDon: dossier, Reglages: reg, Frontend: sub, Adresse: fmt.Sprintf("%s:%d", hote, port),
		Port: port, Reseau: hote != "127.0.0.1", ReseauForce: *reseau && !relie, ReseauVoulu: cfg.Reseau,
		ReseauOuvert: porte.estOuvert, ChangerReseau: porte.changer}
	if dialogueDisponible() {
		opt.Dialogue = dialogueFichier
	}
	opt.Config = func(ouvrir bool) error {
		c := lireConfig(dossier) // le port enregistré, pas celui de -port
		c.Reseau = ouvrir
		return ecrireConfig(dossier, c)
	}
	if !*modeServeur {
		opt.Programme = programmeCourant()
		if *dataFlag != "" { // le dossier par défaut (ou portable) se retrouve seul
			if abs, err := filepath.Abs(dossier); err == nil {
				opt.ArgsMCP = []string{"-data", abs}
			}
		}
	}
	if *modeServeur {
		opt.Dialogue, opt.Config, opt.VeilleMaj = nil, nil, true
		opt.ChangerReseau = nil // un serveur reste derrière son reverse proxy
		// un conteneur en lecture seule range ses fichiers temporaires dans le volume (TMPDIR)
		if err := os.MkdirAll(os.TempDir(), 0o700); err != nil {
			log.Printf("dossier temporaire %s : %v", os.TempDir(), err)
		}
		if opt.Proxies, err = proxiesDemandes(*proxiesFlag); err != nil {
			fatal("Option -proxies : %v", err)
		}
		opt.Limites = serveur.NouvellesLimites(*importsFlag, *quotaUniversFlag<<20, *quotaSauvegardesFlag<<20, *sauvegardesMaxFlag)
		if opt.Comptes, err = comptes.Ouvrir(filepath.Join(dossier, "comptes.json")); err != nil {
			fatal("Impossible de lire les comptes : %v", err)
		}
		if err := premierAdmin(opt.Comptes); err != nil {
			fatal("Impossible de créer le compte administrateur : %v", err)
		}
	}
	var app *serveur.Application
	if !*modeServeur {
		go func() { // le programme d'avant la dernière mise à jour ; il peut tourner encore un instant
			if err := majour.Nettoyer(); err != nil {
				log.Printf("programme d'avant la mise à jour non retiré (il le sera au prochain lancement) : %v", err)
			}
		}()
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
	for _, m := range avertissements {
		log.Print(m)
		app.Avertir(m)
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
	porte.srv = srv
	porte.servir(ln)
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

// proxiesDemandes : la liste de -proxies, sinon de COF_DM_PROXIES, sinon celle par défaut.
func proxiesDemandes(option string) ([]*net.IPNet, error) {
	if option == "" {
		option = os.Getenv("COF_DM_PROXIES")
	}
	if option == "" {
		option = serveur.ProxiesParDefaut
	}
	l, err := serveur.LireProxies(option)
	if err == nil && len(l) == 0 {
		err = fmt.Errorf("aucun réseau dans %q", option)
	}
	return l, err
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
	fmt.Printf("\nPremier lancement du serveur : compte administrateur créé.\n  nom d'utilisateur : admin\n  mot de passe      : %s\nNotez-le et changez-le après la première connexion : il ne sera plus affiché.\nL'administrateur gère le serveur (univers, comptes) sans voir leur contenu : créez-vous un compte MJ pour préparer vos univers.\n\n", mdp)
	return nil
}

func fatal(format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	log.Print(msg)
	fmt.Fprintln(os.Stderr, msg)
	alerte(msg)
	os.Exit(1)
}
