package interactive

import (
	"context"
	interactivestate "denova/internal/interactive/state"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"denova/config"
	"denova/internal/agents/conversationconfig"
	"denova/internal/agents/conversationjournal"
	"denova/internal/interactive/director"
	"denova/internal/style"
)

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
		if previousID := strings.TrimSpace(index.CurrentStoryID); previousID != "" {
			if closeErr := s.evictStoryJournalLocked(previousID); closeErr != nil {
				slog.ErrorContext(context.Background(), fmt.Sprintf("[interactive-story] flush index on story switch failed story_id=%s error=%v", previousID, closeErr))
			}
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
		if err := director.ValidateRunPolicy(*directorRunPolicy); err != nil {
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
		story.StoryTellerID = style.DefaultID
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
	if req.RuntimeConfig != nil {
		if err := conversationconfig.ValidateShape(*req.RuntimeConfig, config.AgentKindInteractiveStory); err != nil {
			return StorySummary{}, err
		}
		branch := meta.Branches["main"]
		value := *req.RuntimeConfig
		branch.RuntimeConfig = &value
		branch.RuntimeConfigRevision = 1
		meta.Branches["main"] = branch
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
	generatedOps := []interactivestate.Op(nil)
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
		branch := meta.Branches["main"]
		branch.Head = initialDeltaID
		meta.Branches["main"] = branch
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
	releaseStory, err := s.acquireStoryMutationLeaseLocked(storyID)
	if err != nil {
		return StorySummary{}, err
	}
	defer releaseStory()

	meta, snapshot, err := s.boundedStorySnapshotWithLimitLocked(storyID, "", 1)
	if err != nil {
		return StorySummary{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	updatedFields := storyConfigUpdatedFields(req)
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
		policy := director.NormalizeRunPolicy(*req.DirectorRunPolicy)
		if err := director.ValidateRunPolicy(policy); err != nil {
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
	appendedEvents := []any(nil)
	if req.StateSchemaPolicy != nil {
		if snapshot.TurnCount > 0 {
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
		branch := meta.Branches[meta.CurrentBranch]
		previousHead := branch.Head
		branch.Head = ""
		meta.Branches[meta.CurrentBranch] = branch
		headMoved := BranchHeadMovedEvent{
			V: schemaVersion, Type: StoryEventTypeBranchHeadMoved, ID: newID("bhm"),
			BranchID: meta.CurrentBranch, Ts: now, PreviousHead: previousHead,
			StateCheckpoint: initialStoryState(), Reason: "state_schema_reinitialized",
		}
		appendedEvents = append(appendedEvents, headMoved)
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
				appendedEvents = append(appendedEvents, newStateDeltaEventWithActorOps(deltaID, "", meta.CurrentBranch, now, initialOps, initialActorOps))
			}
		}
	}
	meta.UpdatedAt = now
	configEvent := StoryConfigUpdatedEvent{
		V: schemaVersion, Type: StoryEventTypeStoryConfigUpdated, ID: newID("scu"),
		ParentID: meta.Branches[meta.CurrentBranch].Head, BranchID: meta.CurrentBranch, Ts: now,
		Fields: updatedFields,
	}
	appendedEvents = append([]any{configEvent}, appendedEvents...)
	if err := s.appendStoryTransactionLocked(storyID, meta, appendedEvents...); err != nil {
		return StorySummary{}, err
	}
	return s.publishStorySummaryLocked(storyID)
}

func storyConfigUpdatedFields(req UpdateStoryRequest) []string {
	fields := make([]string, 0, 12)
	if strings.TrimSpace(req.Title) != "" {
		fields = append(fields, "title")
	}
	if req.Origin != nil {
		fields = append(fields, "origin")
	}
	if strings.TrimSpace(req.StoryTellerID) != "" {
		fields = append(fields, "story_teller_id")
	}
	if NormalizeStoryDirectorID(req.StoryDirectorID) != "" {
		fields = append(fields, "story_director_id")
	}
	if req.DirectorRunPolicy != nil {
		fields = append(fields, "director_run_policy")
	}
	if req.ModuleRefs != nil {
		fields = append(fields, "module_refs")
	}
	if req.ReplyTargetChars != nil {
		fields = append(fields, "reply_target_chars")
	}
	if req.ChoiceCount != nil {
		fields = append(fields, "choice_count")
	}
	if req.Opening != nil {
		fields = append(fields, "opening")
	}
	if req.ImageSettings != nil {
		fields = append(fields, "image_settings")
	}
	if req.StateSchemaPolicy != nil {
		fields = append(fields, "state_schema_policy", "actor_state_schema", "state_schema_initialization")
	}
	return fields
}

func (s *Store) DeleteStory(storyID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	releaseStory, err := s.acquireStoryMutationLeaseLocked(storyID)
	if err != nil {
		return err
	}
	defer releaseStory()

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
	if closeErr := s.evictStoryJournalLocked(storyID); closeErr != nil {
		slog.ErrorContext(context.Background(), fmt.Sprintf("[interactive-story] flush index before story delete failed story_id=%s error=%v", storyID, closeErr))
	}
	if err := os.Remove(s.storyPath(storyID)); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Remove(conversationjournal.SidecarPath(s.storyPath(storyID))); err != nil && !os.IsNotExist(err) {
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
	if err := s.writeIndexLocked(index); err != nil {
		return err
	}
	// Artifact removal is post-commit garbage collection. Canonical story and
	// index references must become unreachable first; otherwise a filesystem
	// sync failure could leave a live story whose recovery paths were deleted.
	// A GC failure must not turn an already-committed delete into a false error.
	if err := s.removeStoryToolArtifacts(storyID); err != nil {
		slog.ErrorContext(context.Background(), fmt.Sprintf("[interactive-story] remove deleted story tool artifacts failed story_id=%s error=%v", storyID, err))
	}
	return nil
}
