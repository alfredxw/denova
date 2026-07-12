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
	if normalizeActorStateFieldName(field.Name) == "" {
		return fmt.Errorf("状态字段缺少 name")
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
				if err := validateTRPGBindingField(system, binding, modifier.Source, modifier.FieldID, true); err != nil {
					return fmt.Errorf("TRPG binding %s modifier: %w", binding.ID, err)
				}
			}
			for _, ref := range binding.NarrativeStateRefs {
				if ref.Source == "scene" {
					continue
				}
				if err := validateTRPGBindingField(system, binding, ref.Source, ref.FieldID, false); err != nil {
					return fmt.Errorf("TRPG binding %s narrative_state_ref: %w", binding.ID, err)
				}
			}
			for _, group := range binding.OutcomeStateChanges {
				for _, change := range group.StateChanges {
					if err := validateTRPGBindingField(system, binding, change.Source, change.FieldID, true); err != nil {
						return fmt.Errorf("TRPG binding %s outcome_state_change: %w", binding.ID, err)
					}
					for _, term := range change.ChangeFormula.Terms {
						if err := validateTRPGBindingField(system, binding, term.Source, term.FieldID, true); err != nil {
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

func validateTRPGBindingField(system StoryDirectorActorStateSystem, binding RuleStateBinding, source, fieldID string, numberRequired bool) error {
	templateID := binding.ActorTemplateID
	if source == "target" {
		templateID = binding.TargetTemplateID
	}
	template := actorStateTemplateByID(system, templateID)
	field, ok := actorStateFieldByID(template, fieldID)
	if !ok {
		return fmt.Errorf("字段不存在: template=%s field=%s", templateID, fieldID)
	}
	if numberRequired && field.Type != "number" {
		return fmt.Errorf("字段必须是 number: template=%s field=%s type=%s", templateID, fieldID, field.Type)
	}
	return nil
}
