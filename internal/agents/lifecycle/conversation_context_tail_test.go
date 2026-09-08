package lifecycle

import (
	"testing"

	agent "github.com/alfredxw/denova/agent"

	agentchat "denova/internal/agents/chat"
	agentcontext "denova/internal/agents/context"
)

func TestProjectConversationContextSelectsUserRequestBeforeProtocolTail(t *testing.T) {
	state := agent.UserMessage("workspace state")
	state.Extra = map[string]any{"agent.context_state": "v1"}
	completion := agent.UserMessage("research complete")
	completion.TaskCompletion = &agent.TaskCompletionMessageMeta{CompletionID: "completion-1", Author: "researcher", Recipient: "writer"}
	prepared := agentchat.AgentContextPreparation{ModelContext: agentcontext.ModelContextResult{
		Messages: []*agent.Message{agent.UserMessage("rendered current request"), state, completion},
	}}
	request := agent.ContextRequest{Run: agent.RunView{ID: "run-1", CommandID: "command-1", Cycle: 1}}
	fragments, err := projectConversationContext(prepared, request, agentcontext.DefaultBudget())
	if err != nil {
		t.Fatal(err)
	}
	if len(fragments) != 1 || fragments[0].Placement != agent.ContextFinalUserMessage || fragments[0].Content != "rendered current request" {
		t.Fatalf("projected turn fragments=%#v", fragments)
	}
	prepared.ModelContext.Messages = []*agent.Message{state, completion}
	if _, err := projectConversationContext(prepared, request, agentcontext.DefaultBudget()); err == nil {
		t.Fatal("context without a user request used a protocol message as input")
	}
}
