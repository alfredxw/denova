package agentrun

import (
	"context"
	agenttool "denova/internal/agents/tool"
	"fmt"
	"log/slog"
	"strings"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/providers"
)

const (
	traceContentSource  = "agent model boundary"
	traceToolSource     = "agent tool boundary"
	traceContentPurpose = "developer trajectory inspection"
)

// TraceContentSink is implemented by sinks that can securely persist exact
// developer content. Metadata-only exporters need not implement this contract.
type TraceContentSink interface {
	RecordTraceContent(record TraceContentRecord) error
}

func recordLLMInputTraceContent(span *Span, callID string, cfg providers.ModelConfig, messages []*agent.Message, tools []*agent.ToolInfo) {
	if span == nil || span.sink == nil || !traceRuntimeConfigSnapshot().CaptureContent {
		return
	}
	sink, ok := span.sink.(TraceContentSink)
	if !ok {
		return
	}
	writeTraceContent(sink, TraceContentRecord{
		TraceID: span.span.TraceID,
		SpanID:  span.span.SpanID,
		CallID:  strings.TrimSpace(callID),
		Type:    "llm_input",
		Payload: map[string]any{
			"source":        traceContentSource,
			"purpose":       traceContentPurpose,
			"model_config":  modelInputLogConfig(cfg),
			"message_count": len(messages),
			"tool_count":    len(tools),
			"messages":      messages,
			"tools":         modelInputLogTools(tools),
		},
	})
}

func recordLLMOutputTraceContent(span *Span, callID string, message *agent.Message, runErr error) {
	if span == nil || span.sink == nil || !traceRuntimeConfigSnapshot().CaptureContent {
		return
	}
	sink, ok := span.sink.(TraceContentSink)
	if !ok {
		return
	}
	payload := map[string]any{
		"source":  traceContentSource,
		"purpose": traceContentPurpose,
		"status":  "success",
		"message": message,
	}
	if runErr != nil {
		payload["status"] = "error"
		payload["error_class"] = ErrorClass(runErr.Error())
	}
	writeTraceContent(sink, TraceContentRecord{
		TraceID: span.span.TraceID,
		SpanID:  span.span.SpanID,
		CallID:  strings.TrimSpace(callID),
		Type:    "llm_output",
		Payload: payload,
	})
}

// recordToolOutputTraceContent supplements message-boundary captures for a
// terminal tool call whose result never becomes part of a later model input.
// The execution projection can be bounded, so the trace retains that fact
// alongside the developer-visible response instead of presenting it as exact.
func recordToolOutputTraceContent(span *Span, result agenttool.ExecutionRecord) {
	if span == nil || span.sink == nil || !traceRuntimeConfigSnapshot().CaptureContent {
		return
	}
	sink, ok := span.sink.(TraceContentSink)
	if !ok {
		return
	}
	callID := strings.TrimSpace(result.ProviderCallID)
	if callID == "" {
		callID = strings.TrimSpace(result.ExecutionID)
	}
	writeTraceContent(sink, TraceContentRecord{
		TraceID: span.span.TraceID,
		SpanID:  span.span.SpanID,
		CallID:  callID,
		Type:    "tool_output",
		Payload: map[string]any{
			"source":           traceToolSource,
			"purpose":          traceContentPurpose,
			"tool_name":        result.ToolName,
			"provider_call_id": result.ProviderCallID,
			"execution_id":     result.ExecutionID,
			"status":           result.Status,
			"result":           result.Result,
			"error":            result.Error,
			"original_bytes":   result.OriginalBytes,
			"returned_bytes":   result.ReturnedBytes,
			"truncated":        result.Truncated,
		},
	})
}

func writeTraceContent(sink TraceContentSink, record TraceContentRecord) {
	if err := sink.RecordTraceContent(record); err != nil {
		slog.ErrorContext(context.Background(), fmt.Sprintf(
			"[agent-trace] developer content write failed record_type=%s call_id=%s error_class=%s",
			record.Type,
			record.CallID,
			ErrorClass(err.Error()),
		))
	}
}
