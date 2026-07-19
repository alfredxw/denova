package app

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"denova/config"
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

func TestAppUpdateWorkspaceSettingsOnlyPersistsAgentOverrides(t *testing.T) {
	ws := t.TempDir()
	novaDir := t.TempDir()
	if err := config.WriteSettingsFile(config.WorkspaceConfigPath(ws), config.Settings{OpenAIModel: "legacy-workspace-model"}); err != nil {
		t.Fatal(err)
	}

	a := &App{
		cfg:       &config.Config{Workspace: ws, NovaDir: novaDir},
		workspace: ws,
	}
	enabled := false
	in := config.Settings{
		OpenAIModel: "ignored-new-model",
		AgentTools: config.AgentToolSettings{
			IDE: config.AgentToolOverride{ShellExecute: &enabled},
		},
	}
	layered, err := a.UpdateWorkspaceSettings(in)
	if err != nil {
		t.Fatal(err)
	}
	out, err := config.ReadSettingsFile(config.WorkspaceConfigPath(ws))
	if err != nil {
		t.Fatal(err)
	}
	if out.OpenAIModel != "legacy-workspace-model" {
		t.Fatalf("legacy workspace general setting should be preserved: %s", out.OpenAIModel)
	}
	if out.AgentTools.IDE.ShellExecute == nil || *out.AgentTools.IDE.ShellExecute {
		t.Fatalf("workspace Agent override not persisted: %#v", out.AgentTools.IDE)
	}
	if layered.Workspace.OpenAIModel != "" || layered.Effective.OpenAIModel == "ignored-new-model" {
		t.Fatalf("workspace general settings must not become effective: %#v", layered)
	}
}

func TestAppUpdateWorkspaceSettingsFiltersLLMInputLogSetting(t *testing.T) {
	ws := t.TempDir()
	novaDir := t.TempDir()

	a := &App{
		cfg:       &config.Config{Workspace: ws, NovaDir: novaDir},
		workspace: ws,
	}
	enabled := true
	retention := 1
	if _, err := a.UpdateWorkspaceSettings(config.Settings{
		LLMInputLogEnabled: &enabled,
		TraceCaptureLevel:  "debug",
		TraceExporter:      "otlp",
		TraceRetentionRuns: &retention,
	}); err != nil {
		t.Fatal(err)
	}
	out, err := config.ReadSettingsFile(config.WorkspaceConfigPath(ws))
	if err != nil {
		t.Fatal(err)
	}
	if out.LLMInputLogEnabled != nil {
		t.Fatalf("workspace llm input log setting should not be persisted: %#v", out.LLMInputLogEnabled)
	}
	if out.TraceCaptureLevel != "" || out.TraceExporter != "" || out.TraceRetentionRuns != nil {
		t.Fatalf("workspace trace debug settings should not be persisted: %#v", out)
	}
}

func TestAppUpdateWorkspaceSettingsRejectsStaleRevision(t *testing.T) {
	ws := t.TempDir()
	novaDir := t.TempDir()
	a := &App{
		cfg:       &config.Config{Workspace: ws, NovaDir: novaDir},
		workspace: ws,
	}
	layered, err := a.UpdateWorkspaceSettings(config.Settings{OpenAIModel: "front-base"})
	if err != nil {
		t.Fatal(err)
	}
	if layered.Revisions.Workspace == "" {
		t.Fatalf("workspace revision should be exposed")
	}

	time.Sleep(2 * time.Millisecond)
	path := config.WorkspaceConfigPath(ws)
	if err := config.WriteSettingsFile(path, config.Settings{OpenAIModel: "agent-model"}); err != nil {
		t.Fatal(err)
	}

	if _, err := a.UpdateWorkspaceSettings(config.Settings{OpenAIModel: "front-stale"}, layered.Revisions.Workspace); !errors.Is(err, config.ErrSettingsRevisionConflict) {
		t.Fatalf("expected revision conflict, got %v", err)
	}
	out, err := config.ReadSettingsFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if out.OpenAIModel != "agent-model" {
		t.Fatalf("stale save should not overwrite external change: %s", out.OpenAIModel)
	}
}

func TestAppSettingsConcurrentSameRevisionAllowsOneWriter(t *testing.T) {
	t.Run("user", func(t *testing.T) {
		ws := t.TempDir()
		novaDir := t.TempDir()
		path := config.UserConfigPath(novaDir)
		if err := config.WriteSettingsFile(path, config.Settings{OpenAIModel: "base"}); err != nil {
			t.Fatal(err)
		}
		baseRevision, err := config.SettingsFileRevision(path)
		if err != nil {
			t.Fatal(err)
		}
		a := &App{cfg: &config.Config{Workspace: ws, NovaDir: novaDir}, workspace: ws}
		models := []string{"first", "second"}
		errs := concurrentSettingsUpdates(t, len(models), func(index int) error {
			_, updateErr := a.UpdateUserSettings(config.Settings{OpenAIModel: models[index]}, baseRevision)
			return updateErr
		})
		assertOneSettingsWriter(t, errs)
		persisted, err := config.ReadSettingsFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if persisted.OpenAIModel != "first" && persisted.OpenAIModel != "second" {
			t.Fatalf("unexpected persisted user model %q", persisted.OpenAIModel)
		}
	})

	t.Run("workspace", func(t *testing.T) {
		ws := t.TempDir()
		novaDir := t.TempDir()
		path := config.WorkspaceConfigPath(ws)
		if err := config.WriteSettingsFile(path, config.Settings{AgentPrompts: config.AgentPromptSettings{
			IDE: config.AgentPromptOverride{SystemPrompt: "base"},
		}}); err != nil {
			t.Fatal(err)
		}
		baseRevision, err := config.SettingsFileRevision(path)
		if err != nil {
			t.Fatal(err)
		}
		a := &App{cfg: &config.Config{Workspace: ws, NovaDir: novaDir}, workspace: ws}
		prompts := []string{"first", "second"}
		errs := concurrentSettingsUpdates(t, len(prompts), func(index int) error {
			_, updateErr := a.UpdateWorkspaceSettings(config.Settings{AgentPrompts: config.AgentPromptSettings{
				IDE: config.AgentPromptOverride{SystemPrompt: prompts[index]},
			}}, baseRevision)
			return updateErr
		})
		assertOneSettingsWriter(t, errs)
		persisted, err := config.ReadSettingsFile(path)
		if err != nil {
			t.Fatal(err)
		}
		prompt := persisted.AgentPrompts.IDE.SystemPrompt
		if prompt != "first" && prompt != "second" {
			t.Fatalf("unexpected persisted workspace prompt %q", prompt)
		}
	})
}

func concurrentSettingsUpdates(t *testing.T, count int, update func(int) error) []error {
	t.Helper()
	errs := make([]error, count)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for index := 0; index < count; index++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			defer func() {
				if recovered := recover(); recovered != nil {
					errs[index] = fmt.Errorf("settings update panic: %v", recovered)
				}
			}()
			<-start
			errs[index] = update(index)
		}(index)
	}
	close(start)
	wg.Wait()
	return errs
}

func assertOneSettingsWriter(t *testing.T, errs []error) {
	t.Helper()
	succeeded := 0
	conflicted := 0
	for _, err := range errs {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, config.ErrSettingsRevisionConflict):
			conflicted++
		default:
			t.Fatalf("unexpected settings update error: %v", err)
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("concurrent settings results: succeeded=%d conflicted=%d errors=%v", succeeded, conflicted, errs)
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

func TestApplyLayeredSettingsToConfigAllowsUnlimitedAgentIdleTimeout(t *testing.T) {
	idleTimeout := 0
	cfg := &config.Config{AgentIdleTimeoutSeconds: 1800}
	applyLayeredSettingsToConfig(cfg, config.LayeredSettings{
		Effective: config.Settings{
			AgentIdleTimeoutSeconds: &idleTimeout,
		},
	})
	if cfg.AgentIdleTimeoutSeconds != 0 {
		t.Fatalf("agent idle timeout = %d, want 0", cfg.AgentIdleTimeoutSeconds)
	}
}

func TestApplyLayeredSettingsToConfigAppliesAgentToolResultLimit(t *testing.T) {
	limitKB := 128
	cfg := &config.Config{}
	applyLayeredSettingsToConfig(cfg, config.LayeredSettings{
		Effective: config.Settings{
			AgentToolResultLimitKB: &limitKB,
		},
	})
	if cfg.AgentToolResultLimitKB != limitKB {
		t.Fatalf("agent tool result limit = %d, want %d", cfg.AgentToolResultLimitKB, limitKB)
	}
}

func TestApplyLayeredSettingsToConfigMapsZeroToolResultLimitToHighDefault(t *testing.T) {
	limitKB := 0
	cfg := &config.Config{AgentToolResultLimitKB: 128}
	applyLayeredSettingsToConfig(cfg, config.LayeredSettings{
		Effective: config.Settings{
			AgentToolResultLimitKB: &limitKB,
		},
	})
	if cfg.AgentToolResultLimitKB != config.DefaultAgentToolResultLimitKB {
		t.Fatalf("agent tool result limit = %d, want %d", cfg.AgentToolResultLimitKB, config.DefaultAgentToolResultLimitKB)
	}
}

func TestAgentIdleTimeoutAllowsUnlimited(t *testing.T) {
	if got := agentIdleTimeout(config.Config{AgentIdleTimeoutSeconds: 0}); got != 0 {
		t.Fatalf("agent idle timeout = %s, want no limit", got)
	}
	if got := agentIdleTimeout(config.Config{AgentIdleTimeoutSeconds: 1800}); got != 30*time.Minute {
		t.Fatalf("agent idle timeout = %s, want 30m", got)
	}
}

func TestApplyLayeredSettingsToConfigAppliesWritingSkillDefaultAndImagePreset(t *testing.T) {
	cfg := &config.Config{}
	applyLayeredSettingsToConfig(cfg, config.LayeredSettings{
		Effective: config.Settings{
			WritingSkillDefault: "novel-heavy",
			IDEImagePresetID:    "realistic",
		},
	})
	if cfg.WritingSkillDefault != "novel-heavy" {
		t.Fatalf("writing skill default = %s, want novel-heavy", cfg.WritingSkillDefault)
	}
	if cfg.IDEImagePresetID != "realistic" {
		t.Fatalf("image preset default = %s, want realistic", cfg.IDEImagePresetID)
	}
}

func TestApplyLayeredSettingsToConfigAppliesLiveOutputChapterBodySetting(t *testing.T) {
	enabled := true
	cfg := &config.Config{}
	applyLayeredSettingsToConfig(cfg, config.LayeredSettings{
		Effective: config.Settings{
			HideChapterBodyLiveOutput: &enabled,
		},
	})
	if !cfg.HideChapterBodyLiveOutput {
		t.Fatalf("HideChapterBodyLiveOutput should be applied")
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
