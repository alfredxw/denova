package app

import (
	"path/filepath"
	"testing"

	"nova/config"
)

func TestAppSettingsReturnsLayered(t *testing.T) {
	ws := t.TempDir()
	novaDir := t.TempDir()

	a := &App{
		cfg:       &config.Config{Workspace: ws, NovaDir: novaDir, OpenAIModel: "x"},
		workspace: ws,
	}
	layered, err := a.Settings()
	if err != nil {
		t.Fatal(err)
	}
	if layered.Effective.OpenAIBaseURL == "" {
		t.Fatalf("default BaseURL should be present")
	}
	if layered.Paths.UserConfig == "" || layered.Paths.WorkspaceConfig == "" || layered.Paths.NovaDir == "" {
		t.Fatalf("settings paths should be exposed: %+v", layered.Paths)
	}
}

func TestAppUpdateUserSettingsPersists(t *testing.T) {
	ws := t.TempDir()
	novaDir := t.TempDir()

	a := &App{
		cfg:       &config.Config{Workspace: ws, NovaDir: novaDir},
		workspace: ws,
	}
	in := config.Settings{OpenAIModel: "user-model"}
	if _, err := a.UpdateUserSettings(in); err != nil {
		t.Fatal(err)
	}
	out, err := config.ReadSettingsFile(filepath.Join(novaDir, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if out.OpenAIModel != "user-model" {
		t.Fatalf("user model not persisted: %s", out.OpenAIModel)
	}
}

func TestAppUpdateWorkspaceSettingsPersists(t *testing.T) {
	ws := t.TempDir()
	novaDir := t.TempDir()

	a := &App{
		cfg:       &config.Config{Workspace: ws, NovaDir: novaDir},
		workspace: ws,
	}
	targetChars := 900
	maxTokens := 0
	in := config.Settings{OpenAIModel: "ws-model", InteractiveReplyTargetChars: &targetChars, InteractiveMaxTokens: &maxTokens}
	if _, err := a.UpdateWorkspaceSettings(in); err != nil {
		t.Fatal(err)
	}
	out, err := config.ReadSettingsFile(filepath.Join(ws, ".nova", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if out.OpenAIModel != "ws-model" {
		t.Fatalf("workspace model not persisted: %s", out.OpenAIModel)
	}
	if out.InteractiveReplyTargetChars == nil || *out.InteractiveReplyTargetChars != 900 {
		t.Fatalf("interactive reply target not persisted: %v", out.InteractiveReplyTargetChars)
	}
	if out.InteractiveMaxTokens == nil || *out.InteractiveMaxTokens != 0 {
		t.Fatalf("interactive max tokens not persisted: %v", out.InteractiveMaxTokens)
	}
}
