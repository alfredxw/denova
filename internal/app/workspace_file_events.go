package app

import (
	"context"
	"denova/internal/workspace/filewatch"
	"fmt"
	"log/slog"
)

// syncWorkspaceFileWatcher follows the committed runtime workspace. Watcher
// setup failure is non-fatal because focus/visibility refresh remains the
// authoritative fallback on filesystems without native notifications.
func (a *App) syncWorkspaceFileWatcher(workspace string) {
	if a == nil || a.workspaceFiles == nil {
		return
	}
	if err := a.workspaceFiles.SetWorkspace(workspace); err != nil {
		slog.WarnContext(context.Background(), fmt.Sprintf("[filewatch] workspace watcher unavailable; foreground refresh remains active workspace=%q err=%v", workspace, err))
		return
	}
	if workspace != "" {
		slog.InfoContext(context.Background(), fmt.Sprintf("[filewatch] workspace watcher active workspace=%q", workspace))
	}
}

// SubscribeWorkspaceFileChanges returns ephemeral invalidation events for the
// current workspace. Every subscription starts with resync, so clients recover
// by re-reading canonical state instead of replaying an event journal.
func (a *App) SubscribeWorkspaceFileChanges() (<-chan filewatch.Event, func()) {
	if a == nil || a.workspaceFiles == nil {
		closed := make(chan filewatch.Event)
		close(closed)
		return closed, func() {}
	}
	return a.workspaceFiles.Subscribe()
}
