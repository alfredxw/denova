package interactive

import (
	interactivestate "denova/internal/interactive/state"
	"encoding/json"
	"fmt"
	"strings"

	"denova/config"
	"denova/internal/agents/conversationconfig"
)

const (
	StoryEventTypeMeta                             = "meta"
	StoryEventTypePlayerInput                      = "player_input_accepted"
	StoryEventTypeTurnInterrupted                  = "turn_interrupted"
	StoryEventTypeModelContextBatch                = "model_context_batch"
	StoryEventTypeModelContextProviderContinuation = "model_context_provider_continuation"
	StoryEventTypeProviderContinuation             = "turn_provider_continuation"
	StoryEventTypeTurn                             = "turn"
	StoryEventTypeStateDelta                       = "state_delta"
	StoryEventTypeBranch                           = "branch"
	StoryEventTypeHotChoices                       = "hot_choices"
	StoryEventTypeTurnVersionSelected              = "turn_version_selected"
	StoryEventTypeTurnNarrativeRevised             = "turn_narrative_revised"
	StoryEventTypeTurnDisplayAppended              = "turn_display_appended"
	StoryEventTypeTurnStateRevised                 = "turn_state_revised"
	StoryEventTypeStoryConfigUpdated               = "story_config_updated"
	StoryEventTypeBranchSwitched                   = "branch_switched"
	StoryEventTypeBranchArchived                   = "branch_archived"
	StoryEventTypeBranchHeadMoved                  = "branch_head_moved"
	StoryEventTypeBranchPlanUpdated                = "branch_plan_updated"

	stateOpSchemaVersion = 2
)

// persistedStoryEventModelContextChanges is the single vocabulary for
// canonical story event rows. Envelope validation and the journal projection
// both consult this table, so adding an event requires an explicit context decision.
var persistedStoryEventModelContextChanges = map[string]bool{
	StoryEventTypePlayerInput:       true,
	StoryEventTypeTurnInterrupted:   true,
	StoryEventTypeModelContextBatch: true,
	// The owning Turn or model-context batch already changes model context.
	StoryEventTypeModelContextProviderContinuation: false,
	// The parent Turn already advances the context revision in the same atomic
	// transaction. This side event only carries its opaque provider state.
	StoryEventTypeProviderContinuation: false,
	StoryEventTypeTurn:                 true,
	StoryEventTypeStateDelta:           true,
	StoryEventTypeBranch:               true,
	StoryEventTypeHotChoices:           false,
	StoryEventTypeTurnVersionSelected:  true,
	StoryEventTypeTurnNarrativeRevised: true,
	StoryEventTypeTurnDisplayAppended:  false,
	StoryEventTypeTurnStateRevised:     true,
	StoryEventTypeStoryConfigUpdated:   true,
	StoryEventTypeBranchSwitched:       false,
	StoryEventTypeBranchArchived:       false,
	StoryEventTypeBranchHeadMoved:      true,
	// The owning Turn already advances the model-context revision in the same
	// atomic transaction. This private event only carries its next-turn plan.
	StoryEventTypeBranchPlanUpdated: false,
}

func storyEventChangesModelContext(eventType string) (bool, error) {
	changesModelContext, ok := persistedStoryEventModelContextChanges[eventType]
	if !ok {
		return false, fmt.Errorf("未知故事事件类型: %q", eventType)
	}
	return changesModelContext, nil
}

// StoryEventEnvelope is the stable schema envelope for every JSONL event row.
// Payload fields remain event-specific, but routing, graph traversal and
// migration decisions must go through this bounded envelope first.
type StoryEventEnvelope struct {
	V        int    `json:"v"`
	Type     string `json:"type"`
	ID       string `json:"id,omitempty"`
	ParentID any    `json:"parent_id,omitempty"`
	BranchID string `json:"branch_id,omitempty"`
	Ts       string `json:"ts,omitempty"`
}

type StoryEventRecord struct {
	Envelope StoryEventEnvelope
	Raw      map[string]any
}

func decodeStoryEventRecord(data []byte) (StoryEventRecord, error) {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return StoryEventRecord{}, err
	}
	return mapToStoryEventRecord(raw)
}

func mapToStoryEventRecord(raw map[string]any) (StoryEventRecord, error) {
	if raw == nil {
		return StoryEventRecord{}, fmt.Errorf("故事事件为空")
	}
	var envelope StoryEventEnvelope
	if err := mapToStruct(raw, &envelope); err != nil {
		return StoryEventRecord{}, err
	}
	if err := validateStoryEventEnvelope(envelope); err != nil {
		return StoryEventRecord{}, err
	}
	if envelope.Type == StoryEventTypeTurn {
		var turn TurnEvent
		if err := mapToStruct(raw, &turn); err != nil {
			return StoryEventRecord{}, err
		}
		if _, err := normalizeResolvedPlayerInputContexts(
			turn.ResolvedPlayerInputContexts, turn.BranchID, turn.PlayerInputID, turn.ConsumedPlayerInputIDs,
		); err != nil {
			return StoryEventRecord{}, err
		}
		if turn.StateDelta != nil {
			if err := validateStateDelta(*turn.StateDelta); err != nil {
				return StoryEventRecord{}, fmt.Errorf("校验回合状态变化失败: %w", err)
			}
		}
	}
	if envelope.Type == StoryEventTypeProviderContinuation {
		var event providerContinuationEvent
		if err := mapToStruct(raw, &event); err != nil {
			return StoryEventRecord{}, err
		}
		if _, err := normalizeProviderContinuationEvent(event); err != nil {
			return StoryEventRecord{}, err
		}
	}
	if envelope.Type == StoryEventTypeModelContextProviderContinuation {
		var event modelContextProviderContinuationEvent
		if err := mapToStruct(raw, &event); err != nil {
			return StoryEventRecord{}, err
		}
		if _, err := normalizeModelContextProviderContinuationEvent(event); err != nil {
			return StoryEventRecord{}, err
		}
	}
	if envelope.Type == StoryEventTypeStateDelta {
		var delta StateDeltaEvent
		if err := mapToStruct(raw, &delta); err != nil {
			return StoryEventRecord{}, err
		}
		if err := validateStateDelta(StateDelta{SchemaVersion: delta.SchemaVersion, Ops: delta.Ops, ActorOps: delta.ActorOps}); err != nil {
			return StoryEventRecord{}, fmt.Errorf("校验状态变化事件失败: %w", err)
		}
	}
	if envelope.Type == StoryEventTypeTurnVersionSelected {
		var selection TurnVersionSelectionEvent
		if err := mapToStruct(raw, &selection); err != nil {
			return StoryEventRecord{}, err
		}
		if err := validateTurnVersionSelection(selection); err != nil {
			return StoryEventRecord{}, err
		}
	}
	if envelope.Type == StoryEventTypeTurnStateRevised {
		var revision TurnStateRevisedEvent
		if err := mapToStruct(raw, &revision); err != nil {
			return StoryEventRecord{}, err
		}
		if revision.StateDelta != nil {
			if err := validateStateDelta(*revision.StateDelta); err != nil {
				return StoryEventRecord{}, fmt.Errorf("校验回合状态修订失败: %w", err)
			}
		}
	}
	if envelope.Type == StoryEventTypeModelContextBatch {
		var batch ModelContextBatchEvent
		if err := mapToStruct(raw, &batch); err != nil {
			return StoryEventRecord{}, err
		}
		if _, err := normalizeModelContextBatchEvent(batch); err != nil {
			return StoryEventRecord{}, err
		}
	}
	if envelope.Type == StoryEventTypePlayerInput {
		var input PlayerInputAcceptedEvent
		if err := mapToStruct(raw, &input); err != nil {
			return StoryEventRecord{}, err
		}
		if _, err := normalizePlayerInputAcceptedEvent(input); err != nil {
			return StoryEventRecord{}, err
		}
	}
	if envelope.Type == StoryEventTypeTurnInterrupted {
		var interruption TurnInterruptedEvent
		if err := mapToStruct(raw, &interruption); err != nil {
			return StoryEventRecord{}, err
		}
		if err := validateTurnInterruptedEvent(interruption); err != nil {
			return StoryEventRecord{}, err
		}
	}
	if envelope.Type == StoryEventTypeBranchPlanUpdated {
		var event BranchPlanUpdatedEvent
		if err := mapToStruct(raw, &event); err != nil {
			return StoryEventRecord{}, err
		}
		if strings.TrimSpace(event.TurnID) == "" || strings.TrimSpace(event.ParentID) != strings.TrimSpace(event.TurnID) {
			return StoryEventRecord{}, fmt.Errorf("branch plan update must reference its owning turn")
		}
		if err := validateBranchPlanMarkdown(event.Markdown); err != nil {
			return StoryEventRecord{}, err
		}
	}
	return StoryEventRecord{Envelope: envelope, Raw: raw}, nil
}

func storyEventRecordForWrite(event any) (StoryEventRecord, error) {
	data, err := json.Marshal(event)
	if err != nil {
		return StoryEventRecord{}, err
	}
	record, err := decodeStoryEventRecord(data)
	if err != nil {
		return StoryEventRecord{}, err
	}
	switch record.Envelope.Type {
	case StoryEventTypeTurn:
		var turn TurnEvent
		if err := mapToStruct(record.Raw, &turn); err != nil {
			return StoryEventRecord{}, err
		}
		if turn.StateDelta != nil {
			if err := validateStateDeltaForWrite(*turn.StateDelta); err != nil {
				return StoryEventRecord{}, fmt.Errorf("校验待写入回合状态变化失败: %w", err)
			}
		}
	case StoryEventTypeStateDelta:
		var delta StateDeltaEvent
		if err := mapToStruct(record.Raw, &delta); err != nil {
			return StoryEventRecord{}, err
		}
		if err := validateStateDeltaForWrite(StateDelta{SchemaVersion: delta.SchemaVersion, Ops: delta.Ops, ActorOps: delta.ActorOps}); err != nil {
			return StoryEventRecord{}, fmt.Errorf("校验待写入状态变化事件失败: %w", err)
		}
	case StoryEventTypeTurnStateRevised:
		var revision TurnStateRevisedEvent
		if err := mapToStruct(record.Raw, &revision); err != nil {
			return StoryEventRecord{}, err
		}
		if revision.StateDelta != nil {
			if err := validateStateDeltaForWrite(*revision.StateDelta); err != nil {
				return StoryEventRecord{}, fmt.Errorf("校验待写入回合状态修订失败: %w", err)
			}
		}
	}
	return record, nil
}

func validateStoryMeta(meta StoryMeta) error {
	meta = normalizeStoryMeta(meta)
	if meta.Type != StoryEventTypeMeta {
		return fmt.Errorf("故事元信息类型无效: %q", meta.Type)
	}
	if meta.V <= 0 || meta.V > schemaVersion {
		return fmt.Errorf("故事元信息 schema 版本不支持: %d", meta.V)
	}
	if strings.TrimSpace(meta.StoryID) == "" {
		return fmt.Errorf("故事元信息缺少 story_id")
	}
	if strings.TrimSpace(meta.CurrentBranch) == "" {
		return fmt.Errorf("故事元信息缺少 current_branch")
	}
	if len(meta.Branches) == 0 {
		return fmt.Errorf("故事元信息缺少 branches")
	}
	for branchID, branch := range meta.Branches {
		if branch.RuntimeConfig == nil {
			if branch.RuntimeConfigRevision != 0 {
				return fmt.Errorf("branch %q has runtime config revision without config", branchID)
			}
			continue
		}
		if branch.RuntimeConfigRevision == 0 {
			return fmt.Errorf("branch %q runtime config revision is missing", branchID)
		}
		if err := conversationconfig.ValidateShape(*branch.RuntimeConfig, config.AgentKindInteractiveStory); err != nil {
			return fmt.Errorf("branch %q runtime config: %w", branchID, err)
		}
	}
	if meta.ReplyTargetChars <= 0 {
		return fmt.Errorf("故事单轮目标字数无效: %d", meta.ReplyTargetChars)
	}
	if err := validateStoryChoiceCount(meta.ChoiceCount); err != nil {
		return err
	}
	if err := validateStoryPlanningMode(meta.PlanningMode); err != nil {
		return err
	}
	if err := validateStoryProtagonist(meta.Protagonist); err != nil {
		return err
	}
	switch meta.ImageSettings.Mode {
	case StoryImageModeManual, StoryImageModeInterval:
	default:
		return fmt.Errorf("互动图像模式无效: %q", meta.ImageSettings.Mode)
	}
	if meta.ImageSettings.IntervalTurns <= 0 {
		return fmt.Errorf("互动图像间隔轮数无效: %d", meta.ImageSettings.IntervalTurns)
	}
	if err := validateStoryCheckSettings(meta.CheckSettings); err != nil {
		return err
	}
	if strings.TrimSpace(meta.ImageSettings.PresetID) == "" {
		return fmt.Errorf("互动图像方案不能为空")
	}
	switch meta.Opening.Mode {
	case StoryOpeningModeAI:
	case StoryOpeningModePreset:
		if strings.TrimSpace(meta.Opening.PresetText) == "" {
			return fmt.Errorf("预设开场白不能为空")
		}
	case StoryOpeningModeCustom:
		if strings.TrimSpace(meta.Opening.CustomText) == "" {
			return fmt.Errorf("自定义开场白不能为空")
		}
	default:
		return fmt.Errorf("故事开场白模式无效: %q", meta.Opening.Mode)
	}
	return nil
}

func validateStoryEventEnvelope(envelope StoryEventEnvelope) error {
	if envelope.V <= 0 || envelope.V > schemaVersion {
		return fmt.Errorf("故事事件 schema 版本不支持: %d", envelope.V)
	}
	if _, err := storyEventChangesModelContext(envelope.Type); err != nil {
		return err
	}
	if strings.TrimSpace(envelope.ID) == "" {
		return fmt.Errorf("故事事件缺少 id: %s", envelope.Type)
	}
	if strings.TrimSpace(envelope.BranchID) == "" {
		return fmt.Errorf("故事事件缺少 branch_id: %s", envelope.ID)
	}
	if strings.TrimSpace(envelope.Ts) == "" {
		return fmt.Errorf("故事事件缺少 ts: %s", envelope.ID)
	}
	return nil
}

func validateTurnVersionSelection(selection TurnVersionSelectionEvent) error {
	if strings.TrimSpace(selection.ReplacedTurnID) == "" || strings.TrimSpace(selection.SelectedTurnID) == "" {
		return fmt.Errorf("回合版本选择缺少 replaced_turn_id 或 selected_turn_id")
	}
	if strings.TrimSpace(selection.ProjectedHeadID) == "" || strings.TrimSpace(selection.ParentID) != strings.TrimSpace(selection.ProjectedHeadID) {
		return fmt.Errorf("回合版本选择的 projected_head_id 与 parent_id 不一致")
	}
	seenSources := make(map[string]bool, len(selection.ProjectedEvents))
	seenProjected := make(map[string]bool, len(selection.ProjectedEvents))
	for _, projection := range selection.ProjectedEvents {
		sourceID := strings.TrimSpace(projection.SourceID)
		projectedID := strings.TrimSpace(projection.ProjectedID)
		if sourceID == "" || projectedID == "" || strings.TrimSpace(projection.EventType) == "" {
			return fmt.Errorf("回合版本选择包含不完整的 suffix 投影")
		}
		if seenSources[sourceID] || seenProjected[projectedID] {
			return fmt.Errorf("回合版本选择包含重复的 suffix 投影")
		}
		seenSources[sourceID] = true
		seenProjected[projectedID] = true
	}
	return nil
}

func newStateDelta(ops []interactivestate.Op) StateDelta {
	return StateDelta{SchemaVersion: stateOpSchemaVersion, Ops: ops}
}

func newStateDeltaWithActorOps(ops []interactivestate.Op, actorOps []ActorStateOp) StateDelta {
	return StateDelta{SchemaVersion: stateOpSchemaVersion, Ops: ops, ActorOps: actorOps}
}

func newStateDeltaEvent(id, parentID, branchID, ts string, ops []interactivestate.Op) StateDeltaEvent {
	return StateDeltaEvent{
		V:             schemaVersion,
		Type:          StoryEventTypeStateDelta,
		ID:            id,
		ParentID:      parentID,
		BranchID:      branchID,
		Ts:            ts,
		SchemaVersion: stateOpSchemaVersion,
		Ops:           ops,
	}
}

func newStateDeltaEventWithActorOps(id, parentID, branchID, ts string, ops []interactivestate.Op, actorOps []ActorStateOp) StateDeltaEvent {
	event := newStateDeltaEvent(id, parentID, branchID, ts, ops)
	event.ActorOps = actorOps
	return event
}

func validateStateDelta(delta StateDelta) error {
	if delta.SchemaVersion < 0 || delta.SchemaVersion > stateOpSchemaVersion {
		return fmt.Errorf("状态变化 schema 版本不支持: %d", delta.SchemaVersion)
	}
	for _, op := range delta.Ops {
		if err := validateStateOp(op); err != nil {
			return err
		}
	}
	for _, op := range delta.ActorOps {
		if err := validateActorStateOp(op); err != nil {
			return err
		}
	}
	return nil
}

func validateStateDeltaForWrite(delta StateDelta) error {
	if err := validateStateDelta(delta); err != nil {
		return err
	}
	for _, op := range delta.Ops {
		if strings.TrimSpace(op.Op) == "set" && op.Value == nil {
			return fmt.Errorf("set 状态操作缺少 value: path=%s", op.Path)
		}
	}
	for _, op := range delta.ActorOps {
		if strings.TrimSpace(op.Op) == "set" && op.Value == nil {
			return fmt.Errorf("set Actor 状态操作缺少 value: actor_id=%s field_id=%s", op.ActorID, op.FieldID)
		}
	}
	return nil
}

func validateActorStateOp(op ActorStateOp) error {
	switch strings.TrimSpace(op.Op) {
	case "set", "inc", "unset":
	default:
		return fmt.Errorf("未知 Actor 状态操作: %q", op.Op)
	}
	if normalizeStatePanelActorID(op.ActorID) == "" {
		return fmt.Errorf("Actor 状态操作缺少 actor_id")
	}
	if normalizeActorStateFieldName(op.FieldID) == "" {
		return fmt.Errorf("Actor 状态操作缺少 field_id")
	}
	return nil
}

func validateStateOp(op interactivestate.Op) error {
	opName := strings.TrimSpace(op.Op)
	switch opName {
	case "set", "merge", "push", "pull", "inc", "unset":
	default:
		return fmt.Errorf("未知状态操作: %q", op.Op)
	}
	path := strings.TrimSpace(op.Path)
	if path == "" {
		return fmt.Errorf("状态操作缺少 path: %s", opName)
	}
	if strings.HasPrefix(path, ".") || strings.HasSuffix(path, ".") || strings.Contains(path, "..") {
		return fmt.Errorf("状态操作 path 无效: %q", op.Path)
	}
	return nil
}
