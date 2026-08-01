package harness

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

// ErrUnavailable reports that a Service has no live durable coordinator.
var ErrUnavailable = errors.New("durable agent harness is unavailable")

// Service is the durable command boundary for chat execution. It owns command
// admission, recovery, ordering, and settlement; chat.Executor owns the model
// loop after a command has been admitted.
type Service struct {
	policy      agentrun.LoopPolicy
	coordinator *coordinator
}

// coordinator coordinates the durable command lane with the profile-specific
// turn executor. It owns lifecycle and ordering only; TurnSpec owns the
// typed state for one model cycle.
type coordinator struct {
	lifecycle context.Context
	cancel    context.CancelFunc
	engine    *harnessEngine
	runtime   *runstate.Runtime
}

// Option configures process-level durable adapter seams.
// Options are applied before any binding can open or recovery can begin.
type Option func(*serviceOptions) error

type serviceOptions struct {
	turnRestorer           TurnRestorer
	structuralRestorer     StructuralRestorer
	domainCommitReconciler DomainCommitReconciler
	inputMaterializer      InputMaterializer
	hostEffectReconciler   agenttoolruntime.HarnessHostEffectReconciler
}

// WithHostEffectReconciler installs the process-level durable admission
// seam for completed tool mutations. It is used both live and during cold
// Runtime journal replay, so it must not depend on a per-turn closure.
func WithHostEffectReconciler(reconciler agenttoolruntime.HarnessHostEffectReconciler) Option {
	return func(options *serviceOptions) error {
		if reconciler == nil {
			return fmt.Errorf("agent harness host effect reconciler is nil")
		}
		options.hostEffectReconciler = reconciler
		return nil
	}
}

// WithStructuralRestorer installs the host callback used only when an
// exact recovery-paused structural command is explicitly replayed after a cold
// process restart.
func WithStructuralRestorer(restorer StructuralRestorer) Option {
	return func(options *serviceOptions) error {
		if restorer == nil {
			return fmt.Errorf("agent harness structural restorer is nil")
		}
		options.structuralRestorer = restorer
		return nil
	}
}

// WithInputMaterializer installs the provider-free canonical input
// outbox used after command acceptance and before Engine.Run.
func WithInputMaterializer(materializer InputMaterializer) Option {
	return func(options *serviceOptions) error {
		if materializer == nil {
			return fmt.Errorf("agent harness input materializer is nil")
		}
		options.inputMaterializer = materializer
		return nil
	}
}

// WithDomainCommitReconciler installs the host's read-only canonical
// receipt lookup. It is consulted only while recovering an accepted domain
// commit intent that has no coordinator receipt.
func WithDomainCommitReconciler(reconciler DomainCommitReconciler) Option {
	return func(options *serviceOptions) error {
		if reconciler == nil {
			return fmt.Errorf("agent harness domain commit reconciler is nil")
		}
		options.domainCommitReconciler = reconciler
		return nil
	}
}

// WithTurnRestorer installs the host callback used to rebuild a queued
// NextTurn's process-local execution dependencies after a process restart.
func WithTurnRestorer(restorer TurnRestorer) Option {
	return func(options *serviceOptions) error {
		if restorer == nil {
			return fmt.Errorf("agent harness turn restorer is nil")
		}
		options.turnRestorer = restorer
		return nil
	}
}

// NewDurableService creates the production Service. Journals live
// below dataDir rather than inside one workspace so workspace agents and
// global automations use one lifecycle without sharing binding identity.
func NewDurableService(ctx context.Context, dataDir string, options ...Option) (*Service, error) {
	root := strings.TrimSpace(dataDir)
	if root == "" {
		return nil, fmt.Errorf("agent runtime data directory is required")
	}
	resolved := serviceOptions{}
	for _, apply := range options {
		if apply == nil {
			return nil, fmt.Errorf("durable chat service option is nil")
		}
		if err := apply(&resolved); err != nil {
			return nil, err
		}
	}
	store, err := filejournal.NewStore(filepath.Join(root, "agent-runtime"))
	if err != nil {
		return nil, err
	}
	return newServiceWithOptions(ctx, agentrun.DefaultLoopPolicy(), store, resolved)
}

// NewEphemeralService creates an in-memory service for tests and short-lived
// local execution. Production wiring should use NewDurableService.
func NewEphemeralService() *Service {
	return NewEphemeralServiceWithPolicy(agentrun.DefaultLoopPolicy())
}

// NewEphemeralServiceWithPolicy creates an in-memory service with an explicit
// loop policy.
func NewEphemeralServiceWithPolicy(policy agentrun.LoopPolicy) *Service {
	service, err := newService(context.Background(), policy, runstate.NewMemoryJournalStore())
	if err != nil {
		// All dependencies are process-local invariants; construction failure is
		// a programming error rather than a recoverable runtime condition.
		panic(err)
	}
	return service
}

func newService(
	ctx context.Context,
	policy agentrun.LoopPolicy,
	journals runstate.JournalStore,
	turnRestorers ...TurnRestorer,
) (*Service, error) {
	options := serviceOptions{}
	if len(turnRestorers) > 0 {
		options.turnRestorer = turnRestorers[0]
	}
	return newServiceWithOptions(ctx, policy, journals, options)
}

func newServiceWithOptions(
	ctx context.Context,
	policy agentrun.LoopPolicy,
	journals runstate.JournalStore,
	options serviceOptions,
) (*Service, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	policy = policy.Normalize()
	lifecycle, cancel := context.WithCancel(ctx)
	engine := newHarnessEngine(agentchat.NewExecutor(policy), options.turnRestorer)
	engine.structuralRestorer = options.structuralRestorer
	engine.domainCommitReconciler = options.domainCommitReconciler
	engine.inputMaterializer = options.inputMaterializer
	engine.hostEffectReconciler = options.hostEffectReconciler
	runtime, err := runstate.NewRuntime(engine, journals, runstate.RuntimeConfig{Lifecycle: lifecycle})
	if err != nil {
		cancel()
		return nil, err
	}
	return &Service{
		policy: policy,
		coordinator: &coordinator{
			lifecycle: lifecycle,
			cancel:    cancel,
			engine:    engine,
			runtime:   runtime,
		},
	}, nil
}

// Close stops all durable bindings owned by this service. It has no internal
// timeout; shutdown cancellation remains controlled by the caller.
func (s *Service) Close(ctx context.Context) error {
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
