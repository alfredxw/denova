package config

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestQuickPromptCommandPreferencePersistsWithoutCustomizingPrompts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	settings := Settings{}
	for _, enabled := range []bool{true, false} {
		changes, err := json.Marshal(map[string]bool{"agent_quick_prompts_in_commands": enabled})
		if err != nil {
			t.Fatal(err)
		}
		settings, err = ApplySettingsMergePatch(settings, changes)
		if err != nil {
			t.Fatal(err)
		}
		if err := WriteSettingsFile(path, settings); err != nil {
			t.Fatal(err)
		}
		settings, err = ReadSettingsFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if settings.AgentQuickPromptsInCommands == nil || *settings.AgentQuickPromptsInCommands != enabled || settings.AgentQuickPrompts != nil {
			t.Fatalf("command preference did not round trip independently: enabled=%v settings=%+v", enabled, settings)
		}
		if err := ValidateWorkspaceSettingsPatch(changes); err == nil {
			t.Fatal("workspace must not override the user's quick prompt preference")
		}
	}
}
