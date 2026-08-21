package imageapp

import (
	"context"
	"strings"
	"testing"

	"denova/config"
	agentchat "denova/internal/agents/chat"
	agentcontext "denova/internal/agents/context"
	agentconversation "denova/internal/agents/conversation"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"

	agent "github.com/alfredxw/denova/agent"
)

func TestImageAgentCommitterAcceptsPreparedStatelessContext(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := prepareImageAgentSession(store)
	if err != nil {
		t.Fatal(err)
	}
	message := imageAgentMessage(AgentGenerateRequest{Purpose: "interactive_image"})
	conversation := &imageAgentConversation{
		journal:       agentconversation.NewSessionConversationForAgent(sess, &config.Config{}, config.AgentKindImage),
		message:       message,
		contextBudget: agentcontext.DefaultBudget(),
	}
	identity := agentrun.CycleIdentity{CommandID: "image-command", OperationID: "image-run", Cycle: 1}
	conversation.BindAgentCycleIdentity(identity)
	committer, err := conversation.NewAgentConversationCommitter(
		agentrun.Options{AgentKind: config.AgentKindImage, SessionID: sess.ID},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := committer.MaterializeInput(context.Background(), agent.InputCommitRequest{
		Identity: agent.CommitIdentity{
			CommandID: string(identity.CommandID), RunID: string(identity.OperationID),
			Cycle: identity.Cycle, Stage: agent.CommitInput,
		},
		Hash: "image-input-hash", Input: agent.Text(message),
	}); err != nil {
		t.Fatal(err)
	}
	assembled, err := conversation.AssembleModelContext(context.Background(), message, agentcontext.ModelContextInput{
		UserMessage: message,
		Budget:      conversation.ModelContextBudget(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := committer.ApplyPreparedContext(context.Background(), agentchat.AgentContextPreparation{
		OriginalMessage: message,
		ModelContext:    assembled,
	}); err != nil {
		t.Fatalf("apply prepared Image Agent context: %v", err)
	}
	history := sess.History()
	if len(history) != 1 || history[0].Role != string(agent.User) || history[0].Content != message {
		t.Fatalf("canonical Image Agent input = %#v", history)
	}
	conversation.message = imageAgentMessage(AgentGenerateRequest{Purpose: "book_cover"})
	if err := committer.ApplyPreparedContext(context.Background(), agentchat.AgentContextPreparation{
		OriginalMessage: message,
		ModelContext:    assembled,
	}); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("mismatched prepared Image Agent context error = %v", err)
	}
}
