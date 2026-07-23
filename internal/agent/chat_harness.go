package agent

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"denova/internal/agentruntime"
)

// chatHarness coordinates the durable command lane with the profile-specific
// turn executor. It owns lifecycle and ordering only; HarnessTurnSpec owns the
// typed state for one model cycle.
type chatHarness struct {
	lifecycle context.Context
	cancel    context.CancelFunc
	engine    *harnessEngine
	runtime   *agentruntime.Runtime
}

// DurableChatServiceOption configures process-level durable adapter seams.
// Options are applied before any binding can open or recovery can begin.
type DurableChatServiceOption func(*durableChatServiceOptions) error

type durableChatServiceOptions struct {
	turnRestorer           HarnessTurnRestorer
	structuralRestorer     HarnessStructuralRestorer
	domainCommitReconciler HarnessDomainCommitReconciler
	inputMaterializer      HarnessInputMaterializer
	hostEffectReconciler   HarnessHostEffectReconciler
}

// WithHarnessHostEffectReconciler installs the process-level durable admission
// seam for completed tool mutations. It is used both live and during cold
// Runtime journal replay, so it must not depend on a per-turn closure.
func WithHarnessHostEffectReconciler(reconciler HarnessHostEffectReconciler) DurableChatServiceOption {
	return func(options *durableChatServiceOptions) error {
		if reconciler == nil {
			return fmt.Errorf("agent harness host effect reconciler is nil")
		}
		options.hostEffectReconciler = reconciler
		return nil
	}
}

// WithHarnessStructuralRestorer installs the host callback used only when an
// exact recovery-paused structural command is explicitly replayed after a cold
// process restart.
func WithHarnessStructuralRestorer(restorer HarnessStructuralRestorer) DurableChatServiceOption {
	return func(options *durableChatServiceOptions) error {
		if restorer == nil {
			return fmt.Errorf("agent harness structural restorer is nil")
		}
		options.structuralRestorer = restorer
		return nil
	}
}

// WithHarnessInputMaterializer installs the provider-free canonical input
// outbox used after command acceptance and before Engine.Run.
func WithHarnessInputMaterializer(materializer HarnessInputMaterializer) DurableChatServiceOption {
	return func(options *durableChatServiceOptions) error {
		if materializer == nil {
			return fmt.Errorf("agent harness input materializer is nil")
		}
		options.inputMaterializer = materializer
		return nil
	}
}

// WithHarnessDomainCommitReconciler installs the host's read-only canonical
// receipt lookup. It is consulted only while recovering an accepted domain
// commit intent that has no coordinator receipt.
func WithHarnessDomainCommitReconciler(reconciler HarnessDomainCommitReconciler) DurableChatServiceOption {
	return func(options *durableChatServiceOptions) error {
		if reconciler == nil {
			return fmt.Errorf("agent harness domain commit reconciler is nil")
		}
		options.domainCommitReconciler = reconciler
		return nil
	}
}

// WithHarnessTurnRestorer installs the host callback used to rebuild a queued
// NextTurn's process-local execution dependencies after a process restart.
func WithHarnessTurnRestorer(restorer HarnessTurnRestorer) DurableChatServiceOption {
	return func(options *durableChatServiceOptions) error {
		if restorer == nil {
			return fmt.Errorf("agent harness turn restorer is nil")
		}
		options.turnRestorer = restorer
		return nil
	}
}

// NewDurableChatService creates the production ChatService. Journals live
// below dataDir rather than inside one workspace so workspace agents and
// global automations use one lifecycle without sharing binding identity.
func NewDurableChatService(ctx context.Context, dataDir string, serviceOptions ...DurableChatServiceOption) (*ChatService, error) {
	root := strings.TrimSpace(dataDir)
	if root == "" {
		return nil, fmt.Errorf("agent runtime data directory is required")
	}
	options := durableChatServiceOptions{}
	for _, apply := range serviceOptions {
		if apply == nil {
			return nil, fmt.Errorf("durable chat service option is nil")
		}
		if err := apply(&options); err != nil {
			return nil, err
		}
	}
	store, err := agentruntime.NewFileJournalStore(filepath.Join(root, "agent-runtime"))
	if err != nil {
		return nil, err
	}
	return newHarnessChatServiceWithOptions(ctx, DefaultLoopPolicy(), store, options)
}

func newHarnessChatService(
	ctx context.Context,
	policy LoopPolicy,
	journals agentruntime.JournalStore,
	turnRestorers ...HarnessTurnRestorer,
) (*ChatService, error) {
	options := durableChatServiceOptions{}
	if len(turnRestorers) > 0 {
		options.turnRestorer = turnRestorers[0]
	}
	return newHarnessChatServiceWithOptions(ctx, policy, journals, options)
}

func newHarnessChatServiceWithOptions(
	ctx context.Context,
	policy LoopPolicy,
	journals agentruntime.JournalStore,
	options durableChatServiceOptions,
) (*ChatService, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	policy = policy.normalized()
	lifecycle, cancel := context.WithCancel(ctx)
	engine := newHarnessEngine(newTurnExecutor(policy), options.turnRestorer)
	engine.structuralRestorer = options.structuralRestorer
	engine.domainCommitReconciler = options.domainCommitReconciler
	engine.inputMaterializer = options.inputMaterializer
	engine.hostEffectReconciler = options.hostEffectReconciler
	runtime, err := agentruntime.NewRuntime(engine, journals, agentruntime.RuntimeConfig{Lifecycle: lifecycle})
	if err != nil {
		cancel()
		return nil, err
	}
	return &ChatService{
		policy: policy,
		harness: &chatHarness{
			lifecycle: lifecycle,
			cancel:    cancel,
			engine:    engine,
			runtime:   runtime,
		},
	}, nil
}

// Close stops all durable bindings owned by this service. It has no internal
// timeout; shutdown cancellation remains controlled by the caller.
func (s *ChatService) Close(ctx context.Context) error {
	if s == nil || s.harness == nil {
		return nil
	}
	return s.harness.close(ctx)
}

func (h *chatHarness) close(ctx context.Context) error {
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
