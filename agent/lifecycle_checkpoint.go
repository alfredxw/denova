package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	runstate "github.com/alfredxw/denova/agent/internal/runstate"
	agentsession "github.com/alfredxw/denova/agent/session"
)

const turnCheckpointRecord = "turn.checkpoint"

// persistedCycle identifies the accepted input and product boundary for an
// unfinished cycle. Messages and capabilities have their own canonical lane.
type persistedCycle struct {
	RunID        string                      `json:"run_id"`
	CommandID    string                      `json:"command_id"`
	Cycle        int                         `json:"cycle"`
	StartedAt    time.Time                   `json:"started_at"`
	Delivery     runstate.DeliveryKind       `json:"delivery"`
	Autonomous   bool                        `json:"autonomous,omitempty"`
	InputCommit  *runstate.DomainCommitState `json:"input_commit,omitempty"`
	OutputCommit *runstate.DomainCommitState `json:"output_commit,omitempty"`
}

func cycleFact(snapshot runstate.TurnSnapshot) persistedCycle {
	return persistedCycle{RunID: string(snapshot.OperationID), CommandID: string(snapshot.CommandID),
		Cycle: snapshot.Cycle, StartedAt: snapshot.StartedAt, Delivery: snapshot.Delivery,
		Autonomous: snapshot.Autonomous, InputCommit: snapshot.InputCommit, OutputCommit: snapshot.OutputCommit}
}

func (run *Run) checkpointCycle(ctx context.Context, snapshot runstate.TurnSnapshot) error {
	session := run.session
	session.mu.Lock()
	defer session.mu.Unlock()
	record, err := sessionRecord(turnCheckpointRecord, cycleFact(snapshot))
	if err != nil {
		return err
	}
	records := []agentsession.Record{record}
	item := session.inputs[string(snapshot.CommandID)]
	if item == nil {
		return errors.New("Agent cycle has no durably accepted input")
	}
	if item.status == inputPending {
		consumed, err := sessionRecord(sessionInputUpdateRecord, persistedInputUpdate{
			CommandID: string(snapshot.CommandID), RunID: run.id, Status: inputConsumed, Delivery: snapshot.Delivery,
		})
		if err != nil {
			return err
		}
		records = append(records, consumed)
	}
	if err := session.appendRecordsLocked(ctx, records...); err != nil {
		return err
	}
	for _, record := range records {
		if record.Kind != turnCheckpointRecord {
			if err := session.replayInput(record); err != nil {
				return err
			}
		}
	}
	run.mu.Lock()
	run.snapshot = snapshot
	run.mu.Unlock()
	return nil
}

func (session *Session) replayCycle(data json.RawMessage) error {
	var cycle persistedCycle
	if err := json.Unmarshal(data, &cycle); err != nil {
		return err
	}
	run := session.runs[cycle.RunID]
	item := session.inputs[cycle.CommandID]
	if run == nil || item == nil || cycle.Cycle < 1 {
		return errors.New("persisted Agent cycle has no accepted Run/input")
	}
	run.cycle = cycle.Cycle
	run.snapshot = runstate.TurnSnapshot{
		ID: runstate.SnapshotID(fmt.Sprintf("%s:%d", run.id, cycle.Cycle)), Binding: session.binding.Clone(),
		OperationID: runstate.OperationID(run.id), CommandID: runstate.CommandID(cycle.CommandID), Cycle: cycle.Cycle,
		StartedAt: cycle.StartedAt, Delivery: cycle.Delivery, Autonomous: cycle.Autonomous,
		Input: item.Input, InputCommit: cycle.InputCommit, OutputCommit: cycle.OutputCommit,
	}
	return nil
}

func (session *Session) removePendingLocked(run *Run) {
	for index, candidate := range session.pending {
		if candidate == run {
			session.pending = append(session.pending[:index], session.pending[index+1:]...)
			return
		}
	}
}

func (run *Run) endHandle() {
	run.endOnce.Do(func() {
		close(run.done)
		run.eventMu.Lock()
		run.eventsEnd = true
		close(run.events)
		run.eventMu.Unlock()
	})
}
