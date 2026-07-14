package interactive

import (
	"fmt"
	"reflect"
	"strings"
	"time"
)

const storyContextFoundationSourceKind = "story_context_foundation"

var storyContextFoundationFields = []struct {
	target  string
	aliases []string
}{
	{target: "当前详细地点", aliases: []string{"当前地点/去向", "当前地点", "去向"}},
	{target: "当前事件", aliases: []string{"当前状态"}},
	{target: "当前场景压力", aliases: []string{"当前目标/压力", "压力"}},
}

// EnsureStoryContextFoundation upgrades legacy stories whose `story` Actor
// was bound to a generic character template. It is deterministic, runs before
// a foreground turn snapshots its parent revision, and does not depend on the
// optional LLM schema review succeeding.
func (s *Store) EnsureStoryContextFoundation(storyID, branchID string) (bool, error) {
	if s == nil {
		return false, fmt.Errorf("互动故事存储不可用")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return false, err
	}
	if meta.ActorStateSchema == nil {
		return false, nil
	}
	branchID, branch, err := resolveBranch(meta, branchID)
	if err != nil {
		return false, err
	}
	path, _ := eventPath(branch.Head, eventsByID(lines))
	state := stateFromPath(path)
	applyLegacyActorStateAliases(state, meta.ActorStateSchema)
	adaptation := storyContextFoundationAdaptation(meta.ActorStateSchema.System, state)
	if len(adaptation.TemplateOps) == 0 && len(adaptation.InitialActorOps) == 0 {
		return false, nil
	}
	targetSystem, record, err := ApplyActorStateSchemaAdaptation(meta.ActorStateSchema.System, meta.ActorStateSchema.TRPGSystem, adaptation)
	if err != nil {
		return false, fmt.Errorf("构建 story_context 基础迁移失败: %w", err)
	}
	sourceTurnID := ""
	if turn := latestTurnForBranchHead(lines, branch.Head); turn != nil {
		sourceTurnID = turn.ID
	}
	ops, actorOps, aliases, warnings, err := buildStateSchemaMigration(meta.ActorStateSchema.System, targetSystem, state, adaptation, sourceTurnID)
	if err != nil {
		return false, fmt.Errorf("迁移 story_context 现值失败: %w", err)
	}
	schemaChanged := !reflect.DeepEqual(normalizeActorStateSystem(meta.ActorStateSchema.System), normalizeActorStateSystem(targetSystem))
	if !schemaChanged && len(ops) == 0 && len(actorOps) == 0 {
		return false, nil
	}
	if err := s.backupStoryBeforeStateSchemaMigration(storyID); err != nil {
		return false, err
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	record.SourceTurnID = sourceTurnID
	record.Summary = "后端补齐 story_context 基础模板并迁移旧字段"
	record.Changes = stateSchemaAdaptationChanges(adaptation)
	record.Warnings = warnings
	target := FreezeActorStateSchemaWithRules(targetSystem, meta.ActorStateSchema.TRPGSystem, false)
	target.Revision = actorStateSchemaRevision(meta.ActorStateSchema)
	if schemaChanged {
		target.Revision++
	}
	target.Adaptation = &record
	target.LegacyFieldPaths = mergeLegacyFieldAliases(meta.ActorStateSchema.LegacyFieldPaths, aliases)
	target.FieldMigrations = mergeActorStateFieldMigrations(meta.ActorStateSchema.FieldMigrations, stateSchemaFieldMigrations(adaptation))
	meta.ActorStateSchema = target
	meta.UpdatedAt = now
	newEvents := []any{}
	if len(ops) > 0 || len(actorOps) > 0 {
		for index := range ops {
			ops[index].SourceKind = storyContextFoundationSourceKind
			ops[index].SourceTurnID = sourceTurnID
		}
		for index := range actorOps {
			actorOps[index].SourceKind = storyContextFoundationSourceKind
			actorOps[index].SourceTurnID = sourceTurnID
		}
		deltaID := newID("sd")
		delta := newStateDeltaEventWithActorOps(deltaID, branch.Head, branchID, now, normalizeStateOps(ops), normalizeActorStateOps(actorOps))
		branch.Head = deltaID
		meta.Branches[branchID] = branch
		newEvents = append(newEvents, delta)
	}
	if err := s.rewriteStoryLocked(storyID, meta, lines, newEvents...); err != nil {
		return false, err
	}
	return true, nil
}

func storyContextFoundationAdaptation(base StoryDirectorActorStateSystem, state map[string]any) ActorStateSchemaAdaptation {
	base = normalizeActorStateSystem(base)
	adaptation := ActorStateSchemaAdaptation{Summary: "补齐 story_context 基础状态"}
	template := actorStateTemplateByID(base, ActorStateStoryContextTemplateID)
	targetTemplate := template
	if template.ID == "" {
		targetTemplate = defaultStoryContextTemplate()
		adaptation.TemplateOps = append(adaptation.TemplateOps, ActorStateTemplateSchemaOp{
			Op: "add", Template: targetTemplate, Reason: "互动回合需要稳定的故事时间、地点、事件与压力状态对象",
		})
	} else {
		fieldOps := storyContextFoundationFieldOps(template)
		if len(fieldOps) > 0 {
			adaptation.TemplateOps = append(adaptation.TemplateOps, ActorStateTemplateSchemaOp{
				Op: "fields", TemplateID: ActorStateStoryContextTemplateID, FieldOps: fieldOps,
			})
			updated, _, err := ApplyActorStateSchemaAdaptation(base, StoryDirectorTRPGSystem{}, adaptation)
			if err == nil {
				targetTemplate = actorStateTemplateByID(updated, ActorStateStoryContextTemplateID)
			}
		}
	}

	initialIndex := actorStateInitialActorIndex(base.InitialActors, DefaultStoryContextActorID)
	var initial ActorStateInitialActor
	if initialIndex >= 0 {
		initial = base.InitialActors[initialIndex]
	}
	rawActors, _ := state[actorStateRoot].(map[string]any)
	rawStory, _ := rawActors[DefaultStoryContextActorID].(map[string]any)
	runtimeTemplateID := normalizeActorStateID(storyContextFoundationString(rawStory, "template_id"))
	needsActorMigration := initialIndex < 0 || normalizeActorStateID(initial.TemplateID) != ActorStateStoryContextTemplateID || rawStory == nil || runtimeTemplateID != ActorStateStoryContextTemplateID
	if !needsActorMigration {
		return adaptation
	}
	values := storyContextFoundationValues(targetTemplate, initial.State, rawStory)
	actor := ActorStateInitialActor{
		ID:          DefaultStoryContextActorID,
		Name:        firstNonEmptyString(storyContextFoundationString(rawStory, "name"), initial.Name, "故事上下文"),
		TemplateID:  ActorStateStoryContextTemplateID,
		Role:        firstNonEmptyString(storyContextFoundationString(rawStory, "role"), initial.Role, "story_context"),
		Description: firstNonEmptyString(storyContextFoundationString(rawStory, "description"), initial.Description, "当前分支的剧内时间、地点、场景和全局压力状态。"),
		State:       values,
	}
	op := "replace"
	if initialIndex < 0 {
		op = "add"
	}
	adaptation.InitialActorOps = append(adaptation.InitialActorOps, ActorStateInitialActorSchemaOp{
		Op: op, ActorID: DefaultStoryContextActorID, Actor: actor, Reason: "将旧 story Actor 绑定到专用 story_context 模板",
	})
	return adaptation
}

func storyContextFoundationString(values map[string]any, key string) string {
	if values == nil || values[key] == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(values[key]))
}

func storyContextFoundationFieldOps(template ActorStateTemplate) []ActorStateFieldSchemaOp {
	defaults := defaultStoryContextTemplate()
	ops := make([]ActorStateFieldSchemaOp, 0, len(storyContextFoundationFields))
	for _, spec := range storyContextFoundationFields {
		if _, exists := actorStateFieldByID(template, spec.target); exists {
			continue
		}
		field, _ := actorStateFieldByID(defaults, spec.target)
		legacy := ""
		for _, alias := range spec.aliases {
			if existing, exists := actorStateFieldByID(template, alias); exists {
				legacy = actorStateFieldID(existing)
				break
			}
		}
		if legacy != "" {
			ops = append(ops, ActorStateFieldSchemaOp{Op: "replace", FieldID: legacy, Field: field, Reason: "迁移旧 story_context 字段名称"})
			continue
		}
		ops = append(ops, ActorStateFieldSchemaOp{Op: "add", Field: field, Reason: "补齐互动回合基础 story_context 字段"})
	}
	return ops
}

func storyContextFoundationValues(target ActorStateTemplate, initialState map[string]any, rawStory map[string]any) map[string]any {
	sources := make([]map[string]any, 0, 2)
	if rawValues, ok := rawStory["state"].(map[string]any); ok {
		sources = append(sources, rawValues)
	}
	if len(initialState) > 0 {
		sources = append(sources, initialState)
	}
	values := map[string]any{}
	for _, field := range target.Fields {
		fieldID := actorStateFieldID(field)
		if value, ok := firstStoryContextFoundationValue(sources, append([]string{fieldID}, storyContextAliasesForTarget(fieldID)...)...); ok && value != nil {
			values[fieldID] = value
		}
	}
	if len(values) == 0 {
		return nil
	}
	return values
}

func storyContextAliasesForTarget(target string) []string {
	for _, spec := range storyContextFoundationFields {
		if actorStateFieldNameKey(spec.target) == actorStateFieldNameKey(target) {
			return spec.aliases
		}
	}
	return nil
}

func firstStoryContextFoundationValue(sources []map[string]any, refs ...string) (any, bool) {
	for _, source := range sources {
		for _, ref := range refs {
			for key, value := range source {
				if actorStateFieldNameKey(key) == actorStateFieldNameKey(ref) {
					return value, true
				}
			}
		}
	}
	return nil, false
}
