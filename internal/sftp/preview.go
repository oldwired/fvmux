package sftp

import (
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"strings"

	// Extra image decoders registered with image.Decode so the preview
	// pane handles them instead of falling back to hex. All three are
	// decode-only blank imports; webp in particular has no encoder.
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"

	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/views"
	"github.com/oldwired/fv-go/pkg/fv/widgets/hexedit"
	"github.com/oldwired/fv-go/pkg/fv/widgets/imageview"
	"github.com/oldwired/fv-go/pkg/fv/widgets/markdown"

	pkgsftp "github.com/pkg/sftp"
)

// maxImageBytes caps full image decode size — bigger files fall back
// to hex view so a stray attempt to preview a 200 MiB tarball with an
// image extension doesn't lock up the dialog.
const maxImageBytes = 8 * 1024 * 1024

// BuildPreview reads enough of path to classify it, then returns the
// right preview widget sized to bounds:
//
//   - .md / .markdown                  → MarkdownView (rendered).
//   - recognized image                 → ImageView (full decode, ≤ 8 MiB).
//     (png/jpg/gif/webp/bmp/tiff)
//   - binary / image-but-too-big       → HexEditor on first 64 KiB.
//   - everything else                  → MarkdownView fenced as `text`.
//
// On any error the result is a MarkdownView containing a "# Error"
// block so the user sees what went wrong instead of a blank pane.
func BuildPreview(s *pkgsftp.Client, path string, bounds geom.Rect) views.View {
	f, err := s.Open(path)
	if err != nil {
		return errorPreview(bounds, err.Error())
	}
	defer func() { _ = f.Close() }()

	buf := make([]byte, maxPreviewBytes)
	n, _ := io.ReadFull(f, buf)
	buf = buf[:n]

	switch Sniff(path, buf) {
	case KindImage:
		if iv := decodeImage(s, path, bounds); iv != nil {
			return iv
		}
		return hexPreview(buf, bounds)
	case KindBinary:
		return hexPreview(buf, bounds)
	case KindMarkdown:
		mv := markdown.New(bounds, nil)
		mv.SetMarkdown(string(buf))
		return mv
	default: // KindText
		mv := markdown.New(bounds, nil)
		mv.SetMarkdown("```\n" + strings.TrimRight(string(buf), "\n") + "\n```")
		return mv
	}
}

// decodeImage attempts a full image decode from the remote file. Caps
// the read at maxImageBytes so a misclassified large file doesn't
// hang. Returns nil on failure so the caller can fall back to hex.
func decodeImage(s *pkgsftp.Client, path string, bounds geom.Rect) views.View {
	f, err := s.Open(path)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	img, _, err := image.Decode(io.LimitReader(f, maxImageBytes))
	if err != nil {
		return nil
	}
	iv := imageview.New(bounds)
	iv.SetImage(img)
	return iv
}

func hexPreview(buf []byte, bounds geom.Rect) views.View {
	src := hexedit.NewMemorySource(buf)
	src.SetReadOnly(true)
	return hexedit.New(bounds, src)
}

func errorPreview(bounds geom.Rect, msg string) views.View {
	mv := markdown.New(bounds, nil)
	mv.SetMarkdown(fmt.Sprintf("# Error\n\n%s", msg))
	return mv
}

// loadingPreview is the transient placeholder shown while a remote file
// is read on a background goroutine (see previewPane.show). It gives the
// user immediate feedback that Enter registered instead of a frozen,
// stale preview pane during the network round-trip.
func loadingPreview(bounds geom.Rect, path string) views.View {
	mv := markdown.New(bounds, nil)
	mv.SetMarkdown("# Loading…\n\n```\n" + path + "\n```")
	return mv
}

// BuildLocalPreview is the local-FS counterpart to BuildPreview. Reads
// from os.Open instead of the SFTP client; same classification rules
// and fallback chain.
func BuildLocalPreview(path string, bounds geom.Rect) views.View {
	f, err := os.Open(path)
	if err != nil {
		return errorPreview(bounds, err.Error())
	}
	defer func() { _ = f.Close() }()

	buf := make([]byte, maxPreviewBytes)
	n, _ := io.ReadFull(f, buf)
	buf = buf[:n]

	switch Sniff(path, buf) {
	case KindImage:
		if iv := decodeLocalImage(path, bounds); iv != nil {
			return iv
		}
		return hexPreview(buf, bounds)
	case KindBinary:
		return hexPreview(buf, bounds)
	case KindMarkdown:
		mv := markdown.New(bounds, nil)
		mv.SetMarkdown(string(buf))
		return mv
	default: // KindText
		mv := markdown.New(bounds, nil)
		mv.SetMarkdown("```\n" + strings.TrimRight(string(buf), "\n") + "\n```")
		return mv
	}
}

func decodeLocalImage(path string, bounds geom.Rect) views.View {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	img, _, err := image.Decode(io.LimitReader(f, maxImageBytes))
	if err != nil {
		return nil
	}
	iv := imageview.New(bounds)
	iv.SetImage(img)
	return iv
}
