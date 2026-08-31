package interactive

import (
	"strings"
	"testing"
)

func TestDefaultStoryPlanningTemplateCoversLongAndShortHorizons(t *testing.T) {
	template := DefaultStoryPlanningTemplateMarkdown()
	sections, err := parseBranchPlanSections(template)
	if err != nil {
		t.Fatal(err)
	}
	for _, heading := range []string{
		"Long-term direction",
		"Mid-term arcs",
		"Near-term beats",
		"Character deployment",
		"Threads and payoffs",
		"Branch possibilities",
		"Continuity and replanning",
	} {
		if _, exists := sections[normalizePlanSectionHeadingKey(heading)]; !exists {
			t.Fatalf("default planning template is missing %q", heading)
		}
	}
}

func TestEmptyCustomPlanningTemplateUsesRuntimeFallbackWithoutRewritingPreset(t *testing.T) {
	director := normalizeStoryDirector(StoryDirector{
		ID: "custom", Name: "Custom",
		Strategy: StoryDirectorStrategy{PromptMarkdown: ""},
	})
	if director.Strategy.PromptMarkdown != "" {
		t.Fatalf("normalization must not rewrite an existing custom preset: %#v", director.Strategy)
	}
	guide := StoryPlanningGuideMarkdown(director, StoryContextMaxBytes)
	if !strings.Contains(guide, "## Long-term direction") || !strings.Contains(guide, "Planning document template") {
		t.Fatalf("empty custom template should receive the runtime fallback:\n%s", guide)
	}
}
