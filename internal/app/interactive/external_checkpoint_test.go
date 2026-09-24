package interactiveapp

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"denova/config"
	"denova/internal/agents/attachment"
	agentrun "denova/internal/agents/run"
	agentruntime "denova/internal/agents/runtime"
	"denova/internal/agents/runtime/external"
	"denova/internal/agents/toolresult"
	"denova/internal/interactive"
	agent "github.com/alfredxw/denova/agent"
)

func TestGameImageCheckpointUsesCanonicalMediaAndSurvivesColdReopen(t *testing.T) {
	store, story, cfg := externalGameFixture(t)
	const url = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aKX0AAAAASUVORK5CYII="
	files, err := attachment.Materialize(cfg.ProjectStoreDir, attachment.StoryScope(story), "references", []attachment.Upload{{Name: "map.png", DataURL: url}})
	if err != nil {
		t.Fatal(err)
	}
	identity := interactive.DomainCommitIdentity{CommandID: "input", OperationID: "operation", Cycle: 1}
	large := strings.Repeat("Preserve the map markings. ", 5000)
	intent, err := interactive.NewPlayerInputIntent(identity, "main", large)
	if err != nil {
		t.Fatal(err)
	}
	intent, err = intent.WithAttachments(files)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitPlayerInput(story, intent); err != nil {
		t.Fatal(err)
	}
	conversation := NewConversation(store, t.TempDir(), cfg.Workspace, story, "main", "Continue", 800, &cfg)
	writer, err := conversation.ToolArtifactStore().BeginToolArtifact(t.Context(), agent.ToolArtifactRequest{ToolName: "read", ToolCallID: "map-read", MIMEType: "image/png", Extension: ".png"})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := base64.StdEncoding.DecodeString(strings.SplitN(url, ",", 2)[1])
	if _, err := writer.Write(body); err != nil {
		t.Fatal(err)
	}
	artifact, err := writer.Commit()
	if err != nil {
		t.Fatal(err)
	}
	toolImage := agent.Attachment{ID: artifact.ID, Name: "tool-map.png", Path: artifact.ReadablePath, MediaType: "image/png", Size: artifact.EstimatedBytes, SHA256: artifact.SHA256}
	messages := []interactive.ModelContextMessage{
		{Role: "assistant", ToolCalls: []interactive.ModelContextToolCall{{ID: "map-read", Type: "function", Function: interactive.ModelContextFunctionCall{Name: "read", Arguments: `{}`}}}},
		{Role: "tool", ToolCallID: "map-read", ToolName: "read", Content: large, Attachments: []agent.Attachment{toolImage}},
	}
	batches, err := interactive.NewModelContextBatchIntents(identity, "main", 0, messages)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendModelContextBatch(story, batches[0]); err != nil {
		t.Fatal(err)
	}
	images, calls := 0, 0
	adapter := gameAdapterFunc(func(ctx context.Context, input external.Input, _ external.Host) (external.Result, error) {
		calls++
		if input.Mode != external.OperationSummarize {
			t.Fatal("unexpected primary execution during preparation")
		}
		for _, message := range input.History {
			for _, file := range message.Attachments {
				got, err := agent.AttachmentDataURL(file)
				if err != nil || got != url {
					t.Fatalf("Game summary lost original pixels: %v", err)
				}
				images++
			}
		}
		return external.Result{Text: "The map is a one-pixel reference; preserve its source."}, nil
	})
	for attempt := range 2 {
		state, err := agentruntime.GameState(agentrun.Options{ProjectID: cfg.ProjectID, AgentKind: config.AgentKindInteractiveStory, StoryID: story, BranchID: "main", Mode: "interactive"}, store)
		if err != nil {
			t.Fatal(err)
		}
		conversation = NewConversation(store, t.TempDir(), cfg.Workspace, story, "main", "Continue", 800, &cfg)
		storyContext, err := store.StoryContext(story, "main")
		if err != nil {
			t.Fatal(err)
		}
		history, _, err := conversation.modelHistoryForCycle(storyContext)
		if err != nil {
			t.Fatal(err)
		}
		projection, err := BuildModelContextProjection(history, nil, storyContext.Snapshot, canonicalToolContextPolicy(toolresult.ContextPolicy{}), agentrun.CycleIdentity{})
		if err != nil {
			t.Fatal(err)
		}
		source := externalGameMessages(projection.Messages)
		if len(source) != 2 || len(source[0].Attachments) != 1 || len(source[1].ToolImages) != 1 {
			t.Fatalf("canonical Game projection omitted image sources: %d messages", len(source))
		}
		userPath, toolPath := source[0].Attachments[0].RuntimePath, source[1].ToolImages[0].RuntimePath
		originalText := source[0].Text
		turn := &ExternalTurn{config: ExternalTurnConfig{Conversation: conversation, Config: cfg, PrepareHistory: state.PrepareExternalHistory}}
		prepared, err := turn.prepareRuntimeInput(t.Context(), external.Input{Selection: config.RuntimeSelection{Kind: config.RuntimeClaude}, Text: "Continue", History: source, HistoryBoundary: gameRuntimeBoundary(storyContext.Snapshot)}, adapter)
		if err != nil {
			t.Fatal(err)
		}
		if len(prepared.History) != 1 || !strings.Contains(prepared.History[0].Text, "one-pixel reference") {
			t.Fatalf("Game recovery lost the visual checkpoint: %+v", prepared.History)
		}
		if images != 2 {
			t.Fatalf("Game summary must read both user and tool pixels exactly once across reopen: images=%d calls=%d", images, calls)
		}
		if source[0].Attachments[0].RuntimePath != userPath || source[1].ToolImages[0].RuntimePath != toolPath || source[0].Text != originalText {
			t.Fatal("Game preparation changed canonical media or source text")
		}
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
		if attempt == 0 {
			indexes, err := filepath.Glob(filepath.Join(store.Root(), "interactive", "story", "*.idx.json"))
			if err != nil {
				t.Fatal(err)
			}
			for _, path := range indexes {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			}
			store = interactive.NewStore(cfg.Workspace)
			defer store.Close()
		}
	}
}
