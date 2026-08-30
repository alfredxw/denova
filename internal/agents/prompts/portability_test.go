package prompts

import (
	"strings"
	"testing"

	"denova/config"
	"denova/internal/book"
)

func TestProjectSystemPromptsAreStableAcrossDataDirectoryRoots(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	cfgA := &config.Config{ProjectID: "project-portable", Workspace: rootA}
	cfgB := &config.Config{ProjectID: "project-portable", Workspace: rootB}
	stateA := book.NewState(rootA)
	stateB := book.NewState(rootB)

	assertPortable := func(name string, left, right SystemPromptComposition, leftErr, rightErr error) {
		t.Helper()
		if leftErr != nil {
			t.Fatalf("compose %s under first root: %v", name, leftErr)
		}
		if rightErr != nil {
			t.Fatalf("compose %s under second root: %v", name, rightErr)
		}
		if left.Instruction() != right.Instruction() {
			t.Fatalf("%s instruction changed after moving the data directory", name)
		}
		if strings.Contains(left.Instruction(), rootA) || strings.Contains(right.Instruction(), rootB) {
			t.Fatalf("%s instruction exposed an absolute runtime root", name)
		}
	}

	ideA, ideAErr := ComposeInstruction(cfgA, stateA, IDEStoryTeller{})
	ideB, ideBErr := ComposeInstruction(cfgB, stateB, IDEStoryTeller{})
	assertPortable("IDE", ideA, ideB, ideAErr, ideBErr)

	gameA, gameAErr := ComposeInteractiveStoryInstruction(cfgA, stateA, InteractiveStorySystemInstructionInput{})
	gameB, gameBErr := ComposeInteractiveStoryInstruction(cfgB, stateB, InteractiveStorySystemInstructionInput{})
	assertPortable("Game", gameA, gameB, gameAErr, gameBErr)

}

func TestStandaloneInteractiveInstructionOmitsRuntimeRoot(t *testing.T) {
	root := t.TempDir()
	instruction := BuildInteractiveStorySystemInstruction(InteractiveStorySystemInstructionInput{Workspace: root})
	if strings.Contains(instruction, root) {
		t.Fatalf("interactive instruction exposed runtime root %q", root)
	}
}
