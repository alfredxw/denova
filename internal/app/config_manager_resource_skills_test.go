package app

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"nova/config"
)

func TestConfigManagerResourceSkillNames(t *testing.T) {
	tests := []struct {
		name string
		req  ConfigManagerRequest
		want []string
	}{
		{
			name: "automation origin",
			req:  ConfigManagerRequest{Origin: "automation", Context: map[string]string{"active_automation_id": "auto-1"}},
			want: []string{configManagerAutomationSkill},
		},
		{
			name: "story memory origin",
			req:  ConfigManagerRequest{Origin: "story_memory", StoryID: "story-1", Context: map[string]string{"selected_structure_id": "current_state"}},
			want: []string{configManagerStoryMemorySkill},
		},
		{
			name: "teller origin",
			req:  ConfigManagerRequest{Origin: "teller", Context: map[string]string{"teller_count": "3"}},
			want: []string{configManagerTellerSkill},
		},
		{
			name: "skills origin",
			req:  ConfigManagerRequest{Origin: "skills", ResourceID: "beats"},
			want: []string{configManagerSkillsSkill},
		},
		{
			name: "agents origin",
			req:  ConfigManagerRequest{Origin: "agents", ResourceID: "user:ide"},
			want: []string{configManagerAgentConfigSkill},
		},
		{
			name: "context signal without origin",
			req:  ConfigManagerRequest{Context: map[string]string{"record_count": "2", "selected_structure_name": "角色"}},
			want: []string{configManagerStoryMemorySkill},
		},
		{
			name: "dedupe automation signals",
			req:  ConfigManagerRequest{Origin: "automation", Context: map[string]string{"automation_scope": "workspace"}},
			want: []string{configManagerAutomationSkill},
		},
		{
			name: "unknown",
			req:  ConfigManagerRequest{Origin: "lore", ResourceID: "lore-config-agent"},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := configManagerResourceSkillNames(tt.req)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("skill names = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestLoadConfigManagerResourceSkillsUsesActiveSkillPrecedence(t *testing.T) {
	root := t.TempDir()
	builtin := filepath.Join(root, "builtin")
	novaDir := filepath.Join(root, "nova")
	workspace := filepath.Join(root, "workspace")
	writeConfigManagerSkill(t, builtin, configManagerAutomationSkill, "builtin body", "config_manager")
	writeConfigManagerSkill(t, filepath.Join(novaDir, "skills"), configManagerAutomationSkill, "user body", "config_manager")
	writeConfigManagerSkill(t, filepath.Join(workspace, ".nova", "skills"), configManagerAutomationSkill, "workspace body", "config_manager")

	cfg := &config.Config{SkillsDir: builtin, NovaDir: novaDir, Workspace: workspace}
	got := loadConfigManagerResourceSkills(context.Background(), cfg, ConfigManagerRequest{Origin: "automation"})
	if len(got) != 1 {
		t.Fatalf("loaded skills = %#v, want one", got)
	}
	if got[0].Name != configManagerAutomationSkill || got[0].Content != "workspace body" {
		t.Fatalf("loaded skill = %#v, want active workspace body", got[0])
	}
}

func TestLoadConfigManagerResourceSkillsRespectsAgentOverride(t *testing.T) {
	root := t.TempDir()
	builtin := filepath.Join(root, "builtin")
	writeConfigManagerSkill(t, builtin, configManagerAutomationSkill, "builtin body", "config_manager")
	disabled := false
	cfg := &config.Config{
		SkillsDir: builtin,
		AgentSkills: config.AgentSkillSettings{
			ConfigManager: config.AgentSkillOverride{configManagerAutomationSkill: disabled},
		},
	}

	got := loadConfigManagerResourceSkills(context.Background(), cfg, ConfigManagerRequest{Origin: "automation"})
	if len(got) != 0 {
		t.Fatalf("loaded skills with override disabled = %#v, want none", got)
	}
}

func TestLoadConfigManagerResourceSkillsBoundsContent(t *testing.T) {
	root := t.TempDir()
	builtin := filepath.Join(root, "builtin")
	writeConfigManagerSkill(t, builtin, configManagerAutomationSkill, strings.Repeat("好", configManagerResourceSkillMaxBytes), "config_manager")
	cfg := &config.Config{SkillsDir: builtin}

	got := loadConfigManagerResourceSkills(context.Background(), cfg, ConfigManagerRequest{Origin: "automation"})
	if len(got) != 1 {
		t.Fatalf("loaded skills = %#v, want one", got)
	}
	if len([]byte(got[0].Content)) > configManagerResourceSkillMaxBytes {
		t.Fatalf("content bytes = %d, want <= %d", len([]byte(got[0].Content)), configManagerResourceSkillMaxBytes)
	}
	if !utf8.ValidString(got[0].Content) {
		t.Fatalf("truncated content should remain valid utf8")
	}
}

func writeConfigManagerSkill(t *testing.T, root, name, body, agent string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: " + name + "\ndescription: test " + name + "\nagent: " + agent + "\n---\n\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
