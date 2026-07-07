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
	TurnID                   string
	ActorState               interactive.StoryDirectorActorStateSystem
	DirectorPlanAllowedPaths []string
	OnActorStateApplied      func(appliedOps int)
	OnStoryMemoryApplied     func(applied int)
	OnStateMaintenanceFailed func(error)
	// DisplayConversation receives display-only progress for background helper
	// agents. It must not receive final assistant text as model-visible context.
	DisplayConversation Conversation
	PrepareTurn         func(context.Context, interactive.TurnCheckRequest) (interactive.RuleResolution, error)
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

type applyActorStatePatchInput struct {
	Patches []interactive.ActorStatePatch `json:"patches" jsonschema:"description=要写入的关键 Actor 结构化状态更新。每条 patch 必须包含 actor_id、template_id、state 和 reason；state 只能使用状态系统 schema 中声明的字段路径。"`
}

type applyStoryMemoryPatchesInput struct {
	Patches []interactive.StoryMemoryPatch `json:"patches" jsonschema:"description=要写入的故事记忆 patch。每条 patch 必须遵守当前注入的 Story Memory schema；op 仅使用 upsert、append、archive、restore。"`
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

type actorStatePatchToolOutput struct {
	AppliedActors []string `json:"applied_actors"`
	Ops           int      `json:"ops"`
	BranchID      string   `json:"branch_id"`
	TurnID        string   `json:"turn_id"`
}

type storyMemoryPatchToolOutput struct {
	AppliedRecords int    `json:"applied_records"`
	BranchID       string `json:"branch_id"`
	TurnID         string `json:"turn_id"`
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

func newInteractiveActorStateTools(ctx InteractiveStoryToolContext) ([]tool.BaseTool, error) {
	ctx.StoryID = strings.TrimSpace(ctx.StoryID)
	ctx.BranchID = strings.TrimSpace(ctx.BranchID)
	ctx.TurnID = strings.TrimSpace(ctx.TurnID)
	if ctx.Store == nil || ctx.StoryID == "" || ctx.TurnID == "" {
		return nil, nil
	}
	applyTool, err := utils.InferTool("apply_actor_state_patch", "创建或更新关键 Actor 的结构化状态。只用于主角、重要角色、反派、势力型 Actor 等会影响后续承接或规则检定的对象；路人和一次性 NPC 留在故事记忆，不写入状态系统。后端会按状态系统 schema 校验 actor 类型、字段路径、值类型和可见性，并把变更写成可重放 StateOp。", func(callCtx context.Context, input applyActorStatePatchInput) (string, error) {
		_ = callCtx
		result, err := interactive.ValidateActorStatePatches(ctx.ActorState, input.Patches, ctx.TurnID)
		if err != nil {
			reportStateMaintenanceFailure(ctx, err)
			return "", err
		}
		if len(result.Ops) == 0 {
			err := fmt.Errorf("Actor 状态更新没有产生可写入操作")
			reportStateMaintenanceFailure(ctx, err)
			return "", err
		}
		if _, err := ctx.Store.AppendStateDelta(ctx.StoryID, interactive.AppendStateDeltaRequest{
			ParentID: ctx.TurnID,
			BranchID: ctx.BranchID,
			Ops:      result.Ops,
		}); err != nil {
			reportStateMaintenanceFailure(ctx, err)
			return "", err
		}
		if ctx.OnActorStateApplied != nil {
			ctx.OnActorStateApplied(len(result.Ops))
		}
		data, err := json.MarshalIndent(actorStatePatchToolOutput{
			AppliedActors: result.AppliedActors,
			Ops:           len(result.Ops),
			BranchID:      ctx.BranchID,
			TurnID:        ctx.TurnID,
		}, "", "  ")
		if err != nil {
			return "", err
		}
		return string(data), nil
	})
	if err != nil {
		return nil, err
	}
	return []tool.BaseTool{applyTool}, nil
}

func newInteractiveStoryMemoryPatchTools(ctx InteractiveStoryToolContext) ([]tool.BaseTool, error) {
	ctx.StoryID = strings.TrimSpace(ctx.StoryID)
	ctx.BranchID = strings.TrimSpace(ctx.BranchID)
	ctx.TurnID = strings.TrimSpace(ctx.TurnID)
	if ctx.Store == nil || ctx.StoryID == "" || ctx.TurnID == "" {
		return nil, nil
	}
	applyTool, err := utils.InferTool("apply_story_memory_patches", "写入当前互动故事分支的故事记忆 patch。只用于已经在本回合正文中成立、后续需要承接的信息；字段、结构、key 和 values 必须来自注入的 Story Memory schema。后端会按分支和结构校验，并写入 story memory 记录。", func(callCtx context.Context, input applyStoryMemoryPatchesInput) (string, error) {
		_ = callCtx
		if len(input.Patches) == 0 {
			err := fmt.Errorf("故事记忆 patch 不能为空")
			reportStateMaintenanceFailure(ctx, err)
			return "", err
		}
		records, err := ctx.Store.ApplyStoryMemoryPatches(ctx.StoryID, ctx.BranchID, ctx.TurnID, input.Patches)
		if err != nil {
			reportStateMaintenanceFailure(ctx, err)
			return "", err
		}
		if ctx.OnStoryMemoryApplied != nil {
			ctx.OnStoryMemoryApplied(len(records))
		}
		data, err := json.MarshalIndent(storyMemoryPatchToolOutput{
			AppliedRecords: len(records),
			BranchID:       ctx.BranchID,
			TurnID:         ctx.TurnID,
		}, "", "  ")
		if err != nil {
			return "", err
		}
		return string(data), nil
	})
	if err != nil {
		return nil, err
	}
	return []tool.BaseTool{applyTool}, nil
}

func reportStateMaintenanceFailure(ctx InteractiveStoryToolContext, err error) {
	if ctx.OnStateMaintenanceFailed != nil && err != nil {
		ctx.OnStateMaintenanceFailed(err)
	}
}

func newInteractiveTurnTools(ctx InteractiveStoryToolContext) ([]tool.BaseTool, error) {
	if ctx.PrepareTurn == nil {
		return nil, nil
	}
	desc := strings.Join([]string{
		"执行本回合一次 1d20 规则检定。Interactive Agent 负责填写用户行为、意图、挑战、消耗、当前状态说明、加成原因和值、难度等级，以及大成功/成功/失败/大失败四档后果；本工具只负责掷骰、应用优势或劣势、求和、判定结果，并返回命中的最终后果。",
		"参数协议：difficulty 必须是 very_easy/easy/normal/hard/very_hard；普通难度使用 normal，不要使用 medium/moderate。rule 可省略；如提供，template 只能是 dice_check，dice 只能是 1d20，roll_mode 只能是 normal/advantage/disadvantage。",
		`最小示例：{"action":"撬锁","intent":"潜入仓库","challenge":"巡逻逼近时开锁","cost":"失败会消耗体力并暴露","state":"主角有简易工具，体力尚可。","difficulty":"normal","outcomes":{"critical_success":{"result":"无声开锁并发现额外线索。"},"success":{"result":"开锁成功但耗时。"},"failure":{"result":"没能打开，巡逻更近。"},"critical_failure":{"result":"工具折断并惊动巡逻。"}}}`,
	}, "\n")
	prepareTool, err := utils.InferTool("prepare_interactive_turn", desc, func(callCtx context.Context, input interactive.TurnCheckRequest) (string, error) {
		resolution, err := ctx.PrepareTurn(callCtx, input)
		if err != nil {
			return "", err
		}
		data, err := json.MarshalIndent(resolution.ToolOutput(), "", "  ")
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
