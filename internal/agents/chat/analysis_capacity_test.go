package chat

import (
	"testing"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	"denova/internal/agents/prompts"
)

func TestInspectedContextAnalysisUsesCapacityAwareProfileOutputReserve(t *testing.T) {
	maxOutput := 4000
	window := 10_000
	disableToolContext := false
	cfg := &config.Config{
		OpenAIContextWindowTokens: window,
		AgentContexts: config.AgentContextSettings{IDE: config.AgentContextOverride{
			ToolResultContextEnabled: &disableToolContext,
		}},
	}
	messages := []*agent.Message{agent.UserMessage("short request")}
	analysis := BuildInspectedContextAnalysis(cfg, config.AgentKindIDE, "ide", prompts.SystemPromptComposition{}, agent.Inspection{
		ModelRequest: agent.ModelRequestInspection{
			Messages: messages,
			Options:  agent.Options{MaxTokens: &maxOutput},
		},
	})
	if analysis.ReservedCompletionTokens != 2500 || analysis.ReservedToolResultTokens != 0 {
		t.Fatalf("analysis reserves = completion:%d tools:%d, want 2500/0",
			analysis.ReservedCompletionTokens, analysis.ReservedToolResultTokens)
	}
	if analysis.ProjectedTokenEstimate != analysis.TokenEstimate+2500 {
		t.Fatalf("projected tokens = %d, want estimate %d + reserve 2500",
			analysis.ProjectedTokenEstimate, analysis.TokenEstimate)
	}
}

func TestLegacyContextAnalysisUsesCapacityAwareProfileOutputReserve(t *testing.T) {
	maxOutput := 4000
	window := 10_000
	disableToolContext := false
	cfg := &config.Config{
		OpenAIContextWindowTokens: window,
		ModelProfiles:             []config.ModelProfileSettings{{ID: "default", MaxTokens: &maxOutput}},
		AgentContexts: config.AgentContextSettings{InteractiveStory: config.AgentContextOverride{
			ToolResultContextEnabled: &disableToolContext,
		}},
	}
	usage := analyzeContextUsage(cfg, config.AgentKindInteractiveStory, "", []*agent.Message{agent.UserMessage("short request")}, 0)
	if usage.completionReserve != 2500 || usage.toolResultReserve != 0 {
		t.Fatalf("legacy analysis reserves = completion:%d tools:%d, want 2500/0",
			usage.completionReserve, usage.toolResultReserve)
	}
}
