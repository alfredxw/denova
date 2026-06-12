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
)

const (
	defaultRunTraceLimit     = 20
	maxRunTraceLimit         = 100
	defaultRunTraceRecordCap = 500
)

type RunTraceSummary struct {
	ID           string    `json:"id"`
	CreatedAt    time.Time `json:"created_at"`
	Path         string    `json:"path"`
	Status       string    `json:"status"`
	Reason       string    `json:"reason,omitempty"`
	Events       int       `json:"events"`
	ContextParts int       `json:"context_parts"`
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
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		var record RunTraceRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			continue
		}
		updateRunTraceSummary(&trace.Summary, record, path)
		if recordCap <= 0 || len(trace.Records) < recordCap {
			trace.Records = append(trace.Records, record)
		} else {
			trace.Truncated = true
		}
	}
	if err := scanner.Err(); err != nil {
		return RunTrace{}, err
	}
	if trace.Summary.ID == "" {
		trace.Summary.ID = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	if trace.Summary.Path == "" {
		trace.Summary.Path = path
	}
	return trace, nil
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
	case "event":
		summary.Events++
	case "context_ledger":
		summary.ContextParts += runTraceContextPartCount(record.Data)
	case "run_finished":
		if status, _ := record.Data["status"].(string); status != "" {
			summary.Status = status
		}
		if reason, _ := record.Data["reason"].(string); reason != "" {
			summary.Reason = reason
		}
	}
	if summary.Status == "" {
		summary.Status = "running"
	}
}

func runTraceContextPartCount(data map[string]any) int {
	parts, ok := data["parts"].([]any)
	if !ok {
		return 0
	}
	return len(parts)
}

func runTraceDir(workspace string) string {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return ""
	}
	return filepath.Join(workspace, filepath.FromSlash(defaultRunLedgerDirectory))
}
