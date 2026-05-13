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
// overwrites would hide a bug.
func (r *Registry) Register(c *Command) {
	if _, dup := r.cmds[c.ID]; dup {
		panic(fmt.Sprintf("commands.Registry: duplicate ID %d (%q)", c.ID, c.Name))
	}
	r.cmds[c.ID] = c
	r.byCategory[c.Category] = append(r.byCategory[c.Category], c)
	if c.Chord != "" {
		r.byChord[c.Chord] = c
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
