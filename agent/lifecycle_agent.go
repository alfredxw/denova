package agent

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"sync"

	runstate "github.com/alfredxw/denova/agent/runtime"
	agentsession "github.com/alfredxw/denova/agent/session"
)

// Limits configures Agent-owned transport and in-memory recovery bounds. Zero
// values select the high production defaults from the durable runtime.
type Limits struct {
	ObservationBuffer      int
	MaxOpenSessions        int
	RetainedEventLimit     int
	RetainedMessageLimit   int
	RetainedCommandLimit   int
	ProjectionTextMaxBytes int

	MaxInputBytes          int
	MaxContextFragments    int
	MaxEngineStateBytes    int64
	MaxInteractionBytes    int64
	MaxPendingInteractions int
}

type Option func(*agentOptions) error

type agentOptions struct {
	store  agentsession.Store
	limits Limits
	trace  TraceSink
}

func WithSessionStore(store agentsession.Store) Option {
	return func(options *agentOptions) error {
		if store == nil {
			return errors.New("Agent Session Store is nil")
		}
		options.store = store
		return nil
	}
}

func WithLimits(limits Limits) Option {
	return func(options *agentOptions) error {
		options.limits = limits
		return nil
	}
}

func WithTrace(sink TraceSink) Option {
	return func(options *agentOptions) error {
		options.trace = sink
		return nil
	}
}

// Agent is the deep public Module that owns Session, Run, durable admission,
// model/tool execution, recovery, and Event publication.
type Agent struct {
	ctx     context.Context
	cancel  context.CancelFunc
	source  Source
	store   agentsession.Store
	trace   TraceSink
	runtime *runstate.Runtime

	mu       sync.RWMutex
	sessions map[string]SessionKey
	closed   bool
}

func New(lifecycle context.Context, source Source, options ...Option) (*Agent, error) {
	if source == nil {
		return nil, errors.New("Agent Definition Source is required")
	}
	if lifecycle == nil {
		lifecycle = context.Background()
	}
	configured := agentOptions{store: agentsession.Memory()}
	for index, option := range options {
		if option == nil {
			continue
		}
		if err := option(&configured); err != nil {
			return nil, fmt.Errorf("Agent option %d: %w", index, err)
		}
	}
	ctx, cancel := context.WithCancel(lifecycle)
	owner := &Agent{
		ctx: ctx, cancel: cancel, source: source, store: configured.store, trace: configured.trace,
		sessions: make(map[string]SessionKey),
	}
	factory := &definitionEngineFactory{source: source, persistent: isPersistentStore(configured.store), trace: configured.trace}
	runtime, err := runstate.NewRuntime(factory, runtimeStoreAdapter{store: configured.store}, runtimeConfig(configured.limits, ctx))
	if err != nil {
		cancel()
		return nil, err
	}
	owner.runtime = runtime
	return owner, nil
}

func runtimeConfig(limits Limits, lifecycle context.Context) runstate.RuntimeConfig {
	config := runstate.RuntimeConfig{
		Lifecycle:              lifecycle,
		ObservationBuffer:      limits.ObservationBuffer,
		MaxOpenBindings:        limits.MaxOpenSessions,
		RetainedEventLimit:     limits.RetainedEventLimit,
		RetainedMessageLimit:   limits.RetainedMessageLimit,
		RetainedCommandLimit:   limits.RetainedCommandLimit,
		ProjectionTextMaxBytes: limits.ProjectionTextMaxBytes,
	}
	config.InputLimits.MaxTextBytes = limits.MaxInputBytes
	config.InputLimits.MaxContextRefs = limits.MaxContextFragments
	config.MemoryLimits.MaxEngineStateBytes = limits.MaxEngineStateBytes
	config.MemoryLimits.MaxInteractionBytes = limits.MaxInteractionBytes
	config.MemoryLimits.MaxPendingInteractions = limits.MaxPendingInteractions
	return config
}

func isPersistentStore(store agentsession.Store) bool {
	volatile, ok := store.(agentsession.VolatileStore)
	return !ok || !volatile.Volatile()
}

func (agent *Agent) Run(ctx context.Context, input Input) (*Run, error) {
	key := SessionKey{Namespace: "temporary", ID: newPublicID("session")}
	session, err := agent.Session(ctx, key)
	if err != nil {
		return nil, err
	}
	run, err := session.Run(ctx, input)
	if err != nil {
		_ = session.Close(context.Background())
		return nil, err
	}
	run.closeSessionWhenSettled()
	return run, nil
}

func (agent *Agent) Session(ctx context.Context, key SessionKey) (*Session, error) {
	if agent == nil || agent.runtime == nil {
		return nil, ErrAgentClosed
	}
	key, err := agentsession.NormalizeKey(key)
	if err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	agent.mu.RLock()
	closed := agent.closed
	agent.mu.RUnlock()
	if closed {
		return nil, ErrAgentClosed
	}
	binding := bindingForSession(key)
	harness, err := agent.runtime.Open(ctx, binding)
	if err != nil {
		return nil, mapRuntimeError(err)
	}
	canonical, _ := agentsession.CanonicalKey(key)
	agent.mu.Lock()
	agent.sessions[canonical] = key
	agent.mu.Unlock()
	return &Session{agent: agent, key: key, binding: binding, harness: harness}, nil
}

func (agent *Agent) CloseSessions(ctx context.Context, selector SessionSelector) error {
	if agent == nil || agent.runtime == nil {
		return ErrAgentClosed
	}
	if err := selector.Validate(); err != nil {
		return err
	}
	agent.mu.RLock()
	keys := make([]SessionKey, 0, len(agent.sessions))
	for _, key := range agent.sessions {
		if selector.Matches(key) {
			keys = append(keys, key)
		}
	}
	agent.mu.RUnlock()
	var result error
	for _, key := range keys {
		if err := agent.runtime.CloseBinding(ctx, bindingForSession(key)); err != nil {
			result = errors.Join(result, mapRuntimeError(err))
		}
		canonical, _ := agentsession.CanonicalKey(key)
		agent.mu.Lock()
		delete(agent.sessions, canonical)
		agent.mu.Unlock()
	}
	return result
}

func (agent *Agent) Close(ctx context.Context) error {
	if agent == nil {
		return nil
	}
	agent.mu.Lock()
	if agent.closed {
		agent.mu.Unlock()
		return nil
	}
	agent.closed = true
	agent.mu.Unlock()
	agent.cancel()
	if agent.runtime == nil {
		return nil
	}
	return mapRuntimeError(agent.runtime.Close(ctx))
}

func bindingForSession(key SessionKey) runstate.BindingRef {
	return runstate.BindingRef{Kind: key.Namespace, Key: key.ID, Labels: maps.Clone(key.Attributes)}
}

func mapRuntimeError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, runstate.ErrRuntimeClosed), errors.Is(err, runstate.ErrHarnessClosed):
		return errors.Join(ErrAgentClosed, err)
	case errors.Is(err, runstate.ErrBusy), errors.Is(err, runstate.ErrQueueConflict):
		return errors.Join(ErrSessionBusy, err)
	case errors.Is(err, runstate.ErrCursorExpired):
		return errors.Join(ErrCursorExpired, err)
	case errors.Is(err, runstate.ErrInteractionStale):
		return errors.Join(ErrInteractionStale, err)
	case errors.Is(err, runstate.ErrRecoveryActionChanged):
		return errors.Join(ErrRecoveryStale, err)
	case errors.Is(err, runstate.ErrHostEffectRequired):
		return errors.Join(ErrRecoveryRequired, err)
	default:
		return err
	}
}
