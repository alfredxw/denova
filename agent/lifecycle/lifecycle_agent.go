// Package lifecycle owns Session and Run handles, task-tree admission, queues,
// journal transactions, and event publication. Engine checkpoints remain opaque
// here; candidate state becomes visible only after the journal accepts it.
package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"sync"

	agentengine "github.com/alfredxw/denova/agent/engine"
	agentevent "github.com/alfredxw/denova/agent/lifecycle/event"
	agenttrace "github.com/alfredxw/denova/agent/lifecycle/trace"
	agentschema "github.com/alfredxw/denova/agent/schema"
	agentsession "github.com/alfredxw/denova/agent/session"
)

type Option func(*agentOptions) error

type agentOptions struct {
	store     agentsession.Store
	trace     agenttrace.TraceSink
	runIDs    RunIDGenerator
	cacheKeys agentschema.CacheKeyGenerator
}

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

func WithTrace(sink agenttrace.TraceSink) Option {
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

func WithCacheKeyGenerator(generate agentschema.CacheKeyGenerator) Option {
	return func(options *agentOptions) error {
		if generate == nil {
			return errors.New("Agent Cache Key generator is nil")
		}
		options.cacheKeys = generate
		return nil
	}
}

type sessionOpening struct {
	key  agentsession.Key
	done chan struct{}
}

// Agent owns Session handles and serializes task-tree admission. Journals own
// accepted inputs and execution facts; handles and waiters are process-local.
type Agent struct {
	ctx       context.Context
	cancel    context.CancelFunc
	source    agentengine.Source
	store     agentsession.Store
	trace     agenttrace.TraceSink
	runIDs    RunIDGenerator
	cacheKeys agentschema.CacheKeyGenerator

	mu          sync.RWMutex
	sessions    map[string]*Session
	opening     map[string]sessionOpening
	closed      bool
	admissionMu sync.Mutex
}

func New(lifecycle context.Context, source agentengine.Source, options ...Option) (*Agent, error) {
	if source == nil {
		return nil, errors.New("Agent Definition Source is required")
	}
	if lifecycle == nil {
		lifecycle = context.Background()
	}
	switch static := source.(type) {
	case agentengine.Definition:
		initialized, err := agentengine.InitializeDefinition(lifecycle, static)
		if err != nil {
			return nil, err
		}
		source = initialized
	case *agentengine.Definition:
		if static == nil {
			return nil, errors.New("Agent Definition Source is required")
		}
		initialized, err := agentengine.InitializeDefinition(lifecycle, *static)
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
	return owner, nil
}

func (agent *Agent) nextRunID(key agentsession.Key) (string, error) {
	if agent.runIDs != nil {
		return agent.runIDs(RunIDRequest{Session: key})
	}
	return newPublicID("run"), nil
}

func (agent *Agent) Run(ctx context.Context, input agentschema.Input) (*Run, error) {
	key := agentsession.Key{Namespace: "temporary", ID: newPublicID("session")}
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

func (agent *Agent) Session(ctx context.Context, key agentsession.Key) (*Session, error) {
	if agent == nil {
		return nil, agentschema.ErrAgentClosed
	}
	key, err := agentsession.NormalizeKey(key)
	if err != nil {
		return nil, err
	}
	canonical, err := agentsession.CanonicalKey(key)
	if err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		agent.mu.Lock()
		if agent.closed {
			agent.mu.Unlock()
			return nil, agentschema.ErrAgentClosed
		}
		if existing := agent.sessions[canonical]; existing != nil {
			agent.mu.Unlock()
			return existing, nil
		}
		if pending, found := agent.opening[canonical]; found {
			agent.mu.Unlock()
			select {
			case <-pending.done:
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		if agent.opening == nil {
			agent.opening = make(map[string]sessionOpening)
		}
		done := make(chan struct{})
		agent.opening[canonical] = sessionOpening{key: key, done: done}
		agent.mu.Unlock()
		// Serialize only this identity. A slow journal must not hold the Agent
		// registry lock or prevent unrelated Sessions from being obtained.
		openCtx, cancel := context.WithCancel(ctx)
		stop := context.AfterFunc(agent.ctx, cancel)
		session, err := agent.openSession(openCtx, key)
		stop()
		cancel()
		agent.mu.Lock()
		closed := agent.closed
		if err == nil && !closed {
			agent.sessions[canonical] = session
		}
		agent.mu.Unlock()
		if err == nil && closed {
			err = errors.Join(agentschema.ErrAgentClosed, session.closeWriter())
			session = nil
		}
		agent.mu.Lock()
		delete(agent.opening, canonical)
		close(done)
		agent.mu.Unlock()
		return session, err
	}
}

func (agent *Agent) openSession(ctx context.Context, key agentsession.Key) (*Session, error) {
	log, err := agent.store.Open(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("open Agent Session transcript: %w", err)
	}
	return agent.loadSession(ctx, key, log, sessionWriter)
}

type sessionAccess uint8

const (
	sessionWriter sessionAccess = iota
	sessionInspection
)

func (agent *Agent) loadSession(ctx context.Context, key agentsession.Key, log agentsession.Log, access sessionAccess) (*Session, error) {
	binding := agentengine.BindingRef{Kind: key.Namespace, Key: key.ID, Labels: maps.Clone(key.Attributes)}
	engine, err := agentengine.New(agentengine.Config{
		Source: agent.source, Session: key, Trace: agent.trace, CacheKeys: agent.cacheKeys,
	})
	if err != nil {
		_ = log.Close()
		return nil, err
	}
	session := &Session{
		agent: agent, key: key, binding: binding, engine: engine, log: log,
		capabilities: make(map[string]json.RawMessage), durableCapabilities: make(map[string]json.RawMessage),
		runs:            make(map[string]*Run),
		inputs:          make(map[string]*acceptedInput),
		controlReceipts: make(map[string]persistedControlReceipt),
		observers:       make(map[uint64]*sessionObserver),
		taskCompletions: newTaskCompletionMailbox(),
	}
	if canonical, ok := log.(agentsession.CanonicalMessageLog); ok {
		session.canonicalMessages = canonical.CanonicalMessages()
	}
	if err := session.replay(ctx, access); err != nil {
		_ = log.Close()
		return nil, err
	}

	self, _ := agentsession.CanonicalKey(key)
	for _, child := range session.recovery.Children.Pending {
		parent, err := ParentSessionKey(child.Key)
		canonical, _ := agentsession.CanonicalKey(parent)
		if err != nil || canonical != self {
			_ = log.Close()
			return nil, fmt.Errorf("child task checkpoint parent identity mismatch")
		}
	}
	if session.revision == 0 {
		session.recovery.Children = childTasks{Initialized: true, Pending: make(map[string]ChildTask)}
	}
	return session, nil
}

// InspectSession reads live state or replays an existing journal without
// registering an execution handle, taking a writer lease, or resuming work.
func (agent *Agent) InspectSession(ctx context.Context, key agentsession.Key) (agentevent.SessionSnapshot, error) {
	canonical, err := agentsession.CanonicalKey(key)
	if err != nil {
		return agentevent.SessionSnapshot{}, err
	}
	agent.mu.RLock()
	live, closed := agent.sessions[canonical], agent.closed
	agent.mu.RUnlock()
	if closed {
		return agentevent.SessionSnapshot{}, agentschema.ErrAgentClosed
	}
	if live != nil {
		live.mu.RLock()
		snapshot := live.snapshotLocked()
		live.mu.RUnlock()
		return snapshot, nil
	}
	reader, ok := agent.store.(agentsession.ReaderStore)
	if !ok {
		return agentevent.SessionSnapshot{}, agentschema.ErrCapabilityUnsupported
	}
	log, err := reader.OpenReader(ctx, key)
	if err != nil {
		return agentevent.SessionSnapshot{}, err
	}
	defer log.Close()
	session, err := agent.loadSession(ctx, key, log, sessionInspection)
	if err != nil {
		return agentevent.SessionSnapshot{}, err
	}
	defer func() {
		for _, run := range session.runs {
			run.cancel()
		}
	}()
	return session.snapshotLocked(), nil
}

func (agent *Agent) ListSessions(ctx context.Context, selector agentsession.Selector) ([]agentsession.Key, error) {
	if agent == nil {
		return nil, agentschema.ErrAgentClosed
	}
	agent.mu.RLock()
	closed := agent.closed
	agent.mu.RUnlock()
	if closed {
		return nil, agentschema.ErrAgentClosed
	}
	return agent.store.List(ctx, selector)
}

// CountActiveSessions reports matching in-process Runs without opening durable
// terminal Sessions. Active Runs are process-owned, so every one of them must
// already have a live Session binding.
func (agent *Agent) CountActiveSessions(ctx context.Context, selector agentsession.Selector) (int, error) {
	if agent == nil {
		return 0, agentschema.ErrAgentClosed
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
		return 0, agentschema.ErrAgentClosed
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
			if errors.Is(err, agentschema.ErrSessionClosed) {
				continue
			}
			return 0, err
		}
		if found && active != nil && !active.isSuspended() {
			count++
		}
	}
	return count, nil
}

func (agent *Agent) CloseSessions(ctx context.Context, selector agentsession.Selector) error {
	if err := selector.Validate(); err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		agent.mu.RLock()
		var waiting <-chan struct{}
		for _, pending := range agent.opening {
			if sessionSelectorMatchesTree(selector, pending.key) {
				waiting = pending.done
				break
			}
		}
		agent.mu.RUnlock()
		if waiting == nil {
			break
		}
		select {
		case <-waiting:
		case <-ctx.Done():
			return ctx.Err()
		}
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
	opening := make([]chan struct{}, 0, len(agent.opening))
	for _, pending := range agent.opening {
		opening = append(opening, pending.done)
	}
	agent.mu.Unlock()
	agent.cancel()
	for _, done := range opening {
		<-done
	}
	var result error
	for _, session := range sessions {
		result = errors.Join(result, session.Close(ctx))
	}
	return result
}

// SessionExists checks one exact identity without replay or catalog discovery.
func (agent *Agent) SessionExists(ctx context.Context, key agentsession.Key) (bool, error) {
	canonical, err := agentsession.CanonicalKey(key)
	if err != nil {
		return false, err
	}
	agent.mu.RLock()
	live, closed := agent.sessions[canonical], agent.closed
	agent.mu.RUnlock()
	if closed {
		return false, agentschema.ErrAgentClosed
	}
	if live != nil {
		return true, nil
	}
	reader, ok := agent.store.(agentsession.ReaderStore)
	if !ok {
		keys, err := agent.ListSessions(ctx, agentsession.Selector{Namespace: key.Namespace, ID: key.ID, Attributes: key.Attributes})
		if err != nil {
			return false, err
		}
		for _, candidate := range keys {
			identity, err := agentsession.CanonicalKey(candidate)
			if err != nil {
				return false, err
			}
			if identity == canonical {
				return true, nil
			}
		}
		return false, nil
	}
	log, err := reader.OpenReader(ctx, key)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, log.Close()
}
