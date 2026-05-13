package app

import (
	"os"
	"syscall"
)

// sendBytes is the common path for control-byte signals (Ctrl-C, Ctrl-\,
// Ctrl-D, …). The byte hits the PTY just like the user pressing the key
// would, so the kernel's tty driver generates the corresponding signal
// for the foreground process group.
func (m *Mux) sendBytes(b ...byte) {
	if t := m.FocusedTerminal(); t != nil {
		_, _ = t.Write(b)
	}
}

func (m *Mux) sendSIGINT()  { m.sendBytes(0x03) } // Ctrl-C
func (m *Mux) sendSIGQUIT() { m.sendBytes(0x1c) } // Ctrl-\
func (m *Mux) sendEOF()     { m.sendBytes(0x04) } // Ctrl-D

// sendSIGTERM signals the child PROCESS directly (not the foreground
// job — useful when the foreground shell is stuck or unresponsive).
// On Windows, Signal(SIGTERM) returns an error; we ignore it.
func (m *Mux) sendSIGTERM() {
	t := m.FocusedTerminal()
	if t == nil {
		return
	}
	pid := t.PID()
	if pid <= 0 {
		return
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return
	}
	_ = proc.Signal(syscall.SIGTERM)
}
