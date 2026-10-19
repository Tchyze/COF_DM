package serveur

import (
	"net/http"
	"sort"
	"strings"

	"github.com/Tchyze/cof_dm/internal/store"
)

// Préparation d'une séance : ce que le MJ doit avoir en tête avant de jouer, rassemblé depuis la
// campagne de la séance. Rien n'est écrit ici ; l'écran compose la préparation et l'enregistre dans
// la fiche de la séance comme toute modification.

type quetePrep struct {
	Fiche         store.Resume `json:"fiche"`
	Statut        string       `json:"statut"`
	Categorie     string       `json:"categorie,omitempty"`
	Objectif      string       `json:"objectif,omitempty"`
	EtapesFaites  int          `json:"etapes_faites"`
	EtapesReste   []string     `json:"etapes_restantes"`
	Indices       string       `json:"indices,omitempty"`
	Lieu          string       `json:"lieu,omitempty"`
	Commanditaire string       `json:"commanditaire,omitempty"`
}

type secretPrep struct {
	Fiche      store.Resume `json:"fiche"`
	Resume     string       `json:"resume,omitempty"`
	Statut     string       `json:"statut"`
	Importance string       `json:"importance,omitempty"`
	Indices    string       `json:"indices,omitempty"`
	Ignorent   []string     `json:"ignorent"` // PJ en jeu de la campagne qui ne le savent pas
}

type rencontrePrep struct {
	rencontreTable
	Quete string `json:"quete,omitempty"`
}

type seancePrecedente struct {
	Fiche  store.Resume `json:"fiche"`
	Resume string       `json:"resume,omitempty"`
}

func (s *Serveur) routesPreparation() {
	s.mux.HandleFunc("GET /api/entites/{id}/preparation", s.preparation)
}

var rangImportance = map[string]int{"capital": 0, "important": 1, "détail": 2, "": 3}

func texteDe(e store.Entite, cle string) string {
	v, _ := e.Data[cle].(string)
	return strings.TrimSpace(v)
}

func resumeDe(e store.Entite) store.Resume {
	return store.Resume{ID: e.ID, Type: e.Type, Nom: e.Nom, Image: e.Image, Modifie: e.Modifie}
}

// preparation : GET /api/entites/{id}/preparation, pour une séance.
func (s *Serveur) preparation(w http.ResponseWriter, r *http.Request) {
	id, _ := idDe(r)
	seance, err := s.store.Lire(id)
	if err != nil || seance.Type != "seance" {
		erreur(w, 404, "Séance introuvable.")
		return
	}
	out := map[string]any{"seance": resumeDe(seance), "campagne": nil, "precedente": nil, "lieu_groupe": nil, "date": "",
		"fils_rouges": "", "pj": []pjTable{}, "quetes": []quetePrep{}, "quetes_a_venir": []store.Resume{},
		"secrets": []secretPrep{}, "pnj": []ficheScene{}, "rencontres": []rencontrePrep{}}
	camp := premierID(seance.Data["campagne"])
	c, err := s.complete(camp)
	if err != nil || c.Type != "campagne" {
		repondre(w, 200, out) // une séance sans campagne : rien à rassembler
		return
	}
	out["campagne"] = resumeDe(c.Entite)
	out["fils_rouges"] = texteDe(c.Entite, "fils_rouges")
	out["date"] = c.Dates["date_actuelle"]
	lieuGroupe := premierID(c.Data["lieu_groupe"])
	if l, err := s.store.Lire(lieuGroupe); err == nil {
		out["lieu_groupe"] = resumeDe(l)
	}
	pjs := s.pjDeTable(camp)
	out["pj"] = pjs

	// la séance jouée juste avant celle-ci : le plus grand numéro inférieur au sien, sinon (sans
	// numéro) la séance jouée modifiée en dernier
	seances, _ := s.store.Lister(store.Filtre{Type: "seance", Campagne: camp})
	n := numeroEntite(seance)
	var prec *store.Entite
	for _, x := range seances {
		if x.ID == seance.ID || x.Carte["statut"] != "jouée" {
			continue
		}
		e, err := s.store.Lire(x.ID)
		if err != nil || e.Archive || (n > 0 && numeroEntite(e) >= n) {
			continue
		}
		if prec == nil || numeroEntite(e) > numeroEntite(*prec) || (numeroEntite(e) == numeroEntite(*prec) && e.Modifie > prec.Modifie) {
			prec = &e
		}
	}
	if prec != nil {
		out["precedente"] = seancePrecedente{Fiche: resumeDe(*prec), Resume: texteDe(*prec, "resume")}
	}

	// quêtes de la campagne : en cours (détaillées), à venir (nommées)
	quetes, _ := s.store.Lister(store.Filtre{Type: "quete", Campagne: camp})
	enCours, aVenir := []quetePrep{}, []store.Resume{}
	scene := []ficheScene{}
	vuPnj := map[int64]bool{}
	ajouterPnj := func(f ficheScene) {
		if f.Fiche.Type != "pnj" || vuPnj[f.Fiche.ID] || f.Statut == "mort" {
			return
		}
		vuPnj[f.Fiche.ID] = true
		scene = append(scene, f)
	}
	for _, x := range quetes {
		switch x.Carte["statut"] {
		case "à venir":
			aVenir = append(aVenir, x)
			continue
		case "en cours":
		default:
			continue
		}
		q, err := s.complete(x.ID)
		if err != nil || q.Archive {
			continue
		}
		p := quetePrep{Fiche: x, Statut: "en cours", Categorie: texteDe(q.Entite, "categorie"), Objectif: texteDe(q.Entite, "objectif"),
			Indices: texteDe(q.Entite, "indices"), EtapesReste: []string{}}
		if l, ok := q.Data["etapes"].([]any); ok {
			for _, e := range l {
				m, _ := e.(map[string]any)
				if fait, _ := m["fait"].(bool); fait {
					p.EtapesFaites++
				} else if t, _ := m["texte"].(string); strings.TrimSpace(t) != "" {
					p.EtapesReste = append(p.EtapesReste, strings.TrimSpace(t))
				}
			}
		}
		if l, err := s.store.Lire(premierID(q.Data["lieu"])); err == nil {
			p.Lieu = l.Nom
		}
		if cm, err := s.store.Lire(premierID(q.Data["commanditaire"])); err == nil {
			p.Commanditaire = cm.Nom
		}
		enCours = append(enCours, p)
		_, fiches := s.rencontresDeTable(q.ID)
		for _, f := range s.sceneDeTable(q, fiches) {
			ajouterPnj(f)
		}
	}
	sort.SliceStable(enCours, func(i, j int) bool {
		return (enCours[i].Categorie == "arc principal") && enCours[j].Categorie != "arc principal"
	})
	out["quetes"], out["quetes_a_venir"] = enCours, aVenir

	// PNJ là où se trouve le groupe (et dans ses sous-lieux directs)
	if lieuGroupe != 0 {
		lieux := map[int64]bool{lieuGroupe: true}
		sous, _ := s.store.Lister(store.Filtre{Type: "lieu"})
		for _, l := range sous {
			if premierID(l.Carte["parent"]) == lieuGroupe {
				lieux[l.ID] = true
			}
		}
		pnjs, _ := s.store.Lister(store.Filtre{Type: "pnj"})
		for _, x := range pnjs {
			if !lieux[premierID(x.Carte["lieu"])] {
				continue
			}
			e, err := s.store.Lire(x.ID)
			if err != nil || e.Archive {
				continue
			}
			statut, _ := e.Data["statut"].(string)
			ajouterPnj(ficheScene{Fiche: x, Resume: apercu(e), Statut: statut, Raison: "là où est le groupe", Secret: aSecret(e)})
		}
	}
	out["pnj"] = scene

	// secrets de la campagne qui ne sont pas encore révélés, les plus importants d'abord
	secrets, _ := s.store.Lister(store.Filtre{Type: "secret", Campagne: camp})
	sp := []secretPrep{}
	for _, x := range secrets {
		if x.Carte["statut"] == "révélé" {
			continue
		}
		e, err := s.complete(x.ID)
		if err != nil || e.Archive {
			continue
		}
		savent := map[int64]bool{}
		for _, rel := range e.Relations {
			if rel.Type == "sait" {
				savent[rel.Autre.ID] = true
			}
		}
		p := secretPrep{Fiche: x, Resume: texteDe(e.Entite, "resume"), Statut: texteDe(e.Entite, "statut"),
			Importance: texteDe(e.Entite, "importance"), Indices: texteDe(e.Entite, "indices"), Ignorent: []string{}}
		for _, pj := range pjs {
			if pj.Statut != "absent" && !savent[pj.Fiche.ID] {
				p.Ignorent = append(p.Ignorent, pj.Fiche.Nom)
			}
		}
		sp = append(sp, p)
	}
	sort.SliceStable(sp, func(i, j int) bool { return rangImportance[sp[i].Importance] < rangImportance[sp[j].Importance] })
	out["secrets"] = sp

	// rencontres préparées : celles de la campagne, et celles de ses quêtes en cours
	dansQuete := map[int64]string{}
	for _, q := range enCours {
		dansQuete[q.Fiche.ID] = q.Fiche.Nom
	}
	toutes, _ := s.store.Lister(store.Filtre{Type: "rencontre"})
	rp := []rencontrePrep{}
	for _, x := range toutes {
		if st, _ := x.Carte["statut"].(string); st == "jouée" || st == "évitée" {
			continue
		}
		e, err := s.store.Lire(x.ID)
		if err != nil || e.Archive {
			continue
		}
		quete, deLaQuete := dansQuete[premierID(e.Data["quete"])]
		if premierID(e.Data["campagne"]) != camp && !deLaQuete {
			continue
		}
		t := rencontreTable{Fiche: x, Resume: apercu(e), Creatures: creaturesEnClair(e)}
		if l, err := s.store.Lire(premierID(e.Data["lieu"])); err == nil {
			t.Lieu = l.Nom
		}
		rp = append(rp, rencontrePrep{rencontreTable: t, Quete: quete})
	}
	out["rencontres"] = rp
	// une fonctionnalité coupée ne remonte pas dans la préparation
	if !s.store.Active("secrets") {
		out["secrets"] = []secretPrep{}
	}
	if !s.store.Active("rencontres") {
		out["rencontres"] = []rencontrePrep{}
	}
	repondre(w, 200, out)
}

func numeroEntite(e store.Entite) float64 { n, _ := e.Data["numero"].(float64); return n }
