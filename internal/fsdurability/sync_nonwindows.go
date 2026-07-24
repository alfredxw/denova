//go:build !windows

package fsdurability

import (
	"errors"
	"io"
	"os"
)

func syncDirectoryHandle(directory *os.File) error {
	if err := directory.Sync(); err != nil && !errors.Is(err, io.ErrClosedPipe) {
		return err
	}
	return nil
}
