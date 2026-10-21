package serveur

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Tchyze/cof_dm/internal/comptes"
)

// La préparation d'une séance rassemble ce que sa campagne a en cours : la séance d'avant, où est
// le groupe, les quêtes et leurs étapes restantes, les secrets à semer, les PNJ probables et les
// rencontres prêtes.
func TestPreparation(t *testing.T) {
	b := nouveau(t)
	port := b.fiche("lieu", "Port-Brume", nil)
	quai := b.fiche("lieu", "Les quais", map[string]any{"parent": port})
	camp := b.fiche("campagne", "Les Marches", map[string]any{"statut": "en cours", "lieu_groupe": port, "fils_rouges": "Le frère disparu."})
	autre := b.fiche("campagne", "Ailleurs", nil)
	aelis := b.fiche("pj", "Aelis", map[string]any{"campagne": camp, "statut": "actif"})
	b.fiche("pj", "Bran", map[string]any{"campagne": camp, "statut": "actif"})
	b.fiche("pj", "Cael", map[string]any{"campagne": camp, "statut": "absent"})
	maire := b.fiche("pnj", "Le maire", map[string]any{"statut": "vivant"})
	b.fiche("pnj", "Docker", map[string]any{"lieu": quai})
	b.fiche("pnj", "Noyé", map[string]any{"lieu": port, "statut": "mort"})
	b.fiche("pnj", "Loin", map[string]any{"lieu": b.fiche("lieu", "Ailleurs", nil)})
	q := b.fiche("quete", "Le phare", map[string]any{"campagne": camp, "statut": "en cours", "commanditaire": maire, "objectif": "Rallumer le phare.",
		"etapes": []any{map[string]any{"texte": "Trouver la clé", "fait": true}, map[string]any{"texte": "Monter au phare", "fait": false}}})
	b.fiche("quete", "La foire", map[string]any{"campagne": camp, "statut": "à venir"})
	b.fiche("quete", "Finie", map[string]any{"campagne": camp, "statut": "réussie"})
	b.fiche("quete", "Hors campagne", map[string]any{"campagne": autre, "statut": "en cours"})
	capital := b.fiche("secret", "Le maire ment", map[string]any{"campagne": camp, "statut": "soupçonné", "importance": "capital", "indices": "Ses bottes mouillées."})
	b.fiche("secret", "Détail", map[string]any{"campagne": camp, "statut": "caché", "importance": "détail"})
	b.fiche("secret", "Déjà su", map[string]any{"campagne": camp, "statut": "révélé"})
	b.local("POST", fmt.Sprintf("/api/secrets/%v/reveler", capital), map[string]any{"qui": []any{aelis}})
	b.fiche("rencontre", "Sur le quai", map[string]any{"quete": q, "statut": "préparée"})
	b.fiche("rencontre", "Vieille", map[string]any{"campagne": camp, "statut": "jouée"})
	b.fiche("rencontre", "Ailleurs", map[string]any{"campagne": autre})
	s1 := b.fiche("seance", "Séance 1", map[string]any{"campagne": camp, "numero": 1, "statut": "jouée", "resume": "Arrivée au port."})
	b.fiche("seance", "Séance 2", map[string]any{"campagne": camp, "numero": 2, "statut": "jouée", "resume": "La clé trouvée."})
	s3 := b.fiche("seance", "Séance 3", map[string]any{"campagne": camp, "numero": 3, "statut": "prévue"})

	c, v := b.local("GET", fmt.Sprintf("/api/entites/%v/preparation", s3), nil)
	if c != 200 {
		t.Fatalf("préparation : %d %v", c, v)
	}
	nom := func(x any) string { return x.(map[string]any)["fiche"].(map[string]any)["nom"].(string) }
	if v["fils_rouges"] != "Le frère disparu." || v["lieu_groupe"].(map[string]any)["nom"] != "Port-Brume" || nom(v["precedente"]) != "Séance 2" {
		t.Fatalf("campagne : %v", v)
	}
	quetes := v["quetes"].([]any)
	if len(quetes) != 1 || nom(quetes[0]) != "Le phare" {
		t.Fatalf("quêtes en cours : %v", quetes)
	}
	if qq := quetes[0].(map[string]any); fmt.Sprint(qq["etapes_restantes"]) != "[Monter au phare]" || qq["etapes_faites"] != float64(1) || qq["commanditaire"] != "Le maire" {
		t.Fatalf("quête : %v", qq)
	}
	if l := v["quetes_a_venir"].([]any); len(l) != 1 || l[0].(map[string]any)["nom"] != "La foire" {
		t.Fatalf("quêtes à venir : %v", l)
	}
	secrets := v["secrets"].([]any)
	if len(secrets) != 2 || nom(secrets[0]) != "Le maire ment" || nom(secrets[1]) != "Détail" {
		t.Fatalf("secrets : %v", secrets)
	}
	if ig := secrets[0].(map[string]any)["ignorent"]; fmt.Sprint(ig) != "[Bran]" {
		t.Fatalf("qui ignore le secret (sans l'absent) : %v", ig)
	}
	var pnj []string
	for _, x := range v["pnj"].([]any) {
		pnj = append(pnj, nom(x))
	}
	if strings.Join(pnj, ",") != "Le maire,Docker" {
		t.Fatalf("PNJ probables : %v", pnj)
	}
	if l := v["rencontres"].([]any); len(l) != 1 || nom(l[0]) != "Sur le quai" || l[0].(map[string]any)["quete"] != "Le phare" {
		t.Fatalf("rencontres : %v", l)
	}

	// la première séance n'a pas de précédente ; une fiche qui n'est pas une séance : 404
	if _, v := b.local("GET", fmt.Sprintf("/api/entites/%v/preparation", s1), nil); v["precedente"] != nil {
		t.Fatalf("séance 1 : %v", v["precedente"])
	}
	if c, _ := b.local("GET", fmt.Sprintf("/api/entites/%v/preparation", camp), nil); c != 404 {
		t.Fatalf("préparation d'une campagne : %d", c)
	}
	// une séance sans campagne : rien à rassembler, sans erreur
	seule := b.fiche("seance", "Seule", nil)
	if c, v := b.local("GET", fmt.Sprintf("/api/entites/%v/preparation", seule), nil); c != 200 || v["campagne"] != nil {
		t.Fatalf("séance sans campagne : %d %v", c, v)
	}
}

// La préparation est réservée au MJ : un joueur ne la lit pas, ni par cette route, ni dans la fiche
// de la séance (le champ « Préparation » appartient au MJ).
func TestPreparationJoueur(t *testing.T) {
	b, reg := bancServeur(t)
	mj := b.connecter("meneur")
	_, v, _ := b.proxy("POST", "/api/entites", map[string]any{"type": "seance", "nom": "Séance 1", "data": map[string]any{"preparation": "SECRET-PREPA"}}, mj)
	id := v["id"]
	b.proxy("POST", fmt.Sprintf("/api/entites/%v/partage", id), map[string]any{"partage": true}, mj)
	reg.Creer("joueuse", "mot-de-passe-joueuse", false)
	reg.Modifier("joueuse", func(c *comptes.Compte) error { c.Roles[b.s.store.ID()] = comptes.Joueur; return nil })
	joueuse := b.connecter("joueuse")
	if c, _, _ := b.proxy("GET", fmt.Sprintf("/api/entites/%v/preparation", id), nil, joueuse); c != 403 {
		t.Fatalf("préparation lue par la joueuse : %d", c)
	}
	if c, _, _ := b.proxy("GET", fmt.Sprintf("/api/entites/%v", id), nil, joueuse); c != 403 {
		t.Fatalf("fiche de séance lue par la joueuse : %d", c)
	}
	// ni dans ce que voient les joueurs (export vers Obsidian « joueurs »)
	if vues, err := b.s.mondeJoueur(); err != nil || strings.Contains(toJSON(vues), "SECRET-PREPA") {
		t.Fatalf("séance vue des joueurs : %v %v", err, vues)
	}
}
