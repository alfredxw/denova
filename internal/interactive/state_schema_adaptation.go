package interactive

import (
	"encoding/json"
	"fmt"
	"strings"
)

const maxActorStateSchemaAdaptationOps = 64

// ActorStateSchemaAdaptation is a bounded, story-creation-only diff over a
// reusable State System. The resulting system is frozen before any story
// state is materialized.
type ActorStateSchemaAdaptation struct {
	Summary         string                           `json:"summary,omitempty"`
	TemplateOps     []ActorStateTemplateSchemaOp     `json:"template_ops,omitempty"`
	InitialActorOps []ActorStateInitialActorSchemaOp `json:"initial_actor_ops,omitempty"`
}

// ActorStateTemplateSchemaOp adds or removes a template, or applies field
// operations to one existing template. Existing template metadata and trait
// rules remain stable when only FieldOps are supplied.
type ActorStateTemplateSchemaOp struct {
	Op         string                    `json:"op"`
	TemplateID string                    `json:"template_id,omitempty"`
	Template   ActorStateTemplate        `json:"template,omitempty"`
	FieldOps   []ActorStateFieldSchemaOp `json:"field_ops,omitempty"`
	Reason     string                    `json:"reason,omitempty"`
}

// ActorStateFieldSchemaOp uses add, replace, or remove. FieldID identifies the
// existing field for replace/remove; Field contains the complete new field for
// add/replace.
type ActorStateFieldSchemaOp struct {
	Op      string          `json:"op"`
	FieldID string          `json:"field_id,omitempty"`
	Field   ActorStateField `json:"field,omitempty"`
	Reason  string          `json:"reason,omitempty"`
}

// ActorStateInitialActorSchemaOp uses add, replace, or remove for story-local
// initial state objects. Dynamic actors created after the opening are not part
// of this initialization contract.
type ActorStateInitialActorSchemaOp struct {
	Op      string                 `json:"op"`
	ActorID string                 `json:"actor_id,omitempty"`
	Actor   ActorStateInitialActor `json:"actor,omitempty"`
	Reason  string                 `json:"reason,omitempty"`
}

// ActorStateSchemaAdaptationRecord is persisted with the frozen story schema
// so the customized contract has an explicit source and a compact audit trail.
type ActorStateSchemaAdaptationRecord struct {
	Source          string `json:"source"`
	Summary         string `json:"summary,omitempty"`
	TemplateOps     int    `json:"template_ops,omitempty"`
	FieldOps        int    `json:"field_ops,omitempty"`
	InitialActorOps int    `json:"initial_actor_ops,omitempty"`
}

func ParseActorStateSchemaAdaptation(content string) (ActorStateSchemaAdaptation, error) {
	payload := strings.TrimSpace(content)
	if start := strings.Index(payload, "{"); start >= 0 {
		if end := strings.LastIndex(payload, "}"); end >= start {
			payload = payload[start : end+1]
		}
	}
	if payload == "" {
		return ActorStateSchemaAdaptation{}, fmt.Errorf("状态结构适配结果为空")
	}
	var adaptation ActorStateSchemaAdaptation
	if err := json.Unmarshal([]byte(payload), &adaptation); err != nil {
		return ActorStateSchemaAdaptation{}, fmt.Errorf("解析状态结构适配结果失败: %w", err)
	}
	adaptation.Summary = trimBytes(adaptation.Summary, maxTurnBriefTextBytes)
	if len(adaptation.TemplateOps) > maxActorStateSchemaAdaptationOps {
		return ActorStateSchemaAdaptation{}, fmt.Errorf("状态结构模板操作过多: %d > %d", len(adaptation.TemplateOps), maxActorStateSchemaAdaptationOps)
	}
	if len(adaptation.InitialActorOps) > maxActorStateSchemaAdaptationOps {
		return ActorStateSchemaAdaptation{}, fmt.Errorf("初始 Actor 操作过多: %d > %d", len(adaptation.InitialActorOps), maxActorStateSchemaAdaptationOps)
	}
	fieldOps := 0
	for index := range adaptation.TemplateOps {
		op := &adaptation.TemplateOps[index]
		op.Op = strings.TrimSpace(op.Op)
		op.TemplateID = normalizeActorStateID(op.TemplateID)
		op.Reason = trimBytes(op.Reason, maxTurnBriefTextBytes)
		fieldOps += len(op.FieldOps)
		for fieldIndex := range op.FieldOps {
			fieldOp := &op.FieldOps[fieldIndex]
			fieldOp.Op = strings.TrimSpace(fieldOp.Op)
			fieldOp.FieldID = normalizeActorStateFieldName(fieldOp.FieldID)
			fieldOp.Reason = trimBytes(fieldOp.Reason, maxTurnBriefTextBytes)
		}
	}
	if fieldOps > maxActorStateSchemaAdaptationOps {
		return ActorStateSchemaAdaptation{}, fmt.Errorf("状态字段操作过多: %d > %d", fieldOps, maxActorStateSchemaAdaptationOps)
	}
	for index := range adaptation.InitialActorOps {
		op := &adaptation.InitialActorOps[index]
		op.Op = strings.TrimSpace(op.Op)
		op.ActorID = normalizeActorStateID(op.ActorID)
		op.Reason = trimBytes(op.Reason, maxTurnBriefTextBytes)
	}
	return adaptation, nil
}

// ApplyActorStateSchemaAdaptation applies a Director-proposed initialization
// diff and validates the complete State System plus every frozen TRPG binding.
func ApplyActorStateSchemaAdaptation(base StoryDirectorActorStateSystem, trpg StoryDirectorTRPGSystem, adaptation ActorStateSchemaAdaptation) (StoryDirectorActorStateSystem, ActorStateSchemaAdaptationRecord, error) {
	system := normalizeActorStateSystem(base)
	requireProtagonistTemplate := actorStateTemplateByID(system, DefaultActorID).ID != ""
	requireStoryTemplate := actorStateTemplateByID(system, ActorStateStoryContextTemplateID).ID != ""
	requireProtagonistActor := actorStateInitialActorIndex(system.InitialActors, DefaultActorID) >= 0
	requireStoryActor := actorStateInitialActorIndex(system.InitialActors, DefaultStoryContextActorID) >= 0
	for _, op := range adaptation.TemplateOps {
		var err error
		system, err = applyActorStateTemplateSchemaOp(system, op)
		if err != nil {
			return StoryDirectorActorStateSystem{}, ActorStateSchemaAdaptationRecord{}, err
		}
	}
	for _, op := range adaptation.InitialActorOps {
		var err error
		system, err = applyActorStateInitialActorSchemaOp(system, op)
		if err != nil {
			return StoryDirectorActorStateSystem{}, ActorStateSchemaAdaptationRecord{}, err
		}
	}
	if err := validateAdaptedActorStateSystem(system, requireProtagonistTemplate, requireStoryTemplate, requireProtagonistActor, requireStoryActor); err != nil {
		return StoryDirectorActorStateSystem{}, ActorStateSchemaAdaptationRecord{}, err
	}
	if err := validateActorStateTRPGReferences(system, trpg); err != nil {
		return StoryDirectorActorStateSystem{}, ActorStateSchemaAdaptationRecord{}, err
	}
	fieldOps := 0
	for _, op := range adaptation.TemplateOps {
		fieldOps += len(op.FieldOps)
	}
	record := ActorStateSchemaAdaptationRecord{
		Source:          "director_agent",
		Summary:         trimBytes(adaptation.Summary, maxTurnBriefTextBytes),
		TemplateOps:     len(adaptation.TemplateOps),
		FieldOps:        fieldOps,
		InitialActorOps: len(adaptation.InitialActorOps),
	}
	return normalizeActorStateSystem(system), record, nil
}

func applyActorStateTemplateSchemaOp(system StoryDirectorActorStateSystem, op ActorStateTemplateSchemaOp) (StoryDirectorActorStateSystem, error) {
	switch op.Op {
	case "add":
		if normalizeActorStateID(op.Template.ID) == "" {
			return system, fmt.Errorf("新增状态模板缺少合法 template.id")
		}
		if len(op.Template.Fields) > maxActorStateFields {
			return system, fmt.Errorf("新增状态模板字段超过上限: template=%s fields=%d limit=%d", op.Template.ID, len(op.Template.Fields), maxActorStateFields)
		}
		for _, field := range op.Template.Fields {
			if err := validateActorStateAdaptationField(field); err != nil {
				return system, fmt.Errorf("新增状态模板 %s: %w", op.Template.ID, err)
			}
		}
		templates := normalizeActorStateTemplates([]ActorStateTemplate{op.Template})
		if len(templates) != 1 || templates[0].ID == "" {
			return system, fmt.Errorf("新增状态模板缺少合法 template.id")
		}
		if actorStateTemplateByID(system, templates[0].ID).ID != "" {
			return system, fmt.Errorf("新增状态模板已存在: %s", templates[0].ID)
		}
		if len(system.Templates) >= maxTurnBriefListItems {
			return system, fmt.Errorf("状态模板数量已达到上限: %d", maxTurnBriefListItems)
		}
		system.Templates = append(system.Templates, templates[0])
		return system, nil
	case "remove":
		templateID := normalizeActorStateID(op.TemplateID)
		if templateID == "" {
			return system, fmt.Errorf("删除状态模板缺少 template_id")
		}
		if templateID == DefaultActorID || templateID == ActorStateStoryContextTemplateID {
			return system, fmt.Errorf("故事基础状态模板不可删除: %s", templateID)
		}
		index := actorStateTemplateIndex(system.Templates, templateID)
		if index < 0 {
			return system, fmt.Errorf("删除的状态模板不存在: %s", templateID)
		}
		system.Templates = append(system.Templates[:index], system.Templates[index+1:]...)
		return system, nil
	case "fields":
		templateID := normalizeActorStateID(op.TemplateID)
		index := actorStateTemplateIndex(system.Templates, templateID)
		if index < 0 {
			return system, fmt.Errorf("状态字段操作引用的模板不存在: %s", templateID)
		}
		template := system.Templates[index]
		for _, fieldOp := range op.FieldOps {
			var err error
			template, err = applyActorStateFieldSchemaOp(template, fieldOp)
			if err != nil {
				return system, fmt.Errorf("状态模板 %s: %w", templateID, err)
			}
		}
		system.Templates[index] = template
		return system, nil
	default:
		return system, fmt.Errorf("状态模板操作无效: %s", op.Op)
	}
}

func applyActorStateFieldSchemaOp(template ActorStateTemplate, op ActorStateFieldSchemaOp) (ActorStateTemplate, error) {
	switch op.Op {
	case "add":
		if err := validateActorStateAdaptationField(op.Field); err != nil {
			return template, err
		}
		fields := normalizeActorStateFields([]ActorStateField{op.Field})
		if len(fields) != 1 || actorStateFieldID(fields[0]) == "" {
			return template, fmt.Errorf("新增状态字段缺少合法 field.name")
		}
		if _, ok := actorStateFieldByID(template, actorStateFieldID(fields[0])); ok {
			return template, fmt.Errorf("新增状态字段已存在: %s", actorStateFieldID(fields[0]))
		}
		if len(template.Fields) >= maxActorStateFields {
			return template, fmt.Errorf("状态字段数量已达到上限: %d", maxActorStateFields)
		}
		template.Fields = append(template.Fields, fields[0])
		template.Fields = normalizeActorStateFields(template.Fields)
		return template, nil
	case "replace":
		index := actorStateFieldIndex(template.Fields, op.FieldID)
		if index < 0 {
			return template, fmt.Errorf("替换的状态字段不存在: %s", op.FieldID)
		}
		if err := validateActorStateAdaptationField(op.Field); err != nil {
			return template, err
		}
		fields := normalizeActorStateFields([]ActorStateField{op.Field})
		if len(fields) != 1 || actorStateFieldID(fields[0]) == "" {
			return template, fmt.Errorf("替换状态字段缺少完整合法 field")
		}
		for fieldIndex, existing := range template.Fields {
			if fieldIndex != index && actorStateFieldNameKey(actorStateFieldID(existing)) == actorStateFieldNameKey(actorStateFieldID(fields[0])) {
				return template, fmt.Errorf("替换后的状态字段重名: %s", actorStateFieldID(fields[0]))
			}
		}
		template.Fields[index] = fields[0]
		template.Fields = normalizeActorStateFields(template.Fields)
		return template, nil
	case "remove":
		index := actorStateFieldIndex(template.Fields, op.FieldID)
		if index < 0 {
			return template, fmt.Errorf("删除的状态字段不存在: %s", op.FieldID)
		}
		template.Fields = append(template.Fields[:index], template.Fields[index+1:]...)
		return template, nil
	default:
		return template, fmt.Errorf("状态字段操作无效: %s", op.Op)
	}
}

func applyActorStateInitialActorSchemaOp(system StoryDirectorActorStateSystem, op ActorStateInitialActorSchemaOp) (StoryDirectorActorStateSystem, error) {
	switch op.Op {
	case "add":
		actor := op.Actor
		actor.ID = normalizeActorStateID(actor.ID)
		if actor.ID == "" {
			return system, fmt.Errorf("新增初始 Actor 缺少合法 actor.id")
		}
		if actorStateInitialActorIndex(system.InitialActors, actor.ID) >= 0 {
			return system, fmt.Errorf("新增初始 Actor 已存在: %s", actor.ID)
		}
		if actorStateTemplateByID(system, actor.TemplateID).ID == "" {
			return system, fmt.Errorf("新增初始 Actor 引用的模板不存在: actor=%s template=%s", actor.ID, actor.TemplateID)
		}
		system.InitialActors = append(system.InitialActors, actor)
	case "replace":
		index := actorStateInitialActorIndex(system.InitialActors, op.ActorID)
		if index < 0 {
			return system, fmt.Errorf("替换的初始 Actor 不存在: %s", op.ActorID)
		}
		actor := op.Actor
		actor.ID = normalizeActorStateID(actor.ID)
		if actor.ID == "" {
			actor.ID = normalizeActorStateID(op.ActorID)
		}
		if actor.ID != normalizeActorStateID(op.ActorID) {
			return system, fmt.Errorf("替换初始 Actor 时不可改变 ID: %s -> %s", op.ActorID, actor.ID)
		}
		if actorStateTemplateByID(system, actor.TemplateID).ID == "" {
			return system, fmt.Errorf("替换初始 Actor 引用的模板不存在: actor=%s template=%s", actor.ID, actor.TemplateID)
		}
		system.InitialActors[index] = actor
	case "remove":
		actorID := normalizeActorStateID(op.ActorID)
		if actorID == DefaultActorID || actorID == DefaultStoryContextActorID {
			return system, fmt.Errorf("故事基础初始 Actor 不可删除: %s", actorID)
		}
		index := actorStateInitialActorIndex(system.InitialActors, actorID)
		if index < 0 {
			return system, fmt.Errorf("删除的初始 Actor 不存在: %s", actorID)
		}
		system.InitialActors = append(system.InitialActors[:index], system.InitialActors[index+1:]...)
	default:
		return system, fmt.Errorf("初始 Actor 操作无效: %s", op.Op)
	}
	if len(system.InitialActors) > maxTurnBriefListItems {
		return system, fmt.Errorf("初始 Actor 数量超过上限: %d", maxTurnBriefListItems)
	}
	return system, nil
}

func actorStateTemplateIndex(templates []ActorStateTemplate, templateID string) int {
	templateID = normalizeActorStateID(templateID)
	for index := range templates {
		if normalizeActorStateID(templates[index].ID) == templateID {
			return index
		}
	}
	return -1
}

func actorStateFieldIndex(fields []ActorStateField, fieldID string) int {
	key := actorStateFieldNameKey(fieldID)
	for index := range fields {
		if actorStateFieldNameKey(actorStateFieldID(fields[index])) == key {
			return index
		}
	}
	return -1
}

func actorStateInitialActorIndex(actors []ActorStateInitialActor, actorID string) int {
	actorID = normalizeActorStateID(actorID)
	for index := range actors {
		if normalizeActorStateID(actors[index].ID) == actorID {
			return index
		}
	}
	return -1
}
