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
	DirectorPlanDocPlan = "plan"

	DirectorPlanStatusWaitingOpening = "waiting_opening"
	DirectorPlanStatusRunning        = "running"
	DirectorPlanStatusReady          = "ready"
	DirectorPlanStatusSkipped        = "skipped"
	DirectorPlanStatusFailed         = "failed"
	DirectorPlanStatusConflict       = "conflict"

	directorPlanFile         = "director.md"
	directorPlanMetadataFile = "metadata.json"

	defaultBranchPlanningTurns = 5
)

// DirectorContextMinBytes is the floor for director.md and director-related
// model context slices so callers do not accidentally reintroduce tiny caps.
const DirectorContextMinBytes = 64 * 1024

const (
	maxDirectorPlanDocBytes  = DirectorContextMinBytes
	directorPlanVisibleBytes = DirectorContextMinBytes
)

var requiredDirectorPlanHeadings = []string{
	"正文Agent可读",
	"后台导演私密",
	"阶段钩子与阅读欲望",
	"资料库锚点",
	"核心角色与关系张力",
	"重要势力与阶段阻力",
	"当前场景与行动空间",
	"信息揭示与线索密度",
	"遭遇、检定与代价",
	"爽点、危机与反转",
	"状态连续性",
	"最近分支安排",
	"伏笔与回收",
}

type StoryDirectorPlanningTemplates struct {
	Plan string `json:"plan,omitempty"`
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
	Plan string `json:"plan"`
}

type DirectorPlanVisibleDocs struct {
	Plan string `json:"plan,omitempty"`
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
		Plan: strings.TrimSpace(`# 导演规划

## 正文Agent可读

### 阶段钩子与阅读欲望
围绕主角当前最想解决的问题、可见收益、未解谜团和下一次反转建立推进动力。每个可玩回合至少推进一个有效信息点、角色关系变化、压力升级、收益/代价或新悬念，避免连续空转。

### 资料库锚点
优先使用资料库中的重要角色、势力、规则、地点和既有关系；非必要不要自创核心角色、组织或世界规则。资料库不足时，新增内容只能作为临时候选，并要与既有设定自洽。

### 核心角色与关系张力
规划男/女主角、关键同伴、阶段性反派、重要势力代表与关系节点的目标、态度和冲突。普通 NPC 只有承担信息、冲突、选择代价或节奏功能时才出现。

### 重要势力与阶段阻力
记录当前阶段能推动压力的势力、派系、组织规则、资源封锁、舆论评价或追捕压力。

### 当前场景与行动空间
明确当前场景、主角处境、直接目标和可玩行动空间，让用户能观察、对话、调查、冒险、交易或保守应对。

### 信息揭示与线索密度
安排本阶段应公开的信息、可发现线索和误导点；失败不应让剧情卡死，失败可以带来代价、不完整信息或危机升级。

### 遭遇、检定与代价
准备可能触发的战斗、谈判、追逐、陷阱、谜题或规则检定，明确成功、部分成功、失败和重大失败的后果。

### 爽点、危机与反转
给后续回合准备阶段性爽点、危险升级、关系爆点、身份揭露、误会反转或伏笔回收，抓住阅读欲望。

### 状态连续性
记录主角、重要角色、势力、资源、任务进度、已公开信息和世界状态的可见变化。

### 最近分支安排
规划最近 {{branch_planning_turns}} 回合内可能的用户方向、裁定要点和承接路径；尊重用户选择，不锁死唯一解。

### 伏笔与回收
标出可给玩家感知的线索、回收点和新悬念。

## 后台导演私密

### 阶段钩子与阅读欲望
维护隐藏真相、阶段高潮、下一次反转和阅读钩子的投放顺序，保证节奏持续向前。

### 资料库锚点
记录后台规划必须遵守的资料库设定、重要角色/势力边界和不可违背的世界规则；新增候选必须注明为何资料库不足。

### 核心角色与关系张力
规划重要角色的隐藏动机、真实立场、关系转折、阶段性敌意或结盟机会。

### 重要势力与阶段阻力
安排势力暗线行动、资源争夺、规则压迫、追杀、审判、交易或舆论压力。

### 当前场景与行动空间
准备场景背后的隐藏资源、陷阱、证据、观察角度和可承接行动。

### 信息揭示与线索密度
规划本阶段应该揭示、暂缓、误导或拆分的信息，确保用户每轮都有可感知收获。

### 遭遇、检定与代价
准备不同裁定等级下的隐藏代价、奖励、敌对反应、资源损耗和失败推进路径。

### 爽点、危机与反转
安排爽点释放、危机升级、角色爆点、反派压迫和反转条件，不让剧情只停留在氛围描写。

### 状态连续性
记录不应直接剧透的隐藏状态、未公开角色动机、幕后势力变化和长期影响。

### 最近分支安排
为最近 {{branch_planning_turns}} 回合的用户选择准备多条承接策略；偏离主线时重规划，不强拉回固定剧本。

### 伏笔与回收
维护伏笔投放、误导、回收条件和替代回收路径。`),
	}
}

func NormalizeStoryDirectorPlanningTemplates(templates StoryDirectorPlanningTemplates) StoryDirectorPlanningTemplates {
	defaults := DefaultStoryDirectorPlanningTemplates()
	templates.Plan = normalizeDirectorPlanTemplate(templates.Plan, defaults.Plan)
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
		Summary:        "后台导演正在规划故事。",
		SourceTurnID:   sourceTurnID,
		UpdatedAt:      time.Now().UTC().Format(time.RFC3339Nano),
		PlannedDocs:    len(requiredDirectorPlanDocKinds()),
		CompletedDocs:  0,
		StartReady:     startReady,
		Blocking:       false,
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
			Blocking:      false,
		}
		if writeErr := s.writeDirectorPlanMetadataLocked(storyID, branchID, plan.Metadata); writeErr != nil {
			return DirectorPlan{}, writeErr
		}
		return DirectorPlan{}, err
	}
	plan.Metadata = s.buildDirectorPlanMetadataLocked(storyID, branchID, NormalizeBranchPlanningTurns(plan.Metadata.BranchPlanningTurns), "interactive_director", sourceTurnID)
	plan.Metadata.LastRun = &DirectorPlanRunStatus{
		Status:        DirectorPlanStatusReady,
		Summary:       firstNonEmpty(strings.TrimSpace(summary), "后台导演已更新导演规划。"),
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
	docs := DirectorPlanDocs{Plan: renderDirectorPlanTemplate(templates.Plan, meta, branchID, seed)}
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
	docs := DirectorPlanDocs{Plan: trimBytes(parent.Docs.Plan+note, maxDirectorPlanDocBytes)}
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
			Plan: ExtractDirectorPlanVisibleSection(docs.Plan),
		},
		Metadata: metadata,
	}, nil
}

func (s *Store) readDirectorPlanDocsLocked(storyID, branchID string) (DirectorPlanDocs, error) {
	data, err := os.ReadFile(filepath.Join(s.directorPlanBranchDir(storyID, branchID), directorPlanFile))
	if err != nil {
		return DirectorPlanDocs{}, err
	}
	return DirectorPlanDocs{Plan: string(data)}, nil
}

func (s *Store) writeDirectorPlanDocsLocked(storyID, branchID string, docs DirectorPlanDocs) error {
	dir := s.directorPlanBranchDir(storyID, branchID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, directorPlanFile), []byte(strings.TrimSpace(docs.Plan)+"\n"), 0o644)
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
	return validateDirectorPlanDoc(DirectorPlanDocPlan, docs.Plan)
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
	if limitBytes < DirectorContextMinBytes {
		limitBytes = DirectorContextMinBytes
	}
	var sb strings.Builder
	writeDirectorPlanContextBlock(&sb, "导演规划", plan.VisibleDocs.Plan)
	return strings.TrimSpace(trimBytes(sb.String(), limitBytes))
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
	return []string{filepath.Join(s.directorPlanBranchDir(storyID, branchID), directorPlanFile)}
}

func directorPlanDocInfos(dir string, docs DirectorPlanDocs) map[string]DirectorPlanDocInfo {
	return map[string]DirectorPlanDocInfo{
		DirectorPlanDocPlan: directorPlanDocInfo(filepath.Join(dir, directorPlanFile), docs.Plan),
	}
}

func directorPlanDocInfo(path, content string) DirectorPlanDocInfo {
	return DirectorPlanDocInfo{Path: filepath.ToSlash(path), Bytes: len([]byte(content)), Hash: textHash(content), VisibleBytes: len([]byte(ExtractDirectorPlanVisibleSection(content)))}
}

func directorPlanHashes(docs DirectorPlanDocs) map[string]string {
	return map[string]string{
		DirectorPlanDocPlan: textHash(docs.Plan),
	}
}

func directorPlanRevision(docs DirectorPlanDocs, updatedAt string) string {
	return textHash(strings.Join([]string{docs.Plan, updatedAt}, "\n---director-plan---\n"))
}

func requiredDirectorPlanDocKinds() []string {
	return []string{DirectorPlanDocPlan}
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
