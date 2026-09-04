package sftp

import (
	"fmt"
	"os"
)

// openSecureLocalRoot opens the exact directory selected by the user and
// rejects a final symlink/junction. os.Root then keeps every descendant open,
// mkdir, remove, and rename beneath that directory even if an attacker swaps
// path components concurrently.
func openSecureLocalRoot(name string) (*os.Root, error) {
	root, err := os.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	opened, err := root.Stat(".")
	if err != nil {
		_ = root.Close()
		return nil, err
	}
	named, err := os.Lstat(name)
	if err != nil {
		_ = root.Close()
		return nil, err
	}
	if named.Mode()&os.ModeSymlink != 0 || !os.SameFile(opened, named) {
		_ = root.Close()
		return nil, fmt.Errorf("%w: %s", ErrUnsafeLocalLink, name)
	}
	return root, nil
}
