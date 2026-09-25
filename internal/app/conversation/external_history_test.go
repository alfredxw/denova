package conversationapp

import (
	"context"
	"encoding/json"
	"fmt"
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
	"denova/internal/agents/runtime/external"
	"denova/internal/agents/session"
	"denova/internal/book"
	projectdomain "denova/internal/project"
	agent "github.com/alfredxw/denova/agent"
)

func TestExternalPreparationDefersCanonicalHistory(t *testing.T) {
	for _, kind := range []string{config.AgentKindIDE, config.AgentKindGeneral} {
		for _, engine := range []config.RuntimeID{config.RuntimeCodex, config.RuntimeClaude} {
			t.Run(fmt.Sprintf("%s/%s", kind, engine), func(t *testing.T) {
				root := t.TempDir()
				workspace := filepath.Join(root, "project")
				if err := os.MkdirAll(workspace, 0700); err != nil {
					t.Fatal(err)
				}
				selection := config.RuntimeSelection{Kind: engine, Codex: &config.CodexRuntimeSettings{Model: "fixture"}}
				if engine == config.RuntimeClaude {
					selection.Codex, selection.Claude = nil, &config.ClaudeRuntimeSettings{Model: "fixture"}
				}
				cfg := config.Config{Workspace: workspace, ProjectID: "project", ProjectStoreDir: filepath.Join(root, "store"), DenovaDir: root, ActiveAgentRuntime: &selection}
				store, err := session.NewStore(filepath.Join(cfg.ProjectStoreDir, "sessions"))
				if err != nil {
					t.Fatal(err)
				}
				defer store.Close()
				sess, err := store.GetOrCreateWithRuntimeConfig("history", conversationconfig.Config{AgentKind: kind, Runtime: &selection})
				if err != nil {
					t.Fatal(err)
				}
				if err := sess.Append(agent.UserMessage("Keep the original ending.")); err != nil {
					t.Fatal(err)
				}
				runtime := Runtime{ProjectID: cfg.ProjectID, ProjectStore: cfg.ProjectStoreDir, ProjectType: projectdomain.TypeGeneral, AgentKind: kind, Session: sess, Config: cfg, Workspace: workspace, BookService: book.NewService(workspace)}
				if kind == config.AgentKindIDE {
					runtime.ProjectType, runtime.State = projectdomain.TypeBook, book.NewState(workspace)
					if err := runtime.State.InitWorkspace(); err != nil {
						t.Fatal(err)
					}
				}
				engines := agentruntime.NewEngines()
				defer engines.Close()
				built, err := BuildExecution(t.Context(), runtime, agents.AgentHostCapabilities{}, engines, "")
				if err != nil {
					t.Fatal(err)
				}
				options := agentrun.Options{ProjectID: cfg.ProjectID, SessionID: sess.ID, AgentKind: kind, Mode: "agent_chat"}
				if err := os.WriteFile(filepath.Join(workspace, "reference.txt"), []byte("The ending stays at the harbor."), 0600); err != nil {
					t.Fatal(err)
				}
				cacheRoot := t.TempDir()
				for turn := range 3 {
					request := agentchat.ChatRequest{CommandID: fmt.Sprintf("continue-%d", turn), Message: fmt.Sprintf("Continue scene %d.", turn)}
					prepared, err := prepareExternal(t.Context(), runtime, request, ProjectConversation(runtime, request), *built.external, options, nil)
					if err != nil {
						t.Fatal(err)
					}
					if len(prepared.Input.History) != 0 {
						t.Fatalf("admission eagerly loaded %d historical messages before checking the provider binding", len(prepared.Input.History))
					}
					loads, attempts := 0, 0
					load := prepared.LoadHistory
					prepared.LoadHistory = func(ctx context.Context) ([]external.Message, error) {
						loads++
						if turn == 1 {
							t.Fatal("aligned provider continuation read canonical history")
						}
						return load(ctx)
					}
					adapter := recoveryAdapter(func(ctx context.Context, input external.Input, host external.Host) (external.Result, error) {
						attempts++
						if input.SessionID != "" {
							if len(input.History) != 0 || loads != 0 {
								t.Fatal("resumed provider received reconstructed history")
							}
							if turn == 2 {
								return external.Result{}, external.ErrSessionUnavailable
							}
						} else {
							if loads != 1 || len(input.History) == 0 || input.History[0].Text != "Keep the original ending." {
								t.Fatalf("cold reconstruction lost original history: loads=%d messages=%+v", loads, input.History)
							}
							toolFound := false
							for _, message := range input.History {
								if strings.Contains(message.Text, request.Message) {
									t.Fatal("reconstruction duplicated the newly admitted input")
								}
								toolFound = toolFound || strings.Contains(message.Text, "The ending stays at the harbor.")
							}
							if turn == 2 && (!toolFound || len(input.History) != 6) {
								t.Fatalf("fallback lost canonical messages or tool observations: %+v", input.History)
							}
						}
						if turn == 0 {
							result, err := host.CallTool(ctx, external.ToolCall{ID: "reference", Name: "read", Arguments: json.RawMessage(`{"path":"reference.txt"}`)})
							if err != nil || !result.Success {
								t.Fatalf("read reference: %+v, %v", result, err)
							}
						}
						return external.Result{SessionID: "provider-session", Text: fmt.Sprintf("Scene %d complete.", turn)}, nil
					})
					prepared.Runtime = &external.Runtime{Selection: selection, CacheRoot: cacheRoot,
						Acquire: func(context.Context) (external.Adapter, func(), error) { return adapter, func() {}, nil }}
					operation, err := engines.Operations.Start(t.Context(), prepared)
					if err != nil {
						t.Fatal(err)
					}
					if outcome := operation.Wait(t.Context()); outcome.Status != agentrun.OutcomeCompleted {
						t.Fatalf("turn %d failed: %+v", turn, outcome)
					}
					wantAttempts, wantLoads := 1, 1
					if turn == 1 {
						wantLoads = 0
					} else if turn == 2 {
						wantAttempts = 2
					}
					if attempts != wantAttempts || loads != wantLoads {
						t.Fatalf("turn %d: provider attempts=%d history loads=%d", turn, attempts, loads)
					}
				}
			})
		}
	}
}
