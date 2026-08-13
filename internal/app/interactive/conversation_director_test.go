package interactiveapp

import (
	"strings"
	"testing"

	"denova/config"
	agents "denova/internal/agents"
	agentcontext "denova/internal/agents/context"
	agentcompaction "denova/internal/agents/context/compaction"
	"denova/internal/agents/prompts"
	"denova/internal/interactive"
)

func TestDirectorContextBudgetCountsExactComposedSystemInstruction(t *testing.T) {
	cfg := &config.Config{
		OpenAIContextWindowTokens: 128000,
		AgentPrompts: config.AgentPromptSettings{
			InteractiveDirector: config.AgentPromptOverride{SystemPrompt: "CUSTOM-DIRECTOR-POLICY"},
		},
	}
	stable := DirectorStableContext{Title: "resident lore", Content: "bounded stable context"}
	composition, err := prompts.ComposeInteractiveDirectorInstruction(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}

	budget, err := newDirectorContextBudget(cfg, DirectorTaskPlanUpdate, stable)
	if err != nil {
		t.Fatal(err)
	}
	emptyPrompt := prompts.InteractiveDirectorInstruction(prompts.InteractiveDirectorPromptInput{})
	overheadMessages := []*agents.Message{
		agents.SystemMessage(composition.Instruction()),
		agents.UserMessage(emptyPrompt),
		agents.UserMessage(agentcontext.StandaloneMessage(stable.Title, stable.Content, "")),
	}
	overheadTokens := agentcontext.EstimateTokens(overheadMessages, nil)
	completionReserve, toolReserve := agentcompaction.EstimateProjectionReserves(cfg, config.AgentKindInteractiveDirector, 1024)
	want := max(0, budget.thresholdTokens-overheadTokens-completionReserve-toolReserve-max(2048, budget.contextWindowTokens/100))
	if budget.initialTokens != want {
		t.Fatalf("source budget = %d, want %d from exact composition hash=%s", budget.initialTokens, want, composition.InstructionHash())
	}
	if !strings.Contains(composition.Instruction(), "Denova Runtime Contract (Non-overridable)") || !strings.Contains(composition.Instruction(), "Output Protocol (Non-overridable)") {
		t.Fatalf("test fixture must include runtime/output wrappers omitted by the former shadow prompt")
	}
}

func TestBuildDirectorInstructionPropagatesCompositionAdmissionError(t *testing.T) {
	maxTotalBytes := 1
	cfg := &config.Config{
		AgentContexts: config.AgentContextSettings{
			InteractiveDirector: config.AgentContextOverride{MaxTotalInjectedBytes: &maxTotalBytes},
		},
	}
	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "fail closed director"})
	if err != nil {
		t.Fatal(err)
	}
	conversation := NewConversation(store, t.TempDir(), workspace, story.ID, "main", "continue", story.ReplyTargetChars, cfg)

	_, err = conversation.BuildDirectorInstruction(interactive.TurnEvent{User: "continue"})
	if err == nil || !strings.Contains(err.Error(), "system prompt exceeds configured total budget") {
		t.Fatalf("composition admission error = %v, want fail-closed total-budget error", err)
	}
}
