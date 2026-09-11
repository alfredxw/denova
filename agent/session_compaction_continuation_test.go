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
	"github.com/alfredxw/denova/agent/session"
)

const continuationEvidence = "LIVE_TOOL_EVIDENCE_72519"

type continuationContextKey struct{}

type continuationModel struct {
	inputs         [][]*agent.Message
	requireContext bool
}

func (model *continuationModel) Generate(ctx context.Context, messages []*agent.Message, _ ...agent.ModelOption) (*agent.Message, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if model.requireContext && ctx.Value(continuationContextKey{}) != "active invocation" {
		return nil, errors.New("model lost the active middleware context")
	}
	model.inputs = append(model.inputs, messages)
	if len(model.inputs) == 1 {
		return agent.AssistantMessage("Reading current evidence", []agent.ToolCall{{
			ID: "live-read", Type: "function", Function: agent.FunctionCall{Name: "read_live", Arguments: `{}`},
		}}), nil
	}
	return agent.AssistantMessage("Completed", nil), nil
}

func (model *continuationModel) Stream(ctx context.Context, messages []*agent.Message, options ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	result, err := model.Generate(ctx, messages, options...)
	return agent.StreamReaderFromArray([]*agent.Message{result}), err
}

type continuationPreparation struct {
	agent.BaseMiddleware
	checkpoints      [][]*agent.Message
	iterations       []int
	beforeAgentCalls int
	failure          string
}

func (middleware *continuationPreparation) BeforeAgent(ctx context.Context, run *agent.RunContext) (context.Context, *agent.RunContext, error) {
	middleware.beforeAgentCalls++
	run.Instruction = "Instruction resolved by BeforeAgent"
	return context.WithValue(ctx, continuationContextKey{}, "active invocation"), run, nil
}

func (middleware *continuationPreparation) BeforeModelCall(ctx context.Context, call *agent.ModelCall, metadata *agent.ModelContext) (context.Context, *agent.ModelCall, error) {
	for _, message := range call.Messages {
		if strings.Contains(message.Content, "Old history checkpoint") {
			call.Messages = append(call.Messages, agent.UserMessage(fmt.Sprintf("CHECKPOINT_PREPARATION_%d", len(middleware.checkpoints)+1)))
			middleware.checkpoints = append(middleware.checkpoints, call.Snapshot().Messages())
			middleware.iterations = append(middleware.iterations, metadata.Iteration)
			if middleware.failure == "preparation" {
				return ctx, nil, errors.New("injected candidate preparation failure")
			}
			if middleware.failure == "validation" {
				call.Messages = append(call.Messages, agent.UserMessage(strings.Repeat("oversized candidate ", 8000)))
			}
			break
		}
	}
	return ctx, call, nil
}

// Exercise the public Session API, real tool execution, the standard planner,
// projection validation and journal recovery. Only model text is deterministic.
func TestCompactionPreservesLiveToolTailAndExecutesValidatedRequest(t *testing.T) {
	for _, failure := range []string{"", "preparation", "validation", "abort", "resume"} {
		name := failure
		if name == "" {
			name = "success"
		}
		t.Run(name, func(t *testing.T) { testCompactionContinuation(t, failure) })
	}
}

func testCompactionContinuation(t *testing.T, failure string) {
	ctx := context.Background()
	model := &continuationModel{requireContext: true}
	middleware := &continuationPreparation{failure: failure}
	toolCalls := 0
	tool, err := agent.InferTool("read_live", "Read current evidence", func(context.Context, struct{}) (string, error) {
		toolCalls++
		return strings.Repeat(continuationEvidence+" ", 1000), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	tools, err := agent.StaticToolsIdentified(agent.CapabilityIdentity{Kind: "test.continuation-tools", Version: 1}, agent.ToolDefinition{
		Tool: tool, Descriptor: agent.ToolDescriptor{
			Source: agent.ToolSourceRead, Execution: agent.ToolExecutionParallelRead,
			MutationScope: agent.ToolMutationNone, PostCheck: agent.ToolPostCheckNone,
			Recovery: agent.ToolRecoveryReadOnly, ResultProjection: agent.ToolResultBoundedModelContext,
			ResultRetention: agent.ToolResultProtected, Steering: agent.SteeringFinishCurrent, MaxResultBytes: 64 << 10,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	summaryCalls := 0
	summaryStarted := make(chan struct{})
	manager := compaction.Standard(compaction.StandardConfig{
		Summarizer: compaction.SummarizerFunc{
			Capability: agent.CapabilityIdentity{Kind: "test.continuation-summary", Version: 1},
			Func: func(ctx context.Context, request compaction.SummaryRequest) (agent.CompactionCheckpoint, error) {
				summaryCalls++
				for _, message := range request.Messages {
					if strings.Contains(message.Content, continuationEvidence) {
						t.Error("active tool result entered the old-history summary source")
					}
				}
				if failure == "abort" || failure == "resume" && summaryCalls == 1 {
					close(summaryStarted)
					<-ctx.Done()
					return agent.CompactionCheckpoint{}, ctx.Err()
				}
				return agent.CompactionCheckpoint{Summary: "Old history checkpoint"}, nil
			},
		},
		TriggerBytes: 12_000, KeepRecentBytes: 100, HardLimitBytes: 1 << 20, SummaryLimitBytes: 8192,
	})
	store := session.Memory()
	definition := agent.Definition{
		Name: "continuation", Model: model, Tools: tools, Permission: permission.FullAccess(),
		Compaction: manager, Middlewares: []agent.Middleware{middleware},
	}
	owner, err := agent.New(ctx, definition, agent.WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(ctx) })
	conversation, err := owner.Session(ctx, agent.NamedSession("active-turn"))
	if err != nil {
		t.Fatal(err)
	}
	raw := []*agent.Message{
		agent.UserMessage(strings.Repeat("Historical instructions. ", 180)), agent.AssistantMessage("Old answer", nil),
		agent.UserMessage("Recent request"), agent.AssistantMessage("Recent answer", nil),
	}
	if err := conversation.LoadCanonicalMessages(ctx, raw); err != nil {
		t.Fatal(err)
	}
	run, err := conversation.Run(ctx, agent.Text("Use the current tool evidence"))
	if err != nil {
		t.Fatal(err)
	}
	status := agent.ResultCompleted
	if failure == "abort" || failure == "resume" {
		select {
		case <-summaryStarted:
		case <-time.After(2 * time.Second):
			t.Fatal("summary did not start")
		}
		if failure == "abort" {
			if _, err := run.Abort(ctx, agent.AbortRequest{Reason: "Cancel during compaction"}); err != nil {
				t.Fatal(err)
			}
			status = agent.ResultAborted
		} else {
			runID := run.ID()
			if _, err := conversation.SuspendAndClose(ctx, agent.SuspendRequest{RunID: runID, IdempotencyKey: "pause-summary"}); err != nil {
				t.Fatal(err)
			}
			if err := owner.Close(ctx); err != nil {
				t.Fatal(err)
			}
			owner, err = agent.New(ctx, definition, agent.WithSessionStore(store))
			if err != nil {
				t.Fatal(err)
			}
			conversation, err = owner.Session(ctx, agent.NamedSession("active-turn"))
			if err != nil {
				t.Fatal(err)
			}
			run, err = conversation.ResumeRun(ctx, agent.ResumeRequest{RunID: runID, IdempotencyKey: "resume-summary"})
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	result, err := run.Wait(ctx)
	if err != nil || result.Status != status {
		t.Fatalf("run: result=%+v error=%v", result, err)
	}
	snapshot, err := conversation.Snapshot(ctx)
	wantSummaryCalls, wantBeforeAgent, wantIteration := 1, 1, 1
	if failure == "resume" {
		wantSummaryCalls, wantBeforeAgent, wantIteration = 2, 2, 0
	}
	if err != nil || summaryCalls != wantSummaryCalls || (snapshot.Compaction != nil) != (failure == "" || failure == "resume") {
		t.Fatalf("compaction: snapshot=%+v calls=%d error=%v", snapshot, summaryCalls, err)
	}
	if toolCalls != 1 {
		t.Fatalf("live tool executed %d times", toolCalls)
	}
	if middleware.beforeAgentCalls != wantBeforeAgent || failure != "abort" && !reflect.DeepEqual(middleware.iterations, []int{wantIteration}) {
		t.Fatalf("candidate left the current invocation: BeforeAgent=%d iterations=%v", middleware.beforeAgentCalls, middleware.iterations)
	}
	last := model.inputs[len(model.inputs)-1]
	var liveResult *agent.Message
	if failure == "abort" {
		if len(model.inputs) != 1 || len(middleware.checkpoints) != 0 {
			t.Fatal("aborted compaction continued preparing or calling the model")
		}
		liveResult = &agent.Message{Role: agent.ToolRole, ToolCallID: "live-read", Content: strings.Repeat(continuationEvidence+" ", 1000)}
	}
	for _, message := range last {
		if message.Role == agent.ToolRole && strings.Contains(message.Content, continuationEvidence) {
			liveResult = message
		}
	}
	if liveResult == nil {
		t.Fatal("post-compaction provider request lost the unselected live tool result")
	}
	if failure != "abort" && len(middleware.checkpoints) != 1 {
		t.Fatalf("candidate prepared %d times", len(middleware.checkpoints))
	}
	if failure == "" || failure == "resume" {
		if !reflect.DeepEqual(last, middleware.checkpoints[0]) {
			t.Fatal("provider did not use the validated request")
		}
	} else if failure != "abort" {
		var historical bool
		for _, message := range last {
			historical = historical || strings.Contains(message.Content, "Historical instructions.")
			if strings.Contains(message.Content, "Old history checkpoint") {
				t.Fatal("unpublished candidate reached the provider")
			}
		}
		if !historical {
			t.Fatal("failed candidate replaced the original history")
		}
	}
	if err := owner.Close(ctx); err != nil {
		t.Fatal(err)
	}
	model.requireContext = false
	owner, err = agent.New(ctx, agent.Definition{
		Name: "continuation", Model: model, Compaction: compaction.Disabled(1<<20, 8192),
	}, agent.WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	conversation, err = owner.Session(ctx, agent.NamedSession("active-turn"))
	if err != nil {
		t.Fatal(err)
	}
	run, err = conversation.Run(ctx, agent.Text("Continue after reopen"))
	if err != nil {
		t.Fatal(err)
	}
	if result, err := run.Wait(ctx); err != nil || result.Status != agent.ResultCompleted {
		t.Fatalf("reopen: result=%+v error=%v", result, err)
	}
	for _, message := range model.inputs[len(model.inputs)-1] {
		if message.Role == agent.ToolRole && message.ToolCallID == liveResult.ToolCallID && message.Content == liveResult.Content {
			return
		}
	}
	t.Fatal("reopen did not retain the canonical live tool result")
}
