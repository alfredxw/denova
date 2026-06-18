package app

import (
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"

	"nova/internal/interactive"
)

const (
	narrativeStartTag  = "<NARRATIVE>"
	narrativeEndTag    = "</NARRATIVE>"
	stateDeltaStartTag = "<STATE_DELTA>"
	stateDeltaEndTag   = "</STATE_DELTA>"
)

type interactiveStatePayload struct {
	Ops                []interactive.StateOp                       `json:"ops"`
	StateOps           []interactive.StateOp                       `json:"state_ops"`
	MemoryEntry        *interactive.InteractiveMemoryCreateRequest `json:"memory_entry"`
	StoryMemoryPatches []interactive.StoryMemoryPatch              `json:"story_memory_patches"`
}

type interactiveHotStatePayload struct {
	Choices []string `json:"choices"`
}

var (
	hotStateStartPattern    = regexp.MustCompile(`(?i)<\s*hot_state\s*>`)
	hotStateEndPattern      = regexp.MustCompile(`(?i)<\s*/\s*hot_state\s*>`)
	hiddenStateStartPattern = regexp.MustCompile(`(?i)<\s*(hot_state|state_delta)\s*>?`)
)

func parseInteractiveAssistantOutput(content string) (string, []interactive.StateOp, *interactive.HotState, error) {
	narrative := extractNarrative(content)
	if strings.TrimSpace(narrative) == "" {
		return "", nil, nil, fmt.Errorf("互动叙事内容为空")
	}

	stateBlock, ok := extractBetween(content, stateDeltaStartTag, stateDeltaEndTag)
	hotState, hotErr := parseInteractiveHotState(content)
	if hotErr != nil {
		log.Printf("[interactive-agent] parse hot state failed err=%v content=%q", hotErr, content)
	}
	if !ok || strings.TrimSpace(stateBlock) == "" {
		return strings.TrimSpace(narrative), nil, hotState, nil
	}
	ops, err := parseInteractiveStateOps(stateBlock)
	if err != nil {
		return strings.TrimSpace(narrative), nil, hotState, nil
	}
	return strings.TrimSpace(narrative), ops, hotState, nil
}

func parseInteractiveStateOps(content string) ([]interactive.StateOp, error) {
	result, err := parseInteractiveMemoryOutput(content)
	if err != nil {
		return nil, err
	}
	if len(result.StateOps) == 0 {
		return nil, fmt.Errorf("互动状态变化不能为空：STATE_DELTA.ops 至少需要一条本回合状态变化")
	}
	return result.StateOps, nil
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

func parseInteractiveHotState(content string) (*interactive.HotState, error) {
	stateBlock, ok := extractPatternBlock(content, hotStateStartPattern, hotStateEndPattern)
	if !ok || strings.TrimSpace(stateBlock) == "" {
		return nil, nil
	}
	var payload interactiveHotStatePayload
	if err := json.Unmarshal([]byte(extractJSONPayload(stateBlock)), &payload); err != nil {
		return nil, fmt.Errorf("解析互动热状态失败: %w", err)
	}
	choices := make([]string, 0, len(payload.Choices))
	seen := map[string]bool{}
	for _, choice := range payload.Choices {
		choice = strings.TrimSpace(choice)
		if choice == "" || seen[choice] {
			continue
		}
		choices = append(choices, choice)
		seen[choice] = true
		if len(choices) >= 5 {
			break
		}
	}
	if len(choices) == 0 {
		return nil, nil
	}
	return &interactive.HotState{Choices: choices}, nil
}

func extractNarrative(content string) string {
	if narrative, ok := extractBetween(content, narrativeStartTag, narrativeEndTag); ok {
		return narrative
	}
	if idx := firstHiddenStateTagIndex(content); idx >= 0 {
		return content[:idx]
	}
	return content
}

func firstHiddenStateTagIndex(content string) int {
	if loc := hiddenStateStartPattern.FindStringIndex(content); loc != nil {
		return loc[0]
	}
	return -1
}

func extractBetween(content, startTag, endTag string) (string, bool) {
	start := strings.Index(content, startTag)
	if start < 0 {
		return "", false
	}
	start += len(startTag)
	end := strings.Index(content[start:], endTag)
	if end < 0 {
		return content[start:], true
	}
	return content[start : start+end], true
}

func extractPatternBlock(content string, startPattern, endPattern *regexp.Regexp) (string, bool) {
	start := startPattern.FindStringIndex(content)
	if start == nil {
		return "", false
	}
	bodyStart := start[1]
	end := endPattern.FindStringIndex(content[bodyStart:])
	if end == nil {
		return content[bodyStart:], true
	}
	return content[bodyStart : bodyStart+end[0]], true
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
		return fmt.Errorf("互动状态变化不能为空：STATE_DELTA.ops 至少需要一条本回合状态变化")
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
