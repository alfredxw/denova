package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
)

// Runtime owns the durable actor for each exact binding. Open, projection, and
// close owners are process-lifetime operations: a caller may stop waiting for
// one without releasing its serialization fence early.
type Runtime struct {
	mu       sync.Mutex
	engines  EngineFactory
	journals JournalStore
	config   RuntimeConfig

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}

	harness    map[string]*Harness
	opening    map[string]*openCall
	projecting map[string]*projectCall
	closing    map[string]*closeCall
	scopes     map[uint64]*scopeCloseCall
	nextScope  uint64

	closed   bool
	closeErr error
}

type openCall struct {
	ready   chan struct{}
	ref     BindingRef
	harness *Harness
	err     error
}

type projectCall struct {
	ready    chan struct{}
	ref      BindingRef
	snapshot StatusSnapshot
	err      error
}

type closeCall struct {
	ready chan struct{}
	ref   BindingRef
	err   error
}

type scopeCloseCall struct {
	ready    chan struct{}
	selector BindingSelector
	err      error
}

// NewRuntime constructs one binding registry. Lifecycle must represent the
// owning App/workspace process, never an HTTP request.
func NewRuntime(engines EngineFactory, journals JournalStore, config RuntimeConfig) (*Runtime, error) {
	if engines == nil {
		return nil, fmt.Errorf("agent engine factory is required")
	}
	if journals == nil {
		return nil, fmt.Errorf("agent journal store is required")
	}
	config = normalizeRuntimeConfig(config)
	lifecycle := config.Lifecycle
	if lifecycle == nil {
		lifecycle = context.Background()
	}
	ctx, cancel := context.WithCancel(lifecycle)
	return &Runtime{
		engines: engines, journals: journals, config: config,
		ctx: ctx, cancel: cancel, done: make(chan struct{}),
		harness: make(map[string]*Harness), opening: make(map[string]*openCall),
		projecting: make(map[string]*projectCall), closing: make(map[string]*closeCall),
		scopes: make(map[uint64]*scopeCloseCall),
	}, nil
}

// ValidateCommandID applies this Runtime's normalized durable envelope before
// an adapter derives fingerprints, opens a binding, or allocates registry
// state. It intentionally exposes only command identity validation rather than
// the complete mutable RuntimeConfig.
func (r *Runtime) ValidateCommandID(commandID string) error {
	if r == nil {
		return ErrRuntimeClosed
	}
	return ValidateCommandID(commandID, r.config.InputLimits)
}

func normalizeRuntimeConfig(config RuntimeConfig) RuntimeConfig {
	if config.ObservationBuffer <= 0 {
		config.ObservationBuffer = 256
	}
	if config.RetainedEventLimit <= 0 {
		config.RetainedEventLimit = 4096
	}
	if config.RetainedMessageLimit <= 0 {
		config.RetainedMessageLimit = 512
	}
	if config.RetainedCommandLimit <= 0 {
		config.RetainedCommandLimit = 4096
	}
	if config.ProjectionTextMaxBytes <= 0 {
		config.ProjectionTextMaxBytes = 1 << 20
	}
	config.MemoryLimits = config.MemoryLimits.normalized()
	config.InputLimits = config.InputLimits.normalized()
	return config
}

// Open returns the single durable actor for binding. The owner open uses the
// Runtime lifecycle; ctx only controls how long this caller waits.
func (r *Runtime) Open(ctx context.Context, binding Binding) (*Harness, error) {
	if r == nil {
		return nil, ErrInvalidBinding
	}
	ref, err := BindingReference(binding)
	if err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	key := bindingJournalKey(ref)
	for {
		r.mu.Lock()
		if r.closed {
			r.mu.Unlock()
			return nil, ErrRuntimeClosed
		}
		if pending := r.matchingScopeCloseLocked(ref); pending != nil {
			r.mu.Unlock()
			if err := waitScopeClose(ctx, r.ctx, pending); err != nil {
				return nil, err
			}
			continue
		}
		if pending := r.closing[key]; pending != nil {
			r.mu.Unlock()
			if err := waitCloseCall(ctx, r.ctx, pending); err != nil {
				return nil, err
			}
			continue
		}
		if h := r.harness[key]; h != nil {
			if h.terminalError() == nil {
				r.mu.Unlock()
				return h, nil
			}
			// A terminal actor has already released, or is about to release, its
			// lease. Never hand stale in-memory state back to another command.
			delete(r.harness, key)
		}
		if pending := r.opening[key]; pending != nil {
			r.mu.Unlock()
			h, err := waitOpenCall(ctx, r.ctx, pending)
			if err != nil {
				return nil, err
			}
			return h, nil
		}
		if pending := r.projecting[key]; pending != nil {
			r.mu.Unlock()
			if _, err := waitProjectCall(ctx, r.ctx, pending); err != nil {
				return nil, err
			}
			continue
		}
		pending := &openCall{ready: make(chan struct{}), ref: ref}
		r.opening[key] = pending
		r.mu.Unlock()
		go r.finishOpen(ref, key, pending)
		return waitOpenCall(ctx, r.ctx, pending)
	}
}

// BindingReference returns the validated, immutable identity carried by one
// supported Binding. Adapters use it to prove that process-local execution
// state matches the durable actor selected by Runtime.Open.
func BindingReference(binding Binding) (BindingRef, error) {
	if binding == nil {
		return BindingRef{}, ErrInvalidBinding
	}
	ref := binding.bindingRef()
	if err := validateBinding(ref); err != nil {
		return BindingRef{}, err
	}
	return ref, nil
}

func (r *Runtime) finishOpen(ref BindingRef, key string, pending *openCall) {
	completed := false
	defer func() {
		if recovered := recover(); recovered != nil && !completed {
			r.mu.Lock()
			if r.opening[key] == pending {
				delete(r.opening, key)
				pending.err = ownerPanicError("open binding", recovered)
				close(pending.ready)
			}
			r.mu.Unlock()
		}
	}()
	h, err := r.openHarness(ref, key)
	r.mu.Lock()
	if r.closed && err == nil {
		// Do not publish an actor after Runtime.Close captured its registry.
		r.mu.Unlock()
		closeErr := safeHarnessClose(h)
		err = errors.Join(ErrRuntimeClosed, closeErr)
		r.mu.Lock()
	}
	if r.opening[key] == pending {
		delete(r.opening, key)
	}
	if err == nil {
		r.harness[key] = h
		pending.harness = h
	}
	pending.err = err
	close(pending.ready)
	completed = true
	r.mu.Unlock()
}

func waitOpenCall(ctx, lifecycle context.Context, pending *openCall) (*Harness, error) {
	select {
	case <-pending.ready:
		return pending.harness, pending.err
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-contextDone(lifecycle):
		return nil, ErrRuntimeClosed
	}
}

func (r *Runtime) openHarness(ref BindingRef, key string) (_ *Harness, resultErr error) {
	j, err := r.journals.OpenJournal(r.ctx, key)
	if err != nil {
		return nil, fmt.Errorf("open agent journal: %w", err)
	}
	leaseOwned := true
	defer func() {
		if !leaseOwned {
			return
		}
		if closeErr := safeJournalClose(j); closeErr != nil && resultErr == nil {
			resultErr = fmt.Errorf("release agent journal lease: %w", closeErr)
		}
	}()
	engine, err := r.engines.NewEngine(r.ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("create agent engine: %w", err)
	}
	h, err := newHarness(
		r.ctx,
		r.ctx,
		ref,
		engine,
		j,
		r.config.ObservationBuffer,
		r.config.InputLimits,
		r.config.ProjectionTextMaxBytes,
		r.config.RetainedEventLimit,
		r.config.RetainedMessageLimit,
		r.config.RetainedCommandLimit,
		r.config.MemoryLimits,
	)
	if err != nil {
		return nil, fmt.Errorf("initialize agent harness: %w", err)
	}
	leaseOwned = false
	return h, nil
}

func validateBinding(ref BindingRef) error {
	invalid := func(reason string) error {
		return fmt.Errorf("%w: %s", ErrInvalidBinding, reason)
	}
	switch ref.Kind {
	case BindingWriting:
		if ref.Workspace == "" || ref.SessionID == "" {
			return invalid("writing bindings require workspace and session")
		}
		if ref.StoryID != "" || ref.BranchID != "" || ref.TaskID != "" {
			return invalid("writing binding contains fields owned by another kind")
		}
		if ref.Profile != ProfileWriting && ref.Profile != ProfileConfigManager && ref.Profile != ProfileImage {
			return invalid("profile is not valid for a writing binding")
		}
	case BindingGame:
		if ref.Workspace == "" || ref.StoryID == "" || ref.BranchID == "" {
			return invalid("game bindings require workspace, story, and branch")
		}
		if ref.TaskID != "" {
			return invalid("game binding contains fields owned by another kind")
		}
		if ref.Profile != ProfileGame && ref.Profile != ProfileDirector {
			return invalid("profile is not valid for a game binding")
		}
	case BindingAutomation:
		if ref.SessionID == "" || ref.TaskID == "" {
			return invalid("automation bindings require session and task")
		}
		if ref.StoryID != "" || ref.BranchID != "" {
			return invalid("automation binding contains fields owned by another kind")
		}
		if ref.Profile != ProfileAutomation {
			return invalid("profile is not valid for an automation binding")
		}
	default:
		return invalid("unknown binding kind")
	}
	return nil
}

func validateBindingSelector(selector BindingSelector) error {
	if selector == (BindingSelector{}) {
		return fmt.Errorf("%w: binding selector must constrain at least one field", ErrInvalidBinding)
	}
	if selector.Kind != "" && selector.Kind != BindingWriting && selector.Kind != BindingGame && selector.Kind != BindingAutomation {
		return fmt.Errorf("%w: unknown selector binding kind", ErrInvalidBinding)
	}
	switch selector.Profile {
	case "", ProfileWriting, ProfileGame, ProfileAutomation, ProfileConfigManager, ProfileImage, ProfileDirector:
		return nil
	default:
		return fmt.Errorf("%w: unknown selector profile", ErrInvalidBinding)
	}
}

func bindingJournalKey(ref BindingRef) string {
	encoded, err := json.Marshal(ref)
	if err != nil {
		// BindingRef contains only strings and closed enum aliases; keep failure
		// impossible at call sites while retaining a deterministic fallback.
		return fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s", ref.Kind, ref.Profile, ref.Workspace, ref.SessionID, ref.StoryID, ref.BranchID, ref.TaskID)
	}
	return string(encoded)
}
