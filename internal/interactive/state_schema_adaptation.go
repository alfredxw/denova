package interactive

import (
	"encoding/json"
	"fmt"
	"strings"
)

const maxActorStateSchemaAdaptationOps = 64

const (
	StateSchemaInitializationWaitingOpening = "waiting_opening"
	StateSchemaInitializationReady          = "ready"
)

// ActorStateSchemaAdaptation is a bounded story-local diff over a reusable
// State System. The opening Game Agent proposes it before any state is
// materialized, and the backend validates it before freezing the result.
type ActorStateSchemaAdaptation struct {
	Summary         string                           `json:"summary,omitempty"`
	TemplateOps     []ActorStateTemplateSchemaOp     `json:"template_ops,omitempty"`
	InitialActorOps []ActorStateInitialActorSchemaOp `json:"initial_actor_ops,omitempty"`
	ActorOps        []ActorStateRuntimeSchemaOp      `json:"actor_ops,omitempty"`
}

// ActorStateTemplateSchemaOp adds or removes a template, or applies field
// operations to one existing template. Existing template metadata and trait
// rules remain stable when only FieldOps are supplied.
type ActorStateTemplateSchemaOp struct {
	Op         string                    `json:"op" jsonschema:"enum=add,enum=remove,enum=fields" jsonschema_description:"add supplies a complete new template; remove deletes template_id; fields applies field_ops to an existing template_id."`
	TemplateID string                    `json:"template_id,omitempty" jsonschema_description:"Required for remove/fields. Copy an existing Template ID exactly."`
	Template   ActorStateTemplate        `json:"template,omitempty" jsonschema_description:"Complete new template required only for add."`
	FieldOps   []ActorStateFieldSchemaOp `json:"field_ops,omitempty" jsonschema:"maxItems=64" jsonschema_description:"Used only with fields; each item is add/replace/remove."`
	Reason     string                    `json:"reason,omitempty" jsonschema_description:"Why this story requires this minimal schema change."`
}

// ActorStateFieldSchemaOp uses add, replace, or remove. FieldID identifies the
// existing field for replace/remove; Field contains the complete new field for
// add/replace.
type ActorStateFieldSchemaOp struct {
	Op      string          `json:"op" jsonschema:"enum=add,enum=replace,enum=remove" jsonschema_description:"Field operation type."`
	FieldID string          `json:"field_id,omitempty" jsonschema_description:"Required for replace/remove. Copy the existing Field ID exactly."`
	Field   ActorStateField `json:"field,omitempty" jsonschema_description:"Complete field definition required for add/replace."`
	Reason  string          `json:"reason,omitempty" jsonschema_description:"Why this story requires this field change."`
}

// ActorStateInitialActorSchemaOp uses add, replace, or remove for story-local
// initial state objects. Dynamic actors created after the opening are not part
// of this initialization contract.
type ActorStateInitialActorSchemaOp struct {
	Op          string                            `json:"op"`
	ActorID     string                            `json:"actor_id,omitempty"`
	Actor       ActorStateInitialActor            `json:"actor,omitempty"`
	Reason      string                            `json:"reason,omitempty"`
	ValueSource *ActorStateSchemaActorValueSource `json:"value_source,omitempty" jsonschema:"-"`
}

// ActorStateRuntimeSchemaOp migrates an Actor already materialized in the
// story. It does not change the reusable initial-Actor definitions.
type ActorStateRuntimeSchemaOp struct {
	Op          string                            `json:"op"`
	ActorID     string                            `json:"actor_id,omitempty"`
	FieldID     string                            `json:"field_id,omitempty" jsonschema:"description=Frozen-schema field_id to initialize when op=set"`
	Value       any                               `json:"value,omitempty" jsonschema:"description=Non-empty field value to write when op=set"`
	Actor       ActorStateInitialActor            `json:"actor,omitempty"`
	Reason      string                            `json:"reason,omitempty"`
	ValueSource *ActorStateSchemaActorValueSource `json:"value_source,omitempty" jsonschema:"-"`
}

// ActorStateSchemaActorValueSource links Actor values produced by a Batch item
// to the exact requirement evidence persisted in the schema adaptation audit.
type ActorStateSchemaActorValueSource struct {
	SourceID string                            `json:"source_id"`
	ItemID   string                            `json:"item_id"`
	Source   ActorStateSchemaRequirementSource `json:"source"`
}

// ActorStateSchemaAdaptationRecord is persisted with the frozen story schema
// so the customized contract has an explicit source and a compact audit trail.
type ActorStateSchemaAdaptationRecord struct {
	Source          string                              `json:"source"`
	Summary         string                              `json:"summary,omitempty"`
	SourceTurnID    string                              `json:"source_turn_id,omitempty"`
	LoreRevision    string                              `json:"lore_revision,omitempty"`
	TemplateOps     int                                 `json:"template_ops,omitempty"`
	FieldOps        int                                 `json:"field_ops,omitempty"`
	InitialActorOps int                                 `json:"initial_actor_ops,omitempty"`
	ActorOps        int                                 `json:"actor_ops,omitempty"`
	ReviewedLoreIDs []string                            `json:"reviewed_lore_ids,omitempty"`
	Requirements    []ActorStateSchemaRequirementReview `json:"requirements,omitempty"`
	Changes         []ActorStateSchemaAdaptationChange  `json:"changes,omitempty"`
	Warnings        []string                            `json:"warnings,omitempty"`
}

// ActorStateSchemaAdaptationChange is a bounded user-visible audit item for
// one schema change proposed by the opening Game Agent.
type ActorStateSchemaAdaptationChange struct {
	Kind        string                            `json:"kind"`
	Op          string                            `json:"op"`
	TemplateID  string                            `json:"template_id,omitempty"`
	FieldID     string                            `json:"field_id,omitempty"`
	TargetID    string                            `json:"target_id,omitempty"`
	ActorID     string                            `json:"actor_id,omitempty"`
	Reason      string                            `json:"reason,omitempty"`
	ValueSource *ActorStateSchemaActorValueSource `json:"value_source,omitempty"`
}

// StateSchemaInitializationStatus tracks the foreground opening handshake. It
// never represents a background Director task because all branches share one
// schema that is frozen with the first committed turn.
type StateSchemaInitializationStatus struct {
	Mode            string                              `json:"mode"`
	Status          string                              `json:"status"`
	Outcome         string                              `json:"outcome,omitempty"`
	SourceTurnID    string                              `json:"source_turn_id,omitempty"`
	BaseRevision    int                                 `json:"base_revision,omitempty"`
	TargetRevision  int                                 `json:"target_revision,omitempty"`
	Summary         string                              `json:"summary,omitempty"`
	LoreRevision    string                              `json:"lore_revision,omitempty"`
	ReviewedLoreIDs []string                            `json:"reviewed_lore_ids,omitempty"`
	Requirements    []ActorStateSchemaRequirementReview `json:"requirements,omitempty"`
	Changes         []ActorStateSchemaAdaptationChange  `json:"changes,omitempty"`
	Warnings        []string                            `json:"warnings,omitempty"`
	StartedAt       string                              `json:"started_at,omitempty"`
	CompletedAt     string                              `json:"completed_at,omitempty"`
	UpdatedAt       string                              `json:"updated_at,omitempty"`
}

func ParseActorStateSchemaAdaptation(content string) (ActorStateSchemaAdaptation, error) {
	payload := strings.TrimSpace(content)
	if start := strings.Index(payload, "{"); start >= 0 {
		if end := strings.LastIndex(payload, "}"); end >= start {
			payload = payload[start : end+1]
		}
	}
	if payload == "" {
		return ActorStateSchemaAdaptation{}, fmt.Errorf("state schema adaptation result is empty")
	}
	var adaptation ActorStateSchemaAdaptation
	if err := json.Unmarshal([]byte(payload), &adaptation); err != nil {
		return ActorStateSchemaAdaptation{}, fmt.Errorf("parse state schema adaptation result: %w", err)
	}
	adaptation.Summary = trimBytes(adaptation.Summary, maxInteractiveTextBytes)
	if len(adaptation.TemplateOps) > maxActorStateSchemaAdaptationOps {
		return ActorStateSchemaAdaptation{}, fmt.Errorf("too many state schema template operations: %d > %d", len(adaptation.TemplateOps), maxActorStateSchemaAdaptationOps)
	}
	if len(adaptation.InitialActorOps) > maxActorStateSchemaAdaptationOps {
		return ActorStateSchemaAdaptation{}, fmt.Errorf("too many initial Actor operations: %d > %d", len(adaptation.InitialActorOps), maxActorStateSchemaAdaptationOps)
	}
	if len(adaptation.ActorOps) > maxActorStateSchemaAdaptationOps {
		return ActorStateSchemaAdaptation{}, fmt.Errorf("too many runtime Actor operations: %d > %d", len(adaptation.ActorOps), maxActorStateSchemaAdaptationOps)
	}
	fieldOps := 0
	for index := range adaptation.TemplateOps {
		op := &adaptation.TemplateOps[index]
		op.Op = strings.TrimSpace(op.Op)
		op.TemplateID = normalizeActorStateID(op.TemplateID)
		op.Reason = trimBytes(op.Reason, maxInteractiveTextBytes)
		fieldOps += len(op.FieldOps)
		for fieldIndex := range op.FieldOps {
			fieldOp := &op.FieldOps[fieldIndex]
			fieldOp.Op = strings.TrimSpace(fieldOp.Op)
			fieldOp.FieldID = normalizeActorStateFieldName(fieldOp.FieldID)
			fieldOp.Reason = trimBytes(fieldOp.Reason, maxInteractiveTextBytes)
		}
	}
	if fieldOps > maxActorStateSchemaAdaptationOps {
		return ActorStateSchemaAdaptation{}, fmt.Errorf("too many state field operations: %d > %d", fieldOps, maxActorStateSchemaAdaptationOps)
	}
	for index := range adaptation.InitialActorOps {
		op := &adaptation.InitialActorOps[index]
		op.Op = strings.TrimSpace(op.Op)
		op.ActorID = normalizeStatePanelActorID(op.ActorID)
		op.Reason = trimBytes(op.Reason, maxInteractiveTextBytes)
		normalizeActorStateSchemaActorValueSource(op.ValueSource)
	}
	for index := range adaptation.ActorOps {
		op := &adaptation.ActorOps[index]
		op.Op = strings.TrimSpace(op.Op)
		op.ActorID = normalizeStatePanelActorID(op.ActorID)
		op.FieldID = normalizeActorStateFieldName(op.FieldID)
		op.Actor.ID = normalizeStatePanelActorID(op.Actor.ID)
		op.Actor.TemplateID = normalizeActorStateID(op.Actor.TemplateID)
		op.Reason = trimBytes(op.Reason, maxInteractiveTextBytes)
		normalizeActorStateSchemaActorValueSource(op.ValueSource)
		if op.Op != "add" && op.Op != "replace" && op.Op != "remove" && op.Op != "set" {
			return ActorStateSchemaAdaptation{}, fmt.Errorf("invalid runtime Actor operation: %s", op.Op)
		}
		if firstNonEmptyString(op.ActorID, op.Actor.ID) == "" {
			return ActorStateSchemaAdaptation{}, fmt.Errorf("runtime Actor operation is missing actor_id")
		}
	}
	return adaptation, nil
}

func normalizeActorStateSchemaActorValueSource(source *ActorStateSchemaActorValueSource) {
	if source == nil {
		return
	}
	source.SourceID = strings.TrimSpace(source.SourceID)
	source.ItemID = strings.TrimSpace(source.ItemID)
	source.Source.Kind = strings.TrimSpace(source.Source.Kind)
	source.Source.ID = strings.TrimSpace(source.Source.ID)
}

// ApplyActorStateSchemaAdaptation applies a Game Agent schema diff and validates
// the complete State System plus every frozen TRPG binding.
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
	if err := validateActorStateRuntimeSchemaOps(system, adaptation.ActorOps); err != nil {
		return StoryDirectorActorStateSystem{}, ActorStateSchemaAdaptationRecord{}, err
	}
	fieldOps := 0
	for _, op := range adaptation.TemplateOps {
		fieldOps += len(op.FieldOps)
	}
	record := ActorStateSchemaAdaptationRecord{
		Source:          "game_agent",
		Summary:         trimBytes(adaptation.Summary, maxInteractiveTextBytes),
		TemplateOps:     len(adaptation.TemplateOps),
		FieldOps:        fieldOps,
		InitialActorOps: len(adaptation.InitialActorOps),
		ActorOps:        len(adaptation.ActorOps),
	}
	return normalizeActorStateSystem(system), record, nil
}

func applyActorStateTemplateSchemaOp(system StoryDirectorActorStateSystem, op ActorStateTemplateSchemaOp) (StoryDirectorActorStateSystem, error) {
	switch op.Op {
	case "add":
		if normalizeActorStateID(op.Template.ID) == "" {
			return system, fmt.Errorf("new state template is missing a valid template.id")
		}
		for _, field := range op.Template.Fields {
			if err := validateActorStateAdaptationField(field); err != nil {
				return system, fmt.Errorf("add state template %s: %w", op.Template.ID, err)
			}
		}
		templates := normalizeActorStateTemplates([]ActorStateTemplate{op.Template})
		if len(templates) != 1 || templates[0].ID == "" {
			return system, fmt.Errorf("new state template is missing a valid template.id")
		}
		if actorStateTemplateByID(system, templates[0].ID).ID != "" {
			return system, fmt.Errorf("state template already exists: %s", templates[0].ID)
		}
		system.Templates = append(system.Templates, templates[0])
		return system, nil
	case "remove":
		templateID := normalizeActorStateID(op.TemplateID)
		if templateID == "" {
			return system, fmt.Errorf("state template removal is missing template_id")
		}
		if templateID == DefaultActorID || templateID == ActorStateStoryContextTemplateID {
			return system, fmt.Errorf("core story state template cannot be removed: %s", templateID)
		}
		index := actorStateTemplateIndex(system.Templates, templateID)
		if index < 0 {
			return system, fmt.Errorf("state template to remove does not exist: %s", templateID)
		}
		system.Templates = append(system.Templates[:index], system.Templates[index+1:]...)
		return system, nil
	case "fields":
		templateID := normalizeActorStateID(op.TemplateID)
		index := actorStateTemplateIndex(system.Templates, templateID)
		if index < 0 {
			return system, fmt.Errorf("state field operation references a missing template: %s", templateID)
		}
		template := system.Templates[index]
		for _, fieldOp := range op.FieldOps {
			var err error
			template, err = applyActorStateFieldSchemaOp(template, fieldOp)
			if err != nil {
				return system, fmt.Errorf("state template %s: %w", templateID, err)
			}
		}
		system.Templates[index] = template
		return system, nil
	default:
		return system, fmt.Errorf("invalid state template operation: %s", op.Op)
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
			return template, fmt.Errorf("new state field is missing a valid field.name")
		}
		if _, ok := actorStateFieldByID(template, actorStateFieldID(fields[0])); ok {
			return template, fmt.Errorf("state field already exists: %s", actorStateFieldID(fields[0]))
		}
		template.Fields = append(template.Fields, fields[0])
		template.Fields = normalizeActorStateFields(template.Fields)
		return template, nil
	case "replace":
		index := actorStateFieldIndex(template.Fields, op.FieldID)
		if index < 0 {
			return template, fmt.Errorf("state field to replace does not exist: %s", op.FieldID)
		}
		if err := validateActorStateAdaptationField(op.Field); err != nil {
			return template, err
		}
		fields := normalizeActorStateFields([]ActorStateField{op.Field})
		if len(fields) != 1 || actorStateFieldID(fields[0]) == "" {
			return template, fmt.Errorf("state field replacement is missing a complete valid field")
		}
		for fieldIndex, existing := range template.Fields {
			if fieldIndex != index && actorStateFieldNameKey(actorStateFieldID(existing)) == actorStateFieldNameKey(actorStateFieldID(fields[0])) {
				return template, fmt.Errorf("replacement state field duplicates a name: %s", actorStateFieldID(fields[0]))
			}
		}
		template.Fields[index] = fields[0]
		template.Fields = normalizeActorStateFields(template.Fields)
		return template, nil
	case "remove":
		index := actorStateFieldIndex(template.Fields, op.FieldID)
		if index < 0 {
			return template, fmt.Errorf("state field to remove does not exist: %s", op.FieldID)
		}
		template.Fields = append(template.Fields[:index], template.Fields[index+1:]...)
		return template, nil
	default:
		return template, fmt.Errorf("invalid state field operation: %s", op.Op)
	}
}

func applyActorStateInitialActorSchemaOp(system StoryDirectorActorStateSystem, op ActorStateInitialActorSchemaOp) (StoryDirectorActorStateSystem, error) {
	switch op.Op {
	case "add":
		actor := op.Actor
		actor.ID = normalizeStatePanelActorID(actor.ID)
		if actor.ID == "" {
			return system, fmt.Errorf("new initial Actor is missing a valid actor.id")
		}
		if actorStateInitialActorIndex(system.InitialActors, actor.ID) >= 0 {
			return system, fmt.Errorf("initial Actor already exists: %s", actor.ID)
		}
		if actorStateTemplateByID(system, actor.TemplateID).ID == "" {
			return system, fmt.Errorf("new initial Actor references a missing template: actor=%s template=%s", actor.ID, actor.TemplateID)
		}
		system.InitialActors = append(system.InitialActors, actor)
	case "replace":
		index := actorStateInitialActorIndex(system.InitialActors, op.ActorID)
		if index < 0 {
			return system, fmt.Errorf("initial Actor to replace does not exist: %s", op.ActorID)
		}
		actor := op.Actor
		actor.ID = normalizeStatePanelActorID(actor.ID)
		if actor.ID == "" {
			actor.ID = normalizeStatePanelActorID(op.ActorID)
		}
		if actor.ID != normalizeStatePanelActorID(op.ActorID) {
			return system, fmt.Errorf("initial Actor ID cannot change during replacement: %s -> %s", op.ActorID, actor.ID)
		}
		if actorStateTemplateByID(system, actor.TemplateID).ID == "" {
			return system, fmt.Errorf("replacement initial Actor references a missing template: actor=%s template=%s", actor.ID, actor.TemplateID)
		}
		system.InitialActors[index] = actor
	case "remove":
		actorID := normalizeStatePanelActorID(op.ActorID)
		if actorID == DefaultActorID || actorID == DefaultStoryContextActorID {
			return system, fmt.Errorf("core story initial Actor cannot be removed: %s", actorID)
		}
		index := actorStateInitialActorIndex(system.InitialActors, actorID)
		if index < 0 {
			return system, fmt.Errorf("initial Actor to remove does not exist: %s", actorID)
		}
		system.InitialActors = append(system.InitialActors[:index], system.InitialActors[index+1:]...)
	default:
		return system, fmt.Errorf("invalid initial Actor operation: %s", op.Op)
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
	actorID = normalizeStatePanelActorID(actorID)
	for index := range actors {
		if normalizeStatePanelActorID(actors[index].ID) == actorID {
			return index
		}
	}
	return -1
}
