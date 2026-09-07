package versions

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"denova/internal/localfs"
)

func TestVersionsIgnoreActiveJournalLeases(t *testing.T) {
	dir := t.TempDir()
	service := newVersionTestService(t, dir)
	settings := DefaultAutoSettings()
	story := "interactive/story/story-main.jsonl"
	visible := []string{"Cargo.lock", story, "notes.jsonl.agent-not-a-lease.lock"}
	for _, path := range visible {
		writeFile(t, dir, path, "content")
	}
	leases := []string{
		story + ".agent-c9c1a17cf6c77daa95e0933265745903.lock",
		story + ".domain.lock",
		story + ".mutation.lock",
	}
	for _, path := range leases {
		writeFile(t, dir, path, "runtime lease")
		release, err := localfs.AcquireLease(t.Context(), filepath.Join(dir, filepath.FromSlash(path)))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := release(); err != nil {
				t.Errorf("release journal lease: %v", err)
			}
		})
	}
	status, err := service.Status(settings)
	if err != nil {
		t.Fatalf("status with active journal leases: %v", err)
	}
	paths := make([]string, 0, len(status.Changes))
	for _, change := range status.Changes {
		paths = append(paths, change.Path)
	}
	if !reflect.DeepEqual(paths, visible) {
		t.Fatalf("versioned paths = %v, want %v", paths, visible)
	}
	created, err := service.Create("Active story", VersionSourceManual, settings)
	if err != nil {
		t.Fatalf("create with active journal leases: %v", err)
	}
	files, err := service.commitFiles(created.Version.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := sortedVersionFilePaths(files); !reflect.DeepEqual(got, visible) {
		t.Fatalf("committed paths = %v, want %v", got, visible)
	}
	status, err = service.Status(settings)
	if err != nil || !status.Clean {
		t.Fatalf("unchanged journal leases must not dirty status: status=%#v err=%v", status, err)
	}
	for _, path := range leases {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(path))); err != nil {
			t.Fatalf("journal lease must remain in place: %v", err)
		}
	}
}
