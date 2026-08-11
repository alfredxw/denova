package lifecycle

import (
	"context"
	"errors"
	"strings"
	"testing"

	agentrun "denova/internal/agents/run"
	agenttool "denova/internal/agents/tool"
	agenttoolruntime "denova/internal/agents/toolruntime"

	agent "github.com/alfredxw/denova/agent"
)

func TestToolEffectApplierPreservesPartialSuccessAndTrustedIdentity(t *testing.T) {
	options := agentrun.Options{
		AgentKind: agentrun.AgentKindIDE, Workspace: "/workspace/book", SessionID: "session-1",
		ProjectID: "project-1", TaskID: "trace-1", ReviewThreadID: "review-1",
	}
	var committed []agenttoolruntime.CommittedToolMutation
	applier, err := NewToolEffectApplier(func(_ context.Context, mutation agenttoolruntime.CommittedToolMutation) error {
		committed = append(committed, mutation)
		return nil
	}, options, nil)
	if err != nil {
		t.Fatal(err)
	}
	effect, present, err := agenttoolruntime.AgentToolMutationEffect(agenttool.ExecutionRecord{
		ToolName: "write", ExecutionID: "nested-write-1", Status: "success",
		Workspace: "/workspace/book", Target: "chapters/one.md", ChangeGroupID: "group-1", ChangeSetID: "change-1",
		MutationReceiptSchema: agenttool.MutationReceiptWorkspaceChange,
		Descriptor: agent.ToolDescriptor{
			Source: agent.ToolSourceWrite, MutationScope: agent.ToolMutationWorkspace,
			PostCheck: agent.ToolPostCheckWorkspaceChange,
		},
	})
	if err != nil || !present {
		t.Fatalf("build effect: present=%v err=%v", present, err)
	}
	requests := []agent.EffectRequest{
		{
			ID: "effect-bad", CallID: "outer-call", Effect: agent.Effect{Kind: "unknown", Data: []byte(`{}`)},
			Identity: agent.CommitIdentity{RunID: "run-1", Cycle: 2, Stage: agent.CommitOutput},
		},
		{
			ID: "effect-good", CallID: "outer-call", Effect: effect,
			Identity: agent.CommitIdentity{RunID: "run-1", Cycle: 2, Stage: agent.CommitOutput},
		},
	}

	results, err := applier(context.Background(), requests)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || results[0].ID != "effect-bad" || results[0].Error == "" ||
		results[1] != (agent.EffectResult{ID: "effect-good", Revision: "effect-good"}) {
		t.Fatalf("effect results = %#v", results)
	}
	if len(committed) != 1 {
		t.Fatalf("committed mutations = %#v", committed)
	}
	got := committed[0]
	if got.EffectID != "effect-good" || got.RuntimeOperation != "run-1" || got.RuntimeCycle != 2 ||
		got.ToolCallID != "nested-write-1" || got.Mutation.Workspace != options.Workspace ||
		got.Origin.ProjectID != options.ProjectID || got.Origin.ReviewThreadID != options.ReviewThreadID {
		t.Fatalf("committed mutation = %#v", got)
	}
	if got.Binding.AgentKind != options.AgentKind || got.Binding.Workspace != options.Workspace || got.Binding.SessionID != options.SessionID {
		t.Fatalf("trusted binding = %#v", got.Binding)
	}
}

func TestToolEffectApplierReportsHostFailurePerItem(t *testing.T) {
	applier, err := NewToolEffectApplier(func(context.Context, agenttoolruntime.CommittedToolMutation) error {
		return errors.New("host unavailable")
	}, agentrun.Options{AgentKind: agentrun.AgentKindIDE, Workspace: "/workspace/book", SessionID: "session-1"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	effect, present, err := agenttoolruntime.AgentToolMutationEffect(agenttool.ExecutionRecord{
		ToolName: "write", ExecutionID: "write-1", Status: "success", Workspace: "/workspace/book",
		Target: "chapter.md", ChangeGroupID: "group-1", ChangeSetID: "change-1",
		MutationReceiptSchema: agenttool.MutationReceiptWorkspaceChange,
		Descriptor:            agent.ToolDescriptor{MutationScope: agent.ToolMutationWorkspace},
	})
	if err != nil || !present {
		t.Fatalf("build effect: present=%v err=%v", present, err)
	}
	results, err := applier(context.Background(), []agent.EffectRequest{{ID: "effect-1", Effect: effect}})
	if err != nil || len(results) != 1 || !strings.Contains(results[0].Error, "host unavailable") {
		t.Fatalf("effect results = %#v, err=%v", results, err)
	}
}

func TestToolEffectApplierRejectsInvalidConstruction(t *testing.T) {
	if _, err := NewToolEffectApplier(nil, agentrun.Options{}, nil); err == nil {
		t.Fatal("expected missing reconciler error")
	}
	if _, err := NewToolEffectApplier(
		func(context.Context, agenttoolruntime.CommittedToolMutation) error { return nil },
		agentrun.Options{AgentKind: "unsupported"},
		nil,
	); err == nil {
		t.Fatal("expected invalid binding error")
	}
}
