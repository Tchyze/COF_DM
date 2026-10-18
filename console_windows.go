package main

import (
	"os/exec"
	"syscall"
)

// sansConsole empêche une fenêtre de console de s'ouvrir derrière la boîte de dialogue.
func sansConsole(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true} }
