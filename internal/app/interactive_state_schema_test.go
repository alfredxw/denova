package app

import (
	"os"
	"strings"
	"testing"

	"denova/internal/book"
	"denova/internal/interactive"
	"denova/internal/workspacepath"
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

func TestBuildStateSchemaAdaptationInstructionRejectsUnreadableLoreCatalog(t *testing.T) {
	workspace := t.TempDir()
	if err := book.NewLoreStore(workspace).Ensure(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(workspacepath.Path(workspace, "lore", "items.json"), []byte("not-json"), 0o644); err != nil {
		t.Fatal(err)
	}
	director := interactive.DefaultStoryDirector()
	req := interactive.CreateStoryRequest{Title: "损坏资料库", StoryDirectorID: director.ID, ActorState: &director.ActorState}
	if _, err := buildStateSchemaAdaptationInstruction(req, director, book.NewState(workspace)); err == nil || !strings.Contains(err.Error(), "资料库") {
		t.Fatalf("state schema review must fail explicitly when resident lore cannot be loaded: %v", err)
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

func TestBuildStateSchemaAdaptationInstructionUsesBoundedResidentLoreRoster(t *testing.T) {
	workspace := t.TempDir()
	store := book.NewLoreStore(workspace)
	if _, err := store.Create(book.LoreItemInput{
		ID: "numeric-rules", Type: "world", Name: "具体数值", Importance: "major", LoadMode: book.LoreLoadModeResident,
		BriefDescription: "定义生命、灵力与修为的数值范围。", Content: "RESIDENT_BODY_MUST_BE_READ_BY_TOOL",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(book.LoreItemInput{
		ID: "side-location", Type: "location", Name: "支线地点", Importance: "major", LoadMode: book.LoreLoadModeAuto,
		BriefDescription: "只在进入支线时读取。", Content: "AUTO_BODY",
	}); err != nil {
		t.Fatal(err)
	}
	director := interactive.DefaultStoryDirector()
	req := interactive.CreateStoryRequest{Title: "规则感知开场", StoryDirectorID: director.ID, ActorState: &director.ActorState}
	instruction, err := buildStateSchemaAdaptationInstruction(req, director, book.NewState(workspace))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"resident_lore_roster", "lore_revision", "具体数值", "numeric-rules"} {
		if !strings.Contains(instruction, want) {
			t.Fatalf("state schema instruction missing resident discovery value %q: %s", want, instruction)
		}
	}
	for _, unexpected := range []string{"RESIDENT_BODY_MUST_BE_READ_BY_TOOL", "支线地点", "AUTO_BODY"} {
		if strings.Contains(instruction, unexpected) {
			t.Fatalf("state schema instruction leaked non-discovery lore value %q: %s", unexpected, instruction)
		}
	}
	if len(instruction) > maxInteractiveStateSchemaPromptBytes {
		t.Fatalf("instruction exceeds bounded payload: %d", len(instruction))
	}
}
