//go:build windows

package revisionfile

import "os"

// syncDirectory is intentionally a no-op on Windows. File.Sync maps to
// FlushFileBuffers there, but Windows rejects the read-only directory handle
// returned by os.Open. The replacement file has already been synced before
// Rename, so treating this unsupported directory sync as a write failure would
// incorrectly report a successful settings save as failed.
func syncDirectory(_ *os.File) error {
	return nil
}
