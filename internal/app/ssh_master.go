package app

import (
	"io"
	"os/exec"
)

// masterAlive asks ssh whether alias's shared ControlMaster is ready. Output
// is discarded because this probe is runtime state, not terminal content.
func masterAlive(alias, sock string) bool {
	cmd := exec.Command("ssh", "-O", "check", "-o", "ControlPath="+sock, alias)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run() == nil
}
