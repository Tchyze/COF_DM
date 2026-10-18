package serveur

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/Tchyze/cof_dm/internal/store"
)

// Après la séance : le MJ reporte en une fois ce qui s'est passé (résumé, PJ présents, date en jeu,
// lieu du groupe, étapes et statut des quêtes, secrets appris, PNJ morts, rencontres jouées) et
// prépare la séance suivante. Tout est contrôlé avant la première écriture : une valeur refusée
// n'en laisse passer aucune. Chaque écriture est ensuite journalisée comme une saisie.

type apresQuete struct {
	ID     int64  `json:"id"`
	Etapes []bool `json:"etapes"` // « fait » de chaque étape, dans l'ordre de la fiche
	Statut string `json:"statut"`
}

type apresSecret struct {
	ID   int64   `json:"id"`
	Qui  []int64 `json:"qui"`
	Note string  `json:"note"`
}

type apresRencontre struct {
	ID     int64  `json:"id"`
	Statut string `json:"statut"`
}

type demandeApres struct {
	Resume     *string          `json:"resume"`
	Presents   []int64          `json:"presents"`
	DateJeu    any              `json:"date_jeu"`
	LieuGroupe int64            `json:"lieu_groupe"`
	Quetes     []apresQuete     `json:"quetes"`
	Secrets    []apresSecret    `json:"secrets"`
	Morts      []int64          `json:"morts"`
	Rencontres []apresRencontre `json:"rencontres"`
	Suivante   bool             `json:"suivante"`
}

func (s *Serveur) routesApres() {
	s.mux.HandleFunc("POST /api/entites/{id}/apres-seance", s.apresSeance)
}

var reNumeroFinal = regexp.MustCompile(`^(.*?)(\d+)\s*$`)

// nomSuivant : « Séance 4 » → « Séance 5 » ; sans numéro dans le nom, « Séance <n> ».
func nomSuivant(nom string, numero int) string {
	if m := reNumeroFinal.FindStringSubmatch(nom); m != nil {
		n, _ := strconv.Atoi(m[2])
		return m[1] + strconv.Itoa(n+1)
	}
	return "Séance " + strconv.Itoa(numero)
}

func (s *Serveur) apresSeance(w http.ResponseWriter, r *http.Request) {
	id, _ := idDe(r)
	seance, err := s.store.Lire(id)
	if err != nil || seance.Type != "seance" {
		erreur(w, 404, "Séance introuvable.")
		return
	}
	var in demandeApres
	if err := lireJSON(r, &in); err != nil {
		erreurStore(w, err)
		return
	}
	camp := premierID(seance.Data["campagne"])

	// 1. ce qu'il faudra écrire, fiche par fiche
	type ecriture struct {
		id   int64
		data map[string]any
	}
	var ecritures []ecriture
	refus := func(format string, a ...any) {
		erreur(w, 422, fmt.Sprintf(format, a...))
	}
	majSeance := map[string]any{"statut": "jouée"}
	if in.Resume != nil {
		majSeance["resume"] = strings.TrimSpace(*in.Resume)
	}
	var presents []any // la validation des fiches lit des listes JSON
	for _, p := range in.Presents {
		presents = append(presents, float64(p))
	}
	if in.Presents != nil {
		majSeance["presents"] = append([]any{}, presents...)
	}
	if in.DateJeu != nil {
		majSeance["date_jeu"] = in.DateJeu
	}
	ecritures = append(ecritures, ecriture{id, majSeance})
	if camp != 0 {
		majCamp := map[string]any{}
		if in.DateJeu != nil {
			majCamp["date_actuelle"] = in.DateJeu
		}
		if in.LieuGroupe != 0 {
			majCamp["lieu_groupe"] = float64(in.LieuGroupe)
		}
		if len(majCamp) > 0 {
			ecritures = append(ecritures, ecriture{camp, majCamp})
		}
	} else if in.LieuGroupe != 0 {
		refus("La séance n'a pas de campagne : le lieu du groupe ne peut pas être noté.")
		return
	}
	for _, q := range in.Quetes {
		e, err := s.store.Lire(q.ID)
		if err != nil || e.Type != "quete" {
			refus("Quête n°%d introuvable.", q.ID)
			return
		}
		maj := map[string]any{}
		if q.Etapes != nil {
			actuelles, _ := e.Data["etapes"].([]any)
			if len(actuelles) != len(q.Etapes) {
				refus("Les étapes de « %s » ont changé entre-temps : rechargez la page.", e.Nom)
				return
			}
			etapes := make([]any, len(actuelles))
			for i, x := range actuelles {
				m, _ := x.(map[string]any)
				etapes[i] = map[string]any{"texte": m["texte"], "fait": q.Etapes[i]}
			}
			maj["etapes"] = etapes
		}
		if q.Statut != "" {
			maj["statut"] = q.Statut
		}
		if len(maj) > 0 {
			ecritures = append(ecritures, ecriture{q.ID, maj})
		}
	}
	for _, p := range in.Morts {
		e, err := s.store.Lire(p)
		if err != nil || e.Type != "pnj" {
			refus("PNJ n°%d introuvable.", p)
			return
		}
		ecritures = append(ecritures, ecriture{p, map[string]any{"statut": "mort"}})
	}
	for _, x := range in.Rencontres {
		e, err := s.store.Lire(x.ID)
		if err != nil || e.Type != "rencontre" {
			refus("Rencontre n°%d introuvable.", x.ID)
			return
		}
		st := x.Statut
		if st == "" {
			st = "jouée"
		}
		ecritures = append(ecritures, ecriture{x.ID, map[string]any{"statut": st, "seance": float64(id)}})
	}
	for _, x := range in.Secrets {
		e, err := s.store.Lire(x.ID)
		if err != nil || e.Type != "secret" {
			refus("Secret n°%d introuvable.", x.ID)
			return
		}
		for _, q := range x.Qui {
			if t, ok := s.store.TypeDeFiche(q); !ok || (t != "pj" && t != "pnj" && t != "faction") {
				refus("« %s » : seuls un PJ, un PNJ ou une faction apprennent un secret.", e.Nom)
				return
			}
		}
	}

	// 2. tout est contrôlé avant d'écrire quoi que ce soit
	for _, e := range ecritures {
		if err := s.store.Verifier(e.id, e.data); err != nil {
			erreurStore(w, err)
			return
		}
	}

	// 3. les écritures
	auteur := auteurDe(r)
	for _, e := range ecritures {
		if _, err := s.store.Modifier(e.id, store.Maj{Data: e.data}, auteur); err != nil {
			erreurStore(w, err)
			return
		}
	}
	revelations := 0
	for _, x := range in.Secrets {
		if len(x.Qui) == 0 {
			continue
		}
		n, _, err := s.revelerSecret(x.ID, x.Qui, id, x.Note, auteur)
		if err != nil {
			erreurStore(w, err)
			return
		}
		revelations += n
	}
	out := map[string]any{"seance": id, "modifiees": len(ecritures), "revelations": revelations, "suivante": nil}
	if in.Suivante {
		numero := int(numeroEntite(seance)) + 1
		data := map[string]any{"statut": "prévue"}
		if camp != 0 {
			data["campagne"] = camp
		}
		if numeroEntite(seance) > 0 {
			data["numero"] = numero
		}
		if in.DateJeu != nil {
			data["date_jeu"] = in.DateJeu
		}
		if in.Presents != nil {
			data["presents"] = append([]any{}, presents...)
		}
		n, err := s.store.Creer("seance", nomSuivant(seance.Nom, max(numero, 2)), data, nil, auteur)
		if err != nil {
			erreurStore(w, err)
			return
		}
		out["suivante"] = n.ID
	}
	repondre(w, 200, out)
}
