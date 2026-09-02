package agentrun

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/providers"

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
	CaptureLevel   string
	Exporter       string
	RetentionRuns  int
	CaptureContent bool
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
	traceRuntimeConfigValue.CaptureLevel = normalizeTraceCaptureLevel(captureLevel)
	traceRuntimeConfigValue.Exporter = normalizeTraceExporter(exporter)
	traceRuntimeConfigValue.RetentionRuns = normalizeTraceRetentionRuns(retentionRuns)
}

// SetTraceContentCaptureEnabled controls developer-only capture of complete
// model-visible inputs and outputs. The local ledger is permission-restricted,
// but callers must still treat these records as potentially sensitive.
func SetTraceContentCaptureEnabled(enabled bool) {
	traceRuntimeMu.Lock()
	defer traceRuntimeMu.Unlock()
	traceRuntimeConfigValue.CaptureContent = enabled
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

// TraceContentRecord is an opt-in developer record tied to one trace span.
// Unlike TraceSpanRecord, Payload intentionally contains exact model-visible
// content so a trajectory can reconstruct prompts, tool exchanges, and response blocks.
type TraceContentRecord struct {
	TraceID string
	SpanID  string
	CallID  string
	Type    string
	Payload map[string]any
}

type Span struct {
	sink     TraceSink
	span     TraceSpanRecord
	observer *Observer
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

func WithStandaloneTrace(ctx context.Context, cfg *config.Config, agentKind, source, mode string, attrs map[string]any) (context.Context, func(error)) {
	if _, ok := agent.SessionKeyFromContext(ctx); !ok {
		ctx = agent.ContextWithSessionKey(ctx, StandaloneSessionKey(cfg, agentKind, source))
	}
	if traceContextFromContext(ctx).sink != nil {
		return ctx, func(error) {}
	}
	if cfg == nil || strings.TrimSpace(cfg.Workspace) == "" {
		return ctx, func(error) {}
	}
	options := Options{
		AgentKind: strings.TrimSpace(agentKind),
		ProjectID: cfg.ProjectID,
		StateRoot: cfg.ProjectStoreDir,
		Workspace: cfg.Workspace,
		Mode:      strings.TrimSpace(mode),
	}
	ledger, err := NewLedgerWithOptions(cfg.Workspace, DefaultLoopPolicy().RunLedger, options)
	if err != nil {
		slog.WarnContext(ctx, fmt.Sprintf("[agent-trace] standalone trace unavailable agent_kind=%s source=%s workspace=%s err=%v", agentKind, source, cfg.Workspace, err))
		return ctx, func(error) {}
	}
	if ledger == nil {
		return ctx, func(error) {}
	}
	rootAttrs := map[string]any{
		"project_id": cfg.ProjectID,
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
	traceCtx := ContextWithObserver(ContextWithRunTrace(ctx, ledger.ID(), ledger, rootSpanID), NewObserver(ledger, rootSpanID))
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
			slog.ErrorContext(ctx, fmt.Sprintf("[agent-trace] standalone trace close failed run_id=%s err=%v", ledger.ID(), err))
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

func StartRootTraceSpan(ledger *Ledger, attrs map[string]any) *Span {
	if ledger == nil || ledger.ID() == "" {
		return nil
	}
	return newSpan(ledger.ID(), ledger, "", "agent_run", attrs)
}

func StartTraceSpan(ctx context.Context, name string, attrs map[string]any) (*Span, context.Context) {
	tc := traceContextFromContext(ctx)
	if tc.sink == nil || tc.traceID == "" {
		return nil, ctx
	}
	span := newSpan(tc.traceID, tc.sink, tc.parentSpanID, name, attrs)
	if span == nil {
		return nil, ctx
	}
	span.observer = ObserverFromContext(ctx)
	return span, ContextWithRunTrace(ctx, tc.traceID, tc.sink, span.SpanID())
}

func RecordCompletedTraceSpan(ctx context.Context, name string, started time.Time, status string, attrs map[string]any) {
	tc := traceContextFromContext(ctx)
	if tc.sink == nil || tc.traceID == "" || started.IsZero() {
		return
	}
	handle := &Span{
		sink: tc.sink,
		span: TraceSpanRecord{
			TraceID:      tc.traceID,
			SpanID:       NewID("span"),
			ParentSpanID: tc.parentSpanID,
			Name:         strings.TrimSpace(name),
			StartedAt:    started.UTC(),
			Attrs:        cloneTraceAttrs(attrs),
		},
	}
	handle.Finish(status, nil)
}

func newSpan(traceID string, sink TraceSink, parentSpanID, name string, attrs map[string]any) *Span {
	if sink == nil || strings.TrimSpace(traceID) == "" {
		return nil
	}
	return &Span{
		sink: sink,
		span: TraceSpanRecord{
			TraceID:      strings.TrimSpace(traceID),
			SpanID:       NewID("span"),
			ParentSpanID: strings.TrimSpace(parentSpanID),
			Name:         strings.TrimSpace(name),
			StartedAt:    time.Now().UTC(),
			Attrs:        cloneTraceAttrs(attrs),
		},
	}
}

func (h *Span) SpanID() string {
	if h == nil {
		return ""
	}
	return h.span.SpanID
}

func (h *Span) Finish(status string, attrs map[string]any) {
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

func BeginLLMCallTrace(ctx context.Context, agentKind, source, mode string, cfg providers.ModelConfig, messages []*agent.Message, tools []*agent.ToolInfo, stream bool) (*Span, string, context.Context) {
	callID := newModelInputCallID()
	attrs := map[string]any{
		"call_id":       callID,
		"agent_kind":    strings.TrimSpace(agentKind),
		"source":        strings.TrimSpace(source),
		"mode":          strings.TrimSpace(mode),
		"provider":      strings.TrimSpace(string(cfg.Provider)),
		"protocol":      strings.TrimSpace(string(cfg.Protocol)),
		"model":         strings.TrimSpace(cfg.Model),
		"base_url":      strings.TrimSpace(cfg.BaseURL),
		"stream":        stream,
		"message_count": len(messages),
		"tool_count":    len(tools),
	}
	if cache := modelInputLogCacheAttribution(messages, modelInputLogTools(tools)); cache.MessageFingerprint != "" || cache.ToolSchemaFingerprint != "" {
		attrs["cache_attribution"] = cache
	}
	observer := ObserverFromContext(ctx)
	span, spanCtx := StartTraceSpan(ctx, "llm_call", attrs)
	if span == nil && observer != nil {
		span = &Span{observer: observer}
	}
	spanID := ""
	traceID := ""
	if span != nil && span.SpanID() != "" {
		spanID = span.SpanID()
		observer.RecordLLMSpan(spanID)
		traceID = traceContextFromContext(spanCtx).traceID
	}
	logFullModelInput(modelInputLogOptions{
		CallID:         callID,
		RunID:          traceID,
		SpanID:         spanID,
		AgentKind:      agentKind,
		Source:         source,
		Mode:           mode,
		Config:         cfg,
		Messages:       messages,
		Tools:          tools,
		CacheScope:     modelInputCacheScope(ctx, agentKind),
		SystemSections: modelInputSystemSectionsFromContext(ctx),
	})
	recordLLMInputTraceContent(span, callID, cfg, messages, tools)
	return span, callID, spanCtx
}

func FinishLLMCallTrace(span *Span, callID, agentKind, source, mode, modelName string, callIndex int, msg *agent.Message, err error, extra map[string]any) {
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
			logModelInputCacheResult(callID, runID, msg.ResponseMeta.Usage)
		}
		if tools := toolNamesFromCalls(msg.ToolCalls); len(tools) > 0 {
			attrs["requested_tools"] = tools
			outcome.RequestedTools = tools
		}
	}
	if span != nil && span.observer != nil && (outcome.FinishReason != "" || len(outcome.RequestedTools) > 0 || outcome.ProviderRequestID != "") {
		span.observer.RecordLLMOutcome(outcome)
	}
	recordLLMOutputTraceContent(span, callID, msg, err)
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

func modelInputCacheScope(ctx context.Context, agentKind string) string {
	sessionKey, ok := agent.SessionKeyFromContext(ctx)
	if !ok {
		return ""
	}
	return strings.Join([]string{
		strings.TrimSpace(sessionKey), strings.TrimSpace(agentKind),
	}, "\x00")
}

func addTokenUsageAttrs(attrs map[string]any, usage *agent.TokenUsage) {
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
