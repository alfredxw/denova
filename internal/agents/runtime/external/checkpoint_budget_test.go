package external

import (
	"context"
	"image/png"
	"strings"
	"testing"

	agentschema "github.com/alfredxw/denova/agent/schema"
)

// budgetedAdapter is an Adapter that also reports a transport payload budget,
// mirroring grok.Client. A summarize attempt answers like the real engine;
// any other run means the trim missed and must not be reached.
type budgetedAdapter struct {
	adapterFunc
	payload    int
	mediaSpace int
}

func (adapter budgetedAdapter) PayloadBudget() (payload, mediaReserve int, ok bool) {
	return adapter.payload, adapter.mediaSpace, true
}

// nearBudgetImageInput builds a rebuilt conversation whose history stays just
// inside the semantic history budget while history plus the current request
// would fill the reported payload once images are encoded.
func nearBudgetImageInput(t *testing.T) Input {
	t.Helper()
	file := checkpointImage(t, png.NoCompression)
	return Input{
		History: []Message{
			{Role: "user", Text: strings.Repeat("story context ", 1520), Cursor: 1},
			{Role: "user", Text: strings.Repeat("story context ", 1520), Cursor: 2},
			{Role: "user", Text: strings.Repeat("story context ", 1520), Cursor: 3},
			{Role: "user", Text: "Use this earlier reference", Attachments: []agentschema.Attachment{file}, Cursor: 4},
		},
		Text: strings.Repeat("continue the story ", 2850),
	}
}

// estimatedReplayHistoryBytes sums the semantic text of the history records
// that will be rendered for the engine prompt after preparation. The current
// request text is excluded: estimateReplayText takes it as its own parameter.
func estimatedReplayHistoryBytes(prepared Input) int {
	total := 0
	for _, message := range prepared.History {
		total += len(message.Text)
	}
	return total
}

// A payload-budgeted adapter carrying images must trim the reconstructed text
// so the media reserve survives: unbounded history plus the current request
// used to pass the product budget and then fail the engine channel.
func TestPayloadBudgetTrimsHistoryForMedia(t *testing.T) {
	const payload, mediaSpace = 120 << 10, 9 << 10
	textCap := payload - mediaSpace
	calls := 0
	prepared, err := (HistoryPreparation{
		Input:   nearBudgetImageInput(t),
		Adapter: budgetedAdapter{adapterFunc: func(_ context.Context, input Input, _ Host) (Result, error) {
			calls++
			if input.Mode != OperationSummarize {
				t.Fatalf("trim budget ran a turn instead of a summary")
			}
			return Result{Text: "Compacted earlier story context exceeding the payload media reserve."}, nil
		}, payload: payload, mediaSpace: mediaSpace},
	}).Prepare(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if calls == 0 {
		t.Fatal("payload budget did not trim the reconstructed history")
	}
	historyBytes := estimatedReplayHistoryBytes(prepared)
	if got := estimateReplayText(historyBytes, 0, len(prepared.Text), len(prepared.History)); got > textCap {
		t.Fatalf("trimmed reconstruction still exceeds the payload text cap: estimate %d > %d", got, textCap)
	}
	if historyBytes+len(prepared.Text) > textCap {
		t.Fatalf("trimmed reconstruction text is not inside the media reserve: %d > %d", historyBytes+len(prepared.Text), textCap)
	}
}

// Without media there is no reserve to protect: the same reconstruction uses
// the provider budget alone and stays untouched.
func TestPayloadBudgetLeavesTextOnlyReconstructionUntouched(t *testing.T) {
	input := nearBudgetImageInput(t)
	input.History = input.History[:3] // drop the image-bearing record
	calls := 0
	prepared, err := (HistoryPreparation{
		Input:   input,
		Adapter: budgetedAdapter{adapterFunc: func(_ context.Context, input Input, _ Host) (Result, error) {
			calls++
			return Result{Text: "unexpected summary"}, nil
		}, payload: 120 << 10, mediaSpace: 9 << 10},
	}).Prepare(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if calls != 0 || len(prepared.History) != 3 {
		t.Fatalf("text-only reconstruction was trimmed: calls=%d history=%d", calls, len(prepared.History))
	}
}

// An adapter without a payload budget keeps the previous behavior: the product
// provider budget applies and no media reservation trims the history.
func TestPayloadBudgetAbsentKeepsProviderBudgetBehavior(t *testing.T) {
	input := nearBudgetImageInput(t)
	calls := 0
	prepared, err := (HistoryPreparation{
		Input:   input,
		Adapter: adapterFunc(func(_ context.Context, input Input, _ Host) (Result, error) {
			calls++
			return Result{Text: "unexpected summary"}, nil
		}),
	}).Prepare(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if calls != 0 || len(prepared.History) != 4 || len(prepared.History[3].Attachments) != 1 {
		t.Fatalf("absent budget changed reconstruction behavior: calls=%d history=%d", calls, len(prepared.History))
	}
}

// A small reconstruction with media stays untouched even under a payload
// budget: there is nothing to trim and no summary is manufactured.
func TestPayloadBudgetSparesSmallImageReconstruction(t *testing.T) {
	file := checkpointImage(t, png.NoCompression)
	input := Input{
		History: []Message{
			{Role: "user", Text: "Previous reference", Attachments: []agentschema.Attachment{file}, Cursor: 1},
			{Role: "assistant", Text: "Confirmed.", Cursor: 2},
		},
		Text: "Continue.",
	}
	calls := 0
	prepared, err := (HistoryPreparation{
		Input:   input,
		Adapter: budgetedAdapter{adapterFunc: func(_ context.Context, input Input, _ Host) (Result, error) {
			calls++
			return Result{Text: "unexpected summary"}, nil
		}, payload: 120 << 10, mediaSpace: 9 << 10},
	}).Prepare(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if calls != 0 || len(prepared.History) != 2 || len(prepared.History[0].Attachments) != 1 {
		t.Fatalf("small image reconstruction was trimmed: calls=%d history=%d", calls, len(prepared.History))
	}
}