package serveur

import (
	"fmt"
	"net"
	"net/http"
	"strings"
)

// ProxiesParDefaut : un reverse proxy sur cette machine, seul cas où rien n'est à régler.
const ProxiesParDefaut = "127.0.0.1/32,::1/128"

// LireProxies lit une liste de réseaux séparés par des virgules (notation CIDR). Une liste qui
// couvrirait toutes les adresses est refusée : n'importe qui pourrait alors dicter son adresse.
func LireProxies(texte string) ([]*net.IPNet, error) {
	var l []*net.IPNet
	for _, m := range strings.Split(texte, ",") {
		m = strings.TrimSpace(m)
		if m == "" {
			continue
		}
		_, reseau, err := net.ParseCIDR(m)
		if err != nil {
			return nil, fmt.Errorf("proxy de confiance %q : attendu sous la forme 172.30.0.0/24 ou fd00::1/128", m)
		}
		if uns, _ := reseau.Mask.Size(); uns == 0 {
			return nil, fmt.Errorf("proxy de confiance %q : toutes les adresses seraient de confiance", m)
		}
		l = append(l, reseau)
	}
	return l, nil
}

// deConfiance : cette adresse est celle d'un proxy déclaré. Une adresse illisible ne l'est jamais.
func (a *acces) deConfiance(hote string) bool {
	ip := net.ParseIP(hote)
	if ip == nil {
		return false
	}
	for _, reseau := range a.proxies {
		if reseau.Contains(ip) {
			return true
		}
	}
	return false
}

// valeursEnTete : les valeurs d'un en-tête à liste, sur une ou plusieurs lignes, sans les espaces.
func valeursEnTete(r *http.Request, nom string) []string {
	l := strings.Split(strings.Join(r.Header.Values(nom), ","), ",")
	for i := range l {
		l[i] = strings.TrimSpace(l[i])
	}
	return l
}

// client remonte X-Forwarded-For depuis la droite, où seuls des proxys déclarés ont pu écrire : la
// première adresse qui n'est pas celle d'un proxy est celle du client. Ce qui est plus à gauche vient
// de lui et ne compte pas. saut : son rang en partant de la droite. ok : la connexion vient d'un
// proxy déclaré ; sinon, les en-têtes sont ceux du client et ne valent rien.
func (a *acces) client(r *http.Request) (hote string, saut int, ok bool) {
	hote = adresseDe(r)
	if a.comptes == nil || !a.deConfiance(hote) {
		return hote, 0, false
	}
	l := valeursEnTete(r, "X-Forwarded-For")
	for i := len(l) - 1; i >= 0; i-- {
		ip := net.ParseIP(l[i])
		if ip == nil {
			break
		}
		hote, saut = ip.String(), len(l)-1-i
		if !a.deConfiance(hote) {
			break
		}
	}
	return hote, saut, true
}

// adresse : l'adresse du client (mode serveur : celle que les proxys déclarés ont vue).
func (a *acces) adresse(r *http.Request) string {
	hote, _, _ := a.client(r)
	return hote
}

// https : la page est servie en HTTPS, directement ou par un proxy déclaré. X-Forwarded-Proto se lit
// comme X-Forwarded-For : la valeur au rang du client. Un proxy qui remplace l'en-tête n'en laisse
// qu'une seule : c'est alors la sienne.
func (a *acces) https(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	_, saut, ok := a.client(r)
	if !ok {
		return false
	}
	l := valeursEnTete(r, "X-Forwarded-Proto")
	return strings.EqualFold(l[max(len(l)-1-saut, 0)], "https")
}
