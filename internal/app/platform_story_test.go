package app

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"denova/config"
	apptask "denova/internal/app/task"
	"denova/internal/interactive"
	"denova/internal/platform"
	"denova/internal/project"
)

func TestPlatformStoryBindingUsesCanonicalJournalAndRetainsFallback(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	workspace := filepath.Join(root, "projects", "story")
	if err := os.MkdirAll(workspace, 0700); err != nil {
		t.Fatal(err)
	}
	registry := project.NewRegistry(root)
	record, err := registry.Add(workspace, project.TypeBook, "Story")
	if err != nil {
		t.Fatal(err)
	}
	layout, err := registry.EnsureStore(record)
	if err != nil {
		t.Fatal(err)
	}
	store := interactive.NewStore(workspace)
	defer store.Close()
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "Existing story", Origin: "Premise"})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := store.AppendTurn(story.ID, interactive.AppendTurnRequest{BranchID: "main", Narrative: "Visible text", Thinking: "Private thinking", ModelContextMessages: []interactive.ModelContextMessage{{Role: "system", Content: "Private instructions"}}})
	if err != nil {
		t.Fatal(err)
	}
	app := &App{projectRegistry: registry, workspace: workspace, cfg: &config.Config{ProjectID: record.ID, NovaDir: root}, interactive: store}
	host := platformStoryHost{app: app}
	instance, err := host.Bind(ctx, platform.Instance{GameID: "test.renderer", ReleaseID: "release-1", StoryID: story.ID, ProjectID: record.ID, Title: "Journey", Setup: map[string]any{}, Models: map[string]string{}, CreatedAt: time.Now().UTC()}, platform.StoryBindingOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if instance.StoryID != story.ID {
		t.Fatal("adoption created another story")
	}
	items, err := host.Instances(ctx)
	if err != nil || len(items) != 1 || items[0].ID != instance.ID {
		t.Fatalf("list = %+v %v", items, err)
	}
	scope := platform.Scope{ProjectID: record.ID, StoryID: story.ID, InstanceID: instance.ID}
	snapshot, err := host.Snapshot(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(snapshot)
	if len(snapshot.Turns) != 1 || snapshot.Turns[0].ID != turn.ID || strings.Contains(string(encoded), "Private") {
		t.Fatalf("unsafe player snapshot: %s", encoded)
	}
	if _, err := host.Bind(ctx, instance, platform.StoryBindingOptions{}); err == nil {
		t.Fatal("duplicate binding accepted")
	}
	assets := filepath.Join(layout.StoreRoot, "extensions", instance.GameID, story.ID, "data-resources")
	if err := os.MkdirAll(assets, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assets, "image.png"), []byte("asset"), 0600); err != nil {
		t.Fatal(err)
	}
	var exported bytes.Buffer
	if err := host.Export(ctx, instance, &exported); err != nil {
		t.Fatal(err)
	}
	archive, err := zip.NewReader(bytes.NewReader(exported.Bytes()), int64(exported.Len()))
	if err != nil {
		t.Fatal(err)
	}
	entries := map[string]string{}
	for _, file := range archive.File {
		reader, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(reader)
		reader.Close()
		if err != nil {
			t.Fatal(err)
		}
		entries[file.Name] = string(data)
	}
	if entries["assets/data-resources/image.png"] != "asset" || !strings.Contains(entries["story.jsonl"], "extension_record") {
		t.Fatalf("incomplete export: %v", entries)
	}
	if err := host.RemoveBinding(ctx, instance); err != nil {
		t.Fatal(err)
	}
	items, err = host.Instances(ctx)
	if err != nil || len(items) != 0 {
		t.Fatalf("removed binding still listed: %+v %v", items, err)
	}
	if _, err := store.Snapshot(story.ID, "main"); err != nil {
		t.Fatal("removal destroyed fallback story", err)
	}
	restored, err := host.Bind(ctx, instance, platform.StoryBindingOptions{})
	if err != nil || restored.ID != instance.ID {
		t.Fatalf("reattach = %+v %v", restored, err)
	}
	// Another Project cannot be implicitly opened by the extension.
	scope.ProjectID = "different-project"
	if _, err := host.Command(ctx, scope, platform.StoryCommand{Kind: platform.StoryAdvance, CommandID: "one", Message: "Continue"}); err == nil {
		t.Fatal("cross-Project run was accepted")
	}
}

func TestPlatformStoryCommandsReplayResumeAndRegenerateWithoutRepeatingInput(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	workspace := filepath.Join(root, "projects", "commands")
	if err := os.MkdirAll(workspace, 0700); err != nil {
		t.Fatal(err)
	}
	registry := project.NewRegistry(root)
	projectRecord, err := registry.Add(workspace, project.TypeBook, "Commands")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.EnsureStore(projectRecord); err != nil {
		t.Fatal(err)
	}
	store := interactive.NewStore(workspace)
	defer store.Close()
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "Commands"})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := store.AppendTurn(story.ID, interactive.AppendTurnRequest{BranchID: "main", User: "Open the door", Narrative: "The door opens"})
	if err != nil {
		t.Fatal(err)
	}
	app := &App{projectRegistry: registry, workspace: workspace, cfg: &config.Config{ProjectID: projectRecord.ID, NovaDir: root}, interactive: store}
	host := platformStoryHost{app: app}
	scope := platform.Scope{ProjectID: projectRecord.ID, StoryID: story.ID, InstanceID: "instance-test"}
	service := app.interactiveService()
	regenerate := InteractiveAgentStartRequest{CommandID: "platform-instance-test-regenerate", StoryID: story.ID, BranchID: "main", Message: "Open the door", RegenerateFromTurnID: turn.ID}
	identity, err := service.resolveInteractiveStart(regenerate)
	if err != nil {
		t.Fatal(err)
	}
	task, err := apptask.NewDeferred(nil)
	if err != nil {
		t.Fatal(err)
	}
	task.RejectStart(context.Canceled)
	if err := service.starts.remember(identity, task); err != nil {
		t.Fatal(err)
	}
	if _, err := host.Command(ctx, scope, platform.StoryCommand{Kind: platform.StoryRegenerate, CommandID: "regenerate", TurnID: turn.ID}); err != nil {
		t.Fatal("regenerate lost original player input", err)
	}
	intent, err := interactive.NewPlayerInputIntent(interactive.DomainCommitIdentity{CommandID: "paused", OperationID: "paused-operation", Cycle: 1}, "main", "Enter the room")
	if err != nil {
		t.Fatal(err)
	}
	input, err := store.CommitPlayerInput(story.ID, intent)
	if err != nil {
		t.Fatal(err)
	}
	interruption, err := store.MarkTurnInterrupted(story.ID, "main", input.Event.ID, "Enter the room", "The room is", "cancelled")
	if err != nil {
		t.Fatal(err)
	}
	resume := InteractiveAgentStartRequest{CommandID: "platform-instance-test-resume", StoryID: story.ID, BranchID: "main", Message: "Continue.", ResumeInterruptionID: interruption.ID}
	identity, err = service.resolveInteractiveStart(resume)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.starts.remember(identity, task); err != nil {
		t.Fatal(err)
	}
	if _, err := host.Command(ctx, scope, platform.StoryCommand{Kind: platform.StoryResume, CommandID: "resume", InterruptionID: interruption.ID}); err != nil {
		t.Fatal("resume requires callers to repeat the original input", err)
	}
	for _, kind := range []platform.StoryCommandKind{platform.StoryResume, platform.StoryRegenerate} {
		_, err := host.Command(ctx, scope, platform.StoryCommand{Kind: kind, CommandID: "bad", Message: "Do not treat this as advance"})
		platformError, ok := err.(*platform.Error)
		if !ok || platformError.Code != "INVALID_ARGUMENT" {
			t.Fatalf("missing %s identifier error = %v", kind, err)
		}
	}
}
