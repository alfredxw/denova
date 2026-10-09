package conversationjournal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestVisitRangeMatchesReadsAndStopsOnVisitorFailure(t *testing.T) {
	journal, _ := largeJournal(t, 12, 512)
	defer journal.Close()
	selected := Range{After: 3, Through: 11, Limit: 4}
	want, err := journal.ReadRange(t.Context(), selected)
	if err != nil {
		t.Fatal(err)
	}
	var got []Record
	if err := journal.VisitRange(t.Context(), selected, func(record Record) error {
		record.Payload = bytes.Clone(record.Payload)
		got = append(got, record)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatal("streamed range differs from the collected range")
	}
	failure := errors.New("stop visiting")
	visits := 0
	if err := journal.VisitRange(t.Context(), selected, func(Record) error { visits++; return failure }); !errors.Is(err, failure) || visits != 1 {
		t.Fatalf("visitor failure: visits=%d err=%v", visits, err)
	}
}

// largeJournal mimics a long writing session: every transaction carries a
// whole chapter, and older transactions sit between sparse anchors.
func largeJournal(tb testing.TB, transactions, payloadBytes int) (*Journal, string) {
	tb.Helper()
	path := filepath.Join(tb.TempDir(), "session.jsonl")
	if err := os.WriteFile(path, []byte("{\"type\":\"session\",\"id\":\"test\"}\n"), 0o600); err != nil {
		tb.Fatal(err)
	}
	journal, err := Open(context.Background(), path, Identity{ID: "session-test", Generation: "generation-1"}, &countingProjection{}, Options{FlushEvery: 1 << 20})
	if err != nil {
		tb.Fatal(err)
	}
	chapter := strings.Repeat("章", payloadBytes/3)
	for value := 1; value <= transactions; value++ {
		payload, err := json.Marshal(map[string]any{"n": value, "text": chapter})
		if err != nil {
			tb.Fatal(err)
		}
		if _, err := journal.Append(context.Background(), Guard{Cursor: journal.Head().Cursor}, payload); err != nil {
			tb.Fatal(err)
		}
	}
	return journal, path
}

func TestReadRangeSkipsPrecedingTransactionsButKeepsChain(t *testing.T) {
	journal, path := largeJournal(t, 40, 2048)
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}
	reopen := func() *Journal {
		t.Helper()
		reopened, err := Open(context.Background(), path, Identity{ID: "session-test", Generation: "generation-1"}, &countingProjection{}, Options{SparseEvery: 1024, RecentRecords: 1})
		if err != nil {
			t.Fatal(err)
		}
		return reopened
	}
	reopened := reopen()
	records, err := reopened.ReadRange(context.Background(), Range{After: 29, Through: 30})
	if err != nil || len(records) != 1 || records[0].Location.Cursor != 30 {
		t.Fatalf("records=%#v err=%v", records, err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}

	// The record right before the range anchors its chain and must still be verified.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	corrupt := bytes.Replace(data, []byte(`"n":29,`), []byte(`"n":92,`), 1)
	if bytes.Equal(corrupt, data) {
		t.Fatal("fixture did not contain transaction 29")
	}
	if err := os.WriteFile(path, corrupt, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := reopen().ReadRange(context.Background(), Range{After: 29, Through: 30}); err == nil {
		t.Fatalf("corrupt predecessor error=%v", err)
	}
}

// BenchmarkReadRangeOldTransactions measures what opening a long session costs:
// single-transaction reads of older message transactions (up to 200 in practice).
func BenchmarkReadRangeOldTransactions(b *testing.B) {
	journal, _ := largeJournal(b, 1300, 40<<10)
	defer journal.Close()
	b.ResetTimer()
	for range b.N {
		for cursor := Cursor(800); cursor < 820; cursor++ {
			if _, err := journal.ReadRange(context.Background(), Range{After: cursor - 1, Through: cursor}); err != nil {
				b.Fatal(err)
			}
		}
	}
}

func TestReadTransactionsScansOnceInCursorOrder(t *testing.T) {
	journal, _ := largeJournal(t, 40, 512)
	defer journal.Close()
	records, err := journal.ReadTransactions(context.Background(), []Cursor{31, 3, 17, 3})
	if err != nil {
		t.Fatal(err)
	}
	var got []Cursor
	for _, record := range records {
		got = append(got, record.Location.Cursor)
		var payload struct {
			N int `json:"n"`
		}
		if err := json.Unmarshal(record.Payload, &payload); err != nil || Cursor(payload.N+1) != record.Location.Cursor {
			t.Fatalf("cursor %d payload n=%d err=%v", record.Location.Cursor, payload.N, err)
		}
	}
	if len(got) != 3 || got[0] != 3 || got[1] != 17 || got[2] != 31 {
		t.Fatalf("cursors = %v", got)
	}
	if _, err := journal.ReadTransactions(context.Background(), []Cursor{5, journal.Head().Cursor + 1}); err == nil {
		t.Fatal("reading past the head must fail")
	}
}

func BenchmarkReadTransactionsOld(b *testing.B) {
	journal, _ := largeJournal(b, 1300, 40<<10)
	defer journal.Close()
	cursors := make([]Cursor, 0, 200)
	for cursor := Cursor(800); cursor < 1000; cursor++ {
		cursors = append(cursors, cursor)
	}
	b.ResetTimer()
	for range b.N {
		if _, err := journal.ReadTransactions(context.Background(), cursors); err != nil {
			b.Fatal(err)
		}
	}
}
