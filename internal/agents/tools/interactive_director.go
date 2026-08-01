package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	agent "github.com/alfredxw/denova/agent"

	"denova/internal/interactive"
)

const submitDirectorPlanUpdateToolName = "submit_director_plan_update"
const SubmitDirectorPlanUpdateToolName = submitDirectorPlanUpdateToolName

type submitDirectorPlanUpdateInput struct {
	Decision interactive.PlanDecision                 `json:"decision" jsonschema:"description=本轮 keep、patch 或 replan 决策及证据；重试时保持 mode 不变，不填写 base_revision"`
	Updates  []interactive.DirectorPlanDocumentUpdate `json:"updates,omitempty" jsonschema:"description=本次要独立校验的文档 Patch；已 accepted 的文件不要重传，未变化文件必须省略"`
	Finalize bool                                     `json:"finalize" jsonschema:"description=是否在接收本次合法 Patch 后完成草稿；存在 rejected 文件时不会 finalize"`
}

type SubmitDirectorPlanUpdateInput = submitDirectorPlanUpdateInput

func newInteractiveDirectorPlanTools(ctx InteractiveContext) ([]agent.ToolDefinition, error) {
	if ctx.SubmitDirectorPlanUpdate == nil {
		return nil, nil
	}
	submit, err := agent.InferTool(submitDirectorPlanUpdateToolName, "增量提交当前分支导演 Markdown Patch。普通更新默认只 patch agent-brief.md；director.md 仅在阶段规划前提失效或重大偏差时更新，lore-context.md 仅在当前/候场/暂离场资料集合变化时更新。每个 update 使用上下文中的 base_hash，优先 replace_section；文件独立 accepted/rejected，重试只发送 retry_documents。finalize 成功前不写工作区，完成后由后端原子发布。keep 使用空 updates 且 finalize=true；replan 至少更新 director.md 与 agent-brief.md，Lore 仍按需。", func(callCtx context.Context, input submitDirectorPlanUpdateInput) (agent.ToolResult, error) {
		receipt, err := ctx.SubmitDirectorPlanUpdate(callCtx, interactive.DirectorPlanUpdateSubmission{Decision: input.Decision, Updates: input.Updates, Finalize: input.Finalize})
		if err != nil {
			return agent.ToolResult{}, fmt.Errorf("提交导演规划失败: %w", err)
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
