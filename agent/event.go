package agent

import (
	"context"
	"sync"
)

// AsyncIterator is an unbounded, blocking event iterator.
type AsyncIterator[T any] struct {
	queue *asyncQueue[T]
}

// Next blocks until a value is available or the generator closes.
func (iterator *AsyncIterator[T]) Next() (T, bool) {
	if iterator == nil || iterator.queue == nil {
		var zero T
		return zero, false
	}
	return iterator.queue.receive()
}

// AsyncGenerator is the producer paired with an AsyncIterator.
type AsyncGenerator[T any] struct {
	queue *asyncQueue[T]
}

// Send enqueues without waiting for a consumer.
func (generator *AsyncGenerator[T]) Send(value T) {
	if generator == nil || generator.queue == nil {
		return
	}
	generator.queue.send(value)
}

// Close is idempotent; queued values remain readable.
func (generator *AsyncGenerator[T]) Close() {
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

// NewAsyncIteratorPair creates an unbounded producer/consumer pair.
func NewAsyncIteratorPair[T any]() (*AsyncIterator[T], *AsyncGenerator[T]) {
	queue := newAsyncQueue[T]()
	return &AsyncIterator[T]{queue: queue}, &AsyncGenerator[T]{queue: queue}
}

// MessageVariant carries either one complete message or an exclusive stream.
type MessageVariant struct {
	IsStreaming   bool
	Message       *Message
	MessageStream *StreamReader[*Message]
	Role          RoleType
	ToolName      string
	// ToolInfos is the validated registry snapshot for model events. It is
	// transport metadata, not part of the emitted transcript Message.
	ToolInfos []*ToolInfo
}

// GetMessage returns or drains the variant's message.
func (variant *MessageVariant) GetMessage() (*Message, error) {
	if variant == nil {
		return nil, nil
	}
	if variant.IsStreaming {
		return ConcatMessageStream(variant.MessageStream)
	}
	return variant.Message, nil
}

// AgentOutput is the payload of one AgentEvent.
type AgentOutput struct {
	MessageOutput    *MessageVariant
	CustomizedOutput any
}

// AgentAction is reserved for transport-neutral host control actions.
type AgentAction struct {
	Interrupted      *InterruptError
	CustomizedAction any
}

// RunStep identifies an event's source in a nested host path.
type RunStep struct {
	agentName string
}

// NewRunStep constructs a stable path element.
func NewRunStep(agentName string) RunStep {
	return RunStep{agentName: agentName}
}

func (step RunStep) String() string { return step.agentName }

// AgentEvent is emitted by the native loop in transcript order.
type AgentEvent struct {
	AgentName string
	RunPath   []RunStep
	Output    *AgentOutput
	Action    *AgentAction
	Err       error
}

// AgentInput contains a caller-owned transcript snapshot.
type AgentInput struct {
	Messages        []*Message
	EnableStreaming bool
}

// AgentEventSink lets a tool forward nested Agent events without depending on
// the parent generator or a product-specific sub-agent type.
type AgentEventSink func(*AgentEvent)

type agentEventSinkContextKey struct{}

// ContextWithEventSink attaches a nested-event sink to a tool context.
func ContextWithEventSink(ctx context.Context, sink AgentEventSink) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, agentEventSinkContextKey{}, sink)
}

// EventSink returns the nested-event sink attached by the native loop.
func EventSink(ctx context.Context) (AgentEventSink, bool) {
	if ctx == nil {
		return nil, false
	}
	sink, ok := ctx.Value(agentEventSinkContextKey{}).(AgentEventSink)
	return sink, ok && sink != nil
}

// EmitEvent forwards a nested event when a sink is present.
func EmitEvent(ctx context.Context, event *AgentEvent) bool {
	sink, ok := EventSink(ctx)
	if !ok {
		return false
	}
	sink(event)
	return true
}

// EventFromMessage builds a model or tool message event.
func EventFromMessage(message *Message, stream *StreamReader[*Message], role RoleType, toolName string) *AgentEvent {
	return &AgentEvent{Output: &AgentOutput{MessageOutput: &MessageVariant{
		IsStreaming:   stream != nil,
		Message:       message,
		MessageStream: stream,
		Role:          role,
		ToolName:      toolName,
	}}}
}

// Runnable is the small execution seam accepted by Runner. Agent is its native implementation.
type Runnable interface {
	Name(context.Context) string
	Description(context.Context) string
	Run(context.Context, *AgentInput, ...AgentRunOption) *AsyncIterator[*AgentEvent]
}
