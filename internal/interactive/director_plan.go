package interactive

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"denova/internal/book"
)

const (
	DirectorPlanDocPlan        = "plan"
	DirectorPlanDocAgentBrief  = "agent_brief"
	DirectorPlanDocLoreContext = "lore_context"

	DirectorPlanStatusWaitingOpening = "waiting_opening"
	DirectorPlanStatusRunning        = "running"
	DirectorPlanStatusReady          = "ready"
	DirectorPlanStatusSkipped        = "skipped"
	DirectorPlanStatusFailed         = "failed"
	DirectorPlanStatusConflict       = "conflict"

	directorPlanFile         = "director.md"
	directorAgentBriefFile   = "agent-brief.md"
	directorPlanMetadataFile = "metadata.json"

	defaultBranchPlanningTurns = 5
)

// DirectorContextMaxBytes is the hard ceiling for a complete director-related
// context fragment. Total prompt assembly remains bounded by the model-aware
// context budget.
const DirectorContextMaxBytes = 128 * 1024

const (
	maxDirectorPlanDocBytes  = DirectorContextMaxBytes
	directorPlanVisibleBytes = DirectorContextMaxBytes
)

type StoryDirectorPlanningTemplates struct {
	Plan       string `json:"plan,omitempty"`
	AgentBrief string `json:"agent_brief,omitempty"`
}

type DirectorPlanSeed struct {
	Templates           StoryDirectorPlanningTemplates `json:"-"`
	BranchPlanningTurns int                            `json:"-"`
	Source              string                         `json:"-"`
	OpeningSummary      string                         `json:"-"`
	InitialStatus       string                         `json:"-"`
	InitialSummary      string                         `json:"-"`
	StartReady          bool                           `json:"-"`
}

type DirectorPlanDocs struct {
	Plan        string `json:"plan"`
	AgentBrief  string `json:"agent_brief"`
	LoreContext string `json:"lore_context"`
}

type DirectorPlanVisibleDocs struct {
	AgentBrief  string `json:"agent_brief,omitempty"`
	LoreContext string `json:"lore_context,omitempty"`
}

type DirectorPlanDocInfo struct {
	Path         string `json:"path"`
	Bytes        int    `json:"bytes"`
	Hash         string `json:"hash"`
	VisibleBytes int    `json:"visible_bytes,omitempty"`
}

type DirectorPlanRunStatus struct {
	Status           string            `json:"status,omitempty"`
	Summary          string            `json:"summary,omitempty"`
	Error            string            `json:"error,omitempty"`
	SourceTurnID     string            `json:"source_turn_id,omitempty"`
	UpdatedAt        string            `json:"updated_at,omitempty"`
	PlannedDocs      int               `json:"planned_docs,omitempty"`
	CompletedDocs    int               `json:"completed_docs,omitempty"`
	StartReady       bool              `json:"start_ready,omitempty"`
	Blocking         bool              `json:"blocking,omitempty"`
	BaselineHashes   map[string]string `json:"baseline_hashes,omitempty"`
	Decision         *PlanDecision     `json:"decision,omitempty"`
	EventOpportunity EventOpportunity  `json:"event_opportunity,omitempty"`
}

type DirectorPlanMetadata struct {
	Version             int                            `json:"version"`
	StoryID             string                         `json:"story_id"`
	BranchID            string                         `json:"branch_id"`
	Revision            string                         `json:"revision"`
	BranchPlanningTurns int                            `json:"branch_planning_turns"`
	UpdatedAt           string                         `json:"updated_at"`
	Source              string                         `json:"source,omitempty"`
	SourceTurnID        string                         `json:"source_turn_id,omitempty"`
	Docs                map[string]DirectorPlanDocInfo `json:"docs,omitempty"`
	LastRun             *DirectorPlanRunStatus         `json:"last_run,omitempty"`
	EventRuntime        DirectorEventRuntime           `json:"event_runtime,omitempty"`
	LoreRevision        string                         `json:"lore_revision,omitempty"`
}

type DirectorPlan struct {
	StoryID     string                  `json:"story_id"`
	BranchID    string                  `json:"branch_id"`
	Docs        DirectorPlanDocs        `json:"docs"`
	VisibleDocs DirectorPlanVisibleDocs `json:"visible_docs,omitempty"`
	Metadata    DirectorPlanMetadata    `json:"metadata"`
}

type DirectorPlanStatus struct {
	StoryID          string               `json:"story_id"`
	BranchID         string               `json:"branch_id"`
	Status           string               `json:"status"`
	Summary          string               `json:"summary,omitempty"`
	Error            string               `json:"error,omitempty"`
	SourceTurnID     string               `json:"source_turn_id,omitempty"`
	UpdatedAt        string               `json:"updated_at,omitempty"`
	PlannedDocs      int                  `json:"planned_docs"`
	CompletedDocs    int                  `json:"completed_docs"`
	DocBytes         int                  `json:"doc_bytes"`
	VisibleBytes     int                  `json:"visible_bytes"`
	StartReady       bool                 `json:"start_ready"`
	Blocking         bool                 `json:"blocking"`
	Revision         string               `json:"revision,omitempty"`
	Decision         *PlanDecision        `json:"decision,omitempty"`
	EventRuntime     DirectorEventRuntime `json:"event_runtime,omitempty"`
	EventOpportunity EventOpportunity     `json:"event_opportunity,omitempty"`
}

type UpdateDirectorPlanRequest struct {
	BranchID     string           `json:"branch_id,omitempty"`
	Docs         DirectorPlanDocs `json:"docs"`
	BaseRevision string           `json:"base_revision,omitempty"`
	Source       string           `json:"source,omitempty"`
	Summary      string           `json:"summary,omitempty"`
}

type RebuildDirectorPlanRequest struct {
	BranchID    string `json:"branch_id,omitempty"`
	Source      string `json:"source,omitempty"`
	ResetEvents bool   `json:"reset_events,omitempty"`
}

type RunDirectorPlanRequest struct {
	BranchID             string `json:"branch_id,omitempty"`
	Source               string `json:"source,omitempty"`
	ForceEventEvaluation bool   `json:"force_event_evaluation,omitempty"`
}

type DirectorPlanRunToken struct {
	StoryID  string            `json:"story_id"`
	BranchID string            `json:"branch_id"`
	Revision string            `json:"revision"`
	Hashes   map[string]string `json:"hashes,omitempty"`
}

func NormalizeStoryDirectorPlanningTemplates(templates StoryDirectorPlanningTemplates) StoryDirectorPlanningTemplates {
	defaults := DefaultStoryDirectorPlanningTemplates()
	templates.Plan = normalizeDirectorPlanTemplate(templates.Plan, defaults.Plan)
	templates.AgentBrief = normalizeDirectorPlanTemplate(templates.AgentBrief, defaults.AgentBrief)
	return templates
}

func normalizeDirectorPlanTemplate(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		value = fallback
	}
	return trimBytes(value, maxDirectorPlanDocBytes)
}

func NormalizeBranchPlanningTurns(value int) int {
	if value <= 0 {
		return defaultBranchPlanningTurns
	}
	if value < 1 {
		return 1
	}
	if value > 12 {
		return 12
	}
	return value
}

func (s *Store) DirectorPlan(storyID, branchID string) (DirectorPlan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	meta, lines, err := s.readStoryLocked(storyID)
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
	snapshot, err := snapshotFromLines(storyID, branchID, meta, lines)
	if err != nil {
		return DirectorPlan{}, err
	}
	plan.Metadata.EventRuntime = reconcileDirectorEventRuntime(plan.Metadata.EventRuntime, snapshot.Turns)
	return plan, nil
}

func (s *Store) DirectorPlanStatus(storyID, branchID string) (DirectorPlanStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	meta, lines, err := s.readStoryLocked(storyID)
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
	snapshot, err := snapshotFromLines(storyID, branchID, meta, lines)
	if err != nil {
		return DirectorPlanStatus{}, err
	}
	plan.Metadata.EventRuntime = reconcileDirectorEventRuntime(plan.Metadata.EventRuntime, snapshot.Turns)
	return DirectorPlanStatusFromPlan(plan, len(snapshot.Turns) > 0), nil
}

func (s *Store) UpdateDirectorPlan(storyID string, req UpdateDirectorPlanRequest) (DirectorPlan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	meta, lines, err := s.readStoryLocked(storyID)
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
	snapshot, err := snapshotFromLines(storyID, branchID, meta, lines)
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
	metadata.LastRun = &DirectorPlanRunStatus{
		Status:        DirectorPlanStatusReady,
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
	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return DirectorPlan{}, err
	}
	branchID, _, err := resolveBranch(meta, req.BranchID)
	if err != nil {
		return DirectorPlan{}, err
	}
	previous, _ := s.readDirectorPlanMetadataLocked(storyID, branchID)
	snapshot, err := snapshotFromLines(storyID, branchID, meta, lines)
	if err != nil {
		return DirectorPlan{}, err
	}
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
		Status:        DirectorPlanStatusReady,
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
	meta, _, err := s.readStoryLocked(storyID)
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
	meta, lines, err := s.readStoryLocked(storyID)
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
	snapshot, err := snapshotFromLines(storyID, branchID, meta, lines)
	if err != nil {
		return err
	}
	director := s.storyDirectorForMeta(meta)
	catalog := DirectorEventCatalogFromStoryDirector(director)
	turns := directorEventTurnsThrough(snapshot.Turns, sourceTurnID)
	metadata.EventRuntime = reconcileDirectorEventRuntime(metadata.EventRuntime, turns)
	forced := len(forceEventEvaluation) > 0 && forceEventEvaluation[0]
	opportunity := directorEventOpportunity(metadata.EventRuntime, turns, director.Strategy.EventFrequency, len(catalog) > 0, forced)
	metadata.LastRun = &DirectorPlanRunStatus{
		Status:           DirectorPlanStatusRunning,
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

func (s *Store) completeDirectorPlanRun(storyID, branchID string, token DirectorPlanRunToken, sourceTurnID, summary string, stagedDocs *DirectorPlanDocs) (DirectorPlan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	storedMetadata, err := s.readDirectorPlanMetadataLocked(storyID, branchID)
	if err != nil {
		return DirectorPlan{}, err
	}
	plan, err := s.readDirectorPlanLocked(storyID, branchID)
	if err != nil {
		return DirectorPlan{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if token.Revision != "" && token.Revision != storedMetadata.Revision {
		storedMetadata.LastRun = &DirectorPlanRunStatus{
			Status:        DirectorPlanStatusConflict,
			Summary:       "后台导演运行期间规划已被手动修改，已跳过覆盖。",
			SourceTurnID:  sourceTurnID,
			UpdatedAt:     now,
			PlannedDocs:   len(requiredDirectorPlanDocKinds()),
			CompletedDocs: len(requiredDirectorPlanDocKinds()),
			StartReady:    true,
			Blocking:      false,
		}
		if err := s.writeDirectorPlanMetadataLocked(storyID, branchID, storedMetadata); err != nil {
			return DirectorPlan{}, err
		}
		return s.readDirectorPlanLocked(storyID, branchID)
	}
	if storedMetadata.LastRun != nil && storedMetadata.LastRun.SourceTurnID != "" && storedMetadata.LastRun.SourceTurnID != sourceTurnID {
		// A newer Director run already owns the branch status. An older completion
		// must not replace its status or replay event decisions against stale turns.
		return s.readDirectorPlanLocked(storyID, branchID)
	}
	decision, err := ParsePlanDecisionJSON(summary)
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
			Status:        DirectorPlanStatusFailed,
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
			Status:        DirectorPlanStatusFailed,
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
	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return DirectorPlan{}, err
	}
	snapshot, err := snapshotFromLines(storyID, branchID, meta, lines)
	if err != nil {
		return DirectorPlan{}, err
	}
	director := s.storyDirectorForMeta(meta)
	opportunity := EventOpportunity{}
	if storedMetadata.LastRun != nil && storedMetadata.LastRun.SourceTurnID == sourceTurnID {
		opportunity = storedMetadata.LastRun.EventOpportunity
	}
	turns := directorEventTurnsThrough(snapshot.Turns, sourceTurnID)
	eventRuntime, err := applyDirectorEventDecision(storedMetadata.EventRuntime, decision.EventDecision, opportunity, sourceTurnID, turns, DirectorEventCatalogFromStoryDirector(director))
	if err != nil {
		return DirectorPlan{}, fmt.Errorf("事件决策校验失败: %w", err)
	}
	storedMetadata.EventRuntime = eventRuntime
	if decision.Mode == PlanDecisionKeep && directorPlanHashesEqual(token.Hashes, directorPlanHashes(plan.Docs)) {
		storedMetadata.LoreRevision, _ = book.NewLoreStore(s.root).Revision()
		storedMetadata.LastRun = &DirectorPlanRunStatus{
			Status:           DirectorPlanStatusReady,
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
		if err := s.writeDirectorPlanMetadataLocked(storyID, branchID, storedMetadata); err != nil {
			return DirectorPlan{}, err
		}
		return s.readDirectorPlanLocked(storyID, branchID)
	}
	if decision.Mode == PlanDecisionKeep {
		decision.Mode = PlanDecisionPatch
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
	plan.Metadata.LoreRevision, _ = book.NewLoreStore(s.root).Revision()
	plan.Metadata.LastRun = &DirectorPlanRunStatus{
		Status:           DirectorPlanStatusReady,
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
	message := "后台导演更新失败，已保留现有规划。"
	errorText := ""
	if runErr != nil {
		errorText = runErr.Error()
	}
	previous := plan.Metadata.LastRun
	startReady := directorPlanRunStartReady(previous)
	baselineHashes := map[string]string(nil)
	if previous != nil {
		baselineHashes = previous.BaselineHashes
	}
	plan.Metadata.LastRun = &DirectorPlanRunStatus{
		Status:        DirectorPlanStatusFailed,
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
		Status:        DirectorPlanStatusSkipped,
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
	initialStatus := firstNonEmpty(seed.InitialStatus, DirectorPlanStatusWaitingOpening)
	initialSummary := firstNonEmpty(seed.InitialSummary, "等待开局完成后由后台导演规划。")
	startReady := seed.StartReady || initialStatus == DirectorPlanStatusReady || initialStatus == DirectorPlanStatusSkipped
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
		Status:        DirectorPlanStatusReady,
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
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
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
