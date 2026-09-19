package providers

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	agent "github.com/alfredxw/denova/agent"
)

func imageAttachment(t *testing.T, data []byte, mediaType string) agent.Attachment {
	t.Helper()
	path := filepath.Join(t.TempDir(), "original")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return agent.Attachment{Name: "reference", RuntimePath: path, MediaType: mediaType, Size: 1, SHA256: fmt.Sprintf("%x", sha256.Sum256(data))}
}

func TestPrepareImagePreservesOriginalAndMatchesVisualPolicy(t *testing.T) {
	var source bytes.Buffer
	picture := image.NewNRGBA(image.Rect(0, 0, 2000, 1500))
	picture.Set(0, 0, color.NRGBA{R: 200, A: 255})
	if err := (&png.Encoder{CompressionLevel: png.NoCompression}).Encode(&source, picture); err != nil {
		t.Fatal(err)
	}
	attachment := imageAttachment(t, source.Bytes(), "image/png")
	for _, name := range []string{"claude-sonnet-4-6", "gpt-5.4", "gpt-6-astra", "unknown-model"} {
		t.Run(name, func(t *testing.T) {
			config := ModelConfig{Model: name, BaseURL: "https://custom.example/v1"}
			prepared, err := config.PrepareImage(attachment, 2)
			if err != nil {
				t.Fatal(err)
			}
			data, err := base64.StdEncoding.DecodeString(prepared.Base64)
			if err != nil {
				t.Fatal(err)
			}
			dimensions, _, err := image.DecodeConfig(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			if name == "unknown-model" || name == "gpt-6-astra" {
				if !bytes.Equal(source.Bytes(), data) {
					t.Fatal("unnecessary image conversion")
				}
			} else {
				if dimensions.Width >= 2000 || dimensions.Height >= 1500 {
					t.Fatalf("image was not resized: %+v", dimensions)
				}
				tokens := config.InputEstimator().ImageTokens
				if tokens(dimensions.Width, dimensions.Height) > tokens(2000, 1500) {
					t.Fatal("prepared image exceeds its visual reserve")
				}
			}
			original, err := agent.ReadAttachmentImage(attachment)
			if err != nil || !bytes.Equal(original, source.Bytes()) {
				t.Fatal("original image was changed")
			}
		})
	}
}

func TestPrepareImagePreservesJPEGOrientation(t *testing.T) {
	var source bytes.Buffer
	if err := jpeg.Encode(&source, image.NewGray(image.Rect(0, 0, 2000, 1000)), nil); err != nil {
		t.Fatal(err)
	}
	// EXIF orientation 6 (90 degrees clockwise), one little-endian TIFF field.
	exif := []byte{'E', 'x', 'i', 'f', 0, 0, 'I', 'I', 42, 0, 8, 0, 0, 0, 1, 0, 0x12, 1, 3, 0, 1, 0, 0, 0, 6, 0, 0, 0, 0, 0, 0, 0}
	data := append([]byte{}, source.Bytes()[:2]...)
	data = append(data, 0xff, 0xe1, 0, byte(len(exif)+2))
	data = append(data, exif...)
	data = append(data, source.Bytes()[2:]...)
	attachment := imageAttachment(t, data, "image/jpeg")
	prepared, err := (ModelConfig{Model: "claude-sonnet-4-6"}).PrepareImage(attachment, 1)
	if err != nil {
		t.Fatal(err)
	}
	resized, _ := base64.StdEncoding.DecodeString(prepared.Base64)
	dimensions, _, err := image.DecodeConfig(bytes.NewReader(resized))
	if err != nil || dimensions.Height <= dimensions.Width || dimensions.Height >= 2000 {
		t.Fatalf("JPEG orientation lost: %+v, %v", dimensions, err)
	}
	if _, err := agent.ReadAttachmentImage(attachment); err != nil {
		t.Fatal(err)
	}
}

func TestPrepareImageRejectsChangedAndOversizedImages(t *testing.T) {
	var source bytes.Buffer
	if err := png.Encode(&source, image.NewGray(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	config := ModelConfig{Model: "claude-sonnet-4-6", Protocol: ProtocolAnthropicMessages}
	attachment := imageAttachment(t, source.Bytes(), "image/png")
	attachment.SHA256 = "changed"
	if _, err := config.PrepareImage(attachment, 1); err == nil {
		t.Fatal("changed image was sent")
	}
	// DecodeConfig accepts this valid IHDR, but full decode would require >64 MP.
	oversized := append([]byte{}, source.Bytes()...)
	binary.BigEndian.PutUint32(oversized[16:20], 8001)
	binary.BigEndian.PutUint32(oversized[20:24], 8001)
	binary.BigEndian.PutUint32(oversized[29:33], crc32.ChecksumIEEE(oversized[12:29]))
	attachment = imageAttachment(t, oversized, "image/png")
	if _, err := config.PrepareImage(attachment, 1); err == nil {
		t.Fatal("oversized image was decoded")
	}
	attachment = imageAttachment(t, source.Bytes(), "image/png")
	if _, err := config.PrepareImage(attachment, 601); err == nil {
		t.Fatal("excess image count was accepted")
	}
	config.BaseURL = "https://custom.example/v1"
	if _, err := config.PrepareImage(attachment, 601); err != nil {
		t.Fatalf("official limit applied to custom gateway: %v", err)
	}
}

func TestPrepareImageChecksActualEncodedBytes(t *testing.T) {
	var source bytes.Buffer
	if err := png.Encode(&source, image.NewGray(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	// The on-disk bytes are authoritative even when the descriptor says one byte.
	data := append(source.Bytes(), make([]byte, 8<<20)...)
	attachment := imageAttachment(t, data, "image/png")
	config := ModelConfig{Model: "claude-sonnet-4-6", Protocol: ProtocolAnthropicMessages}
	_, err := config.PrepareImage(attachment, 1)
	var rejected *APIError
	if !errors.As(err, &rejected) || rejected.ModelErrorReason() != agent.ModelImageInputRejectedReason {
		t.Fatalf("encoded image size did not reach the media limit: %v", err)
	}
	config.BaseURL = "https://custom.example/v1"
	if _, err := config.PrepareImage(attachment, 1); err != nil {
		t.Fatalf("official image byte limit applied to gateway: %v", err)
	}
}

func TestPrepareGIFFirstFramePreservesCanvas(t *testing.T) {
	palette := color.Palette{color.Transparent, color.RGBA{R: 255, A: 255}, color.RGBA{B: 255, A: 255}}
	first := image.NewPaletted(image.Rect(1, 0, 2, 1), palette)
	first.SetColorIndex(1, 0, 1)
	second := image.NewPaletted(image.Rect(0, 0, 3, 2), palette)
	second.SetColorIndex(1, 0, 2)
	var source bytes.Buffer
	if err := gif.EncodeAll(&source, &gif.GIF{Image: []*image.Paletted{first, second}, Delay: []int{1, 1}, Config: image.Config{ColorModel: palette, Width: 3, Height: 2}}); err != nil {
		t.Fatal(err)
	}
	attachment := imageAttachment(t, source.Bytes(), "image/gif")
	prepared, err := (ModelConfig{}).PrepareImage(attachment, 1)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := base64.StdEncoding.DecodeString(prepared.Base64)
	picture, _, err := image.Decode(bytes.NewReader(data))
	if err != nil || picture.Bounds() != image.Rect(0, 0, 3, 2) {
		t.Fatalf("wrong GIF canvas: %v", err)
	}
	if r, g, b, a := picture.At(1, 0).RGBA(); r != 65535 || g != 0 || b != 0 || a != 65535 {
		t.Fatal("first GIF frame was lost")
	}
	if _, _, _, a := picture.At(0, 0).RGBA(); a != 0 {
		t.Fatal("GIF frame was stretched across its canvas")
	}
	if _, err := agent.ReadAttachmentImage(attachment); err != nil {
		t.Fatal(err)
	}
}
