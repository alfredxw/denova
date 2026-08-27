package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestModelUserContentDescribesAttachedCopies(t *testing.T) {
	message := UserMessageWithAttachments("Inspect these files.", []Attachment{{
		ID: "att-1", Name: "notes.md", MediaType: "text/markdown", Size: 42, Path: "/data/notes.md",
	}})
	content := ModelUserContent(message)
	for _, expected := range []string{"Inspect these files.", "# Attached files", `name: "notes.md"`, `path: "/data/notes.md"`, "immutable input copies", "copy it into the workspace or create a new output artifact"} {
		if !strings.Contains(content, expected) {
			t.Fatalf("model content %q does not contain %q", content, expected)
		}
	}
}

func TestAttachmentDataURLVerifiesImmutableCopy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "image.png")
	if err := os.WriteFile(path, []byte("png"), 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte("png"))
	attachment := Attachment{Name: "image.png", MediaType: "image/png", Path: path, SHA256: hex.EncodeToString(digest[:])}
	value, err := AttachmentDataURL(attachment)
	if err != nil {
		t.Fatal(err)
	}
	if value != "data:image/png;base64,cG5n" {
		t.Fatalf("unexpected data URL: %q", value)
	}
	if err := os.WriteFile(path, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := AttachmentDataURL(attachment); err == nil || !strings.Contains(err.Error(), "immutable copy changed") {
		t.Fatalf("modified attachment error = %v", err)
	}
}

func TestNativeImageMediaTypeUsesProtocolIntersection(t *testing.T) {
	for _, mediaType := range []string{"image/jpeg", "image/png", "image/gif", "image/webp"} {
		if !IsNativeImageMediaType(mediaType) {
			t.Fatalf("expected %q to be native", mediaType)
		}
	}
	for _, mediaType := range []string{"image/svg+xml", "image/heic", "text/plain", ""} {
		if IsNativeImageMediaType(mediaType) {
			t.Fatalf("expected %q to remain a filesystem attachment", mediaType)
		}
	}
}
