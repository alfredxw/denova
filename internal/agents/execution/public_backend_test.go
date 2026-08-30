package execution

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"denova/config"
	agentchat "denova/internal/agents/chat"
	agentconversation "denova/internal/agents/conversation"
	agentlifecycle "denova/internal/agents/lifecycle"
	agentprompts "denova/internal/agents/prompts"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	novaskills "denova/internal/agents/skills"
	agenttool "denova/internal/agents/tool"
	agenttoolruntime "denova/internal/agents/toolruntime"
	workspacechange "denova/internal/workspace/change"

	agent "github.com/alfredxw/denova/agent"
	agentpermission "github.com/alfredxw/denova/agent/permission"
	"github.com/alfredxw/denova/agent/providers"
	publictools "github.com/alfredxw/denova/agent/tools"
)

type publicBackendTestModel struct {
	mu        sync.Mutex
	inputs    [][]*agent.Message
	responses []*agent.Message
}

type publicBackendSkillCaptureModel struct {
	mu                     sync.Mutex
	input                  []*agent.Message
	eventsAtFirstModelCall []agentrun.Event
	events                 func() []agentrun.Event
}

func (model *publicBackendSkillCaptureModel) Generate(_ context.Context, input []*agent.Message, _ ...agent.ModelOption) (*agent.Message, error) {
	return model.capture(input), nil
}

func (model *publicBackendSkillCaptureModel) Stream(_ context.Context, input []*agent.Message, _ ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	return agent.StreamReaderFromArray([]*agent.Message{model.capture(input)}), nil
}

func (model *publicBackendSkillCaptureModel) capture(input []*agent.Message) *agent.Message {
	model.mu.Lock()
	defer model.mu.Unlock()
	if model.input == nil {
		model.input = clonePublicBackendMessages(input)
		model.eventsAtFirstModelCall = model.events()
	}
	return agent.AssistantMessage("done", nil)
}

type publicBackendBlockingModel struct{ started chan struct{} }

type publicBackendSteerModel struct {
	mu      sync.Mutex
	calls   int
	started chan struct{}
	release chan struct{}
}

type publicBackendNextTurnModel struct {
	mu      sync.Mutex
	calls   int
	started chan struct{}
	release chan struct{}
}

func (model *publicBackendNextTurnModel) Generate(ctx context.Context, _ []*agent.Message, _ ...agent.ModelOption) (*agent.Message, error) {
	return model.next(ctx)
}

func (model *publicBackendNextTurnModel) Stream(ctx context.Context, _ []*agent.Message, _ ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	message, err := model.next(ctx)
	if err != nil {
		return nil, err
	}
	return agent.StreamReaderFromArray([]*agent.Message{message}), nil
}

func (model *publicBackendNextTurnModel) next(ctx context.Context) (*agent.Message, error) {
	model.mu.Lock()
	model.calls++
	call := model.calls
	model.mu.Unlock()
	if call == 1 {
		close(model.started)
		select {
		case <-model.release:
			return agent.AssistantMessage("first answer", nil), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return agent.AssistantMessage("second answer", nil), nil
}

func (model *publicBackendSteerModel) Generate(ctx context.Context, input []*agent.Message, options ...agent.ModelOption) (*agent.Message, error) {
	return model.next(ctx)
}

func (model *publicBackendSteerModel) Stream(ctx context.Context, input []*agent.Message, options ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	message, err := model.next(ctx)
	if err != nil {
		return nil, err
	}
	return agent.StreamReaderFromArray([]*agent.Message{message}), nil
}

func (model *publicBackendSteerModel) next(ctx context.Context) (*agent.Message, error) {
	model.mu.Lock()
	model.calls++
	call := model.calls
	model.mu.Unlock()
	if call == 1 {
		close(model.started)
		select {
		case <-model.release:
			return agent.AssistantMessage("obsolete answer", nil), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return agent.AssistantMessage("steered answer", nil), nil
}

type publicBackendTestProfile struct {
	prepare   func(context.Context, CycleRestoreRequest) (Cycle, error)
	canonical func(context.Context, CanonicalInputRequest) (agent.CanonicalAdapter, error)
}

func (profile publicBackendTestProfile) ID() ProfileID { return ProfileWriting }

func (profile publicBackendTestProfile) PrepareCycle(ctx context.Context, request CycleRestoreRequest) (Cycle, error) {
	return profile.prepare(ctx, request)
}

func (profile publicBackendTestProfile) CanonicalInput(ctx context.Context, request CanonicalInputRequest) (agent.CanonicalAdapter, error) {
	if profile.canonical == nil {
		return nil, errors.New("public backend test Profile has no canonical input boundary")
	}
	return profile.canonical(ctx, request)
}

func publicBackendTestSessionCanonical(
	sess *session.Session,
) func(context.Context, CanonicalInputRequest) (agent.CanonicalAdapter, error) {
	return func(_ context.Context, request CanonicalInputRequest) (agent.CanonicalAdapter, error) {
		conversation := agentconversation.NewSessionConversationForAgent(sess, nil, request.Options.AgentKind)
		committer, err := agentlifecycle.NewSessionConversationCommitter(agentlifecycle.SessionCommitterConfig{
			Conversation: conversation,
			Session:      sess,
			Options:      request.Options,
			Request:      request.Request,
			InputEffect:  request.Options.InputCommitEffect,
		})
		if err != nil {
			return nil, err
		}
		boundary, err := agentlifecycle.NewConversationBoundary(agentlifecycle.ConversationBoundaryConfig{
			Conversation:      conversation,
			Request:           request.Request,
			Options:           request.Options,
			Committer:         committer,
			ContextIdentity:   agent.CapabilityIdentity{Kind: "context.public-backend-test-admission", Version: 1},
			CanonicalIdentity: request.Identity,
		})
		if err != nil {
			return nil, err
		}
		return boundary.CanonicalAdapter(), nil
	}
}

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
	runtime, err := NewAgentRuntime(ctx, t.TempDir(), WithToolMutationApplier(
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
	toolset, err := agent.StaticToolsIdentified(agent.CapabilityIdentity{Kind: "tools.public-backend-mutation", Version: 1}, agent.ToolDefinition{
		Tool: tool, Descriptor: descriptor,
	})
	if err != nil {
		t.Fatal(err)
	}
	operation, err := runtime.Start(ctx, StartRequest{Cycle: Cycle{
		Definition: agent.Definition{
			Key: "public-backend-mutation", Name: "root", Model: model,
			ModelIdentity: agent.CapabilityIdentity{Kind: "model.public-backend-mutation", Version: 1},
			Permission:    agentpermission.FullAccess(),
			Tools:         toolset,
		},
		Conversation: agentconversation.NewSessionConversationForAgent(sess, nil, agentrun.AgentKindIDE),
		Request:      agentchatRequest("mutation-command", "change it"),
		Options: agentrun.Options{
			AgentKind: agentrun.AgentKindIDE, ProjectID: "project-test", SessionID: sess.ID, Workspace: workspace,
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
	trace, err := agentrun.ReadRunTrace(
		agentrun.TraceLocation{Workspace: workspace}, string(operation.Receipt().OperationID),
	)
	if err != nil {
		t.Fatalf("read public Agent run trace: %v", err)
	}
	if trace.Summary.ToolCalls != 1 || trace.Summary.ToolSuccesses != 1 || trace.Summary.LLMCalls != 2 || trace.Summary.Status != "success" {
		t.Fatalf("public Agent run trace summary = %#v", trace.Summary)
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

func TestAgentRuntimeWorkspaceMutationsRetainConversationDiffReviewScope(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "edited.md"), []byte("before edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "deleted.md"), []byte("before delete\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("public-runtime-diff-review")
	if err != nil {
		t.Fatal(err)
	}
	definitions, err := agenttoolruntime.NewCatalog(&config.Config{Workspace: workspace}).Workspace(
		config.ResolvedAgentToolSettings{config.AgentToolWorkspaceWrite: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	model := &publicBackendTestModel{responses: []*agent.Message{
		agent.AssistantMessage("", []agent.ToolCall{
			{ID: "provider-write", Type: "function", Function: agent.FunctionCall{
				Name: "write", Arguments: `{"path":"created.md","content":"created\n"}`,
			}},
			{ID: "provider-edit", Type: "function", Function: agent.FunctionCall{
				Name: "edit", Arguments: `{"path":"edited.md","edits":[{"old_string":"before edit","new_string":"after edit"}]}`,
			}},
			{ID: "provider-delete", Type: "function", Function: agent.FunctionCall{
				Name: "edit", Arguments: `{"path":"deleted.md","operation":"delete"}`,
			}},
		}),
		agent.AssistantMessage("finished", nil),
	}}
	runtime, err := NewAgentRuntime(ctx, t.TempDir(), WithToolMutationApplier(
		func(context.Context, agenttoolruntime.CommittedToolMutation) error { return nil },
	))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	toolset, err := agent.StaticToolsIdentified(
		agent.CapabilityIdentity{Kind: "tools.public-backend-diff-review", Version: 1}, definitions...,
	)
	if err != nil {
		t.Fatal(err)
	}
	operation, err := runtime.Start(ctx, StartRequest{Cycle: Cycle{
		Definition: agent.Definition{
			Key: "public-backend-diff-review", Name: "root", Model: model,
			ModelIdentity: agent.CapabilityIdentity{Kind: "model.public-backend-diff-review", Version: 1},
			Permission:    agentpermission.FullAccess(),
			Tools:         toolset,
		},
		Conversation: agentconversation.NewSessionConversationForAgent(sess, nil, agentrun.AgentKindIDE),
		Request:      agentchatRequest("diff-review-command", "mutate three files"),
		Options: agentrun.Options{
			AgentKind: agentrun.AgentKindIDE, ProjectID: "project-test", SessionID: sess.ID, Workspace: workspace,
			TaskID: "diff-review-task", RootAgentName: "root",
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if outcome := operation.Wait(ctx); outcome.Status != agentrun.OutcomeCompleted {
		t.Fatalf("outcome = %#v", outcome)
	}
	runID := string(operation.Receipt().OperationID)
	changes, err := workspacechange.ForWorkspace(workspace)
	if err != nil {
		t.Fatal(err)
	}
	groups, err := changes.ListGroups(ctx, workspacechange.ChangeFilter{SessionID: sess.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || groups[0].ID != runID || groups[0].ReviewThreadID != runID ||
		groups[0].ChangeSetCount != 3 {
		t.Fatalf("conversation Diff review groups = %#v, run_id=%q", groups, runID)
	}
	thread, err := changes.GetReviewThread(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	files := make(map[string]workspacechange.ReviewThreadFile, len(thread.Files))
	for _, file := range thread.Files {
		files[file.Path] = file
	}
	if created := files["created.md"]; created.BeforeExists || !created.AfterExists || created.AfterContent != "created\n" {
		t.Fatalf("created file Diff = %#v", created)
	}
	if edited := files["edited.md"]; !edited.BeforeExists || !edited.AfterExists ||
		edited.BeforeContent != "before edit\n" || edited.AfterContent != "after edit\n" {
		t.Fatalf("edited file Diff = %#v", edited)
	}
	if deleted := files["deleted.md"]; !deleted.BeforeExists || deleted.AfterExists ||
		deleted.BeforeContent != "before delete\n" || deleted.AfterContent != "" {
		t.Fatalf("deleted file Diff = %#v", deleted)
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

func writePublicBackendSkillFixture(t *testing.T, root, name, body string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: " + name + "\ndescription: " + name + "\nagent: " + config.AgentKindIDE + "\n---\n\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(dir, novaskills.SkillFileName), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func lastPublicBackendUserContent(messages []*agent.Message) string {
	for index := len(messages) - 1; index >= 0; index-- {
		if messages[index] != nil && messages[index].Role == agent.User {
			return messages[index].Content
		}
	}
	return ""
}

func publicBackendSkillEventNames(events []agentrun.Event) []string {
	result := make([]string, 0)
	for _, event := range events {
		if (event.Type != "tool_call" && event.Type != "tool_result") || event.DataString("name") != "skill" {
			continue
		}
		name := ""
		if event.Type == "tool_call" {
			var arguments struct {
				Name string `json:"name"`
			}
			_ = json.Unmarshal([]byte(event.DataString("args")), &arguments)
			name = arguments.Name
		} else {
			name = strings.TrimPrefix(strings.SplitN(event.DataString("content"), "\n", 2)[0], "# Skill: ")
		}
		result = append(result, event.Type+":"+name)
	}
	return result
}

func publicBackendPersistedSkillNames(history []session.HistoryEntry) []string {
	result := make([]string, 0)
	for _, entry := range history {
		if entry.Role != "tool_call" || entry.Name != "skill" {
			continue
		}
		var arguments struct {
			Name string `json:"name"`
		}
		_ = json.Unmarshal([]byte(entry.Args), &arguments)
		result = append(result, arguments.Name)
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
	runtime, err := NewAgentRuntime(ctx, t.TempDir(), WithToolMutationApplier(
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
			AgentKind: agentrun.AgentKindIDE, ProjectID: "project-test", SessionID: sess.ID, Workspace: workspace,
			TaskID: "task", RootAgentName: "root",
			InputCommitEffect: agentrun.InputCommitEffectFuncs{
				ApplyFunc: func(context.Context, agentrun.InputCommitEffectRequest) error {
					inputCallbacks++
					return nil
				},
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
	runID := string(operation.Receipt().OperationID)
	if !strings.HasPrefix(runID, "run-") {
		t.Fatalf("public run id = %q, want run- prefix", runID)
	}
	trace, err := agentrun.ReadRunTrace(agentrun.TraceLocation{Workspace: workspace}, runID)
	if err != nil {
		t.Fatalf("read public Agent run trace: %v", err)
	}
	if trace.Summary.ID != runID || trace.Summary.Status != "success" || trace.Summary.LLMCalls != 1 ||
		trace.Summary.PromptTokens != 20 || trace.Summary.CachedPromptTokens != 12 || trace.Summary.UncachedPromptTokens != 8 {
		t.Fatalf("public Agent run trace summary = %#v", trace.Summary)
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

func TestAgentRuntimeBindsProviderTraceToDurablePublicRun(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("public-runtime-provider-trace")
	if err != nil {
		t.Fatal(err)
	}
	model := &publicBackendTestModel{responses: []*agent.Message{{
		Role: agent.Assistant, Content: "traced answer",
		Extra: map[string]any{"openai-request-id": "provider-request-1"},
		ResponseMeta: &agent.ResponseMeta{FinishReason: "stop", Usage: &agent.TokenUsage{
			PromptTokens: 20, PromptTokenDetails: agent.PromptTokenDetails{CachedTokens: 12},
			CompletionTokens: 4, TotalTokens: 24,
		}},
	}}}
	runtime, err := NewAgentRuntime(ctx, t.TempDir(), WithToolMutationApplier(
		func(context.Context, agenttoolruntime.CommittedToolMutation) error { return nil },
	))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	operation, err := runtime.Start(ctx, StartRequest{Cycle: Cycle{
		Definition: agent.Definition{
			Key: "public-backend-provider-trace", Name: "root", Model: model,
			ModelIdentity: agent.CapabilityIdentity{Kind: "model.public-backend-provider-trace", Version: 1},
			Middlewares: []agent.Middleware{agent.IdentifyMiddleware(
				agentrun.NewModelInputLoggingMiddleware(
					agentrun.AgentKindIDE,
					providers.ModelConfig{Provider: providers.ProviderOpenAI, Protocol: providers.ProtocolOpenAIResponses, Model: "test-model"},
					0,
					0,
					agentprompts.SystemPromptComposition{},
				),
				agent.CapabilityIdentity{Kind: "middleware.provider-trace", Version: 1},
			)},
		},
		Conversation: agentconversation.NewSessionConversationForAgent(sess, nil, agentrun.AgentKindIDE),
		Request:      agentchatRequest("provider-trace-command", "trace it"),
		Options: agentrun.Options{
			AgentKind: agentrun.AgentKindIDE, ProjectID: "project-test", SessionID: sess.ID, Workspace: workspace,
			TaskID: "provider-trace-task", RootAgentName: "root",
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if outcome := operation.Wait(ctx); outcome.Status != agentrun.OutcomeCompleted {
		t.Fatalf("outcome = %#v", outcome)
	}
	runID := string(operation.Receipt().OperationID)
	trace, err := agentrun.ReadRunTrace(agentrun.TraceLocation{Workspace: workspace}, runID)
	if err != nil {
		t.Fatal(err)
	}
	if trace.Summary.ID != runID || trace.Summary.LLMCalls != 1 || trace.Summary.PromptTokens != 20 ||
		trace.Summary.CachedPromptTokens != 12 || trace.Summary.Status != "success" {
		t.Fatalf("provider trace summary = %#v", trace.Summary)
	}
	llmCalls := 0
	for _, record := range trace.Records {
		if record.Type != "llm_call" {
			continue
		}
		llmCalls++
		attrs, _ := record.Data["attrs"].(map[string]any)
		if attrs["provider_request_id"] != "provider-request-1" || attrs["model"] != "test-model" {
			t.Fatalf("provider trace attrs = %#v", attrs)
		}
	}
	if llmCalls != 1 {
		t.Fatalf("llm trace records = %d, want one provider-boundary record", llmCalls)
	}
}

func TestAgentRuntimeLoadsExplicitSkillsBeforeFirstModelCallAndPersistsCards(t *testing.T) {
	ctx := context.Background()
	skillsDir := t.TempDir()
	writePublicBackendSkillFixture(t, skillsDir, "alpha", "ALPHA_BODY")
	writePublicBackendSkillFixture(t, skillsDir, "beta", "BETA_BODY")
	workspace := t.TempDir()
	cfg := &config.Config{SkillsDir: skillsDir, Workspace: workspace, DenovaDir: t.TempDir()}
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("public-runtime-explicit-skills")
	if err != nil {
		t.Fatal(err)
	}
	var eventsMu sync.Mutex
	var events []agentrun.Event
	snapshotEvents := func() []agentrun.Event {
		eventsMu.Lock()
		defer eventsMu.Unlock()
		return append([]agentrun.Event(nil), events...)
	}
	model := &publicBackendSkillCaptureModel{events: snapshotEvents}
	runtime, err := NewAgentRuntime(ctx, t.TempDir(), WithToolMutationApplier(
		func(context.Context, agenttoolruntime.CommittedToolMutation) error { return nil },
	))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	message := "先处理背景，再用 /alpha，然后 /beta，最后按 /alpha 核对。"
	operation, err := runtime.Start(ctx, StartRequest{Cycle: Cycle{
		Definition: agent.Definition{
			Key: "public-backend-explicit-skills", Name: "root", Model: model,
			ModelIdentity: agent.CapabilityIdentity{Kind: "model.public-backend-explicit-skills", Version: 1},
		},
		Conversation: agentconversation.NewSessionConversationForAgent(sess, cfg, agentrun.AgentKindIDE),
		Request:      agentchatRequest("explicit-skills-command", message),
		Options: agentrun.Options{
			AgentKind: agentrun.AgentKindIDE, ProjectID: "project-test", SessionID: sess.ID, Workspace: workspace,
			TaskID: "explicit-skills-task", RootAgentName: "root",
		},
	}, Emit: func(event agentrun.Event) {
		eventsMu.Lock()
		events = append(events, event)
		eventsMu.Unlock()
	}})
	if err != nil {
		t.Fatal(err)
	}
	if outcome := operation.Wait(ctx); outcome.Status != agentrun.OutcomeCompleted {
		t.Fatalf("outcome = %#v", outcome)
	}
	model.mu.Lock()
	input := clonePublicBackendMessages(model.input)
	firstCallEvents := append([]agentrun.Event(nil), model.eventsAtFirstModelCall...)
	model.mu.Unlock()
	finalUser := lastPublicBackendUserContent(input)
	for _, want := range []string{"# Skill: alpha", "ALPHA_BODY", "# Skill: beta", "BETA_BODY", message} {
		if !strings.Contains(finalUser, want) {
			t.Fatalf("first model input does not contain %q:\n%s", want, finalUser)
		}
	}
	if strings.Count(finalUser, "# Skill: alpha") != 1 {
		t.Fatalf("duplicate Skill invocation entered model input:\n%s", finalUser)
	}
	if got := publicBackendSkillEventNames(firstCallEvents); strings.Join(got, ",") != "tool_call:alpha,tool_result:alpha,tool_call:beta,tool_result:beta" {
		t.Fatalf("visible Skill events before first model call = %#v", got)
	}
	if got := publicBackendPersistedSkillNames(sess.History()); strings.Join(got, ",") != "alpha,beta" {
		t.Fatalf("persisted explicit Skill cards = %#v", got)
	}
}

func TestAgentRuntimePlanAskPersistsAndResumesSamePublicRun(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("public-runtime-plan-ask")
	if err != nil {
		t.Fatal(err)
	}
	ask := publictools.Ask()
	model := &publicBackendTestModel{responses: []*agent.Message{
		agent.AssistantMessage("", []agent.ToolCall{{
			ID: "provider-plan-ask", Type: "function", Function: agent.FunctionCall{
				Name: "ask", Arguments: `{"questions":[{"id":"scope","prompt":"Choose scope","options":[{"value":"minimal","label":"Minimal","description":"Only the shared flow.","recommended":true},{"value":"full","label":"Full","description":"Include adjacent controls."}]}]}`,
			},
		}}),
		agent.AssistantMessage("<proposed_plan># Plan\n\n1. Apply the shared flow.</proposed_plan>", nil),
	}}
	orchestrator := agenttoolruntime.NewOrchestratorMiddleware(agenttoolruntime.OrchestratorConfig{
		AgentKind: agentrun.AgentKindIDE,
		ToolSettings: config.ResolvedAgentToolSettings{
			config.AgentToolAsk: true,
		},
		EnforceToolSettings: true,
		Workspace:           workspace,
	})
	runtime, err := NewAgentRuntime(ctx, t.TempDir(), WithToolMutationApplier(
		func(context.Context, agenttoolruntime.CommittedToolMutation) error { return nil },
	))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	options := agentrun.Options{
		AgentKind: agentrun.AgentKindIDE, ProjectID: "project-test", SessionID: sess.ID, Workspace: workspace,
		TaskID: "plan-ask-task", RootAgentName: "root",
	}
	pending := make(chan map[string]any, 1)
	var eventsMu sync.Mutex
	var events []agentrun.Event
	operation, err := runtime.Start(ctx, StartRequest{Cycle: Cycle{
		Definition: agent.Definition{
			Key: "public-backend-plan-ask", Name: "root", Model: model,
			ModelIdentity: agent.CapabilityIdentity{Kind: "model.public-backend-plan-ask", Version: 1},
			Tools:         ask,
			Middlewares: []agent.Middleware{agent.IdentifyMiddleware(
				orchestrator, agent.CapabilityIdentity{Kind: "middleware.public-backend-plan-ask", Version: 1},
			)},
		},
		Conversation: agentconversation.NewSessionConversationForAgent(sess, &config.Config{Workspace: workspace}, agentrun.AgentKindIDE),
		Request: agentchat.ChatRequest{
			CommandID: "plan-ask-command", Message: "Plan the refactor", PlanMode: true,
		},
		Options: options,
	}, Emit: func(event agentrun.Event) {
		eventsMu.Lock()
		events = append(events, event)
		eventsMu.Unlock()
		if event.Type == "ask_pending" {
			if interaction, ok := event.Data.(map[string]any); ok {
				pending <- interaction
			}
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	outcomes := make(chan agentrun.Outcome, 1)
	go func() { outcomes <- operation.Wait(ctx) }()
	var interaction map[string]any
	select {
	case interaction = <-pending:
	case <-time.After(2 * time.Second):
		t.Fatal("public Agent did not publish a durable Ask interaction")
	}
	interactionID, _ := interaction["id"].(string)
	if interactionID == "" || interaction["status"] != "pending" || interaction["allow_other"] != true {
		t.Fatalf("Ask was projected before durable state: %#v", interaction)
	}
	status, err := runtime.RuntimeStatusProjection(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	foundPending := false
	for _, candidate := range status.PendingInteractions {
		foundPending = foundPending || candidate.ID == interactionID
	}
	if !foundPending {
		t.Fatalf("Ask event had no durable public Interaction: status=%#v interaction=%#v", status, interaction)
	}
	if _, err := runtime.ResolveAsk(ctx, options, interactionID, session.AskAnswered, []agentconversation.HostAskAnswer{{
		QuestionID: "scope", SelectedOptionIDs: []string{"minimal"},
	}}, ""); err != nil {
		t.Fatal(err)
	}
	var outcome agentrun.Outcome
	select {
	case outcome = <-outcomes:
	case <-time.After(2 * time.Second):
		t.Fatal("public Agent did not resume after Ask resolution")
	}
	if outcome.Status != agentrun.OutcomeCompleted {
		t.Fatalf("outcome = %#v", outcome)
	}
	model.mu.Lock()
	inputs := append([][]*agent.Message(nil), model.inputs...)
	model.mu.Unlock()
	if len(inputs) != 2 || !strings.Contains(lastPublicBackendUserContent(inputs[1]), "Plan the refactor") {
		t.Fatalf("model inputs = %#v", inputs)
	}
	secondInput := ""
	for _, message := range inputs[1] {
		if message != nil {
			secondInput += message.Content
		}
	}
	if !strings.Contains(secondInput, `"question_id":"scope"`) || !strings.Contains(secondInput, `"values":["minimal"]`) {
		t.Fatalf("Ask resolution missing from second model call: %s", secondInput)
	}
	eventsMu.Lock()
	projected := append([]agentrun.Event(nil), events...)
	eventsMu.Unlock()
	if countPublicBackendEvent(projected, "ask_pending") != 1 || countPublicBackendEvent(projected, "ask_resolved") != 1 ||
		countPublicBackendEvent(projected, "proposed_plan") == 0 || countPublicBackendEvent(projected, "done") != 1 {
		t.Fatalf("public Plan/Ask events = %#v", projected)
	}
}

func TestAgentRuntimeSteerPreemptsAndContinuesSamePublicRun(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("public-runtime-steer")
	if err != nil {
		t.Fatal(err)
	}
	model := &publicBackendSteerModel{started: make(chan struct{}), release: make(chan struct{})}
	definition := agent.Definition{
		Key: "public-backend-steer", Name: "root", Model: model,
		ModelIdentity: agent.CapabilityIdentity{Kind: "model.public-backend-steer", Version: 1},
		Middlewares: []agent.Middleware{agent.IdentifyMiddleware(
			agentrun.NewModelInputLoggingMiddleware(
				agentrun.AgentKindIDE,
				providers.ModelConfig{Provider: providers.ProviderOpenAI, Protocol: providers.ProtocolOpenAIResponses, Model: "steer-model"},
				0,
				0,
				agentprompts.SystemPromptComposition{},
			),
			agent.CapabilityIdentity{Kind: "middleware.public-backend-steer", Version: 1},
		)},
	}
	options := agentrun.Options{
		AgentKind: agentrun.AgentKindIDE, ProjectID: "project-test", SessionID: sess.ID, Workspace: workspace,
		TaskID: "steer-task", RootAgentName: "root",
	}
	var prepareMu sync.Mutex
	var prepared []CycleRestoreRequest
	profile := publicBackendTestProfile{prepare: func(_ context.Context, request CycleRestoreRequest) (Cycle, error) {
		prepareMu.Lock()
		prepared = append(prepared, request)
		prepareMu.Unlock()
		return Cycle{
			Definition:   definition,
			Conversation: agentconversation.NewSessionConversationForAgent(sess, nil, agentrun.AgentKindIDE),
			Request:      request.Request,
			Options:      request.Options,
		}, nil
	}, canonical: publicBackendTestSessionCanonical(sess)}
	runtime, err := NewAgentRuntime(ctx, t.TempDir(), WithProfiles(profile), WithToolMutationApplier(
		func(context.Context, agenttoolruntime.CommittedToolMutation) error { return nil },
	))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	var eventsMu sync.Mutex
	var events []agentrun.Event
	emit := func(event agentrun.Event) {
		eventsMu.Lock()
		events = append(events, event)
		eventsMu.Unlock()
	}
	operation, err := runtime.Start(ctx, StartRequest{Cycle: Cycle{
		Definition:   definition,
		Conversation: agentconversation.NewSessionConversationForAgent(sess, nil, agentrun.AgentKindIDE),
		Request:      agentchatRequest("start-command", "initial request"), Options: options,
	}, Emit: emit})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-model.started:
	case <-time.After(2 * time.Second):
		t.Fatal("initial model call did not start")
	}
	receipt, err := runtime.SubmitCommand(ctx, CommandRequest{
		Kind: CommandSteer, CommandID: "steer-command", OperationID: operation.Receipt().OperationID,
		Request: agentchatRequest("steer-command", "new direction"), Options: options, Emit: emit,
	})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.OperationID != operation.Receipt().OperationID {
		t.Fatalf("steer receipt = %#v, want same operation", receipt)
	}
	close(model.release)
	outcome := operation.Wait(ctx)
	if outcome.Status != agentrun.OutcomeCompleted || outcome.Content != "steered answer" {
		t.Fatalf("steered outcome = %#v", outcome)
	}
	model.mu.Lock()
	calls := model.calls
	model.mu.Unlock()
	if calls != 2 {
		t.Fatalf("model calls = %d, want interrupted initial call plus steer cycle", calls)
	}
	prepareMu.Lock()
	restored := append([]CycleRestoreRequest(nil), prepared...)
	prepareMu.Unlock()
	if len(restored) != 1 || restored[0].Kind != CommandSteer || restored[0].CommandID != "steer-command" ||
		restored[0].Request.CommandID != "steer-command" || restored[0].Request.Message != "new direction" {
		t.Fatalf("steer cycle restoration = %#v", restored)
	}
	messages := sess.GetMessages()
	if len(messages) != 3 || messages[0].Role != agent.User || messages[0].Content != "initial request" ||
		messages[1].Role != agent.User || messages[1].Content != "new direction" ||
		messages[2].Role != agent.Assistant || messages[2].Content != "steered answer" {
		t.Fatalf("steer canonical messages = %#v", messages)
	}
	eventsMu.Lock()
	projected := append([]agentrun.Event(nil), events...)
	eventsMu.Unlock()
	cycles := make([]map[string]any, 0, 2)
	for _, event := range projected {
		if event.Type == "agent_cycle_started" {
			data, _ := event.Data.(map[string]any)
			cycles = append(cycles, data)
		}
	}
	if len(cycles) != 2 || cycles[0]["command_id"] != "start-command" || cycles[0]["delivery"] != "start_turn" ||
		cycles[1]["command_id"] != "steer-command" || cycles[1]["delivery"] != "steer" {
		t.Fatalf("steer cycle edges = %#v", cycles)
	}
	trace, err := agentrun.ReadRunTrace(agentrun.TraceLocation{Workspace: workspace}, string(operation.Receipt().OperationID))
	if err != nil {
		t.Fatal(err)
	}
	if trace.Summary.LLMCalls != 2 || trace.Summary.Status != "success" {
		t.Fatalf("steer trace summary = %#v", trace.Summary)
	}
	runCreated, rootSpans := 0, 0
	for _, record := range trace.Records {
		switch record.Type {
		case "run_created":
			runCreated++
		case "agent_run":
			rootSpans++
		}
	}
	if runCreated != 1 || rootSpans != 1 {
		t.Fatalf("steer trace opened %d ledgers and %d root spans", runCreated, rootSpans)
	}
}

func TestAgentRuntimeFollowUpQueuesAndContinuesSamePublicRun(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("public-runtime-follow-up")
	if err != nil {
		t.Fatal(err)
	}
	model := &publicBackendNextTurnModel{started: make(chan struct{}), release: make(chan struct{})}
	definition := agent.Definition{
		Key: "public-backend-follow-up", Name: "root", Model: model,
		ModelIdentity: agent.CapabilityIdentity{Kind: "model.public-backend-follow-up", Version: 1},
		Middlewares: []agent.Middleware{agent.IdentifyMiddleware(
			agentrun.NewModelInputLoggingMiddleware(
				agentrun.AgentKindIDE,
				providers.ModelConfig{Provider: providers.ProviderOpenAI, Protocol: providers.ProtocolOpenAIResponses, Model: "follow-up-model"},
				0,
				0,
				agentprompts.SystemPromptComposition{},
			),
			agent.CapabilityIdentity{Kind: "middleware.public-backend-follow-up", Version: 1},
		)},
	}
	options := agentrun.Options{
		AgentKind: agentrun.AgentKindIDE, ProjectID: "project-test", SessionID: sess.ID, Workspace: workspace,
		TaskID: "follow-up-task", RootAgentName: "root",
	}
	var prepareMu sync.Mutex
	var prepared []CycleRestoreRequest
	profile := publicBackendTestProfile{prepare: func(_ context.Context, request CycleRestoreRequest) (Cycle, error) {
		prepareMu.Lock()
		prepared = append(prepared, request)
		prepareMu.Unlock()
		return Cycle{
			Definition: definition, Conversation: agentconversation.NewSessionConversationForAgent(sess, nil, agentrun.AgentKindIDE),
			Request: request.Request, Options: request.Options,
		}, nil
	}, canonical: publicBackendTestSessionCanonical(sess)}
	runtime, err := NewAgentRuntime(ctx, t.TempDir(), WithProfiles(profile), WithToolMutationApplier(
		func(context.Context, agenttoolruntime.CommittedToolMutation) error { return nil },
	))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	var eventsMu sync.Mutex
	var events []agentrun.Event
	emit := func(event agentrun.Event) {
		eventsMu.Lock()
		events = append(events, event)
		eventsMu.Unlock()
	}
	operation, err := runtime.Start(ctx, StartRequest{Cycle: Cycle{
		Definition: definition, Conversation: agentconversation.NewSessionConversationForAgent(sess, nil, agentrun.AgentKindIDE),
		Request: agentchatRequest("follow-start", "first request"), Options: options,
	}, Emit: emit})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-model.started:
	case <-time.After(2 * time.Second):
		t.Fatal("first follow-up model call did not start")
	}
	receipt, err := runtime.SubmitCommand(ctx, CommandRequest{
		Kind: CommandFollowUp, CommandID: "follow-command", OperationID: operation.Receipt().OperationID,
		Request: agentchatRequest("", "second request"), Options: options, Emit: emit,
	})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.CommandID != "follow-command" || receipt.OperationID != operation.Receipt().OperationID {
		t.Fatalf("follow-up receipt=%#v, want same public Run", receipt)
	}
	close(model.release)
	outcome := operation.Wait(ctx)
	if outcome.Status != agentrun.OutcomeCompleted || outcome.Content != "second answer" {
		t.Fatalf("follow-up outcome=%#v", outcome)
	}
	prepareMu.Lock()
	restored := append([]CycleRestoreRequest(nil), prepared...)
	prepareMu.Unlock()
	if len(restored) != 1 || restored[0].Kind != CommandFollowUp || restored[0].CommandID != "follow-command" ||
		restored[0].Request.CommandID != "follow-command" || restored[0].Request.Message != "second request" {
		t.Fatalf("follow-up cycle restoration=%#v", restored)
	}
	if restored[0].Emit == nil {
		t.Fatal("follow-up cycle restoration lost its event projection callback")
	}
	messages := sess.GetMessages()
	if len(messages) != 4 || messages[0].Content != "first request" || messages[1].Content != "first answer" ||
		messages[2].Content != "second request" || messages[3].Content != "second answer" {
		t.Fatalf("follow-up canonical messages=%#v", messages)
	}
	eventsMu.Lock()
	projected := append([]agentrun.Event(nil), events...)
	eventsMu.Unlock()
	cycles := make([]map[string]any, 0, 2)
	for _, event := range projected {
		if event.Type == "agent_cycle_started" {
			data, _ := event.Data.(map[string]any)
			cycles = append(cycles, data)
		}
	}
	if len(cycles) != 2 || cycles[0]["command_id"] != "follow-start" || cycles[0]["delivery"] != "start_turn" ||
		cycles[1]["command_id"] != "follow-command" || cycles[1]["delivery"] != "follow_up" ||
		countPublicBackendEvent(projected, "done") != 1 {
		t.Fatalf("follow-up public events=%#v", projected)
	}
	trace, err := agentrun.ReadRunTrace(agentrun.TraceLocation{Workspace: workspace}, string(operation.Receipt().OperationID))
	if err != nil {
		t.Fatal(err)
	}
	if trace.Summary.LLMCalls != 2 || trace.Summary.Status != "success" {
		t.Fatalf("follow-up trace summary=%#v", trace.Summary)
	}
}

func TestAgentRuntimeCancelQueuedRemovesAcceptedFollowUp(t *testing.T) {
	ctx := context.Background()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("public-runtime-cancel-queued")
	if err != nil {
		t.Fatal(err)
	}
	model := &publicBackendNextTurnModel{started: make(chan struct{}), release: make(chan struct{})}
	definition := agent.Definition{
		Key: "public-backend-cancel-queued", Name: "root", Model: model,
		ModelIdentity: agent.CapabilityIdentity{Kind: "model.public-backend-cancel-queued", Version: 1},
	}
	options := agentrun.Options{
		AgentKind: agentrun.AgentKindIDE, ProjectID: "project-test", SessionID: sess.ID, Workspace: t.TempDir(),
		TaskID: "cancel-queued-task", RootAgentName: "root",
	}
	profile := publicBackendTestProfile{prepare: func(_ context.Context, request CycleRestoreRequest) (Cycle, error) {
		return Cycle{
			Definition: definition, Conversation: agentconversation.NewSessionConversationForAgent(sess, nil, agentrun.AgentKindIDE),
			Request: request.Request, Options: request.Options,
		}, nil
	}, canonical: publicBackendTestSessionCanonical(sess)}
	runtime, err := NewAgentRuntime(ctx, t.TempDir(), WithProfiles(profile), WithToolMutationApplier(
		func(context.Context, agenttoolruntime.CommittedToolMutation) error { return nil },
	))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	operation, err := runtime.Start(ctx, StartRequest{Cycle: Cycle{
		Definition: definition, Conversation: agentconversation.NewSessionConversationForAgent(sess, nil, agentrun.AgentKindIDE),
		Request: agentchatRequest("cancel-queued-start", "first request"), Options: options,
	}})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-model.started:
	case <-time.After(2 * time.Second):
		t.Fatal("initial model call did not start")
	}
	queued, err := runtime.SubmitCommand(ctx, CommandRequest{
		Kind: CommandFollowUp, CommandID: "cancel-queued-target", OperationID: operation.Receipt().OperationID,
		Request: agentchatRequest("cancel-queued-target", "should not run"), Options: options,
	})
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := runtime.SubmitCommand(ctx, CommandRequest{
		Kind: CommandCancelQueued, CommandID: "cancel-queued-control", OperationID: operation.Receipt().OperationID,
		TargetCommandID: queued.CommandID, Reason: "caller changed their mind", Options: options,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.CommandID != "cancel-queued-control" || cancelled.OperationID != operation.Receipt().OperationID {
		t.Fatalf("cancel receipt = %#v", cancelled)
	}
	_, err = runtime.SubmitCommand(ctx, CommandRequest{
		Kind: CommandCancelQueued, CommandID: "cancel-queued-again", OperationID: operation.Receipt().OperationID,
		TargetCommandID: queued.CommandID, Reason: "already removed", Options: options,
	})
	if !errors.Is(err, agentrun.ErrQueueConflict) {
		t.Fatalf("second queue cancellation error = %v, want queue conflict", err)
	}
	close(model.release)
	outcome := operation.Wait(ctx)
	if outcome.Status != agentrun.OutcomeCompleted || outcome.Content != "first answer" {
		t.Fatalf("outcome after cancelling queued input = %#v", outcome)
	}
	model.mu.Lock()
	calls := model.calls
	model.mu.Unlock()
	if calls != 1 {
		t.Fatalf("model calls = %d, want cancelled queued input not to execute", calls)
	}
	messages := sess.GetMessages()
	if len(messages) != 2 || messages[0].Content != "first request" || messages[1].Content != "first answer" {
		t.Fatalf("canonical messages after queue cancellation = %#v", messages)
	}
}

func TestAgentRuntimeNextTurnChainsASeparatePublicRunToOneDisplayTask(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("public-runtime-next-turn")
	if err != nil {
		t.Fatal(err)
	}
	model := &publicBackendNextTurnModel{started: make(chan struct{}), release: make(chan struct{})}
	definition := agent.Definition{
		Key: "public-backend-next-turn", Name: "root", Model: model,
		ModelIdentity: agent.CapabilityIdentity{Kind: "model.public-backend-next-turn", Version: 1},
	}
	options := agentrun.Options{
		AgentKind: agentrun.AgentKindIDE, ProjectID: "project-test", SessionID: sess.ID, Workspace: workspace,
		TaskID: "next-turn-task", RootAgentName: "root",
	}
	profile := publicBackendTestProfile{prepare: func(_ context.Context, request CycleRestoreRequest) (Cycle, error) {
		return Cycle{
			Definition:   definition,
			Conversation: agentconversation.NewSessionConversationForAgent(sess, nil, agentrun.AgentKindIDE),
			Request:      request.Request, Options: request.Options,
		}, nil
	}, canonical: publicBackendTestSessionCanonical(sess)}
	runtime, err := NewAgentRuntime(ctx, t.TempDir(), WithProfiles(profile), WithToolMutationApplier(
		func(context.Context, agenttoolruntime.CommittedToolMutation) error { return nil },
	))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	var eventsMu sync.Mutex
	var events []agentrun.Event
	emit := func(event agentrun.Event) {
		eventsMu.Lock()
		events = append(events, event)
		eventsMu.Unlock()
	}
	operation, err := runtime.Start(ctx, StartRequest{Cycle: Cycle{
		Definition:   definition,
		Conversation: agentconversation.NewSessionConversationForAgent(sess, nil, agentrun.AgentKindIDE),
		Request:      agentchatRequest("next-start", "first request"), Options: options,
	}, Emit: emit})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-model.started:
	case <-time.After(2 * time.Second):
		t.Fatal("first next-turn model call did not start")
	}
	receipt, err := runtime.SubmitCommand(ctx, CommandRequest{
		Kind: CommandNextTurn, CommandID: "next-command", AfterOperationID: operation.Receipt().OperationID,
		Request: agentchatRequest("next-command", "second request"), Options: options, Emit: emit,
	})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.OperationID == "" || receipt.OperationID == operation.Receipt().OperationID {
		t.Fatalf("next-turn receipt = %#v, want a separate public Run", receipt)
	}
	close(model.release)
	outcome := operation.Wait(ctx)
	if outcome.Status != agentrun.OutcomeCompleted || outcome.Content != "second answer" {
		t.Fatalf("next-turn chained outcome = %#v", outcome)
	}
	messages := sess.GetMessages()
	if len(messages) != 4 || messages[0].Content != "first request" || messages[1].Content != "first answer" ||
		messages[2].Content != "second request" || messages[3].Content != "second answer" {
		t.Fatalf("next-turn canonical messages = %#v", messages)
	}
	eventsMu.Lock()
	projected := append([]agentrun.Event(nil), events...)
	eventsMu.Unlock()
	if countPublicBackendEvent(projected, "done") != 1 {
		t.Fatalf("next-turn display terminal events = %#v", projected)
	}
	cycles := make([]map[string]any, 0, 2)
	for _, event := range projected {
		if event.Type == "agent_cycle_started" {
			data, _ := event.Data.(map[string]any)
			cycles = append(cycles, data)
		}
	}
	if len(cycles) != 2 || cycles[0]["command_id"] != "next-start" || cycles[0]["delivery"] != "start_turn" ||
		cycles[1]["command_id"] != "next-command" || cycles[1]["delivery"] != "next_turn" ||
		cycles[0]["operation_id"] == cycles[1]["operation_id"] {
		t.Fatalf("next-turn cycle edges = %#v", cycles)
	}
}

func TestAgentRuntimeRestartRetainsTranscriptAndRunsNewInput(t *testing.T) {
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
			AgentKind: agentrun.AgentKindIDE, ProjectID: "project-test", SessionID: sess.ID, Workspace: workspace,
			TaskID: "replay-task", RootAgentName: "root",
		},
	}
	newRuntime := func() *Runtime {
		runtime, runtimeErr := NewAgentRuntime(ctx, dataDir, WithToolMutationApplier(
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
	runID := string(firstOperation.Receipt().OperationID)
	initialTrace, err := agentrun.ReadRunTrace(agentrun.TraceLocation{Workspace: workspace}, runID)
	if err != nil {
		t.Fatalf("read initial public Agent trace: %v", err)
	}
	if initialTrace.Summary.LLMCalls != 1 || initialTrace.Summary.Status != "success" {
		t.Fatalf("initial public Agent trace = %#v", initialTrace.Summary)
	}
	if err := first.Close(ctx); err != nil {
		t.Fatal(err)
	}

	cycle.Request = agentchatRequest("replay-command-2", "hello again")
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
	if modelCalls != 2 {
		t.Fatalf("model calls = %d, want a fresh call after restart", modelCalls)
	}
	if len(replayEvents) == 0 || replayEvents[len(replayEvents)-1].Type != "done" {
		t.Fatalf("replay events = %#v", replayEvents)
	}
	replayedTrace, err := agentrun.ReadRunTrace(agentrun.TraceLocation{Workspace: workspace}, runID)
	if err != nil {
		t.Fatalf("read replayed public Agent trace: %v", err)
	}
	if replayedTrace.Summary.LLMCalls != 1 || replayedTrace.Summary.Status != "success" {
		t.Fatalf("restart changed the completed public Agent trace = %#v", replayedTrace.Summary)
	}
}

func TestAgentRuntimeRestartMarksUnfinishedRunInterrupted(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	liveDataDir := t.TempDir()
	recoveredDataDir := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("public-runtime-cold-recovery")
	if err != nil {
		t.Fatal(err)
	}
	blocking := &publicBackendBlockingModel{started: make(chan struct{})}
	modelIdentity := agent.CapabilityIdentity{Kind: "model.public-backend-cold-recovery", Version: 1}
	initialDefinition := agent.Definition{
		Key: "public-backend-cold-recovery", Name: "root", Model: blocking, ModelIdentity: modelIdentity,
	}
	options := agentrun.Options{
		AgentKind: agentrun.AgentKindIDE, ProjectID: "project-test", SessionID: sess.ID, Workspace: workspace,
		TaskID: "cold-recovery-task", RootAgentName: "root",
	}
	first, err := NewAgentRuntime(ctx, liveDataDir, WithToolMutationApplier(
		func(context.Context, agenttoolruntime.CommittedToolMutation) error { return nil },
	))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = first.Close(context.Background()) })
	operation, err := first.Start(ctx, StartRequest{Cycle: Cycle{
		Definition:   initialDefinition,
		Conversation: agentconversation.NewSessionConversationForAgent(sess, nil, agentrun.AgentKindIDE),
		Request:      agentchatRequest("recovery-start", "uncertain request"), Options: options,
	}})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-blocking.started:
	case <-time.After(2 * time.Second):
		t.Fatal("unfinished model call did not start")
	}
	runID := operation.Receipt().OperationID
	// Snapshot the flushed transcript while the first process is still inside the
	// provider call. Opening the copy accurately models a process crash without
	// asking Agent.Close to perform its intentional graceful abort.
	sourceFS := os.DirFS(liveDataDir)
	if err := fs.WalkDir(sourceFS, ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if strings.HasSuffix(path, ".lease") {
			return nil
		}
		target := filepath.Join(recoveredDataDir, filepath.FromSlash(path))
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, readErr := fs.ReadFile(sourceFS, path)
		if readErr != nil {
			return readErr
		}
		return os.WriteFile(target, data, 0o600)
	}); err != nil {
		t.Fatal(err)
	}

	recoveredModel := &publicBackendTestModel{responses: []*agent.Message{agent.AssistantMessage("recovered answer", nil)}}
	recoveredDefinition := agent.Definition{
		Key: initialDefinition.Key, Name: "root", Model: recoveredModel, ModelIdentity: modelIdentity,
	}
	profile := publicBackendTestProfile{prepare: func(_ context.Context, request CycleRestoreRequest) (Cycle, error) {
		return Cycle{
			Definition:   recoveredDefinition,
			Conversation: agentconversation.NewSessionConversationForAgent(sess, nil, agentrun.AgentKindIDE),
			Request:      request.Request, Options: request.Options,
		}, nil
	}, canonical: publicBackendTestSessionCanonical(sess)}
	second, err := NewAgentRuntime(ctx, recoveredDataDir, WithProfiles(profile), WithToolMutationApplier(
		func(context.Context, agenttoolruntime.CommittedToolMutation) error { return nil },
	))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = second.Close(context.Background()) })
	observation, err := second.OpenRecoveryObservation(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(observation.Close)
	initialStatus := observation.InitialStatus()
	actions := RuntimeRecoveryActions(initialStatus)
	if initialStatus.Phase != agentrun.PhaseIdle || len(actions) != 0 {
		t.Fatalf("restart status=%#v actions=%#v, want idle without recovery actions", initialStatus, actions)
	}
	if initialStatus.LastOperation == nil || initialStatus.LastOperation.OperationID != runID ||
		initialStatus.LastOperation.CommandID != "recovery-start" ||
		initialStatus.LastOperation.Status != agentrun.OperationFailed ||
		initialStatus.LastOperation.Reason != "Agent process stopped before the turn finished" {
		t.Fatalf("restart did not expose the interrupted run: %#v", initialStatus.LastOperation)
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
	runtime, err := NewAgentRuntime(ctx, t.TempDir(), WithToolMutationApplier(
		func(context.Context, agenttoolruntime.CommittedToolMutation) error { return nil },
	))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	options := agentrun.Options{
		AgentKind: agentrun.AgentKindIDE, ProjectID: "project-test", SessionID: sess.ID, Workspace: workspace,
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

func countPublicBackendEvent(events []agentrun.Event, eventType string) int {
	count := 0
	for _, event := range events {
		if event.Type == eventType {
			count++
		}
	}
	return count
}
