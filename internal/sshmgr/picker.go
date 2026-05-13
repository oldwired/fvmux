package sshmgr

import (
	fvapp "github.com/oldwired/fv-go/pkg/fv/app"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/widgets/fuzzyfinder"
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
	w, h := 70, 14
	if w > desk.Size.X-4 {
		w = desk.Size.X - 4
	}
	if h > desk.Size.Y-4 {
		h = desk.Size.Y - 4
	}
	x := (desk.Size.X - w) / 2
	y := (desk.Size.Y - h) / 2
	ff := fuzzyfinder.New(geom.NewRect(x, y, x+w, y+h), items)
	idx := ff.Run(&a.Desktop.Group)
	if idx < 0 || idx >= len(hosts) {
		return nil
	}
	return hosts[idx]
}
