package interactive

import (
	"errors"
	"reflect"
	"testing"

	agent "github.com/alfredxw/denova/agent"

	"denova/internal/contextmaintenance"
)

func TestStoryToolResultCleanupPersistsProjectionWithoutChangingRichTurn(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	story, err := store.CreateStory(CreateStoryRequest{Title: "cleanup", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	summary := &agent.ToolResultSummary{
		Status: agent.ToolResultSuccess, ResultRetention: agent.ToolResultProtected,
		ContextHints: &agent.ToolResultContextHints{
			Recovery: agent.ToolResultRecoveryHint{
				Kind: agent.ToolResultRecoveryRead, Reference: map[string]any{"path": "lore/cast.md", "start_line": float64(4)},
				ArtifactPath: ".denova/artifacts/story/call-read.log", EstimatedTokens: 12_000,
			},
			SupersessionKey: "read:lore/cast.md",
		},
		ArtifactPersistence: &agent.ToolArtifactPersistence{Attempted: true, Complete: true},
		Artifacts: []agent.ToolArtifactRef{{
			ID: "artifact-story", ReadablePath: ".denova/artifacts/story/call-read.log", ContentType: "text/plain",
			EstimatedBytes: 48_000, EstimatedTokens: 12_000, Complete: true,
		}},
	}
	turn, err := store.AppendTurn(story.ID, AppendTurnRequest{
		BranchID: "main", User: "查看人物资料", Narrative: "她认出了来访者。",
		ModelContextMessages: []ModelContextMessage{
			{
				Role: "assistant",
				ToolCalls: []ModelContextToolCall{{
					ID: "call-read", Type: "function", Function: ModelContextFunctionCall{Name: "read", Arguments: `{"path":"lore/cast.md"}`},
				}},
			},
			{Role: "tool", Content: "complete rich cast notes", ToolCallID: "call-read", ToolName: "read", ToolResult: summary},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	expectedParent := turn.ID
	intent := ToolResultCleanupEvent{
		ID: "cleanup-story-1", AgentKind: "interactive_story", SourceStart: 0, SourceEnd: 4,
		Replacements:    []ToolResultReplacement{{MessageIndex: 3, ToolCallID: "call-read", Placeholder: "[Read result available at .denova/artifacts/story/call-read.log]"}},
		ReclaimedTokens: 10_000, TriggeredAtUsage: 280_000, WarmSuffixTokens: 2_000, RendererVersion: "receipt/v1",
		ExpectedParentID: &expectedParent,
	}
	first, err := store.AppendToolResultCleanup(story.ID, "main", intent)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := store.AppendToolResultCleanup(story.ID, "main", intent)
	if err != nil {
		t.Fatalf("exact cleanup retry failed: %v", err)
	}
	if !reflect.DeepEqual(replayed, first) {
		t.Fatalf("cleanup retry differs:\nfirst=%#v\nretry=%#v", first, replayed)
	}
	distinct := intent
	distinct.ID = "cleanup-story-stale"
	if _, err := store.AppendToolResultCleanup(story.ID, "main", distinct); !errors.Is(err, ErrStoryContextRevisionConflict) {
		t.Fatalf("stale cleanup error = %v, want %v", err, ErrStoryContextRevisionConflict)
	}

	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	assertStoryCleanupSnapshot(t, snapshot, first, summary)
	byID, ok, err := store.ToolResultCleanupByID(story.ID, first.ID)
	if err != nil || !ok || !reflect.DeepEqual(byID, first) {
		t.Fatalf("cleanup by id = %#v ok=%t err=%v", byID, ok, err)
	}

	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reloaded := NewStore(dir)
	defer reloaded.Close()
	reloadedSnapshot, err := reloaded.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	assertStoryCleanupSnapshot(t, reloadedSnapshot, first, summary)

	compaction, err := reloaded.AppendContextCompaction(story.ID, "main", ContextCompactionEvent{
		ID: "compaction-after-story-cleanup",
		CompactionCheckpoint: contextmaintenance.CompactionCheckpoint{
			AgentKind: "interactive_story", Summary: "checkpoint",
		},
		SourceTurnCount: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if compaction.ParentID != first.ID {
		t.Fatalf("compaction parent = %q, want cleanup %q", compaction.ParentID, first.ID)
	}
	afterCompaction, err := reloaded.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if afterCompaction.ToolResultCleanup != nil {
		t.Fatalf("compaction retained an older cleanup: %#v", afterCompaction.ToolResultCleanup)
	}
	if got := afterCompaction.Turns[0].ModelContextMessages[1].Content; got != "complete rich cast notes" {
		t.Fatalf("compaction or cleanup rewrote canonical rich result: %q", got)
	}
}

func assertStoryCleanupSnapshot(t *testing.T, snapshot Snapshot, cleanup ToolResultCleanupEvent, summary *agent.ToolResultSummary) {
	t.Helper()
	if snapshot.ToolResultCleanup == nil || !reflect.DeepEqual(*snapshot.ToolResultCleanup, cleanup) {
		t.Fatalf("snapshot cleanup = %#v, want %#v", snapshot.ToolResultCleanup, cleanup)
	}
	if len(snapshot.Turns) != 1 || len(snapshot.Turns[0].ModelContextMessages) != 2 {
		t.Fatalf("unexpected raw story turn projection: %#v", snapshot.Turns)
	}
	toolMessage := snapshot.Turns[0].ModelContextMessages[1]
	if toolMessage.Content != "complete rich cast notes" || !reflect.DeepEqual(toolMessage.ToolResult, summary) {
		t.Fatalf("rich tool result changed: %#v", toolMessage)
	}
}
