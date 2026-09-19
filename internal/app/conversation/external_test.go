package conversationapp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"denova/config"
	agents "denova/internal/agents"
	agentchat "denova/internal/agents/chat"
	"denova/internal/agents/conversationconfig"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	appagentruntime "denova/internal/app/agentruntime"
	"denova/internal/book"
	projectdomain "denova/internal/project"
)

func TestExternalPreparationUsesProductContextWithoutNativeModel(t *testing.T) {
	for _, kind := range []string{config.AgentKindIDE, config.AgentKindGeneral} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			workspace := filepath.Join(root, "project")
			if err := os.MkdirAll(workspace, 0o755); err != nil {
				t.Fatal(err)
			}
			selection := config.RuntimeSelection{Kind: config.RuntimeCodex, Codex: &config.CodexRuntimeSettings{Model: "external-model"}}
			cfg := config.Config{Workspace: workspace, ProjectID: "project-1", ProjectStoreDir: filepath.Join(root, "store"), DenovaDir: root, ActiveAgentRuntime: &selection}
			store, err := session.NewStore(filepath.Join(cfg.ProjectStoreDir, "sessions"))
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			sess, err := store.GetOrCreateWithRuntimeConfig("session-1", conversationconfig.Config{AgentKind: kind, ProfileID: "deleted-native-model", ThinkingLevel: "medium", ApprovalMode: config.AgentApprovalMode("write"), Runtime: &selection})
			if err != nil {
				t.Fatal(err)
			}
			runtime := Runtime{ProjectID: cfg.ProjectID, ProjectStore: cfg.ProjectStoreDir, ProjectType: projectdomain.TypeGeneral, AgentKind: kind, Session: sess, Config: cfg, Workspace: workspace, BookService: book.NewService(workspace)}
			if kind == config.AgentKindIDE {
				runtime.ProjectType, runtime.State = projectdomain.TypeBook, book.NewState(workspace)
				if err := runtime.State.InitWorkspace(); err != nil {
					t.Fatal(err)
				}
			}
			engines := appagentruntime.NewEngines()
			defer engines.Close()
			built, err := BuildExecution(context.Background(), runtime, agents.AgentHostCapabilities{}, engines, "")
			if err != nil {
				t.Fatal(err)
			}
			request := agentchat.ChatRequest{CommandID: "request-1", Message: "Write a scene with a quiet ending."}
			prepared, err := prepareExternal(t.Context(), runtime, request, ProjectConversation(runtime, request), *built.external, agentrun.Options{}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if prepared.Input.Selection.Codex.Model != "external-model" || !strings.Contains(prepared.Input.Text, request.Message) || prepared.Message.Content != request.Message || prepared.Revision != 1 {
				t.Fatalf("incorrect external input: %#v", prepared.Input)
			}
			foundAsk := false
			for _, definition := range prepared.Definitions {
				info, err := definition.Tool.Info(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				if info.Name == "ask" {
					foundAsk = true
				}
			}
			if !foundAsk {
				t.Fatal("external product assembly omitted Ask")
			}
			if _, err := BuildExecution(t.Context(), runtime, agents.AgentHostCapabilities{}, engines, "automation"); err == nil {
				t.Fatal("automation used an external runtime")
			}
		})
	}
}
