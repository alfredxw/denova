package sessionfile

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/alfredxw/denova/agent/session"
)

func TestReleasedTranscriptUpgradePreservesExactKeyPathAndBackup(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// v0.4.5 allocated child identities include opaque parent_route chunks.
	// They remain identity bytes; newer Runs carry ownership in their input.
	key := session.Key{Namespace: "task.writer", ID: "released-child", Attributes: map[string]string{
		"agent": "writer", "parent_route_chunks": "1", "parent_route_00": "eyJ0eXBlIjoib2xkLXJvdXRlIn0", "parent_route_sha256": "opaque-released-digest",
	}}
	opened, err := store.Open(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	log := opened.(*logFile)
	legacy := session.Record{Kind: "turn.finished", Version: 1, Data: json.RawMessage(`{"run_id":"old-run","command_id":"old-command","status":"completed","output":"released result","at":"2026-09-09T00:00:00Z"}`)}
	if _, err := log.Append(context.Background(), 0, legacy); err != nil {
		t.Fatal(err)
	}
	path := log.path
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}
	opened, err = store.Open(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	log = opened.(*logFile)
	if log.path != path {
		t.Fatal("released Session path changed")
	}
	modern := session.Record{Kind: "session.input", Version: 1, Data: json.RawMessage(`{"fixture":"accepted new Run"}`)}
	if _, err := log.Append(context.Background(), 1, modern); err != nil {
		t.Fatal(err)
	}
	if _, err := log.Append(context.Background(), 2, modern); err != nil {
		t.Fatal(err)
	}
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}
	backup, err := os.ReadFile(path + ".pre-resilience-v1.bak")
	if err != nil || !bytes.Equal(backup, original) {
		t.Fatalf("original backup was missing or replaced: %v", err)
	}
	keys, err := store.List(context.Background(), session.Selector{Namespace: key.Namespace})
	if err != nil || len(keys) != 1 {
		t.Fatalf("released identity listing=%#v error=%v", keys, err)
	}
	want, _ := session.CanonicalKey(key)
	got, _ := session.CanonicalKey(keys[0])
	if got != want {
		t.Fatal("released parent route identity was rewritten")
	}
}
