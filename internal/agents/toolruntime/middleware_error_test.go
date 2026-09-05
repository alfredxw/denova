package toolruntime

import (
	"context"
	agentrun "denova/internal/agents/run"
	agenttool "denova/internal/agents/tool"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"unicode/utf8"

	agent "github.com/alfredxw/denova/agent"
	agentpermission "github.com/alfredxw/denova/agent/permission"
	agenttoolresult "github.com/alfredxw/denova/agent/toolresult"
)

type runObserverMiddleware struct {
	*agent.BaseMiddleware
	observer *agentrun.Observer
}

func (middleware *runObserverMiddleware) WrapToolCall(
	_ context.Context,
	endpoint agent.ToolCallEndpoint,
	_ *agent.ToolContext,
) (agent.ToolCallEndpoint, error) {
	return func(ctx context.Context, arguments string, options ...agent.ToolOption) (agent.ToolResult, error) {
		return endpoint(agentrun.ContextWithObserver(ctx, middleware.observer), arguments, options...)
	}, nil
}

func runPublicToolLifecycle(
	t *testing.T,
	definition agent.Definition,
	observer *agentrun.Observer,
) (agent.Result, error, map[string]agent.ToolResult) {
	t.Helper()
	definition.Middlewares = append([]agent.Middleware{
		&runObserverMiddleware{BaseMiddleware: &agent.BaseMiddleware{}, observer: observer},
	}, definition.Middlewares...)
	definition.Permission = agentpermission.FullAccess()
	owner, err := agent.New(context.Background(), definition)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	run, err := owner.Run(context.Background(), agent.Input{
		Text: "run", IdempotencyKey: "public-tool-lifecycle-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	results := make(map[string]agent.ToolResult)
	for event := range run.Events() {
		finished, ok := event.Payload.(agent.ToolFinished)
		if ok && finished.Projection != nil {
			results[finished.Name] = *finished.Projection
		}
	}
	result, waitErr := run.Wait(context.Background())
	return result, waitErr, results
}

func TestToolOrchestratorKeepsEndpointErrorsLosslessForFixedProcessorAndBoundsAudit(t *testing.T) {
	const tail = "END_OF_UNBOUNDED_ERROR"
	hugeError := errors.New(strings.Repeat("巨大错误", 5000) + tail)
	observer := agentrun.NewObserver(nil, "")
	ctx := agentrun.ContextWithObserver(context.Background(), observer)
	middleware := &OrchestratorMiddleware{agentKind: agentrun.AgentKindIDE, toolResultMaxBytes: 64}
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
	if result.Status != agent.ToolResultError || result.Metadata.ModelTruncated || result.Metadata.DisplayTruncated {
		t.Fatalf("structured error result = %#v", result)
	}
	if !strings.Contains(result.ModelContent, tail) {
		t.Fatalf("model-visible endpoint error was truncated before the fixed processor: model=%d", len(result.ModelContent))
	}
	if len(result.DisplayContent) > maxToolErrorDiagnosticBytes || !utf8.ValidString(result.DisplayContent) || strings.Contains(result.DisplayContent, tail) {
		t.Fatalf("display diagnostic was not independently bounded: bytes=%d valid=%t", len(result.DisplayContent), utf8.ValidString(result.DisplayContent))
	}
	if len(observer.ToolExecutions()) != 1 {
		t.Fatalf("lifecycle records = %d, want 1", len(observer.ToolExecutions()))
	}
	record := observer.ToolExecutions()[0]
	if !record.Truncated || record.ReturnedBytes != len(record.Result) || len(record.Result) > 64 || strings.Contains(record.Result, tail) {
		t.Fatalf("audit result was not independently bounded: %#v", record)
	}
	if len(record.Error) > maxToolErrorDiagnosticBytes || !utf8.ValidString(record.Error) || strings.Contains(record.Error, tail) {
		t.Fatalf("persisted diagnostic is not bounded: bytes=%d valid=%t", len(record.Error), utf8.ValidString(record.Error))
	}
}

func TestToolOrchestratorNormalizesInvalidUTF8InEndpointErrors(t *testing.T) {
	invalidError := errors.New("broken\xffdiagnostic")
	observer := agentrun.NewObserver(nil, "")
	ctx := agentrun.ContextWithObserver(context.Background(), observer)
	middleware := &OrchestratorMiddleware{agentKind: agentrun.AgentKindIDE, toolResultMaxBytes: 256}
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
	if !utf8.ValidString(result.ModelContent) || !utf8.ValidString(result.DisplayContent) || len(observer.ToolExecutions()) != 1 || !utf8.ValidString(observer.ToolExecutions()[0].Error) {
		t.Fatalf("tool error projection must be valid UTF-8: result=%#v records=%#v", result, observer.ToolExecutions())
	}
}

func TestToolOrchestratorUsesDetailsForMutationReceiptWithoutTruncatingProcessorInput(t *testing.T) {
	receipt := `{"schema":"workspace_change.tool_result.v1","status":"applied","workspace":"/workspace/book-a","change_group_id":"group-1","change_set_id":"change-1","path":"chapters/from-receipt.md","revision":"sha256:after"}`
	observer := agentrun.NewObserver(nil, "")
	ctx := agentrun.ContextWithObserver(context.Background(), observer)
	ctx = agent.ContextWithToolArtifactStore(ctx, &processorArtifactStore{})
	middleware := &OrchestratorMiddleware{agentKind: agentrun.AgentKindIDE, toolResultMaxBytes: len(receipt) + 8}
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
	if result.Metadata.ModelTruncated || len(result.ModelContent) <= len(receipt)+8 || len(observer.ToolExecutions()) != 1 {
		t.Fatalf("expected lossless result and bounded lifecycle record: result=%#v records=%#v", result, observer.ToolExecutions())
	}
	record := observer.ToolExecutions()[0]
	if !record.Truncated || len(record.Result) > len(receipt)+8 {
		t.Fatalf("lifecycle audit projection was not bounded: %#v", record)
	}
	if record.ChangeGroupID != "group-1" || record.Workspace != "/workspace/book-a" || record.Revision != "sha256:after" || record.Target != "chapters/from-receipt.md" {
		t.Fatalf("Details receipt was not authoritative: %#v", record)
	}
}

func TestToolOrchestratorPreservesCommittedReceiptWhenEndpointAlsoErrors(t *testing.T) {
	receipt := `{"schema":"workspace_change.tool_result.v1","status":"applied","workspace":"/workspace/book-a","change_group_id":"group-1","change_set_id":"change-1","path":"chapters/committed.md"}`
	observer := agentrun.NewObserver(nil, "")
	ctx := agentrun.ContextWithObserver(context.Background(), observer)
	middleware := &OrchestratorMiddleware{agentKind: agentrun.AgentKindIDE, toolResultMaxBytes: 1024}
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
	if result.Status != agent.ToolResultError || len(observer.ToolExecutions()) != 1 {
		t.Fatalf("result=%#v records=%#v", result, observer.ToolExecutions())
	}
	record := observer.ToolExecutions()[0]
	mutation, committed := agenttool.MutationFromExecutionRecord(record)
	if !committed || record.Status != "error" || mutation.Target != "chapters/committed.md" {
		t.Fatalf("error receipt was not committed: record=%#v mutation=%#v committed=%t", record, mutation, committed)
	}
}

func TestToolOrchestratorDefersInvalidDetailsNormalizationToFixedProcessor(t *testing.T) {
	observer := agentrun.NewObserver(nil, "")
	ctx := agentrun.ContextWithObserver(context.Background(), observer)
	middleware := &OrchestratorMiddleware{agentKind: agentrun.AgentKindIDE, toolResultMaxBytes: 256}
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
	if result.Status != agent.ToolResultSuccess || string(result.Details) != `{"broken"` || result.ModelContent != "looks successful" {
		t.Fatalf("middleware altered the fixed processor input: %#v", result)
	}
	if len(observer.ToolExecutions()) != 1 || observer.ToolExecutions()[0].Status != string(agent.ToolResultError) {
		t.Fatalf("invalid Details lifecycle record = %#v", observer.ToolExecutions())
	}
}

func TestToolOrchestratorPreservesLossyShellEvidenceForFixedProcessor(t *testing.T) {
	observer := agentrun.NewObserver(nil, "")
	ctx := agentrun.ContextWithObserver(context.Background(), observer)
	middleware := &OrchestratorMiddleware{agentKind: agentrun.AgentKindIDE, toolResultMaxBytes: 256}
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
	if err != nil {
		t.Fatal(err)
	}
	if len(observer.ToolExecutions()) != 1 {
		t.Fatalf("tool diagnostics = %#v", observer.ToolExecutions())
	}
	persistence := result.Metadata.ArtifactPersistence
	if result.Status != agent.ToolResultSuccess ||
		result.Metadata.OriginalModelBytes != 64*1024 || !result.Metadata.ModelTruncated ||
		persistence == nil || persistence.Complete || persistence.FailureReason != agent.ToolArtifactFailureCommit {
		t.Fatalf("projected shell failure = %#v", result)
	}
	record := observer.ToolExecutions()[0]
	if record.Status != string(agent.ToolResultSuccess) || record.OriginalBytes != 64*1024 ||
		!record.Truncated || record.Result != result.ModelContent {
		t.Fatalf("persisted shell failure = %#v", record)
	}
}

func TestLossyShellArtifactFailureStopsPublicAgentAfterDiagnosticProjection(t *testing.T) {
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
	middleware := &OrchestratorMiddleware{
		BaseMiddleware: &agent.BaseMiddleware{}, agentKind: agentrun.AgentKindIDE, toolResultMaxBytes: 256,
	}
	toolset, err := agent.StaticTools(
		agent.ToolDefinition{Tool: shellTool, Descriptor: descriptor},
		agent.ToolDefinition{Tool: laterTool, Descriptor: descriptor},
	)
	if err != nil {
		t.Fatal(err)
	}
	definition := agent.Definition{
		Name: "lossy-shell-control", Description: "lossy shell control regression",
		Instructions: "run requested tools", Model: model,
		Middlewares:     []agent.Middleware{middleware},
		ResultProcessor: agenttoolresult.Standard(agenttoolresult.Policy{MaxBytes: 256}),
		Tools:           toolset,
		Execution:       agent.ExecutionPolicy{MaxIterations: 3},
	}
	observer := agentrun.NewObserver(nil, "")
	result, terminalErr, results := runPublicToolLifecycle(t, definition, observer)

	if terminalErr == nil || result.Status != agent.ResultFailed ||
		!strings.Contains(result.Reason, agent.ToolArtifactFailureCommit) || model.calls.Load() != 1 ||
		shellCalls.Load() != 1 || laterCalls.Load() != 0 || len(observer.ToolExecutions()) != 1 {
		t.Fatalf("result=%#v terminal=%v model=%d shell=%d later=%d records=%#v results=%#v",
			result, terminalErr, model.calls.Load(), shellCalls.Load(), laterCalls.Load(), observer.ToolExecutions(), results)
	}
	shellResult, found := results["bash"]
	if !found || shellResult.Status != agent.ToolResultError ||
		shellResult.ResultRetention != agent.ToolResultProtected ||
		shellResult.Metadata.ArtifactPersistence == nil || shellResult.Metadata.ArtifactPersistence.Complete ||
		!shellResult.Metadata.ModelTruncated || observer.ToolExecutions()[0].OriginalBytes != 64*1024 {
		t.Fatalf("paired lossy shell result = %#v", shellResult)
	}
	if later, found := results["later_shell"]; !found || later.SyntheticReason != agent.ToolSyntheticPolicyBlocked {
		t.Fatalf("unstarted later shell result = %#v", later)
	}
}

func TestOrchestratorKeepsLargeResultLosslessUntilPublicProcessor(t *testing.T) {
	const limit = 256
	for _, test := range []struct {
		name, toolName, arguments, details string
	}{
		{name: "ordinary read", toolName: "read", arguments: `{"path":"large.txt"}`},
		{
			name: "workspace mutation", toolName: "write", arguments: `{"path":"chapters/large.md"}`,
			details: `{"schema":"workspace_change.tool_result.v1","status":"applied","workspace":"/workspace/book","change_group_id":"group-large","change_set_id":"change-large","path":"chapters/large.md","review_status":"pending","apply_state":"applied"}`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			content := "HEAD-SENTINEL\n" + strings.Repeat("complete output ", 100) + "\nTAIL-SENTINEL"
			observer := agentrun.NewObserver(nil, "")
			middleware := &OrchestratorMiddleware{
				BaseMiddleware: &agent.BaseMiddleware{}, agentKind: agentrun.AgentKindIDE, toolResultMaxBytes: limit,
			}
			tool, err := agent.InferTool(test.toolName, "large output fixture", func(context.Context, largeToolArgs) (agent.ToolResult, error) {
				result := agent.TextToolResult(content)
				if test.details != "" {
					result.Details = []byte(test.details)
				}
				return result, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			descriptor := testToolContext(test.toolName, "call-large").Definition.Descriptor
			descriptor.MaxResultBytes = limit
			store := &processorArtifactStore{}
			artifacts, err := agent.IdentifyToolArtifactStorage(store, agent.CapabilityIdentity{
				Kind: "test.denova.tool_artifacts", Version: 1, ConfigHash: test.toolName,
			})
			if err != nil {
				t.Fatal(err)
			}
			model := &oneToolThenFinalModel{toolName: test.toolName, arguments: test.arguments}
			toolset, err := agent.StaticTools(agent.ToolDefinition{Tool: tool, Descriptor: descriptor})
			if err != nil {
				t.Fatal(err)
			}
			definition := agent.Definition{
				Name: "lossless-denova-chain", Model: model,
				Tools:       toolset,
				Middlewares: []agent.Middleware{middleware}, Artifacts: artifacts,
				ResultProcessor: agenttoolresult.Standard(agenttoolresult.Policy{MaxBytes: limit}),
			}
			if test.toolName == "write" {
				definition.Effects = agent.EffectApplierFuncs{
					CapabilityIdentity: agent.CapabilityIdentity{Kind: "test.denova.effects", Version: 1},
					ApplyEffectsFn: func(_ context.Context, requests []agent.EffectRequest) ([]agent.EffectResult, error) {
						results := make([]agent.EffectResult, len(requests))
						for index, request := range requests {
							results[index] = agent.EffectResult{ID: request.ID, Revision: "effect"}
						}
						return results, nil
					},
				}
			}
			settled, runErr, results := runPublicToolLifecycle(t, definition, observer)
			if runErr != nil || settled.Status != agent.ResultCompleted {
				t.Fatalf("public run result=%#v error=%v", settled, runErr)
			}
			result, found := results[test.toolName]
			if store.content.String() != content {
				t.Fatalf("artifact lost bytes: got=%d want=%d", store.content.Len(), len(content))
			}
			if !found || !result.Metadata.ModelTruncated || len(result.Artifacts) != 1 ||
				!strings.Contains(result.ModelContent, "HEAD-SENTINEL") || !strings.Contains(result.ModelContent, "TAIL-SENTINEL") {
				t.Fatalf("processed result = %#v", result)
			}
			if len(observer.ToolExecutions()) != 1 || observer.ToolExecutions()[0].OriginalBytes != len(content) ||
				len(observer.ToolExecutions()[0].Result) > limit {
				t.Fatalf("bounded audit record = %#v", observer.ToolExecutions())
			}
			if test.toolName == "write" && observer.ToolExecutions()[0].ChangeSetID != "change-large" {
				t.Fatalf("workspace receipt = %#v", observer.ToolExecutions()[0])
			}
		})
	}
}

type shellControlArgs struct {
	Command string `json:"command"`
}

type largeToolArgs struct {
	Path string `json:"path"`
}

type oneToolThenFinalModel struct {
	calls     atomic.Int32
	toolName  string
	arguments string
}

func (model *oneToolThenFinalModel) Generate(context.Context, []*agent.Message, ...agent.ModelOption) (*agent.Message, error) {
	if model.calls.Add(1) == 1 {
		return agent.AssistantMessage("", []agent.ToolCall{{
			ID: "call-large", Type: "function",
			Function: agent.FunctionCall{Name: model.toolName, Arguments: model.arguments},
		}}), nil
	}
	return agent.AssistantMessage("done", nil), nil
}

func (model *oneToolThenFinalModel) Stream(ctx context.Context, input []*agent.Message, options ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	message, err := model.Generate(ctx, input, options...)
	if err != nil {
		return nil, err
	}
	return agent.StreamReaderFromArray([]*agent.Message{message}), nil
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
