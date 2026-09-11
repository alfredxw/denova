package agent

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// loopConfig configures the provider-neutral model/tool loop.
type loopConfig struct {
	Name            string
	Description     string
	Instruction     string
	Model           BaseChatModel
	Tools           []ToolDefinition
	ResultProcessor ToolResultProcessor
	Artifacts       ToolArtifactStorage

	Middlewares      []Middleware
	Retry            *RetryConfig
	ModelMaxAttempts int

	// MaxIterations is an explicit caller-owned guard. Zero means unlimited;
	// the agent runtime never installs an implicit iteration limit.
	MaxIterations int
	IdleTimeout   time.Duration

	// ToolParallelism bounds one parallel-read stage. Zero or negative values
	// use defaultToolParallelism; values above maxToolParallelism are clamped.
	ToolParallelism int

	// modelCallGate is owned by the Agent lifecycle. It runs after all
	// caller middleware has formed the exact provider-neutral request and may
	// replace the call with the validated preparation after publishing a checkpoint.
	modelCallGate modelCallGate

	// permission is the Agent-owned authorization fence. It is intentionally
	// separate from caller Middleware: policy evaluation must happen before a
	// caller wrapper is invoked, and the exact authorized arguments must remain
	// unchanged until concrete execution.
	permission *permissionMiddleware
}

type preparedModelCall struct {
	ctx             context.Context
	call            *ModelCall
	modelContext    *ModelContext
	state           *RunState
	feedbackIndexes []int
}

type modelCallGate func(context.Context, *ModelCall, *ModelContext) (*preparedModelCall, error)

const (
	defaultToolParallelism = 8
	maxToolParallelism     = 64
)

// modelToolLoop owns one provider-neutral model/tool loop. Session and Run
// lifecycle are intentionally owned by the higher-level Agent module.
type modelToolLoop struct {
	name             string
	description      string
	instruction      string
	model            BaseChatModel
	tools            []ToolDefinition
	middlewares      []Middleware
	resultProcessor  ToolResultProcessor
	artifacts        ToolArtifactStorage
	retry            *RetryConfig
	modelMaxAttempts int
	maxIterations    int
	idleTimeout      time.Duration
	toolParallelism  int
	modelCallGate    modelCallGate
	permission       *permissionMiddleware
}

// errMaxIterations is returned only when the caller explicitly configures a limit.
var errMaxIterations = errors.New("agent reached configured maximum iterations")

// newModelToolLoop validates the model, tool registry, and middleware surface.
func newModelToolLoop(ctx context.Context, config loopConfig) (*modelToolLoop, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if config.Model == nil {
		return nil, errors.New("new agent: model is required")
	}
	tools := append([]ToolDefinition(nil), config.Tools...)
	if _, err := NewRegistry(ctx, tools...); err != nil {
		return nil, fmt.Errorf("new agent: %w", err)
	}
	middlewares := append([]Middleware(nil), config.Middlewares...)
	for index, middleware := range middlewares {
		if middleware == nil {
			return nil, fmt.Errorf("new agent: nil middleware at index %d", index)
		}
	}
	if config.ResultProcessor != nil {
		if err := config.ResultProcessor.Identity().validate("ToolResultProcessor"); err != nil {
			return nil, fmt.Errorf("new agent: %w", err)
		}
	}
	if config.Artifacts != nil {
		if err := config.Artifacts.Identity().validate("ToolArtifactStorage"); err != nil {
			return nil, fmt.Errorf("new agent: %w", err)
		}
	}
	retry := config.Retry
	if config.ModelMaxAttempts < 0 {
		return nil, errors.New("new agent: ModelMaxAttempts cannot be negative")
	}
	if config.MaxIterations < 0 {
		return nil, errors.New("new agent: MaxIterations cannot be negative")
	}
	if config.IdleTimeout < 0 {
		return nil, errors.New("new agent: IdleTimeout cannot be negative")
	}
	parallelism := config.ToolParallelism
	if parallelism < 1 {
		parallelism = defaultToolParallelism
	} else if parallelism > maxToolParallelism {
		parallelism = maxToolParallelism
	}
	return &modelToolLoop{
		name:             config.Name,
		description:      config.Description,
		instruction:      config.Instruction,
		model:            config.Model,
		tools:            tools,
		middlewares:      middlewares,
		resultProcessor:  config.ResultProcessor,
		artifacts:        config.Artifacts,
		retry:            retry,
		modelMaxAttempts: max(1, config.ModelMaxAttempts),
		maxIterations:    config.MaxIterations,
		idleTimeout:      config.IdleTimeout,
		toolParallelism:  parallelism,
		modelCallGate:    config.modelCallGate,
		permission:       config.permission,
	}, nil
}

// Name returns the configured stable agent name.
func (agent *modelToolLoop) Name(context.Context) string {
	if agent == nil {
		return ""
	}
	return agent.name
}

// Description returns the configured host-facing description.
func (agent *modelToolLoop) Description(context.Context) string {
	if agent == nil {
		return ""
	}
	return agent.description
}

// Run starts the native model/tool loop and returns immediately.
func (agent *modelToolLoop) Run(ctx context.Context, input *loopInput, opts ...loopRunOption) *asyncIterator[*loopEvent] {
	iterator, generator := newAsyncIteratorPair[*loopEvent]()
	options := collectLoopRunOptions(opts)
	safeGo(func() {
		agent.run(ctx, input, options, generator)
		generator.Close()
	}, func(err error) {
		if options.cancel != nil {
			options.cancel.finish()
		}
		generator.Send(agent.errorEvent(err))
		generator.Close()
	})
	return iterator
}

func (agent *modelToolLoop) run(parent context.Context, input *loopInput, options *agentRunOptions, events *asyncGenerator[*loopEvent]) {
	if parent == nil {
		parent = context.Background()
	}
	ctx, stop := context.WithCancel(parent)
	defer stop()
	if agent != nil && agent.idleTimeout > 0 {
		var touch func()
		var stopIdle func()
		ctx, touch, stopIdle = startIdleTimeout(ctx, agent.idleTimeout)
		defer stopIdle()
		events = events.withActivity(touch)
	}
	if options.cancel != nil {
		options.cancel.bind(stop)
		defer options.cancel.finish()
	}
	if agent == nil {
		events.Send((&modelToolLoop{}).errorEvent(errors.New("run agent: nil agent")))
		return
	}
	var invocationErr error
	ctx, finishInvocation, invocationErr := beginRootInvocation(ctx, agent.name)
	if invocationErr != nil {
		events.Send(agent.errorEvent(fmt.Errorf("start Agent invocation: %w", invocationErr)))
		return
	}
	defer func() {
		if err := finishInvocation(); err != nil {
			events.Send(agent.errorEvent(fmt.Errorf("finish Agent invocation: %w", err)))
		}
	}()
	if input == nil {
		events.Send(agent.errorEvent(errors.New("run agent: nil input")))
		return
	}
	if input.stablePrefixMessages < 0 || input.stablePrefixMessages > len(input.Messages) {
		events.Send(agent.errorEvent(errors.New("run agent: stable prefix boundary is outside input messages")))
		return
	}
	if err := agent.contextError(ctx, options.cancel); err != nil {
		events.Send(agent.errorEvent(err))
		return
	}

	runContext := &RunContext{
		Instruction: agent.instruction,
		Tools:       append([]ToolDefinition(nil), agent.tools...),
	}
	var err error
	for _, middleware := range agent.middlewares {
		ctx, runContext, err = middleware.BeforeAgent(ctx, runContext)
		if err != nil {
			events.Send(agent.errorEvent(fmt.Errorf("before agent middleware: %w", err)))
			return
		}
		if ctx == nil {
			events.Send(agent.errorEvent(errors.New("before agent middleware returned nil Go context")))
			return
		}
		if runContext == nil {
			events.Send(agent.errorEvent(errors.New("before agent middleware returned nil context")))
			return
		}
	}
	// Artifact persistence is a fixed Definition capability, not a middleware
	// extension point. Rebind after BeforeAgent so middleware cannot replace it;
	// a nil Definition capability deliberately clears inherited ambient access.
	ctx = contextWithDefinitionToolArtifactStorage(ctx, agent.artifacts)
	registry, err := NewRegistry(ctx, runContext.Tools...)
	if err != nil {
		events.Send(agent.errorEvent(fmt.Errorf("prepare tool registry: %w", err)))
		return
	}

	state := &RunState{ToolInfos: registry.Schemas()}
	if runContext.Instruction != "" {
		state.Messages = append(state.Messages, SystemMessage(runContext.Instruction))
	}
	state.Messages = append(state.Messages, cloneMessages(input.Messages)...)
	stablePrefixMessages := input.stablePrefixMessages
	if runContext.Instruction != "" {
		stablePrefixMessages++
	}
	stablePrefixSeed := cloneMessages(state.Messages[:stablePrefixMessages])
	for iteration := 0; ; iteration++ {
		if err := agent.contextError(ctx, options.cancel); err != nil {
			events.Send(agent.errorEvent(err))
			return
		}
		// A task batch is a synchronization boundary. Waiting here keeps an
		// attached child concurrent with the tool batch that launched it, while
		// preventing the next model response from streaming a provisional final
		// answer that would have to be discarded when the child completes.
		if _, waitErr := waitForTrackedTaskCompletionsAtSafePoint(
			ctx,
			options.cancel,
			cancelAfterTools|cancelAfterModel,
		); waitErr != nil {
			events.Send(agent.errorEvent(waitErr))
			return
		}
		if err := agent.deliverPendingTaskCompletions(ctx, state, events); err != nil {
			events.Send(agent.errorEvent(err))
			return
		}
		if agent.maxIterations > 0 && iteration >= agent.maxIterations {
			events.Send(agent.errorEvent(errMaxIterations))
			return
		}

		modelContext := &ModelContext{
			Tools: cloneToolInfos(state.ToolInfos), Iteration: iteration,
			stablePrefixSeed: cloneMessages(stablePrefixSeed), instruction: runContext.Instruction,
		}
		step, err := agent.prepareModelStep(ctx, state, modelContext, input.EnableStreaming)
		if err != nil {
			events.Send(agent.errorEvent(err))
			return
		}
		ctx, state, modelContext = step.ctx, step.state, step.modelContext
		modelCall := step.call
		if agent.modelCallGate != nil {
			restart, gateErr := agent.applyModelCallGate(ctx, modelCall, modelContext, options.cancel)
			if gateErr != nil {
				events.Send(agent.errorEvent(fmt.Errorf("agent model call gate: %w", gateErr)))
				return
			}
			if restart != nil {
				ctx, state, modelContext = restart.ctx, restart.state, restart.modelContext
				modelCall = restart.call
				stablePrefixSeed = cloneMessages(modelContext.stablePrefixSeed)
			}
		}
		modelCall.stablePrefixMessages = authenticatedStablePrefixMessages(modelCall.Messages, stablePrefixSeed)
		if modelRequestCaptureRequested(ctx) {
			projectedMessages, projectionErr := projectToolArtifactPaths(ctx, agent.artifacts, modelCall.Messages)
			if projectionErr != nil {
				events.Send(agent.errorEvent(projectionErr))
				return
			}
			projectedCall := *modelCall
			projectedCall.Messages = projectedMessages
			events.Send(&loopEvent{AgentName: agent.name, Action: &loopAction{
				CustomizedAction: preparedModelRequest{snapshot: projectedCall.Snapshot()},
			}})
			return
		}
		assistant, modelResponseOrdinal, acceptedModelMessages, nextCtx, err := agent.callModelWithRetry(
			ctx,
			modelCall,
			modelContext,
			registry,
			events,
			options.cancel,
		)
		ctx = nextCtx
		stablePrefixSeed = cloneMessages(modelContext.stablePrefixSeed)
		if err != nil {
			var cancelErr *cancelError
			if !errors.As(err, &cancelErr) {
				if contextErr := agent.contextError(ctx, options.cancel); contextErr != nil {
					err = contextErr
				}
			}
			events.Send(agent.errorEvent(err))
			return
		}
		if assistant == nil {
			events.Send(agent.errorEvent(errors.New("model returned nil assistant message")))
			return
		}
		if assistant.Role == "" {
			assistant.Role = Assistant
		}
		if assistant.Role != Assistant {
			events.Send(agent.errorEvent(fmt.Errorf("model returned role %q, want assistant", assistant.Role)))
			return
		}
		state.Messages = cloneMessages(acceptedModelMessages)
		assistant.AgentMeta = &AgentMessageMeta{ModelResponseOrdinal: modelResponseOrdinal}
		state.Messages = append(state.Messages, assistant.Clone())

		for _, middleware := range agent.middlewares {
			ctx, state, err = middleware.AfterModelRewriteState(ctx, state, modelContext)
			if err != nil {
				events.Send(agent.errorEvent(fmt.Errorf("after model middleware: %w", err)))
				return
			}
			if ctx == nil {
				events.Send(agent.errorEvent(errors.New("after model middleware returned nil Go context")))
				return
			}
			if state == nil {
				events.Send(agent.errorEvent(errors.New("after model middleware returned nil state")))
				return
			}
		}
		assistant = lastAssistantMessage(state.Messages, assistant)

		if err := agent.contextError(ctx, options.cancel); err != nil {
			events.Send(agent.errorEvent(err))
			return
		}

		if len(assistant.ToolCalls) == 0 {
			if cancelErr := options.cancel.safePoint(cancelAfterModel); cancelErr != nil {
				events.Send(agent.errorEvent(cancelErr))
				return
			}
			completionReady, waitErr := waitForTrackedTaskCompletionsAtSafePoint(
				ctx,
				options.cancel,
				cancelAfterModel,
			)
			if waitErr != nil {
				events.Send(agent.errorEvent(waitErr))
				return
			}
			if completionReady {
				continue
			}
			for _, middleware := range agent.middlewares {
				ctx, err = middleware.AfterAgent(ctx, state)
				if err != nil {
					events.Send(agent.errorEvent(fmt.Errorf("after agent middleware: %w", err)))
					return
				}
				if ctx == nil {
					events.Send(agent.errorEvent(errors.New("after agent middleware returned nil Go context")))
					return
				}
			}
			return
		}

		preparedCalls := agent.prepareToolCalls(ctx, registry, assistant.ToolCalls, modelResponseOrdinal)
		for index := range preparedCalls {
			assistant.ToolCalls[index] = preparedCalls[index].call
		}
		for index := len(state.Messages) - 1; index >= 0; index-- {
			if state.Messages[index] != nil && state.Messages[index].Role == Assistant {
				state.Messages[index] = assistant.Clone()
				break
			}
		}
		if err := agent.publishToolBatchBoundary(ctx, toolBatchPrepared, []*Message{assistant}, events); err != nil {
			events.Send(agent.errorEvent(err))
			return
		}

		var toolResults []toolExecutionResult
		if finishReason, blocked := modelFinishReasonBlocksToolExecution(assistant.ResponseMeta); blocked {
			toolResults = agent.incompleteModelToolResults(preparedCalls, finishReason, events)
		} else {
			toolResults, err = agent.executePreparedToolBatch(ctx, preparedCalls, events, options.cancel)
		}
		for _, result := range toolResults {
			if result.message == nil {
				continue
			}
			state.Messages = append(state.Messages, result.message.Clone())
			toolEvent := agent.messageEvent(result.message.Clone(), nil, ToolRole, result.message.ToolName)
			toolEvent.Output.MessageOutput.ExecutionID = result.executionID
			toolEvent.Output.MessageOutput.ProviderCallID = result.providerCallID
			events.Send(toolEvent)
		}
		if err != nil {
			if contextErr := agent.contextError(ctx, options.cancel); contextErr != nil {
				err = contextErr
			}
			events.Send(agent.errorEvent(err))
			return
		}
		completedBatch := make([]*Message, 1, len(toolResults)+1)
		completedBatch[0] = assistant.Clone()
		for _, result := range toolResults {
			if result.message != nil {
				completedBatch = append(completedBatch, result.message.Clone())
			}
		}
		if err := agent.publishToolBatchBoundary(ctx, toolBatchCompleted, completedBatch, events); err != nil {
			events.Send(agent.errorEvent(err))
			return
		}

		if cancelErr := options.cancel.safePoint(cancelAfterTools | cancelAfterModel); cancelErr != nil {
			events.Send(agent.errorEvent(cancelErr))
			return
		}
		if err := agent.contextError(ctx, options.cancel); err != nil {
			events.Send(agent.errorEvent(err))
			return
		}
	}
}

func waitForTrackedTaskCompletionsAtSafePoint(
	ctx context.Context,
	cancel *cancelControl,
	point cancelMode,
) (bool, error) {
	var interrupt <-chan struct{}
	if cancel != nil {
		interrupt = cancel.requestedSignal()
	}
	ready, err := waitForTrackedTaskCompletionsFromContext(ctx, interrupt)
	if !errors.Is(err, errTaskCompletionWaitInterrupted) {
		return ready, err
	}
	if cancelErr := cancel.safePoint(point); cancelErr != nil {
		return false, cancelErr
	}
	// A cancellation request for another safe point must not spin on the
	// already-closed signal while attached child work remains.
	return waitForTrackedTaskCompletionsFromContext(ctx, nil)
}

func (agent *modelToolLoop) publishToolBatchBoundary(
	ctx context.Context,
	phase toolBatchPhase,
	messages []*Message,
	events *asyncGenerator[*loopEvent],
) error {
	if !toolStartReceiptRequired(ctx) {
		return nil
	}
	switch phase {
	case toolBatchPrepared:
		if len(messages) != 1 {
			return errors.New("prepared canonical tool batch requires one assistant message")
		}
		if _, err := validateCanonicalToolCallMessage(messages[0]); err != nil {
			return err
		}
	case toolBatchCompleted:
		if err := ValidateContextCommitMessages(messages); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported canonical tool batch phase %q", phase)
	}
	boundary := &toolBatchBoundary{
		phase: phase, messages: cloneMessages(messages), receipt: make(chan error, 1),
	}
	events.Send(&loopEvent{
		AgentName: agent.name,
		RunPath:   []loopRunStep{newLoopRunStep(agent.name)},
		Output:    &loopOutput{ToolBatch: boundary},
	})
	select {
	case err := <-boundary.receipt:
		return err
	case <-ctx.Done():
		return agent.contextError(ctx, nil)
	}
}

func (agent *modelToolLoop) deliverPendingTaskCompletions(
	ctx context.Context,
	state *RunState,
	events *asyncGenerator[*loopEvent],
) error {
	completions := pendingTaskCompletionsFromContext(ctx)
	if len(completions) == 0 {
		return nil
	}
	boundary := &taskCompletionBoundary{
		completions: completions,
		receipt:     make(chan error, 1),
	}
	for _, completion := range completions {
		state.Messages = append(state.Messages, completion.Message.Clone())
	}
	events.Send(&loopEvent{
		AgentName: agent.name,
		RunPath:   []loopRunStep{newLoopRunStep(agent.name)},
		Output:    &loopOutput{TaskCompletions: boundary},
	})
	select {
	case err := <-boundary.receipt:
		return err
	case <-ctx.Done():
		return agent.contextError(ctx, nil)
	}
}

func (agent *modelToolLoop) contextError(ctx context.Context, cancel *cancelControl) error {
	if ctx == nil || ctx.Err() == nil {
		return nil
	}
	if cancel != nil {
		if cancelErr := cancel.immediateError(); cancelErr != nil {
			return cancelErr
		}
	}
	if cause := context.Cause(ctx); cause != nil {
		return cause
	}
	return ctx.Err()
}

func (agent *modelToolLoop) messageEvent(message *Message, stream *StreamReader[*Message], role RoleType, toolName string) *loopEvent {
	event := loopEventFromMessage(message, stream, role, toolName)
	event.AgentName = agent.name
	event.RunPath = []loopRunStep{newLoopRunStep(agent.name)}
	return event
}

func (agent *modelToolLoop) customEvent(value any) *loopEvent {
	return &loopEvent{
		AgentName: agent.name,
		RunPath:   []loopRunStep{newLoopRunStep(agent.name)},
		Output:    &loopOutput{CustomizedOutput: value},
	}
}

func (agent *modelToolLoop) errorEvent(err error) *loopEvent {
	return &loopEvent{AgentName: agent.name, RunPath: []loopRunStep{newLoopRunStep(agent.name)}, Err: err}
}

func cloneMessages(messages []*Message) []*Message {
	if messages == nil {
		return nil
	}
	result := make([]*Message, len(messages))
	for index, message := range messages {
		result[index] = message.Clone()
	}
	return result
}

func lastAssistantMessage(messages []*Message, fallback *Message) *Message {
	for index := len(messages) - 1; index >= 0; index-- {
		if messages[index] != nil && messages[index].Role == Assistant {
			return messages[index].Clone()
		}
	}
	return fallback.Clone()
}
