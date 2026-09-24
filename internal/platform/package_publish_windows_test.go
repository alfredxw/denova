package platform

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestPublishReleaseSupportsLongWindowsDescendants(t *testing.T) {
	root := t.TempDir()
	// Match the observed 235-character destination; its children exceed MAX_PATH.
	parent := filepath.Join(root, strings.Repeat("p", 235-len(root)-66))
	staging, destination := filepath.Join(parent, ".install-123456789"), filepath.Join(parent, strings.Repeat("d", 64))
	child := filepath.Join("works", "tongdu", "art", "background-with-a-long-file-name.png")
	if err := os.MkdirAll(filepath.Dir(filepath.Join(staging, child)), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staging, child), []byte("reviewed image"), 0600); err != nil {
		t.Fatal(err)
	}
	if len(destination) != 235 || len(filepath.Join(destination, child)) <= 260 {
		t.Fatalf("long-path fixture differs from observed failure: destination=%d child=%d", len(destination), len(filepath.Join(destination, child)))
	}
	if err := publishReleaseDirectory(staging, destination); err != nil {
		t.Fatalf("publication failed for long descendants: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(destination, child)); err != nil || string(data) != "reviewed image" {
		t.Fatalf("long descendant was not readable: %q %v", data, err)
	}
}

// Real directory handles reproduce the native Windows rename failure seen when
// a scanner is still reading a freshly staged bundled release.
func TestPublishReleaseWaitsForTransientWindowsHandle(t *testing.T) {
	for index := range 3 {
		root := t.TempDir()
		staging, destination := filepath.Join(root, "staged"), filepath.Join(root, "released")
		if err := os.Mkdir(staging, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(staging, "source.txt"), []byte("reviewed package"), 0600); err != nil {
			t.Fatal(err)
		}
		name, err := windows.UTF16PtrFromString(staging)
		if err != nil {
			t.Fatal(err)
		}
		handle, err := windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(staging, destination); err == nil {
			_ = windows.CloseHandle(handle)
			t.Fatal("test handle did not block a native directory rename")
		}
		result := make(chan error, 1)
		go func() {
			defer func() {
				if value := recover(); value != nil {
					result <- fmt.Errorf("publish release panicked: %v", value)
				}
			}()
			result <- publishReleaseDirectory(staging, destination)
		}()
		time.Sleep(75 * time.Millisecond)
		if err := windows.CloseHandle(handle); err != nil {
			t.Fatal(err)
		}
		if err := <-result; err != nil {
			t.Fatalf("release %d did not recover from a closed Windows handle: %v", index, err)
		}
		data, err := os.ReadFile(filepath.Join(destination, "source.txt"))
		if err != nil || string(data) != "reviewed package" {
			t.Fatalf("published bytes changed: %q %v", data, err)
		}
		if _, err := os.Stat(staging); !os.IsNotExist(err) {
			t.Fatalf("publication left a second staged copy: %v", err)
		}
	}
}

func TestPublishReleasePreservesExistingWindowsDestination(t *testing.T) {
	root := t.TempDir()
	staging, destination := filepath.Join(root, "staged"), filepath.Join(root, "released")
	for _, directory := range []string{staging, destination} {
		if err := os.Mkdir(directory, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(destination, "existing.txt"), []byte("existing release"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := publishReleaseDirectory(staging, destination); err == nil {
		t.Fatal("publication replaced an existing directory")
	}
	if data, err := os.ReadFile(filepath.Join(destination, "existing.txt")); err != nil || string(data) != "existing release" {
		t.Fatalf("existing release was modified: %q %v", data, err)
	}
	if _, err := os.Stat(staging); err != nil {
		t.Fatalf("failed publication discarded the staged release: %v", err)
	}
}

func TestPublishReleaseReturnsPersistentWindowsLock(t *testing.T) {
	root := t.TempDir()
	staging, destination := filepath.Join(root, "staged"), filepath.Join(root, "released")
	if err := os.Mkdir(staging, 0700); err != nil {
		t.Fatal(err)
	}
	name, err := windows.UTF16PtrFromString(staging)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(handle)
	started := time.Now()
	err = publishReleaseDirectory(staging, destination)
	if !errors.Is(err, windows.ERROR_SHARING_VIOLATION) && !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		t.Fatalf("persistent lock did not preserve the native error: %v", err)
	}
	if elapsed := time.Since(started); elapsed < 900*time.Millisecond || elapsed > 2500*time.Millisecond {
		t.Fatalf("persistent lock exceeded its bounded retry contract: %v", elapsed)
	}
	if _, err := os.Stat(staging); err != nil {
		t.Fatalf("persistent lock discarded staging: %v", err)
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("persistent lock published an unexpected destination: %v", err)
	}
}
