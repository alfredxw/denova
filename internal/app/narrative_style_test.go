package app

import (
	"strings"
	"testing"

	"denova/internal/agents/prompts"
	"denova/internal/interactive/teller"
	"denova/internal/style"
)

func TestNarrativeStyleRuntimeHonorsModeAndFallsBackToDefault(t *testing.T) {
	novaDir := t.TempDir()
	library := teller.NewLibrary(novaDir)
	create := func(id, mode string) {
		t.Helper()
		_, err := library.Create(teller.Definition{
			ID: id, Name: id, Modes: []string{mode},
			Slots: []teller.PromptSlot{{ID: "system", Target: "system", Enabled: true, Content: id}},
		})
		if err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}
	create("writing-only", style.ModeWriting)
	create("game-only", style.ModeGame)

	if got := loadWritingTeller(novaDir, "writing-only"); got.ID != "writing-only" {
		t.Fatalf("writing runtime loaded %q", got.ID)
	}
	if got := loadGameTeller(novaDir, "game-only"); got.ID != "game-only" {
		t.Fatalf("game runtime loaded %q", got.ID)
	}
	for _, got := range []teller.Definition{
		loadGameTeller(novaDir, "writing-only"),
		loadWritingTeller(novaDir, "game-only"),
		loadWritingTeller(novaDir, "missing"),
	} {
		if got.ID != style.DefaultID {
			t.Fatalf("incompatible or missing style should fall back to %q, got %q", style.DefaultID, got.ID)
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
			writing := ideStoryTellerFromInteractive(teller, nil)
			if !strings.Contains(writing.Prompt, systemRule) || !strings.Contains(writing.Prompt, turnRule) {
				t.Fatalf("writing prompt assembly omitted a narrative style slot")
			}
			gameSystem := prompts.BuildInteractiveStorySystemInstruction(interactiveStoryTellerSystemInput(teller))
			gameTurn := prompts.InteractiveStoryTurnInstruction("继续当前场景。", turnRule, "")
			if !strings.Contains(gameSystem, systemRule) || !strings.Contains(gameTurn, turnRule) {
				t.Fatalf("game prompt assembly omitted a narrative style slot")
			}
		})
	}
}
