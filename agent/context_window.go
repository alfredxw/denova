package agent

import "context"

// ContextCheckpointRequest records a named point before the current model
// response. The controller owns stable identity and durable staging.
type ContextCheckpointRequest struct {
	Purpose string
}

type ContextCheckpointResult struct {
	ID      string `json:"id"`
	Purpose string `json:"purpose"`
	Staged  bool   `json:"staged"`
}

// ContextRewindRequest projects subsequent model calls from a checkpoint.
// Report is bounded by the implementation and preserves conclusions while
// intermediate exploratory transcript is dropped.
type ContextRewindRequest struct {
	CheckpointID string
	Report       string
}

type ContextRewindResult struct {
	CheckpointID string `json:"checkpoint_id"`
	Dropped      int    `json:"dropped_messages"`
	Staged       bool   `json:"staged"`
}

// ContextWindowRewrite is an ordered structural marker emitted immediately
// before the first model call using a rewound transcript. Hosts use it to keep
// append-only display output separate from the effective assistant answer.
type ContextWindowRewrite struct {
	Kind         string
	CheckpointID string
}

// ContextWindowRewriteSource exposes a consumed rewrite exactly once. It is a
// narrow optional extension so third-party controllers remain source-compatible.
type ContextWindowRewriteSource interface {
	TakeContextWindowRewrite() (ContextWindowRewrite, bool)
}

// ContextToolObservation lets a context controller retain mutation receipts
// without coupling the provider-neutral loop to any product storage domain.
type ContextToolObservation struct {
	Name       string
	CallID     string
	Descriptor ToolDescriptor
	Result     ToolResult
}

// ContextMutationObserver records committed side effects across a root and its
// delegated invocations. It is deliberately separate from transcript rewrite
// authority so child Agents cannot consume a parent's pending rewind.
type ContextMutationObserver interface {
	ObserveTool(context.Context, ContextToolObservation)
}

// ContextWindowController is the root-invocation seam behind checkpoint/rewind.
// BeforeModel may replace the in-memory transcript immediately before the next
// model call; durable publication remains the host Conversation's concern.
type ContextWindowController interface {
	ContextMutationObserver
	BeforeModel(context.Context, []*Message) ([]*Message, error)
	// BeforeComplete may request one more model iteration when a structural
	// context operation (for example an active checkpoint) is unfinished.
	BeforeComplete(context.Context, []*Message) ([]*Message, bool, error)
	Checkpoint(context.Context, ContextCheckpointRequest) (ContextCheckpointResult, error)
	Rewind(context.Context, ContextRewindRequest) (ContextRewindResult, error)
}

type contextWindowControllerKey struct{}
type contextMutationObserverKey struct{}

// ContextWithContextWindowController binds one controller to one Agent run.
func ContextWithContextWindowController(ctx context.Context, controller ContextWindowController) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if controller == nil {
		return ctx
	}
	ctx = context.WithValue(ctx, contextWindowControllerKey{}, controller)
	return context.WithValue(ctx, contextMutationObserverKey{}, ContextMutationObserver(controller))
}

// ContextWindowControllerFromContext returns root-owned rewrite authority.
// Delegated invocations retain the mutation observer but cannot checkpoint,
// rewind, or rewrite their parent's transcript.
func ContextWindowControllerFromContext(ctx context.Context) (ContextWindowController, bool) {
	if ctx == nil || !IsRootInvocation(ctx) {
		return nil, false
	}
	controller, ok := ctx.Value(contextWindowControllerKey{}).(ContextWindowController)
	return controller, ok && controller != nil
}

// ContextMutationObserverFromContext returns the shared side-effect sink for
// both root and delegated invocations.
func ContextMutationObserverFromContext(ctx context.Context) (ContextMutationObserver, bool) {
	if ctx == nil {
		return nil, false
	}
	observer, ok := ctx.Value(contextMutationObserverKey{}).(ContextMutationObserver)
	return observer, ok && observer != nil
}
