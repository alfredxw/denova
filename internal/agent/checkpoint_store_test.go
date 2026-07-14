package agent

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFileCheckpointStorePersistsValuesByHashedKey(t *testing.T) {
	workspace := t.TempDir()
	store := newCheckpointStore(workspace, AgentKindIDE)

	key := "../unsafe/key"
	value := []byte("checkpoint-value")
	if err := store.Set(context.Background(), key, value); err != nil {
		t.Fatal(err)
	}
	got, ok, err := store.Get(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || !bytes.Equal(got, value) {
		t.Fatalf("checkpoint got ok=%v value=%q", ok, got)
	}

	files, err := os.ReadDir(filepath.Join(workspace, ".denova/checkpoints/ide"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("expected one hashed checkpoint file, got %d", len(files))
	}
	if files[0].Name() == key || filepath.Base(files[0].Name()) != files[0].Name() {
		t.Fatalf("checkpoint key should not become a path segment: %q", files[0].Name())
	}
	if err := removeCheckpoint(workspace, AgentKindIDE, key); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := store.Get(context.Background(), key); err != nil || ok {
		t.Fatalf("removed checkpoint remained readable: ok=%t err=%v", ok, err)
	}
}

func TestCheckpointStoreFallsBackToMemoryWithoutWorkspace(t *testing.T) {
	store := newCheckpointStore("", AgentKindIDE)
	if err := store.Set(context.Background(), "k", []byte("v")); err != nil {
		t.Fatal(err)
	}
	got, ok, err := store.Get(context.Background(), "k")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || string(got) != "v" {
		t.Fatalf("memory fallback got ok=%v value=%q", ok, got)
	}
}
