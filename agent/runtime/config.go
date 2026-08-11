package runtime

import (
	"context"
	"errors"
	"fmt"
)

type RuntimeConfig struct {
	ObservationBuffer int
	// MaxOpenBindings bounds actor goroutines and durable journal leases. The
	// Runtime evicts only least-recently-used idle actors; active or observed
	// bindings may temporarily exceed the limit until their state becomes idle.
	MaxOpenBindings int
	// These limits bound actor-owned display recovery memory only. Durable
	// journals and model-visible context are unaffected.
	RetainedEventLimit   int
	RetainedMessageLimit int
	RetainedCommandLimit int
	// ProjectionTextMaxBytes bounds each active/queued text field copied into a
	// StatusSnapshot. It never affects model-visible context or durable history.
	ProjectionTextMaxBytes int
	// MemoryLimits bound aggregate actor-owned payload memory per binding. They
	// are independent from provider context limits and durable journal retention.
	MemoryLimits BindingMemoryLimits
	// InputLimits bound the durable command envelope. They are intentionally
	// independent from model context-window policy: adapters may project less,
	// but no transport can persist an unbounded message or reference catalog.
	InputLimits InputLimits
	// Lifecycle is the owner context for every Engine run. HTTP request
	// cancellation must not be used here; App/workspace shutdown should be.
	Lifecycle context.Context
}

// BindingMemoryLimits are aggregate byte budgets for one durable binding.
// Defaults intentionally leave room for large bounded context fragments while
// preventing count-bounded collections from retaining gigabytes.
// Values are logical payload bytes plus conservative object overhead, not Go
// heap measurements.
type BindingMemoryLimits struct {
	// MaxRetainedBytes covers display events/messages and hot command receipts.
	MaxRetainedBytes int64
	// MaxPendingInputBytes covers active input, queued inputs, and structural
	// restore descriptors.
	MaxPendingInputBytes int64
	// MaxActiveOutputBytes covers combined assistant content/thinking builders
	// and also caps one foreground tool-progress payload.
	MaxActiveOutputBytes int64
	// MaxEngineStateBytes bounds the private cross-cycle Engine checkpoint.
	// It is deliberately high because a complete model transcript may contain
	// large, still-recoverable tool results.
	MaxEngineStateBytes int64
	// MaxHostEffectBytes bounds one durable host-effect outbox payload.
	MaxHostEffectBytes int64
	// MaxPendingHostEffectBytes and MaxPendingHostEffects bound all unacked
	// effects retained by one binding. Acknowledgement releases both budgets.
	MaxPendingHostEffectBytes int64
	MaxPendingHostEffects     int
	// MaxInteractionBytes bounds one request or normalized response;
	// MaxPendingInteractions bounds unresolved/resumable waiters in one cycle.
	MaxInteractionBytes    int64
	MaxPendingInteractions int
}

// DefaultBindingMemoryLimits returns the normalized production budgets.
func DefaultBindingMemoryLimits() BindingMemoryLimits {
	return BindingMemoryLimits{}.normalized()
}

func (limits BindingMemoryLimits) normalized() BindingMemoryLimits {
	if limits.MaxRetainedBytes <= 0 {
		limits.MaxRetainedBytes = 64 << 20
	}
	if limits.MaxPendingInputBytes <= 0 {
		limits.MaxPendingInputBytes = 64 << 20
	}
	if limits.MaxActiveOutputBytes <= 0 {
		limits.MaxActiveOutputBytes = 64 << 20
	}
	if limits.MaxEngineStateBytes <= 0 {
		limits.MaxEngineStateBytes = 64 << 20
	}
	if limits.MaxHostEffectBytes <= 0 {
		limits.MaxHostEffectBytes = 8 << 20
	}
	if limits.MaxPendingHostEffectBytes <= 0 {
		limits.MaxPendingHostEffectBytes = 32 << 20
	}
	if limits.MaxPendingHostEffects <= 0 {
		limits.MaxPendingHostEffects = 1024
	}
	if limits.MaxInteractionBytes <= 0 {
		limits.MaxInteractionBytes = 4 << 20
	}
	if limits.MaxPendingInteractions <= 0 {
		limits.MaxPendingInteractions = 64
	}
	return limits
}

// InputLimits are byte-based because commands are persisted and transported as
// UTF-8 JSON. Zero values select conservative production defaults suitable for
// large bounded context fragments.
type InputLimits struct {
	MaxCommandIDBytes         int
	MaxOperationIDBytes       int
	MaxAbortReasonBytes       int
	MaxTextBytes              int
	MaxContextRefs            int
	MaxContextRefFieldBytes   int
	MaxContextRefBytes        int
	MaxDeclaredContextBytes   int64
	MaxTurnSpecRefBytes       int
	MaxRestoreDescriptorBytes int
}

// DefaultInputLimits returns the fully resolved production command-envelope
// limits. Transport and App admission layers should use this before hashing or
// registering caller-owned command IDs so the Harness is not the first bound.
func DefaultInputLimits() InputLimits {
	return InputLimits{}.normalized()
}

func (limits InputLimits) normalized() InputLimits {
	if limits.MaxCommandIDBytes <= 0 {
		limits.MaxCommandIDBytes = 4 << 10
	}
	if limits.MaxOperationIDBytes <= 0 {
		limits.MaxOperationIDBytes = 4 << 10
	}
	if limits.MaxAbortReasonBytes <= 0 {
		limits.MaxAbortReasonBytes = 1 << 20
	}
	if limits.MaxTextBytes <= 0 {
		limits.MaxTextBytes = 8 << 20
	}
	if limits.MaxContextRefs <= 0 {
		limits.MaxContextRefs = 2048
	}
	if limits.MaxContextRefFieldBytes <= 0 {
		limits.MaxContextRefFieldBytes = 64 << 10
	}
	if limits.MaxContextRefBytes <= 0 {
		limits.MaxContextRefBytes = 16 << 20
	}
	if limits.MaxDeclaredContextBytes <= 0 {
		limits.MaxDeclaredContextBytes = 256 << 20
	}
	if limits.MaxTurnSpecRefBytes <= 0 {
		limits.MaxTurnSpecRefBytes = 512
	}
	if limits.MaxRestoreDescriptorBytes <= 0 {
		limits.MaxRestoreDescriptorBytes = 8 << 20
	}
	return limits
}

var (
	ErrInvalidBinding       = errors.New("invalid agent runtime binding")
	ErrInvalidCommand       = errors.New("invalid agent runtime command")
	ErrBusy                 = errors.New("agent runtime is busy")
	ErrInvalidCursor        = errors.New("invalid agent runtime cursor")
	ErrCursorExpired        = errors.New("agent runtime cursor is older than the retained event window")
	ErrQueueConflict        = errors.New("agent runtime queue already contains that delivery kind")
	ErrStaleOperation       = errors.New("agent runtime command targets a stale operation")
	ErrHarnessFailed        = errors.New("agent runtime harness failed")
	ErrHarnessClosed        = errors.New("agent runtime harness is closed")
	ErrRuntimeClosed        = errors.New("agent runtime is closed")
	ErrDomainCommitRejected = errors.New("agent domain commit authorization rejected")
	ErrByteBudgetExceeded   = errors.New("agent runtime byte budget exceeded")
	ErrHostEffectRequired   = errors.New("agent runtime host effect requires reconciliation")
	ErrInteractionStale     = errors.New("agent runtime interaction is stale")
)

// ByteBudgetScope identifies the actor-owned payload class that rejected new
// data. Callers can use errors.As without parsing an error string.
type ByteBudgetScope string

const (
	ByteBudgetPendingInput ByteBudgetScope = "pending_input"
	ByteBudgetActiveOutput ByteBudgetScope = "active_output"
	ByteBudgetToolProgress ByteBudgetScope = "tool_progress"
	ByteBudgetHostEffect   ByteBudgetScope = "host_effect"
	ByteBudgetEngineState  ByteBudgetScope = "engine_state"
	ByteBudgetInteraction  ByteBudgetScope = "interaction"
)

// ByteBudgetError is returned before input acceptance or when a provider/tool
// stream cannot be retained completely. Stream overflow durably settles as
// OperationIncomplete; it is never reported as completed.
type ByteBudgetError struct {
	Scope    ByteBudgetScope
	Current  int64
	Incoming int64
	Limit    int64
}

func (e *ByteBudgetError) Error() string {
	if e == nil {
		return ErrByteBudgetExceeded.Error()
	}
	return fmt.Sprintf("%v: scope=%s current=%d incoming=%d limit=%d", ErrByteBudgetExceeded, e.Scope, e.Current, e.Incoming, e.Limit)
}

func (e *ByteBudgetError) Unwrap() error { return ErrByteBudgetExceeded }
