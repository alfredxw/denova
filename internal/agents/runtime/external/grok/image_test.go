package grok

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	stddraw "image/draw"
	"os"
	"path/filepath"
	"testing"

	agentschema "github.com/alfredxw/denova/agent/schema"
)

func fixtureAttachment(t *testing.T, width, height int) agentschema.Attachment {
	t.Helper()
	path := filepath.Join(t.TempDir(), "reference.png")
	source := image.NewRGBA(image.Rect(0, 0, width, height))
	stddraw.Draw(source, source.Bounds(), &image.Uniform{C: color.RGBA{R: 200, G: 40, B: 40, A: 255}}, image.Point{}, stddraw.Src)
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

// A reference image must reach the model compressed to fit the single-argument
// limit rather than being dropped or failing the turn.
func TestPromptArgumentCompressesImages(t *testing.T) {
	argument, err := promptArgument("Describe this image.", []agentschema.Attachment{fixtureAttachment(t, 2000, 2000)})
	if err != nil {
		t.Fatal(err)
	}
	if len(argument) > maxPromptArgumentBytes {
		t.Fatalf("argument exceeded its limit: %d", len(argument))
	}
	var blocks []struct {
		Type string `json:"type"`
		Data string `json:"data"`
		MIME string `json:"mimeType"`
	}
	if err := json.Unmarshal([]byte(argument), &blocks); err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 2 || blocks[0].Type != "text" || blocks[1].Type != "image" || blocks[1].MIME != "image/jpeg" || blocks[1].Data == "" {
		t.Fatalf("blocks: %#v", blocks)
	}
}

func TestPromptArgumentRejectsNonImage(t *testing.T) {
	if _, err := promptArgument("x", []agentschema.Attachment{{Name: "notes.txt", MediaType: "text/plain"}}); err == nil {
		t.Fatal("non-image attachment was accepted")
	}
}

// Several images share one budget instead of each claiming the whole argument.
func TestPromptArgumentSharesBudgetAcrossImages(t *testing.T) {
	argument, err := promptArgument("Compare these images.", []agentschema.Attachment{fixtureAttachment(t, 1500, 1500), fixtureAttachment(t, 1500, 1500)})
	if err != nil {
		t.Fatal(err)
	}
	if len(argument) > maxPromptArgumentBytes {
		t.Fatalf("argument exceeded its limit: %d", len(argument))
	}
}
