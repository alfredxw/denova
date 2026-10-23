package antigravity

import (
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/color"
	"image/png"
	stddraw "image/draw"
	"os"
	"path/filepath"
	"testing"

	"denova/internal/agents/runtime/external"

	agentschema "github.com/alfredxw/denova/agent/schema"
)

func referenceAttachment(t *testing.T) agentschema.Attachment {
	t.Helper()
	path := filepath.Join(t.TempDir(), "reference.png")
	source := image.NewRGBA(image.Rect(0, 0, 32, 32))
	stddraw.Draw(source, source.Bounds(), &image.Uniform{C: color.RGBA{R: 255, A: 255}}, image.Point{}, stddraw.Src)
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(file, source); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	return agentschema.Attachment{Name: "reference.png", Path: path, MediaType: "image/png", Size: int64(len(raw)), SHA256: hex.EncodeToString(digest[:])}
}

// The CLI resolves "@name" against its cwd, so the immutable copy must be staged
// there before the turn starts.
func TestStageAttachmentsCopiesReferencesIntoWorkdir(t *testing.T) {
	dir := t.TempDir()
	names, err := stageAttachments(external.Input{Attachments: []agentschema.Attachment{referenceAttachment(t)}}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0] != "ref1.png" {
		t.Fatalf("names: %#v", names)
	}
	info, err := os.Stat(filepath.Join(dir, names[0]))
	if err != nil || info.Size() == 0 {
		t.Fatalf("staged reference: %v %v", info, err)
	}
}

func TestStageAttachmentsRejectsNonImage(t *testing.T) {
	if _, err := stageAttachments(external.Input{Attachments: []agentschema.Attachment{{Name: "notes.txt", MediaType: "text/plain"}}}, t.TempDir()); err == nil {
		t.Fatal("non-image reference was accepted")
	}
}
