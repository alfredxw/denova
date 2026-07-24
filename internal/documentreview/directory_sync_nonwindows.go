//go:build !windows

package documentreview

import (
	"errors"
	"io"
	"os"
)

// syncDirectory flushes namespace metadata on platforms whose File.Sync
// implementation supports directory handles.
func syncDirectory(file *os.File) error {
	if err := file.Sync(); err != nil && !errors.Is(err, io.ErrClosedPipe) {
		return err
	}
	return nil
}
