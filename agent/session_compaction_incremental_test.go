package agent_test

import (
	"context"
	"encoding/json"
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
)

type incrementalModel struct {
	t         *testing.T
	step      int
	inputs    [][]*agent.Message
	summarize func(context.Context) (agent.CompactionCheckpoint, error)
}

func (m *incrementalModel) Generate(ctx context.Context, messages []*agent.Message, _ ...agent.ModelOption) (*agent.Message, error) {
	if len(messages) > 0 && (strings.HasPrefix(messages[len(messages)-1].Content, "[Runtime context compaction request]") || strings.HasPrefix(messages[0].Content, "Create a continuation checkpoint")) {
		checkpoint, err := m.summarize(ctx)
		return agent.AssistantMessage(checkpoint.Summary, nil), err
	}
	m.inputs = append(m.inputs, messages)
	intent, latest := false, m.step == 0
	calls := map[string]bool{}
	for _, message := range messages {
		intent = intent || message.Content == "Keep budget 72519; verify every source before finishing."
		for _, call := range message.ToolCalls {
			calls[call.ID] = true
		}
		if message.Role == agent.ToolRole {
			if !calls[message.ToolCallID] {
				m.t.Errorf("orphan result %s", message.ToolCallID)
			}
			latest = latest || message.ToolCallID == fmt.Sprintf("step-%d", m.step)
		}
	}
	if !intent || !latest {
		m.t.Errorf("step %d lost intent=%v or newest result=%v", m.step, intent, latest)
	}
	if m.step == 10 {
		return agent.AssistantMessage("Verified all sources within budget 72519.", nil), nil
	}
	m.step++
	return agent.AssistantMessage("Checking the next source", []agent.ToolCall{{
		ID: fmt.Sprintf("step-%d", m.step), Type: "function",
		Function: agent.FunctionCall{Name: "evidence", Arguments: fmt.Sprintf(`{"step":%d}`, m.step)},
	}}), nil
}

func (m *incrementalModel) Stream(ctx context.Context, messages []*agent.Message, options ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	result, err := m.Generate(ctx, messages, options...)
	return agent.StreamReaderFromArray([]*agent.Message{result}), err
}

func TestCompactionRepeatsWithinOneUserRun(t *testing.T) {
	for _, extension := range []string{"builtin", "summarizer", "manager"} {
		for _, interruption := range []string{"none", "pause", "abort", "summary_failure"} {
			t.Run(extension+"/"+interruption, func(t *testing.T) { testRepeatedCompaction(t, extension, interruption) })
		}
	}
}

func testRepeatedCompaction(t *testing.T, extension, interruption string) {
	ctx := context.Background()
	model := &incrementalModel{t: t}
	executions := map[int]int{}
	tool, err := agent.InferTool("evidence", "Read one source", func(_ context.Context, input struct {
		Step int `json:"step"`
	}) (string, error) {
		executions[input.Step]++
		return strings.Repeat(fmt.Sprintf("source-%d: verified 72519. ", input.Step), 320), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	toolset, err := agent.StaticToolsIdentified(agent.CapabilityIdentity{Kind: "test.incremental-tools", Version: 1}, agent.ToolDefinition{
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
	summaries := 0
	interrupted := make(chan struct{})
	summarize := func(ctx context.Context, request agent.CompactionCompactRequest) (agent.CompactionCheckpoint, error) {
		summaries++
		if summaries == 4 {
			if interruption == "summary_failure" {
				return agent.CompactionCheckpoint{}, errors.New("injected summary failure after three checkpoints")
			}
			if interruption == "pause" || interruption == "abort" {
				close(interrupted)
				<-ctx.Done()
				return agent.CompactionCheckpoint{}, ctx.Err()
			}
		}
		if extension != "builtin" {
			if len(request.Messages) == 0 {
				t.Error("empty summary delta")
			}
			if summaries > 1 && !strings.Contains(request.Messages[0].Content, "Incremental checkpoint") {
				t.Error("previous checkpoint was not merged")
			}
			if request.Current != nil && (request.Current.ContextData == nil || string(request.Current.ContextData.Data) != `{"budget":72519}`) {
				t.Error("structured checkpoint data did not reach the extension")
			}
		}
		return agent.CompactionCheckpoint{Summary: fmt.Sprintf("Incremental checkpoint %d. Budget 72519. Sources checked; continue verification.", summaries),
			ContextData: &agent.HostData{Type: "test.evidence", Version: 1, Data: json.RawMessage(`{"budget":72519}`)}}, nil
	}
	model.summarize = func(ctx context.Context) (agent.CompactionCheckpoint, error) {
		return summarize(ctx, agent.CompactionCompactRequest{})
	}
	var manager agent.CompactionManager
	switch extension {
	case "builtin":
		manager = compaction.Standard(compaction.StandardConfig{ContextWindowTokens: 6000})
	case "summarizer":
		manager = compaction.Standard(compaction.StandardConfig{
			Summarizer: compaction.SummarizerFunc{Capability: agent.CapabilityIdentity{Kind: "test.incremental-summary", Version: 1}, Func: func(ctx context.Context, r compaction.SummaryRequest) (agent.CompactionCheckpoint, error) {
				return summarize(ctx, agent.CompactionCompactRequest{Messages: r.Messages, Current: r.Current})
			}},
			TriggerBytes: 12000, KeepRecentBytes: 1000, HardLimitBytes: 1 << 20, SummaryLimitBytes: 8192,
		})
	case "manager":
		manager = repeatedCompactionManager{summarize: summarize}
	default:
		t.Fatal("unknown extension")
	}
	definition := agent.Definition{Name: "incremental", Model: model, Tools: toolset, Permission: permission.FullAccess(), Compaction: manager}
	store, err := sessionfile.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	owner, err := agent.New(ctx, definition, agent.WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = owner.Close(ctx) }()
	conversation, err := owner.Session(ctx, agent.NamedSession("one-long-run"))
	if err != nil {
		t.Fatal(err)
	}
	run, err := conversation.Run(ctx, agent.Text("Keep budget 72519; verify every source before finishing."))
	if err != nil {
		t.Fatal(err)
	}
	expectedStatus := agent.ResultCompleted
	if interruption == "pause" || interruption == "abort" {
		select {
		case <-interrupted:
		case <-time.After(3 * time.Second):
			t.Fatal("fourth checkpoint did not start")
		}
		runID := run.ID()
		if interruption == "abort" {
			if _, err := run.Abort(ctx, agent.AbortRequest{Reason: "Cancel after three checkpoints"}); err != nil {
				t.Fatal(err)
			}
			expectedStatus = agent.ResultAborted
		} else {
			if _, err := conversation.SuspendAndClose(ctx, agent.SuspendRequest{RunID: runID, IdempotencyKey: "pause-fourth-checkpoint"}); err != nil {
				t.Fatal(err)
			}
			if err := owner.Close(ctx); err != nil {
				t.Fatal(err)
			}
			owner, err = agent.New(ctx, definition, agent.WithSessionStore(store))
			if err != nil {
				t.Fatal(err)
			}
			conversation, err = owner.Session(ctx, agent.NamedSession("one-long-run"))
			if err != nil {
				t.Fatal(err)
			}
			before, err := conversation.Snapshot(ctx)
			if err != nil || before.Compaction == nil || before.Compaction.Revision != 3 {
				t.Fatalf("interrupted checkpoint was published: %+v %v", before.Compaction, err)
			}
			run, err = conversation.ResumeRun(ctx, agent.ResumeRequest{RunID: runID, IdempotencyKey: "resume-fourth-checkpoint"})
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	result, err := run.Wait(ctx)
	if err != nil || result.Status != expectedStatus {
		t.Fatalf("run=%+v error=%v", result, err)
	}
	if summaries < 3 {
		t.Fatalf("single user Run compacted %d times; want at least 3", summaries)
	}
	for step := 1; step <= model.step; step++ {
		if executions[step] != 1 {
			t.Errorf("source %d executed %d times", step, executions[step])
		}
	}
	committedSummaries := summaries
	if interruption != "none" {
		committedSummaries--
	}
	snapshot, err := conversation.Snapshot(ctx)
	if err != nil || snapshot.Compaction == nil || snapshot.Compaction.Revision != uint64(committedSummaries) {
		t.Fatalf("checkpoint=%+v summaries=%d error=%v", snapshot.Compaction, summaries, err)
	}
	if extension != "builtin" && (snapshot.Compaction.ContextData == nil || string(snapshot.Compaction.ContextData.Data) != `{"budget":72519}`) {
		t.Fatal("structured checkpoint state was not persisted")
	}
	if err := owner.Close(ctx); err != nil {
		t.Fatal(err)
	}
	owner, err = agent.New(ctx, definition, agent.WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	conversation, err = owner.Session(ctx, agent.NamedSession("one-long-run"))
	if err != nil {
		t.Fatal(err)
	}
	restored, err := conversation.Snapshot(ctx)
	if err != nil || !reflect.DeepEqual(restored.Compaction, snapshot.Compaction) {
		t.Fatalf("reopened checkpoint differs: %#v %v", restored.Compaction, err)
	}

}

// This extension owns planning and semantic state; it never sees journal positions.
type repeatedCompactionManager struct {
	summarize func(context.Context, agent.CompactionCompactRequest) (agent.CompactionCheckpoint, error)
}

func (repeatedCompactionManager) Identity() agent.CapabilityIdentity {
	return agent.CapabilityIdentity{Kind: "test.repeated-manager", Version: 1}
}
func (repeatedCompactionManager) SummaryLimitBytes() int { return 8192 }
func (repeatedCompactionManager) Plan(_ context.Context, r agent.CompactionPlanRequest) (agent.CompactionPlan, error) {
	if len(r.Groups) == 0 {
		return agent.CompactionPlan{Action: agent.CompactionNone}, nil
	}
	return agent.CompactionPlan{Action: agent.CompactionCreate, GroupCount: len(r.Groups), Validation: agent.CompactionValidationPolicy{HardLimitBytes: 1 << 20}}, nil
}
func (m repeatedCompactionManager) Compact(ctx context.Context, r agent.CompactionCompactRequest) (agent.CompactionCheckpoint, error) {
	return m.summarize(ctx, r)
}
