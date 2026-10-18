//go:build !serveur

package main

// Application de bureau : un éditeur, sans mode serveur (il n'existe que dans l'image Docker,
// compilée avec l'étiquette « serveur »). Elle peut se relier à un serveur pour synchroniser ses
// univers, ou travailler seule.
const editionServeur = false
