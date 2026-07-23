// Package filelease serializes access to one canonical file identity across
// goroutines and processes. Acquisition waits until ctx is cancelled; it never
// imposes an elapsed-time timeout on its caller.
package filelease

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var processLeases = struct {
	sync.Mutex
	entries map[string]*processLease
}{entries: make(map[string]*processLease)}

type processLease struct {
	token chan struct{}
	refs  int
}

// Acquire combines a context-aware process lease with an advisory OS file
// lock. The returned release function is idempotent. A process crash releases
// the OS lock, while callers remain responsible for their durable recovery
// protocol.
func Acquire(ctx context.Context, lockPath string) (func() error, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	lockPath = canonicalPath(lockPath)
	if strings.TrimSpace(lockPath) == "" {
		return nil, fmt.Errorf("file lease path is required")
	}
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return nil, fmt.Errorf("create file lease directory: %w", err)
	}
	releaseProcess, err := acquireProcess(ctx, lockPath)
	if err != nil {
		return nil, err
	}
	releaseFile, err := acquireOS(ctx, lockPath)
	if err != nil {
		releaseProcess()
		return nil, err
	}
	var once sync.Once
	var releaseErr error
	return func() error {
		once.Do(func() {
			releaseErr = errors.Join(releaseFile(), func() error {
				releaseProcess()
				return nil
			}())
		})
		return releaseErr
	}, nil
}

func acquireProcess(ctx context.Context, path string) (func(), error) {
	processLeases.Lock()
	lease := processLeases.entries[path]
	if lease == nil {
		lease = &processLease{token: make(chan struct{}, 1)}
		lease.token <- struct{}{}
		processLeases.entries[path] = lease
	}
	lease.refs++
	processLeases.Unlock()

	select {
	case <-lease.token:
		var once sync.Once
		return func() {
			once.Do(func() {
				lease.token <- struct{}{}
				releaseProcessReference(path, lease)
			})
		}, nil
	case <-ctx.Done():
		releaseProcessReference(path, lease)
		return nil, ctx.Err()
	}
}

func releaseProcessReference(path string, lease *processLease) {
	processLeases.Lock()
	defer processLeases.Unlock()
	lease.refs--
	if lease.refs == 0 && processLeases.entries[path] == lease {
		delete(processLeases.entries, path)
	}
}

// canonicalPath resolves the longest existing prefix so symlink aliases share
// one process lease even when the lock file itself has not been created yet.
func canonicalPath(path string) string {
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
