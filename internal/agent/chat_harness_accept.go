package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/alfredxw/denova/adk"

	runstate "denova/internal/agent/runtime"
	"denova/internal/book"
)

func (h *chatHarness) run(
	ctx context.Context,
	runner *adk.Runner,
	conversation Conversation,
	bookService *book.Service,
	req ChatRequest,
	options RunOptions,
	emit func(Event),
) RunOutcome {
	accepted, err := h.start(ctx, runner, conversation, bookService, req, options, emit)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return outcomeFromOutput(RunOutcomeAborted, err, err.Error(), "", "")
		}
		emitHarnessError(emit, err)
		return outcomeFromOutput(RunOutcomeFailed, err, err.Error(), "", "")
	}
	return accepted.Wait(ctx)
}

func (h *chatHarness) start(
	ctx context.Context,
	runner *adk.Runner,
	conversation Conversation,
	bookService *book.Service,
	req ChatRequest,
	options RunOptions,
	emit func(Event),
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
	options = options.normalized(workspace)
	req = CaptureChatRequestCallerInput(req)
	binding, err := harnessBindingForOptions(options)
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
	outcomes := make(chan RunOutcome, 1)
	registration, err := h.engine.register(turnRef, command, HarnessTurnSpec{
		CommandID: commandID, CommandKind: AgentCommandStartTurn,
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
	_, ephemeral := binding.(runstate.AutomationBinding)
	return &AcceptedRun{
		owner: h, harness: harness, observation: observation, receipt: receipt,
		conversation: conversation, options: options,
		outcomes: outcomes, emit: emit, stopObserving: stopObserving,
		registration: registration, binding: binding, ephemeralBinding: ephemeral,
	}, nil
}

func newHarnessStartTurn(
	binding runstate.Binding,
	commandID runstate.CommandID,
	req ChatRequest,
	options RunOptions,
) (runstate.StartTurn, string, error) {
	if strings.TrimSpace(string(commandID)) == "" {
		return runstate.StartTurn{}, "", fmt.Errorf("%w: command_id is required", runstate.ErrInvalidCommand)
	}
	req = CaptureChatRequestCallerInput(req)
	semanticSpec := AgentCommandSpec{
		Kind: AgentCommandStartTurn, CommandID: string(commandID),
		Request: req, Options: options,
	}
	turnRef := harnessCommandTurnRef(binding, string(commandID), harnessTurnSpecSemanticFingerprint(semanticSpec))
	caller := chatRequestCallerView(req)
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

// DurableStartTurnFingerprint derives the exact canonical fingerprint that
// StartWithOptions will submit, without opening a binding or registering any
// process-local execution state. Adapters persist it before admission so cold
// reconciliation can distinguish an exact replay from command-ID reuse.
func DurableStartTurnFingerprint(req ChatRequest, options RunOptions, workspace string) (string, error) {
	commandID := runstate.CommandID(strings.TrimSpace(req.CommandID))
	if commandID == "" {
		return "", fmt.Errorf("%w: command_id is required", runstate.ErrInvalidCommand)
	}
	options = options.normalized(workspace)
	req = CaptureChatRequestCallerInput(req)
	binding, err := harnessBindingForOptions(options)
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
func (h *chatHarness) submitStartWhenIdle(
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
