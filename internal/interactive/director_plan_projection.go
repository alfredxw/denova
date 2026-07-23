package interactive

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
)

func validateDirectorPlanDocs(docs DirectorPlanDocs) error {
	if err := validateDirectorPlanDoc(DirectorPlanDocPlan, docs.Plan); err != nil {
		return err
	}
	if err := validateDirectorPlanDoc(DirectorPlanDocAgentBrief, docs.AgentBrief); err != nil {
		return err
	}
	return validateDirectorLoreContextDoc(docs.LoreContext)
}

func validateDirectorPlanDoc(kind, content string) error {
	content = strings.TrimSpace(content)
	if content == "" {
		return fmt.Errorf("导演规划 %s 不能为空", kind)
	}
	if len([]byte(content)) > maxDirectorPlanDocBytes {
		return fmt.Errorf("导演规划 %s 超过大小上限 %d bytes", kind, maxDirectorPlanDocBytes)
	}
	headings := requiredDirectorPrivatePlanHeadings
	if kind == DirectorPlanDocAgentBrief {
		headings = requiredDirectorAgentBriefHeadings
	}
	for _, heading := range headings {
		if !strings.Contains(content, "## "+heading) {
			return fmt.Errorf("导演规划 %s 缺少必填标题: %s", kind, heading)
		}
	}
	return nil
}

func ExtractDirectorPlanVisibleSection(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	start := strings.Index(content, "## 正文Agent可读")
	if start < 0 {
		return ""
	}
	visible := content[start:]
	if end := strings.Index(visible, "## 后台导演私密"); end >= 0 {
		visible = visible[:end]
	}
	return strings.TrimSpace(trimBytes(visible, directorPlanVisibleBytes))
}

func DirectorPlanVisibleContext(plan DirectorPlan, limitBytes int) string {
	if limitBytes <= 0 || limitBytes > DirectorContextMaxBytes {
		limitBytes = DirectorContextMaxBytes
	}
	var sb strings.Builder
	writeDirectorPlanContextBlock(&sb, "正文 Agent 简报（source: agent-brief.md）", plan.VisibleDocs.AgentBrief)
	return strings.TrimSpace(trimBytes(sb.String(), limitBytes))
}

// ExtractDirectorLoreContextActiveSection keeps the human-readable active
// sections while excluding candidate and offstage casting notes from the Game
// Agent. Full lore bodies are resolved separately by the app layer.
func ExtractDirectorLoreContextActiveSection(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	var sb strings.Builder
	section := ""
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			section = strings.TrimSpace(strings.TrimPrefix(trimmed, "## "))
		}
		if activeDirectorLoreContextSections[section] {
			sb.WriteString(line)
			sb.WriteString("\n")
		}
	}
	return strings.TrimSpace(trimBytes(sb.String(), directorPlanVisibleBytes))
}

func DirectorPlanStatusFromPlan(plan DirectorPlan, hasTurns bool) DirectorPlanStatus {
	_ = hasTurns
	run := plan.Metadata.LastRun
	status := DirectorPlanStatusWaitingOpening
	if run != nil && strings.TrimSpace(run.Status) != "" {
		status = strings.TrimSpace(run.Status)
	}
	docBytes, visibleBytes := directorPlanByteTotals(plan.Metadata.Docs)
	plannedDocs := len(requiredDirectorPlanDocKinds())
	completedDocs := directorPlanCompletedDocsForStatus(status)
	startReady := status == DirectorPlanStatusReady || status == DirectorPlanStatusSkipped || status == DirectorPlanStatusConflict
	blocking := false
	summary := ""
	errorText := ""
	sourceTurnID := ""
	updatedAt := plan.Metadata.UpdatedAt
	if run != nil {
		summary = strings.TrimSpace(run.Summary)
		errorText = strings.TrimSpace(run.Error)
		sourceTurnID = strings.TrimSpace(run.SourceTurnID)
		if strings.TrimSpace(run.UpdatedAt) != "" {
			updatedAt = strings.TrimSpace(run.UpdatedAt)
		}
		if run.PlannedDocs > 0 {
			plannedDocs = run.PlannedDocs
		}
		if run.CompletedDocs > 0 || status == DirectorPlanStatusRunning || status == DirectorPlanStatusWaitingOpening || status == DirectorPlanStatusFailed {
			completedDocs = run.CompletedDocs
		}
		if run.StartReady {
			startReady = true
		}
		if status == DirectorPlanStatusRunning {
			completedDocs = directorPlanCompletedDocs(plan.Docs, run.BaselineHashes)
		}
	}
	if completedDocs > plannedDocs {
		completedDocs = plannedDocs
	}
	return DirectorPlanStatus{
		StoryID:          plan.StoryID,
		BranchID:         plan.BranchID,
		Status:           status,
		Summary:          summary,
		Error:            errorText,
		SourceTurnID:     sourceTurnID,
		UpdatedAt:        updatedAt,
		PlannedDocs:      plannedDocs,
		CompletedDocs:    completedDocs,
		DocBytes:         docBytes,
		VisibleBytes:     visibleBytes,
		StartReady:       startReady,
		Blocking:         blocking,
		Revision:         plan.Metadata.Revision,
		Decision:         runDecision(run),
		EventRuntime:     plan.Metadata.EventRuntime,
		EventOpportunity: runEventOpportunity(run),
	}
}

func directorPlanHashesEqual(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
}

func runDecision(run *DirectorPlanRunStatus) *PlanDecision {
	if run == nil || run.Decision == nil {
		return nil
	}
	decision := *run.Decision
	return &decision
}

func runEventOpportunity(run *DirectorPlanRunStatus) EventOpportunity {
	if run == nil {
		return EventOpportunity{}
	}
	return run.EventOpportunity
}

func writeDirectorPlanContextBlock(sb *strings.Builder, title, content string) {
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	sb.WriteString("## ")
	sb.WriteString(title)
	sb.WriteString("\n\n")
	sb.WriteString(content)
	sb.WriteString("\n\n")
}

func (s *Store) directorPlanBranchDir(storyID, branchID string) string {
	return filepath.Join(s.root, "interactive", "stories", storyID, "director", branchID)
}

func directorPlanDocInfos(dir string, docs DirectorPlanDocs) map[string]DirectorPlanDocInfo {
	return map[string]DirectorPlanDocInfo{
		DirectorPlanDocPlan:        directorPlanDocInfo(filepath.Join(dir, directorPlanFile), docs.Plan, ""),
		DirectorPlanDocAgentBrief:  directorPlanDocInfo(filepath.Join(dir, directorAgentBriefFile), docs.AgentBrief, docs.AgentBrief),
		DirectorPlanDocLoreContext: directorPlanDocInfo(filepath.Join(dir, directorLoreContextFile), docs.LoreContext, ExtractDirectorLoreContextActiveSection(docs.LoreContext)),
	}
}

func directorPlanDocInfo(path, content, visible string) DirectorPlanDocInfo {
	return DirectorPlanDocInfo{Path: filepath.ToSlash(path), Bytes: len([]byte(content)), Hash: textHash(content), VisibleBytes: len([]byte(visible))}
}

func directorPlanHashes(docs DirectorPlanDocs) map[string]string {
	return map[string]string{
		DirectorPlanDocPlan:        textHash(docs.Plan),
		DirectorPlanDocAgentBrief:  textHash(docs.AgentBrief),
		DirectorPlanDocLoreContext: textHash(docs.LoreContext),
	}
}

func directorPlanRevision(docs DirectorPlanDocs, updatedAt string) string {
	return textHash(strings.Join([]string{docs.Plan, docs.AgentBrief, docs.LoreContext, updatedAt}, "\n---director-plan---\n"))
}

func requiredDirectorPlanDocKinds() []string {
	return []string{DirectorPlanDocPlan, DirectorPlanDocAgentBrief, DirectorPlanDocLoreContext}
}

func directorPlanRunStartReady(run *DirectorPlanRunStatus) bool {
	if run == nil {
		return false
	}
	if run.StartReady {
		return true
	}
	switch run.Status {
	case DirectorPlanStatusReady, DirectorPlanStatusSkipped, DirectorPlanStatusConflict:
		return true
	default:
		return false
	}
}

func directorPlanCompletedDocsForStatus(status string) int {
	switch status {
	case DirectorPlanStatusReady, DirectorPlanStatusSkipped, DirectorPlanStatusConflict:
		return len(requiredDirectorPlanDocKinds())
	default:
		return 0
	}
}

func directorPlanCompletedDocs(docs DirectorPlanDocs, baseline map[string]string) int {
	if len(baseline) == 0 {
		return 0
	}
	current := directorPlanHashes(docs)
	completed := 0
	for _, kind := range requiredDirectorPlanDocKinds() {
		if baseline[kind] != "" && current[kind] != "" && baseline[kind] != current[kind] {
			completed++
		}
	}
	return completed
}

func directorPlanByteTotals(infos map[string]DirectorPlanDocInfo) (int, int) {
	docBytes := 0
	visibleBytes := 0
	for _, info := range infos {
		docBytes += info.Bytes
		visibleBytes += info.VisibleBytes
	}
	return docBytes, visibleBytes
}

func textHash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:12])
}
