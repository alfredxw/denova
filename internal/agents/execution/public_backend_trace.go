package execution

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	agentrun "denova/internal/agents/run"
	agenttool "denova/internal/agents/tool"

	agent "github.com/alfredxw/denova/agent"
)

// publicAgentRunTrace projects the public Agent lifecycle into Denova's
// bounded local trace. Public Agent events are the canonical source here: they
// cover every model/tool cycle without introducing a second execution ID.
type publicAgentRunTrace struct {
	mu             sync.Mutex
	runID          string
	ledger         *agentrun.Ledger
	rootSpan       *agentrun.Span
	observer       *agentrun.Observer
	openAttempted  bool
	finished       bool
	closed         bool
	modelCallCount int
	toolArgsBytes  map[string]int
}

func newPublicAgentRunTrace(runID string) *publicAgentRunTrace {
	return &publicAgentRunTrace{
		runID: strings.TrimSpace(runID), toolArgsBytes: make(map[string]int),
	}
}

func (registration *publicCycleRegistration) BindPublicRunTrace(ctx context.Context, runID string) context.Context {
	if registration == nil {
		return ctx
	}
	registration.mu.Lock()
	if registration.trace == nil {
		registration.trace = newPublicAgentRunTrace(runID)
	}
	trace := registration.trace
	options := registration.options
	registration.mu.Unlock()
	return trace.bindContext(ctx, options, runID)
}

func publicTraceForRun(registration *publicCycleRegistration, runID string) *publicAgentRunTrace {
	if registration == nil {
		return newPublicAgentRunTrace(runID)
	}
	registration.mu.Lock()
	if registration.trace == nil {
		registration.trace = newPublicAgentRunTrace(runID)
	}
	trace := registration.trace
	registration.mu.Unlock()
	trace.configure(runID)
	return trace
}

func (trace *publicAgentRunTrace) configure(runID string) {
	if trace == nil {
		return
	}
	trace.mu.Lock()
	defer trace.mu.Unlock()
	if trace.runID == "" {
		trace.runID = strings.TrimSpace(runID)
	}
}

func (trace *publicAgentRunTrace) bindContext(ctx context.Context, options agentrun.Options, runID string) context.Context {
	if trace == nil {
		return ctx
	}
	trace.mu.Lock()
	if trace.runID == "" {
		trace.runID = strings.TrimSpace(runID)
	}
	if err := trace.openLocked(options); err != nil {
		trace.mu.Unlock()
		slog.WarnContext(ctx, "[agent-public-runtime] bind run trace failed", "run_id", runID, "error", err)
		return ctx
	}
	ledger := trace.ledger
	rootSpan := trace.rootSpan
	observer := trace.observer
	trace.mu.Unlock()
	if ledger == nil || observer == nil {
		return ctx
	}
	parentSpanID := ""
	if rootSpan != nil {
		parentSpanID = rootSpan.SpanID()
	}
	ctx = agentrun.ContextWithRunTrace(ctx, ledger.ID(), ledger, parentSpanID)
	return agentrun.ContextWithObserver(ctx, observer)
}

func (trace *publicAgentRunTrace) record(registration *publicCycleRegistration, event agent.Event) error {
	if trace == nil {
		return nil
	}
	trace.mu.Lock()
	defer trace.mu.Unlock()
	if trace.finished || trace.closed {
		return nil
	}
	options := agentrun.Options{}
	if registration != nil {
		registration.mu.RLock()
		options = registration.options
		registration.mu.RUnlock()
	}
	if err := trace.openLocked(options); err != nil || trace.ledger == nil {
		return err
	}
	switch payload := event.Payload.(type) {
	case agent.RunAccepted:
		return trace.ledger.Record("run_started", map[string]any{
			"phase": "accepted", "command_id": payload.CommandID,
		})
	case agent.RunStarted:
		return trace.ledger.Record("agent_cycle", map[string]any{
			"phase": "started", "count": payload.Cycle,
		})
	case agent.ModelCompleted:
		trace.modelCallCount++
		if trace.observer != nil && trace.observer.LLMSpanCount() >= trace.modelCallCount {
			return nil
		}
		now := time.Now().UTC()
		cachedTokens := payload.Usage.PromptTokenDetails.CachedTokens
		uncachedTokens := payload.Usage.PromptTokens - cachedTokens
		if uncachedTokens < 0 {
			uncachedTokens = 0
		}
		return trace.ledger.RecordTraceSpan(agentrun.TraceSpanRecord{
			TraceID:   trace.runID,
			SpanID:    fmt.Sprintf("model-%d", trace.modelCallCount),
			Name:      "llm_call",
			Status:    "success",
			StartedAt: now,
			EndedAt:   now,
			Attrs: map[string]any{
				"source":                 payload.Source.Name,
				"finish_reason":          payload.FinishReason,
				"prompt_tokens":          payload.Usage.PromptTokens,
				"cached_prompt_tokens":   cachedTokens,
				"uncached_prompt_tokens": uncachedTokens,
				"completion_tokens":      payload.Usage.CompletionTokens,
				"reasoning_tokens":       payload.Usage.CompletionTokensDetails.ReasoningTokens,
				"total_tokens":           payload.Usage.TotalTokens,
			},
		})
	case agent.ToolStarted:
		if trace.observer != nil && trace.observer.HasToolDecision(payload.CallID, payload.Name) {
			return nil
		}
		argsComplete := true
		trace.toolArgsBytes[payload.CallID] = len(payload.Arguments)
		providerCallID := strings.TrimSpace(payload.ProviderCallID)
		if providerCallID == "" {
			providerCallID = payload.CallID
		}
		source := agenttool.ToolSourceOther
		descriptor := agent.ToolDescriptor{}
		if payload.Descriptor != nil {
			source = agenttool.ToolSource(payload.Descriptor.Source)
			descriptor = *payload.Descriptor
		}
		return trace.ledger.RecordToolDecision(agenttool.Decision{
			ToolName: payload.Name, ProviderCallID: providerCallID, ExecutionID: payload.CallID,
			Source: source, Capability: descriptor.Capability, Action: "allowed",
			MutationScope: descriptor.MutationScope, PostCheck: descriptor.PostCheck, Descriptor: descriptor,
			ArgsBytes: len(payload.Arguments), ArgsComplete: &argsComplete,
		})
	case agent.ToolFinished:
		if trace.observer != nil && trace.observer.HasToolExecution(payload.CallID, payload.Name) {
			return nil
		}
		return trace.recordToolFinished(payload)
	case agent.RunSettled:
		trace.finished = true
		status := publicTraceStatus(payload.Status)
		if trace.rootSpan != nil {
			trace.rootSpan.Finish(status, nil)
		}
		return trace.ledger.RecordFinish(status, payload.Reason, 0)
	default:
		return nil
	}
}

func (trace *publicAgentRunTrace) openLocked(options agentrun.Options) error {
	if trace == nil || trace.ledger != nil || trace.openAttempted || trace.runID == "" {
		return nil
	}
	trace.openAttempted = true
	ledger, err := agentrun.NewLedgerForRun(
		options.Workspace, agentrun.DefaultLoopPolicy().RunLedger, options, trace.runID,
	)
	if err != nil {
		return err
	}
	trace.ledger = ledger
	if ledger != nil {
		trace.rootSpan = agentrun.StartRootTraceSpan(ledger, map[string]any{
			"project_id": options.ProjectID, "agent_kind": options.AgentKind, "mode": options.Mode,
		})
		rootSpanID := ""
		if trace.rootSpan != nil {
			rootSpanID = trace.rootSpan.SpanID()
		}
		trace.observer = agentrun.NewObserverWithIdentity(
			ledger, rootSpanID, trace.runID, options.SessionID, options.ReviewThreadID,
		)
	}
	return nil
}

func (trace *publicAgentRunTrace) recordToolFinished(payload agent.ToolFinished) error {
	status := "success"
	providerCallID := strings.TrimSpace(payload.ProviderCallID)
	if providerCallID == "" {
		providerCallID = payload.CallID
	}
	record := agenttool.ExecutionRecord{
		ToolName: payload.Name, ProviderCallID: providerCallID, ExecutionID: payload.CallID,
	}
	if payload.Descriptor != nil {
		record.Descriptor = *payload.Descriptor
	}
	if argsBytes, ok := trace.toolArgsBytes[payload.CallID]; ok {
		argsComplete := true
		record.ArgsBytes = argsBytes
		record.ArgsComplete = &argsComplete
		delete(trace.toolArgsBytes, payload.CallID)
	}
	if payload.Projection != nil {
		projection := payload.Projection
		status = string(projection.Status)
		record.SyntheticReason = string(projection.SyntheticReason)
		record.OriginalBytes = projection.Metadata.OriginalModelBytes
		record.ReturnedBytes = projection.Metadata.ReturnedModelBytes
		record.Truncated = projection.Metadata.ModelTruncated || projection.Metadata.DisplayTruncated
		record.Target = projection.Metadata.Target
		record.IdempotencyKey = projection.Metadata.IdempotencyKey
	} else if payload.IsError {
		status = "error"
	}
	record.Status = status
	if status != "success" {
		// The local ledger classifies errors but deliberately excludes raw tool
		// output and provider diagnostics.
		record.Error = "tool execution failed"
	}
	return trace.ledger.RecordToolExecution(record)
}

func (trace *publicAgentRunTrace) close() error {
	if trace == nil {
		return nil
	}
	trace.mu.Lock()
	defer trace.mu.Unlock()
	if trace.ledger == nil || trace.closed {
		return nil
	}
	trace.closed = true
	if !trace.finished {
		if trace.rootSpan != nil {
			trace.rootSpan.Finish("error", nil)
		}
		if err := trace.ledger.RecordFinish("error", "public Agent event stream closed before settlement", 0); err != nil {
			_ = trace.ledger.Close()
			return err
		}
	}
	return trace.ledger.Close()
}

func publicTraceStatus(status agent.ResultStatus) string {
	switch status {
	case agent.ResultCompleted:
		return "success"
	case agent.ResultAborted:
		return "aborted"
	case agent.ResultBlocked:
		return "blocked"
	case agent.ResultFailed, agent.ResultIncomplete:
		return "error"
	default:
		return "error"
	}
}
