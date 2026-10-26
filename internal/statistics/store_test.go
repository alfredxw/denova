package statistics

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestCallHistorySurvivesMonthAndDataRootMoves(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "before", "stores", "book")
	at := time.Date(2026, 10, 31, 23, 59, 0, 0, time.UTC)
	calls := []Call{
		{ID: "native", At: at, Kind: ModelCall, Runtime: "native", Model: "model", DurationMS: 500, Usage: &Usage{Prompt: 100, Cached: 80, Output: 20, Total: 120}},
		{ID: "external", At: at.Add(time.Minute), Kind: RuntimeTurn, Runtime: "codex", Failed: true},
	}
	for _, call := range calls {
		if err := AppendCall(root, call); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Rename(filepath.Join(parent, "before"), filepath.Join(parent, "after")); err != nil {
		t.Fatal(err)
	}
	root = filepath.Join(parent, "after", "stores", "book")
	var got []Call
	if err := ReadCalls(root, at.Add(-time.Minute), at.Add(2*time.Minute), func(call Call) { got = append(got, call) }); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, calls) {
		t.Fatalf("portable history: got %+v, want %+v", got, calls)
	}
	var totals Totals
	for _, call := range got {
		totals.Add(call)
	}
	want := Totals{Requests: 1, ExternalTurns: 1, UnknownUsage: 1, Prompt: 100, Cached: 80, Output: 20, Tokens: 120, DurationMS: 500}
	if totals != want {
		t.Fatalf("totals = %+v, want %+v", totals, want)
	}
}

func TestUsageUnknownZeroAndPerformanceDenominators(t *testing.T) {
	ttft := int64(40)
	var totals Totals
	totals.Add(Call{Kind: ModelCall, Usage: &Usage{}, DurationMS: 100, TTFTMS: &ttft})
	totals.Add(Call{Kind: ModelCall, Failed: true, DurationMS: 300})
	totals.Add(Call{Kind: RuntimeTurn, DurationMS: 99999, Usage: &Usage{Total: 7}})
	want := Totals{Requests: 2, ExternalTurns: 1, FailedRequests: 1, UnknownUsage: 1, DurationMS: 400, TTFTMS: 40, TTFTRequests: 1, Tokens: 7}
	if totals != want {
		t.Fatalf("totals = %+v, want %+v", totals, want)
	}
}

func TestTornTailKeepsLaterObservationsReadableAndReportsGap(t *testing.T) {
	root := t.TempDir()
	at := time.Now().UTC()
	call := Call{ID: "before", At: at, Kind: ModelCall}
	if err := AppendCall(root, call); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "statistics", "calls", at.Format("2006-01")+".jsonl")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(`{"id":`); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	call.ID = "after"
	if err := AppendCall(root, call); err != nil {
		t.Fatal(err)
	}
	var ids []string
	err = ReadCalls(root, at.Add(-time.Minute), at.Add(time.Minute), func(call Call) { ids = append(ids, call.ID) })
	if err == nil || !reflect.DeepEqual(ids, []string{"before", "after"}) {
		t.Fatalf("ids=%v error=%v", ids, err)
	}
}
