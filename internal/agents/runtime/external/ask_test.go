package external

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"denova/config"
	"denova/internal/agents/conversation"
	"denova/internal/agents/conversationconfig"
	externaljournal "denova/internal/agents/runtime/external/journal"
	"denova/internal/agents/session"
	agent "github.com/alfredxw/denova/agent"
)

func TestResolveAskRoutesOnlyOwnedQuestions(t *testing.T) {
	_, store, sess := pendingAskFixture(t, `{"questions":[{"id":"tone","prompt":"Which tone?"}]}`)
	defer store.Close()
	ctx := context.Background()
	service := &Service{}
	for _, askID := range []string{"permission-tool-native", "ask-native"} {
		for _, status := range []string{session.AskAnswered, session.AskCancelled} {
			result, owned, err := service.ResolveAsk(ctx, "project-1", sess, askID, status, nil, "cancelled")
			if owned || err != nil || !reflect.DeepEqual(result, conversation.HostAskResolution{}) {
				t.Fatalf("unowned question must fall through: %s %s: %#v, %t, %v", askID, status, result, owned, err)
			}
		}
	}
	answers := []conversation.HostAskAnswer{{QuestionID: "tone", CustomInput: "Calm"}}
	result, owned, err := service.ResolveAsk(ctx, "project-1", sess, "ask-execution-1", session.AskAnswered, answers, "")
	if !owned || err != nil || result.Status != session.AskAnswered {
		t.Fatalf("owned question was not resolved: %#v, %t, %v", result, owned, err)
	}
	_, owned, err = service.ResolveAsk(ctx, "project-1", sess, "ask-execution-1", session.AskCancelled, nil, "cancelled")
	if !owned || !errors.Is(err, ErrAskConflict) {
		t.Fatalf("conflicting answer must not fall through: %t, %v", owned, err)
	}
}

func TestExternalAskUsesCanonicalAnswersAcrossWaitersAndRestart(t *testing.T) {
	for _, engine := range []config.RuntimeID{config.RuntimeCodex, config.RuntimeClaude} {
		t.Run(string(engine), func(t *testing.T) {
			for _, test := range []struct {
				name, arguments string
				answers         []conversation.HostAskAnswer
			}{
				{"text", `{"questions":[{"id":"tone","prompt":"Which tone?"}]}`, []conversation.HostAskAnswer{{QuestionID: "tone", CustomInput: "Restrained"}}},
				{"choice", `{"questions":[{"id":"tone","prompt":"Which tone?","options":[{"value":"calm","label":"Calm","recommended":true},{"value":"bold","label":"Bold"}]}]}`, []conversation.HostAskAnswer{{QuestionID: "tone", SelectedOptionIDs: []string{"calm"}}}},
				{"multiple", `{"questions":[{"id":"tone","prompt":"Which tones?","multiple":true,"options":[{"value":"calm","label":"Calm","recommended":true},{"value":"bold","label":"Bold"}]}]}`, []conversation.HostAskAnswer{{QuestionID: "tone", SelectedOptionIDs: []string{"calm", "bold"}}}},
				{"other", `{"questions":[{"id":"tone","prompt":"Which tone?","options":[{"value":"calm","label":"Calm","recommended":true},{"value":"bold","label":"Bold"}]}]}`, []conversation.HostAskAnswer{{QuestionID: "tone", SelectedOptionIDs: []string{"other"}, CustomInput: "Reflective"}}},
			} {
				t.Run(test.name, func(t *testing.T) {
					ctx := t.Context()
					directory, store, sess := pendingAskFixtureForEngine(t, test.arguments, engine)
					interactions := &Interactions{}
					type outcome struct {
						result conversation.HostAskResolution
						err    error
					}
					waiters := make(chan outcome, 2)
					for range 2 {
						go func() {
							defer func() {
								if recovered := recover(); recovered != nil {
									waiters <- outcome{err: fmt.Errorf("waiter panic: %v", recovered)}
								}
							}()
							result, err := interactions.Wait(ctx, "project-1", sess, "operation-1", "execution-1")
							waiters <- outcome{result, err}
						}()
					}
					path := filepath.Join(directory, sess.ID+".jsonl")
					before, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					if _, err := interactions.Resolve(ctx, "project-1", sess, "ask-execution-1", []conversation.HostAskAnswer{{QuestionID: "unknown", CustomInput: "Invalid"}}, nil); err == nil {
						t.Fatal("invalid answer was accepted")
					}
					after, err := os.ReadFile(path)
					if err != nil || string(before) != string(after) {
						t.Fatal("invalid answer modified the journal")
					}
					saved, err := interactions.Resolve(ctx, "project-1", sess, "ask-execution-1", test.answers, nil)
					if err != nil {
						t.Fatal(err)
					}
					waitDeadline := time.NewTimer(5 * time.Second)
					defer waitDeadline.Stop()
					for range 2 {
						select {
						case completed := <-waiters:
							if completed.err != nil || !reflect.DeepEqual(saved, completed.result) {
								t.Fatalf("waiter did not receive committed answer: %#v", completed)
							}
						case <-waitDeadline.C:
							t.Fatal("waiters did not observe the committed answer")
						}
					}
					second, err := interactions.Resolve(ctx, "project-1", sess, "ask-execution-1", test.answers, nil)
					if err != nil || !reflect.DeepEqual(saved, second) {
						t.Fatalf("same-answer retry failed: %#v, %v", second, err)
					}
					reason := "cancelled"
					if _, err := interactions.Resolve(ctx, "project-1", sess, "ask-execution-1", nil, &reason); !errors.Is(err, ErrAskConflict) {
						t.Fatalf("late cancellation replaced a committed answer: %v", err)
					}
					if err := store.Close(); err != nil {
						t.Fatal(err)
					}
					reopened, err := session.NewStore(directory)
					if err != nil {
						t.Fatal(err)
					}
					defer reopened.Close()
					restored, err := reopened.Get(sess.ID)
					if err != nil {
						t.Fatal(err)
					}
					restarted := &Interactions{}
					waitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
					defer cancel()
					result, err := restarted.Wait(waitCtx, "project-1", restored, "operation-1", "execution-1")
					if err != nil || !reflect.DeepEqual(saved, result) {
						t.Fatalf("restarted waiter required old engine state: %#v, %v", result, err)
					}
				})
			}
		})
	}
}

func TestExternalAskCancellationAndClearRejectLateAnswers(t *testing.T) {
	_, store, sess := pendingAskFixture(t, `{"questions":[{"id":"tone","prompt":"Which tone?"}]}`)
	defer store.Close()
	ctx := context.Background()
	interactions := &Interactions{}
	reason := "user_cancelled"
	resolution, err := interactions.Resolve(ctx, "project-1", sess, "ask-execution-1", nil, &reason)
	if err != nil || resolution.Status != session.AskCancelled {
		t.Fatalf("cancellation was not persisted: %#v, %v", resolution, err)
	}
	if _, err := interactions.Resolve(ctx, "project-1", sess, "ask-execution-1", []conversation.HostAskAnswer{{QuestionID: "tone", CustomInput: "Too late"}}, nil); !errors.Is(err, ErrAskConflict) {
		t.Fatalf("answer replaced cancellation: %v", err)
	}
	if err := sess.Clear(); err != nil {
		t.Fatal(err)
	}
	if _, err := interactions.Resolve(ctx, "project-1", sess, "ask-execution-1", nil, &reason); !errors.Is(err, ErrAskNotFound) {
		t.Fatalf("clear left a live question: %v", err)
	}
}

func pendingAskFixture(t *testing.T, arguments string) (string, *session.Store, *session.Session) {
	return pendingAskFixtureForEngine(t, arguments, config.RuntimeCodex)
}

func pendingAskFixtureForEngine(t *testing.T, arguments string, engine config.RuntimeID) (string, *session.Store, *session.Session) {
	t.Helper()
	if _, err := QuestionRequest("execution-1", json.RawMessage(arguments)); err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	store, err := session.NewStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	selection := conversationconfig.Config{AgentKind: config.AgentKindGeneral, ProfileID: "default", ThinkingLevel: "medium", ApprovalMode: config.AgentApprovalAsk, Runtime: &config.RuntimeSelection{Kind: config.RuntimeCodex, Codex: &config.CodexRuntimeSettings{Model: "fixture-model"}}}
	if engine == config.RuntimeClaude {
		selection.Runtime = &config.RuntimeSelection{Kind: engine, Claude: &config.ClaudeRuntimeSettings{Model: "sonnet"}}
	}
	sess, err := store.GetOrCreateWithRuntimeConfig("questions", selection)
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := externaljournal.NewRecord(externaljournal.OperationAccepted, "operation-1", 1, externaljournal.Accepted{CommandID: "command-1", Fingerprint: "fingerprint-1", Runtime: selection.Engine(), InputMessageID: "input-1"})
	if err != nil {
		t.Fatal(err)
	}
	started, err := externaljournal.NewRecord(externaljournal.ToolStarted, "operation-1", 1, externaljournal.StartedTool{ExecutionID: "execution-1", Tool: "ask", Arguments: json.RawMessage(arguments), Recovery: externaljournal.ReadOnly})
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.UpdateExternal(context.Background(), 1, func(session.ExternalState) (session.ExternalTransaction, error) {
		return session.ExternalTransaction{Records: []externaljournal.Record{accepted, started}, Message: agent.UserMessage("Draft an opening."), Metadata: session.MessageMetadata{MessageID: "input-1"}}, nil
	}); err != nil {
		t.Fatal(err)
	}
	return directory, store, sess
}
