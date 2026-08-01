package filewatch

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

const coalesceWindow = 50 * time.Millisecond

// workspaceWatcher owns one recursive fsnotify registration. Callers receive
// normalized, coalesced batches and never need to understand native event
// ordering, duplicate notifications, or dynamically created directories.
type workspaceWatcher struct {
	root      string
	native    *fsnotify.Watcher
	known     map[string]bool
	events    chan batch
	done      chan struct{}
	closeOnce sync.Once
	wg        sync.WaitGroup
}

func newWorkspaceWatcher(root string) (*workspaceWatcher, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("stat workspace watcher root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("workspace watcher root is not a directory: %s", root)
	}
	native, err := fsnotify.NewBufferedWatcher(256)
	if err != nil {
		return nil, fmt.Errorf("create filesystem watcher: %w", err)
	}
	watcher := &workspaceWatcher{
		root:   filepath.Clean(root),
		native: native,
		known:  make(map[string]bool),
		events: make(chan batch, 8),
		done:   make(chan struct{}),
	}
	if _, err := watcher.watchDirectoryTree(watcher.root, false); err != nil {
		_ = native.Close()
		return nil, err
	}

	watcher.wg.Add(1)
	go watcher.runSafely()
	return watcher, nil
}

func (w *workspaceWatcher) Events() <-chan batch {
	return w.events
}

func (w *workspaceWatcher) Close() error {
	if w == nil {
		return nil
	}
	var closeErr error
	w.closeOnce.Do(func() {
		close(w.done)
		closeErr = w.native.Close()
		w.wg.Wait()
	})
	return closeErr
}

func (w *workspaceWatcher) runSafely() {
	defer w.wg.Done()
	defer close(w.events)
	defer func() {
		if recovered := recover(); recovered != nil {
			slog.ErrorContext(context.Background(), fmt.Sprintf("[filewatch] watcher panic recovered workspace=%q err=%v", w.root, recovered))
		}
	}()
	w.run()
}

func (w *workspaceWatcher) run() {
	pending := newChangeCoalescer()
	pendingResync := false
	var timer *time.Timer
	var timerC <-chan time.Time
	var ready *batch

	schedule := func() {
		if timer != nil {
			return
		}
		timer = time.NewTimer(coalesceWindow)
		timerC = timer.C
	}
	queueChange := func(change Change) {
		pending.add(change)
		schedule()
	}
	queueResync := func(err error) {
		pendingResync = true
		if err != nil {
			if repairErr := w.reconcileDirectoryWatches(); repairErr != nil {
				slog.WarnContext(context.Background(), fmt.Sprintf("[filewatch] filesystem events require authoritative resync and watcher repair was incomplete workspace=%q event_err=%v repair_err=%v", w.root, err, repairErr))
			} else {
				slog.InfoContext(context.Background(), fmt.Sprintf("[filewatch] filesystem events require authoritative resync; recursive watches repaired workspace=%q err=%v", w.root, err))
			}
		}
		schedule()
	}

	for {
		var output chan batch
		var outputValue batch
		if ready != nil {
			output = w.events
			outputValue = *ready
		}

		select {
		case <-w.done:
			if timer != nil {
				timer.Stop()
			}
			return
		case output <- outputValue:
			ready = nil
		case raw, ok := <-w.native.Events:
			if !ok {
				w.deliverFinalResync()
				return
			}
			for _, change := range w.normalizeEvent(raw, queueResync) {
				queueChange(change)
			}
		case err, ok := <-w.native.Errors:
			if !ok {
				w.deliverFinalResync()
				return
			}
			queueResync(err)
		case <-timerC:
			timer = nil
			timerC = nil
			next := batch{changes: pending.take(), resync: pendingResync}
			pendingResync = false
			if len(next.changes) == 0 && !next.resync {
				continue
			}
			if ready == nil {
				ready = &next
			} else {
				merged := newChangeCoalescer()
				for _, change := range ready.changes {
					merged.add(change)
				}
				for _, change := range next.changes {
					merged.add(change)
				}
				ready.changes = merged.take()
				ready.resync = ready.resync || next.resync
			}
		}
	}
}

func (w *workspaceWatcher) deliverFinalResync() {
	select {
	case w.events <- batch{resync: true}:
	case <-w.done:
	}
}

func (w *workspaceWatcher) normalizeEvent(event fsnotify.Event, resync func(error)) []Change {
	rel, ok := w.relativeVisiblePath(event.Name)
	if !ok {
		return nil
	}

	if event.Op&(fsnotify.Remove|fsnotify.Rename) != 0 {
		w.removeDirectoryWatches(rel)
		w.removeKnown(rel)
		return []Change{{Path: rel, Type: ChangeDeleted}}
	}

	if event.Op&fsnotify.Create != 0 {
		wasDirectory, existed := w.known[rel]
		// Lstat deliberately keeps directory symlinks as leaf entries. Following
		// them would extend a workspace watcher beyond its workspace boundary.
		info, err := os.Lstat(event.Name)
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				resync(fmt.Errorf("stat created path %q: %w", rel, err))
			}
			changeType := ChangeAdded
			if existed {
				changeType = ChangeUpdated
			}
			return []Change{{Path: rel, Type: changeType}}
		}
		isDirectory := info.IsDir()
		w.known[rel] = isDirectory
		changeType := ChangeAdded
		if existed {
			changeType = ChangeUpdated
		}
		changes := []Change{{Path: rel, Type: changeType}}
		if isDirectory {
			added, watchErr := w.watchDirectoryTree(event.Name, true)
			changes = append(changes, added...)
			if watchErr != nil {
				resync(watchErr)
			}
		} else if wasDirectory {
			resync(fmt.Errorf("watched directory replaced by file path=%q", rel))
		}
		return changes
	}

	if event.Op&fsnotify.Write != 0 {
		if isDirectory, exists := w.known[rel]; exists && isDirectory {
			return nil
		}
		return []Change{{Path: rel, Type: ChangeUpdated}}
	}

	// Chmod-only notifications are deliberately ignored. They are noisy on
	// macOS and do not change the canonical file content shown by Denova.
	return nil
}

func (w *workspaceWatcher) watchDirectoryTree(root string, emit bool) ([]Change, error) {
	changes := make([]Change, 0)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path != root && strings.HasPrefix(entry.Name(), ".") {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel, ok := w.relativeVisiblePath(path)
		if path == w.root {
			if err := w.native.Add(path); err != nil {
				return fmt.Errorf("watch workspace root %q: %w", path, err)
			}
			return nil
		}
		if !ok {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		_, existed := w.known[rel]
		w.known[rel] = entry.IsDir()
		if entry.IsDir() {
			if err := w.native.Add(path); err != nil {
				return fmt.Errorf("watch workspace directory %q: %w", rel, err)
			}
		}
		if emit && !existed {
			changes = append(changes, Change{Path: rel, Type: ChangeAdded})
		}
		return nil
	})
	if err != nil {
		return changes, fmt.Errorf("register recursive workspace watcher root=%q: %w", root, err)
	}
	return changes, nil
}

func (w *workspaceWatcher) reconcileDirectoryWatches() error {
	previous := w.known
	w.known = make(map[string]bool)
	_, err := w.watchDirectoryTree(w.root, false)
	if err == nil {
		for path, wasDirectory := range previous {
			isDirectory, exists := w.known[path]
			if wasDirectory && (!exists || !isDirectory) {
				w.removeDirectoryWatch(path)
			}
		}
		return nil
	}
	// Preserve knowledge gathered before the failed rescan so a temporary
	// stat/watch error does not turn every existing path into a false create.
	for path, isDirectory := range previous {
		if _, exists := w.known[path]; !exists {
			w.known[path] = isDirectory
		}
	}
	return err
}

func (w *workspaceWatcher) relativeVisiblePath(path string) (string, bool) {
	rel, err := filepath.Rel(w.root, path)
	if err != nil {
		return "", false
	}
	rel = normalizeRelativePath(rel)
	if rel == "" || hasHiddenSegment(rel) {
		return "", false
	}
	return rel, true
}

func (w *workspaceWatcher) removeKnown(path string) {
	delete(w.known, path)
	prefix := path + "/"
	for candidate := range w.known {
		if strings.HasPrefix(candidate, prefix) {
			delete(w.known, candidate)
		}
	}
}

func (w *workspaceWatcher) removeDirectoryWatches(path string) {
	prefix := path + "/"
	for candidate, isDirectory := range w.known {
		if isDirectory && (candidate == path || strings.HasPrefix(candidate, prefix)) {
			w.removeDirectoryWatch(candidate)
		}
	}
}

func (w *workspaceWatcher) removeDirectoryWatch(path string) {
	err := w.native.Remove(filepath.Join(w.root, filepath.FromSlash(path)))
	if err != nil && !errors.Is(err, fsnotify.ErrNonExistentWatch) {
		slog.ErrorContext(context.Background(), fmt.Sprintf("[filewatch] remove recursive directory watch failed workspace=%q path=%q err=%v", w.root, path, err))
	}
}

func hasHiddenSegment(path string) bool {
	for _, segment := range strings.Split(filepath.ToSlash(path), "/") {
		if strings.HasPrefix(segment, ".") {
			return true
		}
	}
	return false
}
