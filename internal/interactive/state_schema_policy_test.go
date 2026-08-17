package interactive

import (
	"strings"
	"testing"
)

func TestOpeningGameStateSchemaInstructionRequiresEvidenceDrivenFields(t *testing.T) {
	policy := StoryStateSchemaPolicy{Mode: StoryStateSchemaModeAdaptTemplate}
	instruction := OpeningGameStateSchemaInstruction(StoryMeta{
		StateSchemaPolicy: &policy,
		StateSchemaInitialization: &StateSchemaInitializationStatus{
			Status: StateSchemaInitializationWaitingOpening,
		},
	})
	for _, required := range []string{
		"default state system includes only broadly useful level and health fields",
		"add or replace it with a dedicated",
		"Fixed d20 resolves randomness only",
		"does not justify D&D-style state fields",
		"opening facts, loaded Lore, or the active TRPG state_binding",
	} {
		if !strings.Contains(instruction, required) {
			t.Fatalf("opening state schema instruction must contain %q: %s", required, instruction)
		}
	}
}
