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
	for _, reference := range []string{
		"style-reference.md", "narrative-style.md", "story-director.md", "event-package.md", "rule-system.md",
		"state-system.md", "image-preset.md", "automation.md", "skill.md", "agent-profile.md",
	} {
		uri := "skill://config-manager/references/" + reference
		if !strings.Contains(text, uri) {
			t.Fatalf("config-manager root does not route %s", uri)
		}
		data, readErr := os.ReadFile(filepath.Join(root, "references", reference))
		if readErr != nil {
			t.Fatalf("read config-manager reference %s: %v", reference, readErr)
		}
		if len(strings.TrimSpace(string(data))) < 80 {
			t.Fatalf("config-manager reference %s is unexpectedly empty", reference)
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
