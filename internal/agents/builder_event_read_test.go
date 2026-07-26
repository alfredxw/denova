package agents

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	"denova/internal/interactive"
)

func TestInteractiveDirectorAssemblyRegistersOneEventReadAndIgnoresWorkspaceOverride(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "chapter.txt"), []byte("local chapter\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := interactive.NewStore(t.TempDir())
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "assembly", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	var sourceTurn string
	for index := range 4 {
		turn, appendErr := store.AppendTurn(story.ID, interactive.AppendTurnRequest{
			BranchID: "main", User: fmt.Sprintf("action %d", index), Narrative: "result",
		})
		if appendErr != nil {
			t.Fatal(appendErr)
		}
		sourceTurn = turn.ID
	}
	cards, err := store.DirectorEventCardReadScope(story.ID, "main", sourceTurn)
	if err != nil || len(cards) == 0 {
		t.Fatalf("event-card scope = %#v error=%v", cards, err)
	}

	var captured agent.AgentConfig
	previous := newNativeAgent
	newNativeAgent = func(_ context.Context, cfg agent.AgentConfig) (agent.Runnable, error) {
		captured = cfg
		return fakeAgent{name: cfg.Name, description: cfg.Description}, nil
	}
	t.Cleanup(func() { newNativeAgent = previous })

	tests := []struct {
		name          string
		workspaceRead bool
	}{
		{name: "default event only"},
		{name: "forced workspace override", workspaceRead: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			captured = agent.AgentConfig{}
			cfg := &config.Config{
				OpenAIBaseURL: "https://example.invalid", OpenAIModel: "test-model", Workspace: workspace,
			}
			if test.workspaceRead {
				cfg.AgentTools.InteractiveDirector = config.AgentToolOverride{config.AgentToolWorkspaceRead: true}
			}
			_, buildErr := BuildInteractiveDirector(context.Background(), cfg, nil, InteractiveStoryToolContext{
				Store: store, StoryID: story.ID, BranchID: "main", TurnID: sourceTurn,
				MaintenanceTask: "director_plan_update",
			})
			if buildErr != nil {
				t.Fatal(buildErr)
			}

			var read *agent.ToolDefinition
			readCount := 0
			for index := range captured.Tools {
				definition := &captured.Tools[index]
				info, infoErr := definition.Tool.Info(context.Background())
				if infoErr != nil {
					t.Fatal(infoErr)
				}
				if info.Name == "read" {
					read = definition
					readCount++
				}
			}
			if readCount != 1 || read == nil {
				t.Fatalf("Director model-visible read registrations = %d, want exactly 1", readCount)
			}
			if read.Descriptor.Capability != config.AgentToolEventRead {
				t.Fatalf("read descriptor capability = %q, want %q", read.Descriptor.Capability, config.AgentToolEventRead)
			}
			local, localErr := read.Tool.Run(context.Background(), `{"path":"chapter.txt"}`)
			if localErr == nil {
				t.Fatalf("event-only read unexpectedly opened the local workspace: %q", local.ModelContent)
			}
			event, eventErr := read.Tool.Run(context.Background(), `{"path":"event://`+cards[0].ID+`"}`)
			if eventErr != nil || !strings.Contains(event.ModelContent, `"schema": "interactive.event_card.read.v1"`) {
				t.Fatalf("read did not retain event adapter: result=%q error=%v", event.ModelContent, eventErr)
			}
		})
	}
}
