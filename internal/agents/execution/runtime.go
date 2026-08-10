package execution

import (
	"context"
	agentchat "denova/internal/agents/chat"
	agentrun "denova/internal/agents/run"
	agenttoolruntime "denova/internal/agents/toolruntime"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	filejournal "github.com/alfredxw/denova/agent/runtime/filejournal"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

// ErrUnavailable reports that a Runtime has no live durable coordinator.
var ErrUnavailable = errors.New("durable agent execution runtime is unavailable")

// Runtime is the process-level Agent execution owner. It owns durable command
// admission, recovery, ordering, and settlement; chat.Executor owns the model
// loop after a command has been admitted.
type Runtime struct {
	policy      agentrun.LoopPolicy
	coordinator *coordinator
}

// coordinator coordinates the durable command lane with the profile-specific
// turn executor. It owns lifecycle and ordering only; cycleSpec owns the
// typed state for one model cycle.
type coordinator struct {
	lifecycle context.Context
	cancel    context.CancelFunc
	engine    *durableEngine
	runtime   *runstate.Runtime
}

// Option configures process-level durable adapter seams.
// Options are applied before any binding can open or recovery can begin.
type Option func(*runtimeOptions) error

type runtimeOptions struct {
	profiles             []Profile
	hostEffectReconciler agenttoolruntime.HostEffectReconciler
}

// WithProfiles installs the complete set of product execution profiles before
// any durable binding can open or recover. Duplicate and unknown IDs fail
// construction so runtime behavior cannot depend on registration order.
func WithProfiles(profiles ...Profile) Option {
	return func(options *runtimeOptions) error {
		registry, err := newProfileRegistry(profiles)
		if err != nil {
			return err
		}
		options.profiles = make([]Profile, 0, len(registry.profiles))
		for _, profile := range profiles {
			options.profiles = append(options.profiles, profile)
		}
		return nil
	}
}

// WithHostEffectReconciler installs the process-level durable admission
// seam for completed tool mutations. It is used both live and during cold
// Runtime journal replay, so it must not depend on a per-turn closure.
func WithHostEffectReconciler(reconciler agenttoolruntime.HostEffectReconciler) Option {
	return func(options *runtimeOptions) error {
		if reconciler == nil {
			return fmt.Errorf("agent execution host effect reconciler is nil")
		}
		options.hostEffectReconciler = reconciler
		return nil
	}
}

// NewDurableRuntime creates the production Runtime. Journals live
// below dataDir rather than inside one workspace so workspace agents and
// global automations use one lifecycle without sharing binding identity.
func NewDurableRuntime(ctx context.Context, dataDir string, options ...Option) (*Runtime, error) {
	root := strings.TrimSpace(dataDir)
	if root == "" {
		return nil, fmt.Errorf("agent runtime data directory is required")
	}
	resolved := runtimeOptions{}
	for _, apply := range options {
		if apply == nil {
			return nil, fmt.Errorf("durable execution runtime option is nil")
		}
		if err := apply(&resolved); err != nil {
			return nil, err
		}
	}
	store, err := filejournal.NewStore(filepath.Join(root, "agent-runtime"))
	if err != nil {
		return nil, err
	}
	return newRuntimeWithOptions(ctx, agentrun.DefaultLoopPolicy(), store, resolved)
}

// NewEphemeralRuntime creates an in-memory runtime for tests and short-lived
// local execution. Production wiring should use NewDurableRuntime.
func NewEphemeralRuntime() *Runtime {
	return NewEphemeralRuntimeWithPolicy(agentrun.DefaultLoopPolicy())
}

// NewEphemeralRuntimeWithPolicy creates an in-memory runtime with an explicit
// loop policy.
func NewEphemeralRuntimeWithPolicy(policy agentrun.LoopPolicy) *Runtime {
	runtime, err := newRuntime(context.Background(), policy, runstate.NewMemoryJournalStore())
	if err != nil {
		// All dependencies are process-local invariants; construction failure is
		// a programming error rather than a recoverable runtime condition.
		panic(err)
	}
	return runtime
}

func newRuntime(
	ctx context.Context,
	policy agentrun.LoopPolicy,
	journals runstate.JournalStore,
) (*Runtime, error) {
	return newRuntimeWithOptions(ctx, policy, journals, runtimeOptions{})
}

func newRuntimeWithOptions(
	ctx context.Context,
	policy agentrun.LoopPolicy,
	journals runstate.JournalStore,
	options runtimeOptions,
) (*Runtime, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	policy = policy.Normalize()
	lifecycle, cancel := context.WithCancel(ctx)
	profiles, err := newProfileRegistry(options.profiles)
	if err != nil {
		cancel()
		return nil, err
	}
	engine := newDurableEngine(agentchat.NewExecutor(policy), profiles)
	engine.hostEffectReconciler = options.hostEffectReconciler
	runtime, err := runstate.NewRuntime(engine, journals, runstate.RuntimeConfig{Lifecycle: lifecycle})
	if err != nil {
		cancel()
		return nil, err
	}
	return &Runtime{
		policy: policy,
		coordinator: &coordinator{
			lifecycle: lifecycle,
			cancel:    cancel,
			engine:    engine,
			runtime:   runtime,
		},
	}, nil
}

// Close stops all durable bindings owned by this runtime. It has no internal
// timeout; shutdown cancellation remains controlled by the caller.
func (s *Runtime) Close(ctx context.Context) error {
	if s == nil || s.coordinator == nil {
		return nil
	}
	return s.coordinator.close(ctx)
}

func (h *coordinator) close(ctx context.Context) error {
	if h == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if h.runtime == nil {
		if h.cancel != nil {
			h.cancel()
		}
		if h.engine != nil {
			h.engine.clear()
		}
		return nil
	}
	err := h.runtime.Close(ctx)
	// Runtime.Close first installs each actor's closing fence and only then
	// cancels engine lifecycle contexts. Do not cancel the parent lifecycle when
	// this caller merely stopped waiting: the owner close remains in flight.
	if err == nil {
		if h.cancel != nil {
			h.cancel()
		}
		if h.engine != nil {
			h.engine.clear()
		}
	}
	return err
}
