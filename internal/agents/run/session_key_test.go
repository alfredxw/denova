package agentrun

import (
	"strings"
	"testing"

	"denova/config"

	agent "github.com/alfredxw/denova/agent"
)

func TestSessionKeyForAgentSessionIsStableOpaqueAndIdentityScoped(t *testing.T) {
	key := agent.SessionKey{
		Namespace: "denova.writing.writing", ID: "session-123",
		Attributes: map[string]string{"workspace": "/private/book", "session_id": "session-123"},
	}
	first, err := SessionKeyForAgentSession(key)
	if err != nil {
		t.Fatal(err)
	}
	second, err := SessionKeyForAgentSession(agent.SessionKey{
		Namespace: key.Namespace, ID: key.ID,
		Attributes: map[string]string{"session_id": "session-123", "workspace": "/private/book"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if first != second || !strings.HasPrefix(first, sessionKeyPrefix) || len(first) != len(sessionKeyPrefix)+32 {
		t.Fatalf("session keys = %q, %q", first, second)
	}
	if strings.Contains(first, "session-123") || strings.Contains(first, "/private/book") {
		t.Fatalf("session key exposed raw identity: %q", first)
	}
	other := key
	other.ID = "session-456"
	other.Attributes = map[string]string{"workspace": "/private/book", "session_id": "session-456"}
	got, err := SessionKeyForAgentSession(other)
	if err != nil {
		t.Fatal(err)
	}
	if got == first {
		t.Fatalf("different Session reused provider key %q", got)
	}
}

func TestStandaloneSessionKeyUsesProjectAndSourceScope(t *testing.T) {
	cfg := &config.Config{ProjectID: "project-1", Workspace: "/book"}
	first := StandaloneSessionKey(cfg, config.AgentKindToolAgent, "chapter_split")
	if got := StandaloneSessionKey(cfg, config.AgentKindToolAgent, "chapter_split"); got != first {
		t.Fatalf("standalone session key changed: %q != %q", got, first)
	}
	if got := StandaloneSessionKey(cfg, config.AgentKindToolAgent, "lore_classification"); got == first {
		t.Fatalf("different standalone source reused session key %q", got)
	}
}
