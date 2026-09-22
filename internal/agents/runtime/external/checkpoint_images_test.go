package external

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	externaljournal "denova/internal/agents/runtime/external/journal"
	agent "github.com/alfredxw/denova/agent"
)

func checkpointImage(t *testing.T, compression png.CompressionLevel) agent.Attachment {
	t.Helper()
	var body bytes.Buffer
	encoder := png.Encoder{CompressionLevel: compression}
	if err := encoder.Encode(&body, image.NewNRGBA(image.Rect(0, 0, 256, 256))); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "reference.png")
	if err := os.WriteFile(path, body.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(body.Bytes())
	return agent.Attachment{ID: "reference", Name: "reference.png", Path: "reference.png", RuntimePath: path,
		MediaType: "image/png", Size: int64(body.Len()), SHA256: hex.EncodeToString(digest[:])}
}

func TestExternalHistoryImageCompressionDoesNotTriggerSummary(t *testing.T) {
	for _, compression := range []png.CompressionLevel{png.BestCompression, png.NoCompression} {
		file := checkpointImage(t, compression)
		input := Input{History: []Message{{Role: "user", Text: "Use this reference", Attachments: []agent.Attachment{file}, Cursor: 1}}}
		calls := 0
		prepared, err := (HistoryPreparation{Input: input, ProviderInputMaxBytes: 8192, Adapter: adapterFunc(func(context.Context, Input, Host) (Result, error) {
			calls++
			return Result{Text: "A reference image."}, nil
		})}).Prepare(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if calls != 0 || len(prepared.History) != 1 || len(prepared.History[0].Attachments) != 1 {
			t.Fatalf("compression %d discarded an image: summary calls=%d, history=%+v", compression, calls, prepared.History)
		}
	}
}

func TestExternalSummaryReceivesUserAndToolImages(t *testing.T) {
	file := checkpointImage(t, png.NoCompression)
	input := Input{History: []Message{
		{Role: "user", Text: strings.Repeat("history ", 20000), Attachments: []agent.Attachment{file}, ToolImages: []agent.Attachment{file}, Cursor: 1},
		{Role: "assistant", Text: "Continue.", Cursor: 2},
	}}
	images := 0
	imageBatches := 0
	_, err := (HistoryPreparation{Input: input, Adapter: adapterFunc(func(ctx context.Context, input Input, host Host) (Result, error) {
		if input.Mode != OperationSummarize || input.SessionID != "" || len(input.Tools) != 0 || SteeringFromContext(ctx) != nil {
			t.Fatal("summary inherited execution or provider session state")
		}
		batchImages := 0
		for _, message := range input.History {
			for _, image := range message.Attachments {
				if !strings.Contains(message.Text, "Source record 1 (cursor 1, user)") || strings.Contains(message.Text, file.RuntimePath) {
					t.Fatalf("image lost its portable source label: %s", message.Text)
				}
				if _, err := agent.ReadAttachmentImage(image); err != nil {
					t.Fatal(err)
				}
				batchImages++
			}
		}
		if batchImages > 1 {
			t.Fatal("conservative image reserve overflowed a maintenance batch")
		}
		if batchImages > 0 {
			imageBatches++
		}
		images += batchImages
		return Result{Text: "The reference is transparent."}, nil
	})}).Prepare(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if images != 2 || imageBatches != 2 {
		t.Fatalf("summary received %d images, want both user and tool images", images)
	}
}

func TestExternalCheckpointPreservesRecentImagesAndPortableSource(t *testing.T) {
	file := checkpointImage(t, png.NoCompression)
	file.Path = "attachments/recent.png"
	input := Input{HistoryBoundary: "branch-head", History: []Message{
		{Role: "user", Text: strings.Repeat("Old context. ", 10000), Cursor: 1},
		{Role: "user", Text: "Follow this recent reference", Attachments: []agent.Attachment{file}, Cursor: 2},
		{Role: "assistant", Text: "Confirmed.", Cursor: 2},
	}}
	input.History[1].Attachments[0].RuntimePath = ""
	original, _ := json.Marshal(input)
	var saved externaljournal.Checkpoint
	calls := 0
	preparation := HistoryPreparation{Input: input, ResolveMedia: func(ctx context.Context, in Input) (Input, error) {
		for i := range in.History {
			if len(in.History[i].Attachments) > 0 {
				in.History[i].Attachments = []agent.Attachment{file}
			}
		}
		return in, nil
	}, Adapter: adapterFunc(func(context.Context, Input, Host) (Result, error) {
		calls++
		return Result{Text: "Old confirmed context."}, nil
	}), SaveCheckpoint: func(cp externaljournal.Checkpoint) error { saved = cp; return nil }}
	prepared, err := preparation.Prepare(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.History) != 3 || len(prepared.History[1].Attachments) != 1 || saved.SourceEnd != 1 || saved.SourceBoundary != input.HistoryBoundary {
		t.Fatalf("recent transaction was not retained: history=%+v checkpoint=%+v", prepared.History, saved)
	}
	if saved.SourceHash != historyHash(input.History[:1]) || saved.Version != checkpointVersion {
		t.Fatalf("checkpoint was not bound to portable source: %+v", saved)
	}
	after, _ := json.Marshal(input)
	if !bytes.Equal(after, original) || input.History[1].Attachments[0].RuntimePath != "" {
		t.Fatal("preparation mutated canonical source")
	}
	priorCalls := calls
	preparation.Checkpoint = &saved
	if _, err := preparation.Prepare(t.Context()); err != nil || calls != priorCalls {
		t.Fatalf("verified multimodal checkpoint was not reused: calls=%d err=%v", calls, err)
	}
	legacy := saved
	legacy.Version, legacy.Summary = 0, "An image-blind legacy summary."
	preparation.Checkpoint = &legacy
	if _, err := preparation.Prepare(t.Context()); err != nil || calls == priorCalls {
		t.Fatalf("legacy summary was not rebuilt: calls=%d err=%v", calls, err)
	}
}

type imageEstimatorAdapter struct {
	adapterFunc
	tokens int
}

func (adapter imageEstimatorAdapter) InputEstimator(Input) agent.InputEstimator {
	return agent.InputEstimator{ImageTokens: func(int, int) int { return adapter.tokens }}
}

func TestExternalSummaryFailureNeverAcknowledgesCoverage(t *testing.T) {
	for _, failure := range []string{"later_batch", "missing_image", "changed_image", "image_budget", "empty_summary", "oversized_summary", "final_input_budget", "cancelled"} {
		t.Run(failure, func(t *testing.T) {
			file := checkpointImage(t, png.BestCompression)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			input := Input{History: []Message{{Role: "user", Text: strings.Repeat("历史資料。", 20000), Attachments: []agent.Attachment{file}, Cursor: 1}}}
			calls, saved, usage := 0, 0, 0
			adapter := imageEstimatorAdapter{tokens: 100, adapterFunc: func(ctx context.Context, input Input, host Host) (Result, error) {
				calls++
				result := Result{Text: "Confirmed image details.", Usage: &agent.TokenUsage{TotalTokens: 7}}
				switch failure {
				case "later_batch":
					if calls == 2 {
						return result, errors.New("second batch failed")
					}
				case "empty_summary":
					result.Text = " "
				case "oversized_summary":
					result.Text = strings.Repeat("x", checkpointSummaryBytes+1)
				case "cancelled":
					cancel()
				}
				return result, nil
			}}
			preparation := HistoryPreparation{Input: input, Adapter: adapter, AddUsage: func(u *agent.TokenUsage) { usage += u.TotalTokens },
				SaveCheckpoint: func(externaljournal.Checkpoint) error { saved++; return nil }}
			switch failure {
			case "missing_image":
				if err := os.Remove(file.RuntimePath); err != nil {
					t.Fatal(err)
				}
			case "changed_image":
				preparation.Input.History[0].Attachments[0].SHA256 = strings.Repeat("0", 64)
			case "image_budget":
				adapter.tokens = historyTokenBudget + 1
				preparation.Adapter = adapter
			case "final_input_budget":
				preparation.ProviderInputMaxBytes = 8192
				preparation.Input.Text = strings.Repeat("x", 10000)
			}
			if _, err := preparation.Prepare(ctx); err == nil || saved != 0 {
				t.Fatalf("failed preparation published coverage: saved=%d err=%v", saved, err)
			}
			if failure == "later_batch" && (calls != 2 || usage != 14) {
				t.Fatalf("failed batch usage was lost: calls=%d usage=%d", calls, usage)
			}
		})
	}
}
