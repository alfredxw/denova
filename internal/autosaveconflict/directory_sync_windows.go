//go:build windows

package autosaveconflict

import "os"

// Directory FlushFileBuffers is unsupported for the handles returned by
// os.Root.Open on Windows. The record itself is still fsynced before rename.
func syncDirectory(_ *os.File) error {
	return nil
}
