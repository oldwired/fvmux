package app

import (
	"fmt"
	"strings"
	"testing"

	fvutf8 "github.com/oldwired/fv-go/pkg/fv/utf8"
)

func TestAboutBodyFitsDefaultDialogWidth(t *testing.T) {
	contentWidth := aboutDialogWidth - 4
	for _, line := range strings.Split(fmt.Sprintf(aboutBody, "v0.0.0"), "\n") {
		if width := fvutf8.StringDisplayWidth(line); width > contentWidth {
			t.Errorf("about line is %d cells wide, exceeds content width %d: %q", width, contentWidth, line)
		}
	}
}
