package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	runstate "github.com/alfredxw/denova/agent/internal/runstate"
	agentsession "github.com/alfredxw/denova/agent/session"
)

func (session *Session) loadRecovery(ctx context.Context) error {
	var index *RecoveryIndex
	var err error
	if log, ok := session.log.(RecoveryLog); ok {
		index, err = log.Recovery(ctx)
	} else {
		index = &RecoveryIndex{}
		_, err = session.log.Replay(ctx, index.Apply)
	}
	if err != nil {
		return err
	}
	session.recovery = index
	for id, input := range index.Inputs {
		session.inputs[id] = &acceptedInput{persistedInput: persistedInput{Receipt: input.Receipt, Kind: input.Kind, Hash: input.Hash}, targetRunID: input.RunID, status: input.Status}
		session.cursor = max(session.cursor, input.Receipt.Cursor)
	}
	return nil
}

func (session *Session) recordCommittedLocked(records []agentsession.Record, first agentsession.Revision) error {
	for offset, record := range records {
		record.Revision = first + agentsession.Revision(offset)
		if err := session.recovery.Apply(record); err != nil {
			session.storageErr = fmt.Errorf("index committed Agent record: %w", err)
			return session.storageErr
		}
	}
	return nil
}

func (session *Session) readRecord(ctx context.Context, revision agentsession.Revision) (agentsession.Record, error) {
	if reader, ok := session.log.(interface {
		ReadRecord(context.Context, agentsession.Revision) (agentsession.Record, error)
	}); ok {
		return reader.ReadRecord(ctx, revision)
	}
	var found agentsession.Record
	_, err := session.log.Replay(ctx, func(record agentsession.Record) error {
		if record.Revision == revision {
			found = record
			found.Data = append(json.RawMessage(nil), record.Data...)
		}
		return nil
	})
	if err != nil {
		return found, err
	}
	if found.Revision == 0 {
		return found, fmt.Errorf("Agent record revision %d is missing", revision)
	}
	return found, nil
}

// retireRunLocked runs only after durable settlement. Existing caller-owned
// handles keep their result, while the Session stops owning their execution
// state. Suspended and queued work is deliberately excluded.
func (session *Session) retireRunLocked(run *Run) {
	if run.result.Status == ResultSuspended {
		return
	}
	delete(session.runs, run.id)
	for _, input := range session.inputs {
		if input.targetRunID == run.id {
			input.status = session.recovery.Inputs[input.Receipt.CommandID].Status
			if input.status == inputPending {
				continue
			}
			input.input = Input{}
			input.Input = runstate.UserInput{}
			input.bytes = 0
		}
	}
	order := session.inputOrder[:0]
	for _, input := range session.inputOrder {
		if input.status == inputPending {
			order = append(order, input)
		}
	}
	clear(session.inputOrder[len(order):])
	session.inputOrder = order
	run.mu.Lock()
	run.snapshot = runstate.TurnSnapshot{}
	run.tools, run.responses, run.interactions, run.openTools, run.toolSources = nil, nil, nil, nil, nil
	run.input = Input{}
	run.thinking.Reset()
	run.mu.Unlock()
}

func (session *Session) restoreRecent(ctx context.Context) error {
	runs := make([]RecoveryRun, 0, len(session.recovery.Runs))
	for _, run := range session.recovery.Runs {
		if run.Settlement != 0 {
			runs = append(runs, run)
		}
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].Settlement < runs[j].Settlement })
	if len(runs) > 32 {
		runs = runs[len(runs)-32:]
	}
	for _, run := range runs {
		record, err := session.readRecord(ctx, run.Settlement)
		if err != nil {
			return err
		}
		var turn persistedTurn
		if err := json.Unmarshal(record.Data, &turn); err != nil {
			return err
		}
		session.addRecentLocked(RunSummary{ID: turn.RunID, CommandID: turn.CommandID, ReceiptCursor: run.Receipt.Cursor, Status: turn.Status, Reason: turn.Reason, Output: turn.Output})
	}
	return nil
}
