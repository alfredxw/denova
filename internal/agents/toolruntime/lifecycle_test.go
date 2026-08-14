package toolruntime

import (
	"context"
	agentrun "denova/internal/agents/run"
	agenttool "denova/internal/agents/tool"
	"reflect"
	"strings"
	"testing"

	"denova/config"
	producttools "denova/internal/agents/tools"

	agent "github.com/alfredxw/denova/agent"
)

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
