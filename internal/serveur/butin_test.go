package serveur

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// Le butin d'une rencontre va au pot commun de sa campagne, une seule fois ; le pot se partage par
// des propositions que le MJ valide (pièces dans la bourse, objets dans l'inventaire), et un partage
// s'annule tant que le PJ a encore ce qu'il a reçu.
func TestButinPotCommun(t *testing.T) {
	b := nouveau(t)
	camp := b.fiche("campagne", "Les Brumes", nil)
	aldric := b.fiche("pj", "Aldric", map[string]any{"campagne": camp, "statut": "actif"})
	brune := b.fiche("pj", "Brune", map[string]any{"campagne": camp, "statut": "actif"})
	seance := b.fiche("seance", "Séance 3", map[string]any{"campagne": camp, "butin": "Une carte."})
	dague := b.fiche("objet", "Dague d'écume", nil)
	pot := fmt.Sprintf("/api/campagnes/%v/pot", camp)
	feuille := func(pj float64) map[string]any {
		_, f := b.local("GET", fmt.Sprintf("/api/entites/%v/feuille", pj), nil)
		return f
	}
	lignes := func(v map[string]any) []map[string]any {
		var l []map[string]any
		j, _ := json.Marshal(v["objets"])
		_ = json.Unmarshal(j, &l)
		return l
	}
	ligneDe := func(v map[string]any, nom string) float64 {
		for _, l := range lignes(v) {
			if l["nom"] == nom {
				return l["id"].(float64)
			}
		}
		t.Fatalf("« %s » absent du pot : %v", nom, v["objets"])
		return 0
	}

	// une rencontre sans campagne ne verse rien ; sans butin chiffré non plus
	butin := map[string]any{"pieces": map[string]any{"po": 30, "pa": 5}, "objets": []any{map[string]any{"fiche": dague}, map[string]any{"nom": "Torche", "quantite": 4}}}
	seule := b.fiche("rencontre", "Embuscade perdue", map[string]any{"butin_chiffre": butin})
	if c, v := b.local("POST", fmt.Sprintf("/api/entites/%v/verser-butin", seule), map[string]any{}); c != 422 || !strings.Contains(fmt.Sprint(v["detail"]), "campagne") {
		t.Fatalf("sans campagne : %d %v", c, v)
	}
	vide := b.fiche("rencontre", "Rien à prendre", map[string]any{"campagne": camp})
	if c, _ := b.local("POST", fmt.Sprintf("/api/entites/%v/verser-butin", vide), map[string]any{}); c != 422 {
		t.Fatalf("sans butin : %d", c)
	}
	// un objet du butin doit être un objet du monde
	if c, _ := b.local("POST", "/api/entites", map[string]any{"type": "rencontre", "nom": "X", "data": map[string]any{"butin_chiffre": map[string]any{"objets": []any{map[string]any{"fiche": aldric}}}}}); c != 422 {
		t.Fatalf("objet qui n'en est pas un : %d", c)
	}

	// versée : le pot a tout, la rencontre est jouée, la séance le note
	renc := b.fiche("rencontre", "Pillards du gué", map[string]any{"campagne": camp, "seance": seance, "butin_chiffre": butin})
	c, v := b.local("POST", fmt.Sprintf("/api/entites/%v/verser-butin", renc), map[string]any{})
	if c != 200 || v["pieces"].(map[string]any)["po"] != 30.0 || len(lignes(v)) != 2 {
		t.Fatalf("versement : %d %v", c, v)
	}
	if v["noms"].(map[string]any)[fmt.Sprint(renc)] != "Pillards du gué" {
		t.Fatalf("noms du pot : %v", v["noms"])
	}
	_, r := b.local("GET", fmt.Sprintf("/api/entites/%v", renc), nil)
	if d := r["data"].(map[string]any); d["butin_verse"] != true || d["statut"] != "jouée" {
		t.Fatalf("rencontre versée : %v", d)
	}
	_, se := b.local("GET", fmt.Sprintf("/api/entites/%v", seance), nil)
	if txt, _ := se["data"].(map[string]any)["butin"].(string); !strings.HasPrefix(txt, "Une carte.") || !strings.Contains(txt, "Pillards du gué : 30 po, 5 pa, Dague d'écume, 4 × Torche") {
		t.Fatalf("séance : %q", txt)
	}
	if c, _ := b.local("POST", fmt.Sprintf("/api/entites/%v/verser-butin", renc), map[string]any{}); c != 409 {
		t.Fatalf("versée deux fois : %d", c)
	}

	// le MJ ajoute ; une torche de plus rejoint la ligne des torches
	if c, v = b.local("POST", pot+"/ajouter", map[string]any{"libelle": "Prime du bourgmestre", "pieces": map[string]any{"po": 10}, "objets": []any{map[string]any{"nom": "torche"}}}); c != 200 || v["pieces"].(map[string]any)["po"] != 40.0 || len(lignes(v)) != 2 {
		t.Fatalf("ajout : %d %v", c, v)
	}
	torche, dg := ligneDe(v, "Torche"), ligneDe(v, "Dague d'écume")

	// une proposition qui demande plus que le pot : refusée
	if c, _ := b.local("POST", pot+"/propositions", map[string]any{"pj": aldric, "pieces": map[string]any{"po": 41}}); c != 422 {
		t.Fatalf("trop demandé : %d", c)
	}
	if c, _ := b.local("POST", pot+"/propositions", map[string]any{"pj": camp, "pieces": map[string]any{"po": 1}}); c != 422 {
		t.Fatalf("partage pour une campagne : %d", c)
	}
	// proposée puis validée : bourse, inventaire, détenteur
	c, v = b.local("POST", pot+"/propositions", map[string]any{"pj": aldric, "pieces": map[string]any{"po": 20}, "objets": []any{map[string]any{"ligne": dg, "quantite": 1}, map[string]any{"ligne": torche, "quantite": 2}}, "note": "sa part"})
	props, _ := v["propositions"].([]any)
	if c != 200 || len(props) != 1 || v["pieces"].(map[string]any)["po"] != 40.0 {
		t.Fatalf("proposition : %d %v", c, v)
	}
	prop := props[0].(map[string]any)["id"]
	if c, v = b.local("POST", fmt.Sprintf("%s/propositions/%v/valider", pot, prop), map[string]any{}); c != 200 || v["pieces"].(map[string]any)["po"] != 20.0 || len(v["propositions"].([]any)) != 0 || len(lignes(v)) != 1 {
		t.Fatalf("validation : %d %v", c, v)
	}
	f := feuille(aldric)
	if f["bourse"].(map[string]any)["po"] != 20.0 || !strings.Contains(fmt.Sprint(f["inventaire"]), "Dague d'écume") || !strings.Contains(fmt.Sprint(f["inventaire"]), "Torche") {
		t.Fatalf("feuille d'Aldric : %v / %v", f["bourse"], f["inventaire"])
	}
	_, o := b.local("GET", fmt.Sprintf("/api/entites/%v", dague), nil)
	if o["data"].(map[string]any)["detenteur"] != aldric {
		t.Fatalf("détenteur de la dague : %v", o["data"])
	}
	// une proposition refusée disparaît sans rien changer
	_, v = b.local("POST", pot+"/propositions", map[string]any{"pj": brune, "pieces": map[string]any{"po": 20}})
	prop = v["propositions"].([]any)[0].(map[string]any)["id"]
	if c, v = b.local("POST", fmt.Sprintf("%s/propositions/%v/refuser", pot, prop), map[string]any{}); c != 200 || len(v["propositions"].([]any)) != 0 || v["pieces"].(map[string]any)["po"] != 20.0 {
		t.Fatalf("refus : %d %v", c, v)
	}
	// le MJ partage directement à Brune
	if c, v = b.local("POST", pot+"/partager", map[string]any{"pj": brune, "pieces": map[string]any{"po": 15, "pa": 5}}); c != 200 || v["pieces"].(map[string]any)["po"] != 5.0 {
		t.Fatalf("partage direct : %d %v", c, v)
	}
	if feuille(brune)["bourse"].(map[string]any)["pa"] != 5.0 {
		t.Fatalf("bourse de Brune : %v", feuille(brune)["bourse"])
	}

	// annulation : Aldric a tout gardé, tout revient au pot ; la dague n'a plus de détenteur
	partage := func(pj float64) float64 {
		var h []map[string]any
		_, v := b.local("GET", pot, nil)
		j, _ := json.Marshal(v["historique"])
		_ = json.Unmarshal(j, &h)
		for _, m := range h {
			if m["genre"] == "partage" && m["pj"] == pj && m["annule"] != true {
				return m["id"].(float64)
			}
		}
		t.Fatalf("pas de partage pour %v : %v", pj, h)
		return 0
	}
	if c, v = b.local("POST", fmt.Sprintf("%s/partages/%v/annuler", pot, partage(aldric)), map[string]any{}); c != 200 || v["pieces"].(map[string]any)["po"] != 25.0 || len(lignes(v)) != 2 {
		t.Fatalf("annulation : %d %v", c, v)
	}
	if f = feuille(aldric); f["bourse"].(map[string]any)["po"] != 0.0 || strings.Contains(fmt.Sprint(f["inventaire"]), "Torche") {
		t.Fatalf("feuille après annulation : %v / %v", f["bourse"], f["inventaire"])
	}
	_, o = b.local("GET", fmt.Sprintf("/api/entites/%v", dague), nil)
	if o["data"].(map[string]any)["detenteur"] != nil {
		t.Fatalf("dague rendue : %v", o["data"])
	}
	// Brune a dépensé ses pièces : son partage ne s'annule plus, et rien ne change
	fb := feuille(brune)
	fb["bourse"] = map[string]any{"po": 3, "pa": 5}
	if c, v := b.local("PUT", fmt.Sprintf("/api/entites/%v/feuille", brune), fb); c != 200 {
		t.Fatalf("dépense : %d %v", c, v)
	}
	if c, v = b.local("POST", fmt.Sprintf("%s/partages/%v/annuler", pot, partage(brune)), map[string]any{}); c != 409 || !strings.Contains(fmt.Sprint(v["detail"]), "15 po") {
		t.Fatalf("annulation impossible : %d %v", c, v)
	}
	if _, v = b.local("GET", pot, nil); v["pieces"].(map[string]any)["po"] != 25.0 {
		t.Fatalf("pot après un refus d'annulation : %v", v["pieces"])
	}

	// retrait par le MJ
	if c, v = b.local("POST", pot+"/retirer", map[string]any{"libelle": "Vendu", "objets": []any{map[string]any{"ligne": ligneDe(v, "Torche"), "quantite": 5}}}); c != 200 || len(lignes(v)) != 1 {
		t.Fatalf("retrait : %d %v", c, v)
	}

	// feuilles coupées : le pot garde tout
	b.local("PUT", "/api/fonctionnalites", map[string]any{"feuilles": false})
	if c, v := b.local("POST", pot+"/partager", map[string]any{"pj": brune, "pieces": map[string]any{"po": 1}}); c != 409 || !strings.Contains(fmt.Sprint(v["detail"]), "feuilles") {
		t.Fatalf("sans feuilles : %d %v", c, v)
	}
	b.local("PUT", "/api/fonctionnalites", map[string]any{"feuilles": true})

	// la dague effacée reste dans le pot et dans le butin de la rencontre, en objet courant
	if c, _ := b.local("DELETE", fmt.Sprintf("/api/entites/%v?definitif=1", dague), nil); c != 200 {
		t.Fatalf("suppression de la dague : %d", c)
	}
	_, v = b.local("GET", pot, nil)
	if l := lignes(v); len(l) != 1 || l[0]["nom"] != "Dague d'écume" || l[0]["fiche"] != nil {
		t.Fatalf("pot après la dague : %v", v["objets"])
	}
	_, r = b.local("GET", fmt.Sprintf("/api/entites/%v", renc), nil)
	if s := fmt.Sprint(r["data"].(map[string]any)["butin_chiffre"]); strings.Contains(s, "fiche") || !strings.Contains(s, "Dague d'écume") {
		t.Fatalf("butin après la dague : %s", s)
	}

	// le journal garde la trace des partages
	_, j := b.local("GET", "/api/journal", nil)
	if s := fmt.Sprint(j); !strings.Contains(s, "butin reçu") || !strings.Contains(s, "pot commun : butin versé") {
		t.Fatalf("journal : %s", s)
	}

	// la campagne effacée emporte son pot
	if c, _ := b.local("DELETE", fmt.Sprintf("/api/entites/%v?definitif=1", camp), nil); c != 200 {
		t.Fatalf("suppression de la campagne : %d", c)
	}
	if docs, _ := b.s.store.ListerDocs("pots"); len(docs) != 0 {
		t.Fatalf("pots restants : %d", len(docs))
	}

	// fonctionnalité coupée : les routes du pot répondent 404
	b.local("PUT", "/api/fonctionnalites", map[string]any{"butin": false})
	if c, _ := b.local("GET", pot, nil); c != 404 {
		t.Fatalf("butin coupé : %d", c)
	}
}
