package interactive

import (
	"fmt"
	"strings"
)

// DirectorPlanUpdateSubmission is the Director Agent's single write boundary.
// A keep decision never carries documents; patch and replan carry both complete
// documents so the backend can validate them before changing either file.
type DirectorPlanUpdateSubmission struct {
	Decision PlanDecision      `json:"decision"`
	Docs     *DirectorPlanDocs `json:"docs,omitempty"`
}

type DirectorPlanUpdateReceipt struct {
	Accepted    bool         `json:"accepted"`
	Mode        string       `json:"mode"`
	DocsUpdated bool         `json:"docs_updated"`
	Decision    PlanDecision `json:"decision"`
}

// StageDirectorPlanRunUpdate validates and stages one model-authored plan
// update while the corresponding run token still owns the branch. Metadata
// and event decisions remain finalized by CompleteDirectorPlanRun.
func (s *Store) StageDirectorPlanRunUpdate(storyID, branchID string, token DirectorPlanRunToken, sourceTurnID string, submission DirectorPlanUpdateSubmission) (DirectorPlanUpdateReceipt, error) {
	if s == nil {
		return DirectorPlanUpdateReceipt{}, fmt.Errorf("互动故事存储不可用")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	metadata, err := s.readDirectorPlanMetadataLocked(storyID, branchID)
	if err != nil {
		return DirectorPlanUpdateReceipt{}, err
	}
	if token.Revision != "" && token.Revision != metadata.Revision {
		return DirectorPlanUpdateReceipt{}, fmt.Errorf("导演规划已被其他操作更新，请重新加载后再提交")
	}
	if metadata.LastRun == nil || metadata.LastRun.Status != DirectorPlanStatusRunning || strings.TrimSpace(metadata.LastRun.SourceTurnID) != strings.TrimSpace(sourceTurnID) {
		return DirectorPlanUpdateReceipt{}, fmt.Errorf("当前导演规划运行已失效，不能提交结果")
	}

	rawMode := strings.TrimSpace(submission.Decision.Mode)
	switch rawMode {
	case PlanDecisionKeep, PlanDecisionPatch, PlanDecisionReplan:
	default:
		return DirectorPlanUpdateReceipt{}, fmt.Errorf("无效的导演规划 mode: %s", rawMode)
	}
	decision := normalizePlanDecision(submission.Decision)
	switch decision.Mode {
	case PlanDecisionKeep:
		if submission.Docs != nil {
			return DirectorPlanUpdateReceipt{}, fmt.Errorf("keep 决策不得提交规划文档")
		}
	case PlanDecisionPatch, PlanDecisionReplan:
		if submission.Docs == nil {
			return DirectorPlanUpdateReceipt{}, fmt.Errorf("%s 决策必须同时提交完整 director.md 与 lore-context.md", decision.Mode)
		}
		if err := validateDirectorPlanDocs(*submission.Docs); err != nil {
			return DirectorPlanUpdateReceipt{}, err
		}
		if err := s.validateDirectorLoreContext(submission.Docs.LoreContext); err != nil {
			return DirectorPlanUpdateReceipt{}, err
		}
		previous, err := s.readDirectorPlanDocsLocked(storyID, branchID)
		if err != nil {
			return DirectorPlanUpdateReceipt{}, err
		}
		if err := s.writeDirectorPlanDocsLocked(storyID, branchID, *submission.Docs); err != nil {
			if restoreErr := s.writeDirectorPlanDocsLocked(storyID, branchID, previous); restoreErr != nil {
				return DirectorPlanUpdateReceipt{}, fmt.Errorf("写入导演规划失败: %v；恢复旧规划也失败: %v", err, restoreErr)
			}
			return DirectorPlanUpdateReceipt{}, err
		}
	}

	return DirectorPlanUpdateReceipt{
		Accepted:    true,
		Mode:        decision.Mode,
		DocsUpdated: submission.Docs != nil,
		Decision:    decision,
	}, nil
}
