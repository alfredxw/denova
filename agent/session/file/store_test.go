package file

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/alfredxw/denova/agent/session"
	"github.com/alfredxw/denova/agent/session/sessiontest"
)

func TestStoreContract(t *testing.T) {
	sessiontest.RunStoreContract(t, func(t testing.TB) session.Store {
		store, err := New(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		return store
	})
}

func TestStoreReopensCommittedRecords(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	key := session.Key{Namespace: "test", ID: "reopen", Attributes: map[string]string{"branch": "main"}}
	first, err := store.Open(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.Append(context.Background(), 0, session.Record{
		Kind: "test.record", Version: 1, Data: json.RawMessage(`{"value":1}`),
	}); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	second, err := store.Open(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	var records []session.Record
	stats, err := second.Replay(context.Background(), func(record session.Record) error {
		records = append(records, record)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if stats.RecordsRead != 1 || len(records) != 1 || records[0].Revision != 1 || string(records[0].Data) != `{"value":1}` {
		t.Fatalf("stats=%#v records=%#v", stats, records)
	}
}
