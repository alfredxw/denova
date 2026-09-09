package agent

import (
	"context"
	"encoding/json"
	"testing"

	agentsession "github.com/alfredxw/denova/agent/session"
)

func TestCanonicalReloadCheckpointsHistoryBeforeNewCapabilities(t *testing.T) {
	ctx := context.Background()
	store := canonicalMessageTestStore{Store: agentsession.Memory()}
	open := func() (*Agent, *Session) {
		owner, err := New(ctx, Definition{Model: &lifecycleModel{}}, WithSessionStore(store))
		if err != nil {
			t.Fatal(err)
		}
		session, err := owner.Session(ctx, NamedSession("reloaded-canonical-history"))
		if err != nil {
			t.Fatal(err)
		}
		return owner, session
	}
	owner, session := open()
	if err := session.LoadCanonicalMessages(ctx, []*Message{UserMessage("previous projection")}); err != nil {
		t.Fatal(err)
	}
	session.mu.Lock()
	err := session.persistTranscriptLocked(ctx)
	session.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	history := []*Message{UserMessage("current canonical history")}
	if err := session.LoadCanonicalMessages(ctx, history); err != nil {
		t.Fatal(err)
	}
	// A subsequent structural operation stores a capability against the imported
	// history without running a model or writing another transcript checkpoint.
	checkpoint := json.RawMessage(`{"id":"current-compaction","revision":1}`)
	session.mu.Lock()
	session.capabilities[compactionCapability] = checkpoint
	err = session.persistCapabilitiesLocked(ctx)
	session.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Close(ctx); err != nil {
		t.Fatal(err)
	}
	owner, session = open()
	defer owner.Close(ctx)
	if err := session.LoadCanonicalMessages(ctx, history); err != nil {
		t.Fatal(err)
	}
	session.mu.RLock()
	preserved := string(session.capabilities[compactionCapability])
	session.mu.RUnlock()
	if preserved != string(checkpoint) {
		t.Fatalf("same canonical history invalidated a newly committed capability: %s", preserved)
	}
}
