package workspacechange

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var errInjectedParentSync = errors.New("injected parent directory sync failure")

func TestApplyParentSyncFailureKeepsPreparedAndBlocksWrites(t *testing.T) {
	service, path := newTestServiceWithFile(t, "before")
	otherPath := "other.md"
	writeTestFile(t, service.workspace, otherPath, "other before")
	originalSync := service.durability.syncRootDirFn
	failSync := true
	chapterSyncs := 0
	service.durability.syncRootDirFn = func(root *os.Root, rel string) error {
		if rel == "chapters" {
			chapterSyncs++
			if failSync && chapterSyncs >= 2 {
				return errInjectedParentSync
			}
		}
		return originalSync(root, rel)
	}

	_, err := service.ApplyEdits(context.Background(), ApplyEditsRequest{
		Path:         path,
		BaseRevision: Revision([]byte("before")),
		Edits:        []TextEdit{{OldString: "before", NewString: "after"}},
		Metadata:     ChangeMetadata{Origin: OriginAgent, ChangeGroupID: "durability-group"},
	})
	assertDurabilityPending(t, err, true)
	if got := readTestFile(t, service.workspace, path); got != "after" {
		t.Fatalf("visible mutation was lost: %q", got)
	}
	if len(service.prepared) != 1 || len(service.changeSets) != 0 {
		t.Fatalf("uncertain mutation was terminally projected: prepared=%d changes=%d", len(service.prepared), len(service.changeSets))
	}

	_, err = service.SaveFile(context.Background(), otherPath, "other after", Revision([]byte("other before")))
	assertDurabilityPending(t, err, true)
	if got := readTestFile(t, service.workspace, otherPath); got != "other before" {
		t.Fatalf("write barrier allowed a later mutation: %q", got)
	}

	failSync = false
	result, err := service.SaveFile(context.Background(), otherPath, "other after", Revision([]byte("other before")))
	if err != nil || !result.Changed {
		t.Fatalf("write did not proceed after durability recovery: result=%#v err=%v", result, err)
	}
	if len(service.prepared) != 0 || len(service.pendingParentSync) != 0 {
		t.Fatalf("durability recovery left pending state: prepared=%d parents=%d", len(service.prepared), len(service.pendingParentSync))
	}
	group, err := service.GetGroup(context.Background(), "durability-group")
	if err != nil || group.ApplyState != ApplyStateApplied {
		t.Fatalf("recovered change was not projected: group=%#v err=%v", group, err)
	}
}

func TestSaveParentSyncFailureCanRetryOriginalRequest(t *testing.T) {
	service, path := newTestServiceWithFile(t, "draft")
	baseRevision := Revision([]byte("draft"))
	originalSync := service.durability.syncRootDirFn
	failSync := true
	chapterSyncs := 0
	service.durability.syncRootDirFn = func(root *os.Root, rel string) error {
		if rel == "chapters" {
			chapterSyncs++
			if failSync && chapterSyncs >= 2 {
				return errInjectedParentSync
			}
		}
		return originalSync(root, rel)
	}

	_, err := service.SaveFile(context.Background(), path, "local draft", baseRevision)
	assertDurabilityPending(t, err, true)
	if got := readTestFile(t, service.workspace, path); got != "local draft" {
		t.Fatalf("save was not visible after the injected failure: %q", got)
	}
	if pending := service.pendingSaves[path]; pending.Revision != Revision([]byte("local draft")) || pending.Durable {
		t.Fatalf("pending save receipt was not retained: %#v", pending)
	}

	failSync = false
	result, err := service.SaveFile(context.Background(), path, "local draft", baseRevision)
	if err != nil {
		t.Fatalf("idempotent save retry failed: %v", err)
	}
	if !result.Changed || result.Revision != Revision([]byte("local draft")) {
		t.Fatalf("unexpected retry receipt: %#v", result)
	}
	if len(service.pendingSaves) != 0 || len(service.pendingParentSync) != 0 {
		t.Fatalf("successful retry retained pending state: saves=%d parents=%d", len(service.pendingSaves), len(service.pendingParentSync))
	}
}

func TestRemoveParentSyncFailureDoesNotRecordOperationProgress(t *testing.T) {
	workspace := t.TempDir()
	path := "chapters/new.md"
	service, err := NewService(workspace)
	if err != nil {
		t.Fatal(err)
	}
	change, err := service.ReplaceFile(context.Background(), ReplaceFileRequest{
		Path: path, Content: "agent-created", BaseRevision: "missing",
		Metadata: ChangeMetadata{Origin: OriginAgent, ChangeGroupID: "remove-durability-group"},
	})
	if err != nil {
		t.Fatal(err)
	}
	originalSync := service.durability.syncRootDirFn
	failSync := true
	service.durability.syncRootDirFn = func(root *os.Root, rel string) error {
		if failSync && rel == "chapters" {
			return errInjectedParentSync
		}
		return originalSync(root, rel)
	}

	_, err = service.Review(context.Background(), ReviewRequest{
		GroupID: change.GroupID, Decision: ReviewDecisionReject,
	})
	assertDurabilityPending(t, err, true)
	if _, statErr := os.Stat(filepath.Join(workspace, filepath.FromSlash(path))); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("removed path is unexpectedly visible: %v", statErr)
	}
	if len(service.operations) != 1 {
		t.Fatalf("remove operation was not retained: %d", len(service.operations))
	}
	for _, prepared := range service.operations {
		if len(prepared.AppliedPaths) != 0 {
			t.Fatalf("remove progress was recorded before parent sync: %#v", prepared.AppliedPaths)
		}
	}

	failSync = false
	service.mu.Lock()
	err = service.reconcilePendingDurabilityLocked()
	service.mu.Unlock()
	if err != nil {
		t.Fatalf("remove operation did not recover: %v", err)
	}
	group, err := service.GetGroup(context.Background(), change.GroupID)
	if err != nil || group.ReviewStatus != ReviewStatusRejected || group.ApplyState != ApplyStateReverted {
		t.Fatalf("recovered remove projection is incomplete: group=%#v err=%v", group, err)
	}
}

func TestPendingSaveRetryVerifiesCurrentRevision(t *testing.T) {
	service, path := newTestServiceWithFile(t, "draft")
	otherPath := "other.md"
	writeTestFile(t, service.workspace, otherPath, "other")
	baseRevision := Revision([]byte("draft"))
	originalSync := service.durability.syncRootDirFn
	failSync := true
	chapterSyncs := 0
	service.durability.syncRootDirFn = func(root *os.Root, rel string) error {
		if rel == "chapters" {
			chapterSyncs++
			if failSync && chapterSyncs >= 2 {
				return errInjectedParentSync
			}
		}
		return originalSync(root, rel)
	}
	if _, err := service.SaveFile(context.Background(), path, "local draft", baseRevision); err == nil {
		t.Fatal("injected parent sync failure did not fail the save")
	}

	failSync = false
	if _, err := service.SaveFile(context.Background(), otherPath, "other changed", Revision([]byte("other"))); err != nil {
		t.Fatalf("failed to reconcile pending save: %v", err)
	}
	writeTestFile(t, service.workspace, path, "external")
	_, err := service.SaveFile(context.Background(), path, "local draft", baseRevision)
	assertChangeErrorCode(t, err, ErrorCodeRevisionConflict)
	if got := readTestFile(t, service.workspace, path); got != "external" {
		t.Fatalf("stale save retry overwrote external content: %q", got)
	}
	if _, ok := service.pendingSaves[path]; ok {
		t.Fatal("divergent save retry retained an invalid receipt")
	}
}

func TestPreparedRecoverySyncsParentBeforeTerminalProjection(t *testing.T) {
	service, path := newTestServiceWithFile(t, "before")
	metadata := normalizeMetadata(ChangeMetadata{Origin: OriginAgent, ChangeGroupID: "recover-sync-group"})
	change := newChangeSet(path, []byte("before"), []byte("after"), true, true, []AppliedEdit{{
		ID: "edit", OldString: "before", NewString: "after", ReviewStatus: ReviewStatusPending,
		Hunks: []Hunk{{ID: "hunk", BeforeStart: 0, BeforeEnd: 6, AfterStart: 0, AfterEnd: 5}},
	}}, metadata)
	beforeBlob, err := service.store.writeBlob([]byte("before"))
	if err != nil {
		t.Fatal(err)
	}
	afterBlob, err := service.store.writeBlob([]byte("after"))
	if err != nil {
		t.Fatal(err)
	}
	change.BeforeBlob, change.AfterBlob = beforeBlob, afterBlob
	if err := service.appendAndApply(ledgerEvent{Type: eventChangePrepared, Metadata: &metadata, ChangeSet: &change}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.atomicWriteVisibleFile(path, []byte("after")); err != nil {
		t.Fatal(err)
	}

	originalSync := service.durability.syncRootDirFn
	service.durability.syncRootDirFn = func(_ *os.Root, rel string) error {
		if rel == "chapters" {
			return errInjectedParentSync
		}
		return nil
	}
	err = service.recoverPrepared()
	assertDurabilityPending(t, err, true)
	if len(service.prepared) != 1 || len(service.changeSets) != 0 {
		t.Fatalf("recovery projected an unsynchronized after-state: prepared=%d changes=%d", len(service.prepared), len(service.changeSets))
	}
	events, err := service.store.readAll()
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if event.Type == eventChangeRecoveredApplied && event.ChangeSetID == change.ID {
			t.Fatal("recovered terminal event was written before parent sync")
		}
	}

	service.durability.syncRootDirFn = originalSync
	if err := service.reconcilePendingDurabilityLocked(); err != nil {
		t.Fatalf("prepared change did not recover after sync resumed: %v", err)
	}
	if len(service.prepared) != 0 || service.changeSets[change.ID] == nil {
		t.Fatalf("prepared recovery did not finalize: prepared=%d change=%#v", len(service.prepared), service.changeSets[change.ID])
	}
}

func TestChangeStoreRejectsPrivateDirectorySymlink(t *testing.T) {
	workspace := t.TempDir()
	external := t.TempDir()
	if err := os.Symlink(external, filepath.Join(workspace, ".denova")); err != nil {
		t.Fatal(err)
	}
	_, err := NewService(workspace)
	assertChangeErrorCode(t, err, ErrorCodeConflict)
	if _, statErr := os.Stat(filepath.Join(external, "changes")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("change store followed private-directory symlink: %v", statErr)
	}
}

func TestChangeStoreRejectsLedgerSymlink(t *testing.T) {
	workspace := t.TempDir()
	changesDir := filepath.Join(workspace, ".denova", "changes")
	if err := os.MkdirAll(filepath.Join(changesDir, "blobs"), 0o700); err != nil {
		t.Fatal(err)
	}
	external := filepath.Join(t.TempDir(), "external-ledger")
	if err := os.WriteFile(external, []byte("do-not-touch"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(changesDir, "ledger.jsonl")); err != nil {
		t.Fatal(err)
	}
	_, err := NewService(workspace)
	assertChangeErrorCode(t, err, ErrorCodeConflict)
	content, readErr := os.ReadFile(external)
	if readErr != nil || string(content) != "do-not-touch" {
		t.Fatalf("ledger symlink target was modified: content=%q err=%v", content, readErr)
	}
}

func TestForWorkspaceCanonicalizesWorkspaceSymlink(t *testing.T) {
	workspace := t.TempDir()
	link := filepath.Join(t.TempDir(), "workspace-link")
	if err := os.Symlink(workspace, link); err != nil {
		t.Fatal(err)
	}
	direct, err := ForWorkspace(workspace)
	if err != nil {
		t.Fatal(err)
	}
	throughLink, err := ForWorkspace(link)
	if err != nil {
		t.Fatal(err)
	}
	if direct != throughLink {
		t.Fatal("one physical workspace received multiple mutation services")
	}
	canonical, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if direct.Workspace() != filepath.Clean(canonical) {
		t.Fatalf("service workspace=%q, want canonical %q", direct.Workspace(), filepath.Clean(canonical))
	}
}

func assertDurabilityPending(t *testing.T, err error, mutated bool) {
	t.Helper()
	var typed *Error
	if !errors.As(err, &typed) || typed.Code != ErrorCodeDurabilityPending {
		t.Fatalf("error=%v, want %s", err, ErrorCodeDurabilityPending)
	}
	got, ok := typed.Details["workspace_mutated"].(bool)
	if !ok || got != mutated {
		t.Fatalf("workspace_mutated=%#v, want %t details=%#v", typed.Details["workspace_mutated"], mutated, typed.Details)
	}
	if stage, _ := typed.Details["mutation_stage"].(mutationStage); stage == mutationStageUnchanged {
		t.Fatalf("pending durability reported unchanged stage: %#v", typed.Details)
	}
	if !strings.Contains(err.Error(), "durability") {
		t.Fatalf("pending error lacks durability context: %v", err)
	}
}
