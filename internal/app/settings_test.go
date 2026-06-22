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
	if layered.Access.LocalURL == "" || layered.Access.LANURL == "" {
		t.Fatalf("settings access URLs should be exposed: %+v", layered.Access)
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

func TestAppUpdateUserSettingsPreservesRemoteAccessPasswordHash(t *testing.T) {
	ws := t.TempDir()
	novaDir := t.TempDir()
	hash, err := config.HashRemoteAccessPassword("secret")
	if err != nil {
		t.Fatal(err)
	}
	if err := config.WriteSettingsFile(filepath.Join(novaDir, "config.toml"), config.Settings{RemoteAccessPasswordHash: hash}); err != nil {
		t.Fatal(err)
	}

	a := &App{
		cfg:       &config.Config{Workspace: ws, NovaDir: novaDir},
		workspace: ws,
	}
	enabled := true
	if _, err := a.UpdateUserSettings(config.Settings{
		AllowLANAccess:       &enabled,
		RemoteAccessUsername: "reader",
	}); err != nil {
		t.Fatal(err)
	}
	out, err := config.ReadSettingsFile(filepath.Join(novaDir, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if out.RemoteAccessPasswordHash != hash {
		t.Fatalf("password hash should be preserved")
	}
	if !config.CheckRemoteAccessPassword(out.RemoteAccessPasswordHash, "secret") {
		t.Fatalf("preserved password hash should verify")
	}
}

func TestAppUpdateWorkspaceSettingsPersists(t *testing.T) {
	ws := t.TempDir()
	novaDir := t.TempDir()

	a := &App{
		cfg:       &config.Config{Workspace: ws, NovaDir: novaDir},
		workspace: ws,
	}
	maxTokens := 0
	in := config.Settings{OpenAIModel: "ws-model", InteractiveMaxTokens: &maxTokens}
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
	if out.InteractiveMaxTokens == nil || *out.InteractiveMaxTokens != 0 {
		t.Fatalf("interactive max tokens not persisted: %v", out.InteractiveMaxTokens)
	}
}

func TestApplyLayeredSettingsToConfigAppliesContextWindow(t *testing.T) {
	contextWindow := 650000
	cfg := &config.Config{}
	applyLayeredSettingsToConfig(cfg, config.LayeredSettings{
		Effective: config.Settings{
			OpenAIContextWindowTokens: &contextWindow,
		},
	})
	if cfg.OpenAIContextWindowTokens != contextWindow {
		t.Fatalf("context window tokens = %d, want %d", cfg.OpenAIContextWindowTokens, contextWindow)
	}
}
