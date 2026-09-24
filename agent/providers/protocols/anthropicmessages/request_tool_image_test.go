package anthropicmessages

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/providers"
)

func TestRequestMessagesPreservesToolImage(t *testing.T) {
	data, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aY9sAAAAASUVORK5CYII=")
	path := filepath.Join(t.TempDir(), "image.png")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	message := agent.ToolMessage(agent.TextToolResult("Image read."), "read-image")
	message.Attachments = []agent.Attachment{{Name: "image.png", MediaType: "image/png", Path: path, Size: int64(len(data)), SHA256: fmt.Sprintf("%x", sha256.Sum256(data))}}
	_, items, err := requestMessages([]*agent.Message{message}, providers.ModelConfig{})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(items)
	if err != nil {
		t.Fatal(err)
	}
	var actual []struct {
		Role    string `json:"role"`
		Content []struct {
			Type      string `json:"type"`
			ToolUseID string `json:"tool_use_id"`
			Content   []struct {
				Type   string `json:"type"`
				Text   string `json:"text"`
				Source struct {
					Type      string `json:"type"`
					MediaType string `json:"media_type"`
					Data      string `json:"data"`
				} `json:"source"`
			} `json:"content"`
		} `json:"content"`
	}
	if err := json.Unmarshal(encoded, &actual); err != nil {
		t.Fatal(err)
	}
	if len(actual) != 1 || actual[0].Role != "user" || len(actual[0].Content) != 1 || actual[0].Content[0].Type != "tool_result" || actual[0].Content[0].ToolUseID != "read-image" || len(actual[0].Content[0].Content) != 2 {
		t.Fatalf("tool image was lost or detached from its call: %s", encoded)
	}
	parts := actual[0].Content[0].Content
	if parts[0].Text != "Image read." || parts[1].Type != "image" || parts[1].Source.Type != "base64" || parts[1].Source.MediaType != "image/png" || parts[1].Source.Data != base64.StdEncoding.EncodeToString(data) {
		t.Fatalf("tool image payload changed: %s", encoded)
	}
}
