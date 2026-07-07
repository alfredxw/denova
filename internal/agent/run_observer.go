package agent

import (
	"context"
	"sync"
	"time"
)

type runObserverKey struct{}

// RunObserver records durable state for one Agent run without changing model-visible behavior.
type RunObserver struct {
	ledger       *RunLedger
	rootSpanID   string
	llmSpanID    string
	pendingTools map[string]*traceSpanHandle
	mu           sync.Mutex
}

func newRunObserver(ledger *RunLedger, rootSpanID string) *RunObserver {
	return &RunObserver{ledger: ledger, rootSpanID: rootSpanID, pendingTools: map[string]*traceSpanHandle{}}
}

func ContextWithRunObserver(ctx context.Context, observer *RunObserver) context.Context {
	if observer == nil {
		return ctx
	}
	return context.WithValue(ctx, runObserverKey{}, observer)
}

func RunObserverFromContext(ctx context.Context) *RunObserver {
	if ctx == nil {
		return nil
	}
	observer, _ := ctx.Value(runObserverKey{}).(*RunObserver)
	return observer
}

func (o *RunObserver) RecordLLMSpan(spanID string) {
	if o == nil || spanID == "" {
		return
	}
	o.mu.Lock()
	o.llmSpanID = spanID
	o.mu.Unlock()
}

func (o *RunObserver) RecordToolDecision(decision ToolDecision) {
	if o == nil || o.ledger == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	_ = o.ledger.RecordToolDecision(decision)
	o.pendingTools[o.toolKey(decision.ToolCallID, decision.ToolName)] = newTraceSpanHandle(o.ledger.ID(), o.ledger, o.parentSpanID(), "tool_call", map[string]any{
		"tool_name":           decision.ToolName,
		"tool_call_id":        decision.ToolCallID,
		"source":              decision.Source,
		"capability":          decision.Capability,
		"action":              decision.Action,
		"reason":              decision.Reason,
		"mutates_workspace":   decision.MutatesWorkspace,
		"requires_post_check": decision.RequiresPostCheck,
		"target":              decision.Target,
	})
}

func (o *RunObserver) RecordToolExecution(result ToolExecutionRecord) {
	if o == nil || o.ledger == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	_ = o.ledger.RecordToolExecution(result)
	key := o.toolKey(result.ToolCallID, result.ToolName)
	span := o.pendingTools[key]
	delete(o.pendingTools, key)
	if span == nil {
		span = newTraceSpanHandle(o.ledger.ID(), o.ledger, o.parentSpanID(), "tool_call", map[string]any{
			"tool_name":    result.ToolName,
			"tool_call_id": result.ToolCallID,
		})
	}
	status := result.Status
	if status == "" {
		status = "success"
	}
	span.Finish(status, map[string]any{
		"tool_name":       result.ToolName,
		"tool_call_id":    result.ToolCallID,
		"capability":      result.Capability,
		"original_bytes":  result.OriginalBytes,
		"returned_bytes":  result.ReturnedBytes,
		"truncated":       result.Truncated,
		"target":          result.Target,
		"idempotency_key": result.IdempotencyKey,
		"error":           result.Error,
		"recorded_at":     time.Now().UTC().Format(time.RFC3339Nano),
	})
}

func (o *RunObserver) RecordMutations(mutations []ToolMutation) {
	if o == nil || o.ledger == nil || len(mutations) == 0 {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	_ = o.ledger.RecordMutations(mutations)
}

func (o *RunObserver) RecordVerification(verification PostRunVerification) {
	if o == nil || o.ledger == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	_ = o.ledger.RecordVerification(verification)
}

func (o *RunObserver) toolKey(callID, name string) string {
	if callID != "" {
		return callID
	}
	return name
}

func (o *RunObserver) parentSpanID() string {
	if o.llmSpanID != "" {
		return o.llmSpanID
	}
	return o.rootSpanID
}
