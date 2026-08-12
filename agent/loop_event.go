package agent

import (
	"context"
	"encoding/json"
	"sync"
)

// asyncIterator is an unbounded, blocking event iterator.
type asyncIterator[T any] struct {
	queue *asyncQueue[T]
}

// Next blocks until a value is available or the generator closes.
func (iterator *asyncIterator[T]) Next() (T, bool) {
	if iterator == nil || iterator.queue == nil {
		var zero T
		return zero, false
	}
	return iterator.queue.receive()
}

// asyncGenerator is the producer paired with an asyncIterator.
type asyncGenerator[T any] struct {
	queue    *asyncQueue[T]
	activity func()
}

// Send enqueues without waiting for a consumer.
func (generator *asyncGenerator[T]) Send(value T) {
	if generator == nil || generator.queue == nil {
		return
	}
	if generator.activity != nil {
		generator.activity()
	}
	generator.queue.send(value)
}

func (generator *asyncGenerator[T]) withActivity(activity func()) *asyncGenerator[T] {
	if generator == nil || activity == nil {
		return generator
	}
	return &asyncGenerator[T]{queue: generator.queue, activity: activity}
}

// Close is idempotent; queued values remain readable.
func (generator *asyncGenerator[T]) Close() {
	if generator == nil || generator.queue == nil {
		return
	}
	generator.queue.close()
}

type asyncQueue[T any] struct {
	mu     sync.Mutex
	ready  *sync.Cond
	values []T
	closed bool
}

func newAsyncQueue[T any]() *asyncQueue[T] {
	queue := &asyncQueue[T]{}
	queue.ready = sync.NewCond(&queue.mu)
	return queue
}

func (queue *asyncQueue[T]) send(value T) {
	queue.mu.Lock()
	if !queue.closed {
		queue.values = append(queue.values, value)
		queue.ready.Signal()
	}
	queue.mu.Unlock()
}

func (queue *asyncQueue[T]) close() {
	queue.mu.Lock()
	queue.closed = true
	queue.ready.Broadcast()
	queue.mu.Unlock()
}

func (queue *asyncQueue[T]) receive() (T, bool) {
	queue.mu.Lock()
	defer queue.mu.Unlock()
	for len(queue.values) == 0 && !queue.closed {
		queue.ready.Wait()
	}
	if len(queue.values) == 0 {
		var zero T
		return zero, false
	}
	value := queue.values[0]
	var zero T
	queue.values[0] = zero
	queue.values = queue.values[1:]
	return value, true
}

// newAsyncIteratorPair creates an unbounded producer/consumer pair.
func newAsyncIteratorPair[T any]() (*asyncIterator[T], *asyncGenerator[T]) {
	queue := newAsyncQueue[T]()
	return &asyncIterator[T]{queue: queue}, &asyncGenerator[T]{queue: queue}
}

// loopMessage carries either one complete message or an exclusive stream.
type loopMessage struct {
	IsStreaming   bool
	Message       *Message
	MessageStream *StreamReader[*Message]
	Role          RoleType
	ToolName      string
	// ExecutionID correlates a tool result with lifecycle/display state while
	// ProviderCallID remains the model transcript pairing identity.
	ExecutionID    string
	ProviderCallID string
	// Assistant tool calls derive their execution IDs from this immutable model
	// response identity plus their source ordinal.
	ToolExecutionNamespace string
	ModelResponseOrdinal   int
	// ToolInfos is the validated registry snapshot for model events. It is
	// transport metadata, not part of the emitted transcript Message.
	ToolInfos       []*ToolInfo
	ToolDefinitions []ToolDefinitionSnapshot
}

// GetMessage returns or drains the variant's message.
func (variant *loopMessage) GetMessage() (*Message, error) {
	if variant == nil {
		return nil, nil
	}
	if variant.IsStreaming {
		return ConcatMessageStream(variant.MessageStream)
	}
	return variant.Message, nil
}

// loopOutput is the payload of one loopEvent.
type loopOutput struct {
	MessageOutput    *loopMessage
	ToolExecution    *toolExecutionEvent
	NestedEvent      *NestedEvent
	CustomizedOutput any
}

// toolExecutionPhase is the exhaustive lifecycle of one concrete call.
type toolExecutionPhase string

const (
	toolExecutionStarted  toolExecutionPhase = "started"
	toolExecutionProgress toolExecutionPhase = "progress"
	toolExecutionFinished toolExecutionPhase = "finished"
)

// toolExecutionEvent is a real-time, non-transcript tool notification. Finished
// events follow completion order; tool messages remain source ordered.
type toolExecutionEvent struct {
	Phase          toolExecutionPhase
	Index          int
	ExecutionID    string
	ProviderCallID string
	ToolName       string
	Arguments      json.RawMessage
	Definition     ToolDefinitionSnapshot
	Delta          string
	Result         *ToolResult
	// startReceipt is installed only by the durable Definition lifecycle. It
	// keeps the concrete endpoint behind the actor's ToolCallStarted receipt
	// while preserving the low-level modelToolLoop's ordinary asynchronous event API.
	startReceipt chan error
}

func (event *toolExecutionEvent) acknowledgeStart(err error) {
	if event == nil || event.startReceipt == nil {
		return
	}
	event.startReceipt <- err
}

// ToolExecutionID returns the durable display/lifecycle identity for one
// assistant tool ordinal. The provider call ID must remain transcript-only.
func (variant *loopMessage) ToolExecutionID(toolOrdinal int) string {
	if variant == nil || variant.ModelResponseOrdinal <= 0 || toolOrdinal < 0 || variant.ToolExecutionNamespace == "" {
		return ""
	}
	return executionIDForNamespace(variant.ToolExecutionNamespace, variant.ModelResponseOrdinal, toolOrdinal)
}

// loopAction is reserved for transport-neutral host control actions.
type loopAction struct {
	CustomizedAction any
}

// loopRunStep identifies an event's source in a nested host path.
type loopRunStep struct {
	agentName string
}

// newLoopRunStep constructs a stable path element.
func newLoopRunStep(agentName string) loopRunStep {
	return loopRunStep{agentName: agentName}
}

func (step loopRunStep) String() string { return step.agentName }

// loopEvent is emitted by the native loop in transcript order.
type loopEvent struct {
	AgentName      string
	RunPath        []loopRunStep
	InvocationID   string
	InvocationType string
	Output         *loopOutput
	Action         *loopAction
	Err            error
}

// loopInput contains a caller-owned transcript snapshot.
type loopInput struct {
	Messages        []*Message
	EnableStreaming bool
	// ResumeToolCalls executes the final assistant tool-call batch before the
	// next model request. It is reserved for durable Interaction recovery.
	ResumeToolCalls bool
	// stablePrefixMessages is set only by the Definition lifecycle after it has
	// assembled accountable Context fragments and the active checkpoint. It is
	// intentionally not part of the public low-level modelToolLoop input surface.
	stablePrefixMessages int
}

// loopEventFromMessage builds a model or tool message event.
func loopEventFromMessage(message *Message, stream *StreamReader[*Message], role RoleType, toolName string) *loopEvent {
	return &loopEvent{Output: &loopOutput{MessageOutput: &loopMessage{
		IsStreaming:   stream != nil,
		Message:       message,
		MessageStream: stream,
		Role:          role,
		ToolName:      toolName,
	}}}
}

// loopRunnable is the small execution seam accepted by loopRunner. modelToolLoop is its native implementation.
type loopRunnable interface {
	Name(context.Context) string
	Description(context.Context) string
	Run(context.Context, *loopInput, ...loopRunOption) *asyncIterator[*loopEvent]
}
