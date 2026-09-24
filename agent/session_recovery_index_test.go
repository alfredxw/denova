package agent

import (
	"encoding/json"
	"strings"
	"testing"

	agentsession "github.com/alfredxw/denova/agent/session"
)

func TestRecoveryIndexKeepsOnlyUnfinishedBodiesAndOriginalReceipts(t *testing.T) {
	var index RecoveryIndex
	apply := func(kind string, value any) {
		record, err := sessionRecord(kind, value)
		if err != nil {
			t.Fatal(err)
		}
		record.Revision = index.Revision + 1
		if err := index.Apply(record); err != nil {
			t.Fatal(err)
		}
	}
	input := persistedInput{Receipt: CommandReceipt{CommandID: "pending", Cursor: 1}, Kind: inputQueue, Hash: "original-idempotency-hash"}
	_, input.Input, _ = encodeInput(Input{Text: "queued body"})
	apply(sessionInputRecord, input)
	host := &HostData{Type: "product", Version: 1, Data: json.RawMessage(`{"scope":"original"}`)}
	apply(sessionInputUpdateRecord, persistedInputUpdate{CommandID: "pending", RunID: "active", Status: inputPending, HostData: host})
	apply(sessionInputUpdateRecord, persistedInputUpdate{CommandID: "pending", RunID: "active", Status: inputConsumed})
	tool := persistedTool{RunID: "active", CallID: "write", Name: "write", Arguments: json.RawMessage(`{"path":"target"}`), Started: true}
	apply(turnToolRecord, tool)
	replay := index.ReplayRecords()
	var binding, unknown bool
	for _, record := range replay {
		binding = binding || strings.Contains(string(record.Data), `"scope":"original"`)
		unknown = unknown || record.Kind == turnToolRecord
	}
	if !binding || !unknown {
		t.Fatal("pending ownership or unknown effect was lost")
	}
	tool.Result = &ToolResult{Status: ToolResultSuccess, ModelContent: "large tool body", DisplayContent: "large tool body"}
	apply(turnToolRecord, tool)
	apply(turnFinishedRecord, persistedTurn{RunID: "active", CommandID: "root", Status: ResultCompleted, Output: "completed output"})
	if len(index.ReplayRecords()) != 0 {
		t.Fatalf("terminal facts retained: %+v", index.ReplayRecords())
	}
	saved := index.Inputs["pending"]
	if saved.Hash != input.Hash || saved.Receipt != input.Receipt || saved.Status != inputConsumed || saved.RunID != "active" {
		t.Fatalf("receipt changed: %+v", saved)
	}
	encoded, err := json.Marshal(index)
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{"queued body", "large tool body", "completed output", "original\"}"} {
		if strings.Contains(string(encoded), body) {
			t.Fatalf("historical body retained: %s", body)
		}
	}
}

func TestCancelledInputReleasesBodyButRemainsIdempotent(t *testing.T) {
	store := agentsession.Memory()
	owner, err := New(t.Context(), Definition{Name: "queue", Model: &lifecycleModel{}}, WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close(t.Context())
	sess, err := owner.Session(t.Context(), NamedSession("cancelled"))
	if err != nil {
		t.Fatal(err)
	}
	input := Input{Text: strings.Repeat("large queued body", 100), IdempotencyKey: "queue"}
	queued, err := sess.Queue(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	control := QueueControlRequest{IdempotencyKey: "cancel"}
	receipt, err := queued.Cancel(t.Context(), control)
	if err != nil {
		t.Fatal(err)
	}
	if sess.inputs["queue"].input.Text != "" || sess.inputs["queue"].Input.Text != "" {
		t.Fatal("cancelled body remains resident")
	}
	if err := owner.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	owner, err = New(t.Context(), Definition{Name: "queue", Model: &lifecycleModel{}}, WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close(t.Context())
	sess, err = owner.Session(t.Context(), NamedSession("cancelled"))
	if err != nil {
		t.Fatal(err)
	}
	retried, err := sess.Queue(t.Context(), input)
	if err != nil || retried.Receipt() != queued.Receipt() {
		t.Fatalf("input retry: %v", err)
	}
	same, err := retried.Cancel(t.Context(), control)
	if err != nil || same != receipt {
		t.Fatalf("control retry: %v", err)
	}
}
