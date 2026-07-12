package app

import (
	"strings"
	"testing"

	"denova/internal/interactive"
)

func TestBuildStateSchemaAdaptationInstructionIsSourcedAndBounded(t *testing.T) {
	director := interactive.DefaultStoryDirector()
	req := interactive.CreateStoryRequest{
		Title:           "群仙夜话",
		Origin:          strings.Repeat("修仙宗门中的成年角色关系与秘境历练。", 500),
		StoryDirectorID: director.ID,
		ActorState:      &director.ActorState,
		Opening:         interactive.StoryOpeningConfig{Mode: interactive.StoryOpeningModeCustom, CustomText: strings.Repeat("开局设定。", 1000)},
	}
	instruction, err := buildStateSchemaAdaptationInstruction(req, director, nil)
	if err != nil {
		t.Fatalf("buildStateSchemaAdaptationInstruction failed: %v", err)
	}
	if len(instruction) > maxInteractiveStateSchemaPromptBytes {
		t.Fatalf("instruction exceeds bounded payload: %d", len(instruction))
	}
	for _, want := range []string{"sources", "story_origin", "state_preset", "trpg_bindings", "max_prompt_bytes"} {
		if !strings.Contains(instruction, want) {
			t.Fatalf("instruction missing sourced section %q: %s", want, instruction)
		}
	}
}

func TestBuildStateSchemaAdaptationInstructionUsesRequestTRPGOverride(t *testing.T) {
	stateSystem := interactive.StoryDirectorActorStateSystem{Templates: []interactive.ActorStateTemplate{{
		ID:     "character",
		Fields: []interactive.ActorStateField{{Name: "敏捷", Type: "number", Default: 0}},
	}}}
	override := interactive.StoryDirectorTRPGSystem{RuleTemplates: []interactive.RuleCheck{{
		ID: "override_check",
		StateBindings: []interactive.RuleStateBinding{{
			ID:              "override_binding",
			ActorTemplateID: "character",
		}},
	}}}
	req := interactive.CreateStoryRequest{Title: "测试", ActorState: &stateSystem, TRPGSystem: &override}
	director := interactive.StoryDirector{ID: "director", TRPGSystem: interactive.StoryDirectorTRPGSystem{RuleTemplates: []interactive.RuleCheck{{
		ID: "preset_check",
		StateBindings: []interactive.RuleStateBinding{{
			ID:              "preset_binding",
			ActorTemplateID: "character",
		}},
	}}}}

	instruction, err := buildStateSchemaAdaptationInstruction(req, director, nil)
	if err != nil {
		t.Fatalf("buildStateSchemaAdaptationInstruction failed: %v", err)
	}
	if !strings.Contains(instruction, `"id":"override_binding"`) {
		t.Fatalf("instruction missing request TRPG override: %s", instruction)
	}
	if strings.Contains(instruction, `"id":"preset_binding"`) {
		t.Fatalf("instruction unexpectedly contains director TRPG binding: %s", instruction)
	}
}
