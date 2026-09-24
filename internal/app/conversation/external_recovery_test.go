package conversationapp

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"denova/config"
	"denova/internal/agents"
	agentchat "denova/internal/agents/chat"
	"denova/internal/agents/conversationconfig"
	agentrun "denova/internal/agents/run"
	agentruntime "denova/internal/agents/runtime"
	"denova/internal/agents/runtime/external"
	"denova/internal/agents/session"
	"denova/internal/book"
	projectdomain "denova/internal/project"
)

type recoveryAdapter func(context.Context, external.Input, external.Host) (external.Result, error)

func (recoveryAdapter) Version() string { return "fixture" }
func (adapter recoveryAdapter) Run(ctx context.Context, input external.Input, host external.Host) (external.Result, error) {
	return adapter(ctx, input, host)
}

func TestExternalRecoveryRecognizesCommittedContinuation(t *testing.T) {
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
	initial := agentchat.ChatRequest{CommandID: "initial", Message: "Inspect the result"}
	for index, command := range []string{"initial", "resumed-attempt"} {
		request := agentchat.ChatRequest{CommandID: command, Message: "Inspect the result"}
		prepared, err := prepareExternal(t.Context(), runtime, request, ProjectConversation(runtime, request), *built.external, options, nil)
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancelCause(t.Context())
		defer cancel(context.Canceled)
		prepared.InputCommandID, prepared.GuidanceCount = initial.CommandID, index
		prepared.Adapter = recoveryAdapter(func(context.Context, external.Input, external.Host) (external.Result, error) {
			if index == 0 {
				cancel(external.ErrSuspended)
				return external.Result{Text: "Partial evidence"}, ctx.Err()
			}
			return external.Result{Text: "Confirmed completion"}, nil
		})
		operation, err := engines.Operations.Start(ctx, prepared)
		if err != nil {
			t.Fatal(err)
		}
		outcome := operation.Wait(ctx)
		if index == 1 && outcome.Status != agentrun.OutcomeCompleted {
			t.Fatalf("continuation did not commit: %+v", outcome)
		}
	}
	before, err := sess.ReadCanonicalMessages(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	cycle, err := built.ExternalFactory(initial, ProjectConversation(runtime, initial), options)(t.Context(), agentruntime.ExternalCycleInput{
		Request: initial, Resume: true,
		Guidance: []agentchat.ChatRequest{{CommandID: "first", Message: "Check it"}, {CommandID: "late", Message: "Check one more item"}},
	}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if outcome := cycle.Wait(t.Context()); outcome.Status != agentrun.OutcomeCompleted {
		t.Fatalf("canonical continuation was executed again: %+v", outcome)
	}
	if receipt, ok := cycle.(interface{ ConsumedGuidance() int }); !ok || receipt.ConsumedGuidance() != 1 {
		t.Fatal("late guidance was incorrectly acknowledged by the completed attempt")
	}
	after, err := sess.ReadCanonicalMessages(t.Context())
	if err != nil || len(after) != len(before) {
		t.Fatalf("recovery wrote another input or output: %d -> %d, %v", len(before), len(after), err)
	}
}
