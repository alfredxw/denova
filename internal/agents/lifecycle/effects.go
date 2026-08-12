package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"strings"

	agentrun "denova/internal/agents/run"
	agenttool "denova/internal/agents/tool"
	agenttoolruntime "denova/internal/agents/toolruntime"

	agent "github.com/alfredxw/denova/agent"
)

// ToolEffectApplier transfers committed Denova Tool mutations from Agent's
// durable outbox into the product-level host reconciler.
type ToolEffectApplier func(context.Context, []agent.EffectRequest) ([]agent.EffectResult, error)

// ToolEffectObserver receives only effects that the product reconciler has
// durably admitted. It is process-local accounting and never effect authority.
type ToolEffectObserver func(agent.EffectRequest, agenttool.Mutation)

// NewToolEffectApplier adapts Denova's existing at-least-once mutation host to
// the public Agent effect outbox. One invalid item does not discard successful
// siblings; every request receives an explicit result.
func NewToolEffectApplier(
	reconciler agenttoolruntime.HostEffectReconciler,
	options agentrun.Options,
	observe ToolEffectObserver,
) (ToolEffectApplier, error) {
	if reconciler == nil {
		return nil, errors.New("Denova Tool effect reconciler is required")
	}
	options = options.Normalize(options.Workspace)
	binding, err := agentrun.RuntimeBindingForOptions(options)
	if err != nil {
		return nil, fmt.Errorf("resolve Denova Tool effect binding: %w", err)
	}
	return func(ctx context.Context, requests []agent.EffectRequest) ([]agent.EffectResult, error) {
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
			err = reconciler(ctx, agenttoolruntime.CommittedToolMutation{
				EffectID: agentrun.HostEffectID(request.ID), Binding: binding,
				RuntimeOperation: agentrun.OperationID(request.Identity.RunID), RuntimeCycle: request.Identity.Cycle,
				ToolCallID: toolCallID, Origin: origin, Mutation: mutation,
			})
			if err != nil {
				result.Error = fmt.Sprintf("reconcile Denova Tool mutation: %v", err)
			} else {
				result.Revision = request.ID
				if observe != nil {
					observe(request, mutation)
				}
			}
			results[index] = result
		}
		return results, nil
	}, nil
}
