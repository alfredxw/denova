package interactive

import (
	"strings"
	"testing"
)

func TestBranchPlanSectionReplacementIgnoresHeadingsInsideFences(t *testing.T) {
	current := "## Long-term direction\n\nOld.\n\n```markdown\n## Example only\n```\n\n## Near-term beats\n\nOld near term."
	draft, accepted, updateErrors, err := applyBranchPlanSectionUpdates(current, []TurnPlanSectionUpdate{{
		Heading:  "Long-term direction",
		Markdown: "New.\n\n### Constraint\n\nKeep this example:\n\n```markdown\n## Not a module\n```",
	}})
	if err != nil || len(updateErrors) != 0 || len(accepted) != 1 {
		t.Fatalf("fenced examples and H3 content should remain valid section bodies: accepted=%#v errors=%#v err=%v", accepted, updateErrors, err)
	}
	if !strings.Contains(draft, "## Not a module") || !strings.Contains(draft, "## Near-term beats\n\nOld near term.") {
		t.Fatalf("section replacement damaged fenced or sibling content:\n%s", draft)
	}
}

func TestBranchPlanSectionReplacementRejectsAmbiguousCurrentHeadings(t *testing.T) {
	current := "## Near-term beats\n\nFirst.\n\n## near-term beats\n\nSecond."
	_, _, _, err := applyBranchPlanSectionUpdates(current, []TurnPlanSectionUpdate{{Heading: "Near-term beats", Markdown: "New."}})
	if err == nil || !strings.Contains(err.Error(), "duplicate H2") {
		t.Fatalf("case-insensitive duplicate headings should require full repair, got %v", err)
	}
}
