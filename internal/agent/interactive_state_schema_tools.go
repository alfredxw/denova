package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"

	"denova/internal/interactive"
)

func newInteractiveStateSchemaTools(ctx InteractiveStoryToolContext) ([]tool.BaseTool, error) {
	if ctx.SubmitStateSchemaProposal == nil {
		return nil, nil
	}
	submitTool, err := utils.InferTool(
		"submit_state_schema_adaptation",
		"提交首轮后或用户显式复审时的状态结构审查提案。必须列出有明确来源的长期状态需求及其覆盖、添加、替换或忽略决策；工具只暂存并校验提案，不直接写 Actor State，最终迁移由后端原子完成。即使无需修改 schema 也必须调用一次。",
		func(callCtx context.Context, input interactive.ActorStateSchemaProposal) (string, error) {
			preview, err := ctx.SubmitStateSchemaProposal(callCtx, input)
			if err != nil {
				return "", err
			}
			data, err := json.MarshalIndent(preview, "", "  ")
			if err != nil {
				return "", fmt.Errorf("序列化状态结构提案预览失败: %w", err)
			}
			return string(data), nil
		},
	)
	if err != nil {
		return nil, err
	}
	return []tool.BaseTool{submitTool}, nil
}
