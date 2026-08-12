package agent

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"sort"
	"sync"

	runstate "github.com/alfredxw/denova/agent/internal/runtime"
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
	store     agentsession.Store
	limits    Limits
	trace     TraceSink
	runIDs    RunIDGenerator
	cacheKeys CacheKeyGenerator
}

// CacheKeyGenerator derives a privacy-safe, stable provider cache-routing key
// from public Session identity. Implementations must return an opaque value;
// raw paths, user identifiers, and other PII must never be returned.
type CacheKeyGenerator func(SessionKey) (string, error)

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

// WithRunIDGenerator delegates externally visible Run identity to the
// embedding application. Storage, tracing, and display layers can then share
// that exact identity without Agent knowing their naming conventions.
func WithRunIDGenerator(generate RunIDGenerator) Option {
	return func(options *agentOptions) error {
		if generate == nil {
			return errors.New("Agent Run ID generator is nil")
		}
		options.runIDs = generate
		return nil
	}
}

func WithCacheKeyGenerator(generate CacheKeyGenerator) Option {
	return func(options *agentOptions) error {
		if generate == nil {
			return errors.New("Agent Cache Key generator is nil")
		}
		options.cacheKeys = generate
		return nil
	}
}

// Agent is the deep public Module that owns Session, Run, durable admission,
// model/tool execution, recovery, and Event publication.
type Agent struct {
	ctx       context.Context
	cancel    context.CancelFunc
	source    Source
	store     agentsession.Store
	trace     TraceSink
	cacheKeys CacheKeyGenerator
	runtime   *runstate.Runtime
	// projectionTextMaxBytes is the public Session status bound. Keep it next
	// to the Runtime configuration so SessionSnapshot never has to expose the
	// internal Harness merely to project queued display text safely.
	projectionTextMaxBytes int

	mu                  sync.RWMutex
	sessions            map[string]SessionKey
	sessionOpenSequence uint64
	sessionOpenings     map[uint64]sessionOpening
	sessionDeletion     *sessionDeletionFence
	closed              bool
}

// sessionOpening bridges the small interval between the public Agent registry
// and Runtime.Open. DeleteSessions waits these calls before installing the
// runtime scope barrier, so an Open that has passed the public fence cannot
// appear between runtime eviction and Store deletion.
type sessionOpening struct {
	key  SessionKey
	done chan struct{}
}

type sessionDeletionFence struct {
	matches func(SessionKey) bool
	done    chan struct{}
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
		sessions:               make(map[string]SessionKey),
		sessionOpenings:        make(map[uint64]sessionOpening),
		projectionTextMaxBytes: normalizedProjectionTextMaxBytes(configured.limits.ProjectionTextMaxBytes),
	}
	cacheKeys := configured.cacheKeys
	if cacheKeys == nil {
		cacheKeys = defaultCacheKey
	}
	owner.cacheKeys = cacheKeys
	factory := &definitionEngineFactory{
		source: source, persistent: isPersistentStore(configured.store), trace: configured.trace,
		cacheKeys: cacheKeys,
	}
	runtime, err := runstate.NewRuntime(
		factory, journalStoreFor(configured.store), runtimeConfig(configured.limits, ctx, configured.runIDs),
	)
	if err != nil {
		cancel()
		return nil, err
	}
	owner.runtime = runtime
	return owner, nil
}

func defaultCacheKey(key SessionKey) (string, error) {
	canonical, err := agentsession.CanonicalKey(key)
	if err != nil {
		return "", err
	}
	digest, err := hashCanonical(struct {
		Version uint16
		Session string
	}{1, canonical})
	if err != nil {
		return "", err
	}
	return "agent-" + digest[:32], nil
}

func normalizedProjectionTextMaxBytes(limit int) int {
	if limit <= 0 {
		return 1 << 20
	}
	return limit
}

func runtimeConfig(limits Limits, lifecycle context.Context, runIDs RunIDGenerator) runstate.RuntimeConfig {
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
	if runIDs != nil {
		config.OperationIDGenerator = func(binding runstate.BindingRef) (runstate.OperationID, error) {
			id, err := runIDs(RunIDRequest{Session: SessionKey{
				Namespace: binding.Kind, ID: binding.Key, Attributes: maps.Clone(binding.Labels),
			}})
			return runstate.OperationID(id), err
		}
	}
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
	run, err := session.start(ctx, input, "", false, runOwnsTemporarySession)
	if err != nil {
		if deleteErr := session.Delete(context.Background()); deleteErr != nil {
			return nil, errors.Join(err, fmt.Errorf("delete temporary Agent Session after rejected Run: %w", deleteErr))
		}
		return nil, err
	}
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
	var openingID uint64
	var opening sessionOpening
	for {
		agent.mu.Lock()
		if agent.closed {
			agent.mu.Unlock()
			return nil, ErrAgentClosed
		}
		if deletion := agent.sessionDeletion; deletion != nil && deletion.matches != nil && deletion.matches(key) {
			done := deletion.done
			agent.mu.Unlock()
			select {
			case <-done:
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-agent.ctx.Done():
				return nil, ErrAgentClosed
			}
		}
		agent.sessionOpenSequence++
		openingID = agent.sessionOpenSequence
		opening = sessionOpening{key: key, done: make(chan struct{})}
		agent.sessionOpenings[openingID] = opening
		agent.mu.Unlock()
		break
	}
	binding := bindingForSession(key)
	harness, err := agent.runtime.Open(ctx, binding)
	canonical, _ := agentsession.CanonicalKey(key)
	agent.mu.Lock()
	delete(agent.sessionOpenings, openingID)
	closed := agent.closed
	if err == nil && !closed {
		agent.sessions[canonical] = key
	}
	close(opening.done)
	agent.mu.Unlock()
	if err != nil {
		return nil, mapRuntimeError(err)
	}
	if closed {
		_ = agent.runtime.CloseBinding(context.Background(), binding)
		return nil, ErrAgentClosed
	}
	return &Session{agent: agent, key: key, binding: binding, harness: harness}, nil
}

// ListSessions returns the exact durable Session identities in one constrained
// scope. It is a read-only catalog operation; callers must still reopen a
// returned key through Session before observing or controlling its lifecycle.
func (agent *Agent) ListSessions(ctx context.Context, selector SessionSelector) ([]SessionKey, error) {
	if agent == nil || agent.runtime == nil || agent.store == nil {
		return nil, ErrAgentClosed
	}
	if err := selector.Validate(); err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	keys, err := agent.store.List(ctx, selector)
	if err != nil {
		return nil, err
	}
	byCanonical := make(map[string]SessionKey, len(keys))
	for _, key := range keys {
		canonical, canonicalErr := agentsession.CanonicalKey(key)
		if canonicalErr != nil {
			return nil, canonicalErr
		}
		byCanonical[canonical] = key
	}
	// Include a binding admitted in this process even when a custom Store's
	// catalog is eventually consistent. Canonical sorting keeps callers and
	// restore fingerprints deterministic across implementations.
	agent.mu.RLock()
	for canonical, key := range agent.sessions {
		if selector.Matches(key) {
			byCanonical[canonical] = key
		}
	}
	agent.mu.RUnlock()
	canonicalKeys := make([]string, 0, len(byCanonical))
	for canonical := range byCanonical {
		canonicalKeys = append(canonicalKeys, canonical)
	}
	sort.Strings(canonicalKeys)
	result := make([]SessionKey, len(canonicalKeys))
	for index, canonical := range canonicalKeys {
		result[index] = byCanonical[canonical]
	}
	return result, nil
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
	case errors.Is(err, runstate.ErrInvalidCommand):
		return errors.Join(ErrInvalidInput, err)
	case errors.Is(err, runstate.ErrBusy), errors.Is(err, runstate.ErrQueueConflict):
		return errors.Join(ErrSessionBusy, err)
	case errors.Is(err, runstate.ErrCursorExpired):
		return errors.Join(ErrCursorExpired, err)
	case errors.Is(err, runstate.ErrInteractionStale):
		return errors.Join(ErrInteractionStale, err)
	case errors.Is(err, runstate.ErrRecoveryActionChanged):
		return errors.Join(ErrRecoveryStale, err)
	case errors.Is(err, runstate.ErrDomainCommitRejected):
		return errors.Join(ErrCanonicalCommitRejected, err)
	case errors.Is(err, runstate.ErrHostEffectRequired):
		return errors.Join(ErrRecoveryRequired, err)
	default:
		return err
	}
}
