package agent

import (
	"context"
	"strings"
	"time"
)

// RunSnapshot is a read-only view of one exact accepted Run. Result is nil until
// settlement; a suspended Run remains resumable. Unlike RecentRuns, lookup is
// not limited by display-history retention and also works after Session replay.
type RunSnapshot struct {
	Receipt    CommandReceipt
	Started    bool
	Suspended  bool
	Result     *Result
	Output     string
	FinishedAt time.Time
}

// CommandRun resolves a root input command through the canonical Session.
// A false result proves absence only in this exact Session, never another one.
func (session *Session) CommandRun(ctx context.Context, commandID string) (*Run, bool, error) {
	if err := session.usable(); err != nil {
		return nil, false, err
	}
	session.mu.RLock()
	defer session.mu.RUnlock()
	input := session.inputs[strings.TrimSpace(commandID)]
	if input == nil {
		return nil, false, nil
	}
	run := session.runs[input.Receipt.RunID]
	return run, run != nil, nil
}

func (run *Run) Snapshot() RunSnapshot {
	run.mu.RLock()
	defer run.mu.RUnlock()
	snapshot := RunSnapshot{
		Receipt: CommandReceipt{CommandID: run.commandID, RunID: run.id, Cursor: run.receipt},
		Started: !run.startedAt.IsZero(), Suspended: run.result.Status == ResultSuspended,
		Output: run.content.String(), FinishedAt: run.finishedAt,
	}
	if run.settled && !snapshot.Suspended {
		result := run.result
		snapshot.Result = &result
	}
	return snapshot
}
