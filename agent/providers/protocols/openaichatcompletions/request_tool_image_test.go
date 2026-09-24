package openaichatcompletions

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/providers"
)

func TestRequestMessagesPreservesToolImageAfterWholeBatch(t *testing.T) {
	data, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aY9sAAAAASUVORK5CYII=")
	path := filepath.Join(t.TempDir(), "image.png")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	message := agent.ToolMessage(agent.TextToolResult("Image read."), "read-image")
	message.Attachments = []agent.Attachment{{Name: "image.png", MediaType: "image/png", Path: path, Size: int64(len(data)), SHA256: fmt.Sprintf("%x", sha256.Sum256(data))}}
	input := []*agent.Message{
		agent.AssistantMessage("", []agent.ToolCall{
			{ID: "read-image", Type: "function", Function: agent.FunctionCall{Name: "read", Arguments: `{"path":"image.png"}`}},
			{ID: "read-text", Type: "function", Function: agent.FunctionCall{Name: "read", Arguments: `{"path":"notes.txt"}`}},
		}),
		message,
		agent.ToolMessage(agent.TextToolResult("Notes."), "read-text"),
		agent.AssistantMessage("Image understood.", nil),
	}
	items, err := requestMessages(input, Compatibility{}, providers.ModelConfig{})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(items)
	if err != nil {
		t.Fatal(err)
	}
	var actual []struct {
		Role       string          `json:"role"`
		ToolCallID string          `json:"tool_call_id"`
		Content    json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(encoded, &actual); err != nil {
		t.Fatal(err)
	}
	roles := make([]string, len(actual))
	for i, item := range actual {
		roles[i] = item.Role
	}
	if !reflect.DeepEqual(roles, []string{"assistant", "tool", "tool", "user", "assistant"}) {
		t.Fatalf("image projection must follow all paired tool results: %s", encoded)
	}
	if actual[1].ToolCallID != "read-image" || string(actual[1].Content) != `"Image read."` || actual[2].ToolCallID != "read-text" {
		t.Fatalf("tool result pairing changed: %s", encoded)
	}
	if !strings.Contains(string(actual[3].Content), `"type":"image_url"`) || !strings.Contains(string(actual[3].Content), "data:image/png;base64,"+base64.StdEncoding.EncodeToString(data)) || !strings.Contains(string(actual[3].Content), "read-image") {
		t.Fatalf("tool image source or payload missing: %s", encoded)
	}
	if len(input) != 4 || input[1].Role != agent.ToolRole || input[1].Attachments[0].Path != path {
		t.Fatal("wire projection mutated canonical history")
	}
}
