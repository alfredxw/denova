package interactiveapp

import (
	"testing"

	"denova/internal/interactive"
)

func TestStoryDirectorForSnapshotKeepsDisabledRuleSystemOff(t *testing.T) {
	preset := interactive.StoryDirector{
		ModuleRefs: interactive.StoryDirectorModuleRefs{RuleSystemDisabled: true},
	}
	snapshot := &interactive.ActorStateSchemaSnapshot{
		TRPGSystem: interactive.StoryDirectorTRPGSystem{
			RuleTemplates: []interactive.RuleCheck{{ID: "frozen-rule"}},
		},
	}

	effective := storyDirectorForSnapshot(preset, snapshot)
	if len(effective.TRPGSystem.RuleTemplates) != 0 {
		t.Fatalf("disabled rule system was restored from the frozen snapshot: %#v", effective.TRPGSystem)
	}
}
