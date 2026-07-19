package interactive

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
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
	ID          string            `json:"id" jsonschema_description:"稳定的 ASCII Template ID。"`
	Name        string            `json:"name" jsonschema_description:"用户可见模板名称。"`
	Description string            `json:"description,omitempty" jsonschema_description:"模板职责的简短说明。"`
	Fields      []ActorStateField `json:"fields,omitempty" jsonschema:"maxItems=64" jsonschema_description:"模板字段；只添加确有长期追踪价值的字段。"`
	TraitRules  []ActorTraitRule  `json:"trait_rules,omitempty"`
	// DisplayGroups is retained only for decoding older Beta presets. The stage
	// ignores it and stores user layout by story + template outside the schema.
	DisplayGroups []string `json:"display_groups,omitempty"`
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
	// ID, Path, and LegacyPath are runtime-only aliases. Presets use Path to map
	// released pre-schema story state into the frozen field identity; reusable
	// modules persist Name only.
	ID                string   `json:"-"`
	Path              string   `json:"-"`
	LegacyPath        string   `json:"-"`
	Name              string   `json:"name" jsonschema_description:"稳定 Field ID 与用户可见名称；同一模板内唯一。"`
	Type              string   `json:"type" jsonschema:"enum=number,enum=string,enum=bool,enum=enum,enum=object,enum=list" jsonschema_description:"字段值类型，只能使用列出的六种类型。"`
	Default           any      `json:"default,omitempty"`
	Min               *float64 `json:"min,omitempty"`
	Max               *float64 `json:"max,omitempty"`
	Options           []string `json:"options,omitempty" jsonschema:"maxItems=24" jsonschema_description:"type=enum 时的有限合法值。"`
	Visibility        string   `json:"visibility,omitempty" jsonschema:"enum=visible,enum=spoiler,enum=hidden" jsonschema_description:"省略时默认 visible。"`
	Description       string   `json:"description,omitempty" jsonschema_description:"字段承接的信息及语义。"`
	UpdateInstruction string   `json:"update_instruction,omitempty" jsonschema_description:"何时更新以及写入完整值还是增量。"`
	// Order is retained only for decoding older Beta presets. Field array order
	// is the fallback; final layout belongs to the user's stage preference.
	Order int `json:"order,omitempty"`
	// Group and Display are optional presentation hints for the stage state
	// ledger. Group clusters fields under one named ledger section; Display
	// pins the field renderer (stat/inline/block/list). Both fall back to
	// shape-based heuristics when empty and never affect state updates.
	Group   string `json:"group,omitempty"`
	Display string `json:"display,omitempty" jsonschema:"enum=stat,enum=inline,enum=block,enum=list" jsonschema_description:"可选展示提示；省略时由值形状推断。"`
}

const ActorStateSchemaVersion = 3

// ActorStateSchemaSnapshot is the story-local state contract. It is frozen at
// story creation so edits to reusable state modules affect only new stories.
type ActorStateSchemaSnapshot struct {
	Version              int                                   `json:"version"`
	Revision             int                                   `json:"revision"`
	System               StoryDirectorActorStateSystem         `json:"system"`
	TRPGSystem           StoryDirectorTRPGSystem               `json:"trpg_system,omitempty"`
	Adaptation           *ActorStateSchemaAdaptationRecord     `json:"adaptation,omitempty"`
	LegacyFieldPaths     map[string]map[string]string          `json:"legacy_field_paths,omitempty"`
	LegacyActorTemplates map[string]string                     `json:"legacy_actor_templates,omitempty"`
	FieldMigrations      map[string][]ActorStateFieldMigration `json:"field_migrations,omitempty"`
}

// ActorStateFieldMigration keeps historical state replayable after a story-
// local field rename or type change.
type ActorStateFieldMigration struct {
	From  string          `json:"from"`
	To    string          `json:"to"`
	Field ActorStateField `json:"field"`
}

// ActorStateOp is the v2 field-level reducer input. FieldID is an exact key,
// not a dotted path, so localized names and punctuation remain safe.
type ActorStateOp struct {
	Op           string `json:"op"`
	ActorID      string `json:"actor_id"`
	FieldID      string `json:"field_id"`
	Value        any    `json:"value,omitempty"`
	Reason       string `json:"reason,omitempty"`
	SourceTurnID string `json:"source_turn_id,omitempty"`
	SourceKind   string `json:"source_kind,omitempty"`
	SourceID     string `json:"source_id,omitempty"`
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
	ActorOps       []ActorStateOp                  `json:"actor_ops,omitempty"`
}

func normalizeActorStateFieldName(value string) string {
	return strings.TrimSpace(norm.NFKC.String(value))
}

// validateActorStateFieldName keeps field identity distinct from the JSON
// Pointer segments used by turn state updates.
func validateActorStateFieldName(value string) error {
	fieldID := normalizeActorStateFieldName(value)
	if fieldID == "" {
		return fmt.Errorf("状态字段名称不能为空 / State field name cannot be empty")
	}
	if strings.Contains(fieldID, "/") {
		return fmt.Errorf("状态字段名称不能包含路径分隔符“/” / State field name cannot contain the path separator “/”: %s", fieldID)
	}
	return nil
}

func actorStateFieldID(field ActorStateField) string {
	return normalizeActorStateFieldName(firstNonEmptyString(field.Name, field.ID, field.Path))
}

func actorStateFieldNameKey(value string) string {
	return cases.Fold().String(normalizeActorStateFieldName(value))
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
	if len(patches) > maxInteractiveListItems {
		patches = patches[:maxInteractiveListItems]
	}
	result := ActorStatePatchResult{AppliedActors: []string{}, CreatedActors: []string{}, AssignedTraits: map[string][]ActorTraitInstance{}, Ops: []StateOp{}}
	workingState := cloneActorStateRoot(currentState)
	seenActors := map[string]bool{}
	for _, patch := range patches {
		patch.SourceTurnID = firstNonEmptyString(patch.SourceTurnID, sourceTurnID)
		normalized, ops, actorOps, created, traits, err := validateActorStatePatch(system, workingState, patch)
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
		result.ActorOps = append(result.ActorOps, actorOps...)
		for _, op := range ops {
			applyStateOp(workingState, op)
		}
		for _, op := range actorOps {
			applyActorStateOp(workingState, op)
		}
	}
	result.Ops = normalizeStateOps(result.Ops)
	result.ActorOps = normalizeActorStateOps(result.ActorOps)
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
	if len(templates) > maxInteractiveListItems {
		templates = templates[:maxInteractiveListItems]
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
		template.Description = trimBytes(template.Description, maxInteractiveTextBytes)
		template.Fields = normalizeActorStateFields(template.Fields)
		template.TraitRules = normalizeActorTraitRules(template.TraitRules)
		template.DisplayGroups = nil
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
	for _, field := range fields {
		field.LegacyPath = strings.TrimSpace(firstNonEmptyString(field.LegacyPath, field.Path))
		field.Path = field.LegacyPath
		field.Name = normalizeActorStateFieldName(firstNonEmptyString(field.Name, field.ID, field.LegacyPath))
		if field.Name == "" {
			continue
		}
		field.ID = field.Name
		field.Type = normalizeActorStateFieldType(field.Type)
		field.Visibility = normalizeStoryDirectorVisibility(field.Visibility)
		field.Description = trimBytes(field.Description, maxInteractiveTextBytes)
		field.UpdateInstruction = trimBytes(field.UpdateInstruction, maxInteractiveTextBytes)
		field.Options = normalizeStringListLimit(field.Options, maxInteractiveListItems)
		field.Group = trimBytes(field.Group, 64)
		field.Display = normalizeActorStateFieldDisplay(field.Display)
		field.Order = 0
		out = append(out, field)
	}
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
	if len(actors) > maxInteractiveListItems {
		actors = actors[:maxInteractiveListItems]
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
		actor.Description = trimBytes(actor.Description, maxInteractiveTextBytes)
		template := actorStateTemplateByID(StoryDirectorActorStateSystem{Templates: templates}, actor.TemplateID)
		actor.State = normalizeActorStateMapForTemplate(actor.State, template)
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

// normalizeActorStateFieldDisplay keeps only the renderer hints the stage
// state ledger understands; anything else falls back to heuristics.
func normalizeActorStateFieldDisplay(value string) string {
	switch strings.TrimSpace(value) {
	case "stat", "inline", "block", "list":
		return strings.TrimSpace(value)
	default:
		return ""
	}
}

func normalizeActorStateMap(values map[string]any) map[string]any {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]any, len(values))
	for key, value := range values {
		key = normalizeActorStateFieldName(key)
		if key == "" {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeActorStateMapForTemplate(values map[string]any, template ActorStateTemplate) map[string]any {
	if len(values) == 0 {
		return nil
	}
	fieldsByRef := actorStateFieldsByReference(template)
	out := make(map[string]any, len(values))
	for ref, value := range values {
		field, ok := fieldsByRef[actorStateFieldNameKey(ref)]
		if !ok {
			continue
		}
		out[actorStateFieldID(field)] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func actorStateFieldsByReference(template ActorStateTemplate) map[string]ActorStateField {
	result := make(map[string]ActorStateField, len(template.Fields)*3)
	for _, field := range normalizeActorStateFields(template.Fields) {
		for _, ref := range []string{actorStateFieldID(field), field.Name, field.LegacyPath, field.Path} {
			if key := actorStateFieldNameKey(ref); key != "" {
				result[key] = field
			}
		}
	}
	return result
}

func actorStateFieldByID(template ActorStateTemplate, fieldID string) (ActorStateField, bool) {
	field, ok := actorStateFieldsByReference(template)[actorStateFieldNameKey(fieldID)]
	return field, ok
}

func actorStateFieldValue(state map[string]any, actorID, fieldID string) any {
	actor, _ := getPath(state, actorStateRoot+"."+normalizeActorStateID(actorID)).(map[string]any)
	if actor == nil {
		return nil
	}
	values, _ := actor["state"].(map[string]any)
	if values == nil {
		return nil
	}
	return values[normalizeActorStateFieldName(fieldID)]
}

func applyLegacyActorStateAliases(state map[string]any, snapshot *ActorStateSchemaSnapshot) {
	if snapshot == nil || len(snapshot.LegacyFieldPaths) == 0 {
		return
	}
	actors, _ := state[actorStateRoot].(map[string]any)
	for actorID, rawActor := range actors {
		actor, _ := rawActor.(map[string]any)
		if actor == nil {
			continue
		}
		templateID := ""
		if rawTemplateID, exists := actor["template_id"]; exists && rawTemplateID != nil {
			templateID = normalizeActorStateID(fmt.Sprint(rawTemplateID))
		}
		if templateID == "" {
			templateID = normalizeActorStateID(snapshot.LegacyActorTemplates[actorID])
			if templateID != "" {
				actor["template_id"] = templateID
			}
		}
		aliases := snapshot.LegacyFieldPaths[templateID]
		if len(aliases) == 0 {
			continue
		}
		fields, _ := actor["state"].(map[string]any)
		if fields == nil {
			fields = map[string]any{}
			actor["state"] = fields
		}
		for legacyPath, fieldID := range aliases {
			fieldID = normalizeActorStateFieldName(fieldID)
			if _, exists := fields[fieldID]; exists {
				continue
			}
			if value := getPathExact(fields, legacyPath); value != nil {
				if migration, ok := actorStateFieldMigrationFor(snapshot, templateID, legacyPath, fieldID); ok {
					if converted, convertedOK := coerceActorStateFieldValue(value, migration.Field); convertedOK {
						value = converted
					} else {
						value = migration.Field.Default
					}
				}
				fields[fieldID] = value
			}
		}
		actors[actorID] = actor
	}
}

func actorStateFieldMigrationFor(snapshot *ActorStateSchemaSnapshot, templateID, from, to string) (ActorStateFieldMigration, bool) {
	if snapshot == nil {
		return ActorStateFieldMigration{}, false
	}
	for _, migration := range snapshot.FieldMigrations[templateID] {
		if normalizeActorStateFieldName(migration.From) == normalizeActorStateFieldName(from) && normalizeActorStateFieldName(migration.To) == normalizeActorStateFieldName(to) {
			return migration, true
		}
	}
	return ActorStateFieldMigration{}, false
}

// enrichLegacyActorStateSchema preserves state that no longer has a matching
// reusable template field. The generated fields and actor-template bindings
// live only in the story snapshot and never leak back into global modules.
func enrichLegacyActorStateSchema(snapshot *ActorStateSchemaSnapshot, state map[string]any) {
	if snapshot == nil {
		return
	}
	actors, _ := state[actorStateRoot].(map[string]any)
	if len(actors) == 0 {
		return
	}
	if snapshot.LegacyFieldPaths == nil {
		snapshot.LegacyFieldPaths = map[string]map[string]string{}
	}
	if snapshot.LegacyActorTemplates == nil {
		snapshot.LegacyActorTemplates = map[string]string{}
	}
	for actorID, rawActor := range actors {
		actor, _ := rawActor.(map[string]any)
		fields, _ := actor["state"].(map[string]any)
		if actor == nil || len(fields) == 0 {
			continue
		}
		templateID := ""
		if rawTemplateID, exists := actor["template_id"]; exists && rawTemplateID != nil {
			templateID = normalizeActorStateID(fmt.Sprint(rawTemplateID))
		}
		if templateID == "" {
			templateID = "legacy_" + normalizeActorStateID(actorID)
			if templateID == "legacy_" {
				templateID = "legacy_actor"
			}
			snapshot.LegacyActorTemplates[actorID] = templateID
		}
		templateIndex := -1
		for i := range snapshot.System.Templates {
			if snapshot.System.Templates[i].ID == templateID {
				templateIndex = i
				break
			}
		}
		if templateIndex < 0 {
			actorName := strings.TrimSpace(fmt.Sprint(actor["name"]))
			if actorName == "" || actorName == "<nil>" {
				actorName = actorID
			}
			snapshot.System.Templates = append(snapshot.System.Templates, ActorStateTemplate{
				ID:          templateID,
				Name:        actorName,
				Description: "Legacy story-only state fields",
			})
			templateIndex = len(snapshot.System.Templates) - 1
		}
		template := &snapshot.System.Templates[templateIndex]
		aliases := snapshot.LegacyFieldPaths[templateID]
		if aliases == nil {
			aliases = map[string]string{}
			snapshot.LegacyFieldPaths[templateID] = aliases
		}
		legacyValues := map[string]any{}
		collectLegacyActorStateLeaves("", fields, legacyValues)
		for legacyPath, value := range legacyValues {
			if strings.TrimSpace(legacyPath) == "" {
				continue
			}
			if _, exists := aliases[legacyPath]; exists {
				continue
			}
			if field, exists := actorStateFieldByID(*template, legacyPath); exists {
				aliases[legacyPath] = actorStateFieldID(field)
				continue
			}
			order := (len(template.Fields) + 1) * 10
			template.Fields = append(template.Fields, ActorStateField{
				Name:       legacyPath,
				Type:       legacyActorStateFieldType(value),
				Visibility: "visible",
				Order:      order,
			})
			aliases[legacyPath] = legacyPath
		}
	}
	if len(snapshot.LegacyFieldPaths) == 0 {
		snapshot.LegacyFieldPaths = nil
	}
	if len(snapshot.LegacyActorTemplates) == 0 {
		snapshot.LegacyActorTemplates = nil
	}
	snapshot.System = normalizeActorStateSystem(snapshot.System)
}

func collectLegacyActorStateLeaves(prefix string, values map[string]any, out map[string]any) {
	for key, value := range values {
		path := strings.TrimSpace(key)
		if prefix != "" {
			path = prefix + "." + path
		}
		if nested, ok := value.(map[string]any); ok && len(nested) > 0 {
			collectLegacyActorStateLeaves(path, nested, out)
			continue
		}
		out[path] = value
	}
}

func legacyActorStateFieldType(value any) string {
	switch value.(type) {
	case bool:
		return "bool"
	case float32, float64, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return "number"
	case []any, []string:
		return "list"
	case map[string]any:
		return "object"
	default:
		return "string"
	}
}

func FreezeActorStateSchema(system StoryDirectorActorStateSystem, includeLegacy bool) *ActorStateSchemaSnapshot {
	return FreezeActorStateSchemaWithRules(system, StoryDirectorTRPGSystem{}, includeLegacy)
}

// FreezeActorStateSchemaWithRules freezes both field definitions and TRPG
// field references so template edits cannot change an existing story.
func FreezeActorStateSchemaWithRules(system StoryDirectorActorStateSystem, trpg StoryDirectorTRPGSystem, includeLegacy bool) *ActorStateSchemaSnapshot {
	system = normalizeActorStateSystem(system)
	legacy := map[string]map[string]string{}
	for templateIndex := range system.Templates {
		template := &system.Templates[templateIndex]
		for fieldIndex := range template.Fields {
			field := &template.Fields[fieldIndex]
			if includeLegacy && strings.TrimSpace(field.LegacyPath) != "" {
				if legacy[template.ID] == nil {
					legacy[template.ID] = map[string]string{}
				}
				legacy[template.ID][strings.TrimSpace(field.LegacyPath)] = actorStateFieldID(*field)
			}
			field.ID = ""
			field.Path = ""
			field.LegacyPath = ""
		}
	}
	if len(legacy) == 0 {
		legacy = nil
	}
	return &ActorStateSchemaSnapshot{Version: ActorStateSchemaVersion, Revision: 1, System: system, TRPGSystem: normalizeFrozenTRPGSystem(trpg), LegacyFieldPaths: legacy}
}

func actorStateSystemFromSnapshot(snapshot *ActorStateSchemaSnapshot, fallback StoryDirectorActorStateSystem) StoryDirectorActorStateSystem {
	if snapshot != nil && snapshot.Version > 0 && len(snapshot.System.Templates) > 0 {
		return normalizeActorStateSystem(snapshot.System)
	}
	return normalizeActorStateSystem(fallback)
}

func normalizeActorStateSchemaSnapshot(snapshot *ActorStateSchemaSnapshot) *ActorStateSchemaSnapshot {
	if snapshot == nil {
		return nil
	}
	next := *snapshot
	if next.Version <= 0 {
		next.Version = ActorStateSchemaVersion
	}
	if next.Revision <= 0 {
		next.Revision = 1
	}
	next.System = normalizeActorStateSystem(next.System)
	next.TRPGSystem = normalizeFrozenTRPGSystem(next.TRPGSystem)
	for templateIndex := range next.System.Templates {
		for fieldIndex := range next.System.Templates[templateIndex].Fields {
			field := &next.System.Templates[templateIndex].Fields[fieldIndex]
			field.ID = ""
			field.Path = ""
			field.LegacyPath = ""
		}
	}
	for templateID, migrations := range next.FieldMigrations {
		for index := range migrations {
			migrations[index].From = normalizeActorStateFieldName(migrations[index].From)
			migrations[index].To = normalizeActorStateFieldName(migrations[index].To)
			if normalized := normalizeActorStateFields([]ActorStateField{migrations[index].Field}); len(normalized) == 1 {
				migrations[index].Field = normalized[0]
			}
		}
		next.FieldMigrations[templateID] = migrations
	}
	return &next
}

func normalizeFrozenTRPGSystem(system StoryDirectorTRPGSystem) StoryDirectorTRPGSystem {
	system.RuleTemplates = normalizeRuleChecks(system.RuleTemplates)
	return system
}

func validateActorStateSystem(system StoryDirectorActorStateSystem) error {
	system = normalizeActorStateSystem(system)
	for _, template := range system.Templates {
		seen := map[string]string{}
		for _, field := range template.Fields {
			fieldID := actorStateFieldID(field)
			if err := validateActorStateFieldName(fieldID); err != nil {
				return fmt.Errorf("Actor 状态模板 %s: %w", template.ID, err)
			}
			key := actorStateFieldNameKey(fieldID)
			if previous, ok := seen[key]; ok {
				return fmt.Errorf("Actor 状态模板 %s 状态名称重复: %s / %s", template.ID, previous, fieldID)
			}
			seen[key] = fieldID
		}
	}
	return nil
}

func actorStateEmpty(system StoryDirectorActorStateSystem) bool {
	return len(system.Templates) == 0 && len(system.InitialActors) == 0 && len(system.TraitPools) == 0
}

func defaultActorStateSystem() StoryDirectorActorStateSystem {
	return actorStateSystemForPreset(defaultActorStatePresetSpec())
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

func validateActorStatePatch(system StoryDirectorActorStateSystem, currentState map[string]any, patch ActorStatePatch) (ActorStatePatch, []StateOp, []ActorStateOp, bool, []ActorTraitInstance, error) {
	system = normalizeActorStateSystem(system)
	patch.ActorID = normalizeActorStateID(patch.ActorID)
	if patch.ActorID == "" {
		return patch, nil, nil, false, nil, fmt.Errorf("Actor 状态更新缺少 actor_id")
	}
	existingActor := getPath(currentState, actorStateRoot+"."+patch.ActorID)
	created := existingActor == nil
	if !created {
		if _, ok := existingActor.(map[string]any); !ok {
			return patch, nil, nil, false, nil, fmt.Errorf("Actor 状态对象结构无效: %s", patch.ActorID)
		}
	}
	existingTemplateID := ""
	if rawTemplateID, ok := getPath(currentState, actorStateActorPath(patch.ActorID, "template_id")).(string); ok {
		existingTemplateID = normalizeActorStateID(rawTemplateID)
	}
	patch.TemplateID = normalizeActorStateID(patch.TemplateID)
	if created && patch.TemplateID == "" {
		return patch, nil, nil, false, nil, fmt.Errorf("创建 Actor 状态对象必须提供 template_id: %s", patch.ActorID)
	}
	bindLegacyTemplate := !created && existingTemplateID == ""
	if !created {
		if bindLegacyTemplate && patch.TemplateID == "" {
			return patch, nil, nil, false, nil, fmt.Errorf("旧 Actor 状态对象缺少 template_id，更新时必须显式绑定: %s", patch.ActorID)
		}
		if !bindLegacyTemplate && patch.TemplateID == "" {
			patch.TemplateID = existingTemplateID
		} else if !bindLegacyTemplate && patch.TemplateID != existingTemplateID {
			return patch, nil, nil, false, nil, fmt.Errorf("已有 Actor 的状态模板不可隐式更换: actor=%s current=%s requested=%s", patch.ActorID, existingTemplateID, patch.TemplateID)
		}
	}
	template := actorStateTemplateByID(system, patch.TemplateID)
	if template.ID == "" {
		return patch, nil, nil, false, nil, fmt.Errorf("Actor 状态模板不存在: %s", patch.TemplateID)
	}
	fieldByReference := actorStateFieldsByReference(template)
	if len(patch.State) == 0 && len(patch.TraitChanges) == 0 && !created && !bindLegacyTemplate {
		return patch, nil, nil, false, nil, fmt.Errorf("Actor 状态更新缺少 state 或 trait_changes")
	}
	reason := trimBytes(patch.Reason, maxInteractiveTextBytes)
	sourceTurnID := trimBytes(patch.SourceTurnID, 128)
	ops := []StateOp{}
	actorOps := []ActorStateOp{}
	if created {
		baseOps, baseActorOps, normalizedState, err := buildNewActorStateOps(template, patch.ActorID, patch.ActorName, patch.Role, patch.Description, patch.State, reason, sourceTurnID)
		if err != nil {
			return patch, nil, nil, false, nil, err
		}
		patch.State = normalizedState
		ops = append(ops, baseOps...)
		actorOps = append(actorOps, baseActorOps...)
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
			ops = append(ops, StateOp{Op: "set", Path: actorStateActorPath(patch.ActorID, "description"), Value: trimBytes(patch.Description, maxInteractiveTextBytes), Reason: reason, SourceTurnID: sourceTurnID})
		}
	}
	if !created {
		keys := make([]string, 0, len(patch.State))
		for key := range patch.State {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			field, ok := fieldByReference[actorStateFieldNameKey(key)]
			if !ok {
				allowed := make([]string, 0, len(template.Fields))
				for _, candidate := range template.Fields {
					allowed = append(allowed, actorStateFieldID(candidate))
				}
				return patch, nil, nil, false, nil, fmt.Errorf("Actor 状态字段不在模板中: actor=%s template=%s field=%s，合法状态名称: %s", patch.ActorID, patch.TemplateID, key, strings.Join(allowed, "、"))
			}
			value, err := normalizeActorStateValue(field, patch.State[key])
			if err != nil {
				return patch, nil, nil, false, nil, err
			}
			fieldID := actorStateFieldID(field)
			delete(patch.State, key)
			patch.State[fieldID] = value
			actorOps = append(actorOps, ActorStateOp{Op: "set", ActorID: patch.ActorID, FieldID: fieldID, Value: value, Reason: reason, SourceTurnID: sourceTurnID})
		}
	}
	traits := actorTraitInstancesFromState(currentState, patch.ActorID)
	if created {
		result, err := rollActorTraits(system, ActorTraitRollRequest{ActorID: patch.ActorID, TemplateID: patch.TemplateID}, "actor_create", sourceTurnID)
		if err != nil {
			return patch, nil, nil, false, nil, err
		}
		traits = result.Traits
	}
	changedTraits := created && len(traits) > 0
	if len(patch.TraitChanges) > 0 {
		nextTraits, changed, err := applyActorTraitChanges(system, template, patch.ActorID, traits, patch.TraitChanges, sourceTurnID)
		if err != nil {
			return patch, nil, nil, false, nil, err
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
	return patch, normalizeStateOps(ops), normalizeActorStateOps(actorOps), created, reportedTraits, nil
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
			return nil, fmt.Errorf("Actor 状态字段 %s 必须是 number", actorStateFieldID(field))
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
		return nil, fmt.Errorf("Actor 状态字段 %s 必须是 bool", actorStateFieldID(field))
	case "enum":
		text := strings.TrimSpace(fmt.Sprint(value))
		for _, option := range field.Options {
			if text == option {
				return text, nil
			}
		}
		return nil, fmt.Errorf("Actor 状态字段 %s 不在枚举选项中: %s", actorStateFieldID(field), text)
	case "object":
		if typed, ok := value.(map[string]any); ok {
			return typed, nil
		}
		return nil, fmt.Errorf("Actor 状态字段 %s 必须是 object", actorStateFieldID(field))
	case "list":
		if typed, ok := value.([]any); ok {
			return typed, nil
		}
		return nil, fmt.Errorf("Actor 状态字段 %s 必须是 list", actorStateFieldID(field))
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
