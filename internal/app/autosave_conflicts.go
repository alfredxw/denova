package app

import (
	"context"
	"fmt"
	"log"
	"strings"

	"denova/internal/autosaveconflict"
)

// RecordAutosaveConflict durably preserves every side of a merge conflict in
// the process-wide Denova data directory before a caller resolves it.
func (a *App) RecordAutosaveConflict(ctx context.Context, input autosaveconflict.Input) (autosaveconflict.AppendResult, error) {
	if a == nil {
		return autosaveconflict.AppendResult{}, fmt.Errorf("record autosave conflict: app is nil")
	}
	a.mu.RLock()
	dataDir := ""
	if a.cfg != nil {
		dataDir = strings.TrimSpace(a.cfg.DataDir())
	}
	a.mu.RUnlock()
	if dataDir == "" {
		return autosaveconflict.AppendResult{}, fmt.Errorf("record autosave conflict: Denova data directory is not configured")
	}
	result, err := autosaveconflict.NewStore(dataDir).Append(ctx, input)
	if err != nil {
		return autosaveconflict.AppendResult{}, fmt.Errorf("record autosave conflict: %w", err)
	}
	log.Printf("[autosave-conflict] recorded resource=%q scope=%q id=%q record_id=%q path=%q", input.Resource, input.Scope, input.ID, result.Record.ID, result.Path)
	return result, nil
}
