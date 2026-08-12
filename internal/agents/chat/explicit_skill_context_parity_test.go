package chat

import (
	"context"
	"strings"
	"testing"

	"denova/config"
	agentcontext "denova/internal/agents/context"
	"denova/internal/agents/prompts"
	"denova/internal/agents/session"
	novaskills "denova/internal/agents/skills"

	agent "github.com/alfredxw/denova/agent"
)

type explicitSkillParityConversation struct {
	budget      agentcontext.Budget
	invocations []novaskills.Invocation
}

func (conversation explicitSkillParityConversation) ResolveExplicitSkills(context.Context, string) ([]novaskills.Invocation, error) {
	return append([]novaskills.Invocation(nil), conversation.invocations...), nil
}

func (conversation explicitSkillParityConversation) AssembleModelContext(
	ctx context.Context,
	_ string,
	input agentcontext.ModelContextInput,
) (agentcontext.ModelContextResult, error) {
	assembled, err := agentcontext.NewAssembler(input.Budget).Assemble(ctx, agentcontext.AssembleRequest{
		Messages: []*agent.Message{agent.UserMessage(input.UserMessage)}, Fragments: input.Fragments,
	})
	return agentcontext.ModelContextResult{Messages: assembled.Messages, Context: assembled}, err
}

func (explicitSkillParityConversation) AppendAssistant(string) error                 { return nil }
func (explicitSkillParityConversation) MarkInterrupted(string, string, string) error { return nil }
func (explicitSkillParityConversation) PendingInterruption() *session.Interruption   { return nil }
func (explicitSkillParityConversation) ResolveInterruption(string) error             { return nil }

func (conversation explicitSkillParityConversation) ModelContextBudget() agentcontext.Budget {
	return conversation.budget
}

func TestExplicitSkillFailsClosedWhenTheTurnContextCannotFitIt(t *testing.T) {
	budget := agentcontext.DefaultBudget()
	budget.MaxTotalBytes = 1024
	conversation := explicitSkillParityConversation{
		budget: budget,
		invocations: []novaskills.Invocation{{
			Name: "alpha", Instructions: "# Skill: alpha\n\n" + strings.Repeat("ALPHA_BODY ", 200),
		}},
	}
	_, err := prepareTurnContext(context.Background(), turnContextPreparationInput{
		Conversation: conversation, Request: ChatRequest{Message: "请使用 /alpha"},
	})
	if err == nil || !strings.Contains(err.Error(), "source=turn.skill.explicit") {
		t.Fatalf("explicit Skill budget error=%v", err)
	}
}

func TestIDEContextAnalysisIncludesTheSameExplicitSkillBodiesAsRuntime(t *testing.T) {
	invocations := []novaskills.Invocation{
		{Name: "alpha", Instructions: "# Skill: alpha\n\nALPHA_ANALYSIS_BODY"},
		{Name: "beta", Instructions: "# Skill: beta\n\nBETA_ANALYSIS_BODY"},
	}
	conversation := explicitSkillParityConversation{budget: agentcontext.DefaultBudget(), invocations: invocations}
	request := ChatRequest{Message: "检查 /alpha，然后按 /beta 复核。"}
	prepared, err := prepareTurnContext(context.Background(), turnContextPreparationInput{
		Conversation: conversation, Request: request,
	})
	if err != nil {
		t.Fatal(err)
	}
	composition, err := prompts.ComposeInstruction(&config.Config{}, nil, prompts.IDEStoryTeller{})
	if err != nil {
		t.Fatal(err)
	}
	analysis := BuildInspectedContextAnalysis(&config.Config{}, config.AgentKindIDE, "ide", composition, agent.Inspection{
		ModelRequest: agent.ModelRequestInspection{Messages: append(
			[]*agent.Message{agent.SystemMessage(composition.Instruction())}, prepared.ModelContext.Messages...,
		)},
	})
	runtimeFinal := lastUserMessageContent(prepared.ModelContext.Messages)
	analysisFinal := analysis.ContextMessages[len(analysis.ContextMessages)-1].Content
	for _, want := range []string{"# Skill: alpha", "ALPHA_ANALYSIS_BODY", "# Skill: beta", "BETA_ANALYSIS_BODY"} {
		if !strings.Contains(runtimeFinal, want) || !strings.Contains(analysisFinal, want) {
			t.Fatalf("runtime/analysis explicit Skill mismatch for %q\nruntime=%s\nanalysis=%s", want, runtimeFinal, analysisFinal)
		}
	}
}
