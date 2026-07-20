package interactive

import (
	"errors"
	"strings"
	"testing"
)

func TestCompileTurnStateUpdatesCreatesActorWithNameAsID(t *testing.T) {
	system := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{{
		ID:     "important_character",
		Fields: []ActorStateField{{Name: "当前状态", Type: "string", Visibility: "visible"}},
	}}}
	update := StateUpdate{
		Op:   TurnStateUpdateCreate,
		Path: formatStateUpdatePath([]string{"柳寒衣"}),
		Value: map[string]any{
			"template_id": "important_character",
			"name":        "柳寒衣",
			"state":       map[string]any{"当前状态": "负伤但清醒"},
		},
	}

	compiled, err := CompileTurnStateUpdates(system, nil, []StateUpdate{update}, TurnStateUpdateCompileOptions{})
	if err != nil {
		t.Fatalf("a new Actor should use its name directly as actor_id: %v", err)
	}
	if len(compiled.Updates) != 1 || compiled.Updates[0].Path != "/柳寒衣" {
		t.Fatalf("the Actor name ID should remain exact: %#v", compiled.Updates)
	}
	if len(compiled.ActorOps) == 0 || compiled.ActorOps[0].ActorID != "柳寒衣" {
		t.Fatalf("compiled Actor operations should preserve the name ID: %#v", compiled.ActorOps)
	}
}

func TestCompileTurnStateUpdatesPreservesStoryLanguageNamePunctuation(t *testing.T) {
	name := "奥莉薇娅·O'Neil"
	system := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{{ID: "important_character"}}}
	compiled, err := CompileTurnStateUpdates(system, nil, []StateUpdate{{
		Op:   TurnStateUpdateCreate,
		Path: formatStateUpdatePath([]string{name}),
		Value: map[string]any{
			"template_id": "important_character",
			"name":        name,
		},
	}}, TurnStateUpdateCompileOptions{})
	if err != nil {
		t.Fatalf("story-language name punctuation should remain part of the Actor ID: %v", err)
	}
	if compiled.Updates[0].Path != formatStateUpdatePath([]string{name}) {
		t.Fatalf("Actor name punctuation was not preserved: %#v", compiled.Updates)
	}
}

func TestCompileTurnStateUpdatesRejectsActorIDDifferentFromName(t *testing.T) {
	system := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{{ID: "important_character"}}}
	_, err := CompileTurnStateUpdates(system, nil, []StateUpdate{{
		Op:   TurnStateUpdateCreate,
		Path: "/liu-han-yi",
		Value: map[string]any{
			"template_id": "important_character",
			"name":        "柳寒衣",
		},
	}}, TurnStateUpdateCompileOptions{})
	var validationError *StateUpdateValidationError
	if !errors.As(err, &validationError) || validationError.Code != "actor_name_id_mismatch" {
		t.Fatalf("an Actor ID different from its name should be rejected explicitly, got %v", err)
	}
}

func TestDecodeTurnSubmissionRequiresActorNameForCreate(t *testing.T) {
	input := DecodeInteractiveTurnSubmissionInput(`{"state_changes":[{"op":"create","actor_id":"柳寒衣","template_id":"important_character"}]}`)
	if input.StateUpdates != nil || len(input.Diagnostics) != 1 {
		t.Fatalf("create without name should be rejected before compilation: %#v", input)
	}
	if !strings.Contains(input.Diagnostics[0].MessageZH, "name 必须与 actor_id 完全相同") {
		t.Fatalf("create diagnostic should explain name=ID: %#v", input.Diagnostics[0])
	}
}

func TestBuiltinStatePanelNameIDsFollowStoryLanguage(t *testing.T) {
	for _, module := range builtinActorStateModules() {
		fields := map[string]ActorStateField{}
		for _, template := range module.ActorState.Templates {
			for _, field := range template.Fields {
				fields[field.Name] = field
			}
		}
		for _, fieldName := range []string{"技能与能力", "重要物品", "当前任务", "地点记录", "势力记录"} {
			field, ok := fields[fieldName]
			if !ok {
				t.Fatalf("state preset %s is missing %s", module.ID, fieldName)
			}
			for _, phrase := range []string{"故事语言", "名称即 ID", "拼音", "slug"} {
				if !strings.Contains(field.UpdateInstruction, phrase) {
					t.Errorf("state preset %s field %s should contain %q: %s", module.ID, fieldName, phrase, field.UpdateInstruction)
				}
			}
		}
		for _, phrase := range []string{"目标 Actor 或势力的名称即 ID", "故事语言"} {
			if !strings.Contains(fields["关系"].UpdateInstruction, phrase) {
				t.Errorf("state preset %s relationship identity should contain %q: %s", module.ID, phrase, fields["关系"].UpdateInstruction)
			}
		}
		if description := fields["在场角色"].Description; !strings.Contains(description, "Actor 名称即 ID") || !strings.Contains(description, "故事语言") {
			t.Errorf("state preset %s present Actors should use name IDs: %s", module.ID, description)
		}
		for _, field := range fields {
			if strings.Contains(field.UpdateInstruction, "地点ID") || strings.Contains(field.UpdateInstruction, "任务ID") {
				t.Errorf("state preset %s field %s still asks for opaque IDs: %s", module.ID, field.Name, field.UpdateInstruction)
			}
		}
	}
}

func TestCompileTurnStateUpdatesRejectsRecordIDDifferentFromName(t *testing.T) {
	system := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{{
		ID:     "protagonist",
		Fields: []ActorStateField{{Name: "重要物品", Type: "object", Visibility: "visible"}},
	}}}
	state := map[string]any{"actors": map[string]any{"protagonist": map[string]any{
		"id": "protagonist", "template_id": "protagonist", "state": map[string]any{"重要物品": map[string]any{}},
	}}}
	_, err := CompileTurnStateUpdates(system, state, []StateUpdate{{
		Op:   TurnStateUpdateReplace,
		Path: formatStateUpdatePath([]string{"protagonist", "重要物品", "zhi-xue-yao"}),
		Value: map[string]any{
			"名称": "止血药",
			"类型": "消耗品",
		},
	}}, TurnStateUpdateCompileOptions{})
	var validationError *StateUpdateValidationError
	if !errors.As(err, &validationError) || validationError.Code != "state_record_name_id_mismatch" {
		t.Fatalf("a state panel record ID different from its name should be rejected explicitly, got %v", err)
	}
}

func TestActorStateRuntimeContextUsesSameActorNameAndID(t *testing.T) {
	context := ActorStateRuntimeContext(defaultActorStateSystem(), nil, DirectorContextMaxBytes)
	for _, phrase := range []string{
		"新建 Actor 时 actor_id 与 name 必须完全相同",
		"使用故事语言中的角色名称",
		`"actor_id": "{{new_actor_name}}"`,
		`"name": "{{new_actor_name}}"`,
	} {
		if !strings.Contains(context, phrase) {
			t.Fatalf("runtime state guide should keep Actor name and ID identical; missing %q in:\n%s", phrase, context)
		}
	}
}
