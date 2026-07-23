package adk

import (
	"context"
	"errors"
	"fmt"
)

// AgentConfig configures the native model/tool loop.
type AgentConfig struct {
	Name        string
	Description string
	Instruction string
	Model       BaseChatModel
	Tools       []BaseTool

	Middlewares []Middleware
	Retry       *RetryConfig

	// MaxIterations is an explicit caller-owned guard. Zero means unlimited;
	// the ADK never installs an implicit iteration limit.
	MaxIterations int

	// EmitToolCompletions emits non-transcript completion notifications as
	// concurrent calls finish. Tool result message events always remain ordered.
	EmitToolCompletions bool
}

// Agent owns the provider-neutral model/tool loop.
type Agent struct {
	name                string
	description         string
	instruction         string
	model               BaseChatModel
	tools               []BaseTool
	middlewares         []Middleware
	retry               *RetryConfig
	maxIterations       int
	emitToolCompletions bool
}

// ErrMaxIterations is returned only when the caller explicitly configures a limit.
var ErrMaxIterations = errors.New("agent reached configured maximum iterations")

// NewAgent validates the model, tool registry, and middleware surface.
func NewAgent(ctx context.Context, config AgentConfig) (*Agent, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if config.Model == nil {
		return nil, errors.New("new agent: model is required")
	}
	tools := append([]BaseTool(nil), config.Tools...)
	if _, err := NewRegistry(ctx, tools...); err != nil {
		return nil, fmt.Errorf("new agent: %w", err)
	}
	middlewares := append([]Middleware(nil), config.Middlewares...)
	for index, middleware := range middlewares {
		if middleware == nil {
			return nil, fmt.Errorf("new agent: nil middleware at index %d", index)
		}
	}
	retry := config.Retry
	if retry != nil && retry.MaxRetries < 0 {
		return nil, errors.New("new agent: retry MaxRetries cannot be negative")
	}
	if config.MaxIterations < 0 {
		return nil, errors.New("new agent: MaxIterations cannot be negative")
	}
	return &Agent{
		name:                config.Name,
		description:         config.Description,
		instruction:         config.Instruction,
		model:               config.Model,
		tools:               tools,
		middlewares:         middlewares,
		retry:               retry,
		maxIterations:       config.MaxIterations,
		emitToolCompletions: config.EmitToolCompletions,
	}, nil
}

// Name returns the configured stable agent name.
func (agent *Agent) Name(context.Context) string {
	if agent == nil {
		return ""
	}
	return agent.name
}

// Description returns the configured host-facing description.
func (agent *Agent) Description(context.Context) string {
	if agent == nil {
		return ""
	}
	return agent.description
}

// ToolCompletion is an optional real-time, non-transcript notification.
type ToolCompletion struct {
	Index    int
	CallID   string
	ToolName string
	Err      error
}

// Run starts the native model/tool loop and returns immediately.
func (agent *Agent) Run(ctx context.Context, input *AgentInput, opts ...AgentRunOption) *AsyncIterator[*AgentEvent] {
	iterator, generator := NewAsyncIteratorPair[*AgentEvent]()
	options := collectAgentRunOptions(opts)
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

func (agent *Agent) run(parent context.Context, input *AgentInput, options *agentRunOptions, events *AsyncGenerator[*AgentEvent]) {
	if parent == nil {
		parent = context.Background()
	}
	ctx, stop := context.WithCancel(parent)
	defer stop()
	if options.cancel != nil {
		options.cancel.bind(stop)
		defer options.cancel.finish()
	}
	if agent == nil {
		events.Send((&Agent{}).errorEvent(errors.New("run agent: nil agent")))
		return
	}
	if input == nil {
		events.Send(agent.errorEvent(errors.New("run agent: nil input")))
		return
	}
	if err := agent.contextError(ctx, options.cancel); err != nil {
		events.Send(agent.errorEvent(err))
		return
	}

	runContext := &RunContext{
		Instruction: agent.instruction,
		Tools:       append([]BaseTool(nil), agent.tools...),
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

	for iteration := 0; ; iteration++ {
		if agent.maxIterations > 0 && iteration >= agent.maxIterations {
			events.Send(agent.errorEvent(ErrMaxIterations))
			return
		}
		if err := agent.contextError(ctx, options.cancel); err != nil {
			events.Send(agent.errorEvent(err))
			return
		}

		modelContext := &ModelContext{Tools: cloneToolInfos(state.ToolInfos), Retry: agent.retry}
		for _, middleware := range agent.middlewares {
			ctx, state, err = middleware.BeforeModelRewriteState(ctx, state, modelContext)
			if err != nil {
				events.Send(agent.errorEvent(fmt.Errorf("before model middleware: %w", err)))
				return
			}
			if ctx == nil {
				events.Send(agent.errorEvent(errors.New("before model middleware returned nil Go context")))
				return
			}
			if state == nil {
				events.Send(agent.errorEvent(errors.New("before model middleware returned nil state")))
				return
			}
		}
		modelContext.Tools = cloneToolInfos(state.ToolInfos)

		modelForCall, err := agent.modelForCall(ctx, modelContext)
		if err != nil {
			events.Send(agent.errorEvent(err))
			return
		}
		modelOptions := []ModelOption{WithTools(state.ToolInfos)}
		assistant, err := agent.callModelWithRetry(
			ctx,
			modelForCall,
			cloneMessages(state.Messages),
			modelOptions,
			input.EnableStreaming,
			events,
			options.cancel,
		)
		if err != nil {
			var cancelErr *CancelError
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

		if cancelErr := options.cancel.safePoint(CancelAfterChatModel); cancelErr != nil {
			events.Send(agent.errorEvent(cancelErr))
			return
		}
		if err := agent.contextError(ctx, options.cancel); err != nil {
			events.Send(agent.errorEvent(err))
			return
		}

		if len(assistant.ToolCalls) == 0 {
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

		var toolResults []toolExecutionResult
		if assistant.ResponseMeta != nil && assistant.ResponseMeta.FinishReason == "length" {
			toolResults = lengthToolResults(assistant.ToolCalls)
		} else {
			toolResults, err = agent.executeToolBatch(ctx, registry, assistant.ToolCalls, events)
		}
		for _, result := range toolResults {
			if result.message == nil {
				continue
			}
			state.Messages = append(state.Messages, result.message.Clone())
			events.Send(agent.messageEvent(result.message.Clone(), nil, Tool, result.message.ToolName))
		}
		if err != nil {
			if IsInterruptError(err) {
				events.Send(agent.interruptEvent(err))
				return
			}
			if contextErr := agent.contextError(ctx, options.cancel); contextErr != nil {
				err = contextErr
			}
			events.Send(agent.errorEvent(err))
			return
		}

		if cancelErr := options.cancel.safePoint(CancelAfterToolCalls); cancelErr != nil {
			events.Send(agent.errorEvent(cancelErr))
			return
		}
		if err := agent.contextError(ctx, options.cancel); err != nil {
			events.Send(agent.errorEvent(err))
			return
		}
	}
}

func (agent *Agent) contextError(ctx context.Context, cancel *cancelControl) error {
	if ctx == nil || ctx.Err() == nil {
		return nil
	}
	if cancel != nil {
		if cancelErr := cancel.immediateError(); cancelErr != nil {
			return cancelErr
		}
	}
	return ctx.Err()
}

func (agent *Agent) messageEvent(message *Message, stream *StreamReader[*Message], role RoleType, toolName string) *AgentEvent {
	event := EventFromMessage(message, stream, role, toolName)
	event.AgentName = agent.name
	event.RunPath = []RunStep{NewRunStep(agent.name)}
	return event
}

func (agent *Agent) customEvent(value any) *AgentEvent {
	return &AgentEvent{
		AgentName: agent.name,
		RunPath:   []RunStep{NewRunStep(agent.name)},
		Output:    &AgentOutput{CustomizedOutput: value},
	}
}

func (agent *Agent) errorEvent(err error) *AgentEvent {
	return &AgentEvent{AgentName: agent.name, RunPath: []RunStep{NewRunStep(agent.name)}, Err: err}
}

func (agent *Agent) interruptEvent(err error) *AgentEvent {
	var interrupt *InterruptError
	if !errors.As(err, &interrupt) {
		return agent.errorEvent(err)
	}
	return &AgentEvent{
		AgentName: agent.name,
		RunPath:   []RunStep{NewRunStep(agent.name)},
		Action:    &AgentAction{Interrupted: interrupt},
		Err:       err,
	}
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
