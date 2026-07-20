package interactive

import (
	"fmt"
	"strings"
)

func validateAdaptedActorStateSystem(system StoryDirectorActorStateSystem, requireProtagonistTemplate, requireStoryTemplate, requireProtagonistActor, requireStoryActor bool) error {
	if requireProtagonistTemplate && actorStateTemplateByID(system, DefaultActorID).ID == "" {
		return fmt.Errorf("适配后的状态系统缺少 protagonist 模板")
	}
	if requireStoryTemplate && actorStateTemplateByID(system, ActorStateStoryContextTemplateID).ID == "" {
		return fmt.Errorf("适配后的状态系统缺少 story_context 模板")
	}
	protagonistIndex := actorStateInitialActorIndex(system.InitialActors, DefaultActorID)
	if requireProtagonistActor && (protagonistIndex < 0 || normalizeActorStateID(system.InitialActors[protagonistIndex].TemplateID) != DefaultActorID) {
		return fmt.Errorf("适配后的状态系统缺少绑定 protagonist 模板的 protagonist 初始 Actor")
	}
	storyIndex := actorStateInitialActorIndex(system.InitialActors, DefaultStoryContextActorID)
	if requireStoryActor && (storyIndex < 0 || normalizeActorStateID(system.InitialActors[storyIndex].TemplateID) != ActorStateStoryContextTemplateID) {
		return fmt.Errorf("适配后的状态系统缺少绑定 story_context 模板的 story 初始 Actor")
	}
	if err := validateActorStateSystem(system); err != nil {
		return err
	}
	if err := validateActorTraitSystem(system); err != nil {
		return err
	}
	for _, template := range system.Templates {
		for _, field := range template.Fields {
			if field.Default == nil {
				continue
			}
			if _, err := normalizeActorStateValue(field, field.Default); err != nil {
				return fmt.Errorf("Actor 状态模板 %s 默认值无效: %w", template.ID, err)
			}
		}
	}
	for _, actor := range system.InitialActors {
		template := actorStateTemplateByID(system, actor.TemplateID)
		if template.ID == "" {
			return fmt.Errorf("初始 Actor 引用了不存在的状态模板: actor=%s template=%s", actor.ID, actor.TemplateID)
		}
		for fieldID, value := range actor.State {
			if value == nil {
				return fmt.Errorf("初始 Actor 状态值不能为空: actor=%s template=%s field=%s", actor.ID, actor.TemplateID, fieldID)
			}
			field, ok := actorStateFieldByID(template, fieldID)
			if !ok {
				return fmt.Errorf("初始 Actor 使用了不存在的状态字段: actor=%s template=%s field=%s", actor.ID, actor.TemplateID, fieldID)
			}
			if _, err := normalizeActorStateValue(field, value); err != nil {
				return fmt.Errorf("初始 Actor 状态值无效: actor=%s template=%s: %w", actor.ID, actor.TemplateID, err)
			}
		}
	}
	return nil
}

func validateActorStateAdaptationField(field ActorStateField) error {
	if err := validateActorStateFieldName(field.Name); err != nil {
		return err
	}
	switch strings.TrimSpace(field.Type) {
	case "number", "string", "bool", "enum", "object", "list":
	default:
		return fmt.Errorf("状态字段 %s type 无效: %s", field.Name, field.Type)
	}
	if field.Visibility != "" {
		switch strings.TrimSpace(field.Visibility) {
		case "visible", "spoiler", "hidden":
		default:
			return fmt.Errorf("状态字段 %s visibility 无效: %s", field.Name, field.Visibility)
		}
	}
	if field.Type == "enum" && len(field.Options) == 0 {
		return fmt.Errorf("enum 状态字段 %s 缺少 options", field.Name)
	}
	if field.Default != nil {
		if _, err := normalizeActorStateValue(field, field.Default); err != nil {
			return err
		}
	}
	return nil
}

func validateActorStateRuntimeSchemaOps(target StoryDirectorActorStateSystem, ops []ActorStateRuntimeSchemaOp) error {
	for index, op := range ops {
		actorID := normalizeStatePanelActorID(firstNonEmptyString(op.ActorID, op.Actor.ID))
		if actorID == "" {
			return fmt.Errorf("运行时 Actor 操作缺少 actor_id: index=%d", index)
		}
		if op.Op == "remove" {
			if actorID == DefaultActorID || actorID == DefaultStoryContextActorID {
				return fmt.Errorf("故事基础运行时 Actor 不可删除: %s", actorID)
			}
			continue
		}
		if op.Op == "set" {
			if normalizeActorStateFieldName(op.FieldID) == "" {
				return fmt.Errorf("运行时 Actor 字段初始化缺少 field_id: actor=%s", actorID)
			}
			if op.Value == nil {
				return fmt.Errorf("运行时 Actor 字段初始化值不能为空: actor=%s field=%s", actorID, op.FieldID)
			}
			if op.Actor.ID != "" || op.Actor.Name != "" || op.Actor.TemplateID != "" || op.Actor.Role != "" || op.Actor.Description != "" || len(op.Actor.State) > 0 {
				return fmt.Errorf("运行时 Actor 字段级 set 不接受 actor 对象: actor=%s field=%s", actorID, op.FieldID)
			}
			continue
		}
		if op.Op != "add" && op.Op != "replace" {
			return fmt.Errorf("运行时 Actor 操作无效: %s", op.Op)
		}
		actor := op.Actor
		actor.ID = normalizeStatePanelActorID(actor.ID)
		if actor.ID == "" {
			return fmt.Errorf("运行时 Actor %s 的 actor.id 不能为空", actorID)
		}
		if actor.ID != actorID {
			return fmt.Errorf("运行时 Actor 操作不可改变 ID: %s -> %s", actorID, actor.ID)
		}
		actor.TemplateID = normalizeActorStateID(actor.TemplateID)
		template := actorStateTemplateByID(target, actor.TemplateID)
		if template.ID == "" {
			return fmt.Errorf("运行时 Actor %s 引用的模板不存在: %s", actorID, actor.TemplateID)
		}
		if actorID == DefaultActorID && actor.TemplateID != DefaultActorID {
			return fmt.Errorf("主角运行时 Actor 必须使用 %s 模板", DefaultActorID)
		}
		if actorID == DefaultStoryContextActorID && actor.TemplateID != ActorStateStoryContextTemplateID {
			return fmt.Errorf("故事上下文运行时 Actor 必须使用 %s 模板", ActorStateStoryContextTemplateID)
		}
		fields := actorStateFieldsByReference(template)
		for rawFieldID, value := range actor.State {
			field, ok := fields[actorStateFieldNameKey(rawFieldID)]
			if !ok {
				return fmt.Errorf("运行时 Actor 状态字段不在模板中: actor=%s template=%s field=%s", actorID, template.ID, rawFieldID)
			}
			if value == nil {
				return fmt.Errorf("运行时 Actor 状态值不能为空: actor=%s field=%s", actorID, actorStateFieldID(field))
			}
			if _, err := normalizeActorStateValue(field, value); err != nil {
				return fmt.Errorf("运行时 Actor %s: %w", actorID, err)
			}
		}
	}
	return nil
}

func validateActorStateTRPGReferences(system StoryDirectorActorStateSystem, trpg StoryDirectorTRPGSystem) error {
	trpg = normalizeFrozenTRPGSystem(trpg)
	for _, rule := range trpg.RuleTemplates {
		for _, binding := range rule.StateBindings {
			if err := validateTRPGBindingTemplate(system, binding.ID, "actor", binding.ActorTemplateID); err != nil {
				return err
			}
			if binding.TargetTemplateID != "" {
				if err := validateTRPGBindingTemplate(system, binding.ID, "target", binding.TargetTemplateID); err != nil {
					return err
				}
			}
			for _, modifier := range binding.Modifiers {
				if err := validateTRPGBindingField(system, binding, modifier.Source, modifier.FieldID, modifier.ValuePath, true); err != nil {
					return fmt.Errorf("TRPG binding %s modifier: %w", binding.ID, err)
				}
			}
			for _, ref := range binding.NarrativeStateRefs {
				if ref.Source == "scene" {
					continue
				}
				if err := validateTRPGBindingField(system, binding, ref.Source, ref.FieldID, nil, false); err != nil {
					return fmt.Errorf("TRPG binding %s narrative_state_ref: %w", binding.ID, err)
				}
			}
			for _, group := range binding.OutcomeStateChanges {
				for _, change := range group.StateChanges {
					if err := validateTRPGBindingField(system, binding, change.Source, change.FieldID, nil, true); err != nil {
						return fmt.Errorf("TRPG binding %s outcome_state_change: %w", binding.ID, err)
					}
					for _, term := range change.ChangeFormula.Terms {
						if err := validateTRPGBindingField(system, binding, term.Source, term.FieldID, term.ValuePath, true); err != nil {
							return fmt.Errorf("TRPG binding %s change_formula: %w", binding.ID, err)
						}
					}
				}
			}
		}
	}
	return nil
}

func validateTRPGBindingTemplate(system StoryDirectorActorStateSystem, bindingID, source, templateID string) error {
	if actorStateTemplateByID(system, templateID).ID == "" {
		return fmt.Errorf("TRPG binding %s 的 %s_template_id 引用不存在的状态模板: %s", bindingID, source, templateID)
	}
	return nil
}

func validateTRPGBindingField(system StoryDirectorActorStateSystem, binding RuleStateBinding, source, fieldID string, valuePath []string, numberRequired bool) error {
	templateID := binding.ActorTemplateID
	if source == "target" {
		templateID = binding.TargetTemplateID
	}
	template := actorStateTemplateByID(system, templateID)
	field, ok := actorStateFieldByID(template, fieldID)
	if !ok {
		return fmt.Errorf("字段不存在: template=%s field=%s", templateID, fieldID)
	}
	valuePath = normalizeRuleStateValuePath(valuePath)
	if numberRequired && len(valuePath) == 0 && field.Type != "number" {
		return fmt.Errorf("字段必须是 number，object 数值需提供 value_path: template=%s field=%s type=%s", templateID, fieldID, field.Type)
	}
	if numberRequired && len(valuePath) > 0 && field.Type != "object" {
		return fmt.Errorf("value_path 只能读取 object 字段: template=%s field=%s type=%s", templateID, fieldID, field.Type)
	}
	return nil
}
