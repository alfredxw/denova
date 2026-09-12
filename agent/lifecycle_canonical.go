package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	runstate "github.com/alfredxw/denova/agent/internal/runstate"
	agentsession "github.com/alfredxw/denova/agent/session"
)

type canonicalRunKey struct{}

func canonicalMessageCheckpoint(encoded json.RawMessage) (persistedMessageCheckpoint, error) {
	state, err := decodeEngineTranscript(encoded)
	if err != nil {
		return persistedMessageCheckpoint{}, err
	}
	committed := len(state.Messages)
	for index := len(state.Messages) - 1; index >= 0; index-- {
		message := state.Messages[index]
		if message.Role == Assistant && len(message.ToolCalls) > 0 {
			if len(state.Messages)-index-1 < len(message.ToolCalls) {
				committed = index
			}
			break
		}
		if message.Role != ToolRole {
			break
		}
	}
	hash, err := hashCanonical(state.Messages[:committed])
	if err != nil {
		return persistedMessageCheckpoint{}, err
	}
	pending := cloneMessages(state.Messages[committed:])
	state.Messages = nil
	metadata, err := json.Marshal(state)
	if err != nil {
		return persistedMessageCheckpoint{}, err
	}
	return persistedMessageCheckpoint{Hash: hash, MessageCount: committed, Metadata: metadata, Pending: pending}, nil
}

// canonicalUpdate describes one existing product boundary. The Session lock
// spans the host commit so inbox acceptance cannot race its logical revision.
type canonicalUpdate struct {
	Stage    CommitStage
	Snapshot runstate.TurnSnapshot
	State    json.RawMessage
	Hash     string
	Tool     *persistedTool
}

func withCanonicalCheckpoint(ctx context.Context, update canonicalUpdate, commit func(CanonicalCheckpoint) error) error {
	run, _ := ctx.Value(canonicalRunKey{}).(*Run)
	if run == nil || !run.session.canonicalMessages {
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
	var nextSnapshot runstate.TurnSnapshot
	var checkpoint persistedMessageCheckpoint
	var completionIDs []string
	prepared := false
	err := commit(func(receipt CommitReceipt) (JournalCheckpoint, error) {
		if prepared {
			return JournalCheckpoint{}, errors.New("canonical commit requested its checkpoint more than once")
		}
		if receipt.Revision == "" {
			return JournalCheckpoint{}, errors.New("canonical checkpoint requires the product revision")
		}
		prepared = true
		nextSnapshot = update.Snapshot
		nextState = append(json.RawMessage(nil), update.State...)
		switch update.Stage {
		case CommitInput:
			input, err := decodeInput(nextSnapshot.Input)
			if err != nil {
				return JournalCheckpoint{}, err
			}
			state, err := decodeEngineTranscript(session.engineState)
			if err != nil {
				return JournalCheckpoint{}, err
			}
			state.DefinitionKey, state.BehaviorKey, state.MaterializedFingerprint, state.PreparationStage = "", "", "", ""
			state.DefinitionOperationID, state.DefinitionCommandID, state.DefinitionCycle = run.id, string(nextSnapshot.CommandID), nextSnapshot.Cycle
			state.ContextSequence = 0
			state.LastResponseOrdinal = 0
			state.ActiveUserIndex, state.ActiveModelUser = len(state.Messages), UserMessageWithAttachments(input.Text, input.Attachments)
			state.Messages = append(state.Messages, state.ActiveModelUser.Clone())
			state.HostData = cloneHostData(input.HostData)
			nextState, err = json.Marshal(state)
			if err != nil {
				return JournalCheckpoint{}, err
			}
			nextSnapshot.InputCommit = &runstate.DomainCommitState{
				Identity: engineCommitIdentity(canonicalCommitIdentity(session.key, nextSnapshot, CommitInput)), Hash: update.Hash, Revision: receipt.Revision,
			}
			consumed, err := sessionRecord(sessionInputUpdateRecord, persistedInputUpdate{
				CommandID: string(nextSnapshot.CommandID), RunID: run.id, Status: inputConsumed, Delivery: nextSnapshot.Delivery,
			})
			if err != nil {
				return JournalCheckpoint{}, err
			}
			records = append(records, consumed)
		case CommitContext:
		case CommitOutput:
			nextSnapshot.OutputCommit = &runstate.DomainCommitState{
				Identity: engineCommitIdentity(canonicalCommitIdentity(session.key, nextSnapshot, CommitOutput)), Hash: update.Hash, Revision: receipt.Revision,
			}
			if len(nextState) == 0 {
				nextState = append(json.RawMessage(nil), session.engineState...)
			}
		default:
			return JournalCheckpoint{}, fmt.Errorf("unsupported canonical checkpoint stage %q", update.Stage)
		}
		cycle, err := sessionRecord(turnCheckpointRecord, cycleFact(nextSnapshot))
		if err != nil {
			return JournalCheckpoint{}, err
		}
		records = append(records, cycle)
		checkpoint, err = canonicalMessageCheckpoint(nextState)
		if err != nil {
			return JournalCheckpoint{}, err
		}
		messageRecord, err := sessionRecord(sessionMessageCheckpointRecord, checkpoint)
		if err != nil {
			return JournalCheckpoint{}, err
		}
		records = append(records, messageRecord)
		if update.Tool != nil {
			fact, err := sessionRecord(turnToolRecord, *update.Tool)
			if err != nil {
				return JournalCheckpoint{}, err
			}
			records = append(records, fact)
		}
		state, err := decodeEngineTranscript(nextState)
		if err != nil {
			return JournalCheckpoint{}, err
		}
		for _, message := range state.Messages {
			if message.TaskCompletion == nil {
				continue
			}
			id := message.TaskCompletion.CompletionID
			if _, delivered := session.taskCompletions.delivered[id]; !delivered {
				completionIDs = append(completionIDs, id)
			}
		}
		if len(completionIDs) > 0 {
			delivery, err := sessionRecord(sessionTaskCompletionDeliveryRecord, persistedTaskCompletionDelivery{IDs: completionIDs})
			if err != nil {
				return JournalCheckpoint{}, err
			}
			records = append(records, delivery)
		}
		return JournalCheckpoint{Session: session.Key(), ExpectedRevision: session.revision, Records: records}, nil
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
	session.revision += agentsession.Revision(len(records))
	session.engineState, session.messageCheckpoint = nextState, checkpoint
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

// CommitProductAcceptance lets an embedded canonical host accept product
// progress together with the current continuation checkpoint. For a concrete
// tool, result must be its confirmed product receipt; outside a tool it is nil.
// The callback must append the supplied checkpoint in its product transaction
// and must not reenter Session methods. Ordinary tools need no such callback.
func CommitProductAcceptance(ctx context.Context, result *ToolResult, commit func(CanonicalCheckpoint) error) error {
	run, _ := ctx.Value(canonicalRunKey{}).(*Run)
	if run == nil {
		return commit(nil)
	}
	if !run.session.canonicalMessages {
		return errors.New("product acceptance requires an embedded canonical journal")
	}
	run.session.mu.RLock()
	state := append(json.RawMessage(nil), run.session.engineState...)
	var fact *persistedTool
	if result != nil {
		stored, found := run.tools[CurrentToolExecutionID(ctx)]
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
	return withCanonicalCheckpoint(ctx, canonicalUpdate{Stage: CommitContext, Snapshot: snapshot, State: state, Tool: fact}, commit)
}
