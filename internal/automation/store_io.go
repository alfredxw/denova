package automation

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// storePathLocks coordinates all Store instances in this process. Stores are
// deliberately short-lived in the app layer, so a mutex on Store itself would
// not protect shared user-scope files from concurrent read-modify-write cycles.
var storePathLocks = newKeyedStoreLocks()

type keyedStoreLocks struct {
	mu      sync.Mutex
	entries map[string]*keyedStoreLock
}

type keyedStoreLock struct {
	mu   sync.Mutex
	refs int
}

func newKeyedStoreLocks() *keyedStoreLocks {
	return &keyedStoreLocks{entries: make(map[string]*keyedStoreLock)}
}

func (l *keyedStoreLocks) lock(path string) func() {
	key := canonicalStorePath(path)
	l.mu.Lock()
	entry := l.entries[key]
	if entry == nil {
		entry = &keyedStoreLock{}
		l.entries[key] = entry
	}
	entry.refs++
	l.mu.Unlock()

	entry.mu.Lock()
	return func() {
		entry.mu.Unlock()
		l.mu.Lock()
		entry.refs--
		if entry.refs == 0 {
			delete(l.entries, key)
		}
		l.mu.Unlock()
	}
}

// canonicalStorePath resolves the longest existing prefix. This makes a real
// workspace path and a symlink alias share one lock even before the JSON file
// itself has been created.
func canonicalStorePath(path string) string {
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
				for i := len(suffix) - 1; i >= 0; i-- {
					resolved = filepath.Join(resolved, suffix[i])
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

func canonicalStoreRoot(path string) string {
	if path == "" {
		return ""
	}
	return canonicalStorePath(path)
}

func durableWriteJSON(path string, data []byte, perm os.FileMode) (err error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}()
	if err = tmp.Chmod(perm); err != nil {
		return err
	}
	if _, err = tmp.Write(data); err != nil {
		return err
	}
	if err = tmp.Sync(); err != nil {
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	if err = os.Rename(tmpPath, path); err != nil {
		return err
	}
	directory, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open automation store directory for sync: %w", err)
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("sync automation store directory: %w", err)
	}
	return nil
}
