//go:build !darwin || !cgo

package filewatch

import "github.com/fsnotify/fsnotify"

type fsnotifyWatcher struct {
	watcher *fsnotify.Watcher
}

func newNativeWatcher(_ string) (nativeWatcher, error) {
	watcher, err := fsnotify.NewBufferedWatcher(256)
	if err != nil {
		return nil, err
	}
	return &fsnotifyWatcher{watcher: watcher}, nil
}

func (w *fsnotifyWatcher) Events() <-chan fsnotify.Event { return w.watcher.Events }
func (w *fsnotifyWatcher) Errors() <-chan error          { return w.watcher.Errors }
func (w *fsnotifyWatcher) Add(path string) error         { return w.watcher.Add(path) }
func (w *fsnotifyWatcher) Remove(path string) error      { return w.watcher.Remove(path) }
func (w *fsnotifyWatcher) Close() error                  { return w.watcher.Close() }
