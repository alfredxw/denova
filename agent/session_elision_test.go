package agent_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/compaction"
	"github.com/alfredxw/denova/agent/permission"
	sessionfile "github.com/alfredxw/denova/agent/session/file"
	"github.com/alfredxw/denova/agent/toolresult"
)

type elisionRunModel struct {
	t               *testing.T
	step            int
	sawElision      bool
	pause           chan struct{}
	paused          bool
	protectedGrowth bool
}

func (model *elisionRunModel) Generate(ctx context.Context, messages []*agent.Message, _ ...agent.ModelOption) (*agent.Message, error) {
	for _, message := range messages {
		if strings.HasPrefix(message.Content, "[Earlier tool output elided;") {
			model.sawElision = true
			if model.pause != nil && !model.paused {
				model.paused = true
				close(model.pause)
				<-ctx.Done()
				return nil, ctx.Err()
			}
		}
	}
	if model.step == 18 {
		return agent.AssistantMessage("Verified 18 sources; budget 72519.", nil), nil
	}
	if !containsElisionTestText(messages, "Keep budget 72519") {
		model.t.Error("Elision lost the active user instruction")
	}
	model.step++
	content := "Read the next source"
	if model.protectedGrowth && model.step >= 12 {
		content += strings.Repeat("Retain this analysis until summarization. ", 150)
	}
	return agent.AssistantMessage(content, []agent.ToolCall{{ID: fmt.Sprintf("evidence-%d", model.step), Type: "function", Function: agent.FunctionCall{Name: "evidence", Arguments: fmt.Sprintf(`{"source":%d}`, model.step)}}}), nil
}

func (model *elisionRunModel) Stream(ctx context.Context, messages []*agent.Message, options ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	response, err := model.Generate(ctx, messages, options...)
	return agent.StreamReaderFromArray([]*agent.Message{response}), err
}

func containsElisionTestText(messages []*agent.Message, text string) bool {
	for _, message := range messages {
		if strings.Contains(message.Content, text) {
			return true
		}
	}
	return false
}

func TestElisionAvoidsSummariesAndSurvivesRestartAndSuspend(t *testing.T) {
	for _, mode := range []string{"summary_only", "elision", "suspend", "elision_then_summary", "summary_failure"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			model := &elisionRunModel{t: t, protectedGrowth: mode == "elision_then_summary" || mode == "summary_failure"}
			if mode == "suspend" {
				model.pause = make(chan struct{})
			}
			executions := map[int]int{}
			tool, err := agent.InferTool("evidence", "Read one source", func(_ context.Context, input struct {
				Source int `json:"source"`
			}) (string, error) {
				executions[input.Source]++
				return fmt.Sprintf("source %d\n", input.Source) + strings.Repeat("data ", 850), nil
			})
			if err != nil {
				t.Fatal(err)
			}
			tools, err := agent.StaticTools(agent.ToolDefinition{Tool: tool, Descriptor: agent.ToolDescriptor{
				Source: agent.ToolSourceRead, Execution: agent.ToolExecutionParallelRead, MutationScope: agent.ToolMutationNone,
				PostCheck: agent.ToolPostCheckNone, Recovery: agent.ToolRecoveryReadOnly, ResultRecoveryKind: agent.ToolResultRecoveryRead,
				ResultProjection: agent.ToolResultBoundedModelContext, ResultRetention: agent.ToolResultDeferred,
				Steering: agent.SteeringFinishCurrent, MaxResultBytes: 32 << 10,
			}})
			if err != nil {
				t.Fatal(err)
			}
			summaries := 0
			summarySawElision := false
			definition := agent.Definition{Model: model, Instructions: "Stable project instructions", Tools: tools, Permission: permission.FullAccess(),
				ResultProcessor: toolresult.Standard(toolresult.Policy{MaxBytes: 32 << 10, ContextWindowTokens: 12_000}),
				Compaction: compaction.Standard(compaction.StandardConfig{ContextWindowTokens: 12_000, TriggerRatio: .85, SummaryLimitBytes: 4096,
					Summarizer: compaction.SummarizerFunc{Capability: agent.CapabilityIdentity{Kind: "test.elision-summary", Version: 1}, Func: func(_ context.Context, request compaction.SummaryRequest) (agent.CompactionCheckpoint, error) {
						summaries++
						summarySawElision = summarySawElision || containsElisionTestText(request.Messages, "[Earlier tool output elided;")
						if mode == "summary_failure" {
							return agent.CompactionCheckpoint{}, errors.New("injected summary failure after Elision")
						}
						return agent.CompactionCheckpoint{Summary: "Checked earlier sources. Continue; budget 72519."}, nil
					}},
				}),
			}
			if mode != "summary_only" {
				definition.Elision = &agent.ElisionPolicy{ContextWindowTokens: 12_000}
			}
			store, err := sessionfile.New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			owner, err := agent.New(ctx, definition, agent.WithSessionStore(store))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = owner.Close(ctx) }()
			conversation, err := owner.Session(ctx, agent.NamedSession("elision-recovery"))
			if err != nil {
				t.Fatal(err)
			}
			run, err := conversation.Run(ctx, agent.Text("Keep budget 72519 and verify 18 sources."))
			if err != nil {
				t.Fatal(err)
			}
			if mode == "suspend" {
				select {
				case <-model.pause:
				case <-time.After(2 * time.Second):
					t.Fatal("Elision did not reach the model")
				}
				runID := run.ID()
				if _, err := conversation.SuspendAndClose(ctx, agent.SuspendRequest{RunID: runID, IdempotencyKey: "pause-after-elision"}); err != nil {
					t.Fatal(err)
				}
				if err := owner.Close(ctx); err != nil {
					t.Fatal(err)
				}
				owner, err = agent.New(ctx, definition, agent.WithSessionStore(store))
				if err != nil {
					t.Fatal(err)
				}
				conversation, err = owner.Session(ctx, agent.NamedSession("elision-recovery"))
				if err != nil {
					t.Fatal(err)
				}
				run, err = conversation.ResumeRun(ctx, agent.ResumeRequest{RunID: runID, IdempotencyKey: "resume-after-elision"})
				if err != nil {
					t.Fatal(err)
				}
			}
			result, err := run.Wait(ctx)
			if err != nil || result.Status != agent.ResultCompleted {
				t.Fatalf("run=%+v err=%v", result, err)
			}
			for source := 1; source <= 18; source++ {
				if executions[source] != 1 {
					t.Fatalf("source %d executed %d times", source, executions[source])
				}
			}
			if mode == "summary_only" {
				if summaries == 0 || model.sawElision {
					t.Fatalf("baseline: summaries=%d elision=%v", summaries, model.sawElision)
				}
				return
			}
			if (!model.protectedGrowth && summaries != 0) || (model.protectedGrowth && summaries == 0) || !model.sawElision {
				t.Fatalf("Elision did not avoid summaries: summaries=%d elision=%v", summaries, model.sawElision)
			}
			before, err := conversation.Inspect(ctx, agent.Text("Continue"))
			if err != nil || before.ElisionMetrics.ResultsElided == 0 {
				t.Fatalf("inspection=%+v err=%v", before.ElisionMetrics, err)
			}
			if err := owner.Close(ctx); err != nil {
				t.Fatal(err)
			}
			owner, err = agent.New(ctx, definition, agent.WithSessionStore(store))
			if err != nil {
				t.Fatal(err)
			}
			conversation, err = owner.Session(ctx, agent.NamedSession("elision-recovery"))
			if err != nil {
				t.Fatal(err)
			}
			after, err := conversation.Inspect(ctx, agent.Text("Continue"))
			if err != nil || !reflect.DeepEqual(before.ModelRequest.Messages, after.ModelRequest.Messages) {
				t.Fatalf("cold Elision projection differs: %v", err)
			}
			if mode != "summary_failure" {
				compacted, err := conversation.Compact(ctx, agent.CompactionRequest{Force: true})
				if err != nil || !compacted.Changed || !summarySawElision {
					t.Fatalf("summary did not consume elided source: changed=%v elision=%v err=%v", compacted.Changed, summarySawElision, err)
				}
			} else if !containsElisionTestText(after.ModelRequest.Messages, "[Earlier tool output elided;") || !summarySawElision {
				t.Fatal("summary failure reverted committed Elision")
			}
			if err := conversation.Clear(ctx); err != nil {
				t.Fatal(err)
			}
			cleared, err := conversation.Inspect(ctx, agent.Text("New task"))
			if err != nil || cleared.ElisionMetrics.ResultsElided != 0 || containsElisionTestText(cleared.ModelRequest.Messages, "Earlier tool") {
				t.Fatalf("Clear retained Elision: %+v %v", cleared.ElisionMetrics, err)
			}
		})
	}
}
