package serveur

import (
	"bytes"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// journalDe : ce que le test écrit au journal du serveur.
func journalDe(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	return &buf
}

func lignesSecurite(buf *bytes.Buffer) []string {
	var l []string
	for _, x := range strings.Split(buf.String(), "\n") {
		if i := strings.Index(x, "securite "); i >= 0 {
			l = append(l, x[i:])
		}
	}
	return l
}

func contient(l []string, attendu string) bool {
	for _, x := range l {
		if x == attendu {
			return true
		}
	}
	return false
}

func TestJournalConnexions(t *testing.T) {
	buf := journalDe(t)
	b, _ := bancServeur(t)
	b.s.acces.pasNom = 0
	depuis := func(ip string) func(*http.Request) {
		return func(r *http.Request) { r.Header.Set("X-Forwarded-For", ip) }
	}
	essai := func(ip, nom, mdp string) int {
		c, _, _ := b.proxy("POST", "/api/acces/connexion", map[string]any{"nom": nom, "mdp": mdp}, depuis(ip))
		return c
	}
	essai("203.0.113.9", "meneur", "mot-de-passe-ruenem")
	essai("203.0.113.10", "Meneur", "MDP-FAUX-A-NE-PAS-VOIR")
	essai("203.0.113.11", "inconnu", "MDP-FAUX-A-NE-PAS-VOIR")
	for i := 0; i < essaisMax; i++ {
		essai("203.0.113.12", "audit", "MDP-FAUX-A-NE-PAS-VOIR")
	}
	l := lignesSecurite(buf)
	for _, attendu := range []string{
		`securite evenement=connexion_ok nom="meneur" ip=203.0.113.9`,
		`securite evenement=connexion_echec nom="meneur" ip=203.0.113.10`,
		`securite evenement=connexion_echec nom="inconnu" ip=203.0.113.11`,
		`securite evenement=connexion_echec nom="audit" ip=203.0.113.12`,
	} {
		if !contient(l, attendu) {
			t.Errorf("manque %s dans\n%s", attendu, strings.Join(l, "\n"))
		}
	}
	if c := essai("203.0.113.12", "audit", "MDP-FAUX-A-NE-PAS-VOIR"); c != 429 {
		t.Fatalf("limite : %d", c)
	}
	if l = lignesSecurite(buf); !contient(l, `securite evenement=limite_atteinte portee=couple nom="audit" ip=203.0.113.12`) {
		t.Errorf("limite non notée :\n%s", strings.Join(l, "\n"))
	}
	if strings.Contains(buf.String(), "MDP-FAUX") || strings.Contains(buf.String(), "mot-de-passe-") {
		t.Fatalf("un mot de passe au journal :\n%s", buf)
	}
}

func TestJournalLimitesAdresseEtNom(t *testing.T) {
	buf := journalDe(t)
	b, _ := bancServeur(t)
	b.s.acces.pasNom = 0
	for i := 0; i <= essaisMaxAdresse; i++ {
		essaiDepuis(b, "198.51.100.1", "nom"+strconv.Itoa(i), "faux")
	}
	if l := lignesSecurite(buf); !contient(l, `securite evenement=limite_atteinte portee=adresse nom="nom`+strconv.Itoa(essaisMaxAdresse)+`" ip=198.51.100.1`) {
		t.Errorf("limite d'adresse non notée :\n%s", strings.Join(l, "\n"))
	}
	buf.Reset()
	for i := 0; i <= essaisSeuilNom; i++ {
		essaiDepuis(b, "198.51.100."+strconv.Itoa(i+2), "meneur", "faux")
	}
	if l := lignesSecurite(buf); !contient(l, `securite evenement=limite_atteinte portee=nom nom="meneur" ip=198.51.100.`+strconv.Itoa(essaisSeuilNom+2)) {
		t.Errorf("ralentissement non noté :\n%s", strings.Join(l, "\n"))
	}
}

// Un nom saisi ne peut ni écrire une ligne de plus, ni imiter un autre événement ou une autre adresse.
func TestJournalInjection(t *testing.T) {
	buf := journalDe(t)
	b, _ := bancServeur(t)
	for _, nom := range []string{"a\nsecurite evenement=connexion_ok", "a\r\nb", "a\" ip=9.9.9.9", "é\x1b[31m\x00z"} {
		essaiDepuis(b, "203.0.113.9", nom, "faux")
	}
	lignes := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lignes) != 4 {
		t.Fatalf("%d lignes :\n%s", len(lignes), buf)
	}
	format := regexp.MustCompile(`securite evenement=connexion_echec nom="(?:[^"\\\x00-\x1f]|\\.)*" ip=203\.0\.113\.9$`)
	for _, x := range lignes {
		if !format.MatchString(x) {
			t.Errorf("ligne hors format : %q", x)
		}
	}
}

// Jetons et mots de passe : l'événement est noté, sans le secret.
func TestJournalCompte(t *testing.T) {
	buf := journalDe(t)
	b, _ := bancServeur(t)
	mj := b.connecter("meneur")
	depuis := func(r *http.Request) { mj(r); r.Header.Set("X-Forwarded-For", "203.0.113.20") }
	_, v, _ := b.proxy("POST", "/api/compte/jetons", map[string]any{"nom": "un libellé\nlibre"}, depuis)
	clair, id := v["jeton"].(string), v["id"].(string)
	_, v, _ = b.proxy("POST", "/api/compte/jetons", map[string]any{"nom": "second"}, depuis)
	second := v["jeton"].(string)
	b.proxy("DELETE", "/api/compte/jetons/"+id, nil, depuis)
	b.proxy("DELETE", "/api/compte/jeton", nil, func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer "+second)
		r.Header.Set("X-Forwarded-For", "203.0.113.21")
	})
	b.proxy("POST", "/api/compte/mdp", map[string]any{"ancien": "FAUX-ANCIEN-MDP", "nouveau": "NOUVEAU-MDP-SECRET"}, depuis)
	b.proxy("POST", "/api/compte/mdp", map[string]any{"ancien": "mot-de-passe-ruenem", "nouveau": "NOUVEAU-MDP-SECRET", "retirer_jetons": true}, depuis)
	l := lignesSecurite(buf)
	for _, attendu := range []string{
		`securite evenement=jeton_cree jeton=` + id + ` nom="meneur" ip=203.0.113.20`,
		`securite evenement=jeton_retire jeton="` + id + `" nom="meneur" ip=203.0.113.20`,
		`securite evenement=jeton_rendu nom="meneur" ip=203.0.113.21`,
		`securite evenement=mdp_echec nom="meneur" ip=203.0.113.20`,
		`securite evenement=mdp_change jetons_retires=0 nom="meneur" ip=203.0.113.20`,
	} {
		if !contient(l, attendu) {
			t.Errorf("manque %s dans\n%s", attendu, strings.Join(l, "\n"))
		}
	}
	for _, secret := range []string{clair, second, "NOUVEAU-MDP-SECRET", "FAUX-ANCIEN-MDP", "libellé"} {
		if strings.Contains(buf.String(), secret) {
			t.Fatalf("%q au journal :\n%s", secret, buf)
		}
	}
}

// La réinitialisation par l'administrateur dit qui l'a faite et combien de jetons sont tombés.
func TestJournalReinitialisation(t *testing.T) {
	a, reg, connecter := serveurMulti(t)
	_ = a
	reg.Creer("cible", "mot-de-passe-elbic", false)
	reg.NouveauJeton("cible", "poste")
	chef := connecter("chef")
	buf := journalDe(t)
	if c, _ := chef.faire("PATCH", "/api/admin/comptes/cible", map[string]any{"mdp": "NOUVEAU-MDP-SECRET"}, "X-Forwarded-For", "203.0.113.30"); c != 200 {
		t.Fatalf("%d", c)
	}
	if l := lignesSecurite(buf); !contient(l, `securite evenement=mdp_reinitialise par="chef" jetons_retires=1 nom="cible" ip=203.0.113.30`) {
		t.Errorf("réinitialisation non notée :\n%s", strings.Join(l, "\n"))
	}
	if strings.Contains(buf.String(), "NOUVEAU-MDP-SECRET") {
		t.Fatalf("mot de passe au journal :\n%s", buf)
	}
}
