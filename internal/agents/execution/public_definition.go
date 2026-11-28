package execution

import (
	"context"
	"fmt"

	agentchat "denova/internal/agents/chat"
	agentconversation "denova/internal/agents/conversation"
	agentdelegation "denova/internal/agents/delegation"
	agentlifecycle "denova/internal/agents/lifecycle"

	"github.com/alfredxw/denova/agent"
	agentcontext "github.com/alfredxw/denova/agent/context"
	agentexecution "github.com/alfredxw/denova/agent/engine/execution"
	agentmiddleware "github.com/alfredxw/denova/agent/engine/middleware"
	agentschema "github.com/alfredxw/denova/agent/schema"
	agentsession "github.com/alfredxw/denova/agent/session"
	agentcanonical "github.com/alfredxw/denova/agent/session/canonical"
	agenttool "github.com/alfredxw/denova/agent/tool"
	publictools "github.com/alfredxw/denova/agent/tool/builtin"
)

func (backend *publicBackend) bindDefinition(
	ctx context.Context,
	request agent.PrepareRequest,
	cycle Cycle,
	registration *publicCycleRegistration,
) (agent.Definition, error) {
	options := cycle.Options.Normalize(cycle.Options.Workspace)
	registration.mu.Lock()
	if registration.projector == nil || !registration.projectorBound {
		registration.projector = agentchat.NewPublicEventProjector(cycle.Conversation, cycle.Request, options, registration.emit)
		registration.projectorBound = true
	}
	projector := registration.projector
	pendingRunStart := registration.pendingRunStart
	commandKind := registration.commandKind
	registration.pendingRunStart = nil
	registration.request, registration.options = cycle.Request, options
	registration.mu.Unlock()
	if pendingRunStart != nil {
		projector.ProjectRunStarted(
			pendingRunStart.runID,
			pendingRunStart.started.Cycle,
			firstPublicCycleValue(pendingRunStart.started.CommandID, cycle.Request.CommandID),
			firstPublicCycleValue(pendingRunStart.started.Delivery, string(commandKind)),
			pendingRunStart.started.StartedAt,
		)
	}
	effectApplier, err := agentlifecycle.NewToolEffectApplier(backend.effects, options, registration.recordMutation)
	if err != nil {
		return agent.Definition{}, err
	}
	var committer agentlifecycle.ConversationCommitter
	switch conversation := cycle.Conversation.(type) {
	case *agentconversation.SessionConversation:
		inputEffect := projectInputCommitEffect(options.InputCommitEffect, projector, options)
		committer, err = agentlifecycle.NewSessionConversationCommitter(agentlifecycle.SessionCommitterConfig{
			Conversation: conversation, Session: conversation.CanonicalSession(), Options: options,
			Request:     cycle.Request,
			InputEffect: inputEffect,
		})
	case agentlifecycle.ConversationCommitterProvider:
		committer, err = conversation.NewAgentConversationCommitter(options)
	default:
		err = fmt.Errorf("Denova conversation %T has no public canonical committer", cycle.Conversation)
	}
	if err != nil {
		return agent.Definition{}, err
	}
	identityConfig := struct {
		Definition string
		Binding    agentsession.Key
	}{cycle.Definition.Key, request.Session.Key}
	boundary, err := agentlifecycle.NewConversationBoundary(agentlifecycle.ConversationBoundaryConfig{
		Conversation: cycle.Conversation, BookService: cycle.BookService,
		Request: cycle.Request, Options: options, Committer: committer,
		ContextIdentity:   publicCapabilityIdentity("denova.context", identityConfig),
		CanonicalIdentity: denovaCanonicalIdentity(request.Session.Key),
		OnPrepared: func(prepared agentchat.AgentContextPreparation) {
			projector.ProjectPreparedContext(request.Run, prepared)
		},
		BeforeCommitOutput: func(commitCtx context.Context) error {
			return backend.waitRunProjection(commitCtx, request.Run.ID)
		},
		ProjectOutput: func(output agentcanonical.OutputCommitRequest) (*agentschema.Message, *agentcanonical.OutputProjection) {
			return agentchat.ProjectCanonicalOutput(cycle.Request, options, output)
		},
	})
	if err != nil {
		return agent.Definition{}, err
	}
	definition := cycle.Definition
	definition.Effects = effectApplier
	definition.AttachmentRoot = options.StateRoot
	definition.Execution.IdleTimeout = options.IdleTimeout
	if taskCatalog, ok := agentdelegation.AsCatalog(definition.Tools); ok {
		parentAttributes, attributeErr := agentdelegation.ParentAttributes(request.Session.Key)
		if attributeErr != nil {
			return agent.Definition{}, attributeErr
		}
		route := request.HostData
		if route == nil {
			route = request.Input.HostData
		}
		var childHostData *agent.HostData
		// Structural compaction prepares tool schemas for the exact model request
		// but cannot execute a tool or start a child. It has no accepted turn route.
		if request.Reason != agent.TurnReasonStructural {
			childHostData, attributeErr = agentdelegation.BindRun(request.Run, route)
			if attributeErr != nil {
				return agent.Definition{}, attributeErr
			}
		}
		children := taskCatalog.Children()
		candidates := make([]publictools.LocalTaskAgent, len(children))
		for index, child := range children {
			candidates[index] = publictools.LocalTaskAgent{
				Name: child.Name, Description: child.Description,
				Opener: backend.agent, Identity: child.Identity,
				Attributes: parentAttributes, LookupAttributes: parentAttributes, HostData: childHostData,
			}
		}
		parentSession, taskErr := backend.agent.Session(ctx, request.Session.Key)
		if taskErr != nil {
			return agent.Definition{}, fmt.Errorf("open parent Agent Session for delegated completion delivery: %w", taskErr)
		}
		executor, taskErr := publictools.NewLocalTasks(publictools.LocalTaskOptions{
			Parallelism: taskCatalog.Parallelism(), CompletionParent: parentSession,
			MaxResultBytes: taskCatalog.MaxResultBytes(),
			Self:           publictools.TaskRef{Agent: definition.Name, Session: request.Session.Key.ID},
		}, candidates...)
		if taskErr != nil {
			return agent.Definition{}, fmt.Errorf("bind delegated Agent executor: %w", taskErr)
		}
		if request.Reason != agent.TurnReasonStructural {
			if taskErr = executor.ReconcileTaskCompletions(ctx); taskErr != nil {
				return agent.Definition{}, fmt.Errorf("reconcile delegated Agent completions: %w", taskErr)
			}
		}
		definition.Tools, taskErr = taskCatalog.Bind(executor)
		if taskErr != nil {
			return agent.Definition{}, fmt.Errorf("bind delegated Agent Toolset: %w", taskErr)
		}
	}
	if provider, ok := cycle.Conversation.(agentchat.ToolArtifactStoreProvider); ok {
		store := provider.ToolArtifactStore()
		if store != nil {
			definition.Artifacts, err = agenttool.IdentifyToolArtifactStorage(
				store, publicCapabilityIdentity("denova.tool_artifacts", identityConfig),
			)
			if err != nil {
				return agent.Definition{}, err
			}
		}
	}
	definition.Context, err = agentcontext.CombineContextSources(definition.Context, boundary.ContextSource())
	if err != nil {
		return agent.Definition{}, fmt.Errorf("compose project and conversation ContextSources: %w", err)
	}
	definition.Canonical = boundary.CanonicalAdapter()
	definition.Permission = agentlifecycle.BindPermissionRuleStore(
		definition.Permission, backend.permissionRuleStore.Load, backend.permissionRuleStore.Persist,
	)
	var trace agentchat.PublicRunTraceBinder = registration
	if agentexecution.IsInspection(ctx) {
		trace = nil
	}
	host := agentchat.NewPublicHostMiddleware(cycle.Request, options, trace)
	definition.Middlewares = append(definition.Middlewares, agentmiddleware.IdentifyMiddleware(
		host, publicCapabilityIdentity("denova.public_host", identityConfig),
	))
	return definition, nil
}
