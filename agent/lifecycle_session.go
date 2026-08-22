package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	runstate "github.com/alfredxw/denova/agent/internal/runstate"
	agentsession "github.com/alfredxw/denova/agent/session"
)

const (
	sessionTranscriptRecord   = "session.transcript"
	sessionCapabilitiesRecord = "session.capabilities"
	turnStartedRecord         = "turn.started"
	turnFinishedRecord        = "turn.finished"
	turnInterruptedRecord     = "turn.interrupted"
	sessionRecordVersion      = 1
	retainedSessionEvents     = 4096
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

type persistedSessionCapabilities struct {
	Capabilities map[string]json.RawMessage `json:"capabilities,omitempty"`
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

// Session serializes Runs for one conversation. Only transcript and
// capability snapshots cross process boundaries; all live coordination is
// intentionally kept here in memory.
type Session struct {
	agent   *Agent
	key     SessionKey
	binding runstate.BindingRef
	engine  runstate.Engine
	log     agentsession.Log

	mu           sync.RWMutex
	closed       bool
	revision     agentsession.Revision
	engineState  json.RawMessage
	capabilities map[string]json.RawMessage
	active       *Run
	maintenance  bool
	pending      []*Run
	runs         map[string]*Run
	recent       []RunSummary
	cursor       Cursor
	history      []Event
	observers    map[uint64]*sessionObserver
	nextObserver uint64
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
		case sessionTranscriptRecord:
			var transcript persistedSessionTranscript
			if err := json.Unmarshal(record.Data, &transcript); err != nil {
				return fmt.Errorf("decode Agent Session transcript at revision %d: %w", record.Revision, err)
			}
			session.engineState = append(json.RawMessage(nil), transcript.EngineState...)
		case sessionCapabilitiesRecord:
			var capabilities persistedSessionCapabilities
			if err := json.Unmarshal(record.Data, &capabilities); err != nil {
				return fmt.Errorf("decode Agent Session capabilities at revision %d: %w", record.Revision, err)
			}
			session.capabilities = cloneRawStateMap(capabilities.Capabilities)
		case turnStartedRecord:
			var turn persistedTurn
			if err := json.Unmarshal(record.Data, &turn); err != nil {
				return fmt.Errorf("decode Agent turn start at revision %d: %w", record.Revision, err)
			}
			unfinished = &turn
		case turnFinishedRecord, turnInterruptedRecord:
			var turn persistedTurn
			if err := json.Unmarshal(record.Data, &turn); err != nil {
				return fmt.Errorf("decode Agent turn settlement at revision %d: %w", record.Revision, err)
			}
			session.addRecentLocked(RunSummary{ID: turn.RunID, CommandID: turn.CommandID, Status: turn.Status, Reason: turn.Reason, Output: turn.Output})
			unfinished = nil
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
		interrupted := *unfinished
		interrupted.Status = ResultIncomplete
		interrupted.Reason = "Agent process stopped before the turn finished"
		interrupted.At = time.Now().UTC()
		if err := session.appendRecordLocked(ctx, turnInterruptedRecord, interrupted); err != nil {
			return err
		}
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
	if err := session.usable(); err != nil {
		return nil, err
	}
	if input.Goal != nil {
		input.Goal = cloneGoalMutation(input.Goal)
		if input.Goal.MutationID == "" {
			input.Goal.MutationID = newPublicID("goal-mutation")
		}
	}
	if _, _, err := encodeInput(input); err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	commandID := strings.TrimSpace(input.IdempotencyKey)
	if commandID == "" {
		commandID = newPublicID("command")
	}
	runID, err := session.agent.nextRunID(session.key)
	if err != nil {
		return nil, err
	}
	run := newPublicRun(session, runID, commandID, input, runstate.DeliveryStart, ownership)

	session.mu.Lock()
	if session.closed {
		session.mu.Unlock()
		return nil, ErrSessionClosed
	}
	if session.active != nil || session.maintenance {
		session.mu.Unlock()
		return nil, ErrSessionBusy
	}
	startedAt := time.Now().UTC()
	run.markStarted(startedAt)
	if err := session.appendRecordLocked(ctx, turnStartedRecord, persistedTurn{
		RunID: runID, CommandID: commandID, At: startedAt,
	}); err != nil {
		session.mu.Unlock()
		return nil, err
	}
	session.active = run
	session.runs[runID] = run
	run.receipt = session.nextCommandCursorLocked()
	session.mu.Unlock()

	run.publish(RunAccepted{CommandID: commandID})
	emitTrace(session.agent.ctx, session.agent.trace, TraceEvent{
		Kind: TraceRunAccepted, Session: session.key, RunID: runID,
	})
	safeGo(run.execute, func(err error) {
		run.finish(Result{Status: ResultFailed, Reason: err.Error()}, err)
	})
	return run, nil
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
	}
	if session.active != nil {
		snapshot.ActiveRunID = session.active.id
		snapshot.ActiveCommandID = session.active.commandID
		snapshot.ActiveReceiptCursor = session.active.Receipt().Cursor
		snapshot.ActiveCycle = session.active.cycleValue()
		snapshot.ActiveOutput = session.active.outputSnapshot()
		snapshot.PendingInteractions = session.active.pendingInteractionRequests()
		snapshot.QueuedRuns = session.active.queuedSnapshots()
		snapshot.OpenTools = session.active.openToolSnapshots()
	}
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
	compaction, compactionPresent, _, _ := compactionStateFrom(session.capabilities)
	compaction, compactionPresent = clearCompaction(compaction, compactionPresent, clear, clearPresent)
	cleanup, cleanupPresent, _, _ := cleanupStateFrom(session.capabilities)
	cleanup, cleanupPresent = clearCleanup(cleanup, cleanupPresent, clear, clearPresent)
	cleanup, cleanupPresent = cleanupAfterCompaction(cleanup, cleanupPresent, compaction, compactionPresent)
	if cleanupPresent && !cleanup.Removed {
		snapshot.Cleanup = &cleanup
	}
	if compactionPresent && !compaction.Removed {
		snapshot.Compaction = &compaction
	}
	if state, ok, _ := transcriptSyncStateFrom(session.capabilities); ok {
		snapshot.TranscriptSync = &state
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
	if session == nil {
		return nil
	}
	session.mu.Lock()
	if session.closed {
		session.mu.Unlock()
		return nil
	}
	session.closed = true
	active := session.active
	pending := append([]*Run(nil), session.pending...)
	session.pending = nil
	for id, observer := range session.observers {
		delete(session.observers, id)
		close(observer.events)
		close(observer.errors)
	}
	session.mu.Unlock()
	if active != nil {
		active.abort("Agent Session closed")
		<-active.done
	}
	for _, run := range pending {
		run.finish(Result{Status: ResultAborted, Reason: "Agent Session closed"}, nil)
	}
	err := session.log.Close()
	canonical, _ := agentsession.CanonicalKey(session.key)
	session.agent.mu.Lock()
	delete(session.agent.sessions, canonical)
	session.agent.mu.Unlock()
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
	next, err := session.log.Append(ctx, session.revision, records...)
	if err != nil {
		return fmt.Errorf("append Agent Session records: %w", err)
	}
	session.revision = next
	return nil
}

func (session *Session) persistTranscriptLocked(ctx context.Context) error {
	return session.appendRecordLocked(ctx, sessionTranscriptRecord, persistedSessionTranscript{
		EngineState: append(json.RawMessage(nil), session.engineState...),
	})
}

func (session *Session) persistCapabilitiesLocked(ctx context.Context) error {
	return session.appendRecordLocked(ctx, sessionCapabilitiesRecord, persistedSessionCapabilities{
		Capabilities: cloneRawStateMap(session.capabilities),
	})
}

func (session *Session) persistStateLocked(ctx context.Context) error {
	transcript, err := json.Marshal(persistedSessionTranscript{
		EngineState: append(json.RawMessage(nil), session.engineState...),
	})
	if err != nil {
		return fmt.Errorf("encode Agent Session transcript: %w", err)
	}
	capabilities, err := json.Marshal(persistedSessionCapabilities{
		Capabilities: cloneRawStateMap(session.capabilities),
	})
	if err != nil {
		return fmt.Errorf("encode Agent Session capabilities: %w", err)
	}
	return session.appendRecordsLocked(ctx,
		agentsession.Record{Kind: sessionTranscriptRecord, Version: sessionRecordVersion, Data: transcript},
		agentsession.Record{Kind: sessionCapabilitiesRecord, Version: sessionRecordVersion, Data: capabilities},
	)
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
		delete(session.runs, session.recent[0].ID)
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
	input.Goal = cloneGoalMutation(input.Goal)
	input.HostData = cloneHostData(input.HostData)
	return input
}
