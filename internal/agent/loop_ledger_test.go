package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"denova/internal/book"
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

func TestFilterToolResultAddsManifestWithoutDefaultTruncation(t *testing.T) {
	content := strings.Repeat("章节正文", 4096)
	filtered := FilterToolResultForModel("write_file", `{"path":"chapters/ch00001.md"}`, content)
	if filtered.Manifest.Source != ToolSourceWrite || !filtered.Manifest.MutatesWorkspace || !filtered.Manifest.RequiresPostCheck {
		t.Fatalf("write_file should be classified as workspace mutation: %#v", filtered.Manifest)
	}
	if filtered.Manifest.Capability != "file_write" {
		t.Fatalf("write_file capability = %q, want file_write", filtered.Manifest.Capability)
	}
	if filtered.Truncated {
		t.Fatalf("default tool result filtering should not truncate")
	}
	if !strings.Contains(filtered.Content, "schema: tool_result.v1") ||
		!strings.Contains(filtered.Content, "mutates_workspace: true") ||
		!strings.Contains(filtered.Content, "capability: file_write") ||
		!strings.Contains(filtered.Content, "target: chapters/ch00001.md") ||
		!strings.Contains(filtered.Content, "idempotency_key: write_file:") {
		t.Fatalf("filtered result should include model-visible metadata: %s", filtered.Content)
	}
	if !strings.Contains(filtered.Content, content) {
		t.Fatalf("filtered result should include full content by default")
	}
}

func TestFilterToolResultBoundsOutputWhenLimitConfigured(t *testing.T) {
	content := strings.Repeat("章节正文", 4096)
	filtered := FilterToolResultForModelWithLimit("write_file", `{"path":"chapters/ch00001.md"}`, content, 8*1024)
	if !filtered.Truncated {
		t.Fatalf("expected long result to be truncated when limit is configured")
	}
	if !strings.Contains(filtered.Content, "[tool result truncated]") ||
		!strings.Contains(filtered.Content, "truncated: true") {
		t.Fatalf("filtered result should include truncation markers: %s", filtered.Content)
	}
	if len(filtered.Content) > 8*1024+1024 {
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
	ledger, err := newRunLedgerWithOptions(workspace, RunLedgerPolicy{Enabled: true, Directory: ".denova/runs", PreviewChars: 8}, RunOptions{
		AgentKind: AgentKindIDE,
		TaskID:    "task-1",
		SessionID: "session-1",
		Workspace: workspace,
		Mode:      "ide",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := ledger.RecordContext([]ContextLedgerPart{{Source: "用户输入", Title: "请求", Included: true}}); err != nil {
		t.Fatal(err)
	}
	if err := ledger.RecordEvent(Event{Type: "tool_result", Data: map[string]interface{}{
		"id":      "call-1",
		"name":    "write_file",
		"content": "写入成功",
	}}); err != nil {
		t.Fatal(err)
	}
	if err := ledger.RecordToolDecision(ToolDecision{
		ToolName:   "write_file",
		ToolCallID: "call-1",
		Source:     ToolSourceWrite,
		Capability: "file_write",
		Action:     "allowed",
		Target:     "chapters/ch01.md",
	}); err != nil {
		t.Fatal(err)
	}
	if err := ledger.RecordToolExecution(ToolExecutionRecord{
		ToolName:      "write_file",
		ToolCallID:    "call-1",
		Status:        "success",
		Capability:    "file_write",
		OriginalBytes: 64,
		ReturnedBytes: 48,
		Truncated:     true,
		Target:        "chapters/ch01.md",
	}); err != nil {
		t.Fatal(err)
	}
	if err := ledger.RecordToolDecision(ToolDecision{
		ToolName:   "write_file",
		ToolCallID: "call-2",
		Source:     ToolSourceWrite,
		Capability: "file_write",
		Action:     "blocked",
		Reason:     "参数不是完整 JSON 对象",
	}); err != nil {
		t.Fatal(err)
	}
	if err := ledger.RecordToolExecution(ToolExecutionRecord{
		ToolName:   "write_file",
		ToolCallID: "call-2",
		Status:     "blocked",
		Capability: "file_write",
		Error:      "参数不是完整 JSON 对象",
	}); err != nil {
		t.Fatal(err)
	}
	if err := ledger.RecordMutations([]ToolMutation{{ToolName: "write_file", Source: ToolSourceWrite, Target: "chapters/ch01.md"}}); err != nil {
		t.Fatal(err)
	}
	if err := ledger.RecordVerification(PostRunVerification{Status: "ok", Mutations: 1}); err != nil {
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
	if summaries[0].AgentKind != AgentKindIDE || summaries[0].TaskID != "task-1" || summaries[0].SessionID != "session-1" || summaries[0].Mutations != 1 || summaries[0].VerificationStatus != "ok" {
		t.Fatalf("trace summary should include durable run state: %#v", summaries[0])
	}
	if summaries[0].ToolCalls != 2 || summaries[0].ToolSuccesses != 1 || summaries[0].ToolBlocked != 1 || summaries[0].ToolTruncated != 1 || summaries[0].InvalidToolArgs != 1 {
		t.Fatalf("trace summary should include tool quality counters: %#v", summaries[0])
	}
	trace, err := ReadRunTrace(workspace, summaries[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(trace.Records) != 10 || trace.Summary.ID != summaries[0].ID {
		t.Fatalf("unexpected trace detail: %#v", trace)
	}
}

func TestRunLedgerRecordsStructuredTraceSpans(t *testing.T) {
	oldTraceConfig := traceRuntimeConfigSnapshot()
	SetTraceRuntimeConfig(TraceCaptureSummary, TraceExporterLocal, 100)
	t.Cleanup(func() {
		SetTraceRuntimeConfig(oldTraceConfig.CaptureLevel, oldTraceConfig.Exporter, oldTraceConfig.RetentionRuns)
	})

	workspace := t.TempDir()
	ledger, err := newRunLedgerWithOptions(workspace, RunLedgerPolicy{Enabled: true, Directory: ".denova/runs", PreviewChars: 8}, RunOptions{
		AgentKind: AgentKindIDE,
		TaskID:    "task-structured-trace",
		Workspace: workspace,
		Mode:      "ide",
	})
	if err != nil {
		t.Fatal(err)
	}
	root := StartRootTraceSpan(ledger, map[string]any{"agent_kind": AgentKindIDE})
	if root == nil || root.SpanID() == "" {
		t.Fatal("expected root trace span")
	}
	ctx := ContextWithRunObserver(ContextWithRunTrace(context.Background(), ledger.ID(), ledger, root.SpanID()), newRunObserver(ledger, root.SpanID()))
	llm, _ := StartTraceSpan(ctx, "llm_call", map[string]any{
		"call_id":    "call-1",
		"model":      "test-model",
		"mode":       "generate",
		"prompt":     strings.Repeat("secret prompt ", 20),
		"tool_count": 1,
	})
	if llm == nil || llm.SpanID() == "" {
		t.Fatal("expected llm trace span")
	}
	RunObserverFromContext(ctx).RecordLLMSpan(llm.SpanID())
	llm.Finish("success", map[string]any{
		"provider_request_id":  "provider-1",
		"finish_reason":        "tool_calls",
		"prompt_tokens":        12,
		"cached_prompt_tokens": 4,
		"completion_tokens":    6,
		"total_tokens":         18,
	})
	RunObserverFromContext(ctx).RecordToolDecision(ToolDecision{
		ToolName:   "read_file",
		ToolCallID: "tool-1",
		Source:     ToolSourceRead,
		Capability: "file_read",
		Action:     "allowed",
		Target:     "chapters/ch01.md",
	})
	RunObserverFromContext(ctx).RecordToolExecution(ToolExecutionRecord{
		ToolName:      "read_file",
		ToolCallID:    "tool-1",
		Status:        "success",
		Capability:    "file_read",
		OriginalBytes: 4096,
		ReturnedBytes: 512,
		Truncated:     true,
		Target:        "chapters/ch01.md",
	})
	root.Finish("success", map[string]any{"generated_bytes": 32})
	if err := ledger.Close(); err != nil {
		t.Fatal(err)
	}

	trace, err := ReadRunTrace(workspace, ledger.ID())
	if err != nil {
		t.Fatal(err)
	}
	if trace.Summary.LLMCalls != 1 {
		t.Fatalf("expected one llm call in summary: %#v", trace.Summary)
	}
	var rootData, llmData, toolData map[string]any
	for _, record := range readRunLedgerRecords(t, ledger.Path()) {
		data, _ := record["data"].(map[string]any)
		switch record["type"] {
		case "agent_run":
			rootData = data
		case "llm_call":
			llmData = data
		case "tool_call":
			toolData = data
		}
	}
	if rootData == nil || llmData == nil || toolData == nil {
		t.Fatalf("expected root, llm, and tool span records: root=%#v llm=%#v tool=%#v", rootData, llmData, toolData)
	}
	if llmData["parent_span_id"] != rootData["span_id"] {
		t.Fatalf("llm parent span mismatch: llm=%#v root=%#v", llmData, rootData)
	}
	if toolData["parent_span_id"] != llmData["span_id"] {
		t.Fatalf("tool parent span should point at llm span: tool=%#v llm=%#v", toolData, llmData)
	}
	llmAttrs := llmData["attrs"].(map[string]any)
	if llmAttrs["provider_request_id"] != "provider-1" || llmAttrs["total_tokens"].(float64) != 18 {
		t.Fatalf("llm attrs should include provider id and tokens: %#v", llmAttrs)
	}
	promptSummary, ok := llmAttrs["prompt"].(map[string]any)
	if !ok || promptSummary["hash"] == "" || promptSummary["preview"] == "" {
		t.Fatalf("prompt should be summarized with hash and preview: %#v", llmAttrs["prompt"])
	}
	encoded, _ := json.Marshal(llmData)
	if strings.Contains(string(encoded), strings.Repeat("secret prompt ", 20)) {
		t.Fatalf("trace span should not persist full prompt: %s", string(encoded))
	}
}

func TestReadRunTraceKeepsHeadAndTailWhenTruncated(t *testing.T) {
	workspace := t.TempDir()
	ledger, err := newRunLedgerWithOptions(workspace, RunLedgerPolicy{Enabled: true, Directory: ".denova/runs", PreviewChars: 8}, RunOptions{
		AgentKind: AgentKindIDE,
		TaskID:    "task-long-trace",
		Workspace: workspace,
		Mode:      "ide",
	})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 620; i++ {
		if err := ledger.Record("event", map[string]any{
			"event_type": "test_event",
			"index":      i,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := ledger.RecordFinish("success", "", 0); err != nil {
		t.Fatal(err)
	}
	if err := ledger.Close(); err != nil {
		t.Fatal(err)
	}

	trace, err := ReadRunTrace(workspace, ledger.ID())
	if err != nil {
		t.Fatal(err)
	}
	if !trace.Truncated {
		t.Fatalf("expected trace to be marked truncated")
	}
	if len(trace.Records) != defaultRunTraceRecordCap {
		t.Fatalf("records = %d, want %d", len(trace.Records), defaultRunTraceRecordCap)
	}
	gap := trace.Records[defaultRunTraceRecordCap/2]
	if gap.Type != "trace_truncated_gap" {
		t.Fatalf("expected gap marker in middle, got %#v", gap)
	}
	if trace.Records[len(trace.Records)-1].Type != "run_finished" {
		t.Fatalf("tail should include run_finished, got %#v", trace.Records[len(trace.Records)-1])
	}
	if omitted, ok := numericInt64Field(gap.Data, "omitted_records"); !ok || omitted <= 0 {
		t.Fatalf("gap should report omitted records: %#v", gap.Data)
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
		Directory:    ".denova/runs",
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

	if !strings.HasPrefix(filepath.ToSlash(ledger.Path()), filepath.ToSlash(filepath.Join(workspace, ".denova/runs"))) {
		t.Fatalf("ledger path should be under workspace .denova/runs: %s", ledger.Path())
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

func TestRunLedgerSkipsTransportStreamEvents(t *testing.T) {
	workspace := t.TempDir()
	ledger, err := newRunLedger(workspace, RunLedgerPolicy{
		Enabled:      true,
		Directory:    ".denova/runs",
		PreviewChars: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range []Event{
		{Type: "run_state", Data: map[string]string{"phase": "started"}},
		{Type: "thinking", Data: map[string]string{"content": "逐帧思考"}},
		{Type: "chunk", Data: map[string]string{"content": "逐帧正文"}},
		{Type: "tool_args_delta", Data: map[string]string{"delta": `{"path"`}},
		{Type: "verification", Data: PostRunVerification{Status: "ok"}},
		{Type: "done", Data: map[string]string{}},
	} {
		if err := ledger.RecordEvent(ev); err != nil {
			t.Fatal(err)
		}
	}
	if err := ledger.RecordEvent(Event{Type: "tool_call", Data: map[string]interface{}{
		"id":   "call-1",
		"name": "write_file",
		"args": `{"path":"chapters/ch01.md"}`,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := ledger.RecordEvent(Event{Type: "error", Data: map[string]string{"message": "runner error"}}); err != nil {
		t.Fatal(err)
	}
	if err := ledger.Close(); err != nil {
		t.Fatal(err)
	}

	records := readRunLedgerRecords(t, ledger.Path())
	if len(records) != 3 {
		t.Fatalf("expected run_created plus 2 semantic event records, got %d: %#v", len(records), records)
	}
	if records[1]["type"] != "event" || records[2]["type"] != "event" {
		t.Fatalf("expected only semantic events after run_created: %#v", records)
	}
	firstEvent := records[1]["data"].(map[string]any)
	secondEvent := records[2]["data"].(map[string]any)
	if firstEvent["event_type"] != "tool_call" || secondEvent["event_type"] != "error" {
		t.Fatalf("unexpected persisted event types: %#v %#v", firstEvent, secondEvent)
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
