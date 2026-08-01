package interactive

import (
	"denova/internal/book/lore"
	"denova/internal/interactive/director"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (s *Store) DirectorPlan(storyID, branchID string) (DirectorPlan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	meta, snapshot, err := s.boundedStorySnapshotWithLimitLocked(storyID, branchID, maxStoryHistoryPageTurns)
	if err != nil {
		return DirectorPlan{}, err
	}
	branchID, _, err = resolveBranch(meta, branchID)
	if err != nil {
		return DirectorPlan{}, err
	}
	plan, err := s.readDirectorPlanLocked(storyID, branchID)
	if err != nil {
		return DirectorPlan{}, err
	}
	plan.Metadata.EventRuntime = reconcileDirectorEventRuntime(plan.Metadata.EventRuntime, snapshot.Turns)
	return plan, nil
}

func (s *Store) DirectorPlanStatus(storyID, branchID string) (DirectorPlanStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	meta, snapshot, err := s.boundedStorySnapshotWithLimitLocked(storyID, branchID, maxStoryHistoryPageTurns)
	if err != nil {
		return DirectorPlanStatus{}, err
	}
	branchID, _, err = resolveBranch(meta, branchID)
	if err != nil {
		return DirectorPlanStatus{}, err
	}
	plan, err := s.readDirectorPlanLocked(storyID, branchID)
	if err != nil {
		return DirectorPlanStatus{}, err
	}
	plan.Metadata.EventRuntime = reconcileDirectorEventRuntime(plan.Metadata.EventRuntime, snapshot.Turns)
	return DirectorPlanStatusFromPlan(plan, snapshot.TurnCount > 0), nil
}

func (s *Store) UpdateDirectorPlan(storyID string, req UpdateDirectorPlanRequest) (DirectorPlan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	meta, snapshot, err := s.boundedStorySnapshotWithLimitLocked(storyID, req.BranchID, maxStoryHistoryPageTurns)
	if err != nil {
		return DirectorPlan{}, err
	}
	branchID, _, err := resolveBranch(meta, req.BranchID)
	if err != nil {
		return DirectorPlan{}, err
	}
	current, err := s.readDirectorPlanLocked(storyID, branchID)
	if err != nil {
		return DirectorPlan{}, err
	}
	current.Metadata.EventRuntime = reconcileDirectorEventRuntime(current.Metadata.EventRuntime, snapshot.Turns)
	if base := strings.TrimSpace(req.BaseRevision); base != "" && base != current.Metadata.Revision {
		return DirectorPlan{}, fmt.Errorf("导演规划已被其他操作更新，请重新加载后再保存")
	}
	if err := validateDirectorPlanDocs(req.Docs); err != nil {
		return DirectorPlan{}, err
	}
	if err := s.validateDirectorLoreContext(req.Docs.LoreContext); err != nil {
		return DirectorPlan{}, err
	}
	if err := s.writeDirectorPlanDocsLocked(storyID, branchID, req.Docs); err != nil {
		return DirectorPlan{}, err
	}
	metadata := s.buildDirectorPlanMetadataLocked(storyID, branchID, NormalizeBranchPlanningTurns(current.Metadata.BranchPlanningTurns), strings.TrimSpace(req.Source), "")
	metadata.EventRuntime = current.Metadata.EventRuntime
	metadata.LoreRevision = current.Metadata.LoreRevision
	metadata.DerivedThroughTurnID = current.Metadata.DerivedThroughTurnID
	metadata.DerivedAt = current.Metadata.DerivedAt
	metadata.LastRun = &DirectorPlanRunStatus{
		Status:        director.PlanStatusReady,
		Summary:       firstNonEmpty(strings.TrimSpace(req.Summary), "导演规划已手动更新。"),
		UpdatedAt:     metadata.UpdatedAt,
		PlannedDocs:   len(requiredDirectorPlanDocKinds()),
		CompletedDocs: len(requiredDirectorPlanDocKinds()),
		StartReady:    true,
		Blocking:      false,
	}
	if err := s.writeDirectorPlanMetadataLocked(storyID, branchID, metadata); err != nil {
		return DirectorPlan{}, err
	}
	return s.readDirectorPlanLocked(storyID, branchID)
}

func (s *Store) RebuildDirectorPlan(storyID string, req RebuildDirectorPlanRequest, seed DirectorPlanSeed) (DirectorPlan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	meta, snapshot, err := s.boundedStorySnapshotWithLimitLocked(storyID, req.BranchID, maxStoryHistoryPageTurns)
	if err != nil {
		return DirectorPlan{}, err
	}
	branchID, _, err := resolveBranch(meta, req.BranchID)
	if err != nil {
		return DirectorPlan{}, err
	}
	previous, _ := s.readDirectorPlanMetadataLocked(storyID, branchID)
	if err := s.seedDirectorPlanLocked(storyID, branchID, meta, seed); err != nil {
		return DirectorPlan{}, err
	}
	plan, err := s.readDirectorPlanLocked(storyID, branchID)
	if err != nil {
		return DirectorPlan{}, err
	}
	if !req.ResetEvents {
		plan.Metadata.EventRuntime = reconcileDirectorEventRuntime(previous.EventRuntime, snapshot.Turns)
	}
	plan.Metadata.LastRun = &DirectorPlanRunStatus{
		Status:        director.PlanStatusReady,
		Summary:       "导演规划已重建。",
		UpdatedAt:     time.Now().UTC().Format(time.RFC3339Nano),
		PlannedDocs:   len(requiredDirectorPlanDocKinds()),
		CompletedDocs: len(requiredDirectorPlanDocKinds()),
		StartReady:    true,
		Blocking:      false,
	}
	if err := s.writeDirectorPlanMetadataLocked(storyID, branchID, plan.Metadata); err != nil {
		return DirectorPlan{}, err
	}
	return s.readDirectorPlanLocked(storyID, branchID)
}

func (s *Store) DirectorPlanRunToken(storyID, branchID string) (DirectorPlanRunToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	meta, _, err := s.boundedStorySnapshotWithLimitLocked(storyID, branchID, 1)
	if err != nil {
		return DirectorPlanRunToken{}, err
	}
	branchID, _, err = resolveBranch(meta, branchID)
	if err != nil {
		return DirectorPlanRunToken{}, err
	}
	plan, err := s.readDirectorPlanLocked(storyID, branchID)
	if err != nil {
		return DirectorPlanRunToken{}, err
	}
	return DirectorPlanRunToken{StoryID: storyID, BranchID: branchID, Revision: plan.Metadata.Revision, Hashes: directorPlanHashes(plan.Docs)}, nil
}

func (s *Store) MarkDirectorPlanRunStarted(storyID, branchID string, token DirectorPlanRunToken, sourceTurnID string, forceEventEvaluation ...bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	meta, snapshot, err := s.boundedStorySnapshotWithLimitLocked(storyID, branchID, maxStoryHistoryPageTurns)
	if err != nil {
		return err
	}
	metadata, err := s.readDirectorPlanMetadataLocked(storyID, branchID)
	if err != nil {
		return err
	}
	// The three Markdown documents can be changed by safe external updates
	// such as a lore-name rename. Synchronize the persisted revision to the
	// run token before claiming this run; API edits during the run still update
	// metadata and therefore retain the existing conflict protection.
	metadata.Revision = token.Revision
	previous := metadata.LastRun
	startReady := directorPlanRunStartReady(previous)
	storyDirector := s.storyDirectorForMeta(meta)
	catalog := DirectorEventCatalogFromStoryDirector(storyDirector)
	turns := directorEventTurnsThrough(snapshot.Turns, sourceTurnID)
	metadata.EventRuntime = reconcileDirectorEventRuntime(metadata.EventRuntime, turns)
	forced := len(forceEventEvaluation) > 0 && forceEventEvaluation[0]
	opportunity := directorEventOpportunity(metadata.EventRuntime, turns, storyDirector.Strategy.EventFrequency, len(catalog) > 0, forced)
	metadata.LastRun = &DirectorPlanRunStatus{
		Status:           director.PlanStatusRunning,
		Summary:          "后台导演正在规划故事。",
		SourceTurnID:     sourceTurnID,
		UpdatedAt:        time.Now().UTC().Format(time.RFC3339Nano),
		PlannedDocs:      len(requiredDirectorPlanDocKinds()),
		CompletedDocs:    0,
		StartReady:       startReady,
		Blocking:         false,
		BaselineHashes:   token.Hashes,
		EventOpportunity: opportunity,
	}
	return s.writeDirectorPlanMetadataLocked(storyID, branchID, metadata)
}

func (s *Store) CompleteDirectorPlanRun(storyID, branchID string, token DirectorPlanRunToken, sourceTurnID, summary string) (DirectorPlan, error) {
	return s.completeDirectorPlanRun(storyID, branchID, token, sourceTurnID, summary, nil)
}

// CompleteDirectorPlanRunWithDocs publishes a finalized run-local Patch draft.
// The three Markdown files remain unchanged while individual documents are
// retried; they are written together only after the draft has finalized.
func (s *Store) CompleteDirectorPlanRunWithDocs(storyID, branchID string, token DirectorPlanRunToken, sourceTurnID, summary string, docs DirectorPlanDocs) (DirectorPlan, error) {
	return s.completeDirectorPlanRun(storyID, branchID, token, sourceTurnID, summary, &docs)
}

func (s *Store) completeDirectorPlanRun(storyID, branchID string, token DirectorPlanRunToken, sourceTurnID, summary string, stagedDocs *DirectorPlanDocs, domainCommits ...*DirectorPlanDomainCommitReceipt) (DirectorPlan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var domainCommit *DirectorPlanDomainCommitReceipt
	if len(domainCommits) > 0 {
		domainCommit = domainCommits[0]
	}
	storedMetadata, err := s.readDirectorPlanMetadataLocked(storyID, branchID)
	if err != nil {
		return DirectorPlan{}, err
	}
	plan, err := s.readDirectorPlanLocked(storyID, branchID)
	if err != nil {
		return DirectorPlan{}, err
	}
	if domainCommit != nil && storedMetadata.LastRun != nil {
		replayed, matchErr := matchDirectorPlanDomainCommit(storedMetadata.LastRun.DomainCommit, domainCommit)
		if matchErr != nil {
			return DirectorPlan{}, matchErr
		}
		if replayed {
			return plan, nil
		}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if token.Revision != "" && token.Revision != storedMetadata.Revision {
		storedMetadata.LastRun = &DirectorPlanRunStatus{
			Status:        director.PlanStatusConflict,
			Summary:       "后台导演运行期间规划已被手动修改，已跳过覆盖。",
			SourceTurnID:  sourceTurnID,
			UpdatedAt:     now,
			PlannedDocs:   len(requiredDirectorPlanDocKinds()),
			CompletedDocs: len(requiredDirectorPlanDocKinds()),
			StartReady:    true,
			Blocking:      false,
		}
		attachDirectorPlanDomainCommit(storedMetadata.LastRun, domainCommit, storedMetadata.Revision)
		if err := s.writeDirectorPlanMetadataLocked(storyID, branchID, storedMetadata); err != nil {
			return DirectorPlan{}, err
		}
		return s.readDirectorPlanLocked(storyID, branchID)
	}
	if storedMetadata.LastRun != nil && storedMetadata.LastRun.SourceTurnID != "" && storedMetadata.LastRun.SourceTurnID != sourceTurnID {
		// A newer Director run already owns the branch status. An older completion
		// must not replace its status or replay event decisions against stale turns.
		if domainCommit != nil {
			return DirectorPlan{}, fmt.Errorf("%w: a newer Director run owns branch %q", ErrDirectorPlanDomainCommitConflict, branchID)
		}
		return s.readDirectorPlanLocked(storyID, branchID)
	}
	decision, err := director.ParseDecisionJSON(summary)
	if err != nil {
		return DirectorPlan{}, fmt.Errorf("导演规划决策格式无效: %w", err)
	}
	decision.BaseRevision = token.Revision
	publishedDocs := plan.Docs
	if stagedDocs != nil {
		if !directorPlanHashesEqual(token.Hashes, directorPlanHashes(publishedDocs)) {
			return DirectorPlan{}, fmt.Errorf("导演规划文件在 Patch 草稿期间发生变化，拒绝覆盖")
		}
		plan.Docs = *stagedDocs
	}
	if err := validateDirectorPlanDocs(plan.Docs); err != nil {
		startReady := directorPlanRunStartReady(storedMetadata.LastRun)
		plan.Metadata.LastRun = &DirectorPlanRunStatus{
			Status:        director.PlanStatusFailed,
			Summary:       "后台导演写入的规划未通过校验。",
			Error:         err.Error(),
			SourceTurnID:  sourceTurnID,
			UpdatedAt:     now,
			PlannedDocs:   len(requiredDirectorPlanDocKinds()),
			CompletedDocs: directorPlanCompletedDocs(plan.Docs, token.Hashes),
			StartReady:    startReady,
			Blocking:      false,
		}
		if writeErr := s.writeDirectorPlanMetadataLocked(storyID, branchID, plan.Metadata); writeErr != nil {
			return DirectorPlan{}, writeErr
		}
		return DirectorPlan{}, err
	}
	if err := s.validateDirectorLoreContext(plan.Docs.LoreContext); err != nil {
		startReady := directorPlanRunStartReady(storedMetadata.LastRun)
		plan.Metadata.LastRun = &DirectorPlanRunStatus{
			Status:        director.PlanStatusFailed,
			Summary:       "后台导演写入的资料工作集未通过校验。",
			Error:         err.Error(),
			SourceTurnID:  sourceTurnID,
			UpdatedAt:     now,
			PlannedDocs:   len(requiredDirectorPlanDocKinds()),
			CompletedDocs: directorPlanCompletedDocs(plan.Docs, token.Hashes),
			StartReady:    startReady,
			Blocking:      false,
		}
		if writeErr := s.writeDirectorPlanMetadataLocked(storyID, branchID, plan.Metadata); writeErr != nil {
			return DirectorPlan{}, writeErr
		}
		return DirectorPlan{}, err
	}
	meta, snapshot, err := s.boundedStorySnapshotWithLimitLocked(storyID, branchID, maxStoryHistoryPageTurns)
	if err != nil {
		return DirectorPlan{}, err
	}
	storyDirector := s.storyDirectorForMeta(meta)
	opportunity := EventOpportunity{}
	if storedMetadata.LastRun != nil && storedMetadata.LastRun.SourceTurnID == sourceTurnID {
		opportunity = storedMetadata.LastRun.EventOpportunity
	}
	turns := directorEventTurnsThrough(snapshot.Turns, sourceTurnID)
	eventRuntime, err := applyDirectorEventDecision(storedMetadata.EventRuntime, decision.EventDecision, opportunity, sourceTurnID, turns, DirectorEventCatalogFromStoryDirector(storyDirector))
	if err != nil {
		return DirectorPlan{}, fmt.Errorf("事件决策校验失败: %w", err)
	}
	storedMetadata.EventRuntime = eventRuntime
	if decision.Mode == director.DecisionKeep && directorPlanHashesEqual(token.Hashes, directorPlanHashes(plan.Docs)) {
		storedMetadata.LoreRevision, _ = lore.NewStore(s.root).Revision()
		storedMetadata.LastRun = &DirectorPlanRunStatus{
			Status:           director.PlanStatusReady,
			Summary:          firstNonEmpty(decision.Reason, "后台导演确认当前计划继续有效。"),
			SourceTurnID:     sourceTurnID,
			UpdatedAt:        now,
			PlannedDocs:      len(requiredDirectorPlanDocKinds()),
			CompletedDocs:    len(requiredDirectorPlanDocKinds()),
			StartReady:       true,
			Blocking:         false,
			Decision:         &decision,
			EventOpportunity: opportunity,
		}
		attachDirectorPlanDomainCommit(storedMetadata.LastRun, domainCommit, storedMetadata.Revision)
		if err := s.writeDirectorPlanMetadataLocked(storyID, branchID, storedMetadata); err != nil {
			return DirectorPlan{}, err
		}
		return s.readDirectorPlanLocked(storyID, branchID)
	}
	if decision.Mode == director.DecisionKeep {
		decision.Mode = director.DecisionPatch
		decision.Reason = firstNonEmpty(decision.Reason, "导演实际修改了计划，按 patch 记录。")
	}
	docsWritten := stagedDocs != nil && !directorPlanHashesEqual(directorPlanHashes(publishedDocs), directorPlanHashes(plan.Docs))
	if docsWritten {
		if err := writeDirectorDocumentChangesAtomically(s.directorPlanBranchDir(storyID, branchID), publishedDocs, plan.Docs); err != nil {
			return DirectorPlan{}, fmt.Errorf("原子发布导演规划文档失败: %w", err)
		}
	}
	plan.Metadata = s.buildDirectorPlanMetadataLocked(storyID, branchID, NormalizeBranchPlanningTurns(plan.Metadata.BranchPlanningTurns), "interactive_director", sourceTurnID)
	plan.Metadata.EventRuntime = eventRuntime
	plan.Metadata.LoreRevision, _ = lore.NewStore(s.root).Revision()
	plan.Metadata.LastRun = &DirectorPlanRunStatus{
		Status:           director.PlanStatusReady,
		Summary:          firstNonEmpty(decision.Reason, strings.TrimSpace(summary), "后台导演已更新导演规划。"),
		SourceTurnID:     sourceTurnID,
		UpdatedAt:        now,
		PlannedDocs:      len(requiredDirectorPlanDocKinds()),
		CompletedDocs:    len(requiredDirectorPlanDocKinds()),
		StartReady:       true,
		Blocking:         false,
		Decision:         &decision,
		EventOpportunity: opportunity,
	}
	attachDirectorPlanDomainCommit(plan.Metadata.LastRun, domainCommit, plan.Metadata.Revision)
	if err := s.writeDirectorPlanMetadataLocked(storyID, branchID, plan.Metadata); err != nil {
		if docsWritten {
			if restoreErr := writeDirectorDocumentChangesAtomically(s.directorPlanBranchDir(storyID, branchID), plan.Docs, publishedDocs); restoreErr != nil {
				return DirectorPlan{}, fmt.Errorf("写入导演规划元数据失败: %v；恢复原文档也失败: %v", err, restoreErr)
			}
		}
		return DirectorPlan{}, err
	}
	return s.readDirectorPlanLocked(storyID, branchID)
}

func (s *Store) MarkDirectorPlanRunFailed(storyID, branchID, sourceTurnID string, runErr error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	plan, err := s.readDirectorPlanLocked(storyID, branchID)
	if err != nil {
		return err
	}
	previous := plan.Metadata.LastRun
	if previous != nil {
		previousSourceTurnID := strings.TrimSpace(previous.SourceTurnID)
		if previousSourceTurnID != "" && previousSourceTurnID != strings.TrimSpace(sourceTurnID) {
			return nil
		}
		if previous.DomainCommit != nil {
			// Once the durable actor authorized and the canonical store recorded
			// this run, a late transport/cancellation error cannot turn it back
			// into a failed projection.
			return nil
		}
	}
	message := "后台导演更新失败，已保留现有规划。"
	errorText := ""
	if runErr != nil {
		errorText = runErr.Error()
	}
	startReady := directorPlanRunStartReady(previous)
	baselineHashes := map[string]string(nil)
	if previous != nil {
		baselineHashes = previous.BaselineHashes
	}
	plan.Metadata.LastRun = &DirectorPlanRunStatus{
		Status:        director.PlanStatusFailed,
		Summary:       message,
		Error:         errorText,
		SourceTurnID:  sourceTurnID,
		UpdatedAt:     time.Now().UTC().Format(time.RFC3339Nano),
		PlannedDocs:   len(requiredDirectorPlanDocKinds()),
		CompletedDocs: directorPlanCompletedDocs(plan.Docs, baselineHashes),
		StartReady:    startReady,
		Blocking:      false,
	}
	return s.writeDirectorPlanMetadataLocked(storyID, branchID, plan.Metadata)
}

func (s *Store) MarkDirectorPlanRunSkipped(storyID, branchID, sourceTurnID, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	plan, err := s.readDirectorPlanLocked(storyID, branchID)
	if err != nil {
		return err
	}
	plan.Metadata.LastRun = &DirectorPlanRunStatus{
		Status:        director.PlanStatusSkipped,
		Summary:       firstNonEmpty(strings.TrimSpace(reason), "后台导演已关闭，跳过规划。"),
		SourceTurnID:  sourceTurnID,
		UpdatedAt:     time.Now().UTC().Format(time.RFC3339Nano),
		PlannedDocs:   len(requiredDirectorPlanDocKinds()),
		CompletedDocs: len(requiredDirectorPlanDocKinds()),
		StartReady:    true,
		Blocking:      false,
	}
	return s.writeDirectorPlanMetadataLocked(storyID, branchID, plan.Metadata)
}

func (s *Store) seedDirectorPlanLocked(storyID, branchID string, meta StoryMeta, seed DirectorPlanSeed) error {
	templates := NormalizeStoryDirectorPlanningTemplates(seed.Templates)
	docs := DirectorPlanDocs{
		Plan:        renderDirectorPlanTemplate(templates.Plan, meta, branchID, seed),
		AgentBrief:  renderDirectorPlanTemplate(templates.AgentBrief, meta, branchID, seed),
		LoreContext: defaultDirectorLoreContextDocument(),
	}
	if err := validateDirectorPlanDocs(docs); err != nil {
		return err
	}
	if err := s.writeDirectorPlanDocsLocked(storyID, branchID, docs); err != nil {
		return err
	}
	metadata := s.buildDirectorPlanMetadataLocked(storyID, branchID, NormalizeBranchPlanningTurns(seed.BranchPlanningTurns), firstNonEmpty(seed.Source, "seed"), "")
	initialStatus := firstNonEmpty(seed.InitialStatus, director.PlanStatusWaitingOpening)
	initialSummary := firstNonEmpty(seed.InitialSummary, "等待开局完成后由后台导演规划。")
	startReady := seed.StartReady || initialStatus == director.PlanStatusReady || initialStatus == director.PlanStatusSkipped
	metadata.LastRun = &DirectorPlanRunStatus{
		Status:        initialStatus,
		Summary:       initialSummary,
		UpdatedAt:     metadata.UpdatedAt,
		PlannedDocs:   len(requiredDirectorPlanDocKinds()),
		CompletedDocs: directorPlanCompletedDocsForStatus(initialStatus),
		StartReady:    startReady,
		Blocking:      false,
	}
	return s.writeDirectorPlanMetadataLocked(storyID, branchID, metadata)
}

func (s *Store) cloneDirectorPlanForBranchLocked(storyID, fromBranchID, branchID, title string) error {
	parent, err := s.readDirectorPlanLocked(storyID, fromBranchID)
	if err != nil {
		return err
	}
	note := fmt.Sprintf("\n\n> 分支说明：本规划从 `%s` 分支创建，当前分支为 `%s`（%s）。用户选择优先，后续后台导演应按本分支独立刷新。\n", fromBranchID, branchID, strings.TrimSpace(title))
	docs := DirectorPlanDocs{
		Plan:        trimBytes(parent.Docs.Plan+note, maxDirectorPlanDocBytes),
		AgentBrief:  parent.Docs.AgentBrief,
		LoreContext: parent.Docs.LoreContext,
	}
	if err := validateDirectorPlanDocs(docs); err != nil {
		return err
	}
	if err := s.writeDirectorPlanDocsLocked(storyID, branchID, docs); err != nil {
		return err
	}
	metadata := s.buildDirectorPlanMetadataLocked(storyID, branchID, NormalizeBranchPlanningTurns(parent.Metadata.BranchPlanningTurns), "branch_seed", "")
	metadata.EventRuntime = parent.Metadata.EventRuntime
	metadata.LoreRevision = parent.Metadata.LoreRevision
	metadata.LastRun = &DirectorPlanRunStatus{
		Status:        director.PlanStatusReady,
		Summary:       "新分支已继承并独立保存导演规划。",
		UpdatedAt:     metadata.UpdatedAt,
		PlannedDocs:   len(requiredDirectorPlanDocKinds()),
		CompletedDocs: len(requiredDirectorPlanDocKinds()),
		StartReady:    true,
		Blocking:      false,
	}
	return s.writeDirectorPlanMetadataLocked(storyID, branchID, metadata)
}

func renderDirectorPlanTemplate(template string, meta StoryMeta, branchID string, seed DirectorPlanSeed) string {
	replacements := map[string]string{
		"{{story_title}}":           meta.Title,
		"{{origin}}":                meta.Origin,
		"{{branch_id}}":             branchID,
		"{{story_teller_id}}":       meta.StoryTellerID,
		"{{story_director_id}}":     meta.StoryDirectorID,
		"{{branch_planning_turns}}": fmt.Sprint(NormalizeBranchPlanningTurns(seed.BranchPlanningTurns)),
		"{{opening_summary}}":       strings.TrimSpace(seed.OpeningSummary),
	}
	out := template
	for key, value := range replacements {
		out = strings.ReplaceAll(out, key, value)
	}
	if strings.TrimSpace(seed.OpeningSummary) != "" && !strings.Contains(out, seed.OpeningSummary) {
		out += "\n\n## 开局摘要\n" + strings.TrimSpace(seed.OpeningSummary)
	}
	return strings.TrimSpace(out)
}

func (s *Store) readDirectorPlanLocked(storyID, branchID string) (DirectorPlan, error) {
	docs, err := s.readDirectorPlanDocsLocked(storyID, branchID)
	if err != nil {
		return DirectorPlan{}, err
	}
	metadata, err := s.readDirectorPlanMetadataLocked(storyID, branchID)
	if os.IsNotExist(err) {
		metadata = s.buildDirectorPlanMetadataLocked(storyID, branchID, defaultBranchPlanningTurns, "missing_metadata", "")
		if writeErr := s.writeDirectorPlanMetadataLocked(storyID, branchID, metadata); writeErr != nil {
			return DirectorPlan{}, writeErr
		}
	} else if err != nil {
		return DirectorPlan{}, err
	}
	metadata.Docs = directorPlanDocInfos(s.directorPlanBranchDir(storyID, branchID), docs)
	metadata.Revision = directorPlanRevision(docs, metadata.UpdatedAt)
	return DirectorPlan{
		StoryID:  storyID,
		BranchID: branchID,
		Docs:     docs,
		VisibleDocs: DirectorPlanVisibleDocs{
			AgentBrief:  strings.TrimSpace(trimBytes(docs.AgentBrief, directorPlanVisibleBytes)),
			LoreContext: ExtractDirectorLoreContextActiveSection(docs.LoreContext),
		},
		Metadata: metadata,
	}, nil
}

func (s *Store) readDirectorPlanDocsLocked(storyID, branchID string) (DirectorPlanDocs, error) {
	dir := s.directorPlanBranchDir(storyID, branchID)
	data, err := os.ReadFile(filepath.Join(dir, directorPlanFile))
	if err != nil {
		return DirectorPlanDocs{}, err
	}
	agentBrief, err := os.ReadFile(filepath.Join(dir, directorAgentBriefFile))
	if err != nil {
		return DirectorPlanDocs{}, err
	}
	loreContext, loreErr := os.ReadFile(filepath.Join(dir, directorLoreContextFile))
	if loreErr != nil {
		return DirectorPlanDocs{}, loreErr
	}
	return DirectorPlanDocs{Plan: string(data), AgentBrief: string(agentBrief), LoreContext: string(loreContext)}, nil
}

func (s *Store) writeDirectorPlanDocsLocked(storyID, branchID string, docs DirectorPlanDocs) error {
	return writeDirectorDocumentsAtomically(s.directorPlanBranchDir(storyID, branchID), docs)
}

func (s *Store) readDirectorPlanMetadataLocked(storyID, branchID string) (DirectorPlanMetadata, error) {
	data, err := os.ReadFile(filepath.Join(s.directorPlanBranchDir(storyID, branchID), directorPlanMetadataFile))
	if err != nil {
		return DirectorPlanMetadata{}, err
	}
	var metadata DirectorPlanMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return DirectorPlanMetadata{}, fmt.Errorf("解析导演规划元数据失败: %w", err)
	}
	metadata.Version = schemaVersion
	metadata.StoryID = storyID
	metadata.BranchID = branchID
	metadata.BranchPlanningTurns = NormalizeBranchPlanningTurns(metadata.BranchPlanningTurns)
	metadata.EventRuntime = normalizeDirectorEventRuntime(metadata.EventRuntime)
	return metadata, nil
}

func (s *Store) writeDirectorPlanMetadataLocked(storyID, branchID string, metadata DirectorPlanMetadata) error {
	metadata.Version = schemaVersion
	metadata.StoryID = storyID
	metadata.BranchID = branchID
	metadata.BranchPlanningTurns = NormalizeBranchPlanningTurns(metadata.BranchPlanningTurns)
	metadata.EventRuntime = normalizeDirectorEventRuntime(metadata.EventRuntime)
	if strings.TrimSpace(metadata.UpdatedAt) == "" {
		metadata.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(s.directorPlanBranchDir(storyID, branchID), directorPlanMetadataFile)
	return writeAtomicBytes(path, append(data, '\n'), 0o644)
}

func (s *Store) buildDirectorPlanMetadataLocked(storyID, branchID string, branchPlanningTurns int, source, sourceTurnID string) DirectorPlanMetadata {
	docs, _ := s.readDirectorPlanDocsLocked(storyID, branchID)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return DirectorPlanMetadata{
		Version:             schemaVersion,
		StoryID:             storyID,
		BranchID:            branchID,
		Revision:            directorPlanRevision(docs, now),
		BranchPlanningTurns: NormalizeBranchPlanningTurns(branchPlanningTurns),
		UpdatedAt:           now,
		Source:              strings.TrimSpace(source),
		SourceTurnID:        strings.TrimSpace(sourceTurnID),
		Docs:                directorPlanDocInfos(s.directorPlanBranchDir(storyID, branchID), docs),
	}
}
