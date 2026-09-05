package execution

import (
	"context"
	"errors"
	"fmt"
	"strings"

	agentdelegation "denova/internal/agents/delegation"
	agentlifecycle "denova/internal/agents/lifecycle"
	agentrun "denova/internal/agents/run"
	agenttoolartifact "denova/internal/agents/toolartifact"

	agent "github.com/alfredxw/denova/agent"
	agentsession "github.com/alfredxw/denova/agent/session"
)

// ChildDefinitionRequest rebuilds one named child from the exact durable
// parent Session. HostData is the immutable accepted parent-turn descriptor;
// delegated Sessions still own isolated transcripts and canonical state.
type ChildDefinitionRequest struct {
	Parent   agent.SessionKey
	Child    string
	HostData *agent.HostData
}

// ChildDefinition carries executable child composition and the current runtime
// workspace resolved from its stable Project identity. Workspace is never persisted.
type ChildDefinition struct {
	Definition agent.Definition
	Workspace  string
}

// ChildDefinitionResolver rebuilds composition from stable product identity
// and current configuration, so cold task recovery never depends on an
// executor-local task map.
type ChildDefinitionResolver interface {
	PrepareChildDefinition(context.Context, ChildDefinitionRequest) (ChildDefinition, error)
}

type ChildDefinitionResolverFunc func(context.Context, ChildDefinitionRequest) (ChildDefinition, error)

func (resolve ChildDefinitionResolverFunc) PrepareChildDefinition(
	ctx context.Context,
	request ChildDefinitionRequest,
) (ChildDefinition, error) {
	if resolve == nil || strings.TrimSpace(request.Child) == "" {
		return ChildDefinition{}, errors.New("delegated Agent Definition resolver is unavailable")
	}
	return resolve(ctx, request)
}

func (backend *publicBackend) resolveTaskDefinition(
	ctx context.Context,
	request agent.PrepareRequest,
) (agent.Definition, error) {
	parent, err := agentdelegation.ParentSession(request.Session.Key)
	if err != nil {
		return agent.Definition{}, err
	}
	childName, err := agentdelegation.ChildName(request.Session.Key)
	if err != nil {
		return agent.Definition{}, err
	}
	binding, err := agentrun.RuntimeBindingFromAgentSessionKey(parent)
	if err != nil {
		return agent.Definition{}, fmt.Errorf("resolve delegated parent binding: %w", err)
	}
	profileID, err := binding.ProfileID()
	if err != nil {
		return agent.Definition{}, err
	}
	if backend.childDefinitions == nil {
		return agent.Definition{}, fmt.Errorf("%w: profile %q cannot rebuild delegated Agents", ErrCyclePreparationUnavailable, profileID)
	}
	route, err := agentdelegation.ParentRoute(request.Session.Key)
	if err != nil {
		return agent.Definition{}, err
	}
	child, err := backend.childDefinitions.PrepareChildDefinition(ctx, ChildDefinitionRequest{
		Parent: parent, Child: childName, HostData: route,
	})
	if err != nil {
		return agent.Definition{}, err
	}
	definition := child.Definition
	if definition.Model == nil || definition.Name != childName {
		return agent.Definition{}, errors.New("delegated Agent Definition does not match its durable selector")
	}
	data, err := agentlifecycle.DecodeTurnHostData(agent.Input{HostData: route})
	if err != nil {
		return agent.Definition{}, err
	}
	options := publicOptions(binding, data)
	registration := backend.registration(parent, data.Caller.CommandID)
	if registration != nil {
		registration.mu.RLock()
		options = registration.options
		registration.mu.RUnlock()
	}
	options.Workspace = child.Workspace
	// Effects belong to the parent product scope, while the child Session keeps
	// its own input, tool protocol and output in its standalone journal. Never
	// bind the parent's conversation Canonical Adapter to this Definition.
	definition.Effects, err = agentlifecycle.NewToolEffectApplier(backend.effects, options, registration.recordMutation)
	if err != nil {
		return agent.Definition{}, fmt.Errorf("bind delegated Agent Tool effects: %w", err)
	}
	definition.Permission = agentlifecycle.BindPermissionRuleStore(
		definition.Permission, backend.permissionRuleStore.Load, backend.permissionRuleStore.Persist,
	)
	if strings.TrimSpace(definition.AttachmentRoot) != "" {
		canonical, _ := agentsession.CanonicalKey(request.Session.Key)
		store, storeErr := agenttoolartifact.NewStateStore(definition.AttachmentRoot, canonical)
		if storeErr != nil {
			return agent.Definition{}, fmt.Errorf("create delegated Agent artifact Store: %w", storeErr)
		}
		definition.Artifacts, err = agent.IdentifyToolArtifactStorage(
			store, publicCapabilityIdentity("denova.task.tool_artifacts", request.Session.Key),
		)
		if err != nil {
			return agent.Definition{}, err
		}
	}
	return definition, nil
}
