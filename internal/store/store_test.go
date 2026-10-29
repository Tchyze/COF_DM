package store

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func ouvrir(t *testing.T) *Store {
	t.Helper()
	s, err := Ouvrir(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Fermer() })
	return s
}

func creer(t *testing.T, s *Store, typ, nom string, data map[string]any) Entite {
	t.Helper()
	e, err := s.Creer(typ, nom, data, nil, "test")
	if err != nil {
		t.Fatalf("création de %s : %v", nom, err)
	}
	return e
}

func TestCreationValidation(t *testing.T) {
	s := ouvrir(t)
	mv := creer(t, s, "lieu", "Mont-Veille", map[string]any{"categorie": "Fort", "impression": "Un fort sur la colline."})
	if mv.Data["categorie"] != "fort" {
		t.Fatalf("choix non normalisé : %v", mv.Data)
	}
	g := creer(t, s, "pnj", "Gédéon", map[string]any{"statut": "vivant", "peuple": "Sélénite", "lieu": float64(mv.ID), "profil_combat": "12", "apparence": "Vieux, [[Mont-Veille]] et [[Clémence]]."})
	if g.Data["profil_combat"] != 12.0 || g.Data["lieu"] != float64(mv.ID) || g.Data["peuple"] != "Sélénite" {
		t.Fatalf("données : %#v", g.Data)
	}
	cas := []map[string]any{
		{"statut": "zombie"},
		{"lieu": float64(g.ID)}, // un PNJ n'est pas un lieu
		{"lieu": float64(999)},  // inexistant
		{"inconnu": "x"},        // champ inconnu
		{"nc": "beaucoup"},      // pas un nombre
	}
	for _, d := range cas {
		_, err := s.Creer("pnj", "X", d, nil, "test")
		var ev ErrValidation
		if !errors.As(err, &ev) {
			t.Errorf("%v accepté (%v)", d, err)
		}
	}
	if _, err := s.Creer("dragon", "X", nil, nil, ""); err == nil {
		t.Error("type inconnu accepté")
	}
	if _, err := s.Creer("pnj", "A [[b]]", nil, nil, ""); err == nil {
		t.Error("nom avec crochets accepté")
	}
	q := creer(t, s, "quete", "Le chariot", map[string]any{"etapes": []any{"Survivre", map[string]any{"texte": "Arriver", "fait": true}}})
	if et := q.Data["etapes"].([]any); len(et) != 2 || et[1].(map[string]any)["fait"] != true {
		t.Fatalf("étapes : %#v", q.Data["etapes"])
	}
	if _, err := s.Creer("seance", "S1", map[string]any{"date_jeu": map[string]any{"an": 1215.0, "mois": 13.0, "jour": 6.0}}, nil, ""); err == nil {
		t.Error("6e jour complémentaire accepté")
	}
	if _, err := s.Creer("seance", "S1", map[string]any{"date_jeu": map[string]any{"an": 1215.0, "mois": 12.0, "jour": 1.0}, "date_reelle": "2026-10-12"}, nil, ""); err != nil {
		t.Error(err)
	}
}

func TestModifierRenommerSupprimer(t *testing.T) {
	s := ouvrir(t)
	mv := creer(t, s, "lieu", "Mont-Veille", nil)
	g := creer(t, s, "pnj", "Gédéon", map[string]any{"lieu": float64(mv.ID), "role": "Va à [[Mont-Veille]] puis [[mont-veille|au fort]]."})
	f := creer(t, s, "faction", "La Compagnie", map[string]any{"siege": float64(mv.ID)})

	refs, _ := s.References(mv.ID)
	if len(refs) != 3 {
		t.Fatalf("références : %+v", refs)
	}
	// modification partielle
	e, err := s.Modifier(g.ID, Maj{Data: map[string]any{"statut": "mort", "role": nil}}, "test")
	if err != nil || e.Data["statut"] != "mort" || e.Data["role"] != nil || e.Data["lieu"] == nil {
		t.Fatalf("fusion : %v %#v", err, e.Data)
	}
	if _, err := s.Modifier(g.ID, Maj{Data: map[string]any{"statut": "vivant"}, Attendu: g.Modifie}, ""); !errors.Is(err, ErrConflit) {
		t.Fatalf("conflit non détecté : %v", err)
	}
	s.Modifier(g.ID, Maj{Data: map[string]any{"role": "Va à [[Mont-Veille]] puis [[mont-veille|au fort]] et [[Mont-Veille#Tour]]."}}, "")
	// renommage : les mentions suivent
	nom := "Fort de Mont-Veille"
	if _, err := s.Modifier(mv.ID, Maj{Nom: &nom}, "test"); err != nil {
		t.Fatal(err)
	}
	g2, _ := s.Lire(g.ID)
	if r := g2.Data["role"].(string); r != "Va à [[Fort de Mont-Veille]] puis [[Fort de Mont-Veille|au fort]] et [[Fort de Mont-Veille#Tour]]." {
		t.Fatalf("mentions non renommées : %q", r)
	}
	if m, _ := s.LiensMorts(); len(m) != 0 {
		t.Fatalf("liens morts après renommage : %+v", m)
	}
	// suppression : retirée des champs des autres fiches
	if _, err := s.Supprimer(mv.ID, "test"); err != nil {
		t.Fatal(err)
	}
	g3, _ := s.Lire(g.ID)
	f3, _ := s.Lire(f.ID)
	if g3.Data["lieu"] != nil || f3.Data["siege"] != nil {
		t.Fatalf("référence orpheline : %#v %#v", g3.Data, f3.Data)
	}
	if m, _ := s.LiensMorts(); len(m) != 1 || m[0].Cible != "Fort de Mont-Veille" {
		t.Fatalf("liens morts après suppression : %+v", m)
	}
}

func TestRelationsGrapheRecherche(t *testing.T) {
	s := ouvrir(t)
	m := creer(t, s, "pnj", "Maëlle Morvan", map[string]any{"alias": "Clémence", "personnalite": "Calculatrice et rancunière."})
	a := creer(t, s, "pnj", "Aurèle Morvan", nil)
	sg := creer(t, s, "faction", "Les Sans-Gages", map[string]any{"dirigeant": float64(a.ID)})
	if _, err := s.Relier(m.ID, a.ID, "parent", DetailRelation{}, "t"); err == nil {
		// Maëlle « parent de » Aurèle : accepté (c'est au MJ de choisir le sens)
	}
	note := "lieutenante"
	r2, err := s.Relier(m.ID, sg.ID, "membre", DetailRelation{Note: &note}, "t")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Relier(m.ID, sg.ID, "membre", DetailRelation{}, "t"); err == nil {
		t.Fatal("doublon accepté")
	}
	if _, err := s.Relier(m.ID, sg.ID, "copain", DetailRelation{}, "t"); err == nil {
		t.Fatal("type inconnu accepté")
	}
	rel, _ := s.Relations(sg.ID)
	if len(rel) != 1 || rel[0].Libelle != "a pour membre" || rel[0].Sens != "entrante" || rel[0].Note != "lieutenante" {
		t.Fatalf("relations vues de la faction : %+v", rel)
	}
	n, ar, err := s.Graphe(m.ID, 1)
	if err != nil || len(n) != 3 || len(ar) < 3 {
		t.Fatalf("graphe : %v %+v %+v", err, n, ar)
	}
	if err := s.Delier(r2, "t"); err != nil {
		t.Fatal(err)
	}
	for q, nom := range map[string]string{"maelle": "Maëlle Morvan", "mealle": "Maëlle Morvan", "rancuniere": "Maëlle Morvan", "clemence": "Maëlle Morvan", "sans gages": "Les Sans-Gages"} {
		l, _ := s.Lister(Filtre{Q: q})
		if len(l) == 0 || l[0].Nom != nom {
			t.Errorf("%q : %+v", q, l)
		}
	}
	if l, _ := s.Lister(Filtre{Type: "pnj"}); len(l) != 2 || l[0].Nom != "Aurèle Morvan" {
		t.Fatalf("liste triée : %+v", l)
	}
	s.Archiver(a.ID, true, "")
	if l, _ := s.Lister(Filtre{Type: "pnj"}); len(l) != 1 {
		t.Fatalf("archive listée : %+v", l)
	}
}

func TestChronologieExport(t *testing.T) {
	s := ouvrir(t)
	c := creer(t, s, "campagne", "Les Moissons", map[string]any{"date_debut": map[string]any{"an": 1215.0, "mois": 12.0, "jour": 1.0}})
	creer(t, s, "evenement", "L'émeute", map[string]any{"date_jeu": map[string]any{"an": 1200.0, "mois": 3.0, "jour": 10.0}})
	creer(t, s, "seance", "Séance 1", map[string]any{"campagne": float64(c.ID), "date_jeu": map[string]any{"an": 1215.0, "mois": 12.0, "jour": 3.0}})
	ch, err := s.Chronologie(c.ID)
	if err != nil || len(ch) != 3 || ch[0].Fiche.Nom != "L'émeute" || !strings.Contains(ch[2].Texte, "Fructidor 1215") {
		t.Fatalf("chronologie : %v %+v", err, ch)
	}
	x, err := s.Exporter()
	if err != nil || len(x.Entites) != 3 {
		t.Fatal(err)
	}
	s2 := ouvrir(t)
	if err := s2.Restaurer(x, "t"); err != nil {
		t.Fatal(err)
	}
	if l, _ := s2.Lister(Filtre{Q: "emeute"}); len(l) != 1 {
		t.Fatalf("restauration non indexée : %+v", l)
	}
}

func TestOrganigramme(t *testing.T) {
	s := ouvrir(t)
	aurele := creer(t, s, "pnj", "Aurèle Morvan", nil)
	sg := creer(t, s, "faction", "Les Sans-Gages", map[string]any{"dirigeant": float64(aurele.ID)})
	bastien := creer(t, s, "pnj", "Bastien Carreau", nil)
	maelle := creer(t, s, "pnj", "Maëlle Morvan", nil)
	moellon := creer(t, s, "pnj", "Moellon", nil)
	cliquet := creer(t, s, "pnj", "Cliquet", map[string]any{"statut": "mort"})
	rouage := creer(t, s, "faction", "La Compagnie du Rouage", nil)
	rang := func(r string) *string { return &r }
	sup := func(id int64) *int64 { return &id }
	vrai := true
	rb, _ := s.Relier(bastien.ID, sg.ID, "membre", DetailRelation{Rang: rang("second")}, "t")
	if _, err := s.Relier(maelle.ID, sg.ID, "membre", DetailRelation{Rang: rang("messagère"), Secrete: &vrai}, "t"); err != nil {
		t.Fatal(err)
	}
	rm, err := s.Relier(moellon.ID, sg.ID, "membre", DetailRelation{Rang: rang("chef de poste"), Superieur: sup(bastien.ID)}, "t")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Relier(cliquet.ID, sg.ID, "membre", DetailRelation{Superieur: sup(rouage.ID)}, "t"); err == nil {
		t.Fatal("supérieur hors de la faction accepté")
	}
	s.Relier(cliquet.ID, sg.ID, "membre", DetailRelation{Superieur: sup(moellon.ID)}, "t")
	s.Relier(rouage.ID, sg.ID, "membre", DetailRelation{Rang: rang("alliés mécaniciens")}, "t")
	// boucle : Bastien sous Moellon alors que Moellon est sous Bastien
	if err := s.ModifierRelation(rb, DetailRelation{Superieur: sup(moellon.ID)}, "t"); err == nil {
		t.Fatal("boucle acceptée")
	}
	o, err := s.Organigramme(sg.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(o) != 1 || o[0].Fiche.Nom != "Aurèle Morvan" || !o[0].Dirigeant || len(o[0].Enfants) != 3 {
		t.Fatalf("racine : %+v", o)
	}
	b := o[0].Enfants[0] // la branche la plus fournie d'abord
	if b.Fiche.Nom != "Bastien Carreau" || b.Rang != "second" || len(b.Enfants) != 1 || b.Enfants[0].Enfants[0].Fiche.Nom != "Cliquet" || b.Enfants[0].Enfants[0].Statut != "mort" {
		t.Fatalf("branche de Bastien : %+v", b)
	}
	// retirer Moellon : Cliquet remonte sous Bastien
	if err := s.Delier(rm, "t"); err != nil {
		t.Fatal(err)
	}
	o, _ = s.Organigramme(sg.ID)
	if b := o[0].Enfants[0]; b.Fiche.Nom != "Bastien Carreau" || len(b.Enfants) != 1 || b.Enfants[0].Fiche.Nom != "Cliquet" {
		t.Fatalf("après retrait : %+v", o[0].Enfants)
	}
	var secret bool
	for _, e := range o[0].Enfants {
		if e.Fiche.Nom == "Maëlle Morvan" {
			secret = e.Secret
		}
	}
	if !secret {
		t.Fatal("appartenance secrète perdue")
	}
	if _, err := s.Organigramme(aurele.ID); err == nil {
		t.Fatal("organigramme d'un PNJ")
	}
}

// Le responsable inscrit sur la fiche d'un PNJ donne sa forme à l'organigramme : un chef, ses
// seconds, leurs hommes, et ceux qui répondent à l'un d'eux sans être membres.
func TestOrganigrammeResponsables(t *testing.T) {
	s := ouvrir(t)
	pnj := func(nom string, data map[string]any) Entite { return creer(t, s, "pnj", nom, data) }
	sous := func(e Entite) map[string]any { return map[string]any{"responsable": float64(e.ID)} }
	aurele := pnj("Aurèle Morvan", nil)
	sg := creer(t, s, "faction", "Les Sans-Gages", map[string]any{"dirigeant": float64(aurele.ID)})
	alderic := pnj("Aldéric", nil)         // hors de la faction
	gautier := pnj("Gautier Roussel", nil) // hors de la faction
	bastien := pnj("Bastien Carreau", sous(aurele))
	maelle := pnj("Maëlle Morvan", sous(aurele))
	moellon := pnj("Moellon", sous(bastien))
	josse := pnj("Josse Taillefer", sous(bastien))
	hilde := pnj("Hilde Ferrat", nil)                     // membre sans responsable : sous le dirigeant
	anselme := pnj("Anselme Delorme", sous(alderic))      // membre qui répond à quelqu'un d'extérieur
	morguenne := pnj("Capitaine Morguenne", sous(aurele)) // pas membre : rattaché
	marmiton := pnj("Marmiton", sous(morguenne))          // membre, sous un rattaché
	pnj("Lubin", sous(josse))                             // pas membre : rattaché, au troisième rang
	pnj("Jehan Corbel", map[string]any{"responsable": float64(gautier.ID), "responsable_secret": float64(maelle.ID)})
	pnj("Sans lien", sous(gautier))
	// ceux d'une autre faction restent dans la leur, sauf l'infiltré (responsable secret)
	rouage := creer(t, s, "faction", "La Compagnie du Rouage", nil)
	garde := creer(t, s, "faction", "La Garde", map[string]any{"dirigeant": float64(gautier.ID)})
	tissonet := pnj("Tissonet", sous(aurele))
	taupe := pnj("La Taupe", map[string]any{"responsable": float64(gautier.ID), "responsable_secret": float64(bastien.ID)})
	s.Relier(tissonet.ID, rouage.ID, "membre", DetailRelation{}, "t")
	s.Relier(taupe.ID, garde.ID, "membre", DetailRelation{}, "t")
	for _, m := range []Entite{bastien, maelle, moellon, josse, hilde, anselme, marmiton} {
		if _, err := s.Relier(m.ID, sg.ID, "membre", DetailRelation{}, "t"); err != nil {
			t.Fatal(err)
		}
	}
	type vu struct {
		parent                 string
		rattache, secret       bool
		repond                 string
		niveau, nombreDEnfants int
	}
	lire := func() (map[string]vu, []Poste) {
		o, err := s.Organigramme(sg.ID)
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]vu{}
		var parcourir func(l []Poste, parent string, niveau int)
		parcourir = func(l []Poste, parent string, niveau int) {
			for _, p := range l {
				if _, deja := out[p.Fiche.Nom]; deja {
					t.Fatalf("« %s » deux fois dans l'organigramme", p.Fiche.Nom)
				}
				v := vu{parent: parent, rattache: p.Rattache, secret: p.Secret, niveau: niveau, nombreDEnfants: len(p.Enfants)}
				if p.Repond != nil {
					v.repond = p.Repond.Nom
				}
				out[p.Fiche.Nom] = v
				parcourir(p.Enfants, p.Fiche.Nom, niveau+1)
			}
		}
		parcourir(o, "", 0)
		return out, o
	}
	v, o := lire()
	if len(o) != 1 || o[0].Fiche.Nom != "Aurèle Morvan" || len(v) != 12 {
		t.Fatalf("un seul chef et douze postes attendus : %d racine(s), %d postes %v", len(o), len(v), v)
	}
	for nom, parent := range map[string]string{"Bastien Carreau": "Aurèle Morvan", "Maëlle Morvan": "Aurèle Morvan", "Moellon": "Bastien Carreau",
		"Josse Taillefer": "Bastien Carreau", "Hilde Ferrat": "Aurèle Morvan", "Anselme Delorme": "Aurèle Morvan",
		"Capitaine Morguenne": "Aurèle Morvan", "Marmiton": "Capitaine Morguenne", "Lubin": "Josse Taillefer", "Jehan Corbel": "Maëlle Morvan"} {
		if v[nom].parent != parent {
			t.Errorf("« %s » sous « %s », attendu sous « %s »", nom, v[nom].parent, parent)
		}
	}
	if o[0].Enfants[0].Fiche.Nom != "Bastien Carreau" || v["Lubin"].niveau != 3 {
		t.Errorf("le second le plus entouré d'abord, Lubin au troisième rang : %s, niveau %d", o[0].Enfants[0].Fiche.Nom, v["Lubin"].niveau)
	}
	if !v["Capitaine Morguenne"].rattache || !v["Lubin"].rattache || v["Marmiton"].rattache || v["Bastien Carreau"].rattache {
		t.Errorf("rattachés : %v", v)
	}
	if j := v["Jehan Corbel"]; !j.secret || !j.rattache {
		t.Errorf("Jehan répond en secret à Maëlle : %+v", j)
	}
	if v["Anselme Delorme"].repond != "Aldéric" || v["Hilde Ferrat"].repond != "" {
		t.Errorf("responsable extérieur : %+v", v["Anselme Delorme"])
	}
	if _, la := v["Sans lien"]; la {
		t.Error("un PNJ sans lien avec la faction figure dans l'organigramme")
	}
	if _, la := v["Tissonet"]; la {
		t.Error("le membre d'une autre faction est rattaché à celle de son responsable")
	}
	if x := v["La Taupe"]; x.parent != "Bastien Carreau" || !x.secret || !x.rattache {
		t.Errorf("l'infiltré d'une autre faction répond en secret à Bastien : %+v", x)
	}
	if r, err := s.Organigramme(rouage.ID); err != nil || len(r) != 1 || r[0].Repond == nil || r[0].Repond.Nom != "Aurèle Morvan" {
		t.Errorf("dans sa faction, Tissonet répond à quelqu'un d'extérieur : %+v", r)
	}
	g, _ := s.Organigramme(garde.ID)
	var membreOrdinaire bool
	for _, p := range g[0].Enfants {
		if p.Fiche.Nom == "La Taupe" {
			membreOrdinaire = !p.Secret && !p.Rattache
		}
	}
	if len(g) != 1 || !membreOrdinaire {
		t.Errorf("dans sa faction affichée, la Taupe est un membre comme un autre : %+v", g)
	}
	// le supérieur réglé dans la faction passe avant le responsable de la fiche
	var relJosse int64
	rels, _ := s.Relations(josse.ID)
	for _, r := range rels {
		if r.Type == "membre" {
			relJosse = r.ID
		}
	}
	if err := s.ModifierRelation(relJosse, DetailRelation{Superieur: &maelle.ID}, "t"); err != nil {
		t.Fatal(err)
	}
	if v, _ = lire(); v["Josse Taillefer"].parent != "Maëlle Morvan" || v["Lubin"].parent != "Josse Taillefer" {
		t.Errorf("supérieur de la faction ignoré : %+v", v["Josse Taillefer"])
	}
	// une boucle (Hilde répond à Moellon qui répond à Hilde) ne perd personne ; se répondre à soi-même non plus
	for id, data := range map[int64]map[string]any{hilde.ID: sous(moellon), moellon.ID: sous(hilde), anselme.ID: sous(anselme)} {
		if _, err := s.Modifier(id, Maj{Data: data}, "t"); err != nil {
			t.Fatal(err)
		}
	}
	if v, o = lire(); len(v) != 12 || len(o) != 1 || v["Anselme Delorme"].parent != "Aurèle Morvan" {
		t.Errorf("après une boucle : %d racine(s), %d postes, Anselme sous « %s »", len(o), len(v), v["Anselme Delorme"].parent)
	}
	// un responsable archivé ne compte plus : son subordonné qui n'est pas membre quitte l'organigramme
	if _, err := s.Archiver(morguenne.ID, true, "t"); err != nil {
		t.Fatal(err)
	}
	v, _ = lire()
	if _, la := v["Capitaine Morguenne"]; la || v["Marmiton"].parent != "Aurèle Morvan" {
		t.Errorf("responsable archivé : %+v", v["Marmiton"])
	}
	// supprimer un responsable efface le lien sur les fiches (intégrité des références)
	if _, err := s.Supprimer(bastien.ID, "t"); err != nil {
		t.Fatal(err)
	}
	if m, _ := s.Lire(moellon.ID); m.Data["responsable"] != float64(hilde.ID) {
		t.Fatalf("lien de Moellon touché à tort : %v", m.Data)
	}
	if _, err := s.Supprimer(hilde.ID, "t"); err != nil {
		t.Fatal(err)
	}
	if m, _ := s.Lire(moellon.ID); m.Data["responsable"] != nil {
		t.Fatalf("référence orpheline après suppression : %v", m.Data)
	}
}

func TestReparer(t *testing.T) {
	chemin := filepath.Join(t.TempDir(), "r.db")
	s, _ := Ouvrir(chemin)
	e := creer(t, s, "pnj", "Roussel", map[string]any{"statut": "vivant"})
	// simule une fiche de l'ancien format
	s.db.Exec(`UPDATE entites SET data=? WHERE id=?`, `{"statut":"vivant","nc":3,"allegeance":"la milice","peuple":"humain","notes":"Droit."}`, e.ID)
	s.db.Exec(`DELETE FROM meta WHERE cle='version_donnees'`)
	s.Fermer()
	s, err := Ouvrir(chemin)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Fermer()
	f, _ := s.Lire(e.ID)
	n, _ := f.Data["notes"].(string)
	if f.Data["nc"] != nil || f.Data["allegeance"] != nil || f.Data["peuple"] != "humain" || !strings.Contains(n, "nc : 3") || !strings.Contains(n, "la milice") || !strings.HasPrefix(n, "Droit.") {
		t.Fatalf("réparation : %#v", f.Data)
	}
}

// Jusqu'à la version 2, le profil de combat d'un PNJ et les créatures d'une rencontre désignaient
// des fiches du tracker externe : à l'ouverture, les créatures sont rattachées au bestiaire de
// l'univers par leur nom, et ce qui ne s'y retrouve pas est retiré et signalé.
func TestReparerLiensTracker(t *testing.T) {
	chemin := filepath.Join(t.TempDir(), "r.db")
	s, _ := Ouvrir(chemin)
	gluon, err := s.CreerDoc("monstres", []byte(`{"nom":"Gluon","nc":1}`), "Gluon", "test")
	if err != nil {
		t.Fatal(err)
	}
	pnj := creer(t, s, "pnj", "Roussel", map[string]any{"statut": "vivant", "notes": "Droit."})
	renc := creer(t, s, "rencontre", "Embuscade", nil)
	vide := creer(t, s, "rencontre", "Fausse piste", nil)
	s.db.Exec(`UPDATE entites SET data=? WHERE id=?`, `{"statut":"vivant","notes":"Droit.","profil_combat":41}`, pnj.ID)
	s.db.Exec(`UPDATE entites SET data=? WHERE id=?`, `{"tracker_rencontre":7,"adversaires":[{"monstre":41,"nom":"gluon","qte":3,"camp":"ennemi"},{"monstre":42,"nom":"Inconnu","qte":1,"camp":"ennemi"}]}`, renc.ID)
	s.db.Exec(`UPDATE entites SET data=? WHERE id=?`, `{"adversaires":[{"monstre":42,"nom":"Inconnu","qte":1,"camp":"ennemi"}]}`, vide.ID)
	s.db.Exec(`UPDATE meta SET valeur='2' WHERE cle='version_donnees'`)
	s.Fermer()
	s, err = Ouvrir(chemin)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Fermer()
	f, _ := s.Lire(pnj.ID)
	if n, _ := f.Data["notes"].(string); f.Data["profil_combat"] != nil || !strings.Contains(n, "n°41 de l'ancien tracker") || !strings.HasPrefix(n, "Droit.") {
		t.Fatalf("PNJ : %#v", f.Data)
	}
	f, _ = s.Lire(renc.ID)
	adv, _ := f.Data["adversaires"].([]any)
	if len(adv) != 1 || adv[0].(map[string]any)["monstre"] != float64(gluon.ID) || adv[0].(map[string]any)["qte"] != 3.0 || f.Data["tracker_rencontre"] != nil {
		t.Fatalf("rencontre : %#v", f.Data)
	}
	if f, _ = s.Lire(vide.ID); f.Data["adversaires"] != nil {
		t.Fatalf("rencontre sans créature retrouvée : %#v", f.Data)
	}
	// version 4 : la fiche de personnage d'un PJ dans le tracker et les combats du tracker d'une
	// séance sont retirés sans bruit ; à partir de la version 3, les profils de combat sont ceux
	// de l'univers et ne sont plus touchés
	s.Fermer()
	s, _ = Ouvrir(chemin)
	pj := creer(t, s, "pj", "Aldric", map[string]any{"secrets": "Rien."})
	seance := creer(t, s, "seance", "Séance 1", nil)
	meneur := creer(t, s, "pnj", "Meneur", map[string]any{"profil_combat": gluon.ID})
	s.db.Exec(`UPDATE entites SET data=? WHERE id=?`, `{"secrets":"Rien.","fiche_tracker":7}`, pj.ID)
	s.db.Exec(`UPDATE entites SET data=? WHERE id=?`, `{"combats":[3,4]}`, seance.ID)
	s.db.Exec(`UPDATE meta SET valeur='3' WHERE cle='version_donnees'`)
	s.Fermer()
	if s, err = Ouvrir(chemin); err != nil {
		t.Fatal(err)
	}
	defer s.Fermer()
	if f, _ = s.Lire(pj.ID); f.Data["fiche_tracker"] != nil || f.Data["secrets"] != "Rien." {
		t.Fatalf("PJ : %#v", f.Data)
	}
	if f, _ = s.Lire(seance.ID); f.Data["combats"] != nil {
		t.Fatalf("séance : %#v", f.Data)
	}
	if f, _ = s.Lire(meneur.ID); f.Data["profil_combat"] != float64(gluon.ID) {
		t.Fatalf("profil de combat de l'univers retiré à tort : %#v", f.Data)
	}
	if _, err := s.Modifier(pj.ID, Maj{Data: map[string]any{"joueur": "Léa"}}, "test"); err != nil {
		t.Fatalf("modification d'un PJ après réparation : %v", err)
	}
	// la fiche réparée se modifie de nouveau
	if _, err := s.Modifier(renc.ID, Maj{Data: map[string]any{"resume": "Au col."}}, "test"); err != nil {
		t.Fatalf("modification après réparation : %v", err)
	}
	// un export garde sa version : restauré, ses liens ne sont pas repris une seconde fois ; un
	// export plus ancien, sans version, l'est
	creer(t, s, "pnj", "Baron", map[string]any{"profil_combat": gluon.ID})
	x, _ := s.Exporter()
	if x.Donnees != versionDonnees {
		t.Fatalf("version de l'export : %q", x.Donnees)
	}
	if err := s.Restaurer(x, "test"); err != nil {
		t.Fatal(err)
	}
	baron := func() Entite {
		l, _ := s.Lister(Filtre{Type: "pnj", Q: "Baron"})
		f, _ := s.Lire(l[0].ID)
		return f
	}
	if baron().Data["profil_combat"] != float64(gluon.ID) {
		t.Fatalf("profil de combat perdu à la restauration : %#v", baron().Data)
	}
	x.Donnees = ""
	if err := s.Restaurer(x, "test"); err != nil {
		t.Fatal(err)
	}
	if baron().Data["profil_combat"] != nil {
		t.Fatalf("ancien export non repris : %#v", baron().Data)
	}
}

// Version 5 des données : à l'ouverture, une feuille qui a encore son ancien équipement le range
// dans son inventaire (texte, armure, bouclier), et l'univers garde sa DEF.
func TestRepriseInventaire(t *testing.T) {
	chemin := filepath.Join(t.TempDir(), "u.cofdm")
	s, err := Ouvrir(chemin)
	if err != nil {
		t.Fatal(err)
	}
	pj := creer(t, s, "pj", "Aldric", map[string]any{"niveau": 2})
	if _, err := s.EcrireFeuille(pj.ID, []byte(`{"pv":7}`), 0, "t"); err != nil {
		t.Fatal(err)
	}
	s.db.Exec(`UPDATE feuilles SET data=? WHERE entite=?`, `{"pv":7,"armure":{"nom":"Cuir","bonus":2},"equipement":"2 torches\nCorde"}`, pj.ID)
	s.db.Exec(`UPDATE meta SET valeur='4' WHERE cle='version_donnees'`)
	s.Fermer()
	if s, err = Ouvrir(chemin); err != nil {
		t.Fatal(err)
	}
	defer s.Fermer()
	raw, _, err := s.LireFeuille(pj.ID)
	if err != nil {
		t.Fatal(err)
	}
	var f map[string]any
	_ = json.Unmarshal(raw, &f)
	inv, _ := f["inventaire"].([]any)
	if len(inv) != 3 || f["equipement"] != nil || f["armure"] != nil || f["defense"] != float64(2) {
		t.Fatalf("feuille reprise : %s", raw)
	}
	if o := inv[1].(map[string]any); o["nom"] != "torches" || o["quantite"] != float64(2) {
		t.Fatalf("ligne : %v", o)
	}
}
