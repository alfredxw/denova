package interactive

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	DirectorDocumentPlan        = "director.md"
	DirectorDocumentAgentBrief  = "agent-brief.md"
	DirectorDocumentLoreContext = "lore-context.md"

	DirectorMarkdownEditReplaceSection  = "replace_section"
	DirectorMarkdownEditReplaceText     = "replace_text"
	DirectorMarkdownEditReplaceDocument = "replace_document"

	maxDirectorDocumentUpdatesPerCall = 3
	maxDirectorMarkdownEditsPerDoc    = 32
)

// DirectorPlanDocumentUpdate patches exactly one Markdown document against
// the run's immutable baseline hash. Accepted documents are retained in the
// run-local draft and must not be resubmitted on later retries.
type DirectorPlanDocumentUpdate struct {
	Document string                 `json:"document" jsonschema:"enum=director.md,enum=agent-brief.md,enum=lore-context.md" jsonschema_description:"要修改的导演 Markdown 文件"`
	BaseHash string                 `json:"base_hash" jsonschema_description:"逐字复制上下文中该文件的 base_hash"`
	Edits    []DirectorMarkdownEdit `json:"edits" jsonschema_description:"基于当前完整快照的最小编辑；固定标题文档优先 replace_section"`
}

// DirectorMarkdownEdit uses stable Markdown headings or exact text instead of
// line numbers. replace_document is reserved for opening initialization,
// explicit rebuilds, or a genuine replan that cannot be expressed safely as
// section edits.
type DirectorMarkdownEdit struct {
	Op      string `json:"op" jsonschema:"enum=replace_section,enum=replace_text,enum=replace_document"`
	Heading string `json:"heading,omitempty" jsonschema_description:"replace_section 的二级标题文本，不含 ##"`
	OldText string `json:"old_text,omitempty" jsonschema_description:"replace_text 要精确且唯一匹配的原文"`
	Content string `json:"content" jsonschema_description:"替换后的 section 正文、精确文本或完整文档"`
}

type directorPlanAcceptedDocument struct {
	content     string
	hash        string
	fingerprint string
}

// DirectorPlanUpdateDraft is owned by one Director Agent run. It keeps
// independently accepted document patches in memory; no workspace file is
// changed until the draft is finalized and the app commits it atomically.
type DirectorPlanUpdateDraft struct {
	baseline  DirectorPlanDocs
	baseHash  map[string]string
	accepted  map[string]directorPlanAcceptedDocument
	decision  *PlanDecision
	finalized bool
}

func NewDirectorPlanUpdateDraft(docs DirectorPlanDocs, token DirectorPlanRunToken) *DirectorPlanUpdateDraft {
	return &DirectorPlanUpdateDraft{
		baseline: docs,
		baseHash: map[string]string{
			DirectorDocumentPlan:        firstNonEmpty(token.Hashes[DirectorPlanDocPlan], textHash(docs.Plan)),
			DirectorDocumentAgentBrief:  firstNonEmpty(token.Hashes[DirectorPlanDocAgentBrief], textHash(docs.AgentBrief)),
			DirectorDocumentLoreContext: firstNonEmpty(token.Hashes[DirectorPlanDocLoreContext], textHash(docs.LoreContext)),
		},
		accepted: map[string]directorPlanAcceptedDocument{},
	}
}

func (d *DirectorPlanUpdateDraft) FinalDocs() (DirectorPlanDocs, bool) {
	if d == nil || !d.finalized {
		return DirectorPlanDocs{}, false
	}
	return d.currentDocs(), true
}

func (d *DirectorPlanUpdateDraft) Decision() (PlanDecision, bool) {
	if d == nil || d.decision == nil {
		return PlanDecision{}, false
	}
	return *d.decision, true
}

func (d *DirectorPlanUpdateDraft) currentDocs() DirectorPlanDocs {
	docs := d.baseline
	for document, accepted := range d.accepted {
		setDirectorDocumentContent(&docs, document, accepted.content)
	}
	return docs
}

func (d *DirectorPlanUpdateDraft) acceptedDocuments() []string {
	result := make([]string, 0, len(d.accepted))
	for _, document := range directorDocumentOrder() {
		if _, ok := d.accepted[document]; ok {
			result = append(result, document)
		}
	}
	return result
}

func directorDocumentOrder() []string {
	return []string{DirectorDocumentPlan, DirectorDocumentAgentBrief, DirectorDocumentLoreContext}
}

func normalizeDirectorDocument(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case DirectorDocumentPlan, DirectorPlanDocPlan:
		return DirectorDocumentPlan
	case DirectorDocumentAgentBrief, DirectorPlanDocAgentBrief:
		return DirectorDocumentAgentBrief
	case DirectorDocumentLoreContext, DirectorPlanDocLoreContext:
		return DirectorDocumentLoreContext
	default:
		return ""
	}
}

func directorDocumentKind(document string) string {
	switch normalizeDirectorDocument(document) {
	case DirectorDocumentPlan:
		return DirectorPlanDocPlan
	case DirectorDocumentAgentBrief:
		return DirectorPlanDocAgentBrief
	case DirectorDocumentLoreContext:
		return DirectorPlanDocLoreContext
	default:
		return ""
	}
}

func directorDocumentContent(docs DirectorPlanDocs, document string) string {
	switch normalizeDirectorDocument(document) {
	case DirectorDocumentPlan:
		return docs.Plan
	case DirectorDocumentAgentBrief:
		return docs.AgentBrief
	case DirectorDocumentLoreContext:
		return docs.LoreContext
	default:
		return ""
	}
}

func setDirectorDocumentContent(docs *DirectorPlanDocs, document, content string) {
	if docs == nil {
		return
	}
	content = strings.TrimSpace(content)
	switch normalizeDirectorDocument(document) {
	case DirectorDocumentPlan:
		docs.Plan = content
	case DirectorDocumentAgentBrief:
		docs.AgentBrief = content
	case DirectorDocumentLoreContext:
		docs.LoreContext = content
	}
}

func directorDocumentUpdateFingerprint(update DirectorPlanDocumentUpdate) string {
	data, _ := json.Marshal(update)
	return textHash(string(data))
}

func applyDirectorDocumentUpdate(base string, update DirectorPlanDocumentUpdate) (string, error) {
	if len(update.Edits) == 0 {
		return "", fmt.Errorf("edits 不能为空")
	}
	if len(update.Edits) > maxDirectorMarkdownEditsPerDoc {
		return "", fmt.Errorf("edits 过多: %d > %d", len(update.Edits), maxDirectorMarkdownEditsPerDoc)
	}
	content := strings.TrimSpace(base)
	for index, edit := range update.Edits {
		next, err := applyDirectorMarkdownEdit(content, edit)
		if err != nil {
			return "", fmt.Errorf("edits[%d]: %w", index, err)
		}
		content = next
	}
	return strings.TrimSpace(content), nil
}

func applyDirectorMarkdownEdit(document string, edit DirectorMarkdownEdit) (string, error) {
	switch strings.TrimSpace(edit.Op) {
	case DirectorMarkdownEditReplaceSection:
		return replaceDirectorMarkdownSection(document, edit.Heading, edit.Content)
	case DirectorMarkdownEditReplaceText:
		oldText := strings.TrimSpace(edit.OldText)
		if oldText == "" {
			return "", fmt.Errorf("replace_text.old_text 不能为空")
		}
		if count := strings.Count(document, oldText); count != 1 {
			return "", fmt.Errorf("replace_text.old_text 必须精确匹配一次，实际 %d 次", count)
		}
		return strings.Replace(document, oldText, strings.TrimSpace(edit.Content), 1), nil
	case DirectorMarkdownEditReplaceDocument:
		if strings.TrimSpace(edit.Content) == "" {
			return "", fmt.Errorf("replace_document.content 不能为空")
		}
		return strings.TrimSpace(edit.Content), nil
	default:
		return "", fmt.Errorf("未知编辑 op: %s", strings.TrimSpace(edit.Op))
	}
}

func replaceDirectorMarkdownSection(document, heading, body string) (string, error) {
	heading = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(heading), "##"))
	if heading == "" {
		return "", fmt.Errorf("replace_section.heading 不能为空")
	}
	lines := strings.Split(strings.ReplaceAll(document, "\r\n", "\n"), "\n")
	start := -1
	end := len(lines)
	wanted := "## " + heading
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == wanted {
			if start >= 0 {
				return "", fmt.Errorf("二级标题重复: %s", heading)
			}
			start = index
			continue
		}
		if start >= 0 && strings.HasPrefix(trimmed, "## ") {
			end = index
			break
		}
	}
	if start < 0 {
		return "", fmt.Errorf("找不到二级标题: %s", heading)
	}
	replacement := []string{lines[start]}
	if body = strings.TrimSpace(body); body != "" {
		replacement = append(replacement, "")
		replacement = append(replacement, strings.Split(body, "\n")...)
	}
	if end < len(lines) {
		replacement = append(replacement, "")
	}
	result := make([]string, 0, len(lines)-(end-start)+len(replacement))
	result = append(result, lines[:start]...)
	result = append(result, replacement...)
	result = append(result, lines[end:]...)
	return strings.TrimSpace(strings.Join(result, "\n")), nil
}
