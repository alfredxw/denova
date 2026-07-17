package interactive

import (
	"fmt"
	"strings"
)

func validateActorStateSchemaBatchValuePolicy(item ActorStateSchemaBatchItem, requirement ActorStateSchemaRequirementReview, path string, audit ActorStateSchemaBatchAudit) *ActorStateSchemaBatchIssue {
	policy := strings.TrimSpace(requirement.ValuePolicy)
	actorID := normalizeActorStateID(requirement.ActorID)
	switch policy {
	case ActorStateSchemaValuePolicySchemaOnly:
		if actorID != "" {
			issue := actorStateSchemaBatchIssue(item.ItemID, "unexpected_actor_id", path+".actor_id", "value_policy=schema_only 时不得指定 actor_id")
			return &issue
		}
	case ActorStateSchemaValuePolicyPreserve:
		if actorID == "" {
			issue := actorStateSchemaBatchIssue(item.ItemID, "missing_actor_id", path+".actor_id", "value_policy=preserve 时必须指定 actor_id")
			return &issue
		}
		currentFieldID := actorStateSchemaBatchPreservedFieldID(item.Adaptation, requirement.TemplateID, requirement.FieldID)
		templateID, value, exists := actorStateSchemaBatchCurrentActorValue(audit.CurrentState, actorID, currentFieldID)
		if normalizeActorStateID(templateID) == "" {
			issue := actorStateSchemaBatchIssue(item.ItemID, "actor_not_found", path+".actor_id", fmt.Sprintf("Actor %s 不存在，无法保留字段 %s 的当前值", actorID, requirement.FieldID))
			return &issue
		}
		if normalizeActorStateID(templateID) != normalizeActorStateID(requirement.TemplateID) {
			issue := actorStateSchemaBatchIssue(item.ItemID, "actor_template_mismatch", path+".template_id", fmt.Sprintf("Actor %s 使用模板 %s，不能按模板 %s 保留字段 %s", actorID, templateID, requirement.TemplateID, requirement.FieldID))
			return &issue
		}
		if !exists || !actorStateSchemaBatchValueInitialized(value) {
			issue := actorStateSchemaBatchIssue(item.ItemID, "actor_value_not_initialized", path+".value_policy", fmt.Sprintf("Actor %s 的字段 %s 没有可保留的当前值；请改用 initialize 并提交字段级 actor_ops set，或明确 defer", actorID, requirement.FieldID))
			return &issue
		}
	case ActorStateSchemaValuePolicyInitialize:
		if actorID == "" {
			issue := actorStateSchemaBatchIssue(item.ItemID, "missing_actor_id", path+".actor_id", "value_policy=initialize 时必须指定 actor_id")
			return &issue
		}
		if issue := validateActorStateSchemaBatchActorTemplate(item, requirement, path, audit.CurrentState); issue != nil {
			return issue
		}
		if !actorStateSchemaBatchHasActorValueOp(item.Adaptation, actorID, requirement.FieldID) {
			itemPath := path
			if index := strings.Index(itemPath, ".requirements["); index >= 0 {
				itemPath = itemPath[:index]
			}
			issue := actorStateSchemaBatchIssue(item.ItemID, "missing_actor_value_initialization", itemPath+".adaptation.actor_ops", fmt.Sprintf("Actor %s 的确定字段 %s 尚未初始化；请在同一 item 提交 {op:set, actor_id, field_id, value}", actorID, requirement.FieldID))
			return &issue
		}
	case ActorStateSchemaValuePolicyDefer:
		if actorID == "" {
			issue := actorStateSchemaBatchIssue(item.ItemID, "missing_actor_id", path+".actor_id", "value_policy=defer 时必须指定 actor_id")
			return &issue
		}
		if issue := validateActorStateSchemaBatchActorTemplate(item, requirement, path, audit.CurrentState); issue != nil {
			return issue
		}
		if strings.TrimSpace(requirement.Reason) == "" {
			issue := actorStateSchemaBatchIssue(item.ItemID, "missing_defer_reason", path+".reason", "延后初始化必须说明当前无法可靠确定值的原因")
			return &issue
		}
		if actorStateSchemaBatchHasActorValueOp(item.Adaptation, actorID, requirement.FieldID) {
			issue := actorStateSchemaBatchIssue(item.ItemID, "deferred_value_has_initialization", path+".value_policy", "defer 字段不能同时提交初始化值；已有可靠值时改用 initialize")
			return &issue
		}
	case "":
		issue := actorStateSchemaBatchIssue(item.ItemID, "missing_value_policy", path+".value_policy", "value_policy 必须是 schema_only、preserve、initialize 或 defer")
		return &issue
	default:
		issue := actorStateSchemaBatchIssue(item.ItemID, "invalid_value_policy", path+".value_policy", "value_policy 必须是 schema_only、preserve、initialize 或 defer")
		return &issue
	}
	return nil
}

func validateActorStateSchemaBatchActorTemplate(item ActorStateSchemaBatchItem, requirement ActorStateSchemaRequirementReview, path string, currentState map[string]any) *ActorStateSchemaBatchIssue {
	actualTemplateID, exists := actorStateSchemaBatchActorTemplate(currentState, item.Adaptation, requirement.ActorID)
	if !exists {
		issue := actorStateSchemaBatchIssue(item.ItemID, "actor_not_found", path+".actor_id", fmt.Sprintf("Actor %s 不存在；字段级初始化和延后策略只能指向当前或同一 item 新增的 Actor", requirement.ActorID))
		return &issue
	}
	if normalizeActorStateID(actualTemplateID) != normalizeActorStateID(requirement.TemplateID) {
		issue := actorStateSchemaBatchIssue(item.ItemID, "actor_template_mismatch", path+".template_id", fmt.Sprintf("Actor %s 使用模板 %s，不能按模板 %s 初始化字段 %s", requirement.ActorID, actualTemplateID, requirement.TemplateID, requirement.FieldID))
		return &issue
	}
	return nil
}

func actorStateSchemaBatchActorTemplate(state map[string]any, adaptation ActorStateSchemaAdaptation, actorID string) (string, bool) {
	actorID = normalizeActorStateID(actorID)
	for _, op := range adaptation.InitialActorOps {
		if op.Op != "add" && op.Op != "replace" {
			continue
		}
		if normalizeActorStateID(firstNonEmptyString(op.ActorID, op.Actor.ID)) == actorID && normalizeActorStateID(op.Actor.TemplateID) != "" {
			return op.Actor.TemplateID, true
		}
	}
	for _, op := range adaptation.ActorOps {
		if op.Op != "add" && op.Op != "replace" {
			continue
		}
		if normalizeActorStateID(firstNonEmptyString(op.ActorID, op.Actor.ID)) == actorID && normalizeActorStateID(op.Actor.TemplateID) != "" {
			return op.Actor.TemplateID, true
		}
	}
	actors, _ := state[actorStateRoot].(map[string]any)
	if actor, ok := actors[actorID].(map[string]any); ok {
		templateID, _ := actor["template_id"].(string)
		if templateID = normalizeActorStateID(templateID); templateID != "" {
			return templateID, true
		}
	}
	return "", false
}

func actorStateSchemaBatchPreservedFieldID(adaptation ActorStateSchemaAdaptation, templateID, targetFieldID string) string {
	templateID = normalizeActorStateID(templateID)
	targetFieldID = normalizeActorStateFieldName(targetFieldID)
	for _, templateOp := range adaptation.TemplateOps {
		if templateOp.Op != "fields" || normalizeActorStateID(templateOp.TemplateID) != templateID {
			continue
		}
		for _, fieldOp := range templateOp.FieldOps {
			if fieldOp.Op == "replace" && normalizeActorStateFieldName(actorStateFieldID(fieldOp.Field)) == targetFieldID {
				return normalizeActorStateFieldName(fieldOp.FieldID)
			}
		}
	}
	return targetFieldID
}

func actorStateSchemaBatchCurrentActorValue(state map[string]any, actorID, fieldID string) (string, any, bool) {
	actors, _ := state[actorStateRoot].(map[string]any)
	actor, _ := actors[normalizeActorStateID(actorID)].(map[string]any)
	if actor == nil {
		return "", nil, false
	}
	values, _ := actor["state"].(map[string]any)
	value, exists := values[normalizeActorStateFieldName(fieldID)]
	templateID, _ := actor["template_id"].(string)
	return templateID, value, exists
}

func actorStateSchemaBatchHasActorValueOp(adaptation ActorStateSchemaAdaptation, actorID, fieldID string) bool {
	actorID = normalizeActorStateID(actorID)
	fieldID = normalizeActorStateFieldName(fieldID)
	for _, op := range adaptation.InitialActorOps {
		if normalizeActorStateID(firstNonEmptyString(op.ActorID, op.Actor.ID)) != actorID {
			continue
		}
		if value, exists := op.Actor.State[fieldID]; exists && value != nil {
			return true
		}
	}
	for _, op := range adaptation.ActorOps {
		if normalizeActorStateID(firstNonEmptyString(op.ActorID, op.Actor.ID)) != actorID {
			continue
		}
		if op.Op == "set" && normalizeActorStateFieldName(op.FieldID) == fieldID && op.Value != nil {
			return true
		}
		if value, exists := op.Actor.State[fieldID]; exists && value != nil {
			return true
		}
	}
	return false
}

func actorStateSchemaBatchValueInitialized(value any) bool {
	if value == nil {
		return false
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text) != ""
	}
	return true
}
