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
				Func: func(context.Context, compaction.SummaryRequest) (agent.CompactionCheckpoint, error) {
					return agent.CompactionCheckpoint{Summary: "contract summary"}, nil
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
			Func: func(context.Context, compaction.SummaryRequest) (agent.CompactionCheckpoint, error) {
				return agent.CompactionCheckpoint{Summary: "summary"}, nil
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
		Groups: []agent.CompactionGroup{{Messages: messages[:2]}}, ModelSnapshot: snapshot,
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

func TestStandardIncludesLifecycleSideForkReserveInTriggerAndValidation(t *testing.T) {
	manager := compaction.Standard(compaction.StandardConfig{
		Summarizer: compaction.SummarizerFunc{
			Capability: agent.CapabilityIdentity{Kind: "compaction.lifecycle-reserve-summary", Version: 1},
			Func: func(context.Context, compaction.SummaryRequest) (agent.CompactionCheckpoint, error) {
				return agent.CompactionCheckpoint{Summary: "summary"}, nil
			},
		},
		TriggerBytes: 1024, KeepRecentBytes: 128, HardLimitBytes: 8 << 20, SummaryLimitBytes: 256 << 10,
		ContextWindowTokens: 2_000, TriggerRatio: .85, RecoveryBand: .8,
	})
	messages := []*agent.Message{
		agent.UserMessage(strings.Repeat("old request ", 100)),
		agent.AssistantMessage("old answer", nil),
		agent.UserMessage("current request"),
		agent.AssistantMessage("current answer", nil),
	}
	plan, err := manager.Plan(context.Background(), agent.CompactionPlanRequest{
		Groups:        []agent.CompactionGroup{{Messages: messages[:2]}},
		ModelSnapshot: (&agent.ModelCall{Messages: messages}).Snapshot(), LifecycleReservedTokens: 1_600,
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Action != agent.CompactionCreate || plan.Validation.ReservedTokens != 1_600 || plan.Metrics.ReservedTokens != 1_600 ||
		plan.Metrics.ProjectedTokensBefore != plan.Metrics.CalibratedTokens(plan.Metrics.EstimatedTokensBefore)+1_600 {
		t.Fatalf("lifecycle-reserved Compaction plan = %#v", plan)
	}
}

func TestStandardUsesCapacityAwareModelOutputReserve(t *testing.T) {
	manager := compaction.Standard(compaction.StandardConfig{
		Summarizer: compaction.SummarizerFunc{
			Capability: agent.CapabilityIdentity{Kind: "compaction.output-cap-summary", Version: 1},
			Func: func(context.Context, compaction.SummaryRequest) (agent.CompactionCheckpoint, error) {
				return agent.CompactionCheckpoint{Summary: "summary"}, nil
			},
		},
		TriggerBytes: 1024, KeepRecentBytes: 128, HardLimitBytes: 8 << 20, SummaryLimitBytes: 256 << 10,
		ContextWindowTokens: 10_000, ReservedTokens: 1000, TriggerRatio: .85, RecoveryBand: .8,
	})
	messages := []*agent.Message{
		agent.UserMessage("old request"),
		agent.AssistantMessage("old answer", nil),
		agent.UserMessage("current request"),
	}
	call := &agent.ModelCall{Messages: messages, Options: []agent.ModelOption{agent.WithMaxTokens(4000)}}
	plan, err := manager.Plan(context.Background(), agent.CompactionPlanRequest{
		Groups: []agent.CompactionGroup{{Messages: messages[:2]}}, ModelSnapshot: call.Snapshot(), Force: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Validation.ReservedTokens != 2500 || plan.Metrics.ReservedTokens != 2500 {
		t.Fatalf("capacity-aware Compaction reserve = validation:%d metrics:%d, want 2500",
			plan.Validation.ReservedTokens, plan.Metrics.ReservedTokens)
	}
}

func TestStandardPlansOnlyCompleteToolBatchBoundaries(t *testing.T) {
	manager := compaction.Standard(compaction.StandardConfig{
		Summarizer: compaction.SummarizerFunc{
			Capability: agent.CapabilityIdentity{Kind: "compaction.atomic-boundary-summary", Version: 1},
			Func: func(context.Context, compaction.SummaryRequest) (agent.CompactionCheckpoint, error) {
				return agent.CompactionCheckpoint{Summary: "summary"}, nil
			},
		},
		TriggerBytes: 1024, KeepRecentBytes: 128, KeepRecentGroups: 1,
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
		Groups:        []agent.CompactionGroup{{Messages: messages[:2]}},
		ModelSnapshot: (&agent.ModelCall{Messages: messages}).Snapshot(), Force: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Action != agent.CompactionCreate || plan.GroupCount != 1 {
		t.Fatalf("atomic Compaction plan = %#v", plan)
	}
	if messages[4].Role != agent.User {
		t.Fatalf("Compaction split a turn/tool batch before %#v", messages[4])
	}
}

func TestStandardSummarizesOversizedOldMessageButKeepsNewestToolGroup(t *testing.T) {
	manager := compaction.Standard(compaction.StandardConfig{
		Summarizer: compaction.SummarizerFunc{Capability: agent.CapabilityIdentity{Kind: "test.oversized-old-source", Version: 1}, Func: func(context.Context, compaction.SummaryRequest) (agent.CompactionCheckpoint, error) {
			return agent.CompactionCheckpoint{Summary: "summary"}, nil
		}},
		TriggerBytes: 4096, KeepRecentBytes: 1024, HardLimitBytes: 1 << 20, SummaryLimitBytes: 1024,
	})
	messages := []*agent.Message{
		agent.UserMessage("Historical task"), agent.AssistantMessage(strings.Repeat("old evidence ", 1000), nil),
		agent.UserMessage("Continue verification"), agent.AssistantMessage("", []agent.ToolCall{{ID: "latest", Type: "function", Function: agent.FunctionCall{Name: "read", Arguments: `{}`}}}),
		{Role: agent.ToolRole, ToolCallID: "latest", Content: "Latest original evidence"},
	}
	plan, err := manager.Plan(t.Context(), agent.CompactionPlanRequest{Groups: []agent.CompactionGroup{{Messages: messages[:2]}}, ModelSnapshot: (&agent.ModelCall{Messages: messages}).Snapshot(), Force: true})
	if err != nil || plan.GroupCount != 1 {
		t.Fatalf("old evidence pinned in retained tail: %+v %v", plan, err)
	}
	messages[4].Content = strings.Repeat("large latest evidence ", 1000)
	plan, err = manager.Plan(t.Context(), agent.CompactionPlanRequest{Groups: []agent.CompactionGroup{{Messages: messages[:2]}}, ModelSnapshot: (&agent.ModelCall{Messages: messages}).Snapshot(), Force: true})
	if err != nil || plan.GroupCount != 1 {
		t.Fatalf("newest complete tool group split: %+v %v", plan, err)
	}
	messages = append(messages, agent.UserMessage("Unconsumed steering: keep the new evidence."))
	plan, err = manager.Plan(t.Context(), agent.CompactionPlanRequest{Groups: []agent.CompactionGroup{{Messages: messages[:2]}}, ModelSnapshot: (&agent.ModelCall{Messages: messages}).Snapshot(), Force: true})
	if err != nil || plan.GroupCount != 1 {
		t.Fatalf("unconsumed steering displaced the newest tool group: %+v %v", plan, err)
	}
}
