package chat

import (
	"testing"
	"time"

	agentconversation "denova/internal/agents/conversation"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"

	agentevent "github.com/alfredxw/denova/agent/lifecycle/event"
	agentmodel "github.com/alfredxw/denova/agent/model"
	agentschema "github.com/alfredxw/denova/agent/schema"
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
	emit := func(payload agentevent.EventPayload) {
		projector.Project(agentevent.Event{RunID: "run", Payload: payload})
	}
	emit(agentevent.AssistantDelta{Delta: "Accepted progress. ", ResponseOrdinal: 1})
	emit(agentevent.ModelCompleted{})
	emit(agentevent.ToolStarted{CallID: "confirmed", Name: "read"})
	emit(agentevent.ToolFinished{CallID: "confirmed", Name: "read", Result: "recorded"})
	emit(agentevent.ThinkingDelta{Delta: "Broken reasoning", ResponseOrdinal: 2})
	emit(agentevent.AssistantDelta{Delta: "Broken partial", ResponseOrdinal: 2})
	emit(agentevent.ToolInputStarted{CallID: "unaccepted", Name: "write"})
	emit(agentevent.ModelRetry{Attempt: 1, MaxAttempts: 3, Delay: time.Second, ResponseOrdinal: 2, OutputState: agentmodel.ModelOutputPartial, Reason: "network"})
	emit(agentevent.AssistantDelta{Delta: "Recovered.", ResponseOrdinal: 3})
	emit(agentevent.ModelCompleted{})
	projector.Finalize(agentschema.ResultCompleted, "")
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
	confirmed, acceptedProgress := false, false
	for _, entry := range loaded.History() {
		if entry.Status == "discarded" || entry.ID == "unaccepted" || entry.Content == "Broken reasoning" || entry.Content == "Broken partial" {
			t.Fatalf("retracted preview should never be persisted to the journal: %#v", entry)
		}
		if entry.ID == "confirmed" && entry.Status == "success" {
			confirmed = true
		}
		if entry.Role == "assistant" && entry.Content == "Accepted progress. " && entry.DisplayPhase == session.DisplayPhaseProgress {
			acceptedProgress = true
		}
	}
	if !confirmed || !acceptedProgress {
		t.Fatalf("restored preview: confirmed=%v acceptedProgress=%v history=%#v", confirmed, acceptedProgress, loaded.History())
	}
}
