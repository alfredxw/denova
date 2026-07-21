package interactive

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"denova/internal/imagepreset"
)

const schemaVersion = 1
const maxStoryLineBytes = 16 * 1024 * 1024
const defaultFirstStoryTitle = "新的开始"

// DefaultStoryReplyTargetChars is the default target length for one interactive story turn.
const DefaultStoryReplyTargetChars = 2000

const (
	DefaultStoryChoiceCount = 5
	MinStoryChoiceCount     = 2
	MaxStoryChoiceCount     = 10
)

const maxStoryOpeningTextRunes = 4000

const (
	StoryOpeningModeAI     = "ai"
	StoryOpeningModePreset = "preset"
	StoryOpeningModeCustom = "custom"
)

const (
	StoryImageModeManual   = "manual"
	StoryImageModeInterval = "interval"
)

// Store manages interactive story data inside a workspace.
type Store struct {
	root    string
	novaDir string
	mu      sync.Mutex
}

// NewStore creates an interactive store rooted at the workspace directory.
func NewStore(root string) *Store {
	return &Store{root: root}
}

// NewStoreWithNovaDir creates an interactive store that can resolve reusable
// director modules from the workspace .denova directory.
func NewStoreWithNovaDir(root, novaDir string) *Store {
	return &Store{root: root, novaDir: strings.TrimSpace(novaDir)}
}

// Root returns the workspace root.
func (s *Store) Root() string {
	return s.root
}

func (s *Store) Index() (Index, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readIndexLocked()
}

// SelectStory persists the workspace-wide story selection. The index is the
// shared source of truth used by every browser connected to this workspace.
func (s *Store) SelectStory(storyID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	storyID = strings.TrimSpace(storyID)
	index, err := s.readIndexLocked()
	if err != nil {
		return err
	}
	for _, story := range index.Stories {
		if story.ID != storyID {
			continue
		}
		if index.CurrentStoryID == storyID {
			return nil
		}
		index.CurrentStoryID = storyID
		return s.writeIndexLocked(index)
	}
	return fmt.Errorf("故事不存在 / Story not found: %s", storyID)
}

func (s *Store) CreateStory(req CreateStoryRequest) (StorySummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.storyDir(), 0o755); err != nil {
		return StorySummary{}, fmt.Errorf("创建互动故事目录失败: %w", err)
	}
	index, err := s.readIndexLocked()
	if err != nil {
		return StorySummary{}, err
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = defaultStoryTitle(index.Stories)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	stateSchemaPolicy := cloneStoryStateSchemaPolicy(req.StateSchemaPolicy)
	directorRunPolicy := cloneStoryDirectorRunPolicy(req.DirectorRunPolicy)
	if directorRunPolicy != nil {
		if err := ValidateStoryDirectorRunPolicy(*directorRunPolicy); err != nil {
			return StorySummary{}, err
		}
	}
	story := StorySummary{
		ID:                newID("st"),
		Title:             title,
		Origin:            strings.TrimSpace(req.Origin),
		StoryTellerID:     strings.TrimSpace(req.StoryTellerID),
		StoryDirectorID:   NormalizeStoryDirectorID(req.StoryDirectorID),
		DirectorRunPolicy: cloneStoryDirectorRunPolicy(directorRunPolicy),
		ModuleRefs:        cloneStoryDirectorModuleRefs(req.ModuleRefs),
		ReplyTargetChars:  normalizeStoryReplyTargetChars(req.ReplyTargetChars),
		ChoiceCount:       normalizeStoryChoiceCount(req.ChoiceCount),
		Opening:           normalizeStoryOpeningConfig(req.Opening),
		ImageSettings:     normalizeStoryImageSettings(req.ImageSettings),
		StateSchemaPolicy: cloneStoryStateSchemaPolicy(stateSchemaPolicy),
		CreatedAt:         now,
		UpdatedAt:         now,
		Branches:          1,
	}
	if err := validateStoryChoiceCount(story.ChoiceCount); err != nil {
		return StorySummary{}, err
	}
	if story.StoryTellerID == "" {
		story.StoryTellerID = "classic"
	}
	if story.StoryDirectorID == "" {
		story.StoryDirectorID = DefaultStoryDirectorID
	}

	meta := StoryMeta{
		V:                 schemaVersion,
		Type:              StoryEventTypeMeta,
		StoryID:           story.ID,
		Title:             story.Title,
		Origin:            story.Origin,
		StoryTellerID:     story.StoryTellerID,
		StoryDirectorID:   story.StoryDirectorID,
		DirectorRunPolicy: cloneStoryDirectorRunPolicy(story.DirectorRunPolicy),
		ModuleRefs:        cloneStoryDirectorModuleRefs(story.ModuleRefs),
		ReplyTargetChars:  story.ReplyTargetChars,
		ChoiceCount:       story.ChoiceCount,
		Opening:           story.Opening,
		ImageSettings:     story.ImageSettings,
		StateSchemaPolicy: cloneStoryStateSchemaPolicy(stateSchemaPolicy),
		InitialTraitRolls: append([]InitialActorTraitRoll(nil), req.InitialTraitRolls...),
		CurrentBranch:     "main",
		Branches: map[string]BranchMeta{
			"main": {CreatedAt: now},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if req.StateSchemaInitialization != nil {
		initialization := *req.StateSchemaInitialization
		initialization.UpdatedAt = now
		if initialization.Status == StateSchemaInitializationReady {
			initialization.CompletedAt = now
		}
		meta.StateSchemaInitialization = &initialization
	} else if storyStateSchemaPolicyRequiresOpeningDraft(stateSchemaPolicy) {
		meta.StateSchemaInitialization = &StateSchemaInitializationStatus{
			Mode:         stateSchemaPolicy.Mode,
			Status:       StateSchemaInitializationWaitingOpening,
			BaseRevision: 1,
			UpdatedAt:    now,
		}
	} else {
		meta.StateSchemaInitialization = &StateSchemaInitializationStatus{
			Mode:           StoryStateSchemaModeFixedTemplate,
			Status:         StateSchemaInitializationReady,
			Outcome:        "fixed",
			BaseRevision:   1,
			TargetRevision: 1,
			CompletedAt:    now,
			UpdatedAt:      now,
		}
	}
	actorState := StoryDirectorActorStateSystem{}
	trpgSystem := StoryDirectorTRPGSystem{}
	if req.ActorState != nil {
		actorState = *req.ActorState
	} else if strings.TrimSpace(s.novaDir) != "" {
		director := s.storyDirectorForMeta(meta)
		actorState = director.ActorState
		trpgSystem = director.TRPGSystem
	}
	if req.TRPGSystem != nil {
		trpgSystem = *req.TRPGSystem
	}
	if !actorStateEmpty(actorState) {
		if err := validateActorStateSystem(actorState); err != nil {
			return StorySummary{}, fmt.Errorf("创建故事的状态系统无效 / Invalid state system for story creation: %w", err)
		}
		meta.ActorStateSchema = FreezeActorStateSchemaWithRules(actorState, trpgSystem, len(req.InitialStateOps) > 0)
		if meta.ActorStateSchema != nil && req.ActorStateAdaptation != nil {
			record := *req.ActorStateAdaptation
			meta.ActorStateSchema.Adaptation = &record
		}
	}
	if storyStateSchemaPolicyRequiresOpeningDraft(stateSchemaPolicy) && meta.ActorStateSchema == nil {
		return StorySummary{}, fmt.Errorf("状态结构初始化策略缺少可冻结的基础状态系统 / State schema policy requires a freezable base state system")
	}
	initialStateOps := normalizeStateOps(req.InitialStateOps)
	generatedOps := []StateOp(nil)
	initialActorOps := []ActorStateOp(nil)
	if meta.ActorStateSchema != nil && !storyStateSchemaPolicyRequiresOpeningDraft(stateSchemaPolicy) {
		generatedOps, initialActorOps, err = BuildActorStateInitialChanges(meta.ActorStateSchema.System, req.InitialTraitRolls)
		if err != nil {
			return StorySummary{}, err
		}
	}
	initialStateOps = normalizeStateOps(append(initialStateOps, generatedOps...))
	initialActorOps = normalizeActorStateOps(initialActorOps)
	if len(initialStateOps) > 0 || len(initialActorOps) > 0 {
		for _, op := range initialStateOps {
			if err := validateStateOp(op); err != nil {
				return StorySummary{}, err
			}
		}
		initialDeltaID := newID("sd")
		meta.Branches["main"] = BranchMeta{Head: initialDeltaID, CreatedAt: now}
		story.Events = 1
	}
	// Store callers that predate story-level policies create a fixed-schema
	// story, including stories that intentionally have no Actor State module.
	// Product entry points opt into dynamic opening modes explicitly.
	if stateSchemaPolicy == nil {
		stateSchemaPolicy = fixedStoryStateSchemaPolicy()
		story.StateSchemaPolicy = cloneStoryStateSchemaPolicy(stateSchemaPolicy)
		meta.StateSchemaPolicy = cloneStoryStateSchemaPolicy(stateSchemaPolicy)
	}
	normalizeFixedStoryStateSchemaInitialization(&meta)
	if err := validateStoryMeta(meta); err != nil {
		return StorySummary{}, err
	}
	events := []any{meta}
	if len(initialStateOps) > 0 || len(initialActorOps) > 0 {
		events = append(events, newStateDeltaEventWithActorOps(meta.Branches["main"].Head, "", "main", now, initialStateOps, initialActorOps))
	}
	if err := writeJSONL(s.storyPath(story.ID), events); err != nil {
		return StorySummary{}, err
	}
	seed := DirectorPlanSeed{Templates: DefaultStoryDirectorPlanningTemplates(), BranchPlanningTurns: defaultBranchPlanningTurns, Source: "story_create"}
	if req.DirectorPlanSeed != nil {
		seed = *req.DirectorPlanSeed
		if seed.Source == "" {
			seed.Source = "story_create"
		}
	}
	if err := s.seedDirectorPlanLocked(story.ID, "main", meta, seed); err != nil {
		_ = os.Remove(s.storyPath(story.ID))
		_ = os.RemoveAll(s.directorPlanBranchDir(story.ID, "main"))
		return StorySummary{}, err
	}

	index.CurrentStoryID = story.ID
	index.Stories = append(index.Stories, story)
	if err := s.writeIndexLocked(index); err != nil {
		return StorySummary{}, err
	}
	return story, nil
}

func (s *Store) UpdateStory(storyID string, req UpdateStoryRequest) (StorySummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return StorySummary{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if title := strings.TrimSpace(req.Title); title != "" {
		meta.Title = title
	}
	if req.Origin != nil {
		meta.Origin = strings.TrimSpace(*req.Origin)
	}
	if tellerID := strings.TrimSpace(req.StoryTellerID); tellerID != "" {
		meta.StoryTellerID = tellerID
	}
	if directorID := NormalizeStoryDirectorID(req.StoryDirectorID); directorID != "" {
		meta.StoryDirectorID = directorID
		meta.ModuleRefs = cloneStoryDirectorModuleRefs(req.ModuleRefs)
	} else if req.ModuleRefs != nil {
		meta.ModuleRefs = cloneStoryDirectorModuleRefs(req.ModuleRefs)
	}
	if req.DirectorRunPolicy != nil {
		policy := NormalizeStoryDirectorRunPolicy(*req.DirectorRunPolicy)
		if err := ValidateStoryDirectorRunPolicy(policy); err != nil {
			return StorySummary{}, err
		}
		meta.DirectorRunPolicy = &policy
	}
	if req.ReplyTargetChars != nil {
		if *req.ReplyTargetChars <= 0 {
			return StorySummary{}, fmt.Errorf("互动故事单轮目标字数必须大于 0")
		}
		meta.ReplyTargetChars = *req.ReplyTargetChars
	}
	if req.ChoiceCount != nil {
		if err := validateStoryChoiceCount(*req.ChoiceCount); err != nil {
			return StorySummary{}, err
		}
		meta.ChoiceCount = *req.ChoiceCount
	}
	if req.Opening != nil {
		meta.Opening = normalizeStoryOpeningConfig(*req.Opening)
	}
	if req.ImageSettings != nil {
		meta.ImageSettings = normalizeStoryImageSettings(*req.ImageSettings)
	}
	rebuiltEvents := []any(nil)
	rebuiltEventCount := -1
	if req.StateSchemaPolicy != nil {
		if storyContainsTurn(lines) {
			return StorySummary{}, fmt.Errorf("首回合提交后不能修改状态结构初始化策略")
		}
		if len(meta.Branches) != 1 {
			return StorySummary{}, fmt.Errorf("故事已有多个分支，不能重建开局状态结构")
		}
		policy := NormalizeStoryStateSchemaPolicy(*req.StateSchemaPolicy)
		if (req.ActorState == nil || actorStateEmpty(*req.ActorState)) && storyStateSchemaPolicyRequiresOpeningDraft(&policy) {
			return StorySummary{}, fmt.Errorf("状态结构初始化策略缺少可冻结的基础状态系统")
		}
		meta.StateSchemaPolicy = &policy
		trpgSystem := StoryDirectorTRPGSystem{}
		if req.TRPGSystem != nil {
			trpgSystem = *req.TRPGSystem
		}
		if req.ActorState == nil || actorStateEmpty(*req.ActorState) {
			meta.ActorStateSchema = nil
		} else {
			if err := validateActorStateSystem(*req.ActorState); err != nil {
				return StorySummary{}, fmt.Errorf("更新故事的状态系统无效 / Invalid state system for story update: %w", err)
			}
			meta.ActorStateSchema = FreezeActorStateSchemaWithRules(*req.ActorState, trpgSystem, false)
		}
		if req.StateSchemaInitialization != nil {
			initialization := *req.StateSchemaInitialization
			initialization.UpdatedAt = now
			if initialization.Status == StateSchemaInitializationReady {
				initialization.CompletedAt = now
			}
			meta.StateSchemaInitialization = &initialization
		} else if storyStateSchemaPolicyRequiresOpeningDraft(&policy) {
			meta.StateSchemaInitialization = &StateSchemaInitializationStatus{Mode: policy.Mode, Status: StateSchemaInitializationWaitingOpening, BaseRevision: 1, UpdatedAt: now}
		} else {
			meta.StateSchemaInitialization = &StateSchemaInitializationStatus{Mode: policy.Mode, Status: StateSchemaInitializationReady, Outcome: "fixed", BaseRevision: 1, TargetRevision: 1, CompletedAt: now, UpdatedAt: now}
		}
		lines = nil
		branch := meta.Branches[meta.CurrentBranch]
		branch.Head = ""
		meta.Branches[meta.CurrentBranch] = branch
		rebuiltEventCount = 0
		if meta.ActorStateSchema != nil && !storyStateSchemaPolicyRequiresOpeningDraft(&policy) {
			initialOps, initialActorOps, err := BuildActorStateInitialChanges(meta.ActorStateSchema.System, meta.InitialTraitRolls)
			if err != nil {
				return StorySummary{}, err
			}
			initialOps = normalizeStateOps(initialOps)
			initialActorOps = normalizeActorStateOps(initialActorOps)
			if len(initialOps) > 0 || len(initialActorOps) > 0 {
				deltaID := newID("sd")
				branch.Head = deltaID
				meta.Branches[meta.CurrentBranch] = branch
				rebuiltEvents = append(rebuiltEvents, newStateDeltaEventWithActorOps(deltaID, "", meta.CurrentBranch, now, initialOps, initialActorOps))
				rebuiltEventCount = 1
			}
		}
	}
	meta.UpdatedAt = now
	if err := s.rewriteStoryLocked(storyID, meta, lines, rebuiltEvents...); err != nil {
		return StorySummary{}, err
	}
	index, err := s.readIndexLocked()
	if err != nil {
		return StorySummary{}, err
	}
	for i := range index.Stories {
		if index.Stories[i].ID == storyID {
			index.Stories[i].Title = meta.Title
			index.Stories[i].Origin = meta.Origin
			index.Stories[i].StoryTellerID = meta.StoryTellerID
			index.Stories[i].StoryDirectorID = normalizedStoryDirectorID(meta.StoryDirectorID)
			index.Stories[i].DirectorRunPolicy = cloneStoryDirectorRunPolicy(meta.DirectorRunPolicy)
			index.Stories[i].ModuleRefs = cloneStoryDirectorModuleRefs(meta.ModuleRefs)
			index.Stories[i].ReplyTargetChars = meta.ReplyTargetChars
			index.Stories[i].ChoiceCount = meta.ChoiceCount
			index.Stories[i].Opening = meta.Opening
			index.Stories[i].ImageSettings = meta.ImageSettings
			index.Stories[i].StateSchemaPolicy = cloneStoryStateSchemaPolicy(meta.StateSchemaPolicy)
			if rebuiltEventCount >= 0 {
				index.Stories[i].Events = rebuiltEventCount
			}
			index.Stories[i].UpdatedAt = now
			if err := s.writeIndexLocked(index); err != nil {
				return StorySummary{}, err
			}
			return index.Stories[i], nil
		}
	}
	return StorySummary{}, fmt.Errorf("故事不存在: %s", storyID)
}

func (s *Store) DeleteStory(storyID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	index, err := s.readIndexLocked()
	if err != nil {
		return err
	}
	next := index.Stories[:0]
	removed := false
	for _, story := range index.Stories {
		if story.ID == storyID {
			removed = true
			continue
		}
		next = append(next, story)
	}
	if !removed {
		return fmt.Errorf("故事不存在: %s", storyID)
	}
	index.Stories = next
	if index.CurrentStoryID == storyID {
		index.CurrentStoryID = ""
		if len(index.Stories) > 0 {
			index.CurrentStoryID = index.Stories[0].ID
		}
	}
	if err := os.Remove(s.storyPath(storyID)); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Remove(s.actorStateSchemaPath(storyID)); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Remove(s.usagePath(storyID)); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.RemoveAll(filepath.Join(s.root, "interactive", "stories", storyID)); err != nil {
		return err
	}
	return s.writeIndexLocked(index)
}

func (s *Store) StoryContext(storyID, branchID string) (StoryContext, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return StoryContext{}, err
	}
	snapshot, err := snapshotFromLines(storyID, branchID, meta, lines)
	if err != nil {
		return StoryContext{}, err
	}
	if plan, planErr := s.readDirectorPlanLocked(storyID, snapshot.BranchID); planErr == nil {
		snapshot.DirectorPlan = &plan
		status := DirectorPlanStatusFromPlan(plan, len(snapshot.Turns) > 0)
		snapshot.DirectorPlanStatus = &status
	}
	usageEvents, err := s.readTokenUsageEventsLocked(storyID, snapshot.BranchID)
	if err != nil {
		return StoryContext{}, err
	}
	snapshot.TokenUsageEvents = usageEvents
	return StoryContext{Meta: meta, Snapshot: snapshot}, nil
}

func (s *Store) AppendContextCompaction(storyID, branchID string, event ContextCompactionEvent) (ContextCompactionEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return ContextCompactionEvent{}, err
	}
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	branch, ok := meta.Branches[branchID]
	if !ok {
		return ContextCompactionEvent{}, fmt.Errorf("分支不存在: %s", branchID)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if event.ID == "" {
		event.ID = newID("cc")
	}
	event.V = schemaVersion
	event.Type = StoryEventTypeCompaction
	event.ParentID = branch.Head
	event.BranchID = branchID
	if event.Ts == "" {
		event.Ts = now
	}
	if event.Epoch <= 0 {
		event.Epoch = nextContextCompactionEpoch(lines, branch.Head)
	}
	branch.Head = event.ID
	meta.Branches[branchID] = branch
	meta.UpdatedAt = now
	if err := s.rewriteStoryLocked(storyID, meta, lines, event); err != nil {
		return ContextCompactionEvent{}, err
	}
	if err := s.touchIndexLocked(storyID, now, 1); err != nil {
		return ContextCompactionEvent{}, err
	}
	return event, nil
}

func (s *Store) AppendContextCompactionRemoval(storyID, branchID string, event ContextCompactionRemovalEvent) (ContextCompactionRemovalEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return ContextCompactionRemovalEvent{}, err
	}
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	branch, ok := meta.Branches[branchID]
	if !ok {
		return ContextCompactionRemovalEvent{}, fmt.Errorf("分支不存在: %s", branchID)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if event.ID == "" {
		event.ID = newID("ccr")
	}
	event.V = schemaVersion
	event.Type = StoryEventTypeCompactionRemoved
	event.ParentID = branch.Head
	event.BranchID = branchID
	if event.Ts == "" {
		event.Ts = now
	}
	branch.Head = event.ID
	meta.Branches[branchID] = branch
	meta.UpdatedAt = now
	if err := s.rewriteStoryLocked(storyID, meta, lines, event); err != nil {
		return ContextCompactionRemovalEvent{}, err
	}
	if err := s.touchIndexLocked(storyID, now, 1); err != nil {
		return ContextCompactionRemovalEvent{}, err
	}
	return event, nil
}

func (s *Store) AppendTurn(storyID string, req AppendTurnRequest) (TurnEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return TurnEvent{}, err
	}
	branchID := req.BranchID
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	branch, ok := meta.Branches[branchID]
	if !ok {
		return TurnEvent{}, fmt.Errorf("分支不存在: %s", branchID)
	}
	if branchIsTerminal(lines, branch.Head) {
		return TurnEvent{}, fmt.Errorf("当前分支已终局，请从历史回合创建新分支后继续")
	}
	parentID := any(nil)
	if branch.Head != "" {
		parentID = branch.Head
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	event := TurnEvent{
		V:                    schemaVersion,
		Type:                 StoryEventTypeTurn,
		ID:                   newID("ev"),
		ParentID:             parentID,
		BranchID:             branchID,
		Ts:                   now,
		User:                 req.User,
		Narrative:            req.Narrative,
		Thinking:             strings.TrimSpace(req.Thinking),
		DisplayEvents:        sanitizeDisplayEvents(req.DisplayEvents),
		ModelContextMessages: sanitizeModelContextMessages(req.ModelContextMessages),
		Flags:                map[string]bool{"pinned": false, "locked": false},
	}
	branch.Head = event.ID
	meta.Branches[branchID] = branch
	meta.UpdatedAt = now
	if err := s.rewriteStoryLocked(storyID, meta, lines, event); err != nil {
		return TurnEvent{}, err
	}
	if err := s.touchIndexLocked(storyID, now, 1); err != nil {
		return TurnEvent{}, err
	}
	return event, nil
}

func (s *Store) AppendTurnWithState(storyID string, req AppendTurnWithStateRequest) (TurnEvent, *StateDeltaEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return TurnEvent{}, nil, err
	}
	branchID := req.BranchID
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	branch, ok := meta.Branches[branchID]
	if !ok {
		return TurnEvent{}, nil, fmt.Errorf("分支不存在: %s", branchID)
	}
	if branchIsTerminal(lines, branch.Head) {
		return TurnEvent{}, nil, fmt.Errorf("当前分支已终局，请从历史回合创建新分支后继续")
	}
	if req.ExpectedParentID != nil && branch.Head != strings.TrimSpace(*req.ExpectedParentID) {
		return TurnEvent{}, nil, fmt.Errorf("当前分支已前进，拒绝提交基于旧版本的回合: expected_parent=%s current_head=%s", strings.TrimSpace(*req.ExpectedParentID), branch.Head)
	}
	parentID := any(nil)
	if branch.Head != "" {
		parentID = branch.Head
	}
	path, _ := eventPath(branch.Head, eventsByID(lines))
	state := stateFromPath(path)
	director := s.storyDirectorForMeta(meta)
	actorState := actorStateSystemFromSnapshot(meta.ActorStateSchema, director.ActorState)
	applyLegacyActorStateAliases(state, meta.ActorStateSchema)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	terminal := (req.TerminalOutcome != nil && req.TerminalOutcome.Terminal) || (req.RuleResolution != nil && req.RuleResolution.TerminalCandidate != nil)
	turnResult := normalizeTurnResultPointer(req.TurnResult, meta.ChoiceCount, terminal)
	if req.TurnResult != nil && turnResult == nil {
		return TurnEvent{}, nil, fmt.Errorf("TurnResult 未通过校验")
	}
	turn := TurnEvent{
		V:                    schemaVersion,
		Type:                 StoryEventTypeTurn,
		ID:                   newID("ev"),
		ParentID:             parentID,
		BranchID:             branchID,
		Ts:                   now,
		User:                 req.User,
		Narrative:            req.Narrative,
		Thinking:             strings.TrimSpace(req.Thinking),
		RunID:                strings.TrimSpace(req.RunID),
		AgentKind:            strings.TrimSpace(req.AgentKind),
		DisplayEvents:        sanitizeDisplayEvents(req.DisplayEvents),
		ModelContextMessages: sanitizeModelContextMessages(req.ModelContextMessages),
		RuleResolution:       normalizeRuleResolutionPointer(req.RuleResolution),
		TurnResult:           turnResult,
		TerminalOutcome:      normalizeTerminalOutcomePointer(req.TerminalOutcome),
		Flags:                map[string]bool{"pinned": false, "locked": false},
	}
	actorState, openingOps, openingActorOps, err := prepareOpeningGameStateSchemaCommit(&meta, lines, state, actorState, branchID, turn.ID, now, req.StateSchemaProposal)
	if err != nil {
		return TurnEvent{}, nil, err
	}
	ops := normalizeStateOps(append(openingOps, req.Ops...))
	actorOps := normalizeActorStateOps(append(openingActorOps, req.ActorOps...))
	if turn.TurnResult != nil && len(turn.TurnResult.StateUpdates) > 0 {
		compiled, err := CompileTurnStateUpdates(actorState, state, turn.TurnResult.StateUpdates, TurnStateUpdateCompileOptions{
			SourceTurnID:             turn.ID,
			RuleResolution:           turn.RuleResolution,
			RuleStateConsumptionMode: director.Strategy.RuleStateConsumptionMode,
		})
		if err != nil {
			return TurnEvent{}, nil, fmt.Errorf("TurnResult state_updates 校验失败: %w", err)
		}
		turn.TurnResult.StateUpdates = compiled.Updates
		for i := range compiled.Ops {
			compiled.Ops[i].SourceKind = StateOpSourceTurnResult
			compiled.Ops[i].SourceID = turn.ID
			compiled.Ops[i].SourceTurnID = turn.ID
		}
		ops = append(ops, compiled.Ops...)
		for i := range compiled.ActorOps {
			compiled.ActorOps[i].SourceKind = StateOpSourceTurnResult
			compiled.ActorOps[i].SourceID = turn.ID
			compiled.ActorOps[i].SourceTurnID = turn.ID
		}
		actorOps = append(actorOps, compiled.ActorOps...)
	}
	if turn.RuleResolution != nil {
		ruleOps, ruleActorOps := applyRuleStateConsumptionV2(state, actorState, turn.ID, turn.RuleResolution, director.Strategy.RuleStateConsumptionMode)
		ops = append(ops, ruleOps...)
		actorOps = append(actorOps, ruleActorOps...)
	}
	branch.Head = turn.ID

	var delta *StateDeltaEvent
	actorOps = normalizeActorStateOps(actorOps)
	if len(ops) > 0 || len(actorOps) > 0 {
		for _, op := range ops {
			if err := validateStateOp(op); err != nil {
				return TurnEvent{}, nil, err
			}
		}
		for _, op := range actorOps {
			if err := validateActorStateOp(op); err != nil {
				return TurnEvent{}, nil, err
			}
		}
		stateDelta := newStateDeltaWithActorOps(ops, actorOps)
		turn.StateDelta = &stateDelta
		turn.StateStatus = "ready"
		stateDeltaEvent := newStateDeltaEventWithActorOps(turn.ID, parentIDString(parentID), branchID, now, ops, actorOps)
		delta = &stateDeltaEvent
	} else if turn.TurnResult != nil {
		turn.StateStatus = "ready"
	} else {
		turn.StateStatus = "pending"
	}

	meta.Branches[branchID] = branch
	meta.UpdatedAt = now
	newEvents := []any{turn}
	if err := s.rewriteStoryLocked(storyID, meta, lines, newEvents...); err != nil {
		return TurnEvent{}, nil, err
	}
	if err := s.touchIndexLocked(storyID, now, 1); err != nil {
		return TurnEvent{}, nil, err
	}
	return turn, delta, nil
}

// AppendTurnDisplayEvent appends a display-only event to an existing turn.
// The event is kept out of future model context and does not move branch head.
func (s *Store) AppendTurnDisplayEvent(storyID, branchID, turnID string, event DisplayEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	events := sanitizeDisplayEvents([]DisplayEvent{event})
	if len(events) == 0 {
		return nil
	}
	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return err
	}
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	branch, ok := meta.Branches[branchID]
	if !ok {
		return fmt.Errorf("分支不存在: %s", branchID)
	}
	turnID = strings.TrimSpace(turnID)
	if turnID == "" {
		return fmt.Errorf("展示事件缺少所属回合")
	}
	_, pathSet := eventPath(branch.Head, eventsByID(lines))
	if !pathSet[turnID] {
		return fmt.Errorf("展示事件回合不属于当前分支路径: %s", turnID)
	}
	updated := false
	for i := range lines {
		raw := lines[i].Raw
		if lines[i].Envelope.ID != turnID || lines[i].Envelope.Type != StoryEventTypeTurn {
			continue
		}
		var turn TurnEvent
		if err := mapToStruct(raw, &turn); err != nil {
			return err
		}
		raw["display_events"] = appendDisplayEvent(turn.DisplayEvents, events[0])
		updated = true
		break
	}
	if !updated {
		return fmt.Errorf("展示事件所属回合不存在: %s", turnID)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	meta.UpdatedAt = now
	if err := s.rewriteStoryLocked(storyID, meta, lines); err != nil {
		return err
	}
	return s.touchIndexLocked(storyID, now, 0)
}

func (s *Store) RewindToTurnParent(storyID string, req RewindTurnRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	turnID := strings.TrimSpace(req.TurnID)
	if turnID == "" {
		return fmt.Errorf("回合 ID 不能为空")
	}
	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return err
	}
	branchID := req.BranchID
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	branch, ok := meta.Branches[branchID]
	if !ok {
		return fmt.Errorf("分支不存在: %s", branchID)
	}
	events := eventsByID(lines)
	path, pathSet := eventPath(branch.Head, events)
	if !pathSet[turnID] {
		return fmt.Errorf("只能编辑当前剧情路径上的回合: %s", turnID)
	}
	var target *StoryEventRecord
	for i := range path {
		if path[i].Envelope.ID == turnID && path[i].Envelope.Type == StoryEventTypeTurn {
			target = &path[i]
			break
		}
	}
	if target == nil {
		return fmt.Errorf("回合不存在: %s", turnID)
	}
	branch.Head = parentIDFromRaw(target.Raw)
	meta.Branches[branchID] = branch
	now := time.Now().UTC().Format(time.RFC3339Nano)
	meta.UpdatedAt = now
	if err := s.rewriteStoryLocked(storyID, meta, lines); err != nil {
		return err
	}
	return s.touchIndexLocked(storyID, now, 0)
}

func (s *Store) SwitchTurnVersion(storyID string, req SwitchTurnVersionRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	turnID := strings.TrimSpace(req.TurnID)
	versionTurnID := strings.TrimSpace(req.VersionTurnID)
	if turnID == "" || versionTurnID == "" {
		return fmt.Errorf("回合版本参数不能为空")
	}
	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return err
	}
	branchID := req.BranchID
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	branch, ok := meta.Branches[branchID]
	if !ok {
		return fmt.Errorf("分支不存在: %s", branchID)
	}
	events := eventsByID(lines)
	path, pathSet := eventPath(branch.Head, events)
	if !pathSet[turnID] {
		return fmt.Errorf("只能切换当前剧情路径上的回合版本: %s", turnID)
	}
	currentIndex := -1
	var current *StoryEventRecord
	for i := range path {
		if path[i].Envelope.ID == turnID && path[i].Envelope.Type == StoryEventTypeTurn {
			current = &path[i]
			currentIndex = i
			break
		}
	}
	if current == nil {
		return fmt.Errorf("回合不存在: %s", turnID)
	}
	target, ok := events[versionTurnID]
	if !ok {
		return fmt.Errorf("目标版本不存在: %s", versionTurnID)
	}
	if target.Envelope.Type != StoryEventTypeTurn {
		return fmt.Errorf("目标版本不是互动回合: %s", versionTurnID)
	}
	if target.Envelope.BranchID != branchID {
		return fmt.Errorf("目标版本不属于当前分支: %s", versionTurnID)
	}
	if parentIDFromRaw(target.Raw) != parentIDFromRaw(current.Raw) {
		return fmt.Errorf("只能在同一剧情位置切换版本")
	}

	nextHead := versionTurnID
	if currentIndex >= 0 && currentIndex < len(path)-1 {
		next := path[currentIndex+1]
		if err := reparentStoryEvent(lines, next, turnID, versionTurnID); err != nil {
			return err
		}
		nextHead = branch.Head
	}
	branch.Head = nextHead
	meta.Branches[branchID] = branch
	now := time.Now().UTC().Format(time.RFC3339Nano)
	meta.UpdatedAt = now
	if err := s.rewriteStoryLocked(storyID, meta, lines); err != nil {
		return err
	}
	return s.touchIndexLocked(storyID, now, 0)
}

func reparentStoryEvent(lines []StoryEventRecord, child StoryEventRecord, oldParentID, newParentID string) error {
	if parentIDFromRaw(child.Raw) != oldParentID {
		return fmt.Errorf("当前剧情路径不连续，无法切换版本: %s", child.Envelope.ID)
	}
	for i := range lines {
		if lines[i].Envelope.ID != child.Envelope.ID || lines[i].Envelope.Type != child.Envelope.Type {
			continue
		}
		if parentIDFromRaw(lines[i].Raw) != oldParentID {
			continue
		}
		lines[i].Raw["parent_id"] = newParentID
		lines[i].Envelope.ParentID = newParentID
		return nil
	}
	return fmt.Errorf("剧情后续节点不存在，无法切换版本: %s", child.Envelope.ID)
}

func (s *Store) AppendStateDelta(storyID string, req AppendStateDeltaRequest) (StateDeltaEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(req.Ops) == 0 && len(req.ActorOps) == 0 {
		return StateDeltaEvent{}, fmt.Errorf("状态变化不能为空")
	}

	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return StateDeltaEvent{}, err
	}
	branchID := req.BranchID
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	branch, ok := meta.Branches[branchID]
	if !ok {
		return StateDeltaEvent{}, fmt.Errorf("分支不存在: %s", branchID)
	}
	parentID := strings.TrimSpace(req.ParentID)
	if parentID == "" {
		parentID = branch.Head
	}
	if parentID == "" {
		return StateDeltaEvent{}, fmt.Errorf("状态变化缺少所属回合")
	}
	if parentID != branch.Head {
		return StateDeltaEvent{}, fmt.Errorf("状态变化所属回合不是当前分支头: turn=%s head=%s", parentID, branch.Head)
	}
	ops := normalizeStateOps(req.Ops)
	actorOps := normalizeActorStateOps(req.ActorOps)
	if len(ops) == 0 && len(actorOps) == 0 {
		return StateDeltaEvent{}, fmt.Errorf("状态变化不能为空")
	}
	for _, op := range ops {
		if err := validateStateOp(op); err != nil {
			return StateDeltaEvent{}, err
		}
	}
	for _, op := range actorOps {
		if err := validateActorStateOp(op); err != nil {
			return StateDeltaEvent{}, err
		}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	event := newStateDeltaEventWithActorOps(parentID, parentID, branchID, now, ops, actorOps)
	updated := false
	for i := range lines {
		raw := lines[i].Raw
		if lines[i].Envelope.ID != parentID || lines[i].Envelope.Type != StoryEventTypeTurn {
			continue
		}
		var turn TurnEvent
		if err := mapToStruct(raw, &turn); err != nil {
			return StateDeltaEvent{}, err
		}
		nextOps := append([]StateOp(nil), ops...)
		nextActorOps := append([]ActorStateOp(nil), actorOps...)
		if turn.StateDelta != nil && len(turn.StateDelta.Ops) > 0 {
			nextOps = append(append([]StateOp(nil), turn.StateDelta.Ops...), nextOps...)
		}
		if turn.StateDelta != nil && len(turn.StateDelta.ActorOps) > 0 {
			nextActorOps = append(append([]ActorStateOp(nil), turn.StateDelta.ActorOps...), nextActorOps...)
		}
		raw["state_delta"] = newStateDeltaWithActorOps(nextOps, nextActorOps)
		raw["state_status"] = "ready"
		delete(raw, "state_error")
		updated = true
		break
	}
	if !updated {
		return StateDeltaEvent{}, fmt.Errorf("状态变化所属回合不存在: %s", parentID)
	}
	meta.UpdatedAt = now
	if err := s.rewriteStoryLocked(storyID, meta, lines); err != nil {
		return StateDeltaEvent{}, err
	}
	if err := s.touchIndexLocked(storyID, now, 0); err != nil {
		return StateDeltaEvent{}, err
	}
	return event, nil
}

func (s *Store) MarkStateFailed(storyID string, req MarkStateFailedRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return err
	}
	branchID := req.BranchID
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	if _, ok := meta.Branches[branchID]; !ok {
		return fmt.Errorf("分支不存在: %s", branchID)
	}
	parentID := strings.TrimSpace(req.ParentID)
	if parentID == "" {
		return fmt.Errorf("状态失败标记缺少所属回合")
	}
	errText := strings.TrimSpace(req.Error)
	if errText == "" {
		errText = "状态生成失败"
	}
	updated := false
	for _, record := range lines {
		raw := record.Raw
		if record.Envelope.ID != parentID || record.Envelope.Type != StoryEventTypeTurn {
			continue
		}
		raw["state_status"] = "failed"
		raw["state_error"] = errText
		updated = true
		break
	}
	if !updated {
		return fmt.Errorf("状态失败标记所属回合不存在: %s", parentID)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	meta.UpdatedAt = now
	if err := s.rewriteStoryLocked(storyID, meta, lines); err != nil {
		return err
	}
	return s.touchIndexLocked(storyID, now, 0)
}

func (s *Store) RerollRuleResolution(storyID, resolutionID string, req RuleResolutionRerollRequest) (RuleResolution, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	resolutionID = strings.TrimSpace(resolutionID)
	if resolutionID == "" {
		return RuleResolution{}, fmt.Errorf("规则结算 ID 不能为空")
	}
	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return RuleResolution{}, err
	}
	branchID, branch, err := resolveBranch(meta, req.BranchID)
	if err != nil {
		return RuleResolution{}, err
	}
	path, pathSet := eventPath(branch.Head, eventsByID(lines))
	var target TurnEvent
	for _, record := range path {
		if record.Envelope.Type != StoryEventTypeTurn {
			continue
		}
		var turn TurnEvent
		if err := mapToStruct(record.Raw, &turn); err != nil {
			continue
		}
		if strings.TrimSpace(req.TurnID) != "" && turn.ID != strings.TrimSpace(req.TurnID) {
			continue
		}
		if turn.RuleResolution != nil && turn.RuleResolution.ID == resolutionID {
			target = turn
			break
		}
	}
	if target.ID == "" {
		return RuleResolution{}, fmt.Errorf("当前分支路径中未找到规则结算: %s", resolutionID)
	}
	request := NormalizeTurnCheckRequest(target.RuleResolution.Request)
	state := stateBeforeTurn(path, target.ID)
	director := s.storyDirectorForMeta(meta)
	actorState := actorStateSystemFromSnapshot(meta.ActorStateSchema, director.ActorState)
	applyLegacyActorStateAliases(state, meta.ActorStateSchema)
	next, err := ResolveTurnRulesWithDirector(storyID, branchID, state, director, request)
	if err != nil {
		return RuleResolution{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	next.CreatedAt = now
	next.ID = newID("rr")
	ruleOps, ruleActorOps := applyRuleStateConsumptionV2(state, actorState, target.ID, &next, director.Strategy.RuleStateConsumptionMode)
	terminalOutcome := terminalOutcomeFromRuleResolution(next, target.ID, target.Narrative)
	updated := false
	for i := range lines {
		if lines[i].Envelope.ID != target.ID || !pathSet[target.ID] {
			continue
		}
		lines[i].Raw["rule_resolution"] = next
		existingOps := []StateOp{}
		existingActorOps := []ActorStateOp{}
		if target.StateDelta != nil {
			existingOps = append(existingOps, target.StateDelta.Ops...)
			existingActorOps = append(existingActorOps, target.StateDelta.ActorOps...)
		}
		nextOps := append(removeRuleResolutionStateOps(existingOps, target.RuleResolution.ID), ruleOps...)
		nextActorOps := append(removeRuleResolutionActorOps(existingActorOps, target.RuleResolution.ID), ruleActorOps...)
		if len(nextOps) > 0 || len(nextActorOps) > 0 {
			for _, op := range nextOps {
				if err := validateStateOp(op); err != nil {
					return RuleResolution{}, err
				}
			}
			lines[i].Raw["state_delta"] = newStateDeltaWithActorOps(nextOps, nextActorOps)
			lines[i].Raw["state_status"] = "ready"
			delete(lines[i].Raw, "state_error")
		} else {
			delete(lines[i].Raw, "state_delta")
			lines[i].Raw["state_status"] = "pending"
			delete(lines[i].Raw, "state_error")
		}
		if terminalOutcome != nil {
			lines[i].Raw["terminal_outcome"] = terminalOutcome
		} else {
			delete(lines[i].Raw, "terminal_outcome")
		}
		updated = true
		break
	}
	if !updated {
		return RuleResolution{}, fmt.Errorf("规则结算所属回合不存在: %s", target.ID)
	}
	meta.UpdatedAt = now
	if err := s.rewriteStoryLocked(storyID, meta, lines); err != nil {
		return RuleResolution{}, err
	}
	if err := s.touchIndexLocked(storyID, now, 0); err != nil {
		return RuleResolution{}, err
	}
	return next, nil
}

func (s *Store) CreateBranch(storyID string, req CreateBranchRequest) (BranchSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return BranchSummary{}, err
	}
	parentID := strings.TrimSpace(req.ParentEventID)
	if parentID == "" {
		return BranchSummary{}, fmt.Errorf("父事件不能为空")
	}
	fromBranch, ok := findEventBranch(lines, parentID)
	if !ok {
		return BranchSummary{}, fmt.Errorf("父事件不存在: %s", parentID)
	}
	branchID := "br_" + strings.TrimPrefix(newID(""), "_")
	now := time.Now().UTC().Format(time.RFC3339Nano)
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = "新分支"
	}
	meta.CurrentBranch = branchID
	meta.Branches[branchID] = BranchMeta{
		Head:      parentID,
		CreatedAt: now,
		From:      fromBranch,
		FromEvent: parentID,
		Title:     title,
	}
	meta.UpdatedAt = now
	event := BranchEvent{
		V:        schemaVersion,
		Type:     StoryEventTypeBranch,
		ID:       newID("ev"),
		ParentID: parentID,
		BranchID: branchID,
		From:     fromBranch,
		Ts:       now,
		Title:    title,
	}
	if err := s.cloneDirectorPlanForBranchLocked(storyID, fromBranch, branchID, title); err != nil {
		return BranchSummary{}, err
	}
	if err := s.rewriteStoryLocked(storyID, meta, lines, event); err != nil {
		_ = os.RemoveAll(s.directorPlanBranchDir(storyID, branchID))
		return BranchSummary{}, err
	}
	if err := s.updateIndexBranchesLocked(storyID, len(meta.Branches), now, 1); err != nil {
		return BranchSummary{}, err
	}
	return BranchSummary{ID: branchID, Head: parentID, From: fromBranch, FromEvent: parentID, Title: title, CreatedAt: now, Current: true}, nil
}

func (s *Store) SwitchBranch(storyID, branchID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return err
	}
	if _, ok := meta.Branches[branchID]; !ok {
		return fmt.Errorf("分支不存在: %s", branchID)
	}
	meta.CurrentBranch = branchID
	meta.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return s.rewriteStoryLocked(storyID, meta, lines)
}

func (s *Store) DeleteBranch(storyID, branchID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	branchID = strings.TrimSpace(branchID)
	if branchID == "" {
		return fmt.Errorf("分支不能为空")
	}
	if branchID == "main" {
		return fmt.Errorf("主线不能删除")
	}
	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return err
	}
	branch, ok := meta.Branches[branchID]
	if !ok {
		return fmt.Errorf("分支不存在: %s", branchID)
	}
	if branch.Head != branch.FromEvent {
		return fmt.Errorf("只能删除尚未产生独立剧情的空分支")
	}
	for id, candidate := range meta.Branches {
		if id != branchID && candidate.From == branchID {
			return fmt.Errorf("该分支已有子分支，不能删除")
		}
	}
	nextLines := make([]StoryEventRecord, 0, len(lines))
	removedEvents := 0
	for _, record := range lines {
		if record.Envelope.Type == StoryEventTypeBranch && record.Envelope.BranchID == branchID {
			removedEvents++
			continue
		}
		nextLines = append(nextLines, record)
	}
	if removedEvents == 0 {
		return fmt.Errorf("分支记录不存在: %s", branchID)
	}
	delete(meta.Branches, branchID)
	if meta.CurrentBranch == branchID {
		if branch.From != "" {
			meta.CurrentBranch = branch.From
		} else {
			meta.CurrentBranch = "main"
		}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	meta.UpdatedAt = now
	if err := s.rewriteStoryLocked(storyID, meta, nextLines); err != nil {
		return err
	}
	_ = os.RemoveAll(s.directorPlanBranchDir(storyID, branchID))
	return s.updateIndexBranchesLocked(storyID, len(meta.Branches), now, -removedEvents)
}

func (s *Store) Branches(storyID string) ([]BranchSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, _, err := s.readStoryLocked(storyID)
	if err != nil {
		return nil, err
	}
	return branchSummaries(meta), nil
}

func (s *Store) Snapshot(storyID, branchID string) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return Snapshot{}, err
	}
	snapshot, err := snapshotFromLines(storyID, branchID, meta, lines)
	if err != nil {
		return Snapshot{}, err
	}
	if plan, planErr := s.readDirectorPlanLocked(storyID, snapshot.BranchID); planErr == nil {
		snapshot.DirectorPlan = &plan
		status := DirectorPlanStatusFromPlan(plan, len(snapshot.Turns) > 0)
		snapshot.DirectorPlanStatus = &status
	}
	usageEvents, err := s.readTokenUsageEventsLocked(storyID, snapshot.BranchID)
	if err != nil {
		return Snapshot{}, err
	}
	snapshot.TokenUsageEvents = usageEvents
	return snapshot, nil
}

func findEventBranch(lines []StoryEventRecord, eventID string) (string, bool) {
	for _, record := range lines {
		if record.Envelope.ID != eventID {
			continue
		}
		return record.Envelope.BranchID, record.Envelope.BranchID != ""
	}
	return "", false
}

func branchIsTerminal(lines []StoryEventRecord, head string) bool {
	turn := latestTurnForBranchHead(lines, head)
	return turn != nil && turn.TerminalOutcome != nil && turn.TerminalOutcome.Terminal
}

func stateFromPath(path []StoryEventRecord) map[string]any {
	state := initialStoryState()
	for _, record := range path {
		switch record.Envelope.Type {
		case StoryEventTypeStateDelta:
			var delta StateDeltaEvent
			if err := mapToStruct(record.Raw, &delta); err == nil {
				for _, op := range delta.Ops {
					applyStateOp(state, op)
				}
				for _, op := range delta.ActorOps {
					applyActorStateOp(state, op)
				}
			}
		case StoryEventTypeTurn:
			var turn TurnEvent
			if err := mapToStruct(record.Raw, &turn); err == nil && turn.StateDelta != nil {
				for _, op := range turn.StateDelta.Ops {
					applyStateOp(state, op)
				}
				for _, op := range turn.StateDelta.ActorOps {
					applyActorStateOp(state, op)
				}
			}
		}
	}
	return state
}

func stateBeforeTurn(path []StoryEventRecord, turnID string) map[string]any {
	state := initialStoryState()
	for _, record := range path {
		if record.Envelope.ID == turnID {
			break
		}
		switch record.Envelope.Type {
		case StoryEventTypeStateDelta:
			var delta StateDeltaEvent
			if err := mapToStruct(record.Raw, &delta); err == nil {
				for _, op := range delta.Ops {
					applyStateOp(state, op)
				}
				for _, op := range delta.ActorOps {
					applyActorStateOp(state, op)
				}
			}
		case StoryEventTypeTurn:
			var turn TurnEvent
			if err := mapToStruct(record.Raw, &turn); err == nil && turn.StateDelta != nil {
				for _, op := range turn.StateDelta.Ops {
					applyStateOp(state, op)
				}
				for _, op := range turn.StateDelta.ActorOps {
					applyActorStateOp(state, op)
				}
			}
		}
	}
	return state
}

func (s *Store) storyDirectorForMeta(meta StoryMeta) StoryDirector {
	if strings.TrimSpace(s.novaDir) == "" {
		return DefaultStoryDirector()
	}
	directorID := normalizedStoryDirectorID(meta.StoryDirectorID)
	director, err := NewStoryDirectorLibrary(s.novaDir).Get(directorID)
	if err == nil {
		return director
	}
	fallback, fallbackErr := NewStoryDirectorLibrary(s.novaDir).Get(DefaultStoryDirectorID)
	if fallbackErr == nil {
		return fallback
	}
	return DefaultStoryDirector()
}

func terminalOutcomeFromRuleResolution(resolution RuleResolution, turnID, narrative string) *TerminalOutcome {
	if resolution.TerminalCandidate == nil {
		return nil
	}
	candidate := resolution.TerminalCandidate
	return normalizeTerminalOutcomePointer(&TerminalOutcome{
		Terminal:              true,
		Type:                  firstNonEmptyString(candidate.Type, "bad_end"),
		Reason:                candidate.Reason,
		FinalNarrativeSummary: trimBytes(narrative, maxInteractiveTextBytes),
		CausedByTurnID:        turnID,
		RuleResolutionID:      resolution.ID,
	})
}

func defaultStoryTitle(stories []StorySummary) string {
	if len(stories) == 0 {
		return defaultFirstStoryTitle
	}
	next := len(stories) + 1
	for _, story := range stories {
		title := strings.TrimSpace(story.Title)
		if !strings.HasPrefix(title, "故事线") {
			continue
		}
		rawNumber := strings.TrimSpace(strings.TrimPrefix(title, "故事线"))
		if rawNumber == "" {
			continue
		}
		number, err := strconv.Atoi(rawNumber)
		if err == nil && number >= next {
			next = number + 1
		}
	}
	if next < 2 {
		next = 2
	}
	return fmt.Sprintf("故事线 %d", next)
}

func normalizeStoryReplyTargetChars(value int) int {
	if value <= 0 {
		return DefaultStoryReplyTargetChars
	}
	return value
}

func normalizeStoryChoiceCount(value int) int {
	if value == 0 {
		return DefaultStoryChoiceCount
	}
	return value
}

func validateStoryChoiceCount(value int) error {
	if value < MinStoryChoiceCount || value > MaxStoryChoiceCount {
		return fmt.Errorf("互动故事行动建议数量必须在 %d 到 %d 之间", MinStoryChoiceCount, MaxStoryChoiceCount)
	}
	return nil
}

func normalizeStorySummary(story StorySummary) StorySummary {
	story.StoryDirectorID = normalizedStoryDirectorID(story.StoryDirectorID)
	story.DirectorRunPolicy = cloneStoryDirectorRunPolicy(story.DirectorRunPolicy)
	story.ReplyTargetChars = normalizeStoryReplyTargetChars(story.ReplyTargetChars)
	story.ChoiceCount = normalizeStoryChoiceCount(story.ChoiceCount)
	story.Opening = normalizeStoryOpeningConfig(story.Opening)
	story.ImageSettings = normalizeStoryImageSettings(story.ImageSettings)
	story.ModuleRefs = cloneStoryDirectorModuleRefs(story.ModuleRefs)
	if story.StateSchemaPolicy == nil {
		story.StateSchemaPolicy = fixedStoryStateSchemaPolicy()
	} else {
		story.StateSchemaPolicy = cloneStoryStateSchemaPolicy(story.StateSchemaPolicy)
	}
	return story
}

func normalizeStoryMeta(meta StoryMeta) StoryMeta {
	legacyFixedSchema := meta.StateSchemaPolicy == nil
	meta.StoryDirectorID = normalizedStoryDirectorID(meta.StoryDirectorID)
	meta.DirectorRunPolicy = cloneStoryDirectorRunPolicy(meta.DirectorRunPolicy)
	meta.ReplyTargetChars = normalizeStoryReplyTargetChars(meta.ReplyTargetChars)
	meta.ChoiceCount = normalizeStoryChoiceCount(meta.ChoiceCount)
	meta.Opening = normalizeStoryOpeningConfig(meta.Opening)
	meta.ImageSettings = normalizeStoryImageSettings(meta.ImageSettings)
	meta.ActorStateSchema = normalizeActorStateSchemaSnapshot(meta.ActorStateSchema)
	meta.ModuleRefs = cloneStoryDirectorModuleRefs(meta.ModuleRefs)
	if legacyFixedSchema {
		meta.StateSchemaPolicy = fixedStoryStateSchemaPolicy()
	} else {
		meta.StateSchemaPolicy = cloneStoryStateSchemaPolicy(meta.StateSchemaPolicy)
	}
	normalizeFixedStoryStateSchemaInitialization(&meta)
	return meta
}

func storyContainsTurn(events []StoryEventRecord) bool {
	for _, event := range events {
		if event.Envelope.Type == StoryEventTypeTurn {
			return true
		}
	}
	return false
}

func cloneStoryDirectorModuleRefs(refs *StoryDirectorModuleRefs) *StoryDirectorModuleRefs {
	if refs == nil {
		return nil
	}
	cloned := NormalizeStoryDirectorModuleRefs(*refs)
	cloned.EventPackageIDs = append([]string(nil), cloned.EventPackageIDs...)
	return &cloned
}

func cloneStoryDirectorRunPolicy(policy *StoryDirectorRunPolicy) *StoryDirectorRunPolicy {
	if policy == nil {
		return nil
	}
	cloned := NormalizeStoryDirectorRunPolicy(*policy)
	return &cloned
}

func normalizedStoryDirectorID(id string) string {
	if id = NormalizeStoryDirectorID(id); id != "" {
		return id
	}
	return DefaultStoryDirectorID
}

func normalizeStoryOpeningConfig(config StoryOpeningConfig) StoryOpeningConfig {
	mode := strings.TrimSpace(config.Mode)
	switch mode {
	case StoryOpeningModePreset, StoryOpeningModeCustom:
	default:
		mode = StoryOpeningModeAI
	}
	normalized := StoryOpeningConfig{
		Mode:       mode,
		PresetID:   strings.TrimSpace(config.PresetID),
		PresetText: truncateStoryOpeningText(config.PresetText),
		CustomText: truncateStoryOpeningText(config.CustomText),
	}
	if mode != StoryOpeningModePreset {
		normalized.PresetID = ""
		normalized.PresetText = ""
	}
	if mode != StoryOpeningModeCustom {
		normalized.CustomText = ""
	}
	return normalized
}

func truncateStoryOpeningText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= maxStoryOpeningTextRunes {
		return text
	}
	return string(runes[:maxStoryOpeningTextRunes])
}

func normalizeStoryImageSettings(settings StoryImageSettings) StoryImageSettings {
	rawMode := strings.TrimSpace(settings.Mode)
	mode := StoryImageModeManual
	interval := settings.IntervalTurns
	switch rawMode {
	case "every_turn":
		mode = StoryImageModeInterval
		interval = 1
	case StoryImageModeInterval:
		mode = StoryImageModeInterval
	default:
		mode = StoryImageModeManual
	}
	if interval <= 0 {
		interval = 3
	}
	if interval > 50 {
		interval = 50
	}
	return StoryImageSettings{
		Mode:          mode,
		IntervalTurns: interval,
		PresetID:      normalizeStoryImagePresetID(settings.PresetID),
	}
}

func normalizeStoryImagePresetID(id string) string {
	id = imagepreset.NormalizeID(id)
	if id == "" {
		return imagepreset.DefaultID
	}
	return id
}

func newID(prefix string) string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return prefix + "_" + strconv.FormatInt(time.Now().UnixNano(), 36) + hex.EncodeToString(b[:])
}
