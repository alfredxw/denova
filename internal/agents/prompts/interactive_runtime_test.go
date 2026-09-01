package prompts

import (
	"strings"
	"testing"
)

func TestInteractiveStoryRuntimeContextDescribesStoryCheckSettings(t *testing.T) {
	enabled := InteractiveStoryRuntimeContext(InteractiveStoryPromptInput{
		ChoiceCount: 5, RuleChecksEnabled: true, CheckDifficultyShift: 1, CheckRollModifier: -2,
		StoryRuleCatalog: "rule catalog",
	})
	for _, want := range []string{
		"## Fixed-rule Checks",
		"Status: enabled",
		"difficulty shift: +1",
		"roll modifier: -2",
		"Story Rule Catalog",
	} {
		if !strings.Contains(enabled, want) {
			t.Fatalf("enabled runtime context missing %q:\n%s", want, enabled)
		}
	}

	disabled := InteractiveStoryRuntimeContext(InteractiveStoryPromptInput{
		ChoiceCount: 5, RuleChecksEnabled: false, StoryRuleCatalog: "must stay hidden",
	})
	if !strings.Contains(disabled, "Status: disabled. Do not call prepare_interactive_turn") {
		t.Fatalf("disabled runtime context lacks the tool boundary:\n%s", disabled)
	}
	if strings.Contains(disabled, "must stay hidden") {
		t.Fatalf("disabled runtime context exposed a rule catalog:\n%s", disabled)
	}
}
