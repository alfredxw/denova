package revisionfile

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"denova/internal/localfs"
)

// LockedFiles holds the same canonical locks used by ordinary document writes.
// The caller owns durable transaction records and rollback; callbacks must not
// call Read or Mutate on these paths while the batch is locked.
type LockedFiles struct{ snapshots map[string]Snapshot }

func WithFiles(ctx context.Context, paths []string, operation func(*LockedFiles) error) error {
	keys := make([]string, 0, len(paths))
	for _, path := range paths {
		keys = append(keys, canonicalPath(path))
	}
	slices.Sort(keys)
	keys = slices.Compact(keys)
	unlocks := make([]func(), 0, len(keys))
	defer func() {
		for i := len(unlocks) - 1; i >= 0; i-- {
			unlocks[i]()
		}
	}()
	for _, key := range keys {
		unlocks = append(unlocks, mutationLocks.Lock(key))
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	files := &LockedFiles{snapshots: map[string]Snapshot{}}
	for _, key := range keys {
		snapshot, err := readSnapshot(key)
		if err != nil {
			return err
		}
		files.snapshots[key] = snapshot
	}
	return operation(files)
}

func (f *LockedFiles) Snapshot(path string) (Snapshot, error) {
	snapshot, ok := f.snapshots[canonicalPath(path)]
	if !ok {
		return Snapshot{}, fmt.Errorf("path is outside locked batch")
	}
	return snapshot, nil
}

// Replace updates the locked snapshot even on a durability error, allowing the
// transaction owner to inspect/restore a rename that reached the filesystem.
func (f *LockedFiles) Replace(path string, content []byte, exists bool) error {
	key := canonicalPath(path)
	current, err := f.Snapshot(key)
	if err != nil {
		return err
	}
	if exists {
		err = atomicReplace(key, content, mutationMode(current, Options{}), directoryMode(Options{}))
	} else if current.Exists {
		err = os.Remove(key)
		if err == nil {
			err = localfs.SyncDirectory(filepath.Dir(key))
		}
	}
	snapshot, readErr := readSnapshot(key)
	if readErr == nil {
		f.snapshots[key] = snapshot
	}
	if err != nil {
		return err
	}
	return readErr
}
