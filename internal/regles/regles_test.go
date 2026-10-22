package regles

import (
	"strings"
	"testing"
)

// Règles d'essai, inventées pour les tests : aucune valeur ne vient d'un livre.
const essai = `{"format":"cof-dm-regles","version":1,
 "listes":{"peuples":["Sélénite"," Sélénite ","Marcheur"],"tailles":["nain","géant"],"categories":["de chair"]},
 "creature":{
  "archetypes":[{"cle":"frêle","description":"Tient mal debout."},{"cle":"trapu","description":"Solide."}],
  "table":{"nain":{"unique":{"nc":0,"FOR":-1,"DEX":3,"CON":-1,"dexSup":true,"def":11,"pv":2,"init":15,"att":1,"dm":"1"}},
           "géant":{"frêle":{"nc":2,"FOR":3,"DEX":0,"CON":3,"def":12,"pv":20,"init":9,"att":4,"dm":"1d6+3"},
                    "trapu":{"nc":4,"FOR":6,"DEX":-1,"CON":6,"forSup":true,"def":15,"pv":40,"init":8,"att":7,"dm":"2d6+6"}}},
  "boss":[{"cle":"meneur","libelle":"Meneur","att":1,"dm":1,"def":1,"pv":7},{"cle":"fou","libelle":"Fou furieux","att":3,"dmDes":1,"pv":4}],
  "voies":[{"cle":"ecorce","nom":"Voie de l'écorce","sup":["CON"],"rangs":["Sève dure","Bois de fer"],"rd":[2,5]},
           {"cle":"vigie","nom":"Voie de la vigie","page":12,"choix":true,"rangs":["Œil ouvert"],"init":[4],"remarque":"Ne dort que d'un œil."}],
  "nc_par_rang_boss":1,"nc_par_rang_voie":1,"nc_par_rang_voie_faible":0.5,"nc_faible":0.5,"nc_attaque_multiple":2,
  "rangs_max_voie":2,"voies_conseillees":1,
  "textes":{"capacite_attaque_multiple":"Frappe deux fois.","alerte_voies_nc":"Trop de voies pour ce NC.","alerte_trop_de_voies":"Trop de voies."}}}`

func TestLire(t *testing.T) {
	r, err := Lire([]byte(essai))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Listes.Peuples) != 2 || r.Listes.Peuples[0] != "Sélénite" || r.EstVide() {
		t.Fatalf("listes : %v", r.Listes)
	}
	for _, mauvais := range []string{`{`, `{"format":"autre"}`, `{"format":"cof-dm-regles","creature":{"table":{}}}`,
		`{"format":"cof-dm-regles","creature":{"table":{"x":{"y":{"pv":0}}}}}`,
		`{"format":"cof-dm-regles","creature":{"table":{"x":{"y":{"pv":1}}},"voies":[{"cle":"a"},{"cle":"a"}]}}`} {
		if _, err := Lire([]byte(mauvais)); err == nil {
			t.Fatalf("fichier accepté : %s", mauvais)
		}
	}
	if !Vide().EstVide() {
		t.Fatal("des règles vides ne le sont pas")
	}
	// une fusion ne retire rien de ce que le nouveau fichier ne dit pas
	n, _ := Lire([]byte(`{"format":"cof-dm-regles","listes":{"profils":["Veilleur"]}}`))
	r.Fusionner(n)
	if len(r.Listes.Peuples) != 2 || len(r.Listes.Profils) != 1 || r.Creature == nil {
		t.Fatalf("fusion : %+v", r.Listes)
	}
}

func TestCalculer(t *testing.T) {
	r, _ := Lire([]byte(essai))
	c := r.Creature
	// le profil seul
	x, err := c.Calculer(DemandeCreature{Taille: "géant", Archetype: "trapu", Int: 2})
	if err != nil || x.Nc != 4 || x.Pv != 40 || x.Defense != 15 || x.Att != 7 || x.Dm != "2d6+6" || x.Carac["INT"] != 2 || strings.Join(x.Superieures, ",") != "FOR" {
		t.Fatalf("profil : %+v %v", x, err)
	}
	// une taille à archétype unique ignore l'archétype demandé
	if x, err := c.Calculer(DemandeCreature{Taille: "nain", Archetype: "trapu"}); err != nil || x.Pv != 2 || strings.Join(x.Superieures, ",") != "DEX" {
		t.Fatalf("archétype unique : %+v %v", x, err)
	}
	for _, d := range []DemandeCreature{{Taille: "inconnue"}, {Taille: "géant", Archetype: "inconnu"}} {
		if _, err := c.Calculer(d); err == nil {
			t.Fatalf("demande acceptée : %+v", d)
		}
	}
	// rangs de boss : chaque rang ajoute ses bonus ; un type inconnu vaut le premier
	x, _ = c.Calculer(DemandeCreature{Taille: "géant", Archetype: "frêle", TypeBoss: "meneur", RangsBoss: 2})
	if x.Nc != 4 || x.Att != 6 || x.Defense != 14 || x.Pv != 34 || x.Dm != "1d6+5" {
		t.Fatalf("boss : %+v", x)
	}
	x, _ = c.Calculer(DemandeCreature{Taille: "géant", Archetype: "frêle", TypeBoss: "fou", RangsBoss: 1})
	if x.Att != 7 || x.Dm != "2d6+3" || x.Pv != 24 {
		t.Fatalf("dé de dégâts ajouté : %+v", x)
	}
	// voies : NC, caractéristique supérieure, RD et initiative du rang atteint, rang plafonné
	x, _ = c.Calculer(DemandeCreature{Taille: "géant", Archetype: "frêle", Voies: map[string]int{"ecorce": 5, "vigie": 1, "absente": 2}, SupMagie: "SAG"})
	if x.Nc != 5 || x.Rd != 5 || x.Init != 13 || strings.Join(x.Superieures, ",") != "CON,SAG" {
		t.Fatalf("voies : %+v", x)
	}
	if !strings.Contains(x.Capacites, "Voie de l'écorce rang 2 : Sève dure, Bois de fer.") || !strings.Contains(x.Capacites, "(livre p. 12)") || !strings.Contains(x.Capacites, "Ne dort que d'un œil.") {
		t.Fatalf("capacités : %q", x.Capacites)
	}
	if len(x.Alertes) != 2 {
		t.Fatalf("alertes : %v", x.Alertes)
	}
	// créature de NC faible sans rang de boss : demi-NC par rang de voie
	if x, _ := c.Calculer(DemandeCreature{Taille: "nain", Voies: map[string]int{"ecorce": 1}}); x.Nc != 0.5 {
		t.Fatalf("NC d'une petite créature : %v", x.Nc)
	}
	if x, _ := c.Calculer(DemandeCreature{Taille: "nain", Voies: map[string]int{"ecorce": 1}, TypeBoss: "meneur", RangsBoss: 1}); x.Nc != 2 {
		t.Fatalf("NC d'une petite créature boss : %v", x.Nc)
	}
	// attaque multiple
	x, _ = c.Calculer(DemandeCreature{Taille: "géant", Archetype: "frêle", AttMultiple: true})
	if x.Nc != 4 || x.Capacites != "Frappe deux fois." {
		t.Fatalf("attaque multiple : %+v", x)
	}
}

func TestAjouterDm(t *testing.T) {
	for _, c := range [][4]any{{"2d6+5", 2, 0, "2d6+7"}, {"2d6+5", 0, 1, "3d6+5"}, {"1d8", 1, 1, "1d8+1+1d6"}, {"1", 2, 0, "3"}, {"1d4-2", 1, 0, "1d4-1"}, {"1d4-1", 1, 0, "1d4"}} {
		if g := ajouterDm(c[0].(string), c[1].(int), c[2].(int)); g != c[3] {
			t.Fatalf("ajouterDm(%v, %v, %v) = %s, attendu %v", c[0], c[1], c[2], g, c[3])
		}
	}
}

// Difficulté des rencontres, sur une table inventée.
const essaiRencontre = `{"format":"cof-dm-regles","rencontre":{
 "points":[{"nc":3,"points":40},{"nc":0,"points":2},{"nc":1,"points":10},{"nc":2,"points":20}],
 "points_au_dela":30,"groupe_reference":2,
 "conditions":[{"cle":"brume","libelle":"Brume","niveau":1,"detail":"On n'y voit rien."},{"cle":"vent","detail":"Sans effet sur le niveau."}],
 "verdicts":[{"cle":"promenade","texte":"Une promenade.","jusqu_a":-1,"inclus":true,"ton":"ok"},
             {"cle":"serre","texte":"Serré.","jusqu_a":1,"ton":"warn"},
             {"cle":"fuite","texte":"Fuyez.","ton":"danger"}],
 "alertes":[{"quand":"nc","seuil":3,"texte":"Gros morceau."},{"quand":"ecart_nc","seuil":2,"texte":"Groupe mal assorti."}]}}`

func TestRencontres(t *testing.T) {
	r, err := Lire([]byte(essaiRencontre))
	if err != nil || r.Rencontre == nil || r.EstVide() {
		t.Fatalf("lecture : %v %+v", err, r)
	}
	x := r.Rencontre
	if x.Points[0].Nc != 0 || x.Conditions[1].Libelle != "vent" {
		t.Fatalf("mise en ordre : %+v", x)
	}
	// points d'une créature : la ligne la plus proche, puis l'extrapolation
	for nc, p := range map[float64]int{0: 2, 0.4: 2, 1: 10, 2.6: 40, 3: 40, 5: 100} {
		if got := x.PointsDe(nc); got != p {
			t.Fatalf("points du NC %v : %d, attendu %d", nc, got, p)
		}
	}
	for p, n := range map[float64]float64{0: 0, 14: 1, 31: 3, 40: 3, 100: 5} {
		if got := x.NiveauDe(p); got != n {
			t.Fatalf("niveau pour %v points : %v, attendu %v", p, got, n)
		}
	}
	// 2 créatures de NC 1 pour 2 PJ de niveau 2 : 20 points, niveau 2, écart 0
	e := x.Evaluer(DemandeRencontre{Groupes: []GroupeRencontre{{Nom: "Gluon", Nc: 1, Qte: 2}, {Nom: "Absent", Nc: 3, Qte: 0}}, NbPj: 2, NiveauPj: 2})
	if e.Points != 20 || e.Niveau != 2 || e.Ecart != 0 || e.Cle != "serre" || e.Ton != "warn" || len(e.Alertes) != 0 || len(e.Detail) != 1 {
		t.Fatalf("évaluation : %+v", e)
	}
	// groupe de 4 : les points sont ramenés au groupe de référence (20 × 2 / 4 = 10, niveau 1)
	e = x.Evaluer(DemandeRencontre{Groupes: []GroupeRencontre{{Nom: "Gluon", Nc: 1, Qte: 2}}, NbPj: 4, NiveauPj: 2})
	if e.PointsAjustes != 10 || e.Niveau != 1 || e.Cle != "promenade" || len(e.Detail) != 2 {
		t.Fatalf("groupe de 4 : %+v", e)
	}
	// la borne exclue : un écart de 1 n'est plus « serré »
	e = x.Evaluer(DemandeRencontre{Groupes: []GroupeRencontre{{Nom: "Gluon", Nc: 1, Qte: 2}}, NbPj: 2, NiveauPj: 1})
	if e.Ecart != 1 || e.Cle != "fuite" {
		t.Fatalf("borne exclue : %+v", e)
	}
	// condition, alertes, NC au-delà de la table
	e = x.Evaluer(DemandeRencontre{Groupes: []GroupeRencontre{{Nom: "Gluon", Nc: 1, Qte: 1}, {Nom: "Titan", Nc: 4, Qte: 1}}, NiveauPj: 9, Condition: "brume"})
	if e.Points != 80 || e.Niveau != 4+1 || e.Cle != "promenade" || len(e.Alertes) != 3 {
		t.Fatalf("condition et alertes : %+v", e)
	}
	if e = x.Evaluer(DemandeRencontre{Groupes: []GroupeRencontre{{Nom: "Gluon", Nc: 1, Qte: 1}}}); e.Cle != "inconnu" || e.Niveau != 1 {
		t.Fatalf("sans niveau de groupe : %+v", e)
	}
	if e = x.Evaluer(DemandeRencontre{NiveauPj: 3}); e.Cle != "vide" {
		t.Fatalf("sans créature : %+v", e)
	}
	// fichiers refusés à l'import
	for _, mauvais := range []string{
		`{"format":"cof-dm-regles","rencontre":{"points":{"1":10}}}`,
		`{"format":"cof-dm-regles","rencontre":{"points":[{"nc":1,"points":10},{"nc":1,"points":20}]}}`,
		`{"format":"cof-dm-regles","rencontre":{"points":[{"nc":1,"points":10},{"nc":2,"points":5}]}}`,
		`{"format":"cof-dm-regles","rencontre":{"points":[{"nc":1,"points":10}],"verdicts":[{"cle":"x","ton":"rose"}]}}`,
		`{"format":"cof-dm-regles","rencontre":{"points":[{"nc":1,"points":10}],"alertes":[{"quand":"jamais","texte":"x"}]}}`} {
		if _, err := Lire([]byte(mauvais)); err == nil {
			t.Fatalf("fichier accepté : %s", mauvais)
		}
	}
	// une partie d'une ancienne forme, déjà rangée dans un univers, ne fait pas perdre le reste
	ancien := `{"format":"cof-dm-regles","listes":{"peuples":["Sélénite"]},"rencontre":{"points":{"1":10}}}`
	if r, err := Relire([]byte(ancien)); err != nil || r.Rencontre != nil || len(r.Listes.Peuples) != 1 {
		t.Fatalf("relecture d'une ancienne forme : %v %+v", err, r)
	}
	// une partie vide vaut absence, et une fusion sans cette partie garde celle en place
	if v, err := Lire([]byte(`{"format":"cof-dm-regles","rencontre":{}}`)); err != nil || v.Rencontre != nil {
		t.Fatalf("partie vide : %v", err)
	}
	n, _ := Lire([]byte(`{"format":"cof-dm-regles","listes":{"profils":["Veilleur"]}}`))
	r.Fusionner(n)
	if r.Rencontre == nil {
		t.Fatal("la fusion a retiré la partie rencontre")
	}
}

// Partie « combat », sur des états inventés.
func TestCombat(t *testing.T) {
	r, err := Lire([]byte(`{"format":"cof-dm-regles","combat":{"de":12,"echec_naturel":2,"critique_dm":3,"chance":7,"budget_actions":true,
	 "etats":[{"nom":" Ébloui ","att":-4,"de":8},{"nom":"Retranché","def":3,"fin":"tour","action":"L"}]}}`))
	if err != nil || r.Combat == nil || r.EstVide() {
		t.Fatalf("lecture : %v", err)
	}
	c := r.Combat
	if c.DeNormal() != 12 || !c.Connu("Ébloui") || c.Etat("Ébloui").De != 8 || c.Etat("Retranché").Action != "L" || c.Rang("Retranché") != 1 {
		t.Fatalf("états : %+v", c.Etats)
	}
	// un état que les règles ne décrivent pas n'a aucun effet et vient après les autres
	if e := c.Etat("Trempé"); c.Connu("Trempé") || e.Def != 0 || e.Bloque || c.Rang("Trempé") != 2 {
		t.Fatalf("état libre : %+v", e)
	}
	// sans règles : le dé par défaut, aucun état connu
	var sans *Combat
	if sans.DeNormal() != 20 || sans.Connu("Ébloui") || sans.Etat("Ébloui").Att != 0 || sans.Rang("x") != 0 {
		t.Fatal("combat sans règles")
	}
	for _, mauvais := range []string{
		`{"format":"cof-dm-regles","combat":{"de":1}}`,
		`{"format":"cof-dm-regles","combat":{"chance":-1}}`,
		`{"format":"cof-dm-regles","combat":{"etats":[{"nom":""}]}}`,
		`{"format":"cof-dm-regles","combat":{"etats":[{"nom":"A"},{"nom":"a"}]}}`,
		`{"format":"cof-dm-regles","combat":{"etats":[{"nom":"A","fin":"jamais"}]}}`,
		`{"format":"cof-dm-regles","combat":{"etats":[{"nom":"A","action":"M"}]}}`} {
		if _, err := Lire([]byte(mauvais)); err == nil {
			t.Fatalf("fichier accepté : %s", mauvais)
		}
	}
	// une fusion sans partie combat garde celle en place
	n, _ := Lire([]byte(`{"format":"cof-dm-regles","listes":{"profils":["Veilleur"]}}`))
	r.Fusionner(n)
	if r.Combat == nil {
		t.Fatal("la fusion a retiré la partie combat")
	}
}

// La partie « montee » du fichier de règles (valeurs inventées) : coût des rangs, niveau minimal,
// formule des PV ; les formes qui ne tiennent pas debout sont refusées.
func TestMontee(t *testing.T) {
	r, err := Lire([]byte(`{"format":"cof-dm-regles","personnage":{"montee":{"pv":["dv","con"],"points":2,"couts":[1,3],"niveau_min":[0,4]}}}`))
	if err != nil {
		t.Fatal(err)
	}
	m := r.Personnage.Montee
	if !m.Lancee() || m.Cout(1) != 1 || m.Cout(2) != 3 || m.Cout(5) != 3 || m.NiveauPour(2) != 4 || m.NiveauPour(3) != 0 {
		t.Fatalf("montée : %+v", m)
	}
	var rien *Montee
	if rien.Lancee() || rien.Cout(4) != 1 || rien.NiveauPour(1) != 0 {
		t.Fatal("sans partie « montee » : saisie libre, rangs à 1 point")
	}
	for _, mauvais := range []string{`{"pv":["cha"]}`, `{"couts":[0]}`, `{"points":-1}`, `{"niveau_min":[-2]}`} {
		if _, err := Lire([]byte(`{"format":"cof-dm-regles","personnage":{"montee":` + mauvais + `}}`)); err == nil {
			t.Fatalf("montée invalide acceptée : %s", mauvais)
		}
	}
}
