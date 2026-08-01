package harness

import (
	"context"
	agentchat "denova/internal/agents/chat"
	"denova/internal/agents/run"
	"errors"
	"fmt"
	"strings"

	agent "github.com/alfredxw/denova/agent"

	"denova/internal/book"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

// RunWithOptions admits and waits for one durable turn.
func (s *Service) RunWithOptions(
	ctx context.Context,
	runner *agent.Runner,
	conversation agentchat.Conversation,
	bookService *book.Service,
	req agentchat.ChatRequest,
	options agentrun.Options,
	emit func(agentrun.Event),
) agentrun.Outcome {
	if s == nil || s.coordinator == nil {
		emitHarnessError(emit, ErrUnavailable)
		return agentrun.NewOutcome(agentrun.OutcomeFailed, ErrUnavailable, ErrUnavailable.Error(), "", "")
	}
	return s.coordinator.run(ctx, runner, conversation, bookService, req, options, emit)
}

// StartWithOptions durably accepts a turn and returns before model settlement.
func (s *Service) StartWithOptions(
	ctx context.Context,
	runner *agent.Runner,
	conversation agentchat.Conversation,
	bookService *book.Service,
	req agentchat.ChatRequest,
	options agentrun.Options,
	emit func(agentrun.Event),
) (*AcceptedRun, error) {
	if s == nil || s.coordinator == nil {
		return nil, ErrUnavailable
	}
	return s.coordinator.start(ctx, runner, conversation, bookService, req, options, emit)
}

func (h *coordinator) run(
	ctx context.Context,
	runner *agent.Runner,
	conversation agentchat.Conversation,
	bookService *book.Service,
	req agentchat.ChatRequest,
	options agentrun.Options,
	emit func(agentrun.Event),
) agentrun.Outcome {
	accepted, err := h.start(ctx, runner, conversation, bookService, req, options, emit)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return agentrun.NewOutcome(agentrun.OutcomeAborted, err, err.Error(), "", "")
		}
		emitHarnessError(emit, err)
		return agentrun.NewOutcome(agentrun.OutcomeFailed, err, err.Error(), "", "")
	}
	return accepted.Wait(ctx)
}

func (h *coordinator) start(
	ctx context.Context,
	runner *agent.Runner,
	conversation agentchat.Conversation,
	bookService *book.Service,
	req agentchat.ChatRequest,
	options agentrun.Options,
	emit func(agentrun.Event),
) (*AcceptedRun, error) {
	if h == nil || h.runtime == nil || h.engine == nil {
		return nil, fmt.Errorf("agent durable runtime is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	workspace := ""
	if bookService != nil {
		workspace = bookService.Workspace()
	}
	commandID := runstate.CommandID(strings.TrimSpace(req.CommandID))
	if commandID == "" {
		return nil, fmt.Errorf("%w: command_id is required", runstate.ErrInvalidCommand)
	}
	if err := h.runtime.ValidateCommandID(string(commandID)); err != nil {
		return nil, err
	}
	options = options.Normalize(workspace)
	req = agentchat.CaptureChatRequestCallerInput(req)
	binding, err := agentrun.BindingForOptions(options)
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

	command, turnRef, err := newHarnessStartTurn(binding, commandID, req, options)
	if err != nil {
		stopObserving()
		return nil, err
	}
	outcomes := make(chan agentrun.Outcome, 1)
	registration, err := h.engine.register(turnRef, command, TurnSpec{
		CommandID: agentrun.CommandID(commandID), CommandKind: CommandStartTurn,
		Runner: runner, Conversation: conversation, BookService: bookService,
		Request: req, Options: options, Emit: emit, Outcome: outcomes,
		CycleCommit: harnessCycleCommitForConversation(conversation),
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
	ephemeral := options.AgentKind == agentrun.AgentKindAutomation
	return &AcceptedRun{
		owner: h, harness: harness, observation: observation, receipt: receipt,
		conversation: conversation, options: options,
		outcomes: outcomes, emit: emit, stopObserving: stopObserving,
		registration: registration, binding: binding, ephemeralBinding: ephemeral,
	}, nil
}

func newHarnessStartTurn(
	binding runstate.BindingRef,
	commandID runstate.CommandID,
	req agentchat.ChatRequest,
	options agentrun.Options,
) (runstate.StartTurn, string, error) {
	if strings.TrimSpace(string(commandID)) == "" {
		return runstate.StartTurn{}, "", fmt.Errorf("%w: command_id is required", runstate.ErrInvalidCommand)
	}
	req = agentchat.CaptureChatRequestCallerInput(req)
	semanticSpec := CommandSpec{
		Kind: CommandStartTurn, CommandID: string(commandID),
		Request: req, Options: options,
	}
	turnRef := harnessCommandTurnRef(binding, string(commandID), harnessTurnSpecSemanticFingerprint(semanticSpec))
	caller := agentchat.CallerView(req)
	input, err := withHarnessInputMaterializationDescriptor(runstate.UserInput{
		Text: caller.Message, ContextRefs: harnessContextRefs(req), TurnSpecRef: turnRef,
	}, semanticSpec)
	if err != nil {
		return runstate.StartTurn{}, "", err
	}
	return runstate.StartTurn{
		ID:    commandID,
		Input: input,
	}, turnRef, nil
}

// StartTurnFingerprint derives the exact canonical fingerprint that
// StartWithOptions will submit, without opening a binding or registering any
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
	command, _, err := newHarnessStartTurn(binding, commandID, req, options)
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
