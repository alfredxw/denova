package conversationapp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"denova/config"
	"denova/internal/agents"
	agentchat "denova/internal/agents/chat"
	"denova/internal/agents/conversationconfig"
	agentrun "denova/internal/agents/run"
	agentruntime "denova/internal/agents/runtime"
	externaljournal "denova/internal/agents/runtime/external/journal"
	"denova/internal/agents/runtime/external"
	"denova/internal/agents/session"
	"denova/internal/book"
	projectdomain "denova/internal/project"
)

// TestExternalRetryRestartsProviderThroughRecoveryEntry reproduces the retry
// controller sequence on the real external journal. The first attempt persists
// its failure receipt; restarting the same CommandID would only replay that
// receipt without invoking the provider again, while the recovery entry starts
// a fresh operation, keeps the committed user input a single canonical message
// and really runs the provider a second time.
func TestExternalRetryRestartsProviderThroughRecoveryEntry(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "project")
	if err := os.MkdirAll(workspace, 0700); err != nil {
		t.Fatal(err)
	}
	selection := config.RuntimeSelection{Kind: config.RuntimeCodex, Codex: &config.CodexRuntimeSettings{Model: "fixture"}}
	cfg := config.Config{Workspace: workspace, ProjectID: "project", ProjectStoreDir: filepath.Join(root, "store"), DenovaDir: root, ActiveAgentRuntime: &selection}
	store, err := session.NewStore(filepath.Join(cfg.ProjectStoreDir, "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	sess, err := store.GetOrCreateWithRuntimeConfig("session", conversationconfig.Config{AgentKind: config.AgentKindGeneral, Runtime: &selection})
	if err != nil {
		t.Fatal(err)
	}
	runtime := Runtime{ProjectID: cfg.ProjectID, ProjectStore: cfg.ProjectStoreDir, ProjectType: projectdomain.TypeGeneral, AgentKind: config.AgentKindGeneral, Session: sess, Config: cfg, Workspace: workspace, BookService: book.NewService(workspace)}
	engines := agentruntime.NewEngines()
	defer engines.Close()
	built, err := BuildExecution(t.Context(), runtime, agents.AgentHostCapabilities{}, engines, "")
	if err != nil {
		t.Fatal(err)
	}
	options := agentrun.Options{ProjectID: cfg.ProjectID, SessionID: sess.ID, AgentKind: config.AgentKindGeneral, Mode: "agent_chat"}
	initial := agentchat.ChatRequest{CommandID: "retry-input", Message: "Inspect the result"}

	var providerCalls atomic.Int32
	adapter := recoveryAdapter(func(context.Context, external.Input, external.Host) (external.Result, error) {
		if providerCalls.Add(1) == 1 {
			return external.Result{Text: "Partial evidence"}, external.RetryableError(errors.New("stream EOF"))
		}
		return external.Result{Text: "Confirmed completion"}, nil
	})

	// First attempt: a fresh start whose provider fails with a retryable error.
	// The failure receipt is persisted for this CommandID.
	prepared, err := prepareExternal(t.Context(), runtime, initial, ProjectConversation(runtime, initial), *built.external, options, nil)
	if err != nil {
		t.Fatal(err)
	}
	prepared.InputCommandID, prepared.GuidanceCount = initial.CommandID, 0
	prepared.Adapter = adapter
	first, err := engines.Operations.Start(t.Context(), prepared)
	if err != nil {
		t.Fatal(err)
	}
	if outcome := first.Wait(t.Context()); outcome.Status != agentrun.OutcomeFailed {
		t.Fatalf("first attempt outcome=%+v", outcome)
	}

	// Control: restarting the same CommandID hits the persisted receipt. The
	// provider must not be invoked again on this path.
	replayed, err := engines.Operations.Start(t.Context(), prepared)
	if err != nil {
		t.Fatal(err)
	}
	if !replayed.Replayed() {
		t.Fatal("duplicate start of the failed CommandID was not replayed")
	}
	if outcome := replayed.Wait(t.Context()); outcome.Status != agentrun.OutcomeFailed {
		t.Fatalf("replayed attempt outcome=%+v", outcome)
	}
	if providerCalls.Load() != 1 {
		t.Fatalf("replayed attempt restarted the provider calls=%d", providerCalls.Load())
	}

	// Recovery entry: a fresh CommandID for the same input restarts the provider
	// while the original user input stays a single canonical message.
	resume := agentchat.ChatRequest{CommandID: "resume-retry", Message: "Continue the accepted task from the confirmed conversation and tool outcomes. Do not repeat effects that already succeeded.", InputVisibility: agentrun.InputModelOnly}
	prepared, err = prepareExternal(t.Context(), runtime, resume, ProjectConversation(runtime, resume), *built.external, options, nil)
	if err != nil {
		t.Fatal(err)
	}
	prepared.InputCommandID, prepared.GuidanceCount = initial.CommandID, 0
	prepared.Adapter = adapter
	second, err := engines.Operations.Start(t.Context(), prepared)
	if err != nil {
		t.Fatal(err)
	}
	if second.Replayed() || second.Receipt().OperationID == first.Receipt().OperationID {
		t.Fatalf("recovery entry replayed the persisted failure instead of restarting the provider: %+v vs %+v", second.Receipt(), first.Receipt())
	}
	if outcome := second.Wait(t.Context()); outcome.Status != agentrun.OutcomeCompleted {
		t.Fatalf("recovery attempt outcome=%+v", outcome)
	}
	if providerCalls.Load() != 2 {
		t.Fatalf("recovery provider calls=%d, want 2", providerCalls.Load())
	}
	messages, err := sess.ReadCanonicalMessages(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	userInputs := 0
	for _, message := range messages {
		if message.Role == "user" && message.Content == initial.Message {
			userInputs++
		}
	}
	if userInputs != 1 {
		t.Fatalf("original user input committed %d times, want 1", userInputs)
	}
	// Journal projection after the retry: the failed original operation and the
	// recovery operation coexist, each closed once, linked by the original input.
	failed, completed := 0, 0
	err = sess.ReadExternal(t.Context(), func(state session.ExternalState) error {
		for _, operation := range state.Projection.Operations {
			if operation.CommandID != initial.CommandID && operation.InputCommandID != initial.CommandID {
				continue
			}
			switch operation.Status {
			case externaljournal.Failed:
				failed++
			case externaljournal.Completed:
				completed++
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if failed != 1 || completed != 1 {
		t.Fatalf("retry journal projection failed=%d completed=%d, want 1 and 1", failed, completed)
	}
}