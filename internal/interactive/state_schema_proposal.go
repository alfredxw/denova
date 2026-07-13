package interactive

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const maxActorStateSchemaRequirementReviews = 64

const (
	StateSchemaLoreReadMaxItemsPerCall = 4
	StateSchemaLoreReadMaxResultBytes  = DirectorContextMaxBytes
	StateSchemaLoreReadMaxTotalBytes   = 2 * StateSchemaLoreReadMaxResultBytes
)

// ActorStateSchemaProposal is the Director-owned, backend-validated review
// submitted after the opening or during an explicit later re-review.
type ActorStateSchemaProposal struct {
	Summary      string                              `json:"summary,omitempty"`
	Requirements []ActorStateSchemaRequirementReview `json:"requirements,omitempty"`
	Adaptation   ActorStateSchemaAdaptation          `json:"adaptation"`
	// ReviewedLoreIDs is derived from successful read_lore_items results rather
	// than accepted from the model.
	ReviewedLoreIDs []string `json:"-"`
	// SourceLoreRevision is captured by the app, not supplied by the model.
	// It records which lore catalog the Director reviewed.
	SourceLoreRevision string `json:"-"`
}

// ActorStateSchemaRequirementSource identifies the bounded evidence used for
// one long-lived state requirement.
type ActorStateSchemaRequirementSource struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// ActorStateSchemaRequirementReview explains whether one sourced requirement
// is already covered, requires a schema operation, or is intentionally ignored.
type ActorStateSchemaRequirementReview struct {
	Source       ActorStateSchemaRequirementSource `json:"source"`
	Requirement  string                            `json:"requirement"`
	ExpectedType string                            `json:"expected_type,omitempty"`
	Min          *float64                          `json:"min,omitempty"`
	Max          *float64                          `json:"max,omitempty"`
	Decision     string                            `json:"decision"`
	TemplateID   string                            `json:"template_id,omitempty"`
	FieldID      string                            `json:"field_id,omitempty"`
	Reason       string                            `json:"reason,omitempty"`
}

// ActorStateSchemaProposalPreview is returned to the Director after validating
// a staged proposal. Applying it remains the Store's responsibility.
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
	proposal.Summary = trimBytes(proposal.Summary, maxTurnBriefTextBytes)
	if len(proposal.Requirements) == 0 {
		return ActorStateSchemaProposal{}, ActorStateSchemaProposalPreview{}, fmt.Errorf("状态结构提案缺少来源化覆盖审查")
	}
	if len(proposal.Requirements) > maxActorStateSchemaRequirementReviews {
		return ActorStateSchemaProposal{}, ActorStateSchemaProposalPreview{}, fmt.Errorf("状态结构需求审查过多: %d > %d", len(proposal.Requirements), maxActorStateSchemaRequirementReviews)
	}
	data, err := json.Marshal(proposal.Adaptation)
	if err != nil {
		return ActorStateSchemaProposal{}, ActorStateSchemaProposalPreview{}, fmt.Errorf("序列化状态结构提案失败: %w", err)
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
	if err := validateActorStateSchemaRequirementReviews(&proposal, targetSystem); err != nil {
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

func validateActorStateSchemaRequirementReviews(proposal *ActorStateSchemaProposal, target StoryDirectorActorStateSystem) error {
	if proposal == nil {
		return fmt.Errorf("状态结构提案不存在")
	}
	reviewedLore := map[string]bool{}
	for _, id := range proposal.ReviewedLoreIDs {
		if id = strings.TrimSpace(id); id != "" {
			reviewedLore[id] = true
		}
	}
	for index := range proposal.Requirements {
		review := &proposal.Requirements[index]
		review.Source.Kind = strings.TrimSpace(review.Source.Kind)
		review.Source.ID = strings.TrimSpace(review.Source.ID)
		review.Requirement = trimBytes(review.Requirement, maxTurnBriefTextBytes)
		review.ExpectedType = strings.TrimSpace(review.ExpectedType)
		review.Decision = strings.TrimSpace(review.Decision)
		review.TemplateID = normalizeActorStateID(review.TemplateID)
		review.FieldID = normalizeActorStateFieldName(review.FieldID)
		review.Reason = trimBytes(review.Reason, maxTurnBriefTextBytes)
		switch review.Source.Kind {
		case "lore", "opening", "turn_result", "trpg":
		default:
			return fmt.Errorf("状态需求来源类型无效: %s", review.Source.Kind)
		}
		if review.Source.ID == "" || review.Requirement == "" {
			return fmt.Errorf("状态需求覆盖审查缺少来源或需求说明")
		}
		if review.Source.Kind == "lore" {
			reviewedLore[review.Source.ID] = true
		}
		if review.Decision == "ignored" {
			if review.Reason == "" {
				return fmt.Errorf("忽略状态需求必须说明理由: source=%s", review.Source.ID)
			}
			continue
		}
		switch review.Decision {
		case "covered", "add", "replace":
		default:
			return fmt.Errorf("状态需求覆盖决策无效: %s", review.Decision)
		}
		if review.ExpectedType == "" {
			return fmt.Errorf("结构化状态需求必须声明 expected_type: source=%s", review.Source.ID)
		}
		switch review.ExpectedType {
		case "number", "string", "bool", "enum", "object", "list":
		default:
			return fmt.Errorf("状态需求 expected_type 无效: %s", review.ExpectedType)
		}
		if review.TemplateID == "" || review.FieldID == "" {
			return fmt.Errorf("状态需求覆盖目标不完整: source=%s", review.Source.ID)
		}
		template := actorStateTemplateByID(target, review.TemplateID)
		field, ok := actorStateFieldByID(template, review.FieldID)
		if !ok {
			return fmt.Errorf("状态需求覆盖字段不存在: template=%s field=%s", review.TemplateID, review.FieldID)
		}
		if review.ExpectedType != "" && field.Type != review.ExpectedType {
			return fmt.Errorf("状态需求字段类型不匹配: template=%s field=%s expected=%s actual=%s", review.TemplateID, review.FieldID, review.ExpectedType, field.Type)
		}
		if review.Min != nil && (field.Min == nil || *field.Min != *review.Min) {
			return fmt.Errorf("状态需求字段 min 不匹配: template=%s field=%s", review.TemplateID, review.FieldID)
		}
		if review.Max != nil && (field.Max == nil || *field.Max != *review.Max) {
			return fmt.Errorf("状态需求字段 max 不匹配: template=%s field=%s", review.TemplateID, review.FieldID)
		}
		if review.Decision != "covered" && !actorStateSchemaAdaptationHasFieldDecision(proposal.Adaptation, review.Decision, review.TemplateID, review.FieldID) {
			return fmt.Errorf("状态需求决策缺少对应 schema 操作: decision=%s template=%s field=%s", review.Decision, review.TemplateID, review.FieldID)
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
			if fieldOp.Op == decision && normalizeActorStateFieldName(fieldOp.Field.Name) == fieldID {
				return true
			}
		}
	}
	return false
}
