package sftp

import (
	"github.com/oldwired/fv-go/pkg/fv/dialogs"
	"github.com/oldwired/fv-go/pkg/fv/geom"
)

// resizableDialog mirrors internal/app/resizable_dialog.go — duplicated
// here rather than promoted to a shared package to keep import lines
// short and avoid a cycle (internal/app imports internal/sftp).
//
// Without this, dragging the SFTP browser too narrow let the preview
// MarkdownView draw with Size.X < 0, panicking screen.MakeDrawBuffer.
// fv-go's resizeLoop now consults Self().SizeLimits(); the override
// below feeds it the layout's actual minimum.
type resizableDialog struct {
	*dialogs.Dialog
	minW, minH int
}

func newResizableDialog(bounds geom.Rect, title string, minW, minH int) *resizableDialog {
	rd := &resizableDialog{
		Dialog: dialogs.NewDialog(bounds, title),
		minW:   minW,
		minH:   minH,
	}
	rd.SetSelf(rd)
	return rd
}

func (r *resizableDialog) SizeLimits() (geom.Point, geom.Point) {
	return geom.Point{X: r.minW, Y: r.minH},
		geom.Point{X: 1 << 14, Y: 1 << 14}
}
