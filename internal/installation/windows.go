//go:build windows

package installation

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unicode/utf16"

	"github.com/Tchyze/cof_dm/internal/majour"
)

// NomProgramme : le programme installé, le même nom que le fichier publié.
const NomProgramme = "COF-DM.exe"

// Windows : où s'installe l'application pour cet utilisateur. Bureau et Menu vides : ceux de
// l'utilisateur (lus par PowerShell, ce qui suit une redirection vers OneDrive).
type Windows struct {
	Dossier string // %LOCALAPPDATA%\Programs\COF DM
	Bureau  string
	Menu    string
	Cle     string // nom de la clé de « Applications installées », sous HKCU\…\Uninstall
}

// ParDefaut : l'installation de l'utilisateur courant.
func ParDefaut() Windows {
	return Windows{Dossier: filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "COF DM"), Cle: "COF DM"}
}

// Programme : le programme installé.
func (w Windows) Programme() string { return filepath.Join(w.Dossier, NomProgramme) }

const racineDesinstallation = `Software\Microsoft\Windows\CurrentVersion\Uninstall\`

// VersionInstallee : la version inscrite dans « Applications installées » ("" : rien d'inscrit).
func (w Windows) VersionInstallee() string {
	cmd := exec.Command("reg", "query", `HKCU\`+racineDesinstallation+w.Cle, "/v", "DisplayVersion")
	cacher(cmd)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		if c := strings.Fields(sc.Text()); len(c) == 3 && c[0] == "DisplayVersion" {
			return c[2]
		}
	}
	return ""
}

// Installer copie « source » (ce programme) dans le dossier d'installation, puis crée les raccourcis
// et l'entrée de « Applications installées ». Un programme déjà installé est remplacé comme par une
// mise à jour, même s'il tourne. Renvoie les dossiers Bureau et Téléchargements de l'utilisateur.
func (w Windows) Installer(source, version string) (bureau, telechargements string, err error) {
	if err := os.MkdirAll(w.Dossier, 0o755); err != nil {
		return "", "", fmt.Errorf("dossier d'installation %s : %w", w.Dossier, err)
	}
	tmp, err := os.CreateTemp(w.Dossier, ".cof-dm-maj-*") // retiré par majour.Nettoyer s'il reste
	if err != nil {
		return "", "", fmt.Errorf("dossier d'installation %s : %w", w.Dossier, err)
	}
	in, err := os.Open(source)
	if err == nil {
		_, err = io.Copy(tmp, in)
		in.Close()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp.Name())
		return "", "", fmt.Errorf("copie du programme : %w", err)
	}
	if _, err := os.Stat(w.Programme()); err == nil {
		err = majour.Remplacer(w.Programme(), tmp.Name())
	} else {
		err = os.Rename(tmp.Name(), w.Programme())
	}
	if err != nil {
		os.Remove(tmp.Name())
		return "", "", err
	}
	return w.Integrer(version)
}

// Integrer crée (ou refait) les raccourcis du Bureau et du menu Démarrer et l'entrée de
// « Applications installées », pour le programme installé en « version ». Renvoie les dossiers
// Bureau et Téléchargements de l'utilisateur.
func (w Windows) Integrer(version string) (bureau, telechargements string, err error) {
	taille := int64(0)
	if fi, err := os.Stat(w.Programme()); err == nil {
		taille = fi.Size() / 1024
	}
	out, err := powershell(scriptIntegrer, map[string]string{
		"COFDM_EXE": w.Programme(), "COFDM_DOSSIER": w.Dossier, "COFDM_VERSION": version, "COFDM_TAILLE": fmt.Sprint(taille),
		"COFDM_CLE": racineDesinstallation + w.Cle, "COFDM_BUREAU": w.Bureau, "COFDM_MENU": w.Menu})
	if err != nil {
		return "", "", fmt.Errorf("raccourcis et « Applications installées » : %w", err)
	}
	l := strings.Split(strings.TrimSpace(out), "\n")
	if len(l) >= 2 {
		bureau, telechargements = strings.TrimSpace(l[0]), strings.TrimSpace(l[1])
	}
	return bureau, telechargements, nil
}

const scriptIntegrer = `$ErrorActionPreference = 'Stop'
$bureau = if ($env:COFDM_BUREAU) { $env:COFDM_BUREAU } else { [Environment]::GetFolderPath('Desktop') }
$menu = if ($env:COFDM_MENU) { $env:COFDM_MENU } else { [Environment]::GetFolderPath('Programs') }
$telech = ''
try { $telech = (New-Object -ComObject Shell.Application).NameSpace('shell:Downloads').Self.Path } catch {}
$s = New-Object -ComObject WScript.Shell
foreach ($d in @($bureau, $menu)) {
  New-Item -ItemType Directory -Force -Path $d | Out-Null
  $l = $s.CreateShortcut((Join-Path $d 'COF DM.lnk'))
  $l.TargetPath = $env:COFDM_EXE
  $l.WorkingDirectory = $env:COFDM_DOSSIER
  $l.IconLocation = $env:COFDM_EXE + ',0'
  $l.Description = 'COF DM'
  $l.Save()
}
$k = 'HKCU:\' + $env:COFDM_CLE
New-Item -Path $k -Force | Out-Null
Set-ItemProperty -Path $k -Name DisplayName -Value 'COF DM'
Set-ItemProperty -Path $k -Name DisplayVersion -Value $env:COFDM_VERSION
Set-ItemProperty -Path $k -Name Publisher -Value 'COF DM'
Set-ItemProperty -Path $k -Name DisplayIcon -Value ($env:COFDM_EXE + ',0')
Set-ItemProperty -Path $k -Name InstallLocation -Value $env:COFDM_DOSSIER
Set-ItemProperty -Path $k -Name UninstallString -Value ('"' + $env:COFDM_EXE + '" -desinstaller')
New-ItemProperty -Path $k -Name EstimatedSize -PropertyType DWord -Value ([int]$env:COFDM_TAILLE) -Force | Out-Null
New-ItemProperty -Path $k -Name NoModify -PropertyType DWord -Value 1 -Force | Out-Null
New-ItemProperty -Path $k -Name NoRepair -PropertyType DWord -Value 1 -Force | Out-Null
Write-Output $bureau
Write-Output $telech
`

// Desinstaller retire les raccourcis et l'entrée de « Applications installées », et, si demandé, le
// dossier des données. Le dossier du programme, lui, ne se retire qu'une fois le programme fermé :
// RetirerApresFermeture.
func (w Windows) Desinstaller() error {
	_, err := powershell(scriptDesinstaller, map[string]string{
		"COFDM_CLE": racineDesinstallation + w.Cle, "COFDM_BUREAU": w.Bureau, "COFDM_MENU": w.Menu})
	return err
}

const scriptDesinstaller = `$ErrorActionPreference = 'Stop'
$bureau = if ($env:COFDM_BUREAU) { $env:COFDM_BUREAU } else { [Environment]::GetFolderPath('Desktop') }
$menu = if ($env:COFDM_MENU) { $env:COFDM_MENU } else { [Environment]::GetFolderPath('Programs') }
foreach ($d in @($bureau, $menu)) { Remove-Item -LiteralPath (Join-Path $d 'COF DM.lnk') -Force -ErrorAction SilentlyContinue }
Remove-Item -LiteralPath ('HKCU:\' + $env:COFDM_CLE) -Recurse -Force -ErrorAction SilentlyContinue
`

// RetirerApresFermeture retire le dossier du programme quelques secondes après la fin de ce
// processus (un programme en cours ne se retire pas sous Windows).
func (w Windows) RetirerApresFermeture() error {
	cmd := exec.Command("cmd")
	cmd.Dir = os.TempDir() // pas dans le dossier à retirer
	cacher(cmd)
	// ligne de commande donnée telle quelle : cmd.exe ne lit pas les guillemets échappés à la façon de Go
	cmd.SysProcAttr.CmdLine = `cmd /c "ping -n 4 127.0.0.1 >nul & rmdir /s /q "` + w.Dossier + `""`
	return cmd.Start()
}

// Demander pose une question oui / non dans une boîte de Windows (non par défaut si defautNon).
func Demander(titre, question string, defautNon bool) bool {
	bouton := "Yes"
	if defautNon {
		bouton = "No"
	}
	out, err := powershell(`Add-Type -AssemblyName PresentationFramework
$r = [System.Windows.MessageBox]::Show($env:COFDM_QUESTION, $env:COFDM_TITRE, 'YesNo', 'Question', $env:COFDM_DEFAUT)
Write-Output $r`, map[string]string{"COFDM_QUESTION": question, "COFDM_TITRE": titre, "COFDM_DEFAUT": bouton})
	return err == nil && strings.TrimSpace(out) == "Yes"
}

// Informer affiche un message dans une boîte de Windows.
func Informer(titre, message string) {
	_, _ = powershell(`Add-Type -AssemblyName PresentationFramework
[System.Windows.MessageBox]::Show($env:COFDM_MESSAGE, $env:COFDM_TITRE, 'OK', 'Information') | Out-Null`,
		map[string]string{"COFDM_MESSAGE": message, "COFDM_TITRE": titre})
}

// powershell lance un script, les valeurs passées par l'environnement (jamais dans le texte du
// script : ni guillemets ni chemin à échapper), sans fenêtre de console.
func powershell(script string, env map[string]string) (string, error) {
	// -EncodedCommand : le script en UTF-16 LE et base64, lu d'un bloc (sur l'entrée standard,
	// PowerShell l'exécuterait ligne par ligne et casserait les blocs sur plusieurs lignes)
	u := utf16.Encode([]rune(script))
	brut := make([]byte, 2*len(u))
	for i, c := range u {
		binary.LittleEndian.PutUint16(brut[2*i:], c)
	}
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-EncodedCommand", base64.StdEncoding.EncodeToString(brut))
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	var sortie, erreurs bytes.Buffer
	cmd.Stdout, cmd.Stderr = &sortie, &erreurs
	cacher(cmd)
	if err := cmd.Run(); err != nil {
		if m := strings.TrimSpace(erreurs.String()); m != "" {
			return "", errors.New(m)
		}
		return "", err
	}
	return sortie.String(), nil
}

// cacher : pas de fenêtre de console derrière l'application (programme sans console).
func cacher(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}
