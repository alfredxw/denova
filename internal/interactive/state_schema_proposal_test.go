package interactive

import (
	"strings"
	"testing"
)

func TestValidateActorStateSchemaProposalRejectsEmptyUnreviewedDiff(t *testing.T) {
	base := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{{
		ID: "protagonist", Fields: []ActorStateField{{Name: "生命", Type: "number", Default: 100}},
	}}}
	_, _, err := ValidateActorStateSchemaProposal(base, StoryDirectorTRPGSystem{}, ActorStateSchemaProposal{Summary: "无需调整"})
	if err == nil || !strings.Contains(err.Error(), "覆盖审查") {
		t.Fatalf("empty diff without a sourced coverage review must fail: %v", err)
	}
}

func TestValidateActorStateSchemaProposalRejectsGenericCoverageForNumericRule(t *testing.T) {
	base := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{{
		ID: "protagonist", Fields: []ActorStateField{{Name: "当前资源", Type: "object", Default: map[string]any{}}},
	}}}
	minValue, maxValue := 0.0, 100.0
	proposal := ActorStateSchemaProposal{
		Summary: "现有资源字段已覆盖灵力",
		Requirements: []ActorStateSchemaRequirementReview{{
			Source: ActorStateSchemaRequirementSource{Kind: "lore", ID: "具体数值"}, Requirement: "灵力必须独立按 0-100 结算",
			ExpectedType: "number", Min: &minValue, Max: &maxValue, Decision: "covered", TemplateID: "protagonist", FieldID: "当前资源",
		}},
	}
	_, _, err := ValidateActorStateSchemaProposal(base, StoryDirectorTRPGSystem{}, proposal)
	if err == nil || !strings.Contains(err.Error(), "number") {
		t.Fatalf("generic object must not cover a numeric requirement: %v", err)
	}
}

func TestValidateActorStateSchemaProposalRejectsUntypedCoverage(t *testing.T) {
	base := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{{
		ID: "protagonist", Fields: []ActorStateField{{Name: "当前资源", Type: "object", Default: map[string]any{}}},
	}}}
	proposal := ActorStateSchemaProposal{
		Summary: "宽泛资源字段已覆盖",
		Requirements: []ActorStateSchemaRequirementReview{{
			Source: ActorStateSchemaRequirementSource{Kind: "lore", ID: "具体数值"}, Requirement: "灵力需要独立结算",
			Decision: "covered", TemplateID: "protagonist", FieldID: "当前资源",
		}},
	}
	_, _, err := ValidateActorStateSchemaProposal(base, StoryDirectorTRPGSystem{}, proposal)
	if err == nil || !strings.Contains(err.Error(), "expected_type") {
		t.Fatalf("structured coverage without an expected type must fail: %v", err)
	}
}

func TestValidateActorStateSchemaProposalAcceptsRequirementAddedWithNewTemplate(t *testing.T) {
	minValue, maxValue := -100.0, 100.0
	proposal := ActorStateSchemaProposal{
		Summary: "新增关系角色模板",
		Requirements: []ActorStateSchemaRequirementReview{{
			Source: ActorStateSchemaRequirementSource{Kind: "opening", ID: "opening-turn"}, Requirement: "重要角色需要好感度",
			ExpectedType: "number", Min: &minValue, Max: &maxValue, Decision: "add", TemplateID: "important_character", FieldID: "好感度",
		}},
		Adaptation: ActorStateSchemaAdaptation{TemplateOps: []ActorStateTemplateSchemaOp{{
			Op: "add", Template: ActorStateTemplate{ID: "important_character", Name: "重要角色", Fields: []ActorStateField{{Name: "好感度", Type: "number", Default: 0, Min: &minValue, Max: &maxValue}}},
		}}},
	}
	if _, _, err := ValidateActorStateSchemaProposal(StoryDirectorActorStateSystem{}, StoryDirectorTRPGSystem{}, proposal); err != nil {
		t.Fatalf("a sourced field in a newly added template should validate: %v", err)
	}
}
