package openaichatcompletions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/providers"
)

func TestRequestMessageSendsAttachedImagesNatively(t *testing.T) {
	path := filepath.Join(t.TempDir(), "image.png")
	if err := os.WriteFile(path, []byte("png"), 0o600); err != nil {
		t.Fatal(err)
	}
	message, err := requestMessage(agent.UserMessageWithAttachments("inspect", []agent.Attachment{{
		Name: "image.png", MediaType: "image/png", Path: path, Size: 3,
	}}), Compatibility{}, providers.ThinkingLevelOff)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	value := string(encoded)
	if !strings.Contains(value, "# Attached files") || !strings.Contains(value, "data:image/png;base64,cG5n") {
		t.Fatalf("native image request missing attachment content: %s", value)
	}
}
