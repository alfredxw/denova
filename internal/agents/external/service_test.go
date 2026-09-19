package external

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"denova/config"
	"denova/internal/agents/conversation"
	"denova/internal/agents/conversationconfig"
	externaljournal "denova/internal/agents/external/journal"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	"denova/internal/agents/toolruntime"
	workspacechange "denova/internal/workspace/change"
	agent "github.com/alfredxw/denova/agent"
	publictools "github.com/alfredxw/denova/agent/tools"
)

type adapterFunc func(context.Context, Input, Host) (Result, error)

func (adapterFunc) Version() string { return "test-engine-v1" }

func (fn adapterFunc) Run(ctx context.Context, input Input, host Host) (Result, error) {
	return fn(ctx, input, host)
}

type interruptedWrite struct {
	agent.Tool
	cancel      context.CancelFunc
	loseReceipt bool
}

func (tool interruptedWrite) Run(ctx context.Context, arguments string, options ...agent.ToolOption) (agent.ToolResult, error) {
	result, err := tool.Tool.Run(ctx, arguments, options...)
	if err != nil {
		return result, err
	}
	tool.cancel()
	if tool.loseReceipt {
		return agent.ToolResult{}, context.Canceled
	}
	return result, context.Canceled
}

func TestExternalOperationPreservesCommittedReceiptAndBlocksUnknownEffect(t *testing.T) {
	for _, loseReceipt := range []bool{false, true} {
		t.Run(fmt.Sprintf("lose_receipt_%t", loseReceipt), func(t *testing.T) {
			service, request, workspace := operationFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			for index, definition := range request.Definitions {
				info, err := definition.Tool.Info(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if info.Name == "write" {
					request.Definitions[index].Tool = interruptedWrite{Tool: definition.Tool, cancel: cancel, loseReceipt: loseReceipt}
				}
			}
			request.Adapter = adapterFunc(func(ctx context.Context, _ Input, host Host) (Result, error) {
				_, err := host.CallTool(ctx, ToolCall{ID: "write-before-cancel", Name: "write", Arguments: json.RawMessage(`{"path":"committed.md","content":"Keep this write."}`)})
				return Result{}, err
			})
			operation, err := service.Start(ctx, request)
			if err != nil {
				t.Fatal(err)
			}
			outcome := operation.Wait(ctx)
			want := agentrun.OutcomeAborted
			if loseReceipt {
				want = agentrun.OutcomeSuspended
			}
			if outcome.Status != want {
				t.Fatalf("outcome=%#v", outcome)
			}
			content, err := os.ReadFile(filepath.Join(workspace, "committed.md"))
			if err != nil || string(content) != "Keep this write." {
				t.Fatalf("committed content=%q err=%v", content, err)
			}
			if err := request.Session.ReadExternal(context.Background(), func(state session.ExternalState) error {
				current := state.Projection.Operations[operation.id]
				for _, tool := range current.Tools {
					if (tool.Finished == nil) != loseReceipt {
						return fmt.Errorf("unexpected tool settlement: %#v", tool)
					}
					if tool.Finished != nil {
						source, err := state.Read(*tool.Finished)
						if err != nil {
							return err
						}
						if !strings.Contains(string(source.Data), "committed.md") {
							return errors.New("lost committed receipt")
						}
					}
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if loseReceipt {
				restarted := &Service{}
				if err := restarted.Recover(context.Background(), request.ProjectID, request.Session); err != nil {
					t.Fatal(err)
				}
				request.CommandID, request.Metadata.MessageID = "new-command", "new-input"
				if err := request.Session.ReadExternal(context.Background(), func(state session.ExternalState) error { request.PreparedCursor = state.Cursor; return nil }); err != nil {
					t.Fatal(err)
				}
				if _, err := restarted.Start(context.Background(), request); !errors.Is(err, externaljournal.ErrBusy) {
					t.Fatalf("unknown effect allowed a new attempt: %v", err)
				}
				changes, err := workspacechange.ForWorkspace(workspace)
				if err != nil {
					t.Fatal(err)
				}
				// Reopening the domain ledger simulates losing the process cache.
				stateRoot := changes.StateRoot()
				if err := workspacechange.ForgetWorkspace(workspace); err != nil {
					t.Fatal(err)
				}
				if err := Reconcile(t.Context(), request.Session, workspace, stateRoot); err != nil {
					t.Fatal(err)
				}
				if err := Reconcile(t.Context(), request.Session, workspace, stateRoot); err != nil {
					t.Fatal(err)
				}
				if err := request.Session.ReadExternal(t.Context(), func(state session.ExternalState) error {
					if err := state.Projection.RequireIdle(); err != nil {
						return err
					}
					for _, tool := range state.Projection.Operations[operation.id].Tools {
						record, err := state.Read(*tool.Finished)
						if err != nil {
							return err
						}
						var finished externaljournal.FinishedTool
						if err := json.Unmarshal(record.Data, &finished); err != nil {
							return err
						}
						if !finished.Success || !strings.Contains(string(record.Data), toolruntime.AgentToolMutationEffectKind) || strings.Contains(string(record.Data), strings.ReplaceAll(workspace, `\`, `\\`)) {
							return fmt.Errorf("invalid recovered receipt: %s", record.Data)
						}
					}
					return nil
				}); err != nil {
					t.Fatal(err)
				}
				changes, err = workspacechange.ForWorkspaceAt(workspace, stateRoot)
				if err != nil {
					t.Fatal(err)
				}
				group, err := changes.GetGroup(t.Context(), operation.id)
				if err != nil || len(group.ChangeSets) != 1 {
					t.Fatalf("recovery replayed a write: %#v, %v", group, err)
				}
			}
		})
	}
}

func TestExternalRecoverExcludesLiveAttemptsAndPreservesPendingAsk(t *testing.T) {
	_, _, sess := pendingAskFixture(t, `{"questions":[{"id":"tone","prompt":"Which tone?"}]}`)
	restarted := &Service{}
	if err := restarted.Recover(context.Background(), "project-1", sess); err != nil {
		t.Fatal(err)
	}
	questions, err := sess.PendingExternalAsks(context.Background())
	if err != nil || len(questions) != 1 || questions[0].AgentOperationID != "operation-1" {
		t.Fatalf("pending questions=%#v err=%v", questions, err)
	}
	if _, err := restarted.Interactions.Resolve(context.Background(), "project-1", sess, questions[0].ID, []conversation.HostAskAnswer{{QuestionID: "tone", CustomInput: "Restrained"}}, nil); err != nil {
		t.Fatal(err)
	}
	page, err := sess.ReadHistoryPage(context.Background(), -1, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Entries) != 2 || page.Entries[1].Ask == nil || page.Entries[1].Ask.Status != session.AskAnswered || page.Entries[1].Ask.Answers[0].CustomInput != "Restrained" {
		t.Fatalf("recovered page=%#v", page)
	}
	service, request, _ := operationFixture(t)
	request.Adapter = adapterFunc(func(context.Context, Input, Host) (Result, error) { return Result{Text: "done"}, nil })
	operation, err := service.Start(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Recover(context.Background(), request.ProjectID, request.Session); err != nil {
		t.Fatal(err)
	}
	if outcome := operation.Wait(context.Background()); outcome.Status != agentrun.OutcomeCompleted {
		t.Fatalf("refresh interrupted a live operation: %#v", outcome)
	}
}

func operationFixture(t *testing.T) (*Service, StartRequest, string) {
	t.Helper()
	workspace, directory := t.TempDir(), t.TempDir()
	store, err := session.NewStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	selection := conversationconfig.Config{AgentKind: config.AgentKindGeneral, ProfileID: "removed-native-model", Runtime: &config.RuntimeSelection{Kind: config.RuntimeCodex, Codex: &config.CodexRuntimeSettings{Model: "fixture-model"}}}
	sess, err := store.GetOrCreateWithRuntimeConfig("operation", selection)
	if err != nil {
		t.Fatal(err)
	}
	settings := config.ResolvedAgentToolSettings{config.AgentToolFilesystemRead: true, config.AgentToolWorkspaceWrite: true, config.AgentToolAsk: true}
	definitions, err := toolruntime.NewCatalog(&config.Config{Workspace: workspace, ProjectStoreDir: directory}).Workspace(settings)
	if err != nil {
		t.Fatal(err)
	}
	asks, err := publictools.Ask().PrepareTools(context.Background(), agent.ToolRequest{})
	if err != nil {
		t.Fatal(err)
	}
	request := StartRequest{
		ProjectID: "project-fixture", AttachmentRoot: directory, Session: sess, CommandID: "command-fixture", Fingerprint: "fixture-v1", Revision: 1,
		Input:   Input{Selection: selection.Engine(), Text: "Draft an opening."},
		Message: *agent.UserMessage("Draft an opening."), Metadata: session.MessageMetadata{MessageID: "input-fixture"},
		Definitions: append(definitions, asks...), ToolPolicy: toolruntime.OrchestratorConfig{AgentKind: config.AgentKindGeneral, Workspace: workspace, ToolSettings: settings, EnforceToolSettings: true, ToolResultMaxBytes: 32768},
		ReviewThreadID: "review-fixture",
	}
	if err := sess.ReadExternal(context.Background(), func(state session.ExternalState) error { request.PreparedCursor = state.Cursor; return nil }); err != nil {
		t.Fatal(err)
	}
	return &Service{}, request, workspace
}

func TestExternalOperationUsesDomainWriteReceiptsAndCanonicalCompletion(t *testing.T) {
	service, request, workspace := operationFixture(t)
	var attempts int
	request.Adapter = adapterFunc(func(ctx context.Context, input Input, host Host) (Result, error) {
		attempts++
		call := ToolCall{ID: "provider-write", Name: "write", Arguments: json.RawMessage(`{"path":"opening.md","content":"The harbor was still."}`)}
		first, err := host.CallTool(ctx, call)
		if err != nil || !first.Success {
			return Result{}, fmt.Errorf("write failed: %#v: %w", first, err)
		}
		repeated, err := host.CallTool(ctx, call)
		if err != nil || !reflect.DeepEqual(first, repeated) {
			return Result{}, fmt.Errorf("duplicate call changed result: %v", err)
		}
		if err := host.Emit(agentrun.Event{Type: "chunk", Data: map[string]any{"content": "Draft saved."}}); err != nil {
			return Result{}, err
		}
		return Result{Text: "Draft saved."}, nil
	})
	operation, err := service.Start(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if messages := request.Session.GetMessages(); len(messages) != 1 || messages[0].Content != request.Message.Content {
		t.Fatalf("acceptance did not publish input: %#v", messages)
	}
	same, err := service.Start(context.Background(), request)
	if err != nil || same != operation {
		t.Fatalf("live duplicate was not reused: %v", err)
	}
	if outcome := operation.Wait(context.Background()); outcome.Status != agentrun.OutcomeCompleted {
		t.Fatalf("outcome=%#v", outcome)
	}
	content, err := os.ReadFile(filepath.Join(workspace, "opening.md"))
	if err != nil || string(content) != "The harbor was still." {
		t.Fatalf("content=%q err=%v", content, err)
	}
	err = request.Session.ReadExternal(context.Background(), func(state session.ExternalState) error {
		current := state.Projection.Operations[operation.id]
		if current.Status != externaljournal.Completed || len(current.Tools) != 1 {
			return fmt.Errorf("operation projection=%#v", current)
		}
		for executionID, tool := range current.Tools {
			if tool.Finished == nil {
				return errors.New("tool returned before settlement")
			}
			source, err := state.Read(*tool.Finished)
			if err != nil {
				return err
			}
			var finished externaljournal.FinishedTool
			if err := json.Unmarshal(source.Data, &finished); err != nil {
				return err
			}
			receipt := finished.Receipt
			if receipt == nil {
				return errors.New("tool receipt is missing")
			}
			parsed, ok := workspacechange.ParseToolReceipt("write", string(receipt.Details))
			if !ok || parsed.ChangeGroupID != operation.id || parsed.ReviewThreadID != request.ReviewThreadID || len(receipt.Effects) != 1 {
				return fmt.Errorf("domain identity was lost: %#v", receipt)
			}
			mutation, err := toolruntime.DecodeAgentToolMutationEffect(receipt.Effects[0])
			if err != nil || mutation.ToolCallID != executionID {
				return fmt.Errorf("execution identity was lost: %#v: %v", mutation, err)
			}
			if strings.Contains(string(source.Data), strings.ReplaceAll(workspace, `\`, `\\`)) {
				return errors.New("receipt persisted an absolute workspace")
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	messages := request.Session.GetMessages()
	if len(messages) != 2 || messages[1].Role != agent.Assistant || messages[1].Content != "Draft saved." {
		t.Fatalf("completion messages=%#v", messages)
	}
	if _, err := operation.CallTool(context.Background(), ToolCall{ID: "late", Name: "write", Arguments: json.RawMessage(`{"path":"late.md","content":"invalid"}`)}); err == nil {
		t.Fatal("late callback was accepted")
	}
	replayed, err := service.Start(context.Background(), request)
	if err != nil || !replayed.Replayed() || replayed.Receipt() != operation.Receipt() {
		t.Fatalf("completed replay changed receipt: %v", err)
	}
	if replayed.Wait(context.Background()).Status != agentrun.OutcomeCompleted || attempts != 1 {
		t.Fatal("completed command reran its adapter")
	}
	request.Fingerprint = "different"
	if _, err := service.Start(context.Background(), request); !errors.Is(err, agentrun.ErrInvalidCommand) {
		t.Fatalf("different input reused a command: %v", err)
	}
}

func TestExternalOperationAnswerAndStopSettleCanonicalQuestion(t *testing.T) {
	for _, action := range []string{"answer", "stop"} {
		t.Run(action, func(t *testing.T) {
			service, request, _ := operationFixture(t)
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			pending := make(chan *session.AskInteraction, 1)
			request.Emit = func(event agentrun.Event) {
				if event.Type == "ask_pending" {
					pending <- event.Data.(*session.AskInteraction)
				}
			}
			request.Adapter = adapterFunc(func(ctx context.Context, _ Input, host Host) (Result, error) {
				answer, err := host.CallTool(ctx, ToolCall{ID: "ask-call", Name: "ask", Arguments: json.RawMessage(`{"questions":[{"id":"tone","prompt":"Which tone?"}]}`)})
				if err != nil {
					return Result{}, err
				}
				if !answer.Success || !strings.Contains(answer.Text, "Restrained") {
					return Result{}, fmt.Errorf("invalid answer: %#v", answer)
				}
				return Result{Text: "A restrained opening."}, nil
			})
			operation, err := service.Start(ctx, request)
			if err != nil {
				t.Fatal(err)
			}
			completed := make(chan agentrun.Outcome, 1)
			go func() {
				defer func() {
					if value := recover(); value != nil {
						completed <- agentrun.NewOutcome(agentrun.OutcomeFailed, fmt.Errorf("test worker panic: %v", value), "", "", "")
					}
				}()
				completed <- operation.Wait(ctx)
			}()
			var ask *session.AskInteraction
			select {
			case ask = <-pending:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			if action == "answer" {
				_, err := service.Interactions.Resolve(ctx, request.ProjectID, request.Session, ask.ID, []conversation.HostAskAnswer{{QuestionID: "tone", CustomInput: "Restrained"}}, nil)
				if err != nil {
					t.Fatal(err)
				}
			} else {
				cancel()
			}
			select {
			case result := <-completed:
				want := agentrun.OutcomeCompleted
				if action == "stop" {
					want = agentrun.OutcomeAborted
				}
				if result.Status != want {
					t.Fatalf("outcome=%#v", result)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("operation did not finish")
			}
			if err := request.Session.ReadExternal(context.Background(), func(state session.ExternalState) error { return state.Projection.RequireIdle() }); err != nil {
				t.Fatal(err)
			}
			if action == "stop" {
				_, err := service.Interactions.Resolve(context.Background(), request.ProjectID, request.Session, ask.ID, []conversation.HostAskAnswer{{QuestionID: "tone", CustomInput: "Late answer"}}, nil)
				if !errors.Is(err, ErrAskConflict) {
					t.Fatalf("stop did not settle Ask: %v", err)
				}
			}
		})
	}
}
