package harness

import (
	"context"
	agentchat "denova/internal/agents/chat"
	agentrun "denova/internal/agents/run"
	agenttoolruntime "denova/internal/agents/toolruntime"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	agent "github.com/alfredxw/denova/agent"

	"denova/internal/book"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

// TurnExecution contains the execution-time dependencies for one
// already accepted command. Queued Steer/FollowUp commands use Prepare to
// build this value only when the actor dequeues their cycle, so dynamic
// workspace context includes effects committed by the preceding cycle.
type TurnExecution struct {
	Runner       *agent.Runner
	Conversation agentchat.Conversation
	BookService  *book.Service
	Request      agentchat.ChatRequest
	Options      agentrun.Options
}

type TurnPreparer func(context.Context) (TurnExecution, error)

func harnessCycleCommitForConversation(conversation agentchat.Conversation) func(context.Context, agentrun.Outcome) error {
	committer, ok := conversation.(agentrun.CycleCommitter)
	if !ok || committer == nil {
		return nil
	}
	return committer.CommitAgentCycle
}

// TurnSpec is the bounded, typed adapter state needed to execute one
// harness cycle through the existing Agent runtime. The durable harness stores
// only its reference; the registration lease keeps heavier process-local values
// until Engine.Run consumes them exactly once.
type TurnSpec struct {
	// agentrun.CommandID correlates the cycle with the user command that selected its
	// TurnSpec. It is display metadata only; durable command identity remains in
	// the coordinator journal.
	CommandID    agentrun.CommandID
	CommandKind  CommandKind
	Runner       *agent.Runner
	Conversation agentchat.Conversation
	BookService  *book.Service
	Request      agentchat.ChatRequest
	Options      agentrun.Options
	Emit         func(agentrun.Event)
	// Prepare is called exactly once by Engine.Run after dequeue and before any
	// model/tool effect. Binding identity remains fixed by Options above.
	Prepare TurnPreparer
	// CycleCommit is the application commit boundary for one model cycle. It is
	// invoked exactly once after the legacy runtime has finished persisting its
	// conversation output and before the durable coordinator settles the cycle.
	// A commit failure fails the operation instead of acknowledging output whose
	// domain projection (for example a game turn event) was not completed.
	CycleCommit func(context.Context, agentrun.Outcome) error
	// Outcome optionally reports the legacy runtime terminal value to the
	// coordinator. Callers must provide room for one value so Engine.Run never
	// depends on a separate consumer goroutine making progress.
	Outcome chan<- agentrun.Outcome
}

// harnessEngine adapts the existing Agent runtime to runstate.Engine. It
// also implements EngineFactory; one concurrent registry can therefore serve
// every binding opened by an runstate.Runtime.
type harnessEngine struct {
	runtime                *agentchat.Executor
	turnRestorer           TurnRestorer
	structuralRestorer     StructuralRestorer
	domainCommitReconciler DomainCommitReconciler
	inputMaterializer      InputMaterializer
	hostEffectReconciler   agenttoolruntime.HarnessHostEffectReconciler

	mu    sync.Mutex
	specs map[string]*harnessTurnSpecRegistration
}

// ErrBindingMismatch reports an adapter request or turn specification
// that does not belong to the exact durable binding used to create the engine.
// The check runs before model or tool effects so a shared registry can never
// route one profile's process-local dependencies into another profile's actor.
var ErrBindingMismatch = errors.New("agent harness binding mismatch")

type bindingHarnessEngine struct {
	owner   *harnessEngine
	binding runstate.BindingRef

	recoveryMu    sync.RWMutex
	recoveryRoute recoveryDisplayRoute
}

func newHarnessEngine(runtime *agentchat.Executor, turnRestorers ...TurnRestorer) *harnessEngine {
	if runtime == nil {
		runtime = agentchat.NewExecutor(agentrun.DefaultLoopPolicy())
	}
	var turnRestorer TurnRestorer
	if len(turnRestorers) > 0 {
		turnRestorer = turnRestorers[0]
	}
	return &harnessEngine{
		runtime: runtime, turnRestorer: turnRestorer,
		specs: make(map[string]*harnessTurnSpecRegistration),
	}
}

// NewEngine returns a lightweight binding guard over the shared turn registry.
// Mutable adapter state remains shared, while every Run is fenced to the exact
// profile and lifecycle identity selected by runstate.Runtime.Open.
func (e *harnessEngine) NewEngine(_ context.Context, binding runstate.BindingRef) (runstate.Engine, error) {
	if e == nil {
		return nil, fmt.Errorf("create agent harness engine: engine is nil")
	}
	return &bindingHarnessEngine{owner: e, binding: binding}, nil
}

func (e *bindingHarnessEngine) Run(
	ctx context.Context,
	request runstate.EngineRequest,
	emit runstate.EngineEventSink,
) (runstate.EngineResult, error) {
	if e == nil || e.owner == nil {
		return runstate.EngineResult{}, fmt.Errorf("run agent harness engine: engine is nil")
	}
	if !request.Binding.Equal(e.binding) || !request.Snapshot.Binding.Equal(e.binding) {
		return runstate.EngineResult{}, fmt.Errorf(
			"%w: factory=%+v request=%+v snapshot=%+v",
			ErrBindingMismatch,
			e.binding,
			request.Binding,
			request.Snapshot.Binding,
		)
	}
	return e.owner.run(ctx, request, emit, &e.binding)
}

func (e *bindingHarnessEngine) ReleasePendingInput(ctx context.Context, input runstate.UserInput) {
	if e == nil || e.owner == nil {
		return
	}
	e.owner.ReleasePendingInput(ctx, input)
}

func (e *bindingHarnessEngine) RestorePendingInput(ctx context.Context, input runstate.QueuedInput) error {
	if e == nil || e.owner == nil {
		return fmt.Errorf("restore agent harness pending input: engine is nil")
	}
	return e.owner.restorePendingInput(e.withRecoveryRoute(ctx), e.binding, input)
}

func (e *bindingHarnessEngine) BindRecoveryContext(ctx context.Context) {
	if e == nil {
		return
	}
	route := recoveryDisplayRouteFromContext(ctx)
	if route.Emit == nil && route.TaskID == "" {
		return
	}
	e.recoveryMu.Lock()
	e.recoveryRoute = route
	e.recoveryMu.Unlock()
}

func (e *bindingHarnessEngine) UnbindRecoveryContext(ctx context.Context) {
	if e == nil {
		return
	}
	owner := recoveryDisplayRouteFromContext(ctx)
	if owner.TaskID == "" {
		return
	}
	e.recoveryMu.Lock()
	if e.recoveryRoute.TaskID == owner.TaskID {
		e.recoveryRoute = recoveryDisplayRoute{}
	}
	e.recoveryMu.Unlock()
}

func (e *bindingHarnessEngine) withRecoveryRoute(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	current := recoveryDisplayRouteFromContext(ctx)
	e.recoveryMu.RLock()
	bound := e.recoveryRoute
	e.recoveryMu.RUnlock()
	if current.Emit == nil {
		current.Emit = bound.Emit
	}
	if current.TaskID == "" {
		current.TaskID = bound.TaskID
	}
	return withRecoveryDisplayRoute(ctx, current)
}

func (e *bindingHarnessEngine) ReconcileDomainCommit(
	ctx context.Context,
	request runstate.DomainCommitReconcileRequest,
) (runstate.DomainCommitReconcileResult, error) {
	if e == nil || e.owner == nil {
		return runstate.DomainCommitReconcileResult{}, fmt.Errorf("reconcile agent harness domain commit: engine is nil")
	}
	if !request.Binding.Equal(e.binding) {
		return runstate.DomainCommitReconcileResult{}, fmt.Errorf(
			"%w: factory=%+v request=%+v",
			ErrBindingMismatch,
			e.binding,
			request.Binding,
		)
	}
	if e.owner.domainCommitReconciler == nil {
		return runstate.DomainCommitReconcileResult{}, nil
	}
	projected, err := agentrun.DomainCommitReconcileRequestFromRuntime(request)
	if err != nil {
		return runstate.DomainCommitReconcileResult{}, err
	}
	result, err := e.owner.domainCommitReconciler(ctx, projected)
	return agentrun.DomainCommitReconcileResultToRuntime(result), err
}

func (e *bindingHarnessEngine) ReconcileHostEffect(ctx context.Context, effect runstate.HostEffect) error {
	if e == nil || e.owner == nil {
		return fmt.Errorf("reconcile agent harness host effect: engine is nil")
	}
	mutation, err := agenttoolruntime.DecodeCommittedToolMutationHostEffect(e.binding, effect)
	if err != nil {
		return err
	}
	if e.owner.hostEffectReconciler == nil {
		return fmt.Errorf("reconcile agent harness host effect: host reconciler is unavailable")
	}
	return e.owner.hostEffectReconciler(ctx, mutation)
}

func (e *harnessEngine) run(
	ctx context.Context,
	request runstate.EngineRequest,
	emit runstate.EngineEventSink,
	expectedBinding *runstate.BindingRef,
) (runstate.EngineResult, error) {
	if e == nil {
		return runstate.EngineResult{}, fmt.Errorf("run agent harness engine: engine is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if emit == nil {
		return runstate.EngineResult{}, fmt.Errorf("run agent harness engine: event sink is required")
	}

	spec, err := e.take(request.Snapshot.Input.TurnSpecRef)
	if err != nil {
		return runstate.EngineResult{}, err
	}
	if expectedBinding != nil {
		if err := validateHarnessTurnBinding(spec.Options, *expectedBinding); err != nil {
			return runstate.EngineResult{}, err
		}
		if spec.Prepare == nil {
			if err := validateHarnessTurnExecutable(spec); err != nil {
				return runstate.EngineResult{}, err
			}
		}
	}
	runtime := e.runtime
	if runtime == nil {
		runtime = agentchat.NewExecutor(agentrun.DefaultLoopPolicy())
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	sink := &harnessEngineSink{emit: emit, cancel: cancel}
	if spec.Prepare != nil {
		var prepared TurnSpec
		preparationControl, prepareErr := prepareHarnessCycleWithControls(runCtx, request.Controls, harnessCyclePrepareFunc(func(prepareCtx context.Context) error {
			var materializeErr error
			prepared, materializeErr = materializeHarnessTurn(prepareCtx, spec)
			return materializeErr
		}))
		if preparationControl != nil {
			cancel()
			outcome, controlErr := harnessPreparationControlOutcome(*preparationControl)
			if controlErr != nil {
				return runstate.EngineResult{}, controlErr
			}
			reportHarnessOutcome(ctx, spec, outcome)
			return harnessEngineResult(outcome)
		}
		if prepareErr != nil {
			outcome := agentrun.NewOutcome(agentrun.OutcomeFailed, prepareErr, prepareErr.Error(), "", "")
			if spec.Emit != nil {
				spec.Emit(agentrun.Event{Type: "error", Data: map[string]string{"message": prepareErr.Error()}})
			}
			reportHarnessOutcome(ctx, spec, outcome)
			cancel()
			return runstate.EngineResult{}, prepareErr
		}
		spec = prepared
		if expectedBinding != nil {
			if err := validateHarnessTurnBinding(spec.Options, *expectedBinding); err != nil {
				cancel()
				return runstate.EngineResult{}, err
			}
			if err := validateHarnessTurnExecutable(spec); err != nil {
				cancel()
				return runstate.EngineResult{}, err
			}
		}
	}
	// Deferred cycles replace process-local Options during preparation. Build
	// the durable tool-effect adapter only after that replacement so its payload
	// carries the restored stable task/run identity.
	bindingIdentity, marshalErr := json.Marshal(request.Binding)
	if marshalErr != nil {
		return runstate.EngineResult{}, fmt.Errorf("encode Agent invocation binding: %w", marshalErr)
	}
	// Runtime never re-enters a crashed Running cycle: recovery pauses it and a
	// later Steer/FollowUp starts cycle+1. The in-cycle model-response ordinal can
	// therefore restart only for an exact explicit replay, where the same ordinal
	// intentionally derives the same ExecutionID. Future same-cycle resumption
	// must first persist and restore that ordinal at the runtime boundary.
	runCtx = agent.ContextWithInvocationIdentity(runCtx, agent.InvocationIdentity{
		Scope: string(bindingIdentity), OperationID: string(request.Snapshot.OperationID), Cycle: request.Snapshot.Cycle,
	})
	runCtx = agent.ContextWithSessionKey(runCtx, agentrun.SessionKeyForBinding(request.Binding))
	var toolObserver agenttoolruntime.ToolLifecycleObserver = harnessToolLifecycleObserver{
		sink: sink, binding: request.Binding, operationID: request.Snapshot.OperationID,
		cycle: request.Snapshot.Cycle, options: spec.Options,
	}
	runCtx = agenttoolruntime.ContextWithToolLifecycleObserver(runCtx, toolObserver)
	if binder, ok := spec.Conversation.(agentrun.CycleIdentityBinder); ok && binder != nil {
		binder.BindAgentCycleIdentity(agentrun.CycleIdentity{
			CommandID: spec.CommandID, OperationID: agentrun.OperationID(request.Snapshot.OperationID), Cycle: request.Snapshot.Cycle,
		})
	}
	if binder, ok := spec.Conversation.(agentrun.AgentKindBinder); ok && binder != nil {
		binder.BindHarnessAgentKind(spec.Options.AgentKind)
	}
	participant, hasDomainCommit := spec.Conversation.(agentrun.DomainCommitParticipant)
	if inputBinder, ok := spec.Conversation.(agentrun.InputDomainCommitBinder); ok && inputBinder != nil && hasDomainCommit {
		inputBinder.BindAgentCycleInputCommit(func() error {
			return coordinateHarnessDomainCommit(runCtx, emit, participant, agentrun.DomainCommitInput, agentrun.Outcome{Status: agentrun.OutcomeCompleted})
		})
	}
	if preparer, ok := spec.Conversation.(agentrun.CyclePreparer); ok && preparer != nil {
		preparationControl, prepareErr := prepareHarnessCycleWithControls(runCtx, request.Controls, preparer)
		if preparationControl != nil {
			cancel()
			outcome, controlErr := harnessPreparationControlOutcome(*preparationControl)
			if controlErr != nil {
				return runstate.EngineResult{}, controlErr
			}
			reportHarnessOutcome(ctx, spec, outcome)
			return harnessEngineResult(outcome)
		}
		if prepareErr != nil {
			outcome := agentrun.NewOutcome(agentrun.OutcomeFailed, prepareErr, prepareErr.Error(), "", "")
			if spec.Emit != nil {
				spec.Emit(agentrun.Event{Type: "error", Data: map[string]string{"message": prepareErr.Error()}})
			}
			reportHarnessOutcome(ctx, spec, outcome)
			cancel()
			return runstate.EngineResult{}, prepareErr
		}
	}
	// The legacy control bridge starts only after preparation. During preparation
	// there is no chat.Executor.Run consumer, so forwarding to its channel would make
	// Abort/Steer deadlock lifecycle shutdown behind an unconsumed send.
	controls, bridgeDone := bridgeHarnessControls(runCtx, request.Controls, cancel)
	spec.Options.Controls = controls
	if spec.Emit != nil {
		spec.Emit(agentrun.Event{Type: "agent_cycle_started", Data: map[string]any{
			"command_id":   string(spec.CommandID),
			"delivery":     string(spec.CommandKind),
			"message":      spec.Request.Message,
			"operation_id": string(request.Snapshot.OperationID),
			"cycle":        request.Snapshot.Cycle,
		}})
	}

	outcome := runtime.Run(
		runCtx,
		spec.Runner,
		spec.Conversation,
		spec.BookService,
		spec.Request,
		spec.Options,
		func(event agentrun.Event) {
			// "done" is an operation terminal signal in the legacy UI protocol,
			// while one harness operation may contain several Agent cycles after
			// steer/follow-up. The coordinator emits it once after durable settle.
			if spec.Emit != nil && event.Type != "done" {
				spec.Emit(event)
			}
			if mapped, ok := harnessDisplayEngineEvent(event); ok {
				_ = sink.send(mapped)
			}
		},
	)
	var commitErr error
	if hasDomainCommit {
		commitErr = coordinateHarnessDomainCommit(ctx, emit, participant, agentrun.DomainCommitOutput, outcome)
	} else {
		commitErr = commitHarnessCycle(ctx, spec.CycleCommit, outcome)
	}
	if commitErr != nil {
		outcome = agentrun.NewOutcome(agentrun.OutcomeFailed, commitErr, commitErr.Error(), outcome.Content, outcome.Thinking)
		if spec.Emit != nil {
			spec.Emit(agentrun.Event{Type: "error", Data: map[string]string{"message": commitErr.Error()}})
		}
	}
	reportHarnessOutcome(ctx, spec, outcome)
	cancel()
	bridgeErr := <-bridgeDone
	if sinkErr := sink.failure(); sinkErr != nil {
		return runstate.EngineResult{}, sinkErr
	}
	if bridgeErr != nil {
		return runstate.EngineResult{}, bridgeErr
	}
	if err := sink.send(runstate.EngineAssistantFinal{
		Content: outcome.Content, Thinking: outcome.Thinking,
	}); err != nil {
		return runstate.EngineResult{}, err
	}
	return harnessEngineResult(outcome)
}

func validateHarnessTurnBinding(options agentrun.Options, expected runstate.BindingRef) error {
	binding, err := agentrun.BindingForOptions(options)
	if err != nil {
		return fmt.Errorf("%w: resolve turn profile: %v", ErrBindingMismatch, err)
	}
	actual, err := runstate.BindingReference(binding)
	if err != nil {
		return fmt.Errorf("%w: resolve turn binding: %v", ErrBindingMismatch, err)
	}
	if !actual.Equal(expected) {
		return fmt.Errorf("%w: turn=%+v runtime=%+v", ErrBindingMismatch, actual, expected)
	}
	return nil
}

func validateHarnessTurnExecutable(spec TurnSpec) error {
	if spec.Runner == nil {
		return fmt.Errorf("%w: runner is required", ErrHarnessTurnSpecInvalid)
	}
	if spec.Conversation == nil {
		return fmt.Errorf("%w: conversation is required", ErrHarnessTurnSpecInvalid)
	}
	return nil
}

type harnessCyclePrepareFunc func(context.Context) error

func (prepare harnessCyclePrepareFunc) PrepareAgentCycle(ctx context.Context) error {
	if prepare == nil {
		return nil
	}
	return prepare(ctx)
}

func materializeHarnessTurn(ctx context.Context, spec TurnSpec) (TurnSpec, error) {
	if spec.Prepare == nil {
		return spec, nil
	}
	execution, err := spec.Prepare(ctx)
	if err != nil {
		return TurnSpec{}, fmt.Errorf("materialize queued agent turn: %w", err)
	}
	spec.Runner = execution.Runner
	spec.Conversation = execution.Conversation
	spec.BookService = execution.BookService
	spec.Request = execution.Request
	spec.Options = preserveHarnessBindingOptions(spec.Options, execution.Options)
	spec.CycleCommit = harnessCycleCommitForConversation(execution.Conversation)
	spec.Prepare = nil
	return spec, nil
}

func preserveHarnessBindingOptions(binding, execution agentrun.Options) agentrun.Options {
	execution.AgentKind = binding.AgentKind
	execution.ProjectID = binding.ProjectID
	execution.TaskID = binding.TaskID
	execution.AutomationTaskID = binding.AutomationTaskID
	execution.SessionID = binding.SessionID
	execution.StoryID = binding.StoryID
	execution.BranchID = binding.BranchID
	// Project content paths may change while a queued command is cold, and the
	// user-owned state root is always current runtime policy rather than command
	// semantics. Prepare resolves both. Legacy non-Project bindings still keep
	// their admitted content workspace.
	if strings.TrimSpace(binding.ProjectID) == "" {
		execution.Workspace = binding.Workspace
	}
	execution.Mode = binding.Mode
	execution.WriteMode = binding.WriteMode
	execution.WriteScope = binding.WriteScope
	return execution
}

func harnessEngineResult(outcome agentrun.Outcome) (runstate.EngineResult, error) {
	switch outcome.Status {
	case agentrun.OutcomeCompleted:
		return runstate.EngineResult{Status: runstate.EngineCompleted}, outcome.Error
	case agentrun.OutcomePreempted:
		return runstate.EngineResult{Status: runstate.EnginePreempted}, outcome.Error
	case agentrun.OutcomeAborted:
		return runstate.EngineResult{Status: runstate.EngineAborted}, outcome.Error
	case agentrun.OutcomeFailed:
		err := outcome.Error
		if err == nil {
			reason := strings.TrimSpace(outcome.Reason)
			if reason == "" {
				reason = "agent run failed"
			}
			err = errors.New(reason)
		}
		return runstate.EngineResult{}, err
	default:
		return runstate.EngineResult{}, fmt.Errorf("unsupported agent run outcome status %q", outcome.Status)
	}
}

var _ runstate.EngineFactory = (*harnessEngine)(nil)
var _ runstate.Engine = (*bindingHarnessEngine)(nil)
var _ runstate.EnginePendingInputReleaser = (*bindingHarnessEngine)(nil)
var _ runstate.EnginePendingInputRestorer = (*bindingHarnessEngine)(nil)
var _ runstate.EngineRecoveryContextBinder = (*bindingHarnessEngine)(nil)
var _ runstate.EngineRecoveryContextUnbinder = (*bindingHarnessEngine)(nil)
var _ runstate.EngineDomainCommitReconciler = (*bindingHarnessEngine)(nil)
