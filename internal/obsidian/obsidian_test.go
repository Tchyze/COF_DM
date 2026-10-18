package obsidian

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"

	"github.com/Tchyze/cof_dm/internal/combat"
	"github.com/Tchyze/cof_dm/internal/modele"
	"github.com/Tchyze/cof_dm/internal/store"
)

// Un univers inventé : une campagne, ses quêtes et ses séances, un PNJ au nom réservé par les
// systèmes de fichiers, deux fiches homonymes, un événement daté, un combat terminé et le journal.
func univers() Univers {
	date := func(an, mois, jour float64) map[string]any {
		return map[string]any{"an": an, "mois": mois, "jour": jour}
	}
	return Univers{
		Calendrier: modele.CalendrierParDefaut(),
		MJ:         true,
		Monstres:   map[int64]string{50: "Crapaud des tourbes"},
		Fiches: []store.Entite{
			{ID: 1, Type: "campagne", Nom: "La Boue", Data: map[string]any{"statut": "en cours", "pitch": "Tout s'enfonce.",
				"themes": "boue · pluie", "fils_rouges": "Le maire ment.", "date_debut": date(1216, 1, 5)}},
			{ID: 2, Type: "pnj", Nom: "Maître Odon : le Vieux", Data: map[string]any{"resume": "Il sait tout de [[Marais]].",
				"peuple": "Humain", "statut": "vivant", "campagnes": []any{float64(1)}, "notes": "Voir [[marais|le marais]] et [[Inconnu]].",
				"profil_combat": float64(50)}, Tags: []string{"aide de jeu"}},
			{ID: 3, Type: "lieu", Nom: "Marais", Data: map[string]any{"impression": "Ça sent.", "categorie": "site"}},
			{ID: 4, Type: "faction", Nom: "Marais", Data: map[string]any{"but": "Tout noyer.", "attitude": "hostile"}},
			{ID: 5, Type: "quete", Nom: "01 - Le pont", Data: map[string]any{"campagne": float64(1), "categorie": "arc principal",
				"ordre": float64(1), "statut": "en cours", "consequences": "Le village est coupé.",
				"etapes": []any{map[string]any{"texte": "Voir le pont", "fait": true}, map[string]any{"texte": "Le réparer", "fait": false}}}},
			{ID: 6, Type: "seance", Nom: "Séance 1", Data: map[string]any{"campagne": float64(1), "numero": float64(1)}},
			{ID: 7, Type: "evenement", Nom: "La crue", Data: map[string]any{"date_jeu": date(1215, 3, 2), "publique": "*Remarque.*\nLe fleuve | déborde.\nSuite.", "verite": "Le maire a ouvert la vanne.",
				"acteurs": []any{float64(2)}, "lieu": float64(3)}},
			{ID: 8, Type: "rencontre", Nom: "Les rats", Data: map[string]any{"adversaires": []any{map[string]any{"monstre": float64(50), "qte": float64(3), "camp": "ennemi"}}}},
			{ID: 9, Type: "pnj", Nom: "L'Oublié", Archive: true, Data: map[string]any{}},
			{ID: 10, Type: "evenement", Nom: "La vanne", Data: map[string]any{"date_jeu": date(1215, 3, 1), "verite": "*Vers 1215.*\n\nLe maire l'ouvre."}},
		},
		Relations: []store.RelationBrute{
			{ID: 1, A: 2, B: 4, Type: "membre", Rang: "trésorier"},
			{ID: 2, A: 2, B: 3, Type: "autre", Note: "y est né"},
			{ID: 3, A: 9, B: 2, Type: "ennemi", Secrete: true},
		},
		Medias: []store.Media{{ID: 1, Entite: 3, Fichier: "0123456789abcdef0123.png", Legende: "Vu du ciel", Principal: true, Contenu: []byte("png")}},
		Combats: []Combat{{ID: 1, Etat: &combat.Etat{Termine: true, SeanceID: 6, CampagneID: 1, RencontreID: ptr(8), Recap: &combat.Recap{
			Nom: "Les rats", Issue: "victoire", Rounds: 3, Date: 1760000000, Ennemis: 3, Vaincus: 3,
			Heros: &combat.Heros{Nom: "Ysolde", Texte: "3 rats"}, Pjs: []combat.BilanPJ{{Nom: "Ysolde", Titre: "La | Lame", Infliges: 12, Touches: 3, Tentatives: 4}},
			Courbe: []combat.PointPv{{R: 1, Pv: 20, Max: 20}, {R: 3, Pv: 9, Max: 20}}}}},
			{ID: 2, Etat: &combat.Etat{Nom: "En cours"}}},
		Journal: []store.LigneJournal{{ID: 1, Quand: 1760000000, Action: "modification", Entite: 2, Nom: "Maître Odon : le Vieux", Auteur: "MJ"}},
	}
}

func ptr(i int64) *int64 { return &i }

func notes(t *testing.T, u Univers) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, f := range Notes(u) {
		out[f.Chemin] = string(f.Contenu)
	}
	return out
}

func contient(t *testing.T, quoi, s string, attendus ...string) {
	t.Helper()
	for _, a := range attendus {
		if !strings.Contains(s, a) {
			t.Errorf("%s : %q manque dans\n%s", quoi, a, s)
		}
	}
}

func TestNotes(t *testing.T) {
	n := notes(t, univers())
	var chemins []string
	for k := range n {
		chemins = append(chemins, k)
	}
	attendus := []string{
		"05 - Campagnes/La Boue/La Boue.md", "05 - Campagnes/La Boue/Fils rouges.md",
		"05 - Campagnes/La Boue/Quêtes/Arc principal/01 - Le pont.md", "05 - Campagnes/La Boue/Séances/Séance 1.md",
		"05 - Campagnes/La Boue/Combats/2025-10-09 Les rats.md", "05 - Campagnes/Sans campagne/Rencontres/Les rats.md",
		"02 - PNJ/Maître Odon le Vieux.md", "01 - Monde/Lieux/Marais.md", "01 - Monde/Factions/Marais (Faction).md",
		"01 - Monde/Événements/La crue.md", "01 - Monde/Événements/La vanne.md", "01 - Monde/Chronologie.md", "Archives/PNJ/L'Oublié.md",
		"Images/Marais - 1.png", "Journal.md",
	}
	for _, a := range attendus {
		if _, ok := n[a]; !ok {
			t.Errorf("%s manque : %v", a, chemins)
		}
	}
	if len(n) != len(attendus) {
		t.Errorf("%d fichiers, %d attendus : %v", len(n), len(attendus), chemins)
	}

	// le PNJ : nom de fichier nettoyé, liens réécrits vers lui, propriétés lues par le script d'import
	odon := n["02 - PNJ/Maître Odon le Vieux.md"]
	contient(t, "PNJ", odon, "---\ntype: pnj\n", "# Maître Odon : le Vieux\n", "\n> Il sait tout de [[Marais]].\n",
		`race: "Humain"`, `campagnes: ["[[La Boue]]"]`, `profil_combat: "Crapaud des tourbes"`, `factions: ["[[Marais (Faction)|Marais]]"]`,
		`tags: ["aide-de-jeu"]`, "cofdm: 2", "## Notes du MJ\n\nVoir [[Marais|le marais]] et [[Inconnu]].",
		"| [[Marais]] | y est né |", "| [[L'Oublié]] | ennemi de — *secret* |")
	if strings.Contains(odon, "resume:") || strings.Contains(odon, "membre de") {
		t.Errorf("PNJ : l'exergue ou l'appartenance à la faction en double\n%s", odon)
	}
	contient(t, "faction", n["01 - Monde/Factions/Marais (Faction).md"], "## Membres\n\n- [[Maître Odon le Vieux|Maître Odon : le Vieux]] (trésorier)",
		"attitude_envers_pj: \"hostile\"", "> Tout noyer.")
	contient(t, "lieu", n["01 - Monde/Lieux/Marais.md"], "![[Marais - 1.png]]\n*Vu du ciel*", `catégorie: "site"`,
		"| [[Maître Odon le Vieux\\|Maître Odon : le Vieux]] | y est né |")
	contient(t, "quête", n["05 - Campagnes/La Boue/Quêtes/Arc principal/01 - Le pont.md"], "ordre: 1", "## Étapes\n\n- [x] Voir le pont\n- [ ] Le réparer",
		"## Enjeux & conséquences\n\nLe village est coupé.")
	campagne := n["05 - Campagnes/La Boue/La Boue.md"]
	contient(t, "campagne", campagne, "## Thèmes & ton\n\n- boue\n- pluie", `date_debut: "5 Vendémiaire 1216"`)
	if strings.Contains(campagne, "Le maire ment") {
		t.Error("les fils rouges sont dans la note de la campagne")
	}
	contient(t, "fils rouges", n["05 - Campagnes/La Boue/Fils rouges.md"], "Le maire ment.", `campagne: "[[La Boue]]"`)
	contient(t, "séance", n["05 - Campagnes/La Boue/Séances/Séance 1.md"], "## Combats\n\n- [[2025-10-09 Les rats]]")
	contient(t, "rencontre", n["05 - Campagnes/Sans campagne/Rencontres/Les rats.md"], "## Créatures\n\n- 3 × Crapaud des tourbes", "## Combats")
	contient(t, "combat", n["05 - Campagnes/La Boue/Combats/2025-10-09 Les rats.md"], "# Les rats — Victoire", `séance: "[[Séance 1]]"`,
		"**Héros du combat : Ysolde** — 3 rats", "| Ysolde | La \\| Lame | 12 | 0 | 3/4 | 0 | 0 |", "PV du groupe : R1 100 % → Fin 45 %")
	contient(t, "chronologie", n["01 - Monde/Chronologie.md"], "| 1 Frimaire 1215 | [[La vanne]] | [[La vanne]] : Le maire l'ouvre. |  |\n| 2 Frimaire",
		"| 2 Frimaire 1215 | [[La crue]] | [[La crue]] : Le fleuve \\| déborde. | [[Maître Odon le Vieux\\|Maître Odon : le Vieux]], [[Marais]] |")
	contient(t, "journal", n["Journal.md"], "| [[Maître Odon le Vieux\\|Maître Odon : le Vieux]] |", "| MJ | modification |")
	contient(t, "archive", n["Archives/PNJ/L'Oublié.md"], "archivé: true")
}

// Export des joueurs : un lien vers une fiche absente devient du texte.
func TestNotesJoueurs(t *testing.T) {
	u := univers()
	u.MJ = false
	odon := notes(t, u)["02 - PNJ/Maître Odon le Vieux.md"]
	contient(t, "PNJ", odon, "Voir [[Marais|le marais]] et Inconnu.")
}

func TestEcrire(t *testing.T) {
	var b bytes.Buffer
	if err := Ecrire(&b, "Mon/univers", univers()); err != nil {
		t.Fatal(err)
	}
	z, err := zip.NewReader(bytes.NewReader(b.Bytes()), int64(b.Len()))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range z.File {
		if !strings.HasPrefix(f.Name, "Mon univers/") {
			t.Fatalf("%s hors du dossier racine", f.Name)
		}
	}
}
