//go:build !windows

package main

import "os/exec"

func sansConsole(*exec.Cmd) {}
