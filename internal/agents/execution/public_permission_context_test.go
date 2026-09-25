package execution

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"denova/config"
	agentconversation "denova/internal/agents/conversation"
	agentlifecycle "denova/internal/agents/lifecycle"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	agenttoolruntime "denova/internal/agents/toolruntime"
	"denova/internal/book"

	agent "github.com/alfredxw/denova/agent"
)

func TestWritingApprovalSurvivesContextFileChanges(t *testing.T) {
	for _, file := range []string{"chapter.md", "AGENTS.md"} {
		for _, choice := range []string{session.ToolApprovalAllowOnceOptionID, session.ToolApprovalDenyOptionID} {
			t.Run(file+"/"+choice, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
				defer cancel()
				workspace := t.TempDir()
				path := filepath.Join(workspace, file)
				if err := os.WriteFile(path, []byte("Original content"), 0600); err != nil {
					t.Fatal(err)
				}
				store, err := session.NewStore(t.TempDir())
				if err != nil {
					t.Fatal(err)
				}
				defer store.Close()
				sess, err := store.GetOrCreate("writing-approval")
				if err != nil {
					t.Fatal(err)
				}
				request := agentchatRequest("approval-command", "Revise the referenced chapter and inspect its quotes")
				var contextSource agent.ContextSource
				if file == "AGENTS.md" {
					contextSource, err = agentlifecycle.NewProjectInstructionsContextSource(nil, agentrun.AgentKindIDE, book.NewState(workspace))
					if err != nil {
						t.Fatal(err)
					}
				} else {
					request.References = []string{file}
				}
				var executions atomic.Int32
				tool, err := agent.InferTool("pwsh", "Inspect chapter quotes", func(context.Context, struct {
					Command string `json:"command"`
				}) (string, error) {
					executions.Add(1)
					return "inspected", nil
				})
				if err != nil {
					t.Fatal(err)
				}
				toolset, err := agent.StaticTools(agent.ToolDefinition{Tool: tool, Descriptor: agent.ToolDescriptor{
					Source: agent.ToolSourceShell, Execution: agent.ToolExecutionParallelRead,
					MutationScope: agent.ToolMutationNone, PostCheck: agent.ToolPostCheckNone,
					Recovery: agent.ToolRecoveryReadOnly, ResultProjection: agent.ToolResultBoundedModelContext,
					ResultRetention: agent.ToolResultDeferred, Steering: agent.SteeringFinishCurrent, MaxResultBytes: 64 << 10,
				}})
				if err != nil {
					t.Fatal(err)
				}
				policy, err := agentlifecycle.NewPermissionPolicy(agentlifecycle.PermissionConfig{
					Mode: config.AgentApprovalWrite, AgentKind: agentrun.AgentKindIDE,
					ProjectID: "project", Workspace: workspace, GOOS: "windows",
				})
				if err != nil {
					t.Fatal(err)
				}
				model := &publicBackendTestModel{}
				for index := 0; index < 2; index++ {
					model.responses = append(model.responses, agent.AssistantMessage("", []agent.ToolCall{{
						ID: fmt.Sprintf("call-%d", index), Type: "function", Function: agent.FunctionCall{
							Name: "pwsh", Arguments: `{"command":"$f='chapter.md'; $t=Get-Content -Raw -Encoding UTF8 $f; ([regex]::Matches($t,[string][char]0x201C)).Count"}`,
						},
					}}))
				}
				model.responses = append(model.responses, agent.AssistantMessage("finished", nil))
				runtime, err := NewAgentRuntime(ctx, t.TempDir(), WithToolMutationApplier(
					func(context.Context, agenttoolruntime.CommittedToolMutation) error { return nil },
				))
				if err != nil {
					t.Fatal(err)
				}
				defer runtime.Close(context.Background())
				options := agentrun.Options{AgentKind: agentrun.AgentKindIDE, Mode: "ide",
					ProjectID: "project", SessionID: sess.ID, Workspace: workspace}
				pending := make(chan string, 2)
				operation, err := runtime.Start(ctx, StartRequest{Cycle: Cycle{
					Definition: agent.Definition{Key: "writing-approval", Name: "writer", Model: model,
						ModelIdentity: agent.CapabilityIdentity{Kind: "test.approval", Version: 1},
						Tools:         toolset, Context: contextSource, Permission: policy},
					Conversation: agentconversation.NewSessionConversationForAgent(sess, nil, agentrun.AgentKindIDE),
					Request:      request, BookService: book.NewService(workspace), Options: options,
				}, Emit: func(event agentrun.Event) {
					if event.Type == "ask_pending" {
						data := event.Data.(map[string]any)
						if data["approval"].(map[string]any)["rule_id"] != "pwsh_dynamic_syntax" {
							t.Errorf("unexpected approval: %#v", data)
						}
						pending <- data["id"].(string)
					}
				}})
				if err != nil {
					t.Fatal(err)
				}
				// Observation delivers the product approval events while the Agent waits.
				outcomes := make(chan agentrun.Outcome, 1)
				observed := make(chan struct{})
				go func() {
					defer close(observed)
					defer func() {
						if recovered := recover(); recovered != nil {
							err := fmt.Errorf("approval observation panic: %v", recovered)
							outcomes <- agentrun.NewOutcome(agentrun.OutcomeFailed, err, err.Error(), "", "")
						}
					}()
					outcomes <- operation.Wait(ctx)
				}()
				defer func() {
					cancel()
					<-observed
				}()
				for index := 0; index < 2; index++ {
					var id string
					select {
					case id = <-pending:
					case outcome := <-outcomes:
						t.Fatalf("ended before approval %d: %#v", index, outcome)
					case <-ctx.Done():
						t.Fatal(ctx.Err())
					}
					option := session.ToolApprovalAllowOnceOptionID
					if index == 1 {
						if err := os.WriteFile(path, []byte("Revised content"), 0600); err != nil {
							t.Fatal(err)
						}
						option = choice
					}
					if got := executions.Load(); got != int32(index) {
						t.Fatalf("executions before approval=%d, want %d", got, index)
					}
					for attempt := 0; attempt < 2; attempt++ {
						result, err := runtime.ResolveAsk(ctx, options, id, session.AskAnswered, []agentconversation.HostAskAnswer{{
							QuestionID: "tool-approval", SelectedOptionIDs: []string{option},
						}}, "")
						if err != nil || result.Status != session.AskAnswered {
							t.Fatalf("approval %d attempt %d: result=%#v error=%v", index, attempt, result, err)
						}
					}
				}
				outcome := <-outcomes
				wantExecutions := int32(2)
				if choice == session.ToolApprovalDenyOptionID {
					wantExecutions = 1
				}
				if outcome.Status != agentrun.OutcomeCompleted || executions.Load() != wantExecutions {
					t.Fatalf("outcome=%#v executions=%d want=%d", outcome, executions.Load(), wantExecutions)
				}
			})
		}
	}
}
