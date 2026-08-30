package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"sync"

	runstate "github.com/alfredxw/denova/agent/internal/runstate"
	agentsession "github.com/alfredxw/denova/agent/session"
)

type Option func(*agentOptions) error

type agentOptions struct {
	store     agentsession.Store
	trace     TraceSink
	runIDs    RunIDGenerator
	cacheKeys CacheKeyGenerator
}

// CacheKeyGenerator derives an opaque, stable provider cache-routing key.
type CacheKeyGenerator func(SessionKey) (string, error)

func WithSessionStore(store agentsession.Store, constructionErrors ...error) Option {
	return func(options *agentOptions) error {
		if err := errors.Join(constructionErrors...); err != nil {
			return fmt.Errorf("construct Agent Session Store: %w", err)
		}
		if store == nil {
			return errors.New("Agent Session Store is nil")
		}
		options.store = store
		return nil
	}
}

func WithTrace(sink TraceSink) Option {
	return func(options *agentOptions) error {
		options.trace = sink
		return nil
	}
}

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

// Agent owns a small set of in-process Sessions. Session stores contain only
// transcript/capability snapshots; live Runs, event cursors, queues, and
// interactions deliberately remain process-local.
type Agent struct {
	ctx       context.Context
	cancel    context.CancelFunc
	source    Source
	store     agentsession.Store
	trace     TraceSink
	runIDs    RunIDGenerator
	cacheKeys CacheKeyGenerator

	mu       sync.RWMutex
	sessions map[string]*Session
	closed   bool
}

func New(lifecycle context.Context, source Source, options ...Option) (*Agent, error) {
	if source == nil {
		return nil, errors.New("Agent Definition Source is required")
	}
	if lifecycle == nil {
		lifecycle = context.Background()
	}
	switch static := source.(type) {
	case Definition:
		initialized, err := initializeDefinition(lifecycle, static)
		if err != nil {
			return nil, err
		}
		source = initialized
	case *Definition:
		if static == nil {
			return nil, errors.New("Agent Definition Source is required")
		}
		initialized, err := initializeDefinition(lifecycle, *static)
		if err != nil {
			return nil, err
		}
		source = initialized
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
		ctx: ctx, cancel: cancel, source: source, store: configured.store,
		trace: configured.trace, runIDs: configured.runIDs,
		cacheKeys: configured.cacheKeys, sessions: make(map[string]*Session),
	}
	if owner.cacheKeys == nil {
		owner.cacheKeys = defaultCacheKey
	}
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

func (agent *Agent) nextRunID(key SessionKey) (string, error) {
	if agent.runIDs != nil {
		return agent.runIDs(RunIDRequest{Session: key})
	}
	return newPublicID("run"), nil
}

func (agent *Agent) Run(ctx context.Context, input Input) (*Run, error) {
	key := SessionKey{Namespace: "temporary", ID: newPublicID("session")}
	session, err := agent.Session(ctx, key)
	if err != nil {
		return nil, err
	}
	run, err := session.start(ctx, input, runOwnsTemporarySession)
	if err != nil {
		_ = session.Delete(context.Background())
		return nil, err
	}
	return run, nil
}

func (agent *Agent) Session(ctx context.Context, key SessionKey) (*Session, error) {
	if agent == nil {
		return nil, ErrAgentClosed
	}
	key, err := agentsession.NormalizeKey(key)
	if err != nil {
		return nil, err
	}
	canonical, err := agentsession.CanonicalKey(key)
	if err != nil {
		return nil, err
	}
	agent.mu.Lock()
	if agent.closed {
		agent.mu.Unlock()
		return nil, ErrAgentClosed
	}
	if existing := agent.sessions[canonical]; existing != nil {
		agent.mu.Unlock()
		return existing, nil
	}
	// Session creation is intentionally serialized. It keeps the ownership
	// model obvious and prevents two callers from competing for the same Store
	// lease before either handle has entered the registry.
	defer agent.mu.Unlock()

	if ctx == nil {
		ctx = context.Background()
	}
	log, err := agent.store.Open(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("open Agent Session transcript: %w", err)
	}
	binding := runstate.BindingRef{Kind: key.Namespace, Key: key.ID, Labels: maps.Clone(key.Attributes)}
	engine, err := (&definitionEngineFactory{
		source: agent.source, trace: agent.trace, cacheKeys: agent.cacheKeys,
	}).NewEngine(agent.ctx, binding)
	if err != nil {
		_ = log.Close()
		return nil, err
	}
	session := &Session{
		agent: agent, key: key, binding: binding, engine: engine, log: log,
		capabilities: make(map[string]json.RawMessage), runs: make(map[string]*Run),
		observers:       make(map[uint64]*sessionObserver),
		taskCompletions: newTaskCompletionMailbox(),
	}
	if err := session.replay(ctx); err != nil {
		_ = log.Close()
		return nil, err
	}

	agent.sessions[canonical] = session
	return session, nil
}

func (agent *Agent) ListSessions(ctx context.Context, selector SessionSelector) ([]SessionKey, error) {
	if agent == nil {
		return nil, ErrAgentClosed
	}
	agent.mu.RLock()
	closed := agent.closed
	agent.mu.RUnlock()
	if closed {
		return nil, ErrAgentClosed
	}
	return agent.store.List(ctx, selector)
}

// CountActiveSessions reports matching in-process Runs without opening durable
// terminal Sessions. Active Runs are process-owned, so every one of them must
// already have a live Session binding.
func (agent *Agent) CountActiveSessions(ctx context.Context, selector SessionSelector) (int, error) {
	if agent == nil {
		return 0, ErrAgentClosed
	}
	if err := selector.Validate(); err != nil {
		return 0, err
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
	}
	agent.mu.RLock()
	if agent.closed {
		agent.mu.RUnlock()
		return 0, ErrAgentClosed
	}
	sessions := make([]*Session, 0, len(agent.sessions))
	for _, session := range agent.sessions {
		if selector.Matches(session.key) {
			sessions = append(sessions, session)
		}
	}
	agent.mu.RUnlock()
	count := 0
	for _, session := range sessions {
		active, found, err := session.Active(ctx)
		if err != nil {
			if errors.Is(err, ErrSessionClosed) {
				continue
			}
			return 0, err
		}
		if found && active != nil {
			count++
		}
	}
	return count, nil
}

func (agent *Agent) CloseSessions(ctx context.Context, selector SessionSelector) error {
	if err := selector.Validate(); err != nil {
		return err
	}
	agent.mu.RLock()
	open := make([]*Session, 0, len(agent.sessions))
	for _, session := range agent.sessions {
		if sessionSelectorMatchesTree(selector, session.key) {
			open = append(open, session)
		}
	}
	agent.mu.RUnlock()
	var result error
	for _, session := range open {
		result = errors.Join(result, session.Close(ctx))
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
	sessions := make([]*Session, 0, len(agent.sessions))
	for _, session := range agent.sessions {
		sessions = append(sessions, session)
	}
	agent.mu.Unlock()
	agent.cancel()
	var result error
	for _, session := range sessions {
		result = errors.Join(result, session.Close(ctx))
	}
	return result
}
