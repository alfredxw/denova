package interactive

import (
	"strings"
	"testing"
)

func TestActorStateRuntimeContextProjectsActiveActorsAndCompactArchiveIndex(t *testing.T) {
	system := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{{
		ID: "opponent", Name: "敌人", Fields: []ActorStateField{{Name: "秘密状态", Type: "string"}},
	}}}
	state := map[string]any{
		actorStateRoot: map[string]any{
			"守门人": map[string]any{"id": "守门人", "name": "守门人", "template_id": "opponent", "state": map[string]any{"秘密状态": "仍在巡逻"}},
			"狼王":  map[string]any{"id": "狼王", "name": "赤瞳狼王", "template_id": "opponent", "state": map[string]any{"秘密状态": "不应进入下一轮上下文的完整遗言"}},
		},
		actorArchiveRoot: map[string]any{
			"狼王": map[string]any{"reason": "本回合已确认死亡", "source_turn_id": "turn-death"},
		},
	}

	context := ActorStateRuntimeContext(system, state, DirectorContextMaxBytes)
	for _, expected := range []string{"Actor ID：`守门人`", "## 已归档 Actor（只读索引）", "Actor ID：`狼王`", "赤瞳狼王", "本回合已确认死亡", "turn-death", "`archive`", "`restore`"} {
		if !strings.Contains(context, expected) {
			t.Fatalf("runtime context should expose active state and compact archive guidance; missing %q in:\n%s", expected, context)
		}
	}
	if strings.Contains(context, "不应进入下一轮上下文的完整遗言") {
		t.Fatalf("archived Actor field values must not enter the next model turn:\n%s", context)
	}
	if strings.Count(context, "Actor ID：`狼王`") != 1 {
		t.Fatalf("archived Actor should appear only in the compact index:\n%s", context)
	}

	projection := ActorStateRuntimeProjection(system, state)
	actors, _ := projection[actorStateRoot].(map[string]any)
	archives, _ := projection[actorArchiveRoot].([]ActorArchiveSummary)
	if len(actors) != 1 || actors["守门人"] == nil || actors["狼王"] != nil {
		t.Fatalf("Director projection should contain only active full Actors: %#v", projection)
	}
	if len(archives) != 1 || archives[0].ActorID != "狼王" || archives[0].Reason != "本回合已确认死亡" {
		t.Fatalf("Director projection should contain a compact archive index: %#v", projection)
	}
}

func TestArchivedActorsCannotParticipateInLaterRuleBindings(t *testing.T) {
	system := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{{ID: "opponent", Fields: []ActorStateField{{Name: "生命值", Type: "number"}}}}}
	state := map[string]any{
		actorStateRoot:   map[string]any{"狼王": map[string]any{"id": "狼王", "template_id": "opponent", "state": map[string]any{"生命值": float64(0)}}},
		actorArchiveRoot: map[string]any{"狼王": map[string]any{"reason": "死亡"}},
	}
	_, err := validateBindingActor(state, system, "狼王", "opponent", "target")
	if err == nil || !strings.Contains(err.Error(), "已归档") {
		t.Fatalf("TRPG actor and target validation must reject archived Actors, got %v", err)
	}
}

func TestStoryHistoryDescribesArchiveAndRestoreSemantically(t *testing.T) {
	changes := storyHistoryStateChanges(&StateDelta{Ops: []StateOp{
		{Op: "set", Path: "actor_archives.狼王", Value: map[string]any{"reason": "确认死亡", "source_turn_id": "turn-death"}, Reason: "确认死亡"},
		{Op: "unset", Path: "actor_archives.斥候", Reason: "确认幸存"},
	}})
	if len(changes) != 2 || !strings.Contains(changes[0], "archive /狼王") || !strings.Contains(changes[0], "确认死亡") || !strings.Contains(changes[1], "restore /斥候") || !strings.Contains(changes[1], "确认幸存") {
		t.Fatalf("history should use Actor lifecycle vocabulary instead of raw overlay paths: %#v", changes)
	}
}
