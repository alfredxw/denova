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
// is already covered, requires a schema operation, or is intentionally ignored.
type ActorStateSchemaRequirementReview struct {
	// ItemID is injected by the Batch backend and links this audit record to
	// Actor value provenance. Model-supplied values are overwritten.
	ItemID      string                            `json:"item_id,omitempty" jsonschema:"-"`
	Source      ActorStateSchemaRequirementSource `json:"source"`
	Requirement string                            `json:"requirement"`
	// ValuePolicy makes Actor value handling explicit instead of treating a
	// sourced schema requirement as if it had also initialized runtime state.
	ValuePolicy  string   `json:"value_policy" jsonschema:"description=该需求的 Actor 值策略：schema_only 仅审查结构；preserve 校验并保留已有值；initialize 必须在同一 item 用字段级 actor_ops set 落值；defer 明确延后且必须说明理由"`
	ActorID      string   `json:"actor_id,omitempty" jsonschema:"description=value_policy 为 preserve、initialize 或 defer 时对应的稳定 actor_id；schema_only 时省略"`
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

// ValidateOpeningGameStateSchemaProposal enforces the Game Agent boundary:
// this tool may only define templates and fields. Actor creation and values
// belong to submit_interactive_turn.state_changes in the same atomic commit.
func ValidateOpeningGameStateSchemaProposal(base StoryDirectorActorStateSystem, trpg StoryDirectorTRPGSystem, proposal ActorStateSchemaProposal) (ActorStateSchemaProposal, ActorStateSchemaProposalPreview, error) {
	if len(proposal.Adaptation.InitialActorOps) > 0 {
		return ActorStateSchemaProposal{}, ActorStateSchemaProposalPreview{}, fmt.Errorf("开局 Game Agent 状态结构提案不能修改 initial_actors；请用 state_changes create 创建 Actor")
	}
	if len(proposal.Adaptation.ActorOps) > 0 {
		return ActorStateSchemaProposal{}, ActorStateSchemaProposalPreview{}, fmt.Errorf("开局 Game Agent 状态结构提案不能写 Actor 值；请用 submit_interactive_turn.state_changes 初始化")
	}
	for _, requirement := range proposal.Requirements {
		if strings.TrimSpace(requirement.ValuePolicy) != ActorStateSchemaValuePolicySchemaOnly {
			return ActorStateSchemaProposal{}, ActorStateSchemaProposalPreview{}, fmt.Errorf("开局 Game Agent 状态结构需求只能使用 value_policy=schema_only")
		}
	}
	return ValidateActorStateSchemaProposal(base, trpg, proposal)
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
			return fmt.Errorf("状态需求来源类型无效: %s", review.Source.Kind)
		}
		if review.Source.ID == "" || review.Requirement == "" {
			return fmt.Errorf("状态需求覆盖审查缺少来源或需求说明")
		}
		if review.Source.Kind == "lore" && !reviewedLore[review.Source.ID] {
			return fmt.Errorf("状态需求引用了未经后端确认审阅的资料: %s", review.Source.ID)
		}
		switch review.ValuePolicy {
		case ActorStateSchemaValuePolicySchemaOnly:
			if review.ActorID != "" {
				return fmt.Errorf("schema_only 状态需求不能指定 actor_id: source=%s actor=%s", review.Source.ID, review.ActorID)
			}
		case ActorStateSchemaValuePolicyPreserve, ActorStateSchemaValuePolicyInitialize, ActorStateSchemaValuePolicyDefer:
			if review.ActorID == "" {
				return fmt.Errorf("状态需求 value_policy=%s 时必须指定 actor_id: source=%s", review.ValuePolicy, review.Source.ID)
			}
			if review.ValuePolicy == ActorStateSchemaValuePolicyDefer && review.Reason == "" {
				return fmt.Errorf("延后 Actor 状态初始化必须说明理由: source=%s actor=%s", review.Source.ID, review.ActorID)
			}
		default:
			return fmt.Errorf("状态需求 value_policy 无效: %s", review.ValuePolicy)
		}
		if review.Decision == "ignored" {
			if review.ValuePolicy != ActorStateSchemaValuePolicySchemaOnly {
				return fmt.Errorf("ignored 状态需求只能使用 value_policy=schema_only: source=%s", review.Source.ID)
			}
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
