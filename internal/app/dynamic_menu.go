package app

import (
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/oldwired/fvmux/internal/menus"
	"github.com/oldwired/fvmux/internal/sftp"
)

// dynamicCmBase reserves Cm codes 0x8000..0x8FFF for dynamic menu
// items. Each menu rebuild reassigns codes starting at the base; the
// closure-table is rebuilt fresh so stale codes from the previous
// rebuild can't accidentally fire.
const dynamicCmBase uint16 = 0x8000

// dispatchTable maps dynamic Cm codes to the closures menus.BuildWithExtras
// produced. Wrapped in a mutex so concurrent OnCommand callbacks (one
// from the menu, one from a timer) see a coherent snapshot.
type dispatchTable struct {
	mu      sync.Mutex
	actions map[uint16]func()
}

func newDispatchTable() *dispatchTable {
	return &dispatchTable{actions: map[uint16]func(){}}
}

func (d *dispatchTable) reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.actions = map[uint16]func(){}
}

func (d *dispatchTable) set(cm uint16, fn func()) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.actions[cm] = fn
}

func (d *dispatchTable) run(cm uint16) bool {
	d.mu.Lock()
	fn, ok := d.actions[cm]
	d.mu.Unlock()
	if !ok || fn == nil {
		return false
	}
	fn()
	return true
}

// DispatchDynamic runs the closure registered for cm during the most
// recent menu rebuild. Returns true if a handler ran. Wired into
// main.go's OnCommand chain before the registry-based dispatch.
func (m *Mux) DispatchDynamic(cm uint16) bool {
	if m.dynamic == nil {
		return false
	}
	return m.dynamic.run(cm)
}

// BuildMenuExtras assembles the dynamic-submenu payload for the next
// menus.BuildWithExtras call. Side effect: rebuilds the Mux's
// dispatch table so the freshly-assigned Cm codes route correctly.
func (m *Mux) BuildMenuExtras() menus.Extras {
	if m.dynamic == nil {
		m.dynamic = newDispatchTable()
	}
	m.dynamic.reset()
	next := dynamicCmBase

	alloc := func(fn func()) uint16 {
		cm := next
		next++
		m.dynamic.set(cm, fn)
		return cm
	}

	var ex menus.Extras

	// Themes, Profiles, and Sessions intentionally do NOT populate
	// dynamic submenus — Ctrl-G T, Ctrl-G C, and Ctrl-G s each own a
	// fuzzy picker so there's one obvious surface per kind.

	// Active SSH masters.
	if m.sshPool != nil {
		for _, c := range m.sshPool.Snapshot() {
			c := c
			label := fmt.Sprintf("%s — %d ref, up %s",
				c.Alias, c.Refs, time.Since(c.Started).Truncate(time.Second))
			ex.Connections = append(ex.Connections, menus.ExtrasItem{
				Label: label,
				Cm:    alloc(func() { /* read-only entry; click does nothing */ }),
			})
		}
	}

	// Active SFTP transfers across every open browser.
	for _, mgr := range sftp.LiveManagers() {
		for _, t := range mgr.Snapshot() {
			t := t
			short := filepath.Base(t.LocalPath)
			if t.Direction == sftp.Download {
				short = filepath.Base(t.RemotePath)
			}
			label := fmt.Sprintf("%s — %d/%d bytes", short, t.Bytes(), t.Size)
			ex.Transfers = append(ex.Transfers, menus.ExtrasItem{
				Label: label,
				Cm:    alloc(func() { /* read-only */ }),
			})
		}
	}

	return ex
}
