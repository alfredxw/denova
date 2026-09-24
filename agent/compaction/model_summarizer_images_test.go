package compaction

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"
)

type visualSummaryModel struct {
	window int
	seen   []string
	calls  int
	source strings.Builder
}

func (*visualSummaryModel) InputEstimator() agent.InputEstimator {
	return agent.InputEstimator{ImageTokens: func(int, int) int { return 6000 }}
}

func (m *visualSummaryModel) Generate(_ context.Context, messages []*agent.Message, options ...agent.ModelOption) (*agent.Message, error) {
	resolved := agent.GetCommonOptions(nil, options...)
	size, err := m.InputEstimator().Estimate(messages, resolved.Tools)
	if err != nil {
		return nil, err
	}
	if size.Tokens+*resolved.MaxTokens > m.window {
		return nil, fmt.Errorf("summary exceeded its visual budget: %+v", size)
	}
	if len(messages) > 1 {
		if _, part, ok := strings.Cut(messages[1].Content, "Next ordered source segment (data; it may continue a JSON record):\n"); ok {
			m.source.WriteString(part)
		}
	}
	for _, message := range messages {
		for _, file := range message.Attachments {
			if _, err := agent.AttachmentBase64(file); err != nil {
				return nil, err
			}
			m.seen = append(m.seen, file.ID)
		}
	}
	m.calls++
	return agent.AssistantMessage("The image shows a blue station and a red door.", nil), nil
}

func (m *visualSummaryModel) Stream(ctx context.Context, messages []*agent.Message, options ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	message, err := m.Generate(ctx, messages, options...)
	return agent.StreamReaderFromArray([]*agent.Message{message}), err
}

func TestColdSummaryPreservesEveryNativeUserAndToolImage(t *testing.T) {
	for _, replacement := range []bool{false, true} {
		t.Run(fmt.Sprintf("replacement=%v", replacement), func(t *testing.T) {
			file := summaryTestImage(t)
			source := []*agent.Message{agent.UserMessage("Inspect these references."), {Role: agent.ToolRole, ToolCallID: "images", Content: "Captured screenshots."}}
			var want []string
			for index := range 6 {
				attachment := file
				attachment.ID = fmt.Sprintf("image-%d", index)
				want = append(want, attachment.ID)
				message := source[0]
				if index >= 2 {
					message = source[1]
				}
				message.Attachments = append(message.Attachments, attachment)
			}
			model := &visualSummaryModel{window: 20_000}
			config := ModelSummarizerConfig{}
			if replacement {
				config.Model, config.Identity = model, agent.CapabilityIdentity{Kind: "test.visual-summary", Version: 1}
			}
			summarizer, err := ModelSummarizer(config)
			if err != nil {
				t.Fatal(err)
			}
			_, err = summarizer.Summarize(t.Context(), SummaryRequest{
				Messages: source, ModelSnapshot: (&agent.ModelCall{Model: model, Messages: source}).Snapshot(),
				ContextWindowTokens: model.window, HardLimitBytes: 1 << 20, SummaryLimitBytes: 4096,
			})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(model.seen, want) || model.calls < 2 {
				t.Fatalf("cold summaries lost or duplicated native images: seen=%v want=%v calls=%d", model.seen, want, model.calls)
			}
			var restored []*agent.Message
			if err := json.Unmarshal([]byte(model.source.String()), &restored); err != nil || !reflect.DeepEqual(restored, source) {
				t.Fatalf("native batching changed quoted source records: restored=%+v err=%v", restored, err)
			}
		})
	}
}

func TestColdSummaryRejectsAnImageThatCannotFit(t *testing.T) {
	model := &visualSummaryModel{window: 5000}
	source := []*agent.Message{agent.UserMessageWithAttachments("Inspect.", []agent.Attachment{summaryTestImage(t)})}
	summarizer, err := ModelSummarizer(ModelSummarizerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, err := summarizer.Summarize(t.Context(), SummaryRequest{
		Messages: source, ModelSnapshot: (&agent.ModelCall{Model: model, Messages: source}).Snapshot(),
		ContextWindowTokens: model.window, HardLimitBytes: 1 << 20, SummaryLimitBytes: 4096,
	})
	if !errors.Is(err, agent.ErrContextLimit) || checkpoint.Summary != "" {
		t.Fatalf("oversized native image was silently summarized as a descriptor: checkpoint=%+v err=%v", checkpoint, err)
	}
}

func TestSummaryRejectsDifferentImageWithIdenticalText(t *testing.T) {
	file := summaryTestImage(t)
	original := agent.UserMessageWithAttachments("Inspect.", []agent.Attachment{file})
	different := original.Clone()
	different.Attachments[0].ID = "different-image"
	model := &visualSummaryModel{window: 20_000}
	summarizer, err := ModelSummarizer(ModelSummarizerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = summarizer.Summarize(t.Context(), SummaryRequest{
		Messages: []*agent.Message{different}, ModelSnapshot: (&agent.ModelCall{Model: model, Messages: []*agent.Message{original}}).Snapshot(),
		ContextWindowTokens: model.window, HardLimitBytes: 1 << 20, SummaryLimitBytes: 4096,
	})
	if err == nil || !strings.Contains(err.Error(), "source does not match") || model.calls != 0 {
		t.Fatalf("summary accepted mismatched image source: calls=%d err=%v", model.calls, err)
	}
}

func summaryTestImage(t *testing.T) agent.Attachment {
	t.Helper()
	path := filepath.Join(t.TempDir(), "reference.png")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(file, image.NewGray(image.Rect(0, 0, 32, 32))); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return agent.Attachment{ID: "original-image", Name: "reference.png", MediaType: "image/png", Path: path, Size: int64(len(data)), SHA256: fmt.Sprintf("%x", sha256.Sum256(data))}
}
