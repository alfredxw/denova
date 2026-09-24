package agentruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"denova/config"
	"denova/internal/agents/conversationconfig"
	"denova/internal/agents/execution"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/runtime/external"
	externaljournal "denova/internal/agents/runtime/external/journal"
	"denova/internal/agents/session"
	"denova/internal/agents/toolruntime"
	agent "github.com/alfredxw/denova/agent"
	publictools "github.com/alfredxw/denova/agent/tools"
)

type cancellationAdapter func(context.Context, external.Input, external.Host) (external.Result, error)

func (cancellationAdapter) Version() string { return "cancellation-test" }
func (adapter cancellationAdapter) Run(ctx context.Context, input external.Input, host external.Host) (external.Result, error) {
	return adapter(ctx, input, host)
}

type countedHostTool struct {
	agent.Tool
	calls *atomic.Int32
}

func (tool countedHostTool) Run(ctx context.Context, args string, opts ...agent.ToolOption) (agent.ToolResult, error) {
	tool.calls.Add(1)
	return tool.Tool.Run(ctx, args, opts...)
}

func TestQueuedHostWriteCancellationSurvivesReopenAndEngineSwitch(t *testing.T) {
	for _, kind := range []string{config.AgentKindIDE, config.AgentKindGeneral} {
		for _, engine := range []config.RuntimeID{config.RuntimeCodex, config.RuntimeClaude} {
			t.Run(kind+"/"+string(engine), func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					workspace, directory := t.TempDir(), t.TempDir()
					store, err := session.NewStore(directory)
					if err != nil {
						t.Fatal(err)
					}
					defer store.Close()
					selection := conversationconfig.Config{AgentKind: kind, ProfileID: "default", ApprovalMode: config.AgentApprovalWrite, Runtime: &config.RuntimeSelection{Kind: engine}}
					if engine == config.RuntimeCodex {
						selection.Runtime.Codex = &config.CodexRuntimeSettings{Model: "test", Sandbox: config.CodexWorkspaceWrite}
					} else {
						selection.Runtime.Claude = &config.ClaudeRuntimeSettings{Model: "test"}
					}
					sess, err := store.GetOrCreateWithRuntimeConfig("queued-write", selection)
					if err != nil {
						t.Fatal(err)
					}
					settings := config.ResolvedAgentToolSettings{config.AgentToolWorkspaceWrite: true, config.AgentToolAsk: true}
					definitions, err := toolruntime.NewCatalog(&config.Config{Workspace: workspace, ProjectStoreDir: directory}).Workspace(settings)
					if err != nil {
						t.Fatal(err)
					}
					var write agent.ToolDefinition
					var calls atomic.Int32
					for i, definition := range definitions {
						info, err := definition.Tool.Info(t.Context())
						if err != nil {
							t.Fatal(err)
						}
						if info.Name == "write" {
							write = definition
							definitions[i].Tool = countedHostTool{Tool: definition.Tool, calls: &calls}
						}
					}
					if write.Tool == nil {
						t.Fatal("fixture has no write tool")
					}
					asks, err := publictools.Ask().PrepareTools(t.Context(), agent.ToolRequest{})
					if err != nil {
						t.Fatal(err)
					}
					policy := toolruntime.OrchestratorConfig{AgentKind: kind, Workspace: workspace, ToolSettings: settings, EnforceToolSettings: true, ToolResultMaxBytes: 32768}
					info, err := write.Tool.Info(t.Context())
					if err != nil {
						t.Fatal(err)
					}
					// Hold the real shared workspace gate until cancellation has settled.
					entered, release := make(chan struct{}), make(chan struct{})
					blocker, err := toolruntime.NewOrchestratorMiddleware(policy).WrapToolCall(t.Context(), func(context.Context, string, ...agent.ToolOption) (agent.ToolResult, error) {
						close(entered)
						<-release
						return agent.TextToolResult("released"), nil
					}, &agent.ToolContext{Name: "write", Definition: agent.ToolDefinitionSnapshot{Info: info, Descriptor: write.Descriptor}})
					if err != nil {
						t.Fatal(err)
					}
					blockerDone := make(chan error, 1)
					go func() {
						defer func() {
							if value := recover(); value != nil {
								blockerDone <- fmt.Errorf("blocker panic: %v", value)
							}
						}()
						_, err := blocker(t.Context(), `{"path":"other.md","content":"other"}`)
						blockerDone <- err
					}()
					defer func() {
						close(release)
						if err := <-blockerDone; err != nil {
							t.Error(err)
						}
					}()
					<-entered
					attempts := 0
					request := external.StartRequest{
						ProjectID: "project", AttachmentRoot: directory, Session: sess, CommandID: "cancel-write", Fingerprint: "cancel-write", Revision: 1,
						Input: external.Input{Selection: selection.Engine(), Text: "Write a chapter."}, Message: *agent.UserMessage("Write a chapter."),
						Metadata: session.MessageMetadata{MessageID: "cancel-input"}, Definitions: append(definitions, asks...), ToolPolicy: policy,
						Adapter: cancellationAdapter(func(ctx context.Context, _ external.Input, host external.Host) (external.Result, error) {
							attempts++
							_, err := host.CallTool(ctx, external.ToolCall{ID: "write", Name: "write", Arguments: json.RawMessage(`{"path":"chapter.md","content":"Do not write this."}`)})
							return external.Result{}, err
						}),
					}
					history, err := external.ReadHistory(t.Context(), sess)
					if err != nil {
						t.Fatal(err)
					}
					request.PreparedCursor = history.Cursor
					service := &external.Service{}
					ctx, cancel := context.WithCancel(t.Context())
					defer cancel()
					operation, err := service.Start(ctx, request)
					if err != nil {
						t.Fatal(err)
					}
					done := make(chan agentrun.Outcome, 1)
					go func() {
						defer func() {
							if value := recover(); value != nil {
								done <- agentrun.NewOutcome(agentrun.OutcomeFailed, fmt.Errorf("operation panic: %v", value), "", "", "")
							}
						}()
						done <- operation.Wait(ctx)
					}()
					synctest.Wait()
					if err := sess.ReadExternal(t.Context(), func(state session.ExternalState) error {
						current := state.Projection.Operations[string(operation.Receipt().OperationID)]
						if current == nil || len(current.Tools) != 1 || current.Status != externaljournal.Running {
							return fmt.Errorf("write was not durably admitted: %+v", current)
						}
						return nil
					}); err != nil {
						t.Fatal(err)
					}
					cancel()
					synctest.Wait()
					if outcome := <-done; outcome.Status != agentrun.OutcomeAborted {
						t.Fatalf("queued cancellation left the operation unresolved: %+v", outcome)
					}
					if calls.Load() != 0 {
						t.Fatal("cancelled tool entered Run")
					}
					if _, err := os.Stat(filepath.Join(workspace, "chapter.md")); !errors.Is(err, os.ErrNotExist) {
						t.Fatalf("cancelled write created a file: %v", err)
					}
					if err := store.Close(); err != nil {
						t.Fatal(err)
					}
					indexes, err := filepath.Glob(filepath.Join(directory, "*.idx.json"))
					if err != nil {
						t.Fatal(err)
					}
					for _, path := range indexes {
						if err := os.Remove(path); err != nil {
							t.Fatal(err)
						}
					}
					reopened, err := session.NewStore(directory)
					if err != nil {
						t.Fatal(err)
					}
					defer reopened.Close()
					request.Session, err = reopened.Get(sess.ID)
					if err != nil {
						t.Fatal(err)
					}
					engines := NewEngines()
					defer engines.Close()
					if err := engines.Operations.Recover(t.Context(), request.ProjectID, request.Session); err != nil {
						t.Fatal(err)
					}
					if err := request.Session.ReadExternal(t.Context(), func(state session.ExternalState) error {
						if err := state.Projection.RequireIdle(); err != nil {
							return err
						}
						for _, tool := range state.Projection.Operations[string(operation.Receipt().OperationID)].Tools {
							record, err := state.Read(*tool.Finished)
							if err != nil {
								return err
							}
							var finished externaljournal.FinishedTool
							if err := json.Unmarshal(record.Data, &finished); err != nil {
								return err
							}
							if finished.Success || !strings.Contains(finished.Result, "did not start") {
								return fmt.Errorf("lost nonexecution result: %+v", finished)
							}
						}
						return nil
					}); err != nil {
						t.Fatal(err)
					}
					replayed, err := engines.Operations.Start(t.Context(), request)
					if err != nil || !replayed.Replayed() || replayed.Receipt() != operation.Receipt() {
						t.Fatalf("cancelled command lost its receipt: %v", err)
					}
					if outcome := replayed.Wait(t.Context()); outcome.Status != agentrun.OutcomeAborted || attempts != 1 {
						t.Fatalf("cancelled command was rerun: %+v attempts=%d", outcome, attempts)
					}
					history, err = external.ReadHistory(t.Context(), request.Session)
					if err != nil {
						t.Fatal(err)
					}
					request.PreparedCursor = history.Cursor
					request.CommandID, request.Fingerprint, request.Metadata.MessageID = "continue", "continue", "continue-input"
					request.Adapter = cancellationAdapter(func(context.Context, external.Input, external.Host) (external.Result, error) {
						return external.Result{Text: "Continued safely."}, nil
					})
					continued, err := engines.Operations.Start(t.Context(), request)
					if err != nil {
						t.Fatal(err)
					}
					if outcome := continued.Wait(t.Context()); outcome.Status != agentrun.OutcomeCompleted {
						t.Fatalf("continuation failed: %+v", outcome)
					}
					native := execution.NewEphemeralRuntime()
					defer native.Close(context.Background())
					selection.Runtime = nil
					options := agentrun.Options{AgentKind: kind, ProjectID: request.ProjectID, Workspace: workspace, SessionID: sess.ID, Mode: "agent_chat"}
					if _, err := engines.ApplyEngineSelection(t.Context(), native, request.Session, options, selection, 1, config.Config{}); err != nil {
						t.Fatalf("cancelled write blocked engine switching: %v", err)
					}
				})
			})
		}
	}
}
