package agent

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

	runstate "github.com/alfredxw/denova/agent/internal/runstate"
	agentsession "github.com/alfredxw/denova/agent/session"
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

type inputEnvelope struct {
	Version     uint16            `json:"version"`
	Context     []ContextFragment `json:"context,omitempty"`
	Attachments []Attachment      `json:"attachments,omitempty"`
	Goal        *GoalMutation     `json:"goal,omitempty"`
	HostData    *HostData         `json:"host_data,omitempty"`
}

type persistedSessionTranscript struct {
	EngineState json.RawMessage `json:"engine_state,omitempty"`
}

type persistedMessageCheckpoint struct {
	Hash         string `json:"hash"`
	MessageCount int    `json:"message_count"`
	// Metadata never duplicates committed product messages. Pending contains
	// only a tool batch that has not yet reached the product commit boundary.
	Metadata json.RawMessage `json:"metadata,omitempty"`
	Pending  []*Message      `json:"pending,omitempty"`
}

type persistedCapability struct {
	Capability string          `json:"capability"`
	State      json.RawMessage `json:"state,omitempty"`
}

type persistedTaskCompletionDelivery struct {
	IDs []string `json:"ids"`
}

type persistedTurn struct {
	RunID     string       `json:"run_id"`
	CommandID string       `json:"command_id"`
	Status    ResultStatus `json:"status,omitempty"`
	Reason    string       `json:"reason,omitempty"`
	Output    string       `json:"output,omitempty"`
	At        time.Time    `json:"at"`
}

type sessionObserver struct {
	events chan Event
	errors chan error
	drops  eventDropState
}

// Session serializes Runs for one conversation. Canonical messages, versioned
// capability updates, and settled turn records may cross process boundaries;
// all live coordination is intentionally kept here in memory.
type Session struct {
	agent   *Agent
	key     SessionKey
	binding runstate.BindingRef
	engine  runstate.Engine
	log     agentsession.Log

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
	messageCheckpoint   persistedMessageCheckpoint
	active              *Run
	maintenance         bool
	pending             []*Run
	runs                map[string]*Run
	inputs              map[string]*acceptedInput
	inputOrder          []*acceptedInput
	controlReceipts     map[string]persistedControlReceipt
	lastControl         persistedSessionControl
	treeControl         persistedTreeControl
	recent              []RunSummary
	cursor              Cursor
	history             []Event
	observers           map[uint64]*sessionObserver
	nextObserver        uint64
	taskCompletions     taskCompletionMailbox
}

func (session *Session) Key() SessionKey {
	if session == nil {
		return SessionKey{}
	}
	key := session.key
	key.Attributes = cloneStringMap(key.Attributes)
	return key
}

func (session *Session) replay(ctx context.Context) error {
	var unfinished *persistedTurn
	stats, err := session.log.Replay(ctx, func(record agentsession.Record) error {
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
			var checkpoint persistedMessageCheckpoint
			if err := json.Unmarshal(record.Data, &checkpoint); err != nil {
				return fmt.Errorf("decode Agent Session message checkpoint at revision %d: %w", record.Revision, err)
			}
			if checkpoint.Hash == "" || checkpoint.MessageCount < 0 {
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
			session.addRecentLocked(RunSummary{ID: turn.RunID, CommandID: turn.CommandID, Status: turn.Status, Reason: turn.Reason, Output: turn.Output})
			if run := session.runs[turn.RunID]; run != nil {
				run.settled, run.result = true, Result{Status: turn.Status, Reason: turn.Reason}
				if turn.Status != ResultCompleted && turn.Status != ResultAborted {
					run.err = &RunError{Result: run.result}
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
	})
	if err != nil {
		return fmt.Errorf("replay Agent Session transcript: %w", err)
	}
	_ = stats
	if unfinished != nil {
		if run := session.runs[unfinished.RunID]; run != nil {
			run.result = Result{Status: ResultSuspended, Reason: "Agent Run requires explicit resume"}
			run.cancel()
			run.endHandle()
			close(run.executionDone)
			return run.restoreEffectInteractions()
		}
		interrupted := *unfinished
		interrupted.Status = ResultIncomplete
		interrupted.Reason = "Agent process stopped before the turn finished"
		// Released journals have no accepted input to resume. Derive incomplete
		// without rewriting historical facts merely because a reader opened them.
		session.addRecentLocked(RunSummary{
			ID: interrupted.RunID, CommandID: interrupted.CommandID,
			Status: interrupted.Status, Reason: interrupted.Reason,
		})
	}
	return nil
}

func (session *Session) Run(ctx context.Context, input Input) (*Run, error) {
	return session.start(ctx, input, runUsesSession)
}

func (session *Session) start(ctx context.Context, input Input, ownership runSessionOwnership) (*Run, error) {
	_, run, err := session.receiveInput(ctx, input, inputRun, "", ownership)
	if err == nil && run == nil {
		return nil, ErrRunSettled
	}
	return run, err
}
func encodeInput(input Input) (json.RawMessage, runstate.UserInput, error) {
	if strings.TrimSpace(input.Text) == "" && len(input.Attachments) == 0 {
		return nil, runstate.UserInput{}, errors.New("Agent Input requires Text or Attachments")
	}
	if input.HostData != nil {
		if strings.TrimSpace(input.HostData.Type) == "" || input.HostData.Version == 0 || !json.Valid(input.HostData.Data) {
			return nil, runstate.UserInput{}, errors.New("Agent Input HostData requires Type, Version, and valid JSON Data")
		}
	}
	if err := validateContextFragments(input.Context); err != nil {
		return nil, runstate.UserInput{}, err
	}
	envelope := inputEnvelope{
		Version: 1, Context: append([]ContextFragment(nil), input.Context...),
		Attachments: cloneAttachments(input.Attachments),
		Goal:        cloneGoalMutation(input.Goal), HostData: cloneHostData(input.HostData),
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return nil, runstate.UserInput{}, fmt.Errorf("encode Agent Input: %w", err)
	}
	references := make([]runstate.ContextRef, 0, len(input.Context))
	for _, fragment := range input.Context {
		references = append(references, runstate.ContextRef{
			Source: fragment.Source, Resource: fragment.Resource, Revision: fragment.Revision,
			Selector: string(fragment.Placement), ByteLimit: fragment.HardLimit,
		})
	}
	return encoded, runstate.UserInput{Text: input.Text, ContextRefs: references, Envelope: encoded}, nil
}

func decodeInput(input runstate.UserInput) (Input, error) {
	result := Input{Text: input.Text}
	if len(input.Envelope) == 0 {
		return result, nil
	}
	var envelope inputEnvelope
	if err := json.Unmarshal(input.Envelope, &envelope); err != nil {
		return Input{}, fmt.Errorf("decode Agent Input: %w", err)
	}
	if envelope.Version != 1 {
		return Input{}, fmt.Errorf("unsupported Agent Input version %d", envelope.Version)
	}
	result.Context = append([]ContextFragment(nil), envelope.Context...)
	result.Attachments = cloneAttachments(envelope.Attachments)
	result.Goal = cloneGoalMutation(envelope.Goal)
	result.HostData = cloneHostData(envelope.HostData)
	return result, nil
}

func cloneGoalMutation(mutation *GoalMutation) *GoalMutation {
	if mutation == nil {
		return nil
	}
	cloned := *mutation
	cloned.Data = append(json.RawMessage(nil), mutation.Data...)
	return &cloned
}

func cloneHostData(data *HostData) *HostData {
	if data == nil {
		return nil
	}
	cloned := *data
	cloned.Data = append(json.RawMessage(nil), data.Data...)
	return &cloned
}

func (session *Session) Active(_ context.Context) (*Run, bool, error) {
	if err := session.usable(); err != nil {
		return nil, false, err
	}
	session.mu.RLock()
	defer session.mu.RUnlock()
	return session.active, session.active != nil, nil
}

func (session *Session) AttachRun(_ context.Context, runID string) (*Run, bool, error) {
	if err := session.usable(); err != nil {
		return nil, false, err
	}
	session.mu.RLock()
	run := session.runs[strings.TrimSpace(runID)]
	session.mu.RUnlock()
	return run, run != nil, nil
}

func (session *Session) RunInput(_ context.Context, runID string) (Input, bool, error) {
	run, found, err := session.AttachRun(context.Background(), runID)
	if err != nil || !found {
		return Input{}, false, err
	}
	run.mu.RLock()
	defer run.mu.RUnlock()
	if run.cycle > 0 && !run.settled {
		input, err := decodeInput(run.snapshot.Input)
		input.IdempotencyKey = string(run.snapshot.CommandID)
		return input, err == nil, err
	}
	return cloneInput(run.input), true, nil
}

func (session *Session) Observe(ctx context.Context, after Cursor) (Observation, error) {
	if err := session.usable(); err != nil {
		return Observation{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	session.mu.Lock()
	snapshot := session.snapshotLocked()
	bufferSize := len(session.history) + 256
	events := make(chan Event, bufferSize)
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
		safeGo(func() {
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
	return Observation{Snapshot: snapshot, Events: events, Errors: errorsChannel}, nil
}

func (session *Session) Snapshot(_ context.Context) (SessionSnapshot, error) {
	if err := session.usable(); err != nil {
		return SessionSnapshot{}, err
	}
	session.mu.RLock()
	defer session.mu.RUnlock()
	return session.snapshotLocked(), nil
}

func (session *Session) snapshotLocked() SessionSnapshot {
	snapshot := SessionSnapshot{Key: session.Key(), Cursor: session.cursor, RetentionStart: 1}
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
			snapshot.ActiveStatus = ResultSuspended
		}
		snapshot.ActiveReceiptCursor = session.active.Receipt().Cursor
		snapshot.ActiveCycle = session.active.cycleValue()
		snapshot.ActiveOutput = session.active.outputSnapshot()
		snapshot.PendingInteractions = session.active.pendingInteractionRequests()
		snapshot.OpenTools = session.active.openToolSnapshots()
	}
	snapshot.QueuedRuns = session.queuedSnapshotsLocked()
	for _, pending := range session.pending {
		snapshot.QueuedRuns = append(snapshot.QueuedRuns, QueuedRunSnapshot{
			ID: pending.id, CommandID: pending.commandID,
			ReceiptCursor: pending.Receipt().Cursor, Delivery: DeliveryNextTurn, Text: pending.input.Text,
		})
	}
	snapshot.RecentRuns = append([]RunSummary(nil), session.recent...)
	if raw, ok := session.capabilities[goalCapability]; ok {
		if state, err := decodeGoalState(raw); err == nil {
			snapshot.Goal = &state
		}
	}
	if raw, ok := session.capabilities[TodoCapability]; ok {
		var state TodoState
		if json.Unmarshal(raw, &state) == nil {
			snapshot.Todo = &state
		}
	}
	clear, clearPresent, _ := clearStateFrom(session.capabilities)
	compaction, compactionPresent, _ := compactionStateFrom(session.capabilities)
	compaction, compactionPresent = clearCompaction(compaction, compactionPresent, clear, clearPresent)
	if compactionPresent && !compaction.Removed {
		snapshot.Compaction = compactionStatePointer(compaction, true)
	}
	if clearPresent {
		snapshot.ClearRevision = clear.Revision
	}
	return snapshot
}

func (session *Session) Goal(_ context.Context) (GoalState, bool, error) {
	if err := session.usable(); err != nil {
		return GoalState{}, false, err
	}
	session.mu.RLock()
	raw, present := session.capabilities[goalCapability]
	session.mu.RUnlock()
	if !present {
		return GoalState{}, false, nil
	}
	state, err := decodeGoalState(raw)
	return state, err == nil && state.Visible(), err
}

func (session *Session) UpdateGoal(ctx context.Context, mutation GoalMutation) (GoalState, error) {
	if err := session.usable(); err != nil {
		return GoalState{}, err
	}
	if mutation.MutationID == "" {
		mutation.MutationID = newPublicID("goal-mutation")
	}
	definition, err := session.agent.source.Prepare(ctx, PrepareRequest{
		Session: SessionView{Key: session.key}, Input: Input{Goal: cloneGoalMutation(&mutation)},
		Reason: TurnReasonGoalMutation,
	})
	if err != nil {
		return GoalState{}, fmt.Errorf("prepare Goal Manager: %w", err)
	}
	if definition.Goal == nil {
		return GoalState{}, ErrCapabilityUnsupported
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	raw, present := session.capabilities[goalCapability]
	var current GoalState
	if present {
		current, err = decodeGoalState(raw)
		if err != nil {
			return GoalState{}, err
		}
	}
	next, err := applyGoalMutation(ctx, definition.Goal, GoalApplyRequest{
		Session: SessionView{Key: session.key}, Current: current, Present: present, Mutation: mutation,
	})
	if err != nil {
		return GoalState{}, err
	}
	encoded, err := json.Marshal(next)
	if err != nil {
		return GoalState{}, err
	}
	session.capabilities[goalCapability] = encoded
	if err := session.persistCapabilitiesLocked(ctx); err != nil {
		return GoalState{}, err
	}
	session.publishLocked(Event{RunID: "", Payload: GoalUpdated{State: next, Present: next.Visible()}})
	return next, nil
}

func decodeGoalState(encoded json.RawMessage) (GoalState, error) {
	var state GoalState
	if err := json.Unmarshal(encoded, &state); err != nil {
		return GoalState{}, fmt.Errorf("decode Goal state: %w", err)
	}
	if state.Revision == 0 || state.Status == "" {
		return GoalState{}, errors.New("Agent Goal state is invalid")
	}
	return state, nil
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
		if active.isSuspended() {
			active.finish(Result{Status: ResultAborted, Reason: "Agent Session closed"}, nil)
		} else {
			active.abort("Agent Session closed")
			<-active.executionDone
		}
	}
	for _, run := range pending {
		run.finish(Result{Status: ResultAborted, Reason: "Agent Session closed"}, nil)
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
		return ErrSessionClosed
	}
	session.mu.RLock()
	closed := session.closed
	session.mu.RUnlock()
	if closed {
		return ErrSessionClosed
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
	next, err := session.log.Append(ctx, session.revision, records...)
	if err != nil {
		err = fmt.Errorf("append Agent Session records: %w", err)
		if errors.Is(err, agentsession.ErrCommitUnknown) || (!errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)) {
			session.storageErr = err
		}
		return err
	}
	session.revision = next
	return nil
}

func (session *Session) persistTranscriptLocked(ctx context.Context) error {
	if session.canonicalMessages {
		checkpoint, err := canonicalMessageCheckpoint(session.engineState)
		if err != nil {
			return err
		}
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
	session.durableCapabilities = cloneRawStateMap(session.capabilities)
	return nil
}

func (session *Session) publishLocked(event Event) {
	session.cursor++
	event.Cursor = session.cursor
	session.history = append(session.history, event)
	if len(session.history) > retainedSessionEvents {
		session.history = append([]Event(nil), session.history[len(session.history)-retainedSessionEvents:]...)
	}
	for _, observer := range session.observers {
		publishLatestEvent(observer.events, event, &observer.drops)
	}
}

func (session *Session) nextCommandCursor() Cursor {
	session.mu.Lock()
	defer session.mu.Unlock()
	return session.nextCommandCursorLocked()
}

func (session *Session) nextCommandCursorLocked() Cursor {
	session.cursor++
	return session.cursor
}

func (session *Session) addRecentLocked(summary RunSummary) {
	session.recent = append(session.recent, summary)
	if len(session.recent) > 32 {
		session.recent = append([]RunSummary(nil), session.recent[len(session.recent)-32:]...)
	}
}

func agentsessionCanonical(key SessionKey) (string, error) { return agentsession.CanonicalKey(key) }

func cloneStringMap(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	result := make(map[string]string, len(input))
	for key, value := range input {
		result[key] = value
	}
	return result
}

func cloneInput(input Input) Input {
	input.Context = append([]ContextFragment(nil), input.Context...)
	input.Attachments = cloneAttachments(input.Attachments)
	input.Goal = cloneGoalMutation(input.Goal)
	input.HostData = cloneHostData(input.HostData)
	return input
}
