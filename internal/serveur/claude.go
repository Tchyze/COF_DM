package serveur

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Tchyze/cof_dm/internal/store"
)

// Brancher Claude (Paramètres → Claude) : l'application inscrit son serveur MCP (ce programme lancé
// avec -mcp) dans la configuration de Claude Desktop ou de Claude Code de cet ordinateur, pour que
// le MJ n'ait rien à taper. Seule l'entrée « cof-dm » de mcpServers est écrite ou retirée ; le reste
// du fichier est gardé, et une copie du fichier d'avant est faite la première fois. Application de
// bureau seulement, sur l'ordinateur du MJ : ces routes touchent à des fichiers de l'ordinateur.

const nomMCP = "cof-dm"

// cibleClaude : la configuration d'un client MCP de cet ordinateur.
type cibleClaude struct {
	Client  string `json:"client"` // "desktop" ou "code"
	Nom     string `json:"nom"`
	Chemin  string `json:"chemin"`
	Present bool   `json:"present"` // le client semble installé (son dossier existe)
	Branche bool   `json:"branche"` // l'entrée cof-dm existe
	AJour   bool   `json:"a_jour"`  // … et lance ce programme avec ces arguments
	Erreur  string `json:"erreur,omitempty"`
}

// ciblesClaude : les fichiers de configuration où inscrire COF DM (remplacé dans les tests).
var ciblesClaude = func() []cibleClaude {
	var out []cibleClaude
	dossier := func(c cibleClaude) cibleClaude {
		if fi, err := os.Stat(filepath.Dir(c.Chemin)); err == nil && fi.IsDir() {
			c.Present = true
		}
		return c
	}
	desktop := func(d string) {
		out = append(out, dossier(cibleClaude{Client: "desktop", Nom: "Claude Desktop", Chemin: filepath.Join(d, "claude_desktop_config.json")}))
	}
	maison, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "windows":
		desktop(filepath.Join(os.Getenv("APPDATA"), "Claude"))
		// version du Microsoft Store : un dossier à part
		l, _ := filepath.Glob(filepath.Join(os.Getenv("LOCALAPPDATA"), "Packages", "Claude_*", "LocalCache", "Roaming", "Claude"))
		for _, d := range l {
			desktop(d)
		}
	case "darwin":
		desktop(filepath.Join(maison, "Library", "Application Support", "Claude"))
	default:
		cfg := os.Getenv("XDG_CONFIG_HOME")
		if cfg == "" {
			cfg = filepath.Join(maison, ".config")
		}
		desktop(filepath.Join(cfg, "Claude"))
	}
	code := cibleClaude{Client: "code", Nom: "Claude Code", Chemin: filepath.Join(maison, ".claude.json")}
	if _, err := os.Stat(code.Chemin); err == nil {
		code.Present = true
	} else if fi, err := os.Stat(filepath.Join(maison, ".claude")); err == nil && fi.IsDir() {
		code.Present = true
	}
	return append(out, code)
}

func (s *Serveur) routesClaude() {
	if s.acces.comptes != nil || s.opt.Programme == "" {
		return // serveur, ou programme inconnu : rien à inscrire
	}
	s.mux.HandleFunc("GET /api/claude", s.lireClaude)
	s.mux.HandleFunc("POST /api/claude", s.reglerClaude)
}

// entreeMCP : ce que le client lance.
func (s *Serveur) entreeMCP(client string) map[string]any {
	args := append([]string{"-mcp"}, s.opt.ArgsMCP...)
	e := map[string]any{"command": s.opt.Programme, "args": args}
	if client == "code" {
		e["type"] = "stdio"
	}
	return e
}

// lireConfigClaude lit un fichier de configuration ; absent : vide.
func lireConfigClaude(chemin string) (map[string]any, error) {
	b, err := os.ReadFile(chemin)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	b = bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))
	if len(bytes.TrimSpace(b)) == 0 {
		return map[string]any{}, nil
	}
	var c map[string]any
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, store.ErrValidation{Msg: "Le fichier " + chemin + " n'est pas du JSON valide : corrigez-le ou retirez-le, il n'a pas été modifié."}
	}
	if c == nil {
		c = map[string]any{}
	}
	return c, nil
}

func (s *Serveur) etatClaude() []cibleClaude {
	out := ciblesClaude()
	for i := range out {
		c := &out[i]
		cfg, err := lireConfigClaude(c.Chemin)
		if err != nil {
			c.Erreur = err.Error()
			continue
		}
		serveurs, _ := cfg["mcpServers"].(map[string]any)
		e, ok := serveurs[nomMCP].(map[string]any)
		c.Branche = ok
		if ok {
			attendu, _ := json.Marshal(s.entreeMCP(c.Client)["args"])
			lu, _ := json.Marshal(e["args"])
			c.AJour = e["command"] == s.opt.Programme && bytes.Equal(attendu, lu)
		}
	}
	return out
}

// lireClaude : où en est l'inscription de COF DM dans Claude.
func (s *Serveur) lireClaude(w http.ResponseWriter, r *http.Request) {
	if !surPlace(w, r) {
		return
	}
	repondre(w, 200, map[string]any{"programme": s.opt.Programme, "args": s.entreeMCP("desktop")["args"],
		"cibles": s.etatClaude(), "commande_code": commandeCode(s.opt.Programme, s.opt.ArgsMCP)})
}

// commandeCode : la même inscription, à taper dans un terminal pour Claude Code.
func commandeCode(programme string, args []string) string {
	q := func(x string) string {
		if strings.ContainsAny(x, " \t\"'") {
			return `"` + strings.ReplaceAll(x, `"`, `\"`) + `"`
		}
		return x
	}
	l := []string{"claude", "mcp", "add", "--scope", "user", nomMCP, "--", q(programme), "-mcp"}
	for _, a := range args {
		l = append(l, q(a))
	}
	return strings.Join(l, " ")
}

// reglerClaude : {"client": "desktop" | "code", "actif": true} inscrit COF DM (ou met son chemin à
// jour) dans chaque configuration de ce client présente sur l'ordinateur ; false l'en retire.
func (s *Serveur) reglerClaude(w http.ResponseWriter, r *http.Request) {
	if !surPlace(w, r) {
		return
	}
	var in struct {
		Client string `json:"client"`
		Actif  bool   `json:"actif"`
	}
	if err := lireJSON(r, &in); err != nil {
		erreurStore(w, err)
		return
	}
	if in.Client != "desktop" && in.Client != "code" {
		erreurStore(w, store.ErrValidation{Msg: "Client inconnu : « desktop » ou « code »."})
		return
	}
	n := 0
	for _, c := range ciblesClaude() {
		if c.Client != in.Client || !c.Present {
			continue
		}
		if err := s.ecrireConfigClaude(c, in.Actif); err != nil {
			erreurStore(w, err)
			return
		}
		n++
	}
	if n == 0 {
		nom := map[string]string{"desktop": "Claude Desktop", "code": "Claude Code"}[in.Client]
		erreur(w, 404, nom+" n'est pas installé sur cet ordinateur (son dossier de configuration est introuvable).")
		return
	}
	detail := "COF DM retiré de "
	if in.Actif {
		detail = "COF DM branché sur "
	}
	s.store.Noter("claude", detail+map[string]string{"desktop": "Claude Desktop", "code": "Claude Code"}[in.Client], auteurDe(r))
	repondre(w, 200, map[string]any{"cibles": s.etatClaude()})
}

func (s *Serveur) ecrireConfigClaude(c cibleClaude, actif bool) error {
	cfg, err := lireConfigClaude(c.Chemin)
	if err != nil {
		return err
	}
	serveurs, ok := cfg["mcpServers"].(map[string]any)
	if !ok {
		if cfg["mcpServers"] != nil {
			return store.ErrValidation{Msg: "Dans " + c.Chemin + ", « mcpServers » n'est pas un objet : fichier laissé tel quel."}
		}
		serveurs = map[string]any{}
	}
	if actif {
		serveurs[nomMCP] = s.entreeMCP(c.Client)
	} else {
		if _, ok := serveurs[nomMCP]; !ok {
			return nil
		}
		delete(serveurs, nomMCP)
	}
	cfg["mcpServers"] = serveurs
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if avant, err := os.ReadFile(c.Chemin); err == nil {
		copie := c.Chemin + ".avant-cof-dm"
		if _, err := os.Stat(copie); errors.Is(err, os.ErrNotExist) {
			_ = os.WriteFile(copie, avant, 0o600)
		}
	}
	// écrit à côté puis remplace : jamais de fichier à moitié écrit
	tmp := c.Chemin + ".cof-dm.tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, c.Chemin); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
