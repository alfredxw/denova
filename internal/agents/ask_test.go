package agents

import (
	"context"
	"testing"
	"time"

	"denova/config"
	"denova/internal/agents/session"
	"denova/internal/agents/toolapproval"
)

func TestRunAskInteractionEmitsPendingAndResolvedAroundSameWaiter(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	sess, err := store.GetOrCreate("ask-events")
	if err != nil {
		t.Fatal(err)
	}
	events := make(chan Event, 2)
	conversation := NewSessionConversation(sess)
	conversation.BindAgentCycleIdentity(HarnessCycleIdentity{
		CommandID: "command-ask", OperationID: "operation-ask", Cycle: 4,
	})
	host := newRunAskInteraction(conversation, RunOptions{
		AgentKind: AgentKindIDE, TaskID: "task-ask",
	}, func(event Event) { events <- event })
	if host == nil {
		t.Fatal("interactive Ask host was not created")
	}
	result := make(chan session.AskResolution, 1)
	errs := make(chan error, 1)
	go func() {
		resolution, askErr := host.Ask(context.Background(), session.AskInteraction{
			ID: "call-ask", ToolCallID: "call-ask",
			Questions: []session.AskQuestion{{
				ID: "choice", Question: "Choose", Options: []session.AskOption{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}},
			}},
		})
		result <- resolution
		errs <- askErr
	}()

	pending := receiveAskEvent(t, events, "ask_pending")
	if pending.ID == "call-ask" || sess.PendingAsk(pending.ID) == nil || pending.Status != session.AskPending || pending.TaskID != "task-ask" ||
		pending.AgentCommandID != "command-ask" || pending.AgentOperationID != "operation-ask" || pending.AgentCycle != 4 {
		t.Fatalf("pending event did not follow durable state: %#v", pending)
	}
	if _, err := sess.ResolveAsk(context.Background(), pending.ID, session.AskAnswered, []session.AskAnswer{{
		QuestionID: "choice", SelectedOptionIDs: []string{"a"},
	}}, ""); err != nil {
		t.Fatal(err)
	}
	resolved := receiveAskEvent(t, events, "ask_resolved")
	if resolved.Status != session.AskAnswered || len(resolved.Answers) != 1 {
		t.Fatalf("resolved event = %#v", resolved)
	}
	if err := <-errs; err != nil {
		t.Fatal(err)
	}
	if resolution := <-result; resolution.Status != session.AskAnswered {
		t.Fatalf("tool resolution = %#v", resolution)
	}

	replayed, err := host.Ask(context.Background(), session.AskInteraction{
		ID: "call-ask", ToolCallID: "call-ask",
		Questions: []session.AskQuestion{{
			ID: "choice", Question: "Choose", Options: []session.AskOption{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}},
		}},
	})
	if err != nil || replayed.Status != session.AskAnswered {
		t.Fatalf("resolved replay = %#v, %v", replayed, err)
	}
	select {
	case event := <-events:
		t.Fatalf("resolved replay emitted duplicate event: %#v", event)
	default:
	}
}

func TestPersistentAskIDIsolatedByTaskAndExecution(t *testing.T) {
	first := persistentAskID("task-1", "tool-same")
	if first == persistentAskID("task-2", "tool-same") || first == persistentAskID("task-1", "tool-other") {
		t.Fatalf("ask identity was not isolated: %q", first)
	}
	if first != persistentAskID("task-1", "tool-same") {
		t.Fatal("ask identity is not deterministic")
	}
}

func TestRunAskInteractionPresentsHostGeneratedToolApproval(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	sess, err := store.GetOrCreate("approval-events")
	if err != nil {
		t.Fatal(err)
	}
	events := make(chan Event, 2)
	host := newRunAskInteraction(NewSessionConversation(sess), RunOptions{
		AgentKind: AgentKindIDE, TaskID: "approval-task",
	}, func(event Event) { events <- event })
	result := make(chan bool, 1)
	errs := make(chan error, 1)
	go func() {
		allowed, approveErr := host.ApproveTool(context.Background(), toolApprovalRequest{
			Mode: config.AgentApprovalAsk, ToolName: "bash",
			ProviderCallID: "provider-1", ExecutionID: "execution-1",
			Arguments: `{"command":"npm test"}`,
			Decision: toolapproval.Decision{
				Action: toolapproval.ActionPrompt, Risk: toolapproval.RiskHigh,
				RuleID: "bash_unlisted_command", Reason: "approval required", Command: "npm test",
			},
		})
		result <- allowed
		errs <- approveErr
	}()
	pending := receiveAskEvent(t, events, "ask_pending")
	if pending.Kind != session.AskKindToolApproval || pending.Approval == nil ||
		pending.Approval.ToolName != "bash" || pending.Approval.ArgsHash == "" {
		t.Fatalf("approval pending event = %#v", pending)
	}
	if _, err := sess.ResolveAsk(context.Background(), pending.ID, session.AskAnswered, []session.AskAnswer{{
		QuestionID: toolApprovalQuestionID, SelectedOptionIDs: []string{toolApprovalAllowID},
	}}, ""); err != nil {
		t.Fatal(err)
	}
	_ = receiveAskEvent(t, events, "ask_resolved")
	if err := <-errs; err != nil {
		t.Fatal(err)
	}
	if !<-result {
		t.Fatal("allow-once resolution was not granted")
	}
}

func TestGeneralAgentInstallsSessionBackedAskInteraction(t *testing.T) {
	t.Parallel()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	sess, err := store.GetOrCreate("general-ask")
	if err != nil {
		t.Fatal(err)
	}
	interaction := newRunAskInteraction(
		NewSessionConversationForAgent(sess, nil, AgentKindGeneral),
		RunOptions{AgentKind: AgentKindGeneral, TaskID: "general-task"},
		nil,
	)
	if interaction == nil {
		t.Fatal("General Agent did not install its interactive Ask host")
	}
}

func receiveAskEvent(t *testing.T, events <-chan Event, eventType string) session.AskInteraction {
	t.Helper()
	select {
	case event := <-events:
		if event.Type != eventType {
			t.Fatalf("event type = %q, want %q", event.Type, eventType)
		}
		interaction, ok := event.Data.(session.AskInteraction)
		if !ok {
			t.Fatalf("event payload = %#v", event.Data)
		}
		return interaction
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for %s", eventType)
		return session.AskInteraction{}
	}
}
