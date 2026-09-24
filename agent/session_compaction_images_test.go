package agent_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/compaction"
	"github.com/alfredxw/denova/agent/providers"
	sessionfile "github.com/alfredxw/denova/agent/session/file"
)

type imageCompactionModel struct {
	window  int
	primary [][]*agent.Message
	summary [][]*agent.Message
}

func (*imageCompactionModel) InputEstimator() agent.InputEstimator {
	return (providers.ModelConfig{Model: "claude-sonnet-4-7"}).InputEstimator()
}

func (m *imageCompactionModel) Generate(_ context.Context, messages []*agent.Message, options ...agent.ModelOption) (*agent.Message, error) {
	size, err := m.InputEstimator().Estimate(messages, agent.GetCommonOptions(nil, options...).Tools)
	if err != nil {
		return nil, err
	}
	if size.Tokens > m.window {
		return nil, fmt.Errorf("image context exceeds window: %d > %d", size.Tokens, m.window)
	}
	for _, message := range messages {
		if strings.Contains(message.Content, "[Runtime context compaction request]") {
			m.summary = append(m.summary, messages)
			return agent.AssistantMessage("The reference shows a blue station with a red door. Continue the current task.", nil), nil
		}
	}
	m.primary = append(m.primary, messages)
	return agent.AssistantMessage("Continued with the reference images.", nil), nil
}

func (m *imageCompactionModel) Stream(ctx context.Context, messages []*agent.Message, options ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	response, err := m.Generate(ctx, messages, options...)
	return agent.StreamReaderFromArray([]*agent.Message{response}), err
}

func TestImageCompactionContinuesAndReopens(t *testing.T) {
	for _, mode := range []string{"automatic", "manual"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			file, err := os.Create(filepath.Join(root, "reference.png"))
			if err != nil {
				t.Fatal(err)
			}
			if err := png.Encode(file, image.NewGray(image.Rect(0, 0, 2048, 2048))); err != nil {
				t.Fatal(err)
			}
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(file.Name())
			if err != nil {
				t.Fatal(err)
			}
			model := &imageCompactionModel{window: 64_000}
			definition := agent.Definition{
				Name: "image-compaction", Model: model, AttachmentRoot: root,
				Compaction: compaction.Standard(compaction.StandardConfig{
					ContextWindowTokens: model.window, TriggerBytes: 217_600, KeepRecentBytes: 64 << 10,
				}),
			}
			store, err := sessionfile.New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			owner, err := agent.New(t.Context(), definition, agent.WithSessionStore(store))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = owner.Close(context.Background()) }()
			conversation, err := owner.Session(t.Context(), agent.NamedSession("images"))
			if err != nil {
				t.Fatal(err)
			}
			var history []*agent.Message
			for index := range 14 {
				file := agent.Attachment{ID: fmt.Sprintf("image-%d", index), Name: "reference.png", MediaType: "image/png", Path: "reference.png", Size: int64(len(data)), SHA256: fmt.Sprintf("%x", sha256.Sum256(data))}
				history = append(history, agent.UserMessageWithAttachments("Inspect the reference.", []agent.Attachment{file}), agent.AssistantMessage("Seen.", nil))
			}
			if err := conversation.LoadCanonicalMessages(t.Context(), history); err != nil {
				t.Fatal(err)
			}
			if mode == "manual" {
				result, err := conversation.Compact(t.Context(), agent.CompactionRequest{Force: true})
				if err != nil || !result.Changed {
					t.Fatalf("manual image compaction: changed=%v err=%v", result.Changed, err)
				}
			}
			run, err := conversation.Run(t.Context(), agent.Text("Continue from the references."))
			if err != nil {
				t.Fatal(err)
			}
			if result, err := run.Wait(t.Context()); err != nil || result.Status != agent.ResultCompleted {
				t.Fatalf("image conversation did not continue: %+v err=%v", result, err)
			}
			before, err := conversation.Snapshot(t.Context())
			if err != nil || before.Compaction == nil || len(model.summary) == 0 {
				t.Fatalf("image pressure produced no checkpoint: %+v err=%v", before.Compaction, err)
			}
			summaryImages := 0
			for _, call := range model.summary {
				for _, message := range call {
					summaryImages += len(message.Attachments)
				}
			}
			if summaryImages == 0 {
				t.Fatal("image checkpoint was generated without any native visual input")
			}
			if err := owner.Close(t.Context()); err != nil {
				t.Fatal(err)
			}
			owner, err = agent.New(t.Context(), definition, agent.WithSessionStore(store))
			if err != nil {
				t.Fatal(err)
			}
			conversation, err = owner.Session(t.Context(), agent.NamedSession("images"))
			if err != nil {
				t.Fatal(err)
			}
			run, err = conversation.Run(t.Context(), agent.Text("Continue after reopening."))
			if err != nil {
				t.Fatal(err)
			}
			if result, err := run.Wait(t.Context()); err != nil || result.Status != agent.ResultCompleted {
				t.Fatalf("reopened image conversation failed: %+v err=%v", result, err)
			}
			last := model.primary[len(model.primary)-1]
			checkpoint, newest := false, false
			for _, message := range last {
				checkpoint = checkpoint || strings.Contains(message.Content, "blue station with a red door")
				for _, file := range message.Attachments {
					if _, err := agent.AttachmentBase64(file); err != nil {
						t.Fatalf("retained image cannot be reopened: %v", err)
					}
					newest = newest || file.ID == "image-13"
				}
			}
			if !checkpoint || !newest {
				t.Fatalf("reopened context lost checkpoint or protected image: checkpoint=%v newest=%v", checkpoint, newest)
			}
		})
	}
}
