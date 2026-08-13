package prompts

import (
	"strings"
	"testing"
)

func TestRenderSkillsCatalogIncludesEveryDescriptionInStableOrder(t *testing.T) {
	content, report, err := renderSkillsCatalog([]SkillCatalogEntry{
		{Name: "zeta", Description: "Route late-stage checks."},
		{Name: "alpha", Description: "Route <draft>\nreview & revision."},
	}, 64*1024)
	if err != nil {
		t.Fatal(err)
	}
	if report.IncludedCount != 2 || report.OmittedCount != 0 || report.TruncatedDescriptionCount != 0 {
		t.Fatalf("unexpected render report: %#v", report)
	}
	alpha := "- alpha: Route &lt;draft&gt; review &amp; revision."
	zeta := "- zeta: Route late-stage checks."
	if !strings.Contains(content, alpha) || !strings.Contains(content, zeta) {
		t.Fatalf("catalog omitted or failed to escape descriptions:\n%s", content)
	}
	if strings.Index(content, alpha) >= strings.Index(content, zeta) {
		t.Fatalf("catalog order is not stable by name:\n%s", content)
	}
	for _, required := range []string{
		"<skills_instructions>",
		"Descriptions are routing metadata only",
		"the task clearly matches a Skill description above, you must use that Skill",
		"call the `skill` tool with its exact catalog name",
		"may already be loaded in context; do not call the tool again",
		"</skills_instructions>",
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("catalog prompt missing %q:\n%s", required, content)
		}
	}
}

func TestRenderSkillsCatalogSharesConstrainedDescriptionBudget(t *testing.T) {
	entries := []SkillCatalogEntry{
		{Name: "alpha", Description: strings.Repeat("a", 200)},
		{Name: "beta", Description: strings.Repeat("b", 200)},
	}
	normalized := normalizeSkillCatalogEntries(entries)
	minimum := len(skillsCatalogHeader) + len(skillsCatalogFooter)
	for _, entry := range normalized {
		minimum += skillCatalogLineBytes(entry, 0)
	}
	content, report, err := renderSkillsCatalog(entries, minimum+40)
	if err != nil {
		t.Fatal(err)
	}
	if len(content) > minimum+40 {
		t.Fatalf("catalog bytes = %d, limit = %d", len(content), minimum+40)
	}
	if report.IncludedCount != 2 || report.OmittedCount != 0 || report.TruncatedDescriptionCount != 2 {
		t.Fatalf("unexpected constrained render report: %#v", report)
	}
	if !strings.Contains(content, "- alpha: a") || !strings.Contains(content, "- beta: b") {
		t.Fatalf("description budget was not shared across every Skill:\n%s", content)
	}
}

func TestRenderSkillsCatalogCapsOneDescriptionWithoutDroppingLaterSkills(t *testing.T) {
	content, report, err := renderSkillsCatalog([]SkillCatalogEntry{
		{Name: "alpha", Description: strings.Repeat("a", maxSkillCatalogDescriptionChars+100)},
		{Name: "beta", Description: "Beta routing description."},
	}, 64*1024)
	if err != nil {
		t.Fatal(err)
	}
	if report.IncludedCount != 2 || report.OmittedCount != 0 || report.TruncatedDescriptionCount != 1 {
		t.Fatalf("unexpected capped render report: %#v", report)
	}
	if !strings.Contains(content, strings.Repeat("a", maxSkillCatalogDescriptionChars)+skillCatalogTruncationSuffix) ||
		!strings.Contains(content, "- beta: Beta routing description.") {
		t.Fatalf("per-description cap did not preserve the full catalog:\n%s", content)
	}
}
