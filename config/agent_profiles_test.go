package config

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestEnsureAgentProfilesMigratesV033SettingsWithBackup(t *testing.T) {
	dataDir := t.TempDir()
	enabled := true
	toolParallelism := 6
	legacy := Settings{
		Theme: "light",
		AgentModels: AgentModelSettings{
			IDE:           AgentModelOverride{ProfileID: "writing-model"},
			ConfigManager: AgentModelOverride{ThinkingLevel: "low"},
		},
		AgentPrompts: AgentPromptSettings{
			Default:       AgentPromptOverride{FlowPrompt: "shared flow"},
			IDE:           AgentPromptOverride{SystemPrompt: "writing instructions"},
			ConfigManager: AgentPromptOverride{FlowPrompt: "retired flow"},
		},
		AgentToolParallelism: &toolParallelism,
		CustomAgents: []CustomAgentConfig{{
			ID: "editor", Name: "Editor", Contract: AgentContractWritingPrimary, Enabled: &enabled,
		}},
		SubAgents: []SubAgentConfig{{
			ID: "researcher", Name: "Researcher", Description: "Research project evidence",
			SystemPrompt: "Return concise evidence.", Parents: []string{AgentKindIDE}, Enabled: &enabled,
		}},
	}
	userPath := UserConfigPath(dataDir)
	if err := WriteSettingsFile(userPath, legacy); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(userPath)
	if err != nil {
		t.Fatal(err)
	}

	if err := EnsureAgentProfiles(dataDir); err != nil {
		t.Fatal(err)
	}

	backup, err := os.ReadFile(userPath + agentProfilesBackupSuffix)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(backup, original) {
		t.Fatal("Agent settings migration backup does not match the v0.3.3 source")
	}
	for _, relative := range []string{
		"AGENTS.md", ".profiles-v1", "main/defaults.toml", "main/writing.toml",
		"custom/editor.toml", "subagents/researcher.toml",
	} {
		if _, err := os.Stat(filepath.Join(AgentProfilesRoot(dataDir), filepath.FromSlash(relative))); err != nil {
			t.Fatalf("migrated Agents Project file %q: %v", relative, err)
		}
	}

	plain, err := ReadSettingsFile(userPath)
	if err != nil {
		t.Fatal(err)
	}
	if plain.Theme != "light" {
		t.Fatalf("ordinary user setting was lost: %#v", plain)
	}
	if hasAgentProfileSettings(plain) {
		t.Fatalf("active Agent settings remained in config.toml: %#v", agentProfileSettings(plain))
	}
	if !reflect.DeepEqual(plain.AgentModels.ConfigManager, legacy.AgentModels.ConfigManager) ||
		!reflect.DeepEqual(plain.AgentPrompts.ConfigManager, legacy.AgentPrompts.ConfigManager) {
		t.Fatalf("retired v0.3.3 tombstones were not preserved: %#v", plain)
	}

	profiles, ready, err := LoadAgentProfileSettings(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if !ready || profiles.AgentModels.IDE.ProfileID != "writing-model" ||
		profiles.AgentPrompts.Default.FlowPrompt != "shared flow" ||
		profiles.AgentPrompts.IDE.SystemPrompt != "writing instructions" ||
		profiles.AgentToolParallelism == nil || *profiles.AgentToolParallelism != toolParallelism ||
		len(profiles.CustomAgents) != 1 || profiles.CustomAgents[0].ID != "editor" ||
		len(profiles.SubAgents) != 1 || profiles.SubAgents[0].ID != "researcher" {
		t.Fatalf("migrated Agent Profiles do not preserve v0.3.3 behavior: %#v", profiles)
	}
}

func TestAgentProfilesIsolateMalformedFilesAndTrackAggregateRevision(t *testing.T) {
	dataDir := t.TempDir()
	if err := EnsureAgentProfiles(dataDir); err != nil {
		t.Fatal(err)
	}
	enabled := true
	if _, err := MutateUserSettings(dataDir, "", func(settings Settings) (Settings, error) {
		settings.CustomAgents = []CustomAgentConfig{{
			ID: "editor", Name: "Editor", Contract: AgentContractGeneralProject, Enabled: &enabled,
		}}
		return settings, nil
	}); err != nil {
		t.Fatal(err)
	}
	invalidPath := filepath.Join(AgentProfilesRoot(dataDir), "custom", "broken.toml")
	if err := os.WriteFile(invalidPath, []byte("schema_version = ["), 0o644); err != nil {
		t.Fatal(err)
	}

	profiles, ready, err := LoadAgentProfileSettings(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if !ready || len(profiles.CustomAgents) != 1 || profiles.CustomAgents[0].ID != "editor" {
		t.Fatalf("one malformed profile invalidated the valid catalog: %#v", profiles.CustomAgents)
	}

	baseRevision, err := UserSettingsRevision(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	generalPath := filepath.Join(AgentProfilesRoot(dataDir), "main", "general.toml")
	content, err := os.ReadFile(generalPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(generalPath, append(content, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	updatedRevision, err := UserSettingsRevision(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if updatedRevision == baseRevision {
		t.Fatal("aggregate user settings revision did not include Agent Profile edits")
	}
	if _, err := MutateUserSettings(dataDir, baseRevision, func(settings Settings) (Settings, error) {
		settings.Theme = "dark"
		return settings, nil
	}); !errors.Is(err, ErrSettingsRevisionConflict) {
		t.Fatalf("stale settings mutation error = %v, want revision conflict", err)
	}
}
