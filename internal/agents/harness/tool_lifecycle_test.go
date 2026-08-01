package harness

import (
	"context"
	"reflect"
	"testing"

	agent "github.com/alfredxw/denova/agent"
	runstate "github.com/alfredxw/denova/agent/runtime"

	"denova/config"
	agentrun "denova/internal/agents/run"
	agenttool "denova/internal/agents/tool"
	producttools "denova/internal/agents/tools"
)

func TestCommittedToolMutationRemainsRuntimeOwnedUntilOutputCommit(t *testing.T) {
	options := agentrun.Options{
		AgentKind: agentrun.AgentKindIDE, TaskID: "task-1", SessionID: "session-1",
		Workspace: "/workspace/book-a", Mode: "ide",
	}
	binding, err := agentrun.BindingForOptions(options)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := runstate.BindingReference(binding)
	if err != nil {
		t.Fatal(err)
	}
	events := make([]string, 0, 3)
	hostEffectCount := 0
	sink := &harnessEngineSink{emit: func(event runstate.EngineEvent) error {
		switch typed := event.(type) {
		case runstate.EngineToolFinished:
			events = append(events, "durable-finish")
			hostEffectCount += len(typed.HostEffects)
		case runstate.EngineHostEffectAcknowledged:
			events = append(events, "runtime-ack")
		}
		return nil
	}}
	observer := harnessToolLifecycleObserver{
		sink: sink, binding: ref, operationID: "operation-1", cycle: 1, options: options,
	}
	if err := observer.AfterTool(context.Background(), agenttool.ExecutionRecord{
		ToolName: "write", ExecutionID: "call-committed", Status: "success",
		Workspace: "/workspace/book-a", Target: "chapters/one.md", ChangeGroupID: "group-1", ChangeSetID: "change-1",
		MutationReceiptSchema: agenttool.MutationReceiptWorkspaceChange,
		Descriptor:            producttools.WorkspaceWriteDescriptor(agent.ToolSourceWrite, config.AgentToolWorkspaceWrite, agent.ToolRecoveryReconcilable),
	}); err != nil {
		t.Fatal(err)
	}
	want := []string{"durable-finish"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("pre-output mutation events = %#v, want Runtime-owned durable outbox %#v", events, want)
	}
	if hostEffectCount != 1 {
		t.Fatalf("committed mutation host effects = %d, want exactly 1", hostEffectCount)
	}
}
