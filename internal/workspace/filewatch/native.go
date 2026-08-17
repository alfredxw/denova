package filewatch

import "github.com/fsnotify/fsnotify"

// nativeWatcher is the private platform seam used by workspaceWatcher. Darwin
// uses one recursive FSEvents stream; platforms without a native recursive
// backend keep the existing directory-by-directory fsnotify implementation.
type nativeWatcher interface {
	Events() <-chan fsnotify.Event
	Errors() <-chan error
	Add(path string) error
	Remove(path string) error
	Close() error
}
