package skills

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuiltinConfigManagerSkillRoutesEveryRegisteredResourceReference(t *testing.T) {
	builtinRoot := filepath.Join("..", "..", "..", "skills")
	root := filepath.Join(builtinRoot, "config-manager")
	content, err := os.ReadFile(filepath.Join(root, SkillFileName))
	if err != nil {
		t.Fatalf("read config-manager skill: %v", err)
	}
	text := string(content)
	for _, required := range []string{"## Mutation semantics", "Complete editable-resource replacement", "Sparse patch", "Sectional layered update", "REVISION_FROM_GET"} {
		if !strings.Contains(text, required) {
			t.Fatalf("config-manager root is missing mutation contract %q", required)
		}
	}
	references := map[string][]string{
		"style-reference.md": {"## Field reference", "## Create example", "## Update example", "160 KiB"},
		"narrative-style.md": {"## Field reference", "## Create example", "## Complete update example", "`turn_context`"},
		"story-director.md":  {"### `module_refs`", "### `strategy`", "## Create example", "`branch_planning_turns`"},
		"event-package.md":   {"## Field reference", "## Create example", "## Complete update example", "8,000 characters"},
		"rule-system.md":     {"## Rule template fields", "## State Binding fields", "## Basic create example", "## State Binding create example"},
		"state-system.md":    {"## Template and field reference", "## Initial Actors", "## Trait pools and rules", "all six field types"},
		"image-preset.md":    {"## Field reference", "`agent_system`", "`tool_request`", "## Complete update example"},
		"automation.md":      {"## Editable fields", "## Schedule fields", "## Trigger fields", "## Sparse update example"},
		"skill.md":           {"## Identity, scope, and revision", "## Root create values", "## Supporting reference lifecycle", "512 KiB"},
		"agent-profile.md":   {"## Kinds and operations", "## Fixed Agent sections", "## Custom SubAgent fields", "## Verification"},
	}
	for reference, requiredFragments := range references {
		uri := "skill://config-manager/references/" + reference
		if !strings.Contains(text, uri) {
			t.Fatalf("config-manager root does not route %s", uri)
		}
		data, readErr := os.ReadFile(filepath.Join(root, "references", reference))
		if readErr != nil {
			t.Fatalf("read config-manager reference %s: %v", reference, readErr)
		}
		referenceText := string(data)
		if len(strings.TrimSpace(referenceText)) < 1000 {
			t.Fatalf("config-manager reference %s is unexpectedly shallow", reference)
		}
		for _, fragment := range requiredFragments {
			if !strings.Contains(referenceText, fragment) {
				t.Fatalf("config-manager reference %s is missing contract fragment %q", reference, fragment)
			}
		}
	}
	for _, obsolete := range []string{"agent-config", "automation-config", "image-preset-config", "story-director-config", "teller-config"} {
		if _, err := os.Stat(filepath.Join(builtinRoot, obsolete, SkillFileName)); !os.IsNotExist(err) {
			t.Fatalf("obsolete config Skill %s still exists", obsolete)
		}
	}

	backend := NewAgentBackend([]Directory{{Scope: ScopeBuiltin, Path: builtinRoot}}, "config_manager", nil)
	reference, err := backend.ReadReference(context.Background(), "skill://config-manager/references/automation.md", 1, 20)
	if err != nil {
		t.Fatalf("read built-in config-manager reference through Backend: %v", err)
	}
	if reference.URI != "skill://config-manager/references/automation.md" || !strings.Contains(reference.Content, "Automation") {
		t.Fatalf("unexpected built-in reference: %+v", reference)
	}
}
