package lifecycle

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	agenthistory "github.com/alfredxw/denova/agent/context/history"
	agentengine "github.com/alfredxw/denova/agent/engine"
	agentexecution "github.com/alfredxw/denova/agent/engine/execution"
	agentgoal "github.com/alfredxw/denova/agent/engine/goal"
	agentasync "github.com/alfredxw/denova/agent/internal/async"
	agentevent "github.com/alfredxw/denova/agent/lifecycle/event"
	agentschema "github.com/alfredxw/denova/agent/schema"
	agentsession "github.com/alfredxw/denova/agent/session"
	agentcanonical "github.com/alfredxw/denova/agent/session/canonical"
)

const (
	sessionTranscriptRecord             = "session.transcript"
	sessionMessageCheckpointRecord      = "session.message_checkpoint"
	sessionCapabilitySetRecord          = "session.capability_set"
	sessionCapabilityDeleteRecord       = "session.capability_delete"
	sessionTaskCompletionDeliveryRecord = "session.task_completion_delivery"
	turnStartedRecord                   = "turn.started"
	turnFinishedRecord                  = "turn.finished"
	turnInterruptedRecord               = "turn.interrupted"
	sessionRecordVersion                = 1
	retainedSessionEvents               = 4096
)

type persistedSessionTranscript struct {
	EngineState json.RawMessage `json:"engine_state,omitempty"`
}

type persistedCapability struct {
	Capability string          `json:"capability"`
	State      json.RawMessage `json:"state,omitempty"`
}

type persistedTaskCompletionDelivery struct {
	IDs []string `json:"ids"`
}

type persistedTurn struct {
	RunID     string                   `json:"run_id"`
	CommandID string                   `json:"command_id"`
	Status    agentschema.ResultStatus `json:"status,omitempty"`
	Reason    string                   `json:"reason,omitempty"`
	Output    string                   `json:"output,omitempty"`
	At        time.Time                `json:"at"`
}

type sessionObserver struct {
	events chan agentevent.Event
	errors chan error
	drops  eventDropState
}

// Session serializes Runs for one conversation. Canonical messages, versioned
// capability updates, and settled turn records may cross process boundaries;
// all live coordination is intentionally kept here in memory.
type Session struct {
	agent    *Agent
	key      agentsession.Key
	binding  agentengine.BindingRef
	engine   agentengine.Runner
	log      agentsession.Log
	recovery *RecoveryIndex

	mu                  sync.RWMutex
	closed              bool
	closing             bool
	closeDone           chan struct{}
	closeErr            error
	storageErr          error
	revision            agentsession.Revision
	engineState         json.RawMessage
	capabilities        map[string]json.RawMessage
	durableCapabilities map[string]json.RawMessage
	canonicalMessages   bool
	canonicalSource     agentcanonical.CanonicalHistorySource
	messageCheckpoint   agentengine.PersistedMessageCheckpoint
	active              *Run
	maintenance         bool
	pending             []*Run
	runs                map[string]*Run
	inputs              map[string]*acceptedInput
	inputOrder          []*acceptedInput
	controlReceipts     map[string]persistedControlReceipt
	lastControl         persistedSessionControl
	treeControl         persistedTreeControl
	recent              []agentevent.RunSummary
	cursor              agentevent.Cursor
	history             []agentevent.Event
	observers           map[uint64]*sessionObserver
	nextObserver        uint64
	taskCompletions     taskCompletionMailbox
	childTasksMu        sync.Mutex
}

func (session *Session) Key() agentsession.Key {
	if session == nil {
		return agentsession.Key{}
	}
	key := session.key
	key.Attributes = agentschema.CloneStringMap(key.Attributes)
	return key
}

func (session *Session) replay(ctx context.Context, access sessionAccess) error {
	if err := session.loadRecovery(ctx); err != nil {
		return err
	}
	if err := session.restoreRecent(ctx); err != nil {
		return err
	}
	var unfinished *persistedTurn
	apply := func(record agentsession.Record) error {
		session.revision = record.Revision
		switch record.Kind {
		case sessionInputRecord, sessionInputUpdateRecord:
			return session.replayInput(record)
		case turnCheckpointRecord:
			return session.replayCycle(record.Data)
		case sessionControlRecord:
			return session.replayControl(record.Data)
		case turnToolRecord:
			return session.replayTool(record.Data)
		case turnInteractionRecord, turnInteractionResponseRecord:
			return session.replayInteraction(record)
		case sessionTranscriptRecord:
			var transcript persistedSessionTranscript
			if err := json.Unmarshal(record.Data, &transcript); err != nil {
				return fmt.Errorf("decode Agent Session transcript at revision %d: %w", record.Revision, err)
			}
			session.engineState = append(json.RawMessage(nil), transcript.EngineState...)
		case sessionMessageCheckpointRecord:
			var checkpoint agentengine.PersistedMessageCheckpoint
			if err := json.Unmarshal(record.Data, &checkpoint); err != nil {
				return fmt.Errorf("decode Agent Session message checkpoint at revision %d: %w", record.Revision, err)
			}
			// An empty hash marks a checkpoint with no retained canonical
			// window; recovery rebuilds it from the canonical source.
			if checkpoint.MessageCount < 0 {
				return fmt.Errorf("decode Agent Session message checkpoint at revision %d: invalid checkpoint", record.Revision)
			}
			session.messageCheckpoint = checkpoint
		case sessionCapabilitySetRecord:
			var capability persistedCapability
			if err := json.Unmarshal(record.Data, &capability); err != nil {
				return fmt.Errorf("decode Agent Session capability set at revision %d: %w", record.Revision, err)
			}
			if strings.TrimSpace(capability.Capability) == "" || !json.Valid(capability.State) {
				return fmt.Errorf("decode Agent Session capability set at revision %d: invalid capability", record.Revision)
			}
			session.capabilities[capability.Capability] = append(json.RawMessage(nil), capability.State...)
			session.durableCapabilities[capability.Capability] = append(json.RawMessage(nil), capability.State...)
		case sessionCapabilityDeleteRecord:
			var capability persistedCapability
			if err := json.Unmarshal(record.Data, &capability); err != nil {
				return fmt.Errorf("decode Agent Session capability delete at revision %d: %w", record.Revision, err)
			}
			if strings.TrimSpace(capability.Capability) == "" {
				return fmt.Errorf("decode Agent Session capability delete at revision %d: invalid capability", record.Revision)
			}
			delete(session.capabilities, capability.Capability)
			delete(session.durableCapabilities, capability.Capability)
		case sessionChildTasksRecord:
			// Already reduced into the current obligation checkpoint.
		case sessionTaskCompletionDeliveryRecord:
			var delivery persistedTaskCompletionDelivery
			if err := json.Unmarshal(record.Data, &delivery); err != nil {
				return fmt.Errorf("decode Agent task completion delivery at revision %d: %w", record.Revision, err)
			}
			if len(delivery.IDs) == 0 {
				return fmt.Errorf("decode Agent task completion delivery at revision %d: empty delivery", record.Revision)
			}
			for _, id := range delivery.IDs {
				id = strings.TrimSpace(id)
				if id == "" || len(id) > maxTaskCompletionIDBytes {
					return fmt.Errorf("decode Agent task completion delivery at revision %d: invalid completion ID", record.Revision)
				}
				session.taskCompletions.delivered[id] = struct{}{}
			}
		case turnStartedRecord:
			var turn persistedTurn
			if err := json.Unmarshal(record.Data, &turn); err != nil {
				return fmt.Errorf("decode Agent turn start at revision %d: %w", record.Revision, err)
			}
			unfinished = &turn
			if run := session.runs[turn.RunID]; run != nil {
				session.active = run
				run.markStarted(turn.At)
				session.removePendingLocked(run)
			}
		case turnFinishedRecord, turnInterruptedRecord:
			var turn persistedTurn
			if err := json.Unmarshal(record.Data, &turn); err != nil {
				return fmt.Errorf("decode Agent turn settlement at revision %d: %w", record.Revision, err)
			}
			session.addRecentLocked(agentevent.RunSummary{ID: turn.RunID, CommandID: turn.CommandID, Status: turn.Status, Reason: turn.Reason, Output: turn.Output})
			if run := session.runs[turn.RunID]; run != nil {
				run.settled, run.result, run.finishedAt = true, agentschema.Result{Status: turn.Status, Reason: turn.Reason}, turn.At
				if turn.Status != agentschema.ResultCompleted && turn.Status != agentschema.ResultAborted {
					run.err = &agentevent.RunError{Result: run.result}
				}
				run.content.WriteString(turn.Output)
				run.cancel()
				run.endHandle()
				close(run.executionDone)
				session.removePendingLocked(run)
				if session.active == run {
					session.active = nil
				}
			}
			if unfinished != nil && unfinished.RunID == turn.RunID {
				unfinished = nil
			}
		default:
			return fmt.Errorf("unsupported Agent Session record %q", record.Kind)
		}
		return nil
	}
	for _, record := range session.recovery.ReplayRecords() {
		if err := apply(record); err != nil {
			return fmt.Errorf("replay Agent Session transcript: %w", err)
		}
	}
	session.revision = session.recovery.Revision
	if unfinished != nil {
		if run := session.runs[unfinished.RunID]; run != nil {
			run.result = agentschema.Result{Status: agentschema.ResultSuspended, Reason: "Agent Run requires explicit resume"}
			run.cancel()
			run.endHandle()
			close(run.executionDone)
			if access == sessionInspection {
				return nil
			}
			return run.restoreEffectInteractions()
		}
		interrupted := *unfinished
		interrupted.Status = agentschema.ResultIncomplete
		interrupted.Reason = "Agent process stopped before the turn finished"
		// Released journals have no accepted input to resume. Derive incomplete
		// without rewriting historical facts merely because a reader opened them.
		session.addRecentLocked(agentevent.RunSummary{
			ID: interrupted.RunID, CommandID: interrupted.CommandID,
			Status: interrupted.Status, Reason: interrupted.Reason,
		})
	}
	return nil
}

func (session *Session) Run(ctx context.Context, input agentschema.Input) (*Run, error) {
	return session.start(ctx, input, runUsesSession)
}

func (session *Session) start(ctx context.Context, input agentschema.Input, ownership runSessionOwnership) (*Run, error) {
	_, run, err := session.receiveInput(ctx, input, inputRun, "", ownership)
	if err == nil && run == nil {
		return nil, agentschema.ErrRunSettled
	}
	return run, err
}

func (session *Session) Active(_ context.Context) (*Run, bool, error) {
	if err := session.usable(); err != nil {
		return nil, false, err
	}
	session.mu.RLock()
	defer session.mu.RUnlock()
	return session.active, session.active != nil, nil
}

func (session *Session) AttachRun(ctx context.Context, runID string) (*Run, bool, error) {
	if err := session.usable(); err != nil {
		return nil, false, err
	}
	session.mu.RLock()
	run := session.runs[strings.TrimSpace(runID)]
	session.mu.RUnlock()
	if run != nil {
		return run, true, nil
	}
	snapshot, found, err := session.RunSnapshot(ctx, runID)
	if err != nil || !found {
		return nil, found, err
	}
	return session.settledHandle(snapshot), true, nil
}

func (session *Session) RunInput(ctx context.Context, runID string) (agentschema.Input, bool, error) {
	run, found, err := session.AttachRun(ctx, runID)
	if err != nil || !found {
		return agentschema.Input{}, false, err
	}
	run.mu.RLock()
	// The cycle counter advances before admission/checkpointing completes.
	// Until a snapshot exists, the accepted Run input remains authoritative.
	if run.snapshot.Cycle > 0 && !run.settled {
		saved := run.snapshot.Input
		commandID := string(run.snapshot.CommandID)
		run.mu.RUnlock()
		input, err := agentengine.DecodeInput(saved)
		input.IdempotencyKey = commandID
		return input, err == nil, err
	}
	if !run.settled {
		input := cloneInput(run.input)
		run.mu.RUnlock()
		return input, true, nil
	}
	run.mu.RUnlock()
	session.mu.RLock()
	entry, found := session.recovery.Inputs[run.commandID]
	session.mu.RUnlock()
	if !found {
		return agentschema.Input{}, false, nil
	}
	record, err := session.readRecord(ctx, entry.Revision)
	if err != nil {
		return agentschema.Input{}, false, err
	}
	var stored persistedInput
	if err := json.Unmarshal(record.Data, &stored); err != nil {
		return agentschema.Input{}, false, err
	}
	input, err := agentengine.DecodeInput(stored.Input)
	input.IdempotencyKey = stored.Receipt.CommandID
	return input, err == nil, err
}

func (session *Session) Observe(ctx context.Context, after agentevent.Cursor) (agentevent.Observation, error) {
	if err := session.usable(); err != nil {
		return agentevent.Observation{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	session.mu.Lock()
	snapshot := session.snapshotLocked()
	bufferSize := len(session.history) + 256
	events := make(chan agentevent.Event, bufferSize)
	errorsChannel := make(chan error, 1)
	for _, event := range session.history {
		if event.Cursor > after {
			events <- event
		}
	}
	session.nextObserver++
	id := session.nextObserver
	session.observers[id] = &sessionObserver{events: events, errors: errorsChannel}
	session.mu.Unlock()
	if done := ctx.Done(); done != nil {
		agentasync.SafeGo(func() {
			<-done
			session.mu.Lock()
			if observer := session.observers[id]; observer != nil {
				delete(session.observers, id)
				close(observer.events)
				close(observer.errors)
			}
			session.mu.Unlock()
		}, func(error) {})
	}
	return agentevent.Observation{Snapshot: snapshot, Events: events, Errors: errorsChannel}, nil
}

func (session *Session) Snapshot(_ context.Context) (agentevent.SessionSnapshot, error) {
	if err := session.usable(); err != nil {
		return agentevent.SessionSnapshot{}, err
	}
	session.mu.RLock()
	defer session.mu.RUnlock()
	return session.snapshotLocked(), nil
}

func (session *Session) snapshotLocked() agentevent.SessionSnapshot {
	snapshot := agentevent.SessionSnapshot{Key: session.Key(), Cursor: session.cursor, RetentionStart: 1, ChildTasksInitialized: session.recovery.Children.Initialized}
	for _, child := range session.recovery.Children.values() {
		snapshot.ChildTasks = append(snapshot.ChildTasks, agentevent.ChildTaskSnapshot{Key: child.Key})
	}
	if len(session.history) > 0 {
		snapshot.RetentionStart = session.history[0].Cursor
	} else {
		snapshot.RetentionStart = session.cursor + 1
	}
	if session.active != nil {
		snapshot.ActiveRunID = session.active.id
		snapshot.ActiveCommandID = session.active.commandID
		snapshot.ActiveAbortPending = session.active.abortRequested()
		if session.active.isSuspended() {
			snapshot.ActiveStatus = agentschema.ResultSuspended
		}
		snapshot.ActiveReceiptCursor = session.active.Receipt().Cursor
		snapshot.ActiveCycle = session.active.cycleValue()
		snapshot.ActiveOutput = session.active.outputSnapshot()
		snapshot.PendingInteractions = session.active.pendingInteractionRequests()
		snapshot.OpenTools = session.active.openToolSnapshots()
	}
	snapshot.QueuedRuns = session.queuedSnapshotsLocked()
	for _, pending := range session.pending {
		snapshot.QueuedRuns = append(snapshot.QueuedRuns, agentevent.QueuedRunSnapshot{
			ID: pending.id, CommandID: pending.commandID,
			ReceiptCursor: pending.Receipt().Cursor, Delivery: agentevent.DeliveryNextTurn, Text: pending.input.Text,
		})
	}
	snapshot.RecentRuns = append([]agentevent.RunSummary(nil), session.recent...)
	if raw, ok := session.capabilities[agentgoal.GoalCapability]; ok {
		if state, err := agentgoal.DecodeGoalState(raw); err == nil {
			snapshot.Goal = &state
		}
	}
	if raw, ok := session.capabilities[agentexecution.TodoCapability]; ok {
		var state agentevent.TodoState
		if json.Unmarshal(raw, &state) == nil {
			snapshot.Todo = &state
		}
	}
	clear, clearPresent, _ := agenthistory.ClearStateFrom(session.capabilities)
	compaction, compactionPresent, _ := agenthistory.CompactionStateFrom(session.capabilities)
	compaction, compactionPresent = agenthistory.ClearCompaction(compaction, compactionPresent, clear, clearPresent)
	if compactionPresent && !compaction.Removed {
		snapshot.Compaction = agenthistory.CompactionStatePointer(compaction, true)
	}
	if clearPresent {
		snapshot.ClearRevision = clear.Revision
	}
	return snapshot
}

func (session *Session) Goal(_ context.Context) (agentgoal.GoalState, bool, error) {
	if err := session.usable(); err != nil {
		return agentgoal.GoalState{}, false, err
	}
	session.mu.RLock()
	raw, present := session.capabilities[agentgoal.GoalCapability]
	session.mu.RUnlock()
	if !present {
		return agentgoal.GoalState{}, false, nil
	}
	state, err := agentgoal.DecodeGoalState(raw)
	return state, err == nil && state.Visible(), err
}

func (session *Session) UpdateGoal(ctx context.Context, mutation agentschema.GoalMutation) (agentgoal.GoalState, error) {
	if err := session.usable(); err != nil {
		return agentgoal.GoalState{}, err
	}
	if mutation.MutationID == "" {
		mutation.MutationID = newPublicID("goal-mutation")
	}
	definition, err := session.agent.source.Prepare(ctx, agentengine.PrepareRequest{
		Session: agentschema.SessionView{Key: session.key}, Input: agentschema.Input{Goal: agentschema.CloneGoalMutation(&mutation)},
		Reason: agentengine.TurnReasonGoalMutation,
	})
	if err != nil {
		return agentgoal.GoalState{}, fmt.Errorf("prepare Goal Manager: %w", err)
	}
	if definition.Goal == nil {
		return agentgoal.GoalState{}, agentschema.ErrCapabilityUnsupported
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	raw, present := session.capabilities[agentgoal.GoalCapability]
	var current agentgoal.GoalState
	if present {
		current, err = agentgoal.DecodeGoalState(raw)
		if err != nil {
			return agentgoal.GoalState{}, err
		}
	}
	next, err := agentgoal.ApplyGoalMutation(ctx, definition.Goal, agentgoal.GoalApplyRequest{
		Session: agentschema.SessionView{Key: session.key}, Current: current, Present: present, Mutation: mutation,
	})
	if err != nil {
		return agentgoal.GoalState{}, err
	}
	encoded, err := json.Marshal(next)
	if err != nil {
		return agentgoal.GoalState{}, err
	}
	session.capabilities[agentgoal.GoalCapability] = encoded
	if err := session.persistCapabilitiesLocked(ctx); err != nil {
		return agentgoal.GoalState{}, err
	}
	session.publishLocked(agentevent.Event{RunID: "", Payload: agentevent.GoalUpdated{State: next, Present: next.Visible()}})
	return next, nil
}

func (session *Session) Close(_ context.Context) error {
	return session.closeForTree("")
}

func (session *Session) closeForTree(releaseTreeID string) error {
	if session == nil {
		return nil
	}
	session.mu.Lock()
	if session.closed {
		err := session.closeErr
		session.mu.Unlock()
		return err
	}
	if session.closing {
		done := session.closeDone
		session.mu.Unlock()
		<-done
		session.mu.RLock()
		err := session.closeErr
		session.mu.RUnlock()
		return err
	}
	session.closing, session.closeDone = true, make(chan struct{})
	active := session.active
	pending := append([]*Run(nil), session.pending...)
	session.mu.Unlock()
	if active != nil {
		if !active.isSuspended() {
			active.abort("Agent Session closed")
			<-active.executionDone
		}
		// The admission fence may suspend execution while Close is waiting.
		// Settle that handle too; finish leaves an already settled result intact.
		active.finish(agentschema.Result{Status: agentschema.ResultAborted, Reason: "Agent Session closed"}, nil)
	}
	for _, run := range pending {
		run.finish(agentschema.Result{Status: agentschema.ResultAborted, Reason: "Agent Session closed"}, nil)
	}
	var err error
	if releaseTreeID != "" {
		_, err = session.acceptTreeControl(context.Background(), "release_tree", releaseTreeID, releaseTreeID+":release", "")
	}
	err = errors.Join(err, session.closeWriter())
	session.mu.Lock()
	session.closeErr = errors.Join(session.storageErr, err)
	close(session.closeDone)
	err = session.closeErr
	session.mu.Unlock()
	return err
}

func (session *Session) usable() error {
	if session == nil || session.agent == nil || session.engine == nil || session.log == nil {
		return agentschema.ErrSessionClosed
	}
	session.mu.RLock()
	closed := session.closed
	session.mu.RUnlock()
	if closed {
		return agentschema.ErrSessionClosed
	}
	return nil
}

func (session *Session) appendRecordLocked(ctx context.Context, kind string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode Agent Session %s: %w", kind, err)
	}
	return session.appendRecordsLocked(ctx, agentsession.Record{
		Kind: kind, Version: sessionRecordVersion, Data: data,
	})

}

func (session *Session) appendRecordsLocked(ctx context.Context, records ...agentsession.Record) error {
	if session.storageErr != nil {
		return session.storageErr
	}
	// Persist the empty child set with a new journal's first transaction. A
	// cold reopen must distinguish new Sessions from released, unadopted ones.
	if session.revision == 0 && session.recovery.Children.Initialized && len(records) > 0 && records[0].Kind != sessionChildTasksRecord {
		initialized, err := sessionRecord(sessionChildTasksRecord, session.recovery.Children)
		if err != nil {
			return err
		}
		records = append([]agentsession.Record{initialized}, records...)
	}
	next, err := session.log.Append(ctx, session.revision, records...)
	if err != nil {
		err = fmt.Errorf("append Agent Session records: %w", err)
		if errors.Is(err, agentsession.ErrCommitUnknown) || (!errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)) {
			session.storageErr = err
		}
		return err
	}
	if err := session.recordCommittedLocked(records, session.revision+1); err != nil {
		return err
	}
	session.revision = next
	return nil
}

func (session *Session) persistTranscriptLocked(ctx context.Context) error {
	if session.canonicalMessages {
		state, checkpoint, err := agentengine.ProjectCheckpoint(session.engineState, session.capabilities)
		if err != nil {
			return err
		}
		session.engineState = state
		previous, _ := json.Marshal(session.messageCheckpoint)
		current, _ := json.Marshal(checkpoint)
		if bytes.Equal(previous, current) {
			return nil
		}
		if err := session.appendRecordLocked(ctx, sessionMessageCheckpointRecord, checkpoint); err != nil {
			return err
		}
		session.messageCheckpoint = checkpoint
		return nil
	}
	return session.appendRecordLocked(ctx, sessionTranscriptRecord, persistedSessionTranscript{
		EngineState: append(json.RawMessage(nil), session.engineState...),
	})
}

func (session *Session) persistCapabilitiesLocked(ctx context.Context) error {
	keys := make(map[string]struct{}, len(session.capabilities)+len(session.durableCapabilities))
	for capability := range session.capabilities {
		keys[capability] = struct{}{}
	}
	for capability := range session.durableCapabilities {
		keys[capability] = struct{}{}
	}
	ordered := make([]string, 0, len(keys))
	for capability := range keys {
		ordered = append(ordered, capability)
	}
	sort.Strings(ordered)
	records := make([]agentsession.Record, 0, len(ordered))
	for _, capability := range ordered {
		current, present := session.capabilities[capability]
		durable, persisted := session.durableCapabilities[capability]
		if present && persisted && bytes.Equal(current, durable) {
			continue
		}
		value := persistedCapability{Capability: capability}
		kind := sessionCapabilityDeleteRecord
		if present {
			kind = sessionCapabilitySetRecord
			value.State = append(json.RawMessage(nil), current...)
		}
		data, err := json.Marshal(value)
		if err != nil {
			return fmt.Errorf("encode Agent Session capability %q: %w", capability, err)
		}
		records = append(records, agentsession.Record{Kind: kind, Version: sessionRecordVersion, Data: data})
	}
	if len(records) == 0 {
		return nil
	}
	if err := session.appendRecordsLocked(ctx, records...); err != nil {
		return err
	}
	session.durableCapabilities = agentschema.CloneRawStateMap(session.capabilities)
	return nil
}

func (session *Session) publishLocked(event agentevent.Event) {
	session.cursor++
	event.Cursor = session.cursor
	session.history = append(session.history, event)
	if len(session.history) > retainedSessionEvents {
		session.history = append([]agentevent.Event(nil), session.history[len(session.history)-retainedSessionEvents:]...)
	}
	for _, observer := range session.observers {
		publishLatestEvent(observer.events, event, &observer.drops)
	}
}

func (session *Session) addRecentLocked(summary agentevent.RunSummary) {
	session.recent = append(session.recent, summary)
	if len(session.recent) > 32 {
		session.recent = append([]agentevent.RunSummary(nil), session.recent[len(session.recent)-32:]...)
	}
}

func agentsessionCanonical(key agentsession.Key) (string, error) {
	return agentsession.CanonicalKey(key)
}

func cloneInput(input agentschema.Input) agentschema.Input {
	input.Context = append([]agentschema.ContextFragment(nil), input.Context...)
	input.Attachments = agentschema.CloneAttachments(input.Attachments)
	input.Goal = agentschema.CloneGoalMutation(input.Goal)
	input.HostData = agentschema.CloneHostData(input.HostData)
	return input
}
