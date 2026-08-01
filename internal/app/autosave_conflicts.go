package app

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"denova/internal/workspace/autosave"
)

// RecordAutosaveConflict durably preserves every side of a merge conflict in
// the process-wide Denova data directory before a caller resolves it.
func (a *App) RecordAutosaveConflict(ctx context.Context, input autosave.Input) (autosave.AppendResult, error) {
	if a == nil {
		return autosave.AppendResult{}, fmt.Errorf("record autosave conflict: app is nil")
	}
	a.mu.RLock()
	dataDir := ""
	if a.cfg != nil {
		dataDir = strings.TrimSpace(a.cfg.DataDir())
	}
	a.mu.RUnlock()
	if dataDir == "" {
		return autosave.AppendResult{}, fmt.Errorf("record autosave conflict: Denova data directory is not configured")
	}
	result, err := autosave.NewStore(dataDir).Append(ctx, input)
	if err != nil {
		return autosave.AppendResult{}, fmt.Errorf("record autosave conflict: %w", err)
	}
	slog.InfoContext(ctx, fmt.Sprintf("[autosave-conflict] recorded resource=%q scope=%q id=%q record_id=%q path=%q", input.Resource, input.Scope, input.ID, result.Record.ID, result.Path))
	return result, nil
}
