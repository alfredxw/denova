package lifecycle

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	agentrun "denova/internal/agents/run"
	agenttool "denova/internal/agents/tool"
	agenttoolruntime "denova/internal/agents/toolruntime"

	agent "github.com/alfredxw/denova/agent"
)

// ToolEffectObserver receives only effects accepted by the product. It is
// process-local accounting and never effect authority.
type ToolEffectObserver func(agent.EffectRequest, agenttool.Mutation)

// NewToolEffectApplier adapts Denova's product mutation host to Agent's direct
// canonical effect API. One invalid item does not discard successful siblings;
// every request receives an explicit result.
func NewToolEffectApplier(
	applier agenttoolruntime.ToolMutationApplier,
	options agentrun.Options,
	observe ToolEffectObserver,
) (agent.EffectApplier, error) {
	if applier == nil {
		return nil, errors.New("Denova Tool mutation applier is required")
	}
	options = options.Normalize(options.Workspace)
	binding, err := agentrun.RuntimeBindingForOptions(options)
	if err != nil {
		return nil, fmt.Errorf("resolve Denova Tool effect binding: %w", err)
	}
	key, err := binding.AgentSessionKey()
	if err != nil {
		return nil, err
	}
	// Product identity is stable when DataRoot moves; runtime paths and observer
	// callbacks must not enter the effect capability's behavior identity.
	identity := agent.CapabilityIdentity{Kind: "denova.tool_effects", Version: 1,
		ConfigHash: fmt.Sprintf("%x", sha256.Sum256([]byte(key.Namespace+"\x00"+key.ID))),
	}
	return agent.EffectApplierFuncs{CapabilityIdentity: identity, ApplyEffectsFn: func(ctx context.Context, requests []agent.EffectRequest) ([]agent.EffectResult, error) {
		results := make([]agent.EffectResult, len(requests))
		origin := agenttoolruntime.ToolMutationOrigin{
			AgentKind: options.AgentKind, ProjectID: options.ProjectID,
			TaskID: options.TaskID, AutomationTaskID: options.AutomationTaskID,
			SessionID: options.SessionID, ReviewThreadID: options.ReviewThreadID,
			StoryID: options.StoryID, BranchID: options.BranchID, TurnID: options.TurnID,
			MaintenanceTask: options.MaintenanceTask, Workspace: options.Workspace, Mode: options.Mode,
		}
		for index, request := range requests {
			result := agent.EffectResult{ID: request.ID}
			if err := ctx.Err(); err != nil {
				result.Error = err.Error()
				results[index] = result
				continue
			}
			mutation, err := agenttoolruntime.DecodeAgentToolMutationEffect(request.Effect)
			if err != nil {
				result.Error = err.Error()
				results[index] = result
				continue
			}
			if strings.TrimSpace(mutation.Workspace) == "" {
				mutation.Workspace = options.Workspace
			}
			toolCallID := strings.TrimSpace(mutation.ToolCallID)
			if toolCallID == "" {
				toolCallID = strings.TrimSpace(request.CallID)
			}
			err = applier(ctx, agenttoolruntime.CommittedToolMutation{
				EffectID: agentrun.HostEffectID(request.ID), Binding: binding,
				RuntimeOperation: agentrun.OperationID(request.Identity.RunID), RuntimeCycle: request.Identity.Cycle,
				ToolCallID: toolCallID, Origin: origin, Mutation: mutation,
			})
			if err != nil {
				result.Error = fmt.Sprintf("apply Denova Tool mutation: %v", err)
				slog.ErrorContext(ctx, "failed to admit committed Agent Tool mutation",
					"project_id", options.ProjectID, "run_id", request.Identity.RunID,
					"tool_call_id", toolCallID, "effect_id", request.ID, "target", mutation.Target, "error", err)
			} else {
				result.Revision = request.ID
				if observe != nil {
					observe(request, mutation)
				}
			}
			results[index] = result
		}
		return results, nil
	}}, nil
}
