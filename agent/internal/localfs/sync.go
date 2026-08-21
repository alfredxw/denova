// Package localfs centralizes cross-platform filesystem durability primitives
// used by the public Agent module.
package localfs

import (
	"errors"
	"os"
)

// SyncDirectory flushes namespace changes for a directory path. Regular files
// must still be synced before a rename or append is published.
func SyncDirectory(path string) error {
	if path == "" {
		path = "."
	}
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	return errors.Join(syncDirectoryHandle(directory), directory.Close())
}
