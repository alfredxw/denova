package agent

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"denova/internal/workspacepath"
)

const (
	defaultRunTraceLimit     = 20
	maxRunTraceLimit         = 100
	defaultRunTraceRecordCap = 500
)

type RunTraceSummary struct {
	ID                    string    `json:"id"`
	CreatedAt             time.Time `json:"created_at"`
	Path                  string    `json:"path"`
	Status                string    `json:"status"`
	Reason                string    `json:"reason,omitempty"`
	Events                int       `json:"events"`
	ContextParts          int       `json:"context_parts"`
	TaskID                string    `json:"task_id,omitempty"`
	AgentKind             string    `json:"agent_kind,omitempty"`
	SessionID             string    `json:"session_id,omitempty"`
	StoryID               string    `json:"story_id,omitempty"`
	BranchID              string    `json:"branch_id,omitempty"`
	TurnID                string    `json:"turn_id,omitempty"`
	MaintenanceTask       string    `json:"maintenance_task,omitempty"`
	Phase                 string    `json:"phase,omitempty"`
	ToolCalls             int       `json:"tool_calls,omitempty"`
	ToolSuccesses         int       `json:"tool_successes,omitempty"`
	ToolBlocked           int       `json:"tool_blocked,omitempty"`
	ToolErrors            int       `json:"tool_errors,omitempty"`
	ToolTruncated         int       `json:"tool_truncated,omitempty"`
	InvalidToolArgs       int       `json:"invalid_tool_args,omitempty"`
	ToolDomainAccepted    int       `json:"tool_domain_accepted,omitempty"`
	ToolDomainRejected    int       `json:"tool_domain_rejected,omitempty"`
	ToolDomainPending     int       `json:"tool_domain_pending,omitempty"`
	ToolDomainDiagnostics int       `json:"tool_domain_diagnostics,omitempty"`
	LLMCalls              int       `json:"llm_calls,omitempty"`
	PromptTokens          int       `json:"prompt_tokens,omitempty"`
	CachedPromptTokens    int       `json:"cached_prompt_tokens,omitempty"`
	UncachedPromptTokens  int       `json:"uncached_prompt_tokens,omitempty"`
	CacheHitRate          float64   `json:"cache_hit_rate,omitempty"`
	DurationMS            int64     `json:"duration_ms,omitempty"`
	Mutations             int       `json:"mutations,omitempty"`
	VerificationStatus    string    `json:"verification_status,omitempty"`
	Recoverable           bool      `json:"recoverable,omitempty"`
}

type RunTrace struct {
	Summary   RunTraceSummary  `json:"summary"`
	Records   []RunTraceRecord `json:"records"`
	Truncated bool             `json:"truncated"`
}

type RunTraceRecord struct {
	Type      string         `json:"type"`
	RunID     string         `json:"run_id"`
	CreatedAt time.Time      `json:"created_at"`
	Data      map[string]any `json:"data,omitempty"`
}

func ListRunTraces(workspace string, limit int) ([]RunTraceSummary, error) {
	dir := runTraceDir(workspace)
	if dir == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = defaultRunTraceLimit
	}
	if limit > maxRunTraceLimit {
		limit = maxRunTraceLimit
	}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return []RunTraceSummary{}, nil
	}
	if err != nil {
		return nil, err
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".jsonl" {
			continue
		}
		files = append(files, filepath.Join(dir, entry.Name()))
	}
	sort.Slice(files, func(i, j int) bool {
		left, _ := os.Stat(files[i])
		right, _ := os.Stat(files[j])
		if left == nil || right == nil {
			return files[i] > files[j]
		}
		return left.ModTime().After(right.ModTime())
	})
	if len(files) > limit {
		files = files[:limit]
	}
	result := make([]RunTraceSummary, 0, len(files))
	for _, file := range files {
		trace, err := readRunTraceFile(file, defaultRunTraceRecordCap)
		if err != nil {
			continue
		}
		result = append(result, trace.Summary)
	}
	return result, nil
}

func ReadRunTrace(workspace, id string) (RunTrace, error) {
	id = strings.TrimSpace(id)
	if id == "" || strings.ContainsAny(id, `/\`) {
		return RunTrace{}, fmt.Errorf("invalid run id")
	}
	path := filepath.Join(runTraceDir(workspace), id+".jsonl")
	return readRunTraceFile(path, defaultRunTraceRecordCap)
}

func readRunTraceFile(path string, recordCap int) (RunTrace, error) {
	file, err := os.Open(path)
	if err != nil {
		return RunTrace{}, err
	}
	defer file.Close()
	trace := RunTrace{}
	var tail []RunTraceRecord
	totalRecords := 0
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		var record RunTraceRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			continue
		}
		totalRecords++
		updateRunTraceSummary(&trace.Summary, record, path)
		if recordCap <= 0 {
			trace.Records = append(trace.Records, record)
			continue
		}
		if trace.Truncated {
			tail = append(tail, record)
			if tailCap := traceTailRecordCap(recordCap); len(tail) > tailCap {
				tail = tail[len(tail)-tailCap:]
			}
			continue
		}
		if len(trace.Records) < recordCap {
			trace.Records = append(trace.Records, record)
			continue
		}
		trace.Truncated = true
		headCap := recordCap / 2
		tailCap := traceTailRecordCap(recordCap)
		tail = append(tail, trace.Records[headCap:]...)
		if len(tail) > tailCap {
			tail = tail[len(tail)-tailCap:]
		}
		trace.Records = trace.Records[:headCap]
		tail = append(tail, record)
		if tailCap := traceTailRecordCap(recordCap); len(tail) > tailCap {
			tail = tail[len(tail)-tailCap:]
		}
	}
	if err := scanner.Err(); err != nil {
		return RunTrace{}, err
	}
	if trace.Truncated {
		omitted := totalRecords - len(trace.Records) - len(tail)
		if omitted < 0 {
			omitted = 0
		}
		trace.Records = append(trace.Records, RunTraceRecord{
			Type:      "trace_truncated_gap",
			RunID:     trace.Summary.ID,
			CreatedAt: trace.Summary.CreatedAt,
			Data: map[string]any{
				"omitted_records": omitted,
				"record_cap":      recordCap,
			},
		})
		trace.Records = append(trace.Records, tail...)
	}
	if trace.Summary.ID == "" {
		trace.Summary.ID = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	if trace.Summary.Path == "" {
		trace.Summary.Path = path
	}
	return trace, nil
}

func traceTailRecordCap(recordCap int) int {
	if recordCap <= 2 {
		return 0
	}
	return recordCap - recordCap/2 - 1
}

func updateRunTraceSummary(summary *RunTraceSummary, record RunTraceRecord, path string) {
	if summary.ID == "" {
		summary.ID = record.RunID
	}
	if summary.CreatedAt.IsZero() || record.CreatedAt.Before(summary.CreatedAt) {
		summary.CreatedAt = record.CreatedAt
	}
	summary.Path = path
	switch record.Type {
	case "run_created":
		summary.TaskID = stringField(record.Data, "task_id")
		summary.AgentKind = stringField(record.Data, "agent_kind")
		summary.SessionID = stringField(record.Data, "session_id")
		summary.StoryID = stringField(record.Data, "story_id")
		summary.BranchID = stringField(record.Data, "branch_id")
		summary.TurnID = stringField(record.Data, "turn_id")
		summary.MaintenanceTask = stringField(record.Data, "maintenance_task")
		summary.Phase = "created"
	case "event":
		summary.Events++
	case "context_ledger":
		summary.ContextParts += runTraceContextPartCount(record.Data)
		summary.Phase = "context_ready"
	case "context_build":
		summary.Phase = "context_ready"
	case "run_context":
		if value := stringField(record.Data, "story_id"); value != "" {
			summary.StoryID = value
		}
		if value := stringField(record.Data, "branch_id"); value != "" {
			summary.BranchID = value
		}
		if value := stringField(record.Data, "turn_id"); value != "" {
			summary.TurnID = value
		}
		if value := stringField(record.Data, "maintenance_task"); value != "" {
			summary.MaintenanceTask = value
		}
	case "llm_call":
		summary.LLMCalls++
		runTraceAddLLMTokenUsage(summary, record.Data)
		summary.Phase = "model_running"
	case "tool_decision":
		summary.ToolCalls++
		if runTraceToolDecisionInvalidArgs(record.Data) {
			summary.InvalidToolArgs++
		}
		summary.Phase = "tool_running"
	case "tool_execution":
		status, truncated := runTraceToolExecutionStatus(record.Data)
		switch status {
		case "success":
			summary.ToolSuccesses++
		case "blocked":
			summary.ToolBlocked++
		case "error":
			summary.ToolErrors++
		}
		if truncated {
			summary.ToolTruncated++
		}
		domainStatus, diagnosticCount := runTraceToolExecutionDomain(record.Data)
		switch domainStatus {
		case "accepted":
			summary.ToolDomainAccepted++
		case "rejected":
			summary.ToolDomainRejected++
		case "pending":
			summary.ToolDomainPending++
		}
		summary.ToolDomainDiagnostics += diagnosticCount
	case "mutations":
		summary.Mutations += runTraceMutationCount(record.Data)
		summary.Phase = "verifying"
	case "post_run_verification":
		summary.VerificationStatus = runTraceVerificationStatus(record.Data)
		summary.Phase = "verified"
	case "run_finished":
		if status, _ := record.Data["status"].(string); status != "" {
			summary.Status = status
		}
		if reason, _ := record.Data["reason"].(string); reason != "" {
			summary.Reason = reason
		}
		summary.Phase = "finished"
	case "agent_run":
		if status, _ := record.Data["status"].(string); status != "" {
			summary.Status = status
		}
		if duration, ok := numericInt64Field(record.Data, "duration_ms"); ok {
			summary.DurationMS = duration
		}
	}
	if summary.Status == "" {
		summary.Status = "running"
	}
	if summary.Status == "running" {
		summary.Recoverable = true
	}
}

func runTraceAddLLMTokenUsage(summary *RunTraceSummary, data map[string]any) {
	if summary == nil {
		return
	}
	attrs := runTraceAttrs(data)
	prompt, _ := numericIntField(attrs, "prompt_tokens")
	cached, _ := numericIntField(attrs, "cached_prompt_tokens")
	uncached, hasUncached := numericIntField(attrs, "uncached_prompt_tokens")
	if prompt == 0 && cached == 0 && !hasUncached {
		prompt, _ = numericIntField(data, "prompt_tokens")
		cached, _ = numericIntField(data, "cached_prompt_tokens")
		uncached, hasUncached = numericIntField(data, "uncached_prompt_tokens")
	}
	if prompt <= 0 && cached <= 0 && uncached <= 0 {
		return
	}
	if !hasUncached && prompt > 0 {
		uncached = uncachedPromptTokens(prompt, cached)
	}
	summary.PromptTokens += prompt
	summary.CachedPromptTokens += cached
	summary.UncachedPromptTokens += uncached
	if summary.PromptTokens > 0 {
		summary.CacheHitRate = roundRatio(float64(summary.CachedPromptTokens) / float64(summary.PromptTokens))
	}
}

func runTraceAttrs(data map[string]any) map[string]any {
	if data == nil {
		return nil
	}
	attrs, _ := data["attrs"].(map[string]any)
	return attrs
}

func runTraceContextPartCount(data map[string]any) int {
	parts, ok := data["parts"].([]any)
	if !ok {
		return 0
	}
	return len(parts)
}

func runTraceMutationCount(data map[string]any) int {
	mutations, ok := data["mutations"].([]any)
	if !ok {
		return 0
	}
	return len(mutations)
}

func runTraceToolDecisionInvalidArgs(data map[string]any) bool {
	decision, ok := data["decision"].(map[string]any)
	if !ok {
		return false
	}
	reason := stringField(decision, "reason")
	return strings.Contains(reason, "参数不是完整 JSON 对象") ||
		strings.Contains(reason, "Tool arguments must be a complete JSON object")
}

func runTraceToolExecutionStatus(data map[string]any) (string, bool) {
	result, ok := data["result"].(map[string]any)
	if !ok {
		return "", false
	}
	truncated, _ := result["truncated"].(bool)
	return stringField(result, "status"), truncated
}

func runTraceToolExecutionDomain(data map[string]any) (string, int) {
	result, ok := data["result"].(map[string]any)
	if !ok {
		return "", 0
	}
	diagnostics, _ := numericIntField(result, "domain_diagnostic_count")
	return stringField(result, "domain_status"), diagnostics
}

func runTraceVerificationStatus(data map[string]any) string {
	verification, ok := data["verification"].(map[string]any)
	if !ok {
		return ""
	}
	return stringField(verification, "status")
}

func numericInt64Field(data map[string]any, key string) (int64, bool) {
	if data == nil {
		return 0, false
	}
	switch value := data[key].(type) {
	case int:
		return int64(value), true
	case int64:
		return value, true
	case float64:
		return int64(value), true
	default:
		return 0, false
	}
}

func numericIntField(data map[string]any, key string) (int, bool) {
	value, ok := numericInt64Field(data, key)
	return int(value), ok
}

func stringField(data map[string]any, key string) string {
	if data == nil {
		return ""
	}
	value, _ := data[key].(string)
	return value
}

func runTraceDir(workspace string) string {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return ""
	}
	return workspacepath.Path(workspace, "runs")
}
