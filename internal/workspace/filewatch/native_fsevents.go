//go:build darwin && cgo

package filewatch

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fsnotify/fsevents"
	"github.com/fsnotify/fsnotify"
)

const fseventsBufferSize = 4096

type fseventsWatcher struct {
	root      string
	realRoot  string
	stream    *fsevents.EventStream
	events    chan fsnotify.Event
	errors    chan error
	done      chan struct{}
	closeOnce sync.Once
	wg        sync.WaitGroup
}

func newNativeWatcher(root string) (nativeWatcher, error) {
	root = filepath.Clean(root)
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace path for FSEvents: %w", err)
	}
	stream := &fsevents.EventStream{
		Events:  make(chan []fsevents.Event, fseventsBufferSize),
		Paths:   []string{realRoot},
		Flags:   fsevents.FileEvents | fsevents.NoDefer | fsevents.WatchRoot,
		Latency: 0,
	}
	if err := stream.Start(); err != nil {
		return nil, fmt.Errorf("watch workspace with FSEvents: %w", err)
	}
	watcher := &fseventsWatcher{
		root: root, realRoot: realRoot,
		stream: stream, events: make(chan fsnotify.Event, fseventsBufferSize),
		errors: make(chan error, 1), done: make(chan struct{}),
	}
	watcher.wg.Add(1)
	go watcher.runSafely()
	return watcher, nil
}

func (w *fseventsWatcher) Events() <-chan fsnotify.Event { return w.events }
func (w *fseventsWatcher) Errors() <-chan error          { return w.errors }

// FSEvents watches the complete root recursively. workspaceWatcher still calls
// Add and Remove while maintaining its portable directory projection, so these
// operations intentionally require no platform work on Darwin.
func (w *fseventsWatcher) Add(string) error    { return nil }
func (w *fseventsWatcher) Remove(string) error { return nil }

func (w *fseventsWatcher) Close() error {
	w.closeOnce.Do(func() {
		w.stream.Stop()
		close(w.done)
		w.wg.Wait()
	})
	return nil
}

func (w *fseventsWatcher) runSafely() {
	defer w.wg.Done()
	defer close(w.events)
	defer close(w.errors)
	defer func() {
		if recovered := recover(); recovered != nil {
			select {
			case w.errors <- fmt.Errorf("FSEvents watcher panic recovered: %v", recovered):
			default:
			}
		}
	}()
	for {
		select {
		case <-w.done:
			return
		case batch, ok := <-w.stream.Events:
			if !ok {
				return
			}
			for _, event := range batch {
				if err := fseventsResyncError(event); err != nil {
					select {
					case w.errors <- err:
					default:
					}
					continue
				}
				normalized := fsnotify.Event{Name: w.logicalPath(event.Path), Op: fsnotifyOperation(event)}
				if normalized.Op == 0 {
					continue
				}
				select {
				case w.events <- normalized:
				case <-w.done:
					return
				}
			}
		}
	}
}

func (w *fseventsWatcher) logicalPath(path string) string {
	path = filepath.Clean(path)
	rel, err := filepath.Rel(w.realRoot, path)
	if err != nil || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return path
	}
	if rel == "." {
		return w.root
	}
	return filepath.Join(w.root, rel)
}

func fsnotifyOperation(event fsevents.Event) fsnotify.Op {
	var operation fsnotify.Op
	if event.Flags&fsevents.ItemCreated != 0 {
		operation |= fsnotify.Create
	}
	if event.Flags&fsevents.ItemModified != 0 {
		operation |= fsnotify.Write
	}
	if event.Flags&fsevents.ItemRemoved != 0 {
		operation |= fsnotify.Remove
	}
	if event.Flags&fsevents.ItemRenamed != 0 {
		if _, err := os.Lstat(event.Path); err == nil {
			operation |= fsnotify.Create
		} else {
			operation |= fsnotify.Rename
		}
	}
	return operation
}

func fseventsResyncError(event fsevents.Event) error {
	const resyncEvents = fsevents.MustScanSubDirs |
		fsevents.UserDropped |
		fsevents.KernelDropped |
		fsevents.EventIDsWrapped |
		fsevents.RootChanged
	if event.Flags&resyncEvents == 0 {
		return nil
	}
	return errors.New("FSEvents history is incomplete; workspace resync required")
}
