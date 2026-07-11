package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"

	"denova/internal/interactive"
)

type readInteractiveEventCardsInput struct {
	EventRefs []string `json:"event_refs" jsonschema:"description=要读取的事件卡 event_ref 列表，格式为 package_id/card_id；一次最多 8 张"`
}

func newInteractiveEventTools(ctx InteractiveStoryToolContext) ([]tool.BaseTool, error) {
	ctx.StoryID = strings.TrimSpace(ctx.StoryID)
	if ctx.Store == nil || ctx.StoryID == "" {
		return nil, nil
	}
	readTool, err := utils.InferTool("read_event_cards", "按 event_ref 读取当前故事导演显式选择的事件包卡片详情。仅在 EventOpportunity.kind=new 时按紧凑索引读取真正相关的少量卡片；一次最多读取 8 张，不能读取未选择事件包或默认回退卡片。", func(callCtx context.Context, input readInteractiveEventCardsInput) (string, error) {
		_ = callCtx
		if len(input.EventRefs) == 0 {
			return "", fmt.Errorf("event_refs 不能为空")
		}
		cards, err := ctx.Store.ReadDirectorEventCards(ctx.StoryID, input.EventRefs)
		if err != nil {
			return "", err
		}
		payload := struct {
			Source map[string]string           `json:"source"`
			Limits map[string]int              `json:"limits"`
			Cards  []interactive.DirectorEvent `json:"cards"`
		}{
			Source: map[string]string{"kind": "selected_story_director_event_cards", "story_id": ctx.StoryID},
			Limits: map[string]int{"max_items": 8, "returned_items": len(cards)},
			Cards:  cards,
		}
		data, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return "", err
		}
		return string(data), nil
	})
	if err != nil {
		return nil, err
	}
	return []tool.BaseTool{readTool}, nil
}
