package sftp

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"

	"github.com/oldwired/fv-go/pkg/fv/geom"

	"golang.org/x/image/bmp"
	"golang.org/x/image/tiff"
)

func TestSniff_ExtraImageExtensions(t *testing.T) {
	for _, name := range []string{"x.webp", "x.bmp", "x.tiff", "x.tif", "X.BMP"} {
		if got := Sniff(name, []byte{0x00, 0x01, 0x02, 0x03}); got != KindImage {
			t.Errorf("Sniff(%q) = %d, want KindImage", name, got)
		}
	}
}

// decodeLocalImage should succeed for the newly-registered formats, not
// fall through to the hex fallback (which returns nil here).
func TestDecodeLocalImage_BMPAndTIFF(t *testing.T) {
	dir := t.TempDir()
	img := image.NewRGBA(image.Rect(0, 0, 3, 3))
	img.Set(1, 1, color.RGBA{R: 200, G: 100, B: 50, A: 255})

	cases := []struct {
		name   string
		encode func(*os.File) error
	}{
		{"pic.bmp", func(f *os.File) error { return bmp.Encode(f, img) }},
		{"pic.tiff", func(f *os.File) error { return tiff.Encode(f, img, nil) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := filepath.Join(dir, tc.name)
			f, err := os.Create(p)
			if err != nil {
				t.Fatal(err)
			}
			if err := tc.encode(f); err != nil {
				_ = f.Close()
				t.Fatalf("encode: %v", err)
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}
			if v := decodeLocalImage(p, geom.NewRect(0, 0, 10, 10)); v == nil {
				t.Fatalf("decodeLocalImage(%s) returned nil — decoder not registered?", tc.name)
			}
		})
	}
}
