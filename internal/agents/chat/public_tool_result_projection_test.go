package chat

import (
	"encoding/json"
	"testing"

	agentrun "denova/internal/agents/run"

	agent "github.com/alfredxw/denova/agent"
)

func TestPublicEventProjectorPreservesDenovaToolResultDisplayAdapters(t *testing.T) {
	var events []agentrun.Event
	projector := NewPublicEventProjector(nil, ChatRequest{}, agentrun.Options{
		AgentKind: "ide", ProjectID: "project", TaskID: "task", RootAgentName: "root",
	}, func(event agentrun.Event) { events = append(events, event) })

	projector.Project(agent.Event{RunID: "run", Payload: agent.ToolFinished{
		CallID: "lore-call", Name: "write_lore_items",
		Projection: &agent.ToolResult{Status: agent.ToolResultSuccess,
			Details: json.RawMessage(`{"item_ids":["character-1"],"deleted_ids":["place-1"]}`)},
	}})
	projector.Project(agent.Event{RunID: "run", Payload: agent.ToolFinished{
		CallID: "image-call", Name: "generate_image",
		Projection: &agent.ToolResult{Status: agent.ToolResultSuccess,
			Details: json.RawMessage(`{"schema":"interactive_image.v1","story_id":"story","branch_id":"main","turn_id":"turn","image_path":"assets/interactive/images/turn.png","meta_path":"assets/interactive/images/turn.json","profile_id":"default","provider":"test","model":"test"}`)},
	}})

	if len(events) != 2 || events[0].Type != "tool_result" || events[1].Type != "tool_result" {
		t.Fatalf("projected ToolResult events = %#v", events)
	}
	lore, _ := events[0].Data.(map[string]any)
	itemIDs, _ := lore["item_ids"].([]string)
	deletedIDs, _ := lore["deleted_ids"].([]string)
	if len(itemIDs) != 1 || itemIDs[0] != "character-1" || len(deletedIDs) != 1 || deletedIDs[0] != "place-1" {
		t.Fatalf("lore display projection = %#v", lore)
	}
	image, _ := events[1].Data.(map[string]any)
	if image["interactive_image"] == nil || image["target"] != "assets/interactive/images/turn.json" {
		t.Fatalf("interactive image display projection = %#v", image)
	}
}

func TestPublicToolResultProjectionRecognizesEveryArtifactLayout(t *testing.T) {
	tests := []struct {
		name   string
		tool   string
		target string
		want   bool
	}{
		{name: "Denova artifact", tool: "read", target: ".denova/artifacts/call.txt", want: true},
		{name: "legacy workspace artifact", tool: "read", target: ".nova/artifacts/call.txt", want: true},
		{name: "journal sidecar", tool: "read", target: "/workspace/session.jsonl.artifacts/call.txt", want: true},
		{name: "Game artifact", tool: "read", target: "/workspace/artifacts/game/call.txt", want: true},
		{name: "Windows separators", tool: "read", target: `C:\\workspace\\session.jsonl.artifacts\\call.txt`, want: true},
		{name: "ordinary source", tool: "read", target: "/workspace/chapter.md", want: false},
		{name: "different tool", tool: "web_fetch", target: "/workspace/session.jsonl.artifacts/call.txt", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isWorkspaceArtifactRead(test.tool, test.target); got != test.want {
				t.Fatalf("isWorkspaceArtifactRead(%q, %q) = %t, want %t", test.tool, test.target, got, test.want)
			}
		})
	}
}
