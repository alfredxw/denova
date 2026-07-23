//go:build !windows

package revisionfile

import (
	"errors"
	"io"
	"os"
)

// syncDirectory persists the rename's directory entry where directory handles
// support Sync. A closed-pipe result is the documented no-op case on filesystems
// that do not expose directory durability through this handle.
func syncDirectory(directory *os.File) error {
	if err := directory.Sync(); err != nil && !errors.Is(err, io.ErrClosedPipe) {
		return err
	}
	return nil
}
