package agentrun

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/providers"
)

const (
	traceContentSource  = "agent model boundary"
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
