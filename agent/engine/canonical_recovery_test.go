package engine

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/alfredxw/denova/agent/context/history"
	"github.com/alfredxw/denova/agent/schema"
	"github.com/alfredxw/denova/agent/session/canonical"
)

type recoveryHistory struct {
	head     canonical.CanonicalHistoryHead
	messages []*schema.Message
}

func (s recoveryHistory) CanonicalHistoryHead(context.Context) (canonical.CanonicalHistoryHead, error) {
	return s.head, nil
}
func (s recoveryHistory) CanonicalMessages(context.Context) ([]*schema.Message, error) {
	panic("recovery must use the streaming visitor")
}
func (s recoveryHistory) VisitCanonicalMessages(_ context.Context, visit func(*schema.Message) error) error {
	for _, message := range s.messages {
		if err := visit(message.Clone()); err != nil {
			return err
		}
	}
	return nil
}

func TestRestoreCanonicalCheckpointAcceptsReleasedInlineMetadata(t *testing.T) {
	source := recoveryHistory{head: canonical.CanonicalHistoryHead{Identity: "lane", Revision: "1"}, messages: []*schema.Message{
		schema.UserMessage("archived user"), schema.AssistantMessage("archived answer", nil),
		schema.UserMessage("current user"), schema.AssistantMessage("current answer", nil),
	}}
	contextState := schema.UserMessage("current workspace context")
	contextState.Extra = map[string]any{"agent.context_state": "v1"}
	source.messages = append(source.messages[:3], contextState, source.messages[3])
	state := engineTranscript{Version: 2, HistoryHead: source.head, Archive: &history.HistoryArchive{From: 0, To: 2}, Messages: canonical.CanonicalContextStateOrder(source.messages)[2:]}
	checkpoint, err := canonicalMessageCheckpoint(state)
	if err != nil {
		t.Fatal(err)
	}
	for _, released := range []bool{false, true} {
		t.Run(map[bool]string{false: "message-free", true: "released-inline"}[released], func(t *testing.T) {
			if released {
				checkpoint.Metadata, err = json.Marshal(state)
				if err != nil {
					t.Fatal(err)
				}
			}
			raw, matched, err := RestoreCanonicalCheckpoint(t.Context(), checkpoint, source.head, source)
			if err != nil || !matched {
				t.Fatalf("restore: matched=%t err=%v", matched, err)
			}
			restored, err := decodeEngineTranscript(raw)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(restored, state) {
				t.Fatalf("restored transcript differs: %#v", restored)
			}
			fresh, err := CanonicalMessageCheckpoint(raw)
			if err != nil {
				t.Fatal(err)
			}
			var metadata engineTranscript
			if err := json.Unmarshal(fresh.Metadata, &metadata); err != nil {
				t.Fatal(err)
			}
			if len(metadata.Messages) != 0 {
				t.Fatal("new checkpoint repeated released message bodies")
			}
		})
	}
}
