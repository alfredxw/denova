package agentrun

import (
	"context"
	"denova/internal/agents/tool"
	"fmt"
	"strings"
	"sync"
	"time"
)

type runObserverKey struct{}

// LLMOutcome captures bounded metadata from the latest model response in one run.
type LLMOutcome struct {
	FinishReason      string
	RequestedTools    []string
	ProviderRequestID string
}

// Observer records bounded diagnostics for one Agent run without changing model-visible behavior.
type Observer struct {
	ledger         *Ledger
	runID          string
	sessionID      string
	reviewThreadID string
	rootSpanID     string
	llmSpanID      string
	lastLLMOutcome LLMOutcome
	pendingTools   map[string]*Span
	toolDecisions  map[string]struct{}
	toolResults    map[string]struct{}
	toolExecutions []agenttool.ExecutionRecord
	llmSpanCount   int
	mu             sync.Mutex
}

func NewObserver(ledger *Ledger, rootSpanID string) *Observer {
	runID := ""
	if ledger != nil {
		runID = strings.TrimSpace(ledger.ID())
	}
	return NewObserverWithIdentity(ledger, rootSpanID, runID, "", "")

}

func NewObserverWithIdentity(ledger *Ledger, rootSpanID, runID, sessionID, reviewThreadID string) *Observer {
	return &Observer{
		ledger:         ledger,
		runID:          strings.TrimSpace(runID),
		sessionID:      strings.TrimSpace(sessionID),
		reviewThreadID: strings.TrimSpace(reviewThreadID),
		rootSpanID:     rootSpanID,
		pendingTools:   map[string]*Span{},
		toolDecisions:  map[string]struct{}{},
		toolResults:    map[string]struct{}{},
	}
}

func ContextWithObserver(ctx context.Context, observer *Observer) context.Context {
	if observer == nil {
		return ctx
	}
	return context.WithValue(ctx, runObserverKey{}, observer)
}

func ObserverFromContext(ctx context.Context) *Observer {
	if ctx == nil {
		return nil
	}
	observer, _ := ctx.Value(runObserverKey{}).(*Observer)
	return observer
}

func (o *Observer) RecordLLMSpan(spanID string) {
	if o == nil || spanID == "" {
		return
	}
	o.mu.Lock()
	o.llmSpanID = spanID
	o.llmSpanCount++
	o.mu.Unlock()
}

// LLMSpanCount reports how many model calls were traced through the provider
// boundary. Lifecycle adapters use it to avoid adding a lower-fidelity
// duplicate when a custom Definition omits that middleware.
func (o *Observer) LLMSpanCount() int {
	if o == nil {
		return 0
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.llmSpanCount
}

func (o *Observer) RecordLLMOutcome(outcome LLMOutcome) {
	if o == nil {
		return
	}
	outcome.FinishReason = strings.TrimSpace(outcome.FinishReason)
	outcome.ProviderRequestID = strings.TrimSpace(outcome.ProviderRequestID)
	outcome.RequestedTools = append([]string(nil), outcome.RequestedTools...)
	o.mu.Lock()
	o.lastLLMOutcome = outcome
	o.mu.Unlock()
}

func (o *Observer) LastLLMOutcome() LLMOutcome {
	if o == nil {
		return LLMOutcome{}
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	outcome := o.lastLLMOutcome
	outcome.RequestedTools = append([]string(nil), outcome.RequestedTools...)
	return outcome
}

// RunID returns the stable run identity available to tools in this context.
// It is intentionally metadata-only; tools must not depend on the run ledger
// contents when applying workspace changes.
func (o *Observer) RunID() string {
	if o == nil {
		return ""
	}
	return o.runID
}

// SessionID identifies the user-visible conversation that owns this run.
func (o *Observer) SessionID() string {
	if o == nil {
		return ""
	}
	return o.sessionID
}

// ReviewThreadID links this run to a multi-run review without changing the
// run-scoped ChangeGroup/Undo boundary.
func (o *Observer) ReviewThreadID() string {
	if o == nil {
		return ""
	}
	return o.reviewThreadID
}

func (o *Observer) RecordToolDecision(decision agenttool.Decision) {
	if o == nil || o.ledger == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.toolDecisions[o.toolKey(decision.ExecutionID, decision.ToolName)] = struct{}{}
	_ = o.ledger.RecordToolDecision(decision)
	attrs := map[string]any{
		"tool_name":        decision.ToolName,
		"execution_id":     decision.ExecutionID,
		"provider_call_id": decision.ProviderCallID,
		"source":           decision.Source,
		"capability":       decision.Capability,
		"action":           decision.Action,
		"reason":           decision.Reason,
		"mutation_scope":   decision.MutationScope,
		"post_check":       decision.PostCheck,
		"target":           decision.Target,
	}
	if decision.ArgsBytes > 0 {
		attrs["args_bytes"] = decision.ArgsBytes
	}
	if decision.ArgsComplete != nil {
		attrs["args_complete"] = *decision.ArgsComplete
	}
	if decision.ModelFinishReason != "" {
		attrs["model_finish_reason"] = decision.ModelFinishReason
	}
	o.pendingTools[o.toolKey(decision.ExecutionID, decision.ToolName)] = newSpan(o.ledger.ID(), o.ledger, o.parentSpanID(), "tool_call", attrs)
}

// HasToolDecision and HasToolExecution distinguish rich middleware records
// from lifecycle fallbacks without exposing trace storage internals.
func (o *Observer) HasToolDecision(executionID, toolName string) bool {
	if o == nil {
		return false
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	_, ok := o.toolDecisions[o.toolKey(executionID, toolName)]
	return ok
}

func (o *Observer) HasToolExecution(executionID, toolName string) bool {
	if o == nil {
		return false
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	_, ok := o.toolResults[o.toolKey(executionID, toolName)]
	return ok
}

func (o *Observer) RecordToolExecution(result agenttool.ExecutionRecord) {
	if o == nil {
		return
	}
	result.RetryModules = append([]string(nil), result.RetryModules...)
	result.LoreItemIDs = append([]string(nil), result.LoreItemIDs...)
	result.DeletedLoreItemIDs = append([]string(nil), result.DeletedLoreItemIDs...)
	o.mu.Lock()
	defer o.mu.Unlock()
	o.toolResults[o.toolKey(result.ExecutionID, result.ToolName)] = struct{}{}
	o.toolExecutions = append(o.toolExecutions, result)
	if o.ledger == nil {
		return
	}
	_ = o.ledger.RecordToolExecution(result)
	key := o.toolKey(result.ExecutionID, result.ToolName)
	span := o.pendingTools[key]
	delete(o.pendingTools, key)
	if span == nil {
		span = newSpan(o.ledger.ID(), o.ledger, o.parentSpanID(), "tool_call", map[string]any{
			"tool_name":        result.ToolName,
			"execution_id":     result.ExecutionID,
			"provider_call_id": result.ProviderCallID,
		})
	}
	status := result.Status
	if status == "" {
		status = "success"
	}
	attrs := map[string]any{
		"tool_name":        result.ToolName,
		"execution_id":     result.ExecutionID,
		"provider_call_id": result.ProviderCallID,
		"capability":       result.Capability,
		"original_bytes":   result.OriginalBytes,
		"returned_bytes":   result.ReturnedBytes,
		"truncated":        result.Truncated,
		"target":           result.Target,
		"idempotency_key":  result.IdempotencyKey,
		"error":            result.Error,
		"recorded_at":      time.Now().UTC().Format(time.RFC3339Nano),
	}
	if result.DomainStatus != "" {
		attrs["domain_status"] = result.DomainStatus
		attrs["domain_diagnostic_count"] = result.DomainDiagnosticCount
		attrs["retry_modules"] = append([]string(nil), result.RetryModules...)
	}
	if result.Workspace != "" {
		attrs["workspace"] = result.Workspace
	}
	if result.ChangeGroupID != "" {
		attrs["change_group_id"] = result.ChangeGroupID
	}
	if result.ReviewThreadID != "" {
		attrs["review_thread_id"] = result.ReviewThreadID
	}
	if result.ChangeSetID != "" {
		attrs["change_set_id"] = result.ChangeSetID
	}
	if result.BaseRevision != "" {
		attrs["base_revision"] = result.BaseRevision
	}
	if result.Revision != "" {
		attrs["revision"] = result.Revision
	}
	if result.ArgsBytes > 0 {
		attrs["args_bytes"] = result.ArgsBytes
	}
	if result.ArgsComplete != nil {
		attrs["args_complete"] = *result.ArgsComplete
	}
	if result.ModelFinishReason != "" {
		attrs["model_finish_reason"] = result.ModelFinishReason
	}
	recordToolOutputTraceContent(span, result)
	span.Finish(status, attrs)
}

// ToolExecutions returns a caller-owned copy of completed tool diagnostics.
func (o *Observer) ToolExecutions() []agenttool.ExecutionRecord {
	if o == nil {
		return nil
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	result := make([]agenttool.ExecutionRecord, len(o.toolExecutions))
	for index, record := range o.toolExecutions {
		record.RetryModules = append([]string(nil), record.RetryModules...)
		record.LoreItemIDs = append([]string(nil), record.LoreItemIDs...)
		record.DeletedLoreItemIDs = append([]string(nil), record.DeletedLoreItemIDs...)
		result[index] = record
	}
	return result
}

// ResolvedMutations projects terminal tool executions into committed workspace
// mutations. No display or stream event participates in this decision.
func (o *Observer) ResolvedMutations() ([]agenttool.Mutation, []string) {
	if o == nil {
		return nil, nil
	}
	o.mu.Lock()
	records := append([]agenttool.ExecutionRecord(nil), o.toolExecutions...)
	o.mu.Unlock()

	mutations := make([]agenttool.Mutation, 0, len(records))
	warnings := make([]string, 0)
	seenCalls := make(map[string]struct{}, len(records))
	for index, record := range records {
		key := strings.TrimSpace(record.ExecutionID)
		if key == "" {
			key = fmt.Sprintf("%s:%d", agenttool.NormalizeName(record.ToolName), index)
		}
		if _, exists := seenCalls[key]; exists {
			continue
		}
		seenCalls[key] = struct{}{}
		resolution := agenttool.ResolveMutation(record)
		if resolution.Committed {
			mutations = append(mutations, resolution.Mutation)
		}
		if resolution.Warning != "" {
			warnings = append(warnings, resolution.Warning)
		}
	}
	return mutations, warnings
}

func (o *Observer) RecordMutations(mutations []agenttool.Mutation) {
	if o == nil || o.ledger == nil || len(mutations) == 0 {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	_ = o.ledger.RecordMutations(mutations)
}

func (o *Observer) RecordVerification(verification agenttool.Verification) {
	if o == nil || o.ledger == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	_ = o.ledger.RecordVerification(verification)
}

func (o *Observer) toolKey(callID, name string) string {
	if callID != "" {
		return callID
	}
	return name
}

func (o *Observer) parentSpanID() string {
	if o.llmSpanID != "" {
		return o.llmSpanID
	}
	return o.rootSpanID
}
