package interactiveapp

import (
	"fmt"
	"strings"

	"denova/internal/interactive"
)

// formatDirectorDocumentsContext keeps model-authored Markdown as Markdown.
// File boundaries carry their own source labels instead of JSON escaping the
// content and making partial edits error-prone.
func formatDirectorDocumentsContext(docs interactive.DirectorPlanDocs, infos map[string]interactive.DirectorPlanDocInfo) string {
	parts := make([]string, 0, 3)
	for _, doc := range []struct {
		name    string
		kind    string
		purpose string
		body    string
	}{
		{name: interactive.DirectorDocumentPlan, kind: interactive.DirectorPlanDocPlan, purpose: "private Director planning; never injected into the prose Agent", body: docs.Plan},
		{name: interactive.DirectorDocumentAgentBrief, kind: interactive.DirectorPlanDocAgentBrief, purpose: "prose-Agent-visible brief; routine updates patch this file only by default", body: docs.AgentBrief},
		{name: interactive.DirectorDocumentLoreContext, kind: interactive.DirectorPlanDocLoreContext, purpose: "branch lore working set; patch only when lore lifecycle changes", body: docs.LoreContext},
	} {
		body := strings.TrimSpace(doc.body)
		if body == "" {
			continue
		}
		hash := strings.TrimSpace(infos[doc.kind].Hash)
		parts = append(parts, fmt.Sprintf("## File: %s\n\n> source: %s; base_hash: `%s`; purpose: %s\n\n%s", doc.name, doc.name, hash, doc.purpose, body))
	}
	return strings.Join(parts, "\n\n---\n\n")
}
