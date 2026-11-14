package grok

import (
	"context"
	"strings"
	"testing"

	"denova/internal/agents/runtime/external"

	agentschema "github.com/alfredxw/denova/agent/schema"
)

// budgetAdapter is a minimal external.Adapter for the shared HistoryPreparation
// step. It mirrors grok.Client's budget report so the product trims the same
// way it does for a real headless turn. A summarize attempt answers with a
// compact summary, exactly like the real CLI maintains the checkpoint; any
// other run means the repair did not trim enough and must not be reached.
type budgetAdapter struct{}

func (budgetAdapter) Version() string { return "budget-test" }
func (budgetAdapter) Run(_ context.Context, input external.Input, _ external.Host) (external.Result, error) {
	if input.Mode != external.OperationSummarize {
		return external.Result{}, errBudgetAdapterUsed
	}
	return external.Result{Text: "Compacted earlier story context that exceeds the payload media reserve."}, nil
}

func (budgetAdapter) PayloadBudget() (payload, mediaReserve int, ok bool) {
	return maxPromptPayloadBytes, promptEnvelopeBytes + promptMediaFloorBytes, true
}

var errBudgetAdapterUsed = &budgetAdapterUsedError{}

type budgetAdapterUsedError struct{}

func (*budgetAdapterUsedError) Error() string { return "budget test adapter must not run" }

// storyContextRepeat and continueStoryRepeat size the near-budget
// reconstruction: three history records stay under the 96 KiB semantic history
// budget (so no summary threshold triggers), while history plus the current
// request fill the whole 120 KiB Grok prompt payload once rendered. The
// fixture-layer and real-CLI repro tests must stay on the same sizing.
const (
	storyContextRepeat  = 1520
	continueStoryRepeat = 2850
)

// TestHistoryReplayNearBudgetCarriesHistoricalImage reproduces the P2 report:
// a rebuilt conversation whose reconstruction text nearly fills the Grok
// prompt payload (121,528 bytes in the report) passes the product budget and
// runs without media, but a single ~144-byte historical PNG makes the headless
// channel reject the turn as historyMediaUnsupported. The shared history
// preparation must trim the text before the payload is encoded so the small
// historical image still fits, without dropping it or failing the turn.
func TestHistoryReplayNearBudgetCarriesHistoricalImage(t *testing.T) {
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
	// The reconstruction stays just inside the product history budget (96 KiB
	// semantic bytes), so no summary is triggered, while history plus the
	// current request fills the whole 120 KiB Grok prompt payload: the report
	// measured 121,528 bytes.
	input := fixtureInput(t)
	input.History = history
	input.Text = strings.Repeat("continue the story ", continueStoryRepeat)

	prepared, err := (external.HistoryPreparation{
		Input:   input,
		Adapter: budgetAdapter{},
	}).Prepare(t.Context())
	if err != nil {
		t.Fatalf("product preparation rejected the reconstruction: %v", err)
	}
	prompt, images, err := renderPrompt(prepared)
	if err != nil {
		t.Fatalf("render prompt: %v", err)
	}
	if len(images) != 1 {
		t.Fatalf("historical image was dropped: %d images", len(images))
	}
	if _, err := promptContent(prompt, images); err != nil {
		t.Fatalf("historical image could not travel with the near-budget reconstruction: %v", err)
	}
}

// PayloadBudget must stay in lockstep with promptContent's own gate: text that
// fills the payload minus the media reserve still leaves the floor for one
// small image, while one more byte drops below the floor and is rejected. Any
// drift here would silently hand the transport a prompt it cannot encode.
func TestPayloadBudgetMatchesPromptContentFloor(t *testing.T) {
	client := &Client{}
	payload, reserve, ok := client.PayloadBudget()
	if !ok || payload != maxPromptPayloadBytes || reserve != promptEnvelopeBytes+promptMediaFloorBytes {
		t.Fatalf("PayloadBudget drifted from the prompt constants: payload=%d reserve=%d ok=%v", payload, reserve, ok)
	}
	image := fixtureAttachment(t, 1, 1)
	tight := strings.Repeat("x", payload-reserve-1)
	if _, err := promptContent(tight, []agentschema.Attachment{image}); err != nil {
		t.Fatalf("a prompt inside the payload reserve must fit its image: %v", err)
	}
	over := strings.Repeat("x", payload-reserve+1)
	if _, err := promptContent(over, []agentschema.Attachment{image}); !external.IsMediaUnsupported(err) {
		t.Fatalf("a prompt beyond the payload reserve must be rejected: %v", err)
	}
}