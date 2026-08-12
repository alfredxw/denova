package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	runstate "github.com/alfredxw/denova/agent/internal/runtime"
	agentsession "github.com/alfredxw/denova/agent/session"
)

type inputEnvelope struct {
	Version  uint16            `json:"version"`
	Context  []ContextFragment `json:"context,omitempty"`
	Goal     *GoalMutation     `json:"goal,omitempty"`
	HostData *HostData         `json:"host_data,omitempty"`
}

// Session is one exact, durable execution lane owned by Agent.
type Session struct {
	agent   *Agent
	key     SessionKey
	binding runstate.BindingRef
	harness *runstate.Harness

	mu     sync.RWMutex
	closed bool
}

func (session *Session) Key() SessionKey {
	if session == nil {
		return SessionKey{}
	}
	key := session.key
	key.Attributes = cloneStringMap(key.Attributes)
	return key
}

func (session *Session) Run(ctx context.Context, input Input) (*Run, error) {
	if input.Goal != nil {
		input.Goal = cloneGoalMutation(input.Goal)
		if input.Goal.MutationID == "" {
			input.Goal.MutationID = newPublicID("goal-mutation")
		}
	}
	return session.start(ctx, input, "", false, runUsesDurableSession)
}

func (session *Session) start(
	ctx context.Context,
	input Input,
	afterRunID string,
	queued bool,
	ownership runSessionOwnership,
) (*Run, error) {
	if err := session.usable(); err != nil {
		return nil, err
	}
	if input.Goal != nil {
		input.Goal = cloneGoalMutation(input.Goal)
		if input.Goal.MutationID == "" {
			input.Goal.MutationID = newPublicID("goal-mutation")
		}
	}
	commandID := strings.TrimSpace(input.IdempotencyKey)
	if commandID == "" {
		commandID = newPublicID("command")
	}
	encoded, runtimeInput, err := encodeInput(input)
	if err != nil {
		return nil, err
	}
	runtimeInput.RestoreDescriptor = encoded
	var receipt runstate.Receipt
	if queued {
		receipt, err = session.harness.Submit(ctx, runstate.NextTurn{
			ID: runstate.CommandID(commandID), AfterOperationID: runstate.OperationID(afterRunID), Input: runtimeInput,
		})
	} else {
		receipt, err = session.harness.Submit(ctx, runstate.StartTurn{
			ID: runstate.CommandID(commandID), Input: runtimeInput,
		})
	}
	if err != nil {
		return nil, mapRuntimeError(err)
	}
	emitTrace(session.agent.ctx, session.agent.trace, TraceEvent{
		Kind: TraceRunAccepted, Session: session.key, RunID: string(receipt.OperationID),
	})
	observeCtx, stopObserving := context.WithCancel(session.agent.ctx)
	replayAfter := runstate.Cursor(0)
	if receipt.Cursor > 0 {
		replayAfter = receipt.Cursor - 1
	}
	observation, err := session.harness.Observe(observeCtx, replayAfter)
	if err != nil {
		stopObserving()
		return nil, mapRuntimeError(err)
	}
	return newPublicRun(session, receipt, observation, observeCtx, stopObserving, ownership), nil
}

func encodeInput(input Input) (json.RawMessage, runstate.UserInput, error) {
	if strings.TrimSpace(input.Text) == "" {
		return nil, runstate.UserInput{}, errors.New("Agent Input Text is required")
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
		Goal: cloneGoalMutation(input.Goal), HostData: cloneHostData(input.HostData),
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
	return encoded, runstate.UserInput{Text: input.Text, ContextRefs: references}, nil
}

func decodeInput(input runstate.UserInput) (Input, error) {
	result := Input{Text: input.Text}
	if len(input.RestoreDescriptor) == 0 {
		return result, nil
	}
	var envelope inputEnvelope
	if err := json.Unmarshal(input.RestoreDescriptor, &envelope); err != nil {
		return Input{}, fmt.Errorf("decode durable Agent Input: %w", err)
	}
	if envelope.Version != 1 {
		return Input{}, fmt.Errorf("unsupported durable Agent Input version %d", envelope.Version)
	}
	result.Context = append([]ContextFragment(nil), envelope.Context...)
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

func (session *Session) Active(ctx context.Context) (*Run, bool, error) {
	if err := session.usable(); err != nil {
		return nil, false, err
	}
	status, err := session.harness.Status(ctx)
	if err != nil {
		return nil, false, mapRuntimeError(err)
	}
	if status.Phase == runstate.PhaseIdle || status.ActiveOperation == "" {
		return nil, false, nil
	}
	observeCtx, stopObserving := context.WithCancel(session.agent.ctx)
	observation, err := session.harness.ObserveFromNow(observeCtx)
	if err != nil {
		stopObserving()
		return nil, false, mapRuntimeError(err)
	}
	receipt := runstate.Receipt{
		CommandID: status.ActiveCommandID, OperationID: status.ActiveOperation, Cursor: status.ActiveReceiptCursor,
	}
	return newPublicRun(session, receipt, observation, observeCtx, stopObserving, runUsesDurableSession), true, nil
}

// AttachRun reconstructs a handle for an active, queued, or recently settled
// Run. This is the restart-safe counterpart to retaining a process-local Run
// pointer and never requires callers to retain command or operation IDs.
func (session *Session) AttachRun(ctx context.Context, runID string) (*Run, bool, error) {
	if err := session.usable(); err != nil {
		return nil, false, err
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil, false, nil
	}
	status, err := session.harness.Status(ctx)
	if err != nil {
		return nil, false, mapRuntimeError(err)
	}
	var commandID runstate.CommandID
	var cursor runstate.Cursor
	if string(status.ActiveOperation) == runID {
		commandID, cursor = status.ActiveCommandID, status.ActiveReceiptCursor
	}
	if commandID == "" {
		for _, queued := range status.Queue {
			if queued.Autonomous || string(queued.OperationID) != runID {
				continue
			}
			commandID, cursor = queued.CommandID, queued.ReceiptCursor
			break
		}
	}
	if commandID == "" {
		for _, summary := range status.RecentOperations {
			if string(summary.OperationID) != runID {
				continue
			}
			commandID, cursor = summary.CommandID, summary.ReceiptCursor
			break
		}
	}
	if commandID == "" {
		return nil, false, nil
	}
	observeCtx, stopObserving := context.WithCancel(session.agent.ctx)
	after := runstate.Cursor(0)
	if cursor > 0 {
		after = cursor - 1
	}
	observation, err := session.harness.Observe(observeCtx, after)
	if err != nil {
		stopObserving()
		return nil, false, mapRuntimeError(err)
	}
	receipt := runstate.Receipt{CommandID: commandID, OperationID: runstate.OperationID(runID), Cursor: cursor}
	return newPublicRun(session, receipt, observation, observeCtx, stopObserving, runUsesDurableSession), true, nil
}

// RunInput returns the exact admitted input for an active, queued, or retained
// Run. It exists for reconnecting product display state; callers must not use
// HostData as model context or mutate it in place.
func (session *Session) RunInput(ctx context.Context, runID string) (Input, bool, error) {
	if err := session.usable(); err != nil {
		return Input{}, false, err
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return Input{}, false, nil
	}
	status, err := session.harness.Status(ctx)
	if err != nil {
		return Input{}, false, mapRuntimeError(err)
	}
	var commandID runstate.CommandID
	if string(status.ActiveOperation) == runID {
		commandID = status.ActiveCommandID
	}
	if commandID == "" {
		for _, queued := range status.Queue {
			if !queued.Autonomous && string(queued.OperationID) == runID {
				commandID = queued.CommandID
				break
			}
		}
	}
	if commandID == "" {
		for _, summary := range status.RecentOperations {
			if string(summary.OperationID) == runID {
				commandID = summary.CommandID
				break
			}
		}
	}
	if commandID == "" {
		return Input{}, false, nil
	}
	runtimeInput, found, err := session.harness.RecoveryInput(ctx, commandID, runstate.OperationID(runID))
	if err != nil || !found {
		return Input{}, found, mapRuntimeError(err)
	}
	input, err := decodeInput(runtimeInput)
	if err != nil {
		return Input{}, false, err
	}
	input.IdempotencyKey = string(commandID)
	return input, true, nil
}

func (session *Session) Observe(ctx context.Context, after Cursor) (Observation, error) {
	if err := session.usable(); err != nil {
		return Observation{}, err
	}
	observation, err := session.harness.Observe(ctx, runstate.Cursor(after))
	if err != nil {
		return Observation{}, mapRuntimeError(err)
	}
	return mapObservation(session.key, observation, session.agent.projectionTextMaxBytes), nil
}

// Snapshot returns the current bounded Session projection without retaining a
// live subscription. It is the status/rehydration counterpart to Observe.
func (session *Session) Snapshot(ctx context.Context) (SessionSnapshot, error) {
	if err := session.usable(); err != nil {
		return SessionSnapshot{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	observeCtx, cancel := context.WithCancel(ctx)
	observation, err := session.harness.ObserveFromNow(observeCtx)
	if err != nil {
		cancel()
		return SessionSnapshot{}, mapRuntimeError(err)
	}
	snapshot := mapSessionSnapshot(session.key, observation.Snapshot, session.agent.projectionTextMaxBytes)
	cancel()
	return snapshot, nil
}

// RecoveryInput returns the exact accepted input selected by a current opaque
// recovery action. Product hosts use it only to rebuild display routing; Agent
// remains the authority that validates and executes the action itself.
func (session *Session) RecoveryInput(ctx context.Context, action RecoveryAction) (Input, bool, error) {
	if err := session.usable(); err != nil {
		return Input{}, false, err
	}
	if strings.TrimSpace(action.ID) == "" || strings.TrimSpace(action.RunID) == "" {
		return Input{}, false, ErrRecoveryStale
	}
	current, err := session.RecoveryActions(ctx)
	if err != nil {
		return Input{}, false, err
	}
	var selected *RecoveryAction
	for index := range current {
		if current[index].ID == action.ID {
			candidate := current[index]
			selected = &candidate
			break
		}
	}
	if selected == nil || *selected != action {
		return Input{}, false, ErrRecoveryStale
	}
	if selected.Kind == RecoveryResumeCompaction {
		return Input{}, false, nil
	}
	if selected.CommandID == "" {
		return session.RunInput(ctx, selected.RunID)
	}
	runtimeInput, found, err := session.harness.RecoveryInput(
		ctx, runstate.CommandID(selected.CommandID), runstate.OperationID(selected.RunID),
	)
	if err != nil || !found {
		return Input{}, found, mapRuntimeError(err)
	}
	input, err := decodeInput(runtimeInput)
	if err != nil {
		return Input{}, false, err
	}
	input.IdempotencyKey = selected.CommandID
	return input, true, nil
}

func (session *Session) Goal(ctx context.Context) (GoalState, bool, error) {
	if err := session.usable(); err != nil {
		return GoalState{}, false, err
	}
	snapshot, err := session.harness.CapabilityState(ctx, goalCapability)
	if err != nil {
		return GoalState{}, false, mapRuntimeError(err)
	}
	if !snapshot.Exists {
		return GoalState{}, false, nil
	}
	state, err := decodeGoalState(snapshot.State)
	if err != nil {
		return GoalState{}, false, err
	}
	return state, state.Visible(), nil
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
	if session.agent != nil && isPersistentStore(session.agent.store) {
		if err := definition.Goal.Identity().validate("Goal"); err != nil {
			return GoalState{}, err
		}
	}
	current, err := session.harness.CapabilityState(ctx, goalCapability)
	if err != nil {
		return GoalState{}, mapRuntimeError(err)
	}
	var state GoalState
	if current.Exists {
		state, err = decodeGoalState(current.State)
		if err != nil {
			return GoalState{}, err
		}
	}
	next, err := applyGoalMutation(ctx, definition.Goal, GoalApplyRequest{
		Session: SessionView{Key: session.key}, Current: state, Present: current.Exists,
		Mutation: mutation,
	})
	if err != nil {
		return GoalState{}, err
	}
	encoded, err := json.Marshal(next)
	if err != nil {
		return GoalState{}, fmt.Errorf("encode Goal state: %w", err)
	}
	if err := session.harness.SetCapabilityState(ctx, goalCapability, current.Descriptor, encoded, false); err != nil {
		return GoalState{}, mapRuntimeError(err)
	}
	return next, nil
}

func decodeGoalState(encoded json.RawMessage) (GoalState, error) {
	var state GoalState
	if err := json.Unmarshal(encoded, &state); err != nil {
		return GoalState{}, fmt.Errorf("decode Goal state: %w", err)
	}
	if state.Revision == 0 || state.Status == "" {
		return GoalState{}, errors.New("durable Goal state is invalid")
	}
	return state, nil
}

func (session *Session) Close(ctx context.Context) error {
	if session == nil {
		return nil
	}
	session.mu.Lock()
	if session.closed {
		session.mu.Unlock()
		return nil
	}
	session.closed = true
	session.mu.Unlock()
	if session.agent == nil || session.agent.runtime == nil {
		return nil
	}
	err := session.agent.runtime.CloseBinding(ctx, session.binding)
	canonical, _ := agentsessionCanonical(session.key)
	session.agent.mu.Lock()
	delete(session.agent.sessions, canonical)
	session.agent.mu.Unlock()
	return mapRuntimeError(err)
}

func (session *Session) usable() error {
	if session == nil || session.agent == nil || session.harness == nil {
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

func agentsessionCanonical(key SessionKey) (string, error) {
	return agentsession.CanonicalKey(key)
}

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
