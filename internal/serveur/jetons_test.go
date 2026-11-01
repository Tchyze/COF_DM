package serveur

import (
	"net/http"
	"testing"
)

func jetonAccepte(b *banc, jeton string) int {
	b.t.Helper()
	c, _, _ := b.proxy("GET", "/api/compte", nil, func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+jeton) })
	return c
}

// Une réinitialisation par l'administrateur retire tous les jetons du compte.
func TestJetonsRetiresParAdministrateur(t *testing.T) {
	a, reg, connecter := serveurMulti(t)
	_ = a
	if _, err := reg.Creer("cible", "mot-de-passe-elbic", false); err != nil {
		t.Fatal(err)
	}
	clair, _, err := reg.NouveauJeton("cible", "poste")
	if err != nil {
		t.Fatal(err)
	}
	chef := connecter("chef")
	if c, v := chef.faire("PATCH", "/api/admin/comptes/cible", map[string]any{"mdp": "un-nouveau-mot-de-passe"}); c != 200 || v["jetons"] != float64(0) {
		t.Fatalf("réinitialisation : %d %v", c, v)
	}
	if _, ok := reg.ParJeton(clair); ok {
		t.Fatal("le jeton survit à la réinitialisation")
	}
}

// Un autre changement du compte par l'administrateur ne touche pas aux jetons.
func TestJetonsGardesSansMdp(t *testing.T) {
	_, reg, connecter := serveurMulti(t)
	reg.Creer("cible", "mot-de-passe-elbic", false)
	clair, _, _ := reg.NouveauJeton("cible", "poste")
	if c, v := connecter("chef").faire("PATCH", "/api/admin/comptes/cible", map[string]any{"inactif": false}); c != 200 {
		t.Fatalf("%d %v", c, v)
	}
	if _, ok := reg.ParJeton(clair); !ok {
		t.Fatal("jeton retiré sans changement de mot de passe")
	}
}

// Le titulaire choisit : avec retirer_jetons, ses jetons tombent et le nombre retiré est rendu ;
// sans, ils restent.
func TestJetonsChangementParTitulaire(t *testing.T) {
	b, reg := bancServeur(t)
	mj := b.connecter("meneur")
	garde, _, _ := reg.NouveauJeton("meneur", "garde")
	if c, v, _ := b.proxy("POST", "/api/compte/mdp", map[string]any{"ancien": "mot-de-passe-ruenem", "nouveau": "deuxieme-mot-de-passe"}, mj); c != 200 || v["jetons_retires"] != float64(0) {
		t.Fatalf("sans retrait : %d %v", c, v)
	}
	if c := jetonAccepte(b, garde); c != 200 {
		t.Fatalf("jeton gardé : %d", c)
	}
	mj = b.connecter2("meneur", "deuxieme-mot-de-passe")
	if _, _, err := reg.NouveauJeton("meneur", "autre"); err != nil {
		t.Fatal(err)
	}
	if c, v, _ := b.proxy("POST", "/api/compte/mdp", map[string]any{"ancien": "deuxieme-mot-de-passe", "nouveau": "troisieme-mot-de-passe", "retirer_jetons": true}, mj); c != 200 || v["jetons_retires"] != float64(2) {
		t.Fatalf("avec retrait : %d %v", c, v)
	}
	if c := jetonAccepte(b, garde); c != 401 {
		t.Fatalf("jeton retiré : %d", c)
	}
	// un mauvais ancien mot de passe ne retire rien
	mj = b.connecter2("meneur", "troisieme-mot-de-passe")
	reste, _, _ := reg.NouveauJeton("meneur", "reste")
	if c, _, _ := b.proxy("POST", "/api/compte/mdp", map[string]any{"ancien": "faux", "nouveau": "quatrieme-mot-de-passe", "retirer_jetons": true}, mj); c != 401 {
		t.Fatalf("mauvais mot de passe : %d", c)
	}
	if c := jetonAccepte(b, reste); c != 200 {
		t.Fatalf("jeton après un essai raté : %d", c)
	}
	// un nouveau mot de passe trop court ne retire rien non plus
	if c, _, _ := b.proxy("POST", "/api/compte/mdp", map[string]any{"ancien": "troisieme-mot-de-passe", "nouveau": "court", "retirer_jetons": true}, mj); c != 422 {
		t.Fatalf("mot de passe trop court : %d", c)
	}
	if c := jetonAccepte(b, reste); c != 200 {
		t.Fatalf("jeton après un refus : %d", c)
	}
}

// La date de dernière utilisation se lit dans le compte.
func TestJetonsDerniereUtilisation(t *testing.T) {
	b, reg := bancServeur(t)
	clair, _, _ := reg.NouveauJeton("meneur", "poste")
	mj := b.connecter("meneur")
	_, v, _ := b.proxy("GET", "/api/compte", nil, mj)
	if j := v["jetons"].([]any)[0].(map[string]any); j["utilise"] != nil {
		t.Fatalf("jamais utilisé : %v", j)
	}
	jetonAccepte(b, clair)
	_, v, _ = b.proxy("GET", "/api/compte", nil, mj)
	if j := v["jetons"].([]any)[0].(map[string]any); j["utilise"] == nil {
		t.Fatalf("utilisé : %v", j)
	}
}
