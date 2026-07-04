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
)

const (
	DirectorPlanDocMainline     = "mainline"
	DirectorPlanDocCurrentEvent = "current_event"
	DirectorPlanDocNextBranches = "next_branches"

	DirectorPlanStatusWaitingOpening = "waiting_opening"
	DirectorPlanStatusRunning        = "running"
	DirectorPlanStatusReady          = "ready"
	DirectorPlanStatusSkipped        = "skipped"
	DirectorPlanStatusFailed         = "failed"
	DirectorPlanStatusConflict       = "conflict"

	directorPlanMainlineFile     = "mainline.md"
	directorPlanCurrentEventFile = "current-event.md"
	directorPlanNextBranchesFile = "next-branches.md"
	directorPlanMetadataFile     = "metadata.json"

	defaultBranchPlanningTurns = 5
	maxDirectorPlanDocBytes    = 24 * 1024
)

var requiredDirectorPlanHeadings = []string{
	"正文Agent可读 / Prose-agent visible",
	"后台导演私密 / Director private",
	"目标 / Goal",
	"节奏、压力与危机 / Pacing, Pressure, Crisis",
	"结果与代价 / Outcome and Cost",
	"状态 / State",
	"分支处理 / Branch Handling",
	"伏笔与回收 / Foreshadowing and Payoff",
}

type StoryDirectorPlanningTemplates struct {
	Mainline     string `json:"mainline,omitempty"`
	CurrentEvent string `json:"current_event,omitempty"`
	NextBranches string `json:"next_branches,omitempty"`
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
	Mainline     string `json:"mainline"`
	CurrentEvent string `json:"current_event"`
	NextBranches string `json:"next_branches"`
}

type DirectorPlanVisibleDocs struct {
	Mainline     string `json:"mainline,omitempty"`
	CurrentEvent string `json:"current_event,omitempty"`
	NextBranches string `json:"next_branches,omitempty"`
}

type DirectorPlanDocInfo struct {
	Path         string `json:"path"`
	Bytes        int    `json:"bytes"`
	Hash         string `json:"hash"`
	VisibleBytes int    `json:"visible_bytes,omitempty"`
}

type DirectorPlanRunStatus struct {
	Status         string            `json:"status,omitempty"`
	Summary        string            `json:"summary,omitempty"`
	Error          string            `json:"error,omitempty"`
	SourceTurnID   string            `json:"source_turn_id,omitempty"`
	UpdatedAt      string            `json:"updated_at,omitempty"`
	PlannedDocs    int               `json:"planned_docs,omitempty"`
	CompletedDocs  int               `json:"completed_docs,omitempty"`
	StartReady     bool              `json:"start_ready,omitempty"`
	Blocking       bool              `json:"blocking,omitempty"`
	BaselineHashes map[string]string `json:"baseline_hashes,omitempty"`
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
}

type DirectorPlan struct {
	StoryID     string                  `json:"story_id"`
	BranchID    string                  `json:"branch_id"`
	Docs        DirectorPlanDocs        `json:"docs"`
	VisibleDocs DirectorPlanVisibleDocs `json:"visible_docs,omitempty"`
	Metadata    DirectorPlanMetadata    `json:"metadata"`
}

type DirectorPlanStatus struct {
	StoryID       string `json:"story_id"`
	BranchID      string `json:"branch_id"`
	Status        string `json:"status"`
	Summary       string `json:"summary,omitempty"`
	Error         string `json:"error,omitempty"`
	SourceTurnID  string `json:"source_turn_id,omitempty"`
	UpdatedAt     string `json:"updated_at,omitempty"`
	PlannedDocs   int    `json:"planned_docs"`
	CompletedDocs int    `json:"completed_docs"`
	DocBytes      int    `json:"doc_bytes"`
	VisibleBytes  int    `json:"visible_bytes"`
	StartReady    bool   `json:"start_ready"`
	Blocking      bool   `json:"blocking"`
	Revision      string `json:"revision,omitempty"`
}

type UpdateDirectorPlanRequest struct {
	BranchID     string           `json:"branch_id,omitempty"`
	Docs         DirectorPlanDocs `json:"docs"`
	BaseRevision string           `json:"base_revision,omitempty"`
	Source       string           `json:"source,omitempty"`
	Summary      string           `json:"summary,omitempty"`
}

type RebuildDirectorPlanRequest struct {
	BranchID string `json:"branch_id,omitempty"`
	Source   string `json:"source,omitempty"`
}

type RunDirectorPlanRequest struct {
	BranchID string `json:"branch_id,omitempty"`
	Source   string `json:"source,omitempty"`
}

type DirectorPlanRunToken struct {
	StoryID  string            `json:"story_id"`
	BranchID string            `json:"branch_id"`
	Revision string            `json:"revision"`
	Hashes   map[string]string `json:"hashes,omitempty"`
}

func DefaultStoryDirectorPlanningTemplates() StoryDirectorPlanningTemplates {
	return StoryDirectorPlanningTemplates{
		Mainline: strings.TrimSpace(`# 大方向 / Mainline

## 正文Agent可读 / Prose-agent visible

### 目标 / Goal
围绕主角的长期目标、核心阻力和阶段成长建立主线。

### 节奏、压力与危机 / Pacing, Pressure, Crisis
用递进压力推动故事，避免连续空转。

### 结果与代价 / Outcome and Cost
每个阶段都需要可见收益、信息推进或代价。

### 状态 / State
记录主角、关键角色、势力与世界状态的长期变化。

### 分支处理 / Branch Handling
用户选择优先；根据故事导演的主线强度决定软牵引或强牵引。

### 伏笔与回收 / Foreshadowing and Payoff
保留可被用户观察、误读、调查和回收的线索。

## 后台导演私密 / Director private

### 目标 / Goal
维护长期真相、潜在反转和主线候选方向。

### 节奏、压力与危机 / Pacing, Pressure, Crisis
规划暗线压力、对手行动和危机升级。

### 结果与代价 / Outcome and Cost
为关键选择准备不同结果与代价。

### 状态 / State
记录不应直接剧透给玩家的隐藏状态。

### 分支处理 / Branch Handling
准备偏离主线后的重规划策略。

### 伏笔与回收 / Foreshadowing and Payoff
规划伏笔埋设、误导、回收和新问题。`),
		CurrentEvent: strings.TrimSpace(`# 当前主线事件 / Current Main Event

## 正文Agent可读 / Prose-agent visible

### 目标 / Goal
明确当前事件的可玩目标，让用户知道能采取行动。

### 节奏、压力与危机 / Pacing, Pressure, Crisis
安排本阶段的外部压力、时间压力、关系压力或资源危机。

### 结果与代价 / Outcome and Cost
给用户行动带来可见后果，避免无成本推进。

### 状态 / State
记录当前场景、角色站位、资源、风险和已公开信息。

### 分支处理 / Branch Handling
允许观察、对话、调查、冒险和保守应对等方向成立。

### 伏笔与回收 / Foreshadowing and Payoff
在当前事件中埋下或回收一个可感知线索。

## 后台导演私密 / Director private

### 目标 / Goal
维护当前事件背后的真实目标和暗线作用。

### 节奏、压力与危机 / Pacing, Pressure, Crisis
规划危机升级点、反转点和 NPC 主动行动。

### 结果与代价 / Outcome and Cost
准备成功、部分成功、失败和重大失败的后续处理。

### 状态 / State
记录隐藏动机、未公开资源和幕后变化。

### 分支处理 / Branch Handling
准备用户偏离当前事件时的合理承接。

### 伏笔与回收 / Foreshadowing and Payoff
明确哪些伏笔本阶段可以露出、回收或继续隐藏。`),
		NextBranches: strings.TrimSpace(`# 最近分支安排 / Next Branches

## 正文Agent可读 / Prose-agent visible

### 目标 / Goal
规划最近 5 回合内用户可能选择的方向，并给出可玩抓手。

### 节奏、压力与危机 / Pacing, Pressure, Crisis
每条候选方向都要有压力、风险或机会。

### 结果与代价 / Outcome and Cost
为常见选择准备即时反馈、阶段后果和代价。

### 状态 / State
记录接下来应保持一致的场景、资源、角色和关系状态。

### 分支处理 / Branch Handling
列出可能用户选择、裁定要点和剧情安排；不要锁死唯一解。

### 伏笔与回收 / Foreshadowing and Payoff
标出可给玩家感知的线索、回收点和新悬念。

## 后台导演私密 / Director private

### 目标 / Goal
维护最近分支背后的主线牵引目标。

### 节奏、压力与危机 / Pacing, Pressure, Crisis
安排不同选择下的危机升级和节奏切换。

### 结果与代价 / Outcome and Cost
准备不同用户选择下的隐藏代价、奖励和失败候选。

### 状态 / State
记录隐藏状态变化与不可直接泄露的信息。

### 分支处理 / Branch Handling
为最近 5 回合的可能用户选择设计裁定要点和剧情安排。

### 伏笔与回收 / Foreshadowing and Payoff
维护伏笔的投放顺序、回收条件和替代回收路径。`),
	}
}

func NormalizeStoryDirectorPlanningTemplates(templates StoryDirectorPlanningTemplates) StoryDirectorPlanningTemplates {
	defaults := DefaultStoryDirectorPlanningTemplates()
	templates.Mainline = normalizeDirectorPlanTemplate(templates.Mainline, defaults.Mainline)
	templates.CurrentEvent = normalizeDirectorPlanTemplate(templates.CurrentEvent, defaults.CurrentEvent)
	templates.NextBranches = normalizeDirectorPlanTemplate(templates.NextBranches, defaults.NextBranches)
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
	meta, _, err := s.readStoryLocked(storyID)
	if err != nil {
		return DirectorPlan{}, err
	}
	branchID, _, err = resolveBranch(meta, branchID)
	if err != nil {
		return DirectorPlan{}, err
	}
	return s.readDirectorPlanLocked(storyID, branchID)
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
	return DirectorPlanStatusFromPlan(plan, len(snapshot.Turns) > 0), nil
}

func (s *Store) UpdateDirectorPlan(storyID string, req UpdateDirectorPlanRequest) (DirectorPlan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	meta, _, err := s.readStoryLocked(storyID)
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
	if base := strings.TrimSpace(req.BaseRevision); base != "" && base != current.Metadata.Revision {
		return DirectorPlan{}, fmt.Errorf("导演规划已被其他操作更新，请重新加载后再保存")
	}
	if err := validateDirectorPlanDocs(req.Docs); err != nil {
		return DirectorPlan{}, err
	}
	if err := s.writeDirectorPlanDocsLocked(storyID, branchID, req.Docs); err != nil {
		return DirectorPlan{}, err
	}
	metadata := s.buildDirectorPlanMetadataLocked(storyID, branchID, NormalizeBranchPlanningTurns(current.Metadata.BranchPlanningTurns), strings.TrimSpace(req.Source), "")
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
	meta, _, err := s.readStoryLocked(storyID)
	if err != nil {
		return DirectorPlan{}, err
	}
	branchID, _, err := resolveBranch(meta, req.BranchID)
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

func (s *Store) MarkDirectorPlanRunStarted(storyID, branchID string, token DirectorPlanRunToken, sourceTurnID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	metadata, err := s.readDirectorPlanMetadataLocked(storyID, branchID)
	if err != nil {
		return err
	}
	previous := metadata.LastRun
	startReady := directorPlanRunStartReady(previous)
	metadata.LastRun = &DirectorPlanRunStatus{
		Status:         DirectorPlanStatusRunning,
		Summary:        "后台导演正在规划开局。",
		SourceTurnID:   sourceTurnID,
		UpdatedAt:      time.Now().UTC().Format(time.RFC3339Nano),
		PlannedDocs:    len(requiredDirectorPlanDocKinds()),
		CompletedDocs:  0,
		StartReady:     startReady,
		Blocking:       !startReady,
		BaselineHashes: token.Hashes,
	}
	return s.writeDirectorPlanMetadataLocked(storyID, branchID, metadata)
}

func (s *Store) CompleteDirectorPlanRun(storyID, branchID string, token DirectorPlanRunToken, sourceTurnID, summary string) (DirectorPlan, error) {
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
			Blocking:      !startReady,
		}
		if writeErr := s.writeDirectorPlanMetadataLocked(storyID, branchID, plan.Metadata); writeErr != nil {
			return DirectorPlan{}, writeErr
		}
		return DirectorPlan{}, err
	}
	plan.Metadata = s.buildDirectorPlanMetadataLocked(storyID, branchID, NormalizeBranchPlanningTurns(plan.Metadata.BranchPlanningTurns), "interactive_director", sourceTurnID)
	plan.Metadata.LastRun = &DirectorPlanRunStatus{
		Status:        DirectorPlanStatusReady,
		Summary:       firstNonEmpty(strings.TrimSpace(summary), "后台导演已更新三层规划。"),
		SourceTurnID:  sourceTurnID,
		UpdatedAt:     now,
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
		Blocking:      !startReady,
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
		Mainline:     renderDirectorPlanTemplate(templates.Mainline, meta, branchID, seed),
		CurrentEvent: renderDirectorPlanTemplate(templates.CurrentEvent, meta, branchID, seed),
		NextBranches: renderDirectorPlanTemplate(templates.NextBranches, meta, branchID, seed),
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
		Mainline:     trimBytes(parent.Docs.Mainline+note, maxDirectorPlanDocBytes),
		CurrentEvent: trimBytes(parent.Docs.CurrentEvent+note, maxDirectorPlanDocBytes),
		NextBranches: trimBytes(parent.Docs.NextBranches+note, maxDirectorPlanDocBytes),
	}
	if err := validateDirectorPlanDocs(docs); err != nil {
		return err
	}
	if err := s.writeDirectorPlanDocsLocked(storyID, branchID, docs); err != nil {
		return err
	}
	metadata := s.buildDirectorPlanMetadataLocked(storyID, branchID, NormalizeBranchPlanningTurns(parent.Metadata.BranchPlanningTurns), "branch_seed", "")
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
		out += "\n\n## 开局摘要 / Opening Summary\n" + strings.TrimSpace(seed.OpeningSummary)
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
			Mainline:     ExtractDirectorPlanVisibleSection(docs.Mainline),
			CurrentEvent: ExtractDirectorPlanVisibleSection(docs.CurrentEvent),
			NextBranches: ExtractDirectorPlanVisibleSection(docs.NextBranches),
		},
		Metadata: metadata,
	}, nil
}

func (s *Store) readDirectorPlanDocsLocked(storyID, branchID string) (DirectorPlanDocs, error) {
	dir := s.directorPlanBranchDir(storyID, branchID)
	read := func(name string) (string, error) {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
	mainline, err := read(directorPlanMainlineFile)
	if err != nil {
		return DirectorPlanDocs{}, err
	}
	current, err := read(directorPlanCurrentEventFile)
	if err != nil {
		return DirectorPlanDocs{}, err
	}
	next, err := read(directorPlanNextBranchesFile)
	if err != nil {
		return DirectorPlanDocs{}, err
	}
	return DirectorPlanDocs{Mainline: mainline, CurrentEvent: current, NextBranches: next}, nil
}

func (s *Store) writeDirectorPlanDocsLocked(storyID, branchID string, docs DirectorPlanDocs) error {
	dir := s.directorPlanBranchDir(storyID, branchID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	files := map[string]string{
		directorPlanMainlineFile:     docs.Mainline,
		directorPlanCurrentEventFile: docs.CurrentEvent,
		directorPlanNextBranchesFile: docs.NextBranches,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(strings.TrimSpace(content)+"\n"), 0o644); err != nil {
			return err
		}
	}
	return nil
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
	return metadata, nil
}

func (s *Store) writeDirectorPlanMetadataLocked(storyID, branchID string, metadata DirectorPlanMetadata) error {
	metadata.Version = schemaVersion
	metadata.StoryID = storyID
	metadata.BranchID = branchID
	metadata.BranchPlanningTurns = NormalizeBranchPlanningTurns(metadata.BranchPlanningTurns)
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
	for kind, content := range map[string]string{
		DirectorPlanDocMainline:     docs.Mainline,
		DirectorPlanDocCurrentEvent: docs.CurrentEvent,
		DirectorPlanDocNextBranches: docs.NextBranches,
	} {
		if err := validateDirectorPlanDoc(kind, content); err != nil {
			return err
		}
	}
	return nil
}

func validateDirectorPlanDoc(kind, content string) error {
	content = strings.TrimSpace(content)
	if content == "" {
		return fmt.Errorf("导演规划 %s 不能为空", kind)
	}
	if len([]byte(content)) > maxDirectorPlanDocBytes {
		return fmt.Errorf("导演规划 %s 超过大小上限 %d bytes", kind, maxDirectorPlanDocBytes)
	}
	for _, heading := range requiredDirectorPlanHeadings {
		if !strings.Contains(content, heading) {
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
	start := strings.Index(content, "## 正文Agent可读 / Prose-agent visible")
	if start < 0 {
		return ""
	}
	visible := content[start:]
	if end := strings.Index(visible, "## 后台导演私密 / Director private"); end >= 0 {
		visible = visible[:end]
	}
	return strings.TrimSpace(trimBytes(visible, 8*1024))
}

func DirectorPlanVisibleContext(plan DirectorPlan, limitBytes int) string {
	if limitBytes <= 0 {
		limitBytes = 12 * 1024
	}
	var sb strings.Builder
	writeDirectorPlanContextBlock(&sb, "大方向规划 / Mainline", plan.VisibleDocs.Mainline)
	writeDirectorPlanContextBlock(&sb, "当前主线事件 / Current Event", plan.VisibleDocs.CurrentEvent)
	writeDirectorPlanContextBlock(&sb, "最近分支安排 / Next Branches", plan.VisibleDocs.NextBranches)
	return strings.TrimSpace(trimBytes(sb.String(), limitBytes))
}

func DirectorPlanStatusFromPlan(plan DirectorPlan, hasTurns bool) DirectorPlanStatus {
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
		blocking = run.Blocking
		if status == DirectorPlanStatusRunning {
			completedDocs = directorPlanCompletedDocs(plan.Docs, run.BaselineHashes)
			blocking = !startReady
		}
	}
	if status == DirectorPlanStatusWaitingOpening && hasTurns && !startReady {
		blocking = true
	}
	if status == DirectorPlanStatusFailed && !startReady {
		blocking = true
	}
	if startReady {
		blocking = false
	}
	if completedDocs > plannedDocs {
		completedDocs = plannedDocs
	}
	return DirectorPlanStatus{
		StoryID:       plan.StoryID,
		BranchID:      plan.BranchID,
		Status:        status,
		Summary:       summary,
		Error:         errorText,
		SourceTurnID:  sourceTurnID,
		UpdatedAt:     updatedAt,
		PlannedDocs:   plannedDocs,
		CompletedDocs: completedDocs,
		DocBytes:      docBytes,
		VisibleBytes:  visibleBytes,
		StartReady:    startReady,
		Blocking:      blocking,
		Revision:      plan.Metadata.Revision,
	}
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

func (s *Store) DirectorPlanAllowedPaths(storyID, branchID string) []string {
	dir := s.directorPlanBranchDir(storyID, branchID)
	return []string{
		filepath.Join(dir, directorPlanMainlineFile),
		filepath.Join(dir, directorPlanCurrentEventFile),
		filepath.Join(dir, directorPlanNextBranchesFile),
	}
}

func directorPlanDocInfos(dir string, docs DirectorPlanDocs) map[string]DirectorPlanDocInfo {
	return map[string]DirectorPlanDocInfo{
		DirectorPlanDocMainline:     directorPlanDocInfo(filepath.Join(dir, directorPlanMainlineFile), docs.Mainline),
		DirectorPlanDocCurrentEvent: directorPlanDocInfo(filepath.Join(dir, directorPlanCurrentEventFile), docs.CurrentEvent),
		DirectorPlanDocNextBranches: directorPlanDocInfo(filepath.Join(dir, directorPlanNextBranchesFile), docs.NextBranches),
	}
}

func directorPlanDocInfo(path, content string) DirectorPlanDocInfo {
	return DirectorPlanDocInfo{Path: filepath.ToSlash(path), Bytes: len([]byte(content)), Hash: textHash(content), VisibleBytes: len([]byte(ExtractDirectorPlanVisibleSection(content)))}
}

func directorPlanHashes(docs DirectorPlanDocs) map[string]string {
	return map[string]string{
		DirectorPlanDocMainline:     textHash(docs.Mainline),
		DirectorPlanDocCurrentEvent: textHash(docs.CurrentEvent),
		DirectorPlanDocNextBranches: textHash(docs.NextBranches),
	}
}

func directorPlanRevision(docs DirectorPlanDocs, updatedAt string) string {
	return textHash(strings.Join([]string{docs.Mainline, docs.CurrentEvent, docs.NextBranches, updatedAt}, "\n---director-plan---\n"))
}

func requiredDirectorPlanDocKinds() []string {
	return []string{DirectorPlanDocMainline, DirectorPlanDocCurrentEvent, DirectorPlanDocNextBranches}
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
