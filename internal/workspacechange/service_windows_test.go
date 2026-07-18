//go:build windows

package workspacechange

import (
	"context"
	"testing"
)

func TestNewServiceInitializesWorkspaceChangeStoreOnWindows(t *testing.T) {
	service, err := NewService(t.TempDir())
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	groups, err := service.ListGroups(context.Background(), ChangeFilter{})
	if err != nil {
		t.Fatalf("ListGroups failed: %v", err)
	}
	if len(groups) != 0 {
		t.Fatalf("new workspace has %d change groups, want 0", len(groups))
	}
}
