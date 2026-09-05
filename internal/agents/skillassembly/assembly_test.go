package skillassembly

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"denova/config"
	"denova/internal/agents/prompts"
)

func TestBuildAdvertisesEffectiveCatalogAndLoadsSkillByName(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	skillsDir := filepath.Join(root, "builtin-skills")
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	writeAssemblySkill(t, skillsDir, "alpha", "Pinned routing guidance.", "Always use alpha.")
	writeAssemblySkill(t, skillsDir, "beta", "General routing guidance.", "Always use beta.")
	writeAssemblySkill(t, skillsDir, "blocked", "Unavailable guidance.", "Never load this.")

	cfg := &config.Config{
		SkillsDir: skillsDir,
		DenovaDir: filepath.Join(root, "data"),
		Workspace: workspace,
		AgentSkills: config.AgentSkillSettings{General: config.AgentSkillOverride{
			"alpha": true, "blocked": false,
		}},
	}
	base, err := prompts.ComposeBuiltinSystemInstruction(
		cfg, config.AgentKindGeneral, "general", workspace,
		"test_base", "Test workflow", "exercise Skill assembly", "Follow the user request.",
	)
	if err != nil {
		t.Fatal(err)
	}

	assembly, err := Build(
		ctx, cfg, config.AgentKindGeneral, true,
		config.ResolvedAgentToolSettings{config.AgentToolSkills: true}, base,
	)
	if err != nil {
		t.Fatal(err)
	}
	prompt := assembly.SystemPrompt.Instruction()
	for _, expected := range []string{
		"- alpha: Pinned routing guidance.",
		"- beta: General routing guidance.",
	} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("system prompt does not contain %q:\n%s", expected, prompt)
		}
	}
	if strings.Contains(prompt, "- blocked:") {
		t.Fatalf("system prompt exposes a blocked Skill:\n%s", prompt)
	}
	if strings.Contains(prompt, "action `list`") {
		t.Fatalf("system prompt still requires model-side Skill discovery:\n%s", prompt)
	}

	if len(assembly.Tools) != 1 {
		t.Fatalf("Skill tools = %d, want 1", len(assembly.Tools))
	}
	info, err := assembly.Tools[0].Tool.Info(ctx)
	if err != nil {
		t.Fatal(err)
	}
	schema, err := info.ParamsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "skill" || len(schema.OneOf) != 0 || schema.Properties.Len() != 1 {
		t.Fatalf("Skill tool schema = %#v", schema)
	}
	if _, ok := schema.Properties.Get("name"); !ok {
		t.Fatalf("Skill tool schema has no name property: %#v", schema)
	}
	if _, ok := schema.Properties.Get("action"); ok {
		t.Fatalf("Skill tool schema still exposes action: %#v", schema)
	}
	if !slices.Contains(schema.Required, "name") {
		t.Fatalf("Skill tool schema does not require name: %#v", schema.Required)
	}

	result, err := assembly.Tools[0].Tool.Run(ctx, `{"name":"beta"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.ModelContent, "# Skill: beta") || !strings.Contains(result.ModelContent, "Always use beta.") {
		t.Fatalf("direct Skill result = %q", result.ModelContent)
	}
	if len(assembly.ReadAdapters) != 1 {
		t.Fatalf("Skill reference adapters = %d, want 1", len(assembly.ReadAdapters))
	}
}

func writeAssemblySkill(t *testing.T, root, name, description, instructions string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: " + name + "\ndescription: " + description + "\n---\n\n" + instructions + "\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
