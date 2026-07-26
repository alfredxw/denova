package agents

import (
	"context"
	"strings"
	"sync"
	"testing"

	"denova/config"
	"denova/internal/agents/session"
	producttools "denova/internal/agents/tools"
	agent "github.com/alfredxw/denova/agent"
)

func TestConfigMaxIterationDefaultsToNativeUnlimited(t *testing.T) {
	if got := configMaxIteration(&config.Config{}); got != 0 {
		t.Fatalf("default max iteration = %d, want native unlimited zero", got)
	}
	if got := configMaxIteration(&config.Config{MaxIteration: 32}); got != 32 {
		t.Fatalf("configured max iteration = %d, want 32", got)
	}
}

func TestTaskAndChildWriteWithSameProviderIDHaveDistinctExecutionIDs(t *testing.T) {
	child := executionIdentityChild{}
	task, err := producttools.NewTask(context.Background(), []agent.Runnable{child})
	if err != nil {
		t.Fatal(err)
	}
	root, err := agent.NewAgent(context.Background(), agent.AgentConfig{
		Name: "root", Model: &taskExecutionIdentityModel{}, Tools: []agent.ToolDefinition{task},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := agent.ContextWithInvocationIdentity(context.Background(), agent.InvocationIdentity{
		Scope: "workspace:identity", OperationID: "operation-task-child", Cycle: 1,
	})
	events := root.Run(ctx, &agent.AgentInput{Messages: []*agent.Message{agent.UserMessage("delegate")}})
	executionIDs := map[string]string{}
	providerIDs := map[string]string{}
	for {
		event, ok := events.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			t.Fatal(event.Err)
		}
		if event.Output == nil || event.Output.ToolExecution == nil || event.Output.ToolExecution.Phase != agent.ToolExecutionStarted {
			continue
		}
		execution := event.Output.ToolExecution
		executionIDs[execution.ToolName] = execution.ExecutionID
		providerIDs[execution.ToolName] = execution.ProviderCallID
	}
	if providerIDs["task"] != "provider-same" || providerIDs["write"] != "provider-same" ||
		executionIDs["task"] == "" || executionIDs["write"] == "" || executionIDs["task"] == executionIDs["write"] {
		t.Fatalf("execution IDs=%v provider IDs=%v", executionIDs, providerIDs)
	}
}

type taskExecutionIdentityModel struct {
	mu    sync.Mutex
	calls int
}

func (model *taskExecutionIdentityModel) Generate(context.Context, []*agent.Message, ...agent.ModelOption) (*agent.Message, error) {
	model.mu.Lock()
	defer model.mu.Unlock()
	model.calls++
	if model.calls == 1 {
		return agent.AssistantMessage("", []agent.ToolCall{{
			ID: "provider-same", Type: "function",
			Function: agent.FunctionCall{Name: "task", Arguments: `{"subagent_type":"writer","description":"write"}`},
		}}), nil
	}
	return agent.AssistantMessage("done", nil), nil
}

func (model *taskExecutionIdentityModel) Stream(ctx context.Context, messages []*agent.Message, options ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	message, err := model.Generate(ctx, messages, options...)
	if err != nil {
		return nil, err
	}
	reader, writer := agent.Pipe[*agent.Message](1)
	writer.Send(message, nil)
	writer.Close()
	return reader, nil
}

type executionIdentityChild struct{}

func (executionIdentityChild) Name(context.Context) string        { return "writer" }
func (executionIdentityChild) Description(context.Context) string { return "Writes one bounded file." }
func (executionIdentityChild) Run(ctx context.Context, _ *agent.AgentInput, _ ...agent.AgentRunOption) *agent.AsyncIterator[*agent.AgentEvent] {
	iterator, generator := agent.NewAsyncIteratorPair[*agent.AgentEvent]()
	generator.Send(&agent.AgentEvent{
		AgentName: "writer", RunPath: []agent.RunStep{agent.NewRunStep("writer")},
		Output: &agent.AgentOutput{ToolExecution: &agent.ToolExecutionEvent{
			Phase: agent.ToolExecutionStarted, Index: 0,
			ExecutionID: agent.ToolExecutionIDForOrdinal(ctx, 1, 0), ProviderCallID: "provider-same", ToolName: "write",
		}},
	})
	generator.Send(&agent.AgentEvent{
		AgentName: "writer", RunPath: []agent.RunStep{agent.NewRunStep("writer")},
		Output: &agent.AgentOutput{MessageOutput: &agent.MessageVariant{
			Message: agent.AssistantMessage("written", nil), Role: agent.Assistant,
		}},
	})
	generator.Close()
	return iterator
}

func TestBuildAgentExposesGeneralAndConfiguredSubAgentsThroughTask(t *testing.T) {
	var captured []agent.AgentConfig
	previous := newNativeAgent
	newNativeAgent = func(_ context.Context, cfg agent.AgentConfig) (agent.Runnable, error) {
		captured = append(captured, cfg)
		return fakeAgent{name: cfg.Name, description: cfg.Description}, nil
	}
	t.Cleanup(func() { newNativeAgent = previous })

	_, err := buildAgent(context.Background(), &config.Config{
		OpenAIBaseURL: "https://example.invalid",
		OpenAIModel:   "test-model",
		AgentTools: config.AgentToolSettings{
			Default: config.AgentToolOverride{
				config.AgentToolWorkspaceRead:  false,
				config.AgentToolWorkspaceWrite: false,
				config.AgentToolShell:          false,
				config.AgentToolSkills:         false,
				config.AgentToolLoreRead:       false,
				config.AgentToolLoreWrite:      false,
				config.AgentToolTodo:           false,
				config.AgentToolWebSearch:      false,
				config.AgentToolWebFetch:       false,
			},
		},
		SubAgents: []config.SubAgentConfig{{
			ID:           "researcher",
			Name:         "Researcher",
			Description:  "Researches delegated context",
			SystemPrompt: "Return concise findings.",
			Parents:      []string{config.AgentKindIDE},
		}},
	}, agentBuildSpec{
		Kind:        config.AgentKindIDE,
		Name:        "DenovaAgent",
		Description: "test",
		Instruction: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(captured) != 3 {
		t.Fatalf("native Agent constructions = %d, want configured + general + root", len(captured))
	}
	if captured[0].Name != "researcher" || captured[1].Name != producttools.GeneralSubAgentName || captured[2].Name != "DenovaAgent" {
		t.Fatalf("unexpected native Agent construction order: %q %q %q", captured[0].Name, captured[1].Name, captured[2].Name)
	}
	var generalOrchestrator, rootOrchestrator *toolOrchestratorMiddleware
	for _, middleware := range captured[1].Middlewares {
		if _, ok := middleware.(*contextCompactionMiddleware); ok {
			t.Fatal("general subagent must not inherit root context compaction middleware")
		}
		if current, ok := middleware.(*toolOrchestratorMiddleware); ok {
			generalOrchestrator = current
		}
	}
	for _, middleware := range captured[2].Middlewares {
		if current, ok := middleware.(*toolOrchestratorMiddleware); ok {
			rootOrchestrator = current
		}
	}
	if generalOrchestrator == nil || rootOrchestrator == nil || generalOrchestrator == rootOrchestrator {
		t.Fatalf("general/root orchestrators must be independent: general=%p root=%p", generalOrchestrator, rootOrchestrator)
	}
	rootTools := toolNamesForTest(t, captured[2].Tools)
	if !rootTools["task"] {
		t.Fatalf("root Agent task tool missing: %v", rootTools)
	}
}

func TestBuildSubAgentInstructionInheritsParentSystemPrompt(t *testing.T) {
	parentInstruction := "# Denova 运行时契约（不可覆盖）\n\n作品根目录：/tmp/book\n父级工具权限边界。"
	instruction := buildSubAgentInstruction(agentBuildSpec{
		Kind:        config.AgentKindIDE,
		Instruction: parentInstruction,
	}, config.SubAgentConfig{
		ID:           "researcher",
		Name:         "Researcher",
		Description:  "Researches delegated context",
		SystemPrompt: "Return concise findings.",
	})

	for _, required := range []string{
		"Denova 运行时契约",
		"/tmp/book",
		"父级工具权限边界",
		"SubAgent 专属说明",
		"Researcher",
		"researcher",
		"Researches delegated context",
		"Return concise findings.",
		"不得覆盖父 Agent 的运行时契约、工具权限、workspace 边界",
	} {
		if !strings.Contains(instruction, required) {
			t.Fatalf("subagent instruction missing %q:\n%s", required, instruction)
		}
	}
	if parentIndex, subIndex := strings.Index(instruction, parentInstruction), strings.Index(instruction, "SubAgent 专属说明"); parentIndex < 0 || subIndex < 0 || parentIndex >= subIndex {
		t.Fatalf("parent prompt should appear before subagent prompt:\n%s", instruction)
	}
}

func TestAutomationSubAgentInheritsReadOnlyModeAndScope(t *testing.T) {
	parent := agentBuildSpec{
		Kind: config.AgentKindAutomation,
		Composition: BuildAutomationInstructionComposition(&config.Config{}, nil, AutomationTaskInstruction{
			Name: "Review", WriteMode: RunWriteModeReadOnly, WriteScope: "none", Workspace: "/tmp/book",
		}),
	}
	instruction, err := composeSubAgentInstruction(&config.Config{}, parent, config.SubAgentConfig{
		ID: "reviewer", Name: "Reviewer", Description: "Reviews without direct writes.",
	})
	if err != nil {
		t.Fatal(err)
	}
	text := instruction.Instruction()
	for _, required := range []string{"执行模式：read_only", "写入范围：none", "read_only` 模式只能输出 review"} {
		if !strings.Contains(text, required) {
			t.Fatalf("automation subagent instruction missing %q:\n%s", required, text)
		}
	}
}

func TestBuildSubAgentInstructionInheritsInteractiveStoryBoundary(t *testing.T) {
	parentInstruction := protectedSystemInstruction(&config.Config{}, config.AgentKindInteractiveStory, "互动故事父级内置规则")
	instruction := buildSubAgentInstruction(agentBuildSpec{
		Kind:        config.AgentKindInteractiveStory,
		Instruction: parentInstruction,
	}, config.SubAgentConfig{
		ID:           "story-researcher",
		Name:         "Story Researcher",
		Description:  "Reads story context for the parent.",
		SystemPrompt: "Only return context findings.",
	})

	for _, required := range []string{
		"禁止修改 workspace 文件",
		"只输出本回合可展示在故事舞台上的故事正文",
		"互动禁写规则",
		"Only return context findings.",
	} {
		if !strings.Contains(instruction, required) {
			t.Fatalf("interactive subagent instruction missing %q:\n%s", required, instruction)
		}
	}
}

func TestBuildAgentCanDisableGeneralSubAgent(t *testing.T) {
	off := false
	var captured []agent.AgentConfig
	previous := newNativeAgent
	newNativeAgent = func(_ context.Context, cfg agent.AgentConfig) (agent.Runnable, error) {
		captured = append(captured, cfg)
		return fakeAgent{name: cfg.Name, description: cfg.Description}, nil
	}
	t.Cleanup(func() { newNativeAgent = previous })

	_, err := buildAgent(context.Background(), &config.Config{
		OpenAIBaseURL: "https://example.invalid",
		OpenAIModel:   "test-model",
		GeneralSubAgents: config.AgentGeneralSubAgentSettings{
			IDE: &off,
		},
		AgentTools: config.AgentToolSettings{
			Default: config.AgentToolOverride{
				config.AgentToolWorkspaceRead:  false,
				config.AgentToolWorkspaceWrite: false,
				config.AgentToolShell:          false,
				config.AgentToolSkills:         false,
				config.AgentToolLoreRead:       false,
				config.AgentToolLoreWrite:      false,
				config.AgentToolTodo:           false,
				config.AgentToolWebSearch:      false,
				config.AgentToolWebFetch:       false,
				config.AgentToolDelegation:     false,
			},
		},
	}, agentBuildSpec{
		Kind:        config.AgentKindIDE,
		Name:        "DenovaAgent",
		Description: "test",
		Instruction: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(captured) != 1 || captured[0].Name != "DenovaAgent" {
		t.Fatalf("general subagent should be absent when configured off: %#v", captured)
	}
	if toolNamesForTest(t, captured[0].Tools)["task"] {
		t.Fatalf("task tool should be absent without any available subagent")
	}
}

func TestSubAgentAssemblyUsesParentToolPolicyKind(t *testing.T) {
	assembly, err := buildChatModelAgentAssembly(context.Background(), &config.Config{}, chatModelAgentAssemblySpec{
		Kind:              "researcher",
		ToolPolicyKind:    config.AgentKindInteractiveStory,
		ToolSettings:      config.ResolvedAgentToolSettings{},
		IncludeCompaction: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	var orchestrator *toolOrchestratorMiddleware
	for _, handler := range assembly.Middlewares {
		if middleware, ok := handler.(*toolOrchestratorMiddleware); ok {
			orchestrator = middleware
			break
		}
	}
	if orchestrator == nil {
		t.Fatalf("expected tool orchestrator middleware")
	}
	if got := orchestrator.effectivePolicyKind(); got != config.AgentKindInteractiveStory {
		t.Fatalf("subagent tool policy should use parent kind, got %q", got)
	}
}

func TestBuildChatModelAgentAssemblyPassesToolResultLimit(t *testing.T) {
	assembly, err := buildChatModelAgentAssembly(context.Background(), &config.Config{AgentToolResultLimitKB: 64}, chatModelAgentAssemblySpec{
		Kind:              config.AgentKindIDE,
		ToolSettings:      config.ResolvedAgentToolSettings{},
		IncludeCompaction: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	var orchestrator *toolOrchestratorMiddleware
	for _, handler := range assembly.Middlewares {
		if middleware, ok := handler.(*toolOrchestratorMiddleware); ok {
			orchestrator = middleware
			break
		}
	}
	if orchestrator == nil {
		t.Fatalf("expected tool orchestrator middleware")
	}
	if got := orchestrator.toolResultLimitBytes(); got != 64*1024 {
		t.Fatalf("tool result limit bytes = %d, want %d", got, 64*1024)
	}
}

func TestSubAgentStreamingDoesNotAppendParentAssistantContent(t *testing.T) {
	var fullContent, fullThinking strings.Builder
	var events []Event
	meta := agentEventMetadata{AgentName: "researcher", RootAgentName: "DenovaAgent", RunPath: []string{"DenovaAgent", "researcher"}, SubAgent: true}
	processNonStreamingEvent(&agent.MessageVariant{Message: agent.AssistantMessage("sub draft", nil)}, &fullContent, &fullThinking, 0, meta, nil, func(ev Event) {
		events = append(events, ev)
	})
	if fullContent.Len() != 0 || fullThinking.Len() != 0 {
		t.Fatalf("subagent output must not append to parent builders content=%q thinking=%q", fullContent.String(), fullThinking.String())
	}
	if len(events) != 1 || events[0].Type != "chunk" || !eventDataBool(events[0].Data, "subagent") {
		t.Fatalf("subagent chunk should still be emitted with metadata: %#v", events)
	}

	rootMeta := agentEventMetadata{AgentName: "DenovaAgent", RootAgentName: "DenovaAgent", RunPath: []string{"DenovaAgent"}}
	processNonStreamingEvent(&agent.MessageVariant{Message: agent.AssistantMessage("root final", nil)}, &fullContent, &fullThinking, 0, rootMeta, nil, func(Event) {})
	if got := fullContent.String(); got != "root final" {
		t.Fatalf("root output should append to parent builder, got %q", got)
	}
}

func TestDisplayRecorderPersistsSubAgentAssistantChunks(t *testing.T) {
	appender := &fakeDisplayAppender{}
	recorder := newDisplayEventRecorder(fakeDisplayConversation{appender: appender}, displayEventRecorderOptions{})
	meta := agentEventMetadata{
		RunID:             "run-1",
		AgentName:         "researcher",
		RootAgentName:     "DenovaAgent",
		RunPath:           []string{"DenovaAgent", "researcher"},
		SubAgent:          true,
		SubAgentSessionID: "run-1-subagent-01-researcher",
		SubAgentType:      "researcher",
	}

	recorder.Record(Event{Type: "chunk", Data: meta.appendTo(map[string]interface{}{"content": "第一段"})})
	recorder.Record(Event{Type: "chunk", Data: meta.appendTo(map[string]interface{}{"content": "第二段"})})

	if len(appender.events) != 1 {
		t.Fatalf("expected one merged display event, got %#v", appender.events)
	}
	event := appender.events[0]
	if event.Role != "assistant" || event.Content != "第一段第二段" {
		t.Fatalf("unexpected persisted subagent event: %#v", event)
	}
	if !event.SubAgent || event.SubAgentSessionID != "run-1-subagent-01-researcher" || event.SubAgentType != "researcher" {
		t.Fatalf("subagent metadata missing: %#v", event)
	}
}

func TestSubAgentWriteToolResultStillTracksMutation(t *testing.T) {
	observer := newRunObserver(nil, "")
	record := ToolExecutionRecord{
		ToolName: "write", ExecutionID: "call-write", Status: "success",
		Descriptor: producttools.WorkspaceWriteDescriptor(agent.ToolSourceWrite, config.AgentToolWorkspaceWrite, agent.ToolRecoveryReconcilable),
	}
	applyToolMutationReceiptToExecutionRecord(&record, agent.ToolResult{Details: []byte(`{"schema":"workspace_change.tool_result.v1","status":"applied","workspace":"/workspace/book-a","change_group_id":"group-1","change_set_id":"change-1","path":"chapters/ch01.md"}`)})
	observer.RecordToolExecution(record)
	mutations, warnings := observer.ResolvedMutations()
	if len(mutations) != 1 {
		t.Fatalf("expected subagent write tool to be tracked, got %#v warnings=%#v", mutations, warnings)
	}
	if mutations[0].Target != "chapters/ch01.md" || mutations[0].PostCheck != ToolPostCheckWorkspaceChange {
		t.Fatalf("unexpected mutation: %#v", mutations[0])
	}
}

type fakeDisplayConversation struct {
	appender *fakeDisplayAppender
}

func (c fakeDisplayConversation) AssembleModelContext(ctx context.Context, _ string, input ModelContextInput) (ModelContextResult, error) {
	return AssembleSingleUserModelContext(ctx, input)
}
func (c fakeDisplayConversation) AppendAssistant(string) error               { return nil }
func (c fakeDisplayConversation) MarkInterrupted(_, _, _ string) error       { return nil }
func (c fakeDisplayConversation) PendingInterruption() *session.Interruption { return nil }
func (c fakeDisplayConversation) ResolveInterruption(string) error           { return nil }
func (c fakeDisplayConversation) AppendDisplayEvent(event session.DisplayEvent) error {
	return c.appender.AppendDisplayEvent(event)
}
func (c fakeDisplayConversation) UpdateDisplayToolStatus(id, name, status string) error {
	return c.appender.UpdateDisplayToolStatus(id, name, status)
}
func (c fakeDisplayConversation) AppendDisplayEventContent(id, role, delta string) error {
	return c.appender.AppendDisplayEventContent(id, role, delta)
}

type fakeDisplayAppender struct {
	events []session.DisplayEvent
}

func (a *fakeDisplayAppender) AppendDisplayEvent(event session.DisplayEvent) error {
	a.events = append(a.events, event)
	return nil
}

func (a *fakeDisplayAppender) UpdateDisplayToolStatus(_, _, _ string) error { return nil }

func (a *fakeDisplayAppender) AppendDisplayEventContent(id, role, delta string) error {
	for index := range a.events {
		if a.events[index].ID == id && a.events[index].Role == role {
			a.events[index].Content += delta
			return nil
		}
	}
	return nil
}

func TestRunSubAgentForwardsDrainedChildEvents(t *testing.T) {
	child := &streamingSubAgent{}
	var forwarded []*agent.AgentEvent
	controller := &subagentContextController{}
	ctx := agent.ContextWithContextWindowController(context.Background(), controller)
	ctx = agent.ContextWithEventSink(ctx, func(event *agent.AgentEvent) {
		forwarded = append(forwarded, event)
	})

	task, err := newToolCatalog(nil).Task(ctx, []agent.Runnable{child})
	if err != nil {
		t.Fatal(err)
	}
	toolResult, err := task.Tool.Run(ctx, `{"subagent_type":"reviewer","description":"inspect the draft"}`)
	if err != nil {
		t.Fatal(err)
	}
	result := toolResult.ModelContent
	if result != "child result" || child.request != "inspect the draft" {
		t.Fatalf("subagent result=%q request=%q", result, child.request)
	}
	if len(forwarded) != 1 || forwarded[0].AgentName != "reviewer" || len(forwarded[0].RunPath) != 1 {
		t.Fatalf("forwarded events = %#v", forwarded)
	}
	if child.scope.Depth != 1 || child.hasRewriter || !child.hasMutationObserver {
		t.Fatalf("child scope=%#v rewriter=%t mutation_observer=%t", child.scope, child.hasRewriter, child.hasMutationObserver)
	}
	variant := forwarded[0].Output.MessageOutput
	if variant == nil || variant.IsStreaming || variant.MessageStream != nil || variant.Message == nil || variant.Message.Content != "child result" {
		t.Fatalf("forwarded child stream must become one reusable complete message: %#v", variant)
	}
}

type streamingSubAgent struct {
	request             string
	scope               agent.InvocationScope
	hasRewriter         bool
	hasMutationObserver bool
}

func (*streamingSubAgent) Name(context.Context) string        { return "reviewer" }
func (*streamingSubAgent) Description(context.Context) string { return "Reviews delegated work." }
func (child *streamingSubAgent) Run(ctx context.Context, input *agent.AgentInput, _ ...agent.AgentRunOption) *agent.AsyncIterator[*agent.AgentEvent] {
	child.scope, _ = agent.InvocationScopeFromContext(ctx)
	_, child.hasRewriter = agent.ContextWindowControllerFromContext(ctx)
	_, child.hasMutationObserver = agent.ContextMutationObserverFromContext(ctx)
	if input != nil && len(input.Messages) > 0 && input.Messages[0] != nil {
		child.request = input.Messages[0].Content
	}
	stream, writer := agent.Pipe[*agent.Message](-1)
	writer.Send(agent.AssistantMessage("child result", nil), nil)
	writer.Close()
	iterator, generator := agent.NewAsyncIteratorPair[*agent.AgentEvent]()
	generator.Send(&agent.AgentEvent{
		AgentName: "reviewer",
		RunPath:   []agent.RunStep{agent.NewRunStep("reviewer")},
		Output: &agent.AgentOutput{MessageOutput: &agent.MessageVariant{
			IsStreaming: true, MessageStream: stream, Role: agent.Assistant,
		}},
	})
	generator.Close()
	return iterator
}

type subagentContextController struct{}

func (*subagentContextController) BeforeModel(_ context.Context, messages []*agent.Message) ([]*agent.Message, error) {
	return messages, nil
}
func (*subagentContextController) BeforeComplete(_ context.Context, messages []*agent.Message) ([]*agent.Message, bool, error) {
	return messages, false, nil
}
func (*subagentContextController) Checkpoint(context.Context, agent.ContextCheckpointRequest) (agent.ContextCheckpointResult, error) {
	return agent.ContextCheckpointResult{}, nil
}
func (*subagentContextController) Rewind(context.Context, agent.ContextRewindRequest) (agent.ContextRewindResult, error) {
	return agent.ContextRewindResult{}, nil
}
func (*subagentContextController) ObserveTool(context.Context, agent.ContextToolObservation) {}

type fakeAgent struct {
	name        string
	description string
}

func (f fakeAgent) Name(context.Context) string        { return f.name }
func (f fakeAgent) Description(context.Context) string { return f.description }
func (f fakeAgent) Run(context.Context, *agent.AgentInput, ...agent.AgentRunOption) *agent.AsyncIterator[*agent.AgentEvent] {
	iter, gen := agent.NewAsyncIteratorPair[*agent.AgentEvent]()
	gen.Close()
	return iter
}

func toolNamesForTest(t *testing.T, tools []agent.ToolDefinition) map[string]bool {
	t.Helper()
	names := make(map[string]bool, len(tools))
	for _, current := range tools {
		info, err := current.Tool.Info(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		names[info.Name] = true
	}
	return names
}
