package agent

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"denova/internal/workspacepath"
)

// RunLedger is a durable JSONL trace for one Agent loop run.
// It records bounded metadata only, never full prompts, tool outputs, or thinking.
type RunLedger struct {
	mu          sync.Mutex
	id          string
	path        string
	previewChar int
	file        *os.File
}

type runLedgerRecord struct {
	Type      string         `json:"type"`
	RunID     string         `json:"run_id"`
	CreatedAt time.Time      `json:"created_at"`
	Data      map[string]any `json:"data,omitempty"`
}

type textSummary struct {
	Bytes   int    `json:"bytes"`
	Chars   int    `json:"chars"`
	Hash    string `json:"hash,omitempty"`
	Preview string `json:"preview"`
}

// TraceSink is the durable destination for structured Agent trace spans.
// The default implementation is the local run ledger; external exporters can
// adapt this interface without changing Agent execution.
type TraceSink interface {
	RecordTraceSpan(span TraceSpanRecord) error
}

func newRunLedger(workspace string, policy RunLedgerPolicy) (*RunLedger, error) {
	return newRunLedgerWithOptions(workspace, policy, RunOptions{})
}

func newRunLedgerWithOptions(workspace string, policy RunLedgerPolicy, options RunOptions) (*RunLedger, error) {
	traceCfg := traceRuntimeConfigSnapshot()
	if !policy.Enabled || traceCfg.CaptureLevel == TraceCaptureOff || strings.TrimSpace(workspace) == "" {
		return nil, nil
	}
	options = options.normalized(workspace)
	if policy.Directory == "" {
		policy.Directory = defaultRunLedgerDirectory
	}
	if policy.PreviewChars <= 0 {
		policy.PreviewChars = defaultRunLedgerPreviewChars
	}
	if traceCfg.CaptureLevel == TraceCaptureDebug && policy.PreviewChars < defaultDebugRunLedgerPreviewChars {
		policy.PreviewChars = defaultDebugRunLedgerPreviewChars
	}
	id := newRunLedgerID()
	dir := filepath.Join(workspace, filepath.FromSlash(policy.Directory))
	if policy.Directory == defaultRunLedgerDirectory {
		dir = workspacepath.Path(workspace, "runs")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create run ledger dir: %w", err)
	}
	path := filepath.Join(dir, id+".jsonl")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open run ledger: %w", err)
	}
	ledger := &RunLedger{id: id, path: path, previewChar: policy.PreviewChars, file: file}
	if err := ledger.Record("run_created", map[string]any{
		"path":             path,
		"task_id":          options.TaskID,
		"agent_kind":       options.AgentKind,
		"session_id":       options.SessionID,
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

func (l *RunLedger) ID() string {
	if l == nil {
		return ""
	}
	return l.id
}

func (l *RunLedger) Path() string {
	if l == nil {
		return ""
	}
	return l.path
}

func (l *RunLedger) RecordContext(parts []ContextLedgerPart) error {
	if l == nil {
		return nil
	}
	return l.Record("context_ledger", map[string]any{
		"parts": parts,
	})
}

func (l *RunLedger) RecordEvent(ev Event) error {
	if l == nil {
		return nil
	}
	if !shouldRecordRunLedgerEvent(ev.Type) {
		return nil
	}
	return l.Record("event", map[string]any{
		"event_type": ev.Type,
		"event_data": l.summarizeEventData(ev.Data),
	})
}

func (l *RunLedger) RecordToolDecision(decision ToolDecision) error {
	if l == nil {
		return nil
	}
	return l.Record("tool_decision", map[string]any{
		"decision": decision,
	})
}

func (l *RunLedger) RecordToolExecution(result ToolExecutionRecord) error {
	if l == nil {
		return nil
	}
	return l.Record("tool_execution", map[string]any{
		"result": result,
	})
}

func (l *RunLedger) RecordMutations(mutations []ToolMutation) error {
	if l == nil || len(mutations) == 0 {
		return nil
	}
	return l.Record("mutations", map[string]any{
		"mutations": mutations,
	})
}

func (l *RunLedger) RecordVerification(verification PostRunVerification) error {
	if l == nil {
		return nil
	}
	return l.Record("post_run_verification", map[string]any{
		"verification": verification,
	})
}

func (l *RunLedger) RecordTraceSpan(span TraceSpanRecord) error {
	if l == nil {
		return nil
	}
	span.Name = strings.TrimSpace(span.Name)
	if span.Name == "" {
		span.Name = "trace_span"
	}
	return l.Record(span.Name, l.traceSpanData(span))
}

func (l *RunLedger) RecordFinish(status, reason string, generatedBytes int) error {
	if l == nil {
		return nil
	}
	return l.Record("run_finished", map[string]any{
		"status":          strings.TrimSpace(status),
		"reason":          strings.TrimSpace(reason),
		"generated_bytes": generatedBytes,
	})
}

func (l *RunLedger) Record(recordType string, data map[string]any) error {
	if l == nil || l.file == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	record := runLedgerRecord{
		Type:      recordType,
		RunID:     l.id,
		CreatedAt: time.Now().UTC(),
		Data:      data,
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

func (l *RunLedger) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	err := l.file.Close()
	l.file = nil
	return err
}

func (l *RunLedger) summarizeEventData(data any) any {
	switch typed := data.(type) {
	case map[string]string:
		return l.summarizeStringMap(typed)
	case map[string]interface{}:
		out := make(map[string]any, len(typed))
		for key, value := range typed {
			out[key] = l.summarizeValue(key, value)
		}
		return out
	case string:
		return l.summarizeText(typed)
	default:
		var normalized any
		if encoded, err := json.Marshal(data); err == nil && json.Unmarshal(encoded, &normalized) == nil {
			return normalized
		}
		return fmt.Sprint(data)
	}
}

func (l *RunLedger) summarizeStringMap(values map[string]string) map[string]any {
	out := make(map[string]any, len(values))
	for key, value := range values {
		out[key] = l.summarizeValue(key, value)
	}
	return out
}

func (l *RunLedger) summarizeValue(key string, value any) any {
	switch typed := value.(type) {
	case string:
		if shouldSummarizeRunLedgerField(key) {
			return l.summarizeText(typed)
		}
		return typed
	default:
		return typed
	}
}

func (l *RunLedger) summarizeText(content string) textSummary {
	content = strings.TrimSpace(content)
	limit := l.previewChar
	if limit <= 0 {
		limit = defaultRunLedgerPreviewChars
	}
	sum := sha256.Sum256([]byte(content))
	return textSummary{
		Bytes:   len(content),
		Chars:   utf8.RuneCountInString(content),
		Hash:    fmt.Sprintf("sha256:%x", sum[:8]),
		Preview: safeLogPreview(content, limit),
	}
}

func (l *RunLedger) traceSpanData(span TraceSpanRecord) map[string]any {
	attrs := make(map[string]any, len(span.Attrs))
	for key, value := range span.Attrs {
		attrs[key] = l.summarizeTraceAttr(key, value)
	}
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
		data["error"] = l.summarizeText(span.Error)
	}
	return data
}

func (l *RunLedger) summarizeTraceAttr(key string, value any) any {
	switch typed := value.(type) {
	case string:
		if shouldSummarizeRunLedgerField(key) || shouldSummarizeTraceAttrKey(key) {
			return l.summarizeText(typed)
		}
		return typed
	case map[string]any:
		out := make(map[string]any, len(typed))
		for childKey, childValue := range typed {
			out[childKey] = l.summarizeTraceAttr(childKey, childValue)
		}
		return out
	default:
		return typed
	}
}

func shouldSummarizeRunLedgerField(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "content", "args", "delta", "message", "error", "result", "thinking":
		return true
	default:
		return false
	}
}

func shouldSummarizeTraceAttrKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	return strings.Contains(key, "prompt") ||
		strings.Contains(key, "content") ||
		strings.Contains(key, "message") ||
		strings.Contains(key, "args") ||
		strings.Contains(key, "result") ||
		strings.Contains(key, "thinking")
}

func shouldRecordRunLedgerEvent(eventType string) bool {
	switch strings.TrimSpace(eventType) {
	case "tool_call", "tool_target", "tool_result", "token_usage", "error", "aborted":
		return true
	default:
		return false
	}
}

func newRunLedgerID() string {
	var b [6]byte
	if _, err := rand.Read(b[:]); err == nil {
		return "run-" + time.Now().UTC().Format("20060102T150405.000000000") + "-" + hex.EncodeToString(b[:])
	}
	return fmt.Sprintf("run-%d", time.Now().UTC().UnixNano())
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
