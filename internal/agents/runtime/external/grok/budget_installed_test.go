package grok

import (
	"context"
	"strings"
	"testing"
	"time"

	"denova/internal/agents/runtime/external"

	agentschema "github.com/alfredxw/denova/agent/schema"
)

// TestInstalledGrokHistoryReplayWithImage exercises the whole cold-recovery
// path against the released executable: a reconstruction whose text fills the
// prompt payload (118 KiB against the 120 KiB cap) plus one small historical
// PNG. The shared history preparation must trim the text through a real
// summarize turn, and the real CLI must then accept the trimmed .json prompt
// file and settle the turn. Before the payload-budget fix this reconstruction
// was rejected as historyMediaUnsupported by the engine channel. The 180s
// budget covers both CLI spawns; a real run has been observed near 95s.
// The model server must be hit at least twice: once for the checkpoint
// summarize attempt and once for the trimmed main turn. Without the trim the
// reconstruction never reaches a second request because the oversized turn is
// rejected before the model server.
func TestInstalledGrokHistoryReplayWithImage(t *testing.T) {
	requests := 0
	_, configPath := modelFixture(t, "Recovered.", func(payload []byte) {
		requests++
	})
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	client, input := installedInput(t, configPath)

	var history []external.Message
	for index := 0; index < 3; index++ {
		history = append(history, external.Message{
			Role:   "user",
			Text:   strings.Repeat("story context ", storyContextRepeat),
			Cursor: uint64(index + 1),
		})
	}
	history = append(history, external.Message{
		Role:        "user",
		Text:        "Use this earlier reference",
		Attachments: []agentschema.Attachment{fixtureAttachment(t, 1, 1)},
		Cursor:      uint64(len(history) + 1),
	})
	input.History = history
	input.Text = strings.Repeat("continue the story ", continueStoryRepeat)

	prepared, err := (external.HistoryPreparation{Input: input, Adapter: client}).Prepare(ctx)
	if err != nil {
		t.Fatalf("real CLI history preparation rejected the reconstruction: %v", err)
	}
	if requests < 2 {
		t.Fatalf("real CLI payload trim never reached the model server twice: requests=%d", requests)
	}
	result, err := client.Run(ctx, prepared, &recordingHost{})
	if err != nil {
		t.Fatalf("real CLI rejected the trimmed image-bearing reconstruction: %v", err)
	}
	if !result.Settled || !strings.Contains(result.Text, "Recovered") || result.SessionID == "" {
		t.Fatalf("real CLI recovery result: %+v", result)
	}
}