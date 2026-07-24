// Package fsdurability centralizes cross-platform directory durability.
// Callers must sync file contents before publishing a namespace mutation;
// this package only flushes the containing directory where the OS supports it.
package fsdurability

import (
	"errors"
	"os"
	"path/filepath"
)

// SyncDirectory flushes namespace changes for a directory path. Windows keeps
// file-level durability but skips the unsupported directory FlushFileBuffers.
func SyncDirectory(path string) error {
	if path == "" {
		path = "."
	}
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	return syncAndCloseDirectory(directory)
}

// SyncRootDirectory flushes namespace changes for a directory opened beneath
// root without weakening os.Root's path-containment guarantees.
func SyncRootDirectory(root *os.Root, rel string) error {
	if rel == "" {
		rel = "."
	}
	directory, err := root.Open(filepath.FromSlash(rel))
	if err != nil {
		return err
	}
	return syncAndCloseDirectory(directory)
}

func syncAndCloseDirectory(directory *os.File) error {
	syncErr := syncDirectoryHandle(directory)
	closeErr := directory.Close()
	return errors.Join(syncErr, closeErr)
}
