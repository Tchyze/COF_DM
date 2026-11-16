package agenda

import (
	"strings"
	"time"
	"unicode/utf8"
)

// Evenement : une séance telle qu'elle part dans un fichier iCalendar (RFC 5545). Seul ce qu'il faut
// pour s'organiser y entre : titre, horaires, lieu, lien de visio, participants, ce qui est dit aux
// joueurs. Jamais une note du MJ.
type Evenement struct {
	UID         string
	Titre       string
	Debut, Fin  time.Time
	JourEntier  bool
	Lieu        string
	URL         string
	Description string
	Annulee     bool
	Modifie     time.Time
}

// ICS écrit un calendrier iCalendar. Les heures partent en UTC : chaque agenda les convertit dans
// l'heure de qui le lit, heure d'été comprise.
func ICS(nom string, evenements []Evenement, maintenant time.Time) []byte {
	var b strings.Builder
	ligne := func(s string) { b.WriteString(plier(s)); b.WriteString("\r\n") }
	ligne("BEGIN:VCALENDAR")
	ligne("VERSION:2.0")
	ligne("PRODID:-//COF DM//Agenda des séances//FR")
	ligne("CALSCALE:GREGORIAN")
	ligne("METHOD:PUBLISH")
	ligne("X-WR-CALNAME:" + echapper(nom))
	// une indication aux agendas qui la suivent (Apple, Outlook) : relire toutes les heures
	ligne("REFRESH-INTERVAL;VALUE=DURATION:PT1H")
	ligne("X-PUBLISHED-TTL:PT1H")
	stamp := maintenant.UTC().Format("20060102T150405Z")
	for _, e := range evenements {
		ligne("BEGIN:VEVENT")
		ligne("UID:" + echapper(e.UID))
		ligne("DTSTAMP:" + stamp)
		if !e.Modifie.IsZero() {
			ligne("LAST-MODIFIED:" + e.Modifie.UTC().Format("20060102T150405Z"))
			// SEQUENCE croît à chaque modification : les agendas remplacent l'ancienne version
			ligne("SEQUENCE:" + itoa(e.Modifie.Unix()-1_700_000_000))
		}
		if e.JourEntier {
			ligne("DTSTART;VALUE=DATE:" + e.Debut.Format("20060102"))
			ligne("DTEND;VALUE=DATE:" + e.Fin.Format("20060102"))
		} else {
			ligne("DTSTART:" + e.Debut.UTC().Format("20060102T150405Z"))
			ligne("DTEND:" + e.Fin.UTC().Format("20060102T150405Z"))
		}
		titre := e.Titre
		if e.Annulee {
			titre = "ANNULÉE — " + titre
			ligne("STATUS:CANCELLED")
		} else {
			ligne("STATUS:CONFIRMED")
		}
		ligne("SUMMARY:" + echapper(titre))
		if e.Lieu != "" {
			ligne("LOCATION:" + echapper(e.Lieu))
		}
		if e.URL != "" {
			ligne("URL:" + echapper(e.URL))
		}
		if e.Description != "" {
			ligne("DESCRIPTION:" + echapper(e.Description))
		}
		ligne("TRANSP:OPAQUE")
		ligne("END:VEVENT")
	}
	ligne("END:VCALENDAR")
	return []byte(b.String())
}

func itoa(n int64) string {
	if n < 0 {
		n = 0
	}
	var t [20]byte
	i := len(t)
	for {
		i--
		t[i] = byte('0' + n%10)
		n /= 10
		if n == 0 {
			break
		}
	}
	return string(t[i:])
}

// echapper : le texte d'une propriété iCalendar (\ ; , et retours à la ligne).
func echapper(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	r := strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\n", `\n`)
	return r.Replace(s)
}

// plier coupe une ligne à 75 octets, sans couper un caractère : la suite commence par une espace.
func plier(s string) string {
	if len(s) <= 75 {
		return s
	}
	var b strings.Builder
	max := 75
	for len(s) > max {
		coupe := max
		for coupe > 0 && !utf8.RuneStart(s[coupe]) {
			coupe--
		}
		b.WriteString(s[:coupe])
		b.WriteString("\r\n ")
		s = s[coupe:]
		max = 74 // l'espace de tête compte
	}
	b.WriteString(s)
	return b.String()
}
