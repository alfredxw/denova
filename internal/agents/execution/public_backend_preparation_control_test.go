package execution

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	agentchat "denova/internal/agents/chat"
	agentconversation "denova/internal/agents/conversation"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	agenttoolruntime "denova/internal/agents/toolruntime"

	agent "github.com/alfredxw/denova/agent"
)

type profileWithoutCyclePreparation struct{}

func (profileWithoutCyclePreparation) ID() ProfileID { return ProfileWriting }

type profileWithoutCanonicalInput struct{}

func (profileWithoutCanonicalInput) ID() ProfileID { return ProfileWriting }
func (profileWithoutCanonicalInput) PrepareCycle(context.Context, CycleRestoreRequest) (Cycle, error) {
	return Cycle{}, nil
}

func TestWithProfilesRejectsNilAndProfilesWithoutCyclePreparation(t *testing.T) {
	tests := []struct {
		name    string
		profile Profile
	}{
		{name: "nil", profile: nil},
		{name: "missing queued-cycle preparation", profile: profileWithoutCyclePreparation{}},
		{name: "missing canonical input", profile: profileWithoutCanonicalInput{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewAgentRuntime(context.Background(), t.TempDir(),
				WithProfiles(test.profile),
				WithToolMutationApplier(func(context.Context, agenttoolruntime.CommittedToolMutation) error { return nil }),
			)
			if !errors.Is(err, ErrProfileInvalid) {
				t.Fatalf("NewAgentRuntime error=%v, want ErrProfileInvalid", err)
			}
		})
	}
}

func TestRequestSemanticFingerprintUsesOnlyFrozenCallerPayload(t *testing.T) {
	request := agentchat.CaptureChatRequestCallerInput(agentchat.ChatRequest{
		CommandID: "semantic-command-1", Message: "draw a lighthouse", ImagePresetID: "preset-1",
		References: []string{"chapter.md"}, Locale: "en-US",
	})
	first := RequestSemanticFingerprint(request)

	// Server-resolved values and a transport retry's command identity do not
	// change the logical payload that was frozen before product preparation.
	request.CommandID = "semantic-command-2"
	request.ImagePreset = agentchat.ImagePresetContext{
		ID: "preset-1", Name: "resolved differently", AgentSystemPrompt: "mutable server context",
	}
	request.StyleRules = nil
	if got := RequestSemanticFingerprint(request); got != first {
		t.Fatalf("resolved server context changed semantic fingerprint: got=%q want=%q", got, first)
	}

	changed := agentchat.CaptureChatRequestCallerInput(agentchat.ChatRequest{
		CommandID: "semantic-command-3", Message: "draw a lighthouse", ImagePresetID: "preset-2",
		References: []string{"chapter.md"}, Locale: "en-US",
	})
	if got := RequestSemanticFingerprint(changed); got == first {
		t.Fatalf("changed caller payload retained fingerprint %q", got)
	}
}

func TestForegroundWorkspaceClosePreservesAgentChatPublicSession(t *testing.T) {
	ctx := context.Background()
	runtime := NewEphemeralRuntime()
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	workspace := t.TempDir()
	writingKey, err := agentrun.AgentSessionKeyForOptions(agentrun.Options{
		AgentKind: agentrun.AgentKindIDE, Workspace: workspace, SessionID: "writing-session",
	})
	if err != nil {
		t.Fatal(err)
	}
	agentChatKey, err := agentrun.AgentSessionKeyForOptions(agentrun.Options{
		AgentKind: agentrun.AgentKindIDE, Mode: agentrun.ModeAgentChat,
		Workspace: workspace, SessionID: "agent-chat-session",
	})
	if err != nil {
		t.Fatal(err)
	}
	writingSession, err := runtime.public.agent.Session(ctx, writingKey)
	if err != nil {
		t.Fatal(err)
	}
	agentChatSession, err := runtime.public.agent.Session(ctx, agentChatKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.CloseForegroundWorkspaceBindings(ctx, workspace); err != nil {
		t.Fatal(err)
	}
	if _, err := writingSession.Snapshot(ctx); !errors.Is(err, agent.ErrSessionClosed) {
		t.Fatalf("writing Snapshot error=%v, want closed Session", err)
	}
	if _, err := agentChatSession.Snapshot(ctx); err != nil {
		t.Fatalf("AgentChat session was closed with foreground workspace: %v", err)
	}
}

func TestAgentRuntimeRejectsInvalidCommandBeforeOpeningSession(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	runtime, err := NewAgentRuntime(ctx, dataDir, WithToolMutationApplier(
		func(context.Context, agenttoolruntime.CommittedToolMutation) error { return nil },
	))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	oversized := strings.Repeat("command", 1<<16)
	options := agentrun.Options{
		AgentKind: agentrun.AgentKindIDE, Workspace: t.TempDir(), SessionID: "invalid-command-session",
	}
	if _, err := runtime.Start(ctx, StartRequest{Cycle: Cycle{
		Request: agentchatRequest(oversized, "must not open"), Options: options,
	}}); !errors.Is(err, agentrun.ErrInvalidCommand) {
		t.Fatalf("oversized Start error=%v, want ErrInvalidCommand", err)
	}
	if _, err := runtime.SubmitCommand(ctx, CommandRequest{
		Kind: CommandFollowUp, CommandID: oversized, OperationID: "missing-operation",
		Request: agentchatRequest(oversized, "must not open"), Options: options,
	}); !errors.Is(err, agentrun.ErrInvalidCommand) {
		t.Fatalf("oversized command error=%v, want ErrInvalidCommand", err)
	}
	runtime.public.mu.RLock()
	registrations, runs := len(runtime.public.registrations), len(runtime.public.runs)
	runtime.public.mu.RUnlock()
	if registrations != 0 || runs != 0 {
		t.Fatalf("invalid command created registrations=%d runs=%d", registrations, runs)
	}
	files := 0
	err = filepath.WalkDir(filepath.Join(dataDir, "agent-transcripts"), func(_ string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			files++
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if files != 0 {
		t.Fatalf("invalid command opened a durable Session with %d file(s)", files)
	}
}

func TestAgentRuntimeAbortCancelsBlockedSteerPreparationAndRetainsAcceptedInput(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	conversationSession, err := store.GetOrCreate("public-runtime-preparation-control")
	if err != nil {
		t.Fatal(err)
	}
	initialModel := &publicBackendSteerModel{started: make(chan struct{}), release: make(chan struct{})}
	initialDefinition := agent.Definition{
		Key: "public-backend-preparation-control", Name: "root", Model: initialModel,
		ModelIdentity: agent.CapabilityIdentity{Kind: "model.public-backend-preparation-control", Version: 1},
	}
	options := agentrun.Options{
		AgentKind: agentrun.AgentKindIDE, SessionID: conversationSession.ID, Workspace: workspace,
		TaskID: "preparation-control-task", RootAgentName: "root",
	}
	preparationStarted := make(chan CycleRestoreRequest, 1)
	preparationCanceled := make(chan struct{})
	materializedInputs := 0
	var materializedMu sync.Mutex
	profile := publicBackendTestProfile{
		prepare: func(ctx context.Context, request CycleRestoreRequest) (Cycle, error) {
			preparationStarted <- request
			<-ctx.Done()
			close(preparationCanceled)
			return Cycle{}, ctx.Err()
		},
		canonical: func(_ context.Context, request CanonicalInputRequest) (agent.CanonicalAdapter, error) {
			return agent.CanonicalAdapterFuncs{
				CapabilityIdentity: request.Identity,
				MaterializeInputFn: func(ctx context.Context, input agent.InputCommitRequest) (agent.CommitReceipt, error) {
					materializedMu.Lock()
					materializedInputs++
					materializedMu.Unlock()
					intent, err := session.NewDomainCommitIntent(session.DomainCommitIdentity{
						CommandID: input.Identity.CommandID, OperationID: input.Identity.RunID, Cycle: input.Identity.Cycle,
					}, agent.UserMessage(request.Input.Text), session.MessageMetadata{AgentKind: agentrun.AgentKindIDE})
					if err != nil {
						return agent.CommitReceipt{}, err
					}
					intent, err = intent.WithAgentCanonicalHash(input.Hash)
					if err != nil {
						return agent.CommitReceipt{}, err
					}
					receipt, err := conversationSession.CommitDomainMessageContext(ctx, intent)
					if err != nil {
						return agent.CommitReceipt{}, err
					}
					return agent.CommitReceipt{Revision: fmt.Sprint(receipt.ContextRevision)}, nil
				},
			}, nil
		},
	}
	runtime, err := NewAgentRuntime(ctx, t.TempDir(), WithProfiles(profile), WithToolMutationApplier(
		func(context.Context, agenttoolruntime.CommittedToolMutation) error { return nil },
	))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	operation, err := runtime.Start(ctx, StartRequest{Cycle: Cycle{
		Definition: initialDefinition,
		Conversation: agentconversation.NewSessionConversationForAgent(
			conversationSession, nil, agentrun.AgentKindIDE,
		),
		Request: agentchatRequest("preparation-start", "initial request"), Options: options,
	}})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-initialModel.started:
	case <-time.After(2 * time.Second):
		t.Fatal("initial model call did not start")
	}
	if _, err := runtime.SubmitCommand(ctx, CommandRequest{
		Kind: CommandSteer, CommandID: "preparation-steer", OperationID: operation.Receipt().OperationID,
		Request: agentchatRequest("preparation-steer", "durable new direction"), Options: options,
	}); err != nil {
		t.Fatal(err)
	}
	// Steer uses the safe after-model boundary. Let the current provider call
	// return so Runtime can select the already-durable successor and enter its
	// product preparation stage.
	close(initialModel.release)
	select {
	case request := <-preparationStarted:
		if request.Kind != CommandSteer || request.CommandID != "preparation-steer" ||
			request.Request.Message != "durable new direction" {
			t.Fatalf("blocked preparation request=%#v", request)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("steer cycle preparation did not start")
	}
	materializedMu.Lock()
	inputWrites := materializedInputs
	materializedMu.Unlock()
	if inputWrites != 1 {
		t.Fatalf("canonical steer input writes=%d, want exactly one before preparation", inputWrites)
	}
	if _, err := runtime.SubmitCommand(ctx, CommandRequest{
		Kind: CommandAbort, CommandID: "preparation-abort", OperationID: operation.Receipt().OperationID,
		Reason: "cancel blocked preparation", Options: options,
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-preparationCanceled:
	case <-time.After(2 * time.Second):
		t.Fatal("Abort did not cancel Profile.PrepareCycle")
	}
	waitCtx, cancelWait := context.WithTimeout(ctx, 2*time.Second)
	defer cancelWait()
	if outcome := operation.Wait(waitCtx); outcome.Status != agentrun.OutcomeAborted {
		t.Fatalf("blocked-preparation outcome=%#v", outcome)
	}
	initialModel.mu.Lock()
	initialCalls := initialModel.calls
	initialModel.mu.Unlock()
	if initialCalls != 1 {
		t.Fatalf("model calls before Abort settlement=%d, want 1", initialCalls)
	}

	// A later independent Run must see the accepted steer input checkpointed by
	// the public Agent even though Abort interrupted product preparation before
	// canonical domain reconciliation or a second model effect could begin.
	reopenModel := &publicBackendTestModel{responses: []*agent.Message{agent.AssistantMessage("reopened", nil)}}
	reopened, err := runtime.Start(ctx, StartRequest{Cycle: Cycle{
		Definition: agent.Definition{
			Key: "public-backend-after-preparation-abort", Name: "root", Model: reopenModel,
			ModelIdentity: agent.CapabilityIdentity{Kind: "model.public-backend-after-preparation-abort", Version: 1},
		},
		Conversation: agentconversation.NewSessionConversationForAgent(
			conversationSession, nil, agentrun.AgentKindIDE,
		),
		Request: agentchatRequest("preparation-reopen", "continue after abort"), Options: options,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if outcome := reopened.Wait(ctx); outcome.Status != agentrun.OutcomeCompleted {
		t.Fatalf("reopened outcome=%#v", outcome)
	}
	reopenModel.mu.Lock()
	reopenedInputs := clonePublicBackendMessages(reopenModel.inputs[0])
	reopenModel.mu.Unlock()
	var visible strings.Builder
	for _, message := range reopenedInputs {
		if message != nil {
			visible.WriteString(message.Content)
			visible.WriteByte('\n')
		}
	}
	if !strings.Contains(visible.String(), "durable new direction") ||
		!strings.Contains(visible.String(), "continue after abort") {
		t.Fatalf("accepted input was lost across preparation Abort: %#v", reopenedInputs)
	}
}
