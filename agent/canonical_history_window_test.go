package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"

	agentsession "github.com/alfredxw/denova/agent/session"
)

func TestCanonicalContextRetiresCompactedBodies(t *testing.T) {
	for _, pairs := range []int{100, 1000} {
		t.Run(strconv.Itoa(pairs), func(t *testing.T) {
			ctx := t.Context()
			store := canonicalMessageTestStore{Store: agentsession.Memory()}
			source := &testHistorySource{head: CanonicalHistoryHead{Identity: "lane", Revision: "1"}}
			for range pairs {
				source.messages = append(source.messages, UserMessage("previous request"), AssistantMessage(strings.Repeat("archived evidence ", 1024), nil))
			}
			source.messages = append(source.messages, UserMessage("keep this request"), AssistantMessage("keep this response", nil))
			open := func() (*Agent, *Session) {
				owner, err := New(ctx, Definition{Model: &lifecycleModel{responses: []*Message{AssistantMessage("answer 1", nil), AssistantMessage("answer 2", nil), AssistantMessage("answer 3", nil)}}, Canonical: source, Compaction: windowCompactionManager{}}, WithSessionStore(store))
				if err != nil {
					t.Fatal(err)
				}
				sess, err := owner.Session(ctx, NamedSession("compacted-window"))
				if err != nil {
					t.Fatal(err)
				}
				return owner, sess
			}
			owner, sess := open()
			compact := compactionRecord{Version: 2, ID: "summary", Revision: 1, Summary: "Previous evidence summarized.", ReplacementTo: pairs * 2}
			sess.capabilities[compactionCapability], _ = json.Marshal(compact)
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
				state, err := decodeEngineTranscript(sess.engineState)
				if err != nil {
					t.Fatal(err)
				}
				actual, err := state.Archive.effectiveCompactionMessages(state.Messages, compact, true, 1024)
				if err != nil {
					t.Fatal(err)
				}
				want, err := effectiveCompactionMessages(canonicalContextStateOrder(source.messages), compact, true, 1024)
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
				run, err := sess.Run(ctx, Text("Continue"))
				if err != nil {
					t.Fatal(err)
				}
				if result, err := run.Wait(ctx); err != nil || result.Status != ResultCompleted {
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
			if source.reads != 1 {
				t.Fatalf("cold aligned checkpoint reread archived bodies: %d", source.reads)
			}
			removed, err := sess.RemoveCompaction(ctx, CompactionRemoveRequest{})
			if err != nil || !removed {
				t.Fatalf("remove: %t %v", removed, err)
			}
			if source.reads != 2 {
				t.Fatalf("explicit removal did not read the original journal once: %d", source.reads)
			}
			state, err := decodeEngineTranscript(sess.engineState)
			if err != nil {
				t.Fatal(err)
			}
			if state.Archive != nil || !reflect.DeepEqual(state.Messages, canonicalContextStateOrder(source.messages)) {
				t.Fatal("removal did not restore exact original messages")
			}
		})
	}
}

func TestCanonicalArchiveRejectsEditedLaneAndRetainsAppends(t *testing.T) {
	ctx := t.Context()
	owner, err := New(ctx, Definition{Model: &lifecycleModel{}}, WithSessionStore(canonicalMessageTestStore{Store: agentsession.Memory()}))
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close(ctx)
	sess, err := owner.Session(ctx, NamedSession("edited-window"))
	if err != nil {
		t.Fatal(err)
	}
	source := &testHistorySource{head: CanonicalHistoryHead{Identity: "lane", Revision: "1"}, messages: []*Message{UserMessage("old"), AssistantMessage("archived", nil), UserMessage("recent"), AssistantMessage("recent answer", nil)}}
	sess.capabilities[compactionCapability], _ = json.Marshal(compactionRecord{Version: 2, ID: "summary", Revision: 1, Summary: "summary", ReplacementTo: 2})
	if err := sess.LoadCanonicalHistory(ctx, source); err != nil {
		t.Fatal(err)
	}
	source.messages = append(source.messages, UserMessage("external runtime append"), AssistantMessage("external answer", nil))
	source.head.Revision = "2"
	if err := sess.LoadCanonicalHistory(ctx, source); err != nil {
		t.Fatal(err)
	}
	if _, present := sess.capabilities[compactionCapability]; !present {
		t.Fatal("append invalidated the accepted summary")
	}
	source.messages[0] = UserMessage("edited archived instruction")
	source.head = CanonicalHistoryHead{Identity: "lane/edited", Revision: "3"}
	if err := sess.LoadCanonicalHistory(ctx, source); err != nil {
		t.Fatal(err)
	}
	if _, present := sess.capabilities[compactionCapability]; present {
		t.Fatal("editing an archived body retained a stale summary")
	}
	state, err := decodeEngineTranscript(sess.engineState)
	if err != nil {
		t.Fatal(err)
	}
	if state.Archive != nil || state.Messages[0].Content != "edited archived instruction" {
		t.Fatal("edited lane was not reconstructed")
	}
	source.messages = nil
	source.head = CanonicalHistoryHead{Identity: "lane/clear", Revision: "4"}
	if err := sess.LoadCanonicalHistory(ctx, source); err != nil {
		t.Fatal(err)
	}
	state, err = decodeEngineTranscript(sess.engineState)
	if err != nil || len(state.Messages) != 0 {
		t.Fatalf("clear resurrected archive: %+v %v", state, err)
	}
}

type windowCompactionManager struct{}

func (windowCompactionManager) Identity() CapabilityIdentity {
	return CapabilityIdentity{Kind: "test.window.compaction", Version: 1}
}
func (windowCompactionManager) SummaryLimitBytes() int { return 1024 }
func (windowCompactionManager) Plan(_ context.Context, request CompactionPlanRequest) (CompactionPlan, error) {
	if !request.Force || len(request.Groups) == 0 {
		return CompactionPlan{Action: CompactionNone}, nil
	}
	return CompactionPlan{Action: CompactionCreate, GroupCount: len(request.Groups), Validation: CompactionValidationPolicy{HardLimitBytes: 4 << 20}}, nil
}
func (windowCompactionManager) Compact(context.Context, CompactionCompactRequest) (CompactionCheckpoint, error) {
	return CompactionCheckpoint{Summary: "Previous evidence summarized."}, nil
}

type testHistorySource struct {
	messages []*Message
	head     CanonicalHistoryHead
	reads    int
}

func (s *testHistorySource) CanonicalHistoryHead(context.Context) (CanonicalHistoryHead, error) {
	return s.head, nil
}
func (s *testHistorySource) CanonicalMessages(context.Context) ([]*Message, error) {
	s.reads++
	return cloneMessages(s.messages), nil
}
func (s *testHistorySource) Identity() CapabilityIdentity {
	return CapabilityIdentity{Kind: "test.window.canonical", Version: 1}
}
func (s *testHistorySource) commit(ctx context.Context, messages []*Message, checkpoint CanonicalCheckpoint) (CommitReceipt, error) {
	revision, _ := strconv.Atoi(s.head.Revision)
	receipt := CommitReceipt{Revision: fmt.Sprint(revision + 1)}
	if checkpoint != nil {
		value, err := checkpoint(receipt)
		if err != nil {
			return CommitReceipt{}, err
		}
		run := ctx.Value(canonicalRunKey{}).(*Run)
		if _, err := run.session.log.Append(ctx, value.ExpectedRevision, value.Records...); err != nil {
			return CommitReceipt{}, err
		}
	}
	s.messages = append(s.messages, cloneMessages(messages)...)
	s.head.Revision = receipt.Revision
	return receipt, nil
}
func (s *testHistorySource) MaterializeInput(ctx context.Context, r InputCommitRequest) (CommitReceipt, error) {
	return s.commit(ctx, []*Message{UserMessageWithAttachments(r.Input.Text, r.Input.Attachments)}, r.Checkpoint)
}
func (s *testHistorySource) CommitContext(ctx context.Context, r ContextCommitRequest) (CommitReceipt, error) {
	messages := make([]*Message, len(r.Messages))
	for i := range r.Messages {
		messages[i] = r.Messages[i].Clone()
	}
	return s.commit(ctx, messages, r.Checkpoint)
}
func (s *testHistorySource) CommitOutput(ctx context.Context, r OutputCommitRequest) (OutputCommitReceipt, error) {
	receipt, err := s.commit(ctx, []*Message{r.Message.Clone()}, r.Checkpoint)
	return OutputCommitReceipt{Revision: receipt.Revision}, err
}

func TestArchivedContextPreservesRawCoordinatesAndState(t *testing.T) {
	raw, state, err := advanceContextState(nil, []ContextFragment{testContextStateFragment("v1", "accepted state")}, contextStateSnapshot{}, compactionRecord{}, false)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		raw = append(raw, UserMessage(fmt.Sprint("instruction ", i)), AssistantMessage(fmt.Sprint("evidence ", i), nil))
	}
	active := 3
	compact := compactionRecord{Version: 2, ID: "first", Revision: 1, Summary: "summary", ReplacementTo: 7, RetainedUserFrom: &active}
	window, archive := archiveHistory(raw, nil, compact, state)
	assertProjection := func() {
		t.Helper()
		if archive.count(window) != len(raw) {
			t.Fatal("raw message count changed")
		}
		if err := validateContextStateSnapshotInArchive(state, window, archive); err != nil {
			t.Fatal(err)
		}
		expected, err := effectiveCompactionMessages(raw, compact, true, 1024)
		if err != nil {
			t.Fatal(err)
		}
		actual, err := archive.effectiveCompactionMessages(window, compact, true, 1024)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(actual, expected) {
			t.Fatal("compaction window changed model messages")
		}
		fullGroups, fullEnds, fullBytes := compactionGroups(raw, raw, compact, true)
		groups, ends, bytes := archive.compactionGroups(window, window, compact, true)
		if !reflect.DeepEqual(groups, fullGroups) || !reflect.DeepEqual(ends, fullEnds) || bytes != fullBytes {
			t.Fatal("compaction group coordinates changed")
		}
		for index := range raw {
			if archive.compactionMessageIndex(window, compact, true, index) != compactionMessageIndex(raw, compact, true, index) {
				t.Fatalf("raw coordinate %d changed", index)
			}
		}
	}
	assertProjection()
	compact.ReplacementTo = 11
	window, archive = archiveHistory(window, archive, compact, state)
	assertProjection()
	expected, nextState, err := advanceContextState(raw, []ContextFragment{testContextStateFragment("v1", "accepted state")}, state, compact, true)
	if err != nil {
		t.Fatal(err)
	}
	actual, windowState, err := archive.advanceContextState(window, []ContextFragment{testContextStateFragment("v1", "accepted state")}, state, compact, true)
	if err != nil || !reflect.DeepEqual(actual, expected) || !reflect.DeepEqual(windowState, nextState) {
		t.Fatalf("state reinjection changed raw positions: %v", err)
	}
}

func TestCanonicalArchivedCheckpointRetainsIncompleteBatchOnColdLoad(t *testing.T) {
	ctx := t.Context()
	store := canonicalMessageTestStore{Store: agentsession.Memory()}
	source := &testHistorySource{head: CanonicalHistoryHead{Identity: "lane", Revision: "1"}, messages: []*Message{UserMessage("old"), AssistantMessage("old answer", nil), UserMessage("current instruction")}}
	open := func() (*Agent, *Session) {
		owner, err := New(ctx, Definition{Model: &lifecycleModel{}}, WithSessionStore(store))
		if err != nil {
			t.Fatal(err)
		}
		session, err := owner.Session(ctx, NamedSession("pending-window"))
		if err != nil {
			t.Fatal(err)
		}
		return owner, session
	}
	owner, sess := open()
	sess.capabilities[compactionCapability], _ = json.Marshal(compactionRecord{Version: 2, ID: "summary", Revision: 1, Summary: "summary", ReplacementTo: 2})
	if err := sess.persistCapabilitiesLocked(ctx); err != nil {
		t.Fatal(err)
	}
	if err := sess.LoadCanonicalHistory(ctx, source); err != nil {
		t.Fatal(err)
	}
	state, err := decodeEngineTranscript(sess.engineState)
	if err != nil {
		t.Fatal(err)
	}
	state.ActiveModelUser, state.ActiveUserIndex = UserMessage("rendered current instruction"), 2
	state.Messages = append(state.Messages, AssistantMessage("inspect", []ToolCall{{ID: "a", Function: FunctionCall{Name: "read", Arguments: `{}`}}, {ID: "b", Function: FunctionCall{Name: "read", Arguments: `{}`}}}), ToolMessage(TextToolResult("first result"), "a"))
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
	if !reflect.DeepEqual(before, sess.engineState) || source.reads != 1 {
		t.Fatal("cold active window lost partial tool batch or reread source")
	}
}

func TestCanonicalArchiveRebuildsWhenSummaryLimitIsLowered(t *testing.T) {
	ctx := t.Context()
	source := &testHistorySource{head: CanonicalHistoryHead{Identity: "lane", Revision: "1"}}
	for range 4 {
		source.messages = append(source.messages, UserMessage("request"), AssistantMessage(strings.Repeat("evidence ", 2000), nil))
	}
	owner, err := New(ctx, Definition{Model: &lifecycleModel{}, Compaction: windowCompactionManager{}}, WithSessionStore(canonicalMessageTestStore{Store: agentsession.Memory()}))
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close(ctx)
	sess, err := owner.Session(ctx, NamedSession("lower-summary-limit"))
	if err != nil {
		t.Fatal(err)
	}
	sess.capabilities[compactionCapability], _ = json.Marshal(compactionRecord{Version: 2, ID: "old-summary", Revision: 1, Summary: strings.Repeat("old summary ", 200), ReplacementTo: 4})
	if err := sess.LoadCanonicalHistory(ctx, source); err != nil {
		t.Fatal(err)
	}
	compacted, err := sess.Compact(ctx, CompactionRequest{Force: true})
	if err != nil || !compacted.Changed || source.reads != 2 {
		t.Fatalf("oversized checkpoint was not rebuilt from its source: %+v reads=%d error=%v", compacted, source.reads, err)
	}
}
