package serveur

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/Tchyze/cof_dm/internal/combat"
	"github.com/Tchyze/cof_dm/internal/obsidian"
)

// exporterObsidian : l'univers en notes Markdown pour un vault Obsidian, dans un zip.
//
// Par défaut, tout part : secrets du MJ, fiches non partagées et archivées, récapitulatifs des
// combats, journal. Avec ?secrets=0, l'export ne contient que ce que voit un joueur (mondeJoueur) :
// les fiches partagées sans leurs champs réservés au MJ, leurs relations ni secrètes ni « sait »,
// sans note ni contexte, leurs images ; ni combat ni journal.
func (s *Serveur) exporterObsidian(w http.ResponseWriter, r *http.Request) {
	mj := r.URL.Query().Get("secrets") != "0"
	x, err := s.store.Exporter()
	if err != nil {
		erreurStore(w, err)
		return
	}
	u := obsidian.Univers{Calendrier: x.Calendrier, MJ: mj, Monstres: map[int64]string{}}
	if mj {
		u.Fiches, u.Relations, u.Medias = x.Entites, x.Relations, x.Medias
		for _, d := range x.Documents {
			switch d.Coll {
			case collMonstres:
				var m struct {
					Nom string `json:"nom"`
				}
				if json.Unmarshal(d.Data, &m) == nil {
					u.Monstres[d.ID] = m.Nom
				}
			case collCombats:
				var st combat.Etat
				if json.Unmarshal(d.Data, &st) == nil && st.Termine && st.Recap != nil {
					u.Combats = append(u.Combats, obsidian.Combat{ID: d.ID, Etat: &st})
				}
			}
		}
		if u.Journal, err = s.store.JournalEntier(); err != nil {
			erreurStore(w, err)
			return
		}
	} else {
		vues, err := s.mondeJoueur()
		if err != nil {
			erreurStore(w, err)
			return
		}
		for _, e := range x.Entites {
			if v, ok := vues[e.ID]; ok {
				u.Fiches = append(u.Fiches, v)
			}
		}
		for _, rel := range x.Relations {
			_, a := vues[rel.A]
			_, b := vues[rel.B]
			if !a || !b || rel.Secrete || rel.Type == "sait" {
				continue
			}
			rel.Note, rel.Contexte = "", 0
			if _, ok := vues[rel.Superieur]; !ok {
				rel.Superieur = 0
			}
			u.Relations = append(u.Relations, rel)
		}
		for _, m := range x.Medias {
			if _, ok := vues[m.Entite]; ok {
				u.Medias = append(u.Medias, m)
			}
		}
	}
	racine, fichier := nomUnivers(s.store.Chemin()), "cof-dm-obsidian-"+time.Now().Format("2006-01-02")
	if !mj {
		racine += " (joueurs)"
		fichier += "-joueurs"
	}
	var b bytes.Buffer
	if err := obsidian.Ecrire(&b, racine, u); err != nil {
		erreurStore(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.zip"`, fichier))
	_, _ = w.Write(b.Bytes())
}
