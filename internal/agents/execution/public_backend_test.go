package execution

import (
	"context"
	"sync"
	"testing"

	agentchat "denova/internal/agents/chat"
	agentconversation "denova/internal/agents/conversation"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	agenttool "denova/internal/agents/tool"
	agenttoolruntime "denova/internal/agents/toolruntime"

	agent "github.com/alfredxw/denova/agent"
	agentpermission "github.com/alfredxw/denova/agent/permission"
)

type publicBackendTestModel struct {
	mu        sync.Mutex
	inputs    [][]*agent.Message
	responses []*agent.Message
}

type publicBackendBlockingModel struct{ started chan struct{} }

func (model *publicBackendBlockingModel) Generate(ctx context.Context, _ []*agent.Message, _ ...agent.ModelOption) (*agent.Message, error) {
	close(model.started)
	<-ctx.Done()
	return nil, ctx.Err()
}

func (model *publicBackendBlockingModel) Stream(ctx context.Context, _ []*agent.Message, _ ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	close(model.started)
	<-ctx.Done()
	return nil, ctx.Err()
}

func (model *publicBackendTestModel) Generate(_ context.Context, input []*agent.Message, _ ...agent.ModelOption) (*agent.Message, error) {
	return model.response(input), nil
}

func (model *publicBackendTestModel) Stream(_ context.Context, input []*agent.Message, _ ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	return agent.StreamReaderFromArray([]*agent.Message{model.response(input)}), nil
}

func (model *publicBackendTestModel) response(input []*agent.Message) *agent.Message {
	model.mu.Lock()
	defer model.mu.Unlock()
	model.inputs = append(model.inputs, clonePublicBackendMessages(input))
	if len(model.responses) > 0 {
		response := model.responses[0].Clone()
		model.responses = model.responses[1:]
		return response
	}
	return &agent.Message{
		Role: agent.Assistant, Content: "public runtime answer",
		ResponseMeta: &agent.ResponseMeta{FinishReason: "stop", Usage: &agent.TokenUsage{
			PromptTokens: 20, PromptTokenDetails: agent.PromptTokenDetails{CachedTokens: 12},
			CompletionTokens: 4, TotalTokens: 24,
		}},
	}
}

func TestAgentRuntimeVerifiesCommittedMutationsBeforeTerminalDisplay(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("public-runtime-mutation")
	if err != nil {
		t.Fatal(err)
	}
	descriptor := agent.ToolDescriptor{
		Source: agent.ToolSourceWrite, Execution: agent.ToolExecutionWorkspaceExclusive,
		MutationScope: agent.ToolMutationWorkspace, PostCheck: agent.ToolPostCheckWorkspaceChange,
		Recovery: agent.ToolRecoveryReconcilable, ResultProjection: agent.ToolResultBoundedModelContext,
		ResultRetention: agent.ToolResultProtected, Steering: agent.SteeringFinishCurrent, MaxResultBytes: 64 << 10,
	}
	effect, present, err := agenttoolruntime.AgentToolMutationEffect(agenttool.ExecutionRecord{
		ToolName: "write", ExecutionID: "write-call", Status: "success", Workspace: workspace,
		Target: "chapter.md", ChangeGroupID: "group", ChangeSetID: "change",
		MutationReceiptSchema: agenttool.MutationReceiptWorkspaceChange, Descriptor: descriptor,
	})
	if err != nil || !present {
		t.Fatalf("mutation effect present=%v err=%v", present, err)
	}
	tool, err := agent.InferTool("write", "write test", func(context.Context, struct{}) (agent.ToolResult, error) {
		return agent.ToolResult{
			ModelContent: "written", DisplayContent: "written", Status: agent.ToolResultSuccess,
			Effects: []agent.Effect{effect},
		}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	model := &publicBackendTestModel{responses: []*agent.Message{
		agent.AssistantMessage("", []agent.ToolCall{{
			ID: "provider-write", Type: "function", Function: agent.FunctionCall{Name: "write", Arguments: `{}`},
		}}),
		agent.AssistantMessage("finished", nil),
	}}
	hostEffects := 0
	runtime, err := NewAgentRuntime(ctx, t.TempDir(), WithHostEffectReconciler(
		func(context.Context, agenttoolruntime.CommittedToolMutation) error {
			hostEffects++
			return nil
		},
	))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	var verified []agenttool.Mutation
	var verification agenttool.Verification
	var events []agentrun.Event
	operation, err := runtime.Start(ctx, StartRequest{Cycle: Cycle{
		Definition: agent.Definition{
			Key: "public-backend-mutation", Name: "root", Model: model,
			ModelIdentity: agent.CapabilityIdentity{Kind: "model.public-backend-mutation", Version: 1},
			Permission:    agentpermission.FullAccess(),
			Tools: agent.StaticToolsIdentified(agent.CapabilityIdentity{Kind: "tools.public-backend-mutation", Version: 1}, agent.ToolDefinition{
				Tool: tool, Descriptor: descriptor,
			}),
		},
		Conversation: agentconversation.NewSessionConversationForAgent(sess, nil, agentrun.AgentKindIDE),
		Request:      agentchatRequest("mutation-command", "change it"),
		Options: agentrun.Options{
			AgentKind: agentrun.AgentKindIDE, SessionID: sess.ID, Workspace: workspace,
			TaskID: "mutation-task", RootAgentName: "root",
			OnMutationsVerified: func(_ context.Context, mutations []agenttool.Mutation, value agenttool.Verification) {
				verified = append([]agenttool.Mutation(nil), mutations...)
				verification = value
			},
		},
	}, Emit: func(event agentrun.Event) { events = append(events, event) }})
	if err != nil {
		t.Fatal(err)
	}
	if outcome := operation.Wait(ctx); outcome.Status != agentrun.OutcomeCompleted {
		t.Fatalf("outcome = %#v", outcome)
	}
	if hostEffects != 1 || len(verified) != 1 || verification.Mutations != 1 {
		t.Fatalf("host_effects=%d verified=%#v verification=%#v", hostEffects, verified, verification)
	}
	verificationIndex, doneIndex := -1, -1
	for index, event := range events {
		switch event.Type {
		case "verification":
			verificationIndex = index
		case "done":
			doneIndex = index
		}
	}
	if verificationIndex < 0 || doneIndex < 0 || verificationIndex >= doneIndex {
		t.Fatalf("verification must precede done: %#v", events)
	}
}

func clonePublicBackendMessages(values []*agent.Message) []*agent.Message {
	result := make([]*agent.Message, len(values))
	for index, value := range values {
		if value != nil {
			result[index] = value.Clone()
		}
	}
	return result
}

func TestAgentRuntimeCommitsARealDenovaSessionAndFinalizesDisplay(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("public-runtime-session")
	if err != nil {
		t.Fatal(err)
	}
	conversation := agentconversation.NewSessionConversationForAgent(sess, nil, agentrun.AgentKindIDE)
	model := &publicBackendTestModel{}
	inputCallbacks := 0
	var events []agentrun.Event
	runtime, err := NewAgentRuntime(ctx, t.TempDir(), WithHostEffectReconciler(
		func(context.Context, agenttoolruntime.CommittedToolMutation) error { return nil },
	))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	operation, err := runtime.Start(ctx, StartRequest{Cycle: Cycle{
		Definition: agent.Definition{
			Key: "public-backend-test", Name: "root", Instructions: "answer",
			Model: model, ModelIdentity: agent.CapabilityIdentity{Kind: "model.public-backend-test", Version: 1},
		},
		Conversation: conversation,
		Request:      agentchatRequest("public-command", "hello"),
		Options: agentrun.Options{
			AgentKind: agentrun.AgentKindIDE, SessionID: sess.ID, Workspace: workspace,
			TaskID: "task", RootAgentName: "root",
			OnUserMessageCommitted: func(context.Context) error {
				inputCallbacks++
				return nil
			},
		},
	}, Emit: func(event agentrun.Event) { events = append(events, event) }})
	if err != nil {
		t.Fatal(err)
	}
	outcome := operation.Wait(ctx)
	if outcome.Status != agentrun.OutcomeCompleted || outcome.Content != "public runtime answer" {
		t.Fatalf("outcome = %#v", outcome)
	}
	if inputCallbacks != 1 {
		t.Fatalf("input callbacks = %d", inputCallbacks)
	}
	messages := sess.GetMessages()
	if len(messages) != 2 || messages[0].Role != agent.User || messages[0].Content != "hello" ||
		messages[1].Role != agent.Assistant || messages[1].Content != "public runtime answer" {
		t.Fatalf("canonical session messages = %#v", messages)
	}
	types := make([]string, len(events))
	for index, event := range events {
		types[index] = event.Type
	}
	if !containsPublicBackendEvent(types, "chunk") || !containsPublicBackendEvent(types, "token_usage") ||
		!containsPublicBackendEvent(types, "done") {
		t.Fatalf("display events = %v", types)
	}
}

func TestAgentRuntimeColdReplayUsesDurableOutputWithoutCallingModel(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	dataDir := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("public-runtime-replay")
	if err != nil {
		t.Fatal(err)
	}
	model := &publicBackendTestModel{}
	cycle := Cycle{
		Definition: agent.Definition{
			Key: "public-backend-replay", Name: "root", Model: model,
			ModelIdentity: agent.CapabilityIdentity{Kind: "model.public-backend-replay", Version: 1},
		},
		Conversation: agentconversation.NewSessionConversationForAgent(sess, nil, agentrun.AgentKindIDE),
		Request:      agentchatRequest("replay-command", "hello once"),
		Options: agentrun.Options{
			AgentKind: agentrun.AgentKindIDE, SessionID: sess.ID, Workspace: workspace,
			TaskID: "replay-task", RootAgentName: "root",
		},
	}
	newRuntime := func() *Runtime {
		runtime, runtimeErr := NewAgentRuntime(ctx, dataDir, WithHostEffectReconciler(
			func(context.Context, agenttoolruntime.CommittedToolMutation) error { return nil },
		))
		if runtimeErr != nil {
			t.Fatal(runtimeErr)
		}
		return runtime
	}
	first := newRuntime()
	firstOperation, err := first.Start(ctx, StartRequest{Cycle: cycle})
	if err != nil {
		t.Fatal(err)
	}
	if outcome := firstOperation.Wait(ctx); outcome.Status != agentrun.OutcomeCompleted {
		t.Fatalf("first outcome = %#v", outcome)
	}
	if err := first.Close(ctx); err != nil {
		t.Fatal(err)
	}

	var replayEvents []agentrun.Event
	second := newRuntime()
	t.Cleanup(func() { _ = second.Close(context.Background()) })
	replayOperation, err := second.Start(ctx, StartRequest{
		Cycle: cycle, Emit: func(event agentrun.Event) { replayEvents = append(replayEvents, event) },
	})
	if err != nil {
		t.Fatal(err)
	}
	outcome := replayOperation.Wait(ctx)
	if outcome.Status != agentrun.OutcomeCompleted || outcome.Content != "public runtime answer" {
		t.Fatalf("replay outcome = %#v", outcome)
	}
	model.mu.Lock()
	modelCalls := len(model.inputs)
	model.mu.Unlock()
	if modelCalls != 1 {
		t.Fatalf("model calls = %d, want one initial call", modelCalls)
	}
	if len(replayEvents) == 0 || replayEvents[len(replayEvents)-1].Type != "done" {
		t.Fatalf("replay events = %#v", replayEvents)
	}
}

func TestAgentRuntimeDisplayCancellationExplicitlyAbortsDurableRun(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("public-runtime-cancel")
	if err != nil {
		t.Fatal(err)
	}
	model := &publicBackendBlockingModel{started: make(chan struct{})}
	runtime, err := NewAgentRuntime(ctx, t.TempDir(), WithHostEffectReconciler(
		func(context.Context, agenttoolruntime.CommittedToolMutation) error { return nil },
	))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	options := agentrun.Options{
		AgentKind: agentrun.AgentKindIDE, SessionID: sess.ID, Workspace: workspace,
		TaskID: "cancel-task", RootAgentName: "root",
	}
	operation, err := runtime.Start(ctx, StartRequest{Cycle: Cycle{
		Definition: agent.Definition{
			Key: "public-backend-cancel", Name: "root", Model: model,
			ModelIdentity: agent.CapabilityIdentity{Kind: "model.public-backend-cancel", Version: 1},
		},
		Conversation: agentconversation.NewSessionConversationForAgent(sess, nil, agentrun.AgentKindIDE),
		Request:      agentchatRequest("cancel-command", "wait"), Options: options,
	}})
	if err != nil {
		t.Fatal(err)
	}
	<-model.started
	waitCtx, cancel := context.WithCancel(ctx)
	cancel()
	if outcome := operation.Wait(waitCtx); outcome.Status != agentrun.OutcomeAborted {
		t.Fatalf("cancel outcome = %#v", outcome)
	}
	status, err := runtime.RuntimeStatusProjection(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	if status.Phase != agentrun.RunPhaseIdle || status.LastOperation == nil || status.LastOperation.Status != agentrun.OperationAborted {
		t.Fatalf("cancelled runtime status = %#v", status)
	}
}

func agentchatRequest(commandID, message string) agentchat.ChatRequest {
	return agentchat.ChatRequest{CommandID: commandID, Message: message, Locale: "en-US"}
}

func containsPublicBackendEvent(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
