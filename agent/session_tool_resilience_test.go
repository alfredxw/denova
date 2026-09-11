package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	agentsession "github.com/alfredxw/denova/agent/session"
	sessionfile "github.com/alfredxw/denova/agent/session/file"
)

type lostToolResultStore struct{ agentsession.Store }
type lostToolResultLog struct{ agentsession.Log }

func (store lostToolResultStore) Open(ctx context.Context, key agentsession.Key) (agentsession.Log, error) {
	log, err := store.Store.Open(ctx, key)
	return lostToolResultLog{log}, err
}

func (log lostToolResultLog) Append(ctx context.Context, expected agentsession.Revision, records ...agentsession.Record) (agentsession.Revision, error) {
	for _, record := range records {
		if record.Kind == turnToolRecord {
			var fact persistedTool
			if err := json.Unmarshal(record.Data, &fact); err != nil {
				return expected, err
			}
			if fact.Result != nil {
				return expected, agentsession.ErrCommitUnknown
			}
		}
	}
	return log.Log.Append(ctx, expected, records...)
}

func TestUnconfirmedWriteRequiresVerificationBeforeAnyNewModelCall(t *testing.T) {
	store := agentsession.Memory()
	var executions atomic.Int32
	tool := testToolDefinition(&functionTool{name: "write", run: func(context.Context, string) (string, error) {
		executions.Add(1)
		return "effect applied", nil
	}})
	tool.Descriptor.Execution, tool.Descriptor.Source = ToolExecutionSessionExclusive, ToolSourceWrite
	tool.Descriptor.MutationScope, tool.Descriptor.PostCheck = ToolMutationExternal, ToolPostCheckExternalReceipt
	tool.Descriptor.Recovery = ToolRecoveryNonIdempotent
	tools, err := StaticTools(tool)
	if err != nil {
		t.Fatal(err)
	}
	policy := &permissionResolutionInvariantPolicy{decision: PermissionResolvedDecision{Allowed: true}}
	definition := Definition{Name: "test", Permission: policy, Tools: tools, Model: &lifecycleModel{responses: []*Message{
		AssistantMessage("", []ToolCall{{ID: "provider-reused", Type: "function", Function: FunctionCall{Name: "write", Arguments: `{"target":"remote"}`}}}),
	}}}
	owner, err := New(context.Background(), definition, WithSessionStore(lostToolResultStore{store}))
	if err != nil {
		t.Fatal(err)
	}
	session, err := owner.Session(context.Background(), NamedSession("unknown-write"))
	if err != nil {
		t.Fatal(err)
	}
	run, err := session.Run(context.Background(), Text("apply operation"))
	if err != nil {
		t.Fatal(err)
	}
	permissionID := waitPermissionInteractionID(t, run)
	if err := run.Respond(context.Background(), permissionID, InteractionResponse{Permission: PermissionAllowOnce}); err != nil {
		t.Fatal(err)
	}
	if result, err := run.Wait(context.Background()); !errors.Is(err, agentsession.ErrCommitUnknown) || result.Status != ResultSuspended {
		t.Fatalf("uncertain result=%#v error=%v", result, err)
	}
	_ = owner.Close(context.Background())
	model := &lifecycleModel{responses: []*Message{AssistantMessage("verified", nil), AssistantMessage("steered", nil)}}
	definition.Model = model
	owner, err = New(context.Background(), definition, WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	session, err = owner.Session(context.Background(), NamedSession("unknown-write"))
	if err != nil {
		t.Fatal(err)
	}
	attached, _, err := session.AttachRun(context.Background(), run.ID())
	if err != nil {
		t.Fatal(err)
	}
	questions := attached.pendingInteractionRequests()
	if len(questions) != 1 || questions[0].Verification == nil || questions[0].Verification.Tool != "write" {
		t.Fatalf("verification missing: %#v", questions)
	}
	resumed, err := session.ResumeRun(context.Background(), ResumeRequest{RunID: run.ID(), IdempotencyKey: "resume"})
	if err != nil {
		t.Fatal(err)
	}
	// Observing the replayed question proves that the recovery gate is active.
	for event := range resumed.Events() {
		if _, ok := event.Payload.(InteractionRequested); ok {
			break
		}
	}
	if _, err := resumed.Steer(context.Background(), Text("continue now")); err != nil {
		t.Fatal(err)
	}
	unknown := InteractionResponse{Answers: []InteractionAnswer{{QuestionID: "effect", Values: []string{"unknown"}}}}
	if err := resumed.Respond(context.Background(), questions[0].ID, unknown); err != nil {
		t.Fatal(err)
	}
	if len(model.calls()) != 0 || executions.Load() != 1 || len(resumed.pendingInteractionRequests()) != 1 {
		t.Fatal("unknown effect gate was bypassed")
	}
	verified := InteractionResponse{Answers: []InteractionAnswer{{QuestionID: "effect", Values: []string{"executed"}}}}
	if err := resumed.Respond(context.Background(), questions[0].ID, verified); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if result, err := resumed.Wait(ctx); err != nil || result.Status != ResultCompleted {
		t.Fatalf("result=%#v error=%v", result, err)
	}
	if executions.Load() != 1 {
		t.Fatalf("external effect repeated %d times", executions.Load())
	}
	calls := model.calls()
	if len(calls) != 2 || !strings.Contains(calls[0][2].Content, "Original result details are unavailable") {
		t.Fatalf("verification projection=%#v", calls)
	}
}

func TestSuspendWaitsForToolAndReleasesFileLease(t *testing.T) {
	store, err := sessionfile.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	started, stopping, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var executions atomic.Int32
	tool := testToolDefinition(&functionTool{name: "write", run: func(ctx context.Context, _ string) (string, error) {
		executions.Add(1)
		close(started)
		<-ToolSteeringDone(ctx)
		close(stopping)
		<-release
		return "confirmed effect", nil
	}})
	toolset, err := StaticTools(tool)
	if err != nil {
		t.Fatal(err)
	}
	definition := Definition{Name: "test", Model: &lifecycleModel{responses: []*Message{AssistantMessage("", []ToolCall{{ID: "call", Type: "function", Function: FunctionCall{Name: "write", Arguments: `{}`}}})}}, Tools: toolset}
	owner, err := New(context.Background(), definition, WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	session, err := owner.Session(context.Background(), NamedSession("file-pause"))
	if err != nil {
		t.Fatal(err)
	}
	run, err := session.Run(context.Background(), Text("perform work"))
	if err != nil {
		t.Fatal(err)
	}
	<-started
	paused := make(chan error, 1)
	safeGo(func() {
		_, err := session.SuspendAndClose(context.Background(), SuspendRequest{RunID: run.ID(), IdempotencyKey: "pause"})
		paused <- err
	}, func(err error) { paused <- err })
	<-stopping
	select {
	case err := <-paused:
		t.Fatalf("pause completed while tool could still write: %v", err)
	default:
	}
	leaseCtx, cancelLease := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancelLease()
	if _, err := store.Open(leaseCtx, session.Key()); err == nil {
		t.Fatal("writer lease released before tool stopped")
	}
	close(release)
	if err := <-paused; err != nil {
		t.Fatal(err)
	}
	if err := owner.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	model := &lifecycleModel{responses: []*Message{AssistantMessage("done", nil)}}
	definition.Model = model
	owner, err = New(context.Background(), definition, WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	session, err = owner.Session(context.Background(), NamedSession("file-pause"))
	if err != nil {
		t.Fatal(err)
	}
	resumed, err := session.ResumeRun(context.Background(), ResumeRequest{RunID: run.ID(), IdempotencyKey: "resume"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resumed.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if executions.Load() != 1 {
		t.Fatalf("tool executions=%d", executions.Load())
	}
	if calls := model.calls(); len(calls) != 1 || len(calls[0]) != 3 || calls[0][2].Content != "confirmed effect" {
		t.Fatalf("resumed model did not receive the confirmed tool result: %#v", calls)
	}
}

func TestAskAnswerSurvivesSuspensionAndReopen(t *testing.T) {
	store := newObservingSessionStore()
	var executions atomic.Int32
	tool := testToolDefinition(&functionTool{name: "ask", run: func(ctx context.Context, _ string) (string, error) {
		executions.Add(1)
		answer, err := RequestInteraction(ctx, InteractionRequest{ID: "ask-" + CurrentToolExecutionID(ctx), Kind: InteractionAsk,
			Questions: []InteractionQuestion{{ID: "name", Prompt: "Name?", AllowFreeText: true}}})
		if err != nil {
			return "", err
		}
		encoded, err := json.Marshal(answer)
		return string(encoded), err
	}})
	tool.Descriptor.Capability, tool.Descriptor.Execution, tool.Descriptor.Steering = "ask", ToolExecutionInteractiveWait, SteeringInterruptibleWait
	toolset, err := StaticTools(tool)
	if err != nil {
		t.Fatal(err)
	}
	definition := Definition{Name: "test", Model: &lifecycleModel{responses: []*Message{AssistantMessage("", []ToolCall{{ID: "call", Type: "function", Function: FunctionCall{Name: "ask", Arguments: `{}`}}})}}, Tools: toolset}
	owner, err := New(context.Background(), definition, WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	session, err := owner.Session(context.Background(), NamedSession("ask-pause"))
	if err != nil {
		t.Fatal(err)
	}
	run, err := session.Run(context.Background(), Text("choose"))
	if err != nil {
		t.Fatal(err)
	}
	var question InteractionRequest
	for event := range run.Events() {
		if request, ok := event.Payload.(InteractionRequested); ok {
			question = request.Request
			break
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := session.SuspendAndClose(ctx, SuspendRequest{RunID: run.ID(), IdempotencyKey: "pause"}); err != nil {
		t.Fatal(err)
	}
	_ = owner.Close(context.Background())
	model := &lifecycleModel{responses: []*Message{AssistantMessage("done", nil)}}
	definition.Model = model
	owner, err = New(context.Background(), definition, WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	session, err = owner.Session(context.Background(), NamedSession("ask-pause"))
	if err != nil {
		t.Fatal(err)
	}
	attached, _, err := session.AttachRun(ctx, run.ID())
	if err != nil {
		t.Fatal(err)
	}
	answer := InteractionResponse{Answers: []InteractionAnswer{{QuestionID: "name", Text: "Ada"}}}
	if err := attached.Respond(ctx, question.ID, answer); err != nil {
		t.Fatal(err)
	}
	if err := attached.Respond(ctx, question.ID, answer); err != nil {
		t.Fatal(err)
	}
	if len(model.calls()) != 0 {
		t.Fatal("answer resumed execution")
	}
	if _, err := session.SuspendAndClose(ctx, SuspendRequest{RunID: run.ID(), IdempotencyKey: "close-answered"}); err != nil {
		t.Fatal(err)
	}
	_ = owner.Close(context.Background())
	owner, err = New(context.Background(), definition, WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	session, err = owner.Session(ctx, NamedSession("ask-pause"))
	if err != nil {
		t.Fatal(err)
	}
	resumed, err := session.ResumeRun(ctx, ResumeRequest{RunID: run.ID(), IdempotencyKey: "resume"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resumed.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	if executions.Load() != 1 {
		t.Fatalf("Ask executed again: %d", executions.Load())
	}
	calls := model.calls()
	if len(calls) != 1 || len(calls[0]) != 3 || !strings.Contains(calls[0][2].Content, "Ada") {
		t.Fatalf("answer missing from model context: %#v", calls)
	}
	if err := resumed.Respond(ctx, question.ID, InteractionResponse{Cancelled: true}); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("answer rewrite error=%v", err)
	}
	request, resolution, err := session.Respond(ctx, question.ID, answer)
	if err != nil || request.ID != question.ID || len(resolution.Answers) != 1 || resolution.Answers[0].Text != "Ada" {
		t.Fatalf("repeated Session answer after reopen and settlement: request=%+v resolution=%+v error=%v", request, resolution, err)
	}
}
