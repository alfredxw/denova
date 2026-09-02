package session

import (
	"context"
	"fmt"
	"testing"

	agent "github.com/alfredxw/denova/agent"
)

func TestReadCanonicalMessagesRebuildsBeyondResidentWindow(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("canonical-window")
	if err != nil {
		t.Fatal(err)
	}

	total := sessionRecentTransactionLimit + 25
	batch := make([]*agent.Message, total)
	for index := 0; index < total; index++ {
		batch[index] = agent.UserMessage(fmt.Sprintf("message-%03d", index))
	}
	if err := sess.AppendContextMessages(batch...); err != nil {
		t.Fatal(err)
	}
	if err := sess.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	sess, err = store.Get("canonical-window")
	if err != nil {
		t.Fatal(err)
	}
	resident, base, durable := sess.MessageWindow()
	if len(resident) != sessionRecentTransactionLimit || base != total-sessionRecentTransactionLimit || durable != total {
		t.Fatalf("resident window len=%d base=%d durable=%d", len(resident), base, durable)
	}

	messages, err := sess.ReadCanonicalMessages(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != total || messages[0].Content != "message-000" || messages[total-1].Content != fmt.Sprintf("message-%03d", total-1) {
		t.Fatalf("canonical messages len=%d first=%q last=%q", len(messages), messages[0].Content, messages[len(messages)-1].Content)
	}
}

func TestReadCanonicalMessagesStartsAfterLatestClear(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("canonical-clear")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(agent.UserMessage("before clear")); err != nil {
		t.Fatal(err)
	}
	if err := sess.Clear(); err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(agent.UserMessage("after clear")); err != nil {
		t.Fatal(err)
	}

	messages, err := sess.ReadCanonicalMessages(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || messages[0].Content != "after clear" {
		t.Fatalf("canonical messages after clear = %#v", messages)
	}
}
