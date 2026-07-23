package agent

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type capturingToolLifecycleObserver struct {
	records []ToolExecutionRecord
}

func (*capturingToolLifecycleObserver) BeforeTool(context.Context, ToolDecision, string) error {
	return nil
}

func (o *capturingToolLifecycleObserver) AfterTool(_ context.Context, record ToolExecutionRecord) error {
	o.records = append(o.records, record)
	return nil
}

func TestToolOrchestratorBoundsInvokableEndpointErrorsForModelAndPersistence(t *testing.T) {
	const tail = "END_OF_UNBOUNDED_ERROR"
	hugeError := errors.New(strings.Repeat("巨大错误", 5000) + tail)
	observer := &capturingToolLifecycleObserver{}
	ctx := ContextWithToolLifecycleObserver(context.Background(), observer)
	middleware := &toolOrchestratorMiddleware{agentKind: AgentKindIDE, toolResultMaxBytes: 64}
	endpoint, err := middleware.WrapInvokableToolCall(
		context.Background(),
		func(context.Context, string, ...tool.Option) (string, error) {
			return "", hugeError
		},
		&adk.ToolContext{Name: "read_file", CallID: "call-error"},
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := endpoint(ctx, `{"path":"large.txt"}`)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"[tool result truncated]", "schema: tool_result.v1", "truncated: true"} {
		if !strings.Contains(result, want) {
			t.Fatalf("bounded model error is missing %q: %q", want, result)
		}
	}
	if strings.Contains(result, tail) {
		t.Fatalf("unbounded error tail entered model context: %q", result)
	}
	if len(observer.records) != 1 {
		t.Fatalf("lifecycle records = %d, want 1", len(observer.records))
	}
	record := observer.records[0]
	if record.Result != result || !record.Truncated || record.ReturnedBytes != len(result) {
		t.Fatalf("persisted result did not use the model projection: %#v", record)
	}
	if len(record.Error) > maxToolErrorDiagnosticBytes || !utf8.ValidString(record.Error) || strings.Contains(record.Error, tail) {
		t.Fatalf("persisted error diagnostic is not safely bounded: bytes=%d valid_utf8=%t tail=%t", len(record.Error), utf8.ValidString(record.Error), strings.Contains(record.Error, tail))
	}
}

func TestToolOrchestratorNormalizesInvalidUTF8InEndpointErrors(t *testing.T) {
	invalidError := errors.New("broken\xffdiagnostic")
	observer := &capturingToolLifecycleObserver{}
	ctx := ContextWithToolLifecycleObserver(context.Background(), observer)
	middleware := &toolOrchestratorMiddleware{agentKind: AgentKindIDE, toolResultMaxBytes: 256}
	endpoint, err := middleware.WrapInvokableToolCall(
		context.Background(),
		func(context.Context, string, ...tool.Option) (string, error) {
			return "", invalidError
		},
		&adk.ToolContext{Name: "read_file", CallID: "call-invalid-utf8-error"},
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := endpoint(ctx, `{"path":"broken.txt"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !utf8.ValidString(result) || len(observer.records) != 1 || !utf8.ValidString(observer.records[0].Error) {
		t.Fatalf("tool error projection must be valid UTF-8: result_valid=%t records=%#v", utf8.ValidString(result), observer.records)
	}
}

func TestStreamableToolNeverInfersMutationReceiptFromTruncatedPreview(t *testing.T) {
	receipt := `{"schema":"workspace_change.tool_result.v1","status":"applied","workspace":"/workspace/book-a","change_group_id":"group-1","path":"chapters/from-receipt.md","revision":"sha256:after"}`
	limit := len(receipt) + 8
	observer := &capturingToolLifecycleObserver{}
	ctx := ContextWithToolLifecycleObserver(context.Background(), observer)
	middleware := &toolOrchestratorMiddleware{agentKind: AgentKindIDE, toolResultMaxBytes: limit}
	endpoint, err := middleware.WrapStreamableToolCall(
		context.Background(),
		func(context.Context, string, ...tool.Option) (*schema.StreamReader[string], error) {
			// The retained prefix is valid receipt JSON plus whitespace even though
			// the actual stream is larger. Parsing that prefix would falsely treat
			// a display truncation boundary as an execution receipt boundary.
			return singleChunkReader(receipt + strings.Repeat(" ", limit+64)), nil
		},
		&adk.ToolContext{Name: "write_file", CallID: "call-truncated-receipt"},
	)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := endpoint(ctx, `{"path":"chapters/from-args.md"}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reader.Recv(); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.Recv(); !errors.Is(err, io.EOF) {
		t.Fatalf("stream EOF = %v", err)
	}
	if len(observer.records) != 1 {
		t.Fatalf("lifecycle records = %d, want 1", len(observer.records))
	}
	record := observer.records[0]
	if !record.Truncated || record.ChangeGroupID != "" || record.Workspace != "" || record.Revision != "" {
		t.Fatalf("truncated preview was treated as a complete receipt: %#v", record)
	}
	mutation, ok := toolMutationFromExecutionRecord(record)
	if !ok || mutation.Target != "chapters/from-args.md" || mutation.ToolCallID != "call-truncated-receipt" {
		t.Fatalf("conservative args-derived mutation = %#v ok=%t", mutation, ok)
	}
}

func TestToolOrchestratorBoundsStreamableEndpointErrorsForModelAndPersistence(t *testing.T) {
	const tail = "END_OF_STREAM_ENDPOINT_ERROR"
	hugeError := errors.New(strings.Repeat("流式错误", 5000) + tail)
	observer := &capturingToolLifecycleObserver{}
	ctx := ContextWithToolLifecycleObserver(context.Background(), observer)
	middleware := &toolOrchestratorMiddleware{agentKind: AgentKindIDE, toolResultMaxBytes: 64}
	endpoint, err := middleware.WrapStreamableToolCall(
		context.Background(),
		func(context.Context, string, ...tool.Option) (*schema.StreamReader[string], error) {
			return nil, hugeError
		},
		&adk.ToolContext{Name: "read_file", CallID: "call-stream-error"},
	)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := endpoint(ctx, `{"path":"large.txt"}`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := reader.Recv()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reader.Recv(); !errors.Is(err, io.EOF) {
		t.Fatalf("stream error = %v, want EOF", err)
	}
	if !strings.Contains(result, "[tool result truncated]") || !strings.Contains(result, "schema: tool_result.v1") || strings.Contains(result, tail) {
		t.Fatalf("stream endpoint error did not use bounded model projection: %q", result)
	}
	if len(observer.records) != 1 {
		t.Fatalf("lifecycle records = %d, want 1", len(observer.records))
	}
	record := observer.records[0]
	if record.Result != result || !record.Truncated || len(record.Error) > maxToolErrorDiagnosticBytes || strings.Contains(record.Error, tail) {
		t.Fatalf("stream endpoint error was persisted without bounds: %#v", record)
	}
}

func TestToolOrchestratorBoundsStreamReadErrorsForModelAndPersistence(t *testing.T) {
	const tail = "END_OF_STREAM_READ_ERROR"
	hugeError := errors.New(strings.Repeat("读取错误", 5000) + tail)
	observer := &capturingToolLifecycleObserver{}
	ctx := ContextWithToolLifecycleObserver(context.Background(), observer)
	middleware := &toolOrchestratorMiddleware{agentKind: AgentKindIDE, toolResultMaxBytes: 64}
	endpoint, err := middleware.WrapStreamableToolCall(
		context.Background(),
		func(context.Context, string, ...tool.Option) (*schema.StreamReader[string], error) {
			reader, writer := schema.Pipe[string](1)
			_ = writer.Send("", hugeError)
			writer.Close()
			return reader, nil
		},
		&adk.ToolContext{Name: "read_file", CallID: "call-stream-read-error"},
	)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := endpoint(ctx, `{"path":"large.txt"}`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := reader.Recv()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reader.Recv(); !errors.Is(err, io.EOF) {
		t.Fatalf("stream error = %v, want EOF", err)
	}
	if !strings.Contains(result, "[tool result truncated]") || !strings.Contains(result, "schema: tool_result.v1") || strings.Contains(result, tail) {
		t.Fatalf("stream read error did not use bounded model projection: %q", result)
	}
	if len(observer.records) != 1 {
		t.Fatalf("lifecycle records = %d, want 1", len(observer.records))
	}
	record := observer.records[0]
	if record.Result != result || !record.Truncated || len(record.Error) > maxToolErrorDiagnosticBytes || strings.Contains(record.Error, tail) {
		t.Fatalf("stream read error was persisted without bounds: %#v", record)
	}
}

func TestToolOrchestratorProjectsNilStreamAsBoundedToolError(t *testing.T) {
	observer := &capturingToolLifecycleObserver{}
	ctx := ContextWithToolLifecycleObserver(context.Background(), observer)
	middleware := &toolOrchestratorMiddleware{agentKind: AgentKindIDE, toolResultMaxBytes: 64}
	endpoint, err := middleware.WrapStreamableToolCall(
		context.Background(),
		func(context.Context, string, ...tool.Option) (*schema.StreamReader[string], error) {
			return nil, nil
		},
		&adk.ToolContext{Name: "read_file", CallID: "call-nil-stream"},
	)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := endpoint(ctx, `{"path":"large.txt"}`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := reader.Recv()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "streamable tool returned a nil result stream") || !strings.Contains(result, "schema: tool_result.v1") {
		t.Fatalf("nil stream did not use the standard model projection: %q", result)
	}
	if len(observer.records) != 1 || observer.records[0].Result != result || observer.records[0].Error == "" {
		t.Fatalf("nil stream error was not recorded through the standard projection: %#v", observer.records)
	}
}

func TestToolOrchestratorProjectsStreamPanicsAsBoundedToolError(t *testing.T) {
	observer := &capturingToolLifecycleObserver{}
	ctx := ContextWithToolLifecycleObserver(context.Background(), observer)
	middleware := &toolOrchestratorMiddleware{agentKind: AgentKindIDE, toolResultMaxBytes: 64}
	endpoint, err := middleware.WrapStreamableToolCall(
		context.Background(),
		func(context.Context, string, ...tool.Option) (*schema.StreamReader[string], error) {
			return &schema.StreamReader[string]{}, nil
		},
		&adk.ToolContext{Name: "read_file", CallID: "call-panic-stream"},
	)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := endpoint(ctx, `{"path":"large.txt"}`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := reader.Recv()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "panic while reading tool result") || !strings.Contains(result, "schema: tool_result.v1") {
		t.Fatalf("stream panic did not use the standard model projection: %q", result)
	}
	if len(observer.records) != 1 || observer.records[0].Result != result || observer.records[0].Error == "" {
		t.Fatalf("stream panic was not recorded through the standard projection: %#v", observer.records)
	}
}
