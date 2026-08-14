package compaction_test

import (
	"context"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/compaction"
	"github.com/alfredxw/denova/agent/compaction/compactiontest"
)

func TestStandardManagerContract(t *testing.T) {
	compactiontest.RunManagerContract(t, func(t testing.TB) agent.CompactionManager {
		manager := compaction.Standard(compaction.StandardConfig{
			Summarizer: compaction.SummarizerFunc{
				Capability: agent.CapabilityIdentity{Kind: "compaction.contract-summary", Version: 1},
				Func: func(context.Context, compaction.SummaryRequest) (compaction.Summary, error) {
					return compaction.Summary{Content: "contract summary", TokenEstimate: 4}, nil
				},
			},
			HardLimitBytes: 8 << 20, SummaryLimitBytes: 256 << 10,
		})
		if err := manager.(agent.DefinitionInitializer).InitializeDefinition(context.Background()); err != nil {
			t.Fatal(err)
		}
		return manager
	})
}

func TestStandardCalibratesPlanFromExactPreviousProviderUsage(t *testing.T) {
	manager := compaction.Standard(compaction.StandardConfig{
		Summarizer: compaction.SummarizerFunc{
			Capability: agent.CapabilityIdentity{Kind: "compaction.calibration-summary", Version: 1},
			Func: func(context.Context, compaction.SummaryRequest) (compaction.Summary, error) {
				return compaction.Summary{Content: "summary", TokenEstimate: 2}, nil
			},
		},
		TriggerBytes: 1024, KeepRecentBytes: 128, HardLimitBytes: 8 << 20, SummaryLimitBytes: 256 << 10,
		ContextWindowTokens: 10_000, TriggerRatio: .85, RecoveryBand: .8,
	})
	previousPrompt := []*agent.Message{agent.UserMessage(strings.Repeat("previous input ", 120))}
	answer := agent.AssistantMessage("previous answer", nil)
	answer.ResponseMeta = &agent.ResponseMeta{Usage: &agent.TokenUsage{PromptTokens: 900}}
	messages := append(previousPrompt, answer, agent.UserMessage(strings.Repeat("new input ", 30)))
	snapshot := (&agent.ModelCall{Messages: messages}).Snapshot()
	plan, err := manager.Plan(context.Background(), agent.CompactionPlanRequest{
		Messages: messages, ModelRequest: messages, ModelSnapshot: snapshot,
	})
	if err != nil {
		t.Fatal(err)
	}
	metrics := plan.Metrics
	if metrics.ObservedPromptTokens != 900 || metrics.ObservedEstimateTokens <= 0 ||
		metrics.ProjectedTokensBefore != metrics.CalibratedTokens(metrics.EstimatedTokensBefore)+metrics.ReservedTokens {
		t.Fatalf("calibrated Standard metrics=%#v", metrics)
	}
}

func TestStandardPlansOnlyCompleteTurnAndToolBatchBoundaries(t *testing.T) {
	manager := compaction.Standard(compaction.StandardConfig{
		Summarizer: compaction.SummarizerFunc{
			Capability: agent.CapabilityIdentity{Kind: "compaction.atomic-boundary-summary", Version: 1},
			Func: func(context.Context, compaction.SummaryRequest) (compaction.Summary, error) {
				return compaction.Summary{Content: "summary", TokenEstimate: 2}, nil
			},
		},
		TriggerBytes: 1024, KeepRecentBytes: 128, KeepRecentTurns: 1,
		HardLimitBytes: 8 << 20, SummaryLimitBytes: 256 << 10,
	})
	messages := []*agent.Message{
		agent.UserMessage(strings.Repeat("old request ", 200)),
		agent.AssistantMessage("", []agent.ToolCall{{
			ID: "read-old", Type: "function",
			Function: agent.FunctionCall{Name: "read", Arguments: `{"path":"chapter.md"}`},
		}}),
		agent.ToolMessage(agent.TextToolResult(strings.Repeat("tool evidence ", 400)), "read-old", agent.WithToolName("read")),
		agent.AssistantMessage("old answer", nil),
		agent.UserMessage("current request"),
		agent.AssistantMessage("current answer", nil),
	}
	plan, err := manager.Plan(context.Background(), agent.CompactionPlanRequest{
		Messages: messages, ModelRequest: messages,
		ModelSnapshot: (&agent.ModelCall{Messages: messages}).Snapshot(), Force: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Action != agent.CompactionCreate || plan.SourceFrom != 0 || plan.SourceTo != 4 {
		t.Fatalf("atomic Compaction range = [%d,%d) action=%q", plan.SourceFrom, plan.SourceTo, plan.Action)
	}
	if messages[plan.SourceTo].Role != agent.User {
		t.Fatalf("Compaction split a turn/tool batch before %#v", messages[plan.SourceTo])
	}
}
