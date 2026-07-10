package interactive

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const (
	DefaultActorStateModuleID = "default"
	DefaultActorID            = "protagonist"

	actorStateRoot      = "actors"
	maxActorStateFields = 64
)

type StoryDirectorActorStateSystem struct {
	Templates     []ActorStateTemplate     `json:"templates,omitempty"`
	InitialActors []ActorStateInitialActor `json:"initial_actors,omitempty"`
	TraitPools    []ActorTraitPool         `json:"trait_pools,omitempty"`
}

type ActorStateTemplate struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Fields      []ActorStateField `json:"fields,omitempty"`
	TraitRules  []ActorTraitRule  `json:"trait_rules,omitempty"`
}

// ActorTraitRule declares which reusable trait pool is available to actors
// created from a state template and how many traits are assigned from it.
type ActorTraitRule struct {
	PoolID    string `json:"pool_id"`
	DrawCount int    `json:"draw_count"`
}

// ActorTraitPool is a reusable library of traits. Draw behavior belongs to
// ActorTraitRule so one pool can be composed differently by each template.
type ActorTraitPool struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Traits      []ActorTraitDefinition `json:"traits,omitempty"`
}

type ActorTraitDefinition struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Summary    string  `json:"summary,omitempty"`
	Weight     float64 `json:"weight,omitempty"`
	Visibility string  `json:"visibility,omitempty"`
}

// ActorTraitInstance is a snapshot of a definition at assignment time. Stories
// therefore remain stable when the reusable trait library is edited later.
type ActorTraitInstance struct {
	PoolID       string `json:"pool_id"`
	PoolName     string `json:"pool_name,omitempty"`
	TraitID      string `json:"trait_id"`
	Name         string `json:"name"`
	Summary      string `json:"summary,omitempty"`
	Visibility   string `json:"visibility,omitempty"`
	SourceKind   string `json:"source_kind,omitempty"`
	SourceID     string `json:"source_id,omitempty"`
	SourceTurnID string `json:"source_turn_id,omitempty"`
}

type ActorTraitChange struct {
	Op       string   `json:"op"`
	PoolID   string   `json:"pool_id"`
	TraitIDs []string `json:"trait_ids,omitempty"`
	Seed     int64    `json:"seed,omitempty"`
}

type ActorStateField struct {
	ID                string   `json:"id,omitempty"`
	Path              string   `json:"path"`
	Name              string   `json:"name"`
	Type              string   `json:"type"`
	Default           any      `json:"default,omitempty"`
	Min               *float64 `json:"min,omitempty"`
	Max               *float64 `json:"max,omitempty"`
	Options           []string `json:"options,omitempty"`
	Visibility        string   `json:"visibility,omitempty"`
	Description       string   `json:"description,omitempty"`
	UpdateInstruction string   `json:"update_instruction,omitempty"`
	Order             int      `json:"order,omitempty"`
}

type ActorStateInitialActor struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	TemplateID  string         `json:"template_id"`
	Role        string         `json:"role,omitempty"`
	Description string         `json:"description,omitempty"`
	State       map[string]any `json:"state,omitempty"`
}

type ActorStatePatch struct {
	ActorID      string             `json:"actor_id"`
	ActorName    string             `json:"actor_name,omitempty"`
	TemplateID   string             `json:"template_id,omitempty"`
	Role         string             `json:"role,omitempty"`
	Description  string             `json:"description,omitempty"`
	State        map[string]any     `json:"state,omitempty"`
	TraitChanges []ActorTraitChange `json:"trait_changes,omitempty"`
	Reason       string             `json:"reason,omitempty"`
	SourceTurnID string             `json:"source_turn_id,omitempty"`
}

type ActorStatePatchResult struct {
	AppliedActors  []string                        `json:"applied_actors"`
	CreatedActors  []string                        `json:"created_actors,omitempty"`
	AssignedTraits map[string][]ActorTraitInstance `json:"assigned_traits,omitempty"`
	Ops            []StateOp                       `json:"ops"`
}

func ValidateActorStatePatches(system StoryDirectorActorStateSystem, patches []ActorStatePatch, sourceTurnID string) (ActorStatePatchResult, error) {
	return ValidateActorStatePatchesAgainstState(system, nil, patches, sourceTurnID)
}

// ValidateActorStatePatchesAgainstState validates patches against the current
// replayed story state so actor creation, immutable template identity, and
// trait lifecycle changes are handled consistently.
func ValidateActorStatePatchesAgainstState(system StoryDirectorActorStateSystem, currentState map[string]any, patches []ActorStatePatch, sourceTurnID string) (ActorStatePatchResult, error) {
	if len(patches) == 0 {
		return ActorStatePatchResult{}, fmt.Errorf("Actor 状态更新不能为空")
	}
	if len(patches) > maxTurnBriefListItems {
		patches = patches[:maxTurnBriefListItems]
	}
	result := ActorStatePatchResult{AppliedActors: []string{}, CreatedActors: []string{}, AssignedTraits: map[string][]ActorTraitInstance{}, Ops: []StateOp{}}
	workingState := cloneActorStateRoot(currentState)
	seenActors := map[string]bool{}
	for _, patch := range patches {
		patch.SourceTurnID = firstNonEmptyString(patch.SourceTurnID, sourceTurnID)
		normalized, ops, created, traits, err := validateActorStatePatch(system, workingState, patch)
		if err != nil {
			return ActorStatePatchResult{}, err
		}
		if !seenActors[normalized.ActorID] {
			seenActors[normalized.ActorID] = true
			result.AppliedActors = append(result.AppliedActors, normalized.ActorID)
		}
		if created {
			result.CreatedActors = append(result.CreatedActors, normalized.ActorID)
		}
		if traits != nil {
			result.AssignedTraits[normalized.ActorID] = traits
		}
		result.Ops = append(result.Ops, ops...)
		for _, op := range ops {
			applyStateOp(workingState, op)
		}
	}
	result.Ops = normalizeStateOps(result.Ops)
	if len(result.AssignedTraits) == 0 {
		result.AssignedTraits = nil
	}
	return result, nil
}

func normalizeActorStateSystem(system StoryDirectorActorStateSystem) StoryDirectorActorStateSystem {
	system.TraitPools = normalizeActorTraitPools(system.TraitPools)
	system.Templates = normalizeActorStateTemplates(system.Templates)
	system.InitialActors = normalizeActorStateInitialActors(system.InitialActors, system.Templates)
	return system
}

func normalizeActorStateTemplates(templates []ActorStateTemplate) []ActorStateTemplate {
	if templates == nil {
		return []ActorStateTemplate{}
	}
	if len(templates) > maxTurnBriefListItems {
		templates = templates[:maxTurnBriefListItems]
	}
	out := make([]ActorStateTemplate, 0, len(templates))
	seen := map[string]bool{}
	for _, template := range templates {
		template.ID = normalizeActorStateID(template.ID)
		if template.ID == "" || seen[template.ID] {
			continue
		}
		seen[template.ID] = true
		template.Name = trimBytes(firstNonEmptyString(template.Name, template.ID), 128)
		template.Description = trimBytes(template.Description, maxTurnBriefTextBytes)
		template.Fields = normalizeActorStateFields(template.Fields)
		template.TraitRules = normalizeActorTraitRules(template.TraitRules)
		out = append(out, template)
	}
	return out
}

func normalizeActorStateFields(fields []ActorStateField) []ActorStateField {
	if fields == nil {
		return []ActorStateField{}
	}
	if len(fields) > maxActorStateFields {
		fields = fields[:maxActorStateFields]
	}
	out := make([]ActorStateField, 0, len(fields))
	seen := map[string]bool{}
	for i, field := range fields {
		field.Path = strings.TrimSpace(field.Path)
		if field.Path == "" || !validStatePathSyntax(field.Path) {
			continue
		}
		key := field.Path
		if seen[key] {
			continue
		}
		seen[key] = true
		field.ID = normalizeActorStateID(firstNonEmptyString(field.ID, strings.ReplaceAll(field.Path, ".", "_")))
		field.Name = trimBytes(firstNonEmptyString(field.Name, field.ID, field.Path), 128)
		field.Type = normalizeActorStateFieldType(field.Type)
		field.Visibility = normalizeStoryDirectorVisibility(field.Visibility)
		field.Description = trimBytes(field.Description, maxTurnBriefTextBytes)
		field.UpdateInstruction = trimBytes(field.UpdateInstruction, maxTurnBriefTextBytes)
		field.Options = normalizeStringListLimit(field.Options, maxTurnBriefListItems)
		if field.Order == 0 {
			field.Order = (i + 1) * 10
		}
		out = append(out, field)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Order != out[j].Order {
			return out[i].Order < out[j].Order
		}
		return out[i].Path < out[j].Path
	})
	return out
}

func normalizeActorStateInitialActors(actors []ActorStateInitialActor, templates []ActorStateTemplate) []ActorStateInitialActor {
	if actors == nil {
		return []ActorStateInitialActor{}
	}
	templateIDs := map[string]bool{}
	for _, template := range templates {
		templateIDs[template.ID] = true
	}
	if len(actors) > maxTurnBriefListItems {
		actors = actors[:maxTurnBriefListItems]
	}
	out := make([]ActorStateInitialActor, 0, len(actors))
	seen := map[string]bool{}
	for _, actor := range actors {
		actor.ID = normalizeActorStateID(actor.ID)
		if actor.ID == "" || seen[actor.ID] {
			continue
		}
		actor.TemplateID = normalizeActorStateID(actor.TemplateID)
		if actor.TemplateID == "" || !templateIDs[actor.TemplateID] {
			continue
		}
		seen[actor.ID] = true
		actor.Name = trimBytes(firstNonEmptyString(actor.Name, actor.ID), 128)
		actor.Role = trimBytes(firstNonEmptyString(actor.Role, actor.TemplateID), 128)
		actor.Description = trimBytes(actor.Description, maxTurnBriefTextBytes)
		actor.State = normalizeActorStateMap(actor.State)
		out = append(out, actor)
	}
	return out
}

func normalizeActorStateFieldType(value string) string {
	switch strings.TrimSpace(value) {
	case "number", "string", "bool", "enum", "object", "list":
		return strings.TrimSpace(value)
	default:
		return "string"
	}
}

func normalizeActorStateMap(values map[string]any) map[string]any {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]any, len(values))
	for key, value := range values {
		key = strings.TrimSpace(key)
		if key == "" || !validStatePathSyntax(key) {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func actorStateEmpty(system StoryDirectorActorStateSystem) bool {
	return len(system.Templates) == 0 && len(system.InitialActors) == 0 && len(system.TraitPools) == 0
}

func defaultActorStateSystem() StoryDirectorActorStateSystem {
	return normalizeActorStateSystem(StoryDirectorActorStateSystem{
		Templates: []ActorStateTemplate{
			actorStateTemplate(DefaultActorID, "默认主角状态表", "记录主角当前可行动、可检定、可结算的通用互动状态；用户可按作品需要新增世界、故事倒计时、特定角色、势力、基地、副本等自定义状态表。", commonProtagonistStateFields()),
			defaultStoryContextTemplate(),
			actorStateTemplate(ActorStateImportantCharacterTemplateID, "默认重要角色状态表", "记录反复登场且会影响互动承接的重要角色状态；特定角色线可以另建独立状态表。", commonImportantCharacterStateFields()),
			actorStateTemplate(ActorStateOpponentTemplateID, "默认敌人/怪物状态表", "记录敌人、怪物、反派、Boss 或异常实体的当前对抗状态；危机、势力或副本也可另建状态表。", commonOpponentStateFields()),
		},
		InitialActors: defaultActorStateInitialActors(),
	})
}

func actorStateInitialOps(system StoryDirectorActorStateSystem) []StateOp {
	ops, err := BuildActorStateInitialOps(system, nil)
	if err != nil {
		return nil
	}
	return ops
}

func actorStateActorPath(actorID, field string) string {
	return actorStateRoot + "." + normalizeActorStateID(actorID) + "." + strings.TrimSpace(field)
}

func actorStateFieldPath(actorID, fieldPath string) string {
	return actorStateActorPath(actorID, "state."+strings.TrimSpace(fieldPath))
}

func normalizeActorStateID(id string) string {
	id = strings.TrimSpace(id)
	var sb strings.Builder
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

func canonicalStatePath(path string) string {
	path = strings.TrimSpace(path)
	if next, ok := legacyActorStatePath(path); ok {
		return next
	}
	return path
}

func legacyActorStatePath(path string) (string, bool) {
	path = strings.TrimSpace(path)
	if path == "" || strings.HasPrefix(path, actorStateRoot+".") {
		return "", false
	}
	root := path
	if idx := strings.Index(path, "."); idx >= 0 {
		root = path[:idx]
	}
	switch root {
	case "resources", "relations", "attributes", "conditions":
		return actorStateFieldPath(DefaultActorID, path), true
	default:
		return "", false
	}
}

func actorStateTemplateByID(system StoryDirectorActorStateSystem, id string) ActorStateTemplate {
	id = normalizeActorStateID(id)
	for _, template := range normalizeActorStateSystem(system).Templates {
		if template.ID == id {
			return template
		}
	}
	return ActorStateTemplate{}
}

func validateActorStatePatch(system StoryDirectorActorStateSystem, currentState map[string]any, patch ActorStatePatch) (ActorStatePatch, []StateOp, bool, []ActorTraitInstance, error) {
	system = normalizeActorStateSystem(system)
	patch.ActorID = normalizeActorStateID(patch.ActorID)
	if patch.ActorID == "" {
		return patch, nil, false, nil, fmt.Errorf("Actor 状态更新缺少 actor_id")
	}
	existingActor := getPath(currentState, actorStateRoot+"."+patch.ActorID)
	created := existingActor == nil
	if !created {
		if _, ok := existingActor.(map[string]any); !ok {
			return patch, nil, false, nil, fmt.Errorf("Actor 状态对象结构无效: %s", patch.ActorID)
		}
	}
	existingTemplateID := ""
	if rawTemplateID, ok := getPath(currentState, actorStateActorPath(patch.ActorID, "template_id")).(string); ok {
		existingTemplateID = normalizeActorStateID(rawTemplateID)
	}
	patch.TemplateID = normalizeActorStateID(patch.TemplateID)
	if created && patch.TemplateID == "" {
		return patch, nil, false, nil, fmt.Errorf("创建 Actor 状态对象必须提供 template_id: %s", patch.ActorID)
	}
	bindLegacyTemplate := !created && existingTemplateID == ""
	if !created {
		if bindLegacyTemplate && patch.TemplateID == "" {
			return patch, nil, false, nil, fmt.Errorf("旧 Actor 状态对象缺少 template_id，更新时必须显式绑定: %s", patch.ActorID)
		}
		if !bindLegacyTemplate && patch.TemplateID == "" {
			patch.TemplateID = existingTemplateID
		} else if !bindLegacyTemplate && patch.TemplateID != existingTemplateID {
			return patch, nil, false, nil, fmt.Errorf("已有 Actor 的状态模板不可隐式更换: actor=%s current=%s requested=%s", patch.ActorID, existingTemplateID, patch.TemplateID)
		}
	}
	template := actorStateTemplateByID(system, patch.TemplateID)
	if template.ID == "" {
		return patch, nil, false, nil, fmt.Errorf("Actor 状态模板不存在: %s", patch.TemplateID)
	}
	fieldByPath := map[string]ActorStateField{}
	for _, field := range template.Fields {
		fieldByPath[field.Path] = field
	}
	if len(patch.State) == 0 && len(patch.TraitChanges) == 0 && !created && !bindLegacyTemplate {
		return patch, nil, false, nil, fmt.Errorf("Actor 状态更新缺少 state 或 trait_changes")
	}
	reason := trimBytes(patch.Reason, maxTurnBriefTextBytes)
	sourceTurnID := trimBytes(patch.SourceTurnID, 128)
	ops := []StateOp{}
	if created {
		baseOps, err := buildNewActorStateOps(template, patch.ActorID, patch.ActorName, patch.Role, patch.Description, patch.State, reason, sourceTurnID)
		if err != nil {
			return patch, nil, false, nil, err
		}
		ops = append(ops, baseOps...)
	} else {
		if bindLegacyTemplate {
			ops = append(ops, StateOp{Op: "set", Path: actorStateActorPath(patch.ActorID, "template_id"), Value: patch.TemplateID, Reason: reason, SourceTurnID: sourceTurnID})
		}
		if strings.TrimSpace(patch.ActorName) != "" {
			ops = append(ops, StateOp{Op: "set", Path: actorStateActorPath(patch.ActorID, "name"), Value: trimBytes(patch.ActorName, 128), Reason: reason, SourceTurnID: sourceTurnID})
		}
		if strings.TrimSpace(patch.Role) != "" {
			ops = append(ops, StateOp{Op: "set", Path: actorStateActorPath(patch.ActorID, "role"), Value: trimBytes(patch.Role, 128), Reason: reason, SourceTurnID: sourceTurnID})
		}
		if strings.TrimSpace(patch.Description) != "" {
			ops = append(ops, StateOp{Op: "set", Path: actorStateActorPath(patch.ActorID, "description"), Value: trimBytes(patch.Description, maxTurnBriefTextBytes), Reason: reason, SourceTurnID: sourceTurnID})
		}
	}
	if !created {
		keys := make([]string, 0, len(patch.State))
		for key := range patch.State {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			field, ok := fieldByPath[strings.TrimSpace(key)]
			if !ok {
				return patch, nil, false, nil, fmt.Errorf("Actor 状态字段不在模板中: actor=%s template=%s field=%s", patch.ActorID, patch.TemplateID, key)
			}
			value, err := normalizeActorStateValue(field, patch.State[key])
			if err != nil {
				return patch, nil, false, nil, err
			}
			ops = append(ops, StateOp{Op: "set", Path: actorStateFieldPath(patch.ActorID, field.Path), Value: value, Reason: reason, SourceTurnID: sourceTurnID})
		}
	}
	traits := actorTraitInstancesFromState(currentState, patch.ActorID)
	if created {
		result, err := rollActorTraits(system, ActorTraitRollRequest{ActorID: patch.ActorID, TemplateID: patch.TemplateID}, "actor_create", sourceTurnID)
		if err != nil {
			return patch, nil, false, nil, err
		}
		traits = result.Traits
	}
	changedTraits := created && len(traits) > 0
	if len(patch.TraitChanges) > 0 {
		nextTraits, changed, err := applyActorTraitChanges(system, template, patch.ActorID, traits, patch.TraitChanges, sourceTurnID)
		if err != nil {
			return patch, nil, false, nil, err
		}
		traits = nextTraits
		changedTraits = changedTraits || changed
	}
	if changedTraits {
		ops = append(ops, StateOp{
			Op:           "set",
			Path:         actorStateActorPath(patch.ActorID, "traits"),
			Value:        traits,
			Reason:       reason,
			SourceTurnID: sourceTurnID,
			SourceKind:   StateOpSourceActorTrait,
			SourceID:     firstNonEmptyString(actorTraitSourceID(traits), fmt.Sprintf("actor-traits:%s", patch.ActorID)),
		})
	}
	var reportedTraits []ActorTraitInstance
	if len(patch.TraitChanges) > 0 {
		reportedTraits = make([]ActorTraitInstance, len(traits))
		copy(reportedTraits, traits)
	} else if created && len(traits) > 0 {
		reportedTraits = traits
	}
	return patch, normalizeStateOps(ops), created, reportedTraits, nil
}

func actorTraitSourceID(traits []ActorTraitInstance) string {
	for index := len(traits) - 1; index >= 0; index-- {
		if strings.TrimSpace(traits[index].SourceID) != "" {
			return strings.TrimSpace(traits[index].SourceID)
		}
	}
	return ""
}

func normalizeActorStateValue(field ActorStateField, value any) (any, error) {
	switch field.Type {
	case "number":
		number, ok := actorStateNumber(value)
		if !ok {
			return nil, fmt.Errorf("Actor 状态字段 %s 必须是 number", field.Path)
		}
		if field.Min != nil && number < *field.Min {
			number = *field.Min
		}
		if field.Max != nil && number > *field.Max {
			number = *field.Max
		}
		return number, nil
	case "bool":
		if typed, ok := value.(bool); ok {
			return typed, nil
		}
		return nil, fmt.Errorf("Actor 状态字段 %s 必须是 bool", field.Path)
	case "enum":
		text := strings.TrimSpace(fmt.Sprint(value))
		for _, option := range field.Options {
			if text == option {
				return text, nil
			}
		}
		return nil, fmt.Errorf("Actor 状态字段 %s 不在枚举选项中: %s", field.Path, text)
	case "object":
		if typed, ok := value.(map[string]any); ok {
			return typed, nil
		}
		return nil, fmt.Errorf("Actor 状态字段 %s 必须是 object", field.Path)
	case "list":
		if typed, ok := value.([]any); ok {
			return typed, nil
		}
		return nil, fmt.Errorf("Actor 状态字段 %s 必须是 list", field.Path)
	default:
		return strings.TrimSpace(fmt.Sprint(value)), nil
	}
}

func actorStateNumber(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case json.Number:
		parsed, err := typed.Float64()
		return parsed, err == nil
	default:
		return 0, false
	}
}
