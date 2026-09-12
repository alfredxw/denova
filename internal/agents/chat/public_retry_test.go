package chat

import (
	"testing"
	"time"

	agentconversation "denova/internal/agents/conversation"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	agent "github.com/alfredxw/denova/agent"
)

func TestModelRetryRetractsOnlyUnacceptedResponse(t *testing.T) {
	dir := t.TempDir()
	store, err := session.NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("retry-preview")
	if err != nil {
		t.Fatal(err)
	}
	var retry agentrun.Event
	projector := NewPublicEventProjector(agentconversation.NewSessionConversation(sess), ChatRequest{}, agentrun.Options{}, func(event agentrun.Event) {
		if event.Type == "model_retry" {
			retry = event
		}
	})
	emit := func(payload agent.EventPayload) { projector.Project(agent.Event{RunID: "run", Payload: payload}) }
	emit(agent.AssistantDelta{Delta: "Accepted progress. ", ResponseOrdinal: 1})
	emit(agent.ModelCompleted{})
	emit(agent.ToolStarted{CallID: "confirmed", Name: "read"})
	emit(agent.ToolFinished{CallID: "confirmed", Name: "read", Result: "recorded"})
	emit(agent.ThinkingDelta{Delta: "Broken reasoning", ResponseOrdinal: 2})
	emit(agent.AssistantDelta{Delta: "Broken partial", ResponseOrdinal: 2})
	emit(agent.ToolInputStarted{CallID: "unaccepted", Name: "write"})
	emit(agent.ModelRetry{Attempt: 1, MaxAttempts: 3, Delay: time.Second, ResponseOrdinal: 2, OutputState: agent.ModelOutputPartial, Reason: "network"})
	emit(agent.AssistantDelta{Delta: "Recovered.", ResponseOrdinal: 3})
	emit(agent.ModelCompleted{})
	projector.Finalize(agent.ResultCompleted, "")
	content, thinking := projector.Output()
	if content != "Accepted progress. Recovered." || thinking != "" {
		t.Fatalf("output = %q / %q", content, thinking)
	}
	ids, ok := retry.Data.(map[string]any)["discard_ids"].([]string)
	if !ok || len(ids) != 3 {
		t.Fatalf("retry = %#v", retry)
	}
	if err := sess.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := session.NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := reopened.GetOrCreate(sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	confirmed, discarded := false, 0
	for _, entry := range loaded.History() {
		if entry.Status == "discarded" {
			discarded++
		}
		if entry.ID == "confirmed" && entry.Status == "success" {
			confirmed = true
		}
	}
	if !confirmed || discarded != 3 {
		t.Fatalf("restored preview: confirmed=%v discarded=%d", confirmed, discarded)
	}
}
