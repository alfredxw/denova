package agents

import (
	"context"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	agent "github.com/alfredxw/denova/agent"
)

type capturingToolLifecycleObserver struct {
	records []ToolExecutionRecord
}

func (*capturingToolLifecycleObserver) BeforeTool(context.Context, ToolDecision, string) error {
	return nil
}

func (observer *capturingToolLifecycleObserver) AfterTool(_ context.Context, record ToolExecutionRecord) error {
	observer.records = append(observer.records, record)
	return nil
}

func TestToolOrchestratorBoundsEndpointErrorsForModelDisplayAndPersistence(t *testing.T) {
	const tail = "END_OF_UNBOUNDED_ERROR"
	hugeError := errors.New(strings.Repeat("巨大错误", 5000) + tail)
	observer := &capturingToolLifecycleObserver{}
	ctx := ContextWithToolLifecycleObserver(context.Background(), observer)
	middleware := &toolOrchestratorMiddleware{agentKind: AgentKindIDE, toolResultMaxBytes: 64}
	endpoint, err := middleware.WrapToolCall(context.Background(),
		func(context.Context, string, ...agent.ToolOption) (agent.ToolResult, error) {
			return agent.ToolResult{}, hugeError
		},
		testToolContext("read", "call-error"),
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := endpoint(ctx, `{"path":"large.txt"}`)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != agent.ToolResultError || !result.Metadata.ModelTruncated || !result.Metadata.DisplayTruncated {
		t.Fatalf("structured error result = %#v", result)
	}
	if !strings.Contains(result.ModelContent, "[tool result truncated]") || strings.Contains(result.ModelContent, tail) || strings.Contains(result.ModelContent, "tool_result.v1") {
		t.Fatalf("bounded model error = %q", result.ModelContent)
	}
	if len(result.ModelContent) > 64 || len(result.DisplayContent) > 64 {
		t.Fatalf("result exceeded descriptor limit: model=%d display=%d", len(result.ModelContent), len(result.DisplayContent))
	}
	if len(observer.records) != 1 {
		t.Fatalf("lifecycle records = %d, want 1", len(observer.records))
	}
	record := observer.records[0]
	if record.Result != result.ModelContent || !record.Truncated || record.ReturnedBytes != len(result.ModelContent) {
		t.Fatalf("persisted result did not use the model projection: %#v", record)
	}
	if len(record.Error) > maxToolErrorDiagnosticBytes || !utf8.ValidString(record.Error) || strings.Contains(record.Error, tail) {
		t.Fatalf("persisted diagnostic is not bounded: bytes=%d valid=%t", len(record.Error), utf8.ValidString(record.Error))
	}
}

func TestToolOrchestratorNormalizesInvalidUTF8InEndpointErrors(t *testing.T) {
	invalidError := errors.New("broken\xffdiagnostic")
	observer := &capturingToolLifecycleObserver{}
	ctx := ContextWithToolLifecycleObserver(context.Background(), observer)
	middleware := &toolOrchestratorMiddleware{agentKind: AgentKindIDE, toolResultMaxBytes: 256}
	endpoint, err := middleware.WrapToolCall(context.Background(),
		func(context.Context, string, ...agent.ToolOption) (agent.ToolResult, error) {
			return agent.ToolResult{}, invalidError
		},
		testToolContext("read", "call-invalid-utf8-error"),
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := endpoint(ctx, `{"path":"broken.txt"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !utf8.ValidString(result.ModelContent) || !utf8.ValidString(result.DisplayContent) || len(observer.records) != 1 || !utf8.ValidString(observer.records[0].Error) {
		t.Fatalf("tool error projection must be valid UTF-8: result=%#v records=%#v", result, observer.records)
	}
}

func TestToolOrchestratorUsesDetailsForMutationReceiptWhenModelContentIsTruncated(t *testing.T) {
	receipt := `{"schema":"workspace_change.tool_result.v1","status":"applied","workspace":"/workspace/book-a","change_group_id":"group-1","change_set_id":"change-1","path":"chapters/from-receipt.md","revision":"sha256:after"}`
	observer := &capturingToolLifecycleObserver{}
	ctx := ContextWithToolLifecycleObserver(context.Background(), observer)
	middleware := &toolOrchestratorMiddleware{agentKind: AgentKindIDE, toolResultMaxBytes: len(receipt) + 8}
	endpoint, err := middleware.WrapToolCall(context.Background(),
		func(context.Context, string, ...agent.ToolOption) (agent.ToolResult, error) {
			result := agent.TextToolResult(strings.Repeat("display and model content ", 100))
			result.Details = []byte(receipt)
			return result, nil
		},
		testToolContext("write", "call-details-receipt"),
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := endpoint(ctx, `{"path":"chapters/from-args.md"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Metadata.ModelTruncated || len(observer.records) != 1 {
		t.Fatalf("expected bounded result and lifecycle record: result=%#v records=%#v", result, observer.records)
	}
	record := observer.records[0]
	if record.ChangeGroupID != "group-1" || record.Workspace != "/workspace/book-a" || record.Revision != "sha256:after" || record.Target != "chapters/from-receipt.md" {
		t.Fatalf("Details receipt was not authoritative: %#v", record)
	}
}

func TestToolOrchestratorPreservesCommittedReceiptWhenEndpointAlsoErrors(t *testing.T) {
	receipt := `{"schema":"workspace_change.tool_result.v1","status":"applied","workspace":"/workspace/book-a","change_group_id":"group-1","change_set_id":"change-1","path":"chapters/committed.md"}`
	observer := &capturingToolLifecycleObserver{}
	ctx := ContextWithToolLifecycleObserver(context.Background(), observer)
	middleware := &toolOrchestratorMiddleware{agentKind: AgentKindIDE, toolResultMaxBytes: 1024}
	endpoint, err := middleware.WrapToolCall(context.Background(),
		func(context.Context, string, ...agent.ToolOption) (agent.ToolResult, error) {
			result := agent.ToolErrorResult("commit reporting failed", "commit reporting failed")
			result.Details = []byte(receipt)
			return result, errors.New("reporting failed after commit")
		},
		testToolContext("write", "call-error-receipt"),
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := endpoint(ctx, `{"path":"chapters/committed.md","content":"done"}`)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != agent.ToolResultError || len(observer.records) != 1 {
		t.Fatalf("result=%#v records=%#v", result, observer.records)
	}
	record := observer.records[0]
	mutation, committed := toolMutationFromExecutionRecord(record)
	if !committed || record.Status != "error" || mutation.Target != "chapters/committed.md" {
		t.Fatalf("error receipt was not committed: record=%#v mutation=%#v committed=%t", record, mutation, committed)
	}
}

func TestToolOrchestratorTurnsInvalidDetailsIntoStructuredToolError(t *testing.T) {
	observer := &capturingToolLifecycleObserver{}
	ctx := ContextWithToolLifecycleObserver(context.Background(), observer)
	middleware := &toolOrchestratorMiddleware{agentKind: AgentKindIDE, toolResultMaxBytes: 256}
	endpoint, err := middleware.WrapToolCall(context.Background(),
		func(context.Context, string, ...agent.ToolOption) (agent.ToolResult, error) {
			result := agent.TextToolResult("looks successful")
			result.Details = []byte(`{"broken"`)
			return result, nil
		},
		testToolContext("read", "call-invalid-details"),
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := endpoint(ctx, `{"path":"broken.txt"}`)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != agent.ToolResultError || len(result.Details) != 0 || !strings.Contains(result.ModelContent, "Invalid structured tool result") {
		t.Fatalf("invalid Details result = %#v", result)
	}
	if len(observer.records) != 1 || observer.records[0].Status != string(agent.ToolResultError) {
		t.Fatalf("invalid Details lifecycle record = %#v", observer.records)
	}
}
