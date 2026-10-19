package main

import (
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/Tchyze/cof_dm/internal/mcp"
	"github.com/Tchyze/cof_dm/internal/reglages"
	"github.com/Tchyze/cof_dm/internal/synchro"
)

// lancerMCP : cof-dm -mcp. Le serveur MCP parle sur l'entrée et la sortie standard (le client MCP,
// Claude Desktop ou Claude Code, le lance) et joint l'application par son API. Sans réglage, il vise
// l'application de bureau de cet ordinateur et lit son jeton dans son dossier de données ;
// COF_DM_URL, COF_DM_JETON et COF_DM_UNIVERS visent un serveur (jeton d'un compte, univers).
func lancerMCP(dossier string, port int) {
	log.SetOutput(os.Stderr) // la sortie standard est réservée au protocole
	log.SetPrefix("cof-dm mcp : ")
	if dossier == "" {
		dossier = dossierParDefaut()
	}
	base := strings.TrimSpace(os.Getenv("COF_DM_URL"))
	if base == "" {
		if port == 0 {
			port = portApplication(dossier) // le port pris au lancement, pas forcément celui de config.json
		}
		base = fmt.Sprintf("http://127.0.0.1:%d", port)
	}
	base, err := synchro.Adresse(base)
	if err != nil {
		arretMCP("COF_DM_URL : %v", err)
	}
	jeton := strings.TrimSpace(os.Getenv("COF_DM_JETON"))
	if jeton == "" && boucleLocale(base) {
		// le jeton de l'application de cet ordinateur ; jamais envoyé à une autre machine
		jeton, _ = reglages.Ouvrir(filepath.Join(dossier, "reglages.json")).Meta("api_jeton")
	}
	if jeton == "" {
		arretMCP("aucun jeton d'API : donnez-le dans COF_DM_JETON (Paramètres → API, ou Mon compte sur un serveur)")
	}
	api := mcp.NouveauClient(base, jeton, strings.TrimSpace(os.Getenv("COF_DM_UNIVERS")))
	log.Printf("COF DM %s, API %s", version, base)
	if err := mcp.Nouveau(api, version).Servir(os.Stdin, os.Stdout); err != nil {
		arretMCP("%v", err)
	}
}

func boucleLocale(base string) bool {
	u, err := url.Parse(base)
	if err != nil {
		return false
	}
	if u.Hostname() == "localhost" {
		return true
	}
	ip := net.ParseIP(u.Hostname())
	return ip != nil && ip.IsLoopback()
}

func arretMCP(format string, a ...any) {
	log.Printf(format, a...)
	os.Exit(1)
}
