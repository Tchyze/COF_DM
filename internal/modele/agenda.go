package modele

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // les fuseaux horaires, même sur un Windows ou une image Docker qui n'en ont pas
)

// Agenda des séances : les dates RÉELLES où la table se retrouve (à ne pas confondre avec le
// calendrier du monde, calendrier.go). Une séance porte sa date réelle, son heure de début et sa
// durée ; le fuseau horaire de l'univers dit dans quelle heure locale elles sont écrites.

// NaturesSeance : ce qu'une fiche « séance » planifie (vide : une séance de jeu).
var NaturesSeance = []string{"séance", "séance zéro", "création de personnages", "autre"}

// DureesSeance : les durées proposées, de 30 minutes en 30 minutes, jusqu'à 12 heures.
var DureesSeance = func() []string {
	var l []string
	for m := 30; m <= 12*60; m += 30 {
		l = append(l, TexteDuree(m))
	}
	return l
}()

// DureeParDefaut : celle d'une séance dont la durée n'est pas précisée (en minutes).
const DureeParDefaut = 4 * 60

// FuseauParDefaut : celui d'un univers qui n'en a pas choisi.
const FuseauParDefaut = "Europe/Paris"

// TexteDuree écrit une durée comme le menu la propose : « 30 min », « 1 h », « 1 h 30 ».
func TexteDuree(minutes int) string {
	h, m := minutes/60, minutes%60
	switch {
	case h == 0:
		return fmt.Sprintf("%d min", m)
	case m == 0:
		return fmt.Sprintf("%d h", h)
	}
	return fmt.Sprintf("%d h %02d", h, m)
}

var reDuree = regexp.MustCompile(`^(?:(\d+) h(?: (\d+))?|(\d+) min)$`)

// MinutesDuree lit une durée du menu (0, false : vide ou illisible).
func MinutesDuree(s string) (int, bool) {
	m := reDuree.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0, false
	}
	if m[3] != "" {
		n, _ := strconv.Atoi(m[3])
		return n, n > 0
	}
	h, _ := strconv.Atoi(m[1])
	mn, _ := strconv.Atoi(m[2])
	return h*60 + mn, h*60+mn > 0
}

var reHeure = regexp.MustCompile(`^([01]?\d|2[0-3])[:hH]([0-5]\d)$`)

// LireHeure lit « 20:30 » (ou « 20h30 ») et la rend en « HH:MM ».
func LireHeure(s string) (string, bool) {
	m := reHeure.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return "", false
	}
	h, _ := strconv.Atoi(m[1])
	return fmt.Sprintf("%02d:%s", h, m[2]), true
}

// Fuseau charge un fuseau horaire (nom IANA : « Europe/Paris ») ; vide : celui par défaut.
func Fuseau(nom string) (*time.Location, error) {
	if strings.TrimSpace(nom) == "" {
		nom = FuseauParDefaut
	}
	if !strings.Contains(nom, "/") && nom != "UTC" {
		return nil, fmt.Errorf("fuseau horaire inconnu : %q (exemple : Europe/Paris)", nom)
	}
	l, err := time.LoadLocation(nom)
	if err != nil {
		return nil, fmt.Errorf("fuseau horaire inconnu : %q (exemple : Europe/Paris)", nom)
	}
	return l, nil
}

// ReponsesSeance : ce qu'un joueur répond à une séance prévue.
var ReponsesSeance = []string{"présent", "peut-être", "absent"}

// ReponseJoueur : la réponse d'un joueur (pour son PJ) à une séance.
type ReponseJoueur struct {
	PJ      int64   `json:"pj"`
	Reponse string  `json:"reponse"`
	Le      float64 `json:"le,omitempty"` // quand il a répondu (secondes Unix)
}

// LireReponses lit le champ « reponses » d'une séance (rangé ou fraîchement validé).
func LireReponses(v any) []ReponseJoueur {
	switch x := v.(type) {
	case []ReponseJoueur:
		return x
	case []any:
		var out []ReponseJoueur
		for _, y := range x {
			m, ok := y.(map[string]any)
			if !ok {
				continue
			}
			r := ReponseJoueur{}
			if n, ok := m["pj"].(float64); ok {
				r.PJ = int64(n)
			}
			r.Reponse, _ = m["reponse"].(string)
			r.Le, _ = m["le"].(float64)
			if r.PJ > 0 && r.Reponse != "" {
				out = append(out, r)
			}
		}
		return out
	}
	return nil
}

// reponses valide [{pj, reponse, le?}] : des PJ du monde, une réponse permise, une par PJ (la
// dernière l'emporte).
func reponses(v any, f Fiches) (any, error) {
	l, ok := v.([]any)
	if !ok {
		if rs, ok := v.([]ReponseJoueur); ok {
			return rs, nil
		}
		return nil, fmt.Errorf(`réponses attendues : [{"pj": 4, "reponse": "présent"}]`)
	}
	var out []ReponseJoueur
	place := map[int64]int{}
	for i, x := range l {
		m, ok := x.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("réponse %d illisible", i+1)
		}
		id, err := nombre(m["pj"])
		if err != nil || id <= 0 || id != math.Trunc(id) {
			return nil, fmt.Errorf("réponse %d : identifiant de PJ attendu", i+1)
		}
		if typ, ok := f.TypeDeFiche(int64(id)); !ok || typ != "pj" {
			return nil, fmt.Errorf("réponse %d : la fiche n°%d n'est pas un PJ", i+1, int64(id))
		}
		rep, _ := m["reponse"].(string)
		valide := false
		for _, o := range ReponsesSeance {
			if rep == o {
				valide = true
			}
		}
		if !valide {
			return nil, fmt.Errorf("réponse %d : %q n'est pas permise (%s)", i+1, rep, strings.Join(ReponsesSeance, ", "))
		}
		r := ReponseJoueur{PJ: int64(id), Reponse: rep}
		if le, err := nombre(m["le"]); err == nil && le > 0 {
			r.Le = le
		}
		if j, deja := place[r.PJ]; deja {
			out[j] = r
			continue
		}
		place[r.PJ] = len(out)
		out = append(out, r)
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}
