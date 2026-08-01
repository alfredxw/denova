package filejournal

import (
	"context"
	"fmt"
	runstate "github.com/alfredxw/denova/agent/runtime"
)

func (j *journal) Load(ctx context.Context) ([]runstate.Event, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if j.closed {
		return nil, fmt.Errorf("file journal is closed")
	}
	if j.activeGeneration.SnapshotFile != "" {
		state := runstate.NewJournalCheckpointState(j.binding)
		if _, err := j.replayCheckpointLocked(ctx, state); err != nil {
			j.initialized = false
			return nil, err
		}
		return state.RetainedEvents(), nil
	}
	events := make([]runstate.Event, 0)
	_, err := j.replayLocked(ctx, true, func(event runstate.Event) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		j.initialized = false
		return nil, err
	}
	return events, nil
}

// Replay decodes one checksummed transaction at a time and sends its events
// directly to reduce. Unlike Load it never retains the complete event history.
func (j *journal) Replay(ctx context.Context, reduce func(runstate.Event) error) (runstate.JournalReplayStats, error) {
	if err := ctx.Err(); err != nil {
		return runstate.JournalReplayStats{}, err
	}
	if reduce == nil {
		return runstate.JournalReplayStats{}, fmt.Errorf("file journal replay reducer is required")
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return runstate.JournalReplayStats{}, fmt.Errorf("file journal is closed")
	}
	if j.activeGeneration.SnapshotFile != "" {
		state := runstate.NewJournalCheckpointState(j.binding)
		stats, err := j.replayCheckpointLocked(ctx, state)
		if err != nil {
			j.initialized = false
			return stats, err
		}
		for _, event := range state.RetainedEvents() {
			if err := reduce(event); err != nil {
				return stats, err
			}
		}
		return stats, nil
	}
	stats, err := j.replayLocked(ctx, true, reduce)
	if err != nil {
		j.initialized = false
		return stats, err
	}
	return stats, nil
}
