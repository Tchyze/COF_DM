//go:build !sqlite_fts5

package store

// La recherche plein texte a besoin du module FTS5 de SQLite, absent de la compilation par défaut
// de go-sqlite3. Compilez avec : go build -tags sqlite_fts5 (voir README, « Développement »).
var _ int = "compiler avec -tags sqlite_fts5"
