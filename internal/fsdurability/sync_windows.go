//go:build windows

package fsdurability

import "os"

// File.Sync maps to FlushFileBuffers on Windows, which rejects the read-only
// directory handles returned by os.Open and os.Root.Open. Regular files remain
// responsible for their own Sync before any rename or append is published.
func syncDirectoryHandle(_ *os.File) error {
	return nil
}
