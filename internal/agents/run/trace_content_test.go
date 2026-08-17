package agentrun

import (
	"context"
	agenttool "denova/internal/agents/tool"
	"encoding/json"
	"os"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/providers"
)

func TestDeveloperTraceContentCapturesModelBoundary(t *testing.T) {
	oldTraceConfig := traceRuntimeConfigSnapshot()
	SetTraceRuntimeConfig(TraceCaptureSummary, TraceExporterLocal, 100)
	SetTraceContentCaptureEnabled(true)
	t.Cleanup(func() {
		SetTraceRuntimeConfig(oldTraceConfig.CaptureLevel, oldTraceConfig.Exporter, oldTraceConfig.RetentionRuns)
		SetTraceContentCaptureEnabled(oldTraceConfig.CaptureContent)
	})

	workspace := t.TempDir()
	ledger, err := NewLedgerWithOptions(workspace, LedgerPolicy{Enabled: true, Directory: ".denova/runs"}, Options{
		AgentKind: AgentKindIDE,
		Workspace: workspace,
		Mode:      "ide",
	})
	if err != nil {
		t.Fatal(err)
	}
	root := StartRootTraceSpan(ledger, map[string]any{"agent_kind": AgentKindIDE})
	observer := NewObserver(ledger, root.SpanID())
	ctx := ContextWithObserver(ContextWithRunTrace(context.Background(), ledger.ID(), ledger, root.SpanID()), observer)
	config := providers.ModelConfig{Model: "trace-model", APIKey: "secret-api-key"}
	messages := []*agent.Message{
		agent.SystemMessage("System prompt with exact instructions"),
		{Role: agent.User, Content: "Inspect this request", Extra: map[string]any{"agent.context.source": "user"}},
	}
	tools := []*agent.ToolInfo{{
		Name: "read",
		Desc: "Read a workspace file",
		ParamsOneOf: agent.NewParamsOneOfByParams(map[string]*agent.ParameterInfo{
			"path": {Type: agent.String, Desc: "Workspace-relative path", Required: true},
		}),
	}}
	span, callID, _ := BeginLLMCallTrace(ctx, AgentKindIDE, "test", "ide", config, messages, tools, true)
	output := &agent.Message{
		Role:             agent.Assistant,
		Content:          "## Result\n\nDone.",
		ReasoningContent: "I should inspect the request first.",
		ResponseMeta: &agent.ResponseMeta{FinishReason: "stop", Usage: &agent.TokenUsage{
			PromptTokens: 18, CompletionTokens: 7, TotalTokens: 25,
		}},
	}
	FinishLLMCallTrace(span, callID, AgentKindIDE, "test", "ide", config.Model, 1, output, nil, map[string]any{"ttft_ms": 125})
	root.Finish("success", nil)
	if err := ledger.Close(); err != nil {
		t.Fatal(err)
	}

	trace, err := ReadRunTrace(TraceLocation{Workspace: workspace}, ledger.ID())
	if err != nil {
		t.Fatal(err)
	}
	if !trace.Summary.ContentCaptured {
		t.Fatalf("trace summary should advertise captured content: %#v", trace.Summary)
	}
	input := findTraceRecord(trace.Records, "llm_input")
	outputRecord := findTraceRecord(trace.Records, "llm_output")
	if input == nil || outputRecord == nil {
		t.Fatalf("expected input and output records: %#v", trace.Records)
	}
	encodedInput, _ := json.Marshal(input.Data)
	encodedOutput, _ := json.Marshal(outputRecord.Data)
	if !strings.Contains(string(encodedInput), "System prompt with exact instructions") ||
		!strings.Contains(string(encodedInput), "Read a workspace file") ||
		!strings.Contains(string(encodedInput), "Workspace-relative path") {
		t.Fatalf("input record is incomplete: %s", encodedInput)
	}
	if !strings.Contains(string(encodedOutput), "I should inspect the request first.") ||
		!strings.Contains(string(encodedOutput), "## Result") {
		t.Fatalf("output record is incomplete: %s", encodedOutput)
	}
	payload, err := os.ReadFile(ledger.Path())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), config.APIKey) {
		t.Fatal("developer trace content must not persist API keys")
	}
}

func TestDeveloperTraceContentDisabledByDefault(t *testing.T) {
	oldTraceConfig := traceRuntimeConfigSnapshot()
	SetTraceRuntimeConfig(TraceCaptureSummary, TraceExporterLocal, 100)
	SetTraceContentCaptureEnabled(false)
	t.Cleanup(func() {
		SetTraceRuntimeConfig(oldTraceConfig.CaptureLevel, oldTraceConfig.Exporter, oldTraceConfig.RetentionRuns)
		SetTraceContentCaptureEnabled(oldTraceConfig.CaptureContent)
	})

	workspace := t.TempDir()
	ledger, err := NewLedger(workspace, LedgerPolicy{Enabled: true, Directory: ".denova/runs"})
	if err != nil {
		t.Fatal(err)
	}
	root := StartRootTraceSpan(ledger, nil)
	ctx := ContextWithObserver(ContextWithRunTrace(context.Background(), ledger.ID(), ledger, root.SpanID()), NewObserver(ledger, root.SpanID()))
	span, callID, _ := BeginLLMCallTrace(ctx, AgentKindIDE, "test", "ide", providers.ModelConfig{}, []*agent.Message{agent.SystemMessage("private")}, nil, true)
	FinishLLMCallTrace(span, callID, AgentKindIDE, "test", "ide", "", 1, agent.AssistantMessage("private output", nil), nil, nil)
	observer := ObserverFromContext(ctx)
	observer.RecordToolDecision(agenttool.Decision{ToolName: "read", ExecutionID: "disabled-call", Action: "allowed"})
	observer.RecordToolExecution(agenttool.ExecutionRecord{ToolName: "read", ExecutionID: "disabled-call", Status: "success", Result: "sensitive tool output"})
	root.Finish("success", nil)
	if err := ledger.Close(); err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(ledger.Path())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), "private") || strings.Contains(string(payload), "sensitive tool output") || strings.Contains(string(payload), "llm_input") || strings.Contains(string(payload), "llm_output") || strings.Contains(string(payload), "tool_output") {
		t.Fatalf("content capture leaked while disabled: %s", payload)
	}
}

func TestRunTraceReaderAcceptsLargeDeveloperRecords(t *testing.T) {
	oldTraceConfig := traceRuntimeConfigSnapshot()
	SetTraceRuntimeConfig(TraceCaptureSummary, TraceExporterLocal, 100)
	t.Cleanup(func() {
		SetTraceRuntimeConfig(oldTraceConfig.CaptureLevel, oldTraceConfig.Exporter, oldTraceConfig.RetentionRuns)
		SetTraceContentCaptureEnabled(oldTraceConfig.CaptureContent)
	})
	workspace := t.TempDir()
	ledger, err := NewLedger(workspace, LedgerPolicy{Enabled: true, Directory: ".denova/runs"})
	if err != nil {
		t.Fatal(err)
	}
	content := strings.Repeat("context", 300_000)
	if err := ledger.RecordTraceContent(TraceContentRecord{
		TraceID: ledger.ID(), SpanID: "span-large", CallID: "call-large", Type: "llm_input",
		Payload: map[string]any{"messages": []*agent.Message{agent.SystemMessage(content)}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := ledger.Close(); err != nil {
		t.Fatal(err)
	}
	trace, err := ReadRunTrace(TraceLocation{Workspace: workspace}, ledger.ID())
	if err != nil {
		t.Fatal(err)
	}
	if record := findTraceRecord(trace.Records, "llm_input"); record == nil {
		t.Fatalf("large content record was not read: %#v", trace)
	}
}

func TestObserverRecordsDeveloperToolOutputContent(t *testing.T) {
	oldTraceConfig := traceRuntimeConfigSnapshot()
	SetTraceRuntimeConfig(TraceCaptureSummary, TraceExporterLocal, 100)
	SetTraceContentCaptureEnabled(true)
	t.Cleanup(func() {
		SetTraceRuntimeConfig(oldTraceConfig.CaptureLevel, oldTraceConfig.Exporter, oldTraceConfig.RetentionRuns)
		SetTraceContentCaptureEnabled(oldTraceConfig.CaptureContent)
	})
	workspace := t.TempDir()
	ledger, err := NewLedger(workspace, LedgerPolicy{Enabled: true, Directory: ".denova/runs"})
	if err != nil {
		t.Fatal(err)
	}
	observer := NewObserver(ledger, "")
	observer.RecordToolDecision(agenttool.Decision{
		ToolName: "read", ProviderCallID: "provider-call", ExecutionID: "execution-call", Action: "allowed",
	})
	observer.RecordToolExecution(agenttool.ExecutionRecord{
		ToolName: "read", ProviderCallID: "provider-call", ExecutionID: "execution-call", Status: "success",
		Result: "complete developer-visible result", OriginalBytes: 33, ReturnedBytes: 33,
	})
	if err := ledger.Close(); err != nil {
		t.Fatal(err)
	}
	trace, err := ReadRunTrace(TraceLocation{Workspace: workspace}, ledger.ID())
	if err != nil {
		t.Fatal(err)
	}
	record := findTraceRecord(trace.Records, "tool_output")
	if record == nil {
		t.Fatalf("tool output content record was not read: %#v", trace)
	}
	content, _ := record.Data["content"].(map[string]any)
	if content["result"] != "complete developer-visible result" || content["provider_call_id"] != "provider-call" {
		t.Fatalf("tool output content = %#v", content)
	}
}

func findTraceRecord(records []RunTraceRecord, recordType string) *RunTraceRecord {
	for index := range records {
		if records[index].Type == recordType {
			return &records[index]
		}
	}
	return nil
}
