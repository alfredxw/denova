//go:build windows

package book

import "os"

// syncDirectory is best-effort on Windows because File.Sync delegates to
// FlushFileBuffers, which does not support the read-only directory handles
// returned by os.Open. Regular file writes retain their own File.Sync.
func syncDirectory(_ *os.File) error {
	return nil
}
