package agents

import (
	"testing"

	"denova/config"
	"denova/internal/agents/prompts"
)

func mustTestPromptComposition(t *testing.T, agentKind, instruction string) prompts.SystemPromptComposition {
	t.Helper()
	composition, err := prompts.ComposeBuiltinSystemInstruction(
		&config.Config{}, agentKind, "test", "", "test_instruction",
		"Test instruction", "provide the instruction required by this test", instruction,
	)
	if err != nil {
		t.Fatal(err)
	}
	return composition
}
