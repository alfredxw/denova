package app

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"denova/internal/book"
	"denova/internal/interactive"
	"denova/internal/workspacepath"
)

func TestBuildStateSchemaAdaptationInstructionIncludesFrozenSchemaAndCurrentActorValues(t *testing.T) {
	stateSystem := interactive.StoryDirectorActorStateSystem{
		Templates: []interactive.ActorStateTemplate{{
			ID: "protagonist", Name: "主角", Fields: []interactive.ActorStateField{
				{Name: "状态", Type: "string", Visibility: "visible"},
				{Name: "生命", Type: "number", Visibility: "visible"},
			},
		}},
		InitialActors: []interactive.ActorStateInitialActor{{ID: "protagonist", Name: "主角", TemplateID: "protagonist"}},
	}
	req := interactive.CreateStoryRequest{Title: "状态上下文", ActorState: &stateSystem}
	director := interactive.StoryDirector{ID: "director", ActorState: stateSystem}
	turn := &interactive.TurnEvent{ID: "opening-turn", BranchID: "main", User: "醒来", Narrative: "主角负伤醒来。"}
	currentState := map[string]any{"actors": map[string]any{
		"protagonist": map[string]any{
			"id": "protagonist", "name": "主角", "template_id": "protagonist", "role": "protagonist",
			"state": map[string]any{"状态": "负伤", "生命": float64(37)},
		},
	}}
	instruction, err := buildStateSchemaAdaptationInstructionAfterOpening(req, director, nil, turn, currentState)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(instruction, `"current_actor_state":{`) || strings.Contains(instruction, `"current_actor_state":"`) {
		t.Fatalf("current Actor snapshot must be a structured JSON object instead of a double-encoded string: %s", instruction)
	}
	var prompt stateSchemaAdaptationPrompt
	if err := json.Unmarshal([]byte(strings.TrimPrefix(instruction, stateSchemaAdaptationInstructionPrefix)), &prompt); err != nil {
		t.Fatalf("decode bounded state-schema prompt: %v", err)
	}
	if len(prompt.StatePreset.Templates) != 1 || len(prompt.StatePreset.Templates[0].Fields) != 2 || prompt.StatePreset.Templates[0].Fields[1].Name != "生命" {
		t.Fatalf("frozen Actor schema must be supplied to the Director: %#v", prompt.StatePreset)
	}
	if prompt.Sources.CurrentActorState == nil {
		t.Fatal("current Actor snapshot must be supplied to the Director")
	}
	actor, _ := prompt.Sources.CurrentActorState.Actors["protagonist"].(map[string]any)
	actorValues, _ := actor["state"].(map[string]any)
	if actor["template_id"] != "protagonist" || actorValues["状态"] != "负伤" || actorValues["生命"] != float64(37) {
		t.Fatalf("current Actor values must be supplied without rewriting: %#v", prompt.Sources.CurrentActorState)
	}
}

func TestBuildStateSchemaAdaptationInstructionKeepsCurrentActorStateWithinHighLimit(t *testing.T) {
	stateSystem := interactive.StoryDirectorActorStateSystem{Templates: []interactive.ActorStateTemplate{{
		ID: "character", Name: "角色", Fields: []interactive.ActorStateField{{Name: "记忆", Type: "string", Visibility: "visible"}},
	}}}
	req := interactive.CreateStoryRequest{Title: "完整状态上下文", ActorState: &stateSystem}
	director := interactive.StoryDirector{ID: "director", ActorState: stateSystem}
	turn := &interactive.TurnEvent{ID: "opening-turn", BranchID: "main", Narrative: "众人到场。"}
	largeValue := strings.Repeat("x", maxInteractiveStateSchemaCurrentActorStateBytes/2)
	actors := make(map[string]any, 30)
	for index := 0; index < 30; index++ {
		actorID := fmt.Sprintf("actor-%02d", index)
		value := "普通状态"
		if index == 29 {
			value = largeValue
		}
		actors[actorID] = map[string]any{
			"id": actorID, "name": actorID, "template_id": "character", "state": map[string]any{"记忆": value},
		}
	}
	instruction, err := buildStateSchemaAdaptationInstructionAfterOpening(req, director, nil, turn, map[string]any{"actors": actors})
	if err != nil {
		t.Fatal(err)
	}
	if len(instruction) <= maxInteractiveStateSchemaNonStatePromptBytes {
		t.Fatalf("regression fixture must exceed the non-state prompt budget: %d", len(instruction))
	}
	if len(instruction) > maxInteractiveStateSchemaTotalPromptBytes {
		t.Fatalf("instruction exceeds the declared total prompt limit: %d", len(instruction))
	}
	var prompt stateSchemaAdaptationPrompt
	if err := json.Unmarshal([]byte(strings.TrimPrefix(instruction, stateSchemaAdaptationInstructionPrefix)), &prompt); err != nil {
		t.Fatalf("decode state-schema prompt: %v", err)
	}
	if prompt.Sources.CurrentActorState == nil {
		t.Fatal("current Actor snapshot must be present")
	}
	if len(prompt.Sources.CurrentActorState.Actors) != len(actors) {
		t.Fatalf("all Actors must be included: got=%d want=%d", len(prompt.Sources.CurrentActorState.Actors), len(actors))
	}
	lastActor, _ := prompt.Sources.CurrentActorState.Actors["actor-29"].(map[string]any)
	lastState, _ := lastActor["state"].(map[string]any)
	if lastState["记忆"] != largeValue {
		t.Fatalf("large Actor value was truncated: got=%d want=%d", len(fmt.Sprint(lastState["记忆"])), len(largeValue))
	}
	if prompt.Limits["max_current_actor_state_bytes"] != maxInteractiveStateSchemaCurrentActorStateBytes || prompt.Limits["max_total_prompt_bytes"] != maxInteractiveStateSchemaTotalPromptBytes {
		t.Fatalf("prompt must declare Actor-state and total hard limits: %#v", prompt.Limits)
	}
}

func TestBuildStateSchemaAdaptationInstructionRejectsActorStateAboveHighLimitWithoutTruncation(t *testing.T) {
	stateSystem := interactive.StoryDirectorActorStateSystem{Templates: []interactive.ActorStateTemplate{{
		ID: "character", Name: "角色", Fields: []interactive.ActorStateField{{Name: "记忆", Type: "string", Visibility: "visible"}},
	}}}
	req := interactive.CreateStoryRequest{Title: "超限状态上下文", ActorState: &stateSystem}
	director := interactive.StoryDirector{ID: "director", ActorState: stateSystem}
	turn := &interactive.TurnEvent{ID: "opening-turn", BranchID: "main", Narrative: "角色到场。"}
	actors := map[string]any{"actor": map[string]any{
		"id": "actor", "name": "actor", "template_id": "character",
		"state": map[string]any{"记忆": strings.Repeat("x", maxInteractiveStateSchemaCurrentActorStateBytes)},
	}}

	instruction, err := buildStateSchemaAdaptationInstructionAfterOpening(req, director, nil, turn, map[string]any{"actors": actors})
	if err == nil || instruction != "" || !strings.Contains(err.Error(), "Current Actor state snapshot exceeds the safety limit") {
		t.Fatalf("oversized Actor state must fail explicitly without a partial prompt: instruction_bytes=%d err=%v", len(instruction), err)
	}
}

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
	if len(instruction) > maxInteractiveStateSchemaNonStatePromptBytes {
		t.Fatalf("instruction exceeds bounded payload: %d", len(instruction))
	}
	for _, want := range []string{"sources", "story_origin", "state_preset", "trpg_bindings", "max_non_state_prompt_bytes"} {
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

func TestStateSchemaTRPGSourceAllowlistMatchesVisibleBindings(t *testing.T) {
	system := interactive.StoryDirectorTRPGSystem{RuleTemplates: []interactive.RuleCheck{
		{ID: "visible", StateBindings: []interactive.RuleStateBinding{{ID: "binding", ActorTemplateID: "protagonist"}}},
		{ID: "not-in-prompt"},
	}}
	rules := compactStateSchemaAdaptationRules(system)
	ids := stateSchemaAdaptationRuleSourceIDs(rules)
	if len(rules) != 1 || rules[0].ID != "visible" || len(ids) != 1 || ids[0] != "visible" {
		t.Fatalf("TRPG source allowlist must derive only from rules visible in dynamic JSON: rules=%#v ids=%#v", rules, ids)
	}
}

func TestBuildStateSchemaAdaptationInstructionSeparatesCompleteResidentLoreFromDynamicJSON(t *testing.T) {
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
	state := book.NewState(workspace)
	workspaceSources, err := stateSchemaAdaptationWorkspaceContext(state)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(workspaceSources.ResidentLore, "RESIDENT_BODY_MUST_BE_READ_BY_TOOL") || strings.Contains(workspaceSources.ResidentLore, "AUTO_BODY") {
		t.Fatalf("stable resident context must contain every resident body and no on-demand body: %q", workspaceSources.ResidentLore)
	}
	instruction, err := buildStateSchemaAdaptationInstruction(req, director, state)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"lore_revision", "numeric-rules", `"source":"enabled resident lore bodies"`, `"complete":true`, `"max_body_bytes":1048576`, `"ids":["numeric-rules"]`} {
		if !strings.Contains(instruction, want) {
			t.Fatalf("state schema instruction missing resident discovery value %q: %s", want, instruction)
		}
	}
	for _, unexpected := range []string{"resident_lore_roster", "RESIDENT_BODY_MUST_BE_READ_BY_TOOL", "具体数值", "支线地点", "AUTO_BODY"} {
		if strings.Contains(instruction, unexpected) {
			t.Fatalf("state schema instruction leaked non-discovery lore value %q: %s", unexpected, instruction)
		}
	}
	if len(instruction) > maxInteractiveStateSchemaNonStatePromptBytes {
		t.Fatalf("instruction exceeds bounded payload: %d", len(instruction))
	}
}

func TestAssembleStateSchemaResidentLoreRejectsRevisionTOCTOU(t *testing.T) {
	reader := &stateSchemaLoreReaderStub{
		revisions: []string{"revision-before", "revision-after"},
		items:     []book.LoreItem{{ID: "rule", LoadMode: book.LoreLoadModeResident, Content: "规则正文"}},
		resident:  "## 规则\n\n规则正文",
	}
	if _, err := assembleResidentLore(reader); err == nil || !strings.Contains(err.Error(), "装配期间发生变化") {
		t.Fatalf("resident Lore assembly must reject mixed revisions: %v", err)
	}
}

func TestAssembleStateSchemaResidentLoreReturnsOneRevisionSnapshot(t *testing.T) {
	reader := &stateSchemaLoreReaderStub{
		revisions: []string{"stable-revision", "stable-revision"},
		items: []book.LoreItem{
			{ID: "resident", LoadMode: book.LoreLoadModeResident, Content: "常驻正文"},
			{ID: "empty", LoadMode: book.LoreLoadModeResident},
			{ID: "auto", LoadMode: book.LoreLoadModeAuto, Content: "按需正文"},
		},
		resident: "## 常驻\n\n常驻正文",
	}
	snapshot, err := assembleResidentLore(reader)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Revision != "stable-revision" || snapshot.Content != reader.resident || snapshot.BodyBytes != len([]byte("常驻正文")) || len(snapshot.IDs) != 1 || snapshot.IDs[0] != "resident" {
		t.Fatalf("resident Lore snapshot mixed sources: %#v", snapshot)
	}
}

type stateSchemaLoreReaderStub struct {
	revisions    []string
	revisionCall int
	items        []book.LoreItem
	resident     string
}

func (r *stateSchemaLoreReaderStub) Revision() (string, error) {
	index := r.revisionCall
	if index >= len(r.revisions) {
		index = len(r.revisions) - 1
	}
	r.revisionCall++
	return r.revisions[index], nil
}

func (r *stateSchemaLoreReaderStub) List() ([]book.LoreItem, error) {
	return append([]book.LoreItem(nil), r.items...), nil
}

func (r *stateSchemaLoreReaderStub) ResidentContextMarkdown() (string, error) {
	return r.resident, nil
}
