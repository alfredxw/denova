package agentrun

import (
	"bufio"
	"context"
	agentcontext "denova/internal/agents/context"
	"denova/internal/agents/tool"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"denova/config"
	"denova/internal/agents/toolresult"
	producttools "denova/internal/agents/tools"
	"denova/internal/book"
	"denova/internal/book/lore"
)

func TestContextLedgerRecordsBoundedSources(t *testing.T) {
	ledger := agentcontext.NewAuditLedger(agentcontext.AuditPolicy{Enabled: true, PreviewChars: 6})
	ledger.AddPart("文件引用", "@chapters/ch01.md", "user_reference", "第一章正文很长很长", "按单文件限制读取", true, true, 12)

	parts := ledger.Parts()
	if len(parts) != 1 {
		t.Fatalf("expected 1 context part, got %d", len(parts))
	}
	part := parts[0]
	if part.Source != "文件引用" || part.Title != "@chapters/ch01.md" || part.Purpose != "user_reference" {
		t.Fatalf("unexpected ledger part identity: %#v", part)
	}
	if part.Bytes == 0 || part.Chars == 0 || part.Hash == "" || part.Preview == "" {
		t.Fatalf("ledger should record bounded size metadata: %#v", part)
	}
	if strings.Contains(part.Preview, "很长很长") {
		t.Fatalf("preview should be bounded, got %q", part.Preview)
	}
	if !part.Included || !part.Truncated || part.Limit != 12 || part.LimitUnit != "bytes" {
		t.Fatalf("ledger should preserve inclusion and truncation metadata: %#v", part)
	}
	if summary := ledger.Summary(); strings.Contains(summary, part.Hash) || strings.Contains(summary, part.Preview) {
		t.Fatalf("normal context summary exposed rich in-memory audit fields: %s", summary)
	}
}

func TestFilterToolResultKeepsContentBelowDefaultLimit(t *testing.T) {
	content := strings.Repeat("章节正文", 4096)
	filtered := toolresult.FilterText(
		"write",
		producttools.WorkspaceWriteDescriptor(agenttool.ToolSourceWrite, config.AgentToolWorkspaceWrite, agenttool.ToolRecoveryReconcilable),
		`{"path":"chapters/ch00001.md"}`,
		content,
		0,
	)
	if filtered.Manifest.Source != agenttool.ToolSourceWrite || filtered.Manifest.MutationScope != agenttool.ToolMutationWorkspace || filtered.Manifest.PostCheck != agenttool.ToolPostCheckWorkspaceChange {
		t.Fatalf("write should be classified as workspace mutation: %#v", filtered.Manifest)
	}
	if filtered.Manifest.Capability != config.AgentToolWorkspaceWrite {
		t.Fatalf("write capability = %q, want %s", filtered.Manifest.Capability, config.AgentToolWorkspaceWrite)
	}
	if filtered.Result.Metadata.ModelTruncated {
		t.Fatalf("tool result below the default limit should not truncate")
	}
	if filtered.Result.ModelContent != content {
		t.Fatalf("model content below the limit changed")
	}
	if filtered.Result.Metadata.Target != "chapters/ch00001.md" ||
		!strings.HasPrefix(filtered.Result.Metadata.IdempotencyKey, "write:") {
		t.Fatalf("structured result metadata = %#v", filtered.Result.Metadata)
	}
	if strings.Contains(filtered.Result.ModelContent, "tool_result.v1") || strings.Contains(filtered.Result.ModelContent, "mutation_scope") {
		t.Fatalf("execution metadata leaked into model content: %s", filtered.Result.ModelContent)
	}
}

func TestFilterToolResultBoundsOutputAboveDefaultLimit(t *testing.T) {
	content := strings.Repeat("x", toolresult.DefaultMaxBytes+1024)
	filtered := toolresult.Filter("read", `{"path":"references/large.txt"}`, content)
	if !filtered.Result.Metadata.ModelTruncated || filtered.Manifest.MaxResultBytes != toolresult.DefaultMaxBytes {
		t.Fatalf("default tool result safety cap was not enforced: %#v", filtered)
	}
	if !strings.Contains(filtered.Result.ModelContent, "[tool result truncated]") {
		t.Fatalf("bounded result should explain its truncation: %s", filtered.Result.ModelContent[len(filtered.Result.ModelContent)-512:])
	}
}

func TestFilterToolResultBoundsOutputWhenLimitConfigured(t *testing.T) {
	content := strings.Repeat("章节正文", 4096)
	filtered := toolresult.FilterWithLimit("write", `{"path":"chapters/ch00001.md"}`, content, 8*1024)
	if !filtered.Result.Metadata.ModelTruncated {
		t.Fatalf("expected long result to be truncated when limit is configured")
	}
	if !strings.Contains(filtered.Result.ModelContent, "[tool result truncated]") {
		t.Fatalf("filtered result should include truncation markers: %s", filtered.Result.ModelContent)
	}
	if len(filtered.Result.ModelContent) > 8*1024+1024 {
		t.Fatalf("filtered result should stay bounded, got %d bytes", len(filtered.Result.ModelContent))
	}
}

func TestPostRunVerifierChecksLoreWriteResult(t *testing.T) {
	workspace := t.TempDir()
	store := lore.NewStore(workspace)
	item, err := store.Create(lore.ItemInput{
		ID:         "hero",
		Type:       "character",
		Name:       "林川",
		Importance: "major",
		LoadMode:   lore.LoadModeResident,
		Content:    "林川是主角。",
	})
	if err != nil {
		t.Fatal(err)
	}
	result := agenttool.VerifyPostRunMutations(book.NewService(workspace), []agenttool.Mutation{{
		ToolName:      "write_lore_items",
		Source:        agenttool.ToolSourceLore,
		MutationScope: agenttool.ToolMutationWorkspace,
		PostCheck:     agenttool.ToolPostCheckWorkspaceChange,
		LoreItemIDs:   []string{item.ID},
	}})
	if result.Status != "ok" {
		t.Fatalf("created lore item should pass verification after default brief generation: %#v", result)
	}
	result = agenttool.VerifyPostRunMutations(book.NewService(workspace), []agenttool.Mutation{{
		ToolName:      "write_lore_items",
		Source:        agenttool.ToolSourceLore,
		MutationScope: agenttool.ToolMutationWorkspace,
		PostCheck:     agenttool.ToolPostCheckWorkspaceChange,
		LoreItemIDs:   []string{"missing-id"},
	}})
	if result.Status != "warning" {
		t.Fatalf("missing changed lore item should warn: %#v", result)
	}
}

func TestRunTraceReaderSummarizesLedger(t *testing.T) {
	workspace := t.TempDir()
	ledger, err := NewLedgerWithOptions(workspace, LedgerPolicy{Enabled: true, Directory: ".denova/runs", PreviewChars: 8}, Options{
		AgentKind:       AgentKindInteractiveStory,
		TaskID:          "task-1",
		SessionID:       "session-1",
		StoryID:         "story-1",
		BranchID:        "main",
		TurnID:          "turn-1",
		MaintenanceTask: "director_plan_update",
		Workspace:       workspace,
		Mode:            "interactive",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := ledger.RecordContext([]agentcontext.AuditPart{{Source: "用户输入", Title: "请求", Included: true}}); err != nil {
		t.Fatal(err)
	}
	if err := ledger.Record("run_context", map[string]any{
		"story_id":         "story-1",
		"branch_id":        "main",
		"turn_id":          "turn-committed",
		"maintenance_task": "director_plan_update",
	}); err != nil {
		t.Fatal(err)
	}
	if err := ledger.RecordEvent(Event{Type: "tool_result", Data: map[string]interface{}{
		"id":      "call-1",
		"name":    "write",
		"content": "写入成功",
	}}); err != nil {
		t.Fatal(err)
	}
	if err := ledger.RecordToolDecision(agenttool.Decision{
		ToolName:    "write",
		ExecutionID: "call-1",
		Source:      agenttool.ToolSourceWrite,
		Capability:  "file_write",
		Action:      "allowed",
		Target:      "chapters/ch01.md",
	}); err != nil {
		t.Fatal(err)
	}
	if err := ledger.RecordToolExecution(agenttool.ExecutionRecord{
		ToolName:              "submit_interactive_turn",
		ExecutionID:           "call-1",
		Status:                "success",
		DomainStatus:          "rejected",
		DomainDiagnosticCount: 2,
		Capability:            "file_write",
		OriginalBytes:         64,
		ReturnedBytes:         48,
		Truncated:             true,
		Target:                "chapters/ch01.md",
	}); err != nil {
		t.Fatal(err)
	}
	if err := ledger.RecordToolDecision(agenttool.Decision{
		ToolName:    "write",
		ExecutionID: "call-2",
		Source:      agenttool.ToolSourceWrite,
		Capability:  "file_write",
		Action:      "blocked",
		Reason:      "参数不是完整 JSON 对象",
	}); err != nil {
		t.Fatal(err)
	}
	if err := ledger.RecordToolExecution(agenttool.ExecutionRecord{
		ToolName:    "write",
		ExecutionID: "call-2",
		Status:      "blocked",
		Capability:  "file_write",
		Error:       "参数不是完整 JSON 对象",
	}); err != nil {
		t.Fatal(err)
	}
	if err := ledger.RecordMutations([]agenttool.Mutation{{ToolName: "write", Source: agenttool.ToolSourceWrite, Target: "chapters/ch01.md"}}); err != nil {
		t.Fatal(err)
	}
	if err := ledger.RecordVerification(agenttool.Verification{Status: "ok", Mutations: 1}); err != nil {
		t.Fatal(err)
	}
	if err := ledger.RecordFinish("success", "", 32); err != nil {
		t.Fatal(err)
	}
	if err := ledger.Close(); err != nil {
		t.Fatal(err)
	}
	location := TraceLocation{Workspace: workspace}
	summaries, err := ListRunTraces(location, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 1 || summaries[0].Status != "success" || summaries[0].Events != 1 || summaries[0].ContextParts != 1 {
		t.Fatalf("unexpected trace summary: %#v", summaries)
	}
	if summaries[0].AgentKind != AgentKindInteractiveStory || summaries[0].TaskID != "task-1" || summaries[0].SessionID != "session-1" || summaries[0].StoryID != "story-1" || summaries[0].BranchID != "main" || summaries[0].TurnID != "turn-committed" || summaries[0].MaintenanceTask != "director_plan_update" || summaries[0].Mutations != 1 || summaries[0].VerificationStatus != "ok" {
		t.Fatalf("trace summary should include durable run state: %#v", summaries[0])
	}
	if summaries[0].ToolCalls != 2 || summaries[0].ToolSuccesses != 1 || summaries[0].ToolBlocked != 1 || summaries[0].ToolTruncated != 1 || summaries[0].InvalidToolArgs != 1 {
		t.Fatalf("trace summary should include tool quality counters: %#v", summaries[0])
	}
	if summaries[0].ToolDomainRejected != 1 || summaries[0].ToolDomainDiagnostics != 2 {
		t.Fatalf("transport success must not hide a rejected domain receipt: %#v", summaries[0])
	}
	trace, err := ReadRunTrace(location, summaries[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(trace.Records) != 11 || trace.Summary.ID != summaries[0].ID {
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
	ledger, err := NewLedgerWithOptions(workspace, LedgerPolicy{Enabled: true, Directory: ".denova/runs", PreviewChars: 8}, Options{
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
	ctx := ContextWithObserver(ContextWithRunTrace(context.Background(), ledger.ID(), ledger, root.SpanID()), NewObserver(ledger, root.SpanID()))
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
	ObserverFromContext(ctx).RecordLLMSpan(llm.SpanID())
	llm.Finish("success", map[string]any{
		"provider_request_id":  "provider-1",
		"finish_reason":        "tool_calls",
		"ttft_ms":              420,
		"prompt_tokens":        12,
		"cached_prompt_tokens": 4,
		"completion_tokens":    6,
		"total_tokens":         18,
	})
	ObserverFromContext(ctx).RecordToolDecision(agenttool.Decision{
		ToolName:    "read",
		ExecutionID: "tool-1",
		Source:      agenttool.ToolSourceRead,
		Capability:  "file_read",
		Action:      "allowed",
		Target:      "chapters/ch01.md",
	})
	ObserverFromContext(ctx).RecordToolExecution(agenttool.ExecutionRecord{
		ToolName:      "read",
		ExecutionID:   "tool-1",
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

	trace, err := ReadRunTrace(TraceLocation{Workspace: workspace}, ledger.ID())
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
	if llmAttrs["provider_request_id"] != "provider-1" || llmAttrs["total_tokens"].(float64) != 18 || llmAttrs["ttft_ms"].(float64) != 420 {
		t.Fatalf("llm attrs should include provider id and tokens: %#v", llmAttrs)
	}
	if _, exists := llmAttrs["prompt"]; exists {
		t.Fatalf("prompt-derived hash or preview must not be persisted: %#v", llmAttrs["prompt"])
	}
	encoded, _ := json.Marshal(llmData)
	if strings.Contains(string(encoded), strings.Repeat("secret prompt ", 20)) {
		t.Fatalf("trace span should not persist full prompt: %s", string(encoded))
	}
}

func TestRunTraceSummaryAggregatesLLMCacheUsage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run-cache.jsonl")
	content := strings.Join([]string{
		`{"type":"run_created","run_id":"run-cache","created_at":"2026-07-09T00:00:00Z","data":{"agent_kind":"ide"}}`,
		`{"type":"llm_call","run_id":"run-cache","created_at":"2026-07-09T00:00:01Z","data":{"attrs":{"prompt_tokens":1000,"cached_prompt_tokens":400,"uncached_prompt_tokens":600,"total_tokens":1200}}}`,
		`{"type":"llm_call","run_id":"run-cache","created_at":"2026-07-09T00:00:02Z","data":{"attrs":{"prompt_tokens":500,"cached_prompt_tokens":500,"total_tokens":650}}}`,
		`{"type":"run_finished","run_id":"run-cache","created_at":"2026-07-09T00:00:03Z","data":{"status":"success"}}`,
	}, "\n")
	if err := os.WriteFile(path, []byte(content+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	trace, err := readRunTraceFile(path, defaultRunTraceRecordCap)
	if err != nil {
		t.Fatal(err)
	}
	if trace.Summary.LLMCalls != 2 {
		t.Fatalf("llm calls = %d, want 2", trace.Summary.LLMCalls)
	}
	if trace.Summary.PromptTokens != 1500 || trace.Summary.CachedPromptTokens != 900 || trace.Summary.UncachedPromptTokens != 600 {
		t.Fatalf("cache token summary mismatch: %#v", trace.Summary)
	}
	if trace.Summary.CacheHitRate != 0.6 {
		t.Fatalf("cache hit rate = %.4f, want 0.6", trace.Summary.CacheHitRate)
	}
}

func TestReadRunTraceKeepsHeadAndTailWhenTruncated(t *testing.T) {
	workspace := t.TempDir()
	ledger, err := NewLedgerWithOptions(workspace, LedgerPolicy{Enabled: true, Directory: ".denova/runs", PreviewChars: 8}, Options{
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

	trace, err := ReadRunTrace(TraceLocation{Workspace: workspace}, ledger.ID())
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
	policy := (LoopPolicy{}).Normalize()
	if !policy.ContextLedger.Enabled || !policy.RunLedger.Enabled {
		t.Fatalf("zero loop policy should enable default ledgers: %#v", policy)
	}
	if policy.RunLedger.Directory != DefaultLoopPolicy().RunLedger.Directory {
		t.Fatalf("zero loop policy should use default run ledger directory: %#v", policy)
	}
}

func TestRunLedgerWritesBoundedJSONLTrace(t *testing.T) {
	workspace := t.TempDir()
	ledger, err := NewLedger(workspace, LedgerPolicy{
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
	if err := ledger.RecordContext([]agentcontext.AuditPart{{Source: "用户输入", Title: "本轮原始请求", Bytes: 12, Chars: 6, Preview: "写一章", Included: true}}); err != nil {
		t.Fatal(err)
	}
	if err := ledger.RecordEvent(Event{Type: "tool_result", Data: map[string]interface{}{
		"id":      "call-1",
		"name":    "read",
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
	if eventData["result_bytes"].(float64) == 0 {
		t.Fatalf("tool result should retain content-free size metadata: %#v", eventData)
	}
	if _, exists := eventData["content"]; exists {
		t.Fatalf("tool result content must not be persisted: %#v", eventData)
	}
}

func TestRunLedgerSkipsTransportStreamEvents(t *testing.T) {
	workspace := t.TempDir()
	ledger, err := NewLedger(workspace, LedgerPolicy{
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
		{Type: "verification", Data: agenttool.Verification{Status: "ok"}},
		{Type: "done", Data: map[string]string{}},
	} {
		if err := ledger.RecordEvent(ev); err != nil {
			t.Fatal(err)
		}
	}
	if err := ledger.RecordEvent(Event{Type: "tool_call", Data: map[string]interface{}{
		"id":   "call-1",
		"name": "write",
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

func TestRunLedgerPersistsOnlyContentFreeTelemetry(t *testing.T) {
	const secret = "SYNTHETIC_SECRET_DO_NOT_PERSIST"
	workspace := t.TempDir()
	ledger, err := NewLedger(workspace, LedgerPolicy{Enabled: true, Directory: ".denova/runs"})
	if err != nil {
		t.Fatal(err)
	}
	if err := ledger.RecordContext([]agentcontext.AuditPart{{
		Source: "user_input", Title: secret, Purpose: "turn_request", Bytes: len(secret), Chars: len(secret),
		Hash: "sha256:" + secret, Preview: secret, Note: "revision=" + secret, Included: true,
	}}); err != nil {
		t.Fatal(err)
	}
	for _, event := range []Event{
		{Type: "tool_call", Data: map[string]any{"id": "call-1", "name": "read", "args": `{"path":"` + secret + `"}`, "target": secret}},
		{Type: "tool_result", Data: map[string]any{"id": "call-1", "name": "read", "content": secret, "status": "success"}},
		{Type: "context_compaction", Data: map[string]any{"phase": "model_step", "status": "delta", "delta": secret}},
		{Type: "context_compaction", Data: map[string]any{
			"phase": "model_step", "status": "completed", "summary": secret, "tokens_before": 100, "tokens_after": 40,
			"cache_identity_status": "exact_primary_snapshot",
			"cache_usage_status":    "zero_or_unreported",
			"cache_miss_reason":     "provider_cached_prefix_zero_or_unreported",
		}},
		{Type: "context_cleanup", Data: map[string]any{
			"phase": "model_step", "status": "failed", "error": secret, "actual_reclaimed_tokens": 12,
			"cache_viable_candidate_tokens": 17, "cleanup_skipped_below_minimum_count": 1,
			"cleanup_skipped_warm_suffix_count": 2,
		}},
		{Type: "context_normalizer", Data: map[string]any{"status": "repaired", "context_normalizer_repair_count": 1}},
	} {
		if err := ledger.RecordEvent(event); err != nil {
			t.Fatal(err)
		}
	}
	if err := ledger.RecordToolDecision(agenttool.Decision{
		ToolName: "read", ExecutionID: "call-1", Action: "blocked", Reason: secret,
		Target: secret, ArgsBytes: len(secret),
	}); err != nil {
		t.Fatal(err)
	}
	if err := ledger.RecordToolExecution(agenttool.ExecutionRecord{
		ToolName: "read", ExecutionID: "call-1", Status: "error", Result: secret, Error: secret,
		Target: secret, IdempotencyKey: "sha256:" + secret, BaseRevision: "sha256:" + secret,
		Revision: "sha256:" + secret, OriginalBytes: len(secret), ReturnedBytes: len(secret),
	}); err != nil {
		t.Fatal(err)
	}
	if err := ledger.RecordMutations([]agenttool.Mutation{{
		ToolName: "write", Source: agenttool.ToolSourceWrite, Target: secret,
		BaseRevision: "sha256:" + secret, Revision: "sha256:" + secret,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := ledger.RecordVerification(agenttool.Verification{
		Status: "warning", Mutations: 1, Warnings: []string{secret},
		Checks: []agenttool.VerificationCheck{{Type: "path", Target: secret, Status: "warning", Message: secret}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := ledger.RecordTraceSpan(TraceSpanRecord{
		TraceID: ledger.ID(), SpanID: "span-1", Name: "llm_call", Status: "error",
		Attrs: map[string]any{"prompt": secret, "content": secret, "revision": "sha256:" + secret, "prompt_tokens": 10, "error": secret},
		Error: secret,
	}); err != nil {
		t.Fatal(err)
	}
	if err := ledger.RecordFinish("error", secret, 0); err != nil {
		t.Fatal(err)
	}
	if err := ledger.Close(); err != nil {
		t.Fatal(err)
	}

	payload, err := os.ReadFile(ledger.Path())
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(payload)
	for _, forbidden := range []string{secret, `"summary"`, `"delta"`, `"preview"`, `"hash"`, `"base_revision"`, `"revision"`, `"target"`, `"args"`} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("durable ledger contains forbidden content %q:\n%s", forbidden, encoded)
		}
	}
	if !strings.Contains(encoded, `"result_bytes"`) || !strings.Contains(encoded, `"error_class":"failure"`) ||
		!strings.Contains(encoded, `"event_type":"context_cleanup"`) || !strings.Contains(encoded, `"event_type":"context_compaction"`) ||
		!strings.Contains(encoded, `"event_type":"context_normalizer"`) ||
		!strings.Contains(encoded, `"cache_viable_candidate_tokens":17`) ||
		!strings.Contains(encoded, `"cleanup_skipped_below_minimum_count":1`) ||
		!strings.Contains(encoded, `"cleanup_skipped_warm_suffix_count":2`) ||
		!strings.Contains(encoded, `"cache_identity_status":"exact_primary_snapshot"`) ||
		!strings.Contains(encoded, `"cache_usage_status":"zero_or_unreported"`) ||
		!strings.Contains(encoded, `"cache_miss_reason":"provider_cached_prefix_zero_or_unreported"`) {
		t.Fatalf("safe maintenance telemetry was not retained:\n%s", encoded)
	}
	if info, err := os.Stat(ledger.Path()); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("ledger mode = %v err=%v, want 0600", infoMode(info), err)
	}
	if info, err := os.Stat(filepath.Dir(ledger.Path())); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("ledger directory mode = %v err=%v, want 0700", infoMode(info), err)
	}
}

func TestRunLedgerSanitizationDoesNotMutateTransientEvent(t *testing.T) {
	workspace := t.TempDir()
	ledger, err := NewLedger(workspace, LedgerPolicy{Enabled: true, Directory: ".denova/runs"})
	if err != nil {
		t.Fatal(err)
	}
	data := map[string]any{
		"phase": "model_step", "status": "completed",
		"summary": "checkpoint remains available to the live UI",
	}
	if err := ledger.RecordEvent(Event{Type: "context_compaction", Data: data}); err != nil {
		t.Fatal(err)
	}
	if data["summary"] != "checkpoint remains available to the live UI" {
		t.Fatalf("durable sanitizer mutated transient payload: %#v", data)
	}
	if err := ledger.Close(); err != nil {
		t.Fatal(err)
	}
}

func infoMode(info os.FileInfo) os.FileMode {
	if info == nil {
		return 0
	}
	return info.Mode().Perm()
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
