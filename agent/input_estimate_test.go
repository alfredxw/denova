package agent

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInputEstimatorDecodesEveryNativeImageFormat(t *testing.T) {
	picture := image.NewNRGBA(image.Rect(0, 0, 31, 47))
	picture.Set(0, 0, color.NRGBA{R: 20, G: 80, B: 180, A: 255})
	for _, format := range []string{"png", "jpeg", "gif", "webp"} {
		t.Run(format, func(t *testing.T) {
			var encoded bytes.Buffer
			var err error
			width, height := 31, 47
			switch format {
			case "png":
				err = png.Encode(&encoded, picture)
			case "jpeg":
				err = jpeg.Encode(&encoded, picture, nil)
			case "gif":
				err = gif.Encode(&encoded, picture, nil)
			case "webp":
				var data []byte
				// One-pixel lossless WebP; decoding uses the existing x/image library.
				data, err = base64.StdEncoding.DecodeString("UklGRiIAAABXRUJQVlA4TBEAAAAvAAAAAAfQ//73v/+BiOh/AAA=")
				encoded.Write(data)
				width, height = 1, 1
			}
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "input."+format)
			if err := os.WriteFile(path, encoded.Bytes(), 0o600); err != nil {
				t.Fatal(err)
			}
			message := UserMessageWithAttachments("Inspect", []Attachment{{
				Name: "input." + format, MediaType: "image/" + format,
				Path: "attachments/original." + format, RuntimePath: path,
				// Deliberately stale descriptor: actual file bytes own transport size.
				Size: 1,
			}})
			estimator := InputEstimator{ImageTokens: func(w, h int) int {
				if w != width || h != height {
					t.Errorf("decoded dimensions = %dx%d, want %dx%d", w, h, width, height)
				}
				return w * h
			}}
			size, err := estimator.Estimate([]*Message{message}, nil)
			if err != nil {
				t.Fatal(err)
			}
			text := EstimateRequestTextTokens([]*Message{message}, nil)
			if size.Tokens != text+width*height || size.Bytes < base64.StdEncoding.EncodedLen(encoded.Len()) {
				t.Fatalf("image accounting = %+v, text tokens = %d", size, text)
			}
			if message.Attachments[0].Size != 1 || message.Attachments[0].Path != "attachments/original."+format {
				t.Fatal("estimation changed durable attachment metadata")
			}
		})
	}
}

func TestInputEstimatorReportsUnavailableImages(t *testing.T) {
	for _, scenario := range []string{"missing", "invalid", "directory"} {
		t.Run(scenario, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "reference.png")
			if scenario == "invalid" {
				if err := os.WriteFile(path, []byte("not an image"), 0o600); err != nil {
					t.Fatal(err)
				}
			} else if scenario == "directory" {
				if err := os.Mkdir(path, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			_, err := (InputEstimator{}).Estimate([]*Message{UserMessageWithAttachments("Inspect", []Attachment{{
				Name: "reference.png", MediaType: "image/png", RuntimePath: path,
			}})}, nil)
			if err == nil || !strings.Contains(err.Error(), "reference.png") {
				t.Fatalf("unavailable image did not produce a locatable error: %v", err)
			}
		})
	}
}

type imageEstimateModel struct{ BaseChatModel }

func (imageEstimateModel) InputEstimator() InputEstimator {
	return InputEstimator{ImageTokens: func(w, h int) int { return w * h }}
}

func TestSnapshotForksPreserveImageEstimator(t *testing.T) {
	path := filepath.Join(t.TempDir(), "input.png")
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewGray(image.Rect(0, 0, 31, 47))); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	message := UserMessageWithAttachments("Inspect", []Attachment{{Name: "input.png", MediaType: "image/png", Path: path}})
	snapshot := (&ModelCall{Model: imageEstimateModel{}, Messages: []*Message{message}}).Snapshot()
	for _, fork := range []*ModelRequestSnapshot{
		snapshot, snapshot.Append(UserMessage("Evaluate")), snapshot.WithOptions(WithoutTools()), snapshot.WithMessages([]*Message{message}),
	} {
		size, err := fork.EstimateInput()
		if err != nil {
			t.Fatal(err)
		}
		if size.Tokens-EstimateRequestTextTokens(fork.Messages(), fork.ResolvedOptions().Tools) != 31*47 {
			t.Fatalf("fork lost the captured visual policy: %+v", size)
		}
		inspectionSize, err := modelRequestInspection(fork).EstimateInput()
		if err != nil || inspectionSize != size {
			t.Fatalf("inspection disagrees with execution: %+v, want %+v, error %v", inspectionSize, size, err)
		}
	}
}
