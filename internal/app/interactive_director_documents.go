package app

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
		{name: interactive.DirectorDocumentPlan, kind: interactive.DirectorPlanDocPlan, purpose: "Director 私密规划，不注入正文 Agent", body: docs.Plan},
		{name: interactive.DirectorDocumentAgentBrief, kind: interactive.DirectorPlanDocAgentBrief, purpose: "正文 Agent 可见简报；普通更新默认只 Patch 本文件", body: docs.AgentBrief},
		{name: interactive.DirectorDocumentLoreContext, kind: interactive.DirectorPlanDocLoreContext, purpose: "分支资料工作集，仅在资料生命周期变化时 Patch", body: docs.LoreContext},
	} {
		body := strings.TrimSpace(doc.body)
		if body == "" {
			continue
		}
		hash := strings.TrimSpace(infos[doc.kind].Hash)
		parts = append(parts, fmt.Sprintf("## 文件：%s\n\n> source: %s；base_hash: `%s`；用途：%s\n\n%s", doc.name, doc.name, hash, doc.purpose, body))
	}
	return strings.Join(parts, "\n\n---\n\n")
}
