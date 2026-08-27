package openairesponses

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
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
	items, err := requestMessage(agent.UserMessageWithAttachments("inspect", []agent.Attachment{{
		Name: "image.png", MediaType: "image/png", Path: path, Size: 3, SHA256: fmt.Sprintf("%x", sha256.Sum256([]byte("png"))),
	}}), providers.ModelConfig{})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(items)
	if err != nil {
		t.Fatal(err)
	}
	value := string(encoded)
	if !strings.Contains(value, "# Attached files") || !strings.Contains(value, "data:image/png;base64,cG5n") {
		t.Fatalf("native image request missing attachment content: %s", value)
	}
}
