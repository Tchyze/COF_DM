package store

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/Tchyze/cof_dm/internal/modele"
)

// migrer ajoute aux univers existants les colonnes apparues après leur création.
func migrer(db *sql.DB) error {
	if err := ajouterColonnes(db, "relations", [][2]string{
		{"rang", `TEXT NOT NULL DEFAULT ''`},
		{"superieur", `INTEGER`},
		{"secrete", `INTEGER NOT NULL DEFAULT 0`},
		{"contexte", `INTEGER`},
	}); err != nil {
		return err
	}
	if err := ajouterColonnes(db, "entites", [][2]string{{"partage", `INTEGER NOT NULL DEFAULT 0`}}); err != nil {
		return err
	}
	return ajouterColonnes(db, "medias", [][2]string{{"contenu", `BLOB`}})
}

func ajouterColonnes(db *sql.DB, table string, colonnes [][2]string) error {
	cols := map[string]bool{}
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var cid, notnull, pk int
		var nom, typ string
		var defaut any
		if rows.Scan(&cid, &nom, &typ, &notnull, &defaut, &pk) == nil {
			cols[nom] = true
		}
	}
	rows.Close()
	for _, c := range colonnes {
		if !cols[c[0]] {
			if _, err := db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + c[0] + ` ` + c[1]); err != nil {
				return err
			}
		}
	}
	return nil
}

// Relation telle que vue depuis une fiche.
type Relation struct {
	ID        int64   `json:"id"`
	Type      string  `json:"type"`
	Libelle   string  `json:"libelle"` // lu depuis la fiche : « membre de », « a pour membre »…
	Sens      string  `json:"sens"`    // "sortante" (la fiche est A) ou "entrante"
	Note      string  `json:"note,omitempty"`
	Rang      string  `json:"rang,omitempty"`      // « membre de » : rang dans la faction
	Superieur int64   `json:"superieur,omitempty"` // « membre de » : supérieur direct dans la faction
	Secrete   bool    `json:"secrete,omitempty"`   // connue du MJ seulement
	Contexte  *Resume `json:"contexte,omitempty"`  // « sait » : séance (ou autre fiche) où il l'a appris
	Autre     Resume  `json:"autre"`
}

// DetailRelation : champs facultatifs d'une relation.
type DetailRelation struct {
	Note      *string `json:"note"`
	Rang      *string `json:"rang"`
	Superieur *int64  `json:"superieur"` // 0 : aucun
	Secrete   *bool   `json:"secrete"`
	Contexte  *int64  `json:"contexte"` // fiche (souvent une séance) où la relation s'est nouée ; 0 : aucune
	Type      *string `json:"type"`
}

const typesHierarchie = "membre"

func typeRelationValide(typ string) error {
	if _, ok := modele.RelationDe(typ); ok {
		return nil
	}
	var cles []string
	for _, r := range modele.Relations {
		cles = append(cles, r.Cle)
	}
	return invalide("Type de relation inconnu %q (types : %s).", typ, strings.Join(cles, ", "))
}

// relationPermise : la relation peut-elle relier ces deux types de fiches (« a reçu » : d'un PJ
// vers une aide de jeu) ?
func relationPermise(typ, a, b string) error {
	r, _ := modele.RelationDe(typ)
	if r.Permise(a, b) {
		return nil
	}
	nom := func(t string) string {
		if x, ok := modele.TypeDe(t); ok {
			return x.Nom
		}
		return t
	}
	return invalide("La relation « %s » ne va pas d'une fiche %s vers une fiche %s.", r.Nom, nom(a), nom(b))
}

// Relier crée une relation de A vers B.
func (s *Store) Relier(a, b int64, typ string, d DetailRelation, auteur string) (int64, error) {
	if err := typeRelationValide(typ); err != nil {
		return 0, err
	}
	if a == b {
		return 0, invalide("Une fiche ne peut pas être reliée à elle-même.")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ea, err := lire(s.db, a)
	if err != nil {
		return 0, err
	}
	eb, err := lire(s.db, b)
	if err != nil {
		return 0, err
	}
	if err := relationPermise(typ, ea.Type, eb.Type); err != nil {
		return 0, err
	}
	var n int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM relations WHERE ((a=? AND b=?) OR (a=? AND b=?)) AND type=?`, a, b, b, a, typ).Scan(&n)
	if n > 0 && typ != "autre" {
		return 0, invalide("Cette relation existe déjà.")
	}
	id, err := prochainID(s.db, "relations")
	if err != nil {
		return 0, err
	}
	if _, err := s.db.Exec(`INSERT INTO relations (id, a, b, type, note, cree) VALUES (?,?,?,?,?,?)`, id, a, b, typ, "", maintenant()); err != nil {
		return 0, err
	}
	if err := appliquerDetail(s.db, id, a, b, typ, d); err != nil {
		_, _ = s.db.Exec(`DELETE FROM relations WHERE id=?`, id)
		return 0, err
	}
	r, _ := modele.RelationDe(typ)
	noter(s.db, "relation", a, ea.Nom, fmt.Sprintf("%s %s %s", ea.Nom, r.Nom, eb.Nom), auteur)
	return id, nil
}

// ModifierRelation change les champs donnés d'une relation (les autres restent).
func (s *Store) ModifierRelation(id int64, d DetailRelation, auteur string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var a, b int64
	var typ string
	if err := s.db.QueryRow(`SELECT a, b, type FROM relations WHERE id=?`, id).Scan(&a, &b, &typ); err != nil {
		return invalide("Relation introuvable.")
	}
	if d.Type != nil {
		if err := typeRelationValide(*d.Type); err != nil {
			return err
		}
		ea, err := lire(s.db, a)
		if err != nil {
			return err
		}
		eb, err := lire(s.db, b)
		if err != nil {
			return err
		}
		if err := relationPermise(*d.Type, ea.Type, eb.Type); err != nil {
			return err
		}
		typ = *d.Type
		if _, err := s.db.Exec(`UPDATE relations SET type=? WHERE id=?`, typ, id); err != nil {
			return err
		}
	}
	if err := appliquerDetail(s.db, id, a, b, typ, d); err != nil {
		return err
	}
	noter(s.db, "relation modifiée", a, "", fmt.Sprintf("relation n°%d", id), auteur)
	return nil
}

// appliquerDetail enregistre note, rang, supérieur et secret. Le supérieur n'a de sens que pour
// « membre de » : il doit appartenir à la même faction (ou la diriger) et ne pas créer de boucle.
func appliquerDetail(q execer, id, a, b int64, typ string, d DetailRelation) error {
	if d.Note != nil {
		if _, err := q.Exec(`UPDATE relations SET note=? WHERE id=?`, strings.TrimSpace(*d.Note), id); err != nil {
			return err
		}
	}
	if d.Rang != nil {
		if _, err := q.Exec(`UPDATE relations SET rang=? WHERE id=?`, strings.TrimSpace(*d.Rang), id); err != nil {
			return err
		}
	}
	if d.Secrete != nil {
		if _, err := q.Exec(`UPDATE relations SET secrete=? WHERE id=?`, *d.Secrete, id); err != nil {
			return err
		}
	}
	if d.Contexte != nil {
		var v any
		if *d.Contexte != 0 {
			if _, err := lire(q, *d.Contexte); err != nil {
				return invalide("Contexte : fiche n°%d introuvable.", *d.Contexte)
			}
			v = *d.Contexte
		}
		if _, err := q.Exec(`UPDATE relations SET contexte=? WHERE id=?`, v, id); err != nil {
			return err
		}
	}
	if d.Superieur != nil {
		sup := *d.Superieur
		if sup != 0 {
			if typ != typesHierarchie {
				return invalide("Un supérieur ne s'indique que pour une relation « membre de ».")
			}
			if sup == a {
				return invalide("Un membre ne peut pas être son propre supérieur.")
			}
			membres, err := membresDe(q, b)
			if err != nil {
				return err
			}
			if !membres[sup] {
				return invalide("Le supérieur doit être membre ou dirigeant de la même faction.")
			}
			// boucle : en remontant depuis le supérieur, on ne doit pas retomber sur le membre
			vu := map[int64]bool{}
			for x := sup; x != 0 && !vu[x]; {
				vu[x] = true
				if x == a {
					return invalide("Ce supérieur créerait une boucle dans l'organigramme.")
				}
				var suivant sql.NullInt64
				_ = q.QueryRow(`SELECT superieur FROM relations WHERE a=? AND b=? AND type='membre' AND id<>?`, x, b, id).Scan(&suivant)
				x = suivant.Int64
			}
		}
		var v any
		if sup != 0 {
			v = sup
		}
		if _, err := q.Exec(`UPDATE relations SET superieur=? WHERE id=?`, v, id); err != nil {
			return err
		}
	}
	return nil
}

// membresDe : fiches membres d'une faction, ou qui la dirigent (relation ou champ « dirigeant »).
func membresDe(q execer, faction int64) (map[int64]bool, error) {
	out := map[int64]bool{}
	rows, err := q.Query(`SELECT a FROM relations WHERE b=? AND type IN ('membre','dirige')`, faction)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			out[id] = true
		}
	}
	rows.Close()
	if e, err := lire(q, faction); err == nil {
		for _, id := range ids(e.Data["dirigeant"]) {
			out[id] = true
		}
	}
	return out, nil
}

func (s *Store) Delier(id int64, auteur string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var a, b int64
	var typ string
	var sup sql.NullInt64
	if err := s.db.QueryRow(`SELECT a, b, type, superieur FROM relations WHERE id=?`, id).Scan(&a, &b, &typ, &sup); err != nil {
		return invalide("Relation introuvable.")
	}
	if _, err := s.db.Exec(`DELETE FROM relations WHERE id=?`, id); err != nil {
		return err
	}
	// ceux qui avaient ce membre pour supérieur passent sous son propre supérieur
	if typ == "membre" || typ == "dirige" {
		_, _ = s.db.Exec(`UPDATE relations SET superieur=? WHERE b=? AND type='membre' AND superieur=?`, sup, b, a)
	}
	noter(s.db, "relation retirée", 0, "", fmt.Sprintf("relation n°%d", id), auteur)
	return nil
}

// Relations renvoie les relations d'une fiche, dans les deux sens.
func (s *Store) Relations(id int64) ([]Relation, error) {
	s.mu.Lock()
	rows, err := s.db.Query(`SELECT id, a, b, type, note, rang, COALESCE(superieur,0), secrete, COALESCE(contexte,0) FROM relations WHERE a=? OR b=? ORDER BY type, id`, id, id)
	if err != nil {
		s.mu.Unlock()
		return nil, err
	}
	type brut struct {
		r        Relation
		a, b     int64
		contexte int64
	}
	var bruts []brut
	for rows.Next() {
		var x brut
		if rows.Scan(&x.r.ID, &x.a, &x.b, &x.r.Type, &x.r.Note, &x.r.Rang, &x.r.Superieur, &x.r.Secrete, &x.contexte) == nil {
			bruts = append(bruts, x)
		}
	}
	rows.Close()
	s.mu.Unlock()
	out := []Relation{}
	for _, x := range bruts {
		tr, _ := modele.RelationDe(x.r.Type)
		v := x.r
		autre := x.b
		v.Sens, v.Libelle = "sortante", tr.Nom
		if x.b == id {
			autre, v.Sens, v.Libelle = x.a, "entrante", tr.Inverse
		}
		if v.Type == "autre" && v.Note != "" {
			v.Libelle = v.Note
		}
		e, err := s.Lire(autre)
		if err != nil {
			continue
		}
		v.Autre = s.resume(e)
		if x.contexte != 0 {
			if c, err := s.Lire(x.contexte); err == nil {
				r := s.resume(c)
				v.Contexte = &r
			}
		}
		out = append(out, v)
	}
	return out, nil
}

// ---------------------------------------------------------------- organigramme

// Poste dans l'organigramme d'une faction.
type Poste struct {
	Fiche     Resume  `json:"fiche"`
	Rang      string  `json:"rang,omitempty"`
	Relation  int64   `json:"relation,omitempty"` // relation « membre de » (0 : dirigeant par le champ, ou rattaché)
	Superieur int64   `json:"superieur,omitempty"`
	Dirigeant bool    `json:"dirigeant,omitempty"`
	Rattache  bool    `json:"rattache,omitempty"` // pas membre : il répond à quelqu'un de l'organigramme
	Repond    *Resume `json:"repond,omitempty"`   // son responsable, quand celui-ci est hors de l'organigramme
	Secret    bool    `json:"secret,omitempty"`
	Statut    string  `json:"statut,omitempty"`
	Enfants   []Poste `json:"enfants"`
}

// Organigramme construit l'arbre d'une faction : les dirigeants en tête, chacun sous celui à qui il
// répond. Dans l'ordre : le supérieur réglé dans la faction, sinon le responsable de sa fiche
// (champs « responsable » puis « responsable_secret » d'un PNJ), sinon le premier dirigeant.
// Y figurent les membres, les sous-factions membres, et les « rattachés » : les PNJ sans aucune
// faction dont le responsable est dans l'organigramme (mercenaires, hommes de main, indicateurs),
// et ceux dont le responsable secret y est (infiltrés), quelle que soit leur faction affichée.
// Celui qui appartient à une autre faction reste dans la sienne : sans cela, un chef commun à deux
// factions amènerait toute l'une dans l'organigramme de l'autre.
func (s *Store) Organigramme(faction int64) ([]Poste, error) {
	f, err := s.Lire(faction)
	if err != nil {
		return nil, err
	}
	if f.Type != "faction" {
		return nil, invalide("L'organigramme est réservé aux factions.")
	}
	s.mu.Lock()
	rows, err := s.db.Query(`SELECT id, a, type, rang, note, COALESCE(superieur,0), secrete FROM relations
		WHERE b=? AND type IN ('membre','dirige') ORDER BY id`, faction)
	if err != nil {
		s.mu.Unlock()
		return nil, err
	}
	postes := map[int64]*Poste{}
	var ordre []int64
	ajouter := func(id int64) *Poste {
		if p, ok := postes[id]; ok {
			return p
		}
		p := &Poste{}
		postes[id] = p
		ordre = append(ordre, id)
		return p
	}
	type ligne struct {
		rel, a, sup     int64
		typ, rang, note string
		secret          bool
	}
	var lignes []ligne
	for rows.Next() {
		var l ligne
		if rows.Scan(&l.rel, &l.a, &l.typ, &l.rang, &l.note, &l.sup, &l.secret) == nil {
			lignes = append(lignes, l)
		}
	}
	rows.Close()
	// ceux qui ont une faction, où que ce soit
	encarte := map[int64]bool{}
	if rows, err = s.db.Query(`SELECT DISTINCT a FROM relations WHERE type IN ('membre','dirige')`); err == nil {
		for rows.Next() {
			var id int64
			if rows.Scan(&id) == nil {
				encarte[id] = true
			}
		}
		rows.Close()
	}
	s.mu.Unlock()
	for _, id := range ids(f.Data["dirigeant"]) {
		p := ajouter(id)
		p.Dirigeant = true
	}
	for _, l := range lignes {
		p := ajouter(l.a)
		if l.typ == "dirige" {
			p.Dirigeant = true
			p.Secret = p.Secret || l.secret
			if p.Rang == "" && l.note != "" {
				p.Rang = l.note
			}
			continue
		}
		p.Relation, p.Superieur, p.Secret = l.rel, l.sup, p.Secret || l.secret
		if l.rang != "" {
			p.Rang = l.rang
		} else if p.Rang == "" {
			p.Rang = l.note
		}
	}
	tout, err := s.Toutes()
	if err != nil {
		return nil, err
	}
	fiches := map[int64]Entite{}
	for _, e := range tout {
		if e.Archive {
			continue
		}
		fiches[e.ID] = e
		if e.Type == "faction" {
			for _, id := range ids(e.Data["dirigeant"]) {
				encarte[id] = true
			}
		}
	}
	// à qui chacun répond, d'après sa fiche : responsable affiché, responsable secret
	responsable := func(id int64, champ string) int64 {
		for _, r := range ids(fiches[id].Data[champ]) {
			if _, ok := fiches[r]; ok && r != id {
				return r
			}
		}
		return 0
	}
	for id, p := range postes {
		e, ok := fiches[id]
		if !ok {
			delete(postes, id)
			continue
		}
		p.Fiche = s.resume(e)
		p.Statut, _ = e.Data["statut"].(string)
		if p.Dirigeant && p.Rang == "" {
			p.Rang = "dirigeant"
		}
	}
	// rattachés, de proche en proche
	for encore := true; encore; {
		encore = false
		for _, e := range tout {
			if _, ok := fiches[e.ID]; !ok || postes[e.ID] != nil {
				continue
			}
			libre := !encarte[e.ID] && postes[responsable(e.ID, "responsable")] != nil
			if !libre && postes[responsable(e.ID, "responsable_secret")] == nil {
				continue
			}
			p := ajouter(e.ID)
			p.Fiche, p.Rattache = s.resume(e), true
			p.Statut, _ = e.Data["statut"].(string)
			encore = true
		}
	}
	// premier dirigeant : tête par défaut de ceux qui ne répondent à personne
	var tete int64
	for _, id := range ordre {
		if p, ok := postes[id]; ok && p.Dirigeant {
			tete = id
			break
		}
	}
	parent := map[int64]int64{}
	// placer sous un parent, sauf si cela ferme une boucle (A répond à B qui répond à A)
	placer := func(id, sous int64) bool {
		if sous == 0 || sous == id || postes[sous] == nil {
			return false
		}
		for x, pas := sous, 0; x != 0 && pas <= len(postes); x, pas = parent[x], pas+1 {
			if x == id {
				return false
			}
		}
		parent[id] = sous
		return true
	}
	for _, id := range ordre {
		p, ok := postes[id]
		if !ok {
			continue
		}
		switch {
		case placer(id, p.Superieur):
		case !(p.Rattache && encarte[id]) && placer(id, responsable(id, "responsable")):
		case placer(id, responsable(id, "responsable_secret")):
			p.Secret = true
		default:
			if r, ok := fiches[responsable(id, "responsable")]; ok {
				res := s.resume(r)
				p.Repond = &res
			}
		}
	}
	for _, id := range ordre {
		if p, ok := postes[id]; ok && parent[id] == 0 && !p.Dirigeant {
			placer(id, tete)
		}
	}
	var construire func(id int64) Poste
	construire = func(id int64) Poste {
		p := *postes[id]
		p.Enfants = []Poste{}
		for _, c := range ordre {
			if parent[c] == id && postes[c] != nil {
				p.Enfants = append(p.Enfants, construire(c))
			}
		}
		sort.SliceStable(p.Enfants, func(i, j int) bool {
			if len(p.Enfants[i].Enfants) != len(p.Enfants[j].Enfants) {
				return len(p.Enfants[i].Enfants) > len(p.Enfants[j].Enfants)
			}
			return Plier(p.Enfants[i].Fiche.Nom) < Plier(p.Enfants[j].Fiche.Nom)
		})
		return p
	}
	out := []Poste{}
	for _, id := range ordre {
		if postes[id] != nil && parent[id] == 0 {
			out = append(out, construire(id))
		}
	}
	return out, nil
}

// ---------------------------------------------------------------- qui sait quoi

// Connaissance : quelqu'un sait un secret (relation « sait »).
type Connaissance struct {
	Relation int64  `json:"relation"`
	Secret   int64  `json:"secret"`
	Qui      int64  `json:"qui"`
	Contexte int64  `json:"contexte,omitempty"`
	Note     string `json:"note,omitempty"`
}

// QuiSaitQuoi renvoie les secrets (d'une campagne et ceux sans campagne, ou tous), les PJ et les PNJ
// qui savent quelque chose, et toutes les connaissances.
func (s *Store) QuiSaitQuoi(campagne int64) (secrets, pj, pnj []Resume, conn []Connaissance, err error) {
	tout, err := s.Toutes()
	if err != nil {
		return
	}
	parID := map[int64]Entite{}
	for _, e := range tout {
		parID[e.ID] = e
	}
	s.mu.Lock()
	rows, err := s.db.Query(`SELECT id, a, b, COALESCE(contexte,0), note FROM relations WHERE type='sait' ORDER BY id`)
	if err != nil {
		s.mu.Unlock()
		return
	}
	for rows.Next() {
		var c Connaissance
		if rows.Scan(&c.Relation, &c.Qui, &c.Secret, &c.Contexte, &c.Note) == nil {
			if sec, ok := parID[c.Secret]; ok && sec.Type == "secret" {
				conn = append(conn, c)
			}
		}
	}
	rows.Close()
	s.mu.Unlock()
	garde := map[int64]bool{}
	connaisseurs := map[int64]bool{}
	for _, e := range tout {
		if e.Type != "secret" || e.Archive {
			continue
		}
		c := ids(e.Data["campagne"])
		if campagne > 0 && len(c) > 0 && c[0] != campagne {
			continue
		}
		garde[e.ID] = true
		secrets = append(secrets, s.resume(e))
	}
	var gardees []Connaissance
	for _, c := range conn {
		if garde[c.Secret] {
			gardees = append(gardees, c)
			connaisseurs[c.Qui] = true
		}
	}
	conn = gardees
	for _, e := range tout {
		if e.Archive {
			continue
		}
		switch e.Type {
		case "pj":
			c := ids(e.Data["campagne"])
			if campagne > 0 && (len(c) == 0 || c[0] != campagne) && !connaisseurs[e.ID] {
				continue
			}
			pj = append(pj, s.resume(e))
		case "pnj", "faction":
			if connaisseurs[e.ID] {
				pnj = append(pnj, s.resume(e))
			}
		}
	}
	trier := func(l []Resume) {
		sort.SliceStable(l, func(i, j int) bool { return Plier(l[i].Nom) < Plier(l[j].Nom) })
	}
	trier(secrets)
	trier(pj)
	trier(pnj)
	if secrets == nil {
		secrets = []Resume{}
	}
	if pj == nil {
		pj = []Resume{}
	}
	if pnj == nil {
		pnj = []Resume{}
	}
	if conn == nil {
		conn = []Connaissance{}
	}
	return
}

// ---------------------------------------------------------------- aides de jeu envoyées

// Envoi : une aide de jeu envoyée à un PJ (relation « recu »).
type Envoi struct {
	Relation int64   `json:"relation"`
	PJ       int64   `json:"pj"`
	Aide     int64   `json:"aide"`
	Seance   int64   `json:"seance,omitempty"` // contexte de la relation
	Date     float64 `json:"date"`             // création de la relation
}

// Envois renvoie tous les envois d'aides de jeu, du plus ancien au plus récent.
func (s *Store) Envois() ([]Envoi, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT id, a, b, COALESCE(contexte,0), cree FROM relations WHERE type='recu' ORDER BY cree, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Envoi{}
	for rows.Next() {
		var e Envoi
		if err := rows.Scan(&e.Relation, &e.PJ, &e.Aide, &e.Seance, &e.Date); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// TypeDeRelation renvoie le type d'une relation.
func (s *Store) TypeDeRelation(id int64) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var typ string
	if err := s.db.QueryRow(`SELECT type FROM relations WHERE id=?`, id).Scan(&typ); err != nil {
		return "", invalide("Relation introuvable.")
	}
	return typ, nil
}
