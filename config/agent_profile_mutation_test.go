package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOrdinaryMutationCannotBypassProfileHistory(t *testing.T) {
	dataDir := t.TempDir()
	if err := EnsureAgentProfiles(dataDir); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(AgentProfilesRoot(dataDir), "main", "writing.toml")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = MutateUserSettings(dataDir, "", func(current Settings) (Settings, error) {
		current.Theme = "dark"
		current.AgentModels.IDE.ThinkingLevel = "high"
		return current, nil
	})
	if err == nil {
		t.Fatal("ordinary mutator bypassed profile history")
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(before) {
		t.Fatalf("profile changed: %q, %v", after, err)
	}
	plain, err := ReadSettingsFile(UserConfigPath(dataDir))
	if err != nil || plain.Theme != "" {
		t.Fatalf("plain settings committed before rejecting mixed mutation: %+v, %v", plain, err)
	}
}
