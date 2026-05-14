package app

import (
	"log/slog"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/dialogs"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/msgbox"
	"github.com/oldwired/fv-go/pkg/fv/views"
	"github.com/oldwired/fv-go/pkg/fv/widgets/logviewer"

	"github.com/oldwired/fvmux/internal/logs"
)

// showLogViewer opens a modal containing fv-go's logviewer widget
// pre-populated with whatever's currently in the ring (most-recent
// 4096 entries by default). Useful for diagnosing weird behaviour
// without restarting under -log=path.
func (m *Mux) showLogViewer() {
	entries := logs.Entries()
	if len(entries) == 0 {
		msgbox.Show(&m.App.Desktop.Group, msgbox.Info,
			"Log ring is empty.", msgbox.OKOnly)
		return
	}

	desk := m.App.Desktop.BaseView()
	w, h := 120, 30
	if w > desk.Size.X-2 {
		w = desk.Size.X - 2
	}
	if h > desk.Size.Y-2 {
		h = desk.Size.Y - 2
	}
	x := (desk.Size.X - w) / 2
	y := (desk.Size.Y - h) / 2
	d := dialogs.NewDialog(geom.NewRect(x, y, x+w, y+h),
		"Log Viewer (Esc to close)")

	vbar := views.NewScrollBar(geom.NewRect(w-2, 1, w-1, h-3))
	d.Insert(vbar)
	lv := logviewer.New(geom.NewRect(1, 1, w-2, h-3), nil, vbar)
	for _, e := range entries {
		lv.AppendAt(e.Time, mapLevel(e.Level), e.Source, e.Msg)
	}
	d.Insert(lv)
	d.Insert(dialogs.NewButton(
		geom.NewRect(w/2-5, h-2, w/2+5, h-1),
		"Cl~o~se", consts.CmCancel, dialogs.BfDefault,
	))
	m.App.Desktop.ExecView(d)
}

func mapLevel(lv slog.Level) logviewer.Level {
	switch {
	case lv >= slog.LevelError:
		return logviewer.LevelError
	case lv >= slog.LevelWarn:
		return logviewer.LevelWarn
	case lv >= slog.LevelInfo:
		return logviewer.LevelInfo
	default:
		return logviewer.LevelDebug
	}
}
