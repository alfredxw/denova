package agent

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nova/internal/book"
)

func TestContextLedgerRecordsBoundedSources(t *testing.T) {
	ledger := NewContextLedger(ContextLedgerPolicy{Enabled: true, PreviewChars: 6})
	ledger.AddPart("文件引用", "@chapters/ch01.md", "user_reference", "第一章正文很长很长", "按单文件限制读取", true, true, 12)

	parts := ledger.Parts()
	if len(parts) != 1 {
		t.Fatalf("expected 1 context part, got %d", len(parts))
	}
	part := parts[0]
	if part.Source != "文件引用" || part.Title != "@chapters/ch01.md" || part.Purpose != "user_reference" {
		t.Fatalf("unexpected ledger part identity: %#v", part)
	}
	if part.Bytes == 0 || part.Chars == 0 || part.Preview == "" {
		t.Fatalf("ledger should record bounded size metadata: %#v", part)
	}
	if strings.Contains(part.Preview, "很长很长") {
		t.Fatalf("preview should be bounded, got %q", part.Preview)
	}
	if !part.Included || !part.Truncated || part.Limit != 12 {
		t.Fatalf("ledger should preserve inclusion and truncation metadata: %#v", part)
	}
}

func TestFilterToolResultAddsManifestAndBoundsOutput(t *testing.T) {
	content := strings.Repeat("章节正文", 4096)
	filtered := FilterToolResultForModel("write_file", `{"path":"chapters/ch00001.md"}`, content)
	if filtered.Manifest.Source != ToolSourceWrite || !filtered.Manifest.MutatesWorkspace || !filtered.Manifest.RequiresPostCheck {
		t.Fatalf("write_file should be classified as workspace mutation: %#v", filtered.Manifest)
	}
	if !filtered.Truncated {
		t.Fatalf("expected long result to be truncated")
	}
	if !strings.Contains(filtered.Content, "schema: tool_result.v1") ||
		!strings.Contains(filtered.Content, "mutates_workspace: true") ||
		!strings.Contains(filtered.Content, "target: chapters/ch00001.md") ||
		!strings.Contains(filtered.Content, "idempotency_key: write_file:") {
		t.Fatalf("filtered result should include model-visible metadata: %s", filtered.Content)
	}
	if len(filtered.Content) > writeToolResultMaxBytes+1024 {
		t.Fatalf("filtered result should stay bounded, got %d bytes", len(filtered.Content))
	}
}

func TestPostRunVerifierChecksLoreWriteResult(t *testing.T) {
	workspace := t.TempDir()
	store := book.NewLoreStore(workspace)
	item, err := store.Create(book.LoreItemInput{
		ID:         "hero",
		Type:       "character",
		Name:       "林川",
		Importance: "major",
		LoadMode:   book.LoreLoadModeResident,
		Content:    "林川是主角。",
	})
	if err != nil {
		t.Fatal(err)
	}
	result := VerifyPostRunMutations(book.NewService(workspace), []ToolMutation{{
		ToolName:    "write_lore_items",
		Source:      ToolSourceLore,
		LoreItemIDs: []string{item.ID},
	}})
	if result.Status != "ok" {
		t.Fatalf("created lore item should pass verification after default brief generation: %#v", result)
	}
	result = VerifyPostRunMutations(book.NewService(workspace), []ToolMutation{{
		ToolName:    "write_lore_items",
		Source:      ToolSourceLore,
		LoreItemIDs: []string{"missing-id"},
	}})
	if result.Status != "warning" {
		t.Fatalf("missing changed lore item should warn: %#v", result)
	}
}

func TestRunTraceReaderSummarizesLedger(t *testing.T) {
	workspace := t.TempDir()
	ledger, err := newRunLedger(workspace, RunLedgerPolicy{Enabled: true, Directory: ".nova/runs", PreviewChars: 8})
	if err != nil {
		t.Fatal(err)
	}
	if err := ledger.RecordContext([]ContextLedgerPart{{Source: "用户输入", Title: "请求", Included: true}}); err != nil {
		t.Fatal(err)
	}
	if err := ledger.RecordEvent(Event{Type: "verification", Data: PostRunVerification{Status: "ok", Mutations: 1}}); err != nil {
		t.Fatal(err)
	}
	if err := ledger.RecordFinish("success", "", 32); err != nil {
		t.Fatal(err)
	}
	if err := ledger.Close(); err != nil {
		t.Fatal(err)
	}
	summaries, err := ListRunTraces(workspace, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 1 || summaries[0].Status != "success" || summaries[0].Events != 1 || summaries[0].ContextParts != 1 {
		t.Fatalf("unexpected trace summary: %#v", summaries)
	}
	trace, err := ReadRunTrace(workspace, summaries[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(trace.Records) != 4 || trace.Summary.ID != summaries[0].ID {
		t.Fatalf("unexpected trace detail: %#v", trace)
	}
}

func TestLoopPolicyZeroValueUsesDefaults(t *testing.T) {
	policy := (LoopPolicy{}).normalized()
	if !policy.ContextLedger.Enabled || !policy.RunLedger.Enabled {
		t.Fatalf("zero loop policy should enable default ledgers: %#v", policy)
	}
	if policy.RunLedger.Directory != defaultRunLedgerDirectory {
		t.Fatalf("zero loop policy should use default run ledger directory: %#v", policy)
	}
}

func TestRunLedgerWritesBoundedJSONLTrace(t *testing.T) {
	workspace := t.TempDir()
	ledger, err := newRunLedger(workspace, RunLedgerPolicy{
		Enabled:      true,
		Directory:    ".nova/runs",
		PreviewChars: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	if ledger == nil {
		t.Fatal("expected run ledger")
	}
	if err := ledger.RecordContext([]ContextLedgerPart{{Source: "用户输入", Title: "本轮原始请求", Bytes: 12, Chars: 6, Preview: "写一章", Included: true}}); err != nil {
		t.Fatal(err)
	}
	if err := ledger.RecordEvent(Event{Type: "tool_result", Data: map[string]interface{}{
		"id":      "call-1",
		"name":    "read_file",
		"content": "这里是一段很长很长的工具返回内容，需要被截断保存",
	}}); err != nil {
		t.Fatal(err)
	}
	if err := ledger.RecordFinish("success", "", 128); err != nil {
		t.Fatal(err)
	}
	if err := ledger.Close(); err != nil {
		t.Fatal(err)
	}

	if !strings.HasPrefix(filepath.ToSlash(ledger.Path()), filepath.ToSlash(filepath.Join(workspace, ".nova/runs"))) {
		t.Fatalf("ledger path should be under workspace .nova/runs: %s", ledger.Path())
	}
	records := readRunLedgerRecords(t, ledger.Path())
	if len(records) != 4 {
		t.Fatalf("expected 4 ledger records, got %d: %#v", len(records), records)
	}
	if records[0]["type"] != "run_created" || records[1]["type"] != "context_ledger" || records[2]["type"] != "event" || records[3]["type"] != "run_finished" {
		t.Fatalf("unexpected record order: %#v", records)
	}

	eventData := records[2]["data"].(map[string]any)["event_data"].(map[string]any)
	content := eventData["content"].(map[string]any)
	if content["bytes"].(float64) == 0 || content["chars"].(float64) == 0 {
		t.Fatalf("content should be summarized with size metadata: %#v", content)
	}
	if strings.Contains(content["preview"].(string), "需要被截断保存") {
		t.Fatalf("tool result preview should be bounded: %#v", content)
	}
}

func readRunLedgerRecords(t *testing.T, path string) []map[string]any {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	var records []map[string]any
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("invalid ledger json %q: %v", line, err)
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return records
}
