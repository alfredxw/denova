package anthropicmessages

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/providers"
)

func TestRequestMessagesSendsAttachedImagesNatively(t *testing.T) {
	data, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aY9sAAAAASUVORK5CYII=")
	path := filepath.Join(t.TempDir(), "image.png")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	_, messages, err := requestMessages([]*agent.Message{agent.UserMessageWithAttachments("inspect", []agent.Attachment{{
		Name: "image.png", MediaType: "image/png", Path: path, Size: int64(len(data)), SHA256: fmt.Sprintf("%x", sha256.Sum256(data)),
	}})}, providers.ModelConfig{})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(messages)
	if err != nil {
		t.Fatal(err)
	}
	value := string(encoded)
	if !strings.Contains(value, "# Attached files") || !strings.Contains(value, `"media_type":"image/png"`) || !strings.Contains(value, `"data":"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aY9sAAAAASUVORK5CYII="`) {
		t.Fatalf("native image request missing attachment content: %s", value)
	}
}
