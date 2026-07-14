package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"

	"denova/internal/interactive"
)

const submitDirectorPlanUpdateToolName = "submit_director_plan_update"

type submitDirectorPlanUpdateInput struct {
	Decision interactive.PlanDecision      `json:"decision" jsonschema:"description=本轮 keep、patch 或 replan 决策及其证据；不要填写 base_revision，后端绑定当前运行版本"`
	Docs     *interactive.DirectorPlanDocs `json:"docs,omitempty" jsonschema:"description=patch/replan 时同时提交两份完整文档；keep 时必须省略"`
}

func newInteractiveDirectorPlanTools(ctx InteractiveStoryToolContext) ([]tool.BaseTool, error) {
	if ctx.SubmitDirectorPlanUpdate == nil {
		return nil, nil
	}
	submit, err := utils.InferTool(submitDirectorPlanUpdateToolName, "一次性提交当前分支的导演规划决策。keep 只提交 decision；patch/replan 必须在 docs 中同时提交完整 plan（director.md）和 lore_context（lore-context.md）。后端会整体校验版本、固定标题、资料引用与大小后再接受。", func(callCtx context.Context, input submitDirectorPlanUpdateInput) (string, error) {
		receipt, err := ctx.SubmitDirectorPlanUpdate(callCtx, interactive.DirectorPlanUpdateSubmission{Decision: input.Decision, Docs: input.Docs})
		if err != nil {
			return "", fmt.Errorf("提交导演规划失败: %w", err)
		}
		data, err := json.Marshal(receipt)
		if err != nil {
			return "", err
		}
		if receipt.Accepted {
			requested := requestInteractiveDirectorPlanCompletion(callCtx)
			log.Printf("[interactive-director] accepted structured plan submission completion_requested=%t", requested)
		}
		return string(data), nil
	})
	if err != nil {
		return nil, err
	}
	return []tool.BaseTool{submit}, nil
}
