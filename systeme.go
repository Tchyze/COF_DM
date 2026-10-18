package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func ouvrirNavigateur(url string) {
	switch runtime.GOOS {
	case "windows":
		_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		_ = exec.Command("open", url).Start()
	default:
		_ = exec.Command("xdg-open", url).Start()
	}
}

// alerte affiche une erreur bloquante à l'utilisateur quand il n'y a pas de console.
func alerte(msg string) {
	switch runtime.GOOS {
	case "windows":
		_ = exec.Command("mshta", "javascript:alert("+jsChaine("COF DM\n\n"+msg)+");close()").Run()
	case "linux":
		if _, err := exec.LookPath("zenity"); err == nil {
			_ = exec.Command("zenity", "--error", "--title=COF DM", "--text="+msg).Run()
		} else if _, err := exec.LookPath("notify-send"); err == nil {
			_ = exec.Command("notify-send", "COF DM", msg).Run()
		}
	}
}

func jsChaine(s string) string {
	out := []rune{'"'}
	for _, r := range s {
		switch r {
		case '"', '\\':
			out = append(out, '\\', r)
		case '\n':
			out = append(out, '\\', 'n')
		default:
			out = append(out, r)
		}
	}
	return string(append(out, '"'))
}

// ---------------------------------------------------------------- boîte « Ouvrir / Enregistrer »

// dialogueDisponible : le système offre une boîte de dialogue de fichiers (sinon le chemin se
// saisit dans l'interface).
func dialogueDisponible() bool {
	switch runtime.GOOS {
	case "windows":
		_, err := exec.LookPath("powershell")
		return err == nil
	case "linux":
		if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
			return false
		}
		_, err1 := exec.LookPath("zenity")
		_, err2 := exec.LookPath("kdialog")
		return err1 == nil || err2 == nil
	}
	return false
}

// dialogueFichier ouvre la boîte de dialogue du système pour choisir un fichier d'univers, à
// ouvrir ou à enregistrer (nom proposé, sans extension). Renvoie "" si l'utilisateur annule.
func dialogueFichier(enregistrer bool, nom, dossier string) (string, error) {
	propose := ""
	if enregistrer && nom != "" {
		propose = nom + ".cofdm"
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		classe, options := "OpenFileDialog", ""
		if enregistrer {
			classe, options = "SaveFileDialog", "$d.OverwritePrompt = $true; $d.DefaultExt = 'cofdm'; $d.FileName = "+psChaine(propose)+"; "
		}
		script := "[Console]::OutputEncoding = [Text.Encoding]::UTF8; Add-Type -AssemblyName System.Windows.Forms; " +
			"$d = New-Object System.Windows.Forms." + classe + "; $d.Filter = 'Univers COF DM (*.cofdm)|*.cofdm|Tous les fichiers (*.*)|*.*'; " +
			"$d.InitialDirectory = " + psChaine(dossier) + "; " + options +
			"if ($d.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) { [Console]::Out.Write($d.FileName) }"
		cmd = exec.Command("powershell", "-NoProfile", "-STA", "-NonInteractive", "-Command", script)
	case "linux":
		depart := filepath.Join(dossier, propose)
		if propose == "" {
			depart = dossier + string(filepath.Separator)
		}
		if _, err := exec.LookPath("zenity"); err == nil {
			args := []string{"--file-selection", "--title=COF DM", "--filename=" + depart, "--file-filter=Univers COF DM | *.cofdm", "--file-filter=Tous les fichiers | *"}
			if enregistrer {
				args = append(args, "--save", "--confirm-overwrite")
			}
			cmd = exec.Command("zenity", args...)
		} else if _, err := exec.LookPath("kdialog"); err == nil {
			mode := "--getopenfilename"
			if enregistrer {
				mode = "--getsavefilename"
			}
			cmd = exec.Command("kdialog", "--title", "COF DM", mode, depart, "Univers COF DM (*.cofdm)")
		}
	}
	if cmd == nil {
		return "", errors.New("aucune boîte de dialogue sur ce système")
	}
	sansConsole(cmd)
	out, err := cmd.Output()
	var sortie *exec.ExitError
	if errors.As(err, &sortie) {
		return "", nil // annulé
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// psChaine écrit une chaîne PowerShell entre apostrophes.
func psChaine(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
