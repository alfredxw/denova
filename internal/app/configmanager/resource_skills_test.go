package configmanager

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"denova/config"
)

func TestConfigManagerAlwaysLoadsOneRootSkill(t *testing.T) {
	requests := []Request{
		{},
		{Origin: "automation"},
		{Origin: "teller", ResourceID: "style"},
		{Origin: "agents", Context: map[string]string{"sub_agent": "researcher"}},
	}
	for _, request := range requests {
		if got := resourceSkillNames(request); !reflect.DeepEqual(got, []string{resourceSkillName}) {
			t.Fatalf("skill names for %+v = %v", request, got)
		}
	}
}

func TestBuildConfigManagerMessageBoundsRequestContext(t *testing.T) {
	message := BuildMessage(Request{
		Instruction: "生成事件包",
		Origin:      "teller",
		Context: map[string]string{
			"large": strings.Repeat("设", requestContextValueMaxBytes+100),
		},
	})
	if !strings.Contains(message, "已按请求上下文上限截断") {
		t.Fatalf("message should mark truncated context:\n%s", message)
	}
	if len([]byte(message)) > requestContextValueMaxBytes+512 {
		t.Fatalf("message context should stay bounded, got %d bytes", len([]byte(message)))
	}
}

func TestLoadConfigManagerRootSkillUsesActivePrecedence(t *testing.T) {
	root := t.TempDir()
	builtin := filepath.Join(root, "builtin")
	novaDir := filepath.Join(root, "nova")
	workspace := filepath.Join(root, "workspace")
	writeConfigManagerSkill(t, builtin, resourceSkillName, "builtin body", RuntimeMode)
	writeConfigManagerSkill(t, filepath.Join(novaDir, "skills"), resourceSkillName, "user body", RuntimeMode)
	writeConfigManagerSkill(t, filepath.Join(workspace, ".nova", "skills"), resourceSkillName, "workspace body", RuntimeMode)

	cfg := &config.Config{SkillsDir: builtin, NovaDir: novaDir, Workspace: workspace}
	got, err := loadResourceSkills(context.Background(), cfg, Request{Origin: "automation"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != resourceSkillName || got[0].Content != "workspace body" {
		t.Fatalf("loaded skill = %#v, want active config-manager workspace body", got)
	}
}

func TestLoadConfigManagerRootSkillRespectsOverride(t *testing.T) {
	builtin := filepath.Join(t.TempDir(), "builtin")
	writeConfigManagerSkill(t, builtin, resourceSkillName, "builtin body", RuntimeMode)
	cfg := &config.Config{
		SkillsDir: builtin,
		AgentSkills: config.AgentSkillSettings{
			ConfigManager: config.AgentSkillOverride{resourceSkillName: false},
		},
	}
	got, err := loadResourceSkills(context.Background(), cfg, Request{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("disabled root skill loaded: %#v", got)
	}
}

func TestLoadConfigManagerRootSkillRejectsOversizedSource(t *testing.T) {
	builtin := filepath.Join(t.TempDir(), "builtin")
	writeConfigManagerSkill(t, builtin, resourceSkillName, strings.Repeat("a", resourceSkillMaxSourceBytes+1), RuntimeMode)
	got, err := loadResourceSkills(context.Background(), &config.Config{SkillsDir: builtin}, Request{})
	if err == nil || !strings.Contains(err.Error(), "resource Skill exceeds hard source limit") || !strings.Contains(err.Error(), "配置 Skill 超过加载硬上限") {
		t.Fatalf("oversized source error = %v", err)
	}
	if got != nil {
		t.Fatalf("oversized source must fail closed: %#v", got)
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
