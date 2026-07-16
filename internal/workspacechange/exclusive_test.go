package workspacechange

import (
	"context"
	"testing"
)

func TestConsistentWorkspaceSnapshotPreservesPendingSaveReceipt(t *testing.T) {
	service, err := NewService(t.TempDir())
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	service.pendingSaves["draft.md"] = pendingSaveIntent{Path: "draft.md", Revision: "sha256:test"}

	if err := service.WithConsistentWorkspaceSnapshot(context.Background(), func() error { return nil }); err != nil {
		t.Fatalf("consistent snapshot: %v", err)
	}
	if _, ok := service.pendingSaves["draft.md"]; !ok {
		t.Fatal("read-only snapshot invalidated the pending save receipt")
	}

	if err := service.WithExclusiveWorkspace(context.Background(), func() error { return nil }); err != nil {
		t.Fatalf("exclusive mutation: %v", err)
	}
	if len(service.pendingSaves) != 0 {
		t.Fatalf("exclusive mutation retained %d stale pending save receipts", len(service.pendingSaves))
	}
}
