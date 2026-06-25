package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/prebuilt/deep"
	"github.com/cloudwego/eino/schema"

	"nova/config"
	"nova/internal/session"
)

func TestConfigMaxIterationDefaultsToUnlimited(t *testing.T) {
	if got := configMaxIteration(&config.Config{}); got != unlimitedAgentMaxIterations {
		t.Fatalf("default max iteration = %d, want %d", got, unlimitedAgentMaxIterations)
	}
	if got := configMaxIteration(&config.Config{MaxIteration: 32}); got != 32 {
		t.Fatalf("configured max iteration = %d, want 32", got)
	}
}

func TestBuildDeepAgentPassesGeneralAndConfiguredSubAgents(t *testing.T) {
	off := false
	var captured *deep.Config
	previous := newDeepAgent
	newDeepAgent = func(_ context.Context, cfg *deep.Config) (adk.ResumableAgent, error) {
		copied := *cfg
		captured = &copied
		return fakeAgent{name: cfg.Name, description: cfg.Description}, nil
	}
	t.Cleanup(func() { newDeepAgent = previous })

	_, err := buildDeepAgent(context.Background(), &config.Config{
		OpenAIBaseURL: "https://example.invalid",
		OpenAIModel:   "test-model",
		AgentTools: config.AgentToolSettings{
			Default: config.AgentToolOverride{
				FileRead:     &off,
				FileWrite:    &off,
				ShellExecute: &off,
				Skills:       &off,
				LoreRead:     &off,
				LoreWrite:    &off,
				Todo:         &off,
				WebSearch:    &off,
			},
		},
		SubAgents: []config.SubAgentConfig{{
			ID:           "researcher",
			Name:         "Researcher",
			Description:  "Researches delegated context",
			SystemPrompt: "Return concise findings.",
			Parents:      []string{config.AgentKindIDE},
		}},
	}, deepAgentSpec{
		Kind:        config.AgentKindIDE,
		Name:        "NovaAgent",
		Description: "test",
		Instruction: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if captured == nil {
		t.Fatalf("expected deep config to be captured")
	}
	if captured.WithoutGeneralSubAgent {
		t.Fatalf("general subagent must stay enabled")
	}
	if !captured.ToolsConfig.EmitInternalEvents {
		t.Fatalf("parent DeepAgent should emit nested internal events")
	}
	if len(captured.SubAgents) != 1 {
		t.Fatalf("expected one configured subagent, got %d", len(captured.SubAgents))
	}
	if got := captured.SubAgents[0].Name(context.Background()); got != "researcher" {
		t.Fatalf("unexpected subagent name: %s", got)
	}
}

func TestBuildSubAgentInstructionInheritsParentSystemPrompt(t *testing.T) {
	parentInstruction := "# Nova 运行时契约（不可覆盖）\n\n作品根目录：/tmp/book\n父级工具权限边界。"
	instruction := buildSubAgentInstruction(deepAgentSpec{
		Kind:        config.AgentKindIDE,
		Instruction: parentInstruction,
	}, config.SubAgentConfig{
		ID:           "researcher",
		Name:         "Researcher",
		Description:  "Researches delegated context",
		SystemPrompt: "Return concise findings.",
	})

	for _, required := range []string{
		"Nova 运行时契约",
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

func TestBuildSubAgentInstructionInheritsInteractiveStoryBoundary(t *testing.T) {
	parentInstruction := protectedSystemInstruction(&config.Config{}, config.AgentKindInteractiveStory, "互动故事父级内置规则")
	instruction := buildSubAgentInstruction(deepAgentSpec{
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
		"<NARRATIVE>",
		"互动禁写规则",
		"Only return context findings.",
	} {
		if !strings.Contains(instruction, required) {
			t.Fatalf("interactive subagent instruction missing %q:\n%s", required, instruction)
		}
	}
}

func TestBuildDeepAgentCanDisableGeneralSubAgent(t *testing.T) {
	off := false
	var captured *deep.Config
	previous := newDeepAgent
	newDeepAgent = func(_ context.Context, cfg *deep.Config) (adk.ResumableAgent, error) {
		copied := *cfg
		captured = &copied
		return fakeAgent{name: cfg.Name, description: cfg.Description}, nil
	}
	t.Cleanup(func() { newDeepAgent = previous })

	_, err := buildDeepAgent(context.Background(), &config.Config{
		OpenAIBaseURL: "https://example.invalid",
		OpenAIModel:   "test-model",
		GeneralSubAgents: config.AgentGeneralSubAgentSettings{
			IDE: &off,
		},
		AgentTools: config.AgentToolSettings{
			Default: config.AgentToolOverride{
				FileRead:     &off,
				FileWrite:    &off,
				ShellExecute: &off,
				Skills:       &off,
				LoreRead:     &off,
				LoreWrite:    &off,
				Todo:         &off,
				WebSearch:    &off,
			},
		},
	}, deepAgentSpec{
		Kind:        config.AgentKindIDE,
		Name:        "NovaAgent",
		Description: "test",
		Instruction: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if captured == nil || !captured.WithoutGeneralSubAgent {
		t.Fatalf("general subagent should be disabled when configured off: %#v", captured)
	}
}

func TestSubAgentAssemblyUsesParentToolPolicyKind(t *testing.T) {
	assembly, err := buildChatModelAgentAssembly(context.Background(), &config.Config{}, chatModelAgentAssemblySpec{
		Kind:           "researcher",
		ToolPolicyKind: config.AgentKindInteractiveStory,
		ToolSettings: config.ResolvedAgentToolSettings{
			FileRead:     false,
			FileWrite:    false,
			ShellExecute: false,
			Skills:       false,
			LoreRead:     false,
			LoreWrite:    false,
			Todo:         false,
			WebSearch:    false,
		},
		IncludeCompaction: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	var orchestrator *toolOrchestratorMiddleware
	for _, handler := range assembly.Handlers {
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

func TestSubAgentStreamingDoesNotAppendParentAssistantContent(t *testing.T) {
	var fullContent, fullThinking strings.Builder
	var events []Event
	meta := agentEventMetadata{AgentName: "researcher", RootAgentName: "NovaAgent", RunPath: []string{"NovaAgent", "researcher"}, SubAgent: true}
	processNonStreamingEvent(&adk.MessageVariant{Message: schema.AssistantMessage("sub draft", nil)}, &fullContent, &fullThinking, meta, func(ev Event) {
		events = append(events, ev)
	})
	if fullContent.Len() != 0 || fullThinking.Len() != 0 {
		t.Fatalf("subagent output must not append to parent builders content=%q thinking=%q", fullContent.String(), fullThinking.String())
	}
	if len(events) != 1 || events[0].Type != "chunk" || !eventDataBool(events[0].Data, "subagent") {
		t.Fatalf("subagent chunk should still be emitted with metadata: %#v", events)
	}

	rootMeta := agentEventMetadata{AgentName: "NovaAgent", RootAgentName: "NovaAgent", RunPath: []string{"NovaAgent"}}
	processNonStreamingEvent(&adk.MessageVariant{Message: schema.AssistantMessage("root final", nil)}, &fullContent, &fullThinking, rootMeta, func(Event) {})
	if got := fullContent.String(); got != "root final" {
		t.Fatalf("root output should append to parent builder, got %q", got)
	}
}

func TestDisplayRecorderPersistsSubAgentAssistantChunks(t *testing.T) {
	appender := &fakeDisplayAppender{}
	recorder := newDisplayEventRecorder(fakeDisplayConversation{appender: appender})
	meta := agentEventMetadata{
		RunID:             "run-1",
		AgentName:         "researcher",
		RootAgentName:     "NovaAgent",
		RunPath:           []string{"NovaAgent", "researcher"},
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
	tracker := newMutationTracker()
	tracker.Observe(Event{Type: "tool_call", Data: map[string]interface{}{
		"id":       "call-write",
		"name":     "write_file",
		"args":     `{"file_path":"chapters/ch01.md","content":"new"}`,
		"subagent": true,
	}})
	tracker.Observe(Event{Type: "tool_result", Data: map[string]interface{}{
		"id":       "call-write",
		"name":     "write_file",
		"content":  "ok",
		"subagent": true,
	}})
	mutations := tracker.Mutations()
	if len(mutations) != 1 {
		t.Fatalf("expected subagent write tool to be tracked, got %#v", mutations)
	}
	if mutations[0].Target != "chapters/ch01.md" || !mutations[0].RequiresPostCheck {
		t.Fatalf("unexpected mutation: %#v", mutations[0])
	}
}

type fakeDisplayConversation struct {
	appender *fakeDisplayAppender
}

func (c fakeDisplayConversation) PrepareMessages(_, _ string) ([]*schema.Message, error) {
	return nil, nil
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

type fakeAgent struct {
	name        string
	description string
}

func (f fakeAgent) Name(context.Context) string        { return f.name }
func (f fakeAgent) Description(context.Context) string { return f.description }
func (f fakeAgent) Run(context.Context, *adk.AgentInput, ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	iter, gen := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
	gen.Close()
	return iter
}
func (f fakeAgent) Resume(context.Context, *adk.ResumeInfo, ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	iter, gen := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
	gen.Close()
	return iter
}
