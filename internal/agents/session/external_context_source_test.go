package session

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	agentschema "github.com/alfredxw/denova/agent/schema"
)

func TestExternalContextSourceSurvivesAppendAndReopenButNotClear(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.jsonl")
	sess, err := createSessionWithRuntimeConfig("history", path, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sess.Close() })
	// Exceed the resident window, so a loader cannot silently use UI history.
	messages := make([]*agentschema.Message, sessionRecentTransactionLimit+25)
	for index := range messages {
		messages[index] = agentschema.UserMessage(fmt.Sprintf("original-%d", index))
	}
	if err := sess.withCanonicalMutation(t.Context(), "publish source fixture", func() error {
		return sess.appendMessagesLocked(messages, make([]MessageMetadata, len(messages)), historyTypeMessage)
	}); err != nil {
		t.Fatal(err)
	}
	var source ExternalContextSource
	if err := sess.ReadExternal(t.Context(), func(state ExternalState) error { source = state.ContextSource; return nil }); err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(agentschema.UserMessage("New input must not enter the captured history.")); err != nil {
		t.Fatal(err)
	}
	if err := sess.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(filepath.Dir(path), "history.idx.json")); err != nil {
		t.Fatal(err)
	}
	sess, err = loadSession(path)
	if err != nil {
		t.Fatal(err)
	}
	read := func(ctx context.Context, selected ExternalContextSource) ([]string, error) {
		var got []string
		err := sess.ReadExternal(ctx, func(state ExternalState) error {
			return state.ScanContext(selected, func(record ExternalContextRecord) error {
				if record.Message != nil {
					got = append(got, record.Message.Content)
				}
				return nil
			})
		})
		return got, err
	}
	got, err := read(t.Context(), source)
	if err != nil || len(got) != len(messages) {
		t.Fatalf("captured interval after cold reopen: messages=%d error=%v", len(got), err)
	}
	for index, text := range got {
		if text != messages[index].Content {
			t.Fatalf("source message %d changed: %q", index, text)
		}
	}
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := read(cancelled, source); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled source read: %v", err)
	}
	foreign := source
	foreign.incarnation = "replacement-journal"
	if _, err := read(t.Context(), foreign); !errors.Is(err, ErrContextRevisionConflict) {
		t.Fatalf("foreign source accepted: %v", err)
	}
	if err := appendReleasedClearMarker(sess); err != nil {
		t.Fatal(err)
	}
	if got, err := read(t.Context(), source); !errors.Is(err, ErrContextRevisionConflict) || len(got) != 0 {
		t.Fatalf("cleared source revived: %v, %v", got, err)
	}
}

func TestExternalContextSourceRejectsClearDuringPagedRead(t *testing.T) {
	for _, clearAt := range []int{1, 70} {
		t.Run(fmt.Sprint(clearAt), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "history.jsonl")
			sess, err := createSessionWithRuntimeConfig("history", path, "", nil)
			if err != nil {
				t.Fatal(err)
			}
			defer sess.Close()
			for index := range 70 {
				if err := sess.Append(agentschema.UserMessage(fmt.Sprintf("original-%d", index))); err != nil {
					t.Fatal(err)
				}
			}
			other, err := loadSession(path)
			if err != nil {
				t.Fatal(err)
			}
			defer other.Close()
			visited := 0
			err = sess.ReadExternal(t.Context(), func(state ExternalState) error {
				return state.ScanContext(state.ContextSource, func(record ExternalContextRecord) error {
					if record.Message != nil {
						visited++
						if visited == clearAt {
							return appendReleasedClearMarker(other)
						}
					}
					return nil
				})
			})
			if !errors.Is(err, ErrContextRevisionConflict) {
				t.Fatalf("source read ignored an independently committed clear: %v", err)
			}
		})
	}
}
