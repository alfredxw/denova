package toolruntime

import (
	"context"
	agentrun "denova/internal/agents/run"
	agenttool "denova/internal/agents/tool"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"denova/config"
	producttools "denova/internal/agents/tools"

	agent "github.com/alfredxw/denova/agent"
)

type recordingToolLifecycleObserver struct {
	mu        sync.Mutex
	events    []string
	beforeErr error
	afterErr  error
}

func (o *recordingToolLifecycleObserver) BeforeTool(_ context.Context, decision agenttool.Decision, arguments string) error {
	o.record("start:" + decision.ExecutionID + ":" + arguments)
	return o.beforeErr
}

func (o *recordingToolLifecycleObserver) AfterTool(_ context.Context, result agenttool.ExecutionRecord) error {
	o.record("finish:" + result.ExecutionID + ":" + result.Status)
	return o.afterErr
}

func (o *recordingToolLifecycleObserver) record(event string) {
	o.mu.Lock()
	o.events = append(o.events, event)
	o.mu.Unlock()
}

func (o *recordingToolLifecycleObserver) snapshot() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]string(nil), o.events...)
}

func TestToolLifecycleStartIsRecordedBeforeInvokableEffect(t *testing.T) {
	observer := &recordingToolLifecycleObserver{}
	ctx := ContextWithToolLifecycleObserver(context.Background(), observer)
	middleware := &OrchestratorMiddleware{agentKind: agentrun.AgentKindIDE}
	endpoint, err := wrapTextToolCallForTest(middleware,
		func(context.Context, string, ...agent.ToolOption) (string, error) {
			observer.record("effect")
			return "ok", nil
		},
		testToolContext("write", "call-1"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := endpoint(ctx, `{"path":"chapter.md"}`); err != nil {
		t.Fatal(err)
	}
	want := []string{
		`start:call-1:{"path":"chapter.md"}`,
		"effect",
		"finish:call-1:success",
	}
	if got := observer.snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("lifecycle ordering = %#v, want %#v", got, want)
	}
}

func TestToolLifecycleStartFailurePreventsEffect(t *testing.T) {
	observer := &recordingToolLifecycleObserver{beforeErr: errors.New("journal unavailable")}
	ctx := ContextWithToolLifecycleObserver(context.Background(), observer)
	middleware := &OrchestratorMiddleware{agentKind: agentrun.AgentKindIDE}
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
	if _, err := endpoint(ctx, `{"path":"chapter.md"}`); err == nil {
		t.Fatal("expected durable start failure")
	}
	if called {
		t.Fatal("tool effect ran without a durable start record")
	}
}

func TestNestedProviderCallIDsReceiveDistinctDurableExecutionIDs(t *testing.T) {
	middleware := &OrchestratorMiddleware{agentKind: agentrun.AgentKindIDE}
	rootCtx := agent.ContextWithToolCall(context.Background(), "call-1", "task")
	rootDecision := middleware.buildToolDecision(rootCtx, testToolContext("write", "call-1"), `{}`)
	childCtx, finishChild, err := agent.BeginChildInvocation(rootCtx, "researcher")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = finishChild() }()
	childDecision := middleware.buildToolDecision(childCtx, testToolContext("write", "call-1"), `{}`)
	if rootDecision.ProviderCallID != "call-1" || childDecision.ProviderCallID != "call-1" {
		t.Fatalf("provider call IDs root=%q child=%q", rootDecision.ProviderCallID, childDecision.ProviderCallID)
	}
	if rootDecision.ExecutionID != "call-1" || childDecision.ExecutionID == "call-1" || childDecision.ExecutionID == rootDecision.ExecutionID {
		t.Fatalf("execution IDs root=%q child=%q", rootDecision.ExecutionID, childDecision.ExecutionID)
	}
	if again := middleware.buildToolDecision(childCtx, testToolContext("write", "call-1"), `{}`).ExecutionID; again != childDecision.ExecutionID {
		t.Fatalf("child execution ID is not deterministic: first=%q again=%q", childDecision.ExecutionID, again)
	}
}

func TestToolMutationResolutionUsesTerminalReceiptInsteadOfStatus(t *testing.T) {
	descriptor := producttools.WorkspaceWriteDescriptor(agent.ToolSourceWrite, config.AgentToolWorkspaceWrite, agent.ToolRecoveryReconcilable)
	receiptedError := agenttool.ExecutionRecord{
		ToolName: "write", ExecutionID: "error-with-receipt", Status: "error", Descriptor: descriptor,
		Workspace: "/workspace/book-a", Target: "chapters/one.md", ChangeGroupID: "group-1", ChangeSetID: "change-1",
		MutationReceiptSchema: agenttool.MutationReceiptWorkspaceChange,
	}
	if resolution := agenttool.ResolveMutation(receiptedError); !resolution.Committed || resolution.Warning != "" {
		t.Fatalf("error with receipt resolution = %#v", resolution)
	}

	missingReceipt := agenttool.ExecutionRecord{ToolName: "write", ExecutionID: "missing", Status: "success", Descriptor: descriptor}
	if resolution := agenttool.ResolveMutation(missingReceipt); resolution.Committed || resolution.Warning == "" {
		t.Fatalf("missing receipt resolution = %#v", resolution)
	}

	blocked := receiptedError
	blocked.ExecutionID = "blocked"
	blocked.Status = "blocked"
	if resolution := agenttool.ResolveMutation(blocked); resolution.Committed || resolution.Warning != "" {
		t.Fatalf("blocked resolution = %#v", resolution)
	}
}

func TestRunObserverProjectsEachTerminalReceiptOnce(t *testing.T) {
	descriptor := producttools.WorkspaceWriteDescriptor(agent.ToolSourceWrite, config.AgentToolWorkspaceWrite, agent.ToolRecoveryReconcilable)
	observer := agentrun.NewObserver(nil, "")
	record := agenttool.ExecutionRecord{
		ToolName: "edit", ExecutionID: "call-1", Status: "error", Descriptor: descriptor,
		Workspace: "/workspace/book-a", Target: "chapters/one.md", ChangeGroupID: "group-1", ChangeSetID: "change-1",
		MutationReceiptSchema: agenttool.MutationReceiptWorkspaceChange,
	}
	observer.RecordToolExecution(record)
	observer.RecordToolExecution(record)
	observer.RecordToolExecution(agenttool.ExecutionRecord{ToolName: "write", ExecutionID: "call-2", Status: "success", Descriptor: descriptor})

	mutations, warnings := observer.ResolvedMutations()
	if len(mutations) != 1 || mutations[0].ToolCallID != "call-1" {
		t.Fatalf("mutations = %#v", mutations)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "call-2") {
		t.Fatalf("warnings = %#v", warnings)
	}
}

func TestToolExecutionRecordBuildsCompleteMutationReceiptFromRawResult(t *testing.T) {
	workspaceReceipt := `{"schema":"workspace_change.tool_result.v1","status":"applied","workspace":"/workspace/book-a","change_group_id":"group-1","review_thread_id":"review-1","change_set_id":"change-1","path":"chapters/ch01.md","base_revision":"sha256:before","revision":"sha256:after","review_status":"pending","apply_state":"applied"}`
	record := agenttool.ExecutionRecord{ToolName: "write", ExecutionID: "write-call", Status: "success", Descriptor: producttools.WorkspaceWriteDescriptor(agent.ToolSourceWrite, config.AgentToolWorkspaceWrite, agent.ToolRecoveryReconcilable)}
	applyToolMutationReceiptToExecutionRecord(&record, agent.ToolResult{Details: []byte(workspaceReceipt)})
	mutation, ok := agenttool.MutationFromExecutionRecord(record)
	if !ok {
		t.Fatal("workspace mutation was not recognized")
	}
	if mutation.Workspace != "/workspace/book-a" || mutation.Target != "chapters/ch01.md" ||
		mutation.ChangeGroupID != "group-1" || mutation.ReviewThreadID != "review-1" || mutation.ChangeSetID != "change-1" ||
		mutation.BaseRevision != "sha256:before" || mutation.Revision != "sha256:after" ||
		mutation.ReviewStatus != "pending" || mutation.ApplyState != "applied" {
		t.Fatalf("workspace mutation receipt = %#v", mutation)
	}

	loreRecord := agenttool.ExecutionRecord{ToolName: "write_lore_items", ExecutionID: "lore-call", Status: "success", Descriptor: producttools.WorkspaceWriteDescriptor(agenttool.ToolSourceLore, config.AgentToolLoreWrite, agent.ToolRecoveryReconcilable)}
	applyToolMutationReceiptToExecutionRecord(&loreRecord, agent.ToolResult{Details: []byte(`{"schema":"lore.write.v1","item_ids":["hero","hero","world"],"deleted_ids":["old"]}`)})
	loreMutation, ok := agenttool.MutationFromExecutionRecord(loreRecord)
	if !ok {
		t.Fatal("lore mutation was not recognized")
	}
	if !reflect.DeepEqual(loreMutation.LoreItemIDs, []string{"hero", "world"}) || !reflect.DeepEqual(loreMutation.DeletedLoreItemIDs, []string{"old"}) {
		t.Fatalf("lore mutation receipt = %#v", loreMutation)
	}
}
