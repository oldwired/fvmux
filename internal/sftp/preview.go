package sftp

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log/slog"
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

const (
	maxImageDimension     = 16384
	maxDecodedImageBytes  = 64 << 20
	maxImageBytesPerPixel = 8
)

// BuildPreview reads enough of path over the SFTP client to classify it,
// then returns the preview widget sized to bounds. See buildPreview for
// the classification and fallback rules.
func BuildPreview(s *pkgsftp.Client, path string, bounds geom.Rect) views.View {
	return buildPreview(path, bounds, func() (io.ReadCloser, error) { return s.Open(path) })
}

// buildPreview classifies the file from a bounded sniff read and returns
// the matching widget:
//
//   - .md / .markdown                  → MarkdownView (rendered).
//   - recognized image                 → ImageView (full decode, ≤ 8 MiB).
//     (png/jpg/gif/webp/bmp/tiff)
//   - binary / image-but-too-big       → HexEditor on first 64 KiB.
//   - everything else                  → MarkdownView fenced as `text`.
//
// open yields a fresh reader over the file each call. Images are read once
// into a bounded byte snapshot, then configuration and pixels are decoded
// from that same snapshot so a remote file cannot change between checks. It is the
// only thing that differs between the remote (s.Open) and local (os.Open)
// entry points. On any open error the result is a MarkdownView "# Error"
// block so the user sees what went wrong instead of a blank pane.
//
// The whole build is panic-isolated: every byte here comes from a file
// the preview exists to open sight-unseen — on a remote server that's
// attacker-controllable input by design — and it flows into parsers
// (image decoders, the markdown renderer) that have had panic bugs
// before (GO-2026-5066 et al. reached exactly this call). A panicking
// parser must degrade to an error pane, never take down the
// multiplexer — especially since previews run on a background
// goroutine, where an unrecovered panic is fatal to the process.
func buildPreview(path string, bounds geom.Rect, open func() (io.ReadCloser, error)) (v views.View) {
	defer func() {
		if r := recover(); r != nil {
			slog.Warn("preview build panicked", "path", path, "panic", r)
			v = errorPreview(bounds, fmt.Sprintf("preview failed: internal panic (%v)", r))
		}
	}()
	f, err := open()
	if err != nil {
		return errorPreview(bounds, err.Error())
	}
	defer func() { _ = f.Close() }()

	buf := make([]byte, maxPreviewBytes)
	n, _ := io.ReadFull(f, buf)
	buf = buf[:n]

	switch Sniff(path, buf) {
	case KindImage:
		if iv := decodeImage(open, bounds); iv != nil {
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

// decodeImage reads one bounded encoded snapshot, validates dimensions through
// DecodeConfig, then decodes pixels from the same bytes. The encoded-byte and decoded-pixel budgets
// are independent: compact images may otherwise declare enough pixels to
// exhaust the process before the decoder can return an error.
// Returns nil on failure — including a panicking decoder — so the caller
// falls back to hex: the hex view renders any bytes safely, which is the
// right degradation for an image a decoder chokes on.
func decodeImage(open func() (io.ReadCloser, error), bounds geom.Rect) (v views.View) {
	defer func() {
		if r := recover(); r != nil {
			slog.Warn("image decode panicked; falling back to hex preview", "panic", r)
			v = nil
		}
	}()
	f, err := open()
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	encoded, readErr := io.ReadAll(io.LimitReader(f, maxImageBytes+1))
	_ = f.Close()
	if readErr != nil || len(encoded) > maxImageBytes {
		return nil
	}
	config, _, configErr := image.DecodeConfig(bytes.NewReader(encoded))
	if configErr != nil || !imageDimensionsAllowed(config.Width, config.Height) {
		return nil
	}
	img, _, err := image.Decode(bytes.NewReader(encoded))
	if err != nil {
		return nil
	}
	decoded := img.Bounds()
	if !imageDimensionsAllowed(decoded.Dx(), decoded.Dy()) {
		return nil
	}
	iv := imageview.New(bounds)
	iv.SetImage(img)
	return iv
}

func imageDimensionsAllowed(width, height int) bool {
	if width <= 0 || height <= 0 || width > maxImageDimension || height > maxImageDimension {
		return false
	}
	maxPixels := maxDecodedImageBytes / maxImageBytesPerPixel
	return int64(width) <= int64(maxPixels)/int64(height)
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

// BuildLocalPreview is the local-FS counterpart to BuildPreview, reading
// from os.Open instead of the SFTP client; same classification rules and
// fallback chain.
func BuildLocalPreview(path string, bounds geom.Rect) views.View {
	return buildPreview(path, bounds, func() (io.ReadCloser, error) { return os.Open(path) })
}

// decodeLocalImage decodes a local-FS image, as decodeImage does for any
// reader. Kept as the local entry point exercised by the image-format
// tests.
func decodeLocalImage(path string, bounds geom.Rect) views.View {
	return decodeImage(func() (io.ReadCloser, error) { return os.Open(path) }, bounds)
}
