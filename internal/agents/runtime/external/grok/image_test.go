package grok

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	stddraw "image/draw"
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"

	agentschema "github.com/alfredxw/denova/agent/schema"
)

// fixtureAttachment builds a flat-fill reference image (compresses well).
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

// A reference image must be compressed to fit the prompt payload budget rather
// than being dropped or failing the turn, and the block must carry the field
// names the installed CLI parses.
func TestPromptContentCompressesImages(t *testing.T) {
	payload, err := promptContent("Describe this image.", []agentschema.Attachment{fixtureAttachment(t, 2000, 2000)})
	if err != nil {
		t.Fatal(err)
	}
	if len(payload) > maxPromptPayloadBytes {
		t.Fatalf("payload exceeded its limit: %d", len(payload))
	}
	var blocks []struct {
		Type     string `json:"type"`
		Data     string `json:"data"`
		MIME     string `json:"mimeType"`
		MIMESpec string `json:"mime_type"`
	}
	if err := json.Unmarshal([]byte(payload), &blocks); err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 2 || blocks[0].Type != "text" || blocks[1].Type != "image" || blocks[1].MIME != "image/jpeg" || blocks[1].Data == "" {
		t.Fatalf("blocks: %#v", blocks)
	}
	if blocks[1].MIMESpec != "" {
		t.Fatalf("mime_type spelling is rejected by the installed CLI: %#v", blocks)
	}
}

func TestPromptContentRejectsNonImage(t *testing.T) {
	if _, err := promptContent("x", []agentschema.Attachment{{Name: "notes.txt", MediaType: "text/plain"}}); err == nil {
		t.Fatal("non-image attachment was accepted")
	}
}

// Several images share one budget instead of each claiming the whole payload.
func TestPromptContentSharesBudgetAcrossImages(t *testing.T) {
	payload, err := promptContent("Compare these images.", []agentschema.Attachment{fixtureAttachment(t, 1500, 1500), fixtureAttachment(t, 1500, 1500)})
	if err != nil {
		t.Fatal(err)
	}
	if len(payload) > maxPromptPayloadBytes {
		t.Fatalf("payload exceeded its limit: %d", len(payload))
	}
}

// photoAttachment mimics a real reference photo: JPEG noise barely compresses,
// so the payload is large even after the quality ladder.
func photoAttachment(t *testing.T, size int) agentschema.Attachment {
	t.Helper()
	path := filepath.Join(t.TempDir(), "photo.jpg")
	source := image.NewRGBA(image.Rect(0, 0, size, size))
	rng := rand.New(rand.NewPCG(1, 42))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			source.Set(x, y, color.RGBA{R: uint8(rng.IntN(256)), G: uint8(rng.IntN(256)), B: uint8(rng.IntN(256)), A: 255})
		}
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := jpeg.Encode(file, source, &jpeg.Options{Quality: 95}); err != nil {
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
	return agentschema.Attachment{Name: "photo.jpg", Path: path, MediaType: "image/jpeg", Size: int64(len(raw)), SHA256: hex.EncodeToString(digest[:])}
}

// A realistic 512×512 photo cannot fit in the Windows command line (max 32766
// content UTF-16 code units, null terminator excluded) even after compression:
// inline transport is structurally impossible there, which is why the payload
// must live in the prompt file.
func TestPromptContentPhotoExceedsWindowsLimit(t *testing.T) {
	payload, err := promptContent("Describe this image.", []agentschema.Attachment{photoAttachment(t, 512)})
	if err != nil {
		t.Fatal(err)
	}
	if len(payload) > maxPromptPayloadBytes {
		t.Fatalf("payload exceeded its budget: %d", len(payload))
	}
	// The payload is pure ASCII base64 JSON, so len(payload) equals its UTF-16
	// code-unit count; adjust the comparison if the text block ever carries
	// non-ASCII characters. The threshold also depends on the compression
	// ladder's floor: shrinking photoAttachment below 512×512 or adding the
	// next quality step would need this number revisited.
	if len(payload) <= 32_766 {
		t.Fatalf("photo payload unexpectedly fits a Windows command line: %d", len(payload))
	}
}