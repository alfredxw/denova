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
		cfg:       &config.Config{Workspace: ws, NovaDir: novaDir, OpenAIModel: "x", RuntimeWebPort: 19091},
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
	if layered.Access.LocalURL != "http://localhost:19091" {
		t.Fatalf("settings access URL should use runtime web port: %+v", layered.Access)
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
	hotChoices := false
	in := config.Settings{OpenAIModel: "ws-model", InteractiveHotChoices: &hotChoices}
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
	if out.InteractiveHotChoices == nil || *out.InteractiveHotChoices {
		t.Fatalf("interactive hot choices not persisted: %v", out.InteractiveHotChoices)
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

func TestApplyLayeredSettingsToConfigAppliesAgentIdleTimeout(t *testing.T) {
	idleTimeout := 240
	cfg := &config.Config{}
	applyLayeredSettingsToConfig(cfg, config.LayeredSettings{
		Effective: config.Settings{
			AgentIdleTimeoutSeconds: &idleTimeout,
		},
	})
	if cfg.AgentIdleTimeoutSeconds != idleTimeout {
		t.Fatalf("agent idle timeout = %d, want %d", cfg.AgentIdleTimeoutSeconds, idleTimeout)
	}
}

func TestApplyLayeredSettingsToConfigAppliesWritingSkillDefault(t *testing.T) {
	cfg := &config.Config{}
	applyLayeredSettingsToConfig(cfg, config.LayeredSettings{
		Effective: config.Settings{
			WritingSkillDefault: "novel-heavy",
		},
	})
	if cfg.WritingSkillDefault != "novel-heavy" {
		t.Fatalf("writing skill default = %s, want novel-heavy", cfg.WritingSkillDefault)
	}
}

func TestApplyLayeredSettingsToConfigClearsMaxIterationWhenUnset(t *testing.T) {
	cfg := &config.Config{MaxIteration: 50}
	applyLayeredSettingsToConfig(cfg, config.LayeredSettings{
		Effective: config.Settings{},
	})
	if cfg.MaxIteration != 0 {
		t.Fatalf("max iteration = %d, want unlimited default 0", cfg.MaxIteration)
	}
}
