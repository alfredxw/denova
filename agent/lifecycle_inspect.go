package agent

import (
	"context"
	"encoding/json"
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

// AcceptedControl returns a journal receipt for retry admission. Callers must
// still invoke the original operation to validate its kind, target and payload.
func (session *Session) AcceptedControl(commandID string) (CommandReceipt, bool) {
	session.mu.RLock()
	defer session.mu.RUnlock()
	control, found := session.controlReceipts[strings.TrimSpace(commandID)]
	return control.Receipt, found
}

// CommandRun resolves a root input command through the canonical Session.
// A false result proves absence only in this exact Session, never another one.
func (session *Session) CommandRun(ctx context.Context, commandID string) (*Run, bool, error) {
	if err := session.usable(); err != nil {
		return nil, false, err
	}
	session.mu.RLock()
	input := session.inputs[strings.TrimSpace(commandID)]
	session.mu.RUnlock()
	if input == nil {
		return nil, false, nil
	}
	return session.AttachRun(ctx, input.Receipt.RunID)
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

// CommandSnapshot resolves a durable command without restoring a historical
// execution handle. Lookup errors are never treated as proof of absence.
func (session *Session) CommandSnapshot(ctx context.Context, commandID string) (RunSnapshot, bool, error) {
	if err := session.usable(); err != nil {
		return RunSnapshot{}, false, err
	}
	session.mu.RLock()
	input := session.inputs[strings.TrimSpace(commandID)]
	session.mu.RUnlock()
	if input == nil {
		return RunSnapshot{}, false, nil
	}
	return session.RunSnapshot(ctx, input.Receipt.RunID)
}

// RunSnapshot reads live state or the exact terminal record on demand. It does
// not register a Run, allocate event buffers, or execute a model/tool.
func (session *Session) RunSnapshot(ctx context.Context, runID string) (RunSnapshot, bool, error) {
	if err := session.usable(); err != nil {
		return RunSnapshot{}, false, err
	}
	runID = strings.TrimSpace(runID)
	session.mu.RLock()
	live := session.runs[runID]
	entry, found := session.recovery.Runs[runID]
	session.mu.RUnlock()
	if live != nil {
		return live.Snapshot(), true, nil
	}
	if !found || entry.Settlement == 0 {
		return RunSnapshot{}, false, nil
	}
	record, err := session.readRecord(ctx, entry.Settlement)
	if err != nil {
		return RunSnapshot{}, false, err
	}
	var turn persistedTurn
	if err := json.Unmarshal(record.Data, &turn); err != nil {
		return RunSnapshot{}, false, err
	}
	return RunSnapshot{Receipt: entry.Receipt, Started: entry.Started, Result: &Result{Status: turn.Status, Reason: turn.Reason}, Output: turn.Output, FinishedAt: turn.At}, true, nil
}

// Settled handles are caller-owned views. They deliberately have no buffered
// stream, engine snapshot, or Session registry entry.
func (session *Session) settledHandle(snapshot RunSnapshot) *Run {
	run := &Run{session: session, id: snapshot.Receipt.RunID, commandID: snapshot.Receipt.CommandID, receipt: snapshot.Receipt.Cursor,
		settled: true, result: *snapshot.Result, finishedAt: snapshot.FinishedAt,
		events: make(chan Event), done: make(chan struct{}), executionDone: make(chan struct{})}
	if snapshot.Started {
		run.startedAt = snapshot.FinishedAt
	}
	run.content.WriteString(snapshot.Output)
	if run.result.Status != ResultCompleted && run.result.Status != ResultAborted {
		run.err = &RunError{Result: run.result}
	}
	run.endHandle()
	close(run.executionDone)
	return run
}
