package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Tchyze/cof_dm/internal/fiches"
	"github.com/Tchyze/cof_dm/internal/modele"
	"github.com/Tchyze/cof_dm/internal/reference"
	"github.com/Tchyze/cof_dm/internal/regles"
)

// normaliserDoc valide un document restauré comme s'il venait d'être saisi.
func normaliserDoc(coll string, data []byte) ([]byte, error) {
	switch coll {
	case "monstres":
		m, err := fiches.NormaliserMonstre(data)
		if err != nil {
			return nil, err
		}
		return json.Marshal(m)
	case "combats": // état d'un combat : relu et complété par internal/combat à son ouverture
		var c map[string]json.RawMessage
		if err := json.Unmarshal(data, &c); err != nil || c == nil {
			return nil, fmt.Errorf("combat illisible")
		}
		return data, nil
	case reference.CollProfils, reference.CollPeuples, reference.CollVoies:
		return reference.NormaliserDoc(coll, data)
	}
	return nil, fmt.Errorf("collection inconnue")
}

// Export est une copie complète des données (images comprises), lisible et réimportable.
type Export struct {
	Format  string `json:"format"`
	Version int    `json:"version"`
	// Donnees : version du modèle des fiches (absente d'un export plus ancien, remis en forme à la
	// restauration).
	Donnees string `json:"donnees,omitempty"`
	// Modele : VersionModele de l'application qui a exporté (absent avant la 1.2.3).
	Modele     int               `json:"modele,omitempty"`
	Date       string            `json:"date"`
	Calendrier modele.Calendrier `json:"calendrier"`
	Entites    []Entite          `json:"entites"`
	Relations  []RelationBrute   `json:"relations"`
	Medias     []Media           `json:"medias"`
	// Bestiaire, profils, peuples et voies, bibliothèque de règles. Absents d'un export plus ancien :
	// la restauration garde alors ceux de l'univers. Reference : les profils, peuples et voies d'un
	// export d'avant le modèle 2, rangés depuis dans Documents.
	Documents []DocumentExport `json:"documents,omitempty"`
	Reference json.RawMessage  `json:"reference,omitempty"`
	Regles    json.RawMessage  `json:"regles,omitempty"`
	// Feuilles de personnage des PJ. Absentes d'un export plus ancien : celles de l'univers sont
	// gardées pour les fiches qui existent encore.
	Feuilles []FeuilleExport `json:"feuilles,omitempty"`
}

// DocumentExport : un document (monstre du bestiaire…) dans un export.
type DocumentExport struct {
	ID      int64           `json:"id"`
	Coll    string          `json:"coll"`
	Data    json.RawMessage `json:"data"`
	Cree    float64         `json:"cree"`
	Modifie float64         `json:"modifie"`
}

// MetaReference : clé de l'ancien réglage qui gardait les profils, peuples et voies (avant le
// modèle 2), repris un par un (reprendreReference).
const MetaReference = "reference"

// MetaRegles : clé du réglage de l'univers qui garde le fichier de règles (listes, tables).
const MetaRegles = "regles"

type RelationBrute struct {
	ID        int64  `json:"id"`
	A         int64  `json:"a"`
	B         int64  `json:"b"`
	Type      string `json:"type"`
	Note      string `json:"note,omitempty"`
	Rang      string `json:"rang,omitempty"`
	Superieur int64  `json:"superieur,omitempty"`
	Secrete   bool   `json:"secrete,omitempty"`
	Contexte  int64  `json:"contexte,omitempty"`
}

const formatExport = "cof-dm"

func (s *Store) Exporter() (Export, error) {
	x := Export{Format: formatExport, Version: 1, Donnees: versionDonnees, Modele: VersionModele, Date: time.Now().Format(time.RFC3339), Calendrier: s.Calendrier()}
	var err error
	if x.Entites, err = s.Toutes(); err != nil {
		return x, err
	}
	if x.Entites == nil {
		x.Entites = []Entite{}
	}
	s.mu.Lock()
	rows, err := s.db.Query(`SELECT id, a, b, type, note, rang, COALESCE(superieur,0), secrete, COALESCE(contexte,0) FROM relations ORDER BY id`)
	if err == nil {
		for rows.Next() {
			var r RelationBrute
			if rows.Scan(&r.ID, &r.A, &r.B, &r.Type, &r.Note, &r.Rang, &r.Superieur, &r.Secrete, &r.Contexte) == nil {
				x.Relations = append(x.Relations, r)
			}
		}
		rows.Close()
		rows, err = s.db.Query(`SELECT id, entite, fichier, nom_original, legende, principal, cree, contenu FROM medias ORDER BY id`)
		if err == nil {
			for rows.Next() {
				var m Media
				if rows.Scan(&m.ID, &m.Entite, &m.Fichier, &m.NomOriginal, &m.Legende, &m.Principal, &m.Cree, &m.Contenu) == nil {
					x.Medias = append(x.Medias, m)
				}
			}
			rows.Close()
		}
	}
	if err == nil {
		rows, err = s.db.Query(`SELECT id, coll, data, cree, modifie FROM documents ORDER BY id`)
		if err == nil {
			x.Documents = []DocumentExport{}
			for rows.Next() {
				var d DocumentExport
				var data string
				if rows.Scan(&d.ID, &d.Coll, &data, &d.Cree, &d.Modifie) == nil {
					d.Data = json.RawMessage(data)
					x.Documents = append(x.Documents, d)
				}
			}
			rows.Close()
		}
		if x.Feuilles, err = feuilles(s.db); err != nil {
			s.mu.Unlock()
			return x, err
		}
		var ref string
		if s.db.QueryRow(`SELECT valeur FROM meta WHERE cle=?`, MetaRegles).Scan(&ref) == nil && ref != "" && json.Valid([]byte(ref)) {
			x.Regles = json.RawMessage(ref)
		}
	}
	s.mu.Unlock()
	return x, err
}

// Restaurer remplace toutes les données par celles d'un export (identifiants conservés), après les
// avoir validées comme une saisie : rien n'est écrit si une seule fiche, relation, image ou feuille
// est refusée. Un export plus ancien que le modèle actuel est remis en forme comme un univers à son
// ouverture ; un export plus récent est refusé.
func (s *Store) Restaurer(x Export, auteur string) error {
	if err := s.restaurer(x, auteur); err != nil {
		return err
	}
	return s.reparer()
}

func (s *Store) restaurer(x Export, auteur string) error {
	if x.Format != formatExport {
		return invalide("Ce fichier n'est pas un export COF DM.")
	}
	if err := verifierExport(x); err != nil {
		return err
	}
	version, _ := strconv.ParseInt(x.Donnees, 10, 64)
	courante := donneesCourantes()
	if err := x.Calendrier.Verifier(); err != nil {
		return invalide("Calendrier de l'export invalide : %v", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	// un export d'avant les fichiers d'univers ne contient pas les images : celles de l'univers
	// qui portent le même nom sont gardées
	for i, m := range x.Medias {
		if len(m.Contenu) == 0 {
			_ = tx.QueryRow(`SELECT contenu FROM medias WHERE fichier=?`, m.Fichier).Scan(&x.Medias[i].Contenu)
		}
	}
	// supprimer les fiches emporte leurs feuilles : celles d'un export qui n'en porte pas sont mises
	// de côté, puis rendues aux fiches qui existent encore
	fs := x.Feuilles
	if fs == nil {
		if fs, err = feuilles(tx); err != nil {
			return err
		}
	}
	for _, q := range []string{`DELETE FROM medias`, `DELETE FROM relations`, `DELETE FROM refs`, `DELETE FROM mentions`, `DELETE FROM recherche`, `DELETE FROM entites`} {
		if _, err := tx.Exec(q); err != nil {
			return err
		}
	}
	// les fiches sont validées comme si elles venaient d'être saisies : d'abord toutes inscrites
	// (un champ peut désigner une fiche plus loin dans l'export), puis leurs champs vérifiés
	presentes := map[int64]string{}
	for i := range x.Entites {
		e := &x.Entites[i]
		if e.ID <= 0 || presentes[e.ID] != "" {
			return invalide("Fiche n°%d : identifiant absent ou en double.", e.ID)
		}
		if _, ok := modele.TypeDe(e.Type); !ok {
			return invalide("Fiche n°%d : type inconnu %q.", e.ID, e.Type)
		}
		if e.Nom, err = nettoyerNom(e.Nom); err != nil {
			return invalide("Fiche n°%d : %v", e.ID, err)
		}
		e.Tags = nettoyerTags(e.Tags)
		presentes[e.ID] = e.Type
		tj, _ := json.Marshal(e.Tags)
		var arch any
		if e.Archive {
			arch = e.Modifie
		}
		if _, err := tx.Exec(`INSERT INTO entites (id, type, nom, cle_nom, data, tags, archive_le, partage, cree, modifie) VALUES (?,?,?,?,'{}',?,?,?,?,?)`,
			e.ID, e.Type, e.Nom, cle(e.Nom), string(tj), arch, e.Partage && e.Type != "secret", e.Cree, e.Modifie); err != nil {
			return fmt.Errorf("fiche n°%d : %w", e.ID, err)
		}
	}
	for i := range x.Entites {
		e := &x.Entites[i]
		// un export d'un modèle plus ancien est remis en forme ensuite (reparer), sans rien perdre
		if version == courante {
			t, _ := modele.TypeDe(e.Type)
			if e.Data == nil {
				e.Data = map[string]any{}
			}
			norm, err := modele.Normaliser(t, e.Data, x.Calendrier, accesFiches{tx})
			if err != nil {
				return invalide("Fiche n°%d (%s) : %v", e.ID, e.Nom, erreurChamps(err))
			}
			// relu comme après un enregistrement, pour que l'index voie la même forme
			dj, _ := json.Marshal(norm)
			e.Data = map[string]any{}
			_ = json.Unmarshal(dj, &e.Data)
		}
		dj, _ := json.Marshal(e.Data)
		if _, err := tx.Exec(`UPDATE entites SET data=? WHERE id=?`, string(dj), e.ID); err != nil {
			return fmt.Errorf("fiche n°%d : %w", e.ID, err)
		}
		if err := indexer(tx, *e); err != nil {
			return err
		}
	}
	// relations : inscrites d'abord, puis leur supérieur et leur contexte vérifiés comme à la saisie
	vues := map[int64]bool{}
	for _, r := range x.Relations {
		switch {
		case r.ID <= 0 || vues[r.ID]:
			return invalide("Relation n°%d : identifiant absent ou en double.", r.ID)
		case presentes[r.A] == "" || presentes[r.B] == "":
			return invalide("Relation n°%d : elle relie une fiche absente de l'export.", r.ID)
		case r.A == r.B:
			return invalide("Relation n°%d : une fiche ne peut pas être reliée à elle-même.", r.ID)
		}
		if err := typeRelationValide(r.Type); err != nil {
			return invalide("Relation n°%d : %v", r.ID, err)
		}
		if err := relationPermise(r.Type, presentes[r.A], presentes[r.B]); err != nil {
			return invalide("Relation n°%d : %v", r.ID, err)
		}
		vues[r.ID] = true
		if _, err := tx.Exec(`INSERT INTO relations (id, a, b, type, note, rang, secrete, cree) VALUES (?,?,?,?,?,?,?,?)`,
			r.ID, r.A, r.B, r.Type, strings.TrimSpace(r.Note), strings.TrimSpace(r.Rang), r.Secrete, maintenant()); err != nil {
			return fmt.Errorf("relation n°%d : %w", r.ID, err)
		}
	}
	for _, r := range x.Relations {
		var d DetailRelation
		if r.Superieur != 0 {
			d.Superieur = &r.Superieur
		}
		if r.Contexte != 0 {
			d.Contexte = &r.Contexte
		}
		if err := appliquerDetail(tx, r.ID, r.A, r.B, r.Type, d); err != nil {
			var ev ErrValidation
			if errors.As(err, &ev) {
				return invalide("Relation n°%d : %v", r.ID, ev)
			}
			return err
		}
	}
	// images : rattachées à une fiche de l'export, sous un nom que le serveur sait servir, une seule
	// image principale par fiche
	principale := map[int64]bool{}
	vues = map[int64]bool{}
	nomsVus := map[string]bool{}
	for i := range x.Medias {
		m := &x.Medias[i]
		switch {
		case m.ID <= 0 || vues[m.ID]:
			return invalide("Image n°%d : identifiant absent ou en double.", m.ID)
		case presentes[m.Entite] == "":
			return invalide("Image n°%d : rattachée à une fiche absente de l'export.", m.ID)
		case !NomMediaValide(m.Fichier) || nomsVus[m.Fichier]:
			return invalide("Image n°%d : nom de fichier refusé ou en double (%q).", m.ID, m.Fichier)
		}
		vues[m.ID], nomsVus[m.Fichier] = true, true
		m.NomOriginal = filepath.Base(m.NomOriginal)
		if m.Principal && principale[m.Entite] {
			m.Principal = false
		}
		principale[m.Entite] = principale[m.Entite] || m.Principal
	}
	for _, m := range x.Medias {
		if _, err := tx.Exec(`INSERT INTO medias (id, entite, fichier, nom_original, legende, principal, cree, contenu) VALUES (?,?,?,?,?,?,?,?)`,
			m.ID, m.Entite, m.Fichier, m.NomOriginal, m.Legende, m.Principal, m.Cree, m.Contenu); err != nil {
			return fmt.Errorf("image n°%d : %w", m.ID, err)
		}
	}
	niveaux := map[int64]int{}
	for _, e := range x.Entites {
		niveaux[e.ID] = 1
		if n, ok := e.Data["niveau"].(float64); ok && n >= 1 {
			niveaux[e.ID] = int(n)
		}
	}
	// feuilles : seulement pour les PJ, relues sans bornes (comme à l'ouverture d'une feuille, pour
	// qu'un changement de règles ne la rende pas illisible)
	for _, f := range fs {
		if presentes[f.Entite] != "pj" {
			continue
		}
		fe, err := fiches.NormaliserFeuille(f.Data, niveaux[f.Entite], nil)
		if err != nil {
			return invalide("Feuille de la fiche n°%d : %v", f.Entite, err)
		}
		if _, err := tx.Exec(`INSERT INTO feuilles (entite, data, modifie) VALUES (?,?,?)`, f.Entite, string(fe.JSON()), f.Modifie); err != nil {
			return fmt.Errorf("feuille de la fiche n°%d : %w", f.Entite, err)
		}
	}
	if x.Documents != nil {
		if _, err := tx.Exec(`DELETE FROM documents`); err != nil {
			return err
		}
		for _, d := range x.Documents {
			data, err := normaliserDoc(d.Coll, d.Data)
			if err != nil {
				return invalide("Document n°%d (%s) : %v", d.ID, d.Coll, err)
			}
			if _, err := tx.Exec(`INSERT INTO documents (id, coll, data, cree, modifie) VALUES (?,?,?,?,?)`, d.ID, d.Coll, string(data), d.Cree, d.Modifie); err != nil {
				return fmt.Errorf("document n°%d : %w", d.ID, err)
			}
		}
	}
	if len(x.Reference) > 0 { // export d'avant le modèle 2 : ses profils, peuples et voies remplacent ceux de l'univers
		ref, err := reference.Lire(x.Reference)
		if err != nil {
			return invalide("Profils et voies de l'export : %v", err)
		}
		if _, err := tx.Exec(`DELETE FROM documents WHERE coll IN (?,?,?)`, reference.CollProfils, reference.CollPeuples, reference.CollVoies); err != nil {
			return err
		}
		if err := reprendre(tx, ref, auteur); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`DELETE FROM meta WHERE cle=?`, MetaReference); err != nil {
		return err
	}
	if len(x.Regles) > 0 {
		rg, err := regles.Relire(x.Regles)
		if err != nil {
			return invalide("Fichier de règles de l'export : %v", err)
		}
		rj, _ := json.Marshal(rg)
		if _, err := tx.Exec(`INSERT OR REPLACE INTO meta (cle, valeur) VALUES (?, ?)`, MetaRegles, string(rj)); err != nil {
			return err
		}
	}
	b, _ := json.Marshal(x.Calendrier)
	if _, err := tx.Exec(`INSERT OR REPLACE INTO meta (cle, valeur) VALUES ('calendrier', ?)`, string(b)); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT OR REPLACE INTO meta (cle, valeur) VALUES ('version_donnees', ?)`, x.Donnees); err != nil {
		return err
	}
	noter(tx, "restauration", 0, "", fmt.Sprintf("%d fiches, %d relations depuis un export du %s", len(x.Entites), len(x.Relations), x.Date), auteur)
	if err := tx.Commit(); err != nil {
		return err
	}
	s.cal = x.Calendrier
	return nil
}
