package autosaveconflict

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestStoreAppendPersistsPrivateAtomicRecord(t *testing.T) {
	dataDir := t.TempDir()
	store := NewStore(dataDir)
	input := testInput()
	input.ID = "../../chapters/escape.md"

	result, err := store.Append(context.Background(), input)
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	if result.Record.ID == "" || strings.ContainsAny(result.Record.ID, `/\\`) {
		t.Fatalf("generated id is not filename-safe: %q", result.Record.ID)
	}
	conflictDir := filepath.Join(dataDir, DirectoryName)
	if filepath.Dir(result.Path) != conflictDir {
		t.Fatalf("record escaped conflict directory: path=%q dir=%q", result.Path, conflictDir)
	}
	if filepath.Base(result.Path) != result.Record.ID+".json" {
		t.Fatalf("record filename = %q, want %q", filepath.Base(result.Path), result.Record.ID+".json")
	}

	dirInfo, err := os.Stat(conflictDir)
	if err != nil {
		t.Fatalf("stat conflict directory: %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("conflict directory mode = %o, want 700", got)
	}
	fileInfo, err := os.Stat(result.Path)
	if err != nil {
		t.Fatalf("stat conflict record: %v", err)
	}
	if got := fileInfo.Mode().Perm(); got != 0o600 {
		t.Fatalf("conflict record mode = %o, want 600", got)
	}

	data, err := os.ReadFile(result.Path)
	if err != nil {
		t.Fatalf("read conflict record: %v", err)
	}
	var persisted Record
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatalf("decode conflict record: %v", err)
	}
	if persisted.ID != result.Record.ID || persisted.Resource != input.Resource || persisted.Scope != input.Scope || persisted.ResourceID != input.ID {
		t.Fatalf("persisted identity = %#v, input = %#v", persisted, input)
	}
	if !sameJSON(persisted.Base.Value, input.Base.Value) || !sameJSON(persisted.External.Value, input.External.Value) {
		t.Fatalf("persisted snapshots changed: %#v", persisted)
	}
	if len(persisted.ConflictPaths) != 1 || strings.Join(persisted.ConflictPaths[0], ".") != "agents.ide.profile" {
		t.Fatalf("persisted conflict paths = %#v", persisted.ConflictPaths)
	}

	entries, err := os.ReadDir(conflictDir)
	if err != nil {
		t.Fatalf("read conflict directory: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(result.Path) {
		t.Fatalf("unexpected files after atomic append: %#v", entries)
	}
}

func TestStoreAppendNeverOverwritesExistingConflict(t *testing.T) {
	store := NewStore(t.TempDir())
	input := testInput()

	first, err := store.Append(context.Background(), input)
	if err != nil {
		t.Fatalf("first Append() error = %v", err)
	}
	firstData, err := os.ReadFile(first.Path)
	if err != nil {
		t.Fatalf("read first record: %v", err)
	}
	second, err := store.Append(context.Background(), input)
	if err != nil {
		t.Fatalf("second Append() error = %v", err)
	}
	if first.Record.ID == second.Record.ID || first.Path == second.Path {
		t.Fatalf("append reused an existing record: first=%#v second=%#v", first, second)
	}
	after, err := os.ReadFile(first.Path)
	if err != nil {
		t.Fatalf("read first record after second append: %v", err)
	}
	if string(after) != string(firstData) {
		t.Fatal("second append changed the first conflict record")
	}
	entries, err := os.ReadDir(filepath.Dir(first.Path))
	if err != nil {
		t.Fatalf("read conflict directory: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("record count = %d, want 2", len(entries))
	}
}

func TestStoreAppendRejectsInvalidInputBeforeWriting(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Input)
	}{
		{name: "resource", mutate: func(input *Input) { input.Resource = "" }},
		{name: "scope", mutate: func(input *Input) { input.Scope = "" }},
		{name: "resource id", mutate: func(input *Input) { input.ID = "" }},
		{name: "strategy", mutate: func(input *Input) { input.Strategy = "" }},
		{name: "base JSON", mutate: func(input *Input) { input.Base.Value = json.RawMessage(`{"broken"`) }},
		{name: "local JSON", mutate: func(input *Input) { input.Local.Value = nil }},
		{name: "conflict paths", mutate: func(input *Input) { input.ConflictPaths = nil }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dataDir := t.TempDir()
			input := testInput()
			tt.mutate(&input)
			_, err := NewStore(dataDir).Append(context.Background(), input)
			if !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("Append() error = %v, want ErrInvalidInput", err)
			}
			if _, statErr := os.Stat(filepath.Join(dataDir, DirectoryName)); !os.IsNotExist(statErr) {
				t.Fatalf("invalid input created conflict storage: %v", statErr)
			}
		})
	}
}

func TestStoreAppendRejectsConflictDirectorySymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires additional privileges on Windows")
	}
	dataDir := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dataDir, DirectoryName)); err != nil {
		t.Fatalf("create conflicts symlink: %v", err)
	}

	_, err := NewStore(dataDir).Append(context.Background(), testInput())
	if err == nil {
		t.Fatal("Append() unexpectedly followed a conflict directory symlink")
	}
	entries, readErr := os.ReadDir(outside)
	if readErr != nil {
		t.Fatalf("read symlink target: %v", readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("Append() wrote outside data directory: %#v", entries)
	}
}

func TestStoreAppendHonorsCanceledContext(t *testing.T) {
	dataDir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := NewStore(dataDir).Append(ctx, testInput())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Append() error = %v, want context.Canceled", err)
	}
	if _, statErr := os.Stat(filepath.Join(dataDir, DirectoryName)); !os.IsNotExist(statErr) {
		t.Fatalf("canceled append created conflict storage: %v", statErr)
	}
}

func testInput() Input {
	return Input{
		Resource: "settings",
		Scope:    "workspace:/books/novel",
		ID:       "workspace-settings",
		Base: Snapshot{
			Revision: "sha256:base",
			Value:    json.RawMessage(`{"agents":{"ide":{"profile":"baseline"}}}`),
		},
		Local: Snapshot{
			Revision: "sha256:local",
			Value:    json.RawMessage(`{"agents":{"ide":{"profile":"local"}}}`),
		},
		External: Snapshot{
			Revision: "sha256:external",
			Value:    json.RawMessage(`{"agents":{"ide":{"profile":"agent"}}}`),
		},
		Merged: Snapshot{
			Revision: "sha256:merged",
			Value:    json.RawMessage(`{"agents":{"ide":{"profile":"local"}}}`),
		},
		Strategy:      "merge_non_overlap_prefer_local",
		ConflictPaths: [][]string{{"agents", "ide", "profile"}},
	}
}

func sameJSON(left, right json.RawMessage) bool {
	var leftValue any
	var rightValue any
	if json.Unmarshal(left, &leftValue) != nil || json.Unmarshal(right, &rightValue) != nil {
		return false
	}
	leftData, leftErr := json.Marshal(leftValue)
	rightData, rightErr := json.Marshal(rightValue)
	return leftErr == nil && rightErr == nil && string(leftData) == string(rightData)
}
