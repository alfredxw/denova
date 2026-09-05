package execution

import (
	"context"
	"testing"
	"time"

	agentconversation "denova/internal/agents/conversation"
	agentdelegation "denova/internal/agents/delegation"
	agentprompts "denova/internal/agents/prompts"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	agenttoolruntime "denova/internal/agents/toolruntime"

	agent "github.com/alfredxw/denova/agent"
	agentpermission "github.com/alfredxw/denova/agent/permission"
	"github.com/alfredxw/denova/agent/providers"
)

func TestChildTracesRemainIndependentAfterParentStops(t *testing.T) {
	for _, kind := range []string{agentrun.AgentKindIDE, agentrun.AgentKindInteractiveStory} {
		t.Run(kind, func(t *testing.T) { testChildTracesAfterParentStops(t, kind) })
	}
}

func testChildTracesAfterParentStops(t *testing.T, kind string) {
	ctx := context.Background()
	agentrun.SetTraceContentCaptureEnabled(true)
	t.Cleanup(func() { agentrun.SetTraceContentCaptureEnabled(false) })
	workspace, stateRoot := t.TempDir(), t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	conversation, err := store.GetOrCreate("parent")
	if err != nil {
		t.Fatal(err)
	}
	models := map[string]*publicBackendNextTurnModel{}
	var children []agentdelegation.Child
	for _, name := range []string{"writer", "reviewer"} {
		model := &publicBackendNextTurnModel{started: make(chan struct{}), release: make(chan struct{})}
		models[name] = model
		children = append(children, agentdelegation.Child{
			Name: name, Description: name, Identity: agent.CapabilityIdentity{Kind: "test." + name, Version: 1},
			Definition: agent.Definition{
				Name: name, Model: model, AttachmentRoot: stateRoot,
				ModelIdentity: agent.CapabilityIdentity{Kind: "test.model." + name, Version: 1},
				Middlewares: []agent.Middleware{agent.IdentifyMiddleware(
					agentrun.NewModelInputLoggingMiddleware(agentrun.AgentKindIDE, providers.ModelConfig{Model: name}, 0, 0, agentprompts.SystemPromptComposition{}),
					agent.CapabilityIdentity{Kind: "test.trace." + name, Version: 1},
				)},
			},
		})
	}
	catalog, err := agentdelegation.NewCatalog(nil, agentdelegation.Config{
		Capability: "test.trace-delegation", Parallelism: 2, MaxResultBytes: 64 << 10,
		ValidationIdentity: agent.CapabilityIdentity{Kind: "test.trace-validation", Version: 1},
		Validate:           func(context.Context, []agent.ToolDefinition) error { return nil },
	}, children...)
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := NewAgentRuntime(ctx, t.TempDir(),
		WithToolMutationApplier(func(context.Context, agenttoolruntime.CommittedToolMutation) error { return nil }),
		WithChildDefinitionResolver(ChildDefinitionResolverFunc(func(_ context.Context, request ChildDefinitionRequest) (ChildDefinition, error) {
			definition, err := agentdelegation.ChildDefinition(agent.Definition{Tools: catalog}, request.Child)
			return ChildDefinition{Definition: definition, Workspace: workspace}, err
		})),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close(ctx) })
	model := &publicBackendTestModel{responses: []*agent.Message{
		agent.AssistantMessage("", []agent.ToolCall{{ID: "start-both", Type: "function", Function: agent.FunctionCall{
			Name: "task", Arguments: `{"action":"start","starts":[{"agent":"writer","prompt":"Write"},{"agent":"reviewer","prompt":"Review"}]}`,
		}}}), agent.AssistantMessage("Started both", nil),
	}}
	operation, err := runtime.Start(ctx, StartRequest{Cycle: Cycle{
		Definition:   agent.Definition{Name: "root", Model: model, ModelIdentity: agent.CapabilityIdentity{Kind: "test.trace-parent", Version: 1}, Tools: catalog, Permission: agentpermission.FullAccess()},
		Conversation: agentconversation.NewSessionConversationForAgent(conversation, nil, kind),
		Request:      agentchatRequest("parallel-parent", "Delegate"),
		Options:      agentrun.Options{AgentKind: kind, ProjectID: "project-test", SessionID: conversation.ID, StoryID: "story-test", BranchID: "main", Workspace: workspace, StateRoot: stateRoot},
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, model := range models {
		select {
		case <-model.started:
		case <-time.After(2 * time.Second):
			t.Fatal("child did not start")
		}
	}
	if _, err := operation.publicHandle.run.Abort(ctx, agent.AbortRequest{Reason: "stop parent only"}); err != nil {
		t.Fatal(err)
	}
	if result := operation.Wait(ctx); result.Status != agentrun.OutcomeAborted {
		t.Fatalf("parent = %#v", result)
	}
	handles := map[string]*publicRunHandle{}
	runtime.public.mu.RLock()
	for _, handle := range runtime.public.runs {
		if handle != operation.publicHandle {
			handles[handle.registration.options.RootAgentName] = handle
		}
	}
	runtime.public.mu.RUnlock()
	if len(handles) != 2 {
		t.Fatalf("child handles = %d", len(handles))
	}
	if _, err := handles["writer"].run.Queue(ctx, agent.Text("Continue writing")); err != nil {
		t.Fatal(err)
	}
	close(models["writer"].release)
	if _, err := handles["reviewer"].run.Abort(ctx, agent.AbortRequest{Reason: "cancel review"}); err != nil {
		t.Fatal(err)
	}
	location := agentrun.TraceLocation{Workspace: workspace, StateRoot: stateRoot}
	for name, handle := range handles {
		waitForChildTrace(t, runtime, handle.run.ID())
		trace, err := agentrun.ReadRunTrace(location, handle.run.ID())
		if err != nil {
			t.Fatal(err)
		}
		if trace.Summary.ParentRunID != operation.publicHandle.run.ID() || trace.Summary.AgentName != name || !trace.Summary.ContentCaptured {
			t.Fatalf("child identity = %#v", trace.Summary)
		}
		if name == "writer" && (trace.Summary.Status != "success" || trace.Summary.LLMCalls != 2) {
			t.Fatalf("queued child = %#v", trace.Summary)
		}
		if name == "reviewer" && trace.Summary.Status != "aborted" {
			t.Fatalf("cancelled child = %#v", trace.Summary)
		}
		created := 0
		for _, record := range trace.Records {
			if record.Type == "run_created" {
				created++
			}
		}
		if created != 1 {
			t.Fatalf("child reused %d ledgers", created)
		}
	}
}
