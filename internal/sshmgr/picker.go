package sshmgr

import (
	fvapp "github.com/oldwired/fv-go/pkg/fv/app"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/widgets/fuzzyfinder"

	"github.com/oldwired/fvmux/internal/ui"
)

// PickHost opens a fuzzy picker over hosts. Returns the chosen Host or
// nil if the user cancels.
func PickHost(a *fvapp.Application, hosts []*Host) *Host {
	if len(hosts) == 0 {
		return nil
	}
	items := make([]string, len(hosts))
	for i, h := range hosts {
		items[i] = h.DisplayRow()
	}
	desk := a.Desktop.BaseView()
	r := ui.CenterRect(desk.Size, 70, 14, 4)
	x, y, w, h := r.A.X, r.A.Y, r.Width(), r.Height()
	ff := fuzzyfinder.New(geom.NewRect(x, y, x+w, y+h), items)
	idx := ff.Run(&a.Desktop.Group)
	if idx < 0 || idx >= len(hosts) {
		return nil
	}
	return hosts[idx]
}
