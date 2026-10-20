package reference

import (
	"encoding/json"
	"strings"
	"testing"
)

// Les voies, profils et peuples des tests sont inventés.

func TestLireAncienFormat(t *testing.T) {
	r, err := Lire([]byte(`{"format":"tracker-cof-reference","version":1,
		"profils":[{"ref":"essai-guetteur","nom":"Guetteur","dv":"d8"}],
		"peuples":[{"nom":"Gens des marais","caracs":"+2 SAG, −1 FOR, au choix +1 CHA"}],
		"voies":[{"nom":"Voie de la lanterne","rangs":[{"nom":"Lueur","sort":true}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if r.Format != Format || r.Version != Version {
		t.Fatalf("format relu : %s %d", r.Format, r.Version)
	}
	if r.Profils[0].Etiquette != Officielle || r.Peuples[0].Etiquette != Maison || r.Voies[0].Etiquette != Maison {
		t.Fatalf("étiquettes par défaut : %s %s %s", r.Profils[0].Etiquette, r.Peuples[0].Etiquette, r.Voies[0].Etiquette)
	}
	if m := r.Peuples[0].Modificateurs; len(m) != 3 || m["SAG"] != 2 || m["FOR"] != -1 || m["CHA"] != 1 {
		t.Fatalf("modificateurs repris du texte : %v", m)
	}
	if len(r.Voies[0].Rangs) != RangsMin || r.Voies[0].Type != "profil" {
		t.Fatalf("voie : %+v", r.Voies[0])
	}
	// un simple tableau de voies
	if r, err := Lire([]byte(`[{"nom":"Voie du gué"}]`)); err != nil || len(r.Voies) != 1 {
		t.Fatalf("tableau de voies : %v", err)
	}
}

func TestLireRefus(t *testing.T) {
	for nom, f := range map[string]string{
		"autre format":     `{"format":"cof-dm-regles"}`,
		"plus récent":      `{"format":"cof-dm-reference","version":3}`,
		"sans nom":         `{"voies":[{"nom":" "}]}`,
		"étiquette":        `{"voies":[{"nom":"Voie du gué","etiquette":"verrouillée"}]}`,
		"bonus inconnu":    `{"voies":[{"nom":"Voie du gué","rangs":[{"nom":"A","bonus":[{"cible":"vitesse","valeur":1}]}]}]}`,
		"carac inconnue":   `{"voies":[{"nom":"Voie du gué","rangs":[{"nom":"A","bonus":[{"cible":"def","valeur":1,"carac":"CHANCE"}]}]}]}`,
		"genre d'action":   `{"voies":[{"nom":"Voie du gué","rangs":[{"nom":"A","action":{"genre":"danse"}}]}]}`,
		"type d'attaque":   `{"voies":[{"nom":"Voie du gué","rangs":[{"nom":"A","action":{"genre":"attaque","attaque":"X"}}]}]}`,
		"coût négatif":     `{"voies":[{"nom":"Voie du gué","rangs":[{"nom":"A","action":{"genre":"sort","cout_pm":-1}}]}]}`,
		"par heure":        `{"voies":[{"nom":"Voie du gué","rangs":[{"nom":"A","utilisations":{"fois":1,"par":"heure"}}]}]}`,
		"repos sans clé":   `{"voies":[{"nom":"Voie du gué","rangs":[{"nom":"A","utilisations":{"fois":1,"par":"repos:"}}]}]}`,
		"zéro fois":        `{"voies":[{"nom":"Voie du gué","rangs":[{"nom":"A","utilisations":{"fois":0,"par":"jour"}}]}]}`,
		"trop de rangs":    `{"voies":[{"nom":"Voie du gué","rangs":[` + strings.Repeat(`{},`, RangsMax) + `{}]}]}`,
		"modificateur":     `{"peuples":[{"nom":"Gens du gué","modificateurs":{"VIT":1}}]}`,
		"carac. de magie":  `{"profils":[{"nom":"Passeur","carac_magie":"FOR"}]}`,
		"pas un JSON":      `{`,
		"profil sans nom":  `{"profils":[{"nom":""}]}`,
		"peuple sans nom":  `{"peuples":[{}]}`,
		"étiquette profil": `{"profils":[{"nom":"Passeur","etiquette":"x"}]}`,
	} {
		if _, err := Lire([]byte(f)); err == nil {
			t.Errorf("%s : accepté", nom)
		}
	}
}

func TestCapaciteComplete(t *testing.T) {
	r, err := Lire([]byte(`{"format":"cof-dm-reference","version":2,"voies":[{"nom":"Voie du gué","etiquette":"officielle","rangs":[
		{"nom":"Pas sûr","bonus":[{"cible":"def","valeur":1,"par_rang":true},{"cible":"init","valeur":0,"carac":"SAG"}]},
		{"nom":"Trait d'eau","action":{"genre":"sort","cout_pm":2,"attaque":"M","degats":"1d6","etat":"trempe"},
		 "utilisations":{"fois":2,"par":"repos:long"}},
		{"nom":"Halte","utilisations":{"fois":1,"par":"jour"}}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	v := r.Voies[0]
	if v.Etiquette != Officielle || len(v.Rangs[0].Bonus) != 2 || !v.Rangs[0].Bonus[0].ParRang || v.Rangs[0].Bonus[1].Carac != "SAG" {
		t.Fatalf("bonus : %+v", v.Rangs[0])
	}
	if a := v.Rangs[1].Action; a == nil || a.CoutPm != 2 || a.Attaque != "M" || v.Rangs[1].Utilisations.Par != "repos:long" {
		t.Fatalf("action : %+v", v.Rangs[1])
	}
	// une étiquette donnée à tout le fichier
	if err := r.Etiqueter(Maison); err != nil || r.Voies[0].Etiquette != Maison {
		t.Fatalf("étiqueter : %v", err)
	}
	if r.Etiqueter("x") == nil {
		t.Fatal("étiquette inconnue acceptée")
	}
}

// Un élément rangé dans l'univers ne garde pas d'identifiant : il lui est donné à la lecture.
func TestNormaliserDoc(t *testing.T) {
	b, err := NormaliserDoc(CollVoies, []byte(`{"id":42,"nom":" Voie du gué "}`))
	if err != nil {
		t.Fatal(err)
	}
	var v Voie
	_ = json.Unmarshal(b, &v)
	if v.ID != 0 || v.Nom != "Voie du gué" || v.Etiquette != Maison || strings.Contains(string(b), `"id"`) {
		t.Fatalf("voie rangée : %s", b)
	}
	if _, err := NormaliserDoc(CollProfils, []byte(`{"nom":""}`)); err == nil {
		t.Fatal("profil sans nom rangé")
	}
	if _, err := NormaliserDoc("monstres", []byte(`{}`)); err == nil {
		t.Fatal("collection inconnue acceptée")
	}
}
