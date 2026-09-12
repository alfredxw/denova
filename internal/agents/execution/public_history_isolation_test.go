package execution

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	agentchat "denova/internal/agents/chat"
	agentconversation "denova/internal/agents/conversation"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"

	agent "github.com/alfredxw/denova/agent"
)

type invalidHistoryConversation struct {
	agentchat.Conversation
	messages []*agent.Message
}

func (conversation invalidHistoryConversation) CanonicalMessages(context.Context) ([]*agent.Message, error) {
	return conversation.messages, nil
}

func TestInvalidHistoryIsolatesAdmissionToSessionOrBranch(t *testing.T) {
	for _, kind := range []string{agentrun.AgentKindIDE, agentrun.AgentKindInteractiveStory} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			runtime := NewEphemeralRuntime()
			t.Cleanup(func() { _ = runtime.Close(context.Background()) })
			store, err := session.NewStore(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			broken, err := store.GetOrCreate("broken")
			if err != nil {
				t.Fatal(err)
			}
			healthy, err := store.GetOrCreate("healthy")
			if err != nil {
				t.Fatal(err)
			}
			model := &publicBackendTestModel{}
			options := agentrun.Options{
				AgentKind: kind, ProjectID: "history-project", Workspace: t.TempDir(),
				SessionID: broken.ID, RootAgentName: "root",
			}
			if kind == agentrun.AgentKindInteractiveStory {
				options.SessionID = ""
				options.StoryID, options.BranchID = "same-story", "broken-branch"
			}
			messages := make([]*agent.Message, 0, 397)
			for range 394 {
				messages = append(messages, agent.UserMessage("earlier history"))
			}
			messages = append(messages,
				agent.AssistantMessage("", []agent.ToolCall{
					{ID: "read-a", Function: agent.FunctionCall{Name: "read", Arguments: `{}`}},
					{ID: "read-b", Function: agent.FunctionCall{Name: "read", Arguments: `{}`}},
				}),
				agent.ToolMessage(agent.TextToolResult("result a"), "read-a"),
				agent.UserMessage("continue"),
			)
			original := clonePublicBackendMessages(messages)
			cycle := Cycle{
				Definition: agent.Definition{
					Name: "root", Model: model,
					ModelIdentity: agent.CapabilityIdentity{Kind: "model.history-isolation", Version: 1},
				},
				Conversation: invalidHistoryConversation{
					Conversation: agentconversation.NewSessionConversationForAgent(broken, nil, kind),
					messages:     messages,
				},
				Request: agentchatRequest("rejected-command", "continue"), Options: options,
			}
			for range 2 {
				operation, err := runtime.Start(ctx, StartRequest{Cycle: cycle})
				if operation != nil || !errors.Is(err, agent.ErrInvalidCanonicalMessages) ||
					!strings.Contains(err.Error(), "message 396 splits an incomplete tool-result batch") {
					t.Fatalf("invalid history admission operation=%v error=%v", operation, err)
				}
			}
			if _, err := runtime.Inspect(ctx, cycle); !errors.Is(err, agent.ErrInvalidCanonicalMessages) {
				t.Fatalf("invalid history inspection error=%v", err)
			}
			if !reflect.DeepEqual(messages, original) || len(model.inputs) != 0 ||
				len(runtime.public.runs) != 0 || len(runtime.public.registrations) != 0 {
				t.Fatal("rejection changed history, called a model, or retained a live run/registration")
			}
			cycle.Conversation = agentconversation.NewSessionConversationForAgent(healthy, nil, kind)
			cycle.Request = agentchatRequest("healthy-command", "continue healthy history")
			if kind == agentrun.AgentKindInteractiveStory {
				cycle.Options.BranchID = "healthy-branch"
			} else {
				cycle.Options.SessionID = healthy.ID
			}
			operation, err := runtime.Start(ctx, StartRequest{Cycle: cycle})
			if err != nil {
				t.Fatalf("unrelated conversation could not start: %v", err)
			}
			if outcome := operation.Wait(ctx); outcome.Status != agentrun.OutcomeCompleted {
				t.Fatalf("unrelated conversation did not complete: %+v", outcome)
			}
			// Replacing the failed projection with valid canonical history must
			// also let the original identity run without a process restart.
			cycle.Options = options
			cycle.Conversation = agentconversation.NewSessionConversationForAgent(broken, nil, kind)
			cycle.Request = agentchatRequest("recovered-command", "continue valid history")
			operation, err = runtime.Start(ctx, StartRequest{Cycle: cycle})
			if err != nil {
				t.Fatalf("failed identity remained poisoned: %v", err)
			}
			if outcome := operation.Wait(ctx); outcome.Status != agentrun.OutcomeCompleted {
				t.Fatalf("recovered identity did not complete: %+v", outcome)
			}
		})
	}
}
