package agents

import (
	"context"
	"sync/atomic"
	"testing"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	producttools "denova/internal/agents/tools"
	"denova/internal/interactive"
)

func TestInteractiveDirectorPlanSubmissionTerminatesAgentRun(t *testing.T) {
	ctx := context.Background()
	var submissions atomic.Int32
	tools, err := producttools.NewInteractiveDirectorPlan(projectInteractiveToolContext(InteractiveStoryToolContext{
		SubmitDirectorPlanUpdate: func(_ context.Context, submission interactive.DirectorPlanUpdateSubmission) (interactive.DirectorPlanUpdateReceipt, error) {
			submissions.Add(1)
			return interactive.DirectorPlanUpdateReceipt{
				Finalized: true,
				Decision:  submission.Decision,
			}, nil
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	chatModel := &interactiveTurnProtocolChatModel{responses: []*agent.Message{
		agent.AssistantMessage("", []agent.ToolCall{{
			ID: "call-director-plan",
			Function: agent.FunctionCall{
				Name:      submitDirectorPlanUpdateToolName,
				Arguments: `{"decision":{"mode":"keep","reason":"当前规划仍然有效"},"updates":[],"finalize":true}`,
			},
		}}),
		agent.AssistantMessage("不应在结构化提交后再次调用模型。", nil),
	}}
	builtAgent, err := agent.NewAgent(ctx, agent.AgentConfig{
		Name:          "interactive-director-terminal-submission-test",
		Description:   "test",
		Instruction:   "test",
		Model:         chatModel,
		MaxIterations: 3,
		Tools:         tools,
	})
	if err != nil {
		t.Fatal(err)
	}
	runner := agent.NewRunner(agent.RunnerConfig{Agent: builtAgent, EnableStreaming: true})
	conversation := &singleInstructionConversation{instruction: "更新导演规划"}
	var events []Event
	outcome := newTurnExecutor(DefaultLoopPolicy()).Run(ctx, runner, conversation, nil, ChatRequest{Message: "更新导演规划"}, RunOptions{
		AgentKind:       config.AgentKindInteractiveDirector,
		RootAgentName:   "interactive-director-terminal-submission-test",
		MaintenanceTask: "director_plan_update",
	}, func(event Event) { events = append(events, event) })

	calls, _, _ := chatModel.snapshot()
	if calls != 1 || submissions.Load() != 1 {
		t.Fatalf("terminal submission must stop before another model call: calls=%d submissions=%d", calls, submissions.Load())
	}
	if conversation.output != "" {
		t.Fatalf("director plan submission should not require redundant assistant prose: %q", conversation.output)
	}
	if countEventType(events, "done") != 1 || countEventType(events, "error") != 0 {
		t.Fatalf("director run did not finish successfully: %#v", events)
	}
	if outcome.Status != RunOutcomeCompleted {
		t.Fatalf("interactive director protocol cancel outcome = %#v, want completed", outcome)
	}
}
