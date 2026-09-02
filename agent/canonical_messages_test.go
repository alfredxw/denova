package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	agentsession "github.com/alfredxw/denova/agent/session"
)

func TestHostCanonicalSessionPersistsOnlyCheckpointAndCapabilityDeltas(t *testing.T) {
	ctx := context.Background()
	base := agentsession.Memory()
	store := canonicalMessageTestStore{Store: base}
	owner, err := New(ctx, Definition{Name: "canonical-host", Model: &lifecycleModel{}}, WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	key := NamedSession("canonical-host-session")
	session, err := owner.Session(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	state := testContextStateFragment("revision-1", "current workspace")
	stateMessage := newContextStateMessage(state, strings.Repeat("a", 64), "initialize", "")
	toolCall := AssistantMessage("", []ToolCall{{
		ID: "call-1", Type: "function", Function: FunctionCall{Name: "inspect", Arguments: `{}`},
	}})
	toolResult := ToolMessage(TextToolResult("complete"), "call-1", WithToolName("inspect"))
	completion := UserMessage("child answer")
	completion.TaskCompletion = &TaskCompletionMessageMeta{
		CompletionID: "completion-1", Author: "researcher", Recipient: "parent",
	}
	physical := []*Message{UserMessage("request"), stateMessage, toolCall, toolResult, completion}
	if err := session.LoadCanonicalMessages(ctx, physical); err != nil {
		t.Fatal(err)
	}

	session.mu.Lock()
	transcript, err := decodeEngineTranscript(session.engineState)
	if err == nil && (len(transcript.Messages) != len(physical) || transcript.Messages[0].Content != stateMessage.Content || transcript.Messages[1].Content != "request") {
		err = errors.New("canonical Context State order was not restored")
	}
	if err == nil {
		err = session.persistTranscriptLocked(ctx)
	}
	if err == nil {
		session.capabilities["test.capability"] = json.RawMessage(`{"revision":1}`)
		err = session.persistCapabilitiesLocked(ctx)
	}
	if err == nil {
		delete(session.capabilities, "test.capability")
		err = session.persistCapabilitiesLocked(ctx)
	}
	session.mu.Unlock()
	if err != nil {
		t.Fatalf("persist canonical state: %v", err)
	}
	if accepted, enqueueErr := session.EnqueueTaskCompletion(ctx, testTaskCompletion("completion-1", "duplicate")); enqueueErr != nil || accepted {
		t.Fatalf("canonical completion redelivery accepted=%t err=%v", accepted, enqueueErr)
	}
	if err := owner.Close(ctx); err != nil {
		t.Fatal(err)
	}

	log, err := base.Open(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	counts := make(map[string]int)
	if _, err := log.Replay(ctx, func(record agentsession.Record) error {
		counts[record.Kind]++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if counts[sessionTranscriptRecord] != 0 || counts[sessionMessageCheckpointRecord] != 1 ||
		counts[sessionCapabilitySetRecord] != 1 || counts[sessionCapabilityDeleteRecord] != 1 ||
		counts[sessionTaskCompletionDeliveryRecord] != 0 {
		t.Fatalf("canonical records=%#v", counts)
	}
}

func TestCanonicalInitialLoadPreservesCapabilitiesFromSameJournal(t *testing.T) {
	ctx := context.Background()
	base := agentsession.Memory()
	owner, err := New(ctx, Definition{Name: "canonical-host", Model: &lifecycleModel{}}, WithSessionStore(canonicalMessageTestStore{Store: base}))
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close(ctx)
	session, err := owner.Session(ctx, NamedSession("canonical-capability-import"))
	if err != nil {
		t.Fatal(err)
	}
	session.mu.Lock()
	session.capabilities[compactionCapability] = json.RawMessage(`{"id":"released-compaction","revision":1}`)
	err = session.persistCapabilitiesLocked(ctx)
	session.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}

	if err := session.LoadCanonicalMessages(ctx, []*Message{UserMessage("released history")}); err != nil {
		t.Fatal(err)
	}
	session.mu.Lock()
	_, preserved := session.capabilities[compactionCapability]
	session.mu.Unlock()
	if !preserved {
		t.Fatal("same-journal compaction capability was discarded on its first canonical load")
	}
}

func TestCanonicalAppendAfterCheckpointPreservesCapabilities(t *testing.T) {
	ctx := context.Background()
	base := agentsession.Memory()
	store := canonicalMessageTestStore{Store: base}
	owner, err := New(ctx, Definition{Name: "canonical-host", Model: &lifecycleModel{}}, WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	key := NamedSession("canonical-prefix")
	session, err := owner.Session(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	first := UserMessage("stable prefix")
	if err := session.LoadCanonicalMessages(ctx, []*Message{first}); err != nil {
		t.Fatal(err)
	}
	session.mu.Lock()
	err = session.persistTranscriptLocked(ctx)
	if err == nil {
		session.capabilities[compactionCapability] = json.RawMessage(`{"id":"prefix-compaction","revision":1}`)
		err = session.persistCapabilitiesLocked(ctx)
	}
	session.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Close(ctx); err != nil {
		t.Fatal(err)
	}

	reopened, err := New(ctx, Definition{Name: "canonical-host", Model: &lifecycleModel{}}, WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close(ctx)
	session, err = reopened.Session(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := session.LoadCanonicalMessages(ctx, []*Message{first, AssistantMessage("new suffix", nil)}); err != nil {
		t.Fatal(err)
	}
	session.mu.Lock()
	_, preserved := session.capabilities[compactionCapability]
	session.mu.Unlock()
	if !preserved {
		t.Fatal("append-only canonical progress discarded a valid capability")
	}
}

func TestCanonicalRewriteInvalidatesOnlyHistoryDependentCapabilities(t *testing.T) {
	ctx := context.Background()
	base := agentsession.Memory()
	store := canonicalMessageTestStore{Store: base}
	owner, err := New(ctx, Definition{Name: "canonical-host", Model: &lifecycleModel{}}, WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	key := NamedSession("canonical-rewrite")
	session, err := owner.Session(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := session.LoadCanonicalMessages(ctx, []*Message{UserMessage("original history")}); err != nil {
		t.Fatal(err)
	}
	session.mu.Lock()
	err = session.persistTranscriptLocked(ctx)
	if err == nil {
		session.capabilities[compactionCapability] = json.RawMessage(`{"id":"old-compaction","revision":1}`)
		session.capabilities[TodoCapability] = json.RawMessage(`{"revision":1,"items":[]}`)
		err = session.persistCapabilitiesLocked(ctx)
	}
	session.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Close(ctx); err != nil {
		t.Fatal(err)
	}

	reopened, err := New(ctx, Definition{Name: "canonical-host", Model: &lifecycleModel{}}, WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close(ctx)
	session, err = reopened.Session(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := session.LoadCanonicalMessages(ctx, []*Message{UserMessage("rewritten history")}); err != nil {
		t.Fatal(err)
	}
	session.mu.Lock()
	_, compactionPresent := session.capabilities[compactionCapability]
	_, todoPresent := session.capabilities[TodoCapability]
	session.mu.Unlock()
	if compactionPresent {
		t.Fatal("history rewrite retained a source-indexed compaction capability")
	}
	if !todoPresent {
		t.Fatal("history rewrite discarded message-independent Todo state")
	}
}

type canonicalMessageTestStore struct{ agentsession.Store }

func (store canonicalMessageTestStore) Open(ctx context.Context, key agentsession.Key) (agentsession.Log, error) {
	log, err := store.Store.Open(ctx, key)
	if err != nil {
		return nil, err
	}
	return canonicalMessageTestLog{Log: log}, nil
}

type canonicalMessageTestLog struct{ agentsession.Log }

func (canonicalMessageTestLog) CanonicalMessages() bool { return true }
