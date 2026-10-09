package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"

	agentcompaction "github.com/alfredxw/denova/agent/context/compaction"
	agenthistory "github.com/alfredxw/denova/agent/context/history"
	agentengine "github.com/alfredxw/denova/agent/engine"
	agentschema "github.com/alfredxw/denova/agent/schema"
	agentsession "github.com/alfredxw/denova/agent/session"
	agentcanonical "github.com/alfredxw/denova/agent/session/canonical"
)

func TestCanonicalContextRetiresCompactedBodies(t *testing.T) {
	for _, pairs := range []int{100, 1000} {
		t.Run(strconv.Itoa(pairs), func(t *testing.T) {
			ctx := t.Context()
			store := canonicalMessageTestStore{Store: agentsession.Memory()}
			source := &testHistorySource{head: agentcanonical.CanonicalHistoryHead{Identity: "lane", Revision: "1"}}
			for range pairs {
				source.messages = append(source.messages, agentschema.UserMessage("previous request"), agentschema.AssistantMessage(strings.Repeat("archived evidence ", 1024), nil))
			}
			source.messages = append(source.messages, agentschema.UserMessage("keep this request"), agentschema.AssistantMessage("keep this response", nil))
			open := func() (*Agent, *Session) {
				owner, err := New(ctx, agentengine.Definition{Model: &lifecycleModel{responses: []*agentschema.Message{agentschema.AssistantMessage("answer 1", nil), agentschema.AssistantMessage("answer 2", nil), agentschema.AssistantMessage("answer 3", nil)}}, Canonical: source, Compaction: windowCompactionManager{}}, WithSessionStore(store))
				if err != nil {
					t.Fatal(err)
				}
				sess, err := owner.Session(ctx, agentsession.Named("compacted-window"))
				if err != nil {
					t.Fatal(err)
				}
				source.log = sess.log
				return owner, sess
			}
			owner, sess := open()
			compact := agenthistory.CompactionRecord{Version: 2, ID: "summary", Revision: 1, Summary: "Previous evidence summarized.", ReplacementTo: pairs * 2}
			sess.capabilities[agenthistory.CompactionCapability], _ = json.Marshal(compact)
			if err := sess.persistCapabilitiesLocked(ctx); err != nil {
				t.Fatal(err)
			}
			if err := sess.LoadCanonicalHistory(ctx, source); err != nil {
				t.Fatal(err)
			}
			assertWindow := func() {
				t.Helper()
				if len(sess.engineState) > 16<<10 {
					t.Fatalf("compacted active context still contains archived bodies: %d bytes", len(sess.engineState))
				}
				state, err := decodeJournalTranscript(sess.engineState)
				if err != nil {
					t.Fatal(err)
				}
				actual, err := state.Archive.EffectiveCompactionMessages(state.Messages, compact, true, 1024)
				if err != nil {
					t.Fatal(err)
				}
				want, err := agenthistory.EffectiveCompactionMessages(agentcanonical.CanonicalContextStateOrder(source.messages), compact, true, 1024)
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(actual, want) {
					t.Fatal("active window changed the model-visible projection")
				}
			}
			assertWindow()
			for range 3 {
				if err := sess.LoadCanonicalHistory(ctx, source); err != nil {
					t.Fatal(err)
				}
				run, err := sess.Run(ctx, agentschema.Text("Continue"))
				if err != nil {
					t.Fatal(err)
				}
				if result, err := run.Wait(ctx); err != nil || result.Status != agentschema.ResultCompleted {
					t.Fatalf("run: %+v %v", result, err)
				}
				assertWindow()
			}
			if source.reads != 1 {
				t.Fatalf("warm source reads = %d, want one initial reconstruction", source.reads)
			}
			if err := owner.Close(ctx); err != nil {
				t.Fatal(err)
			}
			owner, sess = open()
			defer owner.Close(ctx)
			if err := sess.LoadCanonicalHistory(ctx, source); err != nil {
				t.Fatal(err)
			}
			assertWindow()
			if source.reads != 2 {
				t.Fatalf("cold checkpoint did not read its canonical window: %d", source.reads)
			}
			removed, err := sess.RemoveCompaction(ctx, agentcompaction.CompactionRemoveRequest{})
			if err != nil || !removed {
				t.Fatalf("remove: %t %v", removed, err)
			}
			if source.reads != 3 {
				t.Fatalf("explicit removal did not read the original journal once: %d", source.reads)
			}
			state, err := decodeJournalTranscript(sess.engineState)
			if err != nil {
				t.Fatal(err)
			}
			if state.Archive != nil || !reflect.DeepEqual(state.Messages, agentcanonical.CanonicalContextStateOrder(source.messages)) {
				t.Fatal("removal did not restore exact original messages")
			}
		})
	}
}

func TestCanonicalArchiveRejectsEditedLaneAndRetainsAppends(t *testing.T) {
	ctx := t.Context()
	owner, err := New(ctx, agentengine.Definition{Model: &lifecycleModel{}}, WithSessionStore(canonicalMessageTestStore{Store: agentsession.Memory()}))
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close(ctx)
	sess, err := owner.Session(ctx, agentsession.Named("edited-window"))
	if err != nil {
		t.Fatal(err)
	}
	source := &testHistorySource{head: agentcanonical.CanonicalHistoryHead{Identity: "lane", Revision: "1"}, messages: []*agentschema.Message{agentschema.UserMessage("old"), agentschema.AssistantMessage("archived", nil), agentschema.UserMessage("recent"), agentschema.AssistantMessage("recent answer", nil)}}
	sess.capabilities[agenthistory.CompactionCapability], _ = json.Marshal(agenthistory.CompactionRecord{Version: 2, ID: "summary", Revision: 1, Summary: "summary", ReplacementTo: 2})
	if err := sess.LoadCanonicalHistory(ctx, source); err != nil {
		t.Fatal(err)
	}
	source.messages = append(source.messages, agentschema.UserMessage("external runtime append"), agentschema.AssistantMessage("external answer", nil))
	source.head.Revision = "2"
	if err := sess.LoadCanonicalHistory(ctx, source); err != nil {
		t.Fatal(err)
	}
	if _, present := sess.capabilities[agenthistory.CompactionCapability]; !present {
		t.Fatal("append invalidated the accepted summary")
	}
	source.messages[0] = agentschema.UserMessage("edited archived instruction")
	source.head = agentcanonical.CanonicalHistoryHead{Identity: "lane/edited", Revision: "3"}
	if err := sess.LoadCanonicalHistory(ctx, source); err != nil {
		t.Fatal(err)
	}
	if _, present := sess.capabilities[agenthistory.CompactionCapability]; present {
		t.Fatal("editing an archived body retained a stale summary")
	}
	state, err := decodeJournalTranscript(sess.engineState)
	if err != nil {
		t.Fatal(err)
	}
	if state.Archive != nil || state.Messages[0].Content != "edited archived instruction" {
		t.Fatal("edited lane was not reconstructed")
	}
	source.messages = nil
	source.head = agentcanonical.CanonicalHistoryHead{Identity: "lane/clear", Revision: "4"}
	if err := sess.LoadCanonicalHistory(ctx, source); err != nil {
		t.Fatal(err)
	}
	state, err = decodeJournalTranscript(sess.engineState)
	if err != nil || len(state.Messages) != 0 {
		t.Fatalf("clear resurrected archive: %+v %v", state, err)
	}
}

type windowCompactionManager struct{}

func (windowCompactionManager) Identity() agentschema.CapabilityIdentity {
	return agentschema.CapabilityIdentity{Kind: "test.window.compaction", Version: 1}
}

func (windowCompactionManager) SummaryLimitBytes() int { return 1024 }

func (windowCompactionManager) Plan(_ context.Context, request agentcompaction.CompactionPlanRequest) (agenthistory.CompactionPlan, error) {
	if !request.Force || len(request.Groups) == 0 {
		return agenthistory.CompactionPlan{Action: agenthistory.CompactionNone}, nil
	}
	return agenthistory.CompactionPlan{Action: agenthistory.CompactionCreate, GroupCount: len(request.Groups), Validation: agenthistory.CompactionValidationPolicy{HardLimitBytes: 4 << 20}}, nil
}

func (windowCompactionManager) Compact(context.Context, agentcompaction.CompactionCompactRequest) (agentcompaction.CompactionCheckpoint, error) {
	return agentcompaction.CompactionCheckpoint{Summary: "Previous evidence summarized."}, nil
}

type testHistorySource struct {
	log      agentsession.Log
	messages []*agentschema.Message
	head     agentcanonical.CanonicalHistoryHead
	reads    int
}

func (s *testHistorySource) CanonicalHistoryHead(context.Context) (agentcanonical.CanonicalHistoryHead, error) {
	return s.head, nil
}

func (s *testHistorySource) CanonicalMessages(context.Context) ([]*agentschema.Message, error) {
	s.reads++
	return agentschema.CloneMessages(s.messages), nil
}

func (s *testHistorySource) VisitCanonicalMessages(_ context.Context, visit func(*agentschema.Message) error) error {
	s.reads++
	for _, message := range s.messages {
		if err := visit(message.Clone()); err != nil {
			return err
		}
	}
	return nil
}

func (s *testHistorySource) Identity() agentschema.CapabilityIdentity {
	return agentschema.CapabilityIdentity{Kind: "test.window.canonical", Version: 1}
}

func (s *testHistorySource) commit(ctx context.Context, messages []*agentschema.Message, checkpoint agentcanonical.CanonicalCheckpoint) (agentcanonical.CommitReceipt, error) {
	revision, _ := strconv.Atoi(s.head.Revision)
	receipt := agentcanonical.CommitReceipt{Revision: fmt.Sprint(revision + 1)}
	if checkpoint != nil {
		value, err := checkpoint(receipt)
		if err != nil {
			return agentcanonical.CommitReceipt{}, err
		}
		if _, err := s.log.Append(ctx, value.ExpectedRevision, value.Records...); err != nil {
			return agentcanonical.CommitReceipt{}, err
		}
	}
	s.messages = append(s.messages, agentschema.CloneMessages(messages)...)
	s.head.Revision = receipt.Revision
	return receipt, nil
}

func (s *testHistorySource) MaterializeInput(ctx context.Context, r agentcanonical.InputCommitRequest) (agentcanonical.CommitReceipt, error) {
	return s.commit(ctx, []*agentschema.Message{agentschema.UserMessageWithAttachments(r.Input.Text, r.Input.Attachments)}, r.Checkpoint)
}

func (s *testHistorySource) CommitContext(ctx context.Context, r agentcanonical.ContextCommitRequest) (agentcanonical.CommitReceipt, error) {
	messages := make([]*agentschema.Message, len(r.Messages))
	for i := range r.Messages {
		messages[i] = r.Messages[i].Clone()
	}
	return s.commit(ctx, messages, r.Checkpoint)
}

func (s *testHistorySource) CommitOutput(ctx context.Context, r agentcanonical.OutputCommitRequest) (agentcanonical.OutputCommitReceipt, error) {
	receipt, err := s.commit(ctx, []*agentschema.Message{r.Message.Clone()}, r.Checkpoint)
	return agentcanonical.OutputCommitReceipt{Revision: receipt.Revision}, err
}

func TestCanonicalArchivedCheckpointRetainsIncompleteBatchOnColdLoad(t *testing.T) {
	ctx := t.Context()
	store := canonicalMessageTestStore{Store: agentsession.Memory()}
	source := &testHistorySource{head: agentcanonical.CanonicalHistoryHead{Identity: "lane", Revision: "1"}, messages: []*agentschema.Message{agentschema.UserMessage("old"), agentschema.AssistantMessage("old answer", nil), agentschema.UserMessage("current instruction")}}
	open := func() (*Agent, *Session) {
		owner, err := New(ctx, agentengine.Definition{Model: &lifecycleModel{}}, WithSessionStore(store))
		if err != nil {
			t.Fatal(err)
		}
		session, err := owner.Session(ctx, agentsession.Named("pending-window"))
		if err != nil {
			t.Fatal(err)
		}
		source.log = session.log
		return owner, session
	}
	owner, sess := open()
	sess.capabilities[agenthistory.CompactionCapability], _ = json.Marshal(agenthistory.CompactionRecord{Version: 2, ID: "summary", Revision: 1, Summary: "summary", ReplacementTo: 2})
	if err := sess.persistCapabilitiesLocked(ctx); err != nil {
		t.Fatal(err)
	}
	if err := sess.LoadCanonicalHistory(ctx, source); err != nil {
		t.Fatal(err)
	}
	state, err := decodeJournalTranscript(sess.engineState)
	if err != nil {
		t.Fatal(err)
	}
	state.ActiveModelUser, state.ActiveUserIndex = agentschema.UserMessage("rendered current instruction"), 2
	state.Messages = append(state.Messages, agentschema.AssistantMessage("inspect", []agentschema.ToolCall{{ID: "a", Function: agentschema.FunctionCall{Name: "read", Arguments: `{}`}}, {ID: "b", Function: agentschema.FunctionCall{Name: "read", Arguments: `{}`}}}), agentschema.ToolMessage(agentschema.TextToolResult("first result"), "a"))
	sess.engineState, _ = json.Marshal(state)
	if err := sess.persistTranscriptLocked(ctx); err != nil {
		t.Fatal(err)
	}
	before := append(json.RawMessage(nil), sess.engineState...)
	if len(sess.messageCheckpoint.Pending) != 2 || sess.messageCheckpoint.MessageCount != 3 {
		t.Fatalf("invalid pending checkpoint: %+v", sess.messageCheckpoint)
	}
	if err := owner.Close(ctx); err != nil {
		t.Fatal(err)
	}
	owner, sess = open()
	defer owner.Close(ctx)
	if err := sess.LoadCanonicalHistory(ctx, source); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, sess.engineState) || source.reads != 2 {
		t.Fatal("cold canonical recovery lost the partial tool batch")
	}
}

func TestCanonicalArchiveRebuildsWhenSummaryLimitIsLowered(t *testing.T) {
	ctx := t.Context()
	source := &testHistorySource{head: agentcanonical.CanonicalHistoryHead{Identity: "lane", Revision: "1"}}
	for range 4 {
		source.messages = append(source.messages, agentschema.UserMessage("request"), agentschema.AssistantMessage(strings.Repeat("evidence ", 2000), nil))
	}
	owner, err := New(ctx, agentengine.Definition{Model: &lifecycleModel{}, Compaction: windowCompactionManager{}}, WithSessionStore(canonicalMessageTestStore{Store: agentsession.Memory()}))
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close(ctx)
	sess, err := owner.Session(ctx, agentsession.Named("lower-summary-limit"))
	if err != nil {
		t.Fatal(err)
	}
	sess.capabilities[agenthistory.CompactionCapability], _ = json.Marshal(agenthistory.CompactionRecord{Version: 2, ID: "old-summary", Revision: 1, Summary: strings.Repeat("old summary ", 200), ReplacementTo: 4})
	if err := sess.LoadCanonicalHistory(ctx, source); err != nil {
		t.Fatal(err)
	}
	compacted, err := sess.Compact(ctx, agentcompaction.CompactionRequest{Force: true})
	if err != nil || !compacted.Changed || source.reads != 2 {
		t.Fatalf("oversized checkpoint was not rebuilt from its source: %+v reads=%d error=%v", compacted, source.reads, err)
	}
}
