package commands

import (
	"fmt"
	"sort"
	"strings"
)

// Registry indexes commands by ID, category, and chord. There is one
// registry per running fvmux process, built by Defaults() and (later)
// mutated by keybindings.toml overrides.
type Registry struct {
	cmds       map[uint16]*Command
	byCategory map[string][]*Command
	byChord    map[string]*Command
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

// LookupByName returns the first command whose current Name or legacy alias
// exactly matches. It resolves keybindings.toml's user-facing command field.
func (r *Registry) LookupByName(name string) *Command {
	for _, c := range r.cmds {
		if c.Name == name {
			return c
		}
		for _, alias := range c.Aliases {
			if alias == name {
				return c
			}
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

// All returns every registered command sorted by ID. The by-ID order is a
// contract the cheatsheet generator relies on for stable output. r.cmds is
// keyed by ID, so no de-duplication is needed.
func (r *Registry) All() []*Command {
	out := make([]*Command, 0, len(r.cmds))
	for _, c := range r.cmds {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
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
