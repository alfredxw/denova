package interactive

import (
	"strings"
	"testing"
)

func TestApplyActorStateSchemaAdaptationSupportsStorySpecificDimensions(t *testing.T) {
	tests := []struct {
		name       string
		templateID string
		fields     []ActorStateField
		want       []string
	}{
		{
			name:       "romance ensemble",
			templateID: ActorStateImportantCharacterTemplateID,
			fields: []ActorStateField{
				{Name: "好感度", Type: "number", Default: float64(0), Min: floatPointer(-100), Max: floatPointer(100), Visibility: "spoiler"},
				{Name: "关系阶段", Type: "enum", Default: "陌生", Options: []string{"陌生", "熟悉", "暧昧", "恋人"}, Visibility: "spoiler"},
			},
			want: []string{"好感度", "关系阶段"},
		},
		{
			name:       "cultivation progression",
			templateID: DefaultActorID,
			fields: []ActorStateField{
				{Name: "境界", Type: "string", Default: "炼气一层", Visibility: "visible"},
				{Name: "法宝", Type: "list", Default: []any{}, Visibility: "visible"},
				{Name: "功法", Type: "list", Default: []any{}, Visibility: "visible"},
				{Name: "能力", Type: "list", Default: []any{}, Visibility: "visible"},
			},
			want: []string{"境界", "法宝", "功法", "能力"},
		},
		{
			name:       "numeric trpg character sheet",
			templateID: DefaultActorID,
			fields: []ActorStateField{
				{Name: "力量", Type: "number", Default: float64(10), Min: floatPointer(1), Max: floatPointer(20), Visibility: "visible"},
				{Name: "等级", Type: "number", Default: float64(1), Min: floatPointer(1), Visibility: "visible"},
				{Name: "生命值", Type: "number", Default: float64(10), Min: floatPointer(0), Visibility: "visible"},
				{Name: "法术位", Type: "number", Default: float64(0), Min: floatPointer(0), Visibility: "visible"},
			},
			want: []string{"力量", "等级", "生命值", "法术位"},
		},
		{
			name:       "adult relationship state",
			templateID: ActorStateImportantCharacterTemplateID,
			fields: []ActorStateField{
				{Name: "亲密边界", Type: "string", Visibility: "hidden"},
				{Name: "欲望状态", Type: "enum", Options: []string{"平静", "波动", "强烈"}, Default: "平静", Visibility: "hidden"},
				{Name: "亲密特质", Type: "list", Default: []any{}, Visibility: "hidden"},
			},
			want: []string{"亲密边界", "欲望状态", "亲密特质"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fieldOps := make([]ActorStateFieldSchemaOp, 0, len(tt.fields))
			for _, field := range tt.fields {
				fieldOps = append(fieldOps, ActorStateFieldSchemaOp{Op: "add", Field: field, Reason: tt.name})
			}
			system, record, err := ApplyActorStateSchemaAdaptation(defaultActorStateSystem(), StoryDirectorTRPGSystem{}, ActorStateSchemaAdaptation{
				Summary:     tt.name,
				TemplateOps: []ActorStateTemplateSchemaOp{{Op: "fields", TemplateID: tt.templateID, FieldOps: fieldOps}},
			})
			if err != nil {
				t.Fatalf("ApplyActorStateSchemaAdaptation failed: %v", err)
			}
			if record.FieldOps != len(tt.fields) || record.Source != "director_agent" {
				t.Fatalf("unexpected adaptation record: %#v", record)
			}
			template := actorStateTemplateByID(system, tt.templateID)
			for _, fieldID := range tt.want {
				if _, ok := actorStateFieldByID(template, fieldID); !ok {
					t.Fatalf("adapted template %s missing field %s: %#v", tt.templateID, fieldID, template.Fields)
				}
			}
		})
	}
}

func TestApplyActorStateSchemaAdaptationRejectsBrokenTRPGBinding(t *testing.T) {
	base := defaultActorStateSystem()
	index := actorStateTemplateIndex(base.Templates, DefaultActorID)
	base.Templates[index].Fields = append(base.Templates[index].Fields, ActorStateField{Name: "力量", Type: "number", Default: float64(10), Visibility: "visible"})
	trpg := StoryDirectorTRPGSystem{RuleTemplates: []RuleCheck{{
		ID: "attribute-check",
		StateBindings: []RuleStateBinding{{
			ID:              "strength-check",
			ActorTemplateID: DefaultActorID,
			Modifiers:       []RuleStateBindingModifier{{Source: "actor", FieldID: "力量", Effect: "advantage"}},
		}},
	}}}

	_, _, err := ApplyActorStateSchemaAdaptation(base, trpg, ActorStateSchemaAdaptation{TemplateOps: []ActorStateTemplateSchemaOp{{
		Op: "fields", TemplateID: DefaultActorID, FieldOps: []ActorStateFieldSchemaOp{{Op: "remove", FieldID: "力量"}},
	}}})
	if err == nil || !strings.Contains(err.Error(), "TRPG binding") || !strings.Contains(err.Error(), "力量") {
		t.Fatalf("expected broken binding error, got %v", err)
	}
}

func TestApplyActorStateSchemaAdaptationProtectsRuntimeFoundation(t *testing.T) {
	_, _, err := ApplyActorStateSchemaAdaptation(defaultActorStateSystem(), StoryDirectorTRPGSystem{}, ActorStateSchemaAdaptation{TemplateOps: []ActorStateTemplateSchemaOp{{Op: "remove", TemplateID: DefaultActorID}}})
	if err == nil || !strings.Contains(err.Error(), "不可删除") {
		t.Fatalf("expected protected template error, got %v", err)
	}
	_, _, err = ApplyActorStateSchemaAdaptation(defaultActorStateSystem(), StoryDirectorTRPGSystem{}, ActorStateSchemaAdaptation{InitialActorOps: []ActorStateInitialActorSchemaOp{{Op: "remove", ActorID: DefaultStoryContextActorID}}})
	if err == nil || !strings.Contains(err.Error(), "不可删除") {
		t.Fatalf("expected protected actor error, got %v", err)
	}
}

func TestApplyActorStateSchemaAdaptationRemovesUnusedTemplateAndInitialActorTogether(t *testing.T) {
	base := defaultActorStateSystem()
	base.InitialActors = append(base.InitialActors, ActorStateInitialActor{ID: "legacy-guide", Name: "旧向导", TemplateID: ActorStateImportantCharacterTemplateID})
	system, _, err := ApplyActorStateSchemaAdaptation(base, StoryDirectorTRPGSystem{}, ActorStateSchemaAdaptation{
		TemplateOps:     []ActorStateTemplateSchemaOp{{Op: "remove", TemplateID: ActorStateImportantCharacterTemplateID}},
		InitialActorOps: []ActorStateInitialActorSchemaOp{{Op: "remove", ActorID: "legacy-guide"}},
	})
	if err != nil {
		t.Fatalf("remove template and actor failed: %v", err)
	}
	if actorStateTemplateByID(system, ActorStateImportantCharacterTemplateID).ID != "" || actorStateInitialActorIndex(system.InitialActors, "legacy-guide") >= 0 {
		t.Fatalf("removed story-incompatible schema survived: %#v", system)
	}
}

func TestApplyActorStateSchemaAdaptationRejectsInvalidFieldContract(t *testing.T) {
	_, _, err := ApplyActorStateSchemaAdaptation(defaultActorStateSystem(), StoryDirectorTRPGSystem{}, ActorStateSchemaAdaptation{TemplateOps: []ActorStateTemplateSchemaOp{{
		Op: "fields", TemplateID: DefaultActorID, FieldOps: []ActorStateFieldSchemaOp{{Op: "add", Field: ActorStateField{Name: "经验", Type: "integer"}}},
	}}})
	if err == nil || !strings.Contains(err.Error(), "type 无效") {
		t.Fatalf("expected invalid field type error, got %v", err)
	}
}

func floatPointer(value float64) *float64 {
	return &value
}
