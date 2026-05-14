// Command genmd bakes assets/cheatsheet.md from the live commands
// registry. Run via `go generate ./internal/cheatsheet`.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/oldwired/fvmux/internal/cheatsheet"
	"github.com/oldwired/fvmux/internal/commands"
)

func main() {
	out := filepath.Join("..", "..", "assets", "cheatsheet.md")
	body := cheatsheet.Generate(commands.Defaults())
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "genmd:", err)
		os.Exit(1)
	}
	if err := os.WriteFile(out, []byte(body), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "genmd:", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stdout, "wrote %s (%d bytes)\n", out, len(body))
}
