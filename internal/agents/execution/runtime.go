package execution

import (
	"context"
	agenttoolruntime "denova/internal/agents/toolruntime"
	"errors"
	"fmt"
)

// ErrUnavailable reports that a Runtime has no live public Agent backend.
var ErrUnavailable = errors.New("Agent execution runtime is unavailable")

// Runtime adapts Denova product cycles to the public Agent -> Session -> Run
// lifecycle. The public Agent package is the sole owner of durable admission,
// ordering, recovery, transcript, and compaction state.
type Runtime struct {
	public *publicBackend
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

// Close stops all Agent Sessions owned by this runtime. It has no internal
// timeout; shutdown cancellation remains controlled by the caller.
func (s *Runtime) Close(ctx context.Context) error {
	if s == nil || s.public == nil {
		return nil
	}
	return s.public.close(ctx)
}
