package serveur

import (
	"os"
	"path/filepath"
	"strings"
)

// Ce qu'un seul MJ peut demander à un serveur qu'il partage avec d'autres groupes. Sur le bureau, il
// n'y a pas de limites (Options.Limites nil) : les sauvegardes du MJ y restent toutes.
const (
	ImportsSimultanesParDefaut = 1
	QuotaUniversParDefaut      = 2 << 30 // octets
	QuotaSauvegardesParDefaut  = 5 << 30 // octets, par univers
	SauvegardesMaxParDefaut    = 10      // par famille : demandées, d'avant une opération
)

// tailleMaxExport : au-delà, l'export JSON (tout en mémoire, images comprises) ne se fait pas sur un
// serveur ; la restauration ne le reprendrait pas non plus. Une sauvegarde ou une copie reste possible.
const tailleMaxExport = 256 << 20

const motifAvantSuppression = "avant-suppression"

const (
	msgOccupeImport = "Un traitement lourd est déjà en cours sur ce serveur : réessayez dans un instant."
	msgQuotaUnivers = "Le quota de cet univers est atteint : voyez avec l'administrateur du serveur."
)

// Limites : les quotas d'un serveur (zéro : pas de limite pour celui-ci).
type Limites struct {
	lourds           chan struct{} // traitements lourds en cours, sur tout le serveur
	QuotaUnivers     int64
	QuotaSauvegardes int64
	SauvegardesMax   int
}

// NouvellesLimites : imports, le nombre de traitements lourds simultanés (au moins un) ; quotas en octets.
func NouvellesLimites(imports int, quotaUnivers, quotaSauvegardes int64, sauvegardesMax int) *Limites {
	if imports < 1 {
		imports = 1
	}
	return &Limites{lourds: make(chan struct{}, imports), QuotaUnivers: quotaUnivers, QuotaSauvegardes: quotaSauvegardes, SauvegardesMax: sauvegardesMax}
}

// LimitesParDefaut : celles d'un serveur qui ne dit rien.
func LimitesParDefaut() *Limites {
	return NouvellesLimites(ImportsSimultanesParDefaut, QuotaUniversParDefaut, QuotaSauvegardesParDefaut, SauvegardesMaxParDefaut)
}

// prendre : une place pour un traitement lourd, sans attendre. Faux : il n'y en a plus.
func (l *Limites) prendre() (libere func(), ok bool) {
	if l == nil {
		return func() {}, true
	}
	select {
	case l.lourds <- struct{}{}:
		return func() { <-l.lourds }, true
	default:
		return nil, false
	}
}

// familleSauvegarde : ce qui se plafonne ensemble. Vide : jamais purgée ici (une sauvegarde
// automatique suit purgerSauvegardes ; celle d'avant une suppression survit à son univers).
func familleSauvegarde(motif string) string {
	switch {
	case motif == "manuelle":
		return motif
	case motif == motifAvantSuppression:
		return ""
	case strings.HasPrefix(motif, "avant-"):
		return "avant"
	}
	return ""
}

// ranger applique les plafonds au dossier de sauvegardes d'un univers, juste après la sauvegarde
// garder, qui n'est jamais effacée : elle protège peut-être l'opération qui suit. Par famille, les
// plus récentes restent ; puis, au-delà du volume, les plus anciennes partent, sauf la dernière de
// chaque sorte et celles d'avant une suppression. Faux : le volume reste dépassé. Avec refusable (une
// sauvegarde que l'on peut refuser), rien n'est alors effacé : à l'appelant de retirer garder.
func (l *Limites) ranger(dossier, garder string, refusable bool) bool {
	if l == nil {
		return true
	}
	var total, intouchable int64
	var effacer, candidats []Sauvegarde
	vues := map[string]int{}
	for _, s := range listerSauvegardes(dossier) { // de la plus récente à la plus ancienne
		famille, sorte := familleSauvegarde(s.Motif), familleSauvegarde(s.Motif)
		if sorte == "" {
			sorte = s.Motif
		}
		vues[sorte]++
		switch {
		case s.Fichier != garder && famille != "" && l.SauvegardesMax > 0 && vues[sorte] > l.SauvegardesMax:
			effacer = append(effacer, s)
			continue
		case s.Fichier == garder || vues[sorte] == 1 || s.Motif == motifAvantSuppression:
			intouchable += s.Taille
		default:
			candidats = append(candidats, s)
		}
		total += s.Taille
	}
	if refusable && l.QuotaSauvegardes > 0 && intouchable > l.QuotaSauvegardes {
		return false
	}
	for i := len(candidats) - 1; i >= 0 && l.QuotaSauvegardes > 0 && total > l.QuotaSauvegardes; i-- {
		effacer = append(effacer, candidats[i])
		total -= candidats[i].Taille
	}
	for _, s := range effacer {
		_ = os.Remove(filepath.Join(dossier, s.Fichier))
	}
	return l.QuotaSauvegardes <= 0 || total <= l.QuotaSauvegardes
}
