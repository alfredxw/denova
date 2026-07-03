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

const (
	interactiveMemoryToolListLimit    = 24
	interactiveMemoryToolSummaryLimit = 800
)

// InteractiveStoryToolContext provides story-scoped read tools for one
// interactive story run. The story and branch are fixed by the backend; the
// model never supplies them.
type InteractiveStoryToolContext struct {
	Store                    *interactive.Store
	StoryID                  string
	BranchID                 string
	DirectorPlanAllowedPaths []string
	PrepareTurn              func(context.Context, interactive.TurnBrief) (interactive.RuleResolution, error)
}

type listInteractiveMemoriesInput struct {
	Query  string   `json:"query,omitempty" jsonschema:"description=可选检索词，用当前行动、人物、地点、线索或目标描述相关记忆"`
	People []string `json:"people,omitempty" jsonschema:"description=可选人物筛选，匹配记忆 people 字段"`
	Places []string `json:"places,omitempty" jsonschema:"description=可选地点筛选，匹配记忆 places 字段"`
	Tags   []string `json:"tags,omitempty" jsonschema:"description=可选标签筛选，匹配记忆 tags 字段"`
	Limit  int      `json:"limit,omitempty" jsonschema:"description=最多返回多少条索引，默认 12，最大 24"`
}

type readInteractiveMemoriesInput struct {
	IDs   []string `json:"ids" jsonschema:"description=要读取正文的互动长期记忆 ID 列表；可按需一次读取多个相关记忆"`
	Query string   `json:"query,omitempty" jsonschema:"description=可选，说明本次读取记忆是为了回答哪类当前行动或线索；用于记录最近召回"`
}

type interactiveMemoryToolOutput struct {
	Source    interactiveMemoryToolSource `json:"source"`
	Limits    map[string]int              `json:"limits"`
	Truncated bool                        `json:"truncated"`
	Memories  any                         `json:"memories"`
}

type interactiveMemoryToolSource struct {
	Kind     string `json:"kind"`
	StoryID  string `json:"story_id"`
	BranchID string `json:"branch_id"`
	Path     string `json:"path"`
}

type interactiveMemoryIndexItem struct {
	ID         string   `json:"id"`
	BranchID   string   `json:"branch_id"`
	TurnID     string   `json:"turn_id,omitempty"`
	Title      string   `json:"title"`
	Summary    string   `json:"summary"`
	People     []string `json:"people,omitempty"`
	Places     []string `json:"places,omitempty"`
	Tags       []string `json:"tags,omitempty"`
	Importance int      `json:"importance"`
	Manual     bool     `json:"manual,omitempty"`
	UpdatedAt  string   `json:"updated_at,omitempty"`
}

func newInteractiveMemoryTools(ctx InteractiveStoryToolContext) ([]tool.BaseTool, error) {
	ctx.StoryID = strings.TrimSpace(ctx.StoryID)
	ctx.BranchID = strings.TrimSpace(ctx.BranchID)
	if ctx.Store == nil || ctx.StoryID == "" {
		return nil, nil
	}
	listTool, err := utils.InferTool("list_interactive_memories", "列出当前互动故事分支的长期记忆轻量索引。用于根据当前行动、人物、地点、线索或标签判断本轮需要读取哪些历史事实；默认排除归档记忆和其他分支记忆。", func(callCtx context.Context, input listInteractiveMemoriesInput) (string, error) {
		_ = callCtx
		limit := normalizeInteractiveMemoryToolLimit(input.Limit, 12, interactiveMemoryToolListLimit)
		entries, err := ctx.Store.VisibleInteractiveMemories(ctx.StoryID, ctx.BranchID, interactiveMemoryToolListLimit)
		if err != nil {
			return "", err
		}
		filtered := filterInteractiveMemoryToolEntries(entries, input)
		truncated := len(filtered) > limit
		if truncated {
			filtered = filtered[:limit]
		}
		items := make([]interactiveMemoryIndexItem, 0, len(filtered))
		for _, entry := range filtered {
			items = append(items, interactiveMemoryIndexItem{
				ID:         entry.ID,
				BranchID:   entry.BranchID,
				TurnID:     entry.TurnID,
				Title:      entry.Title,
				Summary:    trimInteractiveMemoryToolText(firstNonEmpty(entry.Summary, entry.Content), interactiveMemoryToolSummaryLimit),
				People:     entry.People,
				Places:     entry.Places,
				Tags:       entry.Tags,
				Importance: entry.Importance,
				Manual:     entry.Manual,
				UpdatedAt:  entry.UpdatedAt,
			})
		}
		return marshalInteractiveMemoryToolOutput(interactiveMemoryToolOutput{
			Source:    interactiveMemoryToolSource{Kind: "interactive_memory_index", StoryID: ctx.StoryID, BranchID: ctx.BranchID, Path: fmt.Sprintf("interactive/memory/story-%s.json", ctx.StoryID)},
			Limits:    map[string]int{"max_items": interactiveMemoryToolListLimit, "returned_items": len(items), "summary_bytes_per_item": interactiveMemoryToolSummaryLimit},
			Truncated: truncated,
			Memories:  items,
		})
	})
	if err != nil {
		return nil, err
	}
	readTool, err := utils.InferTool("read_interactive_memories", "按 ID 读取当前互动故事分支的长期记忆完整正文。用于在 list_interactive_memories 判断相关后读取关键记忆；归档记忆和其他分支记忆不可读取。", func(callCtx context.Context, input readInteractiveMemoriesInput) (string, error) {
		_ = callCtx
		entries, err := ctx.Store.ReadVisibleInteractiveMemories(ctx.StoryID, ctx.BranchID, input.IDs, 0)
		if err != nil {
			return "", err
		}
		ids := make([]string, 0, len(entries))
		for _, entry := range entries {
			ids = append(ids, entry.ID)
		}
		if len(ids) > 0 {
			if err := ctx.Store.RecordInteractiveMemoryRecall(ctx.StoryID, ctx.BranchID, "", input.Query, ids); err != nil {
				return "", err
			}
		}
		return marshalInteractiveMemoryToolOutput(interactiveMemoryToolOutput{
			Source:    interactiveMemoryToolSource{Kind: "interactive_memory_entries", StoryID: ctx.StoryID, BranchID: ctx.BranchID, Path: fmt.Sprintf("interactive/memory/story-%s.json", ctx.StoryID)},
			Limits:    map[string]int{"requested_items": len(input.IDs), "returned_items": len(entries)},
			Truncated: false,
			Memories:  entries,
		})
	})
	if err != nil {
		return nil, err
	}
	return []tool.BaseTool{listTool, readTool}, nil
}

func newInteractiveTurnTools(ctx InteractiveStoryToolContext) ([]tool.BaseTool, error) {
	if ctx.PrepareTurn == nil {
		return nil, nil
	}
	prepareTool, err := utils.InferTool("prepare_interactive_turn", "提交本回合 TurnBrief 并执行固定规则结算。Interactive Agent 负责语义理解、事件选择和剧情编排；本工具只校验 TurnBrief、记录审计信息，并执行数值、骰子、词条、资源、关系和终局候选等确定性规则检定，返回 RuleResolution。输出正文前必须先根据返回结果调整叙事。", func(callCtx context.Context, input interactive.TurnBrief) (string, error) {
		resolution, err := ctx.PrepareTurn(callCtx, input)
		if err != nil {
			return "", err
		}
		data, err := json.MarshalIndent(map[string]any{
			"source": map[string]string{
				"kind":      "interactive_rule_resolution",
				"story_id":  ctx.StoryID,
				"branch_id": ctx.BranchID,
			},
			"rule_resolution": resolution,
		}, "", "  ")
		if err != nil {
			return "", err
		}
		return string(data), nil
	})
	if err != nil {
		return nil, err
	}
	return []tool.BaseTool{prepareTool}, nil
}

func normalizeInteractiveMemoryToolLimit(value, fallback, max int) int {
	if value <= 0 {
		value = fallback
	}
	if value > max {
		value = max
	}
	return value
}

func filterInteractiveMemoryToolEntries(entries []interactive.InteractiveMemoryEntry, input listInteractiveMemoriesInput) []interactive.InteractiveMemoryEntry {
	out := make([]interactive.InteractiveMemoryEntry, 0, len(entries))
	query := strings.ToLower(strings.TrimSpace(input.Query))
	people := normalizeInteractiveMemoryToolTerms(input.People)
	places := normalizeInteractiveMemoryToolTerms(input.Places)
	tags := normalizeInteractiveMemoryToolTerms(input.Tags)
	for _, entry := range entries {
		if query != "" && !interactiveMemoryEntryContains(entry, query) {
			continue
		}
		if len(people) > 0 && !interactiveMemoryListIntersects(entry.People, people) {
			continue
		}
		if len(places) > 0 && !interactiveMemoryListIntersects(entry.Places, places) {
			continue
		}
		if len(tags) > 0 && !interactiveMemoryListIntersects(entry.Tags, tags) {
			continue
		}
		out = append(out, entry)
	}
	return out
}

func normalizeInteractiveMemoryToolTerms(values []string) map[string]bool {
	out := map[string]bool{}
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" {
			out[value] = true
		}
	}
	return out
}

func interactiveMemoryListIntersects(values []string, terms map[string]bool) bool {
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if terms[value] {
			return true
		}
	}
	return false
}

func interactiveMemoryEntryContains(entry interactive.InteractiveMemoryEntry, query string) bool {
	haystack := strings.ToLower(strings.Join([]string{
		entry.ID,
		entry.Title,
		entry.Summary,
		entry.Content,
		strings.Join(entry.People, " "),
		strings.Join(entry.Places, " "),
		strings.Join(entry.Tags, " "),
	}, " "))
	for _, term := range strings.Fields(query) {
		if !strings.Contains(haystack, term) {
			return false
		}
	}
	return true
}

func trimInteractiveMemoryToolText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 {
		if value != "" {
			return ""
		}
		return value
	}
	if len(value) <= limit {
		return value
	}
	trimmed, _ := truncateUTF8Bytes(value, limit)
	return strings.TrimSpace(trimmed)
}

func marshalInteractiveMemoryToolOutput(output interactiveMemoryToolOutput) (string, error) {
	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
