package app

import (
	"fmt"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/dialogs"
	"github.com/oldwired/fv-go/pkg/fv/geom"
)

// aboutBody is the text of the About dialog, with %s for the fvmux
// build version supplied at runtime.
//
// The fv-go credit is given LGPL-2.1 prominence on purpose: fv-go is
// the framework fvmux is built against, and LGPL-2.1 requires
// downstreams to make the user aware of the library and how to obtain
// its source. Pointing at the public github.com/oldwired/fv-go repo
// (which go.mod also pins) satisfies the relinking guarantee for a
// statically-linked Go binary.
const aboutBody = "" +
	"  fvmux %s\n" +
	"  A floating-window terminal multiplexer.\n" +
	"\n" +
	"  Copyright (c) 2026 oldwired.\n" +
	"  Released under the MIT License — see LICENSE for the full text.\n" +
	"\n" +
	"  Built on fv-go (LGPL-2.1) — a Go port of the classic\n" +
	"  Free Vision / Turbo Vision desktop.\n" +
	"      Source: github.com/oldwired/fv-go\n" +
	"      License: GNU Lesser General Public License v2.1\n" +
	"\n" +
	"  Other dependencies:\n" +
	"      BurntSushi/toml, atotto/clipboard,\n" +
	"      kevinburke/ssh_config, pkg/sftp,\n" +
	"      shirou/gopsutil, kr/fs, creack/pty,\n" +
	"      golang.org/x/{crypto,sys,term,image}.\n" +
	"\n" +
	"  Full third-party notices: fvmux.spdx.json (release page)."

// showAbout opens a centred modal with version + license credits.
// Wired to commands.CmdAbout — appears as Help → About fvmux…
func (m *Mux) showAbout() {
	desk := m.App.Desktop.BaseView()
	w, h := 66, 22
	if w > desk.Size.X-2 {
		w = desk.Size.X - 2
	}
	if h > desk.Size.Y-2 {
		h = desk.Size.Y - 2
	}
	x := (desk.Size.X - w) / 2
	y := (desk.Size.Y - h) / 2

	body := fmt.Sprintf(aboutBody, m.Opts.Version)

	d := dialogs.NewDialog(geom.NewRect(x, y, x+w, y+h), "About fvmux")
	d.Insert(dialogs.NewStaticText(geom.NewRect(2, 2, w-2, h-4), body))
	d.Insert(dialogs.NewButton(
		geom.NewRect(w/2-5, h-3, w/2+5, h-2),
		"O~K~", consts.CmOK, dialogs.BfDefault,
	))
	m.App.Desktop.ExecView(d)
}
