package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"

	"denova/internal/interactive"
)

const initializeStoryStateSchemaToolName = "initialize_story_state_schema"

func newInteractiveOpeningStateSchemaTools(ctx InteractiveStoryToolContext) ([]tool.BaseTool, error) {
	if ctx.SubmitStateSchemaBatch == nil {
		return nil, nil
	}
	submitTool, err := utils.InferTool(
		initializeStoryStateSchemaToolName,
		"仅在故事首回合正文之前，增量暂存本故事的状态模板与字段结构。每个 item 使用稳定 item_id，并用 opening/opening-draft（或工具允许的 trpg ID）填写来源化 requirement；value_policy 必须是 schema_only。adaptation 只能包含 template_ops，禁止 initial_actor_ops 和 actor_ops：Actor 创建与所有初始值必须稍后通过 submit_interactive_turn.state_changes 提交。工具分别返回 accepted、rejected、blocked；只重试失败项，finalized=true 后再输出开局正文。草稿不会单独写入，只有正文、初始状态和 choices 全部通过时才原子落盘。已有字段足够时，提交 decision=covered 的有来源审查项并使用空 adaptation 后 finalize。",
		func(callCtx context.Context, input interactive.ActorStateSchemaBatch) (string, error) {
			result, err := ctx.SubmitStateSchemaBatch(callCtx, input)
			if err != nil {
				return "", err
			}
			data, err := json.MarshalIndent(result, "", "  ")
			if err != nil {
				return "", fmt.Errorf("序列化开局状态结构 Batch 结果失败: %w", err)
			}
			return string(data), nil
		},
	)
	if err != nil {
		return nil, err
	}
	return []tool.BaseTool{submitTool}, nil
}

func newInteractiveStateSchemaTools(ctx InteractiveStoryToolContext) ([]tool.BaseTool, error) {
	if ctx.SubmitStateSchemaBatch == nil {
		return nil, nil
	}
	submitTool, err := utils.InferTool(
		"submit_state_schema_adaptation",
		"增量提交首轮后或用户显式复审时的状态结构 Batch。每个 item 使用稳定 item_id，自包含来源化 requirement 与对应最小 diff；每个 requirement 必须声明 value_policy，initialize 必须在同一 item 用字段级 actor_ops set 提交可靠非空值。工具分别返回 accepted、rejected、blocked 及精确错误路径，重试时只发送失败或阻塞项。finalize 成功前不修改故事，最终迁移由后端原子完成。",
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
