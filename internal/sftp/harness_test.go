package sftp

import (
	"net"
	"testing"
	"time"

	pkgsftp "github.com/pkg/sftp"
)

// newTestClient starts an in-process pkg/sftp server over an in-memory
// net.Pipe and returns a client connected to it. The server serves the
// real OS filesystem (no chroot), so "remote" paths in tests are just
// absolute paths — point them at t.TempDir(). This lets the transfer
// engine (Manager.copy, remoteReplace, the dir/move helpers) run end to
// end without docker or a system ssh subprocess.
//
// Both ends are torn down via t.Cleanup: closing the client unblocks the
// server's read loop so Serve returns, then we wait for it to drain.
func newTestClient(t *testing.T) *pkgsftp.Client {
	t.Helper()
	clientConn, serverConn := net.Pipe()

	server, err := pkgsftp.NewServer(serverConn)
	if err != nil {
		_ = clientConn.Close()
		_ = serverConn.Close()
		t.Fatalf("NewServer: %v", err)
	}
	srvDone := make(chan struct{})
	go func() {
		defer close(srvDone)
		_ = server.Serve() // returns once the conn closes under it.
	}()

	client, err := pkgsftp.NewClientPipe(clientConn, clientConn)
	if err != nil {
		_ = clientConn.Close()
		_ = server.Close()
		<-srvDone
		t.Fatalf("NewClientPipe: %v", err)
	}

	t.Cleanup(func() {
		_ = client.Close() // closes clientConn → server read errors → Serve returns.
		_ = server.Close() // belt-and-suspenders if the client never connected cleanly.
		select {
		case <-srvDone:
		case <-time.After(2 * time.Second):
			t.Error("sftp test server did not shut down")
		}
	})
	return client
}

// waitTransfer blocks until t leaves StatusActive or the deadline passes,
// then returns the terminal status. Drives the Manager.Start goroutine to
// completion in tests without sleeping on a fixed duration.
func waitTransfer(t *testing.T, tr *Transfer) int32 {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if s := tr.Status(); s != StatusActive {
			return s
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("transfer did not finish within deadline (status=%d)", tr.Status())
	return -1
}
