package context

import (
	"strings"
	"testing"
)

func TestSourceSummaryBoundsPreview(t *testing.T) {
	summary := SourceSummary([]Source{{
		Source:    "文件引用",
		Title:     "@chapter.md",
		Content:   "abcdefghijklmnopqrstuvwxyz",
		Placement: PlacementAuditOnly,
		Included:  true,
		Truncated: true,
		Limit:     10,
	}}, 6)
	for _, want := range []string{`source="文件引用"`, `title="@chapter.md"`, `preview="abcdef..."`, `placement="audit_only"`, "hash=", "truncated=true", "limit=10"} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary missing %q: %s", want, summary)
		}
	}
}
