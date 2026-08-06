package agentrun

import (
	"strings"
	"testing"

	runstate "github.com/alfredxw/denova/agent/runtime"

	"denova/config"
)

func TestSessionKeyForBindingIsStableOpaqueAndIdentityScoped(t *testing.T) {
	binding := runstate.BindingRef{
		Kind: "writing", Profile: "writing", Key: "session-123",
		Labels: map[string]string{"workspace": "/private/book", "session_id": "session-123"},
	}
	first := SessionKeyForBinding(binding)
	second := SessionKeyForBinding(binding.Clone())
	if first != second || !strings.HasPrefix(first, sessionKeyPrefix) || len(first) != len(sessionKeyPrefix)+32 {
		t.Fatalf("session keys = %q, %q", first, second)
	}
	if strings.Contains(first, "session-123") || strings.Contains(first, "/private/book") {
		t.Fatalf("session key exposed raw identity: %q", first)
	}
	other := binding.Clone()
	other.Key = "session-456"
	other.Labels["session_id"] = "session-456"
	if got := SessionKeyForBinding(other); got == first {
		t.Fatalf("different binding reused session key %q", got)
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
