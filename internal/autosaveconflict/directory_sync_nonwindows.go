//go:build !windows

package autosaveconflict

import (
	"errors"
	"io"
	"os"
)

func syncDirectory(directory *os.File) error {
	if err := directory.Sync(); err != nil && !errors.Is(err, io.ErrClosedPipe) {
		return err
	}
	return nil
}
