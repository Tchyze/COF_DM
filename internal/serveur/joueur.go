package serveur

import (
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/Tchyze/cof_dm/internal/modele"
	"github.com/Tchyze/cof_dm/internal/store"
)

// Vue des joueurs (mode serveur, rôle « joueur »). Un joueur ne lit que les fiches que le MJ a
// partagées, privées de leurs champs réservés au MJ ; les fiches « Secret » ne lui sont jamais
// montrées.
//
// Tout part d'une liste blanche : seules les routes ci-dessous lui répondent, chacune avec un
// contenu filtré ici. Le reste de l'API (journal, export, graphe, écran du MJ, combats, écritures…)
// lui est fermé, et une route ajoutée plus tard le reste tant qu'elle n'est pas inscrite ici.

// genresJoueur : les genres de champs montrés aux joueurs. Le profil de combat d'un PNJ, les
// créatures d'une rencontre n'en font pas partie.
var genresJoueur = map[string]bool{
	modele.Texte: true, modele.Long: true, modele.Choix: true, modele.Nombre: true, modele.Booleen: true,
	modele.Ref: true, modele.Refs: true, modele.DateMonde: true, modele.DateReelle: true, modele.Etapes: true,
}

func champJoueur(c modele.Champ) bool { return !c.Secret && genresJoueur[c.Genre] }

// partagee : la fiche est visible des joueurs. Une aide de jeu ne l'est jamais : elle n'arrive aux
// joueurs que par un envoi, dans leurs documents (aides.go).
func partagee(e store.Entite) bool {
	return e.Partage && !e.Archive && e.Type != "secret" && e.Type != "aide"
}

func (s *Serveur) routesJoueur() {
	m := http.NewServeMux()
	m.HandleFunc("GET /api/sante", s.sante)
	m.HandleFunc("GET /api/schema", s.schemaJoueur)
	m.HandleFunc("GET /api/calendrier", s.lireCalendrier)
	m.HandleFunc("GET /api/reference", s.referenceLire) // profils et voies : utiles au joueur qui tient sa feuille
	m.HandleFunc("GET /api/stats", s.statsJoueur)
	m.HandleFunc("GET /api/parametres", func(w http.ResponseWriter, r *http.Request) {
		repondre(w, 200, map[string]any{"jeton": "", "adresse": "", "version": s.opt.Version,
			"univers": map[string]any{"nom": nomUnivers(s.store.Chemin()), "id": s.store.ID()}})
	})
	m.HandleFunc("GET /api/entites", s.listerJoueur)
	m.HandleFunc("GET /api/entites/{id}", s.lireJoueur)
	m.HandleFunc("PATCH /api/entites/{id}", s.modifierJoueur)
	m.HandleFunc("GET /api/entites/{id}/feuille", s.feuilleLireJoueur)
	m.HandleFunc("PUT /api/entites/{id}/feuille", s.feuilleEcrireJoueur)
	m.HandleFunc("GET /api/resoudre", s.resoudreJoueur)
	m.HandleFunc("GET /medias/{fichier}", s.mediaJoueur)
	// les documents reçus : routes de la page des joueurs (routesDocumentsJoueurs)
	m.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		erreur(w, 403, "Cette partie de l'univers est réservée au MJ.")
	})
	s.joueur = m
}

// mondeJoueur : les fiches partagées, par identifiant.
func (s *Serveur) mondeJoueur() (map[int64]store.Entite, error) {
	tout, err := s.store.Toutes()
	if err != nil {
		return nil, err
	}
	vues := map[int64]store.Entite{}
	for _, e := range tout {
		if partagee(e) {
			vues[e.ID] = ficheJoueur(e, nil)
		}
	}
	// les champs qui citent d'autres fiches ne gardent que celles qui sont partagées
	for id, e := range vues {
		vues[id] = ficheJoueur(e, vues)
	}
	return vues, nil
}

// ficheJoueur renvoie la fiche telle qu'un joueur la voit : ses seuls champs montrables. Avec vues,
// les champs Ref / Refs sont réduits aux fiches partagées.
func ficheJoueur(e store.Entite, vues map[int64]store.Entite) store.Entite {
	data := map[string]any{}
	if t, ok := modele.TypeDe(e.Type); ok {
		for _, c := range t.Champs {
			v, present := e.Data[c.Cle]
			if !present || !champJoueur(c) {
				continue
			}
			if vues != nil && (c.Genre == modele.Ref || c.Genre == modele.Refs) {
				gardes := []any{}
				for _, id := range idsDe(v) {
					if _, ok := vues[id]; ok {
						gardes = append(gardes, float64(id))
					}
				}
				switch {
				case c.Genre == modele.Refs:
					v = gardes
				case len(gardes) == 1:
					v = gardes[0]
				default:
					continue
				}
			}
			data[c.Cle] = v
		}
	}
	e.Data = data
	return e
}

func resumeJoueur(e store.Entite) store.Resume {
	r := store.Resume{ID: e.ID, Type: e.Type, Nom: e.Nom, Image: e.Image, Tags: e.Tags, Modifie: e.Modifie, Carte: map[string]any{}}
	if t, ok := modele.TypeDe(e.Type); ok {
		for _, k := range t.Carte {
			if v, ok := e.Data[k]; ok { // e.Data est déjà filtré
				r.Carte[k] = v
			}
		}
	}
	return r
}

func (s *Serveur) schemaJoueur(w http.ResponseWriter, r *http.Request) {
	types := []modele.Type{}
	etat := s.store.Fonctionnalites()
	for _, t := range typesActifs(etat, s.typesDeLUnivers()) {
		if t.Cle == "secret" || t.Cle == "aide" {
			continue
		}
		x := modele.Type{Cle: t.Cle, Nom: t.Nom, Pluriel: t.Pluriel, Icone: t.Icone, Champs: []modele.Champ{}, Carte: []string{}}
		montres := map[string]bool{}
		for _, c := range t.Champs {
			if champJoueur(c) {
				x.Champs = append(x.Champs, c)
				montres[c.Cle] = true
			}
		}
		for _, k := range t.Carte {
			if montres[k] {
				x.Carte = append(x.Carte, k)
			}
		}
		types = append(types, x)
	}
	relations := []modele.TypeRelation{}
	for _, r := range modele.Relations {
		if !r.Reservee { // « a reçu » relie un PJ à une aide de jeu, que le joueur ne voit pas comme une fiche
			relations = append(relations, r)
		}
	}
	repondre(w, 200, map[string]any{"types": types, "relations": relations, "calendrier": s.store.Calendrier(), "genres": map[string]string{}, "fonctionnalites": etat})
}

func (s *Serveur) statsJoueur(w http.ResponseWriter, r *http.Request) {
	vues, err := s.mondeJoueur()
	if err != nil {
		erreurStore(w, err)
		return
	}
	compte := map[string]int{}
	campagnes, quetes := []store.Resume{}, []store.Resume{}
	for _, e := range vues {
		compte[e.Type]++
		switch {
		case e.Type == "campagne":
			campagnes = append(campagnes, resumeJoueur(e))
		case e.Type == "quete" && e.Data["statut"] == "en cours":
			quetes = append(quetes, resumeJoueur(e))
		}
	}
	parNom := func(l []store.Resume) {
		sort.Slice(l, func(i, j int) bool { return Plier(l[i].Nom) < Plier(l[j].Nom) })
	}
	parNom(campagnes)
	parNom(quetes)
	repondre(w, 200, map[string]any{"compte": compte, "total": len(vues), "liens_morts": 0, "journal": []any{},
		"campagnes": campagnes, "seances": []any{}, "quetes_en_cours": quetes})
}

// listerJoueur : ?type=, ?tag=, ?q= (sur le nom et les tags seulement : chercher dans les textes
// révélerait ce que disent les champs réservés au MJ), ?limite=.
func (s *Serveur) listerJoueur(w http.ResponseWriter, r *http.Request) {
	vues, err := s.mondeJoueur()
	if err != nil {
		erreurStore(w, err)
		return
	}
	q := r.URL.Query()
	typ, tag, mots := q.Get("type"), strings.ToLower(strings.TrimPrefix(q.Get("tag"), "#")), strings.Fields(Plier(q.Get("q")))
	out := []store.Resume{}
	for _, e := range vues {
		if typ != "" && e.Type != typ {
			continue
		}
		tags := Plier(strings.Join(e.Tags, " "))
		if tag != "" && !contient(e.Tags, tag) {
			continue
		}
		nom, ok := Plier(e.Nom), true
		for _, m := range mots {
			if !strings.Contains(nom, m) && !strings.Contains(tags, m) {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, resumeJoueur(e))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if a, b := Plier(out[i].Nom), Plier(out[j].Nom); a != b {
			return a < b
		}
		return out[i].ID < out[j].ID
	})
	if lim, _ := strconv.Atoi(q.Get("limite")); lim > 0 && len(out) > lim {
		out = out[:lim]
	}
	repondre(w, 200, out)
}

func contient(tags []string, tag string) bool {
	for _, t := range tags {
		if strings.ToLower(t) == tag {
			return true
		}
	}
	return false
}

// lireJoueur : la fiche partagée et ce qui l'entoure, réduit aux fiches partagées. Les relations
// secrètes, celles qui disent « qui sait quoi » et les notes du MJ sur les relations sont retirées ;
// les fiches qui la citent ne sont pas listées.
func (s *Serveur) lireJoueur(w http.ResponseWriter, r *http.Request) {
	id, err := idDe(r)
	if err != nil {
		erreurStore(w, err)
		return
	}
	s.repondreFicheJoueur(w, r, id)
}

func (s *Serveur) repondreFicheJoueur(w http.ResponseWriter, r *http.Request, id int64) {
	vues, err := s.mondeJoueur()
	if err != nil {
		erreurStore(w, err)
		return
	}
	e, ok := vues[id]
	if !ok { // cachée ou inexistante : la même réponse
		erreurStore(w, store.ErrIntrouvable)
		return
	}
	c := ficheComplete{Entite: e, Relations: []store.Relation{}, References: []store.Reference{},
		Mentions: map[string]store.CibleMention{}, RefsNoms: map[string]store.Resume{}, Dates: map[string]string{}}
	rels, err := s.store.Relations(id)
	if err != nil {
		erreurStore(w, err)
		return
	}
	for _, rel := range rels {
		autre, visible := vues[rel.Autre.ID]
		if rel.Secrete || rel.Type == "sait" || !visible {
			continue
		}
		rel.Autre, rel.Note, rel.Contexte = resumeJoueur(autre), "", nil
		if _, ok := vues[rel.Superieur]; !ok {
			rel.Superieur = 0
		}
		c.Relations = append(c.Relations, rel)
	}
	mentions, err := s.store.MentionsDe(e) // e ne porte plus que ses textes montrables
	if err != nil {
		erreurStore(w, err)
		return
	}
	for k, m := range mentions {
		if _, ok := vues[m.ID]; ok {
			c.Mentions[k] = m
		}
	}
	if c.Medias, err = s.store.Medias(id); err != nil {
		erreurStore(w, err)
		return
	}
	t, _ := modele.TypeDe(e.Type)
	cal := s.store.Calendrier()
	sienne := s.ficheDuJoueur(r, id)
	for _, ch := range t.Champs {
		if sienne && ch.Joueur && champJoueur(ch) {
			c.Modifiable = append(c.Modifiable, ch.Cle)
		}
		v, ok := e.Data[ch.Cle]
		if !ok {
			continue
		}
		switch ch.Genre {
		case modele.Ref, modele.Refs:
			for _, x := range idsDe(v) {
				c.RefsNoms[strconv.FormatInt(x, 10)] = resumeJoueur(vues[x])
			}
		case modele.DateMonde:
			if m, ok := v.(map[string]any); ok {
				c.Dates[ch.Cle] = cal.Texte(modele.DateJeu{An: int(num(m["an"])), Mois: int(num(m["mois"])), Jour: int(num(m["jour"]))})
			}
		}
	}
	repondre(w, 200, c)
}

func (s *Serveur) resoudreJoueur(w http.ResponseWriter, r *http.Request) {
	nom := r.URL.Query().Get("nom")
	vues, err := s.mondeJoueur()
	if err != nil {
		erreurStore(w, err)
		return
	}
	cle := strings.ToLower(strings.TrimSpace(nom))
	for _, t := range modele.Types { // comme pour le MJ : le premier type dans l'ordre du modèle
		for _, e := range vues {
			if e.Type == t.Cle && strings.ToLower(strings.TrimSpace(e.Nom)) == cle {
				repondre(w, 200, map[string]any{"id": e.ID, "nom": e.Nom, "type": e.Type, "image": e.Image, "resume": apercu(e)})
				return
			}
		}
	}
	erreur(w, 404, "Aucune fiche « "+nom+" ».")
}

// mediaJoueur : les images des fiches partagées seulement.
func (s *Serveur) mediaJoueur(w http.ResponseWriter, r *http.Request) {
	f := r.PathValue("fichier")
	if !store.NomMediaValide(f) {
		http.NotFound(w, r)
		return
	}
	id, err := s.store.EntiteDuMedia(f)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if e, err := s.store.Lire(id); err != nil || !partagee(e) {
		http.NotFound(w, r)
		return
	}
	s.servirMedia(w, r)
}

// partager : {"partage": true} montre la fiche aux joueurs (sans ses champs réservés au MJ).
func (s *Serveur) partager(w http.ResponseWriter, r *http.Request) {
	id, err := idDe(r)
	if err != nil {
		erreurStore(w, err)
		return
	}
	var in struct {
		Partage bool `json:"partage"`
	}
	if err := lireJSON(r, &in); err != nil {
		erreurStore(w, err)
		return
	}
	if _, err := s.store.Partager(id, in.Partage, auteurDe(r)); err != nil {
		erreurStore(w, err)
		return
	}
	s.repondreFiche(w, 200, id)
}

// ---------------------------------------------------------------- édition par les joueurs

const metaEdition = "edition_joueurs"

// editionJoueurs : l'administrateur a ouvert l'édition aux joueurs dans cet univers.
func (s *Serveur) editionJoueurs() bool {
	v, _ := s.store.Meta(metaEdition)
	return v == "1"
}

// ficheDuJoueur : l'édition est ouverte et cette fiche est le PJ que joue le compte.
func (s *Serveur) ficheDuJoueur(r *http.Request, id int64) bool {
	c, ok := compteDe(r)
	return ok && id != 0 && c.Persos[s.store.ID()] == id && s.editionJoueurs()
}

// reglerEditionJoueurs : {"active": true} laisse chaque joueur modifier, sur la fiche de son PJ, les
// champs que le modèle lui ouvre (Champ.Joueur). Réglage de l'univers, réservé à l'administrateur.
func (s *Serveur) reglerEditionJoueurs(w http.ResponseWriter, r *http.Request) {
	if !surPlace(w, r) {
		return
	}
	var in struct {
		Active bool `json:"active"`
	}
	if err := lireJSON(r, &in); err != nil {
		erreurStore(w, err)
		return
	}
	v, detail := "", "édition par les joueurs fermée"
	if in.Active {
		v, detail = "1", "édition par les joueurs ouverte"
	}
	if err := s.store.SetMeta(metaEdition, v); err != nil {
		erreurStore(w, err)
		return
	}
	s.store.Noter("accès", detail, auteurDe(r))
	s.joueurs.envoyer(map[string]any{"type": "edition", "active": in.Active})
	repondre(w, 200, map[string]any{"edition_joueurs": in.Active})
}

// modifierJoueur : {"data": {…}, "modifie"?} sur la fiche de son PJ, pour les seuls champs ouverts
// aux joueurs. Ni le nom, ni les tags, ni aucun autre champ.
func (s *Serveur) modifierJoueur(w http.ResponseWriter, r *http.Request) {
	id, err := idDe(r)
	if err != nil {
		erreurStore(w, err)
		return
	}
	if !s.ficheDuJoueur(r, id) {
		erreur(w, 403, "Vous ne pouvez modifier que la fiche de votre PJ, quand l'édition par les joueurs est ouverte.")
		return
	}
	e, err := s.store.Lire(id)
	if err != nil || !partagee(e) { // une fiche cachée n'existe pas pour le joueur, même la sienne
		erreurStore(w, store.ErrIntrouvable)
		return
	}
	var in struct {
		Data    map[string]any `json:"data"`
		Attendu float64        `json:"modifie"`
	}
	if err := lireJSON(r, &in); err != nil {
		erreurStore(w, err)
		return
	}
	t, _ := modele.TypeDe(e.Type)
	if len(in.Data) == 0 {
		erreur(w, 422, "Rien à modifier.")
		return
	}
	for k := range in.Data {
		if ch, ok := t.Champ(k); !ok || !ch.Joueur || !champJoueur(*ch) {
			erreur(w, 403, "Le champ « "+k+" » ne se modifie pas par le joueur.")
			return
		}
	}
	if _, err := s.store.Modifier(id, store.Maj{Data: in.Data, Attendu: in.Attendu}, auteurDe(r)); err != nil {
		erreurStore(w, err)
		return
	}
	s.repondreFicheJoueur(w, r, id)
}
