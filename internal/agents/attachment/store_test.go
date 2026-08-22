package attachment

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMaterializePersistsConversationOwnedCopies(t *testing.T) {
	root := t.TempDir()
	files, err := Materialize(root, SessionScope("session-1"), "command-1", []Upload{
		{Name: "../notes.md", MediaType: "text/markdown", DataURL: dataURL("text/markdown", []byte("hello"))},
		{Name: "pixel.png", MediaType: "image/png", DataURL: dataURL("image/png", tinyPNG)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || files[0].Name != "notes.md" || files[0].Size != 5 || files[1].MediaType != "image/png" {
		t.Fatalf("unexpected attachments: %#v", files)
	}
	for _, file := range files {
		if !strings.HasPrefix(file.Path, filepath.Join(root, "attachments", "v1")+string(filepath.Separator)) {
			t.Fatalf("copy escaped attachment root: %q", file.Path)
		}
		if _, err := os.Stat(file.Path); err != nil {
			t.Fatalf("copy missing: %v", err)
		}
	}
	if err := RemoveScope(root, SessionScope("session-1")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(files[0].Path); !os.IsNotExist(err) {
		t.Fatalf("scope removal left copy behind: %v", err)
	}
}

func TestMaterializeRejectsInvalidBatchWithoutLeavingCopies(t *testing.T) {
	root := t.TempDir()
	_, err := Materialize(root, StoryScope("story-1"), "command-1", []Upload{
		{Name: "valid.txt", DataURL: dataURL("text/plain", []byte("valid"))},
		{Name: "invalid.txt", DataURL: "not-a-data-url"},
	})
	if err == nil {
		t.Fatal("expected invalid attachment batch to fail")
	}
	entries, readErr := os.ReadDir(filepath.Join(root, "attachments"))
	if readErr != nil && !os.IsNotExist(readErr) {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("invalid batch left attachment data behind: %#v", entries)
	}
}

func TestMaterializeExactRetryPreservesModifiedCopy(t *testing.T) {
	root := t.TempDir()
	uploads := []Upload{{Name: "notes.txt", DataURL: dataURL("text/plain", []byte("original"))}}
	files, err := Materialize(root, SessionScope("session-1"), "command-1", uploads)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files[0].Path, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	retried, err := Materialize(root, SessionScope("session-1"), "command-1", uploads)
	if err != nil {
		t.Fatal(err)
	}
	if len(retried) != 1 || retried[0].Path != files[0].Path {
		t.Fatalf("retry attachments = %#v", retried)
	}
	content, err := os.ReadFile(files[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "changed" {
		t.Fatalf("retry overwrote modified copy: %q", content)
	}
}

func TestMaterializeDoesNotTrustNativeImageMIMEClaim(t *testing.T) {
	files, err := Materialize(t.TempDir(), SessionScope("session-1"), "command-1", []Upload{{
		Name: "not-an-image.png", MediaType: "image/png", DataURL: dataURL("image/png", []byte("plain text")),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].MediaType != "text/plain; charset=utf-8" {
		t.Fatalf("spoofed image attachment = %#v", files)
	}
}

func dataURL(mediaType string, data []byte) string {
	return "data:" + mediaType + ";base64," + base64.StdEncoding.EncodeToString(data)
}

var tinyPNG = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
}
