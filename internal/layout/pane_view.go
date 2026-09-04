package layout

import (
	"strings"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	fvtheme "github.com/oldwired/fv-go/pkg/fv/theme"
	fvutf8 "github.com/oldwired/fv-go/pkg/fv/utf8"
	"github.com/oldwired/fv-go/pkg/fv/views"

	"github.com/oldwired/fvmux/internal/session"
	"github.com/oldwired/fvmux/internal/ui"
)

// paneView gives each leaf in a split window a compact heading. Lower panes
// paint it over their existing horizontal divider; only panes at the top of
// the window reserve a row. A reserved heading disappears when its pane is too
// short, so deeply nested layouts degrade to terminals instead of all chrome.
type paneView struct {
	views.Group

	pane   *session.Pane
	header *paneHeader
	// overlayDivider places the heading at y=-1, on the horizontal
	// splitter immediately above this pane, instead of reserving a row.
	overlayDivider bool
}

func newPaneView(bounds geom.Rect, pane *session.Pane, overlayDivider bool) *paneView {
	v := &paneView{pane: pane, overlayDivider: overlayDivider}
	views.InitGroup(&v.Group, bounds)
	v.SetSelf(v)
	v.GrowMode = consts.GfGrowHiX | consts.GfGrowHiY
	v.header = newPaneHeader(geom.Rect{}, pane, overlayDivider)
	v.InsertPassive(v.header)
	v.Insert(pane.Term)
	// Each paneView is assembled before its parent SplitGroup decides which
	// branch is active. Insert makes the terminal locally focused, so clear
	// that provisional flag; app.setPaneFocus will restore it only down the
	// selected owner chain after the complete tree is installed.
	pane.Term.SetState(consts.SfFocused, false)
	v.recalc()
	return v
}

func (v *paneView) ChangeBounds(bounds geom.Rect) {
	v.SetBounds(bounds)
	v.Clip = v.GetExtent()
	v.recalc()
}

func (v *paneView) recalc() {
	w, h := v.Size.X, v.Size.Y
	if w < 0 {
		w = 0
	}
	if h < 0 {
		h = 0
	}
	headerRows := 0
	if !v.overlayDivider && h >= 3 {
		headerRows = 1
	}
	headerY := 0
	if v.overlayDivider {
		headerRows = 1
		headerY = -1
	}
	v.header.SetState(consts.SfVisible, headerRows > 0 && w > 0)
	v.header.ChangeBounds(geom.NewRect(0, headerY, w, headerY+headerRows))
	terminalTop := headerRows
	if v.overlayDivider {
		terminalTop = 0
	}
	v.pane.Term.ChangeBounds(geom.NewRect(0, terminalTop, w, h))
}

type paneHeader struct {
	views.Base
	pane           *session.Pane
	overlayDivider bool
}

func newPaneHeader(bounds geom.Rect, pane *session.Pane, overlayDivider bool) *paneHeader {
	h := &paneHeader{Base: views.NewBase(bounds), pane: pane, overlayDivider: overlayDivider}
	h.SetSelf(h)
	return h
}

func (h *paneHeader) Draw() {
	w := h.Size.X
	if w <= 0 {
		return
	}
	pal := fvtheme.Get()
	lineAttr := pal.SplitterBar
	labelAttr := lineAttr
	active := h.pane != nil && h.pane.Term != nil && h.pane.Term.GetState(consts.SfFocused)
	if active {
		labelAttr = pal.SplitterHandle
	}

	marker := "  "
	if active {
		marker = "▸ "
	}
	label := marker + paneCaption(h.pane) + " "
	label = ui.TruncRight(label, w)
	drawWidth := w
	if h.overlayDivider {
		// Only cover the label-sized left edge of the existing splitter;
		// the rest of its bar and its drag handle remain visible.
		drawWidth = fvutf8.StringDisplayWidth(label)
	}
	row := screen.MakeDrawBuffer(drawWidth)
	for x := 0; x < drawWidth; x++ {
		screen.DrawCell(row, x, "─", lineAttr)
	}
	screen.DrawStr(row, 0, label, labelAttr)
	h.WriteLine(0, 0, drawWidth, 1, row)
}

// paneCaption expresses the two levels of identity without making them look
// interchangeable: SSHAlias is the connection context; DisplayTitle names
// this terminal within that context.
func paneCaption(pane *session.Pane) string {
	if pane == nil {
		return "pane"
	}
	alias := strings.TrimSpace(pane.SSHAlias)
	title := strings.TrimSpace(pane.DisplayTitle())
	switch {
	case alias == "" && title == "":
		return "pane"
	case alias == "":
		return title
	case title == "" || title == alias:
		return alias
	default:
		return alias + " · " + title
	}
}
