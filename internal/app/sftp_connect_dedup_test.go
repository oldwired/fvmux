package app

import "testing"

// TestSftpConnectDedup guards the in-flight guard that keeps a repeated
// Ctrl-G F to the same alias from spawning duplicate SFTP connects while
// one is still negotiating (relevant when the host is slow or dead).
func TestSftpConnectDedup(t *testing.T) {
	m := &Mux{}

	if !m.beginSftpConnect("h") {
		t.Fatal("first connect to an idle alias should proceed")
	}
	if m.beginSftpConnect("h") {
		t.Fatal("second connect while one is in flight must be blocked")
	}
	// A different alias is independent.
	if !m.beginSftpConnect("other") {
		t.Fatal("a different alias should connect independently")
	}

	// Once the first resolves, the alias is connectable again (a browser
	// can be reopened, and multiple browsers to the same alias are allowed
	// once connected — the guard only covers the connecting window).
	m.endSftpConnect("h")
	if !m.beginSftpConnect("h") {
		t.Fatal("after the connect resolved, the alias should connect again")
	}

	// endSftpConnect on an alias that isn't tracked is a harmless no-op.
	m.endSftpConnect("never-started")
}
