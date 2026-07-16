// Flat-listing builders for the dual-pane browser. Each listing pane
// shows its side's current folder contents (../ row + folders + files).
// Every Node is a leaf — listings don't expand; navigation happens via
// keyHandler intercepting Enter and updating the panel's cwd.
package sftp

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/oldwired/fv-go/pkg/fv/widgets/treeview"

	pkgsftp "github.com/pkg/sftp"

	"github.com/oldwired/fvmux/internal/ui"
)

// listingNameW / listingSizeW / listingMtimeW control the per-column
// width inside a listing row. Sum + spaces must fit the listing
// column's effective inner width (TreeView eats 2 chars of indent +
// marker, so usable width = column-2).
const (
	listingNameW  = 16
	listingSizeW  = 6
	listingMtimeW = 5
)

// buildRemoteListing returns a flat list for the SFTP path: a
// synthetic "../" row when cwd has a parent, then folders (sorted),
// then files (sorted).
func buildRemoteListing(s *pkgsftp.Client, cwd string) []*treeview.Node {
	entries, err := s.ReadDir(cwd)
	if err != nil {
		return []*treeview.Node{
			{Label: "<error: " + err.Error() + ">"},
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		di, dj := entries[i].IsDir(), entries[j].IsDir()
		if di != dj {
			return di
		}
		return entries[i].Name() < entries[j].Name()
	})

	out := make([]*treeview.Node, 0, len(entries)+1)
	if parent := remoteParent(cwd); parent != cwd {
		out = append(out, &treeview.Node{
			Label: formatListingRow("../", true, 0, time.Time{}),
			Data:  &fileEntry{Path: parent, IsDir: true, Parent: true},
		})
	}
	for _, e := range entries {
		out = append(out, &treeview.Node{
			Label: formatListingRow(e.Name(), e.IsDir(), e.Size(), e.ModTime()),
			Data:  &fileEntry{Path: joinRemote(cwd, e.Name()), IsDir: e.IsDir()},
		})
	}
	return out
}

// buildLocalListing is the local-FS mirror of buildRemoteListing.
func buildLocalListing(cwd string) []*treeview.Node {
	entries, err := os.ReadDir(cwd)
	if err != nil {
		return []*treeview.Node{
			{Label: "<error: " + err.Error() + ">"},
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		di, dj := entries[i].IsDir(), entries[j].IsDir()
		if di != dj {
			return di
		}
		return entries[i].Name() < entries[j].Name()
	})

	out := make([]*treeview.Node, 0, len(entries)+1)
	if parent := filepath.Dir(cwd); parent != cwd {
		out = append(out, &treeview.Node{
			Label: formatListingRow("../", true, 0, time.Time{}),
			Data:  &fileEntry{Path: parent, IsDir: true, Local: true, Parent: true},
		})
	}
	for _, e := range entries {
		path := filepath.Join(cwd, e.Name())
		var size int64
		var mtime time.Time
		if info, err := e.Info(); err == nil {
			size = info.Size()
			mtime = info.ModTime()
		}
		out = append(out, &treeview.Node{
			Label: formatListingRow(e.Name(), e.IsDir(), size, mtime),
			Data:  &fileEntry{Path: path, IsDir: e.IsDir(), Local: true},
		})
	}
	return out
}

// formatListingRow returns one fixed-width row: "name(N) size(S) mtime(M)".
// "../" gets <up>/<up>; folders show <dir>/—; files show humanized size
// + mtime. All columns padded so the listing renders as a tidy table.
func formatListingRow(name string, isDir bool, size int64, mtime time.Time) string {
	displayName := name
	if isDir && name != "../" {
		displayName = name + "/"
	}
	displayName = ui.TruncRight(displayName, listingNameW)

	var sizeStr, mtimeStr string
	switch {
	case name == "../":
		sizeStr = "<up>"
		mtimeStr = ""
	case isDir:
		sizeStr = "<dir>"
		mtimeStr = formatMtime(mtime)
	default:
		sizeStr = humanSize(size)
		mtimeStr = formatMtime(mtime)
	}
	return fmt.Sprintf("%-*s %*s %-*s",
		listingNameW, displayName,
		listingSizeW, sizeStr,
		listingMtimeW, mtimeStr)
}

// humanSize compresses a byte count to ≤ listingSizeW chars: "123",
// "12K", "5.2M", "1.4G". Loses precision intentionally — the listing
// is for orientation, not bookkeeping.
func humanSize(b int64) string {
	const (
		kib = 1024
		mib = 1024 * 1024
		gib = 1024 * 1024 * 1024
	)
	switch {
	case b < kib:
		return fmt.Sprintf("%d", b)
	case b < mib:
		return fmt.Sprintf("%dK", b/kib)
	case b < gib:
		return fmt.Sprintf("%.1fM", float64(b)/float64(mib))
	default:
		return fmt.Sprintf("%.1fG", float64(b)/float64(gib))
	}
}

// formatMtime returns "MM-DD" for recent dates and " YYYY" for older
// ones. Keeps every output at exactly listingMtimeW characters.
func formatMtime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	thisYear := time.Now().Year()
	if t.Year() == thisYear {
		return t.Format("01-02")
	}
	return fmt.Sprintf(" %04d", t.Year())
}

// remoteParent returns the SFTP-side parent of cwd. "/" (and the empty
// path) are their own parent so the caller knows there's no "../" row to
// emit. Trailing slashes are stripped first, so "/a/b/" and "/a/b" share
// the parent "/a" — bare path.Dir would return "/a/b" for the former.
func remoteParent(cwd string) string {
	if cwd == "" || cwd == "/" {
		return "/"
	}
	parent := path.Dir(strings.TrimRight(cwd, "/"))
	if parent == "." {
		return "/"
	}
	return parent
}
