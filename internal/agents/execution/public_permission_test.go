package execution

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	agent "github.com/alfredxw/denova/agent"

	agentconversation "denova/internal/agents/conversation"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	agenttoolruntime "denova/internal/agents/toolruntime"
)

func TestAgentRuntimeResolvesConsecutiveToolApprovals(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	sess, err := store.GetOrCreate("consecutive-permissions")
	if err != nil {
		t.Fatal(err)
	}
	var executions atomic.Int32
	tool, err := agent.InferTool("approved_tool", "Exercise consecutive tool approvals", func(context.Context, struct{}) (string, error) {
		executions.Add(1)
		return "complete", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	toolset, err := agent.StaticToolsIdentified(agent.CapabilityIdentity{Kind: "tools.consecutive-permissions", Version: 1}, agent.ToolDefinition{
		Tool: tool,
		Descriptor: agent.ToolDescriptor{
			Source: agent.ToolSourceShell, Execution: agent.ToolExecutionParallelRead,
			MutationScope: agent.ToolMutationNone, PostCheck: agent.ToolPostCheckNone,
			Recovery: agent.ToolRecoveryReadOnly, ResultProjection: agent.ToolResultBoundedModelContext,
			ResultRetention: agent.ToolResultDeferred, Steering: agent.SteeringFinishCurrent, MaxResultBytes: 64 << 10,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	model := &publicBackendTestModel{}
	for index := 0; index < 3; index++ {
		model.responses = append(model.responses, agent.AssistantMessage("", []agent.ToolCall{{
			ID: fmt.Sprintf("provider-call-%d", index), Type: "function", Function: agent.FunctionCall{Name: "approved_tool", Arguments: `{}`},
		}}))
	}
	model.responses = append(model.responses, agent.AssistantMessage("finished", nil))
	runtime, err := NewAgentRuntime(ctx, t.TempDir(), WithToolMutationApplier(
		func(context.Context, agenttoolruntime.CommittedToolMutation) error { return nil },
	))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close(context.Background())
	options := agentrun.Options{
		AgentKind: agentrun.AgentKindIDE, Mode: agentrun.ModeAgentChat,
		ProjectID: "permission-project", SessionID: sess.ID, Workspace: t.TempDir(),
		TaskID: "permission-task", RootAgentName: "root",
	}
	pending := make(chan string, 3)
	operation, err := runtime.Start(ctx, StartRequest{Cycle: Cycle{
		Definition: agent.Definition{
			Key: "consecutive-permissions", Name: "root", Model: model,
			ModelIdentity: agent.CapabilityIdentity{Kind: "model.consecutive-permissions", Version: 1}, Tools: toolset,
		},
		Conversation: agentconversation.NewSessionConversationForAgent(sess, nil, agentrun.AgentKindIDE),
		Request:      agentchatRequest("permission-command", "perform three approved actions"),
		Options:      options,
	}, Emit: func(event agentrun.Event) {
		if event.Type == "ask_pending" {
			if value, ok := event.Data.(map[string]any); ok {
				id, _ := value["id"].(string)
				pending <- id
			}
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	outcomes := make(chan agentrun.Outcome, 1)
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				err := fmt.Errorf("permission test wait panic: %v", recovered)
				outcomes <- agentrun.NewOutcome(agentrun.OutcomeFailed, err, err.Error(), "", "")
			}
		}()
		outcomes <- operation.Wait(ctx)
	}()
	for index := 0; index < 3; index++ {
		var id string
		select {
		case id = <-pending:
		case outcome := <-outcomes:
			t.Fatalf("run ended before approval %d: %#v", index+1, outcome)
		case <-ctx.Done():
			t.Fatalf("waiting for approval %d: %v", index+1, ctx.Err())
		}
		if got := executions.Load(); got != int32(index) {
			t.Fatalf("executions before approval %d=%d", index+1, got)
		}
		if _, err := runtime.ResolveAsk(ctx, options, id, session.AskAnswered, []agentconversation.HostAskAnswer{{
			QuestionID: "tool-approval", SelectedOptionIDs: []string{session.ToolApprovalAllowOnceOptionID},
		}}, ""); err != nil {
			cancel()
			t.Fatalf("approval %d: %v", index+1, err)
		}
	}
	select {
	case outcome := <-outcomes:
		if outcome.Status != agentrun.OutcomeCompleted || executions.Load() != 3 {
			t.Fatalf("outcome=%#v executions=%d", outcome, executions.Load())
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}
