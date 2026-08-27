package app

import (
	"testing"

	agent "github.com/alfredxw/denova/agent"
)

func TestInteractiveTaskInfoProjectsAttachmentDescriptors(t *testing.T) {
	original := agent.Attachment{
		ID: "att-1", Name: "map.png", MediaType: "image/png", Size: 42,
		Path: "/private/input/map.png", SHA256: "digest",
	}
	identity := interactiveStartIdentity{request: InteractiveAgentStartRequest{
		CommandID: "command-1", StoryID: "story-1", BranchID: "main",
		Message: "Inspect the map", AttachedFiles: []agent.Attachment{original},
	}}

	info := identity.taskInfo("task-1")
	if len(info.Attachments) != 1 {
		t.Fatalf("attachments = %#v", info.Attachments)
	}
	if got := info.Attachments[0]; got.ID != original.ID || got.Name != original.Name || got.MediaType != original.MediaType || got.Size != original.Size || got.Path != "" || got.SHA256 != "" {
		t.Fatalf("public attachment descriptor = %#v", got)
	}
	if identity.request.AttachedFiles[0].Path != original.Path || identity.request.AttachedFiles[0].SHA256 != original.SHA256 {
		t.Fatalf("task projection mutated canonical attachment: %#v", identity.request.AttachedFiles[0])
	}
}
