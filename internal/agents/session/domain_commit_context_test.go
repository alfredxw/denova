package session

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	agent "github.com/alfredxw/denova/agent"

	"denova/internal/agents/conversationjournal"
)

func TestDomainCommitContextSurvivesMessageWindowTrimming(t *testing.T) {
	for _, batches := range []int{99, 100, 199} {
		t.Run(fmt.Sprintf("batches_%d", batches), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "domain-context.jsonl")
			sess, err := createSession("domain-context", path, "")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = sess.Close() })
			observer, err := loadSession(path)
			if err != nil {
				t.Fatal(err)
			}
			defer observer.Close()
			identity := DomainCommitIdentity{CommandID: "history", OperationID: "history-run", Cycle: 1}
			for sequence := 0; sequence < batches; sequence++ {
				messages := domainContextToolMessages(sequence, 1)
				if _, err := sess.CommitContextBatch(t.Context(), sess.ContextCursor(), identity, sequence, messages); err != nil {
					t.Fatal(err)
				}
			}
			intent, err := NewDomainCommitIntent(
				DomainCommitIdentity{CommandID: "new-input", OperationID: "new-run", Cycle: 1},
				agent.UserMessage("continue writing"), MessageMetadata{},
			)
			if err != nil {
				t.Fatal(err)
			}
			receipt, err := sess.CommitDomainMessage(intent)
			if err != nil {
				t.Fatal(err)
			}
			t.Run("hot", func(t *testing.T) { assertLatestDomainCommitContext(t, sess, intent) })
			if err := observer.RefreshCanonical(t.Context()); err != nil {
				t.Fatal(err)
			}
			t.Run("tail_refresh", func(t *testing.T) { assertLatestDomainCommitContext(t, observer, intent) })
			for _, phase := range []string{"indexed_reload", "rebuilt_index"} {
				if err := sess.Close(); err != nil {
					t.Fatal(err)
				}
				if phase == "rebuilt_index" {
					if err := os.Remove(conversationjournal.SidecarPath(path)); err != nil {
						t.Fatal(err)
					}
				}
				sess, err = loadSession(path)
				if err != nil {
					t.Fatal(err)
				}
				t.Run(phase, func(t *testing.T) { assertLatestDomainCommitContext(t, sess, intent) })
			}
			if retry, err := sess.CommitDomainMessage(intent); err != nil || retry != receipt || sess.MessageCountTotal() != batches*2+1 {
				t.Fatalf("recovered retry=%#v count=%d err=%v", retry, sess.MessageCountTotal(), err)
			}
			next, err := NewDomainCommitIntent(
				DomainCommitIdentity{CommandID: "next-input", OperationID: "next-run", Cycle: 1},
				agent.UserMessage("continue after restart"), MessageMetadata{},
			)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := sess.CommitDomainMessage(next); err != nil {
				t.Fatal(err)
			}
			assertLatestDomainCommitContext(t, sess, next)
		})
	}
}

func TestDomainCommitContextIgnoresDisplayWindowOffset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "display-context.jsonl")
	sess, err := createSession("display-context", path, "")
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	if err := sess.AppendContextMessages(domainContextToolMessages(0, 100)...); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 220; index++ {
		if err := sess.AppendDisplayEvent(DisplayEvent{ID: fmt.Sprintf("display-%d", index), Role: "thinking", Content: "working"}); err != nil {
			t.Fatal(err)
		}
	}
	intent, err := NewDomainCommitIntent(
		DomainCommitIdentity{CommandID: "new-input", OperationID: "new-run", Cycle: 1},
		agent.UserMessage("continue writing"), MessageMetadata{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sess.CommitDomainMessage(intent); err != nil {
		t.Fatal(err)
	}
	assertLatestDomainCommitContext(t, sess, intent)
}

func TestDomainCommitOutsideEffectiveWindowRemainsIdempotent(t *testing.T) {
	for _, pairs := range []int{100, 201} {
		t.Run(fmt.Sprintf("tool_pairs_%d", pairs), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "evicted-context.jsonl")
			sess, err := createSession("evicted-context", path, "")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = sess.Close() })
			intent, err := NewDomainCommitIntent(
				DomainCommitIdentity{CommandID: "old-input", OperationID: "old-run", Cycle: 1},
				agent.UserMessage("start writing"), MessageMetadata{},
			)
			if err != nil {
				t.Fatal(err)
			}
			receipt, err := sess.CommitDomainMessage(intent)
			if err != nil {
				t.Fatal(err)
			}
			if err := sess.AppendContextMessages(domainContextToolMessages(0, pairs)...); err != nil {
				t.Fatal(err)
			}
			for _, phase := range []string{"trimmed", "cleared", "reopened"} {
				if phase == "cleared" {
					if err := sess.AppendClearMarker(); err != nil {
						t.Fatal(err)
					}
				}
				if phase == "reopened" {
					if err := sess.Close(); err != nil {
						t.Fatal(err)
					}
					sess, err = loadSession(path)
					if err != nil {
						t.Fatal(err)
					}
				}
				t.Run(phase, func(t *testing.T) {
					if _, _, found, err := sess.SnapshotContextForDomainCommit(intent.Identity, agent.User, intent.Hash); err != nil || found {
						t.Errorf("evicted input: found=%t err=%v", found, err)
					}
					if retry, err := sess.CommitDomainMessage(intent); err != nil || retry != receipt || sess.MessageCountTotal() != pairs*2+1 {
						t.Errorf("evicted input retry=%#v count=%d err=%v", retry, sess.MessageCountTotal(), err)
					}
				})
			}
		})
	}
}

func domainContextToolMessages(start, pairs int) []*agent.Message {
	messages := make([]*agent.Message, 0, pairs*2)
	for index := start; index < start+pairs; index++ {
		callID := fmt.Sprintf("call-%d", index)
		messages = append(messages,
			agent.AssistantMessage("checking", []agent.ToolCall{{ID: callID, Type: "function", Function: agent.FunctionCall{Name: "inspect", Arguments: `{}`}}}),
			agent.ToolMessage(agent.TextToolResult("evidence"), callID, agent.WithToolName("inspect")),
		)
	}
	return messages
}

func assertLatestDomainCommitContext(t *testing.T, sess *Session, intent DomainCommitIntent) {
	t.Helper()
	snapshot, index, found, err := sess.SnapshotContextForDomainCommit(intent.Identity, intent.Message.Role, intent.Hash)
	if err != nil {
		t.Fatal(err)
	}
	if !found || index != len(snapshot.EffectiveMessages)-1 {
		t.Fatalf("latest input index=%d found=%t messages=%d", index, found, len(snapshot.EffectiveMessages))
	}
	if !reflect.DeepEqual(snapshot.EffectiveMessages[index], &intent.Message) {
		t.Fatalf("latest input=%#v want=%#v", snapshot.EffectiveMessages[index], intent.Message)
	}
}
