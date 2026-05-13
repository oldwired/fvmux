package app

import "github.com/oldwired/fvmux/internal/commands"

// Dispatch looks up command id in reg and invokes its Action, returning
// true if it matched a registered (and enabled) command. The prefix
// listener calls registry actions directly; this helper is for paths
// that hand fvmux a raw uint16 (menu items, status-line shortcuts, the
// fv-go OnCommand callback).
func Dispatch(reg *commands.Registry, ctx *commands.Ctx, id uint16) bool {
	c := reg.ByID(id)
	if c == nil {
		return false
	}
	if c.Enabled != nil && !c.Enabled(ctx) {
		return false
	}
	if c.Action == nil {
		return false
	}
	c.Action(ctx)
	return true
}
