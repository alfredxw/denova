package agents

import (
	"context"
	"io"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
)

func TestInteractiveStoryToolMiddlewareBlocksWriteTools(t *testing.T) {
	middleware := newInteractiveStoryToolMiddleware()
	called := false
	endpoint, err := middleware.WrapInvokableToolCall(
		context.Background(),
		func(context.Context, string, ...agent.ToolOption) (string, error) {
			called = true
			return "ok", nil
		},
		testToolContext("write_file", ""),
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := endpoint(context.Background(), `{"file_path":"/tmp/a"}`)
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("write_file should be blocked before endpoint is called")
	}
	if !strings.Contains(result, "游戏模式禁止使用写文件工具") {
		t.Fatalf("unexpected block result: %s", result)
	}
}

func TestInteractiveTurnReceiptRecordsDomainOutcomeSeparatelyFromTransport(t *testing.T) {
	record := ToolExecutionRecord{ToolName: interactiveTurnSubmissionToolName, Status: "success"}
	applyInteractiveTurnReceiptToExecutionRecord(&record, `{"ready":false,"module_status":{"state_changes":"rejected","choices":"accepted"},"diagnostics":[{"code":"invalid_module"}],"retry_modules":["state_changes"]}`)
	if record.Status != "success" || record.DomainStatus != "rejected" || record.DomainDiagnosticCount != 1 || len(record.RetryModules) != 1 || record.RetryModules[0] != "state_changes" {
		t.Fatalf("transport success should retain the rejected domain outcome: %#v", record)
	}

	accepted := ToolExecutionRecord{ToolName: interactiveTurnSubmissionToolName, Status: "success"}
	applyInteractiveTurnReceiptToExecutionRecord(&accepted, `{"ready":true,"module_status":{"state_changes":"accepted","choices":"accepted"}}`)
	if accepted.DomainStatus != "accepted" || accepted.DomainDiagnosticCount != 0 {
		t.Fatalf("ready receipt should be recorded as domain accepted: %#v", accepted)
	}
}

func TestInteractiveStoryToolMiddlewareAllowsReadTools(t *testing.T) {
	middleware := newInteractiveStoryToolMiddleware()
	called := false
	endpoint, err := middleware.WrapInvokableToolCall(
		context.Background(),
		func(context.Context, string, ...agent.ToolOption) (string, error) {
			called = true
			return "ok", nil
		},
		testToolContext("read_file", ""),
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := endpoint(context.Background(), `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if !called || result != "ok" {
		t.Fatalf("read_file should pass through, called=%v result=%s", called, result)
	}
}

func TestInteractiveDirectorPlanFileMiddlewareBlocksStateTools(t *testing.T) {
	middleware := newInteractiveDirectorPlanFileMiddleware()
	for _, name := range []string{"apply_actor_state_patch"} {
		called := false
		endpoint, err := middleware.WrapInvokableToolCall(
			context.Background(),
			func(context.Context, string, ...agent.ToolOption) (string, error) {
				called = true
				return "ok", nil
			},
			testToolContext(name, ""),
		)
		if err != nil {
			t.Fatal(err)
		}
		result, err := endpoint(context.Background(), `{}`)
		if err != nil {
			t.Fatal(err)
		}
		if called || !strings.Contains(result, "不能写 Actor State") {
			t.Fatalf("%s should be blocked, called=%v result=%s", name, called, result)
		}
	}
}

func TestInteractiveDirectorPlanMiddlewareAllowsStructuredSubmitAndBlocksFiles(t *testing.T) {
	middleware := newInteractiveDirectorPlanFileMiddleware()
	for _, tc := range []struct {
		name    string
		allowed bool
	}{
		{name: submitDirectorPlanUpdateToolName, allowed: true},
		{name: "read_file", allowed: false},
		{name: "write_file", allowed: false},
	} {
		called := false
		endpoint, err := middleware.WrapInvokableToolCall(
			context.Background(),
			func(context.Context, string, ...agent.ToolOption) (string, error) {
				called = true
				return "ok", nil
			},
			testToolContext(tc.name, ""),
		)
		if err != nil {
			t.Fatal(err)
		}
		result, err := endpoint(context.Background(), `{}`)
		if err != nil {
			t.Fatal(err)
		}
		if tc.allowed && (!called || result != "ok") {
			t.Fatalf("%s should pass through, called=%v result=%s", tc.name, called, result)
		}
		if !tc.allowed && (called || !strings.Contains(result, submitDirectorPlanUpdateToolName)) {
			t.Fatalf("%s should be blocked in favor of structured submit, called=%v result=%s", tc.name, called, result)
		}
	}
}

func TestInteractiveDirectorPlanFileMiddlewareBlocksUnauthorizedTools(t *testing.T) {
	middleware := newInteractiveDirectorPlanFileMiddleware()
	called := false
	endpoint, err := middleware.WrapInvokableToolCall(
		context.Background(),
		func(context.Context, string, ...agent.ToolOption) (string, error) {
			called = true
			return "ok", nil
		},
		testToolContext("execute_shell", ""),
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := endpoint(context.Background(), `{"cmd":"ls"}`)
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("unauthorized director tool should be blocked before endpoint is called")
	}
	if !strings.Contains(result, "拒绝工具: execute_shell") {
		t.Fatalf("unexpected block result: %s", result)
	}
}

func TestToolOrchestratorBlocksInteractiveWriteTools(t *testing.T) {
	middleware := &toolOrchestratorMiddleware{agentKind: AgentKindInteractiveStory}
	called := false
	endpoint, err := middleware.WrapInvokableToolCall(
		context.Background(),
		func(context.Context, string, ...agent.ToolOption) (string, error) {
			called = true
			return "ok", nil
		},
		testToolContext("write_file", "call-1"),
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := endpoint(context.Background(), `{"path":"chapters/ch01.md"}`)
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("interactive write tool should be blocked before endpoint is called")
	}
	if !strings.Contains(result, "游戏模式禁止使用写文件工具") {
		t.Fatalf("unexpected block result: %s", result)
	}
}

func TestToolOrchestratorBlocksInteractiveSubAgentWriteTools(t *testing.T) {
	middleware := &toolOrchestratorMiddleware{agentKind: "researcher", policyKind: AgentKindInteractiveStory}
	called := false
	endpoint, err := middleware.WrapInvokableToolCall(
		context.Background(),
		func(context.Context, string, ...agent.ToolOption) (string, error) {
			called = true
			return "ok", nil
		},
		testToolContext("write_file", "call-1"),
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := endpoint(context.Background(), `{"path":"chapters/ch01.md"}`)
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("interactive subagent write tool should be blocked before endpoint is called")
	}
	if !strings.Contains(result, "游戏模式禁止使用写文件工具") {
		t.Fatalf("unexpected block result: %s", result)
	}
}

func TestToolOrchestratorAllowsIDEWriteAndFiltersResult(t *testing.T) {
	middleware := &toolOrchestratorMiddleware{agentKind: AgentKindIDE}
	content := strings.Repeat("正文", 100)
	endpoint, err := middleware.WrapInvokableToolCall(
		context.Background(),
		func(context.Context, string, ...agent.ToolOption) (string, error) {
			return content, nil
		},
		testToolContext("write_file", "call-1"),
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := endpoint(context.Background(), `{"path":"chapters/ch01.md"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "schema: tool_result.v1") ||
		!strings.Contains(result, "mutates_workspace: true") ||
		!strings.Contains(result, "target: chapters/ch01.md") {
		t.Fatalf("result should include filtered metadata: %s", result)
	}
	if !strings.Contains(result, content) {
		t.Fatalf("result below the high default limit should stay complete")
	}
}

func TestToolOrchestratorTruncatesResultWhenLimitConfigured(t *testing.T) {
	middleware := &toolOrchestratorMiddleware{agentKind: AgentKindIDE, toolResultMaxBytes: 128}
	endpoint, err := middleware.WrapInvokableToolCall(
		context.Background(),
		func(context.Context, string, ...agent.ToolOption) (string, error) {
			return strings.Repeat("正文", 200), nil
		},
		testToolContext("write_file", "call-1"),
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := endpoint(context.Background(), `{"path":"chapters/ch01.md"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "[tool result truncated]") ||
		!strings.Contains(result, "truncated: true") {
		t.Fatalf("configured limit should truncate result: %s", result)
	}
}

func TestToolOrchestratorBlocksMalformedJSONArguments(t *testing.T) {
	middleware := &toolOrchestratorMiddleware{agentKind: AgentKindIDE}
	called := false
	endpoint, err := middleware.WrapInvokableToolCall(
		context.Background(),
		func(context.Context, string, ...agent.ToolOption) (string, error) {
			called = true
			return "ok", nil
		},
		testToolContext("write_file", "call-1"),
	)
	if err != nil {
		t.Fatal(err)
	}
	args := "{\"file_path\":\"chapters/ch01.md\",\"content\":\"过了一遍。\\\\n\\\\n韩十四。武监司。三十\n\t^\n\\"
	result, err := endpoint(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("malformed JSON arguments should be blocked before endpoint is called")
	}
	if !strings.Contains(result, "参数不是完整 JSON 对象") ||
		!strings.Contains(result, "Tool arguments must be a complete JSON object") {
		t.Fatalf("unexpected malformed-arguments result: %s", result)
	}
	if strings.Contains(result, "重新发起同一个工具调用") {
		t.Fatalf("malformed-arguments result should not force a same-tool retry: %s", result)
	}
}

func TestToolOrchestratorBlocksValidArgumentsWhenModelHitOutputLimit(t *testing.T) {
	for _, finishReason := range []string{"length", "max_tokens"} {
		t.Run(finishReason, func(t *testing.T) {
			observer := newRunObserver(nil, "root-span")
			observer.RecordLLMOutcome(LLMOutcome{
				FinishReason: finishReason, RequestedTools: []string{"write_file"},
			})
			ctx := ContextWithRunObserver(context.Background(), observer)
			middleware := &toolOrchestratorMiddleware{agentKind: AgentKindIDE}
			called := false
			endpoint, err := middleware.WrapInvokableToolCall(
				context.Background(),
				func(context.Context, string, ...agent.ToolOption) (string, error) {
					called = true
					return "ok", nil
				},
				testToolContext("write_file", "call-output-limit"),
			)
			if err != nil {
				t.Fatal(err)
			}
			result, err := endpoint(ctx, `{"path":"chapters/ch01.md","content":"valid but potentially truncated"}`)
			if err != nil {
				t.Fatal(err)
			}
			if called {
				t.Fatal("output-limited tool call executed even though complete intent was unknowable")
			}
			for _, want := range []string{
				"reason: model_output_token_limit", "retryable: true", "workspace_mutated: false",
				"args_complete: false", "model_finish_reason: " + finishReason,
				"即使 arguments 恰好是合法 JSON", "even though the remaining arguments may be valid JSON",
			} {
				if !strings.Contains(result, want) {
					t.Fatalf("output-limit result missing %q:\n%s", want, result)
				}
			}
		})
	}
}

func TestStreamableToolBlocksValidArgumentsWhenModelHitOutputLimit(t *testing.T) {
	observer := newRunObserver(nil, "root-span")
	observer.RecordLLMOutcome(LLMOutcome{FinishReason: "max_output_tokens", RequestedTools: []string{"read_file"}})
	ctx := ContextWithRunObserver(context.Background(), observer)
	middleware := &toolOrchestratorMiddleware{agentKind: AgentKindIDE}
	called := false
	endpoint, err := middleware.WrapStreamableToolCall(
		context.Background(),
		func(context.Context, string, ...agent.ToolOption) (*agent.StreamReader[string], error) {
			called = true
			return singleChunkReader("unsafe"), nil
		},
		testToolContext("read_file", "call-stream-output-limit"),
	)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := endpoint(ctx, `{"path":"chapters/ch01.md"}`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := reader.Recv()
	if err != nil {
		t.Fatal(err)
	}
	if called || !strings.Contains(result, "model_finish_reason: max_output_tokens") || !strings.Contains(result, "retryable: true") {
		t.Fatalf("stream output-limit gate called=%t result=%q", called, result)
	}
}

func TestToolOrchestratorReturnsContentFilterContextForIncompleteWriteArguments(t *testing.T) {
	workspace := t.TempDir()
	ledger, err := newRunLedger(workspace, RunLedgerPolicy{Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	observer := newRunObserver(ledger, "root-span")
	observer.RecordLLMOutcome(LLMOutcome{
		FinishReason:      "content_filter",
		RequestedTools:    []string{"write_file"},
		ProviderRequestID: "provider-1",
	})
	ctx := ContextWithRunObserver(context.Background(), observer)
	middleware := &toolOrchestratorMiddleware{agentKind: AgentKindIDE}
	called := false
	endpoint, err := middleware.WrapInvokableToolCall(
		context.Background(),
		func(context.Context, string, ...agent.ToolOption) (string, error) {
			called = true
			return "ok", nil
		},
		testToolContext("write_file", "call-content-filter"),
	)
	if err != nil {
		t.Fatal(err)
	}
	args := `{"file_path":"chapters/ch01.md","content":"正文被过滤中断`
	result, err := endpoint(ctx, args)
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("content-filter interrupted arguments should be blocked before endpoint is called")
	}
	for _, want := range []string{
		"reason: model_output_interrupted_by_content_filter",
		"retryable: false",
		"workspace_mutated: false",
		"args_complete: false",
		"model_finish_reason: content_filter",
		"target: chapters/ch01.md",
		"文件未写入",
		"do not retry the same tool call",
	} {
		if !strings.Contains(result, want) {
			t.Fatalf("content-filter context missing %q:\n%s", want, result)
		}
	}
	if strings.Contains(result, "重新发起同一个工具调用") {
		t.Fatalf("content-filter context should not force a same-tool retry: %s", result)
	}
	records := readRunLedgerRecords(t, ledger.Path())
	var decision map[string]any
	var toolAttrs map[string]any
	for _, record := range records {
		data, _ := record["data"].(map[string]any)
		switch record["type"] {
		case "tool_decision":
			decision, _ = data["decision"].(map[string]any)
		case "tool_call":
			toolAttrs, _ = data["attrs"].(map[string]any)
		}
	}
	if decision == nil || toolAttrs == nil {
		t.Fatalf("expected tool decision and trace span records: %#v", records)
	}
	if decision["model_finish_reason"] != "content_filter" || decision["args_complete"] != false {
		t.Fatalf("decision should record incomplete content-filter args: %#v", decision)
	}
	if got, _ := decision["args_bytes"].(float64); int(got) != len(args) {
		t.Fatalf("decision args_bytes = %v, want %d", decision["args_bytes"], len(args))
	}
	if toolAttrs["model_finish_reason"] != "content_filter" || toolAttrs["args_complete"] != false {
		t.Fatalf("tool span should record incomplete content-filter args: %#v", toolAttrs)
	}
}

func TestToolOrchestratorBlocksValidArgumentsWhenModelWasContentFiltered(t *testing.T) {
	observer := newRunObserver(nil, "root-span")
	observer.RecordLLMOutcome(LLMOutcome{FinishReason: "content_filter", RequestedTools: []string{"read_file"}})
	ctx := ContextWithRunObserver(context.Background(), observer)
	middleware := &toolOrchestratorMiddleware{agentKind: AgentKindIDE}
	called := false
	endpoint, err := middleware.WrapInvokableToolCall(
		context.Background(),
		func(context.Context, string, ...agent.ToolOption) (string, error) {
			called = true
			return "unsafe", nil
		},
		testToolContext("read_file", "call-valid-content-filter"),
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := endpoint(ctx, `{"path":"chapters/ch01.md"}`)
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("valid-looking tool arguments executed after content filtering")
	}
	for _, want := range []string{
		"reason: model_output_interrupted_by_content_filter", "retryable: false",
		"workspace_mutated: false", "args_complete: false", "model_finish_reason: content_filter",
		"即使 arguments 恰好是合法 JSON", "even if the remaining arguments happen to be valid JSON",
	} {
		if !strings.Contains(result, want) {
			t.Fatalf("content-filter result missing %q:\n%s", want, result)
		}
	}
}

func TestToolPathFromArgsExtractsPartialFilePath(t *testing.T) {
	args := `{"file_path":"chapters/ch01.md","content":"正文还没闭合`
	if got := toolPathFromArgs(args); got != "chapters/ch01.md" {
		t.Fatalf("partial file_path = %q, want chapters/ch01.md", got)
	}
}

func TestToolOrchestratorAllowsEscapedSpecialCharactersInJSONArguments(t *testing.T) {
	middleware := &toolOrchestratorMiddleware{agentKind: AgentKindIDE}
	called := false
	endpoint, err := middleware.WrapInvokableToolCall(
		context.Background(),
		func(context.Context, string, ...agent.ToolOption) (string, error) {
			called = true
			return "ok", nil
		},
		testToolContext("write_file", "call-1"),
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := endpoint(context.Background(), `{"file_path":"chapters/ch01.md","content":"过了一遍。\\n\\n韩十四。武监司。三十\n\t^\n\""}`)
	if err != nil {
		t.Fatal(err)
	}
	if !called || !strings.Contains(result, "ok") {
		t.Fatalf("escaped special characters should pass through, called=%v result=%s", called, result)
	}
}

func TestToolOrchestratorBlocksMalformedJSONArgumentsForStream(t *testing.T) {
	middleware := &toolOrchestratorMiddleware{agentKind: AgentKindIDE}
	called := false
	endpoint, err := middleware.WrapStreamableToolCall(
		context.Background(),
		func(context.Context, string, ...agent.ToolOption) (*agent.StreamReader[string], error) {
			called = true
			return singleChunkReader("ok"), nil
		},
		testToolContext("write_file", "call-1"),
	)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := endpoint(context.Background(), "{\"file_path\":\"chapters/ch01.md\",\"content\":\"过了一遍\n\t^\n")
	if err != nil {
		t.Fatal(err)
	}
	result, recvErr := reader.Recv()
	if recvErr != nil {
		t.Fatal(recvErr)
	}
	if _, eofErr := reader.Recv(); eofErr != io.EOF {
		t.Fatalf("expected stream EOF after block message, got %v", eofErr)
	}
	if called {
		t.Fatal("malformed JSON stream arguments should be blocked before endpoint is called")
	}
	if !strings.Contains(result, "参数不是完整 JSON 对象") {
		t.Fatalf("unexpected malformed-arguments stream result: %s", result)
	}
}

func TestToolOrchestratorBlocksDisabledCapability(t *testing.T) {
	middleware := &toolOrchestratorMiddleware{
		agentKind:           AgentKindIDE,
		enforceToolSettings: true,
		toolSettings:        config.ResolvedAgentToolSettings{FileRead: true},
	}
	called := false
	endpoint, err := middleware.WrapInvokableToolCall(
		context.Background(),
		func(context.Context, string, ...agent.ToolOption) (string, error) {
			called = true
			return "ok", nil
		},
		testToolContext("write_file", "call-1"),
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := endpoint(context.Background(), `{"file_path":"chapters/ch01.md"}`)
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("disabled file_write capability should block before endpoint is called")
	}
	if !strings.Contains(result, "file_write") || !strings.Contains(result, "disabled for this Agent") {
		t.Fatalf("unexpected disabled capability result: %s", result)
	}
}

func TestToolOrchestratorBlocksUndeclaredDynamicTool(t *testing.T) {
	middleware := &toolOrchestratorMiddleware{
		agentKind: AgentKindIDE, enforceToolSettings: true,
		toolSettings: config.ResolvedAgentToolSettings{FileRead: true, FileWrite: true},
	}
	decision := middleware.buildToolDecision(testToolContext("dynamic_unknown", "call-unknown"), `{}`)
	if decision.Action != "blocked" || !strings.Contains(decision.Reason, "ToolDescriptor") {
		t.Fatalf("undeclared dynamic tool decision = %#v", decision)
	}
	knownWithoutCapability := middleware.buildToolDecision(testToolContext("search_story_history", ""), `{}`)
	if knownWithoutCapability.Action != "allowed" {
		t.Fatalf("declared capability-free tool should remain allowed: %#v", knownWithoutCapability)
	}
}

func TestToolOrchestratorTruncatesStreamResultWhenLimitConfigured(t *testing.T) {
	middleware := &toolOrchestratorMiddleware{agentKind: AgentKindIDE, toolResultMaxBytes: 64}
	endpoint, err := middleware.WrapStreamableToolCall(
		context.Background(),
		func(context.Context, string, ...agent.ToolOption) (*agent.StreamReader[string], error) {
			return singleChunkReader(strings.Repeat("流式正文", 100)), nil
		},
		testToolContext("read_file", "call-1"),
	)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := endpoint(context.Background(), `{"path":"chapters/ch01.md"}`)
	if err != nil {
		t.Fatal(err)
	}
	result, recvErr := reader.Recv()
	if recvErr != nil {
		t.Fatal(recvErr)
	}
	if _, eofErr := reader.Recv(); eofErr != io.EOF {
		t.Fatalf("expected stream EOF after filtered result, got %v", eofErr)
	}
	if !strings.Contains(result, "[tool result truncated]") ||
		!strings.Contains(result, "truncated: true") {
		t.Fatalf("configured stream limit should truncate result: %s", result)
	}
}

func TestFilesystemToolsKeepStableSchemaAcrossSettings(t *testing.T) {
	workspace := t.TempDir()
	tools, err := newToolCatalog(&config.Config{Workspace: workspace}).Filesystem(config.ResolvedAgentToolSettings{
		FileRead:     true,
		FileWrite:    false,
		ShellExecute: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, item := range tools {
		info, err := item.Info(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		names[info.Name] = true
	}
	for _, name := range []string{"ls", "read_file", "glob", "grep"} {
		if !names[name] {
			t.Fatalf("read tool %s should be registered, names=%v", name, names)
		}
	}
	for _, name := range []string{"write_file", "edit_file", "execute"} {
		if !names[name] {
			t.Fatalf("tool %s should keep a stable schema and be blocked by orchestrator, names=%v", name, names)
		}
	}
}
