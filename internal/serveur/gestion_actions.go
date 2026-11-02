package serveur

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Tchyze/cof_dm/internal/combat"
	"github.com/Tchyze/cof_dm/internal/store"
)

// Archiver, désarchiver et supprimer un univers de l'ordinateur du MJ (voir gestion.go). Le serveur d'une
// copie reliée n'est jamais touché : seul le fichier de cet ordinateur l'est.

// bilanUnivers : ce qu'une action ferait perdre de vue. Attente, Conflits et Recus ne valent que pour une
// copie reliée. Combats : lancés ici et pas terminés, ils ne voyagent pas. Direct : joués sur le serveur,
// ils y continuent, seul l'écran de ce poste s'arrête ; rien à confirmer. Inconnu : le fichier ne s'est
// pas laissé compter (abîmé, ou d'un modèle plus récent).
type bilanUnivers struct {
	Attente  int  `json:"attente"`
	Conflits int  `json:"conflits"`
	Recus    int  `json:"recus"`
	Combats  int  `json:"combats"`
	Direct   int  `json:"direct"`
	Inconnu  bool `json:"inconnu"`
}

// depasse : il y a plus de risques que ce que le MJ a vu et confirmé.
func (b bilanUnivers) depasse(vu bilanUnivers) bool {
	return b.Attente > vu.Attente || b.Conflits > vu.Conflits || b.Recus > vu.Recus || b.Combats > vu.Combats || (b.Inconnu && !vu.Inconnu)
}

func bilanDe(st *store.Store) bilanUnivers {
	var b bilanUnivers
	if st.EstCopie() {
		n, err := st.AEnvoyer()
		cs, err2 := st.Conflits()
		b.Attente, b.Conflits, b.Recus = n, len(cs), st.RecusEnAttente()
		b.Inconnu = err != nil || err2 != nil
	}
	docs, err := st.ListerDocs(collCombats)
	if err != nil {
		b.Inconnu = true
	}
	for _, d := range docs {
		var e combat.Etat
		if json.Unmarshal(d.Data, &e) == nil && !e.Termine {
			b.Combats++
		}
	}
	return b
}

// bilanFerme : un univers fermé se compte sur une copie de son fichier, jetée ensuite : compter écrit
// dans la base, et le fichier du MJ ne doit pas bouger pour une simple question.
func (a *Application) bilanFerme(f ficheGeree) bilanUnivers {
	tmp, err := os.CreateTemp(a.opt.DossierDon, ".bilan-*"+store.Extension)
	if err != nil {
		log.Printf("bilan d'un univers : %v", err)
		return bilanUnivers{Inconnu: true}
	}
	tmp.Close()
	defer func() {
		_ = os.Remove(tmp.Name())
		_ = os.Remove(tmp.Name() + suffixeJournal)
	}()
	if err := copierFichier(f.Chemin, tmp.Name()); err != nil {
		log.Printf("bilan d'un univers : %v", err)
		return bilanUnivers{Inconnu: true}
	}
	st, err := store.Ouvrir(tmp.Name())
	if err != nil {
		return bilanUnivers{Inconnu: true}
	}
	defer st.Fermer()
	return bilanDe(st)
}

// surFichier : Windows ou un antivirus tient parfois un fichier un instant après sa fermeture.
func surFichier(f func() error) error {
	err := f()
	if runtime.GOOS != "windows" {
		return err
	}
	for i := 0; err != nil && !errors.Is(err, os.ErrNotExist) && i < 10; i++ {
		time.Sleep(300 * time.Millisecond)
		err = f()
	}
	return err
}

// routeBilanUnivers : ce que le MJ doit savoir avant d'archiver ou de supprimer, et, pour l'univers
// ouvert, ce qui peut s'ouvrir à sa place. POST : compter écrit dans la base d'un univers ouvert.
func (a *Application) routeBilanUnivers(w http.ResponseWriter, r *http.Request) {
	a.gestion.Lock()
	defer a.gestion.Unlock()
	f, ok := a.ficheParCle(r.PathValue("cle"))
	if !ok {
		erreur(w, 404, "Univers inconnu.")
		return
	}
	var b bilanUnivers
	if f.Ouvert {
		a.mu.RLock()
		o := a.tous[f.id]
		a.mu.RUnlock()
		b = bilanDe(o.st)
		b.Direct = o.srv.salles.nombreDistants()
	} else {
		b = a.bilanFerme(f)
	}
	suivants := []map[string]string{}
	if f.Ouvert {
		for _, x := range a.fichesGerees() {
			if x.Cle != f.Cle && x.peut(actionOuvrir) {
				suivants = append(suivants, map[string]string{"cle": x.Cle, "nom": x.Nom})
			}
		}
	}
	repondre(w, 200, map[string]any{"bilan": b, "suivants": suivants})
}

// refusBilan : le bilan dit plus que ce qui a été confirmé. Le détail repart avec le refus, pour le
// montrer au MJ.
func refusBilan(w http.ResponseWriter, b bilanUnivers) {
	repondre(w, 409, map[string]any{"detail": "Cet univers a des changements en attente, un combat en cours ou un contenu illisible : confirmez d'abord, rien n'a été modifié.", "bilan": b})
}

// suivantDe : l'univers à ouvrir à la place de l'univers ouvert. Il en faut un dès qu'il en existe un ;
// sinon, l'application restera sans univers et proposera d'en créer un.
func (a *Application) suivantDe(f ficheGeree, cle string) (ficheGeree, int, string) {
	if !f.Ouvert {
		return ficheGeree{}, 0, ""
	}
	if cle == "" {
		for _, x := range a.fichesGerees() {
			if x.Cle != f.Cle && x.peut(actionOuvrir) {
				return ficheGeree{}, 422, "Dites quel univers ouvrir à la place de celui-ci."
			}
		}
		return ficheGeree{}, 0, ""
	}
	s, ok := a.ficheParCle(cle)
	if !ok || s.Cle == f.Cle || !s.peut(actionOuvrir) {
		return ficheGeree{}, 422, "Cet univers ne peut pas s'ouvrir à la place de celui-ci."
	}
	return s, 0, ""
}

// quitterOuvert retire du service l'univers ouvert pour l'archiver ou le supprimer : requêtes et échange
// attendus, bilan recompté quand plus rien n'écrit, garder (la sauvegarde), ouverture du suivant, puis
// agir (le fichier). Tant qu'agir n'a pas réussi, l'univers revient tel quel (a.gestion tenu).
func (a *Application) quitterOuvert(w http.ResponseWriter, f, suivant ficheGeree, vu bilanUnivers, garder func(*universOuvert) (int, string), agir func() error) bool {
	a.mu.RLock()
	o := a.tous[f.id]
	a.mu.RUnlock()
	if o == nil {
		erreur(w, 404, "Univers inconnu.")
		return false
	}
	if !a.suspendre(o) {
		erreur(w, 409, msgOccupe)
		return false
	}
	defer a.fermetureFinie(f.id)
	if !a.attendreEchange() {
		a.remettre(o)
		erreur(w, 409, msgOccupe)
		return false
	}
	if b := bilanDe(o.st); b.depasse(vu) {
		a.remettre(o)
		refusBilan(w, b)
		return false
	}
	if garder != nil {
		if code, msg := garder(o); code != 0 {
			a.remettre(o)
			erreur(w, code, msg)
			return false
		}
	}
	var prochain *store.Store
	if suivant.Chemin != "" {
		st, err := store.Ouvrir(suivant.Chemin)
		if err != nil {
			a.remettre(o)
			log.Printf("univers suivant non ouvert : %v", err)
			erreur(w, 422, "« "+suivant.Nom+" » ne s'ouvre pas : rien n'a été modifié. Choisissez-en un autre.")
			return false
		}
		_ = st.RetirerMeta(reglagesAppli...)
		_ = st.RetirerMeta(reglagesPerimes...)
		prochain = st
	}
	a.clore(o)
	if err := surFichier(agir); err != nil {
		if prochain != nil {
			_ = prochain.Fermer()
		}
		if !a.reprendreOuvert(f.Chemin) {
			erreur(w, 500, "L'univers ne se rouvre plus : voyez le journal de l'application.")
			return false
		}
		erreur(w, 500, auJournal("Opération impossible, rien n'a été modifié", err))
		return false
	}
	if prochain != nil {
		a.installer(prochain)
		a.choisi()
	} else {
		a.resterSansUnivers()
	}
	return true
}

// resterSansUnivers : plus aucun univers n'est ouvert, ce choix du MJ vaut aussi au prochain lancement.
func (a *Application) resterSansUnivers() {
	_ = a.reg.SetMeta(regUnivers, "")
	_ = a.reg.SetMeta(regSansUnivers, "1")
	a.choisi()
}

// corpsAction : ce que portent archiver et supprimer.
type corpsAction struct {
	Nom     string       `json:"nom"`
	Suivant string       `json:"suivant"`
	BilanVu bilanUnivers `json:"bilan_vu"`
}

// reponseOuverture : ce qui est ouvert après l'action.
func (a *Application) reponseOuverture(out map[string]any) map[string]any {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out["ouvert"], out["aucun_univers"] = "", a.st == nil
	if a.st != nil {
		out["ouvert"] = a.st.Chemin()
	}
	return out
}

// routeArchiverUnivers : {"suivant"?, "bilan_vu"?} range le fichier dans le dossier des archives, sans
// l'ouvrir ni le modifier. Il sort de la liste courante et des récents. Seul un fichier du dossier des
// univers s'archive : ailleurs, il faudrait le déplacer, parfois d'un disque à l'autre.
func (a *Application) routeArchiverUnivers(w http.ResponseWriter, r *http.Request) {
	var in corpsAction
	if err := lireJSON(r, &in); err != nil {
		erreurStore(w, err)
		return
	}
	a.gestion.Lock()
	defer a.gestion.Unlock()
	f, ok := a.ficheParCle(r.PathValue("cle"))
	if !ok {
		erreur(w, 404, "Univers inconnu.")
		return
	}
	if f.Interrompu {
		erreur(w, 409, msgInterrompu)
		return
	}
	if !f.peut(actionArchiver) {
		erreur(w, 422, "Seul un univers rangé dans le dossier des univers s'archive.")
		return
	}
	dossier := a.DossierArchives()
	if err := os.MkdirAll(dossier, 0o700); err != nil {
		erreur(w, 500, auJournal("Archivage impossible, rien n'a été modifié", err))
		return
	}
	if d, sur := a.archivesSures(); !sur {
		erreur(w, 409, "Le dossier des archives n'est pas un dossier ordinaire : "+d)
		return
	}
	cible := filepath.Join(dossier, filepath.Base(f.Chemin))
	if existe(cible) || existe(cible+suffixeJournal) {
		erreur(w, 409, "Un univers archivé porte déjà ce nom : renommez celui-ci d'abord. Rien n'a été modifié.")
		return
	}
	suivant, code, msg := a.suivantDe(f, in.Suivant)
	if code != 0 {
		erreur(w, code, msg)
		return
	}
	deplacer := func() error { return os.Rename(f.Chemin, cible) }
	if f.Ouvert {
		if !a.quitterOuvert(w, f, suivant, in.BilanVu, func(o *universOuvert) (int, string) {
			if _, err := sauvegardeAuto(o.st, a.dossierSauvegardes(o.st)); err != nil {
				log.Printf("sauvegarde automatique : %v", err)
			}
			return 0, ""
		}, deplacer) {
			return
		}
	} else {
		if b := a.bilanFerme(f); b.depasse(in.BilanVu) {
			refusBilan(w, b)
			return
		}
		if err := surFichier(deplacer); err != nil {
			erreur(w, 500, auJournal("Archivage impossible, rien n'a été modifié", err))
			return
		}
	}
	if err := a.retirerRecent(f.Chemin); err != nil {
		log.Printf("univers récents : %v", err)
	}
	a.noterLienArchive(f, cible, true)
	log.Printf("univers « %s » archivé", f.Nom)
	repondre(w, 200, a.reponseOuverture(map[string]any{"archive": true}))
}

// noterLienArchive : le lien d'une copie reliée suit son fichier et dit qu'elle est archivée. Sans cela,
// elle paraîtrait disparue et serait tirée une seconde fois (a.gestion tenu).
func (a *Application) noterLienArchive(f ficheGeree, cible string, archive bool) {
	liens := a.liens()
	for id, l := range liens {
		if lienDeFichier(l, id, f) {
			l.Chemin, l.Archive = cible, archive
			if err := a.ecrireLien(id, &l); err != nil {
				log.Printf("lien de la copie archivée : %v", err)
			}
		}
	}
}

// lienDeFichier : ce lien mène à ce fichier. Un lien sans chemin, d'une ancienne version, se reconnaît à
// l'identifiant.
func lienDeFichier(l lien, id string, f ficheGeree) bool {
	if l.Chemin == "" {
		return id == f.id && f.id != ""
	}
	return cheminCle(l.Chemin) == cheminCle(f.Chemin)
}

// routeDesarchiverUnivers remet le fichier, identique, dans le dossier des univers. Il n'est pas ouvert.
func (a *Application) routeDesarchiverUnivers(w http.ResponseWriter, r *http.Request) {
	a.gestion.Lock()
	defer a.gestion.Unlock()
	f, ok := a.ficheParCle(r.PathValue("cle"))
	if !ok {
		erreur(w, 404, "Univers inconnu.")
		return
	}
	if !f.peut(actionDesarchiver) {
		erreur(w, 422, "Cet univers n'est pas archivé.")
		return
	}
	cible := filepath.Join(a.DossierUnivers(), filepath.Base(f.Chemin))
	if existe(cible) || existe(cible+suffixeJournal) {
		erreur(w, 409, "Un univers porte déjà ce nom : renommez l'un des deux d'abord. Rien n'a été modifié.")
		return
	}
	if err := os.MkdirAll(a.DossierUnivers(), 0o700); err != nil {
		erreur(w, 500, auJournal("Désarchivage impossible, rien n'a été modifié", err))
		return
	}
	if err := surFichier(func() error { return os.Rename(f.Chemin, cible) }); err != nil {
		erreur(w, 500, auJournal("Désarchivage impossible, rien n'a été modifié", err))
		return
	}
	a.noterLienArchive(f, cible, false)
	log.Printf("univers « %s » désarchivé", f.Nom)
	n, _ := a.ficheParCle(cleFichier(cible))
	repondre(w, 200, map[string]any{"univers": n})
}

// sauvegarderFerme copie un univers fermé dans son dossier de sauvegardes et relit la copie. Un fichier
// abîmé ne se relit pas : sa copie n'est pas vérifiée, et la réponse le dit.
func (a *Application) sauvegarderFerme(f ficheGeree) (string, bool, error) {
	id := f.id
	if id == "" {
		id = "sans-identifiant-" + f.Cle
	}
	dossier := filepath.Join(a.opt.DossierDon, "sauvegardes", id)
	if err := os.MkdirAll(dossier, 0o700); err != nil {
		return "", false, err
	}
	copie := filepath.Join(dossier, nomSauvegarde(motifAvantSuppression))
	if err := copierFichier(f.Chemin, copie); err != nil {
		_ = os.Remove(copie)
		return "", false, err
	}
	source, err1 := os.Stat(f.Chemin)
	ecrite, err2 := os.Stat(copie)
	if err1 != nil || err2 != nil || source.Size() != ecrite.Size() {
		_ = os.Remove(copie)
		return "", false, errors.New("la copie n'a pas la taille de l'univers")
	}
	if f.Abime {
		return copie, false, nil
	}
	if _, err := store.Apercu(copie); err != nil {
		_ = os.Remove(copie)
		return "", false, err
	}
	return copie, true, nil
}

// routeSupprimerUnivers : {"nom", "suivant"?, "bilan_vu"?}. Le nom retapé confirme. Une sauvegarde est
// faite et relue avant, rangée avec les autres sauvegardes de l'univers, qui restent : si elle échoue,
// rien n'est supprimé. Pour une copie reliée, seule la copie de cet ordinateur part.
func (a *Application) routeSupprimerUnivers(w http.ResponseWriter, r *http.Request) {
	var in corpsAction
	if err := lireJSON(r, &in); err != nil {
		erreurStore(w, err)
		return
	}
	a.gestion.Lock()
	defer a.gestion.Unlock()
	f, ok := a.ficheParCle(r.PathValue("cle"))
	if !ok {
		erreur(w, 404, "Univers inconnu.")
		return
	}
	if f.Interrompu {
		erreur(w, 409, msgInterrompu)
		return
	}
	if !f.peut(actionSupprimer) {
		erreur(w, 422, "Ce fichier ne se supprime pas ici : ce n'est pas un fichier d'univers ordinaire.")
		return
	}
	if strings.TrimSpace(in.Nom) != f.Nom {
		erreur(w, 422, "Retapez le nom de l'univers (« "+f.Nom+" ») pour confirmer sa suppression.")
		return
	}
	suivant, code, msg := a.suivantDe(f, in.Suivant)
	if code != 0 {
		erreur(w, code, msg)
		return
	}
	sauvegarde, verifiee := "", false
	retirer := func() error { return os.Remove(f.Chemin) }
	if f.Ouvert {
		if !a.quitterOuvert(w, f, suivant, in.BilanVu, func(o *universOuvert) (int, string) {
			dossier := a.dossierSauvegardes(o.st)
			if err := os.MkdirAll(dossier, 0o700); err != nil {
				return 500, auJournal("Sauvegarde impossible, rien n'a été supprimé", err)
			}
			s, err := sauvegarder(o.st, dossier, motifAvantSuppression)
			if err == nil {
				_, err = store.Apercu(s)
			}
			if err != nil {
				return 500, auJournal("Sauvegarde impossible, rien n'a été supprimé", err)
			}
			sauvegarde, verifiee = s, true
			return 0, ""
		}, retirer) {
			return
		}
	} else {
		if b := a.bilanFerme(f); b.depasse(in.BilanVu) {
			refusBilan(w, b)
			return
		}
		s, v, err := a.sauvegarderFerme(f)
		if err != nil {
			erreur(w, 500, auJournal("Sauvegarde impossible, rien n'a été supprimé", err))
			return
		}
		sauvegarde, verifiee = s, v
		if err := surFichier(retirer); err != nil {
			erreur(w, 500, auJournal("Suppression impossible", err))
			return
		}
	}
	if err := a.retirerRecent(f.Chemin); err != nil {
		log.Printf("univers récents : %v", err)
	}
	for id, l := range a.liens() {
		if lienDeFichier(l, id, f) {
			if err := a.ecrireLien(id, nil); err != nil {
				log.Printf("lien de la copie supprimée : %v", err)
			}
		}
	}
	log.Printf("univers « %s » supprimé ; sauvegarde : %s", f.Nom, sauvegarde)
	repondre(w, 200, a.reponseOuverture(map[string]any{"supprime": true, "sauvegarde": sauvegarde,
		"dossier_sauvegardes": filepath.Dir(sauvegarde), "verifiee": verifiee}))
}
