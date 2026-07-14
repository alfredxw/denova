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
	if ctx.SubmitStateSchemaBatch == nil {
		return nil, nil
	}
	submitTool, err := utils.InferTool(
		"submit_state_schema_adaptation",
		"增量提交首轮后或用户显式复审时的状态结构 Batch。每个 item 使用稳定 item_id，自包含来源化 requirement 与对应最小 diff；工具分别返回 accepted、rejected、blocked，重试时只发送失败或阻塞项。finalize 成功前不修改故事，最终迁移由后端原子完成。",
		func(callCtx context.Context, input interactive.ActorStateSchemaBatch) (string, error) {
			result, err := ctx.SubmitStateSchemaBatch(callCtx, input)
			if err != nil {
				return "", err
			}
			data, err := json.MarshalIndent(result, "", "  ")
			if err != nil {
				return "", fmt.Errorf("序列化状态结构 Batch 结果失败: %w", err)
			}
			return string(data), nil
		},
	)
	if err != nil {
		return nil, err
	}
	return []tool.BaseTool{submitTool}, nil
}
