package session

import (
	"testing"

	"denova/config"
)

func TestAgentInstanceSessionIsolatesCustomAgentJournal(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	builtin, err := AgentInstanceSession(store, config.AgentKindImage, "")
	if err != nil {
		t.Fatal(err)
	}
	custom, err := AgentInstanceSession(store, config.AgentKindImage, "concept-artist")
	if err != nil {
		t.Fatal(err)
	}
	if builtin.ID != "image-agent" || custom.ID != "image-agent-concept-artist" || builtin.ID == custom.ID {
		t.Fatalf("Image Agent journal IDs builtin=%q custom=%q", builtin.ID, custom.ID)
	}
}
