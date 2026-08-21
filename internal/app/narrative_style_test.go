package app

import (
	"strings"
	"testing"

	"denova/internal/agents/prompts"
	appagentruntime "denova/internal/app/agentruntime"
	interactiveapp "denova/internal/app/interactive"
	"denova/internal/interactive/teller"
	"denova/internal/style"
)

func TestNarrativeStyleRuntimeSharesCatalogAndFallsBackToDefault(t *testing.T) {
	novaDir := t.TempDir()
	library := teller.NewLibrary(novaDir)
	create := func(id string) {
		t.Helper()
		_, err := library.Create(teller.Definition{
			ID: id, Name: id,
			Slots: []teller.PromptSlot{{ID: "system", Target: "system", Enabled: true, Content: id}},
		})
		if err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}
	create("custom")

	if got := interactiveapp.LoadWritingTeller(novaDir, "custom"); got.ID != "custom" {
		t.Fatalf("writing runtime loaded %q", got.ID)
	}
	if got := interactiveapp.LoadGameTeller(novaDir, "custom"); got.ID != "custom" {
		t.Fatalf("game runtime loaded %q", got.ID)
	}
	for _, got := range []teller.Definition{
		interactiveapp.LoadGameTeller(novaDir, "missing"),
		interactiveapp.LoadWritingTeller(novaDir, "missing"),
	} {
		if got.ID != style.DefaultID {
			t.Fatalf("missing style should fall back to %q, got %q", style.DefaultID, got.ID)
		}
	}
}

func TestBuiltInNarrativeStylePromptsReachWritingAndGameAssemblies(t *testing.T) {
	novaDir := t.TempDir()
	library := teller.NewLibrary(novaDir)
	for _, id := range []string{"rhythm", "classic", "screenwriter", "grimdark", "direct-erotica"} {
		t.Run(id, func(t *testing.T) {
			teller, err := library.Get(id)
			if err != nil {
				t.Fatal(err)
			}
			systemRule := teller.PromptForTargets("system")
			turnRule := teller.PromptForTargets("turn_context")
			writing := appagentruntime.WritingTeller(teller, nil)
			if !strings.Contains(writing.Prompt, systemRule) || !strings.Contains(writing.Prompt, turnRule) {
				t.Fatalf("writing prompt assembly omitted a narrative style slot")
			}
			gameSystem := prompts.BuildInteractiveStorySystemInstruction(interactiveapp.StoryTellerSystemInput(teller))
			gameTurn := prompts.InteractiveStoryTurnInstruction("继续当前场景。", turnRule, "")
			if !strings.Contains(gameSystem, systemRule) || !strings.Contains(gameTurn, turnRule) {
				t.Fatalf("game prompt assembly omitted a narrative style slot")
			}
		})
	}
}
