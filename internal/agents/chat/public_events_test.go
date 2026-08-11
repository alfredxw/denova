package chat

import (
	"encoding/json"
	"testing"

	agentrun "denova/internal/agents/run"

	agent "github.com/alfredxw/denova/agent"
)

type publicEventCompactionConversation struct {
	Conversation
	state *agent.CompactionState
}

func (conversation *publicEventCompactionConversation) BindAgentCompaction(state *agent.CompactionState) error {
	conversation.state = state
	return nil
}

func TestPublicEventProjectorPreservesUsageAndStructuredToolDisplay(t *testing.T) {
	var events []agentrun.Event
	projector := NewPublicEventProjector(nil, ChatRequest{}, agentrun.Options{
		AgentKind: "ide", ProjectID: "project", TaskID: "task", RootAgentName: "root",
	}, func(event agentrun.Event) {
		events = append(events, event)
	})
	projector.Project(agent.Event{RunID: "run", Payload: agent.AssistantDelta{Delta: "draft"}})
	projector.Project(agent.Event{RunID: "run", Payload: agent.ModelCompleted{
		Usage: agent.TokenUsage{
			PromptTokens: 100, PromptTokenDetails: agent.PromptTokenDetails{CachedTokens: 75},
			CompletionTokens: 20, TotalTokens: 120,
		},
		FinishReason: "tool_calls", RequestedTools: []string{"write"},
	}})
	receipt := json.RawMessage(`{"schema":"workspace_change.tool_result.v1","status":"applied","workspace":"/workspace","change_group_id":"group","change_set_id":"change","path":"chapters/one.md","revision":"sha256:after"}`)
	projector.Project(agent.Event{RunID: "run", Payload: agent.ToolFinished{
		CallID: "call", Name: "write", Result: "written",
		Projection: &agent.ToolResult{
			DisplayContent: "written", ModelContent: "written", Details: receipt,
			Status: agent.ToolResultSuccess,
			Metadata: agent.ToolResultMetadata{
				OriginalModelBytes: 20, ReturnedModelBytes: 7, Target: "chapters/one.md",
			},
			ResultRetention: agent.ToolResultProtected,
		},
	}})
	projector.Project(agent.Event{RunID: "run", Payload: agent.ModelCompleted{
		Usage: agent.TokenUsage{
			PromptTokens: 80, PromptTokenDetails: agent.PromptTokenDetails{CachedTokens: 40},
			CompletionTokens: 10, TotalTokens: 90,
		},
		FinishReason: "stop",
	}})
	projector.Project(agent.Event{RunID: "run", Payload: agent.RunSettled{Status: agent.ResultCompleted}})
	projector.Finalize(agent.ResultCompleted, "")

	indexes := map[string]int{}
	for index, event := range events {
		indexes[event.Type] = index
	}
	if indexes["workspace_change"] == 0 || indexes["tool_result"] == 0 || indexes["token_usage"] == 0 || indexes["done"] == 0 {
		t.Fatalf("projected event types = %#v", events)
	}
	if indexes["workspace_change"] > indexes["tool_result"] || indexes["token_usage"] > indexes["done"] {
		t.Fatalf("event ordering = %#v", events)
	}
	usage := events[indexes["token_usage"]]
	if usage.DataString("run_id") != "run" || eventDataInt(usage.Data, "model_calls") != 2 ||
		eventDataInt(usage.Data, "prompt_tokens") != 180 || eventDataInt(usage.Data, "cached_prompt_tokens") != 115 {
		t.Fatalf("token usage = %#v", usage.Data)
	}
	result := events[indexes["tool_result"]]
	if result.DataString("target") != "chapters/one.md" || result.DataString("status") != "success" {
		t.Fatalf("tool result = %#v", result.Data)
	}
}

func TestPublicEventProjectorPublishesAndBindsAgentCompaction(t *testing.T) {
	conversation := &publicEventCompactionConversation{}
	var events []agentrun.Event
	projector := NewPublicEventProjector(conversation, ChatRequest{}, agentrun.Options{
		AgentKind: "interactive_story", TaskID: "task", RootAgentName: "root",
	}, func(event agentrun.Event) { events = append(events, event) })
	state := agent.CompactionState{
		ID: "checkpoint-1", Revision: 3, Summary: "bounded story state", TokenEstimate: 42,
		ReplacementFrom: 1, ReplacementTo: 5,
	}
	projector.Project(agent.Event{RunID: "run", Payload: agent.CompactionCommitted{State: state}})
	if conversation.state == nil || conversation.state.ID != state.ID || conversation.state.Revision != state.Revision {
		t.Fatalf("bound Compaction=%#v", conversation.state)
	}
	if len(events) != 1 || events[0].Type != "context_compaction" ||
		events[0].DataString("status") != "completed" || events[0].DataString("summary") != state.Summary ||
		eventDataInt(events[0].Data, "source_message_count") != 4 {
		t.Fatalf("projected Compaction events=%#v", events)
	}
	projector.Project(agent.Event{RunID: "run", Payload: agent.CompactionRemoved{ID: state.ID, Revision: 4}})
	if conversation.state != nil {
		t.Fatalf("removed Compaction remained bound: %#v", conversation.state)
	}
}
