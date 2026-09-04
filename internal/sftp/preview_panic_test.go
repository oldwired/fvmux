package sftp

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/widgets/hexedit"
)

// panicMagic routes a preview file to the panicking decoder below:
// image.Decode dispatches on leading magic bytes, so any file starting
// with this string exercises the decoder-panic recovery without
// depending on a real x/image bug being present.
const panicMagic = "FVMUXPANICIMG"

const oversizedMagic = "FVMUXOVERSIZEDIMG"

var oversizedDecodeCalled atomic.Bool

func init() {
	image.RegisterFormat("fvmux-panic-test", panicMagic,
		func(io.Reader) (image.Image, error) {
			panic("decoder blew up on crafted input")
		},
		func(io.Reader) (image.Config, error) {
			return image.Config{Width: 1, Height: 1}, nil
		},
	)
	image.RegisterFormat("fvmux-oversized-test", oversizedMagic,
		func(io.Reader) (image.Image, error) {
			oversizedDecodeCalled.Store(true)
			return image.NewRGBA(image.Rect(0, 0, 1, 1)), nil
		},
		func(io.Reader) (image.Config, error) {
			return image.Config{Width: maxImageDimension + 1, Height: 1}, nil
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

func TestDecodeImageUsesOneImmutableSnapshot(t *testing.T) {
	var encoded bytes.Buffer
	pixel := image.NewRGBA(image.Rect(0, 0, 1, 1))
	pixel.Set(0, 0, color.White)
	if err := png.Encode(&encoded, pixel); err != nil {
		t.Fatal(err)
	}
	opens := 0
	open := func() (io.ReadCloser, error) {
		opens++
		return io.NopCloser(bytes.NewReader(encoded.Bytes())), nil
	}
	if v := decodeImage(open, geom.NewRect(0, 0, 10, 10)); v == nil {
		t.Fatal("decodeImage returned nil for a one-pixel PNG")
	}
	if opens != 1 {
		t.Fatalf("decodeImage opened the source %d times, want one immutable snapshot", opens)
	}
}

func TestDecodeImageRejectsOversizedHeaderBeforePixelDecode(t *testing.T) {
	oversizedDecodeCalled.Store(false)
	open := func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader([]byte(oversizedMagic))), nil
	}
	if v := decodeImage(open, geom.NewRect(0, 0, 10, 10)); v != nil {
		t.Fatalf("decodeImage = %T, want nil for oversized dimensions", v)
	}
	if oversizedDecodeCalled.Load() {
		t.Fatal("pixel decoder ran after DecodeConfig exceeded the dimension budget")
	}
}
