package app

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"denova/config"
	agents "denova/internal/agents"
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
			name: "teller origin",
			req:  ConfigManagerRequest{Origin: "teller", Context: map[string]string{"teller_count": "3"}},
			want: []string{configManagerTellerSkill, configManagerStoryDirectorSkill, configManagerImagePresetSkill},
		},
		{
			name: "story director signal",
			req:  ConfigManagerRequest{Context: map[string]string{"story_director_count": "2", "selected_resource": "故事导演"}},
			want: []string{configManagerStoryDirectorSkill},
		},
		{
			name: "actor state signal",
			req:  ConfigManagerRequest{Origin: "actor_state", Context: map[string]string{"actor_state_count": "1", "selected_resource": "状态系统"}},
			want: []string{configManagerTellerSkill, configManagerStoryDirectorSkill, configManagerImagePresetSkill},
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
			name: "dedupe automation signals",
			req:  ConfigManagerRequest{Origin: "automation", Context: map[string]string{"automation_scope": "workspace"}},
			want: []string{configManagerAutomationSkill},
		},
		{
			name: "lore origin",
			req:  ConfigManagerRequest{Origin: "lore", ResourceID: "lore-config-agent"},
			want: []string{configManagerLoreSkill},
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

func TestBuildConfigManagerMessageBoundsRequestContext(t *testing.T) {
	message := buildConfigManagerMessage(ConfigManagerRequest{
		Instruction: "生成事件包",
		Origin:      "teller",
		Context: map[string]string{
			"large": strings.Repeat("设", configManagerRequestContextValueMaxBytes+100),
		},
	})
	if !strings.Contains(message, "已按请求上下文上限截断") {
		t.Fatalf("message should mark truncated context:\n%s", message)
	}
	if len([]byte(message)) > configManagerRequestContextValueMaxBytes+512 {
		t.Fatalf("message context should stay bounded, got %d bytes", len([]byte(message)))
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
	got, err := loadConfigManagerResourceSkills(context.Background(), cfg, ConfigManagerRequest{Origin: "automation"})
	if err != nil {
		t.Fatal(err)
	}
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

	got, err := loadConfigManagerResourceSkills(context.Background(), cfg, ConfigManagerRequest{Origin: "automation"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("loaded skills with override disabled = %#v, want none", got)
	}
}

func TestLoadConfigManagerResourceSkillsPreservesExactSourceForComposerProvenance(t *testing.T) {
	root := t.TempDir()
	builtin := filepath.Join(root, "builtin")
	body := strings.Repeat("a", 256*1024+4096)
	writeConfigManagerSkill(t, builtin, configManagerAutomationSkill, body, "config_manager")
	cfg := &config.Config{SkillsDir: builtin}

	got, err := loadConfigManagerResourceSkills(context.Background(), cfg, ConfigManagerRequest{Origin: "automation"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("loaded skills = %#v, want one", got)
	}
	if got[0].Content != body {
		t.Fatalf("loader changed source before composition: got_bytes=%d want_bytes=%d", len(got[0].Content), len(body))
	}

	composition, err := agents.ComposeConfigManagerInstruction(cfg, nil, got...)
	if err != nil {
		t.Fatal(err)
	}
	var receipt *agents.SystemPromptManifestEntry
	for _, entry := range composition.Manifest() {
		if entry.Source == "配置 Skill" {
			entry := entry
			receipt = &entry
			break
		}
	}
	if receipt == nil {
		t.Fatalf("composition manifest has no resource Skill receipt: %#v", composition.Manifest())
	}
	exactSource := "### /" + configManagerAutomationSkill + "\n\ndescription: test " + configManagerAutomationSkill + "\n\n" + body
	wantOriginalSHA := fmt.Sprintf("%x", sha256.Sum256([]byte(exactSource)))
	if receipt.OriginalBytes != len(exactSource) || receipt.OriginalSHA != wantOriginalSHA {
		t.Fatalf("original provenance = bytes:%d sha:%s, want bytes:%d sha:%s", receipt.OriginalBytes, receipt.OriginalSHA, len(exactSource), wantOriginalSHA)
	}
	if !receipt.Truncated || !receipt.Included || receipt.IncludedBytes > config.DefaultAgentContextMaxFragmentBytes || receipt.IncludedSHA == receipt.OriginalSHA {
		t.Fatalf("included provenance should describe composer truncation: %#v", *receipt)
	}
	if !strings.Contains(composition.Instruction(), "System source truncated by configured context budget") {
		t.Fatalf("model-visible instruction must include the composer truncation marker")
	}
}

func TestLoadConfigManagerResourceSkillsRejectsSourceAboveHardLimitWithoutPartialContent(t *testing.T) {
	root := t.TempDir()
	builtin := filepath.Join(root, "builtin")
	writeConfigManagerSkill(t, builtin, configManagerAutomationSkill, strings.Repeat("a", configManagerResourceSkillMaxSourceBytes+1), "config_manager")

	got, err := loadConfigManagerResourceSkills(context.Background(), &config.Config{SkillsDir: builtin}, ConfigManagerRequest{Origin: "automation"})
	if err == nil || !strings.Contains(err.Error(), "resource Skill exceeds hard source limit") || !strings.Contains(err.Error(), "配置 Skill 超过加载硬上限") {
		t.Fatalf("oversized source error = %v, want bilingual hard-limit error", err)
	}
	if got != nil {
		t.Fatalf("oversized source must fail closed without partial Skills: %#v", got)
	}
}

func TestLoadConfigManagerResourceSkillsRejectsTotalAboveHardLimitWithoutPartialSkills(t *testing.T) {
	root := t.TempDir()
	builtin := filepath.Join(root, "builtin")
	body := strings.Repeat("a", 400*1024)
	for _, name := range []string{configManagerTellerSkill, configManagerStoryDirectorSkill, configManagerImagePresetSkill, configManagerLoreSkill} {
		writeConfigManagerSkill(t, builtin, name, body, "config_manager")
	}
	req := ConfigManagerRequest{
		Origin:  "teller",
		Context: map[string]string{"signals": "write_lore_items"},
	}

	got, err := loadConfigManagerResourceSkills(context.Background(), &config.Config{SkillsDir: builtin}, req)
	if err == nil || !strings.Contains(err.Error(), "resource Skills exceed hard total source limit") || !strings.Contains(err.Error(), "配置 Skills 超过加载总硬上限") {
		t.Fatalf("aggregate source error = %v, want bilingual hard-limit error", err)
	}
	if got != nil {
		t.Fatalf("aggregate overflow must fail closed without partial Skills: %#v", got)
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
