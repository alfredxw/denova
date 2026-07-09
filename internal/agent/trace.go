package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/schema"

	"denova/config"
)

const (
	TraceCaptureSummary = "summary"
	TraceCaptureDebug   = "debug"
	TraceCaptureOff     = "off"
	TraceExporterLocal  = "local"

	defaultTraceRetentionRuns         = 100
	defaultDebugRunLedgerPreviewChars = 1000
)

type traceRuntimeConfig struct {
	CaptureLevel  string
	Exporter      string
	RetentionRuns int
}

var (
	traceRuntimeMu          sync.RWMutex
	traceRuntimeConfigValue = traceRuntimeConfig{
		CaptureLevel:  TraceCaptureSummary,
		Exporter:      TraceExporterLocal,
		RetentionRuns: defaultTraceRetentionRuns,
	}
)

func SetTraceRuntimeConfig(captureLevel, exporter string, retentionRuns int) {
	traceRuntimeMu.Lock()
	defer traceRuntimeMu.Unlock()
	traceRuntimeConfigValue = traceRuntimeConfig{
		CaptureLevel:  normalizeTraceCaptureLevel(captureLevel),
		Exporter:      normalizeTraceExporter(exporter),
		RetentionRuns: normalizeTraceRetentionRuns(retentionRuns),
	}
}

func traceRuntimeConfigSnapshot() traceRuntimeConfig {
	traceRuntimeMu.RLock()
	defer traceRuntimeMu.RUnlock()
	return traceRuntimeConfigValue
}

func normalizeTraceCaptureLevel(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case TraceCaptureOff:
		return TraceCaptureOff
	case TraceCaptureDebug:
		return TraceCaptureDebug
	default:
		return TraceCaptureSummary
	}
}

func normalizeTraceExporter(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	default:
		return TraceExporterLocal
	}
}

func normalizeTraceRetentionRuns(value int) int {
	if value <= 0 {
		return defaultTraceRetentionRuns
	}
	return value
}

type traceContextKey struct{}

type traceContext struct {
	traceID      string
	parentSpanID string
	sink         TraceSink
}

// TraceSpanRecord is the structured span persisted into a run trace.
// It intentionally stores bounded attrs only; content-heavy values are
// summarized by the local sink before they reach disk.
type TraceSpanRecord struct {
	TraceID      string         `json:"trace_id"`
	SpanID       string         `json:"span_id"`
	ParentSpanID string         `json:"parent_span_id,omitempty"`
	Name         string         `json:"name"`
	Status       string         `json:"status"`
	StartedAt    time.Time      `json:"started_at"`
	EndedAt      time.Time      `json:"ended_at"`
	DurationMS   int64          `json:"duration_ms"`
	Attrs        map[string]any `json:"attrs,omitempty"`
	Error        string         `json:"error,omitempty"`
}

type traceSpanHandle struct {
	sink     TraceSink
	span     TraceSpanRecord
	observer *RunObserver
	once     sync.Once
}

func ContextWithRunTrace(ctx context.Context, traceID string, sink TraceSink, parentSpanID string) context.Context {
	if ctx == nil || sink == nil || strings.TrimSpace(traceID) == "" {
		return ctx
	}
	return context.WithValue(ctx, traceContextKey{}, traceContext{
		traceID:      strings.TrimSpace(traceID),
		parentSpanID: strings.TrimSpace(parentSpanID),
		sink:         sink,
	})
}

func withStandaloneRunTrace(ctx context.Context, cfg *config.Config, agentKind, source, mode string, attrs map[string]any) (context.Context, func(error)) {
	if traceContextFromContext(ctx).sink != nil {
		return ctx, func(error) {}
	}
	if cfg == nil || strings.TrimSpace(cfg.Workspace) == "" {
		return ctx, func(error) {}
	}
	options := RunOptions{
		AgentKind: strings.TrimSpace(agentKind),
		Workspace: cfg.Workspace,
		Mode:      strings.TrimSpace(mode),
	}
	ledger, err := newRunLedgerWithOptions(cfg.Workspace, DefaultLoopPolicy().RunLedger, options)
	if err != nil {
		log.Printf("[agent-trace] standalone trace unavailable agent_kind=%s source=%s workspace=%s err=%v", agentKind, source, cfg.Workspace, err)
		return ctx, func(error) {}
	}
	if ledger == nil {
		return ctx, func(error) {}
	}
	rootAttrs := map[string]any{
		"workspace":  cfg.Workspace,
		"agent_kind": options.AgentKind,
		"source":     strings.TrimSpace(source),
		"mode":       options.Mode,
	}
	for key, value := range attrs {
		rootAttrs[key] = value
	}
	root := StartRootTraceSpan(ledger, rootAttrs)
	rootSpanID := ""
	if root != nil {
		rootSpanID = root.SpanID()
	}
	traceCtx := ContextWithRunObserver(ContextWithRunTrace(ctx, ledger.ID(), ledger, rootSpanID), newRunObserver(ledger, rootSpanID))
	_ = ledger.Record("run_started", rootAttrs)
	finished := false
	return traceCtx, func(runErr error) {
		if finished {
			return
		}
		finished = true
		status := "success"
		reason := ""
		if runErr != nil {
			status = "error"
			reason = runErr.Error()
		}
		if root != nil {
			root.Finish(status, map[string]any{"reason": reason})
		}
		_ = ledger.RecordFinish(status, reason, 0)
		if err := ledger.Close(); err != nil {
			log.Printf("[agent-trace] standalone trace close failed run_id=%s err=%v", ledger.ID(), err)
		}
	}
}

func traceContextFromContext(ctx context.Context) traceContext {
	if ctx == nil {
		return traceContext{}
	}
	tc, _ := ctx.Value(traceContextKey{}).(traceContext)
	return tc
}

func StartRootTraceSpan(ledger *RunLedger, attrs map[string]any) *traceSpanHandle {
	if ledger == nil || ledger.ID() == "" {
		return nil
	}
	return newTraceSpanHandle(ledger.ID(), ledger, "", "agent_run", attrs)
}

func StartTraceSpan(ctx context.Context, name string, attrs map[string]any) (*traceSpanHandle, context.Context) {
	tc := traceContextFromContext(ctx)
	if tc.sink == nil || tc.traceID == "" {
		return nil, ctx
	}
	span := newTraceSpanHandle(tc.traceID, tc.sink, tc.parentSpanID, name, attrs)
	if span == nil {
		return nil, ctx
	}
	span.observer = RunObserverFromContext(ctx)
	return span, ContextWithRunTrace(ctx, tc.traceID, tc.sink, span.SpanID())
}

func RecordCompletedTraceSpan(ctx context.Context, name string, started time.Time, status string, attrs map[string]any) {
	tc := traceContextFromContext(ctx)
	if tc.sink == nil || tc.traceID == "" || started.IsZero() {
		return
	}
	handle := &traceSpanHandle{
		sink: tc.sink,
		span: TraceSpanRecord{
			TraceID:      tc.traceID,
			SpanID:       newTraceSpanID(),
			ParentSpanID: tc.parentSpanID,
			Name:         strings.TrimSpace(name),
			StartedAt:    started.UTC(),
			Attrs:        cloneTraceAttrs(attrs),
		},
	}
	handle.Finish(status, nil)
}

func newTraceSpanHandle(traceID string, sink TraceSink, parentSpanID, name string, attrs map[string]any) *traceSpanHandle {
	if sink == nil || strings.TrimSpace(traceID) == "" {
		return nil
	}
	return &traceSpanHandle{
		sink: sink,
		span: TraceSpanRecord{
			TraceID:      strings.TrimSpace(traceID),
			SpanID:       newTraceSpanID(),
			ParentSpanID: strings.TrimSpace(parentSpanID),
			Name:         strings.TrimSpace(name),
			StartedAt:    time.Now().UTC(),
			Attrs:        cloneTraceAttrs(attrs),
		},
	}
}

func (h *traceSpanHandle) SpanID() string {
	if h == nil {
		return ""
	}
	return h.span.SpanID
}

func (h *traceSpanHandle) Finish(status string, attrs map[string]any) {
	if h == nil || h.sink == nil {
		return
	}
	h.once.Do(func() {
		ended := time.Now().UTC()
		if h.span.StartedAt.IsZero() {
			h.span.StartedAt = ended
		}
		h.span.EndedAt = ended
		h.span.DurationMS = ended.Sub(h.span.StartedAt).Milliseconds()
		h.span.Status = normalizeTraceStatus(status)
		if h.span.Attrs == nil {
			h.span.Attrs = map[string]any{}
		}
		for key, value := range attrs {
			if key == "error" {
				if text, ok := value.(string); ok {
					h.span.Error = text
					continue
				}
			}
			h.span.Attrs[key] = value
		}
		_ = h.sink.RecordTraceSpan(h.span)
	})
}

func normalizeTraceStatus(status string) string {
	status = strings.TrimSpace(status)
	if status == "" {
		return "success"
	}
	return status
}

func cloneTraceAttrs(attrs map[string]any) map[string]any {
	if len(attrs) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(attrs))
	for key, value := range attrs {
		out[key] = value
	}
	return out
}

func newTraceSpanID() string {
	var b [6]byte
	if _, err := rand.Read(b[:]); err == nil {
		return "span-" + hex.EncodeToString(b[:])
	}
	return fmt.Sprintf("span-%d", time.Now().UTC().UnixNano())
}

func beginLLMCallTrace(ctx context.Context, agentKind, source, mode string, cfg openai.ChatModelConfig, messages []*schema.Message, tools []*schema.ToolInfo, stream bool) (*traceSpanHandle, string, context.Context) {
	callID := newModelInputCallID()
	attrs := map[string]any{
		"call_id":       callID,
		"agent_kind":    strings.TrimSpace(agentKind),
		"source":        strings.TrimSpace(source),
		"mode":          strings.TrimSpace(mode),
		"model":         strings.TrimSpace(cfg.Model),
		"base_url":      strings.TrimSpace(cfg.BaseURL),
		"stream":        stream,
		"message_count": len(messages),
		"tool_count":    len(tools),
	}
	if cache := modelInputLogCacheAttribution(messages, modelInputLogTools(tools)); cache.MessageFingerprint != "" || cache.ToolSchemaFingerprint != "" {
		attrs["cache_attribution"] = cache
	}
	observer := RunObserverFromContext(ctx)
	span, spanCtx := StartTraceSpan(ctx, "llm_call", attrs)
	if span == nil && observer != nil {
		span = &traceSpanHandle{observer: observer}
	}
	spanID := ""
	traceID := ""
	if span != nil && span.SpanID() != "" {
		spanID = span.SpanID()
		observer.RecordLLMSpan(spanID)
		traceID = traceContextFromContext(spanCtx).traceID
	}
	logFullModelInput(modelInputLogOptions{
		CallID:    callID,
		RunID:     traceID,
		SpanID:    spanID,
		AgentKind: agentKind,
		Source:    source,
		Mode:      mode,
		Config:    cfg,
		Messages:  messages,
		Tools:     tools,
	})
	return span, callID, spanCtx
}

func finishLLMCallTrace(span *traceSpanHandle, callID, agentKind, source, mode, modelName string, callIndex int, msg *schema.Message, err error, extra map[string]any) {
	attrs := cloneTraceAttrs(extra)
	runID := ""
	if span != nil {
		runID = span.span.TraceID
	}
	outcome := LLMOutcome{}
	if msg != nil {
		if requestID := logModelProviderRequestIDForCall(callID, agentKind, source, mode, modelName, runID, callIndex, msg); requestID != "" {
			attrs["provider_request_id"] = requestID
			outcome.ProviderRequestID = requestID
		}
		if msg.ResponseMeta != nil {
			outcome.FinishReason = strings.TrimSpace(msg.ResponseMeta.FinishReason)
			attrs["finish_reason"] = outcome.FinishReason
			addTokenUsageAttrs(attrs, msg.ResponseMeta.Usage)
		}
		if tools := toolNamesFromCalls(msg.ToolCalls); len(tools) > 0 {
			attrs["requested_tools"] = tools
			outcome.RequestedTools = tools
		}
	}
	if span != nil && span.observer != nil && (outcome.FinishReason != "" || len(outcome.RequestedTools) > 0 || outcome.ProviderRequestID != "") {
		span.observer.RecordLLMOutcome(outcome)
	}
	if err != nil {
		attrs["error"] = err.Error()
		if span != nil {
			span.Finish("error", attrs)
		}
		return
	}
	if span != nil {
		span.Finish("success", attrs)
	}
}

func addTokenUsageAttrs(attrs map[string]any, usage *schema.TokenUsage) {
	if attrs == nil || usage == nil {
		return
	}
	if usage.PromptTokens > 0 {
		attrs["prompt_tokens"] = usage.PromptTokens
		attrs["cached_prompt_tokens"] = usage.PromptTokenDetails.CachedTokens
		attrs["uncached_prompt_tokens"] = uncachedPromptTokens(usage.PromptTokens, usage.PromptTokenDetails.CachedTokens)
	}
	if usage.CompletionTokens > 0 {
		attrs["completion_tokens"] = usage.CompletionTokens
	}
	if usage.CompletionTokensDetails.ReasoningTokens > 0 {
		attrs["reasoning_tokens"] = usage.CompletionTokensDetails.ReasoningTokens
	}
	if usage.TotalTokens > 0 {
		attrs["total_tokens"] = usage.TotalTokens
	}
}

func durationMilliseconds(start, end time.Time) int64 {
	if start.IsZero() || end.IsZero() {
		return 0
	}
	return end.Sub(start).Milliseconds()
}
