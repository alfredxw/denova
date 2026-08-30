package attachment

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
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
	wantDigest := sha256.Sum256([]byte("hello"))
	if files[0].SHA256 != hex.EncodeToString(wantDigest[:]) {
		t.Fatalf("attachment digest = %q", files[0].SHA256)
	}
	for _, file := range files {
		if !strings.HasPrefix(file.Path, "attachments/v1/") || strings.Contains(file.Path, "\\") || filepath.IsAbs(file.Path) {
			t.Fatalf("attachment durable path is not portable: %q", file.Path)
		}
		if !strings.HasPrefix(file.RuntimePath, filepath.Join(root, "attachments", "v1")+string(filepath.Separator)) {
			t.Fatalf("copy escaped attachment root: %q", file.RuntimePath)
		}
		info, err := os.Stat(file.RuntimePath)
		if err != nil {
			t.Fatalf("copy missing: %v", err)
		}
		if info.Mode().Perm()&0o222 != 0 {
			t.Fatalf("attachment copy is writable: %o", info.Mode().Perm())
		}
	}
	if err := RemoveScope(root, SessionScope("session-1")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(files[0].RuntimePath); !os.IsNotExist(err) {
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

func TestMaterializeRejectsModifiedInputOnRetry(t *testing.T) {
	root := t.TempDir()
	uploads := []Upload{{Name: "notes.txt", DataURL: dataURL("text/plain", []byte("original"))}}
	files, err := Materialize(root, SessionScope("session-1"), "command-1", uploads)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(files[0].RuntimePath, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files[0].RuntimePath, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Materialize(root, SessionScope("session-1"), "command-1", uploads); err == nil || !strings.Contains(err.Error(), "immutable attachment copy differs") {
		t.Fatalf("retry error = %v", err)
	}
	content, err := os.ReadFile(files[0].RuntimePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "changed" {
		t.Fatalf("retry replaced modified input: %q", content)
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

func TestReadImageRequiresExactConversationScope(t *testing.T) {
	root := t.TempDir()
	files, err := Materialize(root, SessionScope("session-1"), "command-1", []Upload{{
		Name: "pixel.png", MediaType: "image/png", DataURL: dataURL("image/png", tinyPNG),
	}})
	if err != nil {
		t.Fatal(err)
	}
	image, err := ReadImage(root, SessionScope("session-1"), files[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if image.MediaType != "image/png" || !strings.EqualFold(image.SHA256, files[0].SHA256) || string(image.Data) != string(tinyPNG) {
		t.Fatalf("unexpected image: %#v", image)
	}
	if _, err := ReadImage(root, SessionScope("session-2"), files[0].ID); !errors.Is(err, ErrImageNotFound) {
		t.Fatalf("cross-scope read error = %v", err)
	}
	if _, err := ReadImage(root, StoryScope("session-1"), files[0].ID); !errors.Is(err, ErrImageNotFound) {
		t.Fatalf("cross-kind read error = %v", err)
	}
}

func TestReadImageRejectsNonImageAndInvalidID(t *testing.T) {
	root := t.TempDir()
	files, err := Materialize(root, StoryScope("story-1"), "command-1", []Upload{{
		Name: "notes.txt", MediaType: "text/plain", DataURL: dataURL("text/plain", []byte("hello")),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ReadImage(root, StoryScope("story-1"), files[0].ID); !errors.Is(err, ErrImagePreviewDisabled) {
		t.Fatalf("non-image read error = %v", err)
	}
	if _, err := ReadImage(root, StoryScope("story-1"), "../pixel.png"); !errors.Is(err, ErrImageNotFound) {
		t.Fatalf("invalid ID read error = %v", err)
	}
}

func dataURL(mediaType string, data []byte) string {
	return "data:" + mediaType + ";base64," + base64.StdEncoding.EncodeToString(data)
}

var tinyPNG = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
}
