package serveur

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Brancher Claude : l'entrée « cof-dm » est écrite dans la configuration de Claude Desktop et de
// Claude Code, sans toucher au reste du fichier, mise à jour, puis retirée. Un fichier illisible
// n'est pas modifié ; un client absent répond 404 ; rien ne se règle depuis le réseau.
func TestClaude(t *testing.T) {
	dir := t.TempDir()
	desktop := filepath.Join(dir, "Claude", "claude_desktop_config.json")
	code := filepath.Join(dir, ".claude.json")
	os.MkdirAll(filepath.Dir(desktop), 0o700)
	os.WriteFile(desktop, []byte("\xef\xbb\xbf"+`{"mcpServers": {"autre": {"command": "x"}}, "theme": "sombre"}`), 0o600)
	codePresent := false
	ancien := ciblesClaude
	ciblesClaude = func() []cibleClaude {
		return []cibleClaude{{Client: "desktop", Nom: "Claude Desktop", Chemin: desktop, Present: true},
			{Client: "code", Nom: "Claude Code", Chemin: code, Present: codePresent}}
	}
	t.Cleanup(func() { ciblesClaude = ancien })
	b := nouveauAvec(t, Options{Port: 8100, Reseau: true, Programme: `C:\Jeux\COF DM\COF-DM.exe`})

	c, v := b.local("GET", "/api/claude", nil)
	if c != 200 || !strings.Contains(v["commande_code"].(string), `"C:\Jeux\COF DM\COF-DM.exe" -mcp`) {
		t.Fatalf("état : %d %v", c, v)
	}
	if c, v := b.local("POST", "/api/claude", map[string]any{"client": "desktop", "actif": true}); c != 200 {
		t.Fatalf("branchement : %d %v", c, v)
	}
	var cfg map[string]any
	lire := func(chemin string) {
		t.Helper()
		brut, _ := os.ReadFile(chemin)
		cfg = nil
		if err := json.Unmarshal(brut, &cfg); err != nil {
			t.Fatalf("%s : %v", chemin, err)
		}
	}
	lire(desktop)
	srv := cfg["mcpServers"].(map[string]any)
	if cfg["theme"] != "sombre" || srv["autre"] == nil || srv["cof-dm"].(map[string]any)["command"] != `C:\Jeux\COF DM\COF-DM.exe` {
		t.Fatalf("configuration écrite : %v", cfg)
	}
	if _, err := os.Stat(desktop + ".avant-cof-dm"); err != nil {
		t.Fatal("pas de copie du fichier d'avant")
	}
	_, v = b.local("GET", "/api/claude", nil)
	if e := v["cibles"].([]any)[0].(map[string]any); e["branche"] != true || e["a_jour"] != true {
		t.Fatalf("état après branchement : %v", e)
	}
	// programme déplacé : l'entrée n'est plus à jour
	b.s.opt.Programme = `D:\COF-DM.exe`
	_, v = b.local("GET", "/api/claude", nil)
	if e := v["cibles"].([]any)[0].(map[string]any); e["branche"] != true || e["a_jour"] != false {
		t.Fatalf("programme déplacé : %v", e)
	}
	// Claude Code absent, puis présent (fichier créé)
	if c, _ := b.local("POST", "/api/claude", map[string]any{"client": "code", "actif": true}); c != 404 {
		t.Fatalf("Claude Code absent : %d", c)
	}
	codePresent = true
	if c, _ := b.local("POST", "/api/claude", map[string]any{"client": "code", "actif": true}); c != 200 {
		t.Fatalf("Claude Code : %d", c)
	}
	lire(code)
	if e := cfg["mcpServers"].(map[string]any)["cof-dm"].(map[string]any); e["type"] != "stdio" || e["command"] != `D:\COF-DM.exe` {
		t.Fatalf("Claude Code écrit : %v", e)
	}
	// retrait : l'autre serveur reste
	if c, _ := b.local("POST", "/api/claude", map[string]any{"client": "desktop", "actif": false}); c != 200 {
		t.Fatalf("retrait : %d", c)
	}
	lire(desktop)
	if srv := cfg["mcpServers"].(map[string]any); srv["cof-dm"] != nil || srv["autre"] == nil {
		t.Fatalf("après retrait : %v", cfg)
	}
	// fichier illisible : laissé tel quel
	os.WriteFile(desktop, []byte(`{pas du json`), 0o600)
	if c, _ := b.local("POST", "/api/claude", map[string]any{"client": "desktop", "actif": true}); c != 422 {
		t.Fatalf("fichier illisible : %d", c)
	}
	if brut, _ := os.ReadFile(desktop); string(brut) != `{pas du json` {
		t.Fatalf("fichier illisible modifié : %s", brut)
	}
	// depuis le réseau, même avec le jeton : non
	for _, m := range []string{"GET", "POST"} {
		if c, _ := b.api(m, "/api/claude", map[string]any{"client": "desktop", "actif": true}); c != 403 {
			t.Fatalf("%s depuis le réseau : %d", m, c)
		}
	}
	if c, _ := b.local("POST", "/api/claude", map[string]any{"client": "chatgpt", "actif": true}); c != 422 {
		t.Fatalf("client inconnu : %d", c)
	}
}

// Sans programme connu (et sur un serveur), pas de panneau Claude.
func TestClaudeAbsent(t *testing.T) {
	if c, _ := nouveau(t).local("GET", "/api/claude", nil); c == 200 {
		t.Fatal("route Claude sans programme")
	}
	b, _ := bancServeur(t)
	if c, _, _ := b.proxy("GET", "/api/claude", nil, b.connecter("meneur")); c == 200 {
		t.Fatal("route Claude sur un serveur")
	}
}
