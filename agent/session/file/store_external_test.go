package file_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/alfredxw/denova/agent/session"
	sessionfile "github.com/alfredxw/denova/agent/session/file"
	"github.com/alfredxw/denova/agent/session/sessiontest"
)

func TestPublicStoreContract(t *testing.T) {
	sessiontest.RunStoreContract(t, func(t testing.TB) session.Store {
		store, err := sessionfile.New(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		return store
	})
}

func TestFileStoreIgnoresAndReplacesTornFinalTransaction(t *testing.T) {
	root := t.TempDir()
	store, err := sessionfile.New(root)
	if err != nil {
		t.Fatal(err)
	}
	key := session.Key{Namespace: "test", ID: "torn-tail"}
	log, err := store.Open(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := log.Append(context.Background(), 0, session.Record{
		Kind: "test.first", Version: 1, Data: json.RawMessage(`{"value":1}`),
	}); err != nil {
		t.Fatal(err)
	}
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}

	paths, err := filepath.Glob(filepath.Join(root, "*.jsonl"))
	if err != nil || len(paths) != 1 {
		t.Fatalf("transcript paths = %#v, err = %v", paths, err)
	}
	file, err := os.OpenFile(paths[0], os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(`{"incomplete":`); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	log, err = store.Open(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	stats, err := log.Replay(context.Background(), func(session.Record) error { return nil })
	if err != nil || stats.RecordsRead != 1 {
		t.Fatalf("replay stats = %#v, err = %v", stats, err)
	}
	if _, err := log.Append(context.Background(), 1, session.Record{
		Kind: "test.second", Version: 1, Data: json.RawMessage(`{"value":2}`),
	}); err != nil {
		t.Fatal(err)
	}
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}

	log, err = store.Open(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	stats, err = log.Replay(context.Background(), func(session.Record) error { return nil })
	if err != nil || stats.RecordsRead != 2 {
		t.Fatalf("reopened stats = %#v, err = %v", stats, err)
	}
}
