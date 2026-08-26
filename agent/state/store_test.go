package state

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestStoreCurrentAlwaysReadsLiveDirectory(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	empty, err := store.Current(ctx)
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.Update(ctx, ChangeSet{
		BaseRevision: empty.Revision,
		Changes:      []Change{{Path: "prompts/general.md", Content: []byte("first")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Changed {
		t.Fatalf("expected an update, got %#v", result)
	}
	path := filepath.Join(store.Root(), "prompts", "general.md")
	if err := os.WriteFile(path, []byte("second"), 0o600); err != nil {
		t.Fatal(err)
	}

	current, err := store.Current(ctx)
	if err != nil {
		t.Fatal(err)
	}
	assertSnapshotFile(t, current, "prompts/general.md", "second")
	if current.Revision == result.Snapshot.Revision {
		t.Fatal("live directory edit reused the previous management revision")
	}
}

func TestStoreAllowsManagementToRepairInvalidLiveFiles(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	invalidPath := filepath.Join(store.Root(), "invalid.md")
	if err := os.WriteFile(invalidPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	current, err := store.Current(ctx)
	if err != nil {
		t.Fatalf("raw management read must expose invalid live files: %v", err)
	}
	result, err := store.Update(ctx, ChangeSet{
		BaseRevision: current.Revision,
		Changes:      []Change{{Path: "invalid.md", Content: []byte("repaired")}},
	})
	if err != nil {
		t.Fatalf("management update could not repair invalid live State: %v", err)
	}
	assertSnapshotFile(t, result.Snapshot, "invalid.md", "repaired")
}

func TestStoreRejectsInvalidCandidateWithoutChangingLiveDirectory(t *testing.T) {
	store := openTestStore(t)
	before, err := store.Current(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Update(context.Background(), ChangeSet{
		BaseRevision: before.Revision,
		Changes:      []Change{{Path: "invalid.md", Content: nil}},
	})
	var validation *ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
	after, err := store.Current(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != before.Revision {
		t.Fatalf("invalid management update changed live State: before=%s after=%s", before.Revision, after.Revision)
	}
}

func TestStoreWriteAllowsInvalidCandidateForConsumerValidation(t *testing.T) {
	store := openTestStore(t)
	before, err := store.Current(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.Write(context.Background(), ChangeSet{
		BaseRevision: before.Revision,
		Changes:      []Change{{Path: "prompts/general.md", Content: nil}},
	})
	if err != nil {
		t.Fatalf("unchecked State write failed: %v", err)
	}
	assertSnapshotFile(t, result.Snapshot, "prompts/general.md", "")
	if err := store.validate(context.Background(), result.Snapshot); err == nil {
		t.Fatal("expected the consumer validator to reject the written snapshot")
	}
}

func TestPreparedTransactionRetainsMarkerUntilRollbackSucceeds(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	base, err := store.Current(ctx)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := applyChanges(base, []Change{{Path: "prompts/general.md", Content: []byte("candidate")}})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.cacheSnapshot(base); err != nil {
		t.Fatal(err)
	}
	if err := store.cacheSnapshot(candidate); err != nil {
		t.Fatal(err)
	}
	marker := store.transactionPath()
	if err := atomicJSON(marker, transactionRecord{
		BaseRevision: base.Revision, CandidateRevision: candidate.Revision, Stage: "prepared",
	}); err != nil {
		t.Fatal(err)
	}
	originalRoot := store.root
	blockedRoot := filepath.Join(filepath.Dir(originalRoot), "not-a-directory")
	if err := os.WriteFile(blockedRoot, []byte("blocked"), 0o600); err != nil {
		t.Fatal(err)
	}
	store.root = blockedRoot
	if err := store.recoverTransaction(); err == nil {
		t.Fatal("expected injected rollback failure")
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("failed rollback removed its recovery marker: %v", err)
	}

	store.root = originalRoot
	if err := store.recoverTransaction(); err != nil {
		t.Fatalf("retained transaction did not recover: %v", err)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("successful recovery retained marker: %v", err)
	}
}

func TestUpdateRejectsRevisionConflict(t *testing.T) {
	store := openTestStore(t)
	_, err := store.Update(context.Background(), ChangeSet{
		BaseRevision: "stale",
		Changes:      []Change{{Path: "prompts/general.md", Content: []byte("value")}},
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected revision conflict, got %v", err)
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	root := t.TempDir()
	store, err := Open(Options{
		Root: filepath.Join(root, "state"), RuntimeRoot: filepath.Join(root, "runtime"),
		Validator: ValidatorFunc(func(_ context.Context, snapshot Snapshot) []Diagnostic {
			for _, file := range snapshot.Files() {
				if len(file.Content) == 0 {
					return []Diagnostic{{Code: "empty_file", Path: file.Path, Message: "State files cannot be empty"}}
				}
			}
			return nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func assertSnapshotFile(t *testing.T, snapshot Snapshot, path, want string) {
	t.Helper()
	content, err := snapshot.Read(path)
	if err != nil || string(content) != want {
		t.Fatalf("snapshot %s = %q, want %q, err=%v", path, content, want, err)
	}
}
