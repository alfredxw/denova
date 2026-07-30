package agents

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
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
	ctx = agent.ContextWithToolArtifactStore(ctx, &processorArtifactStore{})
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

func TestToolOrchestratorPersistsLossyShellFailureBeforePropagatingControl(t *testing.T) {
	observer := &capturingToolLifecycleObserver{}
	ctx := ContextWithToolLifecycleObserver(context.Background(), observer)
	middleware := &toolOrchestratorMiddleware{agentKind: AgentKindIDE, toolResultMaxBytes: 256}
	toolCtx := testToolContext("bash", "call-lossy-shell")
	toolCtx.Definition.Descriptor = processorShellTestDecision().Descriptor
	endpoint, err := middleware.WrapToolCall(context.Background(),
		func(context.Context, string, ...agent.ToolOption) (agent.ToolResult, error) {
			return lossyShellArtifactFailureResult(), nil
		},
		toolCtx,
	)
	if err != nil {
		t.Fatal(err)
	}

	result, err := endpoint(ctx, `{"command":"produce output"}`)
	if !agent.IsToolControlError(err) {
		t.Fatalf("shell artifact error = %T %v", err, err)
	}
	if len(observer.records) != 1 {
		t.Fatalf("durable finish records = %#v", observer.records)
	}
	persistence := result.Metadata.ArtifactPersistence
	if result.Status != agent.ToolResultError || len(result.ModelContent) > 256 ||
		result.Metadata.OriginalModelBytes != 64*1024 || !result.Metadata.ModelTruncated ||
		persistence == nil || persistence.Complete || persistence.FailureReason != agent.ToolArtifactFailureCommit {
		t.Fatalf("projected shell failure = %#v", result)
	}
	record := observer.records[0]
	if record.Status != string(agent.ToolResultError) || record.OriginalBytes != 64*1024 ||
		!record.Truncated || record.Result != result.ModelContent {
		t.Fatalf("persisted shell failure = %#v", record)
	}
}

func TestLossyShellArtifactFailureStopsNativeAgentAfterDurableFinish(t *testing.T) {
	model := &lossyShellControlModel{}
	var shellCalls, laterCalls atomic.Int32
	shellTool, err := agent.InferTool("bash", "lossy shell fixture", func(context.Context, shellControlArgs) (agent.ToolResult, error) {
		shellCalls.Add(1)
		return lossyShellArtifactFailureResult(), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	laterTool, err := agent.InferTool("later_shell", "must not execute", func(context.Context, shellControlArgs) (agent.ToolResult, error) {
		laterCalls.Add(1)
		return agent.TextToolResult("unexpected"), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	descriptor := processorShellTestDecision().Descriptor
	middleware := &toolOrchestratorMiddleware{
		BaseMiddleware: &agent.BaseMiddleware{}, agentKind: AgentKindIDE, toolResultMaxBytes: 256,
	}
	built, err := agent.NewAgent(context.Background(), agent.AgentConfig{
		Name: "lossy-shell-control", Description: "lossy shell control regression",
		Instruction: "run requested tools", Model: model, MaxIterations: 3,
		Middlewares: []agent.Middleware{middleware},
		Tools: []agent.ToolDefinition{
			{Tool: shellTool, Descriptor: descriptor},
			{Tool: laterTool, Descriptor: descriptor},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	observer := &capturingToolLifecycleObserver{}
	ctx := ContextWithToolLifecycleObserver(context.Background(), observer)
	iterator := agent.NewRunner(agent.RunnerConfig{Agent: built, EnableStreaming: false}).Query(ctx, "run")
	var terminalErr error
	results := make(map[string]*agent.ToolResultSummary)
	for {
		event, ok := iterator.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			terminalErr = event.Err
		}
		if event.Output != nil && event.Output.MessageOutput != nil && event.Output.MessageOutput.Role == agent.ToolRole {
			message := event.Output.MessageOutput.Message
			results[message.ToolName] = message.ToolResult
		}
	}

	if !agent.IsToolControlError(terminalErr) || model.calls.Load() != 1 ||
		shellCalls.Load() != 1 || laterCalls.Load() != 0 || len(observer.records) != 1 {
		t.Fatalf("terminal=%v model=%d shell=%d later=%d records=%#v results=%#v",
			terminalErr, model.calls.Load(), shellCalls.Load(), laterCalls.Load(), observer.records, results)
	}
	shellResult := results["bash"]
	if shellResult == nil || shellResult.Status != agent.ToolResultError ||
		shellResult.ResultRetention != agent.ToolResultProtected ||
		shellResult.ArtifactPersistence == nil || shellResult.ArtifactPersistence.Complete ||
		!shellResult.ModelTruncated || observer.records[0].OriginalBytes != 64*1024 {
		t.Fatalf("paired lossy shell result = %#v", shellResult)
	}
	if !toolResultProtected(&agent.Message{Role: agent.ToolRole, ToolResult: shellResult}) {
		t.Fatalf("lossy protected shell became cleanup-eligible: %#v", shellResult)
	}
	if later := results["later_shell"]; later == nil || later.SyntheticReason != agent.ToolSyntheticPolicyBlocked {
		t.Fatalf("unstarted later shell result = %#v", later)
	}
}

type shellControlArgs struct {
	Command string `json:"command"`
}

type lossyShellControlModel struct{ calls atomic.Int32 }

func (model *lossyShellControlModel) Generate(context.Context, []*agent.Message, ...agent.ModelOption) (*agent.Message, error) {
	model.calls.Add(1)
	return agent.AssistantMessage("", []agent.ToolCall{
		{ID: "call-lossy-shell", Type: "function", Function: agent.FunctionCall{Name: "bash", Arguments: `{"command":"produce output"}`}},
		{ID: "call-later-shell", Type: "function", Function: agent.FunctionCall{Name: "later_shell", Arguments: `{"command":"must not run"}`}},
	}), nil
}

func (model *lossyShellControlModel) Stream(ctx context.Context, input []*agent.Message, options ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	message, err := model.Generate(ctx, input, options...)
	if err != nil {
		return nil, err
	}
	return agent.StreamReaderFromArray([]*agent.Message{message}), nil
}

func lossyShellArtifactFailureResult() agent.ToolResult {
	return agent.ToolResult{
		ModelContent:   `{"schema":"process.result.v1","output_truncated":true,"artifact_error":"commit_failed"}\nbounded head/tail preview`,
		DisplayContent: `{"schema":"process.result.v1","output_truncated":true,"artifact_error":"commit_failed"}\nbounded head/tail preview`,
		Details:        []byte(`{"schema":"process.result.v1","output_truncated":true,"artifact_error":"commit_failed"}`),
		Status:         agent.ToolResultSuccess,
		Metadata: agent.ToolResultMetadata{
			OriginalModelBytes: 64 * 1024, ModelTruncated: true,
			ArtifactPersistence: &agent.ToolArtifactPersistence{
				Attempted: true, Complete: false, FailureReason: agent.ToolArtifactFailureCommit,
			},
		},
	}
}
