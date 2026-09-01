package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"denova/config"
)

type reloadTestHost struct {
	runtime Runtime
	applied config.LayeredSettings
	layer   config.SettingsLayer
}

func (host *reloadTestHost) SettingsRuntime(Target) (Runtime, error) { return host.runtime, nil }

func (host *reloadTestHost) ApplySettings(settings config.LayeredSettings, layer config.SettingsLayer) {
	host.applied = settings
	host.layer = layer
}

func TestSnapshotPublishesReadonlyCompactionPromptSources(t *testing.T) {
	dataDir := t.TempDir()
	if err := config.EnsureAgentProfiles(dataDir); err != nil {
		t.Fatal(err)
	}
	service := NewService(&reloadTestHost{runtime: Runtime{Config: config.Config{DenovaDir: dataDir, NovaDir: dataDir}}})

	layered, err := service.Snapshot(Global())
	if err != nil {
		t.Fatal(err)
	}
	if len(layered.BuiltinCompactionSources.IDE.Sources) != 3 ||
		!strings.Contains(layered.BuiltinCompactionSources.IDE.Sources[2].Content, "Workspace/writing requirements") {
		t.Fatalf("Writing Agent compaction sources = %#v", layered.BuiltinCompactionSources.IDE)
	}
	if !strings.Contains(layered.BuiltinCompactionSources.InteractiveStory.Sources[2].Content, "Game-mode requirements") {
		t.Fatalf("Game Agent compaction sources = %#v", layered.BuiltinCompactionSources.InteractiveStory)
	}
}

func TestReloadAppliesOutOfBandAgentProfileMutation(t *testing.T) {
	dataDir := t.TempDir()
	if err := config.EnsureAgentProfiles(dataDir); err != nil {
		t.Fatal(err)
	}
	profile := []byte("schema_version = 1\nkind = \"general\"\n\n[prompt]\nsystem_prompt = \"Use the updated profile.\"\n")
	if err := os.WriteFile(filepath.Join(config.AgentProfilesRoot(dataDir), "main", "general.toml"), profile, 0o644); err != nil {
		t.Fatal(err)
	}
	host := &reloadTestHost{runtime: Runtime{Config: config.Config{DenovaDir: dataDir, NovaDir: dataDir}}}
	service := NewService(host)

	layered, err := service.Reload(Global(), config.SettingsLayerUser)
	if err != nil {
		t.Fatal(err)
	}
	if got := layered.User.AgentPrompts.General.SystemPrompt; got != "Use the updated profile." {
		t.Fatalf("reloaded General Agent prompt = %q", got)
	}
	if got := host.applied.User.AgentPrompts.General.SystemPrompt; got != "Use the updated profile." {
		t.Fatalf("applied General Agent prompt = %q", got)
	}
	if host.layer != config.SettingsLayerUser {
		t.Fatalf("applied layer = %q", host.layer)
	}
}
