package agents

import (
	"context"
	"strings"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	producttools "denova/internal/agents/tools"
	"denova/internal/workspacechange"
)

const submitDirectorPlanUpdateToolName = producttools.SubmitDirectorPlanUpdateToolName

type submitDirectorPlanUpdateInput = producttools.SubmitDirectorPlanUpdateInput

// newToolCatalog is the only bridge from Agent orchestration into concrete
// tool construction. Runtime metadata is projected into a narrow callback so
// the tools package never imports the agents package.
func newToolCatalog(cfg *config.Config) *producttools.Catalog {
	return producttools.NewCatalog(cfg, agentWorkspaceChangeMetadata)
}

func projectInteractiveToolContext(contexts ...InteractiveStoryToolContext) producttools.InteractiveContext {
	if len(contexts) == 0 {
		return producttools.InteractiveContext{}
	}
	source := contexts[0]
	return producttools.InteractiveContext{
		Store:                     source.Store,
		StoryID:                   source.StoryID,
		BranchID:                  source.BranchID,
		MaintenanceTask:           source.MaintenanceTask,
		OnLoreItemsRead:           source.OnLoreItemsRead,
		SubmitStateSchemaBatch:    source.SubmitStateSchemaBatch,
		SubmitDirectorPlanUpdate:  source.SubmitDirectorPlanUpdate,
		RequestDirectorCompletion: requestInteractiveDirectorPlanCompletion,
		RequestTurnCompletion:     requestInteractiveTurnCompletion,
		PrepareTurn:               source.PrepareTurn,
		SubmitTurnResult:          source.SubmitTurnResult,
	}
}

func agentWorkspaceChangeMetadata(ctx context.Context) workspacechange.ChangeMetadata {
	callID := strings.TrimSpace(agent.ToolCallID(ctx))
	runID := ""
	sessionID := ""
	reviewThreadID := ""
	if observer := RunObserverFromContext(ctx); observer != nil {
		runID = strings.TrimSpace(observer.RunID())
		sessionID = strings.TrimSpace(observer.SessionID())
		reviewThreadID = strings.TrimSpace(observer.ReviewThreadID())
	}
	groupID := runID
	if groupID == "" {
		groupID = callID
	}
	return workspacechange.ChangeMetadata{
		Origin:         workspacechange.OriginAgent,
		ChangeGroupID:  groupID,
		RunID:          runID,
		SessionID:      sessionID,
		ReviewThreadID: reviewThreadID,
		ToolCallID:     callID,
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
