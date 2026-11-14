package lifecycle

import (
	"context"
	"encoding/json"
	"errors"

	agentengine "github.com/alfredxw/denova/agent/engine"
	agentexecution "github.com/alfredxw/denova/agent/engine/execution"
	agentschema "github.com/alfredxw/denova/agent/schema"
	agentsession "github.com/alfredxw/denova/agent/session"
	agentcanonical "github.com/alfredxw/denova/agent/session/canonical"
)

func (run *Run) commitCanonical(_ context.Context, update agentengine.CanonicalUpdate, commit func(agentcanonical.CanonicalCheckpoint) error) error {
	if !run.session.canonicalMessages {
		return commit(nil)
	}
	session := run.session
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.storageErr != nil {
		return session.storageErr
	}
	var records []agentsession.Record
	var nextState json.RawMessage
	var nextSnapshot agentengine.TurnSnapshot
	var checkpoint agentengine.PersistedMessageCheckpoint
	var completionIDs []string
	prepared := false
	err := commit(func(receipt agentcanonical.CommitReceipt) (agentcanonical.JournalCheckpoint, error) {
		// A product journal CAS conflict can retry preparation before anything
		// is committed. Rebuild from the locked Agent state, retaining only the
		// final attempt and leaving previously returned records untouched.
		prepared = false
		records = nil
		completionIDs = nil
		if receipt.Revision == "" {
			return agentcanonical.JournalCheckpoint{}, errors.New("canonical checkpoint requires the product revision")
		}
		candidate, err := agentengine.PrepareCanonicalCommit(session.key, session.engineState, session.capabilities, update, receipt)
		if err != nil {
			return agentcanonical.JournalCheckpoint{}, err
		}
		nextState, nextSnapshot, checkpoint = candidate.State, candidate.Snapshot, candidate.Checkpoint
		if update.Stage == agentcanonical.CommitInput {
			consumed, err := sessionRecord(sessionInputUpdateRecord, persistedInputUpdate{
				CommandID: string(nextSnapshot.CommandID), RunID: run.id, Status: inputConsumed, Delivery: nextSnapshot.Delivery,
			})
			if err != nil {
				return agentcanonical.JournalCheckpoint{}, err
			}
			records = append(records, consumed)
		}
		cycle, err := sessionRecord(turnCheckpointRecord, cycleFact(nextSnapshot))
		if err != nil {
			return agentcanonical.JournalCheckpoint{}, err
		}
		records = append(records, cycle)
		messageRecord, err := sessionRecord(sessionMessageCheckpointRecord, checkpoint)
		if err != nil {
			return agentcanonical.JournalCheckpoint{}, err
		}
		records = append(records, messageRecord)
		capabilityRecords, err := contextCapabilityRecords(update.CapabilityStates)
		if err != nil {
			return agentcanonical.JournalCheckpoint{}, err
		}
		records = append(records, capabilityRecords...)
		if update.Tool != nil {
			fact, err := sessionRecord(turnToolRecord, *update.Tool)
			if err != nil {
				return agentcanonical.JournalCheckpoint{}, err
			}
			records = append(records, fact)
		}
		for _, id := range candidate.CompletionIDs {
			if _, delivered := session.taskCompletions.delivered[id]; !delivered {
				completionIDs = append(completionIDs, id)
			}
		}
		if len(completionIDs) > 0 {
			delivery, err := sessionRecord(sessionTaskCompletionDeliveryRecord, persistedTaskCompletionDelivery{IDs: completionIDs})
			if err != nil {
				return agentcanonical.JournalCheckpoint{}, err
			}
			records = append(records, delivery)
		}
		prepared = true
		return agentcanonical.JournalCheckpoint{Session: session.Key(), ExpectedRevision: session.revision, Records: records}, nil
	})
	if err != nil {
		if errors.Is(err, agentsession.ErrCommitUnknown) {
			session.storageErr = err
		}
		return err
	}
	if !prepared {
		return errors.New("embedded canonical adapter omitted the Agent checkpoint")
	}
	if err := session.recordCommittedLocked(records, session.revision+1); err != nil {
		return err
	}
	session.revision += agentsession.Revision(len(records))
	session.engineState, session.messageCheckpoint = nextState, checkpoint
	// The Engine request can predate compaction or a tool's capability update.
	// Every product commit must keep the live Run on the current Session state.
	nextSnapshot.Capabilities = agentschema.CloneRawStateMap(session.capabilities)
	for capability, value := range update.CapabilityStates {
		session.capabilities[capability] = append(json.RawMessage(nil), value...)
		session.durableCapabilities[capability] = append(json.RawMessage(nil), value...)
		nextSnapshot.Capabilities[capability] = append(json.RawMessage(nil), value...)
	}
	if update.Tool != nil {
		run.tools[update.Tool.CallID] = *update.Tool
	}
	for _, id := range completionIDs {
		session.taskCompletions.delivered[id] = struct{}{}
		delete(session.taskCompletions.pending, id)
	}
	for _, record := range records {
		if record.Kind == sessionInputUpdateRecord {
			if err := session.replayInput(record); err != nil {
				return err
			}
		}
	}
	nextSnapshot.State = append(json.RawMessage(nil), nextState...)
	run.mu.Lock()
	run.snapshot = nextSnapshot
	run.mu.Unlock()
	return nil
}

func (run *Run) commitProductAcceptance(ctx context.Context, result *agentschema.ToolResult, commit func(agentcanonical.CanonicalCheckpoint) error) error {
	if !run.session.canonicalMessages {
		return errors.New("product acceptance requires an embedded canonical journal")
	}
	run.session.mu.RLock()
	state := append(json.RawMessage(nil), run.session.engineState...)
	var fact *agentengine.PersistedTool
	if result != nil {
		stored, found := run.tools[agentexecution.CurrentToolExecutionID(ctx)]
		if !found || !stored.Started {
			run.session.mu.RUnlock()
			return errors.New("product tool acceptance requires its durable intent")
		}
		stored.Result, stored.Source = result, "product"
		fact = &stored
	}
	run.session.mu.RUnlock()
	snapshot, err := run.snapshotForCurrentCycle()
	if err != nil {
		return err
	}
	return run.commitCanonical(ctx, agentengine.CanonicalUpdate{Stage: agentcanonical.CommitContext, Snapshot: snapshot, State: state, Tool: fact}, commit)
}

// runJournalPort is a narrow per-cycle adapter, not a second execution owner.
type runJournalPort struct{ run *Run }

func (port runJournalPort) CommitCanonical(ctx context.Context, update agentengine.CanonicalUpdate, commit func(agentcanonical.CanonicalCheckpoint) error) error {
	return port.run.commitCanonical(ctx, update, commit)
}

func (port runJournalPort) CommitProductAcceptance(ctx context.Context, result *agentschema.ToolResult, commit func(agentcanonical.CanonicalCheckpoint) error) error {
	return port.run.commitProductAcceptance(ctx, result, commit)
}

func (port runJournalPort) ToolFacts() map[string]agentengine.PersistedTool {
	return port.run.toolFacts()
}
