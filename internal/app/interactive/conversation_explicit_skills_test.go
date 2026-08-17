package interactiveapp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"denova/config"
	novaskills "denova/internal/agents/skills"
)

func TestInteractiveConversationResolvesMultipleExplicitSkillsFromWholeMessage(t *testing.T) {
	skillsDir := t.TempDir()
	writeInteractiveExplicitSkill(t, skillsDir, "atmosphere", "ATMOSPHERE_BODY")
	writeInteractiveExplicitSkill(t, skillsDir, "dialogue", "DIALOGUE_BODY")
	workspace := t.TempDir()
	cfg := &config.Config{SkillsDir: skillsDir, Workspace: workspace, DenovaDir: t.TempDir()}
	conversation := NewConversation(nil, cfg.DataDir(), workspace, "story", "main", "", 800, cfg)

	resolved, err := conversation.ResolveExplicitSkills(
		context.Background(),
		"我先观察，然后请用 /atmosphere 推进，句末再用 /dialogue，并重复 /atmosphere。",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved) != 2 || resolved[0].Name != "atmosphere" || resolved[1].Name != "dialogue" {
		t.Fatalf("resolved skills = %#v", resolved)
	}
	if !strings.Contains(resolved[0].Instructions, "ATMOSPHERE_BODY") || !strings.Contains(resolved[1].Instructions, "DIALOGUE_BODY") {
		t.Fatalf("resolved Skill bodies = %#v", resolved)
	}
}

func writeInteractiveExplicitSkill(t *testing.T, root, name, body string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: " + name + "\ndescription: " + name + " instructions\nagent: interactive_story\n---\n\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(dir, novaskills.SkillFileName), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
