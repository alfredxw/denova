package toolruntime

import (
	"context"
	"denova/internal/agents/run"
	agenttool "denova/internal/agents/tool"
	"fmt"
	"strings"

	agent "github.com/alfredxw/denova/agent"
)

type toolLifecycleObserverKey struct{}

// ToolLifecycleObserver is the durability boundary around a real tool effect.
// BeforeTool must return only after the start record is durable. AfterTool must
// return only after the matching completion record is durable. An observer
// failure is fatal to the current run: executing without a start record or
// hiding an uncertain completion would make crash recovery unsafe.
type ToolLifecycleObserver interface {
	BeforeTool(context.Context, agenttool.Decision, string) error
	AfterTool(context.Context, agenttool.ExecutionRecord) error
}

func ContextWithToolLifecycleObserver(ctx context.Context, observer ToolLifecycleObserver) context.Context {
	if observer == nil {
		return ctx
	}
	return context.WithValue(ctx, toolLifecycleObserverKey{}, observer)
}

func ToolLifecycleObserverFromContext(ctx context.Context) ToolLifecycleObserver {
	if ctx == nil {
		return nil
	}
	observer, _ := ctx.Value(toolLifecycleObserverKey{}).(ToolLifecycleObserver)
	return observer
}

func recordToolStart(ctx context.Context, decision agenttool.Decision, arguments string) error {
	observer := ToolLifecycleObserverFromContext(ctx)
	if observer == nil {
		return nil
	}
	if strings.TrimSpace(decision.ExecutionID) == "" {
		return agent.MarkToolControlError(fmt.Errorf("record durable tool start for %q: missing tool call id", decision.ToolName))
	}
	if err := observer.BeforeTool(ctx, decision, arguments); err != nil {
		return agent.MarkToolControlError(fmt.Errorf("record durable tool start for %q: %w", decision.ToolName, err))
	}
	return nil
}

func recordToolFinish(ctx context.Context, record agenttool.ExecutionRecord) error {
	if observer := agentrun.ObserverFromContext(ctx); observer != nil {
		observer.RecordToolExecution(record)
	}
	observer := ToolLifecycleObserverFromContext(ctx)
	if observer == nil {
		return nil
	}
	if err := observer.AfterTool(ctx, record); err != nil {
		return agent.MarkToolControlError(fmt.Errorf("record durable tool finish for %q: %w", record.ToolName, err))
	}
	return nil
}
