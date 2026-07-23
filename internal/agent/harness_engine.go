package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/cloudwego/eino/adk"

	"denova/internal/agentruntime"
	"denova/internal/book"
)

// HarnessCycleCommitter lets a domain conversation publish or reconcile its
// projection at the same boundary where the durable harness settles a cycle.
// The runtime calls it once for every accepted Start/Steer/FollowUp cycle.
type HarnessCycleCommitter interface {
	CommitAgentCycle(context.Context, RunOutcome) error
}

// HarnessCycleIdentity is the durable coordinator identity of one accepted
// model cycle. Domain conversations may persist it beside their own canonical
// commit so retries and crash reconciliation never infer identity from display
// events or process-local task IDs.
type HarnessCycleIdentity struct {
	CommandID   agentruntime.CommandID
	OperationID agentruntime.OperationID
	Cycle       int
}

// HarnessCycleIdentityBinder is an optional domain seam invoked after the
// coordinator has selected the exact operation/cycle and before model effects
// begin.
type HarnessCycleIdentityBinder interface {
	BindAgentCycleIdentity(HarnessCycleIdentity)
}

// HarnessAgentKindBinder aligns canonical Session input metadata with the
// accepted RunOptions profile before model-context assembly.
type HarnessAgentKindBinder interface {
	BindHarnessAgentKind(string)
}

// HarnessTurnExecution contains the execution-time dependencies for one
// already accepted command. Queued Steer/FollowUp commands use Prepare to
// build this value only when the actor dequeues their cycle, so dynamic
// workspace context includes effects committed by the preceding cycle.
type HarnessTurnExecution struct {
	Runner       *adk.Runner
	Conversation Conversation
	BookService  *book.Service
	Request      ChatRequest
	Options      RunOptions
}

type HarnessTurnPreparer func(context.Context) (HarnessTurnExecution, error)

func harnessCycleCommitForConversation(conversation Conversation) func(context.Context, RunOutcome) error {
	committer, ok := conversation.(HarnessCycleCommitter)
	if !ok || committer == nil {
		return nil
	}
	return committer.CommitAgentCycle
}

// HarnessTurnSpec is the bounded, typed adapter state needed to execute one
// harness cycle through the existing Agent runtime. The durable harness stores
// only its reference; the registration lease keeps heavier process-local values
// until Engine.Run consumes them exactly once.
type HarnessTurnSpec struct {
	// CommandID correlates the cycle with the user command that selected its
	// TurnSpec. It is display metadata only; durable command identity remains in
	// the coordinator journal.
	CommandID    agentruntime.CommandID
	CommandKind  AgentCommandKind
	Runner       *adk.Runner
	Conversation Conversation
	BookService  *book.Service
	Request      ChatRequest
	Options      RunOptions
	Emit         func(Event)
	// Prepare is called exactly once by Engine.Run after dequeue and before any
	// model/tool effect. Binding identity remains fixed by Options above.
	Prepare HarnessTurnPreparer
	// CycleCommit is the application commit boundary for one model cycle. It is
	// invoked exactly once after the legacy runtime has finished persisting its
	// conversation output and before the durable coordinator settles the cycle.
	// A commit failure fails the operation instead of acknowledging output whose
	// domain projection (for example a game turn event) was not completed.
	CycleCommit func(context.Context, RunOutcome) error
	// Outcome optionally reports the legacy runtime terminal value to the
	// coordinator. Callers must provide room for one value so Engine.Run never
	// depends on a separate consumer goroutine making progress.
	Outcome chan<- RunOutcome
}

// harnessEngine adapts the existing Agent runtime to agentruntime.Engine. It
// also implements EngineFactory; one concurrent registry can therefore serve
// every binding opened by an agentruntime.Runtime.
type harnessEngine struct {
	runtime                *turnExecutor
	turnRestorer           HarnessTurnRestorer
	structuralRestorer     HarnessStructuralRestorer
	domainCommitReconciler HarnessDomainCommitReconciler
	inputMaterializer      HarnessInputMaterializer
	hostEffectReconciler   HarnessHostEffectReconciler

	mu    sync.Mutex
	specs map[string]*harnessTurnSpecRegistration
}

// ErrHarnessBindingMismatch reports an adapter request or turn specification
// that does not belong to the exact durable binding used to create the engine.
// The check runs before model or tool effects so a shared registry can never
// route one profile's process-local dependencies into another profile's actor.
var ErrHarnessBindingMismatch = errors.New("agent harness binding mismatch")

type bindingHarnessEngine struct {
	owner   *harnessEngine
	binding agentruntime.BindingRef

	recoveryMu    sync.RWMutex
	recoveryRoute recoveryDisplayRoute
}

func newHarnessEngine(runtime *turnExecutor, turnRestorers ...HarnessTurnRestorer) *harnessEngine {
	if runtime == nil {
		runtime = newTurnExecutor(DefaultLoopPolicy())
	}
	var turnRestorer HarnessTurnRestorer
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
// profile and lifecycle identity selected by agentruntime.Runtime.Open.
func (e *harnessEngine) NewEngine(_ context.Context, binding agentruntime.BindingRef) (agentruntime.Engine, error) {
	if e == nil {
		return nil, fmt.Errorf("create agent harness engine: engine is nil")
	}
	return &bindingHarnessEngine{owner: e, binding: binding}, nil
}

func (e *bindingHarnessEngine) Run(
	ctx context.Context,
	request agentruntime.EngineRequest,
	emit agentruntime.EngineEventSink,
) (agentruntime.EngineResult, error) {
	if e == nil || e.owner == nil {
		return agentruntime.EngineResult{}, fmt.Errorf("run agent harness engine: engine is nil")
	}
	if request.Binding != e.binding || request.Snapshot.Binding != e.binding {
		return agentruntime.EngineResult{}, fmt.Errorf(
			"%w: factory=%+v request=%+v snapshot=%+v",
			ErrHarnessBindingMismatch,
			e.binding,
			request.Binding,
			request.Snapshot.Binding,
		)
	}
	return e.owner.run(ctx, request, emit, &e.binding)
}

func (e *bindingHarnessEngine) ReleasePendingInput(ctx context.Context, input agentruntime.UserInput) {
	if e == nil || e.owner == nil {
		return
	}
	e.owner.ReleasePendingInput(ctx, input)
}

func (e *bindingHarnessEngine) RestorePendingInput(ctx context.Context, input agentruntime.QueuedInput) error {
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
	request agentruntime.DomainCommitReconcileRequest,
) (agentruntime.DomainCommitReconcileResult, error) {
	if e == nil || e.owner == nil {
		return agentruntime.DomainCommitReconcileResult{}, fmt.Errorf("reconcile agent harness domain commit: engine is nil")
	}
	if request.Binding != e.binding {
		return agentruntime.DomainCommitReconcileResult{}, fmt.Errorf(
			"%w: factory=%+v request=%+v",
			ErrHarnessBindingMismatch,
			e.binding,
			request.Binding,
		)
	}
	if e.owner.domainCommitReconciler == nil {
		return agentruntime.DomainCommitReconcileResult{}, nil
	}
	return e.owner.domainCommitReconciler(ctx, request)
}

func (e *bindingHarnessEngine) ReconcileHostEffect(ctx context.Context, effect agentruntime.HostEffect) error {
	if e == nil || e.owner == nil {
		return fmt.Errorf("reconcile agent harness host effect: engine is nil")
	}
	mutation, err := decodeCommittedToolMutationHostEffect(e.binding, effect)
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
	request agentruntime.EngineRequest,
	emit agentruntime.EngineEventSink,
	expectedBinding *agentruntime.BindingRef,
) (agentruntime.EngineResult, error) {
	if e == nil {
		return agentruntime.EngineResult{}, fmt.Errorf("run agent harness engine: engine is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if emit == nil {
		return agentruntime.EngineResult{}, fmt.Errorf("run agent harness engine: event sink is required")
	}

	spec, err := e.take(request.Snapshot.Input.TurnSpecRef)
	if err != nil {
		return agentruntime.EngineResult{}, err
	}
	if expectedBinding != nil {
		if err := validateHarnessTurnBinding(spec.Options, *expectedBinding); err != nil {
			return agentruntime.EngineResult{}, err
		}
		if spec.Prepare == nil {
			if err := validateHarnessTurnExecutable(spec); err != nil {
				return agentruntime.EngineResult{}, err
			}
		}
	}
	runtime := e.runtime
	if runtime == nil {
		runtime = newTurnExecutor(DefaultLoopPolicy())
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	sink := &harnessEngineSink{emit: emit, cancel: cancel}
	if spec.Prepare != nil {
		var prepared HarnessTurnSpec
		preparationControl, prepareErr := prepareHarnessCycleWithControls(runCtx, request.Controls, harnessCyclePrepareFunc(func(prepareCtx context.Context) error {
			var materializeErr error
			prepared, materializeErr = materializeHarnessTurn(prepareCtx, spec)
			return materializeErr
		}))
		if preparationControl != nil {
			cancel()
			outcome, controlErr := harnessPreparationControlOutcome(*preparationControl)
			if controlErr != nil {
				return agentruntime.EngineResult{}, controlErr
			}
			reportHarnessOutcome(ctx, spec, outcome)
			return harnessEngineResult(outcome)
		}
		if prepareErr != nil {
			outcome := outcomeFromOutput(RunOutcomeFailed, prepareErr, prepareErr.Error(), "", "")
			if spec.Emit != nil {
				spec.Emit(Event{Type: "error", Data: map[string]string{"message": prepareErr.Error()}})
			}
			reportHarnessOutcome(ctx, spec, outcome)
			cancel()
			return agentruntime.EngineResult{}, prepareErr
		}
		spec = prepared
		if expectedBinding != nil {
			if err := validateHarnessTurnBinding(spec.Options, *expectedBinding); err != nil {
				cancel()
				return agentruntime.EngineResult{}, err
			}
			if err := validateHarnessTurnExecutable(spec); err != nil {
				cancel()
				return agentruntime.EngineResult{}, err
			}
		}
	}
	// Deferred cycles replace process-local Options during preparation. Build
	// the durable tool-effect adapter only after that replacement so its payload
	// carries the restored stable task/run identity.
	var toolObserver ToolLifecycleObserver = harnessToolLifecycleObserver{
		sink: sink, binding: request.Binding, operationID: request.Snapshot.OperationID,
		cycle: request.Snapshot.Cycle, options: spec.Options,
	}
	runCtx = ContextWithToolLifecycleObserver(runCtx, toolObserver)
	if binder, ok := spec.Conversation.(HarnessCycleIdentityBinder); ok && binder != nil {
		binder.BindAgentCycleIdentity(HarnessCycleIdentity{
			CommandID: spec.CommandID, OperationID: request.Snapshot.OperationID, Cycle: request.Snapshot.Cycle,
		})
	}
	if binder, ok := spec.Conversation.(HarnessAgentKindBinder); ok && binder != nil {
		binder.BindHarnessAgentKind(spec.Options.AgentKind)
	}
	participant, hasDomainCommit := spec.Conversation.(HarnessDomainCommitParticipant)
	if inputBinder, ok := spec.Conversation.(HarnessInputDomainCommitBinder); ok && inputBinder != nil && hasDomainCommit {
		inputBinder.BindAgentCycleInputCommit(func() error {
			return coordinateHarnessDomainCommit(runCtx, emit, participant, HarnessDomainCommitInput, RunOutcome{Status: RunOutcomeCompleted})
		})
	}
	if preparer, ok := spec.Conversation.(HarnessCyclePreparer); ok && preparer != nil {
		preparationControl, prepareErr := prepareHarnessCycleWithControls(runCtx, request.Controls, preparer)
		if preparationControl != nil {
			cancel()
			outcome, controlErr := harnessPreparationControlOutcome(*preparationControl)
			if controlErr != nil {
				return agentruntime.EngineResult{}, controlErr
			}
			reportHarnessOutcome(ctx, spec, outcome)
			return harnessEngineResult(outcome)
		}
		if prepareErr != nil {
			outcome := outcomeFromOutput(RunOutcomeFailed, prepareErr, prepareErr.Error(), "", "")
			if spec.Emit != nil {
				spec.Emit(Event{Type: "error", Data: map[string]string{"message": prepareErr.Error()}})
			}
			reportHarnessOutcome(ctx, spec, outcome)
			cancel()
			return agentruntime.EngineResult{}, prepareErr
		}
	}
	// The legacy control bridge starts only after preparation. During preparation
	// there is no turnExecutor.Run consumer, so forwarding to its channel would make
	// Abort/Steer deadlock lifecycle shutdown behind an unconsumed send.
	controls, bridgeDone := bridgeHarnessControls(runCtx, request.Controls, cancel)
	spec.Options.Controls = controls
	if spec.Emit != nil {
		spec.Emit(Event{Type: "agent_cycle_started", Data: map[string]any{
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
		func(event Event) {
			// "done" is an operation terminal signal in the legacy UI protocol,
			// while one harness operation may contain several Eino cycles after
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
		commitErr = coordinateHarnessDomainCommit(ctx, emit, participant, HarnessDomainCommitOutput, outcome)
	} else {
		commitErr = commitHarnessCycle(ctx, spec.CycleCommit, outcome)
	}
	if commitErr != nil {
		outcome = outcomeFromOutput(RunOutcomeFailed, commitErr, commitErr.Error(), outcome.Content, outcome.Thinking)
		if spec.Emit != nil {
			spec.Emit(Event{Type: "error", Data: map[string]string{"message": commitErr.Error()}})
		}
	}
	reportHarnessOutcome(ctx, spec, outcome)
	cancel()
	bridgeErr := <-bridgeDone
	if sinkErr := sink.failure(); sinkErr != nil {
		return agentruntime.EngineResult{}, sinkErr
	}
	if bridgeErr != nil {
		return agentruntime.EngineResult{}, bridgeErr
	}
	if err := sink.send(agentruntime.EngineAssistantFinal{
		Content: outcome.Content, Thinking: outcome.Thinking,
	}); err != nil {
		return agentruntime.EngineResult{}, err
	}
	return harnessEngineResult(outcome)
}

func validateHarnessTurnBinding(options RunOptions, expected agentruntime.BindingRef) error {
	binding, err := harnessBindingForOptions(options)
	if err != nil {
		return fmt.Errorf("%w: resolve turn profile: %v", ErrHarnessBindingMismatch, err)
	}
	actual, err := agentruntime.BindingReference(binding)
	if err != nil {
		return fmt.Errorf("%w: resolve turn binding: %v", ErrHarnessBindingMismatch, err)
	}
	if actual != expected {
		return fmt.Errorf("%w: turn=%+v runtime=%+v", ErrHarnessBindingMismatch, actual, expected)
	}
	return nil
}

func validateHarnessTurnExecutable(spec HarnessTurnSpec) error {
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

func materializeHarnessTurn(ctx context.Context, spec HarnessTurnSpec) (HarnessTurnSpec, error) {
	if spec.Prepare == nil {
		return spec, nil
	}
	execution, err := spec.Prepare(ctx)
	if err != nil {
		return HarnessTurnSpec{}, fmt.Errorf("materialize queued agent turn: %w", err)
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

func preserveHarnessBindingOptions(binding, execution RunOptions) RunOptions {
	execution.AgentKind = binding.AgentKind
	execution.TaskID = binding.TaskID
	execution.AutomationTaskID = binding.AutomationTaskID
	execution.SessionID = binding.SessionID
	execution.StoryID = binding.StoryID
	execution.BranchID = binding.BranchID
	execution.Workspace = binding.Workspace
	execution.Mode = binding.Mode
	return execution
}

func harnessEngineResult(outcome RunOutcome) (agentruntime.EngineResult, error) {
	switch outcome.Status {
	case RunOutcomeCompleted:
		return agentruntime.EngineResult{Status: agentruntime.EngineCompleted}, outcome.Error
	case RunOutcomePreempted:
		return agentruntime.EngineResult{Status: agentruntime.EnginePreempted}, outcome.Error
	case RunOutcomeAborted:
		return agentruntime.EngineResult{Status: agentruntime.EngineAborted}, outcome.Error
	case RunOutcomeFailed:
		err := outcome.Error
		if err == nil {
			reason := strings.TrimSpace(outcome.Reason)
			if reason == "" {
				reason = "agent run failed"
			}
			err = errors.New(reason)
		}
		return agentruntime.EngineResult{}, err
	default:
		return agentruntime.EngineResult{}, fmt.Errorf("unsupported agent run outcome status %q", outcome.Status)
	}
}

var _ agentruntime.EngineFactory = (*harnessEngine)(nil)
var _ agentruntime.Engine = (*bindingHarnessEngine)(nil)
var _ agentruntime.EnginePendingInputReleaser = (*bindingHarnessEngine)(nil)
var _ agentruntime.EnginePendingInputRestorer = (*bindingHarnessEngine)(nil)
var _ agentruntime.EngineRecoveryContextBinder = (*bindingHarnessEngine)(nil)
var _ agentruntime.EngineRecoveryContextUnbinder = (*bindingHarnessEngine)(nil)
var _ agentruntime.EngineDomainCommitReconciler = (*bindingHarnessEngine)(nil)
