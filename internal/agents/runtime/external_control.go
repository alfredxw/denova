package agentruntime

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"sync"

	agentchat "denova/internal/agents/chat"
	agentexecution "denova/internal/agents/execution"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/runtime/external"
)

const externalControlCapability = "denova.external.control.v1"

// ExternalCycleInput is an accepted product input, including guidance admitted
// while running. Resume restores accepted input and effects without replaying
// mutations; the product factory owns that domain-specific recovery boundary.
type ExternalCycleInput struct {
	OperationID          agentrun.OperationID    `json:"-"`
	Request              agentchat.ChatRequest   `json:"request"`
	Delivery             agentrun.DeliveryKind   `json:"delivery"`
	Resume               bool                    `json:"resume,omitempty"`
	Guidance             []agentchat.ChatRequest `json:"guidance,omitempty"`
	GoalID               string                  `json:"goal_id,omitempty"`
	GoalRevision         uint64                  `json:"goal_revision,omitempty"`
	RegenerateFromTurnID string                  `json:"regenerate_from_turn_id,omitempty"`
}
type ExternalCycle interface {
	Wait(context.Context) agentrun.Outcome
}
type ExternalCycleFactory func(context.Context, ExternalCycleInput, func(agentrun.Event), func(context.Context, *external.RuntimeSession) error) (ExternalCycle, error)

// ExternalController orders commands for one peer runtime at the Denova layer.
// The product journal is authoritative; live handles only cancel and emit.
type ExternalController struct {
	mu      sync.Mutex
	store   ProductState
	binding agentrun.RuntimeBinding
	active  *ExternalRun
}

func (engines *Engines) ExternalControl(options agentrun.Options, state ProductState) (*ExternalController, error) {
	binding, err := agentrun.RuntimeBindingForOptions(options)
	if err != nil {
		return nil, err
	}
	key, err := binding.AgentSessionKey()
	if err != nil {
		return nil, err
	}
	encoded, _ := json.Marshal(key)
	engines.controlsMu.Lock()
	if engines.controls == nil {
		engines.controls = map[string]*ExternalController{}
	}
	control := engines.controls[string(encoded)]
	if control == nil {
		control = &ExternalController{store: state, binding: binding}
		engines.controls[string(encoded)] = control
		engines.controlsMu.Unlock()
		return control, nil
	}
	engines.controlsMu.Unlock()

	// Session maintenance may wait for a provider. Never hold the registry lock
	// while waiting for a session, or unrelated conversations lose their controls.
	control.mu.Lock()
	if control.active == nil {
		control.store = state
		control.binding = binding
	}
	control.mu.Unlock()
	return control, nil
}

func (control *ExternalController) Status(ctx context.Context) (agentrun.RuntimeStatus, error) {
	control.mu.Lock()
	defer control.mu.Unlock()
	state, err := control.read(ctx)
	if err != nil {
		return agentrun.RuntimeStatus{}, err
	}
	if state.Phase == agentrun.RunPhaseRunning && control.active == nil {
		// Process loss cannot implicitly restart accepted work or an active Goal.
		err = control.update(ctx, func(current *externalControlState) error {
			current.Phase = agentrun.RunPhaseSuspended
			if current.Current != nil {
				current.Current.Resume = true
			}
			return nil
		})
		if err != nil {
			return agentrun.RuntimeStatus{}, err
		}
		state, err = control.read(ctx)
	}
	view := agentrun.RuntimeStatus{Binding: control.binding, Cursor: agentrun.Cursor(state.Revision), Phase: state.Phase, LastOperation: state.Last}
	if state.Phase != agentrun.RunPhaseIdle {
		view.ActiveOperation, view.ActiveCommandID = state.OperationID, state.CommandID
		view.ActiveCycle = 1
	}
	for _, input := range state.Queue {
		view.Queue = append(view.Queue, agentrun.QueuedCommand{CommandID: agentrun.CommandID(input.Request.CommandID), OperationID: state.OperationID, Delivery: input.Delivery, Message: input.Request.Message, SteerRequested: input.Delivery == agentrun.DeliverySteer})
	}
	return view, err
}

func (control *ExternalController) Submit(ctx context.Context, command Command) (agentrun.CommandReceipt, error) {
	if err := agentrun.ValidateCommandID(command.CommandID); err != nil {
		return agentrun.CommandReceipt{}, err
	}
	control.mu.Lock()
	defer control.mu.Unlock()
	encoded, err := json.Marshal(command)
	if err != nil {
		return agentrun.CommandReceipt{}, err
	}
	fingerprint := controlFingerprint(encoded)
	var receipt agentrun.CommandReceipt
	var interrupt error
	err = control.update(ctx, func(state *externalControlState) error {
		saved, found, err := state.receipt(command.CommandID)
		if err != nil {
			return err
		}
		if found {
			if saved.Fingerprint != fingerprint {
				return agentrun.ErrInvalidCommand
			}
			receipt = saved.Receipt
			return nil
		}
		if state.Phase == agentrun.RunPhaseIdle || command.OperationID != state.OperationID {
			return agentrun.ErrStaleOperation
		}
		switch command.Kind {
		case agentexecution.CommandSuspend:
			state.Phase = agentrun.RunPhaseSuspended
			if state.Current != nil {
				state.Current.Resume = true
			}
			interrupt = external.ErrSuspended
		case agentexecution.CommandAbort:
			state.Phase, state.Current, state.Queue = agentrun.RunPhaseIdle, nil, nil
			if err := state.setOutcome(string(state.CommandID), agentrun.OutcomeAborted); err != nil {
				return err
			}
			interrupt = context.Canceled
		case agentexecution.CommandFollowUp, agentexecution.CommandNextTurn, agentexecution.CommandSteer:
			request := command.Input
			request.CommandID = command.CommandID
			request = agentchat.CaptureChatRequestCallerInput(request)
			input := ExternalCycleInput{Request: request, Delivery: agentrun.DeliveryKind(command.Kind)}
			if command.Kind == agentexecution.CommandSteer && state.Current != nil {
				state.Current.Guidance = append(state.Current.Guidance, request)
				state.Current.Resume = true
			} else {
				state.Queue = append(state.Queue, input)
			}
		case agentexecution.CommandCancelQueued, agentexecution.CommandSteerQueued:
			index := -1
			for i, input := range state.Queue {
				if input.Request.CommandID == string(command.TargetCommandID) {
					index = i
					break
				}
			}
			if index < 0 {
				return agentrun.ErrQueueConflict
			}
			input := state.Queue[index]
			state.Queue = append(state.Queue[:index], state.Queue[index+1:]...)
			if command.Kind == agentexecution.CommandSteerQueued {
				if state.Current != nil {
					state.Current.Guidance = append(state.Current.Guidance, input.Request)
					state.Current.Resume = true
				} else {
					input.Delivery = agentrun.DeliverySteer
					state.Queue = append([]ExternalCycleInput{input}, state.Queue...)
				}
			}
		default:
			return fmt.Errorf("%w: external command %q", agentrun.ErrInvalidCommand, command.Kind)
		}
		receipt = agentrun.CommandReceipt{CommandID: agentrun.CommandID(command.CommandID), OperationID: state.OperationID, Cursor: agentrun.Cursor(state.Revision + 1)}
		state.receipts[command.CommandID] = controlReceipt{Fingerprint: fingerprint, Receipt: receipt}
		return nil
	})
	if err == nil && control.active != nil && interrupt != nil && control.active.cancel != nil {
		control.active.cancel(interrupt)
	}
	if err == nil && control.active != nil {
		control.active.notifySteering()
	}
	return receipt, err
}

// Start durably accepts a task before provider execution. Each product factory
// materializes its own input and acknowledges only its canonical business commit.
func (control *ExternalController) Start(ctx context.Context, input ExternalCycleInput, factory ExternalCycleFactory, emit func(agentrun.Event)) (*ExternalRun, error) {
	request := input.Request
	if err := agentrun.ValidateCommandID(request.CommandID); err != nil {
		return nil, err
	}
	control.mu.Lock()
	defer control.mu.Unlock()
	state, err := control.read(ctx)
	if err != nil {
		return nil, err
	}
	saved, found, err := state.receipt(request.CommandID)
	if err != nil {
		return nil, err
	}
	if found {
		if saved.Fingerprint != externalInputFingerprint(input) {
			return nil, agentrun.ErrInvalidCommand
		}
		if control.active != nil && control.active.receipt == saved.Receipt {
			return control.active, nil
		}
		if saved.Outcome != "" {
			return &ExternalRun{control: control, emit: emit, receipt: saved.Receipt, replayed: true, outcome: agentrun.Outcome{Status: saved.Outcome}}, nil
		}
		return nil, ErrOperationActive
	}
	if control.active != nil {
		return nil, ErrOperationActive
	}
	run := &ExternalRun{control: control, factory: factory, emit: emit}
	err = control.update(ctx, func(state *externalControlState) error {
		if state.Phase != agentrun.RunPhaseIdle {
			return ErrOperationActive
		}
		if _, found, err := state.receipt(request.CommandID); err != nil {
			return err
		} else if found {
			return agentrun.ErrInvalidCommand
		}
		state.OperationID, state.CommandID = agentrun.OperationID("external-run-"+rand.Text()), agentrun.CommandID(request.CommandID)
		input.Request = agentchat.CaptureChatRequestCallerInput(request)
		state.Current = &input
		state.Phase = agentrun.RunPhaseRunning
		run.receipt = agentrun.CommandReceipt{CommandID: state.CommandID, OperationID: state.OperationID, Cursor: agentrun.Cursor(state.Revision + 1)}
		state.receipts[request.CommandID] = controlReceipt{Fingerprint: externalInputFingerprint(input), Receipt: run.receipt}
		return nil
	})
	if err != nil {
		return nil, err
	}
	control.active = run
	return run, nil
}

func (control *ExternalController) Resume(ctx context.Context, action agentexecution.RuntimeRecoveryAction, factory ExternalCycleFactory, emit func(agentrun.Event)) (*ExternalRun, error) {
	control.mu.Lock()
	defer control.mu.Unlock()
	state, err := control.read(ctx)
	if err != nil {
		return nil, err
	}
	key := "recovery:" + RecoveryActionKey(action)
	saved, found, err := state.receipt(key)
	if err != nil {
		return nil, err
	}
	if found {
		if control.active != nil && control.active.recoveryKey == key {
			return control.active, nil
		}
		if saved.Outcome != "" {
			return &ExternalRun{receipt: saved.Receipt, replayed: true, outcome: agentrun.Outcome{Status: saved.Outcome}}, nil
		}
	}
	if control.active != nil {
		return nil, ErrOperationActive
	}
	view := agentrun.RuntimeStatus{Phase: state.Phase, Cursor: agentrun.Cursor(state.Revision), ActiveCommandID: state.CommandID, ActiveOperation: state.OperationID}
	if err := ValidateRecoveryAction(view, action); err != nil {
		return nil, err
	}
	if action.Kind != agentexecution.RuntimeRecoveryResume && action.Kind != agentexecution.RuntimeRecoveryAbort {
		return nil, agentrun.ErrInvalidCommand
	}
	run := &ExternalRun{control: control, factory: factory, emit: emit, recoveryKey: key, receipt: agentrun.CommandReceipt{CommandID: state.CommandID, OperationID: state.OperationID, Cursor: agentrun.Cursor(state.Revision)}}
	err = control.update(ctx, func(current *externalControlState) error {
		current.receipts[key] = controlReceipt{Receipt: run.receipt}
		if action.Kind == agentexecution.RuntimeRecoveryAbort {
			current.Phase, current.Current, current.Queue = agentrun.RunPhaseIdle, nil, nil
			if err := current.setOutcome(string(current.CommandID), agentrun.OutcomeAborted); err != nil {
				return err
			}
			return nil
		}
		current.Phase = agentrun.RunPhaseRunning
		if current.Current != nil {
			current.Current.Resume = true
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	control.active = run
	return run, nil
}

// DetachSuspendedGameForEngineSwitch relinquishes only the old provider
// controller. The Game journal retains its accepted input, draft and dice so
// a newly selected CLI can resume the same canonical turn. This operation is
// only permitted for one quiescent suspended cycle with no queued work.
func (control *ExternalController) DetachSuspendedGameForEngineSwitch(ctx context.Context, commandID string) error {
	control.mu.Lock()
	defer control.mu.Unlock()
	state, err := control.read(ctx)
	if err != nil {
		return err
	}
	if control.active != nil {
		return ErrOperationActive
	}
	if state.Phase == agentrun.RunPhaseIdle {
		return nil
	}
	if state.Phase != agentrun.RunPhaseSuspended || state.Current == nil ||
		state.Current.Request.CommandID != commandID || len(state.Queue) != 0 ||
		len(state.Current.Guidance) != 0 {
		return ErrOperationActive
	}
	return control.update(ctx, func(current *externalControlState) error {
		if current.Phase != agentrun.RunPhaseSuspended || current.Current == nil ||
			current.Current.Request.CommandID != commandID || len(current.Queue) != 0 ||
			len(current.Current.Guidance) != 0 {
			return ErrOperationActive
		}
		current.Phase, current.Current, current.Queue = agentrun.RunPhaseIdle, nil, nil
		return current.setOutcome(string(current.CommandID), agentrun.OutcomeAborted)
	})
}

// Receipt reads the logical product task receipt, independently of the shorter
// provider attempts that materialize its inputs and outputs.
func (control *ExternalController) Receipt(ctx context.Context, commandID string) (agentrun.CommandReceipt, bool, error) {
	control.mu.Lock()
	defer control.mu.Unlock()
	state, err := control.read(ctx)
	if err != nil {
		return agentrun.CommandReceipt{}, false, err
	}
	saved, found, err := state.receipt(commandID)
	return saved.Receipt, found, err
}

// RecoveryInput supplies product routing and display metadata for an explicit
// recovery task. It never resumes execution by itself.
func (control *ExternalController) RecoveryInput(ctx context.Context) (ExternalCycleInput, error) {
	control.mu.Lock()
	defer control.mu.Unlock()
	state, err := control.read(ctx)
	if err != nil {
		return ExternalCycleInput{}, err
	}
	if state.Current != nil {
		return *state.Current, nil
	}
	if len(state.Queue) > 0 {
		return state.Queue[0], nil
	}
	return ExternalCycleInput{Request: agentchat.ChatRequest{CommandID: string(state.CommandID), Message: "Continue the active objective.", InputVisibility: agentrun.InputModelOnly}}, nil
}
