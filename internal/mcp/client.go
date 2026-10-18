package mcp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client parle à l'API de COF DM (application de bureau ou serveur) avec un jeton d'API.
type Client struct {
	Base    string // http://127.0.0.1:8100
	Jeton   string
	Univers string // en mode serveur : l'univers visé (en-tête X-COF-Univers), facultatif
	http    *http.Client
}

// NouveauClient : un client de l'API.
func NouveauClient(base, jeton, univers string) *Client {
	return &Client{Base: strings.TrimRight(base, "/"), Jeton: jeton, Univers: univers,
		http: &http.Client{Timeout: time.Minute}}
}

// tailleReponseMax : une réponse de l'API plus lourde n'est pas lue (export complet, images).
const tailleReponseMax = 32 << 20

// appel envoie une requête et rend le corps JSON de la réponse. Un refus de l'API devient une
// erreur qui reprend son message (« detail »), pour que l'assistant comprenne et corrige.
func (c *Client) appel(methode, chemin string, params url.Values, corps any) (json.RawMessage, error) {
	u := c.Base + chemin
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	var lecteur io.Reader
	if corps != nil {
		b, err := json.Marshal(corps)
		if err != nil {
			return nil, err
		}
		lecteur = bytes.NewReader(b)
	}
	r, err := http.NewRequest(methode, u, lecteur)
	if err != nil {
		return nil, err
	}
	if c.Jeton != "" {
		r.Header.Set("Authorization", "Bearer "+c.Jeton)
	}
	r.Header.Set("X-COF-Auteur", "claude")
	if c.Univers != "" {
		r.Header.Set("X-COF-Univers", c.Univers)
	}
	if corps != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	rep, err := c.http.Do(r)
	if err != nil {
		return nil, fmt.Errorf("COF DM ne répond pas à %s : l'application est-elle lancée ? (%v)", c.Base, err)
	}
	defer rep.Body.Close()
	b, err := io.ReadAll(io.LimitReader(rep.Body, tailleReponseMax+1))
	if err != nil {
		return nil, err
	}
	if len(b) > tailleReponseMax {
		return nil, errors.New("réponse de l'API trop lourde")
	}
	if rep.StatusCode >= 400 {
		var e struct {
			Detail string `json:"detail"`
		}
		_ = json.Unmarshal(b, &e)
		switch {
		case rep.StatusCode == 401:
			return nil, fmt.Errorf("refus (401) : jeton d'API absent ou invalide. %s", e.Detail)
		case e.Detail != "":
			return nil, fmt.Errorf("refus (%d) : %s", rep.StatusCode, e.Detail)
		default:
			return nil, fmt.Errorf("refus (%d)", rep.StatusCode)
		}
	}
	if len(bytes.TrimSpace(b)) == 0 {
		return json.RawMessage(`{"ok": true}`), nil
	}
	return b, nil
}
