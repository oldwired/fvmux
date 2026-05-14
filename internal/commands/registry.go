package commands

import (
	"fmt"
	"strings"
)

// Registry indexes commands by ID, category, and chord. There is one
// registry per running fvmux process, built by Defaults() and (later)
// mutated by keybindings.toml overrides.
type Registry struct {
	cmds       map[uint16]*Command
	byCategory map[string][]*Command
	byChord    map[string]*Command
	providers  []func(*Ctx) []*Command
}

func New() *Registry {
	return &Registry{
		cmds:       map[uint16]*Command{},
		byCategory: map[string][]*Command{},
		byChord:    map[string]*Command{},
	}
}

// Register adds c to the registry. Panics on duplicate ID — silent
// overwrites would hide a bug. Captures c.Chord into FactoryChord so
// ResetChords() can restore the baseline on config reload.
func (r *Registry) Register(c *Command) {
	if _, dup := r.cmds[c.ID]; dup {
		panic(fmt.Sprintf("commands.Registry: duplicate ID %d (%q)", c.ID, c.Name))
	}
	c.FactoryChord = c.Chord
	r.cmds[c.ID] = c
	r.byCategory[c.Category] = append(r.byCategory[c.Category], c)
	if c.Chord != "" {
		r.byChord[c.Chord] = c
	}
}

// LookupByName returns the (first) command whose Name exactly matches.
// Used by keybindings.toml override application — the command field in
// the TOML is the user-facing Name.
func (r *Registry) LookupByName(name string) *Command {
	for _, c := range r.cmds {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// ResetChords reverts every command's chord to its FactoryChord and
// rebuilds the byChord index. Used by Mux.ReloadConfig before
// re-applying prefix + keybindings.toml overrides, so a previously-
// overridden binding that the user has now removed reverts cleanly.
func (r *Registry) ResetChords() {
	r.byChord = map[string]*Command{}
	for _, c := range r.cmds {
		c.Chord = c.FactoryChord
		if c.Chord != "" {
			r.byChord[c.Chord] = c
		}
	}
}

// Override is one entry in keybindings.toml — a chord that should be
// bound to the named command, or removed entirely when Command is "".
type Override struct {
	Chord   string
	Command string // empty ⇒ remove whatever's currently bound to Chord.
}

// ApplyOverrides walks overrides in order and mutates the registry to
// match. Empty-string Command removes the binding from whatever
// command currently owns it. A non-empty Command both unbinds anything
// currently on the chord AND replaces the named command's chord.
//
// Unknown command names are skipped silently — keybindings.toml is
// user-authored and we don't want to crash on typos. The caller can
// inspect the registry afterward for diagnostics.
func (r *Registry) ApplyOverrides(overrides []Override) {
	for _, o := range overrides {
		if o.Chord == "" {
			continue
		}
		if o.Command == "" {
			if c := r.byChord[o.Chord]; c != nil {
				delete(r.byChord, o.Chord)
				c.Chord = ""
			}
			continue
		}
		target := r.LookupByName(o.Command)
		if target == nil {
			continue
		}
		// Free up the chord if some other command currently holds it.
		if other := r.byChord[o.Chord]; other != nil && other != target {
			other.Chord = ""
		}
		// Strip target's old binding (if any) from byChord first.
		if target.Chord != "" {
			delete(r.byChord, target.Chord)
		}
		target.Chord = o.Chord
		r.byChord[o.Chord] = target
	}
}

// ByID returns the command with the given ID, or nil if unknown.
func (r *Registry) ByID(id uint16) *Command { return r.cmds[id] }

// ByCategory returns commands grouped under cat in registration order.
func (r *Registry) ByCategory(cat string) []*Command { return r.byCategory[cat] }

// LookupChord returns the command bound to chord, or nil if unbound.
func (r *Registry) LookupChord(chord string) *Command { return r.byChord[chord] }

// All returns every registered command. Iteration order is by ID for
// stability (matters for the cheatsheet generator).
func (r *Registry) All() []*Command {
	out := make([]*Command, 0, len(r.cmds))
	seen := map[uint16]bool{}
	for _, c := range r.cmds {
		if seen[c.ID] {
			continue
		}
		seen[c.ID] = true
		out = append(out, c)
	}
	sortByID(out)
	return out
}

// RegisterProvider hooks in a dynamic-entry source (themes, profiles,
// sessions, active connections, …). Providers are queried at palette
// open and menu open; they do not pollute byID / byChord indexes.
func (r *Registry) RegisterProvider(p func(*Ctx) []*Command) {
	r.providers = append(r.providers, p)
}

// RebindPrefix rewrites every chord step matching oldToken to newToken
// — e.g., RebindPrefix("C-g", "C-b") turns "C-g %" into "C-b %" and
// "C-g C-g" into "C-b C-b". Idempotent: no-op when old==new. The
// byChord index is rebuilt so LookupChord works against the new
// chords immediately.
func (r *Registry) RebindPrefix(oldToken, newToken string) {
	if oldToken == newToken || oldToken == "" || newToken == "" {
		return
	}
	r.byChord = map[string]*Command{}
	for _, c := range r.cmds {
		if c.Chord != "" {
			c.Chord = rebindChord(c.Chord, oldToken, newToken)
		}
		if c.Chord != "" {
			r.byChord[c.Chord] = c
		}
	}
}

func rebindChord(chord, oldToken, newToken string) string {
	parts := strings.Fields(chord)
	for i, p := range parts {
		if p == oldToken {
			parts[i] = newToken
		}
	}
	return strings.Join(parts, " ")
}

// Providers returns the merged set of dynamic entries for ctx.
func (r *Registry) Providers(ctx *Ctx) []*Command {
	var out []*Command
	for _, p := range r.providers {
		out = append(out, p(ctx)...)
	}
	return out
}

func sortByID(cmds []*Command) {
	for i := 1; i < len(cmds); i++ {
		for j := i; j > 0 && cmds[j-1].ID > cmds[j].ID; j-- {
			cmds[j-1], cmds[j] = cmds[j], cmds[j-1]
		}
	}
}
