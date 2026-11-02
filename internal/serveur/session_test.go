package serveur

import (
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Un compte recréé sous le même nom n'est pas celui d'avant : l'ancienne session n'y entre pas.
func TestSessionCompteRecree(t *testing.T) {
	b, reg := bancServeur(t)
	ancien := b.connecter("meneur")
	if c, _, _ := b.proxy("GET", "/api/compte", nil, ancien); c != 200 {
		t.Fatalf("session ouverte : %d", c)
	}
	if err := reg.Supprimer("meneur"); err != nil {
		t.Fatal(err)
	}
	if c, _, _ := b.proxy("GET", "/api/compte", nil, ancien); c != 401 {
		t.Fatalf("compte supprimé : %d", c)
	}
	if _, err := reg.Creer("meneur", "autre-mot-de-passe", false); err != nil {
		t.Fatal(err)
	}
	if c, _, _ := b.proxy("GET", "/api/compte", nil, ancien); c != 401 {
		t.Fatalf("ancienne session sur le compte recréé : %d", c)
	}
}

// Une session d'avant la graine, pourtant bien signée, ne vaut plus rien.
func TestSessionAncienFormat(t *testing.T) {
	b, reg := bancServeur(t)
	c, _ := reg.Lire("meneur")
	id := base64.RawURLEncoding.EncodeToString(make([]byte, 9))
	v := b.s.acces.signer(strings.Join([]string{strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10), id, c.Nom, strconv.Itoa(c.Version)}, "."))
	code, _, _ := b.proxy("GET", "/api/compte", nil, func(r *http.Request) { r.AddCookie(&http.Cookie{Name: cookieMJ, Value: v}) })
	if code != 401 {
		t.Fatalf("session de l'ancien format : %d", code)
	}
}
