package app

import (
	"encoding/json"
	"fmt"
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
	Ops []interactive.StateOp `json:"ops"`
}

func parseInteractiveAssistantOutput(content string) (string, []interactive.StateOp, error) {
	narrative := extractNarrative(content)
	if strings.TrimSpace(narrative) == "" {
		return "", nil, fmt.Errorf("互动叙事内容为空")
	}

	stateBlock, ok := extractBetween(content, stateDeltaStartTag, stateDeltaEndTag)
	if !ok || strings.TrimSpace(stateBlock) == "" {
		return strings.TrimSpace(narrative), nil, fmt.Errorf("互动状态变化不能为空：请在正文后输出非空 <STATE_DELTA> JSON")
	}
	var payload interactiveStatePayload
	if err := json.Unmarshal([]byte(strings.TrimSpace(stateBlock)), &payload); err != nil {
		return strings.TrimSpace(narrative), nil, fmt.Errorf("解析互动状态失败: %w", err)
	}
	if err := validateStateOps(payload.Ops); err != nil {
		return strings.TrimSpace(narrative), nil, err
	}
	return strings.TrimSpace(narrative), payload.Ops, nil
}

func extractNarrative(content string) string {
	if narrative, ok := extractBetween(content, narrativeStartTag, narrativeEndTag); ok {
		return narrative
	}
	if idx := strings.Index(content, stateDeltaStartTag); idx >= 0 {
		return content[:idx]
	}
	return content
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
		"inventory",
		"resources",
		"world_flags",
		"rules",
		"threads",
		"action_space",
	}
	for _, root := range allowedRoots {
		if path == root || strings.HasPrefix(path, root+".") {
			return true
		}
	}
	return false
}
