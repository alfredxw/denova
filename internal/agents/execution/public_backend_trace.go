package execution

import (
	"fmt"
	"strings"
	"time"

	agentrun "denova/internal/agents/run"
	agenttool "denova/internal/agents/tool"

	agent "github.com/alfredxw/denova/agent"
)

// publicAgentRunTrace projects the public Agent lifecycle into Denova's
// bounded local trace. Public Agent events are the canonical source here: they
// cover every model/tool cycle without introducing a second execution ID.
type publicAgentRunTrace struct {
	runID          string
	enabled        bool
	ledger         *agentrun.Ledger
	openAttempted  bool
	finished       bool
	modelCallCount int
	toolArgsBytes  map[string]int
}

func newPublicAgentRunTrace(runID string, enabled bool) *publicAgentRunTrace {
	return &publicAgentRunTrace{
		runID: strings.TrimSpace(runID), enabled: enabled, toolArgsBytes: make(map[string]int),
	}
}

func (trace *publicAgentRunTrace) record(registration *publicCycleRegistration, event agent.Event) error {
	if trace == nil || !trace.enabled || trace.finished {
		return nil
	}
	if err := trace.open(registration); err != nil || trace.ledger == nil {
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
		argsComplete := true
		trace.toolArgsBytes[payload.CallID] = len(payload.Arguments)
		return trace.ledger.RecordToolDecision(agenttool.Decision{
			ToolName: payload.Name, ProviderCallID: payload.CallID, ExecutionID: payload.CallID,
			Source: agenttool.ToolSourceOther, Action: "allowed",
			ArgsBytes: len(payload.Arguments), ArgsComplete: &argsComplete,
		})
	case agent.ToolFinished:
		return trace.recordToolFinished(payload)
	case agent.RunSettled:
		trace.finished = true
		return trace.ledger.RecordFinish(publicTraceStatus(payload.Status), payload.Reason, 0)
	default:
		return nil
	}
}

func (trace *publicAgentRunTrace) open(registration *publicCycleRegistration) error {
	if trace == nil || !trace.enabled || trace.ledger != nil || trace.openAttempted || registration == nil {
		return nil
	}
	registration.mu.RLock()
	options := registration.options
	registration.mu.RUnlock()
	trace.openAttempted = true
	ledger, err := agentrun.NewLedgerForRun(
		options.Workspace, agentrun.DefaultLoopPolicy().RunLedger, options, trace.runID,
	)
	if err != nil {
		return err
	}
	trace.ledger = ledger
	return nil
}

func (trace *publicAgentRunTrace) recordToolFinished(payload agent.ToolFinished) error {
	status := "success"
	record := agenttool.ExecutionRecord{
		ToolName: payload.Name, ProviderCallID: payload.CallID, ExecutionID: payload.CallID,
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
	if trace == nil || !trace.enabled || trace.ledger == nil {
		return nil
	}
	if !trace.finished {
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
