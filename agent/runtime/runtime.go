package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
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
	access     map[string]uint64
	sequence   uint64
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
		access:     make(map[string]uint64),
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
	if config.MaxOpenBindings <= 0 {
		config.MaxOpenBindings = 32
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
func (r *Runtime) Open(ctx context.Context, binding BindingRef) (*Harness, error) {
	if r == nil {
		return nil, ErrInvalidBinding
	}
	ref := binding.Clone()
	if err := ValidateBindingRef(ref); err != nil {
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
				r.touchBindingLocked(key)
				trim := len(r.harness) > r.config.MaxOpenBindings
				r.mu.Unlock()
				if trim {
					r.trimIdleBindings(key)
				}
				return h, nil
			}
			// A terminal actor has already released, or is about to release, its
			// lease. Never hand stale in-memory state back to another command.
			delete(r.harness, key)
			delete(r.access, key)
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

// BindingReference validates and defensively clones an application-owned
// identity for adapters that need to retain it outside Runtime.Open.
func BindingReference(binding BindingRef) (BindingRef, error) {
	ref := binding.Clone()
	if err := ValidateBindingRef(ref); err != nil {
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
		r.touchBindingLocked(key)
		pending.harness = h
	}
	pending.err = err
	close(pending.ready)
	completed = true
	trim := err == nil && len(r.harness) > r.config.MaxOpenBindings
	r.mu.Unlock()
	if trim {
		r.trimIdleBindings(key)
	}
}

func (r *Runtime) touchBindingLocked(key string) {
	r.sequence++
	r.access[key] = r.sequence
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
	engine, err := r.engines.NewEngine(r.ctx, ref.Clone())
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

const (
	maxBindingKindBytes       = 128
	maxBindingProfileBytes    = 256
	maxBindingKeyBytes        = 4 << 10
	maxBindingLabels          = 64
	maxBindingLabelNameBytes  = 128
	maxBindingLabelValueBytes = 4 << 10
	maxBindingLabelsBytes     = 64 << 10
)

// ValidateBindingRef verifies the complete, application-owned durable
// identity without imposing a product taxonomy.
func ValidateBindingRef(ref BindingRef) error {
	invalid := func(reason string) error {
		return fmt.Errorf("%w: %s", ErrInvalidBinding, reason)
	}
	if strings.TrimSpace(ref.Kind) == "" || ref.Kind != strings.TrimSpace(ref.Kind) {
		return invalid("kind is required and cannot contain surrounding whitespace")
	}
	if len(ref.Kind) > maxBindingKindBytes {
		return invalid(fmt.Sprintf("kind exceeds %d bytes", maxBindingKindBytes))
	}
	if ref.Profile != strings.TrimSpace(ref.Profile) {
		return invalid("profile cannot contain surrounding whitespace")
	}
	if len(ref.Profile) > maxBindingProfileBytes {
		return invalid(fmt.Sprintf("profile exceeds %d bytes", maxBindingProfileBytes))
	}
	if strings.TrimSpace(ref.Key) == "" || ref.Key != strings.TrimSpace(ref.Key) {
		return invalid("key is required and cannot contain surrounding whitespace")
	}
	if len(ref.Key) > maxBindingKeyBytes {
		return invalid(fmt.Sprintf("key exceeds %d bytes", maxBindingKeyBytes))
	}
	if len(ref.Labels) > maxBindingLabels {
		return invalid(fmt.Sprintf("label count exceeds %d", maxBindingLabels))
	}
	total := 0
	for name, value := range ref.Labels {
		if strings.TrimSpace(name) == "" || name != strings.TrimSpace(name) {
			return invalid("label names are required and cannot contain surrounding whitespace")
		}
		if strings.TrimSpace(value) == "" || value != strings.TrimSpace(value) {
			return invalid(fmt.Sprintf("label %q value is required and cannot contain surrounding whitespace", name))
		}
		if len(name) > maxBindingLabelNameBytes {
			return invalid(fmt.Sprintf("label name exceeds %d bytes", maxBindingLabelNameBytes))
		}
		if len(value) > maxBindingLabelValueBytes {
			return invalid(fmt.Sprintf("label %q value exceeds %d bytes", name, maxBindingLabelValueBytes))
		}
		total += len(name) + len(value)
		if total > maxBindingLabelsBytes {
			return invalid(fmt.Sprintf("labels exceed %d bytes", maxBindingLabelsBytes))
		}
	}
	return nil
}

func validateBindingSelector(selector BindingSelector) error {
	if selector.Kind == "" && selector.Profile == "" && selector.Key == "" && len(selector.Labels) == 0 {
		return fmt.Errorf("%w: binding selector must constrain at least one field", ErrInvalidBinding)
	}
	probe := BindingRef{Kind: selector.Kind, Profile: selector.Profile, Key: selector.Key, Labels: selector.Labels}
	if probe.Kind == "" {
		probe.Kind = "selector"
	}
	if probe.Key == "" {
		probe.Key = "selector"
	}
	return ValidateBindingRef(probe)
}

func bindingJournalKey(ref BindingRef) string {
	encoded, err := json.Marshal(ref)
	if err != nil {
		keys := make([]string, 0, len(ref.Labels))
		for key := range ref.Labels {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		fallback := ref.Kind + "|" + ref.Profile + "|" + ref.Key
		for _, key := range keys {
			fallback += "|" + key + "=" + ref.Labels[key]
		}
		return fallback
	}
	return string(encoded)
}
