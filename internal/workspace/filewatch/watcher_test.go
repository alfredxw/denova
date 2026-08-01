package filewatch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fsnotify/fsnotify"
)

func TestWorkspaceWatcherTreatsDirectorySymlinkAsLeaf(t *testing.T) {
	root := t.TempDir()
	external := t.TempDir()
	link := filepath.Join(root, "linked")
	if err := os.Symlink(external, link); err != nil {
		t.Skipf("directory symlinks unavailable: %v", err)
	}
	native, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = native.Close() })
	watcher := &workspaceWatcher{
		root:   root,
		native: native,
		known:  make(map[string]bool),
	}

	changes := watcher.normalizeEvent(fsnotify.Event{Name: link, Op: fsnotify.Create}, func(err error) {
		t.Fatalf("symlink create requested resync: %v", err)
	})
	if len(changes) != 1 || changes[0] != (Change{Path: "linked", Type: ChangeAdded}) {
		t.Fatalf("symlink changes = %#v", changes)
	}
	for _, watched := range native.WatchList() {
		if filepath.Clean(watched) == filepath.Clean(link) {
			t.Fatalf("directory symlink escaped workspace watch boundary: %q", watched)
		}
	}
}

func TestWorkspaceWatcherRemovesMovedDirectoryWatches(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "chapters", "volume")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	native, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = native.Close() })
	watcher := &workspaceWatcher{
		root:   root,
		native: native,
		known:  make(map[string]bool),
	}
	if _, err := watcher.watchDirectoryTree(root, false); err != nil {
		t.Fatal(err)
	}

	watcher.normalizeEvent(fsnotify.Event{
		Name: filepath.Join(root, "chapters"),
		Op:   fsnotify.Rename,
	}, func(err error) {
		t.Fatalf("directory rename requested resync: %v", err)
	})
	removedPrefix := filepath.Clean(filepath.Join(root, "chapters"))
	for _, watched := range native.WatchList() {
		watched = filepath.Clean(watched)
		if watched == removedPrefix || strings.HasPrefix(watched, removedPrefix+string(filepath.Separator)) {
			t.Fatalf("moved directory watch remains registered: %q", watched)
		}
	}
}
