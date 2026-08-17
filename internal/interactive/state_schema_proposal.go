package interactive

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const maxActorStateSchemaRequirementReviews = 64

const (
	ActorStateSchemaValuePolicySchemaOnly = "schema_only"
	ActorStateSchemaValuePolicyPreserve   = "preserve"
	ActorStateSchemaValuePolicyInitialize = "initialize"
	ActorStateSchemaValuePolicyDefer      = "defer"
)

// ActorStateSchemaProposal is the backend-validated, run-local opening schema
// draft produced by the foreground Game Agent.
type ActorStateSchemaProposal struct {
	Summary      string                              `json:"summary,omitempty"`
	Requirements []ActorStateSchemaRequirementReview `json:"requirements,omitempty"`
	Adaptation   ActorStateSchemaAdaptation          `json:"adaptation"`
	// ReviewedLoreIDs is derived from successful read_lore_items results rather
	// than accepted from the model.
	ReviewedLoreIDs []string `json:"-"`
	// SourceLoreRevision is captured by the app, not supplied by the model.
	// It records which lore catalog the opening Game Agent reviewed.
	SourceLoreRevision string `json:"-"`
}

// ActorStateSchemaRequirementSource identifies the bounded evidence used for
// one long-lived state requirement.
type ActorStateSchemaRequirementSource struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// ActorStateSchemaRequirementReview explains whether one sourced requirement
// is covered, changes the schema, removes inherited structure, or is ignored.
type ActorStateSchemaRequirementReview struct {
	// ItemID is injected by the Batch backend and links this audit record to
	// Actor value provenance. Model-supplied values are overwritten.
	ItemID      string                            `json:"item_id,omitempty" jsonschema:"-"`
	Source      ActorStateSchemaRequirementSource `json:"source"`
	Requirement string                            `json:"requirement"`
	// ValuePolicy makes Actor value handling explicit instead of treating a
	// sourced schema requirement as if it had also initialized runtime state.
	ValuePolicy  string   `json:"value_policy" jsonschema:"description=Actor value policy for this requirement: schema_only reviews structure only; preserve validates and keeps an existing value; initialize must set the field through actor_ops in the same item; defer postpones explicitly and requires a reason"`
	ActorID      string   `json:"actor_id,omitempty" jsonschema:"description=Stable actor_id for preserve, initialize, or defer; omit for schema_only"`
	ExpectedType string   `json:"expected_type,omitempty"`
	Min          *float64 `json:"min,omitempty"`
	Max          *float64 `json:"max,omitempty"`
	Decision     string   `json:"decision"`
	TemplateID   string   `json:"template_id,omitempty"`
	FieldID      string   `json:"field_id,omitempty"`
	Reason       string   `json:"reason,omitempty"`
}

// ActorStateSchemaProposalPreview describes a validated, run-local opening
// draft. Persisting it remains the Store's responsibility.
type ActorStateSchemaProposalPreview struct {
	Summary         string `json:"summary,omitempty"`
	TemplateOps     int    `json:"template_ops,omitempty"`
	FieldOps        int    `json:"field_ops,omitempty"`
	InitialActorOps int    `json:"initial_actor_ops,omitempty"`
	ActorOps        int    `json:"actor_ops,omitempty"`
}

// ValidateActorStateSchemaProposal normalizes the model-facing proposal and
// verifies that its schema diff can produce a valid frozen story contract.
func ValidateActorStateSchemaProposal(base StoryDirectorActorStateSystem, trpg StoryDirectorTRPGSystem, proposal ActorStateSchemaProposal) (ActorStateSchemaProposal, ActorStateSchemaProposalPreview, error) {
	proposal.Summary = trimBytes(proposal.Summary, maxInteractiveTextBytes)
	if len(proposal.Requirements) == 0 {
		return ActorStateSchemaProposal{}, ActorStateSchemaProposalPreview{}, fmt.Errorf("state schema proposal has no sourced coverage review")
	}
	if len(proposal.Requirements) > maxActorStateSchemaRequirementReviews {
		return ActorStateSchemaProposal{}, ActorStateSchemaProposalPreview{}, fmt.Errorf("too many state schema requirement reviews: %d > %d", len(proposal.Requirements), maxActorStateSchemaRequirementReviews)
	}
	data, err := json.Marshal(proposal.Adaptation)
	if err != nil {
		return ActorStateSchemaProposal{}, ActorStateSchemaProposalPreview{}, fmt.Errorf("serialize state schema proposal: %w", err)
	}
	adaptation, err := ParseActorStateSchemaAdaptation(string(data))
	if err != nil {
		return ActorStateSchemaProposal{}, ActorStateSchemaProposalPreview{}, err
	}
	if strings.TrimSpace(adaptation.Summary) == "" {
		adaptation.Summary = proposal.Summary
	}
	proposal.Adaptation = adaptation
	targetSystem, _, err := ApplyActorStateSchemaAdaptation(base, trpg, adaptation)
	if err != nil {
		return ActorStateSchemaProposal{}, ActorStateSchemaProposalPreview{}, err
	}
	if err := validateActorStateSchemaRequirementReviews(&proposal, base, targetSystem); err != nil {
		return ActorStateSchemaProposal{}, ActorStateSchemaProposalPreview{}, err
	}
	fieldOps := 0
	for _, op := range adaptation.TemplateOps {
		fieldOps += len(op.FieldOps)
	}
	return proposal, ActorStateSchemaProposalPreview{
		Summary:         firstNonEmptyString(proposal.Summary, adaptation.Summary),
		TemplateOps:     len(adaptation.TemplateOps),
		FieldOps:        fieldOps,
		InitialActorOps: len(adaptation.InitialActorOps),
		ActorOps:        len(adaptation.ActorOps),
	}, nil
}

// ValidateOpeningGameStateSchemaProposal enforces the Game Agent boundary:
// this tool may only define templates and fields. Actor creation and values
// belong to submit_interactive_turn.state_changes in the same atomic commit.
func ValidateOpeningGameStateSchemaProposal(base StoryDirectorActorStateSystem, trpg StoryDirectorTRPGSystem, proposal ActorStateSchemaProposal) (ActorStateSchemaProposal, ActorStateSchemaProposalPreview, error) {
	if len(proposal.Adaptation.InitialActorOps) > 0 {
		return ActorStateSchemaProposal{}, ActorStateSchemaProposalPreview{}, fmt.Errorf("the opening Game Agent schema proposal cannot modify initial_actors; create Actors through state_changes")
	}
	if len(proposal.Adaptation.ActorOps) > 0 {
		return ActorStateSchemaProposal{}, ActorStateSchemaProposalPreview{}, fmt.Errorf("the opening Game Agent schema proposal cannot write Actor values; initialize them through submit_interactive_turn.state_changes")
	}
	for _, requirement := range proposal.Requirements {
		if strings.TrimSpace(requirement.ValuePolicy) != ActorStateSchemaValuePolicySchemaOnly {
			return ActorStateSchemaProposal{}, ActorStateSchemaProposalPreview{}, fmt.Errorf("opening Game Agent schema requirements must use value_policy=schema_only")
		}
	}
	return ValidateActorStateSchemaProposal(base, trpg, proposal)
}

func validateActorStateSchemaRequirementReviews(proposal *ActorStateSchemaProposal, base, target StoryDirectorActorStateSystem) error {
	if proposal == nil {
		return fmt.Errorf("state schema proposal is missing")
	}
	reviewedLore := map[string]bool{}
	for _, id := range proposal.ReviewedLoreIDs {
		if id = strings.TrimSpace(id); id != "" {
			reviewedLore[id] = true
		}
	}
	for index := range proposal.Requirements {
		review := &proposal.Requirements[index]
		review.ItemID = strings.TrimSpace(review.ItemID)
		review.Source.Kind = strings.TrimSpace(review.Source.Kind)
		review.Source.ID = strings.TrimSpace(review.Source.ID)
		review.Requirement = trimBytes(review.Requirement, maxInteractiveTextBytes)
		review.ValuePolicy = strings.TrimSpace(review.ValuePolicy)
		review.ActorID = normalizeStatePanelActorID(review.ActorID)
		review.ExpectedType = strings.TrimSpace(review.ExpectedType)
		review.Decision = strings.TrimSpace(review.Decision)
		review.TemplateID = normalizeActorStateID(review.TemplateID)
		review.FieldID = normalizeActorStateFieldName(review.FieldID)
		review.Reason = trimBytes(review.Reason, maxInteractiveTextBytes)
		switch review.Source.Kind {
		case "lore", "opening", "turn_result", "trpg":
		default:
			return fmt.Errorf("invalid state requirement source kind: %s", review.Source.Kind)
		}
		if review.Source.ID == "" || review.Requirement == "" {
			return fmt.Errorf("state requirement coverage review is missing its source or requirement")
		}
		if review.Source.Kind == "lore" && !reviewedLore[review.Source.ID] {
			return fmt.Errorf("state requirement references lore not confirmed as reviewed by the backend: %s", review.Source.ID)
		}
		switch review.ValuePolicy {
		case ActorStateSchemaValuePolicySchemaOnly:
			if review.ActorID != "" {
				return fmt.Errorf("schema_only state requirements cannot specify actor_id: source=%s actor=%s", review.Source.ID, review.ActorID)
			}
		case ActorStateSchemaValuePolicyPreserve, ActorStateSchemaValuePolicyInitialize, ActorStateSchemaValuePolicyDefer:
			if review.ActorID == "" {
				return fmt.Errorf("state requirement with value_policy=%s must specify actor_id: source=%s", review.ValuePolicy, review.Source.ID)
			}
			if review.ValuePolicy == ActorStateSchemaValuePolicyDefer && review.Reason == "" {
				return fmt.Errorf("deferred Actor state initialization requires a reason: source=%s actor=%s", review.Source.ID, review.ActorID)
			}
		default:
			return fmt.Errorf("invalid state requirement value_policy: %s", review.ValuePolicy)
		}
		if review.Decision == "ignored" {
			if review.ValuePolicy != ActorStateSchemaValuePolicySchemaOnly {
				return fmt.Errorf("ignored state requirements must use value_policy=schema_only: source=%s", review.Source.ID)
			}
			if review.Reason == "" {
				return fmt.Errorf("ignored state requirement requires a reason: source=%s", review.Source.ID)
			}
			continue
		}
		if review.Decision == "remove" {
			if review.ValuePolicy != ActorStateSchemaValuePolicySchemaOnly {
				return fmt.Errorf("removed state requirements must use value_policy=schema_only: source=%s", review.Source.ID)
			}
			if review.TemplateID == "" || review.FieldID == "" {
				return fmt.Errorf("state field removal target is incomplete: source=%s", review.Source.ID)
			}
			if review.Reason == "" {
				return fmt.Errorf("state field removal requires a reason: source=%s", review.Source.ID)
			}
			template := actorStateTemplateByID(base, review.TemplateID)
			field, ok := actorStateFieldByID(template, review.FieldID)
			if !ok {
				return fmt.Errorf("removal review references a missing original state field: template=%s field=%s", review.TemplateID, review.FieldID)
			}
			if review.ExpectedType != "" && field.Type != review.ExpectedType {
				return fmt.Errorf("removal review field type mismatch: template=%s field=%s expected=%s actual=%s", review.TemplateID, review.FieldID, review.ExpectedType, field.Type)
			}
			if !actorStateSchemaAdaptationHasFieldDecision(proposal.Adaptation, review.Decision, review.TemplateID, review.FieldID) {
				return fmt.Errorf("removed state requirement has no matching schema operation: template=%s field=%s", review.TemplateID, review.FieldID)
			}
			continue
		}
		switch review.Decision {
		case "covered", "add", "replace":
		default:
			return fmt.Errorf("invalid state requirement coverage decision: %s", review.Decision)
		}
		if review.ExpectedType == "" {
			return fmt.Errorf("structured state requirement must declare expected_type: source=%s", review.Source.ID)
		}
		switch review.ExpectedType {
		case "number", "string", "bool", "enum", "object", "list":
		default:
			return fmt.Errorf("invalid state requirement expected_type: %s", review.ExpectedType)
		}
		if review.TemplateID == "" || review.FieldID == "" {
			return fmt.Errorf("state requirement coverage target is incomplete: source=%s", review.Source.ID)
		}
		template := actorStateTemplateByID(target, review.TemplateID)
		field, ok := actorStateFieldByID(template, review.FieldID)
		if !ok {
			return fmt.Errorf("state requirement coverage field does not exist: template=%s field=%s", review.TemplateID, review.FieldID)
		}
		if review.ExpectedType != "" && field.Type != review.ExpectedType {
			return fmt.Errorf("state requirement field type mismatch: template=%s field=%s expected=%s actual=%s", review.TemplateID, review.FieldID, review.ExpectedType, field.Type)
		}
		if review.Min != nil && (field.Min == nil || *field.Min != *review.Min) {
			return fmt.Errorf("state requirement field min mismatch: template=%s field=%s", review.TemplateID, review.FieldID)
		}
		if review.Max != nil && (field.Max == nil || *field.Max != *review.Max) {
			return fmt.Errorf("state requirement field max mismatch: template=%s field=%s", review.TemplateID, review.FieldID)
		}
		if review.Decision != "covered" && !actorStateSchemaAdaptationHasFieldDecision(proposal.Adaptation, review.Decision, review.TemplateID, review.FieldID) {
			return fmt.Errorf("state requirement decision has no matching schema operation: decision=%s template=%s field=%s", review.Decision, review.TemplateID, review.FieldID)
		}
	}
	proposal.ReviewedLoreIDs = proposal.ReviewedLoreIDs[:0]
	for id := range reviewedLore {
		proposal.ReviewedLoreIDs = append(proposal.ReviewedLoreIDs, id)
	}
	sort.Strings(proposal.ReviewedLoreIDs)
	return nil
}

func actorStateSchemaAdaptationHasFieldDecision(adaptation ActorStateSchemaAdaptation, decision, templateID, fieldID string) bool {
	for _, templateOp := range adaptation.TemplateOps {
		if decision == "add" && templateOp.Op == "add" && normalizeActorStateID(templateOp.Template.ID) == templateID {
			for _, field := range templateOp.Template.Fields {
				if normalizeActorStateFieldName(field.Name) == fieldID {
					return true
				}
			}
		}
		if templateOp.Op != "fields" || normalizeActorStateID(templateOp.TemplateID) != templateID {
			continue
		}
		for _, fieldOp := range templateOp.FieldOps {
			targetFieldID := actorStateFieldID(fieldOp.Field)
			if fieldOp.Op == "remove" {
				targetFieldID = fieldOp.FieldID
			}
			if fieldOp.Op == decision && normalizeActorStateFieldName(targetFieldID) == fieldID {
				return true
			}
		}
	}
	return false
}
