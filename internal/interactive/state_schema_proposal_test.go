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
		Summary:         "现有资源字段已覆盖灵力",
		ReviewedLoreIDs: []string{"具体数值"},
		Requirements: []ActorStateSchemaRequirementReview{{
			Source: ActorStateSchemaRequirementSource{Kind: "lore", ID: "具体数值"}, Requirement: "灵力必须独立按 0-100 结算",
			ValuePolicy:  ActorStateSchemaValuePolicySchemaOnly,
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
		Summary:         "宽泛资源字段已覆盖",
		ReviewedLoreIDs: []string{"具体数值"},
		Requirements: []ActorStateSchemaRequirementReview{{
			Source: ActorStateSchemaRequirementSource{Kind: "lore", ID: "具体数值"}, Requirement: "灵力需要独立结算",
			ValuePolicy: ActorStateSchemaValuePolicySchemaOnly,
			Decision:    "covered", TemplateID: "protagonist", FieldID: "当前资源",
		}},
	}
	_, _, err := ValidateActorStateSchemaProposal(base, StoryDirectorTRPGSystem{}, proposal)
	if err == nil || !strings.Contains(err.Error(), "expected_type") {
		t.Fatalf("structured coverage without an expected type must fail: %v", err)
	}
}

func TestValidateActorStateSchemaProposalRejectsUnreviewedLoreAtStoreBoundary(t *testing.T) {
	base := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{{
		ID: "protagonist", Fields: []ActorStateField{{Name: "状态", Type: "string"}},
	}}}
	proposal := ActorStateSchemaProposal{Requirements: []ActorStateSchemaRequirementReview{{
		Source: ActorStateSchemaRequirementSource{Kind: "lore", ID: "model-invented"}, Requirement: "长期状态",
		ValuePolicy: ActorStateSchemaValuePolicySchemaOnly, ExpectedType: "string", Decision: "covered", TemplateID: "protagonist", FieldID: "状态",
	}}}
	if _, _, err := ValidateActorStateSchemaProposal(base, StoryDirectorTRPGSystem{}, proposal); err == nil || !strings.Contains(err.Error(), "未经后端确认审阅") {
		t.Fatalf("store validation must not promote a model-supplied Lore ID to reviewed: %v", err)
	}
}

func TestValidateActorStateSchemaProposalAcceptsRequirementAddedWithNewTemplate(t *testing.T) {
	minValue, maxValue := -100.0, 100.0
	proposal := ActorStateSchemaProposal{
		Summary: "新增关系角色模板",
		Requirements: []ActorStateSchemaRequirementReview{{
			Source: ActorStateSchemaRequirementSource{Kind: "opening", ID: "opening-turn"}, Requirement: "重要角色需要好感度",
			ValuePolicy:  ActorStateSchemaValuePolicySchemaOnly,
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

func TestValidateActorStateSchemaProposalValidatesRuntimeActorOps(t *testing.T) {
	base := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{
		{ID: DefaultActorID, Fields: []ActorStateField{{Name: "状态", Type: "string"}, {Name: "生命", Type: "number"}}},
		{ID: ActorStateStoryContextTemplateID, Fields: []ActorStateField{{Name: "地点", Type: "string"}}},
		{ID: "npc", Fields: []ActorStateField{{Name: "态度", Type: "string"}}},
	}}
	tests := []struct {
		name string
		op   ActorStateRuntimeSchemaOp
		want string
	}{
		{name: "remove protagonist", op: ActorStateRuntimeSchemaOp{Op: "remove", ActorID: DefaultActorID}, want: "基础运行时 Actor 不可删除"},
		{name: "remove story", op: ActorStateRuntimeSchemaOp{Op: "remove", ActorID: DefaultStoryContextActorID}, want: "基础运行时 Actor 不可删除"},
		{name: "missing actor payload id", op: ActorStateRuntimeSchemaOp{Op: "add", ActorID: "guide", Actor: ActorStateInitialActor{TemplateID: "npc"}}, want: "actor.id 不能为空"},
		{name: "mismatched actor id", op: ActorStateRuntimeSchemaOp{Op: "replace", ActorID: "guide", Actor: ActorStateInitialActor{ID: "other", TemplateID: "npc"}}, want: "不可改变 ID"},
		{name: "missing template", op: ActorStateRuntimeSchemaOp{Op: "add", ActorID: "guide", Actor: ActorStateInitialActor{ID: "guide", TemplateID: "missing"}}, want: "模板不存在"},
		{name: "protagonist wrong template", op: ActorStateRuntimeSchemaOp{Op: "replace", ActorID: DefaultActorID, Actor: ActorStateInitialActor{ID: DefaultActorID, TemplateID: "npc"}}, want: "主角运行时 Actor 必须使用"},
		{name: "story wrong template", op: ActorStateRuntimeSchemaOp{Op: "replace", ActorID: DefaultStoryContextActorID, Actor: ActorStateInitialActor{ID: DefaultStoryContextActorID, TemplateID: "npc"}}, want: "故事上下文运行时 Actor 必须使用"},
		{name: "unknown state field", op: ActorStateRuntimeSchemaOp{Op: "add", ActorID: "guide", Actor: ActorStateInitialActor{ID: "guide", TemplateID: "npc", State: map[string]any{"秘密": "未知"}}}, want: "字段不在模板中"},
		{name: "nil state value", op: ActorStateRuntimeSchemaOp{Op: "add", ActorID: "guide", Actor: ActorStateInitialActor{ID: "guide", TemplateID: "npc", State: map[string]any{"态度": nil}}}, want: "状态值不能为空"},
		{name: "invalid state value", op: ActorStateRuntimeSchemaOp{Op: "add", ActorID: "guide", Actor: ActorStateInitialActor{ID: "guide", TemplateID: DefaultActorID, State: map[string]any{"生命": "很多"}}}, want: "必须是 number"},
		{name: "set with whole actor payload", op: ActorStateRuntimeSchemaOp{Op: "set", ActorID: "guide", FieldID: "态度", Value: "警觉", Actor: ActorStateInitialActor{ID: "guide", TemplateID: "npc"}}, want: "字段级 set 不接受 actor 对象"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			proposal := ActorStateSchemaProposal{
				Requirements: []ActorStateSchemaRequirementReview{{
					Source: ActorStateSchemaRequirementSource{Kind: "opening", ID: "turn-1"}, Requirement: "持续追踪状态",
					ValuePolicy: ActorStateSchemaValuePolicySchemaOnly, ExpectedType: "string", Decision: "covered", TemplateID: DefaultActorID, FieldID: "状态",
				}},
				Adaptation: ActorStateSchemaAdaptation{ActorOps: []ActorStateRuntimeSchemaOp{test.op}},
			}
			if _, _, err := ValidateActorStateSchemaProposal(base, StoryDirectorTRPGSystem{}, proposal); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("runtime actor validation mismatch: err=%v want=%q", err, test.want)
			}
		})
	}
}
