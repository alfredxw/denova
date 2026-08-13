package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	agent "github.com/alfredxw/denova/agent"

	"denova/internal/interactive"
)

const initializeStoryStateSchemaToolName = "initialize_story_state_schema"

type openingStateSchemaBatchToolInput struct {
	Summary  string                                 `json:"summary,omitempty" jsonschema_description:"Brief summary of this opening state-schema review."`
	Items    []openingStateSchemaBatchItemToolInput `json:"items" jsonschema_description:"Independent proposal items newly submitted or retried in this call. The backend returns accepted/rejected/blocked per item; resend only failed items."`
	Finalize bool                                   `json:"finalize" jsonschema_description:"Whether to complete the draft after this batch succeeds. The backend cannot finalize while any item is rejected or blocked."`
}

type openingStateSchemaBatchItemToolInput struct {
	ItemID       string                                         `json:"item_id" jsonschema_description:"Stable unique idempotency ID using only letters, digits, dots, underscores, colons, or hyphens."`
	DependsOn    []string                                       `json:"depends_on,omitempty" jsonschema:"maxItems=16" jsonschema_description:"Other item_id values this item depends on."`
	Summary      string                                         `json:"summary,omitempty" jsonschema_description:"Brief summary of this item's review or structural change."`
	Requirements []openingStateSchemaRequirementReviewToolInput `json:"requirements" jsonschema:"minItems=1,maxItems=64" jsonschema_description:"Self-contained sourced field reviews. covered/add/replace require template_id, field_id, and expected_type. remove requires the existing template_id, field_id, and reason."`
	Adaptation   openingStateSchemaAdaptationToolInput          `json:"adaptation" jsonschema_description:"Only template_ops are allowed. Create Actors and set values later through submit_interactive_turn.state_changes."`
}

type openingStateSchemaRequirementSourceToolInput struct {
	Kind string `json:"kind" jsonschema:"enum=opening,enum=lore,enum=trpg" jsonschema_description:"Source kind. Use opening for the opening draft, lore only for material already read, and trpg for rule templates."`
	ID   string `json:"id" jsonschema_description:"Stable source ID. When kind=opening, use opening-draft exactly."`
}

type openingStateSchemaRequirementReviewToolInput struct {
	Source       openingStateSchemaRequirementSourceToolInput `json:"source"`
	Requirement  string                                       `json:"requirement,omitempty" jsonschema_description:"Why this story needs to retain, add, or replace this long-lived state field, or why an inherited field is unsuitable. For remove/ignored, a non-empty reason is reused if this is accidentally omitted; other decisions still require it."`
	ValuePolicy  string                                       `json:"value_policy" jsonschema:"enum=schema_only" jsonschema_description:"Always schema_only for the opening structure tool. Do not set Actor values here."`
	ExpectedType string                                       `json:"expected_type,omitempty" jsonschema:"enum=number,enum=string,enum=bool,enum=enum,enum=object,enum=list" jsonschema_description:"Required for covered/add/replace and must match the target field type. May be omitted for remove/ignored."`
	Min          *float64                                     `json:"min,omitempty" jsonschema_description:"Provide only when the source explicitly requires a numeric lower bound and it matches the target field."`
	Max          *float64                                     `json:"max,omitempty" jsonschema_description:"Provide only when the source explicitly requires a numeric upper bound and it matches the target field."`
	Decision     string                                       `json:"decision" jsonschema:"enum=covered,enum=add,enum=replace,enum=remove,enum=ignored" jsonschema_description:"covered retains an existing field; add/replace/remove require a matching template_ops field operation; ignored excludes the source requirement from long-lived state."`
	TemplateID   string                                       `json:"template_id,omitempty" jsonschema_description:"Required for covered/add/replace/remove. Copy the Template ID exactly from the Actor State Handbook."`
	FieldID      string                                       `json:"field_id,omitempty" jsonschema_description:"Required for covered/add/replace/remove. Copy the target Field ID exactly; for add, provide the new Field ID."`
	Reason       string                                       `json:"reason,omitempty" jsonschema_description:"Required for remove/ignored. For other decisions, use only when a tradeoff needs explanation."`
}

type openingStateSchemaAdaptationToolInput struct {
	Summary     string                                   `json:"summary,omitempty" jsonschema_description:"Brief summary of the structural diff."`
	TemplateOps []interactive.ActorStateTemplateSchemaOp `json:"template_ops,omitempty" jsonschema:"maxItems=64" jsonschema_description:"Smallest template and field diff needed for this item's requirements; use an empty array for covered."`
}

func (input openingStateSchemaBatchToolInput) batch() interactive.ActorStateSchemaBatch {
	batch := interactive.ActorStateSchemaBatch{
		Summary:  input.Summary,
		Items:    make([]interactive.ActorStateSchemaBatchItem, 0, len(input.Items)),
		Finalize: input.Finalize,
	}
	for _, item := range input.Items {
		converted := interactive.ActorStateSchemaBatchItem{
			ItemID:       item.ItemID,
			DependsOn:    append([]string(nil), item.DependsOn...),
			Summary:      item.Summary,
			Requirements: make([]interactive.ActorStateSchemaRequirementReview, 0, len(item.Requirements)),
			Adaptation: interactive.ActorStateSchemaAdaptation{
				Summary:     item.Adaptation.Summary,
				TemplateOps: normalizeOpeningStateSchemaTemplateOps(item.Adaptation.TemplateOps),
			},
		}
		for _, requirement := range item.Requirements {
			requirementText := requirement.Requirement
			if strings.TrimSpace(requirementText) == "" && (requirement.Decision == "remove" || requirement.Decision == "ignored") {
				requirementText = requirement.Reason
			}
			converted.Requirements = append(converted.Requirements, interactive.ActorStateSchemaRequirementReview{
				Source: interactive.ActorStateSchemaRequirementSource{
					Kind: requirement.Source.Kind,
					ID:   requirement.Source.ID,
				},
				Requirement:  requirementText,
				ValuePolicy:  requirement.ValuePolicy,
				ExpectedType: requirement.ExpectedType,
				Min:          requirement.Min,
				Max:          requirement.Max,
				Decision:     requirement.Decision,
				TemplateID:   requirement.TemplateID,
				FieldID:      requirement.FieldID,
				Reason:       requirement.Reason,
			})
		}
		batch.Items = append(batch.Items, converted)
	}
	return batch
}

// normalizeOpeningStateSchemaTemplateOps repairs one common model-only shape:
// a boolean marker supplied as the default for a non-boolean field. Dropping
// that optional marker keeps the field required by the initialization guide;
// no user state or model context is invented or discarded.
func normalizeOpeningStateSchemaTemplateOps(ops []interactive.ActorStateTemplateSchemaOp) []interactive.ActorStateTemplateSchemaOp {
	normalized := append([]interactive.ActorStateTemplateSchemaOp(nil), ops...)
	for templateIndex := range normalized {
		normalized[templateIndex].FieldOps = append([]interactive.ActorStateFieldSchemaOp(nil), normalized[templateIndex].FieldOps...)
		for fieldIndex := range normalized[templateIndex].FieldOps {
			fieldOp := &normalized[templateIndex].FieldOps[fieldIndex]
			if fieldOp.Op != "add" && fieldOp.Op != "replace" {
				continue
			}
			if _, isBooleanMarker := fieldOp.Field.Default.(bool); isBooleanMarker && strings.TrimSpace(fieldOp.Field.Type) != "bool" {
				fieldOp.Field.Default = nil
			}
		}
	}
	return normalized
}

func newInteractiveOpeningStateSchemaTools(ctx InteractiveContext) ([]agent.ToolDefinition, error) {
	if ctx.SubmitStateSchemaBatch == nil {
		return nil, nil
	}
	description := strings.Join([]string{
		"Before opening-turn prose only, incrementally stage this story's state templates and field structure. The model-visible input is an opening-only, structure-only contract. Do not submit Actors, initial_actor_ops, or actor_ops.",
		"The opening draft source must be exactly source={\"kind\":\"opening\",\"id\":\"opening-draft\"}. value_policy is always schema_only. covered/add/replace require the existing or target template_id, field_id, and a valid expected_type. remove requires the existing template_id, field_id, reason, and matching field removal operation.",
		"Structural requirements and template_ops use Template IDs from the state handbook, never Actor IDs. For example, story is an actor_id whose template_id is story_context. The backend normalizes only misuse that maps uniquely from an initial Actor and always stores the canonical Template ID.",
		interactive.OpeningStateSchemaFieldSelectionRules,
		"Express friendship, romance, family, mentorship, rivalry, hostility, and other relationship stages according to their story semantics; never derive them automatically from affinity.",
		"Use a covered review for a concrete field with empty template_ops only when no independent structural need exists. The tool returns accepted, rejected, and blocked per item. Retry only failed items and output opening prose only after finalized=true.",
		"A finalized receipt includes initialization_guide. auto_initialized_fields are already covered by template defaults or initial Actor values. required_state_changes lists exact actor_id, template_id, field_id, and type values that the first submit_interactive_turn must fill together. Do not use empty, unset, unknown, or pending placeholders.",
		"The draft is never written alone. Structure, prose, every initial field, and choices persist atomically only when all pass. Create Actors and set all initial values later through submit_interactive_turn.state_changes.",
	}, "\n")
	submitTool, err := agent.InferTool(
		initializeStoryStateSchemaToolName,
		description,
		func(callCtx context.Context, input openingStateSchemaBatchToolInput) (string, error) {
			result, err := ctx.SubmitStateSchemaBatch(callCtx, input.batch())
			if err != nil {
				return "", err
			}
			data, err := json.MarshalIndent(result, "", "  ")
			if err != nil {
				return "", fmt.Errorf("serialize opening state-schema batch result: %w", err)
			}
			return string(data), nil
		},
	)
	if err != nil {
		return nil, err
	}
	definedSubmitTool, err := defineTool(submitTool, interactiveStoryWorkflowDescriptor())
	if err != nil {
		return nil, err
	}
	return []agent.ToolDefinition{definedSubmitTool}, nil
}
