package agentprofiles

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"denova/config"
	"denova/internal/project"
	workspacechange "denova/internal/workspace/change"
)

func profileTestStore(t *testing.T) (string, *workspacechange.Service) {
	t.Helper()
	dataDir := t.TempDir()
	if err := config.EnsureAgentProfiles(dataDir); err != nil {
		t.Fatal(err)
	}
	registry := project.NewRegistry(dataDir)
	record, err := registry.EnsureAgents(config.AgentProfilesRoot(dataDir))
	if err != nil {
		t.Fatal(err)
	}
	layout, err := registry.EnsureStore(record)
	if err != nil {
		t.Fatal(err)
	}
	changes, err := workspacechange.ForWorkspaceAt(layout.ContentRoot, layout.StoreRoot)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = workspacechange.ForgetWorkspace(layout.ContentRoot) })
	return dataDir, changes
}

func TestProfileCommitsPreserveConcurrentEditorSaveAndIndependentSuccess(t *testing.T) {
	dataDir, changes := profileTestStore(t)
	plan, err := config.PlanUserSettingsMutation(dataDir, config.UserSettingsMutationRequest{Mutate: func(current config.Settings) (config.Settings, error) {
		current.AgentModels.IDE.ThinkingLevel = "high"
		current.AgentModels.InteractiveStory.ThinkingLevel = "high"
		return current, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Profiles) != 2 {
		t.Fatalf("plan = %#v", plan)
	}
	var writing config.AgentProfileChange
	for _, change := range plan.Profiles {
		if change.Path == "main/writing.toml" {
			writing = change
		}
	}
	writingPath := filepath.Join(config.AgentProfilesRoot(dataDir), "main", "writing.toml")
	before, err := os.ReadFile(writingPath)
	if err != nil {
		t.Fatal(err)
	}
	edited := "# Concurrent editor update.\n" + string(before)
	if _, err := changes.SaveFile(context.Background(), writing.Path, edited, writing.BaseRevision); err != nil {
		t.Fatal(err)
	}
	err = commit(context.Background(), dataDir, plan)
	var partial *MutationError
	if !errors.As(err, &partial) || !errors.Is(err, config.ErrSettingsRevisionConflict) {
		t.Fatalf("commit error = %v", err)
	}
	want := []FileResult{
		{Path: "agents/main/game.toml", Status: "saved"},
		{Path: "agents/main/writing.toml", Status: "failed", Code: "revision_conflict"},
	}
	if !reflect.DeepEqual(partial.Files, want) {
		t.Fatalf("results = %#v, want %#v", partial.Files, want)
	}
	got, err := os.ReadFile(writingPath)
	if err != nil || string(got) != edited {
		t.Fatalf("editor bytes changed: %q, %v", got, err)
	}
	settings, _, err := config.LoadAgentProfileSettings(dataDir)
	if err != nil || settings.AgentModels.InteractiveStory.ThinkingLevel != "high" {
		t.Fatalf("independent game save = %#v, %v", settings.AgentModels.InteractiveStory, err)
	}
}

func TestInvalidExplicitProfileResetPreservesBytesAndReportsPartialSuccess(t *testing.T) {
	dataDir, _ := profileTestStore(t)
	brokenPath := filepath.Join(config.AgentProfilesRoot(dataDir), "main", "game.toml")
	broken := []byte("# Unfinished config\nschema_version = [")
	if err := os.WriteFile(brokenPath, broken, 0o644); err != nil {
		t.Fatal(err)
	}
	patch := []byte(`{"theme":"dark","agent_models":{"ide":{"thinking_level":"high"},"interactive_story":null}}`)
	paths, err := config.AgentProfilePatchPaths(patch)
	if err != nil {
		t.Fatal(err)
	}
	_, err = Mutate(context.Background(), dataDir, config.UserSettingsMutationRequest{ProfilePaths: paths, Mutate: func(current config.Settings) (config.Settings, error) {
		return config.ApplySettingsMergePatch(current, patch)
	}})
	var partial *MutationError
	if !errors.As(err, &partial) || !errors.Is(err, config.ErrInvalidAgentProfile) {
		t.Fatalf("mutation error = %v", err)
	}
	want := []FileResult{
		{Path: "config.toml", Status: "saved"},
		{Path: "agents/main/game.toml", Status: "failed", Code: "invalid_profile"},
		{Path: "agents/main/writing.toml", Status: "saved"},
	}
	if !reflect.DeepEqual(partial.Files, want) {
		t.Fatalf("results = %#v, want %#v", partial.Files, want)
	}
	got, err := os.ReadFile(brokenPath)
	if err != nil || string(got) != string(broken) {
		t.Fatalf("invalid target overwritten: %q, %v", got, err)
	}
}

func TestProfileReplaceAndDeleteRetainRecoverableOriginalContent(t *testing.T) {
	dataDir, changes := profileTestStore(t)
	enabled := true
	_, err := Mutate(context.Background(), dataDir, config.UserSettingsMutationRequest{Mutate: func(current config.Settings) (config.Settings, error) {
		current.CustomAgents = []config.CustomAgentConfig{{ID: "editor", Name: "Editor", Contract: config.AgentContractGeneralProject, Enabled: &enabled}}
		return current, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(config.AgentProfilesRoot(dataDir), "custom", "editor.toml")
	for _, operation := range []string{"replace", "delete"} {
		t.Run(operation, func(t *testing.T) {
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			before = append([]byte("# Original user comment.\n"), before...)
			if err := os.WriteFile(path, before, 0o644); err != nil {
				t.Fatal(err)
			}
			_, err = Mutate(context.Background(), dataDir, config.UserSettingsMutationRequest{Mutate: func(current config.Settings) (config.Settings, error) {
				if operation == "delete" {
					current.CustomAgents = nil
				} else {
					current.CustomAgents[0].Name = "Revised editor"
				}
				return current, nil
			}})
			if err != nil {
				t.Fatal(err)
			}
			groups, err := changes.ListGroups(context.Background(), workspacechange.ChangeFilter{Path: "custom/editor.toml"})
			if err != nil {
				t.Fatal(err)
			}
			found := ""
			for _, summary := range groups {
				group, err := changes.GetGroup(context.Background(), summary.ID)
				if err != nil {
					t.Fatal(err)
				}
				for _, change := range group.ChangeSets {
					if change.BeforeContent == string(before) && (operation != "delete" || !change.AfterExists) {
						found = summary.ID
					}
				}
			}
			if found == "" {
				t.Fatal("original bytes missing from durable change history")
			}
			if operation == "delete" {
				if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("deleted file error = %v", err)
				}
			}
			if _, err := changes.Undo(context.Background(), workspacechange.HistoryRequest{GroupID: found}); err != nil {
				t.Fatal(err)
			}
			restored, err := os.ReadFile(path)
			if err != nil || string(restored) != string(before) {
				t.Fatalf("history failed to restore exact bytes: %q, %v", restored, err)
			}
		})
	}
}
