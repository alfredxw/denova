package conversationapp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"denova/config"
	"denova/internal/agents"
	agentchat "denova/internal/agents/chat"
	"denova/internal/agents/conversationconfig"
	agentrun "denova/internal/agents/run"
	agentruntime "denova/internal/agents/runtime"
	"denova/internal/agents/session"
	"denova/internal/book"
	projectdomain "denova/internal/project"
)

const standingCreatorSentinel = "NARRATE_IN_SECOND_PERSON_SENTINEL"

// TestExternalStandingInstructionsUseSystemChannel pins that authored project
// rules (AGENTS.md / CREATOR.md) travel in the engine's Instructions — the system
// channel — for runtimes that carry them outside argv, while Grok keeps them in
// the turn text because its system prompt rides the command line.
func TestExternalStandingInstructionsUseSystemChannel(t *testing.T) {
	for _, engine := range []config.RuntimeID{config.RuntimeCodex, config.RuntimeClaude, config.RuntimeAntigravity, config.RuntimeGrok} {
		t.Run(string(engine), func(t *testing.T) {
			root := t.TempDir()
			workspace := filepath.Join(root, "project")
			if err := os.MkdirAll(workspace, 0700); err != nil {
				t.Fatal(err)
			}
			selection := config.RuntimeSelection{Kind: engine}
			switch engine {
			case config.RuntimeCodex:
				selection.Codex = &config.CodexRuntimeSettings{Model: "fixture"}
			case config.RuntimeClaude:
				selection.Claude = &config.ClaudeRuntimeSettings{Model: "fixture"}
			case config.RuntimeAntigravity:
				selection.Antigravity = &config.AntigravityRuntimeSettings{Model: "fixture"}
			case config.RuntimeGrok:
				selection.Grok = &config.GrokRuntimeSettings{Model: "fixture", ContextWindow: 500000}
			}
			cfg := config.Config{Workspace: workspace, ProjectID: "project", ProjectStoreDir: filepath.Join(root, "store"), DenovaDir: root, ActiveAgentRuntime: &selection}
			store, err := session.NewStore(filepath.Join(cfg.ProjectStoreDir, "sessions"))
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			sess, err := store.GetOrCreateWithRuntimeConfig("standing", conversationconfig.Config{AgentKind: config.AgentKindIDE, Runtime: &selection})
			if err != nil {
				t.Fatal(err)
			}
			runtime := Runtime{ProjectID: cfg.ProjectID, ProjectStore: cfg.ProjectStoreDir, ProjectType: projectdomain.TypeBook, AgentKind: config.AgentKindIDE, Session: sess, Config: cfg, Workspace: workspace, State: book.NewState(workspace), BookService: book.NewService(workspace)}
			if err := runtime.State.InitWorkspace(); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(workspace, book.CreatorFileName), []byte(standingCreatorSentinel), 0600); err != nil {
				t.Fatal(err)
			}
			engines := agentruntime.NewEngines()
			defer engines.Close()
			built, err := BuildExecution(t.Context(), runtime, agents.AgentHostCapabilities{}, engines, "")
			if err != nil {
				t.Fatal(err)
			}
			request := agentchat.ChatRequest{CommandID: "turn", Message: "Continue the scene."}
			prepared, err := prepareExternal(t.Context(), runtime, request, ProjectConversation(runtime, request), *built.external, agentrun.Options{ProjectID: cfg.ProjectID, SessionID: sess.ID, AgentKind: config.AgentKindIDE, Mode: "agent_chat"}, nil)
			if err != nil {
				t.Fatal(err)
			}
			inInstructions := strings.Contains(prepared.Input.Instructions, standingCreatorSentinel)
			inText := strings.Contains(prepared.Input.Text, standingCreatorSentinel)
			wantSystem := engine != config.RuntimeGrok
			if wantSystem && (!inInstructions || inText) {
				t.Fatalf("%s must carry standing rules in Instructions only: instructions=%v text=%v", engine, inInstructions, inText)
			}
			if !wantSystem && (inInstructions || !inText) {
				t.Fatalf("%s must keep standing rules in the turn text: instructions=%v text=%v", engine, inInstructions, inText)
			}
		})
	}
}
