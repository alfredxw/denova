package state

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestStoreCurrentAndUpdate(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	empty, err := store.Current(ctx)
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.Update(ctx, ChangeSet{
		BaseRevision: empty.Revision,
		Changes:      []Change{{Path: "prompts/general.md", Content: []byte("Be concise.")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Changed {
		t.Fatalf("expected an update, got %#v", result)
	}
	content, err := result.Snapshot.Read("prompts/general.md")
	if err != nil || string(content) != "Be concise." {
		t.Fatalf("unexpected current prompt %q err=%v", content, err)
	}

}

func TestStorePinsStatePerRunWhileNewRunUsesCurrent(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	current, err := store.Current(ctx)
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.Update(ctx, ChangeSet{
		BaseRevision: current.Revision,
		Changes:      []Change{{Path: "prompts/general.md", Content: []byte("first")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	runOne, err := store.ForRun(ctx, "run-one")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Update(ctx, ChangeSet{
		BaseRevision: first.Snapshot.Revision,
		Changes:      []Change{{Path: "prompts/general.md", Content: []byte("second")}},
	}); err != nil {
		t.Fatal(err)
	}

	recovered, err := store.ForRun(ctx, "run-one")
	if err != nil {
		t.Fatal(err)
	}
	newRun, err := store.ForRun(ctx, "run-two")
	if err != nil {
		t.Fatal(err)
	}
	assertSnapshotFile(t, runOne, "prompts/general.md", "first")
	assertSnapshotFile(t, recovered, "prompts/general.md", "first")
	assertSnapshotFile(t, newRun, "prompts/general.md", "second")
	if recovered.Token == "" || recovered.Token != runOne.Token || newRun.Token == runOne.Token {
		t.Fatalf("unexpected Run tokens first=%q recovered=%q new=%q", runOne.Token, recovered.Token, newRun.Token)
	}
}

func TestDraftValidatesCompleteSnapshotBeforePublish(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	current, err := store.Current(ctx)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := store.BeginDraft(ctx, current.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(draft.Root(), "prompts"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(draft.Root(), "prompts", "general.md"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := draft.Publish(ctx); err == nil {
		t.Fatal("expected complete snapshot validation to reject the draft")
	} else {
		var validation *ValidationError
		if !errors.As(err, &validation) {
			t.Fatalf("expected ValidationError, got %v", err)
		}
	}
	after, err := store.Current(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != current.Revision {
		t.Fatalf("invalid draft changed current State: before=%s after=%s", current.Revision, after.Revision)
	}
}

func TestResumeDraftAllowsRepairingTemporarilyInvalidFiles(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	current, err := store.Current(ctx)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := store.BeginDraft(ctx, current.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(draft.Root(), "invalid.md"), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	resumed, err := store.ResumeDraft(ctx, draft.ID(), draft.BaseRevision())
	if err != nil {
		t.Fatalf("temporarily invalid draft must remain recoverable: %v", err)
	}
	if err := resumed.Validate(ctx); err == nil {
		t.Fatal("invalid resumed draft unexpectedly passed validation")
	}
	if err := os.WriteFile(filepath.Join(resumed.Root(), "invalid.md"), []byte("repaired"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := resumed.Publish(ctx)
	if err != nil || !result.Changed {
		t.Fatalf("repaired resumed draft was not published: result=%#v err=%v", result, err)
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

func TestStoreRejectsCorruptExistingSnapshotCache(t *testing.T) {
	store := openTestStore(t)
	snapshot, err := store.Current(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	cacheRoot := filepath.Join(store.private, "snapshots", snapshot.Revision)
	if err := os.WriteFile(filepath.Join(cacheRoot, "corrupt"), []byte("unexpected"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Current(context.Background()); err == nil {
		t.Fatal("corrupt immutable snapshot cache was silently trusted")
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

func TestStoreBindsAcceptedCommandSnapshotToPublicRun(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	first, err := store.Update(ctx, ChangeSet{Changes: []Change{{Path: "prompts/general.md", Content: []byte("first")}}})
	if err != nil {
		t.Fatal(err)
	}
	command, err := store.ForRun(ctx, "command-one")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Update(ctx, ChangeSet{
		BaseRevision: first.Snapshot.Revision,
		Changes:      []Change{{Path: "prompts/general.md", Content: []byte("second")}},
	}); err != nil {
		t.Fatal(err)
	}
	found, err := store.BindRun(ctx, "public-run", "command-one")
	if err != nil || !found {
		t.Fatalf("BindRun() = (%v, %v), want (true, nil)", found, err)
	}
	bound, err := store.ForRun(ctx, "public-run")
	if err != nil {
		t.Fatal(err)
	}
	if bound.Revision != command.Revision {
		t.Fatalf("bound revision = %q, want %q", bound.Revision, command.Revision)
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
