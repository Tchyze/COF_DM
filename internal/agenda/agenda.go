// Package agenda calcule l'agenda des séances : les dates RÉELLES où la table se retrouve (le
// calendrier du monde, lui, est dans internal/modele/calendrier.go). Il ne lit aucune base : le
// serveur lui donne les créneaux, il rend les conflits et le fichier iCalendar (.ics).
package agenda

import (
	"sort"
	"time"

	"github.com/Tchyze/cof_dm/internal/modele"
)

// EcartMinimal : deux séances d'une même personne plus rapprochées que cela font l'objet d'un
// message (sans se chevaucher).
const EcartMinimal = time.Hour

// Genres de conflit.
const (
	Chevauchement = "chevauchement" // les deux séances se chevauchent : alerte
	Rapproche     = "rapproche"     // moins d'une heure entre la fin de l'une et le début de l'autre : message
)

// Creneau : une séance placée dans le temps réel.
type Creneau struct {
	ID         int64
	Debut, Fin time.Time
	JourEntier bool     // sans heure de début : toute la journée
	Personnes  []string // qui y est attendu (clés stables : un compte, un joueur, le MJ)
}

// Conflit : une autre séance qui gêne celle-ci, et les personnes concernées.
type Conflit struct {
	Autre     int64    `json:"seance"`
	Genre     string   `json:"genre"`
	Personnes []string `json:"personnes"`
}

// Placer calcule le créneau d'une séance à partir de ses champs (date AAAA-MM-JJ, heure HH:MM,
// durée du menu) dans le fuseau donné. ok est faux sans date lisible.
func Placer(date, heure, duree string, fuseau *time.Location) (debut, fin time.Time, jourEntier, ok bool) {
	jour, err := time.ParseInLocation("2006-01-02", date, fuseau)
	if err != nil {
		return time.Time{}, time.Time{}, false, false
	}
	h, okH := modele.LireHeure(heure)
	if !okH {
		return jour, jour.AddDate(0, 0, 1), true, true
	}
	debut, err = time.ParseInLocation("2006-01-02 15:04", date+" "+h, fuseau)
	if err != nil {
		return time.Time{}, time.Time{}, false, false
	}
	minutes, okD := modele.MinutesDuree(duree)
	if !okD {
		minutes = modele.DureeParDefaut
	}
	// une durée s'ajoute en temps écoulé : une séance de 4 h la nuit du passage à l'heure d'hiver
	// finit bien 4 h plus tard
	return debut, debut.Add(time.Duration(minutes) * time.Minute), false, true
}

// Conflits trouve, pour chaque créneau, les autres créneaux qui partagent une personne et qui le
// chevauchent ou en sont trop proches. Le résultat est trié (séance, puis genre) et stable.
func Conflits(creneaux []Creneau) map[int64][]Conflit {
	out := map[int64][]Conflit{}
	for i := range creneaux {
		for j := i + 1; j < len(creneaux); j++ {
			a, b := creneaux[i], creneaux[j]
			communes := communes(a.Personnes, b.Personnes)
			if len(communes) == 0 {
				continue
			}
			genre := Relation(a, b)
			if genre == "" {
				continue
			}
			out[a.ID] = append(out[a.ID], Conflit{Autre: b.ID, Genre: genre, Personnes: communes})
			out[b.ID] = append(out[b.ID], Conflit{Autre: a.ID, Genre: genre, Personnes: communes})
		}
	}
	for id := range out {
		l := out[id]
		sort.Slice(l, func(i, j int) bool {
			if l[i].Genre != l[j].Genre {
				return l[i].Genre == Chevauchement
			}
			return l[i].Autre < l[j].Autre
		})
	}
	return out
}

// Relation : « chevauchement », « rapproche » ou "" entre deux créneaux, sans regarder qui y est.
func Relation(a, b Creneau) string {
	if a.Debut.Before(b.Fin) && b.Debut.Before(a.Fin) {
		return Chevauchement
	}
	if a.JourEntier || b.JourEntier {
		return ""
	}
	ecart := b.Debut.Sub(a.Fin)
	if b.Debut.Before(a.Debut) {
		ecart = a.Debut.Sub(b.Fin)
	}
	if ecart < EcartMinimal {
		return Rapproche
	}
	return ""
}

func communes(a, b []string) []string {
	dans := map[string]bool{}
	for _, x := range a {
		dans[x] = true
	}
	var out []string
	vues := map[string]bool{}
	for _, x := range b {
		if dans[x] && !vues[x] {
			vues[x] = true
			out = append(out, x)
		}
	}
	sort.Strings(out)
	return out
}
