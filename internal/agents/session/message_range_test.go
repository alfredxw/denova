package session

import (
	"context"
	"fmt"
	"testing"

	agent "github.com/alfredxw/denova/agent"
)

func TestReadMessageRangeUsesCanonicalJournalBeyondResidentWindow(t *testing.T) {
	directory := t.TempDir()
	store, err := NewStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("canonical-message-range")
	if err != nil {
		t.Fatal(err)
	}
	total := sessionRecentTransactionLimit + 9
	messages := make([]*agent.Message, total)
	for index := 0; index < total; index++ {
		messages[index] = agent.UserMessage(fmt.Sprintf("canonical-%03d", index))
	}
	if err := sess.AppendContextMessages(messages...); err != nil {
		t.Fatalf("append canonical message batch: %v", err)
	}
	resident, base, count := sess.MessageWindow()
	if len(resident) != sessionRecentTransactionLimit || base != total-sessionRecentTransactionLimit || count != total {
		t.Fatalf("resident window = len:%d base:%d count:%d", len(resident), base, count)
	}

	assertCanonicalMessageRange(t, sess, total)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reloadedStore, err := NewStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := reloadedStore.Get("canonical-message-range")
	if err != nil {
		t.Fatal(err)
	}
	resident, base, count = reloaded.MessageWindow()
	if len(resident) != sessionRecentTransactionLimit || base != total-sessionRecentTransactionLimit || count != total || resident[0].Content != "canonical-009" {
		t.Fatalf("reloaded resident window = len:%d base:%d count:%d first:%q", len(resident), base, count, resident[0].Content)
	}
	assertCanonicalMessageRange(t, reloaded, total)
}

func assertCanonicalMessageRange(t *testing.T, sess *Session, total int) {
	t.Helper()
	messages, err := sess.ReadMessageRange(context.Background(), 0, total)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != total {
		t.Fatalf("canonical range length = %d, want %d", len(messages), total)
	}
	for index, message := range messages {
		want := fmt.Sprintf("canonical-%03d", index)
		if message.Content != want {
			t.Fatalf("canonical message %d = %q, want %q", index, message.Content, want)
		}
	}
}
