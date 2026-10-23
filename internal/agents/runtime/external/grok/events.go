package grok

import (
	"encoding/json"
	"fmt"

	agentevent "github.com/alfredxw/denova/agent/lifecycle/event"
	agentschema "github.com/alfredxw/denova/agent/schema"
)

// headlessEvent is one NDJSON record from
// `grok -p --output-format streaming-json`. The headless channel is not ACP: it
// reports per-token text, plan snapshots, tool mirrors and a terminal `end`.
type headlessEvent struct {
	Type       string              `json:"type"`
	Data       string              `json:"data"`
	StopReason string              `json:"stopReason"`
	Message    string              `json:"message"`
	SessionID  string              `json:"sessionId"`
	Usage      *headlessUsage      `json:"usage"`
	Entries    []headlessPlanEntry `json:"entries"`
	Tools      []string            `json:"tools"`
}

type headlessPlanEntry struct {
	Content  string `json:"content"`
	Priority string `json:"priority"`
	Status   string `json:"status"`
}

type headlessUsage struct {
	Input        int `json:"input_tokens"`
	Output       int `json:"output_tokens"`
	CacheRead    int `json:"cache_read_input_tokens"`
	Reasoning    int `json:"reasoning_tokens"`
	Total        int `json:"total_tokens"`
}

func (usage headlessUsage) tokens() *agentschema.TokenUsage {
	total := usage.Total
	if total == 0 {
		total = usage.Input + usage.Output
	}
	result := &agentschema.TokenUsage{PromptTokens: usage.Input, CompletionTokens: usage.Output, TotalTokens: total}
	result.PromptTokenDetails.CachedTokens = usage.CacheRead
	result.CompletionTokensDetails.ReasoningTokens = usage.Reasoning
	return result
}

// planItems maps a plan snapshot onto the product plan. Unknown statuses are
// refused instead of being folded into a default state.
func planItems(entries []headlessPlanEntry) ([]agentevent.TodoItem, error) {
	items := make([]agentevent.TodoItem, 0, len(entries))
	for index, entry := range entries {
		status := agentevent.TodoStatus(entry.Status)
		switch status {
		case agentevent.TodoPending, agentevent.TodoInProgress, agentevent.TodoCompleted:
		default:
			return nil, fmt.Errorf("unknown Grok plan status %q", entry.Status)
		}
		items = append(items, agentevent.TodoItem{ID: fmt.Sprint(index + 1), Text: entry.Content, Status: status})
	}
	return items, nil
}

// decodeHeadless classifies one line. An unrecognized type is reported so a
// provider addition surfaces instead of being silently treated as text.
func decodeHeadless(line []byte) (headlessEvent, error) {
	var event headlessEvent
	if err := json.Unmarshal(line, &event); err != nil {
		return event, fmt.Errorf("decode Grok headless event: %w", err)
	}
	if event.Type == "" {
		return event, fmt.Errorf("Grok headless event without a type")
	}
	return event, nil
}
