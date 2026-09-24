package external

import (
	"context"
	"errors"
	"time"

	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	agent "github.com/alfredxw/denova/agent"
)

// MaintenanceObserver records only actual provider compaction observations.
// Maintenance never gains permission to invoke product tools.
type MaintenanceObserver struct {
	Append    func(session.DisplayEvent) error
	EmitEvent func(agentrun.Event)
	Completed bool
}

func (observer *MaintenanceObserver) Emit(event agentrun.Event) error {
	if event.Type != "context_compaction" {
		return nil
	}
	if display, ok := CompactionDisplay(event); ok {
		if err := observer.Append(display); err != nil {
			return err
		}
		observer.Completed = display.Status == "success"
	}
	if observer.EmitEvent != nil {
		observer.EmitEvent(event)
	}
	return nil
}
func (*MaintenanceObserver) CallTool(context.Context, ToolCall) (ToolResult, error) {
	return ToolResult{}, errors.New("product tools are unavailable during context maintenance")
}

// CompactionDisplay projects provider facts without inventing a summary or a
// source coverage interval that the provider did not disclose.
func CompactionDisplay(event agentrun.Event) (session.DisplayEvent, bool) {
	status := event.DataString("status")
	if event.Type != "context_compaction" || (status != "completed" && status != "failed") {
		return session.DisplayEvent{}, false
	}
	if status == "completed" {
		status = "success"
	} else {
		status = "error"
	}
	return session.DisplayEvent{ID: event.DataString("id"), Role: "context_compaction", Status: status,
		Content: event.DataString("summary"), Phase: event.DataString("phase"), RuntimeManaged: true, CreatedAt: time.Now().UTC()}, true
}

// UsageDisplay carries aggregate provider totals without inventing a call count.
func UsageDisplay(usage *agent.TokenUsage) session.DisplayEvent {
	return session.DisplayEvent{Role: "token_usage", PromptTokens: usage.PromptTokens, CompletionTokens: usage.CompletionTokens,
		CachedPromptTokens: usage.PromptTokenDetails.CachedTokens, ReasoningTokens: usage.CompletionTokensDetails.ReasoningTokens, TotalTokens: usage.TotalTokens, CreatedAt: time.Now().UTC()}
}
