//go:build windows

package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/Tchyze/cof_dm/internal/installation"
	"github.com/Tchyze/cof_dm/internal/majour"
	"github.com/Tchyze/cof_dm/internal/serveur"
)

// Variables d'environnement passées au programme installé, lancé à la place de celui-ci.
const (
	envRetirer = "COF_DM_RETIRER" // la copie à retirer (l'exe resté sur le Bureau ou dans les Téléchargements)
	envDeplace = "COF_DM_DEPLACE" // l'ancien emplacement du programme (Claude le lançait peut-être de là)
)

// installerAuLancement : une version publiée de l'application s'installe pour l'utilisateur, sans
// droits d'administrateur, dans %LOCALAPPDATA%\Programs\COF DM (raccourcis du Bureau et du menu
// Démarrer, « Applications installées »), puis lance le programme installé. Une copie plus ancienne
// que l'installé lance l'installé sans rien remplacer. Une copie posée sur le Bureau ou dans les
// Téléchargements est ensuite retirée. Renvoie true si ce programme doit s'arrêter là, et un mot
// pour le MJ si l'installation a échoué (l'application s'ouvre alors d'où elle est).
func installerAuLancement(avertir string) (bool, string) {
	if _, publiee := majour.Lire(version); !publiee || os.Getenv("LOCALAPPDATA") == "" {
		return false, "" // version de développement : elle s'ouvre d'où elle est
	}
	w := installation.ParDefaut()
	exe := programmeCourant()
	if exe == "" {
		return false, ""
	}
	installee := w.VersionInstallee()
	_, err := os.Stat(w.Programme())
	switch installation.Decider(exe, w.Programme(), err == nil, version, installee) {
	case installation.Continuer:
		apresInstallation(w, installee)
		return false, ""
	case installation.Installer:
		bureau, telech, err := w.Installer(exe, version)
		if err != nil {
			log.Printf("installation : %v", err)
			return false, fmt.Sprintf("COF DM n'a pas pu s'installer dans « %s » (%v) : il s'ouvre depuis « %s ».", w.Dossier, err, filepath.Dir(exe))
		}
		log.Printf("COF DM %s installé dans %s", version, w.Dossier)
		return lancerInstalle(w, exe, bureau, telech, avertir), ""
	default: // LancerInstalle : l'installé est aussi récent
		bureau, telech, err := w.Integrer(installee) // les raccourcis, s'ils ont disparu
		if err != nil {
			log.Printf("installation : %v", err)
		}
		return lancerInstalle(w, exe, bureau, telech, avertir), ""
	}
}

// lancerInstalle lance le programme installé avec les mêmes options ; il retirera « exe » si c'est
// une copie du Bureau ou des Téléchargements.
func lancerInstalle(w installation.Windows, exe, bureau, telech, avertir string) bool {
	cmd := exec.Command(w.Programme(), os.Args[1:]...)
	cmd.Dir = w.Dossier
	cmd.Env = append(os.Environ(), envDeplace+"="+exe)
	if d := filepath.Dir(exe); (bureau != "" && installation.MemeChemin(d, bureau)) || (telech != "" && installation.MemeChemin(d, telech)) {
		cmd.Env = append(cmd.Env, envRetirer+"="+exe)
	}
	if avertir != "" {
		cmd.Env = append(cmd.Env, envAvertir+"="+avertir)
	}
	if err := cmd.Start(); err != nil {
		log.Printf("lancement du programme installé : %v", err)
		return false // il s'ouvre d'ici
	}
	return true
}

// apresInstallation : le programme installé vient d'être lancé. Après une installation ou une mise à
// jour, sa version est inscrite et ses raccourcis refaits ; Claude suit son nouvel emplacement ; la
// copie d'où il a été installé est retirée (elle tourne encore un instant : réessais).
func apresInstallation(w installation.Windows, installee string) {
	if installee != version {
		go func() {
			if _, _, err := w.Integrer(version); err != nil {
				log.Printf("installation : %v", err)
			}
		}()
	}
	if ancien := os.Getenv(envDeplace); ancien != "" {
		os.Unsetenv(envDeplace)
		if err := serveur.SuivreClaude(ancien, w.Programme()); err != nil {
			log.Printf("Claude : nouvel emplacement de COF DM non inscrit : %v", err)
		}
	}
	if copie := os.Getenv(envRetirer); copie != "" {
		os.Unsetenv(envRetirer)
		go func() {
			var err error
			for i := 0; i < 60; i++ {
				if err = os.Remove(copie); err == nil || os.IsNotExist(err) {
					log.Printf("copie retirée : %s (le raccourci « COF DM » la remplace)", copie)
					return
				}
				time.Sleep(time.Second)
			}
			log.Printf("copie non retirée : %s : %v", copie, err)
		}()
	}
}

// desinstaller : lancé par « Applications installées ». Retire le programme, ses raccourcis et son
// inscription ; les données ne partent que si l'utilisateur le demande.
func desinstaller() {
	const titre = "Désinstaller COF DM"
	w := installation.ParDefaut()
	donnees := dossierParDefaut()
	if !installation.Demander(titre, "Désinstaller COF DM de cet ordinateur ?\n\nVos univers, réglages et sauvegardes restent dans « "+donnees+" », sauf si vous choisissez de les supprimer à l'étape suivante.", false) {
		return
	}
	if applicationOuverte(donnees) {
		installation.Informer(titre, "COF DM est ouvert : fermez-le, puis recommencez la désinstallation.")
		return
	}
	supprimer := installation.Demander(titre, "Supprimer aussi vos données (univers, réglages, sauvegardes) de « "+donnees+" » ?\n\nNon conseillé : elles serviront si vous réinstallez COF DM. Les univers enregistrés ailleurs ne sont pas touchés.", true)
	if err := serveur.SuivreClaude(w.Programme(), ""); err != nil {
		log.Printf("Claude : %v", err)
	}
	if err := w.Desinstaller(); err != nil {
		installation.Informer(titre, "La désinstallation n'a pas pu se faire : "+err.Error())
		return
	}
	message := "COF DM est désinstallé. Vos données restent dans « " + donnees + " »."
	if supprimer {
		if err := os.RemoveAll(donnees); err != nil {
			message = "COF DM est désinstallé, mais une partie de vos données n'a pas pu être supprimée de « " + donnees + " » : " + err.Error()
		} else {
			message = "COF DM est désinstallé, et ses données supprimées."
		}
	}
	if err := w.RetirerApresFermeture(); err != nil {
		message += " Le dossier « " + w.Dossier + " » reste à supprimer à la main."
	}
	installation.Informer(titre, message)
}

// applicationOuverte : une application COF DM répond sur le port qu'elle a publié.
func applicationOuverte(donnees string) bool {
	c := http.Client{Timeout: time.Second}
	r, err := c.Get(fmt.Sprintf("http://127.0.0.1:%d/api/sante", portApplication(donnees)))
	if err != nil {
		return false
	}
	r.Body.Close()
	return true
}
