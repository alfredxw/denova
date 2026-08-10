package execution

import (
	"context"
	agentchat "denova/internal/agents/chat"
	"denova/internal/agents/run"
	"errors"
	"fmt"
	"strings"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

// StartRequest contains one fully prepared initial cycle and its process-local
// display sink. Initial preparation remains caller-controlled so validation can
// fail before durable command acceptance.
type StartRequest struct {
	Cycle Cycle
	Emit  func(agentrun.Event)
}

// Run admits and waits for one prepared durable cycle.
func (s *Runtime) Run(ctx context.Context, request StartRequest) agentrun.Outcome {
	if s == nil || s.coordinator == nil {
		emitExecutionError(request.Emit, ErrUnavailable)
		return agentrun.NewOutcome(agentrun.OutcomeFailed, ErrUnavailable, ErrUnavailable.Error(), "", "")
	}
	return s.coordinator.run(ctx, request)
}

// Start durably accepts a prepared initial cycle and returns before model
// settlement.
func (s *Runtime) Start(ctx context.Context, request StartRequest) (*Operation, error) {
	if s == nil || s.coordinator == nil {
		return nil, ErrUnavailable
	}
	return s.coordinator.start(ctx, request)
}

func (h *coordinator) run(ctx context.Context, request StartRequest) agentrun.Outcome {
	accepted, err := h.start(ctx, request)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return agentrun.NewOutcome(agentrun.OutcomeAborted, err, err.Error(), "", "")
		}
		emitExecutionError(request.Emit, err)
		return agentrun.NewOutcome(agentrun.OutcomeFailed, err, err.Error(), "", "")
	}
	return accepted.Wait(ctx)
}

func (h *coordinator) start(ctx context.Context, request StartRequest) (*Operation, error) {
	if h == nil || h.runtime == nil || h.engine == nil {
		return nil, fmt.Errorf("agent durable runtime is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cycle := request.Cycle
	workspace := ""
	if cycle.BookService != nil {
		workspace = cycle.BookService.Workspace()
	}
	commandID := runstate.CommandID(strings.TrimSpace(cycle.Request.CommandID))
	if commandID == "" {
		return nil, fmt.Errorf("%w: command_id is required", runstate.ErrInvalidCommand)
	}
	if err := h.runtime.ValidateCommandID(string(commandID)); err != nil {
		return nil, err
	}
	cycle.Options = cycle.Options.Normalize(workspace)
	cycle.Request = agentchat.CaptureChatRequestCallerInput(cycle.Request)
	binding, err := agentrun.BindingForOptions(cycle.Options)
	if err != nil {
		return nil, err
	}
	harness, err := h.runtime.Open(h.lifecycle, binding)
	if err != nil {
		return nil, err
	}

	observeCtx, stopObserving := context.WithCancel(h.lifecycle)
	observation, err := harness.ObserveFromNow(observeCtx)
	if err != nil {
		stopObserving()
		return nil, err
	}

	command, turnRef, err := newStartTurn(binding, commandID, cycle.Request, cycle.Options)
	if err != nil {
		stopObserving()
		return nil, err
	}
	outcomes := make(chan agentrun.Outcome, 1)
	registration, err := h.engine.register(turnRef, command, cycleSpec{
		CommandID: agentrun.CommandID(commandID), CommandKind: CommandStartTurn,
		Runner: cycle.Runner, Conversation: cycle.Conversation, BookService: cycle.BookService,
		Request: cycle.Request, Options: cycle.Options, Emit: request.Emit, Outcome: outcomes,
		CycleCommit: cycleCommitForConversation(cycle.Conversation),
		Successor:   cycle.Successor,
	})
	if err != nil {
		stopObserving()
		return nil, err
	}
	receipt, err := h.submitStartWhenIdle(ctx, harness, observation, command)
	if err != nil {
		registration.release()
		stopObserving()
		return nil, err
	}
	if !receipt.Replayed {
		registration.accept()
	}
	ephemeral := cycle.Options.AgentKind == agentrun.AgentKindAutomation
	return &Operation{
		owner: h, harness: harness, observation: observation, receipt: receipt,
		conversation: cycle.Conversation, options: cycle.Options,
		outcomes: outcomes, emit: request.Emit, stopObserving: stopObserving,
		registration: registration, binding: binding, ephemeralBinding: ephemeral,
	}, nil
}

func newStartTurn(
	binding runstate.BindingRef,
	commandID runstate.CommandID,
	req agentchat.ChatRequest,
	options agentrun.Options,
) (runstate.StartTurn, string, error) {
	if strings.TrimSpace(string(commandID)) == "" {
		return runstate.StartTurn{}, "", fmt.Errorf("%w: command_id is required", runstate.ErrInvalidCommand)
	}
	req = agentchat.CaptureChatRequestCallerInput(req)
	semanticSpec := CommandRequest{
		Kind: CommandStartTurn, CommandID: string(commandID),
		Request: req, Options: options,
	}
	turnRef := commandCycleRef(binding, string(commandID), cycleSemanticFingerprint(semanticSpec))
	caller := agentchat.CallerView(req)
	input, err := withInputMaterializationDescriptor(runstate.UserInput{
		Text: caller.Message, ContextRefs: commandContextRefs(req), TurnSpecRef: turnRef,
	}, semanticSpec)
	if err != nil {
		return runstate.StartTurn{}, "", err
	}
	return runstate.StartTurn{
		ID:    commandID,
		Input: input,
	}, turnRef, nil
}

// StartTurnFingerprint derives the exact canonical fingerprint that Start will
// submit, without opening a binding or registering any
// process-local execution state. Adapters persist it before admission so cold
// reconciliation can distinguish an exact replay from command-ID reuse.
func StartTurnFingerprint(req agentchat.ChatRequest, options agentrun.Options, workspace string) (string, error) {
	commandID := runstate.CommandID(strings.TrimSpace(req.CommandID))
	if commandID == "" {
		return "", fmt.Errorf("%w: command_id is required", runstate.ErrInvalidCommand)
	}
	options = options.Normalize(workspace)
	req = agentchat.CaptureChatRequestCallerInput(req)
	binding, err := agentrun.BindingForOptions(options)
	if err != nil {
		return "", err
	}
	command, _, err := newStartTurn(binding, commandID, req, options)
	if err != nil {
		return "", err
	}
	return runstate.CommandFingerprint(command)
}

// submitStartWhenIdle preserves the product contract that independent calls
// on one binding serialize. The durable lane never aborts an unrelated
// operation implicitly.
func (h *coordinator) submitStartWhenIdle(
	caller context.Context,
	harness *runstate.Harness,
	observation runstate.Observation,
	command runstate.StartTurn,
) (runstate.Receipt, error) {
	if snapshotRequiresExplicitRecovery(observation.Snapshot) {
		return runstate.Receipt{}, ErrRecoveryRequired
	}
	events := observation.Events
	errorsCh := observation.Errors
	for {
		receipt, err := harness.Submit(h.lifecycle, command)
		if err == nil {
			return receipt, nil
		}
		if !errors.Is(err, runstate.ErrBusy) {
			return runstate.Receipt{}, err
		}
		if _, resumed, resumeErr := h.resumeRecoveredContextStructuralOperation(caller, harness, ""); resumeErr != nil {
			return runstate.Receipt{}, resumeErr
		} else if resumed {
			goto retry
		}
		for {
			select {
			case event, ok := <-events:
				if !ok {
					return runstate.Receipt{}, fmt.Errorf("agent observation closed while waiting for an idle binding")
				}
				switch event.Payload.(type) {
				case runstate.OperationSettledEvent, runstate.OperationInterruptedEvent:
					goto retry
				case runstate.OperationRecoveryPausedEvent:
					return runstate.Receipt{}, ErrRecoveryRequired
				}
			case observationErr, ok := <-errorsCh:
				if !ok {
					return runstate.Receipt{}, fmt.Errorf("agent observation errors closed while waiting for an idle binding")
				}
				if observationErr != nil {
					return runstate.Receipt{}, observationErr
				}
			case <-caller.Done():
				return runstate.Receipt{}, caller.Err()
			case <-h.lifecycle.Done():
				return runstate.Receipt{}, h.lifecycle.Err()
			}
		}
	retry:
	}
}

func snapshotRequiresExplicitRecovery(snapshot runstate.StateSnapshot) bool {
	if snapshot.RecoveryPaused {
		return true
	}
	for _, item := range snapshot.Queue {
		if item.Delivery == runstate.DeliveryNextTurn {
			return true
		}
	}
	return false
}
