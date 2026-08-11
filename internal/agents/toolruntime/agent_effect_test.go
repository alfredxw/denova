package toolruntime

import (
	"encoding/json"
	"testing"

	agenttool "denova/internal/agents/tool"

	agent "github.com/alfredxw/denova/agent"
)

func TestAgentToolMutationEffectRoundTripsCommittedReceipt(t *testing.T) {
	record := agenttool.ExecutionRecord{
		ToolName: "write", ExecutionID: "nested-write-1", Status: "success",
		Workspace: "/workspace/book", Target: "chapters/one.md",
		ChangeGroupID: "group-1", ChangeSetID: "change-1",
		MutationReceiptSchema: agenttool.MutationReceiptWorkspaceChange,
		Descriptor: agent.ToolDescriptor{
			Source: agent.ToolSourceWrite, MutationScope: agent.ToolMutationWorkspace,
			PostCheck: agent.ToolPostCheckWorkspaceChange,
		},
	}

	effect, present, err := AgentToolMutationEffect(record)
	if err != nil {
		t.Fatal(err)
	}
	if !present {
		t.Fatal("committed mutation did not produce an Agent effect")
	}
	mutation, err := DecodeAgentToolMutationEffect(effect)
	if err != nil {
		t.Fatal(err)
	}
	want, ok := agenttool.MutationFromExecutionRecord(record)
	gotJSON, gotErr := json.Marshal(mutation)
	wantJSON, wantErr := json.Marshal(want)
	if !ok || gotErr != nil || wantErr != nil || string(gotJSON) != string(wantJSON) {
		t.Fatalf("round-trip mutation = %#v, want %#v", mutation, want)
	}
}

func TestAgentToolMutationEffectIgnoresUncommittedResult(t *testing.T) {
	effect, present, err := AgentToolMutationEffect(agenttool.ExecutionRecord{
		ToolName: "write", ExecutionID: "write-1", Status: "success",
		Descriptor: agent.ToolDescriptor{MutationScope: agent.ToolMutationWorkspace},
	})
	if err != nil || present || effect.Kind != "" {
		t.Fatalf("uncommitted result effect = %#v, present=%v, err=%v", effect, present, err)
	}
}
