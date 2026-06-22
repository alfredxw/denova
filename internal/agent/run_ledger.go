package agent

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
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
	Preview string `json:"preview"`
}

func newRunLedger(workspace string, policy RunLedgerPolicy) (*RunLedger, error) {
	return newRunLedgerWithOptions(workspace, policy, RunOptions{})
}

func newRunLedgerWithOptions(workspace string, policy RunLedgerPolicy, options RunOptions) (*RunLedger, error) {
	if !policy.Enabled || strings.TrimSpace(workspace) == "" {
		return nil, nil
	}
	options = options.normalized(workspace)
	if policy.Directory == "" {
		policy.Directory = defaultRunLedgerDirectory
	}
	if policy.PreviewChars <= 0 {
		policy.PreviewChars = defaultRunLedgerPreviewChars
	}
	id := newRunLedgerID()
	dir := filepath.Join(workspace, filepath.FromSlash(policy.Directory))
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
		"path":       path,
		"task_id":    options.TaskID,
		"agent_kind": options.AgentKind,
		"session_id": options.SessionID,
		"workspace":  options.Workspace,
		"mode":       options.Mode,
	}); err != nil {
		_ = file.Close()
		return nil, err
	}
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
	return textSummary{
		Bytes:   len(content),
		Chars:   utf8.RuneCountInString(content),
		Preview: safeLogPreview(content, limit),
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
