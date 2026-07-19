package interactive

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

const stateSchemaMigrationSourceKind = "state_schema_initialization"

// prepareOpeningGameStateSchemaCommit prepares the schema and initial Actor
// state in memory. The caller persists the returned operations together with
// the opening Turn, so any later validation or write error leaves both schema
// and state untouched on disk.
func prepareOpeningGameStateSchemaCommit(meta *StoryMeta, events []StoryEventRecord, state map[string]any, actorState StoryDirectorActorStateSystem, branchID, sourceTurnID, now string, proposal *ActorStateSchemaProposal) (StoryDirectorActorStateSystem, []StateOp, []ActorStateOp, error) {
	if meta == nil {
		return StoryDirectorActorStateSystem{}, nil, nil, fmt.Errorf("故事元信息不存在")
	}
	policy := meta.StateSchemaPolicy
	if !storyStateSchemaPolicyRequiresOpeningDraft(policy) {
		if proposal != nil {
			return StoryDirectorActorStateSystem{}, nil, nil, fmt.Errorf("固定状态模板故事不能提交开局结构提案")
		}
		return actorState, nil, nil, nil
	}
	if meta.ActorStateSchema == nil {
		return StoryDirectorActorStateSystem{}, nil, nil, fmt.Errorf("故事缺少冻结状态结构")
	}
	actorState = meta.ActorStateSchema.System
	if meta.StateSchemaInitialization == nil {
		return StoryDirectorActorStateSystem{}, nil, nil, fmt.Errorf("故事缺少状态结构初始化状态")
	}
	status := *meta.StateSchemaInitialization
	rawActors, _ := state[actorStateRoot].(map[string]any)
	hasActors := len(rawActors) > 0
	if status.Status == StateSchemaInitializationReady {
		if proposal != nil {
			return StoryDirectorActorStateSystem{}, nil, nil, fmt.Errorf("故事状态结构已经冻结，不能再次提交开局结构提案")
		}
		if hasActors {
			return actorState, nil, nil, nil
		}
		ops, actorOps, err := BuildActorStateInitialChanges(actorState, meta.InitialTraitRolls)
		if err != nil {
			return StoryDirectorActorStateSystem{}, nil, nil, err
		}
		markOpeningStateBootstrapSources(ops, actorOps, sourceTurnID)
		for _, op := range ops {
			applyStateOp(state, op)
		}
		for _, op := range actorOps {
			applyActorStateOp(state, op)
		}
		return actorState, normalizeStateOps(ops), normalizeActorStateOps(actorOps), nil
	}
	if status.Status != StateSchemaInitializationWaitingOpening {
		return StoryDirectorActorStateSystem{}, nil, nil, fmt.Errorf("状态结构初始化状态不允许提交开局: %s", status.Status)
	}
	if proposal == nil {
		return StoryDirectorActorStateSystem{}, nil, nil, fmt.Errorf("开局 Game Agent 尚未完成状态结构草案")
	}
	if hasActors {
		return StoryDirectorActorStateSystem{}, nil, nil, fmt.Errorf("动态状态结构故事在开局提交前不应存在已物化 Actor")
	}
	if len(meta.Branches) != 1 || branchID != meta.CurrentBranch {
		return StoryDirectorActorStateSystem{}, nil, nil, fmt.Errorf("状态结构只能在唯一的初始分支上完成")
	}
	if storyContainsTurn(events) {
		return StoryDirectorActorStateSystem{}, nil, nil, fmt.Errorf("状态结构只能随首个故事回合原子提交")
	}
	normalized, _, err := ValidateOpeningGameStateSchemaProposal(meta.ActorStateSchema.System, meta.ActorStateSchema.TRPGSystem, *proposal)
	if err != nil {
		return StoryDirectorActorStateSystem{}, nil, nil, err
	}
	targetSystem, record, err := ApplyActorStateSchemaAdaptation(meta.ActorStateSchema.System, meta.ActorStateSchema.TRPGSystem, normalized.Adaptation)
	if err != nil {
		return StoryDirectorActorStateSystem{}, nil, nil, err
	}
	schemaChanged := !reflect.DeepEqual(normalizeActorStateSystem(meta.ActorStateSchema.System), normalizeActorStateSystem(targetSystem))
	_, _, aliases, warnings, err := buildStateSchemaMigration(meta.ActorStateSchema.System, targetSystem, state, normalized.Adaptation, sourceTurnID)
	if err != nil {
		return StoryDirectorActorStateSystem{}, nil, nil, err
	}
	record.Source = "game_agent"
	record.SourceTurnID = sourceTurnID
	record.Summary = firstNonEmptyString(normalized.Summary, record.Summary)
	record.LoreRevision = strings.TrimSpace(normalized.SourceLoreRevision)
	record.ReviewedLoreIDs = append([]string(nil), normalized.ReviewedLoreIDs...)
	record.Requirements = append([]ActorStateSchemaRequirementReview(nil), normalized.Requirements...)
	record.Changes = stateSchemaAdaptationChanges(normalized.Adaptation)
	record.Warnings = warnings
	target := FreezeActorStateSchemaWithRules(targetSystem, meta.ActorStateSchema.TRPGSystem, false)
	target.Revision = status.BaseRevision
	if target.Revision <= 0 {
		target.Revision = actorStateSchemaRevision(meta.ActorStateSchema)
	}
	if schemaChanged {
		target.Revision++
	}
	target.Adaptation = &record
	target.LegacyFieldPaths = mergeLegacyFieldAliases(meta.ActorStateSchema.LegacyFieldPaths, aliases)
	target.FieldMigrations = mergeActorStateFieldMigrations(meta.ActorStateSchema.FieldMigrations, stateSchemaFieldMigrations(normalized.Adaptation))
	ops, actorOps, err := BuildActorStateInitialChanges(targetSystem, meta.InitialTraitRolls)
	if err != nil {
		return StoryDirectorActorStateSystem{}, nil, nil, err
	}
	markOpeningStateBootstrapSources(ops, actorOps, sourceTurnID)
	for _, op := range ops {
		applyStateOp(state, op)
	}
	for _, op := range actorOps {
		applyActorStateOp(state, op)
	}
	status.Status = StateSchemaInitializationReady
	status.Outcome = "unchanged"
	if schemaChanged {
		status.Outcome = "changed"
	}
	status.SourceTurnID = sourceTurnID
	status.TargetRevision = target.Revision
	status.Summary = record.Summary
	status.LoreRevision = record.LoreRevision
	status.ReviewedLoreIDs = append([]string(nil), record.ReviewedLoreIDs...)
	status.Requirements = append([]ActorStateSchemaRequirementReview(nil), record.Requirements...)
	status.Changes = append([]ActorStateSchemaAdaptationChange(nil), record.Changes...)
	status.Warnings = append([]string(nil), warnings...)
	status.StartedAt = now
	status.CompletedAt = now
	status.UpdatedAt = now
	meta.ActorStateSchema = target
	meta.StateSchemaInitialization = &status
	return targetSystem, normalizeStateOps(ops), normalizeActorStateOps(actorOps), nil
}

func markOpeningStateBootstrapSources(ops []StateOp, actorOps []ActorStateOp, sourceTurnID string) {
	for index := range ops {
		ops[index].SourceKind = stateSchemaMigrationSourceKind
		ops[index].SourceID = "opening_state_schema"
		ops[index].SourceTurnID = sourceTurnID
	}
	for index := range actorOps {
		actorOps[index].SourceKind = stateSchemaMigrationSourceKind
		actorOps[index].SourceID = "opening_state_schema"
		actorOps[index].SourceTurnID = sourceTurnID
	}
}

func actorStateSchemaRevision(snapshot *ActorStateSchemaSnapshot) int {
	if snapshot == nil || snapshot.Revision <= 0 {
		return 1
	}
	return snapshot.Revision
}

func buildStateSchemaMigration(base, target StoryDirectorActorStateSystem, state map[string]any, adaptation ActorStateSchemaAdaptation, sourceTurnID string) ([]StateOp, []ActorStateOp, map[string]map[string]string, []string, error) {
	rawActors, _ := state[actorStateRoot].(map[string]any)
	actorChanges, actorFieldChanges, err := splitStateSchemaActorChanges(adaptation)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	fieldSources := map[string]map[string]string{}
	aliases := map[string]map[string]string{}
	for _, templateOp := range adaptation.TemplateOps {
		if templateOp.TemplateID == "" {
			continue
		}
		for _, fieldOp := range templateOp.FieldOps {
			if fieldOp.Op != "replace" {
				continue
			}
			targetID := actorStateFieldID(fieldOp.Field)
			if targetID == "" || fieldOp.FieldID == "" {
				continue
			}
			if fieldSources[templateOp.TemplateID] == nil {
				fieldSources[templateOp.TemplateID] = map[string]string{}
			}
			fieldSources[templateOp.TemplateID][targetID] = fieldOp.FieldID
			if targetID != fieldOp.FieldID {
				if aliases[templateOp.TemplateID] == nil {
					aliases[templateOp.TemplateID] = map[string]string{}
				}
				aliases[templateOp.TemplateID][fieldOp.FieldID] = targetID
			}
		}
	}
	var ops []StateOp
	var actorOps []ActorStateOp
	var warnings []string
	schemaChanged := !reflect.DeepEqual(normalizeActorStateSystem(base), normalizeActorStateSystem(target))
	handledActors := map[string]bool{}
	for actorID, actorChange := range actorChanges {
		valueSourceID := stateSchemaActorValueSourceID(actorChange.ValueSource)
		if actorChange.Op == "remove" {
			if _, exists := rawActors[actorID]; exists {
				ops = append(ops, StateOp{Op: "unset", Path: actorStateRoot + "." + actorID, Reason: actorChange.Reason, SourceID: valueSourceID})
			}
			handledActors[actorID] = true
			continue
		}
		if actorChange.Op != "add" && actorChange.Op != "replace" {
			continue
		}
		actor := actorChange.Actor
		template := actorStateTemplateByID(target, actor.TemplateID)
		if template.ID == "" {
			return nil, nil, nil, nil, fmt.Errorf("Actor %s 的目标模板不存在: %s", actorID, actor.TemplateID)
		}
		current, _ := rawActors[actorID].(map[string]any)
		actorState := actor.State
		var cleanupOps []ActorStateOp
		var err error
		if current != nil {
			currentTemplateID := normalizeActorStateID(fmt.Sprint(current["template_id"]))
			baseTemplate := actorStateTemplateByID(base, currentTemplateID)
			currentValues, _ := current["state"].(map[string]any)
			var migrationWarnings []string
			actorState, cleanupOps, migrationWarnings, err = resolveStateSchemaActorValues(actorID, baseTemplate, template, currentValues, actor.State, fieldSources[template.ID])
			if err != nil {
				return nil, nil, nil, nil, err
			}
			warnings = append(warnings, migrationWarnings...)
			currentName, _ := current["name"].(string)
			currentRole, _ := current["role"].(string)
			currentDescription, _ := current["description"].(string)
			actor.Name = firstNonEmptyString(actor.Name, currentName)
			actor.Role = firstNonEmptyString(actor.Role, currentRole)
			actor.Description = firstNonEmptyString(actor.Description, currentDescription)
		}
		baseOps, baseActorOps, _, err := buildNewActorStateOps(template, actorID, actor.Name, actor.Role, actor.Description, actorState, "状态结构适配", sourceTurnID)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		if current, ok := rawActors[actorID].(map[string]any); ok {
			if traits := current["traits"]; traits != nil {
				baseOps = append(baseOps, StateOp{Op: "set", Path: actorStateActorPath(actorID, "traits"), Value: traits, SourceID: valueSourceID})
			}
		}
		for index := range baseOps {
			baseOps[index].SourceID = valueSourceID
		}
		for index := range baseActorOps {
			baseActorOps[index].SourceID = valueSourceID
		}
		for index := range cleanupOps {
			cleanupOps[index].SourceID = valueSourceID
		}
		ops = append(ops, baseOps...)
		actorOps = append(actorOps, baseActorOps...)
		actorOps = append(actorOps, cleanupOps...)
		handledActors[actorID] = true
	}
	for actorID, rawActor := range rawActors {
		if handledActors[actorID] {
			continue
		}
		actor, _ := rawActor.(map[string]any)
		if actor == nil {
			continue
		}
		templateID := normalizeActorStateID(fmt.Sprint(actor["template_id"]))
		baseTemplate := actorStateTemplateByID(base, templateID)
		targetTemplate := actorStateTemplateByID(target, templateID)
		if targetTemplate.ID == "" {
			return nil, nil, nil, nil, fmt.Errorf("模板 %s 已删除，但 Actor %s 没有迁移或删除操作", templateID, actorID)
		}
		fieldChanges := actorFieldChanges[actorID]
		if !schemaChanged && len(fieldChanges) == 0 {
			continue
		}
		values, _ := actor["state"].(map[string]any)
		if values == nil {
			values = map[string]any{}
		}
		fieldIDs := sortedStateSchemaActorFieldChangeIDs(fieldChanges)
		if !schemaChanged {
			fieldOps, err := buildStateSchemaActorFieldInitializationOps(targetTemplate, actorID, fieldIDs, fieldChanges, sourceTurnID)
			if err != nil {
				return nil, nil, nil, nil, err
			}
			actorOps = append(actorOps, fieldOps...)
			handledActors[actorID] = true
			continue
		}
		explicitValues := map[string]any{}
		for _, fieldID := range fieldIDs {
			explicitValues[fieldID] = fieldChanges[fieldID].Value
		}
		migratedValues, cleanupOps, migrationWarnings, err := resolveStateSchemaActorValues(actorID, baseTemplate, targetTemplate, values, explicitValues, fieldSources[templateID])
		if err != nil {
			return nil, nil, nil, nil, err
		}
		valueOps, _, err := buildActorStateValueOps(targetTemplate, actorID, migratedValues, "状态结构适配", sourceTurnID)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		applyStateSchemaActorFieldProvenance(valueOps, fieldChanges)
		actorOps = append(actorOps, valueOps...)
		actorOps = append(actorOps, cleanupOps...)
		warnings = append(warnings, migrationWarnings...)
		handledActors[actorID] = true
	}
	for actorID := range actorFieldChanges {
		if !handledActors[actorID] {
			return nil, nil, nil, nil, fmt.Errorf("Actor 字段初始化目标不存在: %s", actorID)
		}
	}
	if len(warnings) > maxActorStateSchemaAdaptationOps {
		warnings = warnings[:maxActorStateSchemaAdaptationOps]
	}
	return ops, actorOps, aliases, warnings, nil
}

// resolveStateSchemaActorValues carries existing runtime values into a target
// template. Explicit non-null values win; defaults only fill genuinely absent
// fields. Cleanup operations remain ordered after the replacement sets.
func resolveStateSchemaActorValues(actorID string, baseTemplate, targetTemplate ActorStateTemplate, current, explicit map[string]any, fieldSources map[string]string) (map[string]any, []ActorStateOp, []string, error) {
	if current == nil {
		current = map[string]any{}
	}
	explicitValues := map[string]any{}
	fieldByReference := actorStateFieldsByReference(targetTemplate)
	explicitKeys := make([]string, 0, len(explicit))
	for key := range explicit {
		explicitKeys = append(explicitKeys, key)
	}
	sort.Strings(explicitKeys)
	for _, rawKey := range explicitKeys {
		value := explicit[rawKey]
		if value == nil {
			continue
		}
		key := strings.TrimSpace(rawKey)
		field, ok := fieldByReference[actorStateFieldNameKey(key)]
		if !ok {
			return nil, nil, nil, fmt.Errorf("Actor 状态字段不在目标模板中: actor=%s template=%s field=%s", actorID, targetTemplate.ID, key)
		}
		normalized, err := normalizeActorStateValue(field, value)
		if err != nil {
			return nil, nil, nil, err
		}
		explicitValues[actorStateFieldID(field)] = normalized
	}

	values := map[string]any{}
	targetIDs := map[string]bool{}
	migratedSourceIDs := map[string]bool{}
	for _, sourceID := range fieldSources {
		migratedSourceIDs[sourceID] = true
	}
	cleanupOps := []ActorStateOp{}
	for _, field := range targetTemplate.Fields {
		targetID := actorStateFieldID(field)
		targetIDs[targetID] = true
		sourceID := firstNonEmptyString(fieldSources[targetID], targetID)
		value, exists := explicitValues[targetID]
		if !exists {
			value, exists = current[sourceID]
			if exists && value == nil {
				exists = false
			}
		}
		if !exists && sourceID != targetID {
			value, exists = current[targetID]
			if exists && value == nil {
				exists = false
			}
		}
		if !exists {
			continue
		}
		converted, ok := coerceActorStateFieldValue(value, field)
		if !ok || converted == nil {
			return nil, nil, nil, fmt.Errorf("Actor %s 的现有字段 %s 无法转换为 %s；请提供显式非空迁移值", actorID, targetID, field.Type)
		}
		values[targetID] = converted
		if sourceID != targetID {
			if _, sourceExists := current[sourceID]; sourceExists {
				cleanupOps = append(cleanupOps, ActorStateOp{Op: "unset", ActorID: actorID, FieldID: sourceID, Reason: "字段已迁移到 " + targetID})
			}
		}
	}
	for _, field := range baseTemplate.Fields {
		fieldID := actorStateFieldID(field)
		if targetIDs[fieldID] || migratedSourceIDs[fieldID] {
			continue
		}
		if _, exists := current[fieldID]; exists {
			cleanupOps = append(cleanupOps, ActorStateOp{Op: "unset", ActorID: actorID, FieldID: fieldID, Reason: "字段已从故事状态结构移除"})
		}
	}
	return values, cleanupOps, nil, nil
}

func coerceActorStateFieldValue(value any, field ActorStateField) (any, bool) {
	if value == nil {
		return nil, field.Default == nil
	}
	switch field.Type {
	case "number":
		if number, ok := actorStateNumber(value); ok {
			return clampActorStateNumber(number, field), true
		}
		if text, ok := value.(string); ok {
			number, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
			if err == nil {
				return clampActorStateNumber(number, field), true
			}
		}
	case "bool":
		if boolean, ok := value.(bool); ok {
			return boolean, true
		}
		if text, ok := value.(string); ok {
			boolean, err := strconv.ParseBool(strings.TrimSpace(text))
			if err == nil {
				return boolean, true
			}
		}
	case "enum":
		text, ok := value.(string)
		if ok {
			for _, option := range field.Options {
				if text == option {
					return text, true
				}
			}
		}
	case "object":
		if object, ok := value.(map[string]any); ok {
			return object, true
		}
	case "list":
		if list, ok := value.([]any); ok {
			return list, true
		}
	default:
		if text, ok := value.(string); ok {
			return text, true
		}
		switch value.(type) {
		case float64, float32, int, int64, int32, bool, json.Number:
			return fmt.Sprint(value), true
		}
	}
	return nil, false
}

func clampActorStateNumber(value float64, field ActorStateField) float64 {
	if field.Min != nil && value < *field.Min {
		value = *field.Min
	}
	if field.Max != nil && value > *field.Max {
		value = *field.Max
	}
	return value
}

func mergeLegacyFieldAliases(current, additions map[string]map[string]string) map[string]map[string]string {
	result := map[string]map[string]string{}
	for templateID, aliases := range current {
		result[templateID] = map[string]string{}
		for from, to := range aliases {
			result[templateID][from] = to
		}
	}
	for templateID, aliases := range additions {
		if result[templateID] == nil {
			result[templateID] = map[string]string{}
		}
		for from, to := range aliases {
			result[templateID][from] = to
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func stateSchemaFieldMigrations(adaptation ActorStateSchemaAdaptation) map[string][]ActorStateFieldMigration {
	result := map[string][]ActorStateFieldMigration{}
	for _, templateOp := range adaptation.TemplateOps {
		for _, fieldOp := range templateOp.FieldOps {
			if fieldOp.Op != "replace" || fieldOp.FieldID == "" || actorStateFieldID(fieldOp.Field) == "" {
				continue
			}
			result[templateOp.TemplateID] = append(result[templateOp.TemplateID], ActorStateFieldMigration{From: fieldOp.FieldID, To: actorStateFieldID(fieldOp.Field), Field: fieldOp.Field})
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func mergeActorStateFieldMigrations(current, additions map[string][]ActorStateFieldMigration) map[string][]ActorStateFieldMigration {
	result := map[string][]ActorStateFieldMigration{}
	for templateID, migrations := range current {
		result[templateID] = append([]ActorStateFieldMigration(nil), migrations...)
	}
	for templateID, migrations := range additions {
		for _, migration := range migrations {
			replaced := false
			for index := range result[templateID] {
				if result[templateID][index].From == migration.From {
					result[templateID][index] = migration
					replaced = true
					break
				}
			}
			if !replaced {
				result[templateID] = append(result[templateID], migration)
			}
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}
