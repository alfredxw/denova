//go:build windows

package localfs

import "os"

// File.Sync maps to FlushFileBuffers on Windows, which rejects the read-only
// directory handles returned by os.Open. File-level Sync still protects the
// contents published before a rename or append.
func syncDirectoryHandle(_ *os.File) error {
	return nil
}
