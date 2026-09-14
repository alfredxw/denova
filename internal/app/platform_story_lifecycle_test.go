package app

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"denova/config"
	"denova/internal/interactive"
	"denova/internal/platform"
	"denova/internal/project"
)

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
	manager.ConfigureStories(platformStoryHost{app: app})
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
	previewRequest.Preview = true
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
