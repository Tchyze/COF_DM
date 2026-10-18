package serveur

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/Tchyze/cof_dm/internal/fiches"
	"github.com/Tchyze/cof_dm/internal/modele"
	"github.com/Tchyze/cof_dm/internal/reference"
	"github.com/Tchyze/cof_dm/internal/regles"
	"github.com/Tchyze/cof_dm/internal/store"
)

// Bestiaire et bibliothèque de règles (repris de tracker_COF). Les monstres, officiels et maison,
// et les profils, peuples et voies vivent dans le fichier d'univers. Le contenu officiel n'est pas
// embarqué : il s'importe depuis les fichiers du MJ.

const (
	collMonstres  = "monstres"
	tailleImport  = 32 << 20
	formatBiblio  = "cof-dm-bibliotheque"
	versionBiblio = 1
)

func (s *Serveur) routesBestiaire() {
	m := s.mux
	m.HandleFunc("GET /api/bestiaire", s.bestiaireLister)
	m.HandleFunc("POST /api/bestiaire", s.bestiaireCreer)
	m.HandleFunc("GET /api/bestiaire/tables", s.bestiaireTables)
	m.HandleFunc("POST /api/bestiaire/calculer", s.bestiaireCalculer)
	m.HandleFunc("POST /api/bestiaire/import", s.bestiaireImporter)
	m.HandleFunc("GET /api/bestiaire/{id}", s.bestiaireLire)
	m.HandleFunc("PUT /api/bestiaire/{id}", s.bestiaireModifier)
	m.HandleFunc("DELETE /api/bestiaire/{id}", s.bestiaireSupprimer)
	m.HandleFunc("POST /api/bestiaire/{id}/dupliquer", s.bestiaireDupliquer)
	m.HandleFunc("GET /api/reference", s.referenceLire)
	m.HandleFunc("POST /api/reference/import", s.referenceImporter)
	m.HandleFunc("DELETE /api/reference", s.referenceVider)
	m.HandleFunc("GET /api/regles", s.reglesLire)
	m.HandleFunc("POST /api/regles/import", s.reglesImporter)
	m.HandleFunc("DELETE /api/regles", s.reglesVider)
	m.HandleFunc("GET /api/bibliotheque", s.bibliothequeExporter)
	m.HandleFunc("POST /api/bibliotheque", s.bibliothequeImporter)
}

func erreurFiche(w http.ResponseWriter, err error) {
	var ev fiches.ErrValidation
	if errors.As(err, &ev) {
		erreur(w, 422, ev.Error())
		return
	}
	erreurStore(w, err)
}

// docMonstre : un monstre tel que l'API le montre, avec son identifiant et sa date de modification.
func docMonstre(d store.Document) map[string]any {
	var m map[string]any
	if err := json.Unmarshal(d.Data, &m); err != nil || m == nil {
		m = map[string]any{}
	}
	m["id"], m["modifie"] = d.ID, d.Modifie
	return m
}

func lireCorps(r *http.Request, max int64) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r.Body, max+1))
	if err == nil && int64(len(b)) > max {
		err = store.ErrValidation{Msg: "Fichier trop lourd."}
	}
	return b, err
}

func (s *Serveur) bestiaireLister(w http.ResponseWriter, r *http.Request) {
	l, err := s.store.ListerDocs(collMonstres)
	if err != nil {
		erreurStore(w, err)
		return
	}
	out := make([]map[string]any, 0, len(l))
	for _, d := range l {
		out = append(out, docMonstre(d))
	}
	repondre(w, 200, out)
}

func (s *Serveur) bestiaireLire(w http.ResponseWriter, r *http.Request) {
	id, err := idDe(r)
	if err == nil {
		var d store.Document
		if d, err = s.store.LireDoc(collMonstres, id); err == nil {
			repondre(w, 200, docMonstre(d))
			return
		}
	}
	erreur(w, 404, "Monstre introuvable.")
}

func (s *Serveur) bestiaireCreer(w http.ResponseWriter, r *http.Request) {
	corps, err := lireCorps(r, tailleMaxJSON)
	if err != nil {
		erreurStore(w, err)
		return
	}
	m, err := fiches.NormaliserMonstre(corps)
	if err != nil {
		erreurFiche(w, err)
		return
	}
	data, _ := json.Marshal(m)
	d, err := s.store.CreerDoc(collMonstres, data, m.Nom, auteurDe(r))
	if err != nil {
		erreurStore(w, err)
		return
	}
	repondre(w, 201, docMonstre(d))
}

// Plier : comparaison sans accents ni casse.
func Plier(s string) string { return store.Plier(s) }

// monstre lit un monstre du bestiaire.
func (s *Serveur) monstre(id int64) (*fiches.Monstre, error) {
	d, err := s.store.LireDoc(collMonstres, id)
	if err != nil {
		return nil, err
	}
	var m fiches.Monstre
	if err := json.Unmarshal(d.Data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *Serveur) bestiaireModifier(w http.ResponseWriter, r *http.Request) {
	id, err := idDe(r)
	if err != nil {
		erreur(w, 404, "Monstre introuvable.")
		return
	}
	corps, err := lireCorps(r, tailleMaxJSON)
	if err != nil {
		erreurStore(w, err)
		return
	}
	m, err := fiches.NormaliserMonstre(corps)
	if err != nil {
		erreurFiche(w, err)
		return
	}
	ancien, err := s.monstre(id)
	if err != nil {
		erreur(w, 404, "Monstre introuvable.")
		return
	}
	if ancien.Verrouillee() {
		erreur(w, 409, "Fiche officielle verrouillée : dupliquez-la pour créer une version maison modifiable.")
		return
	}
	data, _ := json.Marshal(m)
	d, err := s.store.MajDoc(collMonstres, id, data, m.Nom, auteurDe(r))
	if err != nil {
		erreurStore(w, err)
		return
	}
	repondre(w, 200, docMonstre(d))
}

func (s *Serveur) bestiaireSupprimer(w http.ResponseWriter, r *http.Request) {
	id, err := idDe(r)
	if err != nil {
		erreur(w, 404, "Monstre introuvable.")
		return
	}
	m, err := s.monstre(id)
	if err != nil {
		erreur(w, 404, "Monstre introuvable.")
		return
	}
	if l := s.usagesMonstre(id); len(l) > 0 {
		if len(l) > 5 {
			l = append(l[:5], "…")
		}
		erreur(w, 409, "Ce monstre sert à des fiches ("+strings.Join(l, ", ")+") : retirez-le d'abord de ces fiches.")
		return
	}
	if err := s.store.SupprimerDoc(collMonstres, id, m.Nom, auteurDe(r)); err != nil {
		erreurStore(w, err)
		return
	}
	repondre(w, 200, map[string]any{"supprime": true})
}

// bestiaireDupliquer fait d'un monstre (officiel ou non) une copie maison modifiable.
func (s *Serveur) bestiaireDupliquer(w http.ResponseWriter, r *http.Request) {
	id, _ := idDe(r)
	m, err := s.monstre(id)
	if err != nil {
		erreur(w, 404, "Monstre introuvable.")
		return
	}
	origine := m.Nom
	if m.Livre != "" {
		origine = fmt.Sprintf("%s (%s p. %d)", m.Nom, m.Livre, m.Page)
	}
	m.Nom += " (maison)"
	m.Source, m.Ref, m.Livre, m.Page = "maison", "", "", 0
	if m.Notes != "" {
		m.Notes += "\n"
	}
	m.Notes += "Copie de " + origine + "."
	data, _ := json.Marshal(m)
	d, err := s.store.CreerDoc(collMonstres, data, m.Nom, auteurDe(r))
	if err != nil {
		erreurStore(w, err)
		return
	}
	repondre(w, 201, docMonstre(d))
}

type rejetImport struct {
	Ligne int    `json:"ligne"`
	Nom   string `json:"nom"`
	Motif string `json:"motif"`
}

type bilanMonstres struct {
	Crees    int           `json:"crees"`
	MisAJour int           `json:"mis_a_jour"`
	Rejets   []rejetImport `json:"rejets"`
}

// importerMonstres ajoute des monstres ; ceux dont la référence (« ref ») existe déjà sont mis à
// jour, les monstres maison de l'univers ne sont jamais modifiés.
func (s *Serveur) importerMonstres(liste []json.RawMessage) (bilanMonstres, error) {
	res := bilanMonstres{Rejets: []rejetImport{}}
	existants, err := s.store.ListerDocs(collMonstres)
	if err != nil {
		return res, err
	}
	parRef := map[string]int64{}
	for _, d := range existants {
		var m fiches.Monstre
		if json.Unmarshal(d.Data, &m) == nil && m.Ref != "" {
			parRef[m.Ref] = d.ID
		}
	}
	for i, raw := range liste {
		m, err := fiches.NormaliserMonstre(raw)
		if err != nil {
			var nom struct {
				Nom string `json:"nom"`
			}
			_ = json.Unmarshal(raw, &nom)
			res.Rejets = append(res.Rejets, rejetImport{i + 1, nom.Nom, err.Error()})
			continue
		}
		data, _ := json.Marshal(m)
		if id, ok := parRef[m.Ref]; ok && m.Ref != "" {
			if _, err := s.store.MajDoc(collMonstres, id, data, m.Nom, ""); err == nil {
				res.MisAJour++
			}
			continue
		}
		d, err := s.store.CreerDoc(collMonstres, data, m.Nom, "")
		if err != nil {
			res.Rejets = append(res.Rejets, rejetImport{i + 1, m.Nom, err.Error()})
			continue
		}
		if m.Ref != "" {
			parRef[m.Ref] = d.ID
		}
		res.Crees++
	}
	return res, nil
}

// bestiaireImporter : un tableau de monstres, ou un objet {"monstres": [...]}.
func (s *Serveur) bestiaireImporter(w http.ResponseWriter, r *http.Request) {
	corps, err := lireCorps(r, tailleImport)
	if err != nil {
		erreurStore(w, err)
		return
	}
	var liste []json.RawMessage
	if json.Unmarshal(corps, &liste) != nil {
		var obj struct {
			Monstres []json.RawMessage `json:"monstres"`
		}
		if json.Unmarshal(corps, &obj) != nil || obj.Monstres == nil {
			erreur(w, 422, "Format non reconnu : attendu un tableau de monstres ou un objet {\"monstres\": [...]}.")
			return
		}
		liste = obj.Monstres
	}
	res, err := s.importerMonstres(liste)
	if err != nil {
		erreurStore(w, err)
		return
	}
	s.store.Noter("bestiaire", fmt.Sprintf("import : %d ajoutés, %d mis à jour, %d rejetés", res.Crees, res.MisAJour, len(res.Rejets)), auteurDe(r))
	repondre(w, 200, res)
}

// ---------------------------------------------------------------- créateur de monstre

// sansCreateur : l'univers n'a pas de règles pour concevoir une créature.
const sansCreateur = "Le créateur de monstre a besoin d'un fichier de règles : importez-le dans Paramètres → Bestiaire et règles."

// bestiaireTables : les tables du créateur de monstre, telles que le fichier de règles les donne.
func (s *Serveur) bestiaireTables(w http.ResponseWriter, r *http.Request) {
	rg := s.lireRegles()
	if rg.Creature == nil {
		erreur(w, 404, sansCreateur)
		return
	}
	repondre(w, 200, map[string]any{"creature": rg.Creature, "listes": rg.Listes})
}

// bestiaireCalculer conçoit une créature selon les règles de l'univers : un calcul, rien n'est
// enregistré.
func (s *Serveur) bestiaireCalculer(w http.ResponseWriter, r *http.Request) {
	var d regles.DemandeCreature
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&d); err != nil {
		erreur(w, 422, "Demande illisible.")
		return
	}
	rg := s.lireRegles()
	if rg.Creature == nil {
		erreur(w, 404, sansCreateur)
		return
	}
	c, err := rg.Creature.Calculer(d)
	if err != nil {
		erreur(w, 422, err.Error())
		return
	}
	repondre(w, 200, c)
}

// ---------------------------------------------------------------- profils, peuples et voies

var verrouReference sync.Mutex

func (s *Serveur) lireReference() *reference.Reference { return referenceDe(s.store) }

func referenceDe(st *store.Store) *reference.Reference {
	v, ok := st.Meta(store.MetaReference)
	ref := reference.Vide()
	if !ok || v == "" || json.Unmarshal([]byte(v), ref) != nil || ref.Normaliser() != nil {
		return reference.Vide()
	}
	return ref
}

func (s *Serveur) ecrireReference(ref *reference.Reference) error {
	b, err := json.Marshal(ref)
	if err != nil {
		return err
	}
	return s.store.SetMeta(store.MetaReference, string(b))
}

func totalReference(ref *reference.Reference) map[string]int {
	return map[string]int{"profils": len(ref.Profils), "peuples": len(ref.Peuples), "voies": len(ref.Voies)}
}

func (s *Serveur) referenceLire(w http.ResponseWriter, r *http.Request) {
	repondre(w, 200, s.lireReference())
}

// fusionnerReference ajoute ou remplace (même référence, ou même nom) les éléments de n.
func (s *Serveur) fusionnerReference(n *reference.Reference) (reference.Bilan, *reference.Reference, error) {
	verrouReference.Lock()
	defer verrouReference.Unlock()
	ref := s.lireReference()
	bilan := ref.Fusionner(n)
	return bilan, ref, s.ecrireReference(ref)
}

func (s *Serveur) referenceImporter(w http.ResponseWriter, r *http.Request) {
	corps, err := lireCorps(r, tailleImport)
	if err != nil {
		erreurStore(w, err)
		return
	}
	n, err := reference.Lire(corps)
	if err != nil {
		erreur(w, 422, err.Error())
		return
	}
	bilan, ref, err := s.fusionnerReference(n)
	if err != nil {
		erreurStore(w, err)
		return
	}
	s.store.Noter("règles", "import de profils, peuples et voies", auteurDe(r))
	repondre(w, 200, map[string]any{"bilan": bilan, "alertes": ref.Problemes(), "total": totalReference(ref)})
}

// referenceVider retire tous les profils, peuples et voies (les fiches ne changent pas).
func (s *Serveur) referenceVider(w http.ResponseWriter, r *http.Request) {
	verrouReference.Lock()
	defer verrouReference.Unlock()
	if err := s.ecrireReference(reference.Vide()); err != nil {
		erreurStore(w, err)
		return
	}
	s.store.Noter("règles", "profils, peuples et voies retirés", auteurDe(r))
	repondre(w, 200, map[string]bool{"ok": true})
}

// ---------------------------------------------------------------- fichier de règles

// Le fichier de règles apporte ce que le code ne contient pas : les listes de choix des fiches
// (peuples, profils, tailles, catégories) et les tables du créateur de monstre.

func (s *Serveur) lireRegles() *regles.Regles { return reglesDe(s.store) }

// orVide : des règles de feuille sans aucune borne quand l'univers n'en a pas.
func orVide(f *regles.Feuille) *regles.Feuille {
	if f == nil {
		return &regles.Feuille{}
	}
	return f
}

func reglesDe(st *store.Store) *regles.Regles {
	v, ok := st.Meta(store.MetaRegles)
	if !ok || v == "" {
		return regles.Vide()
	}
	rg, err := regles.Relire([]byte(v))
	if err != nil {
		return regles.Vide()
	}
	return rg
}

func (s *Serveur) ecrireRegles(rg *regles.Regles) error {
	if rg.EstVide() {
		return s.store.SetMeta(store.MetaRegles, "")
	}
	b, err := json.Marshal(rg)
	if err != nil {
		return err
	}
	return s.store.SetMeta(store.MetaRegles, string(b))
}

// fusionnerRegles ajoute un fichier de règles à celles de l'univers : chaque partie qu'il apporte
// remplace celle qui était là.
func (s *Serveur) fusionnerRegles(n *regles.Regles) (*regles.Regles, error) {
	verrouReference.Lock()
	defer verrouReference.Unlock()
	rg := s.lireRegles()
	rg.Fusionner(n)
	return rg, s.ecrireRegles(rg)
}

func resumeRegles(rg *regles.Regles) map[string]any {
	l := rg.Listes
	return map[string]any{"titre": rg.Titre, "createur": rg.Creature != nil, "personnage": rg.Personnage != nil, "rencontre": rg.Rencontre != nil, "combat": rg.Combat != nil,
		"listes": map[string]int{"peuples": len(l.Peuples), "profils": len(l.Profils), "peuples_pnj": len(l.PeuplesPNJ),
			"profils_pnj": len(l.ProfilsPNJ), "tailles": len(l.Tailles), "categories": len(l.Categories)}}
}

func (s *Serveur) reglesLire(w http.ResponseWriter, r *http.Request) {
	repondre(w, 200, s.lireRegles())
}

func (s *Serveur) reglesImporter(w http.ResponseWriter, r *http.Request) {
	corps, err := lireCorps(r, tailleImport)
	if err != nil {
		erreurStore(w, err)
		return
	}
	n, err := regles.Lire(corps)
	if err != nil {
		erreur(w, 422, majuscule(err.Error())+".")
		return
	}
	rg, err := s.fusionnerRegles(n)
	if err != nil {
		erreurStore(w, err)
		return
	}
	s.store.Noter("règles", "fichier de règles importé", auteurDe(r))
	repondre(w, 200, resumeRegles(rg))
}

// reglesVider retire le fichier de règles : les fiches gardent leurs valeurs, leurs champs se
// saisissent librement et le créateur de monstre n'est plus proposé.
func (s *Serveur) reglesVider(w http.ResponseWriter, r *http.Request) {
	verrouReference.Lock()
	defer verrouReference.Unlock()
	if err := s.ecrireRegles(regles.Vide()); err != nil {
		erreurStore(w, err)
		return
	}
	s.store.Noter("règles", "fichier de règles retiré", auteurDe(r))
	repondre(w, 200, map[string]bool{"ok": true})
}

// listeDeRegles renvoie une liste de choix du fichier de règles, par son nom.
func listeDeRegles(rg *regles.Regles, nom string) []string {
	switch nom {
	case "peuples":
		return rg.Listes.Peuples
	case "profils":
		return rg.Listes.Profils
	case "peuples_pnj":
		return rg.Listes.PeuplesPNJ
	case "profils_pnj":
		return rg.Listes.ProfilsPNJ
	case "tailles":
		return rg.Listes.Tailles
	case "categories":
		return rg.Listes.Categories
	}
	return nil
}

// typesDeLUnivers : les types de fiches, dont les champs « Liste » reçoivent les choix du fichier
// de règles de l'univers.
func (s *Serveur) typesDeLUnivers() []modele.Type {
	rg := s.lireRegles()
	out := make([]modele.Type, len(modele.Types))
	for i, t := range modele.Types {
		t.Champs = append([]modele.Champ(nil), t.Champs...)
		for k := range t.Champs {
			if c := &t.Champs[k]; c.Liste != "" {
				c.Options = listeDeRegles(rg, c.Liste)
			}
		}
		out[i] = t
	}
	return out
}

// ---------------------------------------------------------------- bibliothèque d'un univers

// Bibliotheque : le bestiaire et les règles d'un univers en un fichier, pour les reprendre dans un
// autre.
type Bibliotheque struct {
	Format    string               `json:"format"`
	Version   int                  `json:"version"`
	Monstres  []json.RawMessage    `json:"monstres"`
	Reference *reference.Reference `json:"reference"`
	Regles    *regles.Regles       `json:"regles,omitempty"`
}

// bibliotheque rassemble le bestiaire et les règles de l'univers.
func (s *Serveur) bibliotheque() (Bibliotheque, error) { return bibliothequeDe(s.store) }

func bibliothequeDe(st *store.Store) (Bibliotheque, error) {
	b := Bibliotheque{Format: formatBiblio, Version: versionBiblio, Monstres: []json.RawMessage{}, Reference: referenceDe(st)}
	if rg := reglesDe(st); !rg.EstVide() {
		b.Regles = rg
	}
	l, err := st.ListerDocs(collMonstres)
	for _, d := range l {
		b.Monstres = append(b.Monstres, d.Data)
	}
	return b, err
}

// recevoirBibliotheque ajoute une bibliothèque à l'univers. Les monstres officiels déjà présents
// (même « ref ») sont mis à jour ; un monstre maison du même nom n'est pas ajouté une seconde fois.
func (s *Serveur) recevoirBibliotheque(b Bibliotheque, auteur string) (map[string]any, error) {
	existants, err := s.store.ListerDocs(collMonstres)
	if err != nil {
		return nil, err
	}
	maison := map[string]bool{}
	for _, d := range existants {
		var m fiches.Monstre
		if json.Unmarshal(d.Data, &m) == nil && m.Ref == "" {
			maison[Plier(m.Nom)] = true
		}
	}
	var liste []json.RawMessage
	deja := 0
	for _, raw := range b.Monstres {
		var m fiches.Monstre
		if json.Unmarshal(raw, &m) == nil && m.Ref == "" && maison[Plier(m.Nom)] {
			deja++
			continue
		}
		liste = append(liste, raw)
	}
	res, err := s.importerMonstres(liste)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"monstres": res, "deja_presents": deja}
	if b.Reference != nil {
		if err := b.Reference.Normaliser(); err != nil {
			return nil, store.ErrValidation{Msg: "Profils et voies illisibles : " + err.Error()}
		}
		bilan, ref, err := s.fusionnerReference(b.Reference)
		if err != nil {
			return nil, err
		}
		out["reference"], out["total"] = bilan, totalReference(ref)
	}
	if b.Regles != nil {
		if err := b.Regles.Normaliser(); err != nil {
			return nil, store.ErrValidation{Msg: "Fichier de règles illisible : " + err.Error()}
		}
		rg, err := s.fusionnerRegles(b.Regles)
		if err != nil {
			return nil, err
		}
		out["regles"] = resumeRegles(rg)
	}
	s.store.Noter("bibliothèque", fmt.Sprintf("bestiaire et règles repris : %d monstres ajoutés, %d mis à jour", res.Crees, res.MisAJour), auteur)
	return out, nil
}

func (s *Serveur) bibliothequeExporter(w http.ResponseWriter, r *http.Request) {
	b, err := s.bibliotheque()
	if err != nil {
		erreurStore(w, err)
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="bibliotheque-cof-dm.json"`)
	repondre(w, 200, b)
}

func (s *Serveur) bibliothequeImporter(w http.ResponseWriter, r *http.Request) {
	corps, err := lireCorps(r, tailleImport)
	if err != nil {
		erreurStore(w, err)
		return
	}
	var b Bibliotheque
	if json.Unmarshal(corps, &b) != nil || b.Format != formatBiblio {
		erreur(w, 422, "Ce fichier n'est pas une bibliothèque COF DM (bestiaire et règles d'un univers).")
		return
	}
	out, err := s.recevoirBibliotheque(b, auteurDe(r))
	if err != nil {
		erreurStore(w, err)
		return
	}
	repondre(w, 200, out)
}
