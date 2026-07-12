package interactive

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const stateSchemaMigrationSourceKind = "state_schema_initialization"

func rejectMutationDuringStateSchemaInitialization(meta StoryMeta) error {
	if meta.StateSchemaInitialization != nil && meta.StateSchemaInitialization.Status == StateSchemaInitializationRunning {
		return fmt.Errorf("状态结构正在根据首轮正文适配，请等待完成后再修改故事历史")
	}
	return nil
}

func (s *Store) StateSchemaInitializationStatus(storyID string) (StateSchemaInitializationStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	meta, _, err := s.readStoryLocked(storyID)
	if err != nil {
		return StateSchemaInitializationStatus{}, err
	}
	if meta.StateSchemaInitialization == nil {
		return StateSchemaInitializationStatus{Mode: StateSchemaAdaptationModeOff, Status: StateSchemaInitializationSkipped}, nil
	}
	return *meta.StateSchemaInitialization, nil
}

func (s *Store) ClaimStateSchemaInitialization(storyID, sourceTurnID string) (StateSchemaInitializationStatus, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return StateSchemaInitializationStatus{}, false, err
	}
	if meta.StateSchemaInitialization == nil || meta.StateSchemaInitialization.Mode == StateSchemaAdaptationModeOff {
		return StateSchemaInitializationStatus{Mode: StateSchemaAdaptationModeOff, Status: StateSchemaInitializationSkipped}, false, nil
	}
	status := *meta.StateSchemaInitialization
	if status.Status == StateSchemaInitializationReady || status.Status == StateSchemaInitializationSkipped || status.Status == StateSchemaInitializationRunning {
		return status, false, nil
	}
	sourceTurnID = strings.TrimSpace(sourceTurnID)
	if sourceTurnID == "" {
		return status, false, fmt.Errorf("状态结构初始化缺少首轮回合")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	status.Status = StateSchemaInitializationRunning
	status.SourceTurnID = sourceTurnID
	status.BaseRevision = actorStateSchemaRevision(meta.ActorStateSchema)
	status.TargetRevision = status.BaseRevision + 1
	status.Error = ""
	status.StartedAt = now
	status.CompletedAt = ""
	status.UpdatedAt = now
	meta.StateSchemaInitialization = &status
	meta.UpdatedAt = now
	if err := s.rewriteStoryLocked(storyID, meta, lines); err != nil {
		return StateSchemaInitializationStatus{}, false, err
	}
	return status, true, nil
}

func (s *Store) MarkStateSchemaInitializationFailed(storyID, sourceTurnID string, cause error) error {
	if cause == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return err
	}
	if meta.StateSchemaInitialization == nil || meta.StateSchemaInitialization.Status != StateSchemaInitializationRunning {
		return nil
	}
	if source := strings.TrimSpace(sourceTurnID); source != "" && meta.StateSchemaInitialization.SourceTurnID != source {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	status := *meta.StateSchemaInitialization
	status.Status = StateSchemaInitializationFailed
	status.Error = trimBytes(cause.Error(), maxTurnBriefTextBytes)
	status.CompletedAt = now
	status.UpdatedAt = now
	meta.StateSchemaInitialization = &status
	meta.UpdatedAt = now
	return s.rewriteStoryLocked(storyID, meta, lines)
}

func (s *Store) ResetStateSchemaInitialization(storyID string) (StateSchemaInitializationStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return StateSchemaInitializationStatus{}, err
	}
	if meta.StateSchemaInitialization == nil || meta.StateSchemaInitialization.Mode == StateSchemaAdaptationModeOff {
		return StateSchemaInitializationStatus{}, fmt.Errorf("当前故事已固定使用原始状态预设")
	}
	status := *meta.StateSchemaInitialization
	if status.Status == StateSchemaInitializationReady {
		return status, nil
	}
	if status.Status == StateSchemaInitializationRunning {
		return status, fmt.Errorf("状态结构初始化正在运行")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	status.Status = StateSchemaInitializationWaitingOpening
	status.Error = ""
	status.CompletedAt = ""
	status.UpdatedAt = now
	meta.StateSchemaInitialization = &status
	meta.UpdatedAt = now
	if err := s.rewriteStoryLocked(storyID, meta, lines); err != nil {
		return StateSchemaInitializationStatus{}, err
	}
	return status, nil
}

func (s *Store) ResumeInterruptedStateSchemaInitialization(storyID string) (StateSchemaInitializationStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return StateSchemaInitializationStatus{}, err
	}
	if meta.StateSchemaInitialization == nil {
		return StateSchemaInitializationStatus{}, fmt.Errorf("故事没有动态状态结构初始化任务")
	}
	status := *meta.StateSchemaInitialization
	if status.Status != StateSchemaInitializationRunning && status.Status != StateSchemaInitializationWaitingOpening {
		return status, nil
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	status.Status = StateSchemaInitializationWaitingOpening
	status.Error = ""
	status.CompletedAt = ""
	status.UpdatedAt = now
	meta.StateSchemaInitialization = &status
	meta.UpdatedAt = now
	if err := s.rewriteStoryLocked(storyID, meta, lines); err != nil {
		return StateSchemaInitializationStatus{}, err
	}
	return status, nil
}

func (s *Store) SkipStateSchemaInitialization(storyID string) (StateSchemaInitializationStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return StateSchemaInitializationStatus{}, err
	}
	status := StateSchemaInitializationStatus{Mode: StateSchemaAdaptationModeOff, Status: StateSchemaInitializationSkipped, BaseRevision: actorStateSchemaRevision(meta.ActorStateSchema)}
	if meta.StateSchemaInitialization != nil {
		status = *meta.StateSchemaInitialization
		if status.Status == StateSchemaInitializationRunning {
			return status, fmt.Errorf("状态结构初始化正在运行，完成后再执行固定操作")
		}
		if status.Status == StateSchemaInitializationReady {
			return status, fmt.Errorf("状态结构已经完成动态适配")
		}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	status.Mode = StateSchemaAdaptationModeOff
	status.Status = StateSchemaInitializationSkipped
	status.Error = ""
	status.CompletedAt = now
	status.UpdatedAt = now
	meta.StateSchemaInitialization = &status
	meta.UpdatedAt = now
	if err := s.rewriteStoryLocked(storyID, meta, lines); err != nil {
		return StateSchemaInitializationStatus{}, err
	}
	return status, nil
}

func (s *Store) ApplyStateSchemaInitialization(storyID, branchID, sourceTurnID string, adaptation ActorStateSchemaAdaptation) (StateSchemaInitializationStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return StateSchemaInitializationStatus{}, err
	}
	if meta.StateSchemaInitialization == nil || meta.StateSchemaInitialization.Status != StateSchemaInitializationRunning {
		return StateSchemaInitializationStatus{}, fmt.Errorf("状态结构初始化未处于运行状态")
	}
	status := *meta.StateSchemaInitialization
	if strings.TrimSpace(sourceTurnID) == "" || status.SourceTurnID != strings.TrimSpace(sourceTurnID) {
		return status, fmt.Errorf("状态结构初始化源回合已变化")
	}
	if status.BaseRevision != actorStateSchemaRevision(meta.ActorStateSchema) {
		return status, fmt.Errorf("状态结构 revision 已变化: expected=%d current=%d", status.BaseRevision, actorStateSchemaRevision(meta.ActorStateSchema))
	}
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	branch, ok := meta.Branches[branchID]
	if !ok {
		return status, fmt.Errorf("分支不存在: %s", branchID)
	}
	if len(meta.Branches) != 1 {
		return status, fmt.Errorf("状态结构初始化期间检测到多个分支，拒绝自动迁移")
	}
	path, _ := eventPath(branch.Head, eventsByID(lines))
	state := stateFromPath(path)
	applyLegacyActorStateAliases(state, meta.ActorStateSchema)
	targetSystem, record, err := ApplyActorStateSchemaAdaptation(meta.ActorStateSchema.System, meta.ActorStateSchema.TRPGSystem, adaptation)
	if err != nil {
		return status, err
	}
	record.SourceTurnID = sourceTurnID
	record.Changes = stateSchemaAdaptationChanges(adaptation)
	ops, actorOps, aliases, warnings, err := buildStateSchemaMigration(meta.ActorStateSchema.System, targetSystem, state, adaptation, sourceTurnID)
	if err != nil {
		return status, err
	}
	record.Warnings = warnings
	target := FreezeActorStateSchemaWithRules(targetSystem, meta.ActorStateSchema.TRPGSystem, false)
	target.Revision = status.TargetRevision
	target.Adaptation = &record
	target.LegacyFieldPaths = mergeLegacyFieldAliases(meta.ActorStateSchema.LegacyFieldPaths, aliases)
	target.FieldMigrations = mergeActorStateFieldMigrations(meta.ActorStateSchema.FieldMigrations, stateSchemaFieldMigrations(adaptation))
	if err := s.backupStoryBeforeStateSchemaMigration(storyID); err != nil {
		return status, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	status.Status = StateSchemaInitializationReady
	status.Summary = record.Summary
	status.Error = ""
	status.Changes = record.Changes
	status.Warnings = warnings
	status.CompletedAt = now
	status.UpdatedAt = now
	meta.ActorStateSchema = target
	meta.StateSchemaInitialization = &status
	meta.UpdatedAt = now
	newEvents := []any{}
	if len(ops) > 0 || len(actorOps) > 0 {
		deltaID := newID("sd")
		for index := range ops {
			ops[index].SourceKind = stateSchemaMigrationSourceKind
			ops[index].SourceTurnID = sourceTurnID
		}
		for index := range actorOps {
			actorOps[index].SourceKind = stateSchemaMigrationSourceKind
			actorOps[index].SourceTurnID = sourceTurnID
		}
		delta := newStateDeltaEventWithActorOps(deltaID, branch.Head, branchID, now, normalizeStateOps(ops), normalizeActorStateOps(actorOps))
		branch.Head = deltaID
		meta.Branches[branchID] = branch
		newEvents = append(newEvents, delta)
	}
	if err := s.rewriteStoryLocked(storyID, meta, lines, newEvents...); err != nil {
		return status, err
	}
	return status, nil
}

func actorStateSchemaRevision(snapshot *ActorStateSchemaSnapshot) int {
	if snapshot == nil || snapshot.Revision <= 0 {
		return 1
	}
	return snapshot.Revision
}

func stateSchemaAdaptationChanges(adaptation ActorStateSchemaAdaptation) []ActorStateSchemaAdaptationChange {
	changes := make([]ActorStateSchemaAdaptationChange, 0, maxActorStateSchemaAdaptationOps)
	for _, templateOp := range adaptation.TemplateOps {
		if len(templateOp.FieldOps) == 0 {
			targetID := templateOp.Template.ID
			changes = append(changes, ActorStateSchemaAdaptationChange{Kind: "template", Op: templateOp.Op, TemplateID: firstNonEmptyString(templateOp.TemplateID, targetID), TargetID: targetID, Reason: templateOp.Reason})
		}
		for _, fieldOp := range templateOp.FieldOps {
			changes = append(changes, ActorStateSchemaAdaptationChange{Kind: "field", Op: fieldOp.Op, TemplateID: templateOp.TemplateID, FieldID: fieldOp.FieldID, TargetID: actorStateFieldID(fieldOp.Field), Reason: fieldOp.Reason})
		}
	}
	for _, actorOp := range adaptation.InitialActorOps {
		changes = append(changes, ActorStateSchemaAdaptationChange{Kind: "actor", Op: actorOp.Op, ActorID: firstNonEmptyString(actorOp.ActorID, actorOp.Actor.ID), TargetID: actorOp.Actor.TemplateID, Reason: actorOp.Reason})
	}
	for _, actorOp := range adaptation.ActorOps {
		changes = append(changes, ActorStateSchemaAdaptationChange{Kind: "actor", Op: actorOp.Op, ActorID: firstNonEmptyString(actorOp.ActorID, actorOp.Actor.ID), TargetID: actorOp.Actor.TemplateID, Reason: actorOp.Reason})
	}
	if len(changes) > maxActorStateSchemaAdaptationOps {
		changes = changes[:maxActorStateSchemaAdaptationOps]
	}
	return changes
}

func buildStateSchemaMigration(base, target StoryDirectorActorStateSystem, state map[string]any, adaptation ActorStateSchemaAdaptation, sourceTurnID string) ([]StateOp, []ActorStateOp, map[string]map[string]string, []string, error) {
	rawActors, _ := state[actorStateRoot].(map[string]any)
	actorChanges := map[string]ActorStateInitialActorSchemaOp{}
	for _, op := range adaptation.InitialActorOps {
		actorID := firstNonEmptyString(op.ActorID, op.Actor.ID)
		if actorID != "" {
			actorChanges[actorID] = op
		}
	}
	for _, op := range adaptation.ActorOps {
		actorID := firstNonEmptyString(op.ActorID, op.Actor.ID)
		if actorID != "" {
			actorChanges[actorID] = ActorStateInitialActorSchemaOp{Op: op.Op, ActorID: actorID, Actor: op.Actor, Reason: op.Reason}
		}
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
	handledActors := map[string]bool{}
	for actorID, actorChange := range actorChanges {
		if actorChange.Op == "remove" {
			if _, exists := rawActors[actorID]; exists {
				ops = append(ops, StateOp{Op: "unset", Path: actorStateRoot + "." + actorID, Reason: actorChange.Reason})
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
		baseOps, baseActorOps, _, err := buildNewActorStateOps(template, actorID, actor.Name, actor.Role, actor.Description, actor.State, "状态结构适配", sourceTurnID)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		if current, ok := rawActors[actorID].(map[string]any); ok {
			if traits := current["traits"]; traits != nil {
				baseOps = append(baseOps, StateOp{Op: "set", Path: actorStateActorPath(actorID, "traits"), Value: traits})
			}
		}
		ops = append(ops, baseOps...)
		actorOps = append(actorOps, baseActorOps...)
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
		values, _ := actor["state"].(map[string]any)
		if values == nil {
			values = map[string]any{}
		}
		targetIDs := map[string]bool{}
		for _, field := range targetTemplate.Fields {
			targetID := actorStateFieldID(field)
			targetIDs[targetID] = true
			sourceID := targetID
			if source := fieldSources[templateID][targetID]; source != "" {
				sourceID = source
			}
			value, exists := values[sourceID]
			if !exists {
				value = field.Default
			}
			converted, convertedOK := coerceActorStateFieldValue(value, field)
			if !convertedOK {
				converted, _ = coerceActorStateFieldValue(field.Default, field)
				warnings = append(warnings, fmt.Sprintf("Actor %s 的字段 %s 无法转换为 %s，已使用新默认值", actorID, targetID, field.Type))
			}
			actorOps = append(actorOps, ActorStateOp{Op: "set", ActorID: actorID, FieldID: targetID, Value: converted, Reason: "状态结构适配"})
			if sourceID != targetID {
				actorOps = append(actorOps, ActorStateOp{Op: "unset", ActorID: actorID, FieldID: sourceID, Reason: "字段已迁移到 " + targetID})
			}
		}
		for _, field := range baseTemplate.Fields {
			fieldID := actorStateFieldID(field)
			if targetIDs[fieldID] || aliases[templateID][fieldID] != "" {
				continue
			}
			if _, exists := values[fieldID]; exists {
				actorOps = append(actorOps, ActorStateOp{Op: "unset", ActorID: actorID, FieldID: fieldID, Reason: "字段已从故事状态结构移除"})
			}
		}
	}
	if len(warnings) > maxActorStateSchemaAdaptationOps {
		warnings = warnings[:maxActorStateSchemaAdaptationOps]
	}
	return ops, actorOps, aliases, warnings, nil
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

func (s *Store) backupStoryBeforeStateSchemaMigration(storyID string) error {
	data, err := os.ReadFile(s.storyPath(storyID))
	if err != nil {
		return fmt.Errorf("读取状态结构迁移备份失败: %w", err)
	}
	root := strings.TrimSpace(s.novaDir)
	if root == "" {
		root = filepath.Join(s.root, ".denova")
	}
	backupDir := filepath.Join(root, "backups", "state-schema-adaptation", time.Now().UTC().Format("20060102T150405.000000000Z"))
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return fmt.Errorf("创建状态结构迁移备份目录失败: %w", err)
	}
	if err := os.WriteFile(filepath.Join(backupDir, "story-"+storyID+".jsonl"), data, 0o644); err != nil {
		return fmt.Errorf("写入状态结构迁移备份失败: %w", err)
	}
	return nil
}
