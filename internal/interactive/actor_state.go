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
}

type ActorStateTemplate struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Fields      []ActorStateField `json:"fields,omitempty"`
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
	ActorID      string         `json:"actor_id"`
	ActorName    string         `json:"actor_name,omitempty"`
	TemplateID   string         `json:"template_id,omitempty"`
	Role         string         `json:"role,omitempty"`
	Description  string         `json:"description,omitempty"`
	State        map[string]any `json:"state,omitempty"`
	Reason       string         `json:"reason,omitempty"`
	SourceTurnID string         `json:"source_turn_id,omitempty"`
}

type ActorStatePatchResult struct {
	AppliedActors []string  `json:"applied_actors"`
	Ops           []StateOp `json:"ops"`
}

func ValidateActorStatePatches(system StoryDirectorActorStateSystem, patches []ActorStatePatch, sourceTurnID string) (ActorStatePatchResult, error) {
	if len(patches) == 0 {
		return ActorStatePatchResult{}, fmt.Errorf("Actor 状态更新不能为空")
	}
	if len(patches) > maxTurnBriefListItems {
		patches = patches[:maxTurnBriefListItems]
	}
	result := ActorStatePatchResult{AppliedActors: []string{}, Ops: []StateOp{}}
	seenActors := map[string]bool{}
	for _, patch := range patches {
		patch.SourceTurnID = firstNonEmptyString(patch.SourceTurnID, sourceTurnID)
		normalized, ops, err := validateActorStatePatch(system, patch)
		if err != nil {
			return ActorStatePatchResult{}, err
		}
		if !seenActors[normalized.ActorID] {
			seenActors[normalized.ActorID] = true
			result.AppliedActors = append(result.AppliedActors, normalized.ActorID)
		}
		result.Ops = append(result.Ops, ops...)
	}
	result.Ops = normalizeStateOps(result.Ops)
	return result, nil
}

func normalizeActorStateSystem(system StoryDirectorActorStateSystem) StoryDirectorActorStateSystem {
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
	return len(system.Templates) == 0 && len(system.InitialActors) == 0
}

func defaultActorStateSystem() StoryDirectorActorStateSystem {
	hpMin, hpMax := 0.0, 10.0
	staminaMin, staminaMax := 0.0, 5.0
	affectionMin, affectionMax := -100.0, 100.0
	return normalizeActorStateSystem(StoryDirectorActorStateSystem{
		Templates: []ActorStateTemplate{{
			ID:          "protagonist",
			Name:        "主角",
			Description: "主角可计算状态模板，用于规则检定、资源消耗和长期承接。",
			Fields: []ActorStateField{
				{
					ID:          "hp",
					Path:        "resources.hp",
					Name:        "生命",
					Type:        "number",
					Default:     10.0,
					Min:         &hpMin,
					Max:         &hpMax,
					Visibility:  "visible",
					Description: "主角当前生命或伤势承受能力。",
					Order:       10,
				},
				{
					ID:          "stamina",
					Path:        "resources.stamina",
					Name:        "体力",
					Type:        "number",
					Default:     5.0,
					Min:         &staminaMin,
					Max:         &staminaMax,
					Visibility:  "visible",
					Description: "奔跑、战斗、潜入等高消耗行动的资源。",
					Order:       20,
				},
				{
					ID:          "affection",
					Path:        "relations.affection",
					Name:        "好感",
					Type:        "number",
					Default:     0.0,
					Min:         &affectionMin,
					Max:         &affectionMax,
					Visibility:  "spoiler",
					Description: "重要角色或势力对主角的亲近度，可按对象拆分。",
					Order:       30,
				},
			},
		}},
		InitialActors: []ActorStateInitialActor{{
			ID:         DefaultActorID,
			Name:       "主角",
			TemplateID: "protagonist",
			Role:       "protagonist",
		}},
	})
}

func actorStateInitialOps(system StoryDirectorActorStateSystem) []StateOp {
	system = normalizeActorStateSystem(system)
	if actorStateEmpty(system) {
		return nil
	}
	templates := map[string]ActorStateTemplate{}
	for _, template := range system.Templates {
		templates[template.ID] = template
	}
	ops := []StateOp{}
	for _, actor := range system.InitialActors {
		template, ok := templates[actor.TemplateID]
		if !ok {
			continue
		}
		ops = append(ops,
			StateOp{Op: "set", Path: actorStateActorPath(actor.ID, "id"), Value: actor.ID},
			StateOp{Op: "set", Path: actorStateActorPath(actor.ID, "name"), Value: actor.Name},
			StateOp{Op: "set", Path: actorStateActorPath(actor.ID, "template_id"), Value: actor.TemplateID},
			StateOp{Op: "set", Path: actorStateActorPath(actor.ID, "role"), Value: actor.Role},
		)
		if strings.TrimSpace(actor.Description) != "" {
			ops = append(ops, StateOp{Op: "set", Path: actorStateActorPath(actor.ID, "description"), Value: actor.Description})
		}
		for _, field := range template.Fields {
			if field.Default != nil {
				ops = append(ops, StateOp{Op: "set", Path: actorStateFieldPath(actor.ID, field.Path), Value: field.Default})
			}
			if field.Type == "number" && field.Max != nil {
				ops = append(ops, StateOp{Op: "set", Path: actorStateFieldPath(actor.ID, field.Path+"_max"), Value: *field.Max})
			}
		}
		keys := make([]string, 0, len(actor.State))
		for key := range actor.State {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			ops = append(ops, StateOp{Op: "set", Path: actorStateFieldPath(actor.ID, key), Value: actor.State[key]})
		}
	}
	return normalizeStateOps(ops)
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

func validateActorStatePatch(system StoryDirectorActorStateSystem, patch ActorStatePatch) (ActorStatePatch, []StateOp, error) {
	system = normalizeActorStateSystem(system)
	patch.ActorID = normalizeActorStateID(patch.ActorID)
	if patch.ActorID == "" {
		return patch, nil, fmt.Errorf("Actor 状态更新缺少 actor_id")
	}
	patch.TemplateID = normalizeActorStateID(patch.TemplateID)
	if patch.TemplateID == "" {
		patch.TemplateID = "protagonist"
	}
	template := actorStateTemplateByID(system, patch.TemplateID)
	if template.ID == "" {
		return patch, nil, fmt.Errorf("Actor 状态模板不存在: %s", patch.TemplateID)
	}
	fieldByPath := map[string]ActorStateField{}
	for _, field := range template.Fields {
		fieldByPath[field.Path] = field
	}
	if len(patch.State) == 0 {
		return patch, nil, fmt.Errorf("Actor 状态更新缺少 state")
	}
	reason := trimBytes(patch.Reason, maxTurnBriefTextBytes)
	sourceTurnID := trimBytes(patch.SourceTurnID, 128)
	ops := []StateOp{
		{Op: "set", Path: actorStateActorPath(patch.ActorID, "id"), Value: patch.ActorID, Reason: reason, SourceTurnID: sourceTurnID},
		{Op: "set", Path: actorStateActorPath(patch.ActorID, "template_id"), Value: patch.TemplateID, Reason: reason, SourceTurnID: sourceTurnID},
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
	keys := make([]string, 0, len(patch.State))
	for key := range patch.State {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		field, ok := fieldByPath[strings.TrimSpace(key)]
		if !ok {
			return patch, nil, fmt.Errorf("Actor 状态字段不在模板中: actor=%s template=%s field=%s", patch.ActorID, patch.TemplateID, key)
		}
		value, err := normalizeActorStateValue(field, patch.State[key])
		if err != nil {
			return patch, nil, err
		}
		ops = append(ops, StateOp{Op: "set", Path: actorStateFieldPath(patch.ActorID, field.Path), Value: value, Reason: reason, SourceTurnID: sourceTurnID})
	}
	return patch, normalizeStateOps(ops), nil
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
