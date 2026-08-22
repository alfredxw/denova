package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	agent "github.com/alfredxw/denova/agent"

	"denova/internal/interactive"
	"denova/internal/interactive/director"
)

const submitDirectorPlanUpdateToolName = "submit_director_plan_update"
const SubmitDirectorPlanUpdateToolName = submitDirectorPlanUpdateToolName

type submitDirectorPlanUpdateInput struct {
	Decision director.Decision                        `json:"decision" jsonschema:"description=This turn's keep, patch, or replan decision and evidence. Preserve mode on retry and omit base_revision."`
	Updates  []interactive.DirectorPlanDocumentUpdate `json:"updates,omitempty" jsonschema:"description=Document patches to validate independently. Do not resend accepted files and omit unchanged files."`
	Finalize bool                                     `json:"finalize" jsonschema:"description=Whether to complete the draft after accepting valid patches in this call. The draft cannot finalize while any file is rejected."`
}

type SubmitDirectorPlanUpdateInput = submitDirectorPlanUpdateInput

func newInteractiveDirectorPlanTools(ctx InteractiveContext) ([]agent.ToolDefinition, error) {
	if ctx.SubmitDirectorPlanUpdate == nil {
		return nil, nil
	}
	submit, err := agent.InferTool(submitDirectorPlanUpdateToolName, "Submit incremental director Markdown patches for the current branch. Ordinary updates normally patch only agent-brief.md. Update director.md only when phase-planning premises fail or a major deviation occurs; update lore-context.md only when its current, waiting, or temporarily absent sets change. Each update uses the base_hash from context and should prefer replace_section. Files are accepted or rejected independently; retry only retry_documents. Nothing is written before finalize succeeds, after which the backend publishes atomically. keep uses empty updates with finalize=true. replan updates at least director.md and agent-brief.md; lore remains optional.", func(callCtx context.Context, input submitDirectorPlanUpdateInput) (agent.ToolResult, error) {
		receipt, err := ctx.SubmitDirectorPlanUpdate(callCtx, interactive.DirectorPlanUpdateSubmission{Decision: input.Decision, Updates: input.Updates, Finalize: input.Finalize})
		if err != nil {
			return agent.ToolResult{}, fmt.Errorf("submit director plan: %w", err)
		}
		data, err := json.Marshal(receipt)
		if err != nil {
			return agent.ToolResult{}, err
		}
		if receipt.Finalized && ctx.RequestDirectorCompletion != nil {
			requested := ctx.RequestDirectorCompletion(callCtx)
			slog.InfoContext(callCtx, fmt.Sprintf("[interactive-director] finalized structured plan patch completion_requested=%t changed_documents=%v", requested, receipt.ChangedDocuments))
		}
		result := agent.TextToolResult(string(data))
		result.Details = data
		return result, nil
	})
	if err != nil {
		return nil, err
	}
	definedSubmit, err := defineTool(submit, workspaceWriteDescriptor(ToolSourceHistory, "", agent.ToolRecoveryReconcilable))
	if err != nil {
		return nil, err
	}
	return []agent.ToolDefinition{definedSubmit}, nil
}

// NewInteractiveDirectorPlan builds the structured director plan submission
// tool for one background Director run.
func NewInteractiveDirectorPlan(ctx InteractiveContext) ([]agent.ToolDefinition, error) {
	return newInteractiveDirectorPlanTools(ctx)
}
