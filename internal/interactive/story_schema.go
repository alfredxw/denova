package interactive

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	StoryEventTypeMeta              = "meta"
	StoryEventTypeTurn              = "turn"
	StoryEventTypeStateDelta        = "state_delta"
	StoryEventTypeBranch            = "branch"
	StoryEventTypeHotChoices        = "hot_choices"
	StoryEventTypeCompaction        = "context_compaction"
	StoryEventTypeCompactionRemoved = "context_compaction_removed"

	stateOpSchemaVersion = 2
)

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
		if turn.StateDelta != nil {
			if err := validateStateDelta(*turn.StateDelta); err != nil {
				return StoryEventRecord{}, fmt.Errorf("校验回合状态变化失败: %w", err)
			}
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
	return StoryEventRecord{Envelope: envelope, Raw: raw}, nil
}

func storyEventRecordForWrite(event any) (StoryEventRecord, error) {
	data, err := json.Marshal(event)
	if err != nil {
		return StoryEventRecord{}, err
	}
	return decodeStoryEventRecord(data)
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
	if meta.ReplyTargetChars <= 0 {
		return fmt.Errorf("故事单轮目标字数无效: %d", meta.ReplyTargetChars)
	}
	switch meta.ImageSettings.Mode {
	case StoryImageModeManual, StoryImageModeInterval:
	default:
		return fmt.Errorf("互动图像模式无效: %q", meta.ImageSettings.Mode)
	}
	if meta.ImageSettings.IntervalTurns <= 0 {
		return fmt.Errorf("互动图像间隔轮数无效: %d", meta.ImageSettings.IntervalTurns)
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
	switch envelope.Type {
	case StoryEventTypeTurn, StoryEventTypeStateDelta, StoryEventTypeBranch, StoryEventTypeHotChoices, StoryEventTypeCompaction, StoryEventTypeCompactionRemoved:
	default:
		return fmt.Errorf("未知故事事件类型: %q", envelope.Type)
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

func newStateDelta(ops []StateOp) StateDelta {
	return StateDelta{SchemaVersion: stateOpSchemaVersion, Ops: ops}
}

func newStateDeltaWithActorOps(ops []StateOp, actorOps []ActorStateOp) StateDelta {
	return StateDelta{SchemaVersion: stateOpSchemaVersion, Ops: ops, ActorOps: actorOps}
}

func newStateDeltaEvent(id, parentID, branchID, ts string, ops []StateOp) StateDeltaEvent {
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

func newStateDeltaEventWithActorOps(id, parentID, branchID, ts string, ops []StateOp, actorOps []ActorStateOp) StateDeltaEvent {
	event := newStateDeltaEvent(id, parentID, branchID, ts, ops)
	event.ActorOps = actorOps
	return event
}

func validateStateDelta(delta StateDelta) error {
	if delta.SchemaVersion < 0 || delta.SchemaVersion > stateOpSchemaVersion {
		return fmt.Errorf("状态变化 schema 版本不支持: %d", delta.SchemaVersion)
	}
	if len(delta.Ops) == 0 {
		return nil
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

func validateActorStateOp(op ActorStateOp) error {
	switch strings.TrimSpace(op.Op) {
	case "set", "inc", "unset":
	default:
		return fmt.Errorf("未知 Actor 状态操作: %q", op.Op)
	}
	if normalizeActorStateID(op.ActorID) == "" {
		return fmt.Errorf("Actor 状态操作缺少 actor_id")
	}
	if normalizeActorStateFieldName(op.FieldID) == "" {
		return fmt.Errorf("Actor 状态操作缺少 field_id")
	}
	return nil
}

func validateStateOp(op StateOp) error {
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
