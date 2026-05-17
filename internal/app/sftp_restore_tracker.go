package app

import "sync"

// sftpRestoreTracker holds one cancel channel per pending
// scheduleSftpRestore poll loop, keyed by SSH alias. session-change
// surfaces (newSession, openSessionPicker) cancel all in-flight
// restores so a stale poll from a previous session can't fire after
// the user has moved on — without this, doing Ctrl-G H to an alias
// the previous session had an SFTP browser for would pop the
// browser unexpectedly once the master came up.
type sftpRestoreTracker struct {
	mu      sync.Mutex
	pending map[string]chan struct{}
}

// start registers a new pending restore for alias and returns the
// channel the goroutine should select on. If another restore for the
// same alias is already pending, it gets cancelled first (last-write-
// wins; a fresh schedule supersedes a stale one).
func (t *sftpRestoreTracker) start(alias string) chan struct{} {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.pending == nil {
		t.pending = map[string]chan struct{}{}
	}
	if old, ok := t.pending[alias]; ok {
		close(old)
	}
	ch := make(chan struct{})
	t.pending[alias] = ch
	return ch
}

// finish removes the channel from the registry, but only if it's
// still the one we registered (start may have replaced it).
func (t *sftpRestoreTracker) finish(alias string, ch chan struct{}) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if cur, ok := t.pending[alias]; ok && cur == ch {
		delete(t.pending, alias)
	}
}

// cancelAll closes every pending channel and empties the map. Called
// from newSession + openSessionPicker before they remove windows /
// load a new snapshot, so stale poll goroutines exit cleanly without
// firing their CallSoon callbacks.
func (t *sftpRestoreTracker) cancelAll() {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, ch := range t.pending {
		close(ch)
	}
	t.pending = nil
}
