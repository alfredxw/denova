package openairesponses

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
	items, err := requestMessage(message, providers.ModelConfig{}, 1)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(items)
	if err != nil {
		t.Fatal(err)
	}
	var actual []struct {
		Type   string `json:"type"`
		CallID string `json:"call_id"`
		Output []struct {
			Type     string `json:"type"`
			Text     string `json:"text"`
			ImageURL string `json:"image_url"`
		} `json:"output"`
	}
	if err := json.Unmarshal(encoded, &actual); err != nil {
		t.Fatalf("tool output must contain native content parts: %s: %v", encoded, err)
	}
	if len(actual) != 1 || actual[0].Type != "function_call_output" || actual[0].CallID != "read-image" || len(actual[0].Output) != 2 || actual[0].Output[0].Text != "Image read." || actual[0].Output[1].Type != "input_image" || actual[0].Output[1].ImageURL != "data:image/png;base64,"+base64.StdEncoding.EncodeToString(data) {
		t.Fatalf("tool image was lost or detached from its call: %s", encoded)
	}
}
