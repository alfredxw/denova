package skills

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuiltinWebResearchSkillDefinesVerifiedResearchWorkflow(t *testing.T) {
	builtin := filepath.Join("..", "..", "..", "skills")
	backend := NewAgentBackend([]Directory{{Scope: ScopeBuiltin, Path: builtin}}, "ide", nil)

	skill, err := backend.Get(context.Background(), "web-research")
	if err != nil {
		t.Fatalf("Get(web-research) error = %v", err)
	}
	if skill.Agent != "" {
		t.Fatalf("web-research should be shared by Skills-enabled agents, agent = %q", skill.Agent)
	}
	for _, required := range []string{
		"web_search",
		"web_fetch",
		"2–4",
		"warnings",
		"search snippets",
		"untrusted",
		"primary sources",
		"independent sources",
		"actual source URL",
	} {
		if !strings.Contains(skill.Content, required) {
			t.Fatalf("web-research missing required instruction %q", required)
		}
	}
}
