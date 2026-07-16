package sftp

import (
	"errors"
	"image"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/widgets/hexedit"
)

// panicMagic routes a preview file to the panicking decoder below:
// image.Decode dispatches on leading magic bytes, so any file starting
// with this string exercises the decoder-panic recovery without
// depending on a real x/image bug being present.
const panicMagic = "FVMUXPANICIMG"

func init() {
	image.RegisterFormat("fvmux-panic-test", panicMagic,
		func(io.Reader) (image.Image, error) {
			panic("decoder blew up on crafted input")
		},
		func(io.Reader) (image.Config, error) {
			return image.Config{}, errors.New("no config")
		},
	)
}

// writePanicImage lays down a .png-named file whose content triggers
// the panicking decoder — the shape of a crafted image on a hostile
// server (Sniff classifies by extension; image.Decode by magic).
func writePanicImage(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "crafted.png")
	if err := os.WriteFile(path, []byte(panicMagic+"garbage"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestDecodeImage_PanickingDecoderFallsBackNil pins the recover inside
// decodeImage: a panicking decoder must yield the normal nil return
// (hex fallback), not unwind — previews run on background goroutines,
// where an unrecovered panic kills the whole multiplexer.
func TestDecodeImage_PanickingDecoderFallsBackNil(t *testing.T) {
	path := writePanicImage(t)
	if v := decodeLocalImage(path, geom.NewRect(0, 0, 40, 12)); v != nil {
		t.Fatalf("decodeLocalImage = %T; want nil fallback after decoder panic", v)
	}
}

// TestBuildPreview_PanickingDecoderDegradesToHex drives the full
// preview build over the crafted file: classification says image, the
// decoder panics, and the user must still get a usable pane — the hex
// view — never a crash.
func TestBuildPreview_PanickingDecoderDegradesToHex(t *testing.T) {
	path := writePanicImage(t)
	v := BuildLocalPreview(path, geom.NewRect(0, 0, 40, 12))
	if v == nil {
		t.Fatal("BuildLocalPreview = nil; want a fallback widget")
	}
	if _, ok := v.(*hexedit.HexEditor); !ok {
		t.Fatalf("BuildLocalPreview = %T; want *hexedit.HexEditor fallback", v)
	}
}
