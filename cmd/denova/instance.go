package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"denova/internal/portablepath"
	"github.com/gofrs/flock"
)

var errDataDirectoryInUse = errors.New("data directory is already in use by another Denova process")

// Both distributions hold this OS lock until their application has closed.
// A process crash releases the lock; the file's presence is not ownership.
func acquireDataDirectory(root string) (func(), error) {
	if err := portablepath.CheckPath(root, "runtime/instance.lock"); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(root, "runtime"), 0o700); err != nil {
		return nil, err
	}
	lock := flock.New(filepath.Join(root, "runtime", "instance.lock"), flock.SetPermissions(0o600))
	locked, err := lock.TryLock()
	if err != nil {
		return nil, fmt.Errorf("lock data directory: %w", err)
	}
	if !locked {
		return nil, errDataDirectoryInUse
	}
	return func() {
		if err := lock.Unlock(); err != nil {
			slog.Error("data_directory_unlock_failed", "error", err)
		}
	}, nil
}
