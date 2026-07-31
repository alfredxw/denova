package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gofrs/flock"
)

const fileJournalLeaseRetryInterval = 10 * time.Millisecond

// acquireFileJournalLease keeps local-file coordination private to the file
// journal implementation instead of expanding the public Agent interface.
func acquireFileJournalLease(ctx context.Context, lockPath string) (func() error, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	lockPath = strings.TrimSpace(lockPath)
	if lockPath == "" {
		return nil, fmt.Errorf("file journal lease path is required")
	}
	lockPath = filepath.Clean(lockPath)
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return nil, fmt.Errorf("create file journal lease directory: %w", err)
	}

	lease := flock.New(lockPath, flock.SetPermissions(0o600))
	locked, err := lease.TryLockContext(ctx, fileJournalLeaseRetryInterval)
	if err != nil {
		return nil, fmt.Errorf("acquire file journal lease: %w", err)
	}
	if !locked {
		return nil, fmt.Errorf("acquire file journal lease: lock was not acquired")
	}

	var once sync.Once
	var releaseErr error
	return func() error {
		once.Do(func() {
			releaseErr = lease.Unlock()
		})
		return releaseErr
	}, nil
}
