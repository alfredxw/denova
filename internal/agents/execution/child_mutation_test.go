package execution

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"denova/config"
	"denova/internal/agents/canonicalstore"
	agentconversation "denova/internal/agents/conversation"
	agentdelegation "denova/internal/agents/delegation"
	agentprompts "denova/internal/agents/prompts"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	agenttool "denova/internal/agents/tool"
	agenttoolruntime "denova/internal/agents/toolruntime"
	"denova/internal/project"

	agent "github.com/alfredxw/denova/agent"
	agentpermission "github.com/alfredxw/denova/agent/permission"
	"github.com/alfredxw/denova/agent/providers"
	publictools "github.com/alfredxw/denova/agent/tools"
)

func TestDelegatedWorkspaceMutationCompletesAndKeepsItsOwnJournal(t *testing.T) {
	agentrun.SetTraceContentCaptureEnabled(true)
	t.Cleanup(func() { agentrun.SetTraceContentCaptureEnabled(false) })
	ctx := context.Background()
	dataDir, workspace := t.TempDir(), t.TempDir()
	registry := project.NewRegistry(dataDir)
	projectRecord, err := registry.Add(workspace, project.TypeGeneral, "Delegated writing")
	if err != nil {
		t.Fatal(err)
	}
	layout, err := registry.EnsureStore(projectRecord)
	if err != nil {
		t.Fatal(err)
	}
	productStore, err := session.NewStore(layout.SessionsDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = productStore.Close() })
	sess, err := productStore.GetOrCreate("delegated-writing")
	if err != nil {
		t.Fatal(err)
	}
	journalStore, err := canonicalstore.New(dataDir, registry)
	if err != nil {
		t.Fatal(err)
	}
	definitions, err := agenttoolruntime.NewCatalog(&config.Config{Workspace: workspace}).Workspace(
		config.ResolvedAgentToolSettings{config.AgentToolWorkspaceWrite: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	toolset, err := agent.StaticTools(definitions...)
	if err != nil {
		t.Fatal(err)
	}
	childModel := &publicBackendTestModel{responses: []*agent.Message{
		agent.AssistantMessage("", []agent.ToolCall{{ID: "write-chapter", Type: "function", Function: agent.FunctionCall{
			Name: "write", Arguments: `{"path":"chapter.md","content":"Draft chapter.\n"}`,
		}}}),
		agent.AssistantMessage("", []agent.ToolCall{{ID: "revise-chapter", Type: "function", Function: agent.FunctionCall{
			Name: "edit", Arguments: `{"path":"chapter.md","edits":[{"old_string":"Draft","new_string":"Finished"}]}`,
		}}}),
		agent.AssistantMessage("Chapter saved and verified.", nil),
	}}
	child := agent.Definition{
		Key: "test.delegated-writer", Name: "writer", Model: childModel,
		AttachmentRoot: layout.StoreRoot,
		ModelIdentity:  agent.CapabilityIdentity{Kind: "test.child-model", Version: 1},
		Tools:          toolset, Permission: agentpermission.FullAccess(),
		Middlewares: []agent.Middleware{agent.IdentifyMiddleware(
			agenttoolruntime.NewOrchestratorMiddleware(agenttoolruntime.OrchestratorConfig{
				AgentKind: agentrun.AgentKindIDE, Workspace: workspace,
			}), agent.CapabilityIdentity{Kind: "test.child-orchestrator", Version: 1},
		), agent.IdentifyMiddleware(
			agentrun.NewModelInputLoggingMiddleware(agentrun.AgentKindIDE, providers.ModelConfig{Model: "child-test"}, 0, 0, agentprompts.SystemPromptComposition{}),
			agent.CapabilityIdentity{Kind: "test.child-model-trace", Version: 1},
		)},
	}
	catalog, err := agentdelegation.NewCatalog(nil, agentdelegation.Config{
		Capability: "test.delegation", MaxResultBytes: 64 << 10, Parallelism: 1,
		ValidationIdentity: agent.CapabilityIdentity{Kind: "test.delegation-validation", Version: 1},
		Validate:           func(context.Context, []agent.ToolDefinition) error { return nil },
	}, agentdelegation.Child{
		Name: "writer", Description: "Writes the delegated chapter", Definition: child,
		Identity: agent.CapabilityIdentity{Kind: "test.writer", Version: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	parentModel := &publicBackendTestModel{responses: []*agent.Message{
		agent.AssistantMessage("", []agent.ToolCall{{ID: "delegate-chapter", Type: "function", Function: agent.FunctionCall{
			Name: "task", Arguments: `{"action":"start","starts":[{"agent":"writer","idempotency_key":"chapter-once","prompt":"Write and revise the delegated chapter."}]}`,
		}}}),
		agent.AssistantMessage("Delegated chapter complete.", nil),
	}}
	var committed []agenttoolruntime.CommittedToolMutation
	options := []Option{
		WithSessionStore(journalStore),
		WithToolMutationApplier(func(_ context.Context, mutation agenttoolruntime.CommittedToolMutation) error {
			committed = append(committed, mutation)
			return nil
		}),
		WithChildDefinitionResolver(ChildDefinitionResolverFunc(func(_ context.Context, request ChildDefinitionRequest) (ChildDefinition, error) {
			definition, err := agentdelegation.ChildDefinition(agent.Definition{Tools: catalog}, request.Child)
			return ChildDefinition{Definition: definition, Workspace: workspace}, err
		})),
	}
	runtime, err := NewAgentRuntime(ctx, dataDir, options...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	var verified []agenttool.Mutation
	operation, err := runtime.Start(ctx, StartRequest{Cycle: Cycle{
		Definition: agent.Definition{
			Key: "test.parent-writer", Name: "root", Model: parentModel,
			ModelIdentity: agent.CapabilityIdentity{Kind: "test.parent-model", Version: 1},
			Tools:         catalog, Permission: agentpermission.FullAccess(),
		},
		Conversation: agentconversation.NewSessionConversationForAgent(sess, nil, agentrun.AgentKindIDE),
		Request:      agentchatRequest("parent-command", "Delegate one chapter."),
		Options: agentrun.Options{
			AgentKind: agentrun.AgentKindIDE, ProjectID: projectRecord.ID, SessionID: sess.ID,
			Workspace: workspace, StateRoot: layout.StoreRoot, TaskID: "parent-task", RootAgentName: "root",
			OnMutationsVerified: func(_ context.Context, mutations []agenttool.Mutation, _ agenttool.Verification) {
				verified = mutations
			},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if outcome := operation.Wait(ctx); outcome.Status != agentrun.OutcomeCompleted {
		t.Fatalf("parent outcome = %#v", outcome)
	}
	var completion string
	for _, message := range parentModel.inputs[len(parentModel.inputs)-1] {
		if message.TaskCompletion != nil {
			completion = message.Content
		}
	}
	if !strings.Contains(completion, "Status: completed") || !strings.Contains(completion, "Chapter saved and verified.") {
		content, _ := os.ReadFile(filepath.Join(workspace, "chapter.md"))
		t.Fatalf("parent completion = %q; actual file = %q", completion, content)
	}
	content, err := os.ReadFile(filepath.Join(workspace, "chapter.md"))
	if err != nil || string(content) != "Finished chapter.\n" {
		t.Fatalf("chapter = %q, error = %v", content, err)
	}
	if len(committed) != 2 || len(verified) != 2 {
		t.Fatalf("committed = %#v, verified = %#v", committed, verified)
	}
	for _, mutation := range committed {
		if mutation.Binding.ProjectID != projectRecord.ID || mutation.Origin.SessionID != sess.ID ||
			mutation.Origin.TaskID != "parent-task" || string(mutation.RuntimeOperation) == string(operation.Receipt().OperationID) {
			t.Fatalf("delegated mutation lost parent product scope or child Run identity: %#v", mutation)
		}
	}
	parentContext, err := sess.SnapshotContext()
	if err != nil {
		t.Fatal(err)
	}
	for _, message := range parentContext.EffectiveMessages {
		for _, call := range message.ToolCalls {
			if call.Function.Name == "write" || call.Function.Name == "edit" {
				t.Fatal("child tool protocol leaked into the parent canonical journal")
			}
		}
	}
	keys, err := runtime.public.agent.ListSessions(ctx, agent.SessionSelector{All: true})
	if err != nil {
		t.Fatal(err)
	}
	var childKey agent.SessionKey
	for _, key := range keys {
		if strings.HasPrefix(key.Namespace, "task.") {
			if childKey.ID != "" {
				t.Fatal("one delegated chapter created multiple child Sessions")
			}
			childKey = key
		}
	}
	if childKey.ID == "" {
		t.Fatal("delegated Session is missing")
	}
	childRunID := string(committed[0].RuntimeOperation)
	waitForChildTrace(t, runtime, childRunID)
	location := agentrun.TraceLocation{Workspace: workspace, StateRoot: layout.StoreRoot}
	childTrace, err := agentrun.ReadRunTrace(location, childRunID)
	if err != nil {
		t.Fatalf("read delegated Run trace: %v", err)
	}
	if childTrace.Summary.LLMCalls != 3 || childTrace.Summary.ToolCalls != 2 || childTrace.Summary.Status != "success" ||
		childTrace.Summary.SessionID != childKey.ID || childTrace.Summary.ParentRunID != string(operation.Receipt().OperationID) || !childTrace.Summary.ContentCaptured {
		t.Fatalf("delegated Run trace = %#v", childTrace.Summary)
	}
	contentRecords := map[string]int{}
	for _, record := range childTrace.Records {
		contentRecords[record.Type]++
	}
	if contentRecords["llm_input"] != 3 || contentRecords["llm_output"] != 3 || contentRecords["tool_output"] != 2 {
		t.Fatalf("child boundary content = %v", contentRecords)
	}
	parentTrace, err := agentrun.ReadRunTrace(location, string(operation.Receipt().OperationID))
	if err != nil || len(parentTrace.Children) != 1 || parentTrace.Children[0].ID != childRunID || parentTrace.Summary.LLMCalls != 2 || parentTrace.Summary.ToolCalls != 1 {
		t.Fatalf("parent trace mixed or lost child execution: %#v, error = %v", parentTrace, err)
	}
	if err := runtime.Close(ctx); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewAgentRuntime(ctx, dataDir, options...)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close(ctx)
	childSession, err := reopened.public.agent.Session(ctx, childKey)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := childSession.Snapshot(ctx)
	if err != nil || len(snapshot.RecentRuns) != 1 || snapshot.RecentRuns[0].Status != agent.ResultCompleted || snapshot.RecentRuns[0].Output != "Chapter saved and verified." {
		t.Fatalf("reopened child = %#v, error = %v", snapshot, err)
	}
	inspection, err := childSession.Inspect(ctx, agent.Text("Inspect the saved chapter."))
	if err != nil {
		t.Fatal(err)
	}
	toolResults := 0
	for _, message := range inspection.ModelRequest.Messages {
		if message.Role == agent.ToolRole {
			toolResults++
		}
	}
	if toolResults != 2 {
		t.Fatalf("reopened child retained %d tool results, want both write and edit", toolResults)
	}
	executor, err := publictools.NewLocalTasks(publictools.LocalTaskOptions{Parallelism: 1}, publictools.LocalTaskAgent{
		Name: "writer", Opener: reopened.public.agent, Identity: catalog.Children()[0].Identity,
		Attributes: childKey.Attributes,
	})
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := executor.Start(ctx, publictools.TaskRequest{
		Agent: "writer", IdempotencyKey: "chapter-once", Prompt: "Write and revise the delegated chapter.",
	})
	if err != nil || replayed.Ref.Run != snapshot.RecentRuns[0].ID || len(committed) != 2 {
		t.Fatalf("replayed delegated task = %#v, error = %v", replayed, err)
	}
	childSession, err = reopened.public.agent.Session(ctx, childKey)
	if err != nil {
		t.Fatal(err)
	}
	// A cold child must rebuild product effect routing without a live parent
	// registration.
	childModel.responses = []*agent.Message{
		agent.AssistantMessage("", []agent.ToolCall{{ID: "cold-revision", Type: "function", Function: agent.FunctionCall{
			Name: "edit", Arguments: `{"path":"chapter.md","edits":[{"old_string":"Finished","new_string":"Final"}]}`,
		}}}),
		agent.AssistantMessage("Cold revision saved.", nil),
	}
	input := agent.Input{Text: "Revise the existing chapter.", IdempotencyKey: "cold-revision"}
	coldRun, err := childSession.Run(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := coldRun.Wait(ctx); err != nil || result.Status != agent.ResultCompleted {
		t.Fatalf("cold child result = %#v, error = %v", result, err)
	}
	waitForChildTrace(t, reopened, coldRun.ID())
	coldTrace, err := agentrun.ReadRunTrace(location, coldRun.ID())
	if err != nil || coldTrace.Summary.Status != "success" || coldTrace.Summary.LLMCalls != 2 || !coldTrace.Summary.ContentCaptured {
		t.Fatalf("cold child trace = %#v, error = %v", coldTrace.Summary, err)
	}
	if len(committed) != 3 || committed[2].Binding.ProjectID != projectRecord.ID ||
		committed[2].Origin.SessionID != sess.ID || committed[2].Mutation.Workspace != workspace {
		t.Fatalf("cold child mutation routing = %#v", committed)
	}
	content, err = os.ReadFile(filepath.Join(workspace, "chapter.md"))
	if err != nil || string(content) != "Final chapter.\n" {
		t.Fatalf("cold chapter = %q, error = %v", content, err)
	}
}

func waitForChildTrace(t *testing.T, runtime *Runtime, runID string) {
	t.Helper()
	runtime.public.mu.RLock()
	handle := runtime.public.runs[runID]
	runtime.public.mu.RUnlock()
	if handle == nil {
		t.Fatalf("child Run %s has no trace consumer", runID)
	}
	select {
	case <-handle.done:
	case <-time.After(2 * time.Second):
		t.Fatal("child trace did not settle")
	}
}
