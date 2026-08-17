package handlers

import (
	"bytes"
	"strings"
	"testing"

	"denova/internal/workspace/filewatch"
)

func TestWriteProjectFileEvent(t *testing.T) {
	var output bytes.Buffer
	event := filewatch.Event{
		ProjectID: "project-demo",
		Workspace: "/books/demo",
		Source:    "watcher",
		Changes: []filewatch.Change{
			{Path: "chapters/ch01.md", Type: filewatch.ChangeUpdated},
		},
		Paths: []string{"chapters/ch01.md"},
	}
	if err := writeProjectFileEvent(&output, event); err != nil {
		t.Fatal(err)
	}
	got := output.String()
	if !strings.HasPrefix(got, "event: workspace-change\ndata: {") {
		t.Fatalf("unexpected SSE envelope: %q", got)
	}
	if !strings.Contains(got, `"project_id":"project-demo"`) || !strings.Contains(got, `"workspace":"/books/demo"`) || !strings.Contains(got, `"type":"updated"`) {
		t.Fatalf("event payload missing fields: %q", got)
	}
	if !strings.HasSuffix(got, "\n\n") {
		t.Fatalf("SSE event missing boundary: %q", got)
	}
}
