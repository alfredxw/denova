package app

import (
	"testing"

	agent "github.com/alfredxw/denova/agent"

	"denova/internal/interactive"
)

func TestRedactInteractiveSnapshotAttachmentPaths(t *testing.T) {
	attachment := agent.Attachment{ID: "att-1", Name: "notes.md", MediaType: "text/markdown", Size: 12, Path: "/private/user-data/notes.md"}
	snapshot := interactive.Snapshot{
		Turns:               []interactive.TurnEvent{{Attachments: []agent.Attachment{attachment}}},
		PendingPlayerInputs: []interactive.PlayerInputAcceptedEvent{{Attachments: []agent.Attachment{attachment}}},
		CurrentTurn:         &interactive.TurnEvent{Attachments: []agent.Attachment{attachment}},
	}

	redactInteractiveSnapshotAttachmentPaths(&snapshot)

	for label, files := range map[string][]agent.Attachment{
		"turn":          snapshot.Turns[0].Attachments,
		"pending input": snapshot.PendingPlayerInputs[0].Attachments,
		"current turn":  snapshot.CurrentTurn.Attachments,
	} {
		if len(files) != 1 || files[0].Path != "" || files[0].ID != attachment.ID || files[0].Name != attachment.Name {
			t.Fatalf("%s attachments = %#v", label, files)
		}
	}
	if attachment.Path == "" {
		t.Fatal("redaction mutated the source attachment")
	}
}
