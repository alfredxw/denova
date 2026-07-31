package localfs

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

const leaseRetryInterval = 10 * time.Millisecond

// AcquireLease serializes access to one canonical file identity across
// goroutines and processes. Acquisition waits until ctx is cancelled and does
// not impose an elapsed-time timeout. The returned release function is
// idempotent; process termination releases the underlying OS lock.
func AcquireLease(ctx context.Context, lockPath string) (func() error, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	lockPath = canonicalLeasePath(lockPath)
	if lockPath == "" {
		return nil, fmt.Errorf("file lease path is required")
	}
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return nil, fmt.Errorf("create file lease directory: %w", err)
	}

	lease := flock.New(lockPath, flock.SetPermissions(0o600))
	locked, err := lease.TryLockContext(ctx, leaseRetryInterval)
	if err != nil {
		return nil, fmt.Errorf("acquire file lease: %w", err)
	}
	if !locked {
		return nil, fmt.Errorf("acquire file lease: lock was not acquired")
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

// canonicalLeasePath resolves the longest existing prefix so symlink aliases
// continue to share one lease even when the lock file itself does not exist.
func canonicalLeasePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	abs = filepath.Clean(abs)
	prefix := abs
	suffix := []string{}
	for {
		if _, statErr := os.Lstat(prefix); statErr == nil {
			if resolved, resolveErr := filepath.EvalSymlinks(prefix); resolveErr == nil {
				for index := len(suffix) - 1; index >= 0; index-- {
					resolved = filepath.Join(resolved, suffix[index])
				}
				return filepath.Clean(resolved)
			}
			break
		}
		parent := filepath.Dir(prefix)
		if parent == prefix {
			break
		}
		suffix = append(suffix, filepath.Base(prefix))
		prefix = parent
	}
	return abs
}
