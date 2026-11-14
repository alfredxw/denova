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
	"slices"
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
// there before the turn starts. Current and historical images share the staging
// step but keep disjoint short-name prefixes.
func TestStageAttachmentsCopiesReferencesIntoWorkdir(t *testing.T) {
	dir := t.TempDir()
	image := referenceAttachment(t)
	media, err := stageAttachments(external.Input{
		Attachments: []agentschema.Attachment{image},
		History: []external.Message{
			{Role: "user", Attachments: []agentschema.Attachment{image}},
			{Role: "assistant", ToolImages: []agentschema.Attachment{image}},
		},
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(media.Request) != 1 || media.Request[0] != "ref1.png" {
		t.Fatalf("request names: %#v", media.Request)
	}
	if len(media.History) != 2 || !slices.Equal(media.History[0], []string{"hist1.png"}) || !slices.Equal(media.History[1], []string{"hist2.png"}) {
		t.Fatalf("history names: %#v", media.History)
	}
	for _, name := range append(append([]string{}, media.Request...), media.History[0]...) {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil || info.Size() == 0 {
			t.Fatalf("staged reference: %v %v", info, err)
		}
	}
}

func TestStageAttachmentsRejectsNonImage(t *testing.T) {
	if _, err := stageAttachments(external.Input{Attachments: []agentschema.Attachment{{Name: "notes.txt", MediaType: "text/plain"}}}, t.TempDir()); err == nil {
		t.Fatal("non-image reference was accepted")
	}
}

// Non-image historical media has no reference channel; only images are staged.
func TestStageAttachmentsSkipsNonImageHistory(t *testing.T) {
	image := referenceAttachment(t)
	media, err := stageAttachments(external.Input{History: []external.Message{
		{Role: "user", Attachments: []agentschema.Attachment{{Name: "notes.txt", MediaType: "text/plain"}, image}},
	}}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(media.History[0], []string{"hist1.png"}) {
		t.Fatalf("history names: %#v", media.History)
	}
}
