package skills

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestResolveExplicitInvocationsScansWholeMessageInFirstOccurrenceOrder(t *testing.T) {
	root := t.TempDir()
	writeSkillFileForAgents(t, root, "alpha-dir", "alpha", "Alpha", "ide")
	writeSkillFileForAgents(t, root, "beta-dir", "beta", "Beta", "ide")
	writeSkillFileForAgents(t, root, "game-dir", "game-only", "Game", "interactive_story")
	backend := NewAgentBackend([]Directory{{Scope: ScopeUser, Path: root}}, "ide", nil)

	resolved := backend.ResolveExplicitInvocations(
		context.Background(),
		"路径 docs/alpha 和 https://example.com/beta 不算；中文句中可用/alpha，然后 /beta，重复 /alpha，未知 /missing，子路径 /beta/file 不算。",
	)
	if len(resolved) != 2 || resolved[0].Name != "alpha" || resolved[1].Name != "beta" {
		t.Fatalf("resolved skills = %#v", resolved)
	}
}

func TestResolveExplicitInvocationsRequiresExactActiveSkillName(t *testing.T) {
	root := t.TempDir()
	writeSkillFileForAgents(t, root, "short-dir", "alpha", "Alpha", "ide")
	writeSkillFileForAgents(t, root, "long-dir", "alpha-plus", "Alpha plus", "ide")
	backend := NewAgentBackend([]Directory{{Scope: ScopeUser, Path: root}}, "ide", map[string]bool{"alpha-plus": false})

	resolved := backend.ResolveExplicitInvocations(context.Background(), "/alpha-plus /alpha_suffix /alpha")
	if len(resolved) != 1 || resolved[0].Name != "alpha" {
		t.Fatalf("resolved skills = %#v", resolved)
	}
}

func TestFormatForModelBoundsTheCompleteUTF8Result(t *testing.T) {
	skill := Skill{FrontMatter: FrontMatter{Name: "alpha", Description: "Alpha"}, Content: strings.Repeat("界", 100)}
	formatted := FormatForModel(skill, 180)
	if len(formatted) > 180 {
		t.Fatalf("formatted bytes = %d, want <= 180", len(formatted))
	}
	if !utf8.ValidString(formatted) {
		t.Fatalf("formatted Skill is not valid UTF-8: %q", formatted)
	}
	if !strings.HasPrefix(formatted, "# Skill: alpha") || !strings.Contains(formatted, "Reference root: skill://alpha/references/") || !strings.Contains(formatted, "instructions truncated") {
		t.Fatalf("formatted Skill = %q", formatted)
	}
}
