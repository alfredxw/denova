package agent

import (
	"context"
	"errors"
	"io"
	"reflect"
	"sync"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"denova/internal/agentruntime"
)

type recordingToolLifecycleObserver struct {
	mu        sync.Mutex
	events    []string
	beforeErr error
	afterErr  error
}

func (o *recordingToolLifecycleObserver) BeforeTool(_ context.Context, decision ToolDecision, arguments string) error {
	o.record("start:" + decision.ToolCallID + ":" + arguments)
	return o.beforeErr
}

func (o *recordingToolLifecycleObserver) AfterTool(_ context.Context, result ToolExecutionRecord) error {
	o.record("finish:" + result.ToolCallID + ":" + result.Status)
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
	middleware := &toolOrchestratorMiddleware{agentKind: AgentKindIDE}
	endpoint, err := middleware.WrapInvokableToolCall(
		context.Background(),
		func(context.Context, string, ...tool.Option) (string, error) {
			observer.record("effect")
			return "ok", nil
		},
		&adk.ToolContext{Name: "write_file", CallID: "call-1"},
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
	middleware := &toolOrchestratorMiddleware{agentKind: AgentKindIDE}
	called := false
	endpoint, err := middleware.WrapInvokableToolCall(
		context.Background(),
		func(context.Context, string, ...tool.Option) (string, error) {
			called = true
			return "ok", nil
		},
		&adk.ToolContext{Name: "write_file", CallID: "call-1"},
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

func TestCommittedToolMutationRemainsRuntimeOwnedUntilOutputCommit(t *testing.T) {
	options := RunOptions{
		AgentKind: AgentKindIDE, TaskID: "task-1", SessionID: "session-1",
		Workspace: "/workspace/book-a", Mode: "ide",
	}
	binding, err := harnessBindingForOptions(options)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := agentruntime.BindingReference(binding)
	if err != nil {
		t.Fatal(err)
	}
	events := make([]string, 0, 3)
	sink := &harnessEngineSink{emit: func(event agentruntime.EngineEvent) error {
		switch event.(type) {
		case agentruntime.EngineToolFinished:
			events = append(events, "durable-finish")
		case agentruntime.EngineHostEffectAcknowledged:
			events = append(events, "runtime-ack")
		}
		return nil
	}}
	observer := harnessToolLifecycleObserver{
		sink: sink, binding: ref, operationID: "operation-1", cycle: 1, options: options,
	}
	if err := observer.AfterTool(context.Background(), ToolExecutionRecord{
		ToolName: "write_file", ToolCallID: "call-committed", Status: "success",
		Workspace: "/workspace/book-a", Target: "chapters/one.md",
	}); err != nil {
		t.Fatal(err)
	}
	want := []string{"durable-finish"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("pre-output mutation events = %#v, want Runtime-owned durable outbox %#v", events, want)
	}
}

func TestToolExecutionRecordBuildsCompleteMutationReceiptFromRawResult(t *testing.T) {
	workspaceReceipt := `{"schema":"workspace_change.tool_result.v1","status":"applied","workspace":"/workspace/book-a","change_group_id":"group-1","review_thread_id":"review-1","change_set_id":"change-1","path":"chapters/ch01.md","base_revision":"sha256:before","revision":"sha256:after","review_status":"pending","apply_state":"applied"}`
	record := ToolExecutionRecord{ToolName: "write_file", ToolCallID: "write-call", Status: "success"}
	applyToolMutationReceiptToExecutionRecord(&record, workspaceReceipt)
	mutation, ok := toolMutationFromExecutionRecord(record)
	if !ok {
		t.Fatal("workspace mutation was not recognized")
	}
	if mutation.Workspace != "/workspace/book-a" || mutation.Target != "chapters/ch01.md" ||
		mutation.ChangeGroupID != "group-1" || mutation.ReviewThreadID != "review-1" || mutation.ChangeSetID != "change-1" ||
		mutation.BaseRevision != "sha256:before" || mutation.Revision != "sha256:after" ||
		mutation.ReviewStatus != "pending" || mutation.ApplyState != "applied" {
		t.Fatalf("workspace mutation receipt = %#v", mutation)
	}

	loreRecord := ToolExecutionRecord{ToolName: "write_lore_items", ToolCallID: "lore-call", Status: "success"}
	applyToolMutationReceiptToExecutionRecord(&loreRecord, "updated\nitem_ids: [\"hero\",\"hero\",\"world\"]\ndeleted_ids: [\"old\"]")
	loreMutation, ok := toolMutationFromExecutionRecord(loreRecord)
	if !ok {
		t.Fatal("lore mutation was not recognized")
	}
	if !reflect.DeepEqual(loreMutation.LoreItemIDs, []string{"hero", "world"}) || !reflect.DeepEqual(loreMutation.DeletedLoreItemIDs, []string{"old"}) {
		t.Fatalf("lore mutation receipt = %#v", loreMutation)
	}
}

func TestToolLifecycleFinishIsRecordedAfterStreamDrain(t *testing.T) {
	observer := &recordingToolLifecycleObserver{}
	ctx := ContextWithToolLifecycleObserver(context.Background(), observer)
	middleware := &toolOrchestratorMiddleware{agentKind: AgentKindIDE}
	endpoint, err := middleware.WrapStreamableToolCall(
		context.Background(),
		func(context.Context, string, ...tool.Option) (*schema.StreamReader[string], error) {
			observer.record("effect")
			return singleChunkReader("ok"), nil
		},
		&adk.ToolContext{Name: "read_file", CallID: "call-2"},
	)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := endpoint(ctx, `{"path":"chapter.md"}`)
	if err != nil {
		t.Fatal(err)
	}
	if got := observer.snapshot(); len(got) != 2 {
		t.Fatalf("finish must wait for stream drain, got %#v", got)
	}
	if _, err := reader.Recv(); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.Recv(); !errors.Is(err, io.EOF) {
		t.Fatalf("stream error = %v, want EOF", err)
	}
	want := []string{
		`start:call-2:{"path":"chapter.md"}`,
		"effect",
		"finish:call-2:success",
	}
	if got := observer.snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("lifecycle ordering = %#v, want %#v", got, want)
	}
}
