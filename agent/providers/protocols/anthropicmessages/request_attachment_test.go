package anthropicmessages

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/providers"
)

func TestRequestMessagesSendsAttachedImagesNatively(t *testing.T) {
	path := filepath.Join(t.TempDir(), "image.png")
	if err := os.WriteFile(path, []byte("png"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, messages, err := requestMessages([]*agent.Message{agent.UserMessageWithAttachments("inspect", []agent.Attachment{{
		Name: "image.png", MediaType: "image/png", Path: path, Size: 3,
	}})}, providers.ModelConfig{})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(messages)
	if err != nil {
		t.Fatal(err)
	}
	value := string(encoded)
	if !strings.Contains(value, "# Attached files") || !strings.Contains(value, `"media_type":"image/png"`) || !strings.Contains(value, `"data":"cG5n"`) {
		t.Fatalf("native image request missing attachment content: %s", value)
	}
}
