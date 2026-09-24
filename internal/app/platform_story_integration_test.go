package app

import (
	"archive/zip"
	"bytes"
	"context"
	"denova/config"
	apptask "denova/internal/app/task"
	"denova/internal/interactive"
	"denova/internal/platform"
	"denova/internal/project"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
	host := newPlatformTestStoryHost(app)
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
	host := newPlatformTestStoryHost(app)
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

func TestPlatformManagedStoryUpgradeExportAndReattachPreserveResources(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	workspace := filepath.Join(root, "projects", "managed")
	if err := os.MkdirAll(workspace, 0700); err != nil {
		t.Fatal(err)
	}
	registry := project.NewRegistry(root)
	projectRecord, err := registry.Add(workspace, project.TypeBook, "Managed")
	if err != nil {
		t.Fatal(err)
	}
	layout, err := registry.EnsureStore(projectRecord)
	if err != nil {
		t.Fatal(err)
	}
	store := interactive.NewStore(workspace)
	defer store.Close()
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "Existing Story"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendTurn(story.ID, interactive.AppendTurnRequest{BranchID: "main", Narrative: "Original narrative"}); err != nil {
		t.Fatal(err)
	}
	app := &App{projectRegistry: registry, workspace: workspace, cfg: &config.Config{ProjectID: projectRecord.ID, NovaDir: root}, interactive: store}
	manager := platform.New(root, registry)
	manager.ConfigureStories(newPlatformTestStoryHost(app))
	defer manager.Close(ctx)
	source := t.TempDir()
	for name, content := range map[string]string{"index.html": "<!doctype html><p>Test view</p>", "en.json": `{"writer":"Writer"}`, "zh.json": `{"writer":"作者"}`} {
		if err := os.WriteFile(filepath.Join(source, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	install := func(version string) (platform.Candidate, platform.Release) {
		t.Helper()
		raw := fmt.Sprintf(`{"manifestVersion":1,"id":"test.managed","version":%q,"apiMajor":1,"name":{"en-US":"Managed","zh-CN":"托管"},"locales":{"en-US":"en.json","zh-CN":"zh.json"},"permissions":{"required":["stories.read","stories.write"],"optional":[]},"modelSlots":[{"id":"writer","titleKey":"writer","kind":"text","required":false}],"views":[{"id":"stage","source":{"kind":"static","path":"index.html"}}],"game":{"viewId":"stage","storage":{"kind":"story","saveFormat":"test-v1"},"story":{"modelSlot":"writer"}}}`, version)
		if err := os.WriteFile(filepath.Join(source, "denova.game.json"), []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		candidate, err := manager.PreviewDirectory(platform.Game, source)
		if err != nil {
			t.Fatal(err)
		}
		release, err := manager.Install(candidate.ID, candidate.Manifest.Permissions.Required)
		if err != nil {
			t.Fatal(err)
		}
		return candidate, release
	}
	_, old := install("1.0.0")
	request := platform.CreateInstance{GameID: old.Manifest.ID, ReleaseID: old.Ref.ReleaseID, Title: "Renderer", ProjectID: projectRecord.ID, StoryID: story.ID, Setup: map[string]any{}, Models: map[string]string{}}
	instance, err := manager.CreateInstance(request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "games", instance.GameID, "instances", instance.ID, "instance.json")); !os.IsNotExist(err) {
		t.Fatal("managed binding has a second instance.json authority", err)
	}
	opened, err := manager.OpenInstance(ctx, instance.ID, platform.OpenOptions{ParentOrigin: "http://127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	if opened.Context.Scope.StoryID != story.ID {
		t.Fatal("runtime lost Story scope")
	}
	resources := filepath.Join(layout.StoreRoot, "extensions", instance.GameID, story.ID, "data-resources")
	if err := os.MkdirAll(resources, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(resources, "cg.png"), []byte("generated-image"), 0600); err != nil {
		t.Fatal(err)
	}
	candidate, next := install("1.1.0")
	upgraded, err := manager.UpgradeInstance(ctx, instance.ID, next.Ref.ReleaseID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if upgraded.ID != instance.ID || upgraded.StoryID != story.ID || upgraded.ReleaseID != next.Ref.ReleaseID {
		t.Fatalf("upgrade changed identity: %+v", upgraded)
	}
	preview, err := manager.PreparePreview(candidate.ID, candidate.Manifest.Permissions.Required)
	if err != nil {
		t.Fatal(err)
	}
	previewRequest := request
	previewRequest.ReleaseID = preview.Ref.ReleaseID
	if _, err := manager.CreateInstance(previewRequest); err == nil {
		t.Fatal("preview adopted a real Story")
	}
	var exported bytes.Buffer
	if err := manager.ExportInstance(ctx, instance.ID, &exported); err != nil {
		t.Fatal(err)
	}
	archive, err := zip.NewReader(bytes.NewReader(exported.Bytes()), int64(exported.Len()))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, file := range archive.File {
		if file.Name != "assets/data-resources/cg.png" {
			continue
		}
		reader, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(reader)
		reader.Close()
		if err != nil {
			t.Fatal(err)
		}
		found = string(data) == "generated-image"
	}
	if !found {
		t.Fatal("export omitted generated resources")
	}
	backup, err := manager.RemoveInstance(ctx, instance.ID)
	if err != nil || backup == "" {
		t.Fatalf("remove backup = %q %v", backup, err)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(backup))); err != nil {
		t.Fatal("backup missing", err)
	}
	request.ReleaseID = next.Ref.ReleaseID
	restored, err := manager.CreateInstance(request)
	if err != nil {
		t.Fatal(err)
	}
	if restored.ID != instance.ID {
		t.Fatal("reattach changed identity")
	}
	data, err := os.ReadFile(filepath.Join(resources, "cg.png"))
	if err != nil || string(data) != "generated-image" {
		t.Fatal("reattach lost resources", err)
	}
	index, err := app.InteractiveStories()
	if err != nil {
		t.Fatal(err)
	}
	if index.CurrentStoryID != story.ID || len(index.Stories) != 1 {
		encoded, _ := json.Marshal(index)
		t.Fatalf("extension lifecycle changed builtin selection: %s", encoded)
	}
}

func TestPlatformStorySceneProjectsNativeVersionWithoutPrivateContext(t *testing.T) {
	host, scope, store := newPlatformStateTestHost(t)
	_, err := host.Configure(context.Background(), scope, platform.StoryConfiguration{ModuleRefs: &platform.StoryModuleRefs{ActorStateID: interactive.ActorStateXiuxianID}, InitialActors: []platform.StoryInitialActor{{ID: "沈清霜", Name: "沈清霜", TemplateID: "important_character"}}})
	if err != nil {
		t.Fatal(err)
	}
	turn, _, err := store.AppendTurnWithState(scope.StoryID, interactive.AppendTurnWithStateRequest{BranchID: "main", Narrative: "First promise", Thinking: "PRIVATE_THINKING", ModelContextMessages: []interactive.ModelContextMessage{{Role: "system", Content: "PRIVATE_CONTEXT"}}, ActorOps: []interactive.ActorStateOp{{Op: "set", ActorID: "沈清霜", FieldID: "对主角好感度", Value: 12, Reason: "Promise kept"}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RewindToTurnParent(scope.StoryID, interactive.RewindTurnRequest{BranchID: "main", TurnID: turn.ID}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(scope.StoryID, interactive.AppendTurnWithStateRequest{BranchID: "main", Narrative: "Replacement", ActorOps: []interactive.ActorStateOp{{Op: "set", ActorID: "沈清霜", FieldID: "对主角好感度", Value: 55}}}); err != nil {
		t.Fatal(err)
	}
	scope.BranchID = "main"
	scene, err := host.Scene(context.Background(), scope, turn.ID, turn.ID)
	if err != nil {
		t.Fatal(err)
	}
	if scene.Turn.ID != turn.ID || scene.PreviousTurn != nil || scene.State.SourceRevision != scene.Turn.Revision || scene.State.StateSchema == nil {
		t.Fatalf("invalid scene projection: %+v", scene)
	}
	value := scene.State.State["actors"].(map[string]any)["沈清霜"].(map[string]any)["state"].(map[string]any)["对主角好感度"]
	if value != float64(12) || len(scene.Turn.StateChanges) != 1 || scene.Turn.StateChanges[0].Reason != "Promise kept" {
		t.Fatalf("scene used current or untraceable state: %+v", scene)
	}
	encoded, err := json.Marshal(scene)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "PRIVATE_") {
		t.Fatalf("private data escaped scene DTO: %s", encoded)
	}
	for _, test := range []struct{ turn, revision, code string }{{"missing", "missing", "NOT_FOUND"}, {turn.ID, "wrong", "DOCUMENT_CONFLICT"}} {
		_, err := host.Scene(context.Background(), scope, test.turn, test.revision)
		var problem *platform.Error
		if !errors.As(err, &problem) || problem.Code != test.code {
			t.Fatalf("scene error = %v, want %s", err, test.code)
		}
	}
}
