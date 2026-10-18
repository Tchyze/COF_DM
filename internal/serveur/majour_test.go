package serveur

import (
	"testing"
	"time"

	"github.com/Tchyze/cof_dm/internal/majour"
)

// Les mises à jour du bureau : canal rangé dans les réglages, recherche dans ce canal, installation
// sur demande seulement, puis relance ; rien sur un serveur, rien depuis le réseau.
func TestMiseAJour(t *testing.T) {
	var canaux []string
	var installee *majour.Disponible
	trouvee := &majour.Disponible{Version: "9.9.9", Notes: "Nouveautés."}
	chercher, installer := chercherMaj, installerMaj
	t.Cleanup(func() { chercherMaj, installerMaj = chercher, installer })
	chercherMaj = func(canal, actuelle string) (*majour.Disponible, error) {
		canaux = append(canaux, canal)
		if canal == majour.Test {
			return trouvee, nil
		}
		return nil, nil
	}
	installerMaj = func(d *majour.Disponible) (string, error) { installee = d; return "/chemin/COF-DM.exe", nil }

	b := nouvelleAppli(t, t.TempDir())
	relance := make(chan string, 1)
	b.a.opt.Relancer = func(p string) { relance <- p }

	if c, v := b.local("GET", "/api/univers/mise-a-jour", nil); c != 200 || v["canal"] != majour.Stable || v["disponible"] != nil {
		t.Fatalf("état : %d %v", c, v)
	}
	if c, _ := b.appel(true, "POST", "/api/univers/mise-a-jour/chercher", nil); c != 403 {
		t.Fatalf("depuis le réseau : %d", c)
	}
	if c, _ := b.local("POST", "/api/univers/mise-a-jour/installer", nil); c != 409 {
		t.Fatalf("installer sans mise à jour : %d", c)
	}
	if c, _ := b.local("PUT", "/api/univers/mise-a-jour/canal", map[string]any{"canal": "nocturne"}); c != 422 {
		t.Fatalf("canal inconnu : %d", c)
	}
	c, v := b.local("PUT", "/api/univers/mise-a-jour/canal", map[string]any{"canal": majour.Test})
	if c != 200 || v["canal"] != majour.Test || v["disponible"] == nil || canaux[len(canaux)-1] != majour.Test {
		t.Fatalf("canal test : %d %v (%v)", c, v, canaux)
	}
	if r, _ := b.a.reg.Meta(regCanal); r != majour.Test {
		t.Fatalf("canal enregistré : %q", r)
	}
	if installee != nil {
		t.Fatal("installée sans clic")
	}
	if c, v := b.local("POST", "/api/univers/mise-a-jour/installer", nil); c != 200 || v["installee"] != "9.9.9" || installee != trouvee {
		t.Fatalf("installation : %d %v", c, v)
	}
	select {
	case p := <-relance:
		if p != "/chemin/COF-DM.exe" {
			t.Fatalf("relance : %q", p)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("pas de relance")
	}
	// retour au canal stable : plus rien à proposer
	if _, v := b.local("PUT", "/api/univers/mise-a-jour/canal", map[string]any{"canal": majour.Stable}); v["disponible"] != nil {
		t.Fatalf("retour au stable : %v", v)
	}

	// un serveur se met à jour par docker compose
	_, _, connecter := serveurMulti(t)
	chef := connecter("chef")
	if c, _ := chef.faire("GET", "/api/univers/mise-a-jour", nil); c != 404 {
		t.Fatalf("sur un serveur : %d", c)
	}
}
