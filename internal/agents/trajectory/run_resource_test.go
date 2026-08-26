package trajectory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agentrun "denova/internal/agents/run"

	agenttools "github.com/alfredxw/denova/agent/tools"
)

func TestRunResourcePaginatesAndReconstructsLLMInputs(t *testing.T) {
	stateRoot := t.TempDir()
	workspace := filepath.Join(t.TempDir(), "workspace")
	runID := "run-projection"
	createdAt := time.Date(2026, time.August, 26, 2, 1, 13, 0, time.UTC)
	tools := []any{map[string]any{
		"name":       "read",
		"parameters": map[string]any{"type": "object"},
	}}
	firstMessages := []any{
		map[string]any{"role": "system", "content": "Inspect " + filepath.ToSlash(stateRoot) + "/runs safely."},
		map[string]any{"role": "user", "content": "Diagnose the run."},
	}
	secondMessages := append(append([]any{}, firstMessages...),
		map[string]any{"role": "assistant", "content": "I will inspect it."},
		map[string]any{"role": "tool", "content": "partial evidence"},
		map[string]any{"role": "user", "content": "Also check retries."},
	)
	resetMessages := []any{
		map[string]any{"role": "system", "content": "Use a compact context."},
		map[string]any{"role": "user", "content": "Continue diagnosis."},
	}
	writeTestRunTrace(t, stateRoot, runID, []agentrun.RunTraceRecord{
		testRunRecord(runID, createdAt, "run_created", map[string]any{"task_id": "task-1", "agent_kind": "harness"}),
		testLLMInputRecord(runID, createdAt.Add(time.Second), "call-1", firstMessages, tools),
		testLLMInputRecord(runID, createdAt.Add(2*time.Second), "call-2", secondMessages, tools),
		testLLMInputRecord(runID, createdAt.Add(3*time.Second), "call-3", resetMessages, tools),
		testRunRecord(runID, createdAt.Add(4*time.Second), "run_finished", map[string]any{"status": "success"}),
	})

	catalog, resource := testRunCatalog(stateRoot, workspace, runID)
	lines := readAllRunResourceLines(t, catalog, resource, 2)
	if len(lines) != 6 {
		t.Fatalf("line count = %d, want 6", len(lines))
	}
	var manifest runResourceManifest
	if err := json.Unmarshal([]byte(lines[0]), &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Type != "run_summary" || manifest.Schema != runResourceSchema || manifest.RecordCount != 5 {
		t.Fatalf("manifest = %#v", manifest)
	}
	if manifest.Summary.Status != "success" || manifest.Summary.Path != "" {
		t.Fatalf("summary = %#v", manifest.Summary)
	}
	joined := strings.Join(lines, "\n")
	for _, privatePath := range []string{stateRoot, filepath.ToSlash(stateRoot), workspace, filepath.ToSlash(workspace)} {
		if privatePath != "" && strings.Contains(strings.ToLower(joined), strings.ToLower(privatePath)) {
			t.Fatalf("projected trajectory leaked private path %q", privatePath)
		}
	}
	if !strings.Contains(joined, "[private-root]") {
		t.Fatal("projected trajectory did not retain the private path redaction marker")
	}
	if strings.Contains(lines[3], "Inspect [private-root]") {
		t.Fatal("append projection repeated unchanged system content")
	}

	modes := make([]string, 0, 3)
	var messages []any
	var definitions []any
	for _, line := range lines[1:] {
		var record agentrun.RunTraceRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatal(err)
		}
		if record.Type != "llm_input" {
			continue
		}
		projection, ok := record.Data["content"].(map[string]any)
		if !ok {
			t.Fatalf("llm_input content = %#v", record.Data["content"])
		}
		mode, _ := projection["mode"].(string)
		modes = append(modes, mode)
		switch mode {
		case "snapshot", "reset":
			messages, _ = projection["messages"].([]any)
			definitions, _ = projection["tools"].([]any)
		case "append":
			baseCount := int(projection["base_message_count"].(float64))
			if baseCount != len(messages) {
				t.Fatalf("append base_message_count = %d, current messages = %d", baseCount, len(messages))
			}
			added, _ := projection["added_messages"].([]any)
			messages = append(messages, added...)
		default:
			t.Fatalf("unexpected projection mode %q", mode)
		}
		metadata, ok := projection["metadata"].(map[string]any)
		if !ok {
			t.Fatalf("projection metadata = %#v", projection["metadata"])
		}
		reconstructed := make(map[string]any, len(metadata)+2)
		for key, value := range metadata {
			reconstructed[key] = value
		}
		reconstructed["messages"] = messages
		reconstructed["tools"] = definitions
		hash, err := hashJSON(reconstructed)
		if err != nil {
			t.Fatal(err)
		}
		if hash != projection["input_sha256"] {
			t.Fatalf("reconstructed input hash = %s, projection = %v", hash, projection["input_sha256"])
		}
	}
	if got := strings.Join(modes, ","); got != "snapshot,append,reset" {
		t.Fatalf("projection modes = %s", got)
	}
	if !strings.Contains(joined, "Also check retries.") {
		t.Fatal("append projection lost the steering user message")
	}
	if !strings.Contains(lines[4], "system_changed") || !strings.Contains(lines[4], "messages_not_append_only") {
		t.Fatalf("reset reasons missing from %s", lines[4])
	}
}

func TestRunResourceReadsBeyondUIRecordCap(t *testing.T) {
	stateRoot := t.TempDir()
	runID := "run-long-history"
	createdAt := time.Date(2026, time.August, 26, 3, 0, 0, 0, time.UTC)
	records := make([]agentrun.RunTraceRecord, 505)
	for index := range records {
		records[index] = testRunRecord(runID, createdAt.Add(time.Duration(index)*time.Millisecond), "event", map[string]any{"index": index})
	}
	writeTestRunTrace(t, stateRoot, runID, records)
	catalog, resource := testRunCatalog(stateRoot, "", runID)

	result, err := catalog.read(context.Background(), readInput{Path: resource, Offset: 502, Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 506 || result.Limit != 5 || result.Truncated || result.NextOffset != 0 {
		t.Fatalf("window = %#v", result)
	}
	if !strings.Contains(result.Content, `"index":500`) || !strings.Contains(result.Content, `"index":504`) {
		t.Fatalf("late trace records were not returned: %s", result.Content)
	}
}

func TestRunResourceSupportsExactLargeLineContinuation(t *testing.T) {
	stateRoot := t.TempDir()
	runID := "run-large-line"
	writeTestRunTrace(t, stateRoot, runID, []agentrun.RunTraceRecord{
		testRunRecord(runID, time.Date(2026, time.August, 26, 4, 0, 0, 0, time.UTC), "run_created", map[string]any{
			"payload": strings.Repeat("界", 16<<10),
		}),
	})
	catalog, resource := testRunCatalog(stateRoot, "", runID)
	adapter, err := NewReadAdapter(catalog)
	if err != nil {
		t.Fatal(err)
	}
	definition, err := agenttools.Read([]agenttools.ReadAdapter{adapter}, agenttools.WithMaxResultBytes(1024))
	if err != nil {
		t.Fatal(err)
	}
	first, err := definition.Tool.Run(context.Background(), fmt.Sprintf(`{"path":%q,"offset":2,"limit":1}`, resource))
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Limits struct {
			Offset         int    `json:"offset"`
			NextOffset     int    `json:"next_offset"`
			NextByteOffset int    `json:"next_byte_offset"`
			Unit           string `json:"unit"`
		} `json:"limits"`
	}
	if err := json.Unmarshal(first.Details, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Limits.Offset != 2 || envelope.Limits.NextOffset != 2 || envelope.Limits.NextByteOffset <= 0 || envelope.Limits.Unit != "lines" {
		t.Fatalf("large-line continuation = %#v", envelope.Limits)
	}
	full, err := catalog.read(context.Background(), readInput{Path: resource, Offset: 2, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	continued, err := catalog.read(context.Background(), readInput{
		Path: resource, Offset: 2, ByteOffset: envelope.Limits.NextByteOffset, Limit: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if continued.Content != full.Content[envelope.Limits.NextByteOffset:] {
		t.Fatal("byte_offset continuation did not return the exact UTF-8 suffix")
	}
}

func testRunCatalog(stateRoot, workspace, runID string) (Catalog, string) {
	source := Source{ProjectID: "project-1", Name: "Test Project", StateRoot: stateRoot, Workspace: workspace}
	return Catalog{Sources: func(context.Context) ([]Source, error) { return []Source{source}, nil }}, RunURI(source.ProjectID, runID)
}

func readAllRunResourceLines(t *testing.T, catalog Catalog, resource string, limit int) []string {
	t.Helper()
	lines := make([]string, 0)
	offset := 1
	for {
		result, err := catalog.read(context.Background(), readInput{Path: resource, Offset: offset, Limit: limit})
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(strings.TrimSuffix(result.Content, "\n"), "\n") {
			if line != "" {
				lines = append(lines, line)
			}
		}
		if !result.Truncated {
			return lines
		}
		if result.NextOffset <= offset {
			t.Fatalf("pagination did not advance: %#v", result)
		}
		offset = result.NextOffset
	}
}

func testLLMInputRecord(runID string, createdAt time.Time, callID string, messages, tools []any) agentrun.RunTraceRecord {
	return testRunRecord(runID, createdAt, "llm_input", map[string]any{
		"call_id": callID,
		"content": map[string]any{
			"source":        "agent model boundary",
			"purpose":       "developer trajectory inspection",
			"message_count": len(messages),
			"tool_count":    len(tools),
			"messages":      messages,
			"tools":         tools,
		},
	})
}

func testRunRecord(runID string, createdAt time.Time, recordType string, data map[string]any) agentrun.RunTraceRecord {
	return agentrun.RunTraceRecord{Type: recordType, RunID: runID, CreatedAt: createdAt, Data: data}
}

func writeTestRunTrace(t *testing.T, stateRoot, runID string, records []agentrun.RunTraceRecord) {
	t.Helper()
	directory := filepath.Join(stateRoot, "runs")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	var content bytes.Buffer
	encoder := json.NewEncoder(&content)
	encoder.SetEscapeHTML(false)
	for _, record := range records {
		if err := encoder.Encode(record); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(directory, runID+".jsonl"), content.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}
