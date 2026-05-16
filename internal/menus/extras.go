package menus

import (
	fvmenus "github.com/oldwired/fv-go/pkg/fv/menus"
)

// ExtrasItem is one dynamically-built menu row — a label and the
// Cm command code that fires when picked. The Mux owns the Cm →
// closure mapping; menus.Build just turns them into fvmenus.Items.
type ExtrasItem struct {
	Label string
	Cm    uint16
}

// Extras bundles every dynamic submenu Build assembles. Empty slices
// suppress the submenu entirely so a fresh fvmux without saved
// sessions doesn't show an empty "Open Session" submenu.
//
// Themes and Profiles deliberately don't appear here — their
// fuzzy pickers (Ctrl-G T and Ctrl-G C) are the single source of
// truth for selection.
type Extras struct {
	Sessions    []ExtrasItem // File → Open Session
	Connections []ExtrasItem // Connections → Active Connections
	Transfers   []ExtrasItem // Transfer → Active Transfers
}

// itemsFromExtras converts a slice of ExtrasItem to fvmenus.Items so
// the existing menu builders can splice them in.
func itemsFromExtras(xs []ExtrasItem) []*fvmenus.Item {
	out := make([]*fvmenus.Item, 0, len(xs))
	for _, x := range xs {
		out = append(out, &fvmenus.Item{Name: x.Label, Command: x.Cm})
	}
	return out
}
