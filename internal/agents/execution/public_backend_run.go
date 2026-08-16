package execution

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	agentchat "denova/internal/agents/chat"
	agentlifecycle "denova/internal/agents/lifecycle"
	agentrun "denova/internal/agents/run"
	agenttool "denova/internal/agents/tool"
	agenttoolruntime "denova/internal/agents/toolruntime"

	agent "github.com/alfredxw/denova/agent"
)

func (backend *publicBackend) start(ctx context.Context, request StartRequest) (*Operation, error) {
	cycle := request.Cycle
	workspace := cycle.Options.Workspace
	if cycle.BookService != nil {
		workspace = cycle.BookService.Workspace()
	}
	cycle.Options = cycle.Options.Normalize(workspace)
	cycle.Request = agentchat.CaptureChatRequestCallerInput(cycle.Request)
	commandID := strings.TrimSpace(cycle.Request.CommandID)
	if commandID == "" {
		return nil, errors.New("Denova public Agent Start requires command_id")
	}
	if err := agentrun.ValidateCommandID(commandID); err != nil {
		return nil, err
	}
	key, err := agentrun.AgentSessionKeyForOptions(cycle.Options)
	if err != nil {
		return nil, err
	}
	sessionHandle, err := backend.agent.Session(ctx, key)
	if err != nil {
		return nil, err
	}
	if err := syncCanonicalTranscript(ctx, sessionHandle, cycle.Conversation); err != nil {
		return nil, err
	}
	registration := &publicCycleRegistration{
		cycle: &cycle, request: cycle.Request, options: cycle.Options, emit: request.Emit,
		commandKind:    CommandStartTurn,
		projector:      agentchat.NewPublicEventProjector(cycle.Conversation, cycle.Request, cycle.Options, request.Emit),
		projectorBound: cycle.Conversation != nil,
	}
	backend.rememberRegistration(key, commandID, registration)
	input, err := agentlifecycle.TurnInput(agentlifecycle.TurnStart, cycle.Request, cycle.Options)
	if err != nil {
		return nil, err
	}
	publicRun, err := sessionHandle.Run(ctx, input)
	if err != nil {
		return nil, err
	}
	handle := backend.trackRun(sessionHandle, publicRun, registration, "")
	return &Operation{
		publicBackend: backend, publicHandle: handle, publicReceipt: mapPublicReceipt(publicRun),
	}, nil
}

func syncCanonicalTranscript(
	ctx context.Context,
	session *agent.Session,
	conversation agentchat.Conversation,
) error {
	synchronizer, ok := conversation.(CanonicalTranscriptSynchronizer)
	if !ok {
		return nil
	}
	request, err := synchronizer.CanonicalTranscript(ctx)
	if err != nil {
		return err
	}
	_, err = session.SyncTranscript(ctx, request)
	return err
}

func (backend *publicBackend) submit(ctx context.Context, spec CommandRequest) (agentrun.CommandReceipt, error) {
	spec.Request = agentchat.CaptureChatRequestCallerInput(spec.Request)
	spec.Options = spec.Options.Normalize(spec.Options.Workspace)
	key, err := agentrun.AgentSessionKeyForOptions(spec.Options)
	if err != nil {
		return agentrun.CommandReceipt{}, err
	}
	commandID := strings.TrimSpace(spec.CommandID)
	if commandID == "" {
		return agentrun.CommandReceipt{}, errors.New("Denova public Agent command requires command_id")
	}
	if err := agentrun.ValidateCommandID(commandID); err != nil {
		return agentrun.CommandReceipt{}, err
	}
	backend.mu.RLock()
	target := backend.runs[string(spec.OperationID)]
	if target == nil {
		target = backend.runs[string(spec.AfterOperationID)]
	}
	backend.mu.RUnlock()
	if target == nil {
		sessionHandle, openErr := backend.agent.Session(ctx, key)
		if openErr != nil {
			return agentrun.CommandReceipt{}, openErr
		}
		runID := string(spec.OperationID)
		if runID == "" {
			runID = string(spec.AfterOperationID)
		}
		attached, found, attachErr := sessionHandle.AttachRun(ctx, runID)
		if attachErr != nil {
			return agentrun.CommandReceipt{}, attachErr
		}
		if !found {
			return agentrun.CommandReceipt{}, agent.ErrNoActiveRun
		}
		target = backend.trackRun(sessionHandle, attached, nil, "")
	}
	switch spec.Kind {
	case CommandAbort:
		receipt, err := target.run.Abort(ctx, agent.AbortRequest{
			Reason: spec.Reason, IdempotencyKey: commandID,
		})
		if err != nil {
			return agentrun.CommandReceipt{}, err
		}
		return mapPublicCommandReceipt(receipt), nil
	case CommandSteerQueued, CommandCancelQueued:
		queued, found, queuedErr := target.run.Queued(ctx, string(spec.TargetCommandID))
		if queuedErr != nil {
			return agentrun.CommandReceipt{}, queuedErr
		}
		if !found {
			return agentrun.CommandReceipt{}, agent.ErrNoActiveRun
		}
		control := agent.QueueControlRequest{IdempotencyKey: commandID, Reason: spec.Reason}
		var receipt agent.CommandReceipt
		if spec.Kind == CommandSteerQueued {
			receipt, queuedErr = queued.Interrupt(ctx, control)
		} else {
			receipt, queuedErr = queued.Cancel(ctx, control)
		}
		if queuedErr != nil {
			return agentrun.CommandReceipt{}, queuedErr
		}
		return mapPublicCommandReceipt(receipt), nil
	}
	spec.Request.CommandID = commandID
	registration := &publicCycleRegistration{request: spec.Request, options: spec.Options, emit: spec.Emit, commandKind: spec.Kind}
	backend.rememberRegistration(key, commandID, registration)
	turnKind, err := publicTurnKind(spec.Kind)
	if err != nil {
		return agentrun.CommandReceipt{}, err
	}
	input, err := agentlifecycle.TurnInput(turnKind, spec.Request, spec.Options)
	if err != nil {
		return agentrun.CommandReceipt{}, err
	}
	switch spec.Kind {
	case CommandSteer:
		receipt, err := target.run.Steer(ctx, input)
		if err != nil {
			return agentrun.CommandReceipt{}, err
		}
		return mapPublicCommandReceipt(receipt), nil
	case CommandFollowUp:
		queued, err := target.run.Queue(ctx, input)
		if err != nil {
			return agentrun.CommandReceipt{}, err
		}
		return mapPublicCommandReceipt(queued.Receipt()), nil
	case CommandNextTurn:
		next, err := target.run.FollowUp(ctx, input)
		if err != nil {
			return agentrun.CommandReceipt{}, err
		}
		backend.trackRun(target.session, next, registration, target.run.ID())
		return mapPublicReceipt(next), nil
	default:
		return agentrun.CommandReceipt{}, fmt.Errorf("unsupported Denova public Agent command %q", spec.Kind)
	}
}

func (backend *publicBackend) trackRun(
	sessionHandle *agent.Session,
	publicRun *agent.Run,
	registration *publicCycleRegistration,
	parentRunID string,
) *publicRunHandle {
	trace := publicTraceForRun(registration, publicRun.ID())
	handle := &publicRunHandle{
		session: sessionHandle, run: publicRun, registration: registration, trace: trace, done: make(chan struct{}),
	}
	backend.mu.Lock()
	backend.runs[publicRun.ID()] = handle
	if parentRunID != "" {
		backend.successors[parentRunID] = handle
	}
	backend.mu.Unlock()
	go func() {
		defer close(handle.done)
		defer func() {
			if err := trace.close(); err != nil {
				slog.Warn("[agent-public-runtime] close run trace failed", "run_id", publicRun.ID(), "error", err)
			}
		}()
		defer func() {
			if recovered := recover(); recovered != nil {
				slog.Error("[agent-public-runtime] event projection panicked", "run_id", publicRun.ID(), "panic", recovered)
			}
		}()
		currentCycle := 0
		for event := range publicRun.Events() {
			started, cycleStarted := event.Payload.(agent.RunStarted)
			if cycleStarted {
				currentCycle = started.Cycle
				cycleRegistration := backend.bindStartedCycleRegistration(
					publicRun.ID(), sessionHandle.Key(), started, registration,
				)
				if cycleRegistration != nil {
					cycleRegistration.projectOrDeferRunStarted(event.RunID, started)
				}
			}
			cycleRegistration := backend.cycleRegistration(publicRun.ID(), currentCycle, registration)
			if err := trace.record(cycleRegistration, event); err != nil {
				slog.Warn("[agent-public-runtime] record run trace failed", "run_id", publicRun.ID(), "error", err)
			}
			projector := projectorForRegistration(cycleRegistration)
			if projector != nil {
				projector.Project(event)
			}
		}
	}()
	return handle
}

func (backend *publicBackend) bindStartedCycleRegistration(
	runID string,
	key agent.SessionKey,
	started agent.RunStarted,
	fallback *publicCycleRegistration,
) *publicCycleRegistration {
	registration := backend.registration(key, started.CommandID)
	if registration == nil {
		registration = fallback
	}
	if registration == nil || started.Cycle <= 0 {
		return registration
	}
	backend.mu.Lock()
	if backend.cycles[runID] == nil {
		backend.cycles[runID] = make(map[int]*publicCycleRegistration)
	}
	backend.cycles[runID][started.Cycle] = registration
	backend.mu.Unlock()
	return registration
}

func (registration *publicCycleRegistration) projectOrDeferRunStarted(runID string, started agent.RunStarted) {
	registration.mu.Lock()
	projector := registration.projector
	if projector == nil {
		registration.pendingRunStart = &pendingPublicRunStart{runID: runID, started: started}
		registration.mu.Unlock()
		return
	}
	commandID := firstPublicCycleValue(started.CommandID, registration.request.CommandID)
	delivery := firstPublicCycleValue(started.Delivery, string(registration.commandKind))
	registration.mu.Unlock()
	projector.ProjectRunStarted(runID, started.Cycle, commandID, delivery, started.StartedAt)
}

func (backend *publicBackend) projector(
	runID string,
	cycle int,
	fallback *publicCycleRegistration,
) *agentchat.PublicEventProjector {
	return projectorForRegistration(backend.cycleRegistration(runID, cycle, fallback))
}

func (backend *publicBackend) cycleRegistration(
	runID string,
	cycle int,
	fallback *publicCycleRegistration,
) *publicCycleRegistration {
	backend.mu.RLock()
	registration := backend.cycles[runID][cycle]
	backend.mu.RUnlock()
	if registration == nil {
		return fallback
	}
	return registration
}

func projectorForRegistration(registration *publicCycleRegistration) *agentchat.PublicEventProjector {
	if registration == nil {
		return nil
	}
	registration.mu.RLock()
	defer registration.mu.RUnlock()
	return registration.projector
}

func (backend *publicBackend) wait(
	ctx context.Context,
	handle *publicRunHandle,
) agentrun.Outcome {
	if ctx == nil {
		ctx = context.Background()
	}
	current := handle
	for current != nil {
		result, err := current.run.Wait(ctx)
		if ctx != nil && ctx.Err() != nil {
			_, abortErr := current.run.Abort(context.Background(), agent.AbortRequest{Reason: "Denova display task was cancelled"})
			if abortErr != nil && !errors.Is(abortErr, agent.ErrRunSettled) && !errors.Is(abortErr, agent.ErrAgentClosed) {
				return agentrun.NewOutcome(agentrun.OutcomeFailed, abortErr, abortErr.Error(), "", "")
			}
			result, err = current.run.Wait(context.Background())
		}
		select {
		case <-current.done:
		case <-ctx.Done():
			return agentrun.NewOutcome(agentrun.OutcomeAborted, ctx.Err(), ctx.Err().Error(), "", "")
		}
		registrations := backend.runCycleRegistrations(current.run.ID(), current.registration)
		content, thinking := latestProjectedOutput(registrations)
		outcome := publicResultOutcome(result, err, content, thinking)
		if outcome.Status != agentrun.OutcomeCompleted {
			flushPublicCycleProjectors(registrations, result.Status, result.Reason, true)
			return outcome
		}
		for _, registration := range registrations {
			registration.registration.verifyMutations(ctx)
		}
		registration := current.registration
		if registration != nil {
			registration.mu.RLock()
			cycle := registration.cycle
			registration.mu.RUnlock()
			if cycle != nil && cycle.Successor != nil {
				if successorErr := cycle.Successor(ctx, agentrun.OperationID(current.run.ID()), outcome); successorErr != nil {
					return agentrun.NewOutcome(agentrun.OutcomeFailed, successorErr, successorErr.Error(), content, thinking)
				}
			}
		}
		backend.mu.RLock()
		next := backend.successors[current.run.ID()]
		backend.mu.RUnlock()
		if next == nil {
			flushPublicCycleProjectors(registrations, result.Status, result.Reason, true)
			return outcome
		}
		flushPublicCycleProjectors(registrations, result.Status, result.Reason, false)
		current = next
	}
	return agentrun.NewOutcome(agentrun.OutcomeFailed, errors.New("Denova public Agent run disappeared"), "run disappeared", "", "")
}

type publicCycleRegistrationAt struct {
	cycle        int
	registration *publicCycleRegistration
}

func (backend *publicBackend) runCycleRegistrations(runID string, fallback *publicCycleRegistration) []publicCycleRegistrationAt {
	backend.mu.RLock()
	byCycle := backend.cycles[runID]
	result := make([]publicCycleRegistrationAt, 0, len(byCycle)+1)
	for cycle, registration := range byCycle {
		if registration != nil {
			result = append(result, publicCycleRegistrationAt{cycle: cycle, registration: registration})
		}
	}
	backend.mu.RUnlock()
	if len(result) == 0 && fallback != nil {
		result = append(result, publicCycleRegistrationAt{cycle: 0, registration: fallback})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].cycle < result[j].cycle })
	return result
}

func latestProjectedOutput(registrations []publicCycleRegistrationAt) (string, string) {
	for index := len(registrations) - 1; index >= 0; index-- {
		registration := registrations[index].registration
		registration.mu.RLock()
		projector := registration.projector
		registration.mu.RUnlock()
		if projector == nil {
			continue
		}
		content, thinking := projector.Output()
		if content != "" || thinking != "" {
			return content, thinking
		}
	}
	return "", ""
}

func flushPublicCycleProjectors(registrations []publicCycleRegistrationAt, status agent.ResultStatus, reason string, terminal bool) {
	for index, item := range registrations {
		item.registration.mu.RLock()
		projector := item.registration.projector
		item.registration.mu.RUnlock()
		if projector == nil {
			continue
		}
		if index != len(registrations)-1 {
			projector.Flush()
		} else if terminal {
			projector.Finalize(status, reason)
		} else {
			projector.SummarizeRun(status)
		}
	}
}

func (registration *publicCycleRegistration) recordMutation(request agent.EffectRequest, mutation agenttool.Mutation) {
	if registration == nil {
		return
	}
	id := strings.TrimSpace(request.ID)
	if id == "" {
		return
	}
	registration.mu.Lock()
	defer registration.mu.Unlock()
	if registration.mutations == nil {
		registration.mutations = make(map[string]agenttool.Mutation)
	}
	if _, exists := registration.mutations[id]; exists {
		return
	}
	mutation.LoreItemIDs = append([]string(nil), mutation.LoreItemIDs...)
	mutation.DeletedLoreItemIDs = append([]string(nil), mutation.DeletedLoreItemIDs...)
	registration.mutations[id] = mutation
	registration.mutationOrder = append(registration.mutationOrder, id)
}

func (registration *publicCycleRegistration) verifyMutations(ctx context.Context) {
	if registration == nil {
		return
	}
	registration.mu.Lock()
	if registration.verificationDone {
		registration.mu.Unlock()
		return
	}
	registration.verificationDone = true
	mutations := make([]agenttool.Mutation, 0, len(registration.mutationOrder))
	for _, id := range registration.mutationOrder {
		mutations = append(mutations, registration.mutations[id])
	}
	cycle := registration.cycle
	options := registration.options
	projector := registration.projector
	registration.mu.Unlock()
	if len(mutations) == 0 || cycle == nil {
		return
	}
	verification := agenttool.VerifyPostRunMutations(cycle.BookService, mutations)
	verification = agenttoolruntime.ApplyMutationWarnings(options, verification, nil)
	if projector != nil && (verification.Mutations > 0 || len(verification.Warnings) > 0) {
		projector.EmitProduct(agentrun.Event{Type: "post_run_verification", Data: verification})
		projector.EmitProduct(agentrun.Event{Type: "verification", Data: verification})
	}
	if options.OnMutationsVerified != nil {
		invokeMutationVerificationCallback(options.OnMutationsVerified, context.WithoutCancel(ctx), mutations, verification)
	}
}

func invokeMutationVerificationCallback(
	callback func(context.Context, []agenttool.Mutation, agenttool.Verification),
	ctx context.Context,
	mutations []agenttool.Mutation,
	verification agenttool.Verification,
) {
	defer func() {
		if recovered := recover(); recovered != nil {
			slog.ErrorContext(ctx, "[agent-public-runtime] mutation verification callback panicked", "panic", recovered)
		}
	}()
	callback(ctx, mutations, verification)
}
