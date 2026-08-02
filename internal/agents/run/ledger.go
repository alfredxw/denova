package agentrun

import (
	agentcontext "denova/internal/agents/context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	agenttool "denova/internal/agents/tool"
)

// Ledger is a durable JSONL trace for one Agent loop run.
// It records bounded metadata only, never full prompts, tool outputs, or thinking.
type Ledger struct {
	mu   sync.Mutex
	id   string
	path string
	file *os.File
}

type runLedgerRecord struct {
	Type      string         `json:"type"`
	RunID     string         `json:"run_id"`
	CreatedAt time.Time      `json:"created_at"`
	Data      map[string]any `json:"data,omitempty"`
}

type ContentMetrics struct {
	Bytes int `json:"bytes"`
	Chars int `json:"chars"`
}

// TraceSink is the durable destination for structured Agent trace spans.
// The default implementation is the local run ledger; external exporters can
// adapt this interface without changing Agent execution.
type TraceSink interface {
	RecordTraceSpan(span TraceSpanRecord) error
}

func NewLedger(workspace string, policy LedgerPolicy) (*Ledger, error) {
	return NewLedgerWithOptions(workspace, policy, Options{})
}

func NewLedgerWithOptions(workspace string, policy LedgerPolicy, options Options) (*Ledger, error) {
	traceCfg := traceRuntimeConfigSnapshot()
	if !policy.Enabled || traceCfg.CaptureLevel == TraceCaptureOff || strings.TrimSpace(workspace) == "" {
		return nil, nil
	}
	options = options.Normalize(workspace)
	defaults := DefaultLoopPolicy().RunLedger
	if policy.Directory == "" {
		policy.Directory = defaults.Directory
	}
	if policy.PreviewChars <= 0 {
		policy.PreviewChars = defaults.PreviewChars
	}
	if traceCfg.CaptureLevel == TraceCaptureDebug && policy.PreviewChars < defaultDebugRunLedgerPreviewChars {
		policy.PreviewChars = defaultDebugRunLedgerPreviewChars
	}
	id := NewID("run")
	dir := filepath.Join(workspace, filepath.FromSlash(policy.Directory))
	if policy.Directory == defaults.Directory {
		dir = primaryRunTraceDir(TraceLocation{Workspace: workspace, StateRoot: options.StateRoot})
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create run ledger dir: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, fmt.Errorf("secure run ledger dir: %w", err)
	}
	path := filepath.Join(dir, id+".jsonl")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open run ledger: %w", err)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("secure run ledger: %w", err)
	}
	ledger := &Ledger{id: id, path: path, file: file}
	if err := ledger.Record("run_created", map[string]any{
		"path":             path,
		"project_id":       options.ProjectID,
		"task_id":          options.TaskID,
		"agent_kind":       options.AgentKind,
		"session_id":       options.SessionID,
		"review_thread_id": options.ReviewThreadID,
		"story_id":         options.StoryID,
		"branch_id":        options.BranchID,
		"turn_id":          options.TurnID,
		"maintenance_task": options.MaintenanceTask,
		"workspace":        options.Workspace,
		"mode":             options.Mode,
	}); err != nil {
		_ = file.Close()
		return nil, err
	}
	pruneRunTraceFiles(dir, traceCfg.RetentionRuns, path)
	return ledger, nil
}

func (l *Ledger) ID() string {
	if l == nil {
		return ""
	}
	return l.id
}

func (l *Ledger) Path() string {
	if l == nil {
		return ""
	}
	return l.path
}

func (l *Ledger) RecordContext(parts []agentcontext.AuditPart) error {
	if l == nil {
		return nil
	}
	safeParts := make([]map[string]any, 0, len(parts))
	for _, part := range parts {
		safeParts = append(safeParts, map[string]any{
			"source":     strings.TrimSpace(part.Source),
			"purpose":    strings.TrimSpace(part.Purpose),
			"bytes":      part.Bytes,
			"chars":      part.Chars,
			"included":   part.Included,
			"truncated":  part.Truncated,
			"limit":      part.Limit,
			"limit_unit": strings.TrimSpace(part.LimitUnit),
		})
	}
	return l.writeRecord("context_ledger", map[string]any{
		"parts": safeParts,
	})
}

func (l *Ledger) RecordEvent(ev Event) error {
	if l == nil {
		return nil
	}
	data, ok := sanitizeRunLedgerEvent(ev)
	if !ok {
		return nil
	}
	return l.writeRecord("event", map[string]any{
		"event_type": ev.Type,
		"event_data": data,
	})
}

func (l *Ledger) RecordToolDecision(decision agenttool.Decision) error {
	if l == nil {
		return nil
	}
	safe := map[string]any{
		"tool_name":           decision.ToolName,
		"provider_call_id":    decision.ProviderCallID,
		"execution_id":        decision.ExecutionID,
		"source":              decision.Source,
		"capability":          decision.Capability,
		"action":              decision.Action,
		"mutation_scope":      decision.MutationScope,
		"post_check":          decision.PostCheck,
		"args_bytes":          decision.ArgsBytes,
		"args_complete":       decision.ArgsComplete,
		"model_finish_reason": decision.ModelFinishReason,
	}
	if reasonClass := ErrorClass(decision.Reason); reasonClass != "" {
		safe["reason_class"] = reasonClass
		if reasonClass == "invalid_arguments" {
			// Keep the stable compatibility marker consumed by RunTraceSummary;
			// never retain the provider/model supplied diagnostic itself.
			safe["reason"] = "Tool arguments must be a complete JSON object"
		}
	}
	return l.writeRecord("tool_decision", map[string]any{
		"decision": compactLedgerMap(safe),
	})
}

func (l *Ledger) RecordToolExecution(result agenttool.ExecutionRecord) error {
	if l == nil {
		return nil
	}
	safe := map[string]any{
		"tool_name":               result.ToolName,
		"provider_call_id":        result.ProviderCallID,
		"execution_id":            result.ExecutionID,
		"status":                  result.Status,
		"synthetic_reason":        result.SyntheticReason,
		"domain_status":           result.DomainStatus,
		"domain_diagnostic_count": result.DomainDiagnosticCount,
		"retry_modules":           result.RetryModules,
		"capability":              result.Capability,
		"original_bytes":          result.OriginalBytes,
		"returned_bytes":          result.ReturnedBytes,
		"truncated":               result.Truncated,
		"args_bytes":              result.ArgsBytes,
		"args_complete":           result.ArgsComplete,
		"model_finish_reason":     result.ModelFinishReason,
		"review_status":           result.ReviewStatus,
		"apply_state":             result.ApplyState,
		"mutation_receipt_schema": result.MutationReceiptSchema,
		"lore_item_count":         len(result.LoreItemIDs),
		"deleted_lore_item_count": len(result.DeletedLoreItemIDs),
	}
	if errorClass := ErrorClass(result.Error); errorClass != "" {
		safe["error_class"] = errorClass
	}
	return l.writeRecord("tool_execution", map[string]any{
		"result": compactLedgerMap(safe),
	})
}

func (l *Ledger) RecordMutations(mutations []agenttool.Mutation) error {
	if l == nil || len(mutations) == 0 {
		return nil
	}
	safe := make([]map[string]any, 0, len(mutations))
	for _, mutation := range mutations {
		safe = append(safe, compactLedgerMap(map[string]any{
			"tool_name":      mutation.ToolName,
			"source":         mutation.Source,
			"mutation_scope": mutation.MutationScope,
			"post_check":     mutation.PostCheck,
		}))
	}
	return l.writeRecord("mutations", map[string]any{
		"mutations": safe,
	})
}

func (l *Ledger) RecordVerification(verification agenttool.Verification) error {
	if l == nil {
		return nil
	}
	checks := make([]map[string]any, 0, len(verification.Checks))
	for _, check := range verification.Checks {
		checks = append(checks, compactLedgerMap(map[string]any{
			"type":   check.Type,
			"status": check.Status,
		}))
	}
	return l.writeRecord("post_run_verification", map[string]any{
		"verification": compactLedgerMap(map[string]any{
			"status":        verification.Status,
			"checks":        checks,
			"warning_count": len(verification.Warnings),
			"mutations":     verification.Mutations,
		}),
	})
}

func (l *Ledger) RecordTraceSpan(span TraceSpanRecord) error {
	if l == nil {
		return nil
	}
	span.Name = strings.TrimSpace(span.Name)
	if span.Name == "" {
		span.Name = "trace_span"
	}
	return l.writeRecord(span.Name, l.traceSpanData(span))
}

func (l *Ledger) RecordFinish(status, reason string, generatedBytes int) error {
	if l == nil {
		return nil
	}
	data := map[string]any{
		"status":          strings.TrimSpace(status),
		"generated_bytes": generatedBytes,
	}
	if reasonClass := ErrorClass(reason); reasonClass != "" {
		data["reason_class"] = reasonClass
	}
	return l.writeRecord("run_finished", data)
}

func (l *Ledger) Record(recordType string, data map[string]any) error {
	return l.writeRecord(recordType, sanitizeDirectRunLedgerRecord(recordType, data))
}

func (l *Ledger) writeRecord(recordType string, data map[string]any) error {
	if l == nil || l.file == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	record := runLedgerRecord{
		Type:      recordType,
		RunID:     l.id,
		CreatedAt: time.Now().UTC(),
		Data:      compactLedgerMap(data),
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if _, err := l.file.Write(append(encoded, '\n')); err != nil {
		return err
	}
	return nil
}

func (l *Ledger) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	err := l.file.Close()
	l.file = nil
	return err
}

func (l *Ledger) traceSpanData(span TraceSpanRecord) map[string]any {
	attrs := sanitizeTraceAttrs(span.Attrs)
	data := map[string]any{
		"trace_id":       strings.TrimSpace(span.TraceID),
		"span_id":        strings.TrimSpace(span.SpanID),
		"parent_span_id": strings.TrimSpace(span.ParentSpanID),
		"name":           strings.TrimSpace(span.Name),
		"status":         strings.TrimSpace(span.Status),
		"started_at":     span.StartedAt,
		"ended_at":       span.EndedAt,
		"duration_ms":    span.DurationMS,
		"attrs":          attrs,
	}
	if span.Error != "" {
		data["error_class"] = ErrorClass(span.Error)
	}
	return compactLedgerMap(data)
}

func shouldRecordRunLedgerEvent(eventType string) bool {
	switch strings.TrimSpace(eventType) {
	case "tool_call", "tool_target", "tool_result", "token_usage", "error", "aborted",
		"context_cleanup", "context_compaction", "context_normalizer":
		return true
	default:
		return false
	}
}

func sanitizeRunLedgerEvent(event Event) (map[string]any, bool) {
	eventType := strings.TrimSpace(event.Type)
	if !shouldRecordRunLedgerEvent(eventType) {
		return nil, false
	}
	data := normalizeLedgerData(event.Data)
	switch eventType {
	case "tool_call", "tool_target", "tool_result":
		keys := []string{
			"run_id", "agent_kind", "agent_name", "root_agent_name", "sub_agent_session_id",
			"id", "provider_call_id", "name", "index", "source", "mutation_scope", "post_check",
			"max_result_bytes", "status", "synthetic_reason", "model_truncated", "display_truncated",
			"tool_result_original_tokens", "tool_result_inline_tokens", "tool_result_retention_mode",
			"tool_result_context_value", "tool_result_recovery_kind", "artifact_count",
			"artifact_persist_attempted", "artifact_persist_complete", "artifact_persist_failure_count",
			"artifact_reread_count",
		}
		out := selectLedgerFields(data, keys...)
		if args, ok := data["args"].(string); ok {
			out["args_bytes"] = len(args)
		}
		if content, ok := data["content"].(string); ok {
			out["result_bytes"] = len(content)
		}
		if _, hasTarget := data["target"]; hasTarget {
			out["target_present"] = true
		}
		return compactLedgerMap(out), true
	case "token_usage":
		return selectLedgerFields(data,
			"created_at", "run_id", "agent_kind", "prompt_tokens", "cached_prompt_tokens",
			"uncached_prompt_tokens", "cache_hit_rate", "completion_tokens", "reasoning_tokens",
			"total_tokens", "model_calls", "generated_bytes", "usage_calls",
		), true
	case "context_cleanup":
		out := selectLedgerFields(data,
			"phase", "status", "action", "trigger_reason", "pressure_scope", "pressure", "full_pressure",
			"local_projected_tokens", "observed_prompt_tokens", "effective_tokens", "stable_prefix_tokens",
			"candidate_tokens", "cache_viable_candidate_tokens", "cleanup_skipped_below_minimum_count",
			"cleanup_skipped_warm_suffix_count", "eager_receipt_candidate_count", "eager_receipt_applied_count",
			"eager_receipt_fallback_count", "superseded_candidate_count", "discardable_candidate_count",
			"minimum_cleanup_tokens", "protected_result_count", "estimated_reclaimed_tokens",
			"actual_reclaimed_tokens", "projected_tokens_after", "pressure_after", "full_pressure_after",
			"earliest_changed_index", "warm_suffix_tokens", "placeholder_tokens", "replacement_count",
			"eager_only", "provider_cache_state", "cleanup_execution_mode", "placeholder_renderer_version",
		)
		appendLedgerErrorClass(out, data, "error")
		return compactLedgerMap(out), true
	case "context_compaction":
		if strings.EqualFold(strings.TrimSpace(stringLedgerField(data, "status")), "delta") {
			// Streaming checkpoint text is transient UI data, not durable telemetry.
			return nil, false
		}
		out := selectLedgerFields(data,
			"phase", "status", "attempt", "estimated_tokens_before", "observed_prompt_tokens",
			"observed_estimate_tokens", "tokens_before", "projected_tokens_before",
			"reserved_completion_tokens", "reserved_tool_result_tokens", "tokens_after",
			"projected_tokens_after",
			"context_window_tokens", "strategy", "threshold", "trigger_reason", "recovery_band",
			"recovery_target_tokens", "recovery_band_met", "degraded", "target_ratio", "epoch",
			"source_message_count", "message_count_before", "message_count_after", "skipped_reason",
			"execution_mode", "fallback_reason", "compaction_input_tokens", "compaction_prompt_tokens",
			"checkpoint_output_reserve", "safety_margin_tokens", "cache_expected_prefix_tokens",
			"cache_read_tokens", "cache_write_tokens", "cache_write_tokens_known", "cache_identity_status",
			"cache_usage_status", "cache_miss_reason", "cache_hit_ratio",
			"layer_count", "consecutive_failures", "failure_fuse_open",
		)
		appendLedgerErrorClass(out, data, "error")
		return compactLedgerMap(out), true
	case "context_normalizer":
		return selectLedgerFields(data, "status", "context_normalizer_repair_count"), true
	case "error", "aborted":
		out := selectLedgerFields(data, "run_id", "agent_kind", "phase")
		appendLedgerErrorClass(out, data, "message", "reason", "error")
		return compactLedgerMap(out), true
	default:
		return nil, false
	}
}

func sanitizeDirectRunLedgerRecord(recordType string, data map[string]any) map[string]any {
	switch strings.TrimSpace(recordType) {
	case "run_created", "run_started", "run_context":
		out := selectLedgerFields(data,
			"task_id", "agent_kind", "session_id", "review_thread_id", "story_id", "branch_id", "turn_id",
			"maintenance_task", "mode", "source", "references", "lore_references", "style_scenes", "selections",
			"plan_mode", "writing_skill", "message_bytes", "message_chars",
		)
		if metrics, ok := data["message"].(ContentMetrics); ok {
			out["message_bytes"] = metrics.Bytes
			out["message_chars"] = metrics.Chars
		}
		return compactLedgerMap(out)
	case "run_finished":
		out := selectLedgerFields(data, "status", "generated_bytes")
		appendLedgerErrorClass(out, data, "reason", "error")
		return compactLedgerMap(out)
	case "event":
		return selectLedgerFields(data, "event_type", "index")
	default:
		return selectLedgerFields(data, "status", "phase", "count", "duration_ms")
	}
}

func sanitizeTraceAttrs(attrs map[string]any) map[string]any {
	out := selectLedgerFields(attrs,
		"agent_kind", "source", "mode", "model", "call_id", "provider_request_id", "finish_reason",
		"attempt", "message_count", "tool_count", "history_messages", "context_parts", "message_chars",
		"agent_message_chars", "plan_mode", "writing_skill", "prompt_tokens", "cached_prompt_tokens",
		"uncached_prompt_tokens", "completion_tokens", "reasoning_tokens", "total_tokens", "generated_bytes",
		"tool_name", "execution_id", "provider_call_id", "capability", "action", "mutation_scope", "post_check",
		"args_bytes", "args_complete", "model_finish_reason", "original_bytes", "returned_bytes", "truncated",
		"domain_status", "domain_diagnostic_count", "retry_modules", "recorded_at", "status",
		"context_window_tokens", "input_tokens", "output_tokens", "cache_read_tokens", "cache_write_tokens",
		"cache_write_tokens_known", "cache_identity_status", "cache_usage_status", "cache_miss_reason",
		"reason_class", "phase", "epoch", "estimated_tokens_before",
		"observed_prompt_tokens", "observed_estimate_tokens", "tokens_before", "projected_tokens_before",
		"reserved_completion_tokens", "reserved_tool_result_tokens", "tokens_after", "projected_tokens_after",
		"threshold", "target_ratio", "recovery_band", "recovery_target_tokens", "recovery_band_met",
		"degraded", "source_message_count", "message_count_before", "message_count_after", "execution_mode",
		"compaction_input_tokens", "compaction_prompt_tokens", "checkpoint_output_reserve", "safety_margin_tokens",
		"cache_expected_prefix_tokens", "cache_hit_ratio", "layer_count", "consecutive_failures", "failure_fuse_open",
	)
	appendLedgerErrorClass(out, attrs, "error", "reason")
	return compactLedgerMap(out)
}

func normalizeLedgerData(data any) map[string]any {
	if values, ok := data.(map[string]any); ok {
		return values
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		return nil
	}
	var values map[string]any
	if err := json.Unmarshal(encoded, &values); err != nil {
		return nil
	}
	return values
}

func selectLedgerFields(data map[string]any, keys ...string) map[string]any {
	out := make(map[string]any, len(keys))
	for _, key := range keys {
		if value, ok := data[key]; ok && safeLedgerMetadataValue(value) {
			out[key] = value
		}
	}
	return compactLedgerMap(out)
}

func safeLedgerMetadataValue(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, json.Number:
		return true
	case string:
		return len(typed) <= 512
	case *bool:
		return typed != nil
	case []string:
		if len(typed) > 64 {
			return false
		}
		for _, item := range typed {
			if len(item) > 128 {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func compactLedgerMap(data map[string]any) map[string]any {
	for key, value := range data {
		switch typed := value.(type) {
		case nil:
			delete(data, key)
		case string:
			if strings.TrimSpace(typed) == "" {
				delete(data, key)
			}
		case int:
			if typed == 0 {
				delete(data, key)
			}
		case []string:
			if len(typed) == 0 {
				delete(data, key)
			}
		}
	}
	return data
}

func appendLedgerErrorClass(out, data map[string]any, keys ...string) {
	for _, key := range keys {
		value, _ := data[key].(string)
		if class := ErrorClass(value); class != "" {
			out["error_class"] = class
			return
		}
	}
}

func stringLedgerField(data map[string]any, key string) string {
	value, _ := data[key].(string)
	return value
}

// ErrorClass reduces an unrestricted diagnostic to a bounded telemetry class.
func ErrorClass(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	switch {
	case strings.Contains(value, "complete json"), strings.Contains(value, "invalid argument"),
		strings.Contains(value, "参数不是完整"), strings.Contains(value, "schema"), strings.Contains(value, "parse"):
		return "invalid_arguments"
	case strings.Contains(value, "cancel"), strings.Contains(value, "aborted"), strings.Contains(value, "中止"):
		return "canceled"
	case strings.Contains(value, "deadline"), strings.Contains(value, "timeout"), strings.Contains(value, "超时"):
		return "timeout"
	case strings.Contains(value, "permission"), strings.Contains(value, "denied"), strings.Contains(value, "unauthorized"), strings.Contains(value, "forbidden"):
		return "permission_denied"
	case strings.Contains(value, "not found"), strings.Contains(value, "no such"), strings.Contains(value, "不存在"):
		return "not_found"
	case strings.Contains(value, "conflict"):
		return "conflict"
	case strings.Contains(value, "panic"):
		return "panic"
	default:
		return "failure"
	}
}

func pruneRunTraceFiles(dir string, retention int, keepPath string) {
	if retention <= 0 {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	type traceFile struct {
		path    string
		modTime time.Time
	}
	files := make([]traceFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".jsonl" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, traceFile{path: path, modTime: info.ModTime()})
	}
	if len(files) <= retention {
		return
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.After(files[j].modTime)
	})
	keepPath = filepath.Clean(keepPath)
	kept := 0
	for _, file := range files {
		if kept < retention || filepath.Clean(file.path) == keepPath {
			kept++
			continue
		}
		_ = os.Remove(file.path)
	}
}
