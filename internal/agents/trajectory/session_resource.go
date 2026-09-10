package trajectory

import (
	"context"
	"errors"
	"math"
	"os"

	agenttools "github.com/alfredxw/denova/agent/tools"

	"denova/internal/agents/session"
)

type sessionResourceManifest struct {
	Schema     string `json:"schema"`
	Type       string `json:"type"`
	Project    Source `json:"project"`
	SessionID  string `json:"session_id"`
	LineFormat string `json:"line_format"`
}

func (catalog Catalog) readSessionResource(ctx context.Context, resource string, source Source, sessionID string, input readInput, limit int) (agenttools.ReadResult, error) {
	offset := max(1, input.Offset)
	if offset > math.MaxInt-limit {
		return agenttools.ReadResult{}, errors.New("trajectory offset is too large")
	}
	directory := sessionDir(source.StateRoot)
	if _, err := os.Stat(directory); err != nil {
		return agenttools.ReadResult{}, err
	}
	store, err := session.NewStore(directory)
	if err != nil {
		return agenttools.ReadResult{}, err
	}
	defer store.Close()
	target, err := store.Get(sessionID)
	if err != nil {
		return agenttools.ReadResult{}, err
	}

	// History positions are zero-based; JSONL line 1 is the manifest. The
	// indexed reader may extend its start to preserve a whole UI turn. Trim
	// that lookbehind here so the read tool's exact line limit still applies.
	start := max(0, offset-2)
	end := offset + limit - 2
	page, err := target.ReadHistoryPage(ctx, end, max(1, end-start))
	if err != nil {
		return agenttools.ReadResult{}, err
	}
	lines := make([]string, 0, limit)
	if offset == 1 {
		line, err := marshalRedactedJSONLine(sessionResourceManifest{
			Schema: "denova.trajectory.session.v2", Type: "session_summary",
			Project: source, SessionID: sessionID,
			LineFormat: "Chronological history entries follow. Null lines omit private reasoning while preserving history positions.",
		}, source)
		if err != nil {
			return agenttools.ReadResult{}, err
		}
		lines = append(lines, line)
	}
	from := min(max(0, start-page.NextBefore), len(page.Entries))
	for _, entry := range page.Entries[from:] {
		if err := ctx.Err(); err != nil {
			return agenttools.ReadResult{}, err
		}
		// Keep one line per indexed history position. Dropping a reasoning row
		// would renumber later evidence and break offset/byte_offset continuation.
		var value any = entry
		if entry.Role == "thinking" {
			value = nil
		}
		line, err := marshalRedactedJSONLine(value, source)
		if err != nil {
			return agenttools.ReadResult{}, err
		}
		lines = append(lines, line)
	}
	return trajectoryLineResult(resource, "trajectory_session", input, lines, page.Total+1)
}
