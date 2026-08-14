package execution

import (
	"context"
	"denova/config"
	agenttoolruntime "denova/internal/agents/toolruntime"
	"errors"
	"fmt"
)

// ErrUnavailable reports that a Runtime has no live public Agent backend.
var ErrUnavailable = errors.New("Agent execution runtime is unavailable")

// Runtime adapts Denova product cycles to the public Agent -> Session -> Run
// lifecycle. Agent owns in-process ordering plus transcript and capability
// persistence; Denova owns canonical conversations and product effects.
type Runtime struct {
	public *publicBackend
}

// Option configures process-level adapter seams before any Session opens.
type Option func(*runtimeOptions) error

type runtimeOptions struct {
	profiles            []Profile
	toolMutationApplier agenttoolruntime.ToolMutationApplier
	permissionRuleStore PermissionRuleStore
	childDefinitions    ChildDefinitionResolver
}

// PermissionRuleStore is the process-owned durable authorization catalog.
// Loading on evaluation makes settings changes and remembered rules visible
// without changing the immutable Definition behavior identity.
type PermissionRuleStore struct {
	Load    func(context.Context) ([]config.AgentApprovalRule, error)
	Persist func(context.Context, config.AgentApprovalRule) error
}

// WithChildDefinitionResolver installs the process-owned Definition resolver
// for local delegated Agents.
func WithChildDefinitionResolver(resolver ChildDefinitionResolver) Option {
	return func(options *runtimeOptions) error {
		if resolver == nil {
			return fmt.Errorf("delegated Agent Definition resolver is nil")
		}
		options.childDefinitions = resolver
		return nil
	}
}

// WithPermissionRuleStore installs the process-owned user settings mutation
// used by public Permission when the user chooses a remembered workspace rule.
func WithPermissionRuleStore(store PermissionRuleStore) Option {
	return func(options *runtimeOptions) error {
		if store.Load == nil || store.Persist == nil {
			return fmt.Errorf("agent execution Permission rule store requires load and persist")
		}
		options.permissionRuleStore = store
		return nil
	}
}

// WithProfiles installs the complete set of product execution profiles before
// any Session opens. Duplicate and unknown IDs fail
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

// WithToolMutationApplier installs the product-owned idempotent destination for
// completed tool mutations.
func WithToolMutationApplier(applier agenttoolruntime.ToolMutationApplier) Option {
	return func(options *runtimeOptions) error {
		if applier == nil {
			return fmt.Errorf("agent execution Tool mutation applier is nil")
		}
		options.toolMutationApplier = applier
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
