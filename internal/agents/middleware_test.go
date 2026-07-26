package agents

import (
	"context"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	"denova/internal/interactive"
)

func TestInteractiveStoryToolMiddlewareBlocksWorkspaceAndHostMutations(t *testing.T) {
	middleware := newInteractiveStoryToolMiddleware()
	for _, name := range []string{"write", "edit", "bash", "pwsh"} {
		called := false
		endpoint, err := wrapTextToolCallForTest(middleware,
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
		if called || !strings.Contains(result, "workspace 或宿主副作用") {
			t.Fatalf("%s should be blocked before endpoint, called=%t result=%s", name, called, result)
		}
	}
}

func TestInteractiveTurnReceiptRecordsDomainOutcomeSeparatelyFromTransport(t *testing.T) {
	record := ToolExecutionRecord{ToolName: interactiveTurnSubmissionToolName, Status: "success"}
	applyInteractiveTurnReceiptToExecutionRecord(&record, agent.ToolResult{Details: []byte(`{"ready":false,"module_status":{"state_changes":"rejected","choices":"accepted"},"diagnostics":[{"code":"invalid_module"}],"retry_modules":["state_changes"]}`)})
	if record.Status != "success" || record.DomainStatus != "rejected" || record.DomainDiagnosticCount != 1 || len(record.RetryModules) != 1 || record.RetryModules[0] != "state_changes" {
		t.Fatalf("transport success should retain the rejected domain outcome: %#v", record)
	}

	accepted := ToolExecutionRecord{ToolName: interactiveTurnSubmissionToolName, Status: "success"}
	applyInteractiveTurnReceiptToExecutionRecord(&accepted, agent.ToolResult{Details: []byte(`{"ready":true,"module_status":{"state_changes":"accepted","choices":"accepted"}}`)})
	if accepted.DomainStatus != "accepted" || accepted.DomainDiagnosticCount != 0 {
		t.Fatalf("ready receipt should be recorded as domain accepted: %#v", accepted)
	}
}

func TestInteractiveStoryToolMiddlewareAllowsReadTools(t *testing.T) {
	middleware := newInteractiveStoryToolMiddleware()
	called := false
	endpoint, err := wrapTextToolCallForTest(middleware,
		func(context.Context, string, ...agent.ToolOption) (string, error) {
			called = true
			return "ok", nil
		},
		testToolContext("read", ""),
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := endpoint(context.Background(), `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if !called || result != "ok" {
		t.Fatalf("read should pass through, called=%v result=%s", called, result)
	}
}

func TestInteractiveStoryToolMiddlewareAllowsDomainWorkflowMutations(t *testing.T) {
	definitions, err := newToolCatalog(nil).InteractiveStory(projectInteractiveToolContext(InteractiveStoryToolContext{
		PrepareTurn: func(context.Context, interactive.TurnCheckRequest) (interactive.RuleResolution, error) {
			return interactive.RuleResolution{}, nil
		},
		SubmitTurnResult: func(context.Context, interactive.TurnSubmissionInput) (interactive.TurnSubmissionReceipt, error) {
			return interactive.TurnSubmissionReceipt{}, nil
		},
		SubmitStateSchemaBatch: func(context.Context, interactive.ActorStateSchemaBatch) (interactive.ActorStateSchemaBatchResult, error) {
			return interactive.ActorStateSchemaBatchResult{}, nil
		},
	}))(config.ResolvedAgentToolSettings{})
	if err != nil {
		t.Fatal(err)
	}

	wanted := map[string]bool{
		"prepare_interactive_turn":      false,
		"submit_interactive_turn":       false,
		"initialize_story_state_schema": false,
	}
	middleware := newInteractiveStoryToolMiddleware()
	for _, definition := range definitions {
		info, infoErr := definition.Tool.Info(context.Background())
		if infoErr != nil {
			t.Fatal(infoErr)
		}
		if _, tracked := wanted[info.Name]; !tracked {
			continue
		}
		wanted[info.Name] = true
		if definition.Descriptor.Execution != agent.ToolExecutionSessionExclusive ||
			definition.Descriptor.MutationScope != agent.ToolMutationSession ||
			definition.Descriptor.PostCheck != agent.ToolPostCheckSessionState {
			t.Fatalf("%s must remain a session-scoped domain workflow: %+v", info.Name, definition.Descriptor)
		}
		toolContext := &agent.ToolContext{
			Name: info.Name,
			Definition: agent.ToolDefinitionSnapshot{
				Info: info, Descriptor: definition.Descriptor,
			},
		}
		called := false
		endpoint, wrapErr := wrapTextToolCallForTest(middleware,
			func(context.Context, string, ...agent.ToolOption) (string, error) {
				called = true
				return "ok", nil
			},
			toolContext,
		)
		if wrapErr != nil {
			t.Fatal(wrapErr)
		}
		result, runErr := endpoint(context.Background(), `{}`)
		if runErr != nil {
			t.Fatal(runErr)
		}
		if !called || result != "ok" {
			t.Fatalf("%s should pass the game-mode domain boundary, called=%t result=%q", info.Name, called, result)
		}
		decision := (&toolOrchestratorMiddleware{
			policyKind: config.AgentKindInteractiveStory, enforceToolSettings: true,
		}).buildToolDecision(context.Background(), toolContext, `{}`)
		if decision.Action != "allowed" {
			t.Fatalf("%s should pass the orchestrator policy boundary: %#v", info.Name, decision)
		}
	}
	for name, found := range wanted {
		if !found {
			t.Fatalf("interactive story catalog is missing %s", name)
		}
	}
}

func TestInteractiveDirectorPlanFileMiddlewareBlocksStateTools(t *testing.T) {
	middleware := newInteractiveDirectorPlanFileMiddleware()
	for _, name := range []string{"apply_actor_state_patch"} {
		called := false
		endpoint, err := wrapTextToolCallForTest(middleware,
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
		name      string
		arguments string
		allowed   bool
	}{
		{name: submitDirectorPlanUpdateToolName, arguments: `{}`, allowed: true},
		{name: "read", arguments: `{"path":"chapters/one.md"}`, allowed: false},
		{name: "read", arguments: `{"path":"event://package/card"}`, allowed: true},
		{name: "write", arguments: `{"path":"director-plan.md","content":"x"}`, allowed: false},
	} {
		called := false
		endpoint, err := wrapTextToolCallForTest(middleware,
			func(context.Context, string, ...agent.ToolOption) (string, error) {
				called = true
				return "ok", nil
			},
			testToolContext(tc.name, ""),
		)
		if err != nil {
			t.Fatal(err)
		}
		result, err := endpoint(context.Background(), tc.arguments)
		if err != nil {
			t.Fatal(err)
		}
		if tc.allowed && (!called || result != "ok") {
			t.Fatalf("%s %s should pass through, called=%v result=%s", tc.name, tc.arguments, called, result)
		}
		if !tc.allowed && (called || !strings.Contains(result, submitDirectorPlanUpdateToolName)) {
			t.Fatalf("%s %s should be blocked in favor of structured submit, called=%v result=%s", tc.name, tc.arguments, called, result)
		}
	}
}

func TestInteractiveDirectorPlanFileMiddlewareBlocksUnauthorizedTools(t *testing.T) {
	middleware := newInteractiveDirectorPlanFileMiddleware()
	called := false
	endpoint, err := wrapTextToolCallForTest(middleware,
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
	endpoint, err := wrapTextToolCallForTest(middleware,
		func(context.Context, string, ...agent.ToolOption) (string, error) {
			called = true
			return "ok", nil
		},
		testToolContext("write", "call-1"),
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
	if !strings.Contains(result, "workspace 或宿主副作用") {
		t.Fatalf("unexpected block result: %s", result)
	}
}

func TestToolOrchestratorBlocksInteractiveSubAgentWriteTools(t *testing.T) {
	middleware := &toolOrchestratorMiddleware{agentKind: "researcher", policyKind: AgentKindInteractiveStory}
	called := false
	endpoint, err := wrapTextToolCallForTest(middleware,
		func(context.Context, string, ...agent.ToolOption) (string, error) {
			called = true
			return "ok", nil
		},
		testToolContext("write", "call-1"),
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
	if !strings.Contains(result, "workspace 或宿主副作用") {
		t.Fatalf("unexpected block result: %s", result)
	}
}

func TestToolOrchestratorKeepsExecutionMetadataOutOfModelResult(t *testing.T) {
	middleware := &toolOrchestratorMiddleware{agentKind: AgentKindIDE}
	content := strings.Repeat("正文", 100)
	endpoint, err := wrapTextToolCallForTest(middleware,
		func(context.Context, string, ...agent.ToolOption) (string, error) {
			return content, nil
		},
		testToolContext("write", "call-1"),
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := endpoint(context.Background(), `{"path":"chapters/ch01.md"}`)
	if err != nil {
		t.Fatal(err)
	}
	if result != content {
		t.Fatalf("result below the high default limit changed")
	}
	if strings.Contains(result, "tool_result.v1") || strings.Contains(result, "mutates_workspace") {
		t.Fatalf("execution metadata leaked into model result: %s", result)
	}
}

func TestToolOrchestratorTruncatesResultWhenLimitConfigured(t *testing.T) {
	middleware := &toolOrchestratorMiddleware{agentKind: AgentKindIDE, toolResultMaxBytes: 128}
	endpoint, err := wrapTextToolCallForTest(middleware,
		func(context.Context, string, ...agent.ToolOption) (string, error) {
			return strings.Repeat("正文", 200), nil
		},
		testToolContext("write", "call-1"),
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := endpoint(context.Background(), `{"path":"chapters/ch01.md"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "[tool result truncated]") || len(result) > 128 {
		t.Fatalf("configured limit should truncate result: %s", result)
	}
}

func TestToolOrchestratorBlocksMalformedJSONArguments(t *testing.T) {
	middleware := &toolOrchestratorMiddleware{agentKind: AgentKindIDE}
	called := false
	endpoint, err := wrapTextToolCallForTest(middleware,
		func(context.Context, string, ...agent.ToolOption) (string, error) {
			called = true
			return "ok", nil
		},
		testToolContext("write", "call-1"),
	)
	if err != nil {
		t.Fatal(err)
	}
	args := "{\"path\":\"chapters/ch01.md\",\"content\":\"过了一遍。\\\\n\\\\n韩十四。武监司。三十\n\t^\n\\"
	result, err := endpoint(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("malformed JSON arguments should be blocked before endpoint is called")
	}
	if !strings.Contains(result, "参数不是完整 JSON 对象") ||
		!strings.Contains(result, "arguments are not a complete JSON object") {
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
				FinishReason: finishReason, RequestedTools: []string{"write"},
			})
			ctx := ContextWithRunObserver(context.Background(), observer)
			middleware := &toolOrchestratorMiddleware{agentKind: AgentKindIDE}
			called := false
			endpoint, err := wrapTextToolCallForTest(middleware,
				func(context.Context, string, ...agent.ToolOption) (string, error) {
					called = true
					return "ok", nil
				},
				testToolContext("write", "call-output-limit"),
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
			} {
				if !strings.Contains(result, want) {
					t.Fatalf("output-limit result missing %q:\n%s", want, result)
				}
			}
		})
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
		RequestedTools:    []string{"write"},
		ProviderRequestID: "provider-1",
	})
	ctx := ContextWithRunObserver(context.Background(), observer)
	middleware := &toolOrchestratorMiddleware{agentKind: AgentKindIDE}
	called := false
	endpoint, err := wrapTextToolCallForTest(middleware,
		func(context.Context, string, ...agent.ToolOption) (string, error) {
			called = true
			return "ok", nil
		},
		testToolContext("write", "call-content-filter"),
	)
	if err != nil {
		t.Fatal(err)
	}
	args := `{"path":"chapters/ch01.md","content":"正文被过滤中断`
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
		"blocked execution with no side effects",
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
	observer.RecordLLMOutcome(LLMOutcome{FinishReason: "content_filter", RequestedTools: []string{"read"}})
	ctx := ContextWithRunObserver(context.Background(), observer)
	middleware := &toolOrchestratorMiddleware{agentKind: AgentKindIDE}
	called := false
	endpoint, err := wrapTextToolCallForTest(middleware,
		func(context.Context, string, ...agent.ToolOption) (string, error) {
			called = true
			return "unsafe", nil
		},
		testToolContext("read", "call-valid-content-filter"),
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
	} {
		if !strings.Contains(result, want) {
			t.Fatalf("content-filter result missing %q:\n%s", want, result)
		}
	}
}

func TestToolPathFromArgsExtractsPartialFilePath(t *testing.T) {
	args := `{"path":"chapters/ch01.md","content":"正文还没闭合`
	if got := toolPathFromArgs(args); got != "chapters/ch01.md" {
		t.Fatalf("partial path = %q, want chapters/ch01.md", got)
	}
}

func TestToolOrchestratorAllowsEscapedSpecialCharactersInJSONArguments(t *testing.T) {
	middleware := &toolOrchestratorMiddleware{agentKind: AgentKindIDE}
	called := false
	endpoint, err := wrapTextToolCallForTest(middleware,
		func(context.Context, string, ...agent.ToolOption) (string, error) {
			called = true
			return "ok", nil
		},
		testToolContext("write", "call-1"),
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := endpoint(context.Background(), `{"path":"chapters/ch01.md","content":"过了一遍。\\n\\n韩十四。武监司。三十\n\t^\n\""}`)
	if err != nil {
		t.Fatal(err)
	}
	if !called || !strings.Contains(result, "ok") {
		t.Fatalf("escaped special characters should pass through, called=%v result=%s", called, result)
	}
}

func TestToolOrchestratorBlocksDisabledCapability(t *testing.T) {
	middleware := &toolOrchestratorMiddleware{
		agentKind:           AgentKindIDE,
		enforceToolSettings: true,
		toolSettings:        config.ResolvedAgentToolSettings{config.AgentToolWorkspaceRead: true},
	}
	called := false
	endpoint, err := wrapTextToolCallForTest(middleware,
		func(context.Context, string, ...agent.ToolOption) (string, error) {
			called = true
			return "ok", nil
		},
		testToolContext("write", "call-1"),
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := endpoint(context.Background(), `{"path":"chapters/ch01.md"}`)
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("disabled workspace_write capability should block before endpoint is called")
	}
	if !strings.Contains(result, "workspace_write") || !strings.Contains(result, "disabled for this Agent") {
		t.Fatalf("unexpected disabled capability result: %s", result)
	}
}

func TestToolOrchestratorBlocksUndeclaredDynamicTool(t *testing.T) {
	middleware := &toolOrchestratorMiddleware{
		agentKind: AgentKindIDE, enforceToolSettings: true,
		toolSettings: config.ResolvedAgentToolSettings{config.AgentToolWorkspaceRead: true, config.AgentToolWorkspaceWrite: true},
	}
	decision := middleware.buildToolDecision(context.Background(), testToolContext("dynamic_unknown", "call-unknown"), `{}`)
	if decision.Action != "blocked" || !strings.Contains(decision.Reason, "ToolDescriptor") {
		t.Fatalf("undeclared dynamic tool decision = %#v", decision)
	}
	knownWithoutCapability := middleware.buildToolDecision(context.Background(), testToolContext("search_story_history", ""), `{}`)
	if knownWithoutCapability.Action != "allowed" {
		t.Fatalf("declared capability-free tool should remain allowed: %#v", knownWithoutCapability)
	}
}

func TestWorkspaceToolsOmitDisabledDefinitions(t *testing.T) {
	workspace := t.TempDir()
	tools, err := newToolCatalog(&config.Config{Workspace: workspace}).Workspace(config.ResolvedAgentToolSettings{
		config.AgentToolWorkspaceRead:  true,
		config.AgentToolWorkspaceWrite: false,
		config.AgentToolShell:          false,
	})
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, item := range tools {
		info, err := item.Tool.Info(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		names[info.Name] = true
	}
	for _, name := range []string{"read", "glob", "grep"} {
		if !names[name] {
			t.Fatalf("read tool %s should be registered, names=%v", name, names)
		}
	}
	for _, name := range []string{"write", "edit", "bash", "pwsh"} {
		if names[name] {
			t.Fatalf("disabled tool %s must not be registered, names=%v", name, names)
		}
	}
}
