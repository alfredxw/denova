package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"denova/config"
	"denova/internal/agentprofiles"
	appsettings "denova/internal/app/settings"
)

func TestModelSelectionPreservesUnrelatedAgentProfiles(t *testing.T) {
	for _, mode := range []string{ConversationModeWriting, ConversationModeInteractive} {
		t.Run(string(mode), func(t *testing.T) {
			application := newExecutionProfileTestApp(t)
			binding := ConversationConfigBinding{Mode: mode, ProjectID: application.ProjectID()}
			other := "writing.toml"
			if mode == ConversationModeWriting {
				binding.SessionID = application.session.ID
				other = "game.toml"
			}
			root := config.AgentProfilesRoot(application.cfg.DataDir())
			brokenPath := filepath.Join(root, "main", other)
			broken := []byte("# Keep this unfinished user edit.\nschema_version = [\n")
			if err := os.WriteFile(brokenPath, broken, 0o644); err != nil {
				t.Fatal(err)
			}
			generalPath := filepath.Join(root, "main", "general.toml")
			general, err := os.ReadFile(generalPath)
			if err != nil {
				t.Fatal(err)
			}
			general = append([]byte("# Preserve unrelated formatting.\n"), general...)
			if err := os.WriteFile(generalPath, general, 0o644); err != nil {
				t.Fatal(err)
			}
			current, err := application.ConversationConfig(context.Background(), binding)
			if err != nil {
				t.Fatal(err)
			}
			thinking := "high"
			if _, err := application.PatchConversationConfig(context.Background(), binding, ConversationConfigPatch{ThinkingLevel: &thinking}, current.Revision); err != nil {
				t.Fatal(err)
			}
			for path, want := range map[string][]byte{brokenPath: broken, generalPath: general} {
				got, err := os.ReadFile(path)
				if err != nil || string(got) != string(want) {
					t.Errorf("unrelated profile %s changed: got %q, want %q, error %v", path, got, want, err)
				}
			}
		})
	}
}

func TestSettingsPatchRefreshesIndependentSuccessAfterInvalidProfile(t *testing.T) {
	application := newExecutionProfileTestApp(t)
	path := filepath.Join(config.AgentProfilesRoot(application.cfg.DataDir()), "main", "game.toml")
	broken := "# Preserve the user's unfinished game settings.\nschema_version = ["
	if err := os.WriteFile(path, []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := application.SettingsService().Patch(appsettings.Global(), config.SettingsLayerUser,
		[]byte(`{"agent_models":{"ide":{"thinking_level":"high"},"interactive_story":{"thinking_level":"max"}}}`), "")
	var partial *agentprofiles.MutationError
	if !errors.As(err, &partial) || !errors.Is(err, config.ErrInvalidAgentProfile) {
		t.Fatalf("patch error = %v", err)
	}
	if result.User.AgentModels.IDE.ThinkingLevel != "high" || application.cfg.AgentModels.IDE.ThinkingLevel != "high" {
		t.Fatalf("successful writing save was not refreshed: response=%+v runtime=%+v", result.User.AgentModels.IDE, application.cfg.AgentModels.IDE)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != broken {
		t.Fatalf("invalid game profile changed: %q, %v", got, err)
	}
}
