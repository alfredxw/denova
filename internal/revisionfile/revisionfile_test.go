package revisionfile

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestReplaceIfRevisionAllowsOnlyOneConcurrentWriter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.toml")
	if err := os.WriteFile(path, []byte("base"), 0o644); err != nil {
		t.Fatal(err)
	}
	base := Revision([]byte("base"))
	contents := [][]byte{[]byte("first"), []byte("second")}
	errs := make([]error, len(contents))
	start := make(chan struct{})
	var wg sync.WaitGroup
	for index := range contents {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			defer func() {
				if recovered := recover(); recovered != nil {
					errs[index] = fmt.Errorf("revision write panic: %v", recovered)
				}
			}()
			<-start
			_, errs[index] = ReplaceIfRevision(context.Background(), path, base, contents[index], Options{})
		}(index)
	}
	close(start)
	wg.Wait()

	succeeded := 0
	conflicted := 0
	for _, err := range errs {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrRevisionConflict):
			conflicted++
		default:
			t.Fatalf("unexpected write error: %v", err)
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("concurrent CAS results: succeeded=%d conflicted=%d errors=%v", succeeded, conflicted, errs)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "first" && string(data) != "second" {
		t.Fatalf("unexpected persisted content %q", data)
	}
}

func TestReplaceIfRevisionAtomicallyReplacesAndPreservesMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.toml")
	if err := os.WriteFile(path, []byte("before"), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	result, err := ReplaceIfRevision(context.Background(), path, Revision([]byte("before")), []byte("after"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Changed || result.Revision != Revision([]byte("after")) {
		t.Fatalf("unexpected result: %#v", result)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(before, after) {
		t.Fatal("replacement reused the target inode instead of renaming a complete temporary file")
	}
	if after.Mode().Perm() != 0o600 {
		t.Fatalf("file mode changed: got=%o want=600", after.Mode().Perm())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "after" {
		t.Fatalf("persisted content = %q", data)
	}
	temps, err := filepath.Glob(filepath.Join(dir, ".settings.toml.denova-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(temps) != 0 {
		t.Fatalf("temporary files leaked: %v", temps)
	}
}
