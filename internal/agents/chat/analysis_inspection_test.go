package chat

import (
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	"denova/internal/agents/prompts"
)

func TestBuildInspectedContextAnalysisUsesExactMaintainedMiddlewareFinalRequest(t *testing.T) {
	maxTokens := 321
	inspection := agent.Inspection{
		Cleanup: &agent.CleanupState{ID: "cleanup-1", Revision: 4},
		Compaction: &agent.CompactionState{
			ID: "compaction-2", Revision: 2, Summary: "CHECKPOINT_EXACT",
			ReplacementFrom: 1, ReplacementTo: 5, TokenEstimate: 1400,
			Metrics: agent.CompactionMetrics{
				ProjectedTokensBefore: 9000, ProjectedTokensAfter: 1400,
				ContextWindowTokens: 12000, Threshold: .75, RecoveryBand: .55,
				SourceMessageCount: 4,
			},
		},
		ModelRequest: agent.ModelRequestInspection{
			Messages: []*agent.Message{
				agent.SystemMessage("EXACT_SYSTEM_AFTER_MIDDLEWARE"),
				agent.UserMessage("stable prefix"),
				agent.UserMessage("CHECKPOINT_EXACT"),
				agent.ToolMessage(agent.ToolResult{Status: agent.ToolResultSuccess, ModelContent: "[cleaned result]"}, "call-clean", agent.WithToolName("read")),
				agent.UserMessage("EXACT_CURRENT_TURN"),
			},
			StablePrefixMessages: 3,
			Options:              agent.Options{MaxTokens: &maxTokens, Tools: []*agent.ToolInfo{{Name: "read"}}},
		},
		ContextFragments: []agent.ContextFragment{
			{
				Source: "workspace.review.feedback", Purpose: "trusted review feedback",
				Resource: "review-feedback", Revision: "review:4", Placement: agent.ContextAuditOnly,
				Content: "EXACT_CURRENT_TURN", HardLimit: 64 << 10,
			},
			{
				Source: "hidden.context", Purpose: "removed by middleware", Resource: "hidden",
				Revision: "hidden:1", Placement: agent.ContextAuditOnly,
				Content: "NOT_IN_FINAL_REQUEST", HardLimit: 64 << 10,
			},
		},
	}
	analysis := BuildInspectedContextAnalysis(
		&config.Config{}, config.AgentKindIDE, "ide", prompts.SystemPromptComposition{}, inspection,
	)
	if analysis.SystemPrompt != "EXACT_SYSTEM_AFTER_MIDDLEWARE" {
		t.Fatalf("system prompt = %q", analysis.SystemPrompt)
	}
	joined := contextAnalysisJoinedContent(analysis.ContextMessages)
	for _, want := range []string{"stable prefix", "CHECKPOINT_EXACT", "[cleaned result]", "EXACT_CURRENT_TURN"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("exact inspected context is missing %q: %s", want, joined)
		}
	}
	if analysis.ReservedCompletionTokens != maxTokens || analysis.Compaction == nil ||
		analysis.Compaction.ID != "compaction-2" || analysis.Compaction.TokensBefore != 9000 ||
		analysis.Compaction.TokensAfter != 1400 || !analysis.CompactionActive || analysis.CompactionEpoch != 2 {
		t.Fatalf("maintenance projection = %#v analysis=%#v", analysis.Compaction, analysis)
	}
	if len(analysis.ContextParts) != 1 || analysis.ContextParts[0].Source != "workspace.review.feedback" ||
		analysis.ContextParts[0].Content != "EXACT_CURRENT_TURN" ||
		!strings.Contains(analysis.ContextParts[0].Note, "revision=review:4") {
		t.Fatalf("context provenance = %#v", analysis.ContextParts)
	}
}

func contextAnalysisJoinedContent(parts []ContextAnalysisPart) string {
	var joined strings.Builder
	for _, part := range parts {
		joined.WriteString(part.Content)
		joined.WriteByte('\n')
	}
	return joined.String()
}
