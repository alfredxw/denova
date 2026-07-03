package app

import (
	"encoding/json"
	"fmt"
	"strings"

	"denova/internal/interactive"
)

const (
	thinkOpenTag  = "<think>"
	thinkCloseTag = "</think>"
)

type interactiveStatePayload struct {
	Ops                []interactive.StateOp                       `json:"ops"`
	StateOps           []interactive.StateOp                       `json:"state_ops"`
	MemoryEntry        *interactive.InteractiveMemoryCreateRequest `json:"memory_entry"`
	StoryMemoryPatches []interactive.StoryMemoryPatch              `json:"story_memory_patches"`
}

func parseInteractiveAssistantOutput(content string) (string, error) {
	narrative := extractNarrative(content)
	if strings.TrimSpace(narrative) == "" {
		return "", fmt.Errorf("互动叙事内容为空")
	}
	return strings.TrimSpace(narrative), nil
}

type interactiveMemoryAgentResult struct {
	StateOps           []interactive.StateOp
	MemoryEntry        *interactive.InteractiveMemoryCreateRequest
	StoryMemoryPatches []interactive.StoryMemoryPatch
}

func parseInteractiveMemoryOutput(content string) (interactiveMemoryAgentResult, error) {
	var payload interactiveStatePayload
	if err := json.Unmarshal([]byte(extractJSONPayload(content)), &payload); err != nil {
		return interactiveMemoryAgentResult{}, fmt.Errorf("解析互动记忆失败: %w", err)
	}
	ops := payload.StateOps
	if len(ops) == 0 {
		ops = payload.Ops
	}
	if len(ops) > 0 {
		if err := validateStateOps(ops); err != nil {
			return interactiveMemoryAgentResult{}, err
		}
	}
	patches := payload.StoryMemoryPatches
	if len(patches) == 0 && payload.MemoryEntry != nil {
		patches = []interactive.StoryMemoryPatch{interactiveMemoryEntryToStoryPatch(*payload.MemoryEntry)}
	}
	return interactiveMemoryAgentResult{StateOps: ops, MemoryEntry: payload.MemoryEntry, StoryMemoryPatches: patches}, nil
}

func interactiveMemoryEntryToStoryPatch(entry interactive.InteractiveMemoryCreateRequest) interactive.StoryMemoryPatch {
	values := map[string]string{
		"event": strings.TrimSpace(firstNonEmptyString(entry.Summary, entry.Content, entry.Title)),
	}
	if len(entry.Places) > 0 {
		values["place"] = strings.Join(entry.Places, "，")
	}
	return interactive.StoryMemoryPatch{
		Op:          "append",
		StructureID: "plot_summary",
		Key:         strings.TrimSpace(entry.Title),
		Values:      values,
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func extractNarrative(content string) string {
	return stripThinkPrelude(content)
}

// stripThinkPrelude 移除叙事正文里残留的思考块，兜底模型把思考混入正文的情况：
// 配对 <think>...</think>、未闭合 <think>...，以及无开始标签、仅以 </think> 收尾的前言。
func stripThinkPrelude(s string) string {
	for {
		open := thinkIndexFold(s, thinkOpenTag)
		if open < 0 {
			break
		}
		closeIdx := thinkIndexFold(s[open:], thinkCloseTag)
		if closeIdx < 0 {
			s = s[:open]
			break
		}
		s = s[:open] + s[open+closeIdx+len(thinkCloseTag):]
	}
	if closeIdx := thinkIndexFold(s, thinkCloseTag); closeIdx >= 0 {
		s = s[closeIdx+len(thinkCloseTag):]
	}
	return strings.TrimSpace(s)
}

func thinkIndexFold(s, sub string) int {
	return strings.Index(strings.ToLower(s), strings.ToLower(sub))
}

func extractJSONPayload(content string) string {
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "```") {
		content = strings.TrimPrefix(content, "```json")
		content = strings.TrimPrefix(content, "```")
		content = strings.TrimSpace(content)
		content = strings.TrimSuffix(content, "```")
	}
	return strings.TrimSpace(content)
}

func validateStateOps(ops []interactive.StateOp) error {
	if len(ops) == 0 {
		return fmt.Errorf("互动状态变化不能为空：state_ops 至少需要一条本回合状态变化")
	}
	for _, op := range ops {
		switch op.Op {
		case "set", "merge", "push", "pull", "inc", "unset":
		default:
			return fmt.Errorf("不支持的互动状态操作: %s", op.Op)
		}
		if !isAllowedStatePath(op.Path) {
			return fmt.Errorf("不支持的互动状态路径: %s", op.Path)
		}
	}
	return nil
}

func isAllowedStatePath(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	allowedRoots := []string{
		"on_stage",
		"characters",
		"events",
		"location",
		"time",
		"pov",
		"scene",
		"action_space",
		"inventory",
		"resources",
		"world_flags",
		"rules",
		"threads",
	}
	for _, root := range allowedRoots {
		if path == root || strings.HasPrefix(path, root+".") {
			return true
		}
	}
	return false
}
