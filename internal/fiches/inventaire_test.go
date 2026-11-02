package fiches

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/Tchyze/cof_dm/internal/modele"
	"github.com/Tchyze/cof_dm/internal/reference"
	"github.com/Tchyze/cof_dm/internal/regles"
)

// Un objet porté compte comme un bonus de capacité, avec son nom dans le détail ; une arme portée
// donne sa ligne d'attaque, qui ne se range pas ; un objet qui n'est pas porté ne compte pas.
func TestInventairePorte(t *testing.T) {
	raw := []byte(`{"valeurs":{"FOR":14,"DEX":12,"CON":10,"INT":10,"SAG":10,"CHA":10},"pv":10,"attaques":[{"nom":"Poing","type":"C"}],
		"inventaire":[
			{"nom":"Casque cabossé","porte":true,"emplacement":"tete","bonus":[{"cible":"def","valeur":2}]},
			{"nom":"Cotte","porte":true,"emplacement":"corps","bonus":[{"cible":"def","valeur":4},{"cible":"init","valeur":-1}]},
			{"nom":"Lame courte","porte":true,"emplacement":"main_principale","arme":{"type":"C","dm":"1d6","mod":1}},
			{"nom":"Arc","emplacement":"deux_mains","arme":{"type":"D","dm":"1d8"}},
			{"nom":"Torche","quantite":3}]}`)
	f, err := NormaliserFeuille(raw, 2, &regles.Feuille{DefenseBase: 8})
	if err != nil {
		t.Fatal(err)
	}
	if f.Defense != 8+1+2+4 || f.Init != 12-1 {
		t.Fatalf("DEF %d, Init %d", f.Defense, f.Init)
	}
	if e := f.Effets; e == nil || e.Base["def"] != 9 || e.Bonus["def"] != 6 || len(e.Detail) != 3 || e.Detail[0].Objet != "Casque cabossé" || e.Detail[0].Valeur != 2 {
		t.Fatalf("détail : %+v", f.Effets)
	}
	if len(f.Attaques) != 2 || f.Attaques[1].Nom != "Lame courte" || f.Attaques[1].Bonus != 2+2+1 || f.Attaques[1].Objet == 0 {
		t.Fatalf("attaques : %+v", f.Attaques)
	}
	if f.Inventaire[4].Quantite != 3 || f.Inventaire[4].ID != 5 {
		t.Fatalf("lignes : %+v", f.Inventaire)
	}
	// rangée sans l'attaque de l'arme ; relue, elle la retrouve, une seule fois
	var range_ Feuille
	_ = json.Unmarshal(f.Stockage(), &range_)
	if len(range_.Attaques) != 1 {
		t.Fatalf("attaques rangées : %+v", range_.Attaques)
	}
	g, err := NormaliserFeuille(f.JSON(), 2, &regles.Feuille{DefenseBase: 8})
	if err != nil || len(g.Attaques) != 2 || g.Defense != f.Defense {
		t.Fatalf("relue : %v %+v", err, g)
	}
}

// Un seul objet par emplacement (ou son nombre de places) ; une arme à deux mains prend les deux
// mains ; un emplacement que les règles ne connaissent plus reste porté, avec une place.
func TestEmplacements(t *testing.T) {
	ecrire := func(objets string, rg *regles.Feuille) error {
		_, err := NormaliserFeuille([]byte(`{"pv":5,"inventaire":[`+objets+`]}`), 1, rg)
		return err
	}
	cas := []struct {
		objets string
		refus  string
	}{
		{`{"nom":"Casque","porte":true,"emplacement":"tete"},{"nom":"Heaume","porte":true,"emplacement":"tete"}`, "Tête : « Heaume » ne se porte pas avec « Casque »"},
		{`{"nom":"Casque","porte":true,"emplacement":"tete"},{"nom":"Heaume","emplacement":"tete"}`, ""},
		{`{"nom":"A","porte":true,"emplacement":"anneau"},{"nom":"B","porte":true,"emplacement":"anneau"}`, ""},
		{`{"nom":"A","porte":true,"emplacement":"anneau"},{"nom":"B","porte":true,"emplacement":"anneau"},{"nom":"C","porte":true,"emplacement":"anneau"}`, "Anneau"},
		{`{"nom":"Pavois","porte":true,"emplacement":"main_secondaire"},{"nom":"Grande hache","porte":true,"emplacement":"deux_mains"}`, "Main secondaire : « Grande hache »"},
		{`{"nom":"Corde","porte":true}`, "sans emplacement"},
		// par son nom, avec ou sans accent ni majuscule : le même emplacement
		{`{"nom":"Casque","porte":true,"emplacement":"tete"},{"nom":"Heaume","porte":true,"emplacement":"tête"}`, "Tête : « Heaume » ne se porte pas avec « Casque »"},
		{`{"nom":"Pavois","porte":true,"emplacement":"Main secondaire"},{"nom":"Arc","porte":true,"emplacement":"deux mains"}`, "Main secondaire : « Arc »"},
		{`{"nom":"Cape","porte":true,"emplacement":"dos"},{"nom":"Sac","porte":true,"emplacement":"dos"}`, "dos : « Sac »"},
		{`{"nom":"Talisman","porte":true,"bonus":[{"cible":"chance","valeur":1}],"emplacement":"cou"}`, "bonus de -20 à +20"},
	}
	for _, c := range cas {
		err := ecrire(c.objets, nil)
		if (c.refus == "") != (err == nil) || (err != nil && !strings.Contains(err.Error(), c.refus)) {
			t.Errorf("%s : %v", c.objets, err)
		}
	}
	// les emplacements du fichier de règles remplacent ceux par défaut
	rg := &regles.Feuille{Emplacements: []regles.Emplacement{{Cle: "ceinture", Nom: "Ceinture", Nombre: 3}}}
	if err := ecrire(`{"nom":"A","porte":true,"emplacement":"ceinture"},{"nom":"B","porte":true,"emplacement":"ceinture"},{"nom":"C","porte":true,"emplacement":"ceinture"}`, rg); err != nil {
		t.Errorf("trois places : %v", err)
	}
	// relue sans règles (synchronisation), une feuille n'est pas refusée pour ce qui est porté
	if _, err := RelireFeuille([]byte(`{"pv":5,"inventaire":[{"nom":"A","porte":true,"emplacement":"tete"},{"nom":"B","porte":true,"emplacement":"tete"}]}`), 1); err != nil {
		t.Errorf("relue : %v", err)
	}
}

// Une ancienne feuille : son texte d'équipement devient des lignes, son armure et son bouclier des
// objets portés, et sa DEF ne change pas.
func TestRepriseEquipement(t *testing.T) {
	raw := []byte(`{"valeurs":{"FOR":10,"DEX":12,"CON":10,"INT":10,"SAG":10,"CHA":10},"pv":8,
		"armure":{"nom":"Cuir","bonus":2},"bouclier":{"nom":"","bonus":1},
		"equipement":"- 3 torches\nCorde de chanvre\n\n• Rations x5\n12 flèches"}`)
	f, err := RelireFeuille(raw, 1)
	if err != nil {
		t.Fatal(err)
	}
	var l []string
	for _, o := range f.Inventaire {
		l = append(l, o.Nom+"×"+itoa(o.Quantite)+map[bool]string{true: " porté", false: ""}[o.Porte])
	}
	if strings.Join(l, " ; ") != "Cuir×1 porté ; Bouclier×1 porté ; torches×3 ; Corde de chanvre×1 ; Rations×5 ; flèches×12" {
		t.Fatalf("lignes : %s", strings.Join(l, " ; "))
	}
	if f.Defense != 1+2+1 {
		t.Fatalf("DEF : %d", f.Defense)
	}
	if s := string(f.Stockage()); strings.Contains(s, `"equipement"`) || strings.Contains(s, `"armure"`) {
		t.Fatalf("rangée : %s", s)
	}
	// une feuille qui a déjà son inventaire ne reprend rien
	if f, _ := RelireFeuille([]byte(`{"pv":8,"inventaire":[],"equipement":"Corde"}`), 1); len(f.Inventaire) != 0 {
		t.Fatalf("reprise en double : %+v", f.Inventaire)
	}
}

// Le joueur change le nom, la quantité et ce qu'il porte ; les bonus, l'arme et l'emplacement d'un
// objet restent ceux du MJ, et ce qu'il ajoute n'a ni bonus ni arme.
func TestGarderChiffres(t *testing.T) {
	ancienne, _ := NormaliserFeuille([]byte(`{"pv":5,"inventaire":[{"id":1,"nom":"Casque","emplacement":"tete","bonus":[{"cible":"def","valeur":2}]}]}`), 1, nil)
	// une ligne ajoutée sans identifiant ne prend pas les chiffres de celle qui en reçoit le numéro ;
	// une ligne portée sans emplacement reprend celui du MJ avant d'être validée
	raw := []byte(`{"pv":5,"inventaire":[
		{"nom":"Pierre trouvée"},
		{"id":1,"nom":"Mon casque","porte":true,"bonus":[{"cible":"def","valeur":9}]},
		{"id":7,"nom":"Épée trouvée","quantite":2,"emplacement":"main_principale","arme":{"dm":"3d6"},"bonus":[{"cible":"att","valeur":5}]}]}`)
	garde, err := GarderChiffres(raw, ancienne)
	if err != nil {
		t.Fatal(err)
	}
	f, err := NormaliserFeuille(garde, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	p, a, b := f.Inventaire[0], f.Inventaire[1], f.Inventaire[2]
	if a.Nom != "Mon casque" || !a.Porte || a.Emplacement != "tete" || a.Bonus[0].Valeur != 2 || a.ID != 1 {
		t.Fatalf("ligne du MJ : %+v", a)
	}
	if p.ID != 2 || p.Bonus != nil || p.Emplacement != "" {
		t.Fatalf("ligne ajoutée sans identifiant : %+v", p)
	}
	if b.ID != 3 || b.Bonus != nil || b.Arme != nil || b.Quantite != 2 || b.Emplacement != "main_principale" {
		t.Fatalf("ligne ajoutée : %+v", b)
	}
}

// La casse de la clé ne change rien, qu'elle soit seule ou répétée ; aucune variante ne reste dans le
// résultat, qui ne se lit plus que d'une façon.
func TestGarderChiffresCasse(t *testing.T) {
	ancienne, _ := NormaliserFeuille([]byte(`{"pv":5,"inventaire":[{"id":1,"nom":"Casque","fiche":3,"bonus":[{"cible":"def","valeur":2}]}]}`), 1, nil)
	forge := `[{"nom":"Forgé","fiche":9,"bonus":[{"cible":"def","valeur":20}],"arme":{"dm":"3d6"}}]`
	for _, corps := range []string{
		`{"pv":5,"Inventaire":` + forge + `}`,
		`{"pv":5,"INVENTAIRE":` + forge + `}`,
		`{"pv":5,"inventaire":[],"Inventaire":` + forge + `}`,
		`{"pv":5,"Inventaire":` + forge + `,"inventaire":[]}`,
		`{"pv":5,"inventaire":` + forge + `,"inventaire":` + forge + `}`,
	} {
		garde, err := GarderChiffres([]byte(corps), ancienne)
		if err != nil {
			t.Fatalf("%s : %v", corps, err)
		}
		var champs map[string]json.RawMessage
		_ = json.Unmarshal(garde, &champs)
		if _, reste := champs["Inventaire"]; reste || champs["INVENTAIRE"] != nil {
			t.Fatalf("%s : une variante de la clé reste : %s", corps, garde)
		}
		f, err := NormaliserFeuille(garde, 1, nil)
		if err != nil {
			t.Fatalf("%s : %v", corps, err)
		}
		for _, o := range f.Inventaire {
			if o.Fiche != 0 || o.Bonus != nil || o.Arme != nil {
				t.Fatalf("%s : ligne non contrôlée : %+v", corps, o)
			}
		}
	}
}

// Ce qui n'est pas une feuille lisible est une erreur ; sans inventaire, les lignes d'avant restent et
// l'ancien texte d'équipement n'en ajoute aucune.
func TestGarderChiffresCorpsInattendu(t *testing.T) {
	ancienne, _ := NormaliserFeuille([]byte(`{"pv":5,"inventaire":[{"id":1,"nom":"Casque","bonus":[{"cible":"def","valeur":2}]}]}`), 1, nil)
	for _, corps := range []string{`pas du json`, `null`, `[]`, `"texte"`, `{"inventaire":5}`, `{"Inventaire":"x"}`, `{"inventaire":[{"fiche":"3"}]}`} {
		if garde, err := GarderChiffres([]byte(corps), ancienne); err == nil {
			t.Errorf("%s accepté : %s", corps, garde)
		}
	}
	garde, err := GarderChiffres([]byte(`{"pv":5,"armure":{"nom":"Forgée","bonus":15},"equipement":"Autre"}`), ancienne)
	if err != nil {
		t.Fatal(err)
	}
	f, err := NormaliserFeuille(garde, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Inventaire) != 1 || f.Inventaire[0].Nom != "Casque" || f.Inventaire[0].Bonus[0].Valeur != 2 {
		t.Fatalf("sans inventaire : %+v", f.Inventaire)
	}
	if garde, err = GarderChiffres([]byte(`{"pv":5,"armure":{"nom":"Forgée","bonus":15}}`), nil); err != nil {
		t.Fatal(err)
	}
	if f, err = NormaliserFeuille(garde, 1, nil); err != nil || len(f.Inventaire) != 0 {
		t.Fatalf("première feuille d'un joueur : %+v %v", f, err)
	}
}

func TestLiesAjoutes(t *testing.T) {
	ancienne := &Feuille{Inventaire: []Objet{{Fiche: 3}, {Nom: "libre"}}}
	if (&Feuille{Inventaire: []Objet{{Fiche: 3}, {Nom: "autre"}}}).LiesAjoutes(ancienne) {
		t.Fatal("objet déjà lié signalé")
	}
	if !(&Feuille{Inventaire: []Objet{{Fiche: 4}}}).LiesAjoutes(ancienne) || !(&Feuille{Inventaire: []Objet{{Fiche: 4}}}).LiesAjoutes(nil) {
		t.Fatal("nouvel objet lié non signalé")
	}
}

func itoa(n int) string { return strconv.Itoa(n) }

// Les bonus d'un objet se saisissent sur les mêmes cibles, qu'il soit une ligne de l'inventaire ou une
// fiche « Objet ».
func TestCiblesBonusObjet(t *testing.T) {
	if want := append(slices.Clone(reference.CiblesBonus), Caracs...); !slices.Equal(modele.CiblesBonus, want) {
		t.Fatalf("modele.CiblesBonus %v, attendu %v", modele.CiblesBonus, want)
	}
}
