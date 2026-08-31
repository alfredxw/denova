package settings

import (
	"os"
	"path/filepath"
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
